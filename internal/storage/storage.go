// Copyright 2025 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package storage

import (
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/canonical/tenant-service/internal/db"
	"github.com/canonical/tenant-service/internal/logging"
	"github.com/canonical/tenant-service/internal/monitoring"
	"github.com/canonical/tenant-service/internal/tracing"
	"github.com/canonical/tenant-service/internal/types"
	"github.com/google/uuid"
)

var _ StorageInterface = (*Storage)(nil)

type Storage struct {
	db db.DBClientInterface

	logger  logging.LoggerInterface
	tracer  tracing.TracingInterface
	monitor monitoring.MonitorInterface
}

func NewStorage(c db.DBClientInterface, tracer tracing.TracingInterface, monitor monitoring.MonitorInterface, logger logging.LoggerInterface) *Storage {
	s := new(Storage)

	s.db = c

	s.logger = logger
	s.tracer = tracer
	s.monitor = monitor

	return s
}

func (s *Storage) recordLatencyFor(operation string, start time.Time, err error) {
	status := "success"
	if err != nil {
		status = "error"
	}
	s.monitor.SetStorageResponseTimeMetric(map[string]string{
		"operation": operation,
		"status":    status,
	}, time.Since(start).Seconds())
}

func (s *Storage) CreateTenant(ctx context.Context, t *types.Tenant) (tenant *types.Tenant, err error) {
	defer func(start time.Time) { s.recordLatencyFor("CreateTenant", start, err) }(time.Now())
	ctx, span := s.tracer.Start(ctx, "storage.CreateTenant")
	defer span.End()

	id, err := uuid.NewV7()
	if err != nil {
		return nil, fmt.Errorf("failed to generate tenant ID: %w", err)
	}

	var newTenant types.Tenant
	err = s.db.Statement(ctx).
		Insert("tenants").
		Columns("id", "name", "enabled").
		Values(id.String(), t.Name, t.Enabled).
		Suffix("RETURNING id, name, created_at, enabled, personal_identity_id, mfa_requirement").
		QueryRowContext(ctx).
		Scan(&newTenant.ID, &newTenant.Name, &newTenant.CreatedAt, &newTenant.Enabled, &newTenant.PersonalIdentityID, &newTenant.MFARequirement)

	if err != nil {
		return nil, fmt.Errorf("failed to insert tenant: %w", err)
	}

	return &newTenant, nil
}

// CreatePersonalTenant creates userID's personal tenant. When userID has one
// already it returns that one, and false.
func (s *Storage) CreatePersonalTenant(ctx context.Context, userID, name string) (tenant *types.Tenant, created bool, err error) {
	defer func(start time.Time) { s.recordLatencyFor("CreatePersonalTenant", start, err) }(time.Now())
	ctx, span := s.tracer.Start(ctx, "storage.CreatePersonalTenant")
	defer span.End()

	id, err := uuid.NewV7()
	if err != nil {
		return nil, false, fmt.Errorf("failed to generate tenant ID: %w", err)
	}

	var newTenant types.Tenant
	err = s.db.Statement(ctx).
		Insert("tenants").
		Columns("id", "name", "enabled", "personal_identity_id").
		Values(id.String(), name, true, userID).
		Suffix("ON CONFLICT (personal_identity_id) DO NOTHING RETURNING id, name, created_at, enabled, personal_identity_id, mfa_requirement").
		QueryRowContext(ctx).
		Scan(&newTenant.ID, &newTenant.Name, &newTenant.CreatedAt, &newTenant.Enabled, &newTenant.PersonalIdentityID, &newTenant.MFARequirement)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			existing, err := s.getTenant(ctx, sq.Eq{"personal_identity_id": userID}, false)
			if err != nil {
				return nil, false, err
			}
			return existing, false, nil
		}
		return nil, false, fmt.Errorf("failed to insert personal tenant: %w", err)
	}

	return &newTenant, true, nil
}

func (s *Storage) GetPersonalTenantByUserID(ctx context.Context, userID string) (tenant *types.Tenant, err error) {
	defer func(start time.Time) { s.recordLatencyFor("GetPersonalTenantByUserID", start, err) }(time.Now())
	ctx, span := s.tracer.Start(ctx, "storage.GetPersonalTenantByUserID")
	defer span.End()

	return s.getTenant(ctx, sq.Eq{"personal_identity_id": userID}, false)
}

func (s *Storage) GetTenantByID(ctx context.Context, id string) (tenant *types.Tenant, err error) {
	defer func(start time.Time) { s.recordLatencyFor("GetTenantByID", start, err) }(time.Now())
	ctx, span := s.tracer.Start(ctx, "storage.GetTenantByID")
	defer span.End()

	return s.getTenant(ctx, sq.Eq{"id": id}, false)
}

// getTenant returns the tenant matching where. With lock, its row is locked
// until the transaction ends as an update of the row locks it: other writers
// of the row wait, a new membership or invitation of the tenant does not.
func (s *Storage) getTenant(ctx context.Context, where sq.Eq, lock bool) (*types.Tenant, error) {
	query := s.db.Statement(ctx).
		Select("id", "name", "created_at", "enabled", "personal_identity_id", "mfa_requirement").
		From("tenants").
		Where(where)
	if lock {
		query = query.Suffix("FOR NO KEY UPDATE")
	}

	var t types.Tenant
	err := query.QueryRowContext(ctx).
		Scan(&t.ID, &t.Name, &t.CreatedAt, &t.Enabled, &t.PersonalIdentityID, &t.MFARequirement)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to get tenant: %w", err)
	}
	// Outside a transaction the lock ended with the statement.
	if lock && !db.InTx(ctx) {
		return nil, ErrNoTransaction
	}

	return &t, nil
}

func (s *Storage) ListTenants(ctx context.Context, options ...types.ListOption) (tenants []*types.Tenant, token string, err error) {
	defer func(start time.Time) { s.recordLatencyFor("ListTenants", start, err) }(time.Now())
	ctx, span := s.tracer.Start(ctx, "storage.ListTenants")
	defer span.End()

	pageSize, cursorID, opts, err := resolveListOptions(options)
	if err != nil {
		return nil, "", err
	}

	query := s.db.Statement(ctx).
		Select("id", "name", "created_at", "enabled", "personal_identity_id", "mfa_requirement").
		From("tenants").
		OrderBy("id").
		Limit(pageSize + 1)

	if cursorID != "" {
		query = query.Where(sq.Gt{"id": cursorID})
	}
	if opts.Enabled != nil {
		query = query.Where(sq.Eq{"enabled": *opts.Enabled})
	}

	rows, err := query.QueryContext(ctx)
	if err != nil {
		return nil, "", fmt.Errorf("failed to list tenants: %w", err)
	}
	defer rows.Close()

	tenants = make([]*types.Tenant, 0)
	for rows.Next() {
		var t types.Tenant
		if err := rows.Scan(&t.ID, &t.Name, &t.CreatedAt, &t.Enabled, &t.PersonalIdentityID, &t.MFARequirement); err != nil {
			return nil, "", fmt.Errorf("failed to scan tenant: %w", err)
		}
		tenants = append(tenants, &t)
	}

	if err := rows.Err(); err != nil {
		return nil, "", fmt.Errorf("error iterating tenant rows: %w", err)
	}

	var nextPageToken string
	if uint64(len(tenants)) > pageSize {
		nextPageToken = encodePageToken(tenants[pageSize-1].ID)
		tenants = tenants[:pageSize]
	}

	return tenants, nextPageToken, nil
}

// ListTenantsByUserID returns userID's personal tenant first.
func (s *Storage) ListTenantsByUserID(ctx context.Context, userID string, options ...types.ListOption) (tenants []*types.Tenant, err error) {
	defer func(start time.Time) { s.recordLatencyFor("ListTenantsByUserID", start, err) }(time.Now())
	ctx, span := s.tracer.Start(ctx, "storage.ListTenantsByUserID")
	defer span.End()

	_, _, opts, err := resolveListOptions(options)
	if err != nil {
		return nil, err
	}

	query := s.db.Statement(ctx).
		Select("t.id", "t.name", "t.created_at", "t.enabled", "t.personal_identity_id", "t.mfa_requirement").
		From("tenants t").
		Join("memberships m ON t.id = m.tenant_id").
		Where(sq.Eq{"m.kratos_identity_id": userID}).
		OrderBy("t.personal_identity_id IS NOT DISTINCT FROM m.kratos_identity_id DESC", "t.id")

	if opts.Enabled != nil {
		query = query.Where(sq.Eq{"t.enabled": *opts.Enabled})
	}

	rows, err := query.QueryContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list tenants: %w", err)
	}
	defer rows.Close()

	tenants = make([]*types.Tenant, 0)
	for rows.Next() {
		var t types.Tenant
		if err := rows.Scan(&t.ID, &t.Name, &t.CreatedAt, &t.Enabled, &t.PersonalIdentityID, &t.MFARequirement); err != nil {
			return nil, fmt.Errorf("failed to scan tenant: %w", err)
		}
		tenants = append(tenants, &t)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration error: %w", err)
	}

	return tenants, nil
}

func (s *Storage) ListMembersByTenantID(ctx context.Context, tenantID string, options ...types.ListOption) (memberships []*types.Membership, token string, err error) {
	defer func(start time.Time) { s.recordLatencyFor("ListMembersByTenantID", start, err) }(time.Now())
	ctx, span := s.tracer.Start(ctx, "storage.ListMembersByTenantID")
	defer span.End()

	pageSize, cursorID, opts, err := resolveListOptions(options)
	if err != nil {
		return nil, "", err
	}

	query := s.db.Statement(ctx).
		Select("id", "tenant_id", "kratos_identity_id", "created_at").
		From("memberships").
		Where(sq.Eq{"tenant_id": tenantID}).
		OrderBy("id").
		Limit(pageSize + 1)

	if cursorID != "" {
		query = query.Where(sq.Gt{"id": cursorID})
	}
	if opts.IdentityID != "" {
		query = query.Where(sq.Eq{"kratos_identity_id": opts.IdentityID})
	}

	rows, err := query.QueryContext(ctx)
	if err != nil {
		return nil, "", fmt.Errorf("failed to list members: %w", err)
	}
	defer rows.Close()

	members := make([]*types.Membership, 0)
	for rows.Next() {
		var m types.Membership
		if err := rows.Scan(&m.ID, &m.TenantID, &m.KratosIdentityID, &m.CreatedAt); err != nil {
			return nil, "", fmt.Errorf("failed to scan member: %w", err)
		}
		members = append(members, &m)
	}

	if err := rows.Err(); err != nil {
		return nil, "", fmt.Errorf("rows iteration error: %w", err)
	}

	var nextPageToken string
	if uint64(len(members)) > pageSize {
		nextPageToken = encodePageToken(members[pageSize-1].ID)
		members = members[:pageSize]
	}

	return members, nextPageToken, nil
}

func (s *Storage) GetMemberByTenantAndUserID(ctx context.Context, tenantID, userID string) (membership *types.Membership, err error) {
	defer func(start time.Time) { s.recordLatencyFor("GetMemberByTenantAndUserID", start, err) }(time.Now())
	ctx, span := s.tracer.Start(ctx, "storage.GetMemberByTenantAndUserID")
	defer span.End()

	var m types.Membership
	err = s.db.Statement(ctx).
		Select("id", "tenant_id", "kratos_identity_id", "created_at").
		From("memberships").
		Where(sq.Eq{
			"tenant_id":          tenantID,
			"kratos_identity_id": userID,
		}).
		QueryRowContext(ctx).
		Scan(&m.ID, &m.TenantID, &m.KratosIdentityID, &m.CreatedAt)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to get member: %w", err)
	}

	return &m, nil
}

func (s *Storage) GetActiveMemberByTenantAndUserID(ctx context.Context, tenantID, userID string) (membership *types.Membership, err error) {
	defer func(start time.Time) { s.recordLatencyFor("GetActiveMemberByTenantAndUserID", start, err) }(time.Now())
	ctx, span := s.tracer.Start(ctx, "storage.GetActiveMemberByTenantAndUserID")
	defer span.End()

	var m types.Membership
	err = s.db.Statement(ctx).
		Select("m.id", "m.tenant_id", "m.kratos_identity_id", "m.created_at").
		From("memberships m").
		Join("tenants t ON t.id = m.tenant_id").
		Where(sq.Eq{
			"m.tenant_id":          tenantID,
			"m.kratos_identity_id": userID,
			"t.enabled":            true,
		}).
		QueryRowContext(ctx).
		Scan(&m.ID, &m.TenantID, &m.KratosIdentityID, &m.CreatedAt)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to get active member: %w", err)
	}

	return &m, nil
}

// AddMember returns ErrDuplicateKey when userID is a member already. The
// conflict is absorbed (ON CONFLICT DO NOTHING) instead of raised: a unique
// violation would abort the request's transaction, and with it whatever the
// caller goes on to write.
func (s *Storage) AddMember(ctx context.Context, tenantID, userID string) (membershipID string, err error) {
	defer func(start time.Time) { s.recordLatencyFor("AddMember", start, err) }(time.Now())
	ctx, span := s.tracer.Start(ctx, "storage.AddMember")
	defer span.End()

	id, err := uuid.NewV7()
	if err != nil {
		return "", fmt.Errorf("failed to generate membership ID: %w", err)
	}

	err = s.db.Statement(ctx).
		Insert("memberships").
		Columns("id", "tenant_id", "kratos_identity_id").
		Values(id.String(), tenantID, userID).
		Suffix("ON CONFLICT (tenant_id, kratos_identity_id) DO NOTHING RETURNING id").
		QueryRowContext(ctx).
		Scan(&membershipID)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrDuplicateKey
		}
		if IsForeignKeyViolation(err) {
			return "", ErrForeignKeyViolation
		}
		return "", fmt.Errorf("failed to add member: %w", err)
	}

	return membershipID, nil
}

func (s *Storage) DeleteMember(ctx context.Context, tenantID, userID string) (err error) {
	defer func(start time.Time) { s.recordLatencyFor("DeleteMember", start, err) }(time.Now())
	ctx, span := s.tracer.Start(ctx, "storage.DeleteMember")
	defer span.End()

	res, err := s.db.Statement(ctx).
		Delete("memberships").
		Where(sq.Eq{
			"tenant_id":          tenantID,
			"kratos_identity_id": userID,
		}).
		ExecContext(ctx)
	if err != nil {
		return fmt.Errorf("failed to delete member: %w", err)
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check rows affected: %w", err)
	}
	if rows == 0 {
		return ErrNotFound
	}

	return nil
}

// UpdateTenant updates fields specified in paths.
// If paths is empty or nil, no update is performed except if we decide default behavior is full update.
// Here we follow typical PATCH semantics: update only what's in paths.
// If paths contains "name", update name.
// If paths contains "enabled", update enabled status.
func (s *Storage) UpdateTenant(ctx context.Context, tenant *types.Tenant, paths []string) (err error) {
	defer func(start time.Time) { s.recordLatencyFor("UpdateTenant", start, err) }(time.Now())
	ctx, span := s.tracer.Start(ctx, "storage.UpdateTenant")
	defer span.End()

	if len(paths) == 0 {
		return nil
	}

	updateMap := make(map[string]interface{})
	for _, p := range paths {
		switch p {
		case "name":
			updateMap["name"] = tenant.Name
		case "enabled":
			updateMap["enabled"] = tenant.Enabled
		}
	}

	if len(updateMap) == 0 {
		return nil
	}

	query := s.db.Statement(ctx).
		Update("tenants").
		SetMap(updateMap).
		Where(sq.Eq{"id": tenant.ID})

	_, err = query.ExecContext(ctx)
	if err != nil {
		return fmt.Errorf("failed to update tenant: %w", err)
	}

	return nil
}

func (s *Storage) DeleteTenant(ctx context.Context, id string) (err error) {
	defer func(start time.Time) { s.recordLatencyFor("DeleteTenant", start, err) }(time.Now())
	ctx, span := s.tracer.Start(ctx, "storage.DeleteTenant")
	defer span.End()

	_, err = s.db.Statement(ctx).
		Delete("tenants").
		Where(sq.Eq{"id": id}).
		ExecContext(ctx)

	if err != nil {
		return fmt.Errorf("failed to delete tenant: %w", err)
	}
	return nil
}

func encodePageToken(id string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(id))
}

func decodePageToken(token string) (string, error) {
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return "", ErrInvalidPageToken
	}
	parsed, err := uuid.Parse(string(raw))
	if err != nil {
		return "", ErrInvalidPageToken
	}
	return parsed.String(), nil
}

// resolveListOptions applies the given functional options and decodes the page
// token. It returns the effective page size, the cursor ID to use in a
// WHERE clause (empty string when no token was provided), and the full ListOptions.
func resolveListOptions(options []types.ListOption) (pageSize uint64, cursorID string, opts types.ListOptions, err error) {
	for _, o := range options {
		o(&opts)
	}
	pageSize = opts.ResolvePageSize()
	if opts.PageToken != "" {
		cursorID, err = decodePageToken(opts.PageToken)
		if err != nil {
			return 0, "", opts, err
		}
	}
	return pageSize, cursorID, opts, nil
}

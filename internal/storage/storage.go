// Copyright 2025 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0

package storage

import (
	"context"
	"errors"
	"fmt"

	sq "github.com/Masterminds/squirrel"
	"github.com/canonical/tenant-service/internal/db"
	"github.com/canonical/tenant-service/internal/logging"
	"github.com/canonical/tenant-service/internal/monitoring"
	"github.com/canonical/tenant-service/internal/tracing"
	"github.com/canonical/tenant-service/internal/types"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
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

func (s *Storage) CreateTenant(ctx context.Context, t *types.Tenant) (*types.Tenant, error) {
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
		Suffix("RETURNING id, name, created_at, enabled").
		QueryRowContext(ctx).
		Scan(&newTenant.ID, &newTenant.Name, &newTenant.CreatedAt, &newTenant.Enabled)

	if err != nil {
		return nil, fmt.Errorf("failed to insert tenant: %w", err)
	}

	return &newTenant, nil
}

func (s *Storage) GetTenantByID(ctx context.Context, id string) (*types.Tenant, error) {
	ctx, span := s.tracer.Start(ctx, "storage.GetTenantByID")
	defer span.End()

	var t types.Tenant
	err := s.db.Statement(ctx).
		Select("id", "name", "created_at", "enabled").
		From("tenants").
		Where(sq.Eq{"id": id}).
		QueryRowContext(ctx).
		Scan(&t.ID, &t.Name, &t.CreatedAt, &t.Enabled)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to get tenant: %w", err)
	}

	return &t, nil
}

func (s *Storage) ListTenants(ctx context.Context) ([]*types.Tenant, error) {
	ctx, span := s.tracer.Start(ctx, "storage.ListTenants")
	defer span.End()

	query := s.db.Statement(ctx).
		Select("id", "name", "created_at", "enabled").
		From("tenants")

	rows, err := query.QueryContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list tenants: %w", err)
	}
	defer rows.Close()

	var tenants []*types.Tenant
	for rows.Next() {
		var t types.Tenant
		if err := rows.Scan(&t.ID, &t.Name, &t.CreatedAt, &t.Enabled); err != nil {
			return nil, fmt.Errorf("failed to scan tenant: %w", err)
		}
		tenants = append(tenants, &t)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating tenant rows: %w", err)
	}

	return tenants, nil
}

func (s *Storage) ListActiveTenantsByUserID(ctx context.Context, userID string) ([]*types.Tenant, error) {
	return s.listTenantsByUserID(ctx, userID, false)
}

func (s *Storage) ListTenantsByUserID(ctx context.Context, userID string) ([]*types.Tenant, error) {
	return s.listTenantsByUserID(ctx, userID, true)
}

func (s *Storage) listTenantsByUserID(ctx context.Context, userID string, showDisabled bool) ([]*types.Tenant, error) {
	ctx, span := s.tracer.Start(ctx, "storage.ListTenantsByUserID")
	defer span.End()

	query := s.db.Statement(ctx).
		Select("t.id", "t.name", "t.created_at", "t.enabled").
		From("tenants t").
		Join("memberships m ON t.id = m.tenant_id").
		Where(sq.Eq{"m.kratos_identity_id": userID})

	if !showDisabled {
		query = query.Where(sq.Eq{"t.enabled": true})
	}

	rows, err := query.QueryContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list tenants: %w", err)
	}
	defer rows.Close()

	var tenants []*types.Tenant
	for rows.Next() {
		var t types.Tenant
		if err := rows.Scan(&t.ID, &t.Name, &t.CreatedAt, &t.Enabled); err != nil {
			return nil, fmt.Errorf("failed to scan tenant: %w", err)
		}
		tenants = append(tenants, &t)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration error: %w", err)
	}

	return tenants, nil
}

func (s *Storage) ListMembersByTenantID(ctx context.Context, tenantID string) ([]*types.Membership, error) {
	ctx, span := s.tracer.Start(ctx, "storage.ListMembersByTenantID")
	defer span.End()

	query := s.db.Statement(ctx).
		Select("id", "tenant_id", "kratos_identity_id", "role", "created_at").
		From("memberships").
		Where(sq.Eq{"tenant_id": tenantID})

	rows, err := query.QueryContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list members: %w", err)
	}
	defer rows.Close()

	var members []*types.Membership
	for rows.Next() {
		var m types.Membership
		if err := rows.Scan(&m.ID, &m.TenantID, &m.KratosIdentityID, &m.Role, &m.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan member: %w", err)
		}
		members = append(members, &m)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration error: %w", err)
	}

	return members, nil
}

func (s *Storage) AddMember(ctx context.Context, tenantID, userID, role string) (string, error) {
	ctx, span := s.tracer.Start(ctx, "storage.AddMember")
	defer span.End()

	id, err := uuid.NewV7()
	if err != nil {
		return "", fmt.Errorf("failed to generate membership ID: %w", err)
	}

	_, err = s.db.Statement(ctx).
		Insert("memberships").
		Columns("id", "tenant_id", "kratos_identity_id", "role").
		Values(id.String(), tenantID, userID, role).
		ExecContext(ctx)

	if err != nil {
		if IsDuplicateKeyError(err) {
			return "", ErrDuplicateKey
		}
		if IsForeignKeyViolation(err) {
			return "", ErrForeignKeyViolation
		}
		return "", fmt.Errorf("failed to add member: %w", err)
	}

	return id.String(), nil
}

func (s *Storage) UpdateMember(ctx context.Context, tenantID, userID, role string) error {
	ctx, span := s.tracer.Start(ctx, "storage.UpdateMember")
	defer span.End()

	res, err := s.db.Statement(ctx).
		Update("memberships").
		Set("role", role).
		Where(sq.Eq{
			"tenant_id":          tenantID,
			"kratos_identity_id": userID,
		}).
		ExecContext(ctx)

	if err != nil {
		return fmt.Errorf("failed to update member: %w", err)
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("member not found")
	}

	return nil
}

// UpdateTenant updates fields specified in paths.
// If paths is empty or nil, no update is performed except if we decide default behavior is full update.
// Here we follow typical PATCH semantics: update only what's in paths.
// If paths contains "name", update name.
// If paths contains "enabled", update enabled status.
func (s *Storage) UpdateTenant(ctx context.Context, tenant *types.Tenant, paths []string) error {
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

	_, err := query.ExecContext(ctx)
	if err != nil {
		return fmt.Errorf("failed to update tenant: %w", err)
	}

	return nil
}

func (s *Storage) DeleteTenant(ctx context.Context, id string) error {
	ctx, span := s.tracer.Start(ctx, "storage.DeleteTenant")
	defer span.End()

	_, err := s.db.Statement(ctx).
		Delete("tenants").
		Where(sq.Eq{"id": id}).
		ExecContext(ctx)

	if err != nil {
		return fmt.Errorf("failed to delete tenant: %w", err)
	}
	return nil
}

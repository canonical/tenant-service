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

func (s *Storage) GetInviteByToken(ctx context.Context, token string) (*types.Invite, error) {
	ctx, span := s.tracer.Start(ctx, "storage.GetInviteByToken")
	defer span.End()

	var i types.Invite
	err := s.db.Statement(ctx).
		Select("token", "tenant_id", "email", "role", "created_at").
		From("invites").
		Where(sq.Eq{"token": token}).
		QueryRowContext(ctx).
		Scan(&i.Token, &i.TenantID, &i.Email, &i.Role, &i.CreatedAt)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to get invite: %w", err)
	}

	return &i, nil
}

func (s *Storage) CreateInvite(ctx context.Context, invite *types.Invite) (*types.Invite, error) {
	ctx, span := s.tracer.Start(ctx, "storage.CreateInvite")
	defer span.End()

	id, err := uuid.NewV7()
	if err != nil {
		return nil, fmt.Errorf("failed to generate invite ID: %w", err)
	}

	// Use a new struct or update the existing one?
	// To be safe and return the full state as stored:
	invite.ID = id.String()

	_, err = s.db.Statement(ctx).
		Insert("invites").
		Columns("id", "token", "tenant_id", "email", "role").
		Values(invite.ID, invite.Token, invite.TenantID, invite.Email, invite.Role).
		ExecContext(ctx)

	if err != nil {
		return nil, fmt.Errorf("failed to create invite: %w", err)
	}

	return invite, nil
}

func (s *Storage) UpdateTenant(ctx context.Context, id string, name string, ownerIDs []string) error {
	ctx, span := s.tracer.Start(ctx, "storage.UpdateTenant")
	defer span.End()

	// Update name if provided
	if name != "" {
		_, err := s.db.Statement(ctx).
			Update("tenants").
			Set("name", name).
			Where(sq.Eq{"id": id}).
			ExecContext(ctx)
		if err != nil {
			return fmt.Errorf("failed to update tenant name: %w", err)
		}
	}

	// Update owners if provided
	if len(ownerIDs) > 0 {
		// Delete existing owners
		// We use a transaction implicitly via the context if middleware set it up.
		// If not, these are separate statements which is risky but acceptable per instructions.
		_, err := s.db.Statement(ctx).
			Delete("memberships").
			Where(sq.Eq{"tenant_id": id}).
			Where(sq.Eq{"role": "owner"}).
			ExecContext(ctx)
		if err != nil {
			return fmt.Errorf("failed to remove existing owners: %w", err)
		}

		// Insert new owners
		stmt := s.db.Statement(ctx).
			Insert("memberships").
			Columns("id", "tenant_id", "kratos_identity_id", "role")

		for _, ownerID := range ownerIDs {
			memID, _ := uuid.NewV7()
			stmt = stmt.Values(memID.String(), id, ownerID, "owner")
		}

		_, err = stmt.ExecContext(ctx)
		if err != nil {
			if IsDuplicateKeyError(err) {
				return ErrDuplicateKey
			}
			return fmt.Errorf("failed to add new owners: %w", err)
		}
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

func (s *Storage) SetTenantStatus(ctx context.Context, id string, enabled bool) error {
	ctx, span := s.tracer.Start(ctx, "storage.SetTenantStatus")
	defer span.End()

	_, err := s.db.Statement(ctx).
		Update("tenants").
		Set("enabled", enabled).
		Where(sq.Eq{"id": id}).
		ExecContext(ctx)

	if err != nil {
		return fmt.Errorf("failed to set tenant status: %w", err)
	}
	return nil
}

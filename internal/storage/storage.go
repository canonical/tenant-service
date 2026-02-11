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

	var newTenant types.Tenant
	err := s.db.Statement(ctx).
		Insert("tenants").
		Columns("name").
		Values(t.Name).
		Suffix("RETURNING id, name, created_at").
		QueryRowContext(ctx).
		Scan(&newTenant.ID, &newTenant.Name, &newTenant.CreatedAt)

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
		Select("id", "name", "created_at").
		From("tenants").
		Where(sq.Eq{"id": id}).
		QueryRowContext(ctx).
		Scan(&t.ID, &t.Name, &t.CreatedAt)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to get tenant: %w", err)
	}

	return &t, nil
}

func (s *Storage) ListTenantsByUserID(ctx context.Context, userID string) ([]*types.Tenant, error) {
	ctx, span := s.tracer.Start(ctx, "storage.ListTenantsByUserID")
	defer span.End()

	query := s.db.Statement(ctx).
		Select("t.id", "t.name", "t.created_at").
		From("tenants t").
		Join("memberships m ON t.id = m.tenant_id").
		Where(sq.Eq{"m.kratos_identity_id": userID})

	rows, err := query.QueryContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list tenants: %w", err)
	}
	defer rows.Close()

	var tenants []*types.Tenant
	for rows.Next() {
		var t types.Tenant
		if err := rows.Scan(&t.ID, &t.Name, &t.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan tenant: %w", err)
		}
		tenants = append(tenants, &t)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration error: %w", err)
	}

	return tenants, nil
}

func (s *Storage) AddMember(ctx context.Context, tenantID, userID, role string) error {
	ctx, span := s.tracer.Start(ctx, "storage.AddMember")
	defer span.End()

	_, err := s.db.Statement(ctx).
		Insert("memberships").
		Columns("tenant_id", "kratos_identity_id", "role").
		Values(tenantID, userID, role).
		ExecContext(ctx)

	if err != nil {
		if IsDuplicateKeyError(err) {
			return ErrDuplicateKey
		}
		if IsForeignKeyViolation(err) {
			return ErrForeignKeyViolation
		}
		return fmt.Errorf("failed to add member: %w", err)
	}

	return nil
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

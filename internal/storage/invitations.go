// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/canonical/tenant-service/internal/types"
)

// AddInvitation invites email to tenantID for lifetime. An invitation that
// exists already is renewed.
func (s *Storage) AddInvitation(ctx context.Context, tenantID, email string, lifetime time.Duration) (err error) {
	defer func(start time.Time) { s.recordLatencyFor("AddInvitation", start, err) }(time.Now())
	ctx, span := s.tracer.Start(ctx, "storage.AddInvitation")
	defer span.End()

	_, err = s.db.Statement(ctx).
		Insert("tenant_invitations").
		Columns("tenant_id", "email", "expires_at").
		Values(tenantID, email, sq.Expr("NOW() + make_interval(secs => ?)", lifetime.Seconds())).
		Suffix("ON CONFLICT (tenant_id, email) DO UPDATE SET expires_at = EXCLUDED.expires_at").
		ExecContext(ctx)

	if err != nil {
		return fmt.Errorf("failed to add invitation: %w", err)
	}

	return nil
}

// HasInvitationByTenantAndEmail reports whether email has an unexpired
// invitation to tenantID.
func (s *Storage) HasInvitationByTenantAndEmail(ctx context.Context, tenantID, email string) (found bool, err error) {
	defer func(start time.Time) { s.recordLatencyFor("HasInvitationByTenantAndEmail", start, err) }(time.Now())
	ctx, span := s.tracer.Start(ctx, "storage.HasInvitationByTenantAndEmail")
	defer span.End()

	var one int
	err = s.db.Statement(ctx).
		Select("1").
		From("tenant_invitations").
		Where(sq.Eq{"tenant_id": tenantID, "email": email}).
		Where("expires_at > NOW()").
		QueryRowContext(ctx).
		Scan(&one)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("failed to get invitation: %w", err)
	}

	return true, nil
}

func (s *Storage) DeleteInvitation(ctx context.Context, tenantID, email string) (err error) {
	defer func(start time.Time) { s.recordLatencyFor("DeleteInvitation", start, err) }(time.Now())
	ctx, span := s.tracer.Start(ctx, "storage.DeleteInvitation")
	defer span.End()

	_, err = s.db.Statement(ctx).
		Delete("tenant_invitations").
		Where(sq.Eq{"tenant_id": tenantID, "email": email}).
		ExecContext(ctx)

	if err != nil {
		return fmt.Errorf("failed to delete invitation: %w", err)
	}

	return nil
}

// ListInvitedTenantsByEmail returns the enabled tenants email, an address in
// domain, has an unexpired invitation to that still admits it, without those
// excludeUserID is a member of ("" excludes none).
func (s *Storage) ListInvitedTenantsByEmail(ctx context.Context, email, domain, excludeUserID string) (tenants []*types.SignInTenant, err error) {
	defer func(start time.Time) { s.recordLatencyFor("ListInvitedTenantsByEmail", start, err) }(time.Now())
	ctx, span := s.tracer.Start(ctx, "storage.ListInvitedTenantsByEmail")
	defer span.End()

	query := s.db.Statement(ctx).
		Select("t.id", "t.name", "t.created_at", "t.enabled", "t.personal_identity_id", "t.mfa_requirement").
		From("tenants t").
		Join("tenant_invitations i ON i.tenant_id = t.id").
		Where(sq.Eq{"t.enabled": true, "i.email": email}).
		Where("i.expires_at > NOW()").
		// A tenant that requires SSO and has domains has no sign-in for an
		// address outside them: its invitation admits nothing.
		Where(`NOT (t.sso_enforcement = ?
			AND EXISTS (SELECT 1 FROM tenant_sso_bindings b WHERE b.tenant_id = t.id AND b.active)
			AND EXISTS (SELECT 1 FROM tenant_sso_domains d WHERE d.tenant_id = t.id)
			AND NOT EXISTS (SELECT 1 FROM tenant_sso_domains d WHERE d.tenant_id = t.id AND d.domain = ?))`,
			types.EnforcementRequired, domain).
		OrderBy("t.id")

	if excludeUserID != "" {
		query = query.Where("NOT EXISTS (SELECT 1 FROM memberships m WHERE m.tenant_id = t.id AND m.kratos_identity_id = ?)", excludeUserID)
	}

	rows, err := query.QueryContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list invited tenants: %w", err)
	}
	defer rows.Close()

	tenants = make([]*types.SignInTenant, 0)
	for rows.Next() {
		t := types.SignInTenant{Invited: true}
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

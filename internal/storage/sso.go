// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package storage

import (
	"context"
	"database/sql"
	"fmt"
	"slices"
	"time"

	sq "github.com/Masterminds/squirrel"

	"github.com/canonical/tenant-service/internal/types"
)

func (s *Storage) GetTenantSSOPolicy(ctx context.Context, tenantID string) (policy *types.TenantSSOPolicy, err error) {
	defer func(start time.Time) { s.recordLatencyFor("GetTenantSSOPolicy", start, err) }(time.Now())
	ctx, span := s.tracer.Start(ctx, "storage.GetTenantSSOPolicy")
	defer span.End()

	return s.getTenantSSOPolicy(ctx, tenantID)
}

// LockTenantSSOPolicy locks the tenant's row until the transaction ends:
// UpdateTenantSSOPolicy, UpdateTenantSSODomains and DeleteTenantSSOBinding are
// called with this lock held. The policy is read in a statement of its own,
// after the lock: a statement that waited for the lock would still see the
// bindings and domains as they were when it started.
func (s *Storage) LockTenantSSOPolicy(ctx context.Context, tenantID string) (tenant *types.Tenant, policy *types.TenantSSOPolicy, err error) {
	defer func(start time.Time) { s.recordLatencyFor("LockTenantSSOPolicy", start, err) }(time.Now())
	ctx, span := s.tracer.Start(ctx, "storage.LockTenantSSOPolicy")
	defer span.End()

	tenant, err = s.getTenant(ctx, sq.Eq{"id": tenantID}, true)
	if err != nil {
		return nil, nil, err
	}

	policy, err = s.getTenantSSOPolicy(ctx, tenantID)
	if err != nil {
		return nil, nil, err
	}

	return tenant, policy, nil
}

// getTenantSSOPolicy reads the policy in one statement, so that a read outside
// a transaction sees one state of it. The joins give every binding once per
// domain and every domain once per binding: each is kept once.
func (s *Storage) getTenantSSOPolicy(ctx context.Context, tenantID string) (*types.TenantSSOPolicy, error) {
	rows, err := s.db.Statement(ctx).
		Select("t.id", "t.sso_enforcement", "t.sso_auto_join", "b.connection_id", "b.active", "d.domain").
		From("tenants t").
		LeftJoin("tenant_sso_bindings b ON b.tenant_id = t.id").
		LeftJoin("tenant_sso_domains d ON d.tenant_id = t.id").
		Where(sq.Eq{"t.id": tenantID}).
		OrderBy("b.connection_id", "d.domain").
		QueryContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get tenant sso policy: %w", err)
	}
	defer rows.Close()

	var policy *types.TenantSSOPolicy
	for rows.Next() {
		if policy == nil {
			policy = &types.TenantSSOPolicy{Domains: []string{}, Bindings: []types.SSOBinding{}}
		}
		var (
			connection, domain sql.NullString
			active             sql.NullBool
		)
		if err := rows.Scan(&policy.TenantID, &policy.Enforcement, &policy.AutoJoin, &connection, &active, &domain); err != nil {
			return nil, fmt.Errorf("failed to scan tenant sso policy: %w", err)
		}
		binding := types.SSOBinding{ConnectionID: connection.String, Active: active.Bool}
		if connection.Valid && !slices.Contains(policy.Bindings, binding) {
			policy.Bindings = append(policy.Bindings, binding)
		}
		if domain.Valid && !slices.Contains(policy.Domains, domain.String) {
			policy.Domains = append(policy.Domains, domain.String)
		}
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration error: %w", err)
	}
	if policy == nil {
		return nil, ErrNotFound
	}

	return policy, nil
}

// UpdateTenantSSOPolicy returns ErrDuplicateKey when another tenant binds one
// of the connections.
func (s *Storage) UpdateTenantSSOPolicy(ctx context.Context, tenantID, enforcement string, autoJoin bool, bindings []types.SSOBinding) (err error) {
	defer func(start time.Time) { s.recordLatencyFor("UpdateTenantSSOPolicy", start, err) }(time.Now())
	ctx, span := s.tracer.Start(ctx, "storage.UpdateTenantSSOPolicy")
	defer span.End()

	_, err = s.db.Statement(ctx).
		Update("tenants").
		Set("sso_enforcement", enforcement).
		Set("sso_auto_join", autoJoin).
		Where(sq.Eq{"id": tenantID}).
		ExecContext(ctx)
	if err != nil {
		return fmt.Errorf("failed to update tenant sso policy: %w", err)
	}

	_, err = s.db.Statement(ctx).
		Delete("tenant_sso_bindings").
		Where(sq.Eq{"tenant_id": tenantID}).
		ExecContext(ctx)
	if err != nil {
		return fmt.Errorf("failed to delete tenant sso bindings: %w", err)
	}

	if len(bindings) > 0 {
		insert := s.db.Statement(ctx).
			Insert("tenant_sso_bindings").
			Columns("tenant_id", "connection_id", "active")
		for _, b := range bindings {
			insert = insert.Values(tenantID, b.ConnectionID, b.Active)
		}
		if _, err = insert.ExecContext(ctx); err != nil {
			if IsDuplicateKeyError(err) {
				return ErrDuplicateKey
			}
			return fmt.Errorf("failed to insert tenant sso bindings: %w", err)
		}
	}

	return nil
}

func (s *Storage) UpdateTenantSSODomains(ctx context.Context, tenantID string, domains []string) (err error) {
	defer func(start time.Time) { s.recordLatencyFor("UpdateTenantSSODomains", start, err) }(time.Now())
	ctx, span := s.tracer.Start(ctx, "storage.UpdateTenantSSODomains")
	defer span.End()

	_, err = s.db.Statement(ctx).
		Delete("tenant_sso_domains").
		Where(sq.Eq{"tenant_id": tenantID}).
		ExecContext(ctx)
	if err != nil {
		return fmt.Errorf("failed to delete tenant sso domains: %w", err)
	}

	if len(domains) > 0 {
		insert := s.db.Statement(ctx).
			Insert("tenant_sso_domains").
			Columns("tenant_id", "domain")
		for _, d := range domains {
			insert = insert.Values(tenantID, d)
		}
		if _, err = insert.ExecContext(ctx); err != nil {
			return fmt.Errorf("failed to insert tenant sso domains: %w", err)
		}
	}

	return nil
}

func (s *Storage) DeleteTenantSSOBinding(ctx context.Context, tenantID, connectionID string) (err error) {
	defer func(start time.Time) { s.recordLatencyFor("DeleteTenantSSOBinding", start, err) }(time.Now())
	ctx, span := s.tracer.Start(ctx, "storage.DeleteTenantSSOBinding")
	defer span.End()

	_, err = s.db.Statement(ctx).
		Delete("tenant_sso_bindings").
		Where(sq.Eq{"tenant_id": tenantID, "connection_id": connectionID}).
		ExecContext(ctx)
	if err != nil {
		return fmt.Errorf("failed to delete tenant sso binding: %w", err)
	}

	return nil
}

// ListAutoJoinCandidatesByDomain returns the enabled tenants whose auto-join
// admits domain, without those excludeUserID is a member of ("" excludes none).
func (s *Storage) ListAutoJoinCandidatesByDomain(ctx context.Context, domain, excludeUserID string) (tenants []*types.SignInTenant, err error) {
	defer func(start time.Time) { s.recordLatencyFor("ListAutoJoinCandidatesByDomain", start, err) }(time.Now())
	ctx, span := s.tracer.Start(ctx, "storage.ListAutoJoinCandidatesByDomain")
	defer span.End()

	query := s.db.Statement(ctx).
		Select("t.id", "t.name", "t.created_at", "t.enabled", "t.personal_identity_id", "t.mfa_requirement").
		From("tenants t").
		Join("tenant_sso_domains d ON d.tenant_id = t.id").
		Where(sq.Eq{
			"t.enabled":         true,
			"t.sso_auto_join":   true,
			"t.sso_enforcement": types.EnforcementRequired,
			"d.domain":          domain,
		}).
		Where("EXISTS (SELECT 1 FROM tenant_sso_bindings b WHERE b.tenant_id = t.id AND b.active)").
		OrderBy("t.id")

	if excludeUserID != "" {
		query = query.Where("NOT EXISTS (SELECT 1 FROM memberships m WHERE m.tenant_id = t.id AND m.kratos_identity_id = ?)", excludeUserID)
	}

	rows, err := query.QueryContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list auto-join candidates: %w", err)
	}
	defer rows.Close()

	tenants = make([]*types.SignInTenant, 0)
	for rows.Next() {
		t := types.SignInTenant{AutoJoinCandidate: true}
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

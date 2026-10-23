// Copyright 2025 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package storage

import (
	"context"
	"time"

	"github.com/canonical/tenant-service/internal/types"
)

type StorageInterface interface {
	CreateTenant(ctx context.Context, t *types.Tenant) (*types.Tenant, error)
	GetTenantByID(ctx context.Context, id string) (*types.Tenant, error)
	ListTenants(ctx context.Context, opts ...types.ListOption) ([]*types.Tenant, string, error)
	ListTenantsByUserID(ctx context.Context, userID string, opts ...types.ListOption) ([]*types.Tenant, error)
	UpdateTenant(ctx context.Context, tenant *types.Tenant, paths []string) error
	DeleteTenant(ctx context.Context, id string) error
	AddMember(ctx context.Context, tenantID, userID string) (string, error)
	DeleteMember(ctx context.Context, tenantID, userID string) error
	GetMemberByTenantAndUserID(ctx context.Context, tenantID, userID string) (*types.Membership, error)
	ListMembersByTenantID(ctx context.Context, tenantID string, opts ...types.ListOption) ([]*types.Membership, string, error)
	// GetActiveMemberByTenantAndUserID returns the membership only when the tenant is enabled.
	// Returns ErrNotFound if the membership does not exist or the tenant is disabled.
	GetActiveMemberByTenantAndUserID(ctx context.Context, tenantID, userID string) (*types.Membership, error)
	CreatePersonalTenant(ctx context.Context, userID, name string) (*types.Tenant, bool, error)
	GetPersonalTenantByUserID(ctx context.Context, userID string) (*types.Tenant, error)
	GetTenantSSOPolicy(ctx context.Context, tenantID string) (*types.TenantSSOPolicy, error)
	LockTenantSSOPolicy(ctx context.Context, tenantID string) (*types.Tenant, *types.TenantSSOPolicy, error)
	UpdateTenantSSOPolicy(ctx context.Context, tenantID, enforcement string, autoJoin bool, bindings []types.SSOBinding) error
	UpdateTenantSSODomains(ctx context.Context, tenantID string, domains []string) error
	DeleteTenantSSOBinding(ctx context.Context, tenantID, connectionID string) error
	UpdateTenantMFARequirement(ctx context.Context, tenantID, requirement string) (string, error)
	ListAutoJoinCandidatesByDomain(ctx context.Context, domain, excludeUserID string) ([]*types.SignInTenant, error)
	ListInvitedTenantsByEmail(ctx context.Context, email, domain, excludeUserID string) ([]*types.SignInTenant, error)
	AddInvitation(ctx context.Context, tenantID, email string, lifetime time.Duration) error
	HasInvitationByTenantAndEmail(ctx context.Context, tenantID, email string) (bool, error)
	DeleteInvitation(ctx context.Context, tenantID, email string) error
}

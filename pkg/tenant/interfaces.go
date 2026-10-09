// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package tenant

import (
	"context"
	"time"

	"github.com/canonical/tenant-service/internal/types"
	ory "github.com/ory/client-go"
)

type ServiceInterface interface {
	InviteMember(ctx context.Context, tenantID, email string) (*types.Invitation, error)
	CreateTenant(ctx context.Context, name string) (*types.Tenant, error)
	CreatePersonalTenant(ctx context.Context, identityID, email string) (*types.Tenant, bool, error)
	UpdateTenant(ctx context.Context, tenant *types.Tenant, paths []string) (*types.Tenant, error)
	DeleteTenant(ctx context.Context, id string) error
	ProvisionUser(ctx context.Context, tenantID, email string) error
	JoinTenant(ctx context.Context, tenantID, identityID string) error
	RemoveTenantUser(ctx context.Context, tenantID, userID string) error
	ListTenantsByUserID(ctx context.Context, userID string, opts ...types.ListOption) ([]*types.Tenant, error)
	ListTenants(ctx context.Context, opts ...types.ListOption) ([]*types.Tenant, string, error)
	ListTenantUsers(ctx context.Context, tenantID string, includeEmails bool, opts ...types.ListOption) ([]*types.TenantUser, string, error)
	LookupTenantsByEmail(ctx context.Context, email string) ([]*types.Tenant, error)
	LookupTenantsByIdentityID(ctx context.Context, identityID string) ([]*types.Tenant, error)
	ListSignInTenants(ctx context.Context, email string) ([]*types.SignInTenant, error)
	GetSignInContext(ctx context.Context, tenantID, email, identityID string) (*types.SignInContext, error)
	GetTenantSSOPolicy(ctx context.Context, tenantID string) (*types.TenantSSOPolicy, error)
	PutTenantSSOPolicy(ctx context.Context, tenantID, enforcement string, autoJoin bool, bindings []types.SSOBinding) (*types.TenantSSOPolicy, error)
	SetTenantSSODomains(ctx context.Context, tenantID string, domains []string) (*types.TenantSSOPolicy, error)
	RemoveTenantSSOBinding(ctx context.Context, tenantID, connectionID string) error
	GetTenantMFAPolicy(ctx context.Context, tenantID string) (*types.TenantMFAPolicy, error)
	PutTenantMFAPolicy(ctx context.Context, tenantID, requirement string) (*types.TenantMFAPolicy, error)
}

type StorageInterface interface {
	CreateTenant(ctx context.Context, t *types.Tenant) (*types.Tenant, error)
	UpdateTenant(ctx context.Context, tenant *types.Tenant, paths []string) error
	DeleteTenant(ctx context.Context, id string) error
	AddMember(ctx context.Context, tenantID, userID string) (string, error)
	GetTenantByID(ctx context.Context, id string) (*types.Tenant, error)
	ListTenantsByUserID(ctx context.Context, userID string, opts ...types.ListOption) ([]*types.Tenant, error)
	ListTenants(ctx context.Context, opts ...types.ListOption) ([]*types.Tenant, string, error)
	GetMemberByTenantAndUserID(ctx context.Context, tenantID, userID string) (*types.Membership, error)
	ListMembersByTenantID(ctx context.Context, tenantID string, opts ...types.ListOption) ([]*types.Membership, string, error)
	DeleteMember(ctx context.Context, tenantID, userID string) error
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

type KratosClientInterface interface {
	GetIdentityIDByEmail(ctx context.Context, email string) (string, error)
	CreateIdentity(ctx context.Context, email string) (string, error)
	GetIdentity(ctx context.Context, id string) (*ory.Identity, error)
	GetIdentities(ctx context.Context, ids []string) (map[string]*ory.Identity, error)
	CreateRecoveryLink(ctx context.Context, identityID string, expiresIn string) (string, string, error)
}

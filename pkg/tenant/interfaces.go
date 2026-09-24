// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package tenant

import (
	"context"

	"github.com/canonical/tenant-service/internal/types"
	ory "github.com/ory/client-go"
)

type ServiceInterface interface {
	InviteMember(ctx context.Context, tenantID, email string) (string, string, error)
	CreateTenant(ctx context.Context, name string) (*types.Tenant, error)
	UpdateTenant(ctx context.Context, tenant *types.Tenant, paths []string) (*types.Tenant, error)
	DeleteTenant(ctx context.Context, id string) error
	ProvisionUser(ctx context.Context, tenantID, email string) error
	ListTenantsByUserID(ctx context.Context, userID string, opts ...types.ListOption) ([]*types.Tenant, error)
	ListTenants(ctx context.Context, opts ...types.ListOption) ([]*types.Tenant, string, error)
	ListTenantUsers(ctx context.Context, tenantID string, includeEmails bool, opts ...types.ListOption) ([]*types.TenantUser, string, error)
	LookupTenantsByEmail(ctx context.Context, email string) ([]*types.Tenant, error)
	LookupTenantsByIdentityID(ctx context.Context, identityID string) ([]*types.Tenant, error)
}

type StorageInterface interface {
	CreateTenant(ctx context.Context, t *types.Tenant) (*types.Tenant, error)
	UpdateTenant(ctx context.Context, tenant *types.Tenant, paths []string) error
	DeleteTenant(ctx context.Context, id string) error
	AddMember(ctx context.Context, tenantID, userID string) (string, error)
	GetTenantByID(ctx context.Context, id string) (*types.Tenant, error)
	ListTenantsByUserID(ctx context.Context, userID string, opts ...types.ListOption) ([]*types.Tenant, error)
	ListTenants(ctx context.Context, opts ...types.ListOption) ([]*types.Tenant, string, error)
	ListMembersByTenantID(ctx context.Context, tenantID string, opts ...types.ListOption) ([]*types.Membership, string, error)
}

type KratosClientInterface interface {
	GetIdentityIDByEmail(ctx context.Context, email string) (string, error)
	CreateIdentity(ctx context.Context, email string) (string, error)
	GetIdentities(ctx context.Context, ids []string) (map[string]*ory.Identity, error)
	CreateRecoveryLink(ctx context.Context, identityID string, expiresIn string) (string, string, error)
}

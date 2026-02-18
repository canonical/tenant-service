// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0

package tenant

import (
	"context"

	"github.com/canonical/tenant-service/internal/openfga"
	"github.com/canonical/tenant-service/internal/types"
	ory "github.com/ory/client-go"
)

type ServiceInterface interface {
	InviteMember(ctx context.Context, tenantID, email, role string) (string, string, error)
	CreateTenant(ctx context.Context, name string) (*types.Tenant, error)
	UpdateTenant(ctx context.Context, id, name string, ownerIDs []string) (*types.Tenant, error)
	DeleteTenant(ctx context.Context, id string) error
	ProvisionUser(ctx context.Context, tenantID, email, role string) error
	ActivateTenant(ctx context.Context, tenantID string) error
	DeactivateTenant(ctx context.Context, tenantID string) error
	ListUserTenants(ctx context.Context, userID string) ([]*types.Tenant, error)
	ListTenantsByUserID(ctx context.Context, userID string) ([]*types.Tenant, error)
	ListTenants(ctx context.Context) ([]*types.Tenant, error)
	ListTenantUsers(ctx context.Context, tenantID string) ([]*types.TenantUser, error)
}

type StorageInterface interface {
	CreateInvite(ctx context.Context, invite *types.Invite) (*types.Invite, error)
	CreateTenant(ctx context.Context, t *types.Tenant) (*types.Tenant, error)
	UpdateTenant(ctx context.Context, id, name string, ownerIDs []string) error
	DeleteTenant(ctx context.Context, id string) error
	SetTenantStatus(ctx context.Context, id string, enabled bool) error
	AddMember(ctx context.Context, tenantID, userID, role string) (string, error)
	GetTenantByID(ctx context.Context, id string) (*types.Tenant, error)
	GetInviteByToken(ctx context.Context, token string) (*types.Invite, error)
	ListTenantsByUserID(ctx context.Context, userID string) ([]*types.Tenant, error)
	ListTenants(ctx context.Context) ([]*types.Tenant, error)
	ListActiveTenantsByUserID(ctx context.Context, userID string) ([]*types.Tenant, error)
	ListMembersByTenantID(ctx context.Context, tenantID string) ([]*types.Membership, error)
}

type AuthzInterface interface {
	Check(ctx context.Context, user, relation, object string, tuples ...openfga.Tuple) (bool, error)
	AssignTenantOwner(ctx context.Context, tenantID, userID string) error
	AssignTenantMember(ctx context.Context, tenantID, userID string) error
	DeleteTenant(ctx context.Context, tenantID string) error
} // Fixed signature to match implementation

type KratosClientInterface interface {
	GetIdentityIDByEmail(ctx context.Context, email string) (string, error)
	CreateIdentity(ctx context.Context, email string) (string, error)
	GetIdentity(ctx context.Context, id string) (*ory.Identity, error)
	CreateRecoveryLink(ctx context.Context, identityID string, expiresIn string) (string, string, error)
}

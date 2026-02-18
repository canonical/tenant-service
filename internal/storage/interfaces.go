// Copyright 2025 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0

package storage

import (
	"context"

	"github.com/canonical/tenant-service/internal/types"
)

type StorageInterface interface {
	CreateTenant(ctx context.Context, t *types.Tenant) (*types.Tenant, error)
	GetTenantByID(ctx context.Context, id string) (*types.Tenant, error)
	ListTenants(ctx context.Context) ([]*types.Tenant, error)
	ListTenantsByUserID(ctx context.Context, userID string) ([]*types.Tenant, error)
	ListActiveTenantsByUserID(ctx context.Context, userID string) ([]*types.Tenant, error)
	UpdateTenant(ctx context.Context, id string, name string, ownerIDs []string) error
	DeleteTenant(ctx context.Context, id string) error
	SetTenantStatus(ctx context.Context, id string, enabled bool) error
	AddMember(ctx context.Context, tenantID, userID, role string) (string, error)
	GetInviteByToken(ctx context.Context, token string) (*types.Invite, error)
	CreateInvite(ctx context.Context, invite *types.Invite) (*types.Invite, error)
	ListMembersByTenantID(ctx context.Context, tenantID string) ([]*types.Membership, error)
}

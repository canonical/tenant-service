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
	UpdateTenant(ctx context.Context, tenant *types.Tenant, paths []string) error
	DeleteTenant(ctx context.Context, id string) error
	AddMember(ctx context.Context, tenantID, userID, role string) (string, error)
	UpdateMember(ctx context.Context, tenantID, userID, role string) error
	ListMembersByTenantID(ctx context.Context, tenantID string) ([]*types.Membership, error)
}

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
	ListTenantsByUserID(ctx context.Context, userID string) ([]*types.Tenant, error)
	AddMember(ctx context.Context, tenantID, userID, role string) error
	GetInviteByToken(ctx context.Context, token string) (*types.Invite, error)
}

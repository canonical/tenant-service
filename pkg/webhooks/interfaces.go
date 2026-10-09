// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package webhooks

import (
	"context"

	"github.com/canonical/tenant-service/internal/types"
)

// StorageInterface defines the storage operations required by the webhooks package.
// It is a subset of the internal/storage interface.
type StorageInterface interface {
	GetActiveMemberByTenantAndUserID(ctx context.Context, tenantID, userID string) (*types.Membership, error)
}

// PersonalTenantCreatorInterface defines the tenant service operations required by the webhooks package.
type PersonalTenantCreatorInterface interface {
	CreatePersonalTenant(ctx context.Context, identityID, email string) (*types.Tenant, bool, error)
}

// ServiceInterface defines the webhook service operations.
type ServiceInterface interface {
	HandleRegistration(ctx context.Context, identityID, email string) error
	HandleTokenHook(ctx context.Context, req *TokenHookRequest) (*TokenHookResponse, error)
	HandleLoginHook(ctx context.Context, identityID, email, tenantID string) error
}

// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0

package webhooks

import (
	"context"

	"github.com/canonical/tenant-service/internal/types"
	"github.com/ory/hydra/v2/oauth2"
)

// StorageInterface defines the storage operations required by the webhooks package.
// It is a subset of the internal/storage interface.
type StorageInterface interface {
	CreateTenant(ctx context.Context, t *types.Tenant) (*types.Tenant, error)
	AddMember(ctx context.Context, tenantID, userID, role string) (string, error)
	ListActiveTenantsByUserID(ctx context.Context, userID string) ([]*types.Tenant, error)
}

// AuthorizerInterface defines the authorization operations required by the webhooks package.
// It is a subset of the internal/authorization interface.
type AuthorizerInterface interface {
	AssignTenantOwner(ctx context.Context, tenantID, userID string) error
}

// ServiceInterface defines the webhook service operations.
type ServiceInterface interface {
	HandleRegistration(ctx context.Context, identityID, email string) error
	HandleTokenHook(ctx context.Context, req *oauth2.TokenHookRequest) (*TokenHookResponse, error)
}

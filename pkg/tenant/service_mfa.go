// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package tenant

import (
	"context"
	"errors"
	"fmt"

	"github.com/canonical/tenant-service/internal/logging"
	"github.com/canonical/tenant-service/internal/storage"
	"github.com/canonical/tenant-service/internal/types"
	"github.com/canonical/tenant-service/pkg/authentication"
)

func (s *Service) GetTenantMFAPolicy(ctx context.Context, tenantID string) (*types.TenantMFAPolicy, error) {
	ctx, span := s.tracer.Start(ctx, "tenant.Service.GetTenantMFAPolicy")
	defer span.End()

	s.logger.Debugw("getting tenant mfa policy", "tenant_id", tenantID)

	tenant, err := s.getTenant(ctx, span, tenantID)
	if err != nil {
		return nil, err
	}

	return &types.TenantMFAPolicy{TenantID: tenant.ID, Requirement: tenant.MFARequirement}, nil
}

func (s *Service) PutTenantMFAPolicy(ctx context.Context, tenantID, requirement string) (*types.TenantMFAPolicy, error) {
	ctx, span := s.tracer.Start(ctx, "tenant.Service.PutTenantMFAPolicy")
	defer span.End()

	actor, _ := authentication.GetUserID(ctx)
	s.logger.Debugw("writing tenant mfa policy",
		"tenant_id", tenantID,
		"mfa_requirement", requirement,
		"actor", actor,
	)

	before, err := s.storage.UpdateTenantMFARequirement(ctx, tenantID, requirement)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return nil, ErrTenantNotFound
		}
		s.recordError(span, "failed to write tenant mfa policy", err, "tenant_id", tenantID)
		return nil, fmt.Errorf("failed to write tenant mfa policy: %w", err)
	}

	s.logger.Security().AdminAction(actor, "put_tenant_mfa_policy", "tenant.Service.PutTenantMFAPolicy", tenantID,
		logging.WithLabel("before", before),
		logging.WithLabel("after", requirement),
	)
	return &types.TenantMFAPolicy{TenantID: tenantID, Requirement: requirement}, nil
}

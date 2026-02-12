// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0

package webhooks

import (
	"context"
	"fmt"

	"github.com/canonical/tenant-service/internal/logging"
	"github.com/canonical/tenant-service/internal/monitoring"
	"github.com/canonical/tenant-service/internal/tracing"
	"github.com/canonical/tenant-service/internal/types"
)

type Service struct {
	storage StorageInterface
	authz   AuthorizerInterface
	tracer  tracing.TracingInterface
	monitor monitoring.MonitorInterface
	logger  logging.LoggerInterface
}

func NewService(
	storage StorageInterface,
	authz AuthorizerInterface,
	tracer tracing.TracingInterface,
	monitor monitoring.MonitorInterface,
	logger logging.LoggerInterface,
) *Service {
	return &Service{
		storage: storage,
		authz:   authz,
		tracer:  tracer,
		monitor: monitor,
		logger:  logger,
	}
}

func (s *Service) HandleRegistration(ctx context.Context, identityID, email string) error {
	ctx, span := s.tracer.Start(ctx, "webhooks.Service.HandleRegistration")
	defer span.End()

	s.logger.Debugf("Handling registration for identity %s with email %s", identityID, email)

	if identityID == "" || email == "" {
		return fmt.Errorf("identity ID or email is empty")
	}

	// 1. Create a tenant named '{Email}'s Org'
	tenantName := fmt.Sprintf("%s's Org", email)
	tenant := &types.Tenant{
		Name: tenantName,
		// ID and CreatedAt are handled by DB/Storage usually, checking interfaces
	}

	newTenant, err := s.storage.CreateTenant(ctx, tenant)
	if err != nil {
		return fmt.Errorf("failed to create tenant: %w", err)
	}

	// 2. Add the user as 'owner'
	err = s.storage.AddMember(ctx, newTenant.ID, identityID, "owner")
	if err != nil {
		return fmt.Errorf("failed to add member: %w", err)
	}

	// 3. Call OpenFGA to write the tuple
	err = s.authz.AssignTenantOwner(ctx, newTenant.ID, identityID)
	if err != nil {
		return fmt.Errorf("failed to assign tenant owner in authz: %w", err)
	}

	s.logger.Infof("Successfully provisioned tenant %s for user %s", newTenant.ID, identityID)
	return nil
}

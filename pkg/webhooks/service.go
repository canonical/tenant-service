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
	"github.com/ory/hydra/v2/oauth2"
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

	if identityID == "" {
		return fmt.Errorf("identity ID is empty")
	}

	// 1. Create a tenant named '{Email}'s Org'
	tenantName := fmt.Sprintf("%s's Org", email)
	if email == "" {
		tenantName = ""
	}

	tenant := &types.Tenant{
		Name:    tenantName,
		Enabled: false,
	}

	newTenant, err := s.storage.CreateTenant(ctx, tenant)
	if err != nil {
		return fmt.Errorf("failed to create tenant: %w", err)
	}

	// 2. Add the user as 'owner'
	_, err = s.storage.AddMember(ctx, newTenant.ID, identityID, "owner")
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

func (s *Service) HandleTokenHook(ctx context.Context, req *oauth2.TokenHookRequest) (*TokenHookResponse, error) {
	ctx, span := s.tracer.Start(ctx, "webhooks.Service.HandleTokenHook")
	defer span.End()

	// Determine User ID
	var userID string
	s.logger.Debugf("Received token hook request: %+v", req)
	if req.Session != nil && req.Session.Subject != "" {
		userID = req.Session.Subject
	}

	if userID == "" {
		return nil, fmt.Errorf("could not identify user from request")
	}

	s.logger.Debugf("Handling token hook for user %s", userID)

	// Fetch Tenants
	tenants, err := s.storage.ListActiveTenantsByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to list tenants: %w", err)
	}

	// Format Response
	tenantList := make([]string, 0, len(tenants))
	for _, t := range tenants {
		tenantList = append(tenantList, t.ID)
	}

	resp := TokenHookResponse{
		Session: struct {
			IDToken     map[string]interface{} `json:"id_token,omitempty"`
			AccessToken map[string]interface{} `json:"access_token,omitempty"`
		}{
			IDToken:     map[string]interface{}{},
			AccessToken: map[string]interface{}{},
		},
	}

	if len(tenantList) > 0 {
		resp.Session.IDToken["tenants"] = tenantList
		resp.Session.AccessToken["tenants"] = tenantList
	}

	return &resp, nil
}

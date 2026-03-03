// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0

package webhooks

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

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

// recordError records an error on the span and emits a structured error log.
// The "error" key is always appended to keysAndValues automatically.
func (s *Service) recordError(span trace.Span, msg string, err error, keysAndValues ...interface{}) {
	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())
	s.logger.Errorw(msg, append(keysAndValues, "error", err)...)
}

func (s *Service) HandleRegistration(ctx context.Context, identityID, email string) error {
	ctx, span := s.tracer.Start(ctx, "webhooks.Service.HandleRegistration")
	defer span.End()

	s.logger.Debugw("handling registration webhook", "identity_id", identityID, "email", email)

	if identityID == "" {
		err := fmt.Errorf("identity ID is empty")
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
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
		s.recordError(span, "failed to create tenant on registration", err,
			"identity_id", identityID,
			"email", email,
		)
		return fmt.Errorf("failed to create tenant: %w", err)
	}

	// 2. Add the user as 'owner'
	_, err = s.storage.AddMember(ctx, newTenant.ID, identityID, "owner")
	if err != nil {
		s.recordError(span, "failed to add owner member on registration", err,
			"tenant_id", newTenant.ID,
			"identity_id", identityID,
		)
		return fmt.Errorf("failed to add member: %w", err)
	}

	// 3. Call OpenFGA to write the tuple
	err = s.authz.AssignTenantOwner(ctx, newTenant.ID, identityID)
	if err != nil {
		s.recordError(span, "failed to assign tenant owner in authz on registration", err,
			"tenant_id", newTenant.ID,
			"identity_id", identityID,
		)
		return fmt.Errorf("failed to assign tenant owner in authz: %w", err)
	}

	s.logger.Infow("tenant provisioned on registration",
		"tenant_id", newTenant.ID,
		"identity_id", identityID,
		"email", email,
	)
	s.logger.Security().AdminAction(identityID, "self_registration", "webhooks.Service.HandleRegistration", newTenant.ID)
	return nil
}

func (s *Service) HandleTokenHook(ctx context.Context, req *oauth2.TokenHookRequest) (*TokenHookResponse, error) {
	ctx, span := s.tracer.Start(ctx, "webhooks.Service.HandleTokenHook")
	defer span.End()

	// Determine User ID
	var userID string
	if req.Session != nil && req.Session.Subject != "" {
		userID = req.Session.Subject
	}

	s.logger.Debugw("handling token hook", "user_id", userID)

	if userID == "" {
		err := fmt.Errorf("could not identify user from request")
		s.recordError(span, "token hook request missing user subject", err)
		return nil, err
	}

	// Fetch Tenants
	tenants, err := s.storage.ListActiveTenantsByUserID(ctx, userID)
	if err != nil {
		s.recordError(span, "failed to list tenants for token hook", err, "user_id", userID)
		return nil, fmt.Errorf("failed to list tenants: %w", err)
	}

	// Format Response
	tenantList := make([]string, 0, len(tenants))
	for _, t := range tenants {
		tenantList = append(tenantList, t.ID)
	}

	s.logger.Debugw("token hook tenants resolved", "user_id", userID, "tenant_count", len(tenantList))

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

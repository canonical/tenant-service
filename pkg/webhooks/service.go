// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0

package webhooks

import (
	"context"
	"errors"
	"fmt"

	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/canonical/tenant-service/internal/logging"
	"github.com/canonical/tenant-service/internal/monitoring"
	"github.com/canonical/tenant-service/internal/storage"
	"github.com/canonical/tenant-service/internal/tracing"
	"github.com/canonical/tenant-service/internal/types"
	"github.com/ory/hydra/v2/oauth2"
)

// Service provides webhook business logic.
type Service struct {
	storage StorageInterface
	authz   AuthorizerInterface
	tracer  tracing.TracingInterface
	monitor monitoring.MonitorInterface
	logger  logging.LoggerInterface
}

// NewService creates a new webhook service.
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

	if email == "" {
		err := fmt.Errorf("identity email is required")
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		s.recordRegistrationMetric("webhook_registration_failure")
		return err
	}

	// 1. Create a tenant named '{Email}'s Org'
	tenantName := fmt.Sprintf("%s's Org", email)

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

	// Extract the tenant_id the Login UI placed in the session at the consent step.
	tenantID := s.extractTenantIDFromSession(req)

	resp := TokenHookResponse{
		Session: struct {
			IDToken     map[string]interface{} `json:"id_token,omitempty"`
			AccessToken map[string]interface{} `json:"access_token,omitempty"`
		}{
			IDToken:     map[string]interface{}{},
			AccessToken: map[string]interface{}{},
		},
	}

	if tenantID == "" {
		// No tenant was selected (e.g. user logged in without tenant context, pending activation).
		// Return a valid response without a tenant_id claim.
		s.logger.Debugw("token hook: no tenant_id in session, skipping tenant claim", "user_id", userID)
		return &resp, nil
	}

	// Validate that the user is still an active member of the requested tenant.
	_, err := s.storage.GetActiveMemberByTenantAndUserID(ctx, tenantID, userID)
	if err != nil {
		if isNotFound(err) {
			s.recordError(span, "token hook: user is not an active member of tenant", ErrNotMember,
				"user_id", userID,
				"tenant_id", tenantID,
			)
			return nil, ErrNotMember
		}
		s.recordError(span, "token hook: failed to validate tenant membership", err,
			"user_id", userID,
			"tenant_id", tenantID,
		)
		return nil, fmt.Errorf("failed to validate tenant membership: %w", err)
	}

	s.logger.Debugw("token hook: injecting tenant_id claim", "user_id", userID, "tenant_id", tenantID)
	resp.Session.IDToken["tenant_id"] = tenantID
	resp.Session.AccessToken["tenant_id"] = tenantID

	return &resp, nil
}

// HandleLoginHook validates that the given identity is permitted to log in.
//
// If tenantID is non-empty, it checks that the identity is an active member of
// that tenant, returning ErrNotMember if not.
//
// If tenantID is empty, the hook performs orphaned-identity reconciliation: if
// the identity has no memberships (registration webhook previously failed), it
// re-runs the registration logic to create a disabled shadow tenant and assign
// the user as owner. Login is always allowed when tenantID is absent.
func (s *Service) HandleLoginHook(ctx context.Context, identityID, email, tenantID string) error {
	ctx, span := s.tracer.Start(ctx, "webhooks.Service.HandleLoginHook")
	defer span.End()

	s.logger.Debugw("handling login hook",
		"identity_id", identityID,
		"tenant_id", tenantID,
	)

	if identityID == "" {
		// Empty identity_id means Kratos fired the hook for an intermediate authentication
		// step (before all factors are satisfied, flow.state != "passed_challenge"). The
		// Jsonnet template returns {} in that case. Treat this as a no-op.
		s.logger.Debugw("login hook: empty identity_id, skipping (intermediate auth step)")
		return nil
	}

	if tenantID != "" {
		// Verify active membership.
		_, err := s.storage.GetActiveMemberByTenantAndUserID(ctx, tenantID, identityID)
		if err != nil {
			if isNotFound(err) {
				s.recordError(span, "login hook: user is not an active member of tenant", ErrNotMember,
					"identity_id", identityID,
					"tenant_id", tenantID,
				)
				return ErrNotMember
			}
			s.recordError(span, "login hook: failed to check tenant membership", err,
				"identity_id", identityID,
				"tenant_id", tenantID,
			)
			return fmt.Errorf("failed to check tenant membership: %w", err)
		}

		s.logger.Debugw("login hook: membership verified", "identity_id", identityID, "tenant_id", tenantID)
		s.logger.Security().AdminAction(identityID, "login_with_tenant", "webhooks.Service.HandleLoginHook", tenantID)
		return nil
	}

	// No tenant selected — check for orphaned identity and reconcile if needed.
	hasMembership, err := s.storage.HasAnyMembership(ctx, identityID)
	if err != nil {
		s.recordError(span, "login hook: failed to check membership existence", err, "identity_id", identityID)
		return fmt.Errorf("failed to check membership: %w", err)
	}

	if !hasMembership {
		s.logger.Infow("login hook: orphaned identity detected, running lazy reconciliation",
			"identity_id", identityID,
			"email", email,
		)
		if err := s.HandleRegistration(ctx, identityID, email); err != nil {
			s.recordError(span, "login hook: lazy reconciliation failed", err, "identity_id", identityID)
			if counterErr := s.monitor.IncrementCounter(map[string]string{"operation": "login_reconciliation_error", "identity_id": identityID}); counterErr != nil {
				s.logger.Errorw("failed to increment reconciliation error counter", "error", counterErr)
			}
			return fmt.Errorf("failed to reconcile orphaned identity: %w", err)
		}
		s.logger.Infow("login hook: lazy reconciliation succeeded", "identity_id", identityID)
	}

	return nil
}

// isNotFound reports whether err is a storage "not found" sentinel.
func isNotFound(err error) bool {
	return errors.Is(err, storage.ErrNotFound)
}

// extractTenantIDFromSession retrieves the tenant_id safely from the OAuth2 session extra payload.
//
// The Login UI guarantees that the "_none" sentinel (cookies.NoTenantAvailable)
// is never forwarded to the consent session. If it were to arrive here, the
// membership check would fail (no tenant with id "_none" exists), resulting
// in a 403 — correct fail-closed behavior.
func (s *Service) extractTenantIDFromSession(req *oauth2.TokenHookRequest) string {
	if req.Session != nil && req.Session.Extra != nil {
		if v, ok := req.Session.Extra["_tenant_id"].(string); ok {
			return v
		}
	}
	return ""
}

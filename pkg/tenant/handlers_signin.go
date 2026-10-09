// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package tenant

import (
	"context"
	"net/mail"

	"buf.build/go/protovalidate"
	v0 "github.com/canonical/identity-platform-api/v0/tenant"
	"github.com/canonical/tenant-service/internal/logging"
	"github.com/canonical/tenant-service/internal/monitoring"
	"github.com/canonical/tenant-service/internal/tracing"
	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// SignInHandler implements TenantSignInService, which is served over gRPC only.
type SignInHandler struct {
	v0.UnimplementedTenantSignInServiceServer
	service   ServiceInterface
	tracer    tracing.TracingInterface
	monitor   monitoring.MonitorInterface
	logger    logging.LoggerInterface
	validator protovalidate.Validator
}

// NewSignInHandler creates a new sign-in API handler.
func NewSignInHandler(
	service ServiceInterface,
	validator protovalidate.Validator,
	tracer tracing.TracingInterface,
	monitor monitoring.MonitorInterface,
	logger logging.LoggerInterface,
) *SignInHandler {
	return &SignInHandler{
		service:   service,
		tracer:    tracer,
		monitor:   monitor,
		logger:    logger,
		validator: validator,
	}
}

func (h *SignInHandler) ListSignInTenants(ctx context.Context, req *v0.ListSignInTenantsRequest) (*v0.ListSignInTenantsResponse, error) {
	ctx, span := h.tracer.Start(ctx, "tenant.SignInHandler.ListSignInTenants")
	defer span.End()

	if err := h.validator.Validate(req); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid request: %v", err)
	}

	tenants, err := h.service.ListSignInTenants(ctx, req.Email)
	if err != nil {
		return nil, failed(h.logger, err, "failed to list sign-in tenants")
	}

	return &v0.ListSignInTenantsResponse{Tenants: signInTenantsToProto(tenants)}, nil
}

func (h *SignInHandler) GetSignInContext(ctx context.Context, req *v0.GetSignInContextRequest) (*v0.GetSignInContextResponse, error) {
	ctx, span := h.tracer.Start(ctx, "tenant.SignInHandler.GetSignInContext")
	defer span.End()

	if err := h.validator.Validate(req); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid request: %v", err)
	}
	if _, err := uuid.Parse(req.TenantId); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid tenant_id: must be a valid UUID")
	}
	if req.Email == "" && req.IdentityId == "" {
		return nil, status.Errorf(codes.InvalidArgument, "one of email or identity_id must be provided")
	}
	if req.IdentityId != "" {
		if _, err := uuid.Parse(req.IdentityId); err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "invalid identity_id: must be a valid UUID")
		}
	} else if _, err := mail.ParseAddress(req.Email); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid email: %v", err)
	}

	c, err := h.service.GetSignInContext(ctx, req.TenantId, req.Email, req.IdentityId)
	if err != nil {
		return nil, failed(h.logger, err, "failed to get sign-in context", "tenant_id", req.TenantId)
	}

	return &v0.GetSignInContextResponse{Context: signInContextToProto(c)}, nil
}

func (h *SignInHandler) JoinTenant(ctx context.Context, req *v0.JoinTenantRequest) (*v0.JoinTenantResponse, error) {
	ctx, span := h.tracer.Start(ctx, "tenant.SignInHandler.JoinTenant")
	defer span.End()

	if err := h.validator.Validate(req); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid request: %v", err)
	}
	if _, err := uuid.Parse(req.TenantId); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid tenant_id: must be a valid UUID")
	}
	if _, err := uuid.Parse(req.IdentityId); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid identity_id: must be a valid UUID")
	}

	if err := h.service.JoinTenant(ctx, req.TenantId, req.IdentityId); err != nil {
		return nil, failed(h.logger, err, "failed to join tenant",
			"tenant_id", req.TenantId,
			"identity_id", req.IdentityId,
		)
	}

	return &v0.JoinTenantResponse{}, nil
}

func (h *SignInHandler) CreatePersonalTenant(ctx context.Context, req *v0.CreatePersonalTenantRequest) (*v0.CreatePersonalTenantResponse, error) {
	ctx, span := h.tracer.Start(ctx, "tenant.SignInHandler.CreatePersonalTenant")
	defer span.End()

	if err := h.validator.Validate(req); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid request: %v", err)
	}
	if _, err := uuid.Parse(req.IdentityId); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid identity_id: must be a valid UUID")
	}

	tenant, _, err := h.service.CreatePersonalTenant(ctx, req.IdentityId, "")
	if err != nil {
		return nil, failed(h.logger, err, "failed to create personal tenant", "identity_id", req.IdentityId)
	}

	return &v0.CreatePersonalTenantResponse{Tenant: tenantToProto(tenant)}, nil
}

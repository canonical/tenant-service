// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package tenant

import (
	"context"
	"strings"

	"buf.build/go/protovalidate"
	v0 "github.com/canonical/identity-platform-api/v0/tenant"
	"github.com/canonical/tenant-service/internal/logging"
	"github.com/canonical/tenant-service/internal/monitoring"
	"github.com/canonical/tenant-service/internal/tracing"
	"github.com/canonical/tenant-service/internal/types"
	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// SSOPolicyHandler implements TenantSSOPolicyService, which is served over
// gRPC only.
type SSOPolicyHandler struct {
	v0.UnimplementedTenantSSOPolicyServiceServer
	service   ServiceInterface
	tracer    tracing.TracingInterface
	monitor   monitoring.MonitorInterface
	logger    logging.LoggerInterface
	validator protovalidate.Validator
}

// NewSSOPolicyHandler creates a new SSO policy API handler.
func NewSSOPolicyHandler(
	service ServiceInterface,
	validator protovalidate.Validator,
	tracer tracing.TracingInterface,
	monitor monitoring.MonitorInterface,
	logger logging.LoggerInterface,
) *SSOPolicyHandler {
	return &SSOPolicyHandler{
		service:   service,
		tracer:    tracer,
		monitor:   monitor,
		logger:    logger,
		validator: validator,
	}
}

func (h *SSOPolicyHandler) GetTenantSSOPolicy(ctx context.Context, req *v0.GetTenantSSOPolicyRequest) (*v0.GetTenantSSOPolicyResponse, error) {
	ctx, span := h.tracer.Start(ctx, "tenant.SSOPolicyHandler.GetTenantSSOPolicy")
	defer span.End()

	if err := h.validator.Validate(req); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid request: %v", err)
	}
	if _, err := uuid.Parse(req.TenantId); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid tenant_id: must be a valid UUID")
	}

	policy, err := h.service.GetTenantSSOPolicy(ctx, req.TenantId)
	if err != nil {
		return nil, failed(h.logger, err, "failed to get tenant sso policy", "tenant_id", req.TenantId)
	}

	return &v0.GetTenantSSOPolicyResponse{Policy: ssoPolicyToProto(policy)}, nil
}

func (h *SSOPolicyHandler) PutTenantSSOPolicy(ctx context.Context, req *v0.PutTenantSSOPolicyRequest) (*v0.PutTenantSSOPolicyResponse, error) {
	ctx, span := h.tracer.Start(ctx, "tenant.SSOPolicyHandler.PutTenantSSOPolicy")
	defer span.End()

	if err := h.validator.Validate(req); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid request: %v", err)
	}
	if _, err := uuid.Parse(req.TenantId); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid tenant_id: must be a valid UUID")
	}

	// Connection ids in lower case, as the database returns them.
	bindings := make([]types.SSOBinding, len(req.Bindings))
	for i, b := range req.Bindings {
		bindings[i] = types.SSOBinding{
			ConnectionID: strings.ToLower(b.GetConnectionId()),
			Active:       b.GetActive(),
		}
	}

	policy, err := h.service.PutTenantSSOPolicy(ctx, req.TenantId, enforcementFromProto(req.Enforcement), req.AutoJoin, bindings)
	if err != nil {
		return nil, failed(h.logger, err, "failed to write tenant sso policy", "tenant_id", req.TenantId)
	}

	return &v0.PutTenantSSOPolicyResponse{Policy: ssoPolicyToProto(policy)}, nil
}

func (h *SSOPolicyHandler) SetTenantSSODomains(ctx context.Context, req *v0.SetTenantSSODomainsRequest) (*v0.SetTenantSSODomainsResponse, error) {
	ctx, span := h.tracer.Start(ctx, "tenant.SSOPolicyHandler.SetTenantSSODomains")
	defer span.End()

	if err := h.validator.Validate(req); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid request: %v", err)
	}
	if _, err := uuid.Parse(req.TenantId); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid tenant_id: must be a valid UUID")
	}

	policy, err := h.service.SetTenantSSODomains(ctx, req.TenantId, req.Domains)
	if err != nil {
		return nil, failed(h.logger, err, "failed to set tenant sso domains", "tenant_id", req.TenantId)
	}

	return &v0.SetTenantSSODomainsResponse{Policy: ssoPolicyToProto(policy)}, nil
}

func (h *SSOPolicyHandler) RemoveTenantSSOBinding(ctx context.Context, req *v0.RemoveTenantSSOBindingRequest) (*v0.RemoveTenantSSOBindingResponse, error) {
	ctx, span := h.tracer.Start(ctx, "tenant.SSOPolicyHandler.RemoveTenantSSOBinding")
	defer span.End()

	if err := h.validator.Validate(req); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid request: %v", err)
	}
	if _, err := uuid.Parse(req.TenantId); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid tenant_id: must be a valid UUID")
	}

	if err := h.service.RemoveTenantSSOBinding(ctx, req.TenantId, strings.ToLower(req.ConnectionId)); err != nil {
		return nil, failed(h.logger, err, "failed to remove tenant sso binding", "tenant_id", req.TenantId, "connection_id", req.ConnectionId)
	}

	return &v0.RemoveTenantSSOBindingResponse{}, nil
}

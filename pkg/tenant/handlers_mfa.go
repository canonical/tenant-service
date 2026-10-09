// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package tenant

import (
	"context"

	v0 "github.com/canonical/identity-platform-api/v0/tenant"
	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (h *Handler) GetTenantMFAPolicy(ctx context.Context, req *v0.GetTenantMFAPolicyRequest) (*v0.GetTenantMFAPolicyResponse, error) {
	ctx, span := h.tracer.Start(ctx, "tenant.Handler.GetTenantMFAPolicy")
	defer span.End()

	if err := h.validator.Validate(req); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid request: %v", err)
	}
	if _, err := uuid.Parse(req.TenantId); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid tenant_id: must be a valid UUID")
	}

	policy, err := h.service.GetTenantMFAPolicy(ctx, req.TenantId)
	if err != nil {
		return nil, failed(h.logger, err, "failed to get tenant mfa policy", "tenant_id", req.TenantId)
	}

	return &v0.GetTenantMFAPolicyResponse{Policy: mfaPolicyToProto(policy)}, nil
}

func (h *Handler) PutTenantMFAPolicy(ctx context.Context, req *v0.PutTenantMFAPolicyRequest) (*v0.PutTenantMFAPolicyResponse, error) {
	ctx, span := h.tracer.Start(ctx, "tenant.Handler.PutTenantMFAPolicy")
	defer span.End()

	if err := h.validator.Validate(req); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid request: %v", err)
	}
	if _, err := uuid.Parse(req.TenantId); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid tenant_id: must be a valid UUID")
	}

	policy, err := h.service.PutTenantMFAPolicy(ctx, req.TenantId, mfaRequirementFromProto(req.Requirement))
	if err != nil {
		return nil, failed(h.logger, err, "failed to write tenant mfa policy", "tenant_id", req.TenantId)
	}

	return &v0.PutTenantMFAPolicyResponse{Policy: mfaPolicyToProto(policy)}, nil
}

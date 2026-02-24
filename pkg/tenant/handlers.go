// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0

package tenant

import (
	"context"

	"github.com/canonical/tenant-service/internal/logging"
	"github.com/canonical/tenant-service/internal/monitoring"
	"github.com/canonical/tenant-service/internal/tracing"
	"github.com/canonical/tenant-service/internal/types"
	"github.com/canonical/tenant-service/pkg/authentication"
	v0 "github.com/canonical/tenant-service/v0"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

type Handler struct {
	v0.UnimplementedTenantServiceServer
	service ServiceInterface
	tracer  tracing.TracingInterface
	monitor monitoring.MonitorInterface
	logger  logging.LoggerInterface
}

func NewHandler(
	service ServiceInterface,
	tracer tracing.TracingInterface,
	monitor monitoring.MonitorInterface,
	logger logging.LoggerInterface,
) *Handler {
	return &Handler{
		service: service,
		tracer:  tracer,
		monitor: monitor,
		logger:  logger,
	}
}

func (h *Handler) InviteMember(ctx context.Context, req *v0.InviteMemberRequest) (*v0.InviteMemberResponse, error) {
	ctx, span := h.tracer.Start(ctx, "tenant.Handler.InviteMember")
	defer span.End()

	if req.TenantId == "" || req.Email == "" || req.Role == "" {
		return nil, status.Error(codes.InvalidArgument, "tenant_id, email, and role are required")
	}

	link, code, err := h.service.InviteMember(ctx, req.TenantId, req.Email, req.Role)
	if err != nil {
		// In a real app, you might map specific error types to gRPC codes here
		return nil, status.Errorf(codes.Internal, "failed to invite member: %v", err)
	}

	return &v0.InviteMemberResponse{
		Status: "invited",
		Link:   link,
		Code:   code,
	}, nil
}

func (h *Handler) ListMyTenants(ctx context.Context, req *v0.ListMyTenantsRequest) (*v0.ListMyTenantsResponse, error) {
	ctx, span := h.tracer.Start(ctx, "tenant.Handler.ListMyTenants")
	defer span.End()

	// Extract user_id from context
	userID, ok := authentication.GetUserID(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "unauthenticated")
	}

	tenants, err := h.service.ListTenantsByUserID(ctx, userID)
	if err != nil {
		h.logger.Errorf("failed to list tenants: %v", err)
		return nil, status.Errorf(codes.Internal, "failed to list tenants: %v", err)
	}

	pbTenants := make([]*v0.Tenant, len(tenants))
	for i, t := range tenants {
		pbTenants[i] = &v0.Tenant{
			Id:        t.ID,
			Name:      t.Name,
			CreatedAt: t.CreatedAt.String(),
			Enabled:   t.Enabled,
		}
	}

	return &v0.ListMyTenantsResponse{
		Tenants: pbTenants,
	}, nil
}

func (h *Handler) ListTenants(ctx context.Context, req *v0.ListTenantsRequest) (*v0.ListTenantsResponse, error) {
	ctx, span := h.tracer.Start(ctx, "tenant.Handler.ListTenants")
	defer span.End()

	tenants, err := h.service.ListTenants(ctx)
	if err != nil {
		h.logger.Errorf("failed to list all tenants: %v", err)
		return nil, status.Errorf(codes.Internal, "failed to list all tenants: %v", err)
	}

	pbTenants := make([]*v0.Tenant, len(tenants))
	for i, t := range tenants {
		pbTenants[i] = &v0.Tenant{
			Id:        t.ID,
			Name:      t.Name,
			CreatedAt: t.CreatedAt.String(),
			Enabled:   t.Enabled,
		}
	}

	return &v0.ListTenantsResponse{
		Tenants: pbTenants,
	}, nil
}

func (h *Handler) CreateTenant(ctx context.Context, req *v0.CreateTenantRequest) (*v0.CreateTenantResponse, error) {
	ctx, span := h.tracer.Start(ctx, "tenant.Handler.CreateTenant")
	defer span.End()

	if req.Name == "" {
		return nil, status.Error(codes.InvalidArgument, "tenant name is required")
	}

	tenant, err := h.service.CreateTenant(ctx, req.Name)
	if err != nil {
		h.logger.Errorf("failed to create tenant: %v", err)
		return nil, status.Errorf(codes.Internal, "failed to create tenant: %v", err)
	}

	return &v0.CreateTenantResponse{
		Tenant: &v0.Tenant{
			Id:        tenant.ID,
			Name:      tenant.Name,
			CreatedAt: tenant.CreatedAt.String(),
			Enabled:   tenant.Enabled,
		},
	}, nil
}

func (h *Handler) UpdateTenant(ctx context.Context, req *v0.UpdateTenantRequest) (*v0.UpdateTenantResponse, error) {
	ctx, span := h.tracer.Start(ctx, "tenant.Handler.UpdateTenant")
	defer span.End()

	if req.Tenant == nil {
		return nil, status.Error(codes.InvalidArgument, "tenant body is required")
	}

	// If update_mask is provided, use it. Otherwise, assume full update (or at least name and enabled).
	var paths []string
	if req.UpdateMask != nil {
		paths = req.UpdateMask.Paths
	}

	updateData := &types.Tenant{
		ID:      req.Tenant.Id, // From URL usually
		Name:    req.Tenant.Name,
		Enabled: req.Tenant.Enabled,
	}

	tenant, err := h.service.UpdateTenant(ctx, updateData, paths)
	if err != nil {
		h.logger.Errorf("failed to update tenant: %v", err)
		return nil, status.Errorf(codes.Internal, "failed to update tenant: %v", err)
	}

	return &v0.UpdateTenantResponse{
		Tenant: &v0.Tenant{
			Id:        tenant.ID,
			Name:      tenant.Name,
			CreatedAt: tenant.CreatedAt.String(),
			Enabled:   tenant.Enabled,
		},
	}, nil
}

func (h *Handler) DeleteTenant(ctx context.Context, req *v0.DeleteTenantRequest) (*emptypb.Empty, error) {
	ctx, span := h.tracer.Start(ctx, "tenant.Handler.DeleteTenant")
	defer span.End()

	if err := h.service.DeleteTenant(ctx, req.TenantId); err != nil {
		h.logger.Errorf("failed to delete tenant: %v", err)
		return nil, status.Errorf(codes.Internal, "failed to delete tenant: %v", err)
	}

	return &emptypb.Empty{}, nil
}

func (h *Handler) ProvisionUser(ctx context.Context, req *v0.ProvisionUserRequest) (*v0.ProvisionUserResponse, error) {
	ctx, span := h.tracer.Start(ctx, "tenant.Handler.ProvisionUser")
	defer span.End()

	if err := h.service.ProvisionUser(ctx, req.TenantId, req.Email, req.Role); err != nil {
		h.logger.Errorf("failed to provision user: %v", err)
		return nil, status.Errorf(codes.Internal, "failed to provision user: %v", err)
	}

	return &v0.ProvisionUserResponse{
		Status: "provisioned",
	}, nil
}

func (h *Handler) UpdateTenantUser(ctx context.Context, req *v0.UpdateTenantUserRequest) (*v0.UpdateTenantUserResponse, error) {
	ctx, span := h.tracer.Start(ctx, "tenant.Handler.UpdateTenantUser")
	defer span.End()

	if req.TenantId == "" || req.UserId == "" || req.Role == "" {
		return nil, status.Error(codes.InvalidArgument, "tenant_id, user_id, and role are required")
	}

	user, err := h.service.UpdateTenantUser(ctx, req.TenantId, req.UserId, req.Role)
	if err != nil {
		h.logger.Errorf("failed to update tenant user: %v", err)
		return nil, status.Errorf(codes.Internal, "failed to update tenant user: %v", err)
	}

	return &v0.UpdateTenantUserResponse{
		User: &v0.TenantUser{
			UserId: user.UserID,
			Role:   user.Role,
			Email:  user.Email,
		},
	}, nil
}

func (h *Handler) ListUserTenants(ctx context.Context, req *v0.ListUserTenantsRequest) (*v0.ListUserTenantsResponse, error) {
	ctx, span := h.tracer.Start(ctx, "tenant.Handler.ListUserTenants")
	defer span.End()

	tenants, err := h.service.ListUserTenants(ctx, req.UserId)
	if err != nil {
		h.logger.Errorf("failed to list user tenants: %v", err)
		return nil, status.Errorf(codes.Internal, "failed to list user tenants: %v", err)
	}

	pbTenants := make([]*v0.Tenant, len(tenants))
	for i, t := range tenants {
		pbTenants[i] = &v0.Tenant{
			Id:        t.ID,
			Name:      t.Name,
			CreatedAt: t.CreatedAt.String(),
			Enabled:   t.Enabled,
		}
	}

	return &v0.ListUserTenantsResponse{
		Tenants: pbTenants,
	}, nil
}

func (h *Handler) ListTenantUsers(ctx context.Context, req *v0.ListTenantUsersRequest) (*v0.ListTenantUsersResponse, error) {
	ctx, span := h.tracer.Start(ctx, "tenant.Handler.ListTenantUsers")
	defer span.End()

	users, err := h.service.ListTenantUsers(ctx, req.TenantId)
	if err != nil {
		h.logger.Errorf("failed to list tenant users: %v", err)
		return nil, status.Errorf(codes.Internal, "failed to list tenant users: %v", err)
	}

	pbUsers := make([]*v0.TenantUser, len(users))
	for i, u := range users {
		pbUsers[i] = &v0.TenantUser{
			UserId: u.UserID,
			Email:  u.Email,
			Role:   u.Role,
		}
	}

	return &v0.ListTenantUsersResponse{
		Users: pbUsers,
	}, nil
}

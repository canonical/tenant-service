// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package tenant

import (
	"context"
	"errors"
	"net/mail"
	"strings"
	"time"

	"buf.build/go/protovalidate"
	v0 "github.com/canonical/identity-platform-api/v0/tenant"
	"github.com/canonical/tenant-service/internal/logging"
	"github.com/canonical/tenant-service/internal/monitoring"
	"github.com/canonical/tenant-service/internal/storage"
	"github.com/canonical/tenant-service/internal/tracing"
	"github.com/canonical/tenant-service/internal/types"
	"github.com/canonical/tenant-service/pkg/authentication"
	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func tenantToProto(t *types.Tenant) *v0.Tenant {
	return &v0.Tenant{
		Id:        t.ID,
		Name:      t.Name,
		CreatedAt: t.CreatedAt.Format(time.RFC3339),
		Enabled:   t.Enabled,
	}
}

func tenantsToProto(tenants []*types.Tenant) []*v0.Tenant {
	pb := make([]*v0.Tenant, len(tenants))
	for i, t := range tenants {
		pb[i] = tenantToProto(t)
	}
	return pb
}

// Handler implements the gRPC and HTTP API endpoints.
type Handler struct {
	v0.UnimplementedTenantServiceServer
	service   ServiceInterface
	tracer    tracing.TracingInterface
	monitor   monitoring.MonitorInterface
	logger    logging.LoggerInterface
	validator protovalidate.Validator
}

// NewHandler creates a new tenant API handler.
func NewHandler(
	service ServiceInterface,
	validator protovalidate.Validator,
	tracer tracing.TracingInterface,
	monitor monitoring.MonitorInterface,
	logger logging.LoggerInterface,
) *Handler {
	return &Handler{
		service:   service,
		tracer:    tracer,
		monitor:   monitor,
		logger:    logger,
		validator: validator,
	}
}

func (h *Handler) InviteMember(ctx context.Context, req *v0.InviteMemberRequest) (*v0.InviteMemberResponse, error) {
	ctx, span := h.tracer.Start(ctx, "tenant.Handler.InviteMember")
	defer span.End()

	if err := h.validator.Validate(req); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid request: %v", err)
	}
	if _, err := uuid.Parse(req.TenantId); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid tenant_id: must be a valid UUID")
	}
	if _, err := mail.ParseAddress(req.Email); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid email: %v", err)
	}
	if strings.TrimSpace(req.Role) == "" {
		return nil, status.Errorf(codes.InvalidArgument, "role is required")
	}

	link, code, err := h.service.InviteMember(ctx, req.TenantId, req.Email, req.Role)
	if err != nil {
		h.logger.Errorw("failed to invite member",
			"tenant_id", req.TenantId,
			"email", req.Email,
			"role", req.Role,
			"error", err,
		)
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

	if err := h.validator.Validate(req); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid request: %v", err)
	}

	// Extract user_id from context
	userID, ok := authentication.GetUserID(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "unauthenticated")
	}

	opts := []types.ListOption{}
	if req.Enabled != nil {
		opts = append(opts, types.WithEnabled(*req.Enabled))
	}
	tenants, err := h.service.ListTenantsByUserID(ctx, userID, opts...)
	if err != nil {
		h.logger.Errorw("failed to list tenants", "user_id", userID, "error", err)
		return nil, status.Errorf(codes.Internal, "failed to list tenants: %v", err)
	}

	return &v0.ListMyTenantsResponse{
		Tenants: tenantsToProto(tenants),
	}, nil
}

func (h *Handler) ListTenants(ctx context.Context, req *v0.ListTenantsRequest) (*v0.ListTenantsResponse, error) {
	ctx, span := h.tracer.Start(ctx, "tenant.Handler.ListTenants")
	defer span.End()

	if err := h.validator.Validate(req); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid request: %v", err)
	}

	opts := []types.ListOption{types.WithPageToken(req.PageToken), types.WithPageSize(req.PageSize)}
	if req.Enabled != nil {
		opts = append(opts, types.WithEnabled(*req.Enabled))
	}
	tenants, nextPageToken, err := h.service.ListTenants(ctx, opts...)
	if err != nil {
		if errors.Is(err, storage.ErrInvalidPageToken) {
			return nil, status.Errorf(codes.InvalidArgument, "invalid page token")
		}
		h.logger.Errorw("failed to list all tenants", "error", err)
		return nil, status.Errorf(codes.Internal, "failed to list all tenants: %v", err)
	}

	return &v0.ListTenantsResponse{
		Tenants:       tenantsToProto(tenants),
		NextPageToken: nextPageToken,
	}, nil
}

func (h *Handler) CreateTenant(ctx context.Context, req *v0.CreateTenantRequest) (*v0.CreateTenantResponse, error) {
	ctx, span := h.tracer.Start(ctx, "tenant.Handler.CreateTenant")
	defer span.End()

	if err := h.validator.Validate(req); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid request: %v", err)
	}
	if strings.TrimSpace(req.Name) == "" {
		return nil, status.Errorf(codes.InvalidArgument, "name is required")
	}

	tenant, err := h.service.CreateTenant(ctx, req.Name)
	if err != nil {
		h.logger.Errorw("failed to create tenant", "name", req.Name, "error", err)
		return nil, status.Errorf(codes.Internal, "failed to create tenant: %v", err)
	}

	return &v0.CreateTenantResponse{
		Tenant: tenantToProto(tenant),
	}, nil
}

func (h *Handler) UpdateTenant(ctx context.Context, req *v0.UpdateTenantRequest) (*v0.UpdateTenantResponse, error) {
	ctx, span := h.tracer.Start(ctx, "tenant.Handler.UpdateTenant")
	defer span.End()

	if err := h.validator.Validate(req); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid request: %v", err)
	}
	if req.Tenant == nil {
		return nil, status.Errorf(codes.InvalidArgument, "tenant is required")
	}

	// If update_mask is provided, use it. Otherwise, assume full update (or at least name and enabled).
	var paths []string
	if req.UpdateMask != nil {
		paths = req.UpdateMask.Paths
	}

	updateData := &types.Tenant{
		ID:      req.TenantId,
		Name:    req.Tenant.Name,
		Enabled: *req.Tenant.Enabled,
	}

	tenant, err := h.service.UpdateTenant(ctx, updateData, paths)
	if err != nil {
		h.logger.Errorw("failed to update tenant", "tenant_id", req.TenantId, "error", err)
		return nil, status.Errorf(codes.Internal, "failed to update tenant: %v", err)
	}

	return &v0.UpdateTenantResponse{
		Tenant: tenantToProto(tenant),
	}, nil
}

func (h *Handler) DeleteTenant(ctx context.Context, req *v0.DeleteTenantRequest) (*v0.DeleteTenantResponse, error) {
	ctx, span := h.tracer.Start(ctx, "tenant.Handler.DeleteTenant")
	defer span.End()

	if err := h.validator.Validate(req); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid request: %v", err)
	}
	if _, err := uuid.Parse(req.TenantId); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid tenant_id: must be a valid UUID")
	}

	if err := h.service.DeleteTenant(ctx, req.TenantId); err != nil {
		h.logger.Errorw("failed to delete tenant", "tenant_id", req.TenantId, "error", err)
		return nil, status.Errorf(codes.Internal, "failed to delete tenant: %v", err)
	}

	return &v0.DeleteTenantResponse{Status: 0}, nil
}

func (h *Handler) ProvisionUser(ctx context.Context, req *v0.ProvisionUserRequest) (*v0.ProvisionUserResponse, error) {
	ctx, span := h.tracer.Start(ctx, "tenant.Handler.ProvisionUser")
	defer span.End()

	if err := h.validator.Validate(req); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid request: %v", err)
	}
	if _, err := uuid.Parse(req.TenantId); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid tenant_id: must be a valid UUID")
	}

	if err := h.service.ProvisionUser(ctx, req.TenantId, req.Email, req.Role); err != nil {
		h.logger.Errorw("failed to provision user",
			"tenant_id", req.TenantId,
			"email", req.Email,
			"role", req.Role,
			"error", err,
		)
		return nil, status.Errorf(codes.Internal, "failed to provision user: %v", err)
	}

	return &v0.ProvisionUserResponse{
		Status: "provisioned",
	}, nil
}

func (h *Handler) UpdateTenantUser(ctx context.Context, req *v0.UpdateTenantUserRequest) (*v0.UpdateTenantUserResponse, error) {
	ctx, span := h.tracer.Start(ctx, "tenant.Handler.UpdateTenantUser")
	defer span.End()

	if err := h.validator.Validate(req); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid request: %v", err)
	}
	if _, err := uuid.Parse(req.TenantId); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid tenant_id: must be a valid UUID")
	}
	if _, err := uuid.Parse(req.UserId); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid user_id: must be a valid UUID")
	}

	user, err := h.service.UpdateTenantUser(ctx, req.TenantId, req.UserId, req.Role)
	if err != nil {
		h.logger.Errorw("failed to update tenant user",
			"tenant_id", req.TenantId,
			"user_id", req.UserId,
			"role", req.Role,
			"error", err,
		)
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

	if err := h.validator.Validate(req); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid request: %v", err)
	}
	if _, err := uuid.Parse(req.UserId); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid user_id: must be a valid UUID")
	}

	opts := []types.ListOption{}
	if req.Enabled != nil {
		opts = append(opts, types.WithEnabled(*req.Enabled))
	}
	tenants, err := h.service.ListTenantsByUserID(ctx, req.UserId, opts...)
	if err != nil {
		h.logger.Errorw("failed to list user tenants", "user_id", req.UserId, "error", err)
		return nil, status.Errorf(codes.Internal, "failed to list user tenants: %v", err)
	}

	return &v0.ListUserTenantsResponse{
		Tenants: tenantsToProto(tenants),
	}, nil
}

func (h *Handler) ListTenantUsers(ctx context.Context, req *v0.ListTenantUsersRequest) (*v0.ListTenantUsersResponse, error) {
	ctx, span := h.tracer.Start(ctx, "tenant.Handler.ListTenantUsers")
	defer span.End()

	if err := h.validator.Validate(req); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid request: %v", err)
	}
	if _, err := uuid.Parse(req.TenantId); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid tenant_id: must be a valid UUID")
	}

	opts := []types.ListOption{types.WithPageToken(req.PageToken), types.WithPageSize(req.PageSize)}
	if req.Role != nil {
		opts = append(opts, types.WithRole(*req.Role))
	}
	if req.IdentityId != nil {
		opts = append(opts, types.WithIdentityID(*req.IdentityId))
	} else if req.Email != nil {
		opts = append(opts, types.WithEmail(*req.Email))
	}
	users, nextPageToken, err := h.service.ListTenantUsers(ctx, req.TenantId, req.GetIncludeEmails(), opts...)
	if err != nil {
		if errors.Is(err, storage.ErrInvalidPageToken) {
			return nil, status.Errorf(codes.InvalidArgument, "invalid page token")
		}
		h.logger.Errorw("failed to list tenant users", "tenant_id", req.TenantId, "error", err)
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
		Users:         pbUsers,
		NextPageToken: nextPageToken,
	}, nil
}

func (h *Handler) LookupTenants(ctx context.Context, req *v0.LookupTenantsRequest) (*v0.LookupTenantsResponse, error) {
	ctx, span := h.tracer.Start(ctx, "tenant.Handler.LookupTenants")
	defer span.End()

	if req.Email != "" && req.IdentityId != "" {
		return nil, status.Errorf(codes.InvalidArgument, "exactly one of email or identity_id must be provided")
	}
	if req.Email == "" && req.IdentityId == "" {
		return nil, status.Errorf(codes.InvalidArgument, "one of email or identity_id must be provided")
	}
	if req.Email != "" {
		if _, err := mail.ParseAddress(req.Email); err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "invalid email: %v", err)
		}
	}
	if req.IdentityId != "" {
		if _, err := uuid.Parse(req.IdentityId); err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "invalid identity_id: must be a valid UUID")
		}
	}

	var (
		tenants []*types.Tenant
		err     error
	)
	if req.IdentityId != "" {
		tenants, err = h.service.LookupTenantsByIdentityID(ctx, req.IdentityId)
	} else {
		tenants, err = h.service.LookupTenantsByEmail(ctx, req.Email)
	}
	if err != nil {
		h.logger.Errorw("failed to look up tenants", "error", err)
		return nil, status.Errorf(codes.Internal, "failed to look up tenants")
	}

	return &v0.LookupTenantsResponse{
		Tenants: tenantsToProto(tenants),
	}, nil
}

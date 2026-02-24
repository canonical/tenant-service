// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0

package tenant

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/canonical/tenant-service/internal/types"
	"github.com/canonical/tenant-service/pkg/authentication"
	v0 "github.com/canonical/tenant-service/v0"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

//go:generate mockgen -build_flags=--mod=mod -package tenant -destination ./mock_tenant.go -source=./interfaces.go
//go:generate mockgen -build_flags=--mod=mod -package tenant -destination ./mock_logger.go -source=../../internal/logging/interfaces.go
//go:generate mockgen -build_flags=--mod=mod -package tenant -destination ./mock_monitor.go -source=../../internal/monitoring/interfaces.go
//go:generate mockgen -build_flags=--mod=mod -package tenant -destination ./mock_tracing.go -source=../../internal/tracing/interfaces.go

func TestHandler_InviteMember(t *testing.T) {
	tests := []struct {
		name       string
		request    *v0.InviteMemberRequest
		setupMocks func(*MockServiceInterface)
		wantErr    bool
		wantCode   codes.Code
	}{
		{
			name: "success",
			request: &v0.InviteMemberRequest{
				TenantId: "tenant-123",
				Email:    "user@example.com",
				Role:     "member",
			},
			setupMocks: func(mockSvc *MockServiceInterface) {
				mockSvc.EXPECT().InviteMember(gomock.Any(), "tenant-123", "user@example.com", "member").
					Return("https://link", "code123", nil)
			},
			wantErr: false,
		},
		{
			name: "missing tenant_id",
			request: &v0.InviteMemberRequest{
				Email: "user@example.com",
				Role:  "member",
			},
			setupMocks: func(mockSvc *MockServiceInterface) {},
			wantErr:    true,
			wantCode:   codes.InvalidArgument,
		},
		{
			name: "missing email",
			request: &v0.InviteMemberRequest{
				TenantId: "tenant-123",
				Role:     "member",
			},
			setupMocks: func(mockSvc *MockServiceInterface) {},
			wantErr:    true,
			wantCode:   codes.InvalidArgument,
		},
		{
			name: "service error",
			request: &v0.InviteMemberRequest{
				TenantId: "tenant-123",
				Email:    "user@example.com",
				Role:     "member",
			},
			setupMocks: func(mockSvc *MockServiceInterface) {
				mockSvc.EXPECT().InviteMember(gomock.Any(), "tenant-123", "user@example.com", "member").
					Return("", "", errors.New("service error"))
			},
			wantErr:  true,
			wantCode: codes.Internal,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockSvc := NewMockServiceInterface(ctrl)
			mockTracer := NewMockTracingInterface(ctrl)
			mockLogger := NewMockLoggerInterface(ctrl)
			mockMonitor := NewMockMonitorInterface(ctrl)

			h := NewHandler(mockSvc, mockTracer, mockMonitor, mockLogger)

			mockTracer.EXPECT().Start(gomock.Any(), "tenant.Handler.InviteMember").
				Return(context.Background(), trace.SpanFromContext(context.Background()))
			tt.setupMocks(mockSvc)

			resp, err := h.InviteMember(context.Background(), tt.request)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error but got none")
				}
				st, ok := status.FromError(err)
				if ok && st.Code() != tt.wantCode {
					t.Errorf("expected code %v, got %v", tt.wantCode, st.Code())
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if resp == nil {
					t.Error("expected response but got nil")
				}
			}
		})
	}
}

func TestHandler_ListMyTenants(t *testing.T) {
	now := time.Now()
	tenants := []*types.Tenant{
		{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now, Enabled: true},
		{ID: "tenant-2", Name: "Tenant 2", CreatedAt: now, Enabled: false},
	}

	tests := []struct {
		name       string
		ctx        context.Context
		setupMocks func(*MockServiceInterface, *MockLoggerInterface)
		wantErr    bool
		wantCode   codes.Code
	}{
		{
			name: "success",
			ctx:  authentication.WithUserID(context.Background(), "user-123"),
			setupMocks: func(mockSvc *MockServiceInterface, mockLogger *MockLoggerInterface) {
				mockSvc.EXPECT().ListTenantsByUserID(gomock.Any(), "user-123").Return(tenants, nil)
			},
			wantErr: false,
		},
		{
			name:       "unauthenticated",
			ctx:        context.Background(),
			setupMocks: func(mockSvc *MockServiceInterface, mockLogger *MockLoggerInterface) {},
			wantErr:    true,
			wantCode:   codes.Unauthenticated,
		},
		{
			name: "service error",
			ctx:  authentication.WithUserID(context.Background(), "user-123"),
			setupMocks: func(mockSvc *MockServiceInterface, mockLogger *MockLoggerInterface) {
				mockSvc.EXPECT().ListTenantsByUserID(gomock.Any(), "user-123").Return(nil, errors.New("service error"))
				mockLogger.EXPECT().Errorf(gomock.Any(), gomock.Any())
			},
			wantErr:  true,
			wantCode: codes.Internal,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockSvc := NewMockServiceInterface(ctrl)
			mockTracer := NewMockTracingInterface(ctrl)
			mockLogger := NewMockLoggerInterface(ctrl)
			mockMonitor := NewMockMonitorInterface(ctrl)

			h := NewHandler(mockSvc, mockTracer, mockMonitor, mockLogger)

			mockTracer.EXPECT().Start(gomock.Any(), "tenant.Handler.ListMyTenants").
				Return(tt.ctx, trace.SpanFromContext(tt.ctx))
			tt.setupMocks(mockSvc, mockLogger)

			resp, err := h.ListMyTenants(tt.ctx, &v0.ListMyTenantsRequest{})

			if tt.wantErr {
				if err == nil {
					t.Error("expected error but got none")
				}
				st, ok := status.FromError(err)
				if ok && st.Code() != tt.wantCode {
					t.Errorf("expected code %v, got %v", tt.wantCode, st.Code())
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if resp == nil || len(resp.Tenants) != len(tenants) {
					t.Errorf("expected %d tenants, got %v", len(tenants), resp)
				}
			}
		})
	}
}

func TestHandler_ListTenants(t *testing.T) {
	now := time.Now()
	tenants := []*types.Tenant{
		{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now, Enabled: true},
	}

	tests := []struct {
		name       string
		setupMocks func(*MockServiceInterface, *MockLoggerInterface)
		wantErr    bool
	}{
		{
			name: "success",
			setupMocks: func(mockSvc *MockServiceInterface, mockLogger *MockLoggerInterface) {
				mockSvc.EXPECT().ListTenants(gomock.Any()).Return(tenants, nil)
			},
			wantErr: false,
		},
		{
			name: "service error",
			setupMocks: func(mockSvc *MockServiceInterface, mockLogger *MockLoggerInterface) {
				mockSvc.EXPECT().ListTenants(gomock.Any()).Return(nil, errors.New("service error"))
				mockLogger.EXPECT().Errorf(gomock.Any(), gomock.Any())
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockSvc := NewMockServiceInterface(ctrl)
			mockTracer := NewMockTracingInterface(ctrl)
			mockLogger := NewMockLoggerInterface(ctrl)
			mockMonitor := NewMockMonitorInterface(ctrl)

			h := NewHandler(mockSvc, mockTracer, mockMonitor, mockLogger)

			mockTracer.EXPECT().Start(gomock.Any(), "tenant.Handler.ListTenants").
				Return(context.Background(), trace.SpanFromContext(context.Background()))
			tt.setupMocks(mockSvc, mockLogger)

			resp, err := h.ListTenants(context.Background(), &v0.ListTenantsRequest{})

			if tt.wantErr {
				if err == nil {
					t.Error("expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if resp == nil {
					t.Error("expected response but got nil")
				}
			}
		})
	}
}

func TestHandler_CreateTenant(t *testing.T) {
	now := time.Now()
	tenant := &types.Tenant{ID: "tenant-123", Name: "Test Tenant", CreatedAt: now, Enabled: true}

	tests := []struct {
		name       string
		request    *v0.CreateTenantRequest
		setupMocks func(*MockServiceInterface, *MockLoggerInterface)
		wantErr    bool
		wantCode   codes.Code
	}{
		{
			name:    "success",
			request: &v0.CreateTenantRequest{Name: "Test Tenant"},
			setupMocks: func(mockSvc *MockServiceInterface, mockLogger *MockLoggerInterface) {
				mockSvc.EXPECT().CreateTenant(gomock.Any(), "Test Tenant").Return(tenant, nil)
			},
			wantErr: false,
		},
		{
			name:       "missing name",
			request:    &v0.CreateTenantRequest{},
			setupMocks: func(mockSvc *MockServiceInterface, mockLogger *MockLoggerInterface) {},
			wantErr:    true,
			wantCode:   codes.InvalidArgument,
		},
		{
			name:    "service error",
			request: &v0.CreateTenantRequest{Name: "Test Tenant"},
			setupMocks: func(mockSvc *MockServiceInterface, mockLogger *MockLoggerInterface) {
				mockSvc.EXPECT().CreateTenant(gomock.Any(), "Test Tenant").Return(nil, errors.New("service error"))
				mockLogger.EXPECT().Errorf(gomock.Any(), gomock.Any())
			},
			wantErr:  true,
			wantCode: codes.Internal,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockSvc := NewMockServiceInterface(ctrl)
			mockTracer := NewMockTracingInterface(ctrl)
			mockLogger := NewMockLoggerInterface(ctrl)
			mockMonitor := NewMockMonitorInterface(ctrl)

			h := NewHandler(mockSvc, mockTracer, mockMonitor, mockLogger)

			mockTracer.EXPECT().Start(gomock.Any(), "tenant.Handler.CreateTenant").
				Return(context.Background(), trace.SpanFromContext(context.Background()))
			tt.setupMocks(mockSvc, mockLogger)

			resp, err := h.CreateTenant(context.Background(), tt.request)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error but got none")
				}
				st, ok := status.FromError(err)
				if ok && st.Code() != tt.wantCode {
					t.Errorf("expected code %v, got %v", tt.wantCode, st.Code())
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if resp == nil {
					t.Error("expected response but got nil")
				}
			}
		})
	}
}

func TestHandler_UpdateTenant(t *testing.T) {
	now := time.Now()
	tenant := &types.Tenant{ID: "tenant-123", Name: "Updated", CreatedAt: now, Enabled: true}

	tests := []struct {
		name       string
		request    *v0.UpdateTenantRequest
		setupMocks func(*MockServiceInterface, *MockLoggerInterface)
		wantErr    bool
		wantCode   codes.Code
	}{
		{
			name: "success",
			request: &v0.UpdateTenantRequest{
				Tenant:     &v0.Tenant{Id: "tenant-123", Name: "Updated", Enabled: true},
				UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"name"}},
			},
			setupMocks: func(mockSvc *MockServiceInterface, mockLogger *MockLoggerInterface) {
				mockSvc.EXPECT().UpdateTenant(gomock.Any(), gomock.Any(), []string{"name"}).Return(tenant, nil)
			},
			wantErr: false,
		},
		{
			name:       "missing tenant",
			request:    &v0.UpdateTenantRequest{},
			setupMocks: func(mockSvc *MockServiceInterface, mockLogger *MockLoggerInterface) {},
			wantErr:    true,
			wantCode:   codes.InvalidArgument,
		},
		{
			name: "service error",
			request: &v0.UpdateTenantRequest{
				Tenant: &v0.Tenant{Id: "tenant-123", Name: "Updated"},
			},
			setupMocks: func(mockSvc *MockServiceInterface, mockLogger *MockLoggerInterface) {
				mockSvc.EXPECT().UpdateTenant(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, errors.New("service error"))
				mockLogger.EXPECT().Errorf(gomock.Any(), gomock.Any())
			},
			wantErr:  true,
			wantCode: codes.Internal,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockSvc := NewMockServiceInterface(ctrl)
			mockTracer := NewMockTracingInterface(ctrl)
			mockLogger := NewMockLoggerInterface(ctrl)
			mockMonitor := NewMockMonitorInterface(ctrl)

			h := NewHandler(mockSvc, mockTracer, mockMonitor, mockLogger)

			mockTracer.EXPECT().Start(gomock.Any(), "tenant.Handler.UpdateTenant").
				Return(context.Background(), trace.SpanFromContext(context.Background()))
			tt.setupMocks(mockSvc, mockLogger)

			resp, err := h.UpdateTenant(context.Background(), tt.request)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error but got none")
				}
				st, ok := status.FromError(err)
				if ok && st.Code() != tt.wantCode {
					t.Errorf("expected code %v, got %v", tt.wantCode, st.Code())
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if resp == nil {
					t.Error("expected response but got nil")
				}
			}
		})
	}
}

func TestHandler_DeleteTenant(t *testing.T) {
	tests := []struct {
		name       string
		request    *v0.DeleteTenantRequest
		setupMocks func(*MockServiceInterface, *MockLoggerInterface)
		wantErr    bool
	}{
		{
			name:    "success",
			request: &v0.DeleteTenantRequest{TenantId: "tenant-123"},
			setupMocks: func(mockSvc *MockServiceInterface, mockLogger *MockLoggerInterface) {
				mockSvc.EXPECT().DeleteTenant(gomock.Any(), "tenant-123").Return(nil)
			},
			wantErr: false,
		},
		{
			name:    "service error",
			request: &v0.DeleteTenantRequest{TenantId: "tenant-123"},
			setupMocks: func(mockSvc *MockServiceInterface, mockLogger *MockLoggerInterface) {
				mockSvc.EXPECT().DeleteTenant(gomock.Any(), "tenant-123").Return(errors.New("service error"))
				mockLogger.EXPECT().Errorf(gomock.Any(), gomock.Any())
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockSvc := NewMockServiceInterface(ctrl)
			mockTracer := NewMockTracingInterface(ctrl)
			mockLogger := NewMockLoggerInterface(ctrl)
			mockMonitor := NewMockMonitorInterface(ctrl)

			h := NewHandler(mockSvc, mockTracer, mockMonitor, mockLogger)

			mockTracer.EXPECT().Start(gomock.Any(), "tenant.Handler.DeleteTenant").
				Return(context.Background(), trace.SpanFromContext(context.Background()))
			tt.setupMocks(mockSvc, mockLogger)

			_, err := h.DeleteTenant(context.Background(), tt.request)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error but got none")
				}
			} else if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestHandler_ProvisionUser(t *testing.T) {
	tests := []struct {
		name       string
		request    *v0.ProvisionUserRequest
		setupMocks func(*MockServiceInterface, *MockLoggerInterface)
		wantErr    bool
	}{
		{
			name: "success",
			request: &v0.ProvisionUserRequest{
				TenantId: "tenant-123",
				Email:    "user@example.com",
				Role:     "member",
			},
			setupMocks: func(mockSvc *MockServiceInterface, mockLogger *MockLoggerInterface) {
				mockSvc.EXPECT().ProvisionUser(gomock.Any(), "tenant-123", "user@example.com", "member").Return(nil)
			},
			wantErr: false,
		},
		{
			name: "service error",
			request: &v0.ProvisionUserRequest{
				TenantId: "tenant-123",
				Email:    "user@example.com",
				Role:     "member",
			},
			setupMocks: func(mockSvc *MockServiceInterface, mockLogger *MockLoggerInterface) {
				mockSvc.EXPECT().ProvisionUser(gomock.Any(), "tenant-123", "user@example.com", "member").
					Return(errors.New("service error"))
				mockLogger.EXPECT().Errorf(gomock.Any(), gomock.Any())
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockSvc := NewMockServiceInterface(ctrl)
			mockTracer := NewMockTracingInterface(ctrl)
			mockLogger := NewMockLoggerInterface(ctrl)
			mockMonitor := NewMockMonitorInterface(ctrl)

			h := NewHandler(mockSvc, mockTracer, mockMonitor, mockLogger)

			mockTracer.EXPECT().Start(gomock.Any(), "tenant.Handler.ProvisionUser").
				Return(context.Background(), trace.SpanFromContext(context.Background()))
			tt.setupMocks(mockSvc, mockLogger)

			resp, err := h.ProvisionUser(context.Background(), tt.request)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if resp == nil || resp.Status != "provisioned" {
					t.Error("expected provisioned status")
				}
			}
		})
	}
}

func TestHandler_UpdateTenantUser(t *testing.T) {
	user := &types.TenantUser{UserID: "user-123", Email: "user@example.com", Role: "owner"}

	tests := []struct {
		name       string
		request    *v0.UpdateTenantUserRequest
		setupMocks func(*MockServiceInterface, *MockLoggerInterface)
		wantErr    bool
		wantCode   codes.Code
	}{
		{
			name: "success",
			request: &v0.UpdateTenantUserRequest{
				TenantId: "tenant-123",
				UserId:   "user-123",
				Role:     "owner",
			},
			setupMocks: func(mockSvc *MockServiceInterface, mockLogger *MockLoggerInterface) {
				mockSvc.EXPECT().UpdateTenantUser(gomock.Any(), "tenant-123", "user-123", "owner").Return(user, nil)
			},
			wantErr: false,
		},
		{
			name: "missing tenant_id",
			request: &v0.UpdateTenantUserRequest{
				UserId: "user-123",
				Role:   "owner",
			},
			setupMocks: func(mockSvc *MockServiceInterface, mockLogger *MockLoggerInterface) {},
			wantErr:    true,
			wantCode:   codes.InvalidArgument,
		},
		{
			name: "service error",
			request: &v0.UpdateTenantUserRequest{
				TenantId: "tenant-123",
				UserId:   "user-123",
				Role:     "owner",
			},
			setupMocks: func(mockSvc *MockServiceInterface, mockLogger *MockLoggerInterface) {
				mockSvc.EXPECT().UpdateTenantUser(gomock.Any(), "tenant-123", "user-123", "owner").
					Return(nil, errors.New("service error"))
				mockLogger.EXPECT().Errorf(gomock.Any(), gomock.Any())
			},
			wantErr:  true,
			wantCode: codes.Internal,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockSvc := NewMockServiceInterface(ctrl)
			mockTracer := NewMockTracingInterface(ctrl)
			mockLogger := NewMockLoggerInterface(ctrl)
			mockMonitor := NewMockMonitorInterface(ctrl)

			h := NewHandler(mockSvc, mockTracer, mockMonitor, mockLogger)

			mockTracer.EXPECT().Start(gomock.Any(), "tenant.Handler.UpdateTenantUser").
				Return(context.Background(), trace.SpanFromContext(context.Background()))
			tt.setupMocks(mockSvc, mockLogger)

			resp, err := h.UpdateTenantUser(context.Background(), tt.request)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error but got none")
				}
				st, ok := status.FromError(err)
				if ok && st.Code() != tt.wantCode {
					t.Errorf("expected code %v, got %v", tt.wantCode, st.Code())
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if resp == nil {
					t.Error("expected response but got nil")
				}
			}
		})
	}
}

func TestHandler_ListUserTenants(t *testing.T) {
	now := time.Now()
	tenants := []*types.Tenant{
		{ID: "tenant-1", Name: "Tenant 1", CreatedAt: now, Enabled: true},
	}

	tests := []struct {
		name       string
		request    *v0.ListUserTenantsRequest
		setupMocks func(*MockServiceInterface, *MockLoggerInterface)
		wantErr    bool
	}{
		{
			name:    "success",
			request: &v0.ListUserTenantsRequest{UserId: "user-123"},
			setupMocks: func(mockSvc *MockServiceInterface, mockLogger *MockLoggerInterface) {
				mockSvc.EXPECT().ListUserTenants(gomock.Any(), "user-123").Return(tenants, nil)
			},
			wantErr: false,
		},
		{
			name:    "service error",
			request: &v0.ListUserTenantsRequest{UserId: "user-123"},
			setupMocks: func(mockSvc *MockServiceInterface, mockLogger *MockLoggerInterface) {
				mockSvc.EXPECT().ListUserTenants(gomock.Any(), "user-123").Return(nil, errors.New("service error"))
				mockLogger.EXPECT().Errorf(gomock.Any(), gomock.Any())
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockSvc := NewMockServiceInterface(ctrl)
			mockTracer := NewMockTracingInterface(ctrl)
			mockLogger := NewMockLoggerInterface(ctrl)
			mockMonitor := NewMockMonitorInterface(ctrl)

			h := NewHandler(mockSvc, mockTracer, mockMonitor, mockLogger)

			mockTracer.EXPECT().Start(gomock.Any(), "tenant.Handler.ListUserTenants").
				Return(context.Background(), trace.SpanFromContext(context.Background()))
			tt.setupMocks(mockSvc, mockLogger)

			resp, err := h.ListUserTenants(context.Background(), tt.request)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if resp == nil {
					t.Error("expected response but got nil")
				}
			}
		})
	}
}

func TestHandler_ListTenantUsers(t *testing.T) {
	users := []*types.TenantUser{
		{UserID: "user-1", Email: "user1@example.com", Role: "owner"},
	}

	tests := []struct {
		name       string
		request    *v0.ListTenantUsersRequest
		setupMocks func(*MockServiceInterface, *MockLoggerInterface)
		wantErr    bool
	}{
		{
			name:    "success",
			request: &v0.ListTenantUsersRequest{TenantId: "tenant-123"},
			setupMocks: func(mockSvc *MockServiceInterface, mockLogger *MockLoggerInterface) {
				mockSvc.EXPECT().ListTenantUsers(gomock.Any(), "tenant-123").Return(users, nil)
			},
			wantErr: false,
		},
		{
			name:    "service error",
			request: &v0.ListTenantUsersRequest{TenantId: "tenant-123"},
			setupMocks: func(mockSvc *MockServiceInterface, mockLogger *MockLoggerInterface) {
				mockSvc.EXPECT().ListTenantUsers(gomock.Any(), "tenant-123").Return(nil, errors.New("service error"))
				mockLogger.EXPECT().Errorf(gomock.Any(), gomock.Any())
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockSvc := NewMockServiceInterface(ctrl)
			mockTracer := NewMockTracingInterface(ctrl)
			mockLogger := NewMockLoggerInterface(ctrl)
			mockMonitor := NewMockMonitorInterface(ctrl)

			h := NewHandler(mockSvc, mockTracer, mockMonitor, mockLogger)

			mockTracer.EXPECT().Start(gomock.Any(), "tenant.Handler.ListTenantUsers").
				Return(context.Background(), trace.SpanFromContext(context.Background()))
			tt.setupMocks(mockSvc, mockLogger)

			resp, err := h.ListTenantUsers(context.Background(), tt.request)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if resp == nil {
					t.Error("expected response but got nil")
				}
			}
		})
	}
}

// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0

package tenant

import (
	"context"
	"errors"
	"testing"
	"time"

	"buf.build/go/protovalidate"
	"github.com/canonical/tenant-service/internal/types"
	"github.com/canonical/tenant-service/pkg/authentication"
	v0 "github.com/canonical/tenant-service/v0"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

var testValidator = func() protovalidate.Validator {
	v, err := protovalidate.New()
	if err != nil {
		panic(err)
	}
	return v
}()

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
				TenantId: "11111111-1111-1111-1111-111111111111",
				Email:    "user@example.com",
				Role:     "member",
			},
			setupMocks: func(mockSvc *MockServiceInterface) {
				mockSvc.EXPECT().InviteMember(gomock.Any(), "11111111-1111-1111-1111-111111111111", "user@example.com", "member").
					Return("https://link", "code123", nil)
			},
			wantErr: false,
		},
		{
			name: "service error",
			request: &v0.InviteMemberRequest{
				TenantId: "11111111-1111-1111-1111-111111111111",
				Email:    "user@example.com",
				Role:     "member",
			},
			setupMocks: func(mockSvc *MockServiceInterface) {
				mockSvc.EXPECT().InviteMember(gomock.Any(), "11111111-1111-1111-1111-111111111111", "user@example.com", "member").
					Return("", "", errors.New("service error"))
			},
			wantErr:  true,
			wantCode: codes.Internal,
		},
		{
			name: "invalid tenant_id",
			request: &v0.InviteMemberRequest{
				TenantId: "not-a-uuid",
				Email:    "user@example.com",
				Role:     "member",
			},
			setupMocks: func(mockSvc *MockServiceInterface) {},
			wantErr:    true,
			wantCode:   codes.InvalidArgument,
		},
		{
			name: "invalid email",
			request: &v0.InviteMemberRequest{
				TenantId: "11111111-1111-1111-1111-111111111111",
				Email:    "not-an-email",
				Role:     "member",
			},
			setupMocks: func(mockSvc *MockServiceInterface) {},
			wantErr:    true,
			wantCode:   codes.InvalidArgument,
		},
		{
			name: "empty role",
			request: &v0.InviteMemberRequest{
				TenantId: "11111111-1111-1111-1111-111111111111",
				Email:    "user@example.com",
				Role:     "",
			},
			setupMocks: func(mockSvc *MockServiceInterface) {},
			wantErr:    true,
			wantCode:   codes.InvalidArgument,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockSvc := NewMockServiceInterface(ctrl)
			mockTracer := NewMockTracingInterface(ctrl)
			mockLogger := NewMockLoggerInterface(ctrl)
			setupLoggerMock(ctrl, mockLogger)
			mockMonitor := NewMockMonitorInterface(ctrl)

			h := NewHandler(mockSvc, testValidator, mockTracer, mockMonitor, mockLogger)

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
		name              string
		ctx               context.Context
		setupMocks        func(*MockServiceInterface, *MockLoggerInterface)
		wantErr           bool
		wantCode          codes.Code
		wantNextPageToken string
	}{
		{
			name: "success",
			ctx:  authentication.WithUserID(context.Background(), "user-123"),
			setupMocks: func(mockSvc *MockServiceInterface, mockLogger *MockLoggerInterface) {
				mockSvc.EXPECT().ListTenantsByUserID(gomock.Any(), "user-123", gomock.Any()).Return(tenants, "", nil)
			},
			wantErr: false,
		},
		{
			name: "success with next page token",
			ctx:  authentication.WithUserID(context.Background(), "user-123"),
			setupMocks: func(mockSvc *MockServiceInterface, mockLogger *MockLoggerInterface) {
				mockSvc.EXPECT().ListTenantsByUserID(gomock.Any(), "user-123", gomock.Any()).Return(tenants, "next-token-abc", nil)
			},
			wantErr:           false,
			wantNextPageToken: "next-token-abc",
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
				mockSvc.EXPECT().ListTenantsByUserID(gomock.Any(), "user-123", gomock.Any()).Return(nil, "", errors.New("service error"))
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
			setupLoggerMock(ctrl, mockLogger)
			mockMonitor := NewMockMonitorInterface(ctrl)

			h := NewHandler(mockSvc, testValidator, mockTracer, mockMonitor, mockLogger)

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
				if resp != nil && resp.NextPageToken != tt.wantNextPageToken {
					t.Errorf("expected NextPageToken %q, got %q", tt.wantNextPageToken, resp.NextPageToken)
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
		name              string
		setupMocks        func(*MockServiceInterface, *MockLoggerInterface)
		wantErr           bool
		wantNextPageToken string
	}{
		{
			name: "success",
			setupMocks: func(mockSvc *MockServiceInterface, mockLogger *MockLoggerInterface) {
				mockSvc.EXPECT().ListTenants(gomock.Any(), gomock.Any()).Return(tenants, "", nil)
			},
			wantErr: false,
		},
		{
			name: "success with next page token",
			setupMocks: func(mockSvc *MockServiceInterface, mockLogger *MockLoggerInterface) {
				mockSvc.EXPECT().ListTenants(gomock.Any(), gomock.Any()).Return(tenants, "next-token-xyz", nil)
			},
			wantErr:           false,
			wantNextPageToken: "next-token-xyz",
		},
		{
			name: "service error",
			setupMocks: func(mockSvc *MockServiceInterface, mockLogger *MockLoggerInterface) {
				mockSvc.EXPECT().ListTenants(gomock.Any(), gomock.Any()).Return(nil, "", errors.New("service error"))
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
			setupLoggerMock(ctrl, mockLogger)
			mockMonitor := NewMockMonitorInterface(ctrl)

			h := NewHandler(mockSvc, testValidator, mockTracer, mockMonitor, mockLogger)

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
				if resp != nil && resp.NextPageToken != tt.wantNextPageToken {
					t.Errorf("expected NextPageToken %q, got %q", tt.wantNextPageToken, resp.NextPageToken)
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
			name:    "service error",
			request: &v0.CreateTenantRequest{Name: "Test Tenant"},
			setupMocks: func(mockSvc *MockServiceInterface, mockLogger *MockLoggerInterface) {
				mockSvc.EXPECT().CreateTenant(gomock.Any(), "Test Tenant").Return(nil, errors.New("service error"))
			},
			wantErr:  true,
			wantCode: codes.Internal,
		},
		{
			name:       "empty name",
			request:    &v0.CreateTenantRequest{Name: ""},
			setupMocks: func(mockSvc *MockServiceInterface, mockLogger *MockLoggerInterface) {},
			wantErr:    true,
			wantCode:   codes.InvalidArgument,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockSvc := NewMockServiceInterface(ctrl)
			mockTracer := NewMockTracingInterface(ctrl)
			mockLogger := NewMockLoggerInterface(ctrl)
			setupLoggerMock(ctrl, mockLogger)
			mockMonitor := NewMockMonitorInterface(ctrl)

			h := NewHandler(mockSvc, testValidator, mockTracer, mockMonitor, mockLogger)

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
				Tenant:     &v0.Tenant{Id: "11111111-1111-1111-1111-111111111111", Name: "Updated", Enabled: true},
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
				Tenant: &v0.Tenant{Id: "11111111-1111-1111-1111-111111111111", Name: "Updated"},
			},
			setupMocks: func(mockSvc *MockServiceInterface, mockLogger *MockLoggerInterface) {
				mockSvc.EXPECT().UpdateTenant(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, errors.New("service error"))
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
			setupLoggerMock(ctrl, mockLogger)
			mockMonitor := NewMockMonitorInterface(ctrl)

			h := NewHandler(mockSvc, testValidator, mockTracer, mockMonitor, mockLogger)

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
		wantCode   codes.Code
	}{
		{
			name:    "success",
			request: &v0.DeleteTenantRequest{TenantId: "11111111-1111-1111-1111-111111111111"},
			setupMocks: func(mockSvc *MockServiceInterface, mockLogger *MockLoggerInterface) {
				mockSvc.EXPECT().DeleteTenant(gomock.Any(), "11111111-1111-1111-1111-111111111111").Return(nil)
			},
			wantErr: false,
		},
		{
			name:    "service error",
			request: &v0.DeleteTenantRequest{TenantId: "11111111-1111-1111-1111-111111111111"},
			setupMocks: func(mockSvc *MockServiceInterface, mockLogger *MockLoggerInterface) {
				mockSvc.EXPECT().DeleteTenant(gomock.Any(), "11111111-1111-1111-1111-111111111111").Return(errors.New("service error"))
			},
			wantErr: true,
		},
		{
			name:       "invalid tenant_id",
			request:    &v0.DeleteTenantRequest{TenantId: "not-a-uuid"},
			setupMocks: func(mockSvc *MockServiceInterface, mockLogger *MockLoggerInterface) {},
			wantErr:    true,
			wantCode:   codes.InvalidArgument,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockSvc := NewMockServiceInterface(ctrl)
			mockTracer := NewMockTracingInterface(ctrl)
			mockLogger := NewMockLoggerInterface(ctrl)
			setupLoggerMock(ctrl, mockLogger)
			mockMonitor := NewMockMonitorInterface(ctrl)

			h := NewHandler(mockSvc, testValidator, mockTracer, mockMonitor, mockLogger)

			mockTracer.EXPECT().Start(gomock.Any(), "tenant.Handler.DeleteTenant").
				Return(context.Background(), trace.SpanFromContext(context.Background()))
			tt.setupMocks(mockSvc, mockLogger)

			_, err := h.DeleteTenant(context.Background(), tt.request)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error but got none")
				}
				st, ok := status.FromError(err)
				if ok && tt.wantCode != 0 && st.Code() != tt.wantCode {
					t.Errorf("expected code %v, got %v", tt.wantCode, st.Code())
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
		wantCode   codes.Code
	}{
		{
			name: "success",
			request: &v0.ProvisionUserRequest{
				TenantId: "11111111-1111-1111-1111-111111111111",
				Email:    "user@example.com",
				Role:     "member",
			},
			setupMocks: func(mockSvc *MockServiceInterface, mockLogger *MockLoggerInterface) {
				mockSvc.EXPECT().ProvisionUser(gomock.Any(), "11111111-1111-1111-1111-111111111111", "user@example.com", "member").Return(nil)
			},
			wantErr: false,
		},
		{
			name: "service error",
			request: &v0.ProvisionUserRequest{
				TenantId: "11111111-1111-1111-1111-111111111111",
				Email:    "user@example.com",
				Role:     "member",
			},
			setupMocks: func(mockSvc *MockServiceInterface, mockLogger *MockLoggerInterface) {
				mockSvc.EXPECT().ProvisionUser(gomock.Any(), "11111111-1111-1111-1111-111111111111", "user@example.com", "member").
					Return(errors.New("service error"))
			},
			wantErr: true,
		},
		{
			name: "invalid tenant_id",
			request: &v0.ProvisionUserRequest{
				TenantId: "not-a-uuid",
				Email:    "user@example.com",
				Role:     "member",
			},
			setupMocks: func(mockSvc *MockServiceInterface, mockLogger *MockLoggerInterface) {},
			wantErr:    true,
			wantCode:   codes.InvalidArgument,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockSvc := NewMockServiceInterface(ctrl)
			mockTracer := NewMockTracingInterface(ctrl)
			mockLogger := NewMockLoggerInterface(ctrl)
			setupLoggerMock(ctrl, mockLogger)
			mockMonitor := NewMockMonitorInterface(ctrl)

			h := NewHandler(mockSvc, testValidator, mockTracer, mockMonitor, mockLogger)

			mockTracer.EXPECT().Start(gomock.Any(), "tenant.Handler.ProvisionUser").
				Return(context.Background(), trace.SpanFromContext(context.Background()))
			tt.setupMocks(mockSvc, mockLogger)

			resp, err := h.ProvisionUser(context.Background(), tt.request)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error but got none")
				}
				st, ok := status.FromError(err)
				if ok && tt.wantCode != 0 && st.Code() != tt.wantCode {
					t.Errorf("expected code %v, got %v", tt.wantCode, st.Code())
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
	user := &types.TenantUser{UserID: "22222222-2222-2222-2222-222222222222", Email: "user@example.com", Role: "owner"}

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
				TenantId: "11111111-1111-1111-1111-111111111111",
				UserId:   "22222222-2222-2222-2222-222222222222",
				Role:     "owner",
			},
			setupMocks: func(mockSvc *MockServiceInterface, mockLogger *MockLoggerInterface) {
				mockSvc.EXPECT().UpdateTenantUser(gomock.Any(), "11111111-1111-1111-1111-111111111111", "22222222-2222-2222-2222-222222222222", "owner").Return(user, nil)
			},
			wantErr: false,
		},
		{
			name: "service error",
			request: &v0.UpdateTenantUserRequest{
				TenantId: "11111111-1111-1111-1111-111111111111",
				UserId:   "22222222-2222-2222-2222-222222222222",
				Role:     "owner",
			},
			setupMocks: func(mockSvc *MockServiceInterface, mockLogger *MockLoggerInterface) {
				mockSvc.EXPECT().UpdateTenantUser(gomock.Any(), "11111111-1111-1111-1111-111111111111", "22222222-2222-2222-2222-222222222222", "owner").
					Return(nil, errors.New("service error"))
			},
			wantErr:  true,
			wantCode: codes.Internal,
		},
		{
			name: "invalid tenant_id",
			request: &v0.UpdateTenantUserRequest{
				TenantId: "not-a-uuid",
				UserId:   "22222222-2222-2222-2222-222222222222",
				Role:     "owner",
			},
			setupMocks: func(mockSvc *MockServiceInterface, mockLogger *MockLoggerInterface) {},
			wantErr:    true,
			wantCode:   codes.InvalidArgument,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockSvc := NewMockServiceInterface(ctrl)
			mockTracer := NewMockTracingInterface(ctrl)
			mockLogger := NewMockLoggerInterface(ctrl)
			setupLoggerMock(ctrl, mockLogger)
			mockMonitor := NewMockMonitorInterface(ctrl)

			h := NewHandler(mockSvc, testValidator, mockTracer, mockMonitor, mockLogger)

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
		name              string
		request           *v0.ListUserTenantsRequest
		setupMocks        func(*MockServiceInterface, *MockLoggerInterface)
		wantErr           bool
		wantCode          codes.Code
		wantNextPageToken string
	}{
		{
			name:    "success",
			request: &v0.ListUserTenantsRequest{UserId: "22222222-2222-2222-2222-222222222222"},
			setupMocks: func(mockSvc *MockServiceInterface, mockLogger *MockLoggerInterface) {
				mockSvc.EXPECT().ListUserTenants(gomock.Any(), "22222222-2222-2222-2222-222222222222", gomock.Any()).Return(tenants, "", nil)
			},
			wantErr: false,
		},
		{
			name:    "success with next page token",
			request: &v0.ListUserTenantsRequest{UserId: "22222222-2222-2222-2222-222222222222"},
			setupMocks: func(mockSvc *MockServiceInterface, mockLogger *MockLoggerInterface) {
				mockSvc.EXPECT().ListUserTenants(gomock.Any(), "22222222-2222-2222-2222-222222222222", gomock.Any()).Return(tenants, "next-token-usr", nil)
			},
			wantErr:           false,
			wantNextPageToken: "next-token-usr",
		},
		{
			name:    "service error",
			request: &v0.ListUserTenantsRequest{UserId: "22222222-2222-2222-2222-222222222222"},
			setupMocks: func(mockSvc *MockServiceInterface, mockLogger *MockLoggerInterface) {
				mockSvc.EXPECT().ListUserTenants(gomock.Any(), "22222222-2222-2222-2222-222222222222", gomock.Any()).Return(nil, "", errors.New("service error"))
			},
			wantErr: true,
		},
		{
			name:       "invalid user_id",
			request:    &v0.ListUserTenantsRequest{UserId: "not-a-uuid"},
			setupMocks: func(mockSvc *MockServiceInterface, mockLogger *MockLoggerInterface) {},
			wantErr:    true,
			wantCode:   codes.InvalidArgument,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockSvc := NewMockServiceInterface(ctrl)
			mockTracer := NewMockTracingInterface(ctrl)
			mockLogger := NewMockLoggerInterface(ctrl)
			setupLoggerMock(ctrl, mockLogger)
			mockMonitor := NewMockMonitorInterface(ctrl)

			h := NewHandler(mockSvc, testValidator, mockTracer, mockMonitor, mockLogger)

			mockTracer.EXPECT().Start(gomock.Any(), "tenant.Handler.ListUserTenants").
				Return(context.Background(), trace.SpanFromContext(context.Background()))
			tt.setupMocks(mockSvc, mockLogger)

			resp, err := h.ListUserTenants(context.Background(), tt.request)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error but got none")
				}
				st, ok := status.FromError(err)
				if ok && tt.wantCode != 0 && st.Code() != tt.wantCode {
					t.Errorf("expected code %v, got %v", tt.wantCode, st.Code())
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if resp == nil {
					t.Error("expected response but got nil")
				}
				if resp != nil && resp.NextPageToken != tt.wantNextPageToken {
					t.Errorf("expected NextPageToken %q, got %q", tt.wantNextPageToken, resp.NextPageToken)
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
		name              string
		request           *v0.ListTenantUsersRequest
		setupMocks        func(*MockServiceInterface, *MockLoggerInterface)
		wantErr           bool
		wantCode          codes.Code
		wantNextPageToken string
	}{
		{
			name:    "success",
			request: &v0.ListTenantUsersRequest{TenantId: "11111111-1111-1111-1111-111111111111"},
			setupMocks: func(mockSvc *MockServiceInterface, mockLogger *MockLoggerInterface) {
				mockSvc.EXPECT().ListTenantUsers(gomock.Any(), "11111111-1111-1111-1111-111111111111", gomock.Any()).Return(users, "", nil)
			},
			wantErr: false,
		},
		{
			name:    "success with next page token",
			request: &v0.ListTenantUsersRequest{TenantId: "11111111-1111-1111-1111-111111111111"},
			setupMocks: func(mockSvc *MockServiceInterface, mockLogger *MockLoggerInterface) {
				mockSvc.EXPECT().ListTenantUsers(gomock.Any(), "11111111-1111-1111-1111-111111111111", gomock.Any()).Return(users, "next-token-tu", nil)
			},
			wantErr:           false,
			wantNextPageToken: "next-token-tu",
		},
		{
			name:    "service error",
			request: &v0.ListTenantUsersRequest{TenantId: "11111111-1111-1111-1111-111111111111"},
			setupMocks: func(mockSvc *MockServiceInterface, mockLogger *MockLoggerInterface) {
				mockSvc.EXPECT().ListTenantUsers(gomock.Any(), "11111111-1111-1111-1111-111111111111", gomock.Any()).Return(nil, "", errors.New("service error"))
			},
			wantErr: true,
		},
		{
			name:       "invalid tenant_id",
			request:    &v0.ListTenantUsersRequest{TenantId: "not-a-uuid"},
			setupMocks: func(mockSvc *MockServiceInterface, mockLogger *MockLoggerInterface) {},
			wantErr:    true,
			wantCode:   codes.InvalidArgument,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockSvc := NewMockServiceInterface(ctrl)
			mockTracer := NewMockTracingInterface(ctrl)
			mockLogger := NewMockLoggerInterface(ctrl)
			setupLoggerMock(ctrl, mockLogger)
			mockMonitor := NewMockMonitorInterface(ctrl)

			h := NewHandler(mockSvc, testValidator, mockTracer, mockMonitor, mockLogger)

			mockTracer.EXPECT().Start(gomock.Any(), "tenant.Handler.ListTenantUsers").
				Return(context.Background(), trace.SpanFromContext(context.Background()))
			tt.setupMocks(mockSvc, mockLogger)

			resp, err := h.ListTenantUsers(context.Background(), tt.request)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error but got none")
				}
				st, ok := status.FromError(err)
				if ok && tt.wantCode != 0 && st.Code() != tt.wantCode {
					t.Errorf("expected code %v, got %v", tt.wantCode, st.Code())
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if resp == nil {
					t.Error("expected response but got nil")
				}
				if resp != nil && resp.NextPageToken != tt.wantNextPageToken {
					t.Errorf("expected NextPageToken %q, got %q", tt.wantNextPageToken, resp.NextPageToken)
				}
			}
		})
	}
}

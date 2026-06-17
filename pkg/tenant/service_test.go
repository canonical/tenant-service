// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package tenant

import (
	"context"
	"errors"
	"testing"

	"github.com/canonical/tenant-service/internal/storage"
	"github.com/canonical/tenant-service/internal/types"
	ory "github.com/ory/client-go"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/mock/gomock"
)

//go:generate mockgen -build_flags=--mod=mod -package tenant -destination ./mock_tenant.go -source=./interfaces.go
//go:generate mockgen -build_flags=--mod=mod -package tenant -destination ./mock_logger.go -source=../../internal/logging/interfaces.go
//go:generate mockgen -build_flags=--mod=mod -package tenant -destination ./mock_monitor.go -source=../../internal/monitoring/interfaces.go
//go:generate mockgen -build_flags=--mod=mod -package tenant -destination ./mock_tracing.go -source=../../internal/tracing/interfaces.go

// setupLoggerMock configures a MockLoggerInterface with AnyTimes() stubs for all
// structured logging methods (w-suffix) and for the security logger.
func setupLoggerMock(ctrl *gomock.Controller, mockLogger *MockLoggerInterface) *MockSecurityLoggerInterface {
	mockSecurityLogger := NewMockSecurityLoggerInterface(ctrl)
	mockLogger.EXPECT().Debugw(gomock.Any(), gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Infow(gomock.Any(), gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Errorw(gomock.Any(), gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Warnw(gomock.Any(), gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Security().Return(mockSecurityLogger).AnyTimes()
	mockSecurityLogger.EXPECT().AdminAction(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()
	return mockSecurityLogger
}

func TestService_ListTenantsByUserID(t *testing.T) {
	userID := "user-123"
	expectedTenants := []*types.Tenant{
		{ID: "tenant-1", Name: "Tenant 1"},
		{ID: "tenant-2", Name: "Tenant 2"},
	}
	dbErr := errors.New("db error")

	testCases := []struct {
		name            string
		setupMocks      func(*MockStorageInterface)
		expectedTenants []*types.Tenant
		expectedErr     error
	}{
		{
			name: "success",
			setupMocks: func(mockStorage *MockStorageInterface) {
				mockStorage.EXPECT().ListTenantsByUserID(gomock.Any(), userID, gomock.Any()).Return(expectedTenants, nil)
			},
			expectedTenants: expectedTenants,
			expectedErr:     nil,
		},
		{
			name: "empty result",
			setupMocks: func(mockStorage *MockStorageInterface) {
				mockStorage.EXPECT().ListTenantsByUserID(gomock.Any(), userID, gomock.Any()).Return([]*types.Tenant{}, nil)
			},
			expectedTenants: []*types.Tenant{},
			expectedErr:     nil,
		},
		{
			name: "storage error",
			setupMocks: func(mockStorage *MockStorageInterface) {
				mockStorage.EXPECT().ListTenantsByUserID(gomock.Any(), userID, gomock.Any()).Return(nil, dbErr)
			},
			expectedTenants: nil,
			expectedErr:     dbErr,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockStorage := NewMockStorageInterface(ctrl)
			mockAuthz := NewMockAuthzInterface(ctrl)
			mockKratos := NewMockKratosClientInterface(ctrl)
			mockTracer := NewMockTracingInterface(ctrl)
			mockLogger := NewMockLoggerInterface(ctrl)
			setupLoggerMock(ctrl, mockLogger)
			mockMonitor := NewMockMonitorInterface(ctrl)

			s := NewService(mockStorage, mockAuthz, mockKratos, "1h", mockTracer, mockMonitor, mockLogger)

			mockTracer.EXPECT().Start(gomock.Any(), "tenant.Service.ListTenantsByUserID").Return(context.Background(), trace.SpanFromContext(context.Background()))
			tc.setupMocks(mockStorage)

			tenants, err := s.ListTenantsByUserID(context.Background(), userID)

			if tc.expectedErr != nil {
				if !errors.Is(err, tc.expectedErr) {
					t.Errorf("expected error %v, got %v", tc.expectedErr, err)
				}
			} else if err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			if len(tenants) != len(tc.expectedTenants) {
				t.Errorf("expected %d tenants, got %d", len(tc.expectedTenants), len(tenants))
			}
		})
	}
}

func TestService_ListTenants(t *testing.T) {
	expectedTenants := []*types.Tenant{
		{ID: "tenant-1", Name: "Tenant 1"},
		{ID: "tenant-2", Name: "Tenant 2"},
	}
	dbErr := errors.New("db error")

	testCases := []struct {
		name            string
		setupMocks      func(*MockStorageInterface)
		expectedTenants []*types.Tenant
		expectedErr     error
	}{
		{
			name: "success",
			setupMocks: func(mockStorage *MockStorageInterface) {
				mockStorage.EXPECT().ListTenants(gomock.Any(), gomock.Any()).Return(expectedTenants, "", nil)
			},
			expectedTenants: expectedTenants,
			expectedErr:     nil,
		},
		{
			name: "storage error",
			setupMocks: func(mockStorage *MockStorageInterface) {
				mockStorage.EXPECT().ListTenants(gomock.Any(), gomock.Any()).Return(nil, "", dbErr)
			},
			expectedTenants: nil,
			expectedErr:     dbErr,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockStorage := NewMockStorageInterface(ctrl)
			mockAuthz := NewMockAuthzInterface(ctrl)
			mockKratos := NewMockKratosClientInterface(ctrl)
			mockTracer := NewMockTracingInterface(ctrl)
			mockLogger := NewMockLoggerInterface(ctrl)
			setupLoggerMock(ctrl, mockLogger)
			mockMonitor := NewMockMonitorInterface(ctrl)

			s := NewService(mockStorage, mockAuthz, mockKratos, "1h", mockTracer, mockMonitor, mockLogger)

			mockTracer.EXPECT().Start(gomock.Any(), "tenant.Service.ListTenants").Return(context.Background(), trace.SpanFromContext(context.Background()))
			tc.setupMocks(mockStorage)

			tenants, _, err := s.ListTenants(context.Background())

			if tc.expectedErr != nil {
				if !errors.Is(err, tc.expectedErr) {
					t.Errorf("expected error %v, got %v", tc.expectedErr, err)
				}
			} else if err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			if len(tenants) != len(tc.expectedTenants) {
				t.Errorf("expected %d tenants, got %d", len(tc.expectedTenants), len(tenants))
			}
		})
	}
}

func TestService_InviteMember(t *testing.T) {
	tenantID := "tenant-123"
	email := "user@example.com"
	identityID := "identity-456"
	recoveryLink := "https://recovery.link/abc"
	recoveryCode := "code123"

	testCases := []struct {
		name         string
		role         string
		setupMocks   func(*MockStorageInterface, *MockAuthzInterface, *MockKratosClientInterface, *MockLoggerInterface, *MockMonitorInterface)
		expectedLink string
		expectedCode string
		expectedErr  bool
	}{
		{
			name: "success - new user as member",
			role: "member",
			setupMocks: func(mockStorage *MockStorageInterface, mockAuthz *MockAuthzInterface, mockKratos *MockKratosClientInterface, mockLogger *MockLoggerInterface, mockMonitor *MockMonitorInterface) {
				mockKratos.EXPECT().GetIdentityIDByEmail(gomock.Any(), email).Return("", nil)
				mockKratos.EXPECT().CreateIdentity(gomock.Any(), email).Return(identityID, nil)
				mockStorage.EXPECT().AddMember(gomock.Any(), tenantID, identityID, "member").Return("member-id", nil)
				mockAuthz.EXPECT().AssignTenantMember(gomock.Any(), tenantID, identityID).Return(nil)
				mockKratos.EXPECT().CreateRecoveryLink(gomock.Any(), identityID, "1h").Return(recoveryLink, recoveryCode, nil)
				mockMonitor.EXPECT().IncrementCounter(map[string]string{"operation": "invitation_sent", "role": "member"}).Return(nil)
			},
			expectedLink: recoveryLink,
			expectedCode: recoveryCode,
			expectedErr:  false,
		},
		{
			name: "success - existing user as owner",
			role: "owner",
			setupMocks: func(mockStorage *MockStorageInterface, mockAuthz *MockAuthzInterface, mockKratos *MockKratosClientInterface, mockLogger *MockLoggerInterface, mockMonitor *MockMonitorInterface) {
				mockKratos.EXPECT().GetIdentityIDByEmail(gomock.Any(), email).Return(identityID, nil)
				mockStorage.EXPECT().AddMember(gomock.Any(), tenantID, identityID, "owner").Return("member-id", nil)
				mockAuthz.EXPECT().AssignTenantOwner(gomock.Any(), tenantID, identityID).Return(nil)
				mockKratos.EXPECT().CreateRecoveryLink(gomock.Any(), identityID, "1h").Return(recoveryLink, recoveryCode, nil)
				mockMonitor.EXPECT().IncrementCounter(map[string]string{"operation": "invitation_sent", "role": "owner"}).Return(nil)
			},
			expectedLink: recoveryLink,
			expectedCode: recoveryCode,
			expectedErr:  false,
		},
		{
			name: "success - duplicate key treated as reinvite",
			role: "member",
			setupMocks: func(mockStorage *MockStorageInterface, mockAuthz *MockAuthzInterface, mockKratos *MockKratosClientInterface, mockLogger *MockLoggerInterface, mockMonitor *MockMonitorInterface) {
				mockKratos.EXPECT().GetIdentityIDByEmail(gomock.Any(), email).Return(identityID, nil)
				mockStorage.EXPECT().AddMember(gomock.Any(), tenantID, identityID, "member").Return("", storage.ErrDuplicateKey)
				mockAuthz.EXPECT().AssignTenantMember(gomock.Any(), tenantID, identityID).Return(nil)
				mockKratos.EXPECT().CreateRecoveryLink(gomock.Any(), identityID, "1h").Return(recoveryLink, recoveryCode, nil)
				mockMonitor.EXPECT().IncrementCounter(map[string]string{"operation": "invitation_sent", "role": "member"}).Return(nil)
			},
			expectedLink: recoveryLink,
			expectedCode: recoveryCode,
			expectedErr:  false,
		},
		{
			name: "error - failed to check identity",
			role: "member",
			setupMocks: func(mockStorage *MockStorageInterface, mockAuthz *MockAuthzInterface, mockKratos *MockKratosClientInterface, mockLogger *MockLoggerInterface, mockMonitor *MockMonitorInterface) {
				mockKratos.EXPECT().GetIdentityIDByEmail(gomock.Any(), email).Return("", errors.New("kratos error"))
			},
			expectedErr: true,
		},
		{
			name: "error - failed to create identity",
			role: "member",
			setupMocks: func(mockStorage *MockStorageInterface, mockAuthz *MockAuthzInterface, mockKratos *MockKratosClientInterface, mockLogger *MockLoggerInterface, mockMonitor *MockMonitorInterface) {
				mockKratos.EXPECT().GetIdentityIDByEmail(gomock.Any(), email).Return("", nil)
				mockKratos.EXPECT().CreateIdentity(gomock.Any(), email).Return("", errors.New("kratos error"))
			},
			expectedErr: true,
		},
		{
			name: "error - failed to add member",
			role: "member",
			setupMocks: func(mockStorage *MockStorageInterface, mockAuthz *MockAuthzInterface, mockKratos *MockKratosClientInterface, mockLogger *MockLoggerInterface, mockMonitor *MockMonitorInterface) {
				mockKratos.EXPECT().GetIdentityIDByEmail(gomock.Any(), email).Return(identityID, nil)
				mockStorage.EXPECT().AddMember(gomock.Any(), tenantID, identityID, "member").Return("", errors.New("storage error"))
			},
			expectedErr: true,
		},
		{
			name: "error - failed to assign authz",
			role: "member",
			setupMocks: func(mockStorage *MockStorageInterface, mockAuthz *MockAuthzInterface, mockKratos *MockKratosClientInterface, mockLogger *MockLoggerInterface, mockMonitor *MockMonitorInterface) {
				mockKratos.EXPECT().GetIdentityIDByEmail(gomock.Any(), email).Return(identityID, nil)
				mockStorage.EXPECT().AddMember(gomock.Any(), tenantID, identityID, "member").Return("member-id", nil)
				mockAuthz.EXPECT().AssignTenantMember(gomock.Any(), tenantID, identityID).Return(errors.New("authz error"))
			},
			expectedErr: true,
		},
		{
			name: "error - failed to create recovery link",
			role: "member",
			setupMocks: func(mockStorage *MockStorageInterface, mockAuthz *MockAuthzInterface, mockKratos *MockKratosClientInterface, mockLogger *MockLoggerInterface, mockMonitor *MockMonitorInterface) {
				mockKratos.EXPECT().GetIdentityIDByEmail(gomock.Any(), email).Return(identityID, nil)
				mockStorage.EXPECT().AddMember(gomock.Any(), tenantID, identityID, "member").Return("member-id", nil)
				mockAuthz.EXPECT().AssignTenantMember(gomock.Any(), tenantID, identityID).Return(nil)
				mockKratos.EXPECT().CreateRecoveryLink(gomock.Any(), identityID, "1h").Return("", "", errors.New("kratos error"))
			},
			expectedErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockStorage := NewMockStorageInterface(ctrl)
			mockAuthz := NewMockAuthzInterface(ctrl)
			mockKratos := NewMockKratosClientInterface(ctrl)
			mockTracer := NewMockTracingInterface(ctrl)
			mockLogger := NewMockLoggerInterface(ctrl)
			setupLoggerMock(ctrl, mockLogger)
			mockMonitor := NewMockMonitorInterface(ctrl)

			s := NewService(mockStorage, mockAuthz, mockKratos, "1h", mockTracer, mockMonitor, mockLogger)

			mockTracer.EXPECT().Start(gomock.Any(), "tenant.Service.InviteMember").Return(context.Background(), trace.SpanFromContext(context.Background()))
			tc.setupMocks(mockStorage, mockAuthz, mockKratos, mockLogger, mockMonitor)

			link, code, err := s.InviteMember(context.Background(), tenantID, email, tc.role)

			if tc.expectedErr {
				if err == nil {
					t.Error("expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if link != tc.expectedLink {
					t.Errorf("expected link %s, got %s", tc.expectedLink, link)
				}
				if code != tc.expectedCode {
					t.Errorf("expected code %s, got %s", tc.expectedCode, code)
				}
			}
		})
	}
}

func TestService_CreateTenant(t *testing.T) {
	name := "Test Tenant"
	createdTenant := &types.Tenant{ID: "tenant-123", Name: name, Enabled: true}

	testCases := []struct {
		name        string
		setupMocks  func(*MockStorageInterface)
		expectedErr bool
	}{
		{
			name: "success",
			setupMocks: func(mockStorage *MockStorageInterface) {
				mockStorage.EXPECT().CreateTenant(gomock.Any(), gomock.Any()).DoAndReturn(
					func(_ context.Context, t *types.Tenant) (*types.Tenant, error) {
						if t.Name != name {
							return nil, errors.New("wrong name")
						}
						if !t.Enabled {
							return nil, errors.New("should be enabled")
						}
						return createdTenant, nil
					})
			},
			expectedErr: false,
		},
		{
			name: "storage error",
			setupMocks: func(mockStorage *MockStorageInterface) {
				mockStorage.EXPECT().CreateTenant(gomock.Any(), gomock.Any()).Return(nil, errors.New("storage error"))
			},
			expectedErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockStorage := NewMockStorageInterface(ctrl)
			mockAuthz := NewMockAuthzInterface(ctrl)
			mockKratos := NewMockKratosClientInterface(ctrl)
			mockTracer := NewMockTracingInterface(ctrl)
			mockLogger := NewMockLoggerInterface(ctrl)
			setupLoggerMock(ctrl, mockLogger)
			mockMonitor := NewMockMonitorInterface(ctrl)

			s := NewService(mockStorage, mockAuthz, mockKratos, "1h", mockTracer, mockMonitor, mockLogger)

			mockTracer.EXPECT().Start(gomock.Any(), "admin.CreateTenant").Return(context.Background(), trace.SpanFromContext(context.Background()))
			tc.setupMocks(mockStorage)

			tenant, err := s.CreateTenant(context.Background(), name)

			if tc.expectedErr {
				if err == nil {
					t.Error("expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if tenant == nil {
					t.Error("expected tenant but got nil")
				}
			}
		})
	}
}

func TestService_UpdateTenant(t *testing.T) {
	tenant := &types.Tenant{ID: "tenant-123", Name: "Updated Name"}
	paths := []string{"name"}
	updatedTenant := &types.Tenant{ID: "tenant-123", Name: "Updated Name", Enabled: true}

	testCases := []struct {
		name        string
		setupMocks  func(*MockStorageInterface)
		expectedErr bool
	}{
		{
			name: "success",
			setupMocks: func(mockStorage *MockStorageInterface) {
				mockStorage.EXPECT().UpdateTenant(gomock.Any(), tenant, paths).Return(nil)
				mockStorage.EXPECT().GetTenantByID(gomock.Any(), tenant.ID).Return(updatedTenant, nil)
			},
			expectedErr: false,
		},
		{
			name: "update error",
			setupMocks: func(mockStorage *MockStorageInterface) {
				mockStorage.EXPECT().UpdateTenant(gomock.Any(), tenant, paths).Return(errors.New("storage error"))
			},
			expectedErr: true,
		},
		{
			name: "get error",
			setupMocks: func(mockStorage *MockStorageInterface) {
				mockStorage.EXPECT().UpdateTenant(gomock.Any(), tenant, paths).Return(nil)
				mockStorage.EXPECT().GetTenantByID(gomock.Any(), tenant.ID).Return(nil, errors.New("not found"))
			},
			expectedErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockStorage := NewMockStorageInterface(ctrl)
			mockAuthz := NewMockAuthzInterface(ctrl)
			mockKratos := NewMockKratosClientInterface(ctrl)
			mockTracer := NewMockTracingInterface(ctrl)
			mockLogger := NewMockLoggerInterface(ctrl)
			setupLoggerMock(ctrl, mockLogger)
			mockMonitor := NewMockMonitorInterface(ctrl)

			s := NewService(mockStorage, mockAuthz, mockKratos, "1h", mockTracer, mockMonitor, mockLogger)

			mockTracer.EXPECT().Start(gomock.Any(), "admin.UpdateTenant").Return(context.Background(), trace.SpanFromContext(context.Background()))
			tc.setupMocks(mockStorage)

			result, err := s.UpdateTenant(context.Background(), tenant, paths)

			if tc.expectedErr {
				if err == nil {
					t.Error("expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if result == nil {
					t.Error("expected tenant but got nil")
				}
			}
		})
	}
}

func TestService_DeleteTenant(t *testing.T) {
	tenantID := "tenant-123"

	testCases := []struct {
		name        string
		setupMocks  func(*MockStorageInterface, *MockAuthzInterface, *MockLoggerInterface)
		expectedErr bool
	}{
		{
			name: "success",
			setupMocks: func(mockStorage *MockStorageInterface, mockAuthz *MockAuthzInterface, mockLogger *MockLoggerInterface) {
				mockStorage.EXPECT().DeleteTenant(gomock.Any(), tenantID).Return(nil)
				mockAuthz.EXPECT().DeleteTenant(gomock.Any(), tenantID).Return(nil)
			},
			expectedErr: false,
		},
		{
			name: "storage error",
			setupMocks: func(mockStorage *MockStorageInterface, mockAuthz *MockAuthzInterface, mockLogger *MockLoggerInterface) {
				mockStorage.EXPECT().DeleteTenant(gomock.Any(), tenantID).Return(errors.New("storage error"))
			},
			expectedErr: true,
		},
		{
			name: "authz error - logged but not failed",
			setupMocks: func(mockStorage *MockStorageInterface, mockAuthz *MockAuthzInterface, mockLogger *MockLoggerInterface) {
				mockStorage.EXPECT().DeleteTenant(gomock.Any(), tenantID).Return(nil)
				mockAuthz.EXPECT().DeleteTenant(gomock.Any(), tenantID).Return(errors.New("authz error"))
			},
			expectedErr: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockStorage := NewMockStorageInterface(ctrl)
			mockAuthz := NewMockAuthzInterface(ctrl)
			mockKratos := NewMockKratosClientInterface(ctrl)
			mockTracer := NewMockTracingInterface(ctrl)
			mockLogger := NewMockLoggerInterface(ctrl)
			setupLoggerMock(ctrl, mockLogger)
			mockMonitor := NewMockMonitorInterface(ctrl)

			s := NewService(mockStorage, mockAuthz, mockKratos, "1h", mockTracer, mockMonitor, mockLogger)

			mockTracer.EXPECT().Start(gomock.Any(), "admin.DeleteTenant").Return(context.Background(), trace.SpanFromContext(context.Background()))
			tc.setupMocks(mockStorage, mockAuthz, mockLogger)

			err := s.DeleteTenant(context.Background(), tenantID)

			if tc.expectedErr {
				if err == nil {
					t.Error("expected error but got none")
				}
			} else if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestService_ProvisionUser(t *testing.T) {
	tenantID := "tenant-123"
	email := "user@example.com"
	identityID := "identity-456"

	testCases := []struct {
		name        string
		role        string
		setupMocks  func(*MockStorageInterface, *MockAuthzInterface, *MockKratosClientInterface, *MockMonitorInterface)
		expectedErr bool
	}{
		{
			name: "success - new user as member",
			role: "member",
			setupMocks: func(mockStorage *MockStorageInterface, mockAuthz *MockAuthzInterface, mockKratos *MockKratosClientInterface, mockMonitor *MockMonitorInterface) {
				mockKratos.EXPECT().GetIdentityIDByEmail(gomock.Any(), email).Return("", nil)
				mockKratos.EXPECT().CreateIdentity(gomock.Any(), email).Return(identityID, nil)
				mockStorage.EXPECT().AddMember(gomock.Any(), tenantID, identityID, "member").Return("member-id", nil)
				mockAuthz.EXPECT().AssignTenantMember(gomock.Any(), tenantID, identityID).Return(nil)
				mockMonitor.EXPECT().IncrementCounter(map[string]string{"operation": "user_provisioned", "role": "member"}).Return(nil)
			},
			expectedErr: false,
		},
		{
			name: "success - existing user as owner",
			role: "owner",
			setupMocks: func(mockStorage *MockStorageInterface, mockAuthz *MockAuthzInterface, mockKratos *MockKratosClientInterface, mockMonitor *MockMonitorInterface) {
				mockKratos.EXPECT().GetIdentityIDByEmail(gomock.Any(), email).Return(identityID, nil)
				mockStorage.EXPECT().AddMember(gomock.Any(), tenantID, identityID, "owner").Return("member-id", nil)
				mockAuthz.EXPECT().AssignTenantOwner(gomock.Any(), tenantID, identityID).Return(nil)
				mockMonitor.EXPECT().IncrementCounter(map[string]string{"operation": "user_provisioned", "role": "owner"}).Return(nil)
			},
			expectedErr: false,
		},
		{
			name: "success - admin role",
			role: "admin",
			setupMocks: func(mockStorage *MockStorageInterface, mockAuthz *MockAuthzInterface, mockKratos *MockKratosClientInterface, mockMonitor *MockMonitorInterface) {
				mockKratos.EXPECT().GetIdentityIDByEmail(gomock.Any(), email).Return(identityID, nil)
				mockStorage.EXPECT().AddMember(gomock.Any(), tenantID, identityID, "admin").Return("member-id", nil)
				mockAuthz.EXPECT().AssignTenantMember(gomock.Any(), tenantID, identityID).Return(nil)
				mockMonitor.EXPECT().IncrementCounter(map[string]string{"operation": "user_provisioned", "role": "admin"}).Return(nil)
			},
			expectedErr: false,
		},
		{
			name: "error - kratos error",
			role: "member",
			setupMocks: func(mockStorage *MockStorageInterface, mockAuthz *MockAuthzInterface, mockKratos *MockKratosClientInterface, mockMonitor *MockMonitorInterface) {
				mockKratos.EXPECT().GetIdentityIDByEmail(gomock.Any(), email).Return("", errors.New("kratos error"))
			},
			expectedErr: true,
		},
		{
			name: "error - unknown role",
			role: "superadmin",
			setupMocks: func(mockStorage *MockStorageInterface, mockAuthz *MockAuthzInterface, mockKratos *MockKratosClientInterface, mockMonitor *MockMonitorInterface) {
				mockKratos.EXPECT().GetIdentityIDByEmail(gomock.Any(), email).Return(identityID, nil)
				mockStorage.EXPECT().AddMember(gomock.Any(), tenantID, identityID, "superadmin").Return("member-id", nil)
			},
			expectedErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockStorage := NewMockStorageInterface(ctrl)
			mockAuthz := NewMockAuthzInterface(ctrl)
			mockKratos := NewMockKratosClientInterface(ctrl)
			mockTracer := NewMockTracingInterface(ctrl)
			mockLogger := NewMockLoggerInterface(ctrl)
			setupLoggerMock(ctrl, mockLogger)
			mockMonitor := NewMockMonitorInterface(ctrl)

			s := NewService(mockStorage, mockAuthz, mockKratos, "1h", mockTracer, mockMonitor, mockLogger)

			mockTracer.EXPECT().Start(gomock.Any(), "admin.ProvisionUser").Return(context.Background(), trace.SpanFromContext(context.Background()))
			tc.setupMocks(mockStorage, mockAuthz, mockKratos, mockMonitor)

			err := s.ProvisionUser(context.Background(), tenantID, email, tc.role)

			if tc.expectedErr {
				if err == nil {
					t.Error("expected error but got none")
				}
			} else if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestService_ListTenantUsers(t *testing.T) {
	tenantID := "tenant-123"
	identityID1 := "identity-1"
	identityID2 := "identity-2"
	members := []*types.Membership{
		{KratosIdentityID: identityID1, Role: "owner"},
		{KratosIdentityID: identityID2, Role: "member"},
	}
	identity1 := &ory.Identity{
		Traits: map[string]interface{}{"email": "user1@example.com"},
	}
	identity2 := &ory.Identity{
		Traits: map[string]interface{}{"email": "user2@example.com"},
	}

	testCases := []struct {
		name          string
		includeEmails bool
		setupMocks    func(*MockStorageInterface, *MockKratosClientInterface, *MockLoggerInterface)
		expectedErr   bool
		checkResult   func(t *testing.T, users []*types.TenantUser)
	}{
		{
			name:          "success with emails",
			includeEmails: true,
			setupMocks: func(mockStorage *MockStorageInterface, mockKratos *MockKratosClientInterface, mockLogger *MockLoggerInterface) {
				mockStorage.EXPECT().ListMembersByTenantID(gomock.Any(), tenantID, gomock.Any()).Return(members, "", nil)
				mockKratos.EXPECT().GetIdentities(gomock.Any(), []string{identityID1, identityID2}).Return(map[string]*ory.Identity{
					identityID1: identity1,
					identityID2: identity2,
				}, nil)
			},
			expectedErr: false,
			checkResult: func(t *testing.T, users []*types.TenantUser) {
				if users[0].Email != "user1@example.com" {
					t.Errorf("expected email user1@example.com, got %s", users[0].Email)
				}
				if users[1].Email != "user2@example.com" {
					t.Errorf("expected email user2@example.com, got %s", users[1].Email)
				}
			},
		},
		{
			name:          "include_emails true - kratos error fails",
			includeEmails: true,
			setupMocks: func(mockStorage *MockStorageInterface, mockKratos *MockKratosClientInterface, mockLogger *MockLoggerInterface) {
				mockStorage.EXPECT().ListMembersByTenantID(gomock.Any(), tenantID, gomock.Any()).Return(members, "", nil)
				mockKratos.EXPECT().GetIdentities(gomock.Any(), gomock.Any()).Return(nil, errors.New("kratos error"))
			},
			expectedErr: true,
		},
		{
			name:          "include_emails false - skips kratos",
			includeEmails: false,
			setupMocks: func(mockStorage *MockStorageInterface, mockKratos *MockKratosClientInterface, mockLogger *MockLoggerInterface) {
				mockStorage.EXPECT().ListMembersByTenantID(gomock.Any(), tenantID, gomock.Any()).Return(members, "", nil)
				// No Kratos call expected
			},
			expectedErr: false,
			checkResult: func(t *testing.T, users []*types.TenantUser) {
				for _, u := range users {
					if u.Email != "" {
						t.Errorf("expected empty email with include_emails=false, got %s", u.Email)
					}
				}
			},
		},
		{
			name:          "storage error",
			includeEmails: false,
			setupMocks: func(mockStorage *MockStorageInterface, mockKratos *MockKratosClientInterface, mockLogger *MockLoggerInterface) {
				mockStorage.EXPECT().ListMembersByTenantID(gomock.Any(), tenantID, gomock.Any()).Return(nil, "", errors.New("storage error"))
			},
			expectedErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockStorage := NewMockStorageInterface(ctrl)
			mockAuthz := NewMockAuthzInterface(ctrl)
			mockKratos := NewMockKratosClientInterface(ctrl)
			mockTracer := NewMockTracingInterface(ctrl)
			mockLogger := NewMockLoggerInterface(ctrl)
			setupLoggerMock(ctrl, mockLogger)
			mockMonitor := NewMockMonitorInterface(ctrl)

			s := NewService(mockStorage, mockAuthz, mockKratos, "1h", mockTracer, mockMonitor, mockLogger)

			mockTracer.EXPECT().Start(gomock.Any(), "admin.ListTenantUsers").Return(context.Background(), trace.SpanFromContext(context.Background()))
			tc.setupMocks(mockStorage, mockKratos, mockLogger)

			users, _, err := s.ListTenantUsers(context.Background(), tenantID, tc.includeEmails)

			if tc.expectedErr {
				if err == nil {
					t.Error("expected error but got none")
				}
			} else if err != nil {
				t.Errorf("unexpected error: %v", err)
			} else if users == nil {
				t.Error("expected users but got nil")
			} else if tc.checkResult != nil {
				tc.checkResult(t, users)
			}
		})
	}
}

func TestService_UpdateTenantUser(t *testing.T) {
	tenantID := "tenant-123"
	userID := "user-456"
	identity := &ory.Identity{
		Traits: map[string]interface{}{"email": "user@example.com"},
	}

	testCases := []struct {
		name        string
		newRole     string
		setupMocks  func(*MockStorageInterface, *MockAuthzInterface, *MockKratosClientInterface, *MockLoggerInterface)
		expectedErr bool
	}{
		{
			name:    "success - promote member to owner",
			newRole: "owner",
			setupMocks: func(mockStorage *MockStorageInterface, mockAuthz *MockAuthzInterface, mockKratos *MockKratosClientInterface, mockLogger *MockLoggerInterface) {
				mockStorage.EXPECT().GetMemberByTenantAndUserID(gomock.Any(), tenantID, userID).Return(&types.Membership{KratosIdentityID: userID, Role: "member"}, nil)
				mockAuthz.EXPECT().AssignTenantOwner(gomock.Any(), tenantID, userID).Return(nil)
				mockAuthz.EXPECT().RemoveTenantMember(gomock.Any(), tenantID, userID).Return(nil)
				mockStorage.EXPECT().UpdateMember(gomock.Any(), tenantID, userID, "owner").Return(nil)
				mockKratos.EXPECT().GetIdentity(gomock.Any(), userID).Return(identity, nil)
			},
			expectedErr: false,
		},
		{
			name:    "success - same role no change",
			newRole: "member",
			setupMocks: func(mockStorage *MockStorageInterface, mockAuthz *MockAuthzInterface, mockKratos *MockKratosClientInterface, mockLogger *MockLoggerInterface) {
				mockStorage.EXPECT().GetMemberByTenantAndUserID(gomock.Any(), tenantID, userID).Return(&types.Membership{KratosIdentityID: userID, Role: "member"}, nil)
			},
			expectedErr: false,
		},
		{
			name:    "error - user not found",
			newRole: "owner",
			setupMocks: func(mockStorage *MockStorageInterface, mockAuthz *MockAuthzInterface, mockKratos *MockKratosClientInterface, mockLogger *MockLoggerInterface) {
				mockStorage.EXPECT().GetMemberByTenantAndUserID(gomock.Any(), tenantID, userID).Return(nil, storage.ErrNotFound)
			},
			expectedErr: true,
		},
		{
			name:    "error - invalid role",
			newRole: "superadmin",
			setupMocks: func(mockStorage *MockStorageInterface, mockAuthz *MockAuthzInterface, mockKratos *MockKratosClientInterface, mockLogger *MockLoggerInterface) {
				mockStorage.EXPECT().GetMemberByTenantAndUserID(gomock.Any(), tenantID, userID).Return(&types.Membership{KratosIdentityID: userID, Role: "member"}, nil)
			},
			expectedErr: true,
		},
		{
			name:    "error - kratos identity fetch fails",
			newRole: "owner",
			setupMocks: func(mockStorage *MockStorageInterface, mockAuthz *MockAuthzInterface, mockKratos *MockKratosClientInterface, mockLogger *MockLoggerInterface) {
				mockStorage.EXPECT().GetMemberByTenantAndUserID(gomock.Any(), tenantID, userID).Return(&types.Membership{KratosIdentityID: userID, Role: "member"}, nil)
				mockAuthz.EXPECT().AssignTenantOwner(gomock.Any(), tenantID, userID).Return(nil)
				mockAuthz.EXPECT().RemoveTenantMember(gomock.Any(), tenantID, userID).Return(nil)
				mockStorage.EXPECT().UpdateMember(gomock.Any(), tenantID, userID, "owner").Return(nil)
				mockKratos.EXPECT().GetIdentity(gomock.Any(), userID).Return(nil, errors.New("kratos unavailable"))
			},
			expectedErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockStorage := NewMockStorageInterface(ctrl)
			mockAuthz := NewMockAuthzInterface(ctrl)
			mockKratos := NewMockKratosClientInterface(ctrl)
			mockTracer := NewMockTracingInterface(ctrl)
			mockLogger := NewMockLoggerInterface(ctrl)
			setupLoggerMock(ctrl, mockLogger)
			mockMonitor := NewMockMonitorInterface(ctrl)

			s := NewService(mockStorage, mockAuthz, mockKratos, "1h", mockTracer, mockMonitor, mockLogger)

			mockTracer.EXPECT().Start(gomock.Any(), "admin.UpdateTenantUser").Return(context.Background(), trace.SpanFromContext(context.Background()))
			tc.setupMocks(mockStorage, mockAuthz, mockKratos, mockLogger)

			user, err := s.UpdateTenantUser(context.Background(), tenantID, userID, tc.newRole)

			if tc.expectedErr {
				if err == nil {
					t.Error("expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if user == nil {
					t.Error("expected user but got nil")
				}
			}
		})
	}
}
func TestService_LookupTenantsByEmail(t *testing.T) {
	email := "alice@example.com"
	identityID := "identity-abc"
	expectedTenants := []*types.Tenant{
		{ID: "tenant-1", Name: "My Org", Enabled: true},
	}

	testCases := []struct {
		name        string
		setupMocks  func(*MockStorageInterface, *MockKratosClientInterface)
		expectedLen int
		expectedErr bool
	}{
		{
			name: "success - email found with active tenants",
			setupMocks: func(mockStorage *MockStorageInterface, mockKratos *MockKratosClientInterface) {
				mockKratos.EXPECT().GetIdentityIDByEmail(gomock.Any(), email).Return(identityID, nil)
				mockStorage.EXPECT().ListTenantsByUserID(gomock.Any(), identityID, gomock.Any()).Return(expectedTenants, nil)
			},
			expectedLen: 1,
		},
		{
			name: "success - email not known to Kratos, returns empty list",
			setupMocks: func(mockStorage *MockStorageInterface, mockKratos *MockKratosClientInterface) {
				mockKratos.EXPECT().GetIdentityIDByEmail(gomock.Any(), email).Return("", nil)
			},
			expectedLen: 0,
		},
		{
			name: "error - kratos lookup fails",
			setupMocks: func(mockStorage *MockStorageInterface, mockKratos *MockKratosClientInterface) {
				mockKratos.EXPECT().GetIdentityIDByEmail(gomock.Any(), email).Return("", errors.New("kratos error"))
			},
			expectedErr: true,
		},
		{
			name: "error - storage error on list active tenants",
			setupMocks: func(mockStorage *MockStorageInterface, mockKratos *MockKratosClientInterface) {
				mockKratos.EXPECT().GetIdentityIDByEmail(gomock.Any(), email).Return(identityID, nil)
				mockStorage.EXPECT().ListTenantsByUserID(gomock.Any(), identityID, gomock.Any()).Return(nil, errors.New("db error"))
			},
			expectedErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockStorage := NewMockStorageInterface(ctrl)
			mockAuthz := NewMockAuthzInterface(ctrl)
			mockKratos := NewMockKratosClientInterface(ctrl)
			mockTracer := NewMockTracingInterface(ctrl)
			mockLogger := NewMockLoggerInterface(ctrl)
			setupLoggerMock(ctrl, mockLogger)
			mockMonitor := NewMockMonitorInterface(ctrl)

			s := NewService(mockStorage, mockAuthz, mockKratos, "1h", mockTracer, mockMonitor, mockLogger)

			mockTracer.EXPECT().Start(gomock.Any(), "tenant.Service.LookupTenantsByEmail").
				Return(context.Background(), trace.SpanFromContext(context.Background()))
			tc.setupMocks(mockStorage, mockKratos)

			tenants, err := s.LookupTenantsByEmail(context.Background(), email)

			if tc.expectedErr {
				if err == nil {
					t.Error("expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if len(tenants) != tc.expectedLen {
					t.Errorf("expected %d tenants, got %d", tc.expectedLen, len(tenants))
				}
			}
		})
	}
}
func TestService_LookupTenantsByIdentityID(t *testing.T) {
	identityID := "identity-abc"
	expectedTenants := []*types.Tenant{
		{ID: "tenant-1", Name: "My Org", Enabled: true},
	}

	testCases := []struct {
		name        string
		setupMocks  func(*MockStorageInterface)
		expectedLen int
		expectedErr bool
	}{
		{
			name: "success - active tenants found",
			setupMocks: func(mockStorage *MockStorageInterface) {
				mockStorage.EXPECT().ListTenantsByUserID(gomock.Any(), identityID, gomock.Any()).Return(expectedTenants, nil)
			},
			expectedLen: 1,
		},
		{
			name: "success - no tenants",
			setupMocks: func(mockStorage *MockStorageInterface) {
				mockStorage.EXPECT().ListTenantsByUserID(gomock.Any(), identityID, gomock.Any()).Return([]*types.Tenant{}, nil)
			},
			expectedLen: 0,
		},
		{
			name: "error - storage failure",
			setupMocks: func(mockStorage *MockStorageInterface) {
				mockStorage.EXPECT().ListTenantsByUserID(gomock.Any(), identityID, gomock.Any()).Return(nil, errors.New("db error"))
			},
			expectedErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockStorage := NewMockStorageInterface(ctrl)
			mockAuthz := NewMockAuthzInterface(ctrl)
			mockKratos := NewMockKratosClientInterface(ctrl)
			mockTracer := NewMockTracingInterface(ctrl)
			mockLogger := NewMockLoggerInterface(ctrl)
			setupLoggerMock(ctrl, mockLogger)
			mockMonitor := NewMockMonitorInterface(ctrl)

			s := NewService(mockStorage, mockAuthz, mockKratos, "1h", mockTracer, mockMonitor, mockLogger)

			mockTracer.EXPECT().Start(gomock.Any(), "tenant.Service.LookupTenantsByIdentityID").
				Return(context.Background(), trace.SpanFromContext(context.Background()))
			tc.setupMocks(mockStorage)

			tenants, err := s.LookupTenantsByIdentityID(context.Background(), identityID)

			if tc.expectedErr {
				if err == nil {
					t.Error("expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if len(tenants) != tc.expectedLen {
					t.Errorf("expected %d tenants, got %d", tc.expectedLen, len(tenants))
				}
			}
		})
	}
}

func TestService_ListTenantUsers_EmailFilter(t *testing.T) {
	tenantID := "11111111-1111-1111-1111-111111111111"
	email := "alice@example.com"
	identityID := "22222222-2222-2222-2222-222222222222"
	members := []*types.Membership{
		{ID: "m-1", TenantID: tenantID, KratosIdentityID: identityID, Role: "member"},
	}

	testCases := []struct {
		name        string
		opts        []types.ListOption
		setupMocks  func(*MockStorageInterface, *MockKratosClientInterface)
		expectedLen int
		expectedErr bool
	}{
		{
			name: "email filter resolved to identity_id",
			opts: []types.ListOption{types.WithEmail(email)},
			setupMocks: func(mockStorage *MockStorageInterface, mockKratos *MockKratosClientInterface) {
				mockKratos.EXPECT().GetIdentityIDByEmail(gomock.Any(), email).Return(identityID, nil)
				mockStorage.EXPECT().ListMembersByTenantID(gomock.Any(), tenantID, optionsMatcher{check: func(o types.ListOptions) bool {
					return o.IdentityID == identityID && o.Email == ""
				}}).Return(members, "", nil)
			},
			expectedLen: 1,
		},
		{
			name: "email unknown in kratos returns empty",
			opts: []types.ListOption{types.WithEmail(email)},
			setupMocks: func(mockStorage *MockStorageInterface, mockKratos *MockKratosClientInterface) {
				mockKratos.EXPECT().GetIdentityIDByEmail(gomock.Any(), email).Return("", nil)
			},
			expectedLen: 0,
		},
		{
			name: "kratos error on email resolution",
			opts: []types.ListOption{types.WithEmail(email)},
			setupMocks: func(mockStorage *MockStorageInterface, mockKratos *MockKratosClientInterface) {
				mockKratos.EXPECT().GetIdentityIDByEmail(gomock.Any(), email).Return("", errors.New("kratos error"))
			},
			expectedErr: true,
		},
		{
			name: "role filter passed to storage",
			opts: []types.ListOption{types.WithRole("owner")},
			setupMocks: func(mockStorage *MockStorageInterface, mockKratos *MockKratosClientInterface) {
				mockStorage.EXPECT().ListMembersByTenantID(gomock.Any(), tenantID, gomock.Any()).Return(members, "", nil)
			},
			expectedLen: 1,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockStorage := NewMockStorageInterface(ctrl)
			mockAuthz := NewMockAuthzInterface(ctrl)
			mockKratos := NewMockKratosClientInterface(ctrl)
			mockTracer := NewMockTracingInterface(ctrl)
			mockLogger := NewMockLoggerInterface(ctrl)
			setupLoggerMock(ctrl, mockLogger)
			mockMonitor := NewMockMonitorInterface(ctrl)

			s := NewService(mockStorage, mockAuthz, mockKratos, "1h", mockTracer, mockMonitor, mockLogger)

			mockTracer.EXPECT().Start(gomock.Any(), "admin.ListTenantUsers").
				Return(context.Background(), trace.SpanFromContext(context.Background()))
			tc.setupMocks(mockStorage, mockKratos)

			users, _, err := s.ListTenantUsers(context.Background(), tenantID, false, tc.opts...)

			if tc.expectedErr {
				if err == nil {
					t.Error("expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if len(users) != tc.expectedLen {
					t.Errorf("expected %d users, got %d", tc.expectedLen, len(users))
				}
			}
		})
	}
}

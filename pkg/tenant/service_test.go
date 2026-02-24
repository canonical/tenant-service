// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0

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
				mockStorage.EXPECT().ListTenantsByUserID(gomock.Any(), userID).Return(expectedTenants, nil)
			},
			expectedTenants: expectedTenants,
			expectedErr:     nil,
		},
		{
			name: "empty result",
			setupMocks: func(mockStorage *MockStorageInterface) {
				mockStorage.EXPECT().ListTenantsByUserID(gomock.Any(), userID).Return([]*types.Tenant{}, nil)
			},
			expectedTenants: []*types.Tenant{},
			expectedErr:     nil,
		},
		{
			name: "storage error",
			setupMocks: func(mockStorage *MockStorageInterface) {
				mockStorage.EXPECT().ListTenantsByUserID(gomock.Any(), userID).Return(nil, dbErr)
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
				mockStorage.EXPECT().ListTenants(gomock.Any()).Return(expectedTenants, nil)
			},
			expectedTenants: expectedTenants,
			expectedErr:     nil,
		},
		{
			name: "storage error",
			setupMocks: func(mockStorage *MockStorageInterface) {
				mockStorage.EXPECT().ListTenants(gomock.Any()).Return(nil, dbErr)
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
			mockMonitor := NewMockMonitorInterface(ctrl)

			s := NewService(mockStorage, mockAuthz, mockKratos, "1h", mockTracer, mockMonitor, mockLogger)

			mockTracer.EXPECT().Start(gomock.Any(), "tenant.Service.ListTenants").Return(context.Background(), trace.SpanFromContext(context.Background()))
			tc.setupMocks(mockStorage)

			tenants, err := s.ListTenants(context.Background())

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
		setupMocks   func(*MockStorageInterface, *MockAuthzInterface, *MockKratosClientInterface, *MockLoggerInterface)
		expectedLink string
		expectedCode string
		expectedErr  bool
	}{
		{
			name: "success - new user as member",
			role: "member",
			setupMocks: func(mockStorage *MockStorageInterface, mockAuthz *MockAuthzInterface, mockKratos *MockKratosClientInterface, mockLogger *MockLoggerInterface) {
				mockKratos.EXPECT().GetIdentityIDByEmail(gomock.Any(), email).Return("", nil)
				mockLogger.EXPECT().Infof(gomock.Any(), gomock.Any())
				mockKratos.EXPECT().CreateIdentity(gomock.Any(), email).Return(identityID, nil)
				mockStorage.EXPECT().AddMember(gomock.Any(), tenantID, identityID, "member").Return("member-id", nil)
				mockAuthz.EXPECT().AssignTenantMember(gomock.Any(), tenantID, identityID).Return(nil)
				mockKratos.EXPECT().CreateRecoveryLink(gomock.Any(), identityID, "1h").Return(recoveryLink, recoveryCode, nil)
			},
			expectedLink: recoveryLink,
			expectedCode: recoveryCode,
			expectedErr:  false,
		},
		{
			name: "success - existing user as owner",
			role: "owner",
			setupMocks: func(mockStorage *MockStorageInterface, mockAuthz *MockAuthzInterface, mockKratos *MockKratosClientInterface, mockLogger *MockLoggerInterface) {
				mockKratos.EXPECT().GetIdentityIDByEmail(gomock.Any(), email).Return(identityID, nil)
				mockStorage.EXPECT().AddMember(gomock.Any(), tenantID, identityID, "owner").Return("member-id", nil)
				mockAuthz.EXPECT().AssignTenantOwner(gomock.Any(), tenantID, identityID).Return(nil)
				mockKratos.EXPECT().CreateRecoveryLink(gomock.Any(), identityID, "1h").Return(recoveryLink, recoveryCode, nil)
			},
			expectedLink: recoveryLink,
			expectedCode: recoveryCode,
			expectedErr:  false,
		},
		{
			name: "success - duplicate key treated as reinvite",
			role: "member",
			setupMocks: func(mockStorage *MockStorageInterface, mockAuthz *MockAuthzInterface, mockKratos *MockKratosClientInterface, mockLogger *MockLoggerInterface) {
				mockKratos.EXPECT().GetIdentityIDByEmail(gomock.Any(), email).Return(identityID, nil)
				mockStorage.EXPECT().AddMember(gomock.Any(), tenantID, identityID, "member").Return("", storage.ErrDuplicateKey)
				mockAuthz.EXPECT().AssignTenantMember(gomock.Any(), tenantID, identityID).Return(nil)
				mockKratos.EXPECT().CreateRecoveryLink(gomock.Any(), identityID, "1h").Return(recoveryLink, recoveryCode, nil)
			},
			expectedLink: recoveryLink,
			expectedCode: recoveryCode,
			expectedErr:  false,
		},
		{
			name: "error - failed to check identity",
			role: "member",
			setupMocks: func(mockStorage *MockStorageInterface, mockAuthz *MockAuthzInterface, mockKratos *MockKratosClientInterface, mockLogger *MockLoggerInterface) {
				mockKratos.EXPECT().GetIdentityIDByEmail(gomock.Any(), email).Return("", errors.New("kratos error"))
				mockLogger.EXPECT().Errorf(gomock.Any(), gomock.Any())
			},
			expectedErr: true,
		},
		{
			name: "error - failed to create identity",
			role: "member",
			setupMocks: func(mockStorage *MockStorageInterface, mockAuthz *MockAuthzInterface, mockKratos *MockKratosClientInterface, mockLogger *MockLoggerInterface) {
				mockKratos.EXPECT().GetIdentityIDByEmail(gomock.Any(), email).Return("", nil)
				mockLogger.EXPECT().Infof(gomock.Any(), gomock.Any())
				mockKratos.EXPECT().CreateIdentity(gomock.Any(), email).Return("", errors.New("kratos error"))
				mockLogger.EXPECT().Errorf(gomock.Any(), gomock.Any())
			},
			expectedErr: true,
		},
		{
			name: "error - failed to add member",
			role: "member",
			setupMocks: func(mockStorage *MockStorageInterface, mockAuthz *MockAuthzInterface, mockKratos *MockKratosClientInterface, mockLogger *MockLoggerInterface) {
				mockKratos.EXPECT().GetIdentityIDByEmail(gomock.Any(), email).Return(identityID, nil)
				mockStorage.EXPECT().AddMember(gomock.Any(), tenantID, identityID, "member").Return("", errors.New("storage error"))
				mockLogger.EXPECT().Errorf(gomock.Any(), gomock.Any())
			},
			expectedErr: true,
		},
		{
			name: "error - failed to assign authz",
			role: "member",
			setupMocks: func(mockStorage *MockStorageInterface, mockAuthz *MockAuthzInterface, mockKratos *MockKratosClientInterface, mockLogger *MockLoggerInterface) {
				mockKratos.EXPECT().GetIdentityIDByEmail(gomock.Any(), email).Return(identityID, nil)
				mockStorage.EXPECT().AddMember(gomock.Any(), tenantID, identityID, "member").Return("member-id", nil)
				mockAuthz.EXPECT().AssignTenantMember(gomock.Any(), tenantID, identityID).Return(errors.New("authz error"))
				mockLogger.EXPECT().Errorf(gomock.Any(), gomock.Any())
			},
			expectedErr: true,
		},
		{
			name: "error - failed to create recovery link",
			role: "member",
			setupMocks: func(mockStorage *MockStorageInterface, mockAuthz *MockAuthzInterface, mockKratos *MockKratosClientInterface, mockLogger *MockLoggerInterface) {
				mockKratos.EXPECT().GetIdentityIDByEmail(gomock.Any(), email).Return(identityID, nil)
				mockStorage.EXPECT().AddMember(gomock.Any(), tenantID, identityID, "member").Return("member-id", nil)
				mockAuthz.EXPECT().AssignTenantMember(gomock.Any(), tenantID, identityID).Return(nil)
				mockKratos.EXPECT().CreateRecoveryLink(gomock.Any(), identityID, "1h").Return("", "", errors.New("kratos error"))
				mockLogger.EXPECT().Errorf(gomock.Any(), gomock.Any())
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
			mockMonitor := NewMockMonitorInterface(ctrl)

			s := NewService(mockStorage, mockAuthz, mockKratos, "1h", mockTracer, mockMonitor, mockLogger)

			mockTracer.EXPECT().Start(gomock.Any(), "tenant.Service.InviteMember").Return(context.Background(), trace.SpanFromContext(context.Background()))
			tc.setupMocks(mockStorage, mockAuthz, mockKratos, mockLogger)

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
				mockLogger.EXPECT().Errorf(gomock.Any(), gomock.Any())
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
		setupMocks  func(*MockStorageInterface, *MockAuthzInterface, *MockKratosClientInterface)
		expectedErr bool
	}{
		{
			name: "success - new user as member",
			role: "member",
			setupMocks: func(mockStorage *MockStorageInterface, mockAuthz *MockAuthzInterface, mockKratos *MockKratosClientInterface) {
				mockKratos.EXPECT().GetIdentityIDByEmail(gomock.Any(), email).Return("", nil)
				mockKratos.EXPECT().CreateIdentity(gomock.Any(), email).Return(identityID, nil)
				mockStorage.EXPECT().AddMember(gomock.Any(), tenantID, identityID, "member").Return("member-id", nil)
				mockAuthz.EXPECT().AssignTenantMember(gomock.Any(), tenantID, identityID).Return(nil)
			},
			expectedErr: false,
		},
		{
			name: "success - existing user as owner",
			role: "owner",
			setupMocks: func(mockStorage *MockStorageInterface, mockAuthz *MockAuthzInterface, mockKratos *MockKratosClientInterface) {
				mockKratos.EXPECT().GetIdentityIDByEmail(gomock.Any(), email).Return(identityID, nil)
				mockStorage.EXPECT().AddMember(gomock.Any(), tenantID, identityID, "owner").Return("member-id", nil)
				mockAuthz.EXPECT().AssignTenantOwner(gomock.Any(), tenantID, identityID).Return(nil)
			},
			expectedErr: false,
		},
		{
			name: "success - admin role",
			role: "admin",
			setupMocks: func(mockStorage *MockStorageInterface, mockAuthz *MockAuthzInterface, mockKratos *MockKratosClientInterface) {
				mockKratos.EXPECT().GetIdentityIDByEmail(gomock.Any(), email).Return(identityID, nil)
				mockStorage.EXPECT().AddMember(gomock.Any(), tenantID, identityID, "admin").Return("member-id", nil)
				mockAuthz.EXPECT().AssignTenantMember(gomock.Any(), tenantID, identityID).Return(nil)
			},
			expectedErr: false,
		},
		{
			name: "error - kratos error",
			role: "member",
			setupMocks: func(mockStorage *MockStorageInterface, mockAuthz *MockAuthzInterface, mockKratos *MockKratosClientInterface) {
				mockKratos.EXPECT().GetIdentityIDByEmail(gomock.Any(), email).Return("", errors.New("kratos error"))
			},
			expectedErr: true,
		},
		{
			name: "error - unknown role",
			role: "superadmin",
			setupMocks: func(mockStorage *MockStorageInterface, mockAuthz *MockAuthzInterface, mockKratos *MockKratosClientInterface) {
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
			mockMonitor := NewMockMonitorInterface(ctrl)

			s := NewService(mockStorage, mockAuthz, mockKratos, "1h", mockTracer, mockMonitor, mockLogger)

			mockTracer.EXPECT().Start(gomock.Any(), "admin.ProvisionUser").Return(context.Background(), trace.SpanFromContext(context.Background()))
			tc.setupMocks(mockStorage, mockAuthz, mockKratos)

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

func TestService_ListUserTenants(t *testing.T) {
	userID := "user-123"
	expectedTenants := []*types.Tenant{
		{ID: "tenant-1", Name: "Tenant 1"},
		{ID: "tenant-2", Name: "Tenant 2"},
	}

	testCases := []struct {
		name        string
		setupMocks  func(*MockStorageInterface)
		expectedErr bool
	}{
		{
			name: "success",
			setupMocks: func(mockStorage *MockStorageInterface) {
				mockStorage.EXPECT().ListTenantsByUserID(gomock.Any(), userID).Return(expectedTenants, nil)
			},
			expectedErr: false,
		},
		{
			name: "storage error",
			setupMocks: func(mockStorage *MockStorageInterface) {
				mockStorage.EXPECT().ListTenantsByUserID(gomock.Any(), userID).Return(nil, errors.New("storage error"))
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
			mockMonitor := NewMockMonitorInterface(ctrl)

			s := NewService(mockStorage, mockAuthz, mockKratos, "1h", mockTracer, mockMonitor, mockLogger)

			mockTracer.EXPECT().Start(gomock.Any(), "admin.ListUserTenants").Return(context.Background(), trace.SpanFromContext(context.Background()))
			tc.setupMocks(mockStorage)

			tenants, err := s.ListUserTenants(context.Background(), userID)

			if tc.expectedErr {
				if err == nil {
					t.Error("expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if len(tenants) != len(expectedTenants) {
					t.Errorf("expected %d tenants, got %d", len(expectedTenants), len(tenants))
				}
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
		name        string
		setupMocks  func(*MockStorageInterface, *MockKratosClientInterface, *MockLoggerInterface)
		expectedErr bool
	}{
		{
			name: "success",
			setupMocks: func(mockStorage *MockStorageInterface, mockKratos *MockKratosClientInterface, mockLogger *MockLoggerInterface) {
				mockStorage.EXPECT().ListMembersByTenantID(gomock.Any(), tenantID).Return(members, nil)
				mockKratos.EXPECT().GetIdentity(gomock.Any(), identityID1).Return(identity1, nil)
				mockKratos.EXPECT().GetIdentity(gomock.Any(), identityID2).Return(identity2, nil)
			},
			expectedErr: false,
		},
		{
			name: "success - kratos error handled",
			setupMocks: func(mockStorage *MockStorageInterface, mockKratos *MockKratosClientInterface, mockLogger *MockLoggerInterface) {
				mockStorage.EXPECT().ListMembersByTenantID(gomock.Any(), tenantID).Return(members, nil)
				mockKratos.EXPECT().GetIdentity(gomock.Any(), identityID1).Return(nil, errors.New("kratos error"))
				mockLogger.EXPECT().Warn(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any())
				mockKratos.EXPECT().GetIdentity(gomock.Any(), identityID2).Return(identity2, nil)
			},
			expectedErr: false,
		},
		{
			name: "storage error",
			setupMocks: func(mockStorage *MockStorageInterface, mockKratos *MockKratosClientInterface, mockLogger *MockLoggerInterface) {
				mockStorage.EXPECT().ListMembersByTenantID(gomock.Any(), tenantID).Return(nil, errors.New("storage error"))
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
			mockMonitor := NewMockMonitorInterface(ctrl)

			s := NewService(mockStorage, mockAuthz, mockKratos, "1h", mockTracer, mockMonitor, mockLogger)

			mockTracer.EXPECT().Start(gomock.Any(), "admin.ListTenantUsers").Return(context.Background(), trace.SpanFromContext(context.Background()))
			tc.setupMocks(mockStorage, mockKratos, mockLogger)

			users, err := s.ListTenantUsers(context.Background(), tenantID)

			if tc.expectedErr {
				if err == nil {
					t.Error("expected error but got none")
				}
			} else if err != nil {
				t.Errorf("unexpected error: %v", err)
			} else if users == nil {
				t.Error("expected users but got nil")
			}
		})
	}
}

func TestService_UpdateTenantUser(t *testing.T) {
	tenantID := "tenant-123"
	userID := "user-456"
	currentMembers := []*types.Membership{
		{KratosIdentityID: userID, Role: "member"},
	}
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
				mockStorage.EXPECT().ListMembersByTenantID(gomock.Any(), tenantID).Return(currentMembers, nil)
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
				mockStorage.EXPECT().ListMembersByTenantID(gomock.Any(), tenantID).Return(currentMembers, nil)
			},
			expectedErr: false,
		},
		{
			name:    "error - user not found",
			newRole: "owner",
			setupMocks: func(mockStorage *MockStorageInterface, mockAuthz *MockAuthzInterface, mockKratos *MockKratosClientInterface, mockLogger *MockLoggerInterface) {
				mockStorage.EXPECT().ListMembersByTenantID(gomock.Any(), tenantID).Return([]*types.Membership{}, nil)
			},
			expectedErr: true,
		},
		{
			name:    "error - invalid role",
			newRole: "superadmin",
			setupMocks: func(mockStorage *MockStorageInterface, mockAuthz *MockAuthzInterface, mockKratos *MockKratosClientInterface, mockLogger *MockLoggerInterface) {
				mockStorage.EXPECT().ListMembersByTenantID(gomock.Any(), tenantID).Return(currentMembers, nil)
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

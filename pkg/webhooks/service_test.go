// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0

package webhooks

import (
	"context"
	"errors"
	"testing"

	storagePkg "github.com/canonical/tenant-service/internal/storage"
	"github.com/canonical/tenant-service/internal/types"
	"github.com/ory/hydra/v2/oauth2"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/mock/gomock"
)

//go:generate mockgen -build_flags=--mod=mod -package webhooks -destination ./mock_webhooks.go -source=./interfaces.go
//go:generate mockgen -build_flags=--mod=mod -package webhooks -destination ./mock_logger.go -source=../../internal/logging/interfaces.go
//go:generate mockgen -build_flags=--mod=mod -package webhooks -destination ./mock_monitor.go -source=../../internal/monitoring/interfaces.go
//go:generate mockgen -build_flags=--mod=mod -package webhooks -destination ./mock_tracing.go -source=../../internal/tracing/interfaces.go

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

func TestService_HandleRegistration(t *testing.T) {
	identityID := "identity-123"
	email := "user@example.com"
	tenant := &types.Tenant{ID: "tenant-123", Name: "user@example.com's Org", Enabled: false}

	testCases := []struct {
		name        string
		identityID  string
		email       string
		setupMocks  func(*MockStorageInterface, *MockAuthorizerInterface, *MockLoggerInterface, *MockMonitorInterface)
		expectedErr bool
	}{
		{
			name:       "success",
			identityID: identityID,
			email:      email,
			setupMocks: func(mockStorage *MockStorageInterface, mockAuthz *MockAuthorizerInterface, mockLogger *MockLoggerInterface, mockMonitor *MockMonitorInterface) {
				mockMonitor.EXPECT().IncrementCounter(gomock.Any()).Return(nil).AnyTimes()
				mockStorage.EXPECT().CreateTenant(gomock.Any(), gomock.Any()).DoAndReturn(
					func(_ context.Context, t *types.Tenant) (*types.Tenant, error) {
						if t.Name != "user@example.com's Org" {
							return nil, errors.New("wrong tenant name")
						}
						if t.Enabled {
							return nil, errors.New("tenant should start disabled")
						}
						return tenant, nil
					})
				mockStorage.EXPECT().AddMember(gomock.Any(), tenant.ID, identityID, "owner").Return("member-id", nil)
				mockAuthz.EXPECT().AssignTenantOwner(gomock.Any(), tenant.ID, identityID).Return(nil)
			},
			expectedErr: false,
		},
		{
			name:       "error - empty email",
			identityID: identityID,
			email:      "",
			setupMocks: func(mockStorage *MockStorageInterface, mockAuthz *MockAuthorizerInterface, mockLogger *MockLoggerInterface, mockMonitor *MockMonitorInterface) {
				mockMonitor.EXPECT().IncrementCounter(gomock.Any()).Return(nil).AnyTimes()
			},
			expectedErr: true,
		},
		{
			name:       "error - empty identity id",
			identityID: "",
			email:      email,
			setupMocks: func(mockStorage *MockStorageInterface, mockAuthz *MockAuthorizerInterface, mockLogger *MockLoggerInterface, mockMonitor *MockMonitorInterface) {
				mockMonitor.EXPECT().IncrementCounter(gomock.Any()).Return(nil).AnyTimes()
			},
			expectedErr: true,
		},
		{
			name:       "error - failed to create tenant",
			identityID: identityID,
			email:      email,
			setupMocks: func(mockStorage *MockStorageInterface, mockAuthz *MockAuthorizerInterface, mockLogger *MockLoggerInterface, mockMonitor *MockMonitorInterface) {
				mockMonitor.EXPECT().IncrementCounter(gomock.Any()).Return(nil).AnyTimes()
				mockStorage.EXPECT().CreateTenant(gomock.Any(), gomock.Any()).Return(nil, errors.New("storage error"))
			},
			expectedErr: true,
		},
		{
			name:       "error - failed to add member",
			identityID: identityID,
			email:      email,
			setupMocks: func(mockStorage *MockStorageInterface, mockAuthz *MockAuthorizerInterface, mockLogger *MockLoggerInterface, mockMonitor *MockMonitorInterface) {
				mockMonitor.EXPECT().IncrementCounter(gomock.Any()).Return(nil).AnyTimes()
				mockStorage.EXPECT().CreateTenant(gomock.Any(), gomock.Any()).Return(tenant, nil)
				mockStorage.EXPECT().AddMember(gomock.Any(), tenant.ID, identityID, "owner").Return("", errors.New("storage error"))
			},
			expectedErr: true,
		},
		{
			name:       "error - failed to assign authz",
			identityID: identityID,
			email:      email,
			setupMocks: func(mockStorage *MockStorageInterface, mockAuthz *MockAuthorizerInterface, mockLogger *MockLoggerInterface, mockMonitor *MockMonitorInterface) {
				mockMonitor.EXPECT().IncrementCounter(gomock.Any()).Return(nil).AnyTimes()
				mockStorage.EXPECT().CreateTenant(gomock.Any(), gomock.Any()).Return(tenant, nil)
				mockStorage.EXPECT().AddMember(gomock.Any(), tenant.ID, identityID, "owner").Return("member-id", nil)
				mockAuthz.EXPECT().AssignTenantOwner(gomock.Any(), tenant.ID, identityID).Return(errors.New("authz error"))
			},
			expectedErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockStorage := NewMockStorageInterface(ctrl)
			mockAuthz := NewMockAuthorizerInterface(ctrl)
			mockTracer := NewMockTracingInterface(ctrl)
			mockLogger := NewMockLoggerInterface(ctrl)
			setupLoggerMock(ctrl, mockLogger)
			mockMonitor := NewMockMonitorInterface(ctrl)

			s := NewService(mockStorage, mockAuthz, mockTracer, mockMonitor, mockLogger)

			mockTracer.EXPECT().Start(gomock.Any(), "webhooks.Service.HandleRegistration").
				Return(context.Background(), trace.SpanFromContext(context.Background()))
			tc.setupMocks(mockStorage, mockAuthz, mockLogger, mockMonitor)

			err := s.HandleRegistration(context.Background(), tc.identityID, tc.email)

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

func TestService_HandleTokenHook(t *testing.T) {
	userID := "user-123"
	tenantID := "tenant-456"
	membership := &types.Membership{
		ID:               "mem-1",
		TenantID:         tenantID,
		KratosIdentityID: userID,
		Role:             "owner",
	}

	makeSession := func(subject string, extra map[string]interface{}) *oauth2.TokenHookRequest {
		s := oauth2.NewSession(subject)
		if extra != nil {
			s.Extra = extra
		}
		return &oauth2.TokenHookRequest{Session: s}
	}

	testCases := []struct {
		name         string
		request      *oauth2.TokenHookRequest
		setupMocks   func(*MockStorageInterface, *MockLoggerInterface)
		expectedErr  bool
		validateResp func(*testing.T, *TokenHookResponse)
	}{
		{
			name:    "success - tenant_id in session, valid member",
			request: makeSession(userID, map[string]interface{}{"_tenant_id": tenantID}),
			setupMocks: func(mockStorage *MockStorageInterface, mockLogger *MockLoggerInterface) {
				mockStorage.EXPECT().GetActiveMemberByTenantAndUserID(gomock.Any(), tenantID, userID).Return(membership, nil)
			},
			expectedErr: false,
			validateResp: func(t *testing.T, resp *TokenHookResponse) {
				if resp == nil {
					t.Fatal("expected response but got nil")
				}
				if resp.Session.IDToken["tenant_id"] != tenantID {
					t.Errorf("expected tenant_id=%s in id_token, got %v", tenantID, resp.Session.IDToken["tenant_id"])
				}
				if resp.Session.AccessToken["tenant_id"] != tenantID {
					t.Errorf("expected tenant_id=%s in access_token, got %v", tenantID, resp.Session.AccessToken["tenant_id"])
				}
			},
		},
		{
			name:    "success - no tenant_id in session",
			request: makeSession(userID, nil),
			setupMocks: func(mockStorage *MockStorageInterface, mockLogger *MockLoggerInterface) {
				// no storage call expected
			},
			expectedErr: false,
			validateResp: func(t *testing.T, resp *TokenHookResponse) {
				if resp == nil {
					t.Fatal("expected response but got nil")
				}
				if resp.Session.IDToken["tenant_id"] != nil {
					t.Errorf("expected no tenant_id in id_token, got %v", resp.Session.IDToken["tenant_id"])
				}
				if resp.Session.AccessToken["tenant_id"] != nil {
					t.Errorf("expected no tenant_id in access_token, got %v", resp.Session.AccessToken["tenant_id"])
				}
			},
		},
		{
			name:    "success - tenant_id is empty string",
			request: makeSession(userID, map[string]interface{}{"_tenant_id": ""}),
			setupMocks: func(mockStorage *MockStorageInterface, mockLogger *MockLoggerInterface) {
				// no storage call expected
			},
			expectedErr: false,
			validateResp: func(t *testing.T, resp *TokenHookResponse) {
				if resp == nil {
					t.Fatal("expected response but got nil")
				}
				if resp.Session.IDToken["tenant_id"] != nil {
					t.Errorf("expected no tenant_id in id_token, got %v", resp.Session.IDToken["tenant_id"])
				}
			},
		},
		{
			name:    "error - tenant_id present, user not active member",
			request: makeSession(userID, map[string]interface{}{"_tenant_id": tenantID}),
			setupMocks: func(mockStorage *MockStorageInterface, mockLogger *MockLoggerInterface) {
				mockStorage.EXPECT().GetActiveMemberByTenantAndUserID(gomock.Any(), tenantID, userID).
					Return(nil, storagePkg.ErrNotFound)
			},
			expectedErr: true,
		},
		{
			name: "error - no user id in session",
			request: &oauth2.TokenHookRequest{
				Session: oauth2.NewSession(""),
			},
			setupMocks:  func(*MockStorageInterface, *MockLoggerInterface) {},
			expectedErr: true,
		},
		{
			name:        "error - nil session",
			request:     &oauth2.TokenHookRequest{},
			setupMocks:  func(*MockStorageInterface, *MockLoggerInterface) {},
			expectedErr: true,
		},
		{
			name:    "error - storage error",
			request: makeSession(userID, map[string]interface{}{"_tenant_id": tenantID}),
			setupMocks: func(mockStorage *MockStorageInterface, mockLogger *MockLoggerInterface) {
				mockStorage.EXPECT().GetActiveMemberByTenantAndUserID(gomock.Any(), tenantID, userID).
					Return(nil, errors.New("storage error"))
			},
			expectedErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockStorage := NewMockStorageInterface(ctrl)
			mockAuthz := NewMockAuthorizerInterface(ctrl)
			mockTracer := NewMockTracingInterface(ctrl)
			mockLogger := NewMockLoggerInterface(ctrl)
			setupLoggerMock(ctrl, mockLogger)
			mockMonitor := NewMockMonitorInterface(ctrl)

			s := NewService(mockStorage, mockAuthz, mockTracer, mockMonitor, mockLogger)

			mockTracer.EXPECT().Start(gomock.Any(), "webhooks.Service.HandleTokenHook").
				Return(context.Background(), trace.SpanFromContext(context.Background()))
			tc.setupMocks(mockStorage, mockLogger)

			resp, err := s.HandleTokenHook(context.Background(), tc.request)

			if tc.expectedErr {
				if err == nil {
					t.Error("expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if tc.validateResp != nil {
					tc.validateResp(t, resp)
				}
			}
		})
	}
}

func TestService_HandleLoginHook(t *testing.T) {
	identityID := "identity-abc"
	email := "alice@example.com"
	tenantID := "tenant-xyz"
	membership := &types.Membership{
		ID:               "mem-1",
		TenantID:         tenantID,
		KratosIdentityID: identityID,
		Role:             "owner",
	}
	

	testCases := []struct {
		name        string
		identityID  string
		email       string
		tenantID    string
		setupMocks  func(*MockStorageInterface, *MockAuthorizerInterface, *MockMonitorInterface)
		expectedErr bool
		expectErrIs error
	}{
		{
			name:       "success - valid active member",
			identityID: identityID,
			email:      email,
			tenantID:   tenantID,
			setupMocks: func(mockStorage *MockStorageInterface, mockAuthz *MockAuthorizerInterface, _ *MockMonitorInterface) {
				mockStorage.EXPECT().GetActiveMemberByTenantAndUserID(gomock.Any(), tenantID, identityID).
					Return(membership, nil)
			},
		},
		{
			name:        "error - tenant_id present, user not a member",
			identityID:  identityID,
			email:       email,
			tenantID:    tenantID,
			expectedErr: true,
			expectErrIs: ErrNotMember,
			setupMocks: func(mockStorage *MockStorageInterface, _ *MockAuthorizerInterface, _ *MockMonitorInterface) {
				mockStorage.EXPECT().GetActiveMemberByTenantAndUserID(gomock.Any(), tenantID, identityID).
					Return(nil, storagePkg.ErrNotFound)
			},
		},
		{
			name:       "success - empty identity_id (intermediate auth step, no-op)",
			identityID: "",
			email:      email,
			tenantID:   tenantID,
			setupMocks: func(*MockStorageInterface, *MockAuthorizerInterface, *MockMonitorInterface) {},
		},
		{
			name:        "error - storage error on GetActiveMember",
			identityID:  identityID,
			email:       email,
			tenantID:    tenantID,
			expectedErr: true,
			setupMocks: func(mockStorage *MockStorageInterface, _ *MockAuthorizerInterface, _ *MockMonitorInterface) {
				mockStorage.EXPECT().GetActiveMemberByTenantAndUserID(gomock.Any(), tenantID, identityID).
					Return(nil, errors.New("db error"))
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockStorage := NewMockStorageInterface(ctrl)
			mockAuthz := NewMockAuthorizerInterface(ctrl)
			mockTracer := NewMockTracingInterface(ctrl)
			mockLogger := NewMockLoggerInterface(ctrl)
			setupLoggerMock(ctrl, mockLogger)
			mockMonitor := NewMockMonitorInterface(ctrl)

			s := NewService(mockStorage, mockAuthz, mockTracer, mockMonitor, mockLogger)

			mockTracer.EXPECT().Start(gomock.Any(), "webhooks.Service.HandleLoginHook").
				Return(context.Background(), trace.SpanFromContext(context.Background()))

			tc.setupMocks(mockStorage, mockAuthz, mockMonitor)

			err := s.HandleLoginHook(context.Background(), tc.identityID, tc.email, tc.tenantID)

			if tc.expectedErr {
				if err == nil {
					t.Error("expected error but got none")
					return
				}
				if tc.expectErrIs != nil && !errors.Is(err, tc.expectErrIs) {
					t.Errorf("expected error %v, got %v", tc.expectErrIs, err)
				}
			} else if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

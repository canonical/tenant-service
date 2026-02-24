// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0

package webhooks

import (
	"context"
	"errors"
	"testing"

	"github.com/canonical/tenant-service/internal/types"
	"github.com/ory/hydra/v2/oauth2"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/mock/gomock"
)

//go:generate mockgen -build_flags=--mod=mod -package webhooks -destination ./mock_webhooks.go -source=./interfaces.go
//go:generate mockgen -build_flags=--mod=mod -package webhooks -destination ./mock_logger.go -source=../../internal/logging/interfaces.go
//go:generate mockgen -build_flags=--mod=mod -package webhooks -destination ./mock_monitor.go -source=../../internal/monitoring/interfaces.go
//go:generate mockgen -build_flags=--mod=mod -package webhooks -destination ./mock_tracing.go -source=../../internal/tracing/interfaces.go

func TestService_HandleRegistration(t *testing.T) {
	identityID := "identity-123"
	email := "user@example.com"
	tenant := &types.Tenant{ID: "tenant-123", Name: "user@example.com's Org", Enabled: false}

	testCases := []struct {
		name        string
		identityID  string
		email       string
		setupMocks  func(*MockStorageInterface, *MockAuthorizerInterface, *MockLoggerInterface)
		expectedErr bool
	}{
		{
			name:       "success",
			identityID: identityID,
			email:      email,
			setupMocks: func(mockStorage *MockStorageInterface, mockAuthz *MockAuthorizerInterface, mockLogger *MockLoggerInterface) {
				mockLogger.EXPECT().Debugf(gomock.Any(), gomock.Any(), gomock.Any())
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
				mockLogger.EXPECT().Infof(gomock.Any(), gomock.Any(), gomock.Any())
			},
			expectedErr: false,
		},
		{
			name:       "success - empty email",
			identityID: identityID,
			email:      "",
			setupMocks: func(mockStorage *MockStorageInterface, mockAuthz *MockAuthorizerInterface, mockLogger *MockLoggerInterface) {
				mockLogger.EXPECT().Debugf(gomock.Any(), gomock.Any(), gomock.Any())
				mockStorage.EXPECT().CreateTenant(gomock.Any(), gomock.Any()).DoAndReturn(
					func(_ context.Context, t *types.Tenant) (*types.Tenant, error) {
						if t.Name != "" {
							return nil, errors.New("expected empty tenant name")
						}
						return tenant, nil
					})
				mockStorage.EXPECT().AddMember(gomock.Any(), tenant.ID, identityID, "owner").Return("member-id", nil)
				mockAuthz.EXPECT().AssignTenantOwner(gomock.Any(), tenant.ID, identityID).Return(nil)
				mockLogger.EXPECT().Infof(gomock.Any(), gomock.Any(), gomock.Any())
			},
			expectedErr: false,
		},
		{
			name:       "error - empty identity id",
			identityID: "",
			email:      email,
			setupMocks: func(mockStorage *MockStorageInterface, mockAuthz *MockAuthorizerInterface, mockLogger *MockLoggerInterface) {
				mockLogger.EXPECT().Debugf(gomock.Any(), gomock.Any(), gomock.Any())
			},
			expectedErr: true,
		},
		{
			name:       "error - failed to create tenant",
			identityID: identityID,
			email:      email,
			setupMocks: func(mockStorage *MockStorageInterface, mockAuthz *MockAuthorizerInterface, mockLogger *MockLoggerInterface) {
				mockLogger.EXPECT().Debugf(gomock.Any(), gomock.Any(), gomock.Any())
				mockStorage.EXPECT().CreateTenant(gomock.Any(), gomock.Any()).Return(nil, errors.New("storage error"))
			},
			expectedErr: true,
		},
		{
			name:       "error - failed to add member",
			identityID: identityID,
			email:      email,
			setupMocks: func(mockStorage *MockStorageInterface, mockAuthz *MockAuthorizerInterface, mockLogger *MockLoggerInterface) {
				mockLogger.EXPECT().Debugf(gomock.Any(), gomock.Any(), gomock.Any())
				mockStorage.EXPECT().CreateTenant(gomock.Any(), gomock.Any()).Return(tenant, nil)
				mockStorage.EXPECT().AddMember(gomock.Any(), tenant.ID, identityID, "owner").Return("", errors.New("storage error"))
			},
			expectedErr: true,
		},
		{
			name:       "error - failed to assign authz",
			identityID: identityID,
			email:      email,
			setupMocks: func(mockStorage *MockStorageInterface, mockAuthz *MockAuthorizerInterface, mockLogger *MockLoggerInterface) {
				mockLogger.EXPECT().Debugf(gomock.Any(), gomock.Any(), gomock.Any())
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
			mockMonitor := NewMockMonitorInterface(ctrl)

			s := NewService(mockStorage, mockAuthz, mockTracer, mockMonitor, mockLogger)

			mockTracer.EXPECT().Start(gomock.Any(), "webhooks.Service.HandleRegistration").
				Return(context.Background(), trace.SpanFromContext(context.Background()))
			tc.setupMocks(mockStorage, mockAuthz, mockLogger)

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
	tenants := []*types.Tenant{
		{ID: "tenant-1", Name: "Tenant 1", Enabled: true},
		{ID: "tenant-2", Name: "Tenant 2", Enabled: true},
	}

	testCases := []struct {
		name         string
		request      *oauth2.TokenHookRequest
		setupMocks   func(*MockStorageInterface, *MockLoggerInterface)
		expectedErr  bool
		validateResp func(*testing.T, *TokenHookResponse)
	}{
		{
			name: "success - user with tenants",
			request: &oauth2.TokenHookRequest{
				Session: oauth2.NewSession(userID),
			},
			setupMocks: func(mockStorage *MockStorageInterface, mockLogger *MockLoggerInterface) {
				mockLogger.EXPECT().Debugf(gomock.Any(), gomock.Any()).Times(2)
				mockStorage.EXPECT().ListActiveTenantsByUserID(gomock.Any(), userID).Return(tenants, nil)
			},
			expectedErr: false,
			validateResp: func(t *testing.T, resp *TokenHookResponse) {
				if resp == nil {
					t.Fatal("expected response but got nil")
				}
				if resp.Session.IDToken["tenants"] == nil {
					t.Error("expected tenants in ID token")
				}
				if resp.Session.AccessToken["tenants"] == nil {
					t.Error("expected tenants in access token")
				}
				tenantList, ok := resp.Session.IDToken["tenants"].([]string)
				if !ok || len(tenantList) != 2 {
					t.Errorf("expected 2 tenants in ID token, got %v", resp.Session.IDToken["tenants"])
				}
			},
		},
		{
			name: "success - user with no tenants",
			request: &oauth2.TokenHookRequest{
				Session: oauth2.NewSession(userID),
			},
			setupMocks: func(mockStorage *MockStorageInterface, mockLogger *MockLoggerInterface) {
				mockLogger.EXPECT().Debugf(gomock.Any(), gomock.Any()).Times(2)
				mockStorage.EXPECT().ListActiveTenantsByUserID(gomock.Any(), userID).Return([]*types.Tenant{}, nil)
			},
			expectedErr: false,
			validateResp: func(t *testing.T, resp *TokenHookResponse) {
				if resp == nil {
					t.Fatal("expected response but got nil")
				}
				// Empty tenant list should not add 'tenants' key
				if resp.Session.IDToken["tenants"] != nil {
					t.Error("expected no tenants key in ID token for empty list")
				}
			},
		},
		{
			name: "error - no user id in session",
			request: &oauth2.TokenHookRequest{
				Session: oauth2.NewSession(""),
			},
			setupMocks: func(mockStorage *MockStorageInterface, mockLogger *MockLoggerInterface) {
				mockLogger.EXPECT().Debugf(gomock.Any(), gomock.Any())
			},
			expectedErr: true,
		},
		{
			name:    "error - nil session",
			request: &oauth2.TokenHookRequest{},
			setupMocks: func(mockStorage *MockStorageInterface, mockLogger *MockLoggerInterface) {
				mockLogger.EXPECT().Debugf(gomock.Any(), gomock.Any())
			},
			expectedErr: true,
		},
		{
			name: "error - storage error",
			request: &oauth2.TokenHookRequest{
				Session: oauth2.NewSession(userID),
			},
			setupMocks: func(mockStorage *MockStorageInterface, mockLogger *MockLoggerInterface) {
				mockLogger.EXPECT().Debugf(gomock.Any(), gomock.Any()).Times(2)
				mockStorage.EXPECT().ListActiveTenantsByUserID(gomock.Any(), userID).Return(nil, errors.New("storage error"))
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

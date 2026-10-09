// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package tenant

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	v1 "github.com/canonical/authorization-service/api/v1"
	"github.com/canonical/tenant-service/internal/kratos"
	"github.com/canonical/tenant-service/internal/permissions"
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
	mockSecurityLogger.EXPECT().AdminAction(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()
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
			mockPublisher := permissions.NewMockPublisher(ctrl)
			mockKratos := NewMockKratosClientInterface(ctrl)
			mockTracer := NewMockTracingInterface(ctrl)
			mockLogger := NewMockLoggerInterface(ctrl)
			setupLoggerMock(ctrl, mockLogger)
			mockMonitor := NewMockMonitorInterface(ctrl)

			s := NewService(mockStorage, mockPublisher, mockKratos, time.Hour, mockTracer, mockMonitor, mockLogger)

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
			mockPublisher := permissions.NewMockPublisher(ctrl)
			mockKratos := NewMockKratosClientInterface(ctrl)
			mockTracer := NewMockTracingInterface(ctrl)
			mockLogger := NewMockLoggerInterface(ctrl)
			setupLoggerMock(ctrl, mockLogger)
			mockMonitor := NewMockMonitorInterface(ctrl)

			s := NewService(mockStorage, mockPublisher, mockKratos, time.Hour, mockTracer, mockMonitor, mockLogger)

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
	const email = "Bob@initech.example"
	invited := strings.ToLower(email)
	canView := grantOp(tTenant, tIdentity, permissions.RelationCanView)

	testCases := []struct {
		name        string
		setup       func(*MockStorageInterface, *MockKratosClientInterface, *permissions.MockPublisher)
		wantErr     error
		wantAny     bool
		wantLink    bool
		wantPending bool
	}{
		{
			name: "a personal tenant: nobody is invited to it",
			setup: func(st *MockStorageInterface, k *MockKratosClientInterface, p *permissions.MockPublisher) {
				k.EXPECT().GetIdentityIDByEmail(gomock.Any(), email).Return("", nil)
				st.EXPECT().GetTenantByID(gomock.Any(), tTenant).Return(personalTenant(), nil)
			},
			wantErr: ErrPersonalTenant,
		},
		{
			// Kratos is asked for the account before the first statement, and
			// creates it and its recovery link before the first write.
			name: "new account at an OFF tenant: recovery link and code, then membership, its invitation cleared, then can_view",
			setup: func(st *MockStorageInterface, k *MockKratosClientInterface, p *permissions.MockPublisher) {
				gomock.InOrder(
					k.EXPECT().GetIdentityIDByEmail(gomock.Any(), email).Return("", nil),
					st.EXPECT().GetTenantByID(gomock.Any(), tTenant).Return(orgTenant(), nil),
					st.EXPECT().GetTenantSSOPolicy(gomock.Any(), tTenant).Return(storedPolicy(types.EnforcementOff, false, nil), nil),
					k.EXPECT().CreateIdentity(gomock.Any(), email).Return(tIdentity, nil),
					k.EXPECT().CreateRecoveryLink(gomock.Any(), tIdentity, "1h0m0s").Return("https://link", "123456", nil),
					st.EXPECT().AddMember(gomock.Any(), tTenant, tIdentity).Return("m", nil),
					st.EXPECT().DeleteInvitation(gomock.Any(), tTenant, invited).Return(nil),
					p.EXPECT().Publish(gomock.Any(), tTenant, canView).Times(1),
				)
			},
			wantLink: true,
		},
		{
			name: "new account at an OPTIONAL tenant: can_view, recovery link",
			setup: func(st *MockStorageInterface, k *MockKratosClientInterface, p *permissions.MockPublisher) {
				k.EXPECT().GetIdentityIDByEmail(gomock.Any(), email).Return("", nil)
				st.EXPECT().GetTenantByID(gomock.Any(), tTenant).Return(orgTenant(), nil)
				st.EXPECT().GetTenantSSOPolicy(gomock.Any(), tTenant).Return(policy(types.EnforcementOptional, true, false), nil)
				k.EXPECT().CreateIdentity(gomock.Any(), email).Return(tIdentity, nil)
				k.EXPECT().CreateRecoveryLink(gomock.Any(), tIdentity, "1h0m0s").Return("https://link", "123456", nil)
				st.EXPECT().AddMember(gomock.Any(), tTenant, tIdentity).Return("m", nil)
				st.EXPECT().DeleteInvitation(gomock.Any(), tTenant, invited).Return(nil)
				p.EXPECT().Publish(gomock.Any(), tTenant, canView).Times(1)
			},
			wantLink: true,
		},
		{
			name: "existing account anywhere: a pending invitation, no membership, nothing published, never a recovery code",
			setup: func(st *MockStorageInterface, k *MockKratosClientInterface, p *permissions.MockPublisher) {
				k.EXPECT().GetIdentityIDByEmail(gomock.Any(), email).Return(tIdentity, nil)
				st.EXPECT().GetTenantByID(gomock.Any(), tTenant).Return(orgTenant(), nil)
				st.EXPECT().GetTenantSSOPolicy(gomock.Any(), tTenant).Return(storedPolicy(types.EnforcementOff, false, nil), nil)
				st.EXPECT().GetMemberByTenantAndUserID(gomock.Any(), tTenant, tIdentity).Return(nil, storage.ErrNotFound)
				st.EXPECT().AddInvitation(gomock.Any(), tTenant, invited, time.Hour).Return(nil)
			},
			wantPending: true,
		},
		{
			name: "new address at a REQUIRED tenant: a pending invitation, no account, nothing published",
			setup: func(st *MockStorageInterface, k *MockKratosClientInterface, p *permissions.MockPublisher) {
				k.EXPECT().GetIdentityIDByEmail(gomock.Any(), email).Return("", nil)
				st.EXPECT().GetTenantByID(gomock.Any(), tTenant).Return(orgTenant(), nil)
				st.EXPECT().GetTenantSSOPolicy(gomock.Any(), tTenant).Return(policy(types.EnforcementRequired, true, false, "initech.example"), nil)
				st.EXPECT().AddInvitation(gomock.Any(), tTenant, invited, time.Hour).Return(nil)
			},
			wantPending: true,
		},
		{
			name: "stored REQUIRED without an active binding is OFF: recovery link",
			setup: func(st *MockStorageInterface, k *MockKratosClientInterface, p *permissions.MockPublisher) {
				k.EXPECT().GetIdentityIDByEmail(gomock.Any(), email).Return("", nil)
				st.EXPECT().GetTenantByID(gomock.Any(), tTenant).Return(orgTenant(), nil)
				st.EXPECT().GetTenantSSOPolicy(gomock.Any(), tTenant).Return(policy(types.EnforcementRequired, false, false, "other.example"), nil)
				k.EXPECT().CreateIdentity(gomock.Any(), email).Return(tIdentity, nil)
				k.EXPECT().CreateRecoveryLink(gomock.Any(), tIdentity, "1h0m0s").Return("https://link", "123456", nil)
				st.EXPECT().AddMember(gomock.Any(), tTenant, tIdentity).Return("m", nil)
				st.EXPECT().DeleteInvitation(gomock.Any(), tTenant, invited).Return(nil)
				p.EXPECT().Publish(gomock.Any(), tTenant, canView).Times(1)
			},
			wantLink: true,
		},
		{
			name: "REQUIRED tenant with domains: an address outside them is refused",
			setup: func(st *MockStorageInterface, k *MockKratosClientInterface, p *permissions.MockPublisher) {
				k.EXPECT().GetIdentityIDByEmail(gomock.Any(), email).Return("", nil)
				st.EXPECT().GetTenantByID(gomock.Any(), tTenant).Return(orgTenant(), nil)
				st.EXPECT().GetTenantSSOPolicy(gomock.Any(), tTenant).Return(policy(types.EnforcementRequired, true, false, "initech.test"), nil)
			},
			wantErr: ErrDomainNotAllowed,
		},
		{
			name: "re-invite of a member: no insert, nothing published",
			setup: func(st *MockStorageInterface, k *MockKratosClientInterface, p *permissions.MockPublisher) {
				k.EXPECT().GetIdentityIDByEmail(gomock.Any(), email).Return(tIdentity, nil)
				st.EXPECT().GetTenantByID(gomock.Any(), tTenant).Return(orgTenant(), nil)
				st.EXPECT().GetTenantSSOPolicy(gomock.Any(), tTenant).Return(storedPolicy(types.EnforcementOff, false, nil), nil)
				st.EXPECT().GetMemberByTenantAndUserID(gomock.Any(), tTenant, tIdentity).Return(&types.Membership{}, nil)
			},
		},
		{
			name: "unknown tenant: no account created",
			setup: func(st *MockStorageInterface, k *MockKratosClientInterface, p *permissions.MockPublisher) {
				k.EXPECT().GetIdentityIDByEmail(gomock.Any(), email).Return("", nil)
				st.EXPECT().GetTenantByID(gomock.Any(), tTenant).Return(nil, storage.ErrNotFound)
			},
			wantErr: ErrTenantNotFound,
		},
		{
			name: "kratos failure: no statement",
			setup: func(st *MockStorageInterface, k *MockKratosClientInterface, p *permissions.MockPublisher) {
				k.EXPECT().GetIdentityIDByEmail(gomock.Any(), email).Return("", errors.New("kratos down"))
			},
			wantAny: true,
		},
		{
			name: "recovery link fails: no membership, nothing published",
			setup: func(st *MockStorageInterface, k *MockKratosClientInterface, p *permissions.MockPublisher) {
				k.EXPECT().GetIdentityIDByEmail(gomock.Any(), email).Return("", nil)
				st.EXPECT().GetTenantByID(gomock.Any(), tTenant).Return(orgTenant(), nil)
				st.EXPECT().GetTenantSSOPolicy(gomock.Any(), tTenant).Return(storedPolicy(types.EnforcementOff, false, nil), nil)
				k.EXPECT().CreateIdentity(gomock.Any(), email).Return(tIdentity, nil)
				k.EXPECT().CreateRecoveryLink(gomock.Any(), tIdentity, "1h0m0s").Return("", "", errors.New("kratos down"))
			},
			wantAny: true,
		},
		{
			name: "membership write fails: nothing published",
			setup: func(st *MockStorageInterface, k *MockKratosClientInterface, p *permissions.MockPublisher) {
				k.EXPECT().GetIdentityIDByEmail(gomock.Any(), email).Return("", nil)
				st.EXPECT().GetTenantByID(gomock.Any(), tTenant).Return(orgTenant(), nil)
				st.EXPECT().GetTenantSSOPolicy(gomock.Any(), tTenant).Return(storedPolicy(types.EnforcementOff, false, nil), nil)
				k.EXPECT().CreateIdentity(gomock.Any(), email).Return(tIdentity, nil)
				k.EXPECT().CreateRecoveryLink(gomock.Any(), tIdentity, "1h0m0s").Return("https://link", "123456", nil)
				st.EXPECT().AddMember(gomock.Any(), tTenant, tIdentity).Return("", errors.New("db down"))
			},
			wantAny: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			s, st, k, p, m := newTestService(ctrl)
			m.EXPECT().IncrementCounter(gomock.Any()).Return(nil).AnyTimes()
			tc.setup(st, k, p)

			inv, err := s.InviteMember(context.Background(), tTenant, email)
			switch {
			case tc.wantErr != nil:
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("want %v, got %v", tc.wantErr, err)
				}
			case tc.wantAny:
				if err == nil {
					t.Fatal("expected an error")
				}
			default:
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if inv.Pending != tc.wantPending {
					t.Fatalf("pending invitation: want %v, got %+v", tc.wantPending, inv)
				}
				if got := inv.Link != "" && inv.Code != ""; got != tc.wantLink {
					t.Fatalf("recovery link: want %v, got %+v", tc.wantLink, inv)
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
			mockPublisher := permissions.NewMockPublisher(ctrl)
			mockKratos := NewMockKratosClientInterface(ctrl)
			mockTracer := NewMockTracingInterface(ctrl)
			mockLogger := NewMockLoggerInterface(ctrl)
			setupLoggerMock(ctrl, mockLogger)
			mockMonitor := NewMockMonitorInterface(ctrl)

			s := NewService(mockStorage, mockPublisher, mockKratos, time.Hour, mockTracer, mockMonitor, mockLogger)

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
			mockPublisher := permissions.NewMockPublisher(ctrl)
			mockKratos := NewMockKratosClientInterface(ctrl)
			mockTracer := NewMockTracingInterface(ctrl)
			mockLogger := NewMockLoggerInterface(ctrl)
			setupLoggerMock(ctrl, mockLogger)
			mockMonitor := NewMockMonitorInterface(ctrl)

			s := NewService(mockStorage, mockPublisher, mockKratos, time.Hour, mockTracer, mockMonitor, mockLogger)

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

	revokeOps := func(identityIDs ...string) []any {
		ops := make([]any, 0, len(identityIDs)*3)
		for _, id := range identityIDs {
			for _, relation := range []string{permissions.RelationCanView, permissions.RelationCanEdit, permissions.RelationCanDelete} {
				ops = append(ops, &v1.PermissionOperation{
					Op:       v1.PermissionOp_PERMISSION_OP_DELETE,
					Subject:  "user:" + id,
					Relation: relation,
					Object:   "tenant:" + tenantID,
				})
			}
		}
		return ops
	}

	pageToken := func(token string) gomock.Matcher {
		return optionsMatcher{check: func(o types.ListOptions) bool { return o.PageToken == token }}
	}

	testCases := []struct {
		name        string
		setupMocks  func(*MockStorageInterface, *permissions.MockPublisher, *MockLoggerInterface)
		expectedErr bool
	}{
		{
			name: "success - single page revokes all relations per member",
			setupMocks: func(mockStorage *MockStorageInterface, mockPublisher *permissions.MockPublisher, mockLogger *MockLoggerInterface) {
				gomock.InOrder(
					mockStorage.EXPECT().ListMembersByTenantID(gomock.Any(), tenantID, pageToken("")).Return([]*types.Membership{
						{KratosIdentityID: "identity-1"},
						{KratosIdentityID: "identity-2"},
					}, "", nil),
					mockStorage.EXPECT().DeleteTenant(gomock.Any(), tenantID).Return(nil),
					mockPublisher.EXPECT().Publish(gomock.Any(), tenantID, revokeOps("identity-1", "identity-2")...).Times(1),
				)
			},
			expectedErr: false,
		},
		{
			name: "success - multiple pages are all collected into a single publish",
			setupMocks: func(mockStorage *MockStorageInterface, mockPublisher *permissions.MockPublisher, mockLogger *MockLoggerInterface) {
				gomock.InOrder(
					mockStorage.EXPECT().ListMembersByTenantID(gomock.Any(), tenantID, pageToken("")).Return([]*types.Membership{
						{KratosIdentityID: "identity-1"},
					}, "page-2", nil),
					mockStorage.EXPECT().ListMembersByTenantID(gomock.Any(), tenantID, pageToken("page-2")).Return([]*types.Membership{
						{KratosIdentityID: "identity-2"},
					}, "", nil),
					mockStorage.EXPECT().DeleteTenant(gomock.Any(), tenantID).Return(nil),
					mockPublisher.EXPECT().Publish(gomock.Any(), tenantID, revokeOps("identity-1", "identity-2")...).Times(1),
				)
			},
			expectedErr: false,
		},
		{
			name: "success - no members publishes no ops",
			setupMocks: func(mockStorage *MockStorageInterface, mockPublisher *permissions.MockPublisher, mockLogger *MockLoggerInterface) {
				gomock.InOrder(
					mockStorage.EXPECT().ListMembersByTenantID(gomock.Any(), tenantID, pageToken("")).Return(nil, "", nil),
					mockStorage.EXPECT().DeleteTenant(gomock.Any(), tenantID).Return(nil),
					mockPublisher.EXPECT().Publish(gomock.Any(), tenantID).Times(1),
				)
			},
			expectedErr: false,
		},
		{
			name: "storage error - nothing published",
			setupMocks: func(mockStorage *MockStorageInterface, mockPublisher *permissions.MockPublisher, mockLogger *MockLoggerInterface) {
				mockStorage.EXPECT().ListMembersByTenantID(gomock.Any(), tenantID, pageToken("")).Return([]*types.Membership{
					{KratosIdentityID: "identity-1"},
				}, "", nil)
				mockStorage.EXPECT().DeleteTenant(gomock.Any(), tenantID).Return(errors.New("storage error"))
			},
			expectedErr: true,
		},
		{
			name: "list members error - fails tenant deletion",
			setupMocks: func(mockStorage *MockStorageInterface, mockPublisher *permissions.MockPublisher, mockLogger *MockLoggerInterface) {
				mockStorage.EXPECT().ListMembersByTenantID(gomock.Any(), tenantID, pageToken("")).Return(nil, "", errors.New("list error"))
			},
			expectedErr: true,
		},
		{
			name: "list members error on second page - fails tenant deletion",
			setupMocks: func(mockStorage *MockStorageInterface, mockPublisher *permissions.MockPublisher, mockLogger *MockLoggerInterface) {
				gomock.InOrder(
					mockStorage.EXPECT().ListMembersByTenantID(gomock.Any(), tenantID, pageToken("")).Return([]*types.Membership{
						{KratosIdentityID: "identity-1"},
					}, "page-2", nil),
					mockStorage.EXPECT().ListMembersByTenantID(gomock.Any(), tenantID, pageToken("page-2")).Return(nil, "", errors.New("list error")),
				)
			},
			expectedErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockStorage := NewMockStorageInterface(ctrl)
			mockPublisher := permissions.NewMockPublisher(ctrl)
			mockKratos := NewMockKratosClientInterface(ctrl)
			mockTracer := NewMockTracingInterface(ctrl)
			mockLogger := NewMockLoggerInterface(ctrl)
			setupLoggerMock(ctrl, mockLogger)
			mockMonitor := NewMockMonitorInterface(ctrl)

			s := NewService(mockStorage, mockPublisher, mockKratos, time.Hour, mockTracer, mockMonitor, mockLogger)

			mockTracer.EXPECT().Start(gomock.Any(), "admin.DeleteTenant").Return(context.Background(), trace.SpanFromContext(context.Background()))
			tc.setupMocks(mockStorage, mockPublisher, mockLogger)

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
	const email = "Hank@hooli.example"
	canView := grantOp(tTenant, tIdentity, permissions.RelationCanView)

	testCases := []struct {
		name    string
		setup   func(*MockStorageInterface, *MockKratosClientInterface, *permissions.MockPublisher)
		wantErr error
		wantAny bool
	}{
		{
			// The account is created only for a tenant that exists, and before
			// the first write.
			name: "new account, membership, its invitation cleared, then can_view",
			setup: func(st *MockStorageInterface, k *MockKratosClientInterface, p *permissions.MockPublisher) {
				gomock.InOrder(
					k.EXPECT().GetIdentityIDByEmail(gomock.Any(), email).Return("", nil),
					st.EXPECT().GetTenantByID(gomock.Any(), tTenant).Return(orgTenant(), nil),
					k.EXPECT().CreateIdentity(gomock.Any(), email).Return(tIdentity, nil),
					st.EXPECT().AddMember(gomock.Any(), tTenant, tIdentity).Return("m", nil),
					st.EXPECT().DeleteInvitation(gomock.Any(), tTenant, strings.ToLower(email)).Return(nil),
					p.EXPECT().Publish(gomock.Any(), tTenant, canView).Times(1),
				)
			},
		},
		{
			// No admission is asked for: neither an invitation nor the policy is read.
			name: "existing account, membership, then can_view",
			setup: func(st *MockStorageInterface, k *MockKratosClientInterface, p *permissions.MockPublisher) {
				gomock.InOrder(
					k.EXPECT().GetIdentityIDByEmail(gomock.Any(), email).Return(tIdentity, nil),
					st.EXPECT().GetTenantByID(gomock.Any(), tTenant).Return(orgTenant(), nil),
					st.EXPECT().GetMemberByTenantAndUserID(gomock.Any(), tTenant, tIdentity).Return(nil, storage.ErrNotFound),
					st.EXPECT().AddMember(gomock.Any(), tTenant, tIdentity).Return("m", nil),
					st.EXPECT().DeleteInvitation(gomock.Any(), tTenant, strings.ToLower(email)).Return(nil),
					p.EXPECT().Publish(gomock.Any(), tTenant, canView).Times(1),
				)
			},
		},
		{
			name: "membership write fails, nothing published",
			setup: func(st *MockStorageInterface, k *MockKratosClientInterface, p *permissions.MockPublisher) {
				st.EXPECT().GetTenantByID(gomock.Any(), tTenant).Return(orgTenant(), nil)
				k.EXPECT().GetIdentityIDByEmail(gomock.Any(), email).Return(tIdentity, nil)
				st.EXPECT().GetMemberByTenantAndUserID(gomock.Any(), tTenant, tIdentity).Return(nil, storage.ErrNotFound)
				st.EXPECT().AddMember(gomock.Any(), tTenant, tIdentity).Return("", errors.New("db down"))
			},
			wantAny: true,
		},
		{
			name: "an existing member is AlreadyExists, nothing published",
			setup: func(st *MockStorageInterface, k *MockKratosClientInterface, p *permissions.MockPublisher) {
				st.EXPECT().GetTenantByID(gomock.Any(), tTenant).Return(orgTenant(), nil)
				k.EXPECT().GetIdentityIDByEmail(gomock.Any(), email).Return(tIdentity, nil)
				st.EXPECT().GetMemberByTenantAndUserID(gomock.Any(), tTenant, tIdentity).Return(&types.Membership{}, nil)
			},
			wantErr: ErrAlreadyMember,
		},
		{
			name: "a concurrent call added the membership first: AlreadyExists, nothing published",
			setup: func(st *MockStorageInterface, k *MockKratosClientInterface, p *permissions.MockPublisher) {
				st.EXPECT().GetTenantByID(gomock.Any(), tTenant).Return(orgTenant(), nil)
				k.EXPECT().GetIdentityIDByEmail(gomock.Any(), email).Return(tIdentity, nil)
				st.EXPECT().GetMemberByTenantAndUserID(gomock.Any(), tTenant, tIdentity).Return(nil, storage.ErrNotFound)
				st.EXPECT().AddMember(gomock.Any(), tTenant, tIdentity).Return("", storage.ErrDuplicateKey)
			},
			wantErr: ErrAlreadyMember,
		},
		{
			name: "a personal tenant is refused, no account created",
			setup: func(st *MockStorageInterface, k *MockKratosClientInterface, p *permissions.MockPublisher) {
				k.EXPECT().GetIdentityIDByEmail(gomock.Any(), email).Return("", nil)
				st.EXPECT().GetTenantByID(gomock.Any(), tTenant).Return(personalTenant(), nil)
			},
			wantErr: ErrPersonalTenant,
		},
		{
			name: "unknown tenant, no account created",
			setup: func(st *MockStorageInterface, k *MockKratosClientInterface, p *permissions.MockPublisher) {
				k.EXPECT().GetIdentityIDByEmail(gomock.Any(), email).Return("", nil)
				st.EXPECT().GetTenantByID(gomock.Any(), tTenant).Return(nil, storage.ErrNotFound)
			},
			wantErr: ErrTenantNotFound,
		},
		{
			name: "failed to create identity, nothing written",
			setup: func(st *MockStorageInterface, k *MockKratosClientInterface, p *permissions.MockPublisher) {
				k.EXPECT().GetIdentityIDByEmail(gomock.Any(), email).Return("", nil)
				st.EXPECT().GetTenantByID(gomock.Any(), tTenant).Return(orgTenant(), nil)
				k.EXPECT().CreateIdentity(gomock.Any(), email).Return("", errors.New("kratos error"))
			},
			wantAny: true,
		},
		{
			name: "kratos failure: no statement",
			setup: func(st *MockStorageInterface, k *MockKratosClientInterface, p *permissions.MockPublisher) {
				k.EXPECT().GetIdentityIDByEmail(gomock.Any(), email).Return("", errors.New("kratos down"))
			},
			wantAny: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			s, st, k, p, m := newTestService(ctrl)
			m.EXPECT().IncrementCounter(map[string]string{"operation": "user_provisioned"}).Return(nil).AnyTimes()
			tc.setup(st, k, p)

			err := s.ProvisionUser(context.Background(), tTenant, email)
			switch {
			case tc.wantErr != nil:
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("want %v, got %v", tc.wantErr, err)
				}
			case tc.wantAny:
				if err == nil {
					t.Fatal("expected an error")
				}
			default:
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestService_JoinTenant(t *testing.T) {
	const email = "Hank@hooli.example"
	address := strings.ToLower(email)
	hooli := func() *types.TenantSSOPolicy {
		return policy(types.EnforcementRequired, true, true, "hooli.example")
	}
	canView := grantOp(tTenant, tIdentity, permissions.RelationCanView)

	testCases := []struct {
		name    string
		setup   func(*MockStorageInterface, *MockKratosClientInterface, *permissions.MockPublisher)
		wantErr error
		wantAny bool
	}{
		{
			// Kratos is asked before the first statement.
			name: "auto-join, membership, then can_view",
			setup: func(st *MockStorageInterface, k *MockKratosClientInterface, p *permissions.MockPublisher) {
				gomock.InOrder(
					k.EXPECT().GetIdentity(gomock.Any(), tIdentity).Return(identity(tIdentity, email), nil),
					st.EXPECT().GetTenantByID(gomock.Any(), tTenant).Return(orgTenant(), nil),
				)
				st.EXPECT().GetMemberByTenantAndUserID(gomock.Any(), tTenant, tIdentity).Return(nil, storage.ErrNotFound)
				st.EXPECT().HasInvitationByTenantAndEmail(gomock.Any(), tTenant, address).Return(false, nil)
				st.EXPECT().GetTenantSSOPolicy(gomock.Any(), tTenant).Return(hooli(), nil)
				gomock.InOrder(
					st.EXPECT().AddMember(gomock.Any(), tTenant, tIdentity).Return("m", nil),
					st.EXPECT().DeleteInvitation(gomock.Any(), tTenant, address).Return(nil),
					p.EXPECT().Publish(gomock.Any(), tTenant, canView).Times(1),
				)
			},
		},
		{
			name: "a pending invitation admits, is spent, then can_view",
			setup: func(st *MockStorageInterface, k *MockKratosClientInterface, p *permissions.MockPublisher) {
				st.EXPECT().GetTenantByID(gomock.Any(), tTenant).Return(orgTenant(), nil)
				k.EXPECT().GetIdentity(gomock.Any(), tIdentity).Return(identity(tIdentity, email), nil)
				st.EXPECT().GetMemberByTenantAndUserID(gomock.Any(), tTenant, tIdentity).Return(nil, storage.ErrNotFound)
				st.EXPECT().GetTenantSSOPolicy(gomock.Any(), tTenant).Return(storedPolicy(types.EnforcementOff, false, nil), nil)
				st.EXPECT().HasInvitationByTenantAndEmail(gomock.Any(), tTenant, address).Return(true, nil)
				gomock.InOrder(
					st.EXPECT().AddMember(gomock.Any(), tTenant, tIdentity).Return("m", nil),
					st.EXPECT().DeleteInvitation(gomock.Any(), tTenant, address).Return(nil),
					p.EXPECT().Publish(gomock.Any(), tTenant, canView).Times(1),
				)
			},
		},
		{
			name: "a pending invitation of an address outside the domains the tenant has got since admits nothing",
			setup: func(st *MockStorageInterface, k *MockKratosClientInterface, p *permissions.MockPublisher) {
				st.EXPECT().GetTenantByID(gomock.Any(), tTenant).Return(orgTenant(), nil)
				k.EXPECT().GetIdentity(gomock.Any(), tIdentity).Return(identity(tIdentity, email), nil)
				st.EXPECT().GetMemberByTenantAndUserID(gomock.Any(), tTenant, tIdentity).Return(nil, storage.ErrNotFound)
				st.EXPECT().GetTenantSSOPolicy(gomock.Any(), tTenant).Return(policy(types.EnforcementRequired, true, false, "elsewhere.example"), nil)
				st.EXPECT().HasInvitationByTenantAndEmail(gomock.Any(), tTenant, address).Return(true, nil)
			},
			wantErr: ErrNotAdmitted,
		},
		{
			name: "unknown identity: no statement",
			setup: func(st *MockStorageInterface, k *MockKratosClientInterface, p *permissions.MockPublisher) {
				k.EXPECT().GetIdentity(gomock.Any(), tIdentity).Return(nil, kratos.ErrNotFound)
			},
			wantErr: ErrIdentityNotFound,
		},
		{
			name: "a member already is success, nothing written or published",
			setup: func(st *MockStorageInterface, k *MockKratosClientInterface, p *permissions.MockPublisher) {
				st.EXPECT().GetTenantByID(gomock.Any(), tTenant).Return(orgTenant(), nil)
				k.EXPECT().GetIdentity(gomock.Any(), tIdentity).Return(identity(tIdentity, email), nil)
				st.EXPECT().GetMemberByTenantAndUserID(gomock.Any(), tTenant, tIdentity).Return(&types.Membership{}, nil)
			},
		},
		{
			name: "a concurrent call added the membership first: success, nothing published",
			setup: func(st *MockStorageInterface, k *MockKratosClientInterface, p *permissions.MockPublisher) {
				st.EXPECT().GetTenantByID(gomock.Any(), tTenant).Return(orgTenant(), nil)
				k.EXPECT().GetIdentity(gomock.Any(), tIdentity).Return(identity(tIdentity, email), nil)
				st.EXPECT().GetMemberByTenantAndUserID(gomock.Any(), tTenant, tIdentity).Return(nil, storage.ErrNotFound)
				st.EXPECT().GetTenantSSOPolicy(gomock.Any(), tTenant).Return(storedPolicy(types.EnforcementOff, false, nil), nil)
				st.EXPECT().HasInvitationByTenantAndEmail(gomock.Any(), tTenant, address).Return(true, nil)
				st.EXPECT().AddMember(gomock.Any(), tTenant, tIdentity).Return("", storage.ErrDuplicateKey)
			},
		},
		{
			name: "membership write fails, nothing published",
			setup: func(st *MockStorageInterface, k *MockKratosClientInterface, p *permissions.MockPublisher) {
				st.EXPECT().GetTenantByID(gomock.Any(), tTenant).Return(orgTenant(), nil)
				k.EXPECT().GetIdentity(gomock.Any(), tIdentity).Return(identity(tIdentity, email), nil)
				st.EXPECT().GetMemberByTenantAndUserID(gomock.Any(), tTenant, tIdentity).Return(nil, storage.ErrNotFound)
				st.EXPECT().GetTenantSSOPolicy(gomock.Any(), tTenant).Return(hooli(), nil)
				st.EXPECT().HasInvitationByTenantAndEmail(gomock.Any(), tTenant, address).Return(false, nil)
				st.EXPECT().AddMember(gomock.Any(), tTenant, tIdentity).Return("", errors.New("db down"))
			},
			wantAny: true,
		},
		{
			name: "a disabled tenant admits nobody",
			setup: func(st *MockStorageInterface, k *MockKratosClientInterface, p *permissions.MockPublisher) {
				disabled := orgTenant()
				disabled.Enabled = false
				st.EXPECT().GetTenantByID(gomock.Any(), tTenant).Return(disabled, nil)
				k.EXPECT().GetIdentity(gomock.Any(), tIdentity).Return(identity(tIdentity, email), nil)
				st.EXPECT().GetMemberByTenantAndUserID(gomock.Any(), tTenant, tIdentity).Return(nil, storage.ErrNotFound)
				st.EXPECT().GetTenantSSOPolicy(gomock.Any(), tTenant).Return(hooli(), nil)
			},
			wantErr: ErrNotAdmitted,
		},
		{
			name: "a personal tenant admits nobody",
			setup: func(st *MockStorageInterface, k *MockKratosClientInterface, p *permissions.MockPublisher) {
				owner := tOther
				st.EXPECT().GetTenantByID(gomock.Any(), tTenant).Return(&types.Tenant{ID: tTenant, Enabled: true, PersonalIdentityID: &owner}, nil)
				k.EXPECT().GetIdentity(gomock.Any(), tIdentity).Return(identity(tIdentity, email), nil)
				st.EXPECT().GetMemberByTenantAndUserID(gomock.Any(), tTenant, tIdentity).Return(nil, storage.ErrNotFound)
				st.EXPECT().GetTenantSSOPolicy(gomock.Any(), tTenant).Return(storedPolicy(types.EnforcementOff, false, nil), nil)
			},
			wantErr: ErrNotAdmitted,
		},
		{
			name: "unknown tenant",
			setup: func(st *MockStorageInterface, k *MockKratosClientInterface, p *permissions.MockPublisher) {
				k.EXPECT().GetIdentity(gomock.Any(), tIdentity).Return(identity(tIdentity, email), nil)
				st.EXPECT().GetTenantByID(gomock.Any(), tTenant).Return(nil, storage.ErrNotFound)
			},
			wantErr: ErrTenantNotFound,
		},
		{
			name: "kratos failure: no statement",
			setup: func(st *MockStorageInterface, k *MockKratosClientInterface, p *permissions.MockPublisher) {
				k.EXPECT().GetIdentity(gomock.Any(), tIdentity).Return(nil, errors.New("kratos down"))
			},
			wantAny: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			s, st, k, p, m := newTestService(ctrl)
			m.EXPECT().IncrementCounter(map[string]string{"operation": "tenant_joined"}).Return(nil).AnyTimes()
			tc.setup(st, k, p)

			err := s.JoinTenant(context.Background(), tTenant, tIdentity)
			switch {
			case tc.wantErr != nil:
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("want %v, got %v", tc.wantErr, err)
				}
			case tc.wantAny:
				if err == nil {
					t.Fatal("expected an error")
				}
			default:
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
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
		{KratosIdentityID: identityID1},
		{KratosIdentityID: identityID2},
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
				if len(users) != 2 {
					t.Fatalf("expected 2 users, got %d", len(users))
				}
				if users[0].UserID != identityID1 || users[1].UserID != identityID2 {
					t.Errorf("unexpected user IDs: %s, %s", users[0].UserID, users[1].UserID)
				}
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
			mockPublisher := permissions.NewMockPublisher(ctrl)
			mockKratos := NewMockKratosClientInterface(ctrl)
			mockTracer := NewMockTracingInterface(ctrl)
			mockLogger := NewMockLoggerInterface(ctrl)
			setupLoggerMock(ctrl, mockLogger)
			mockMonitor := NewMockMonitorInterface(ctrl)

			s := NewService(mockStorage, mockPublisher, mockKratos, time.Hour, mockTracer, mockMonitor, mockLogger)

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
			mockPublisher := permissions.NewMockPublisher(ctrl)
			mockKratos := NewMockKratosClientInterface(ctrl)
			mockTracer := NewMockTracingInterface(ctrl)
			mockLogger := NewMockLoggerInterface(ctrl)
			setupLoggerMock(ctrl, mockLogger)
			mockMonitor := NewMockMonitorInterface(ctrl)

			s := NewService(mockStorage, mockPublisher, mockKratos, time.Hour, mockTracer, mockMonitor, mockLogger)

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
			mockPublisher := permissions.NewMockPublisher(ctrl)
			mockKratos := NewMockKratosClientInterface(ctrl)
			mockTracer := NewMockTracingInterface(ctrl)
			mockLogger := NewMockLoggerInterface(ctrl)
			setupLoggerMock(ctrl, mockLogger)
			mockMonitor := NewMockMonitorInterface(ctrl)

			s := NewService(mockStorage, mockPublisher, mockKratos, time.Hour, mockTracer, mockMonitor, mockLogger)

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
		{ID: "m-1", TenantID: tenantID, KratosIdentityID: identityID},
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
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockStorage := NewMockStorageInterface(ctrl)
			mockPublisher := permissions.NewMockPublisher(ctrl)
			mockKratos := NewMockKratosClientInterface(ctrl)
			mockTracer := NewMockTracingInterface(ctrl)
			mockLogger := NewMockLoggerInterface(ctrl)
			setupLoggerMock(ctrl, mockLogger)
			mockMonitor := NewMockMonitorInterface(ctrl)

			s := NewService(mockStorage, mockPublisher, mockKratos, time.Hour, mockTracer, mockMonitor, mockLogger)

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

func TestService_CreatePersonalTenant(t *testing.T) {
	t.Run("tenant and membership, then can_delete for the account; name from Kratos", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		s, st, k, p, m := newTestService(ctrl)
		m.EXPECT().IncrementCounter(gomock.Any()).Return(nil).AnyTimes()

		// Kratos is asked before the first statement.
		gomock.InOrder(
			k.EXPECT().GetIdentity(gomock.Any(), tIdentity).Return(identity(tIdentity, "ivy@hooli.example"), nil),
			st.EXPECT().GetPersonalTenantByUserID(gomock.Any(), tIdentity).Return(nil, storage.ErrNotFound),
		)
		st.EXPECT().ListInvitedTenantsByEmail(gomock.Any(), "ivy@hooli.example", "hooli.example", "").Return(nil, nil)
		st.EXPECT().ListTenantsByUserID(gomock.Any(), tIdentity, enabledOnly{}).Return(nil, nil)
		gomock.InOrder(
			st.EXPECT().CreatePersonalTenant(gomock.Any(), tIdentity, "ivy@hooli.example's Org").Return(personalTenant(), true, nil),
			st.EXPECT().AddMember(gomock.Any(), tTenant, tIdentity).Return("m", nil),
			p.EXPECT().Publish(gomock.Any(), tTenant, grantOp(tTenant, tIdentity, permissions.RelationCanDelete)).Times(1),
		)

		tn, created, err := s.CreatePersonalTenant(context.Background(), tIdentity, "")
		if err != nil || !created || tn.ID != tTenant || !tn.Enabled {
			t.Fatalf("got %+v, %v, %v", tn, created, err)
		}
	})

	t.Run("a repeat returns the existing tenant and publishes nothing", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		s, st, k, _, _ := newTestService(ctrl)

		k.EXPECT().GetIdentity(gomock.Any(), tIdentity).Return(identity(tIdentity, "ivy@hooli.example"), nil)
		st.EXPECT().GetPersonalTenantByUserID(gomock.Any(), tIdentity).Return(personalTenant(), nil)

		tn, created, err := s.CreatePersonalTenant(context.Background(), tIdentity, "")
		if err != nil || created || tn.ID != tTenant {
			t.Fatalf("got %+v, %v, %v", tn, created, err)
		}
	})

	t.Run("a concurrent creator won: its tenant; the winner publishes", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		s, st, k, _, _ := newTestService(ctrl)

		st.EXPECT().GetPersonalTenantByUserID(gomock.Any(), tIdentity).Return(nil, storage.ErrNotFound)
		settled(st, k)
		st.EXPECT().CreatePersonalTenant(gomock.Any(), tIdentity, "ivy@hooli.example's Org").Return(personalTenant(), false, nil)

		tn, created, err := s.CreatePersonalTenant(context.Background(), tIdentity, "")
		if err != nil || created || tn.ID != tTenant {
			t.Fatalf("got %+v, %v, %v", tn, created, err)
		}
	})

	t.Run("a failed write publishes nothing", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		s, st, k, _, _ := newTestService(ctrl)

		st.EXPECT().GetPersonalTenantByUserID(gomock.Any(), tIdentity).Return(nil, storage.ErrNotFound)
		settled(st, k)
		st.EXPECT().CreatePersonalTenant(gomock.Any(), tIdentity, "ivy@hooli.example's Org").Return(nil, false, errors.New("db down"))

		if _, _, err := s.CreatePersonalTenant(context.Background(), tIdentity, ""); err == nil {
			t.Fatal("expected an error")
		}
	})

	t.Run("the address given: Kratos is not asked", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		s, st, _, _, _ := newTestService(ctrl)

		st.EXPECT().GetPersonalTenantByUserID(gomock.Any(), tIdentity).Return(nil, storage.ErrNotFound)
		st.EXPECT().ListInvitedTenantsByEmail(gomock.Any(), "ivy@hooli.example", "hooli.example", "").Return(nil, nil)
		st.EXPECT().ListTenantsByUserID(gomock.Any(), tIdentity, enabledOnly{}).Return(nil, nil)
		st.EXPECT().CreatePersonalTenant(gomock.Any(), tIdentity, "ivy@hooli.example's Org").Return(personalTenant(), false, nil)

		if _, _, err := s.CreatePersonalTenant(context.Background(), tIdentity, "ivy@hooli.example"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("an unknown account: no statement", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		s, _, k, _, _ := newTestService(ctrl)

		k.EXPECT().GetIdentity(gomock.Any(), tIdentity).Return(nil, kratos.ErrNotFound)

		if _, _, err := s.CreatePersonalTenant(context.Background(), tIdentity, ""); !errors.Is(err, ErrIdentityNotFound) {
			t.Fatalf("want ErrIdentityNotFound, got %v", err)
		}
	})

	t.Run("the account belongs to a tenant already: no personal tenant, nothing published", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		s, st, k, _, _ := newTestService(ctrl)

		st.EXPECT().GetPersonalTenantByUserID(gomock.Any(), tIdentity).Return(nil, storage.ErrNotFound)
		k.EXPECT().GetIdentity(gomock.Any(), tIdentity).Return(identity(tIdentity, "iris@initech.example"), nil)
		st.EXPECT().ListInvitedTenantsByEmail(gomock.Any(), "iris@initech.example", "initech.example", "").Return(nil, nil)
		st.EXPECT().ListTenantsByUserID(gomock.Any(), tIdentity, enabledOnly{}).Return([]*types.Tenant{{ID: tTenant}}, nil)

		if _, _, err := s.CreatePersonalTenant(context.Background(), tIdentity, ""); !errors.Is(err, ErrHasTenant) {
			t.Fatalf("want ErrHasTenant, got %v", err)
		}
	})

	t.Run("a pending invitation: no personal tenant, nothing written or published", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		s, st, k, _, _ := newTestService(ctrl)

		st.EXPECT().GetPersonalTenantByUserID(gomock.Any(), tIdentity).Return(nil, storage.ErrNotFound)
		k.EXPECT().GetIdentity(gomock.Any(), tIdentity).Return(identity(tIdentity, "iris@initech.example"), nil)
		st.EXPECT().ListInvitedTenantsByEmail(gomock.Any(), "iris@initech.example", "initech.example", "").Return([]*types.SignInTenant{
			{Tenant: types.Tenant{ID: tTenant}},
		}, nil)

		if _, _, err := s.CreatePersonalTenant(context.Background(), tIdentity, ""); !errors.Is(err, ErrHasTenant) {
			t.Fatalf("want ErrHasTenant, got %v", err)
		}
	})
}

// settled expects CreatePersonalTenant's checks for an account with no pending
// invitation and no tenant.
func settled(st *MockStorageInterface, k *MockKratosClientInterface) {
	k.EXPECT().GetIdentity(gomock.Any(), tIdentity).Return(identity(tIdentity, "ivy@hooli.example"), nil)
	st.EXPECT().ListInvitedTenantsByEmail(gomock.Any(), "ivy@hooli.example", "hooli.example", "").Return(nil, nil)
	st.EXPECT().ListTenantsByUserID(gomock.Any(), tIdentity, enabledOnly{}).Return(nil, nil)
}

func TestService_RemoveTenantUser(t *testing.T) {
	tenantID := "tenant-123"
	userID := "user-456"
	organisation := &types.Tenant{ID: tenantID, Enabled: true}

	testCases := []struct {
		name       string
		setupMocks func(*MockStorageInterface, *permissions.MockPublisher)
		wantErr    error
		wantAnyErr bool
	}{
		{
			name: "membership removed, then every relation revoked",
			setupMocks: func(mockStorage *MockStorageInterface, mockPublisher *permissions.MockPublisher) {
				gomock.InOrder(
					mockStorage.EXPECT().GetTenantByID(gomock.Any(), tenantID).Return(organisation, nil),
					mockStorage.EXPECT().DeleteMember(gomock.Any(), tenantID, userID).Return(nil),
					mockPublisher.EXPECT().Publish(gomock.Any(), tenantID, revokeOpsFor(tenantID, userID)...).Times(1),
				)
			},
		},
		{
			name: "a personal tenant: refused, the membership stays",
			setupMocks: func(mockStorage *MockStorageInterface, _ *permissions.MockPublisher) {
				mockStorage.EXPECT().GetTenantByID(gomock.Any(), tenantID).Return(&types.Tenant{ID: tenantID, Enabled: true, PersonalIdentityID: &userID}, nil)
			},
			wantErr: ErrPersonalTenant,
		},
		{
			name: "unknown tenant",
			setupMocks: func(mockStorage *MockStorageInterface, _ *permissions.MockPublisher) {
				mockStorage.EXPECT().GetTenantByID(gomock.Any(), tenantID).Return(nil, storage.ErrNotFound)
			},
			wantErr: ErrTenantNotFound,
		},
		{
			name: "not a member: ErrMemberNotFound, nothing published",
			setupMocks: func(mockStorage *MockStorageInterface, _ *permissions.MockPublisher) {
				mockStorage.EXPECT().GetTenantByID(gomock.Any(), tenantID).Return(organisation, nil)
				mockStorage.EXPECT().DeleteMember(gomock.Any(), tenantID, userID).Return(storage.ErrNotFound)
			},
			wantErr: ErrMemberNotFound,
		},
		{
			name: "storage fails: nothing published",
			setupMocks: func(mockStorage *MockStorageInterface, _ *permissions.MockPublisher) {
				mockStorage.EXPECT().GetTenantByID(gomock.Any(), tenantID).Return(organisation, nil)
				mockStorage.EXPECT().DeleteMember(gomock.Any(), tenantID, userID).Return(errors.New("db down"))
			},
			wantAnyErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockStorage := NewMockStorageInterface(ctrl)
			mockKratos := NewMockKratosClientInterface(ctrl)
			mockPublisher := permissions.NewMockPublisher(ctrl)
			mockTracer := NewMockTracingInterface(ctrl)
			mockLogger := NewMockLoggerInterface(ctrl)
			setupLoggerMock(ctrl, mockLogger)

			s := NewService(mockStorage, mockPublisher, mockKratos, time.Hour, mockTracer, NewMockMonitorInterface(ctrl), mockLogger)
			mockTracer.EXPECT().Start(gomock.Any(), "admin.RemoveTenantUser").Return(context.Background(), trace.SpanFromContext(context.Background()))
			tc.setupMocks(mockStorage, mockPublisher)

			err := s.RemoveTenantUser(context.Background(), tenantID, userID)
			switch {
			case tc.wantErr != nil:
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("want %v, got %v", tc.wantErr, err)
				}
			case tc.wantAnyErr:
				if err == nil {
					t.Fatal("expected an error")
				}
			case err != nil:
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

const (
	tTenant     = "0190a000-0000-7000-8000-000000000001"
	tIdentity   = "0190a000-0000-7000-8000-0000000000a1"
	tConnection = "0190a000-0000-7000-8000-0000000000c1"
	tOther      = "0190a000-0000-7000-8000-0000000000c2"
)

func orgTenant() *types.Tenant {
	return &types.Tenant{ID: tTenant, Name: "Initech", Enabled: true, MFARequirement: types.MFARequirementNone}
}

func personalTenant() *types.Tenant {
	t := orgTenant()
	id := tIdentity
	t.PersonalIdentityID = &id
	return t
}

// policy returns a stored policy with one binding to tConnection.
func policy(enforcement string, active, autoJoin bool, domains ...string) *types.TenantSSOPolicy {
	if domains == nil {
		domains = []string{}
	}
	return &types.TenantSSOPolicy{
		TenantID:    tTenant,
		Enforcement: enforcement,
		AutoJoin:    autoJoin,
		Domains:     domains,
		Bindings:    []types.SSOBinding{{ConnectionID: tConnection, Active: active}},
	}
}

// storedPolicy returns a stored policy with the given bindings.
func storedPolicy(enforcement string, autoJoin bool, domains []string, bindings ...types.SSOBinding) *types.TenantSSOPolicy {
	if domains == nil {
		domains = []string{}
	}
	return &types.TenantSSOPolicy{TenantID: tTenant, Enforcement: enforcement, AutoJoin: autoJoin, Domains: domains, Bindings: append([]types.SSOBinding{}, bindings...)}
}

func identity(id, email string) *ory.Identity {
	return &ory.Identity{Id: id, Traits: map[string]interface{}{"email": email}}
}

// newTestService builds a Service whose tracer accepts any span.
func newTestService(ctrl *gomock.Controller) (*Service, *MockStorageInterface, *MockKratosClientInterface, *permissions.MockPublisher, *MockMonitorInterface) {
	mockStorage := NewMockStorageInterface(ctrl)
	mockPublisher := permissions.NewMockPublisher(ctrl)
	mockKratos := NewMockKratosClientInterface(ctrl)
	mockTracer := NewMockTracingInterface(ctrl)
	mockLogger := NewMockLoggerInterface(ctrl)
	setupLoggerMock(ctrl, mockLogger)
	mockMonitor := NewMockMonitorInterface(ctrl)

	mockTracer.EXPECT().Start(gomock.Any(), gomock.Any()).DoAndReturn(
		func(c context.Context, _ string, _ ...trace.SpanStartOption) (context.Context, trace.Span) {
			return c, trace.SpanFromContext(c)
		}).AnyTimes()

	return NewService(mockStorage, mockPublisher, mockKratos, time.Hour, mockTracer, mockMonitor, mockLogger),
		mockStorage, mockKratos, mockPublisher, mockMonitor
}

// grantOp is the permission event of a relation granted to identityID on
// tenantID.
func grantOp(tenantID, identityID, relation string) *v1.PermissionOperation {
	return &v1.PermissionOperation{
		Op:       v1.PermissionOp_PERMISSION_OP_WRITE,
		Subject:  "user:" + identityID,
		Relation: relation,
		Object:   "tenant:" + tenantID,
	}
}

// revokeOpsFor is the revocation of every relation identityID may hold on
// tenantID, as Publish's variadic arguments.
func revokeOpsFor(tenantID, identityID string) []any {
	ops := make([]any, 0, 3)
	for _, relation := range []string{permissions.RelationCanView, permissions.RelationCanEdit, permissions.RelationCanDelete} {
		ops = append(ops, &v1.PermissionOperation{
			Op:       v1.PermissionOp_PERMISSION_OP_DELETE,
			Subject:  "user:" + identityID,
			Relation: relation,
			Object:   "tenant:" + tenantID,
		})
	}
	return ops
}

// enabledOnly matches the list option that keeps enabled tenants only: an
// account whose only tenants are disabled has none to sign in to.
type enabledOnly struct{}

func (enabledOnly) Matches(x any) bool {
	opt, ok := x.(types.ListOption)
	if !ok {
		return false
	}
	var o types.ListOptions
	opt(&o)
	return o.Enabled != nil && *o.Enabled
}

func (enabledOnly) String() string { return "enabled tenants only" }

// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package tenant

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/canonical/tenant-service/internal/kratos"
	"github.com/canonical/tenant-service/internal/logging"
	"github.com/canonical/tenant-service/internal/permissions"
	"github.com/canonical/tenant-service/internal/storage"
	"github.com/canonical/tenant-service/internal/types"
	"github.com/canonical/tenant-service/pkg/authentication"
	"go.uber.org/mock/gomock"
)

func TestService_ListSignInTenants(t *testing.T) {
	// Mixed case: invitations and domains are matched lower-cased.
	email := "alice@Example.COM"
	identityID := "identity-abc"
	memberships := []*types.Tenant{
		{ID: "tenant-1", Name: "My Org", Enabled: true},
	}
	candidates := []*types.SignInTenant{
		{Tenant: types.Tenant{ID: "tenant-2", Name: "Hooli", Enabled: true}, AutoJoinCandidate: true},
	}

	testCases := []struct {
		name               string
		setupMocks         func(*MockStorageInterface, *MockKratosClientInterface)
		expectedLen        int
		expectedCandidates int
		expectedErr        bool
	}{
		{
			name: "success - email found with active tenants, no candidate",
			setupMocks: func(mockStorage *MockStorageInterface, mockKratos *MockKratosClientInterface) {
				mockKratos.EXPECT().GetIdentityIDByEmail(gomock.Any(), email).Return(identityID, nil)
				mockStorage.EXPECT().ListTenantsByUserID(gomock.Any(), identityID, enabledOnly{}).Return(memberships, nil)
				mockStorage.EXPECT().ListInvitedTenantsByEmail(gomock.Any(), strings.ToLower(email), "example.com", identityID).Return([]*types.SignInTenant{}, nil)
				mockStorage.EXPECT().ListAutoJoinCandidatesByDomain(gomock.Any(), "example.com", identityID).Return([]*types.SignInTenant{}, nil)
			},
			expectedLen: 1,
		},
		{
			name: "success - memberships followed by auto-join candidates",
			setupMocks: func(mockStorage *MockStorageInterface, mockKratos *MockKratosClientInterface) {
				mockKratos.EXPECT().GetIdentityIDByEmail(gomock.Any(), email).Return(identityID, nil)
				mockStorage.EXPECT().ListTenantsByUserID(gomock.Any(), identityID, enabledOnly{}).Return(memberships, nil)
				mockStorage.EXPECT().ListInvitedTenantsByEmail(gomock.Any(), strings.ToLower(email), "example.com", identityID).Return([]*types.SignInTenant{}, nil)
				mockStorage.EXPECT().ListAutoJoinCandidatesByDomain(gomock.Any(), "example.com", identityID).Return(candidates, nil)
			},
			expectedLen:        2,
			expectedCandidates: 1,
		},
		{
			name: "success - email not known to Kratos, no candidate, returns empty list",
			setupMocks: func(mockStorage *MockStorageInterface, mockKratos *MockKratosClientInterface) {
				mockKratos.EXPECT().GetIdentityIDByEmail(gomock.Any(), email).Return("", nil)
				mockStorage.EXPECT().ListInvitedTenantsByEmail(gomock.Any(), strings.ToLower(email), "example.com", "").Return([]*types.SignInTenant{}, nil)
				mockStorage.EXPECT().ListAutoJoinCandidatesByDomain(gomock.Any(), "example.com", "").Return([]*types.SignInTenant{}, nil)
			},
			expectedLen: 0,
		},
		{
			name: "success - email not known to Kratos still gets auto-join candidates",
			setupMocks: func(mockStorage *MockStorageInterface, mockKratos *MockKratosClientInterface) {
				mockKratos.EXPECT().GetIdentityIDByEmail(gomock.Any(), email).Return("", nil)
				mockStorage.EXPECT().ListInvitedTenantsByEmail(gomock.Any(), strings.ToLower(email), "example.com", "").Return([]*types.SignInTenant{}, nil)
				mockStorage.EXPECT().ListAutoJoinCandidatesByDomain(gomock.Any(), "example.com", "").Return(candidates, nil)
			},
			expectedLen:        1,
			expectedCandidates: 1,
		},
		{
			name: "error - storage error on list auto-join candidates",
			setupMocks: func(mockStorage *MockStorageInterface, mockKratos *MockKratosClientInterface) {
				mockKratos.EXPECT().GetIdentityIDByEmail(gomock.Any(), email).Return("", nil)
				mockStorage.EXPECT().ListInvitedTenantsByEmail(gomock.Any(), strings.ToLower(email), "example.com", "").Return([]*types.SignInTenant{}, nil)
				mockStorage.EXPECT().ListAutoJoinCandidatesByDomain(gomock.Any(), "example.com", "").Return(nil, errors.New("db error"))
			},
			expectedErr: true,
		},
		{
			name: "success - an address with no account gets the tenants it is invited to",
			setupMocks: func(mockStorage *MockStorageInterface, mockKratos *MockKratosClientInterface) {
				mockKratos.EXPECT().GetIdentityIDByEmail(gomock.Any(), email).Return("", nil)
				mockStorage.EXPECT().ListInvitedTenantsByEmail(gomock.Any(), strings.ToLower(email), "example.com", "").Return([]*types.SignInTenant{
					{Tenant: types.Tenant{ID: "tenant-3", Name: "Initech", Enabled: true}, Invited: true},
				}, nil)
				mockStorage.EXPECT().ListAutoJoinCandidatesByDomain(gomock.Any(), "example.com", "").Return([]*types.SignInTenant{}, nil)
			},
			expectedLen: 1,
		},
		{
			name: "success - a tenant both invited and a candidate is listed once",
			setupMocks: func(mockStorage *MockStorageInterface, mockKratos *MockKratosClientInterface) {
				mockKratos.EXPECT().GetIdentityIDByEmail(gomock.Any(), email).Return("", nil)
				mockStorage.EXPECT().ListInvitedTenantsByEmail(gomock.Any(), strings.ToLower(email), "example.com", "").Return([]*types.SignInTenant{
					{Tenant: types.Tenant{ID: "tenant-2", Name: "Hooli", Enabled: true}, Invited: true},
				}, nil)
				mockStorage.EXPECT().ListAutoJoinCandidatesByDomain(gomock.Any(), "example.com", "").Return(candidates, nil)
			},
			expectedLen:        1,
			expectedCandidates: 1,
		},
		{
			name: "error - storage error on list invited tenants",
			setupMocks: func(mockStorage *MockStorageInterface, mockKratos *MockKratosClientInterface) {
				mockKratos.EXPECT().GetIdentityIDByEmail(gomock.Any(), email).Return("", nil)
				mockStorage.EXPECT().ListInvitedTenantsByEmail(gomock.Any(), strings.ToLower(email), "example.com", "").Return(nil, errors.New("db error"))
			},
			expectedErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			s, mockStorage, mockKratos, _, _ := newTestService(ctrl)
			tc.setupMocks(mockStorage, mockKratos)

			tenants, err := s.ListSignInTenants(context.Background(), email)

			if tc.expectedErr {
				if err == nil {
					t.Error("expected error but got none")
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if len(tenants) != tc.expectedLen {
				t.Errorf("expected %d tenants, got %d", tc.expectedLen, len(tenants))
			}
			gotCandidates := 0
			for _, tn := range tenants {
				if tn.AutoJoinCandidate {
					gotCandidates++
				}
				if tn.ID == "tenant-1" && (tn.AutoJoinCandidate || tn.Invited) {
					t.Errorf("a membership has no flag, got %+v", tn)
				}
			}
			if gotCandidates != tc.expectedCandidates {
				t.Errorf("expected %d auto-join candidates, got %d", tc.expectedCandidates, gotCandidates)
			}
		})
	}
}

func TestService_GetSignInContext(t *testing.T) {
	const email = "bob@Initech.example"

	testCases := []struct {
		name    string
		email   string
		id      string
		setup   func(*MockStorageInterface, *MockKratosClientInterface, *permissions.MockPublisher)
		want    types.SignInContext
		wantErr error
	}{
		{
			name:  "member of an OPTIONAL tenant, by address",
			email: email,
			setup: func(st *MockStorageInterface, k *MockKratosClientInterface, _ *permissions.MockPublisher) {
				tn := orgTenant()
				tn.MFARequirement = types.MFARequirementRequired
				st.EXPECT().GetTenantByID(gomock.Any(), tTenant).Return(tn, nil)
				k.EXPECT().GetIdentityIDByEmail(gomock.Any(), email).Return(tIdentity, nil)
				st.EXPECT().GetActiveMemberByTenantAndUserID(gomock.Any(), tTenant, tIdentity).Return(&types.Membership{}, nil)
				st.EXPECT().GetTenantSSOPolicy(gomock.Any(), tTenant).Return(policy(types.EnforcementOptional, true, false), nil)
			},
			want: types.SignInContext{
				Member: true, Enforcement: types.EnforcementOptional, ConnectionIDs: []string{tConnection},
				MFARequirement: types.MFARequirementRequired, AccountExists: true,
			},
		},
		{
			name: "by account: its own address is used",
			id:   tIdentity,
			setup: func(st *MockStorageInterface, k *MockKratosClientInterface, _ *permissions.MockPublisher) {
				st.EXPECT().GetTenantByID(gomock.Any(), tTenant).Return(orgTenant(), nil)
				k.EXPECT().GetIdentity(gomock.Any(), tIdentity).Return(identity(tIdentity, "bob@initech.example"), nil)
				st.EXPECT().GetActiveMemberByTenantAndUserID(gomock.Any(), tTenant, tIdentity).Return(nil, storage.ErrNotFound)
				st.EXPECT().GetTenantSSOPolicy(gomock.Any(), tTenant).Return(policy(types.EnforcementRequired, true, false, "initech.example"), nil)
				st.EXPECT().HasInvitationByTenantAndEmail(gomock.Any(), tTenant, "bob@initech.example").Return(false, nil)
			},
			want: types.SignInContext{
				Enforcement: types.EnforcementRequired, ConnectionIDs: []string{tConnection},
				MFARequirement: types.MFARequirementNone, AccountExists: true,
			},
		},
		{
			name:  "address outside the domains: no connections",
			email: "bob@elsewhere.example",
			setup: func(st *MockStorageInterface, k *MockKratosClientInterface, _ *permissions.MockPublisher) {
				st.EXPECT().GetTenantByID(gomock.Any(), tTenant).Return(orgTenant(), nil)
				k.EXPECT().GetIdentityIDByEmail(gomock.Any(), "bob@elsewhere.example").Return("", nil)
				st.EXPECT().GetTenantSSOPolicy(gomock.Any(), tTenant).Return(policy(types.EnforcementRequired, true, false, "initech.example"), nil)
				st.EXPECT().HasInvitationByTenantAndEmail(gomock.Any(), tTenant, "bob@elsewhere.example").Return(false, nil)
			},
			want: types.SignInContext{
				Enforcement: types.EnforcementRequired, ConnectionIDs: []string{},
				MFARequirement: types.MFARequirementNone,
			},
		},
		{
			name:  "no active binding: OFF whatever is stored",
			email: email,
			setup: func(st *MockStorageInterface, k *MockKratosClientInterface, _ *permissions.MockPublisher) {
				st.EXPECT().GetTenantByID(gomock.Any(), tTenant).Return(orgTenant(), nil)
				k.EXPECT().GetIdentityIDByEmail(gomock.Any(), email).Return("", nil)
				st.EXPECT().GetTenantSSOPolicy(gomock.Any(), tTenant).Return(policy(types.EnforcementRequired, false, false), nil)
				st.EXPECT().HasInvitationByTenantAndEmail(gomock.Any(), tTenant, strings.ToLower(email)).Return(false, nil)
			},
			want: types.SignInContext{
				Enforcement: types.EnforcementOff, ConnectionIDs: []string{},
				MFARequirement: types.MFARequirementNone,
			},
		},
		{
			name:  "auto-join admits an address with no account",
			email: "hank@hooli.example",
			setup: func(st *MockStorageInterface, k *MockKratosClientInterface, _ *permissions.MockPublisher) {
				st.EXPECT().GetTenantByID(gomock.Any(), tTenant).Return(orgTenant(), nil)
				k.EXPECT().GetIdentityIDByEmail(gomock.Any(), "hank@hooli.example").Return("", nil)
				st.EXPECT().GetTenantSSOPolicy(gomock.Any(), tTenant).Return(policy(types.EnforcementRequired, true, true, "hooli.example"), nil)
				st.EXPECT().HasInvitationByTenantAndEmail(gomock.Any(), tTenant, "hank@hooli.example").Return(false, nil)
			},
			want: types.SignInContext{
				Enforcement: types.EnforcementRequired, ConnectionIDs: []string{tConnection},
				AutoJoinAdmits: true, MFARequirement: types.MFARequirementNone,
			},
		},
		{
			name:  "a pending invitation admits an address with no account",
			email: "iris@initech.example",
			setup: func(st *MockStorageInterface, k *MockKratosClientInterface, _ *permissions.MockPublisher) {
				st.EXPECT().GetTenantByID(gomock.Any(), tTenant).Return(orgTenant(), nil)
				k.EXPECT().GetIdentityIDByEmail(gomock.Any(), "iris@initech.example").Return("", nil)
				st.EXPECT().GetTenantSSOPolicy(gomock.Any(), tTenant).Return(policy(types.EnforcementRequired, true, false, "initech.example"), nil)
				st.EXPECT().HasInvitationByTenantAndEmail(gomock.Any(), tTenant, "iris@initech.example").Return(true, nil)
			},
			want: types.SignInContext{
				Enforcement: types.EnforcementRequired, ConnectionIDs: []string{tConnection},
				InvitationAdmits: true, MFARequirement: types.MFARequirementNone,
			},
		},
		{
			name:  "a pending invitation of an address outside the domains of a tenant that requires SSO admits nothing",
			email: "bob@elsewhere.example",
			setup: func(st *MockStorageInterface, k *MockKratosClientInterface, _ *permissions.MockPublisher) {
				st.EXPECT().GetTenantByID(gomock.Any(), tTenant).Return(orgTenant(), nil)
				k.EXPECT().GetIdentityIDByEmail(gomock.Any(), "bob@elsewhere.example").Return("", nil)
				st.EXPECT().GetTenantSSOPolicy(gomock.Any(), tTenant).Return(policy(types.EnforcementRequired, true, false, "initech.example"), nil)
				st.EXPECT().HasInvitationByTenantAndEmail(gomock.Any(), tTenant, "bob@elsewhere.example").Return(true, nil)
			},
			want: types.SignInContext{
				Enforcement: types.EnforcementRequired, ConnectionIDs: []string{},
				MFARequirement: types.MFARequirementNone,
			},
		},
		{
			name:  "auto-join does not admit a member",
			email: "hank@hooli.example",
			setup: func(st *MockStorageInterface, k *MockKratosClientInterface, _ *permissions.MockPublisher) {
				st.EXPECT().GetTenantByID(gomock.Any(), tTenant).Return(orgTenant(), nil)
				k.EXPECT().GetIdentityIDByEmail(gomock.Any(), "hank@hooli.example").Return(tIdentity, nil)
				st.EXPECT().GetActiveMemberByTenantAndUserID(gomock.Any(), tTenant, tIdentity).Return(&types.Membership{}, nil)
				st.EXPECT().GetTenantSSOPolicy(gomock.Any(), tTenant).Return(policy(types.EnforcementRequired, true, true, "hooli.example"), nil)
			},
			want: types.SignInContext{
				Member: true, Enforcement: types.EnforcementRequired,
				ConnectionIDs: []string{tConnection}, MFARequirement: types.MFARequirementNone, AccountExists: true,
			},
		},
		{
			name:  "a personal tenant admits nobody",
			email: email,
			setup: func(st *MockStorageInterface, k *MockKratosClientInterface, _ *permissions.MockPublisher) {
				st.EXPECT().GetTenantByID(gomock.Any(), tTenant).Return(personalTenant(), nil)
				k.EXPECT().GetIdentityIDByEmail(gomock.Any(), email).Return("", nil)
				st.EXPECT().GetTenantSSOPolicy(gomock.Any(), tTenant).Return(storedPolicy(types.EnforcementOff, false, nil), nil)
			},
			want: types.SignInContext{
				Enforcement: types.EnforcementOff, ConnectionIDs: []string{},
				MFARequirement: types.MFARequirementNone,
			},
		},
		{
			name:  "unknown tenant",
			email: email,
			setup: func(st *MockStorageInterface, k *MockKratosClientInterface, _ *permissions.MockPublisher) {
				st.EXPECT().GetTenantByID(gomock.Any(), tTenant).Return(nil, storage.ErrNotFound)
			},
			wantErr: ErrTenantNotFound,
		},
		{
			name: "unknown account",
			id:   tIdentity,
			setup: func(st *MockStorageInterface, k *MockKratosClientInterface, _ *permissions.MockPublisher) {
				st.EXPECT().GetTenantByID(gomock.Any(), tTenant).Return(orgTenant(), nil)
				k.EXPECT().GetIdentity(gomock.Any(), tIdentity).Return(nil, kratos.ErrNotFound)
			},
			wantErr: ErrIdentityNotFound,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			s, st, k, p, _ := newTestService(ctrl)
			tc.setup(st, k, p)

			got, err := s.GetSignInContext(context.Background(), tTenant, tc.email, tc.id)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("want %v, got %v", tc.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			assertSignInContext(t, *got, tc.want)
		})
	}
}

func assertSignInContext(t *testing.T, got, want types.SignInContext) {
	t.Helper()
	if got.ConnectionIDs == nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("want %+v, got %+v", want, got)
	}
}

func TestService_GetTenantSSOPolicy(t *testing.T) {
	t.Run("never written: OFF", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		s, st, _, _, _ := newTestService(ctrl)
		st.EXPECT().GetTenantByID(gomock.Any(), tTenant).Return(orgTenant(), nil)
		st.EXPECT().GetTenantSSOPolicy(gomock.Any(), tTenant).Return(storedPolicy(types.EnforcementOff, false, nil), nil)

		p, err := s.GetTenantSSOPolicy(context.Background(), tTenant)
		if err != nil || p.EffectiveEnforcement() != types.EnforcementOff || p.Bindings == nil {
			t.Fatalf("got %+v, %v", p, err)
		}
	})

	t.Run("stored", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		s, st, _, _, _ := newTestService(ctrl)
		st.EXPECT().GetTenantByID(gomock.Any(), tTenant).Return(orgTenant(), nil)
		st.EXPECT().GetTenantSSOPolicy(gomock.Any(), tTenant).Return(policy(types.EnforcementOptional, true, false), nil)

		p, err := s.GetTenantSSOPolicy(context.Background(), tTenant)
		if err != nil || len(p.Bindings) != 1 || p.Enforcement != types.EnforcementOptional {
			t.Fatalf("got %+v, %v", p, err)
		}
	})

	t.Run("personal tenant: no SSO policy", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		s, st, _, _, _ := newTestService(ctrl)
		st.EXPECT().GetTenantByID(gomock.Any(), tTenant).Return(personalTenant(), nil)

		if _, err := s.GetTenantSSOPolicy(context.Background(), tTenant); !errors.Is(err, ErrPersonalTenant) {
			t.Fatalf("want ErrPersonalTenant, got %v", err)
		}
	})

	t.Run("unknown tenant", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		s, st, _, _, _ := newTestService(ctrl)
		st.EXPECT().GetTenantByID(gomock.Any(), tTenant).Return(nil, storage.ErrNotFound)

		if _, err := s.GetTenantSSOPolicy(context.Background(), tTenant); !errors.Is(err, ErrTenantNotFound) {
			t.Fatalf("want ErrTenantNotFound, got %v", err)
		}
	})
}

var errDatabase = errors.New("db down")

// locked expects a policy write to take the lock of an organisation's policy.
func locked(st *MockStorageInterface, stored *types.TenantSSOPolicy) {
	st.EXPECT().LockTenantSSOPolicy(gomock.Any(), tTenant).Return(orgTenant(), stored, nil)
}

func TestService_PutTenantSSOPolicy(t *testing.T) {
	active := types.SSOBinding{ConnectionID: tConnection, Active: true}
	inactive := types.SSOBinding{ConnectionID: tOther}
	fresh := func() *types.TenantSSOPolicy { return storedPolicy(types.EnforcementOff, false, nil) }

	testCases := []struct {
		name        string
		enforcement string
		autoJoin    bool
		bindings    []types.SSOBinding
		setup       func(*MockStorageInterface)
		wantErr     error
		check       func(*testing.T, *types.TenantSSOPolicy)
	}{
		{
			name:        "enforcement, auto-join and bindings are written, the stored domains kept",
			enforcement: types.EnforcementRequired, autoJoin: true, bindings: []types.SSOBinding{active, inactive},
			setup: func(st *MockStorageInterface) {
				gomock.InOrder(
					st.EXPECT().LockTenantSSOPolicy(gomock.Any(), tTenant).
						Return(orgTenant(), storedPolicy(types.EnforcementOptional, false, []string{"hooli.example"}, inactive), nil),
					st.EXPECT().UpdateTenantSSOPolicy(gomock.Any(), tTenant, types.EnforcementRequired, true, []types.SSOBinding{active, inactive}).Return(nil),
				)
			},
			check: func(t *testing.T, p *types.TenantSSOPolicy) {
				if p.TenantID != tTenant || p.Enforcement != types.EnforcementRequired || !p.AutoJoin ||
					len(p.Domains) != 1 || p.Domains[0] != "hooli.example" || len(p.Bindings) != 2 {
					t.Fatalf("got %+v", p)
				}
			},
		},
		{
			name:        "no bindings keeps the row",
			enforcement: types.EnforcementOptional,
			setup: func(st *MockStorageInterface) {
				locked(st, storedPolicy(types.EnforcementRequired, false, nil, active))
				st.EXPECT().UpdateTenantSSOPolicy(gomock.Any(), tTenant, types.EnforcementOptional, false, gomock.Len(0)).Return(nil)
			},
			check: func(t *testing.T, p *types.TenantSSOPolicy) {
				if len(p.Bindings) != 0 || p.Enforcement != types.EnforcementOptional {
					t.Fatalf("got %+v", p)
				}
			},
		},
		{
			// This write cannot break the rules of the domains.
			name:        "the stored domains are not checked again",
			enforcement: types.EnforcementOptional, bindings: []types.SSOBinding{active},
			setup: func(st *MockStorageInterface) {
				locked(st, storedPolicy(types.EnforcementOff, false, []string{"not a domain", "hooli"}))
				st.EXPECT().UpdateTenantSSOPolicy(gomock.Any(), tTenant, types.EnforcementOptional, false, []types.SSOBinding{active}).Return(nil)
			},
			check: func(t *testing.T, p *types.TenantSSOPolicy) {
				if len(p.Domains) != 2 || len(p.Bindings) != 1 {
					t.Fatalf("got %+v", p)
				}
			},
		},
		{
			name:        "auto-join when no domains are stored",
			enforcement: types.EnforcementRequired, autoJoin: true, bindings: []types.SSOBinding{active},
			setup:   func(st *MockStorageInterface) { locked(st, fresh()) },
			wantErr: ErrAutoJoinNeedsRequiredAndDomains,
		},
		{
			name:        "auto-join without REQUIRED",
			enforcement: types.EnforcementOptional, autoJoin: true, bindings: []types.SSOBinding{active},
			setup: func(st *MockStorageInterface) {
				locked(st, storedPolicy(types.EnforcementOptional, false, []string{"hooli.example"}))
			},
			wantErr: ErrAutoJoinNeedsRequiredAndDomains,
		},
		{
			name:        "REQUIRED without an active binding",
			enforcement: types.EnforcementRequired, bindings: []types.SSOBinding{inactive},
			setup: func(st *MockStorageInterface) {
				locked(st, storedPolicy(types.EnforcementOptional, false, nil, active))
			},
			wantErr: ErrRequiredNeedsActiveBinding,
		},
		{
			name:        "REQUIRED with no bindings at all",
			enforcement: types.EnforcementRequired,
			setup:       func(st *MockStorageInterface) { locked(st, fresh()) },
			wantErr:     ErrRequiredNeedsActiveBinding,
		},
		{
			name:        "a connection bound twice",
			enforcement: types.EnforcementOptional, bindings: []types.SSOBinding{active, active},
			setup:   func(st *MockStorageInterface) { locked(st, fresh()) },
			wantErr: ErrInvalidSSOPolicy,
		},
		{
			name:        "a connection another tenant binds",
			enforcement: types.EnforcementOptional, bindings: []types.SSOBinding{active},
			setup: func(st *MockStorageInterface) {
				locked(st, fresh())
				st.EXPECT().UpdateTenantSSOPolicy(gomock.Any(), tTenant, types.EnforcementOptional, false, gomock.Any()).Return(storage.ErrDuplicateKey)
			},
			wantErr: ErrConnectionBound,
		},
		{
			name:        "the write fails",
			enforcement: types.EnforcementOptional, bindings: []types.SSOBinding{active},
			setup: func(st *MockStorageInterface) {
				locked(st, fresh())
				st.EXPECT().UpdateTenantSSOPolicy(gomock.Any(), tTenant, types.EnforcementOptional, false, gomock.Any()).Return(errDatabase)
			},
			wantErr: errDatabase,
		},
		{
			name:        "personal tenant",
			enforcement: types.EnforcementOptional, bindings: []types.SSOBinding{active},
			setup: func(st *MockStorageInterface) {
				st.EXPECT().LockTenantSSOPolicy(gomock.Any(), tTenant).Return(personalTenant(), fresh(), nil)
			},
			wantErr: ErrPersonalTenant,
		},
		{
			name:        "unknown tenant",
			enforcement: types.EnforcementOptional, bindings: []types.SSOBinding{active},
			setup: func(st *MockStorageInterface) {
				st.EXPECT().LockTenantSSOPolicy(gomock.Any(), tTenant).Return(nil, nil, storage.ErrNotFound)
			},
			wantErr: ErrTenantNotFound,
		},
		{
			name:        "the lock is not granted in time",
			enforcement: types.EnforcementOptional, bindings: []types.SSOBinding{active},
			setup: func(st *MockStorageInterface) {
				st.EXPECT().LockTenantSSOPolicy(gomock.Any(), tTenant).Return(nil, nil, errDatabase)
			},
			wantErr: errDatabase,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			s, st, _, _, _ := newTestService(ctrl)
			tc.setup(st)

			got, err := s.PutTenantSSOPolicy(context.Background(), tTenant, tc.enforcement, tc.autoJoin, tc.bindings)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) || got != nil {
					t.Fatalf("want %v, got %+v, %v", tc.wantErr, got, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			tc.check(t, got)
		})
	}
}

// strictSecurityLogger gives s a security logger that takes only the records
// the test expects.
func strictSecurityLogger(ctrl *gomock.Controller, s *Service) *MockSecurityLoggerInterface {
	security := NewMockSecurityLoggerInterface(ctrl)
	logger := NewMockLoggerInterface(ctrl)
	logger.EXPECT().Debugw(gomock.Any(), gomock.Any()).AnyTimes()
	logger.EXPECT().Security().Return(security).AnyTimes()
	s.logger = logger
	return security
}

func TestService_PutTenantSSOPolicy_AdminAction(t *testing.T) {
	active := types.SSOBinding{ConnectionID: tConnection, Active: true}
	ctx := authentication.WithUserID(context.Background(), "sso-service")

	t.Run("a write records who changed which tenant's policy, from what to what", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		s, st, _, _, _ := newTestService(ctrl)
		locked(st, storedPolicy(types.EnforcementOff, false, nil))
		st.EXPECT().UpdateTenantSSOPolicy(gomock.Any(), tTenant, types.EnforcementOptional, false, []types.SSOBinding{active}).Return(nil)
		strictSecurityLogger(ctrl, s).EXPECT().AdminAction("sso-service", "put_tenant_sso_policy", "tenant.Service.PutTenantSSOPolicy", tTenant,
			logging.WithLabel("before", "enforcement=off auto_join=false bindings=[]"),
			logging.WithLabel("after", "enforcement=optional auto_join=false bindings=[{ConnectionID:"+tConnection+" Active:true}]"),
		)

		if _, err := s.PutTenantSSOPolicy(ctx, tTenant, types.EnforcementOptional, false, []types.SSOBinding{active}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("a refused write records nothing", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		s, st, _, _, _ := newTestService(ctrl)
		locked(st, storedPolicy(types.EnforcementOff, false, nil))
		strictSecurityLogger(ctrl, s)

		if _, err := s.PutTenantSSOPolicy(ctx, tTenant, types.EnforcementRequired, false, nil); !errors.Is(err, ErrRequiredNeedsActiveBinding) {
			t.Fatalf("want ErrRequiredNeedsActiveBinding, got %v", err)
		}
	})
}

func TestService_SetTenantSSODomains(t *testing.T) {
	active := types.SSOBinding{ConnectionID: tConnection, Active: true}
	inactive := types.SSOBinding{ConnectionID: tOther}

	testCases := []struct {
		name    string
		domains []string
		setup   func(*MockStorageInterface)
		wantErr error
		check   func(*testing.T, *types.TenantSSOPolicy)
	}{
		{
			name:    "the domains are normalised and written, the rest kept",
			domains: []string{" Hooli.Example ", "acme.example.", "hooli.example"},
			setup: func(st *MockStorageInterface) {
				gomock.InOrder(
					st.EXPECT().LockTenantSSOPolicy(gomock.Any(), tTenant).
						Return(orgTenant(), storedPolicy(types.EnforcementRequired, false, []string{"old.example"}, active), nil),
					st.EXPECT().UpdateTenantSSODomains(gomock.Any(), tTenant, []string{"acme.example", "hooli.example"}).Return(nil),
				)
			},
			check: func(t *testing.T, p *types.TenantSSOPolicy) {
				if p.TenantID != tTenant || len(p.Domains) != 2 || p.Domains[0] != "acme.example" || p.Domains[1] != "hooli.example" ||
					len(p.Bindings) != 1 || p.Enforcement != types.EnforcementRequired {
					t.Fatalf("got %+v", p)
				}
			},
		},
		{
			name:    "a tenant whose policy was never written stays OFF",
			domains: []string{"hooli.example"},
			setup: func(st *MockStorageInterface) {
				locked(st, storedPolicy(types.EnforcementOff, false, nil))
				st.EXPECT().UpdateTenantSSODomains(gomock.Any(), tTenant, []string{"hooli.example"}).Return(nil)
			},
			check: func(t *testing.T, p *types.TenantSSOPolicy) {
				if p.Enforcement != types.EnforcementOff || len(p.Domains) != 1 {
					t.Fatalf("got %+v", p)
				}
			},
		},
		{
			name: "cleared",
			setup: func(st *MockStorageInterface) {
				locked(st, storedPolicy(types.EnforcementRequired, false, []string{"hooli.example"}, active))
				st.EXPECT().UpdateTenantSSODomains(gomock.Any(), tTenant, gomock.Len(0)).Return(nil)
			},
			check: func(t *testing.T, p *types.TenantSSOPolicy) {
				if p.Domains == nil || len(p.Domains) != 0 {
					t.Fatalf("got %+v", p)
				}
			},
		},
		{
			// This write cannot break the rules of the bindings.
			name:    "the stored bindings are not checked again",
			domains: []string{"hooli.example"},
			setup: func(st *MockStorageInterface) {
				locked(st, storedPolicy(types.EnforcementRequired, false, nil, inactive))
				st.EXPECT().UpdateTenantSSODomains(gomock.Any(), tTenant, []string{"hooli.example"}).Return(nil)
			},
			check: func(t *testing.T, p *types.TenantSSOPolicy) {
				if len(p.Domains) != 1 || len(p.Bindings) != 1 {
					t.Fatalf("got %+v", p)
				}
			},
		},
		{
			name: "the domains of an auto-joining tenant cannot be cleared",
			setup: func(st *MockStorageInterface) {
				locked(st, storedPolicy(types.EnforcementRequired, true, []string{"hooli.example"}, active))
			},
			wantErr: ErrAutoJoinNeedsRequiredAndDomains,
		},
		{
			name:    "a bad domain",
			domains: []string{"https://hooli.example"},
			setup: func(st *MockStorageInterface) {
				locked(st, storedPolicy(types.EnforcementOptional, false, nil))
			},
			wantErr: ErrInvalidSSOPolicy,
		},
		{
			name:    "the write fails",
			domains: []string{"hooli.example"},
			setup: func(st *MockStorageInterface) {
				locked(st, storedPolicy(types.EnforcementOptional, false, nil))
				st.EXPECT().UpdateTenantSSODomains(gomock.Any(), tTenant, gomock.Any()).Return(errDatabase)
			},
			wantErr: errDatabase,
		},
		{
			name:    "personal tenant",
			domains: []string{"hooli.example"},
			setup: func(st *MockStorageInterface) {
				st.EXPECT().LockTenantSSOPolicy(gomock.Any(), tTenant).Return(personalTenant(), storedPolicy(types.EnforcementOff, false, nil), nil)
			},
			wantErr: ErrPersonalTenant,
		},
		{
			name:    "unknown tenant",
			domains: []string{"hooli.example"},
			setup: func(st *MockStorageInterface) {
				st.EXPECT().LockTenantSSOPolicy(gomock.Any(), tTenant).Return(nil, nil, storage.ErrNotFound)
			},
			wantErr: ErrTenantNotFound,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			s, st, _, _, _ := newTestService(ctrl)
			tc.setup(st)

			got, err := s.SetTenantSSODomains(context.Background(), tTenant, tc.domains)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) || got != nil {
					t.Fatalf("want %v, got %+v, %v", tc.wantErr, got, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			tc.check(t, got)
		})
	}
}

func TestService_RemoveTenantSSOBinding(t *testing.T) {
	active := types.SSOBinding{ConnectionID: tConnection, Active: true}
	inactive := types.SSOBinding{ConnectionID: tOther}

	testCases := []struct {
		name       string
		connection string
		setup      func(*MockStorageInterface)
		wantErr    error
	}{
		{
			name:       "an inactive binding of a REQUIRED tenant goes",
			connection: tOther,
			setup: func(st *MockStorageInterface) {
				gomock.InOrder(
					st.EXPECT().LockTenantSSOPolicy(gomock.Any(), tTenant).
						Return(orgTenant(), storedPolicy(types.EnforcementRequired, true, []string{"hooli.example"}, active, inactive), nil),
					st.EXPECT().DeleteTenantSSOBinding(gomock.Any(), tTenant, tOther).Return(nil),
				)
			},
		},
		{
			name:       "one of two active bindings of a REQUIRED tenant goes",
			connection: tOther,
			setup: func(st *MockStorageInterface) {
				locked(st, storedPolicy(types.EnforcementRequired, false, nil, active, types.SSOBinding{ConnectionID: tOther, Active: true}))
				st.EXPECT().DeleteTenantSSOBinding(gomock.Any(), tTenant, tOther).Return(nil)
			},
		},
		{
			name:       "the only active binding of an OPTIONAL tenant goes",
			connection: tConnection,
			setup: func(st *MockStorageInterface) {
				locked(st, storedPolicy(types.EnforcementOptional, false, nil, active))
				st.EXPECT().DeleteTenantSSOBinding(gomock.Any(), tTenant, tConnection).Return(nil)
			},
		},
		{
			name:       "the only active binding of a REQUIRED tenant stays",
			connection: tConnection,
			setup: func(st *MockStorageInterface) {
				locked(st, storedPolicy(types.EnforcementRequired, false, nil, active, inactive))
			},
			wantErr: ErrRequiredNeedsActiveBinding,
		},
		{
			name:       "a connection the policy does not bind: success, nothing is written",
			connection: tOther,
			setup: func(st *MockStorageInterface) {
				locked(st, storedPolicy(types.EnforcementRequired, false, nil, active))
			},
		},
		{
			name:       "a policy that was never written: success, nothing is written",
			connection: tOther,
			setup: func(st *MockStorageInterface) {
				locked(st, storedPolicy(types.EnforcementOff, false, nil))
			},
		},
		{
			name:       "the write fails",
			connection: tConnection,
			setup: func(st *MockStorageInterface) {
				locked(st, storedPolicy(types.EnforcementOptional, false, nil, active))
				st.EXPECT().DeleteTenantSSOBinding(gomock.Any(), tTenant, tConnection).Return(errDatabase)
			},
			wantErr: errDatabase,
		},
		{
			name:       "unknown tenant",
			connection: tOther,
			setup: func(st *MockStorageInterface) {
				st.EXPECT().LockTenantSSOPolicy(gomock.Any(), tTenant).Return(nil, nil, storage.ErrNotFound)
			},
			wantErr: ErrTenantNotFound,
		},
		{
			name:       "personal tenant",
			connection: tOther,
			setup: func(st *MockStorageInterface) {
				st.EXPECT().LockTenantSSOPolicy(gomock.Any(), tTenant).Return(personalTenant(), storedPolicy(types.EnforcementOff, false, nil), nil)
			},
			wantErr: ErrPersonalTenant,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			s, st, _, _, _ := newTestService(ctrl)
			tc.setup(st)

			err := s.RemoveTenantSSOBinding(context.Background(), tTenant, tc.connection)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("want %v, got %v", tc.wantErr, err)
			}
		})
	}
}

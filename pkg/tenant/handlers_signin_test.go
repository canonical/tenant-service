// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package tenant

import (
	"context"
	"errors"
	"strings"
	"testing"

	v0 "github.com/canonical/identity-platform-api/v0/tenant"
	"github.com/canonical/tenant-service/internal/types"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// newTestSignInHandler builds a SignInHandler whose tracer accepts any span.
func newTestSignInHandler(t *testing.T) (*SignInHandler, *MockServiceInterface) {
	svc, tracer, monitor, logger := newHandlerMocks(t)
	return NewSignInHandler(svc, testValidator, tracer, monitor, logger), svc
}

func TestSignInHandler_ListSignInTenants(t *testing.T) {
	t.Run("maps the tenants and their flags", func(t *testing.T) {
		h, svc := newTestSignInHandler(t)
		svc.EXPECT().ListSignInTenants(gomock.Any(), "hank@hooli.example").Return([]*types.SignInTenant{
			{Tenant: types.Tenant{ID: "t1", Name: "Beta"}},
			{Tenant: types.Tenant{ID: "t2", Name: "Hooli"}, AutoJoinCandidate: true},
			{Tenant: types.Tenant{ID: "t3", Name: "Initech"}, Invited: true},
		}, nil)

		res, err := h.ListSignInTenants(context.Background(), &v0.ListSignInTenantsRequest{Email: "hank@hooli.example"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(res.Tenants) != 3 {
			t.Fatalf("unexpected result: %v", res.Tenants)
		}
		if s := res.Tenants[0]; s.Tenant.Id != "t1" || s.AutoJoinCandidate || s.Invited {
			t.Errorf("expected Beta to be a membership, got %v", s)
		}
		if s := res.Tenants[1]; s.Tenant.Id != "t2" || !s.AutoJoinCandidate || s.Invited {
			t.Errorf("expected Hooli to be an auto-join candidate, got %v", s)
		}
		if s := res.Tenants[2]; s.Tenant.Id != "t3" || s.AutoJoinCandidate || !s.Invited {
			t.Errorf("expected Initech to be an invitation, got %v", s)
		}
	})

	t.Run("bad email", func(t *testing.T) {
		h, _ := newTestSignInHandler(t)
		if _, err := h.ListSignInTenants(context.Background(), &v0.ListSignInTenantsRequest{Email: "nope"}); status.Code(err) != codes.InvalidArgument {
			t.Fatalf("want InvalidArgument, got %v", err)
		}
	})
}

func TestSignInHandler_GetSignInContext(t *testing.T) {
	t.Run("maps the context", func(t *testing.T) {
		h, svc := newTestSignInHandler(t)
		svc.EXPECT().GetSignInContext(gomock.Any(), tTenant, "bob@initech.example", "").Return(&types.SignInContext{
			Member: true, Enforcement: types.EnforcementRequired, ConnectionIDs: []string{tConnection},
			InvitationAdmits: true, MFARequirement: types.MFARequirementRequired, AccountExists: true,
		}, nil)

		resp, err := h.GetSignInContext(context.Background(), &v0.GetSignInContextRequest{TenantId: tTenant, Email: "bob@initech.example"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		c := resp.Context
		if c.Enforcement != v0.Enforcement_ENFORCEMENT_REQUIRED || c.MfaRequirement != v0.MFARequirement_MFA_REQUIREMENT_REQUIRED ||
			!c.Member || len(c.ConnectionIds) != 1 || !c.InvitationAdmits || c.AutoJoinAdmits || !c.AccountExists {
			t.Fatalf("got %+v", c)
		}
	})

	t.Run("OFF", func(t *testing.T) {
		h, svc := newTestSignInHandler(t)
		svc.EXPECT().GetSignInContext(gomock.Any(), tTenant, "", tIdentity).Return(&types.SignInContext{
			Enforcement: types.EnforcementOff, MFARequirement: types.MFARequirementNone}, nil)

		resp, err := h.GetSignInContext(context.Background(), &v0.GetSignInContextRequest{TenantId: tTenant, IdentityId: tIdentity})
		if err != nil || resp.Context.Enforcement != v0.Enforcement_ENFORCEMENT_OFF || resp.Context.MfaRequirement != v0.MFARequirement_MFA_REQUIREMENT_NONE {
			t.Fatalf("got %+v, %v", resp, err)
		}
	})

	for name, req := range map[string]*v0.GetSignInContextRequest{
		"neither email nor identity": {TenantId: tTenant},
		"bad identity":               {TenantId: tTenant, IdentityId: "nope"},
		"bad email":                  {TenantId: tTenant, Email: "nope"},
		"bad tenant":                 {TenantId: "nope", Email: "bob@initech.example"},
	} {
		t.Run(name, func(t *testing.T) {
			h, _ := newTestSignInHandler(t)
			if _, err := h.GetSignInContext(context.Background(), req); status.Code(err) != codes.InvalidArgument {
				t.Fatalf("want InvalidArgument, got %v", err)
			}
		})
	}

	t.Run("unknown tenant", func(t *testing.T) {
		h, svc := newTestSignInHandler(t)
		svc.EXPECT().GetSignInContext(gomock.Any(), tTenant, gomock.Any(), gomock.Any()).Return(nil, ErrTenantNotFound)
		_, err := h.GetSignInContext(context.Background(), &v0.GetSignInContextRequest{TenantId: tTenant, Email: "bob@initech.example"})
		if status.Code(err) != codes.NotFound {
			t.Fatalf("want NotFound, got %v", err)
		}
	})
}

func TestSignInHandler_JoinTenant(t *testing.T) {
	req := &v0.JoinTenantRequest{TenantId: tTenant, IdentityId: tIdentity}

	t.Run("joined, or a member already", func(t *testing.T) {
		h, svc := newTestSignInHandler(t)
		svc.EXPECT().JoinTenant(gomock.Any(), tTenant, tIdentity).Return(nil)
		if _, err := h.JoinTenant(context.Background(), req); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("not admitted", func(t *testing.T) {
		h, svc := newTestSignInHandler(t)
		svc.EXPECT().JoinTenant(gomock.Any(), tTenant, tIdentity).Return(ErrNotAdmitted)
		_, err := h.JoinTenant(context.Background(), req)
		st, _ := status.FromError(err)
		if st.Code() != codes.FailedPrecondition || reasonOf(err) != "NOT_ADMITTED" || !strings.HasPrefix(st.Message(), "NOT_ADMITTED: ") {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("unknown tenant", func(t *testing.T) {
		h, svc := newTestSignInHandler(t)
		svc.EXPECT().JoinTenant(gomock.Any(), tTenant, tIdentity).Return(ErrTenantNotFound)
		if _, err := h.JoinTenant(context.Background(), req); status.Code(err) != codes.NotFound {
			t.Fatalf("want NotFound, got %v", err)
		}
	})

	t.Run("service error", func(t *testing.T) {
		h, svc := newTestSignInHandler(t)
		svc.EXPECT().JoinTenant(gomock.Any(), tTenant, tIdentity).Return(errors.New("kratos down"))
		_, err := h.JoinTenant(context.Background(), req)
		if st, _ := status.FromError(err); st.Code() != codes.Internal || st.Message() != "failed to join tenant" {
			t.Fatalf("got %v", err)
		}
	})

	for name, bad := range map[string]*v0.JoinTenantRequest{
		"bad tenant":   {TenantId: "nope", IdentityId: tIdentity},
		"bad identity": {TenantId: tTenant, IdentityId: "nope"},
		"no identity":  {TenantId: tTenant},
	} {
		t.Run(name, func(t *testing.T) {
			h, _ := newTestSignInHandler(t)
			if _, err := h.JoinTenant(context.Background(), bad); status.Code(err) != codes.InvalidArgument {
				t.Fatalf("want InvalidArgument, got %v", err)
			}
		})
	}
}

func TestSignInHandler_CreatePersonalTenant(t *testing.T) {
	t.Run("created, or the existing one", func(t *testing.T) {
		h, svc := newTestSignInHandler(t)
		svc.EXPECT().CreatePersonalTenant(gomock.Any(), tIdentity, "").Return(&types.Tenant{ID: tTenant, Enabled: true}, true, nil)

		resp, err := h.CreatePersonalTenant(context.Background(), &v0.CreatePersonalTenantRequest{IdentityId: tIdentity})
		if err != nil || resp.Tenant.Id != tTenant {
			t.Fatalf("got %+v, %v", resp, err)
		}
	})

	t.Run("unknown account", func(t *testing.T) {
		h, svc := newTestSignInHandler(t)
		svc.EXPECT().CreatePersonalTenant(gomock.Any(), tIdentity, "").Return(nil, false, ErrIdentityNotFound)

		_, err := h.CreatePersonalTenant(context.Background(), &v0.CreatePersonalTenantRequest{IdentityId: tIdentity})
		if status.Code(err) != codes.NotFound {
			t.Fatalf("want NotFound, got %v", err)
		}
	})

	t.Run("bad id", func(t *testing.T) {
		h, _ := newTestSignInHandler(t)
		_, err := h.CreatePersonalTenant(context.Background(), &v0.CreatePersonalTenantRequest{IdentityId: "nope"})
		if status.Code(err) != codes.InvalidArgument {
			t.Fatalf("want InvalidArgument, got %v", err)
		}
	})
}

// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package tenant

import (
	"context"
	"strings"
	"testing"

	v0 "github.com/canonical/identity-platform-api/v0/tenant"
	"github.com/canonical/tenant-service/internal/types"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// newTestSSOPolicyHandler builds an SSOPolicyHandler whose tracer accepts any
// span.
func newTestSSOPolicyHandler(t *testing.T) (*SSOPolicyHandler, *MockServiceInterface) {
	svc, tracer, monitor, logger := newHandlerMocks(t)
	return NewSSOPolicyHandler(svc, testValidator, tracer, monitor, logger), svc
}

func TestSSOPolicyHandler(t *testing.T) {
	t.Run("get: a policy never written answers OFF", func(t *testing.T) {
		h, svc := newTestSSOPolicyHandler(t)
		svc.EXPECT().GetTenantSSOPolicy(gomock.Any(), tTenant).Return(&types.TenantSSOPolicy{TenantID: tTenant}, nil)

		resp, err := h.GetTenantSSOPolicy(context.Background(), &v0.GetTenantSSOPolicyRequest{TenantId: tTenant})
		if err != nil || resp.Policy.TenantId != tTenant || resp.Policy.Enforcement != v0.Enforcement_ENFORCEMENT_OFF || resp.Policy.Domains == nil {
			t.Fatalf("got %+v, %v", resp, err)
		}
	})

	t.Run("put: maps enums and bindings", func(t *testing.T) {
		h, svc := newTestSSOPolicyHandler(t)
		svc.EXPECT().PutTenantSSOPolicy(gomock.Any(), tTenant, types.EnforcementRequired, true, []types.SSOBinding{{ConnectionID: tConnection, Active: true}}).
			Return(&types.TenantSSOPolicy{
				TenantID: tTenant, Enforcement: types.EnforcementRequired, AutoJoin: true, Domains: []string{"hooli.example"},
				Bindings: []types.SSOBinding{{ConnectionID: tConnection, Active: true}},
			}, nil)

		// The service gets the connection ids in lower case.
		resp, err := h.PutTenantSSOPolicy(context.Background(), &v0.PutTenantSSOPolicyRequest{
			TenantId: tTenant, Enforcement: v0.Enforcement_ENFORCEMENT_REQUIRED, AutoJoin: true,
			Bindings: []*v0.SSOBinding{{ConnectionId: strings.ToUpper(tConnection), Active: true}},
		})
		if err != nil || resp.Policy.Enforcement != v0.Enforcement_ENFORCEMENT_REQUIRED || len(resp.Policy.Domains) != 1 ||
			len(resp.Policy.Bindings) != 1 || resp.Policy.Bindings[0].ConnectionId != tConnection || !resp.Policy.Bindings[0].Active {
			t.Fatalf("got %+v, %v", resp, err)
		}
	})

	t.Run("put: a binding's connection id must be a UUID", func(t *testing.T) {
		h, _ := newTestSSOPolicyHandler(t)
		_, err := h.PutTenantSSOPolicy(context.Background(), &v0.PutTenantSSOPolicyRequest{
			TenantId: tTenant, Enforcement: v0.Enforcement_ENFORCEMENT_OPTIONAL,
			Bindings: []*v0.SSOBinding{{ConnectionId: tConnection}, {ConnectionId: "conn-1"}},
		})
		if status.Code(err) != codes.InvalidArgument {
			t.Fatalf("want InvalidArgument, got %v", err)
		}
	})

	t.Run("put: the tenant id must be a UUID", func(t *testing.T) {
		h, _ := newTestSSOPolicyHandler(t)
		_, err := h.PutTenantSSOPolicy(context.Background(), &v0.PutTenantSSOPolicyRequest{
			TenantId: "acme", Enforcement: v0.Enforcement_ENFORCEMENT_OPTIONAL})
		if status.Code(err) != codes.InvalidArgument {
			t.Fatalf("want InvalidArgument, got %v", err)
		}
	})

	t.Run("put: OFF is not a stored value", func(t *testing.T) {
		h, _ := newTestSSOPolicyHandler(t)
		_, err := h.PutTenantSSOPolicy(context.Background(), &v0.PutTenantSSOPolicyRequest{
			TenantId: tTenant, Enforcement: v0.Enforcement_ENFORCEMENT_OFF})
		if status.Code(err) != codes.InvalidArgument {
			t.Fatalf("want InvalidArgument, got %v", err)
		}
	})

	t.Run("put: a rule of the policy keeps its reason", func(t *testing.T) {
		h, svc := newTestSSOPolicyHandler(t)
		svc.EXPECT().PutTenantSSOPolicy(gomock.Any(), tTenant, types.EnforcementRequired, false, gomock.Any()).Return(nil, ErrRequiredNeedsActiveBinding)
		_, err := h.PutTenantSSOPolicy(context.Background(), &v0.PutTenantSSOPolicyRequest{
			TenantId: tTenant, Enforcement: v0.Enforcement_ENFORCEMENT_REQUIRED})
		if status.Code(err) != codes.FailedPrecondition || reasonOf(err) != "REQUIRED_NEEDS_ACTIVE_BINDING" {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("set domains", func(t *testing.T) {
		h, svc := newTestSSOPolicyHandler(t)
		svc.EXPECT().SetTenantSSODomains(gomock.Any(), tTenant, []string{"hooli.example"}).
			Return(&types.TenantSSOPolicy{TenantID: tTenant, Enforcement: types.EnforcementOptional, Domains: []string{"hooli.example"}}, nil)

		resp, err := h.SetTenantSSODomains(context.Background(), &v0.SetTenantSSODomainsRequest{TenantId: tTenant, Domains: []string{"hooli.example"}})
		if err != nil || len(resp.Policy.Domains) != 1 {
			t.Fatalf("got %+v, %v", resp, err)
		}
	})

	t.Run("set domains: the tenant id must be a UUID", func(t *testing.T) {
		h, _ := newTestSSOPolicyHandler(t)
		if _, err := h.SetTenantSSODomains(context.Background(), &v0.SetTenantSSODomainsRequest{TenantId: "acme"}); status.Code(err) != codes.InvalidArgument {
			t.Fatalf("want InvalidArgument, got %v", err)
		}
	})

	t.Run("remove binding", func(t *testing.T) {
		h, svc := newTestSSOPolicyHandler(t)
		svc.EXPECT().RemoveTenantSSOBinding(gomock.Any(), tTenant, tConnection).Return(nil)

		// The service gets the connection id in lower case.
		_, err := h.RemoveTenantSSOBinding(context.Background(), &v0.RemoveTenantSSOBindingRequest{TenantId: tTenant, ConnectionId: strings.ToUpper(tConnection)})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("remove binding: the last active one of a REQUIRED tenant keeps its reason", func(t *testing.T) {
		h, svc := newTestSSOPolicyHandler(t)
		svc.EXPECT().RemoveTenantSSOBinding(gomock.Any(), tTenant, tConnection).Return(ErrRequiredNeedsActiveBinding)
		_, err := h.RemoveTenantSSOBinding(context.Background(), &v0.RemoveTenantSSOBindingRequest{TenantId: tTenant, ConnectionId: tConnection})
		if status.Code(err) != codes.FailedPrecondition || reasonOf(err) != "REQUIRED_NEEDS_ACTIVE_BINDING" {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("remove binding: the connection id must be a UUID", func(t *testing.T) {
		h, _ := newTestSSOPolicyHandler(t)
		_, err := h.RemoveTenantSSOBinding(context.Background(), &v0.RemoveTenantSSOBindingRequest{TenantId: tTenant, ConnectionId: "conn-1"})
		if status.Code(err) != codes.InvalidArgument {
			t.Fatalf("want InvalidArgument, got %v", err)
		}
	})
}

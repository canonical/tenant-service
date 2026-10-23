// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package tenant

import (
	"context"
	"testing"

	v0 "github.com/canonical/identity-platform-api/v0/tenant"
	"github.com/canonical/tenant-service/internal/types"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestHandler_MFAPolicy(t *testing.T) {
	t.Run("get", func(t *testing.T) {
		h, svc := newTestHandler(t)
		svc.EXPECT().GetTenantMFAPolicy(gomock.Any(), tTenant).Return(&types.TenantMFAPolicy{TenantID: tTenant, Requirement: types.MFARequirementRequired}, nil)

		resp, err := h.GetTenantMFAPolicy(context.Background(), &v0.GetTenantMFAPolicyRequest{TenantId: tTenant})
		if err != nil || resp.Policy.Requirement != v0.MFARequirement_MFA_REQUIREMENT_REQUIRED {
			t.Fatalf("got %+v, %v", resp, err)
		}
	})

	t.Run("put", func(t *testing.T) {
		h, svc := newTestHandler(t)
		svc.EXPECT().PutTenantMFAPolicy(gomock.Any(), tTenant, types.MFARequirementRequired).Return(&types.TenantMFAPolicy{TenantID: tTenant, Requirement: types.MFARequirementRequired}, nil)

		resp, err := h.PutTenantMFAPolicy(context.Background(), &v0.PutTenantMFAPolicyRequest{TenantId: tTenant, Requirement: v0.MFARequirement_MFA_REQUIREMENT_REQUIRED})
		if err != nil || resp.Policy.Requirement != v0.MFARequirement_MFA_REQUIREMENT_REQUIRED {
			t.Fatalf("got %+v, %v", resp, err)
		}
	})

	t.Run("put: unspecified refused", func(t *testing.T) {
		h, _ := newTestHandler(t)
		_, err := h.PutTenantMFAPolicy(context.Background(), &v0.PutTenantMFAPolicyRequest{TenantId: tTenant})
		if status.Code(err) != codes.InvalidArgument {
			t.Fatalf("want InvalidArgument, got %v", err)
		}
	})

	t.Run("get: unknown tenant", func(t *testing.T) {
		h, svc := newTestHandler(t)
		svc.EXPECT().GetTenantMFAPolicy(gomock.Any(), tTenant).Return(nil, ErrTenantNotFound)
		if _, err := h.GetTenantMFAPolicy(context.Background(), &v0.GetTenantMFAPolicyRequest{TenantId: tTenant}); status.Code(err) != codes.NotFound {
			t.Fatalf("want NotFound, got %v", err)
		}
	})
}

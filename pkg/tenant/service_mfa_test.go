// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package tenant

import (
	"context"
	"errors"
	"testing"

	"github.com/canonical/tenant-service/internal/storage"
	"github.com/canonical/tenant-service/internal/types"
	"go.uber.org/mock/gomock"
)

func TestService_MFAPolicy(t *testing.T) {
	t.Run("get", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		s, st, _, _, _ := newTestService(ctrl)
		tn := orgTenant()
		tn.MFARequirement = types.MFARequirementRequired
		st.EXPECT().GetTenantByID(gomock.Any(), tTenant).Return(tn, nil)

		p, err := s.GetTenantMFAPolicy(context.Background(), tTenant)
		if err != nil || p.Requirement != types.MFARequirementRequired {
			t.Fatalf("got %+v, %v", p, err)
		}
	})

	t.Run("get: unknown tenant", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		s, st, _, _, _ := newTestService(ctrl)
		st.EXPECT().GetTenantByID(gomock.Any(), tTenant).Return(nil, storage.ErrNotFound)

		if _, err := s.GetTenantMFAPolicy(context.Background(), tTenant); !errors.Is(err, ErrTenantNotFound) {
			t.Fatalf("want ErrTenantNotFound, got %v", err)
		}
	})

	t.Run("put", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		s, st, _, _, _ := newTestService(ctrl)
		st.EXPECT().UpdateTenantMFARequirement(gomock.Any(), tTenant, types.MFARequirementRequired).Return(types.MFARequirementNone, nil)

		p, err := s.PutTenantMFAPolicy(context.Background(), tTenant, types.MFARequirementRequired)
		if err != nil || p.Requirement != types.MFARequirementRequired {
			t.Fatalf("got %+v, %v", p, err)
		}
	})

	t.Run("put: unknown tenant", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		s, st, _, _, _ := newTestService(ctrl)
		st.EXPECT().UpdateTenantMFARequirement(gomock.Any(), tTenant, types.MFARequirementNone).Return("", storage.ErrNotFound)

		if _, err := s.PutTenantMFAPolicy(context.Background(), tTenant, types.MFARequirementNone); !errors.Is(err, ErrTenantNotFound) {
			t.Fatalf("want ErrTenantNotFound, got %v", err)
		}
	})
}

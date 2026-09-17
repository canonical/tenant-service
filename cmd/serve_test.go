// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package cmd

import (
	"context"
	"testing"

	v1 "github.com/canonical/authorization-service/api/v1"
	"github.com/canonical/tenant-service/internal/permissions"
	"go.uber.org/mock/gomock"
)

func TestPublishStaticPermissions(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPublisher := permissions.NewMockPublisher(ctrl)

	mockPublisher.EXPECT().Publish(
		gomock.Any(),
		"",
		&v1.PermissionOperation{
			Op:       v1.PermissionOp_PERMISSION_OP_WRITE,
			Subject:  "user:*",
			Relation: "can_view",
			Object:   "account:me",
		},
	).Times(1)

	publishStaticPermissions(context.Background(), mockPublisher)
}

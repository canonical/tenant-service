// Copyright 2025 Canonical Ltd
// SPDX-License-Identifier: AGPL-3.0

package authorization

import (
	"context"

	fga "github.com/openfga/go-sdk"
	"github.com/openfga/go-sdk/client"

	"github.com/canonical/tenant-service/internal/openfga"
)

type AuthorizerInterface interface {
	ListObjects(context.Context, string, string, string) ([]string, error)
	Check(context.Context, string, string, string, ...openfga.Tuple) (bool, error)
	FilterObjects(context.Context, string, string, string, []string) ([]string, error)
	ValidateModel(context.Context) error

	AssignTenantOwner(context.Context, string, string) error
	AssignTenantMember(context.Context, string, string) error
	RemoveTenantOwner(context.Context, string, string) error
	RemoveTenantMember(context.Context, string, string) error
	// AssignPrivilegedAdmin assigns a user as a privileged admin in the authorization system.
	// This user will have admin access to all tenants linked to that privileged group.
	AssignPrivilegedAdmin(context.Context, string, string) error
	// LinkTenantToPrivileged acts as a binder between a tenant and a privileged group.
	// This way, privileged admins can access the tenant.
	LinkTenantToPrivileged(context.Context, string, string) error

	DeleteTenant(context.Context, string) error
	CheckTenantAccess(context.Context, string, string, string) (bool, error)
}

type AuthzClientInterface interface {
	ListObjects(context.Context, string, string, string) ([]string, error)
	Check(context.Context, string, string, string, ...openfga.Tuple) (bool, error)
	BatchCheck(context.Context, ...openfga.TupleWithContext) (bool, error)
	ReadModel(context.Context) (*fga.AuthorizationModel, error)
	CompareModel(context.Context, fga.AuthorizationModel) (bool, error)
	ReadTuples(context.Context, string, string, string, string) (*client.ClientReadResponse, error)
	WriteTuple(ctx context.Context, user, relation, object string) error
	DeleteTuple(ctx context.Context, user, relation, object string) error
	DeleteTuples(context.Context, ...openfga.Tuple) error
}

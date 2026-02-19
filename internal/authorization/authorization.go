// Copyright 2025 Canonical Ltd
// SPDX-License-Identifier: AGPL-3.0

package authorization

import (
	"context"
	"fmt"
	"slices"

	"github.com/canonical/tenant-service/internal/logging"
	"github.com/canonical/tenant-service/internal/monitoring"
	"github.com/canonical/tenant-service/internal/openfga"
	"github.com/canonical/tenant-service/internal/tracing"
)

var ErrInvalidAuthModel = fmt.Errorf("invalid authorization model schema")

type Authorizer struct {
	client AuthzClientInterface

	tracer  tracing.TracingInterface
	monitor monitoring.MonitorInterface
	logger  logging.LoggerInterface
}

func (a *Authorizer) Check(ctx context.Context, user string, relation string, object string, contextualTuples ...openfga.Tuple) (bool, error) {
	ctx, span := a.tracer.Start(ctx, "authorization.Authorizer.Check")
	defer span.End()

	return a.client.Check(ctx, user, relation, object, contextualTuples...)
}

func (a *Authorizer) ListObjects(ctx context.Context, user string, relation string, objectType string) ([]string, error) {
	ctx, span := a.tracer.Start(ctx, "authorization.Authorizer.ListObjects")
	defer span.End()

	return a.client.ListObjects(ctx, user, relation, objectType)
}

func (a *Authorizer) FilterObjects(ctx context.Context, user string, relation string, objectType string, objs []string) ([]string, error) {
	ctx, span := a.tracer.Start(ctx, "authorization.Authorizer.FilterObjects")
	defer span.End()

	allowedObjs, err := a.ListObjects(ctx, user, relation, objectType)
	if err != nil {
		return nil, err
	}

	var ret []string
	for _, obj := range allowedObjs {
		if slices.Contains(objs, obj) {
			ret = append(ret, obj)
		}
	}
	return ret, nil
}

func (a *Authorizer) ValidateModel(ctx context.Context) error {
	ctx, span := a.tracer.Start(ctx, "authorization.Authorizer.ValidateModel")
	defer span.End()

	v0AuthzModel := NewAuthorizationModelProvider("v0")
	model := *v0AuthzModel.GetModel()

	eq, err := a.client.CompareModel(ctx, model)
	if err != nil {
		return err
	}
	if !eq {
		return ErrInvalidAuthModel
	}
	return nil
}

func (a *Authorizer) AssignTenantOwner(ctx context.Context, tenantId, userId string) error {
	ctx, span := a.tracer.Start(ctx, "authorization.Authorizer.AssignTenantOwner")
	defer span.End()

	return a.client.WriteTuple(ctx, UserTuple(userId), OWNER_RELATION, TenantTuple(tenantId))
}

func (a *Authorizer) AssignPrivilegedAdmin(ctx context.Context, privilegedId, userId string) error {
	ctx, span := a.tracer.Start(ctx, "authorization.Authorizer.AssignPrivilegedAdmin")
	defer span.End()

	return a.client.WriteTuple(ctx, UserTuple(userId), ADMIN_RELATION, PrivilegedTuple(privilegedId))
}

func (a *Authorizer) LinkTenantToPrivileged(ctx context.Context, tenantId, privilegedId string) error {
	ctx, span := a.tracer.Start(ctx, "authorization.Authorizer.LinkTenantToPrivileged")
	defer span.End()

	return a.client.WriteTuple(ctx, PrivilegedTuple(privilegedId), PRIVILEGED_RELATION, TenantTuple(tenantId))
}

func (a *Authorizer) AssignTenantMember(ctx context.Context, tenantId, userId string) error {
	ctx, span := a.tracer.Start(ctx, "authorization.Authorizer.AssignTenantMember")
	defer span.End()

	return a.client.WriteTuple(ctx, UserTuple(userId), MEMBER_RELATION, TenantTuple(tenantId))
}

func (a *Authorizer) RemoveTenantOwner(ctx context.Context, tenantId, userId string) error {
	ctx, span := a.tracer.Start(ctx, "authorization.Authorizer.RemoveTenantOwner")
	defer span.End()

	return a.client.DeleteTuple(ctx, UserTuple(userId), OWNER_RELATION, TenantTuple(tenantId))
}

func (a *Authorizer) RemoveTenantMember(ctx context.Context, tenantId, userId string) error {
	ctx, span := a.tracer.Start(ctx, "authorization.Authorizer.RemoveTenantMember")
	defer span.End()

	return a.client.DeleteTuple(ctx, UserTuple(userId), MEMBER_RELATION, TenantTuple(tenantId))
}

func (a *Authorizer) CheckTenantAccess(ctx context.Context, tenantId, userId, relation string) (bool, error) {
	ctx, span := a.tracer.Start(ctx, "authorization.Authorizer.CheckTenantAccess")
	defer span.End()

	return a.Check(ctx, UserTuple(userId), relation, TenantTuple(tenantId))
}

func (a *Authorizer) DeleteTenant(ctx context.Context, tenantId string) error {
	ctx, span := a.tracer.Start(ctx, "authorization.Authorizer.DeleteTenant")
	defer span.End()

	cToken := ""
	for {
		r, err := a.client.ReadTuples(ctx, "", "", TenantTuple(tenantId), cToken)
		if err != nil {
			a.logger.Errorf("error when retrieving tuples: %s", err)
			return err
		}
		if len(r.Tuples) == 0 {
			break
		}
		ts := make([]openfga.Tuple, len(r.Tuples))
		for i, t := range r.Tuples {
			ts[i] = *openfga.NewTuple(t.Key.User, t.Key.Relation, t.Key.Object)
		}
		if err := a.client.DeleteTuples(ctx, ts...); err != nil {
			a.logger.Errorf("error when deleting tuples %v: %s", ts, err)
			return err
		}
		if r.ContinuationToken == "" {
			break
		}
		cToken = r.ContinuationToken
	}
	return nil
}

func NewAuthorizer(client AuthzClientInterface, tracer tracing.TracingInterface, monitor monitoring.MonitorInterface, logger logging.LoggerInterface) *Authorizer {
	authorizer := new(Authorizer)
	authorizer.client = client
	authorizer.tracer = tracer
	authorizer.monitor = monitor
	authorizer.logger = logger

	return authorizer
}

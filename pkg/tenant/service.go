// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package tenant

import (
	"context"
	"errors"
	"fmt"

	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	v1 "github.com/canonical/authorization-service/api/v1"
	"github.com/canonical/tenant-service/internal/logging"
	"github.com/canonical/tenant-service/internal/monitoring"
	"github.com/canonical/tenant-service/internal/permissions"
	"github.com/canonical/tenant-service/internal/storage"
	"github.com/canonical/tenant-service/internal/tracing"
	"github.com/canonical/tenant-service/internal/types"
	"github.com/canonical/tenant-service/pkg/authentication"
	ory "github.com/ory/client-go"
)

// Service provides tenant business logic.
type Service struct {
	storage            StorageInterface
	publisher          permissions.Publisher
	kratos             KratosClientInterface
	invitationLifetime string
	tracer             tracing.TracingInterface
	monitor            monitoring.MonitorInterface
	logger             logging.LoggerInterface
}

// NewService creates a new tenant service.
func NewService(
	storage StorageInterface,
	publisher permissions.Publisher,
	kratos KratosClientInterface,
	invitationLifetime string,
	tracer tracing.TracingInterface,
	monitor monitoring.MonitorInterface,
	logger logging.LoggerInterface,
) *Service {
	return &Service{
		storage:            storage,
		publisher:          publisher,
		kratos:             kratos,
		invitationLifetime: invitationLifetime,
		tracer:             tracer,
		monitor:            monitor,
		logger:             logger,
	}
}

// recordError records an error on the span and emits a structured error log.
// The "error" key is always appended to keysAndValues automatically.
func (s *Service) recordError(span trace.Span, msg string, err error, keysAndValues ...interface{}) {
	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())
	s.logger.Errorw(msg, append(keysAndValues, "error", err)...)
}

func (s *Service) ListTenantsByUserID(ctx context.Context, userID string, opts ...types.ListOption) ([]*types.Tenant, error) {
	ctx, span := s.tracer.Start(ctx, "tenant.Service.ListTenantsByUserID")
	defer span.End()

	s.logger.Debugw("listing tenants for user", "user_id", userID)

	tenants, err := s.storage.ListTenantsByUserID(ctx, userID, opts...)
	if err != nil {
		s.recordError(span, "failed to list tenants for user", err, "user_id", userID)
		return nil, err
	}
	return tenants, nil
}

func (s *Service) ListTenants(ctx context.Context, opts ...types.ListOption) ([]*types.Tenant, string, error) {
	ctx, span := s.tracer.Start(ctx, "tenant.Service.ListTenants")
	defer span.End()

	s.logger.Debugw("listing all tenants")

	tenants, nextPageToken, err := s.storage.ListTenants(ctx, opts...)
	if err != nil {
		s.recordError(span, "failed to list tenants", err)
		return nil, "", err
	}

	return tenants, nextPageToken, nil
}

func (s *Service) InviteMember(ctx context.Context, tenantID, email string) (string, string, error) {
	ctx, span := s.tracer.Start(ctx, "tenant.Service.InviteMember")
	defer span.End()

	actor, _ := authentication.GetUserID(ctx)
	s.logger.Debugw("inviting member to tenant",
		"tenant_id", tenantID,
		"email", email,
		"actor", actor,
	)

	// 1. Ensure Identity Exists in Kratos
	identityID, err := s.kratos.GetIdentityIDByEmail(ctx, email)
	if err != nil {
		s.recordError(span, "failed to check identity existence", err,
			"tenant_id", tenantID,
			"email", email,
		)
		return "", "", fmt.Errorf("failed to check identity")
	}

	if identityID == "" {
		s.logger.Infow("creating new identity for invited email",
			"tenant_id", tenantID,
			"email", email,
		)
		identityID, err = s.kratos.CreateIdentity(ctx, email)
		if err != nil {
			s.recordError(span, "failed to create identity for invited email", err,
				"tenant_id", tenantID,
				"email", email,
			)
			return "", "", fmt.Errorf("failed to provision user")
		}
	}

	// 2. Add Member to Database (idempotent for duplicate key)
	if _, err := s.storage.AddMember(ctx, tenantID, identityID); err != nil {
		if !errors.Is(err, storage.ErrDuplicateKey) {
			s.recordError(span, "failed to add member to storage", err,
				"tenant_id", tenantID,
				"user_id", identityID,
			)
			return "", "", fmt.Errorf("failed to add member")
		}
		// If duplicate (already a member), we proceed to send recovery link as a re-invite.
	}

	// 3. Grant view access asynchronously via Kafka. Elevated permissions are
	// managed through the authorization service API. Re-inviting an existing
	// member is safe: duplicate writes are ignored by the authorization service.
	s.publisher.Publish(ctx, tenantID, permissions.TenantPermissionOp(
		v1.PermissionOp_PERMISSION_OP_WRITE,
		identityID,
		permissions.RelationCanView,
		tenantID,
	))

	// 4. Generate Kratos Recovery Link
	// We use the configured lifetime for the link
	link, code, err := s.kratos.CreateRecoveryLink(ctx, identityID, s.invitationLifetime)
	if err != nil {
		s.recordError(span, "failed to create recovery link", err,
			"tenant_id", tenantID,
			"user_id", identityID,
		)
		return "", "", fmt.Errorf("failed to generate invitation link")
	}

	s.logger.Infow("member invited successfully",
		"tenant_id", tenantID,
		"user_id", identityID,
		"email", email,
	)
	s.logger.Security().AdminAction(actor, "invite_member", "tenant.Service.InviteMember", tenantID+":"+email)
	s.incrementCounter("invitation_sent")
	return link, code, nil
}

func (s *Service) CreateTenant(ctx context.Context, name string) (*types.Tenant, error) {
	ctx, span := s.tracer.Start(ctx, "admin.CreateTenant")
	defer span.End()

	actor, _ := authentication.GetUserID(ctx)
	s.logger.Debugw("creating tenant", "name", name, "actor", actor)

	t := &types.Tenant{
		Name:    name,
		Enabled: true, // Admin created tenants are enabled by default
	}

	created, err := s.storage.CreateTenant(ctx, t)
	if err != nil {
		s.recordError(span, "failed to create tenant", err, "name", name)
		return nil, fmt.Errorf("failed to create tenant: %w", err)
	}

	s.logger.Infow("tenant created", "tenant_id", created.ID, "name", created.Name)
	s.logger.Security().AdminAction(actor, "create_tenant", "tenant.Service.CreateTenant", created.ID)
	return created, nil
}

func (s *Service) UpdateTenant(ctx context.Context, tenant *types.Tenant, paths []string) (*types.Tenant, error) {
	ctx, span := s.tracer.Start(ctx, "admin.UpdateTenant")
	defer span.End()

	actor, _ := authentication.GetUserID(ctx)
	s.logger.Debugw("updating tenant", "tenant_id", tenant.ID, "paths", paths, "actor", actor)

	if err := s.storage.UpdateTenant(ctx, tenant, paths); err != nil {
		s.recordError(span, "failed to update tenant", err, "tenant_id", tenant.ID)
		return nil, fmt.Errorf("failed to update tenant: %w", err)
	}

	updated, err := s.storage.GetTenantByID(ctx, tenant.ID)
	if err != nil {
		s.recordError(span, "failed to get updated tenant", err, "tenant_id", tenant.ID)
		return nil, fmt.Errorf("failed to get updated tenant: %w", err)
	}

	s.logger.Infow("tenant updated", "tenant_id", updated.ID, "name", updated.Name, "enabled", updated.Enabled)
	s.logger.Security().AdminAction(actor, "update_tenant", "tenant.Service.UpdateTenant", updated.ID)
	return updated, nil
}

func (s *Service) DeleteTenant(ctx context.Context, id string) error {
	ctx, span := s.tracer.Start(ctx, "admin.DeleteTenant")
	defer span.End()

	actor, _ := authentication.GetUserID(ctx)
	s.logger.Debugw("deleting tenant", "tenant_id", id, "actor", actor)

	// Collect every membership prior to deletion to revoke permissions in authorization-service.
	// Memberships cascade-delete with the tenant, so this must happen first.
	var ops []*v1.PermissionOperation
	pageToken := ""
	for {
		members, next, err := s.storage.ListMembersByTenantID(ctx, id, types.WithPageToken(pageToken))
		if err != nil {
			s.recordError(span, "failed to list tenant members prior to deletion", err, "tenant_id", id)
			return fmt.Errorf("failed to list tenant members prior to deletion: %w", err)
		}
		for _, m := range members {
			ops = append(ops, permissions.RevokeTenantPermissionOps(m.KratosIdentityID, id)...)
		}
		if next == "" {
			break
		}
		pageToken = next
	}

	if err := s.storage.DeleteTenant(ctx, id); err != nil {
		s.recordError(span, "failed to delete tenant from storage", err, "tenant_id", id)
		return fmt.Errorf("failed to delete tenant from storage: %w", err)
	}

	s.publisher.Publish(ctx, id, ops...)

	s.logger.Infow("tenant deleted", "tenant_id", id)
	s.logger.Security().AdminAction(actor, "delete_tenant", "tenant.Service.DeleteTenant", id)
	return nil
}

func (s *Service) ProvisionUser(ctx context.Context, tenantID, email string) error {
	ctx, span := s.tracer.Start(ctx, "admin.ProvisionUser")
	defer span.End()

	actor, _ := authentication.GetUserID(ctx)
	s.logger.Debugw("provisioning user",
		"tenant_id", tenantID,
		"email", email,
		"actor", actor,
	)

	// 1. Find or Create Identity
	identityID, err := s.kratos.GetIdentityIDByEmail(ctx, email)
	if err != nil {
		s.recordError(span, "failed to look up identity", err,
			"tenant_id", tenantID,
			"email", email,
		)
		return err
	}
	if identityID == "" {
		s.logger.Infow("creating new identity for provisioned user",
			"tenant_id", tenantID,
			"email", email,
		)
		identityID, err = s.kratos.CreateIdentity(ctx, email)
		if err != nil {
			s.recordError(span, "failed to create identity for provisioned user", err,
				"tenant_id", tenantID,
				"email", email,
			)
			return fmt.Errorf("failed to create identity: %w", err)
		}
	}

	// 2. Add to Storage
	if _, err := s.storage.AddMember(ctx, tenantID, identityID); err != nil {
		s.recordError(span, "failed to add provisioned member to storage", err,
			"tenant_id", tenantID,
			"user_id", identityID,
		)
		return fmt.Errorf("failed to add member to storage: %w", err)
	}

	// 3. Grant view access asynchronously via Kafka. Elevated permissions are
	// managed through the authorization service API.
	s.publisher.Publish(ctx, tenantID, permissions.TenantPermissionOp(
		v1.PermissionOp_PERMISSION_OP_WRITE,
		identityID,
		permissions.RelationCanView,
		tenantID,
	))

	s.logger.Infow("user provisioned",
		"tenant_id", tenantID,
		"user_id", identityID,
		"email", email,
	)
	s.logger.Security().AdminAction(actor, "provision_user", "tenant.Service.ProvisionUser", tenantID+":"+email)
	s.incrementCounter("user_provisioned")
	return nil
}

func (s *Service) ListTenantUsers(ctx context.Context, tenantID string, includeEmails bool, opts ...types.ListOption) ([]*types.TenantUser, string, error) {
	ctx, span := s.tracer.Start(ctx, "admin.ListTenantUsers")
	defer span.End()

	s.logger.Debugw("listing members for tenant", "tenant_id", tenantID)

	// Resolve email filter to identity_id, if provided.
	listOpts := types.ApplyOptions(opts...)
	if listOpts.Email != "" {
		identityID, err := s.kratos.GetIdentityIDByEmail(ctx, listOpts.Email)
		if err != nil {
			s.recordError(span, "failed to resolve email to identity", err, "tenant_id", tenantID)
			return nil, "", fmt.Errorf("failed to resolve email: %w", err)
		}
		if identityID == "" {
			// Unknown email — return empty result set.
			return []*types.TenantUser{}, "", nil
		}
		listOpts.Email = ""
		listOpts.IdentityID = identityID
	}

	members, nextPageToken, err := s.storage.ListMembersByTenantID(ctx, tenantID, types.WithListOptions(listOpts))
	if err != nil {
		s.recordError(span, "failed to list members", err, "tenant_id", tenantID)
		return nil, "", fmt.Errorf("failed to list members: %w", err)
	}

	var identityMap map[string]*ory.Identity
	if includeEmails {
		ids := make([]string, len(members))
		for i, m := range members {
			ids[i] = m.KratosIdentityID
		}
		var err error
		identityMap, err = s.kratos.GetIdentities(ctx, ids)
		if err != nil {
			s.recordError(span, "failed to fetch identities", err, "tenant_id", tenantID)
			return nil, "", fmt.Errorf("failed to fetch identities: %w", err)
		}
	}

	users := make([]*types.TenantUser, 0, len(members))
	for _, m := range members {
		email := ""
		if identity, ok := identityMap[m.KratosIdentityID]; ok {
			if traits, ok := identity.Traits.(map[string]interface{}); ok {
				if e, ok := traits["email"].(string); ok {
					email = e
				}
			}
		}
		users = append(users, &types.TenantUser{
			UserID: m.KratosIdentityID,
			Email:  email,
		})
	}

	return users, nextPageToken, nil
}

func (s *Service) incrementCounter(operation string) {
	if err := s.monitor.IncrementCounter(map[string]string{"operation": operation}); err != nil {
		s.logger.Warnf("failed to increment counter %s: %v", operation, err)
	}
}

// LookupTenantsByEmail returns the enabled tenants that the given email belongs to.
// If the email is not known to Kratos, an empty slice is returned (no error).
// This method is called by the unauthenticated tenant lookup endpoint used by the Login UI.
func (s *Service) LookupTenantsByEmail(ctx context.Context, email string) ([]*types.Tenant, error) {
	ctx, span := s.tracer.Start(ctx, "tenant.Service.LookupTenantsByEmail")
	defer span.End()

	s.logger.Debugw("looking up tenants by email")

	identityID, err := s.kratos.GetIdentityIDByEmail(ctx, email)
	if err != nil {
		s.recordError(span, "failed to look up identity by email", err, "email", email)
		return nil, fmt.Errorf("failed to look up identity: %w", err)
	}

	if identityID == "" {
		// Unknown email — return empty list; do not reveal whether the email is registered.
		s.logger.Debugw("lookup: email not found in Kratos")
		return []*types.Tenant{}, nil
	}

	tenants, err := s.storage.ListTenantsByUserID(ctx, identityID, types.WithEnabled(true))
	if err != nil {
		s.recordError(span, "failed to list active tenants for identity", err, "email", email, "identity_id", identityID)
		return nil, fmt.Errorf("failed to list tenants: %w", err)
	}

	s.logger.Debugw("lookup: tenants found", "count", len(tenants))
	return tenants, nil
}

// LookupTenantsByIdentityID returns the enabled tenants that the given Kratos identity belongs to.
// This skips the Kratos email-to-identity resolution and queries the database directly.
// It is used by privileged internal callers (hook-service, Login UI) that already know the identity ID.
func (s *Service) LookupTenantsByIdentityID(ctx context.Context, identityID string) ([]*types.Tenant, error) {
	ctx, span := s.tracer.Start(ctx, "tenant.Service.LookupTenantsByIdentityID")
	defer span.End()

	s.logger.Debugw("looking up tenants by identity ID")

	tenants, err := s.storage.ListTenantsByUserID(ctx, identityID, types.WithEnabled(true))
	if err != nil {
		s.recordError(span, "failed to list active tenants for identity", err, "identity_id", identityID)
		return nil, fmt.Errorf("failed to list tenants: %w", err)
	}

	s.logger.Debugw("lookup: tenants found", "count", len(tenants))
	return tenants, nil
}

// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0

package tenant

import (
	"context"
	"errors"
	"fmt"

	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/canonical/tenant-service/internal/logging"
	"github.com/canonical/tenant-service/internal/monitoring"
	"github.com/canonical/tenant-service/internal/storage"
	"github.com/canonical/tenant-service/internal/tracing"
	"github.com/canonical/tenant-service/internal/types"
	"github.com/canonical/tenant-service/pkg/authentication"
)

type Service struct {
	storage            StorageInterface
	authz              AuthzInterface
	kratos             KratosClientInterface
	invitationLifetime string
	tracer             tracing.TracingInterface
	monitor            monitoring.MonitorInterface
	logger             logging.LoggerInterface
}

func NewService(
	storage StorageInterface,
	authz AuthzInterface,
	kratos KratosClientInterface,
	invitationLifetime string,
	tracer tracing.TracingInterface,
	monitor monitoring.MonitorInterface,
	logger logging.LoggerInterface,
) *Service {
	return &Service{
		storage:            storage,
		authz:              authz,
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

func (s *Service) ListTenantsByUserID(ctx context.Context, userID string, opts types.ListOptions) ([]*types.Tenant, string, error) {
	ctx, span := s.tracer.Start(ctx, "tenant.Service.ListTenantsByUserID")
	defer span.End()

	s.logger.Debugw("listing tenants for user", "user_id", userID)

	tenants, nextPageToken, err := s.storage.ListTenantsByUserID(ctx, userID, opts)
	if err != nil {
		s.recordError(span, "failed to list tenants for user", err, "user_id", userID)
	}
	return tenants, nextPageToken, err
}

func (s *Service) ListTenants(ctx context.Context, opts types.ListOptions) ([]*types.Tenant, string, error) {
	ctx, span := s.tracer.Start(ctx, "tenant.Service.ListTenants")
	defer span.End()

	s.logger.Debugw("listing all tenants")

	tenants, nextPageToken, err := s.storage.ListTenants(ctx, opts)
	if err != nil {
		s.recordError(span, "failed to list tenants", err)
		return nil, "", err
	}

	return tenants, nextPageToken, nil
}

func (s *Service) InviteMember(ctx context.Context, tenantID, email, role string) (string, string, error) {
	ctx, span := s.tracer.Start(ctx, "tenant.Service.InviteMember")
	defer span.End()

	actor, _ := authentication.GetUserID(ctx)
	s.logger.Debugw("inviting member to tenant",
		"tenant_id", tenantID,
		"email", email,
		"role", role,
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
	if _, err := s.storage.AddMember(ctx, tenantID, identityID, role); err != nil {
		if !errors.Is(err, storage.ErrDuplicateKey) {
			s.recordError(span, "failed to add member to storage", err,
				"tenant_id", tenantID,
				"user_id", identityID,
				"role", role,
			)
			return "", "", fmt.Errorf("failed to add member")
		}
		// If duplicate (already a member), we proceed to send recovery link as a re-invite.
	}

	// 3. Assign Role in OpenFGA (Authorization)
	// Map 'role' string to specific authz method
	if role == "owner" {
		err = s.authz.AssignTenantOwner(ctx, tenantID, identityID)
	} else {
		// Default to member for 'member' and 'admin' roles, as OpenFGA model might not distinguish them yet
		err = s.authz.AssignTenantMember(ctx, tenantID, identityID)
	}

	if err != nil {
		s.recordError(span, "failed to assign role in authz", err,
			"tenant_id", tenantID,
			"user_id", identityID,
			"role", role,
		)
		return "", "", fmt.Errorf("failed to assign permissions")
	}

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
		"role", role,
	)
	s.logger.Security().AdminAction(actor, "invite_member", "tenant.Service.InviteMember", tenantID+":"+email)
	s.incrementCounter("invitation_sent", role)
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

	if err := s.storage.DeleteTenant(ctx, id); err != nil {
		s.recordError(span, "failed to delete tenant from storage", err, "tenant_id", id)
		return fmt.Errorf("failed to delete tenant from storage: %w", err)
	}

	if err := s.authz.DeleteTenant(ctx, id); err != nil {
		// Log error but don't fail, storage is already deleted
		s.logger.Errorw("failed to delete tenant from authz", "tenant_id", id, "error", err)
	}

	s.logger.Infow("tenant deleted", "tenant_id", id)
	s.logger.Security().AdminAction(actor, "delete_tenant", "tenant.Service.DeleteTenant", id)
	return nil
}

func (s *Service) ProvisionUser(ctx context.Context, tenantID, email, role string) error {
	ctx, span := s.tracer.Start(ctx, "admin.ProvisionUser")
	defer span.End()

	actor, _ := authentication.GetUserID(ctx)
	s.logger.Debugw("provisioning user",
		"tenant_id", tenantID,
		"email", email,
		"role", role,
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
	if _, err := s.storage.AddMember(ctx, tenantID, identityID, role); err != nil {
		s.recordError(span, "failed to add provisioned member to storage", err,
			"tenant_id", tenantID,
			"user_id", identityID,
			"role", role,
		)
		return fmt.Errorf("failed to add member to storage: %w", err)
	}

	// 3. Add to AuthZ
	var authzErr error
	switch role {
	case "owner":
		authzErr = s.authz.AssignTenantOwner(ctx, tenantID, identityID)
	case "member", "admin":
		// Proto has owner, admin, member.
		authzErr = s.authz.AssignTenantMember(ctx, tenantID, identityID)
	default:
		err := fmt.Errorf("unknown role: %s", role)
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	if authzErr != nil {
		s.recordError(span, "failed to assign role in authz", authzErr,
			"tenant_id", tenantID,
			"user_id", identityID,
			"role", role,
		)
		return fmt.Errorf("failed to assign role in authz: %w", authzErr)
	}

	s.logger.Infow("user provisioned",
		"tenant_id", tenantID,
		"user_id", identityID,
		"email", email,
		"role", role,
	)
	s.logger.Security().AdminAction(actor, "provision_user", "tenant.Service.ProvisionUser", tenantID+":"+email)
	s.incrementCounter("user_provisioned", role)
	return nil
}

func (s *Service) ListUserTenants(ctx context.Context, userID string, opts types.ListOptions) ([]*types.Tenant, string, error) {
	ctx, span := s.tracer.Start(ctx, "admin.ListUserTenants")
	defer span.End()

	s.logger.Debugw("listing tenants for user (admin)", "user_id", userID)

	tenants, nextPageToken, err := s.storage.ListTenantsByUserID(ctx, userID, opts)
	if err != nil {
		s.recordError(span, "failed to list tenants for user", err, "user_id", userID)
		return nil, "", fmt.Errorf("failed to list tenants for user: %w", err)
	}

	return tenants, nextPageToken, nil
}

func (s *Service) ListTenantUsers(ctx context.Context, tenantID string, opts types.ListOptions) ([]*types.TenantUser, string, error) {
	ctx, span := s.tracer.Start(ctx, "admin.ListTenantUsers")
	defer span.End()

	s.logger.Debugw("listing members for tenant", "tenant_id", tenantID)

	members, nextPageToken, err := s.storage.ListMembersByTenantID(ctx, tenantID, opts)
	if err != nil {
		s.recordError(span, "failed to list members", err, "tenant_id", tenantID)
		return nil, "", fmt.Errorf("failed to list members: %w", err)
	}

	var users []*types.TenantUser
	for _, m := range members {
		email := ""
		// Fetch identity details from Kratos to get email
		identity, err := s.kratos.GetIdentity(ctx, m.KratosIdentityID)
		if err != nil {
			// Log error but continue, user might have been deleted from Kratos but not from our DB
			s.logger.Warnw("failed to get identity for user; continuing with unknown email",
				"tenant_id", tenantID,
				"user_id", m.KratosIdentityID,
				"error", err,
			)
			email = "unknown"
		} else {
			// Extract email from traits
			if traits, ok := identity.Traits.(map[string]interface{}); ok {
				if e, ok := traits["email"].(string); ok {
					email = e
				}
			}
		}

		users = append(users, &types.TenantUser{
			UserID: m.KratosIdentityID,
			Email:  email,
			Role:   m.Role,
		})
	}

	return users, nextPageToken, nil
}

func (s *Service) UpdateTenantUser(ctx context.Context, tenantID, userID, role string) (*types.TenantUser, error) {
	ctx, span := s.tracer.Start(ctx, "admin.UpdateTenantUser")
	defer span.End()

	actor, _ := authentication.GetUserID(ctx)
	s.logger.Debugw("updating tenant user role",
		"tenant_id", tenantID,
		"user_id", userID,
		"role", role,
		"actor", actor,
	)

	// 1. Get current member to check if exists and current role
	members, _, err := s.storage.ListMembersByTenantID(ctx, tenantID, types.ListOptions{})
	if err != nil {
		s.recordError(span, "failed to check current membership", err,
			"tenant_id", tenantID,
			"user_id", userID,
		)
		return nil, fmt.Errorf("failed to check current membership: %w", err)
	}

	var currentMember *types.Membership
	for _, m := range members {
		if m.KratosIdentityID == userID {
			currentMember = m
			break
		}
	}
	if currentMember == nil {
		err := fmt.Errorf("user %s not found in tenant %s", userID, tenantID)
		s.recordError(span, "user not found in tenant", err, "tenant_id", tenantID, "user_id", userID)
		return nil, err
	}

	if currentMember.Role == role {
		return &types.TenantUser{
			UserID: userID,
			Role:   role,
			// Email is fetched separately if needed or just return partial
		}, nil
	}

	// 2. AuthZ Update
	// Remove old role relation first to avoid transient permission issues?
	// Or add new first?
	// If demoting owner -> member: Add member, remove owner.
	// If promoting member -> owner: Add owner, remove member (optional but clean).

	// Add new role
	switch role {
	case "owner":
		if err := s.authz.AssignTenantOwner(ctx, tenantID, userID); err != nil {
			s.recordError(span, "failed to assign owner role in authz", err,
				"tenant_id", tenantID,
				"user_id", userID,
			)
			return nil, fmt.Errorf("failed to assign owner role: %w", err)
		}
	case "member", "admin":
		if err := s.authz.AssignTenantMember(ctx, tenantID, userID); err != nil {
			s.recordError(span, "failed to assign member role in authz", err,
				"tenant_id", tenantID,
				"user_id", userID,
			)
			return nil, fmt.Errorf("failed to assign member role: %w", err)
		}
	default:
		err := fmt.Errorf("invalid role: %s", role)
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}

	// Remove old role
	switch currentMember.Role {
	case "owner":
		if err := s.authz.RemoveTenantOwner(ctx, tenantID, userID); err != nil {
			s.logger.Errorw("failed to remove old owner relation from authz",
				"tenant_id", tenantID,
				"user_id", userID,
				"error", err,
			)
			// Continue, as new role is assigned.
		}
	case "member", "admin":
		if role == "owner" {
			// If promoting to owner, we can remove the member relation to be clean
			if err := s.authz.RemoveTenantMember(ctx, tenantID, userID); err != nil {
				s.logger.Errorw("failed to remove old member relation from authz",
					"tenant_id", tenantID,
					"user_id", userID,
					"error", err,
				)
			}
		}
	}

	// 3. Storage Update
	if err := s.storage.UpdateMember(ctx, tenantID, userID, role); err != nil {
		s.recordError(span, "failed to update member in storage", err,
			"tenant_id", tenantID,
			"user_id", userID,
			"role", role,
		)
		return nil, err
	}

	// 4. Return updated user
	identity, err := s.kratos.GetIdentity(ctx, userID)
	email := ""
	if err == nil {
		if traits, ok := identity.Traits.(map[string]interface{}); ok {
			if e, ok := traits["email"].(string); ok {
				email = e
			}
		}
	} else {
		s.logger.Warnw("failed to fetch identity email after role update; returning empty",
			"tenant_id", tenantID,
			"user_id", userID,
			"error", err,
		)
	}

	s.logger.Infow("tenant user role updated",
		"tenant_id", tenantID,
		"user_id", userID,
		"role", role,
		"previous_role", currentMember.Role,
	)
	s.logger.Security().AdminAction(actor, "update_tenant_user", "tenant.Service.UpdateTenantUser", tenantID+":"+userID)

	return &types.TenantUser{
		UserID: userID,
		Email:  email,
		Role:   role,
	}, nil
}

func (s *Service) incrementCounter(operation, role string) {
	if err := s.monitor.IncrementCounter(map[string]string{"operation": operation, "role": role}); err != nil {
		s.logger.Warnf("failed to increment counter %s: %v", operation, err)
	}
}

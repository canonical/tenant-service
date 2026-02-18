// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0

package tenant

import (
	"context"
	"errors"
	"fmt"

	"github.com/canonical/tenant-service/internal/logging"
	"github.com/canonical/tenant-service/internal/monitoring"
	"github.com/canonical/tenant-service/internal/storage"
	"github.com/canonical/tenant-service/internal/tracing"
	"github.com/canonical/tenant-service/internal/types"
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

func (s *Service) ListTenantsByUserID(ctx context.Context, userID string) ([]*types.Tenant, error) {
	ctx, span := s.tracer.Start(ctx, "tenant.Service.ListTenantsByUserID")
	defer span.End()

	return s.storage.ListTenantsByUserID(ctx, userID)
}

func (s *Service) ListTenants(ctx context.Context) ([]*types.Tenant, error) {
	ctx, span := s.tracer.Start(ctx, "tenant.Service.ListTenants")
	defer span.End()

	return s.storage.ListTenants(ctx)
}

func (s *Service) InviteMember(ctx context.Context, tenantID, email, role string) (string, string, error) {
	ctx, span := s.tracer.Start(ctx, "tenant.Service.InviteMember")
	defer span.End()

	// 1. Ensure Identity Exists in Kratos
	identityID, err := s.kratos.GetIdentityIDByEmail(ctx, email)
	if err != nil {
		s.logger.Errorf("Failed to check identity existence: %v", err)
		return "", "", fmt.Errorf("failed to check identity")
	}

	if identityID == "" {
		s.logger.Infof("Creating new identity for email %s", email)
		identityID, err = s.kratos.CreateIdentity(ctx, email)
		if err != nil {
			s.logger.Errorf("Failed to create identity: %v", err)
			return "", "", fmt.Errorf("failed to provision user")
		}
	}

	// 2. Add Member to Database (idempotent for duplicate key)
	if _, err := s.storage.AddMember(ctx, tenantID, identityID, role); err != nil {
		if !errors.Is(err, storage.ErrDuplicateKey) {
			s.logger.Errorf("Failed to add member to storage: %v", err)
			return "", "", fmt.Errorf("failed to add member")
		}
		// If duplicate (already a member), we proceed to send recovery link as a password reset/re-invite mechanism.
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
		s.logger.Errorf("Failed to assign role in authz: %v", err)
		return "", "", fmt.Errorf("failed to assign permissions")
	}

	// 4. Generate Kratos Recovery Link
	// We use the configured lifetime for the link
	link, code, err := s.kratos.CreateRecoveryLink(ctx, identityID, s.invitationLifetime)
	if err != nil {
		s.logger.Errorf("Failed to create recovery link: %v", err)
		return "", "", fmt.Errorf("failed to generate invitation link")
	}

	return link, code, nil
}

func (s *Service) CreateTenant(ctx context.Context, name string) (*types.Tenant, error) {
	ctx, span := s.tracer.Start(ctx, "admin.CreateTenant")
	defer span.End()

	t := &types.Tenant{
		Name:    name,
		Enabled: true, // Admin created tenants are enabled by default
	}

	created, err := s.storage.CreateTenant(ctx, t)
	if err != nil {
		return nil, fmt.Errorf("failed to create tenant: %w", err)
	}

	return created, nil
}

func (s *Service) UpdateTenant(ctx context.Context, id, name string, ownerIDs []string) (*types.Tenant, error) {
	ctx, span := s.tracer.Start(ctx, "admin.UpdateTenant")
	defer span.End()

	if err := s.storage.UpdateTenant(ctx, id, name, ownerIDs); err != nil {
		return nil, fmt.Errorf("failed to update tenant: %w", err)
	}

	updated, err := s.storage.GetTenantByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get updated tenant: %w", err)
	}

	if len(ownerIDs) > 0 {
		// Attempt to sync FGA - naive approach: add new owners.
		for _, ownerID := range ownerIDs {
			if err := s.authz.AssignTenantOwner(ctx, id, ownerID); err != nil {
				s.logger.Errorf("failed to assign owner in FGA: %v", err)
				// Don't fail the request, just log
			}
		}
	}

	return updated, nil
}

func (s *Service) DeleteTenant(ctx context.Context, id string) error {
	ctx, span := s.tracer.Start(ctx, "admin.DeleteTenant")
	defer span.End()

	if err := s.storage.DeleteTenant(ctx, id); err != nil {
		return fmt.Errorf("failed to delete tenant from storage: %w", err)
	}

	if err := s.authz.DeleteTenant(ctx, id); err != nil {
		// Log error but don't fail, storage is already deleted
		s.logger.Errorf("failed to delete tenant from authz: %v", err)
	}

	return nil
}

func (s *Service) ProvisionUser(ctx context.Context, tenantID, email, role string) error {
	ctx, span := s.tracer.Start(ctx, "admin.ProvisionUser")
	defer span.End()

	// 1. Find or Create Identity
	identityID, err := s.kratos.GetIdentityIDByEmail(ctx, email)
	if err != nil {
		return err
	}
	if identityID == "" {
		identityID, err = s.kratos.CreateIdentity(ctx, email)
		if err != nil {
			return fmt.Errorf("failed to create identity: %w", err)
		}
	}

	// 2. Add to Storage
	if _, err := s.storage.AddMember(ctx, tenantID, identityID, role); err != nil {
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
		return fmt.Errorf("unknown role: %s", role)
	}

	if authzErr != nil {
		return fmt.Errorf("failed to assign role in authz: %w", authzErr)
	}

	return nil
}

func (s *Service) ActivateTenant(ctx context.Context, tenantID string) error {
	ctx, span := s.tracer.Start(ctx, "admin.ActivateTenant")
	defer span.End()

	if err := s.storage.SetTenantStatus(ctx, tenantID, true); err != nil {
		return fmt.Errorf("failed to activate tenant: %w", err)
	}

	return nil
}

func (s *Service) DeactivateTenant(ctx context.Context, tenantID string) error {
	ctx, span := s.tracer.Start(ctx, "admin.DeactivateTenant")
	defer span.End()

	if err := s.storage.SetTenantStatus(ctx, tenantID, false); err != nil {
		return fmt.Errorf("failed to deactivate tenant: %w", err)
	}

	return nil
}

func (s *Service) ListUserTenants(ctx context.Context, userID string) ([]*types.Tenant, error) {
	ctx, span := s.tracer.Start(ctx, "admin.ListUserTenants")
	defer span.End()

	tenants, err := s.storage.ListTenantsByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to list tenants for user: %w", err)
	}

	return tenants, nil
}

func (s *Service) ListTenantUsers(ctx context.Context, tenantID string) ([]*types.TenantUser, error) {
	ctx, span := s.tracer.Start(ctx, "admin.ListTenantUsers")
	defer span.End()

	members, err := s.storage.ListMembersByTenantID(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to list members: %w", err)
	}

	var users []*types.TenantUser
	for _, m := range members {
		email := ""
		// Fetch identity details from Kratos to get email
		identity, err := s.kratos.GetIdentity(ctx, m.KratosIdentityID)
		if err != nil {
			// Log error but continue, user might have been deleted from Kratos but not from our DB
			s.logger.Warn("failed to get identity for user", "user_id", m.KratosIdentityID, "err", err)
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

	return users, nil
}

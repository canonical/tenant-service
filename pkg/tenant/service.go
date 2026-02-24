// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0

package tenant

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"

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

	tenants, err := s.storage.ListTenantsByUserID(ctx, userID)
	return tenants, err
}

func (s *Service) ListTenants(ctx context.Context) ([]*types.Tenant, error) {
	ctx, span := s.tracer.Start(ctx, "tenant.Service.ListTenants")
	defer span.End()

	tenants, err := s.storage.ListTenants(ctx)
	if err != nil {
		return nil, err
	}

	return tenants, nil
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

func (s *Service) UpdateTenant(ctx context.Context, tenant *types.Tenant, paths []string) (*types.Tenant, error) {
	ctx, span := s.tracer.Start(ctx, "admin.UpdateTenant")
	defer span.End()

	if err := s.storage.UpdateTenant(ctx, tenant, paths); err != nil {
		return nil, fmt.Errorf("failed to update tenant: %w", err)
	}

	updated, err := s.storage.GetTenantByID(ctx, tenant.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to get updated tenant: %w", err)
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

func (s *Service) UpdateTenantUser(ctx context.Context, tenantID, userID, role string) (*types.TenantUser, error) {
	ctx, span := s.tracer.Start(ctx, "admin.UpdateTenantUser")
	defer span.End()

	// 1. Get current member to check if exists and current role
	members, err := s.storage.ListMembersByTenantID(ctx, tenantID)
	if err != nil {
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
		return nil, fmt.Errorf("user %s not found in tenant %s", userID, tenantID)
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
			return nil, fmt.Errorf("failed to assign owner role: %w", err)
		}
	case "member", "admin":
		if err := s.authz.AssignTenantMember(ctx, tenantID, userID); err != nil {
			return nil, fmt.Errorf("failed to assign member role: %w", err)
		}
	default:
		return nil, fmt.Errorf("invalid role: %s", role)
	}

	// Remove old role
	switch currentMember.Role {
	case "owner":
		if err := s.authz.RemoveTenantOwner(ctx, tenantID, userID); err != nil {
			s.logger.Errorf("failed to remove old owner relation: %v", err)
			// Continue, as new role is assigned.
		}
	case "member", "admin":
		if role == "owner" {
			// If promoting to owner, we can remove the member relation to be clean
			if err := s.authz.RemoveTenantMember(ctx, tenantID, userID); err != nil {
				s.logger.Errorf("failed to remove old member relation: %v", err)
			}
		}
	}

	// 3. Storage Update
	if err := s.storage.UpdateMember(ctx, tenantID, userID, role); err != nil {
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
	}

	return &types.TenantUser{
		UserID: userID,
		Email:  email,
		Role:   role,
	}, nil
}

func encodePageToken(offset uint64) string {
	return base64.URLEncoding.EncodeToString([]byte(strconv.FormatUint(offset, 10)))
}

func decodePageToken(token string) (uint64, error) {
	if token == "" {
		return 0, nil
	}
	data, err := base64.URLEncoding.DecodeString(token)
	if err != nil {
		return 0, err
	}
	return strconv.ParseUint(string(data), 10, 64)
}

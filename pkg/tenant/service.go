// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package tenant

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	v1 "github.com/canonical/authorization-service/api/v1"
	"github.com/canonical/tenant-service/internal/kratos"
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
	invitationLifetime time.Duration
	tracer             tracing.TracingInterface
	monitor            monitoring.MonitorInterface
	logger             logging.LoggerInterface
}

// NewService creates a new tenant service.
func NewService(
	storage StorageInterface,
	publisher permissions.Publisher,
	kratos KratosClientInterface,
	invitationLifetime time.Duration,
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

func (s *Service) InviteMember(ctx context.Context, tenantID, email string) (*types.Invitation, error) {
	ctx, span := s.tracer.Start(ctx, "tenant.Service.InviteMember")
	defer span.End()

	actor, _ := authentication.GetUserID(ctx)
	s.logger.Debugw("inviting member to tenant",
		"tenant_id", tenantID,
		"email", email,
		"actor", actor,
	)

	// 1. Check the Identity in Kratos, before the first statement: the
	// request's transaction begins there, and should not stay open meanwhile.
	identityID, err := s.kratos.GetIdentityIDByEmail(ctx, email)
	if err != nil {
		s.recordError(span, "failed to check identity existence", err,
			"tenant_id", tenantID,
			"email", email,
		)
		return nil, fmt.Errorf("failed to check identity")
	}

	tenant, err := s.getTenant(ctx, span, tenantID)
	if err != nil {
		return nil, err
	}
	if tenant.IsPersonal() {
		s.recordError(span, "invitation refused: personal tenant", ErrPersonalTenant, "tenant_id", tenantID)
		return nil, ErrPersonalTenant
	}

	policy, err := s.getSSOPolicy(ctx, span, tenantID)
	if err != nil {
		return nil, err
	}
	required := policy.RequiresSSO()
	if !policy.InvitationAdmitsDomain(emailDomain(email)) {
		s.recordError(span, "invitation refused: address outside the tenant's domains", ErrDomainNotAllowed,
			"tenant_id", tenantID,
			"email", email,
		)
		return nil, ErrDomainNotAllowed
	}

	if identityID != "" {
		// An existing account is never sent a recovery link: it would hand the
		// account over to whoever holds the invitation.
		_, err := s.storage.GetMemberByTenantAndUserID(ctx, tenantID, identityID)
		if err == nil {
			return &types.Invitation{}, nil
		}
		if !errors.Is(err, storage.ErrNotFound) {
			s.recordError(span, "failed to check membership", err,
				"tenant_id", tenantID,
				"user_id", identityID,
			)
			return nil, fmt.Errorf("failed to check membership: %w", err)
		}
		return s.invitePending(ctx, span, tenantID, email, actor)
	}
	if required {
		return s.invitePending(ctx, span, tenantID, email, actor)
	}

	// 2. Create the Identity and its Recovery Link, before the first write:
	// no row stays locked while Kratos answers.
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
		return nil, fmt.Errorf("failed to provision user")
	}

	link, code, err := s.kratos.CreateRecoveryLink(ctx, identityID, s.invitationLifetime.String())
	if err != nil {
		s.recordError(span, "failed to create recovery link", err,
			"tenant_id", tenantID,
			"user_id", identityID,
		)
		return nil, fmt.Errorf("failed to generate invitation link")
	}

	// 3. Add Member to Database
	if err := s.addMember(ctx, span, tenantID, identityID, email); err != nil {
		return nil, err
	}

	s.logger.Infow("member invited successfully",
		"tenant_id", tenantID,
		"user_id", identityID,
		"email", email,
	)
	s.logger.Security().AdminAction(actor, "invite_member", "tenant.Service.InviteMember", tenantID+":"+email)
	s.incrementCounter("invitation_sent")
	return &types.Invitation{Link: link, Code: code}, nil
}

// invitePending leaves the membership to the user's sign-in to the tenant.
func (s *Service) invitePending(ctx context.Context, span trace.Span, tenantID, email, actor string) (*types.Invitation, error) {
	if err := s.storage.AddInvitation(ctx, tenantID, strings.ToLower(email), s.invitationLifetime); err != nil {
		s.recordError(span, "failed to add invitation", err,
			"tenant_id", tenantID,
			"email", email,
		)
		return nil, fmt.Errorf("failed to add invitation: %w", err)
	}

	s.logger.Infow("pending invitation recorded",
		"tenant_id", tenantID,
		"email", email,
	)
	s.logger.Security().AdminAction(actor, "invite_member_pending", "tenant.Service.InviteMember", tenantID+":"+email)
	s.incrementCounter("invitation_pending")
	return &types.Invitation{Pending: true}, nil
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

// CreatePersonalTenant returns identityID's personal tenant and whether it
// created it. An account that belongs to an enabled tenant or has a pending
// invitation gets none (ErrHasTenant): an invitation waits for the user to
// sign in to its tenant. With no email, the account's is read from Kratos.
func (s *Service) CreatePersonalTenant(ctx context.Context, identityID, email string) (*types.Tenant, bool, error) {
	ctx, span := s.tracer.Start(ctx, "tenant.Service.CreatePersonalTenant")
	defer span.End()

	actor, _ := authentication.GetUserID(ctx)
	s.logger.Debugw("creating personal tenant",
		"identity_id", identityID,
		"actor", actor,
	)

	if email == "" {
		// Kratos before the first statement, as in InviteMember.
		identity, err := s.getIdentity(ctx, span, identityID)
		if err != nil {
			return nil, false, err
		}
		email = identityEmail(identity)
	}

	existing, err := s.storage.GetPersonalTenantByUserID(ctx, identityID)
	if err == nil {
		return existing, false, nil
	}
	if !errors.Is(err, storage.ErrNotFound) {
		s.recordError(span, "failed to get personal tenant", err, "identity_id", identityID)
		return nil, false, fmt.Errorf("failed to get personal tenant: %w", err)
	}

	invited, err := s.storage.ListInvitedTenantsByEmail(ctx, strings.ToLower(email), emailDomain(email), "")
	if err != nil {
		s.recordError(span, "failed to list invited tenants", err, "identity_id", identityID)
		return nil, false, fmt.Errorf("failed to list invited tenants: %w", err)
	}
	if len(invited) > 0 {
		return nil, false, ErrHasTenant
	}

	// Only enabled tenants count: an account whose tenants are all disabled
	// has nowhere to sign in.
	tenants, err := s.storage.ListTenantsByUserID(ctx, identityID, types.WithEnabled(true))
	if err != nil {
		s.recordError(span, "failed to list tenants for identity", err, "identity_id", identityID)
		return nil, false, fmt.Errorf("failed to list tenants: %w", err)
	}
	if len(tenants) > 0 {
		return nil, false, ErrHasTenant
	}

	tenant, created, err := s.storage.CreatePersonalTenant(ctx, identityID, types.PersonalTenantName(email))
	if err != nil {
		s.recordError(span, "failed to create personal tenant", err, "identity_id", identityID)
		return nil, false, fmt.Errorf("failed to create personal tenant: %w", err)
	}
	if !created {
		// A concurrent call created it first, and published its permission.
		return tenant, false, nil
	}

	if _, err := s.storage.AddMember(ctx, tenant.ID, identityID); err != nil {
		s.recordError(span, "failed to add member to personal tenant", err,
			"tenant_id", tenant.ID,
			"identity_id", identityID,
		)
		return nil, false, fmt.Errorf("failed to add member: %w", err)
	}

	// Grant the account full control of its tenant asynchronously via Kafka.
	s.publisher.Publish(ctx, tenant.ID, permissions.TenantPermissionOp(
		v1.PermissionOp_PERMISSION_OP_WRITE,
		identityID,
		permissions.RelationCanDelete,
		tenant.ID,
	))

	s.logger.Infow("personal tenant created", "tenant_id", tenant.ID, "identity_id", identityID)
	s.logger.Security().AdminAction(actor, "create_personal_tenant", "tenant.Service.CreatePersonalTenant", tenant.ID)
	s.incrementCounter("personal_tenant_created")
	return tenant, true, nil
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
		if errors.Is(err, storage.ErrNotFound) {
			return nil, ErrTenantNotFound
		}
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

	// 1. Find the Identity, before the first statement as in InviteMember
	identityID, err := s.kratos.GetIdentityIDByEmail(ctx, email)
	if err != nil {
		s.recordError(span, "failed to look up identity", err,
			"tenant_id", tenantID,
			"email", email,
		)
		return err
	}

	tenant, err := s.getTenant(ctx, span, tenantID)
	if err != nil {
		return err
	}
	if tenant.IsPersonal() {
		s.recordError(span, "provisioning refused: personal tenant", ErrPersonalTenant, "tenant_id", tenantID)
		return ErrPersonalTenant
	}

	if identityID != "" {
		_, err := s.storage.GetMemberByTenantAndUserID(ctx, tenantID, identityID)
		if err == nil {
			return ErrAlreadyMember
		}
		if !errors.Is(err, storage.ErrNotFound) {
			s.recordError(span, "failed to check membership", err,
				"tenant_id", tenantID,
				"user_id", identityID,
			)
			return fmt.Errorf("failed to check membership: %w", err)
		}
	}

	// 2. Create the Identity if absent, for a tenant that exists
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

	// 3. Add to Storage
	if err := s.addMember(ctx, span, tenantID, identityID, email); err != nil {
		return err
	}

	s.logger.Infow("user provisioned",
		"tenant_id", tenantID,
		"user_id", identityID,
		"email", email,
	)
	s.logger.Security().AdminAction(actor, "provision_user", "tenant.Service.ProvisionUser", tenantID+":"+email)
	s.incrementCounter("user_provisioned")
	return nil
}

// JoinTenant makes the account a member of the tenant when the tenant admits
// its address by a pending invitation, which is spent, or by auto-join
// (ErrNotAdmitted otherwise). An account that is a member already is success.
func (s *Service) JoinTenant(ctx context.Context, tenantID, identityID string) error {
	ctx, span := s.tracer.Start(ctx, "tenant.Service.JoinTenant")
	defer span.End()

	actor, _ := authentication.GetUserID(ctx)
	s.logger.Debugw("joining tenant",
		"tenant_id", tenantID,
		"identity_id", identityID,
		"actor", actor,
	)

	// 1. Read the Identity, before the first statement as in InviteMember
	identity, err := s.getIdentity(ctx, span, identityID)
	if err != nil {
		return err
	}
	email := identityEmail(identity)

	tenant, err := s.getTenant(ctx, span, tenantID)
	if err != nil {
		return err
	}

	_, err = s.storage.GetMemberByTenantAndUserID(ctx, tenantID, identityID)
	if err == nil {
		return nil
	}
	if !errors.Is(err, storage.ErrNotFound) {
		s.recordError(span, "failed to check membership", err,
			"tenant_id", tenantID,
			"user_id", identityID,
		)
		return fmt.Errorf("failed to check membership: %w", err)
	}

	// 2. Check what admits the address
	policy, err := s.getSSOPolicy(ctx, span, tenantID)
	if err != nil {
		return err
	}
	invitation, autoJoin, err := s.admission(ctx, span, tenant, policy, strings.ToLower(email))
	if err != nil {
		return err
	}
	admittedBy := ""
	switch {
	case invitation:
		admittedBy = "invitation"
	case autoJoin:
		admittedBy = "auto_join"
	default:
		s.recordError(span, "join refused: address not admitted", ErrNotAdmitted,
			"tenant_id", tenantID,
			"email", email,
		)
		return ErrNotAdmitted
	}

	// 3. Add to Storage
	if err := s.addMember(ctx, span, tenantID, identityID, email); err != nil {
		if errors.Is(err, ErrAlreadyMember) {
			// A concurrent call added it first.
			return nil
		}
		return err
	}

	s.logger.Infow("user joined tenant",
		"tenant_id", tenantID,
		"user_id", identityID,
		"email", email,
		"admitted_by", admittedBy,
	)
	s.logger.Security().AdminAction(actor, "join_tenant", "tenant.Service.JoinTenant", tenantID+":"+email)
	s.incrementCounter("tenant_joined")
	return nil
}

// addMember makes identityID a member of the tenant (ErrAlreadyMember when it
// is one), deletes the invitation of email and grants view access.
func (s *Service) addMember(ctx context.Context, span trace.Span, tenantID, identityID, email string) error {
	if _, err := s.storage.AddMember(ctx, tenantID, identityID); err != nil {
		if errors.Is(err, storage.ErrDuplicateKey) {
			return ErrAlreadyMember
		}
		s.recordError(span, "failed to add member to storage", err,
			"tenant_id", tenantID,
			"user_id", identityID,
		)
		return fmt.Errorf("failed to add member: %w", err)
	}
	// The membership supersedes a pending invitation: left behind, it would
	// admit the user again after a removal.
	if err := s.storage.DeleteInvitation(ctx, tenantID, strings.ToLower(email)); err != nil {
		s.recordError(span, "failed to delete invitation", err, "tenant_id", tenantID)
		return fmt.Errorf("failed to delete invitation: %w", err)
	}

	// Grant view access asynchronously via Kafka. Elevated permissions are
	// managed through the authorization service API.
	s.publisher.Publish(ctx, tenantID, permissions.TenantPermissionOp(
		v1.PermissionOp_PERMISSION_OP_WRITE,
		identityID,
		permissions.RelationCanView,
		tenantID,
	))

	return nil
}

// admission reports what admits email to tenant at a sign-in: a pending
// invitation, auto-join. A disabled tenant and a personal tenant admit nobody.
// An invitation admits an address only while InviteMember would still invite
// it: the tenant may have required SSO, or got its domains, since.
func (s *Service) admission(ctx context.Context, span trace.Span, tenant *types.Tenant, policy *types.TenantSSOPolicy, email string) (invitation, autoJoin bool, err error) {
	if !tenant.Enabled || tenant.IsPersonal() {
		return false, false, nil
	}

	if email != "" {
		invitation, err = s.storage.HasInvitationByTenantAndEmail(ctx, tenant.ID, email)
		if err != nil {
			s.recordError(span, "failed to get invitation", err, "tenant_id", tenant.ID)
			return false, false, fmt.Errorf("failed to get invitation: %w", err)
		}
	}

	domain := emailDomain(email)
	return invitation && policy.InvitationAdmitsDomain(domain), policy.AutoJoinAdmitsDomain(domain), nil
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

// RemoveTenantUser revokes every relation the member may hold on the tenant:
// which ones were granted through the authorization service is not known here.
func (s *Service) RemoveTenantUser(ctx context.Context, tenantID, userID string) error {
	ctx, span := s.tracer.Start(ctx, "admin.RemoveTenantUser")
	defer span.End()

	actor, _ := authentication.GetUserID(ctx)
	s.logger.Debugw("removing tenant user",
		"tenant_id", tenantID,
		"user_id", userID,
		"actor", actor,
	)

	tenant, err := s.getTenant(ctx, span, tenantID)
	if err != nil {
		return err
	}
	// Its account would be locked out of a personal tenant for good.
	if tenant.IsPersonal() {
		s.recordError(span, "removal refused: personal tenant", ErrPersonalTenant,
			"tenant_id", tenantID,
			"user_id", userID,
		)
		return ErrPersonalTenant
	}

	if err := s.storage.DeleteMember(ctx, tenantID, userID); err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return fmt.Errorf("%w: user %s in tenant %s", ErrMemberNotFound, userID, tenantID)
		}
		s.recordError(span, "failed to remove member from storage", err,
			"tenant_id", tenantID,
			"user_id", userID,
		)
		return fmt.Errorf("failed to remove member: %w", err)
	}

	// Revoke asynchronously via Kafka; the authorization service ignores
	// deletes of tuples that do not exist.
	s.publisher.Publish(ctx, tenantID, permissions.RevokeTenantPermissionOps(userID, tenantID)...)

	s.logger.Infow("tenant user removed", "tenant_id", tenantID, "user_id", userID)
	s.logger.Security().AdminAction(actor, "remove_tenant_user", "tenant.Service.RemoveTenantUser", tenantID+":"+userID)
	return nil
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

// getTenant returns the tenant, ErrTenantNotFound when it does not exist.
func (s *Service) getTenant(ctx context.Context, span trace.Span, tenantID string) (*types.Tenant, error) {
	tenant, err := s.storage.GetTenantByID(ctx, tenantID)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return nil, ErrTenantNotFound
		}
		s.recordError(span, "failed to get tenant", err, "tenant_id", tenantID)
		return nil, fmt.Errorf("failed to get tenant: %w", err)
	}
	return tenant, nil
}

// getIdentity returns the identity, ErrIdentityNotFound when it does not exist.
func (s *Service) getIdentity(ctx context.Context, span trace.Span, identityID string) (*ory.Identity, error) {
	identity, err := s.kratos.GetIdentity(ctx, identityID)
	if err != nil {
		if errors.Is(err, kratos.ErrNotFound) {
			return nil, ErrIdentityNotFound
		}
		s.recordError(span, "failed to get identity", err, "identity_id", identityID)
		return nil, fmt.Errorf("failed to get identity: %w", err)
	}
	return identity, nil
}

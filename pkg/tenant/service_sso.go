// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package tenant

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"go.opentelemetry.io/otel/trace"

	"github.com/canonical/tenant-service/internal/logging"
	"github.com/canonical/tenant-service/internal/storage"
	"github.com/canonical/tenant-service/internal/types"
	"github.com/canonical/tenant-service/pkg/authentication"
)

func (s *Service) GetTenantSSOPolicy(ctx context.Context, tenantID string) (*types.TenantSSOPolicy, error) {
	ctx, span := s.tracer.Start(ctx, "tenant.Service.GetTenantSSOPolicy")
	defer span.End()

	s.logger.Debugw("getting tenant sso policy", "tenant_id", tenantID)

	tenant, err := s.getTenant(ctx, span, tenantID)
	if err != nil {
		return nil, err
	}
	if tenant.IsPersonal() {
		return nil, ErrPersonalTenant
	}

	return s.getSSOPolicy(ctx, span, tenantID)
}

// PutTenantSSOPolicy leaves the stored domains as they are.
func (s *Service) PutTenantSSOPolicy(ctx context.Context, tenantID, enforcement string, autoJoin bool, bindings []types.SSOBinding) (*types.TenantSSOPolicy, error) {
	ctx, span := s.tracer.Start(ctx, "tenant.Service.PutTenantSSOPolicy")
	defer span.End()

	actor, _ := authentication.GetUserID(ctx)
	s.logger.Debugw("writing tenant sso policy", "tenant_id", tenantID, "actor", actor)

	policy, err := s.lockSSOPolicy(ctx, span, tenantID)
	if err != nil {
		return nil, err
	}

	if err := validateBindings(bindings); err != nil {
		return nil, err
	}
	before := policyLabel(policy)
	policy.Enforcement, policy.AutoJoin, policy.Bindings = enforcement, autoJoin, bindings
	if autoJoin && (enforcement != types.EnforcementRequired || len(policy.Domains) == 0) {
		return nil, ErrAutoJoinNeedsRequiredAndDomains
	}
	if enforcement == types.EnforcementRequired && !policy.HasActiveBinding() {
		return nil, ErrRequiredNeedsActiveBinding
	}

	if err := s.storage.UpdateTenantSSOPolicy(ctx, tenantID, enforcement, autoJoin, bindings); err != nil {
		if errors.Is(err, storage.ErrDuplicateKey) {
			return nil, ErrConnectionBound
		}
		s.recordError(span, "failed to write tenant sso policy", err, "tenant_id", tenantID)
		return nil, fmt.Errorf("failed to write tenant sso policy: %w", err)
	}

	s.logger.Security().AdminAction(actor, "put_tenant_sso_policy", "tenant.Service.PutTenantSSOPolicy", tenantID,
		logging.WithLabel("before", before),
		logging.WithLabel("after", policyLabel(policy)),
	)
	return policy, nil
}

// SetTenantSSODomains leaves the stored enforcement, auto-join and bindings as
// they are.
func (s *Service) SetTenantSSODomains(ctx context.Context, tenantID string, domains []string) (*types.TenantSSOPolicy, error) {
	ctx, span := s.tracer.Start(ctx, "tenant.Service.SetTenantSSODomains")
	defer span.End()

	actor, _ := authentication.GetUserID(ctx)
	s.logger.Debugw("setting tenant sso domains", "tenant_id", tenantID, "actor", actor)

	policy, err := s.lockSSOPolicy(ctx, span, tenantID)
	if err != nil {
		return nil, err
	}

	before := policy.Domains
	policy.Domains, err = normaliseDomains(domains)
	if err != nil {
		return nil, err
	}
	if policy.AutoJoin && len(policy.Domains) == 0 {
		return nil, ErrAutoJoinNeedsRequiredAndDomains
	}

	if err := s.storage.UpdateTenantSSODomains(ctx, tenantID, policy.Domains); err != nil {
		s.recordError(span, "failed to write tenant sso domains", err, "tenant_id", tenantID)
		return nil, fmt.Errorf("failed to write tenant sso domains: %w", err)
	}

	s.logger.Security().AdminAction(actor, "set_tenant_sso_domains", "tenant.Service.SetTenantSSODomains", tenantID,
		logging.WithLabel("before", strings.Join(before, ",")),
		logging.WithLabel("after", strings.Join(policy.Domains, ",")),
	)
	return policy, nil
}

// RemoveTenantSSOBinding writes nothing when the policy does not bind the
// connection.
func (s *Service) RemoveTenantSSOBinding(ctx context.Context, tenantID, connectionID string) error {
	ctx, span := s.tracer.Start(ctx, "tenant.Service.RemoveTenantSSOBinding")
	defer span.End()

	actor, _ := authentication.GetUserID(ctx)
	s.logger.Debugw("removing tenant sso binding",
		"tenant_id", tenantID,
		"connection_id", connectionID,
		"actor", actor,
	)

	policy, err := s.lockSSOPolicy(ctx, span, tenantID)
	if err != nil {
		return err
	}

	rest := slices.DeleteFunc(slices.Clone(policy.Bindings), func(b types.SSOBinding) bool { return b.ConnectionID == connectionID })
	if len(rest) == len(policy.Bindings) {
		return nil
	}
	before := policyLabel(policy)
	policy.Bindings = rest
	if policy.Enforcement == types.EnforcementRequired && !policy.HasActiveBinding() {
		return ErrRequiredNeedsActiveBinding
	}

	if err := s.storage.DeleteTenantSSOBinding(ctx, tenantID, connectionID); err != nil {
		s.recordError(span, "failed to remove tenant sso binding", err, "tenant_id", tenantID, "connection_id", connectionID)
		return fmt.Errorf("failed to remove tenant sso binding: %w", err)
	}

	s.logger.Security().AdminAction(actor, "remove_tenant_sso_binding", "tenant.Service.RemoveTenantSSOBinding", tenantID,
		logging.WithLabel("before", before),
		logging.WithLabel("after", policyLabel(policy)),
	)
	return nil
}

// lockSSOPolicy locks the tenant's row until the transaction ends and returns
// its policy as stored: two writes of one policy cannot undo each other.
func (s *Service) lockSSOPolicy(ctx context.Context, span trace.Span, tenantID string) (*types.TenantSSOPolicy, error) {
	tenant, policy, err := s.storage.LockTenantSSOPolicy(ctx, tenantID)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return nil, ErrTenantNotFound
		}
		s.recordError(span, "failed to lock tenant sso policy", err, "tenant_id", tenantID)
		return nil, fmt.Errorf("failed to lock tenant sso policy: %w", err)
	}
	if tenant.IsPersonal() {
		return nil, ErrPersonalTenant
	}
	return policy, nil
}

// ListSignInTenants returns the enabled tenants email may sign in to: those
// its account is a member of, then those it is invited to, then those it could
// auto-join. The last two do not depend on the email being known to Kratos:
// the answer does not reveal it.
func (s *Service) ListSignInTenants(ctx context.Context, email string) ([]*types.SignInTenant, error) {
	ctx, span := s.tracer.Start(ctx, "tenant.Service.ListSignInTenants")
	defer span.End()

	s.logger.Debugw("listing sign-in tenants")

	identityID, err := s.kratos.GetIdentityIDByEmail(ctx, email)
	if err != nil {
		s.recordError(span, "failed to look up identity by email", err, "email", email)
		return nil, fmt.Errorf("failed to look up identity: %w", err)
	}

	tenants := make([]*types.SignInTenant, 0)
	if identityID != "" {
		members, err := s.storage.ListTenantsByUserID(ctx, identityID, types.WithEnabled(true))
		if err != nil {
			s.recordError(span, "failed to list active tenants for identity", err, "email", email, "identity_id", identityID)
			return nil, fmt.Errorf("failed to list tenants: %w", err)
		}
		tenants = memberTenants(members)
	}

	domain := emailDomain(email)
	invited, err := s.storage.ListInvitedTenantsByEmail(ctx, strings.ToLower(email), domain, identityID)
	if err != nil {
		s.recordError(span, "failed to list invited tenants", err, "email", email)
		return nil, fmt.Errorf("failed to list invited tenants: %w", err)
	}
	tenants = append(tenants, invited...)

	if domain != "" {
		candidates, err := s.storage.ListAutoJoinCandidatesByDomain(ctx, domain, identityID)
		if err != nil {
			s.recordError(span, "failed to list auto-join candidates", err, "email", email)
			return nil, fmt.Errorf("failed to list auto-join candidates: %w", err)
		}
		tenants = mergeCandidates(tenants, candidates)
	}

	s.logger.Debugw("sign-in tenants found", "count", len(tenants))
	return tenants, nil
}

// GetSignInContext uses the account's address, and ignores email, when
// identityID is given.
func (s *Service) GetSignInContext(ctx context.Context, tenantID, email, identityID string) (*types.SignInContext, error) {
	ctx, span := s.tracer.Start(ctx, "tenant.Service.GetSignInContext")
	defer span.End()

	s.logger.Debugw("getting sign-in context", "tenant_id", tenantID)

	tenant, err := s.getTenant(ctx, span, tenantID)
	if err != nil {
		return nil, err
	}

	c := &types.SignInContext{MFARequirement: tenant.MFARequirement}

	if identityID != "" {
		identity, err := s.getIdentity(ctx, span, identityID)
		if err != nil {
			return nil, err
		}
		email = identityEmail(identity)
	} else {
		identityID, err = s.kratos.GetIdentityIDByEmail(ctx, email)
		if err != nil {
			s.recordError(span, "failed to look up identity by email", err, "tenant_id", tenantID)
			return nil, fmt.Errorf("failed to look up identity: %w", err)
		}
	}
	c.AccountExists = identityID != ""

	if c.AccountExists {
		// A member of a disabled tenant cannot sign in to it: not a member here.
		_, err := s.storage.GetActiveMemberByTenantAndUserID(ctx, tenantID, identityID)
		if err == nil {
			c.Member = true
		} else if !errors.Is(err, storage.ErrNotFound) {
			s.recordError(span, "failed to get membership", err, "tenant_id", tenantID, "identity_id", identityID)
			return nil, fmt.Errorf("failed to get membership: %w", err)
		}
	}

	policy, err := s.getSSOPolicy(ctx, span, tenantID)
	if err != nil {
		return nil, err
	}

	c.Enforcement = policy.EffectiveEnforcement()
	c.ConnectionIDs = make([]string, 0)
	if policy.AppliesToDomain(emailDomain(email)) {
		c.ConnectionIDs = policy.ActiveConnectionIDs()
	}
	if !c.Member {
		c.InvitationAdmits, c.AutoJoinAdmits, err = s.admission(ctx, span, tenant, policy, strings.ToLower(email))
		if err != nil {
			return nil, err
		}
	}

	return c, nil
}

// getSSOPolicy returns the policy, ErrTenantNotFound when the tenant does not exist.
func (s *Service) getSSOPolicy(ctx context.Context, span trace.Span, tenantID string) (*types.TenantSSOPolicy, error) {
	policy, err := s.storage.GetTenantSSOPolicy(ctx, tenantID)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return nil, ErrTenantNotFound
		}
		s.recordError(span, "failed to get tenant sso policy", err, "tenant_id", tenantID)
		return nil, fmt.Errorf("failed to get tenant sso policy: %w", err)
	}
	return policy, nil
}

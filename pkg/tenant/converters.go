// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package tenant

import (
	v0 "github.com/canonical/identity-platform-api/v0/tenant"
	"github.com/canonical/tenant-service/internal/types"
)

func signInTenantsToProto(tenants []*types.SignInTenant) []*v0.SignInTenant {
	pb := make([]*v0.SignInTenant, len(tenants))
	for i, t := range tenants {
		pb[i] = &v0.SignInTenant{
			Tenant:            tenantToProto(&t.Tenant),
			AutoJoinCandidate: t.AutoJoinCandidate,
			Invited:           t.Invited,
		}
	}
	return pb
}

func signInContextToProto(c *types.SignInContext) *v0.SignInContext {
	return &v0.SignInContext{
		Member:           c.Member,
		Enforcement:      enforcementToProto(c.Enforcement),
		ConnectionIds:    c.ConnectionIDs,
		AutoJoinAdmits:   c.AutoJoinAdmits,
		InvitationAdmits: c.InvitationAdmits,
		MfaRequirement:   mfaRequirementToProto(c.MFARequirement),
		AccountExists:    c.AccountExists,
	}
}

func enforcementToProto(e string) v0.Enforcement {
	switch e {
	case types.EnforcementOptional:
		return v0.Enforcement_ENFORCEMENT_OPTIONAL
	case types.EnforcementRequired:
		return v0.Enforcement_ENFORCEMENT_REQUIRED
	default:
		return v0.Enforcement_ENFORCEMENT_OFF
	}
}

func enforcementFromProto(e v0.Enforcement) string {
	switch e {
	case v0.Enforcement_ENFORCEMENT_OPTIONAL:
		return types.EnforcementOptional
	case v0.Enforcement_ENFORCEMENT_REQUIRED:
		return types.EnforcementRequired
	default:
		return ""
	}
}

func mfaRequirementToProto(r string) v0.MFARequirement {
	switch r {
	case types.MFARequirementRequired:
		return v0.MFARequirement_MFA_REQUIREMENT_REQUIRED
	default:
		return v0.MFARequirement_MFA_REQUIREMENT_NONE
	}
}

func mfaRequirementFromProto(r v0.MFARequirement) string {
	switch r {
	case v0.MFARequirement_MFA_REQUIREMENT_NONE:
		return types.MFARequirementNone
	case v0.MFARequirement_MFA_REQUIREMENT_REQUIRED:
		return types.MFARequirementRequired
	default:
		return ""
	}
}

func ssoPolicyToProto(p *types.TenantSSOPolicy) *v0.TenantSSOPolicy {
	bindings := make([]*v0.SSOBinding, len(p.Bindings))
	for i, b := range p.Bindings {
		bindings[i] = &v0.SSOBinding{
			ConnectionId: b.ConnectionID,
			Active:       b.Active,
		}
	}
	domains := p.Domains
	if domains == nil {
		domains = make([]string, 0)
	}
	return &v0.TenantSSOPolicy{
		TenantId: p.TenantID,
		// As stored; OFF until the enforcement is first written.
		Enforcement: enforcementToProto(p.Enforcement),
		AutoJoin:    p.AutoJoin,
		Domains:     domains,
		Bindings:    bindings,
	}
}

func mfaPolicyToProto(p *types.TenantMFAPolicy) *v0.TenantMFAPolicy {
	return &v0.TenantMFAPolicy{
		TenantId:    p.TenantID,
		Requirement: mfaRequirementToProto(p.Requirement),
	}
}

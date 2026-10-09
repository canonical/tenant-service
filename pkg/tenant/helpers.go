// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package tenant

import (
	"fmt"

	"github.com/canonical/tenant-service/internal/types"
	ory "github.com/ory/client-go"
)

// memberTenants returns the tenants of a member as sign-in tenants: no flag
// applies to a membership.
func memberTenants(tenants []*types.Tenant) []*types.SignInTenant {
	members := make([]*types.SignInTenant, len(tenants))
	for i, t := range tenants {
		members[i] = &types.SignInTenant{Tenant: *t}
	}
	return members
}

// mergeCandidates appends the auto-join candidates to tenants; a tenant the
// address is already invited to keeps one entry with both flags.
func mergeCandidates(tenants, candidates []*types.SignInTenant) []*types.SignInTenant {
	byID := make(map[string]*types.SignInTenant, len(tenants))
	for _, t := range tenants {
		byID[t.ID] = t
	}
	for _, c := range candidates {
		if t, ok := byID[c.ID]; ok {
			t.AutoJoinCandidate = t.AutoJoinCandidate || c.AutoJoinCandidate
			continue
		}
		tenants = append(tenants, c)
	}
	return tenants
}

// identityEmail returns the email trait of an identity, "" when it has none.
func identityEmail(identity *ory.Identity) string {
	if identity == nil {
		return ""
	}
	if traits, ok := identity.Traits.(map[string]interface{}); ok {
		if e, ok := traits["email"].(string); ok {
			return e
		}
	}
	return ""
}

// policyLabel renders a policy's enforcement, auto-join and bindings for an
// admin-action record.
func policyLabel(p *types.TenantSSOPolicy) string {
	return fmt.Sprintf("enforcement=%s auto_join=%t bindings=%+v", p.Enforcement, p.AutoJoin, p.Bindings)
}

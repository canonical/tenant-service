// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package types

import (
	"time"
)

const (
	defaultPageSize int32 = 100
	maxPageSize     int32 = 100
)

// PersonalTenantName is the default name of an account's personal tenant.
func PersonalTenantName(email string) string {
	return email + "'s Org"
}

type Tenant struct {
	ID        string    `db:"id"`
	Name      string    `db:"name"`
	CreatedAt time.Time `db:"created_at"`
	Enabled   bool      `db:"enabled"`
	// PersonalIdentityID is the account whose personal tenant this is; nil for an organisation.
	PersonalIdentityID *string `db:"personal_identity_id"`
	MFARequirement     string  `db:"mfa_requirement"`
}

// IsPersonal reports whether the tenant is an account's personal tenant.
func (t *Tenant) IsPersonal() bool {
	return t != nil && t.PersonalIdentityID != nil
}

// SignInTenant is a tenant an address may sign in to. Both flags are false
// for a membership.
type SignInTenant struct {
	Tenant
	// AutoJoinCandidate: not a membership, the tenant auto-joins the address's domain.
	AutoJoinCandidate bool
	// Invited: a pending invitation; the address may have no account yet.
	Invited bool
}

// Enforcement values. Only "optional" and "required" can be written, and
// with no active binding a tenant is off whatever is stored.
const (
	EnforcementOff      = "off"
	EnforcementOptional = "optional"
	EnforcementRequired = "required"
)

// SSOBinding is a tenant's use of one SSO connection. Only active bindings
// count for company sign-in.
type SSOBinding struct {
	ConnectionID string `db:"connection_id"`
	Active       bool   `db:"active"`
}

// TenantSSOPolicy is which SSO connections a tenant uses and how.
type TenantSSOPolicy struct {
	TenantID    string       `db:"id"`
	Enforcement string       `db:"sso_enforcement"`
	AutoJoin    bool         `db:"sso_auto_join"`
	Domains     []string     `db:"-"`
	Bindings    []SSOBinding `db:"-"`
}

// HasActiveBinding reports whether at least one binding is active.
func (p *TenantSSOPolicy) HasActiveBinding() bool {
	if p == nil {
		return false
	}
	for _, b := range p.Bindings {
		if b.Active {
			return true
		}
	}
	return false
}

// ActiveConnectionIDs returns the connections of the active bindings.
func (p *TenantSSOPolicy) ActiveConnectionIDs() []string {
	ids := make([]string, 0)
	if p == nil {
		return ids
	}
	for _, b := range p.Bindings {
		if b.Active {
			ids = append(ids, b.ConnectionID)
		}
	}
	return ids
}

// EffectiveEnforcement is the stored enforcement when the tenant has an
// active binding, and off otherwise.
func (p *TenantSSOPolicy) EffectiveEnforcement() string {
	if !p.HasActiveBinding() {
		return EnforcementOff
	}
	return p.Enforcement
}

// RequiresSSO reports whether the effective enforcement is "required".
func (p *TenantSSOPolicy) RequiresSSO() bool {
	return p.EffectiveEnforcement() == EnforcementRequired
}

// AppliesToDomain reports whether the tenant's company sign-ins apply to an
// address in domain: the tenant lists no domains, or lists this one.
func (p *TenantSSOPolicy) AppliesToDomain(domain string) bool {
	if p == nil || len(p.Domains) == 0 {
		return true
	}
	return p.ListsDomain(domain)
}

// ListsDomain reports whether domain is one of the tenant's domains.
func (p *TenantSSOPolicy) ListsDomain(domain string) bool {
	if p == nil || domain == "" {
		return false
	}
	for _, d := range p.Domains {
		if d == domain {
			return true
		}
	}
	return false
}

// InvitationAdmitsDomain reports whether an invitation admits an address in
// domain: a tenant that requires SSO has no sign-in for an address its
// company sign-ins do not apply to.
func (p *TenantSSOPolicy) InvitationAdmitsDomain(domain string) bool {
	return !p.RequiresSSO() || p.AppliesToDomain(domain)
}

// AutoJoinAdmitsDomain reports whether auto-join admits an address in domain.
func (p *TenantSSOPolicy) AutoJoinAdmitsDomain(domain string) bool {
	return p != nil && p.AutoJoin && p.RequiresSSO() && p.ListsDomain(domain)
}

// MFA requirement values.
const (
	MFARequirementNone     = "none"
	MFARequirementRequired = "required"
)

// TenantMFAPolicy is a tenant's MFA policy.
type TenantMFAPolicy struct {
	TenantID    string
	Requirement string
}

// SignInContext is what a sign-in into a tenant needs to know about an
// address or an account.
type SignInContext struct {
	Member           bool
	Enforcement      string
	ConnectionIDs    []string
	AutoJoinAdmits   bool
	InvitationAdmits bool
	MFARequirement   string
	AccountExists    bool
}

// Invitation is the outcome of an invitation: a recovery link and code for a
// new account, Pending when the membership waits for the user to sign in to
// the tenant, empty for someone who is a member already.
type Invitation struct {
	Link    string
	Code    string
	Pending bool
}

type Membership struct {
	ID               string    `db:"id"`
	TenantID         string    `db:"tenant_id"`
	KratosIdentityID string    `db:"kratos_identity_id"`
	CreatedAt        time.Time `db:"created_at"`
}

type TenantUser struct {
	UserID string
	Email  string
}

// ListOptions holds pagination and filter parameters for List* operations.
type ListOptions struct {
	PageToken string
	PageSize  int32

	// Tenant filters
	Enabled *bool // nil = no filter

	// Membership filters
	IdentityID string // "" = no filter; resolved from email in service layer
	Email      string // "" = no filter; resolved to IdentityID in service layer before storage call
}

// ListOption is a functional option for configuring ListOptions.
type ListOption func(*ListOptions)

// WithPageToken sets the pagination cursor token.
func WithPageToken(token string) ListOption {
	return func(o *ListOptions) {
		o.PageToken = token
	}
}

// WithPageSize sets the number of items per page.
func WithPageSize(size int32) ListOption {
	return func(o *ListOptions) {
		o.PageSize = size
	}
}

// WithEnabled filters by tenant enabled status.
func WithEnabled(v bool) ListOption {
	return func(o *ListOptions) {
		o.Enabled = &v
	}
}

// WithIdentityID filters memberships by Kratos identity ID.
func WithIdentityID(id string) ListOption {
	return func(o *ListOptions) {
		o.IdentityID = id
	}
}

// WithEmail filters memberships by user email. Resolved to IdentityID in the service layer.
func WithEmail(email string) ListOption {
	return func(o *ListOptions) {
		o.Email = email
	}
}

// ApplyOptions materialises a slice of ListOption into a ListOptions struct.
func ApplyOptions(opts ...ListOption) ListOptions {
	var o ListOptions
	for _, opt := range opts {
		opt(&o)
	}
	return o
}

// WithListOptions returns a single ListOption that replaces the destination with o.
// It is used to pass a pre-built, mutated ListOptions back into a variadic interface.
func WithListOptions(o ListOptions) ListOption {
	return func(dst *ListOptions) { *dst = o }
}

// ResolvePageSize returns the effective page size. If PageSize is <= 0 the default
// page size is returned; if it exceeds maxPageSize it is clamped to maxPageSize.
func (o ListOptions) ResolvePageSize() uint64 {
	if o.PageSize <= 0 {
		return uint64(defaultPageSize)
	}
	if o.PageSize > maxPageSize {
		return uint64(maxPageSize)
	}
	return uint64(o.PageSize)
}

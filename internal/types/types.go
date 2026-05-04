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

type Tenant struct {
	ID        string    `db:"id"`
	Name      string    `db:"name"`
	CreatedAt time.Time `db:"created_at"`
	Enabled   bool      `db:"enabled"`
}

type Membership struct {
	ID               string    `db:"id"`
	TenantID         string    `db:"tenant_id"`
	KratosIdentityID string    `db:"kratos_identity_id"`
	Role             string    `db:"role"`
	CreatedAt        time.Time `db:"created_at"`
}

type TenantUser struct {
	UserID string
	Email  string
	Role   string
}

// ListOptions holds pagination and filter parameters for List* operations.
type ListOptions struct {
	PageToken string
	PageSize  int32

	// Tenant filters
	Enabled *bool // nil = no filter

	// Membership filters
	Role       string // "" = no filter; exact match
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

// WithRole filters memberships by exact role match.
func WithRole(role string) ListOption {
	return func(o *ListOptions) {
		o.Role = role
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

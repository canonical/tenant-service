// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0

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

// ListOptions holds pagination parameters for List* operations.
// TODO: add Enabled *bool and Role string filter fields once issue #12 follow-up work is done
// (migrating showDisabled from ListActiveTenantsByUserID requires changes across pkg/webhooks).
type ListOptions struct {
	PageToken string
	PageSize  int32
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

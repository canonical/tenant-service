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

// ResolvePageSize returns the effective page size, clamped between 1 and maxPageSize.
func (o ListOptions) ResolvePageSize() uint64 {
	if o.PageSize <= 0 {
		return uint64(defaultPageSize)
	}
	if o.PageSize > maxPageSize {
		return uint64(maxPageSize)
	}
	return uint64(o.PageSize)
}

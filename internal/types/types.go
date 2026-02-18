// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0

package types

import (
	"time"
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

type Invite struct {
	ID        string    `db:"id"`
	Token     string    `db:"token"`
	TenantID  string    `db:"tenant_id"`
	Email     string    `db:"email"`
	Role      string    `db:"role"`
	CreatedAt time.Time `db:"created_at"`
}

type TenantUser struct {
	UserID string
	Email  string
	Role   string
}

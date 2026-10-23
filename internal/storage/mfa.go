// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	sq "github.com/Masterminds/squirrel"
)

// UpdateTenantMFARequirement returns the value it replaced.
func (s *Storage) UpdateTenantMFARequirement(ctx context.Context, tenantID, requirement string) (before string, err error) {
	defer func(start time.Time) { s.recordLatencyFor("UpdateTenantMFARequirement", start, err) }(time.Now())
	ctx, span := s.tracer.Start(ctx, "storage.UpdateTenantMFARequirement")
	defer span.End()

	// The subquery locks the row and reads the value the update replaces, so
	// the value returned is exact under concurrent writes.
	old := sq.Select("id", "mfa_requirement").
		From("tenants").
		Where(sq.Eq{"id": tenantID}).
		Suffix("FOR NO KEY UPDATE")
	err = s.db.Statement(ctx).
		Update("tenants t").
		Set("mfa_requirement", requirement).
		FromSelect(old, "old").
		Where("t.id = old.id").
		Suffix("RETURNING old.mfa_requirement").
		QueryRowContext(ctx).
		Scan(&before)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrNotFound
		}
		return "", fmt.Errorf("failed to update tenant mfa requirement: %w", err)
	}

	return before, nil
}

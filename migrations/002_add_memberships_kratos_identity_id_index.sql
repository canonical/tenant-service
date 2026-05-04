--  Copyright 2026 Canonical Ltd.
--  SPDX-License-Identifier: AGPL-3.0

-- +goose Up
-- +goose StatementBegin

-- Add a standalone index on kratos_identity_id to support efficient lookups
-- in ListTenantsByUserID, which filters on m.kratos_identity_id = ?
-- The existing UNIQUE(tenant_id, kratos_identity_id) composite index cannot
-- be used as a leading-column index for this predicate.
CREATE INDEX idx_memberships_kratos_identity_id ON memberships (kratos_identity_id);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP INDEX IF EXISTS idx_memberships_kratos_identity_id;

-- +goose StatementEnd

--  Copyright 2026 Canonical Ltd.
--  SPDX-License-Identifier: AGPL-3.0-only

-- +goose Up
-- +goose StatementBegin

-- Membership permissions are owned by the authorization service; the tenant
-- service no longer tracks a per-membership role.
ALTER TABLE memberships DROP COLUMN IF EXISTS role;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

-- Previous role values cannot be recovered; existing rows are restored as 'member'.
ALTER TABLE memberships
    ADD COLUMN role VARCHAR(50) NOT NULL DEFAULT 'member'
    CHECK (role IN ('owner', 'admin', 'member'));

-- +goose StatementEnd

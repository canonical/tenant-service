--  Copyright 2026 Canonical Ltd.
--  SPDX-License-Identifier: AGPL-3.0

-- +goose Up
-- +goose StatementBegin

-- Rename the column to reflect its broader purpose (not just Kratos identities).
ALTER TABLE memberships RENAME COLUMN kratos_identity_id TO identity_id;

-- Relax the type from UUID to TEXT to accommodate Hydra client IDs.
ALTER TABLE memberships ALTER COLUMN identity_id TYPE TEXT;

-- Add a discriminator column to distinguish human users from OAuth2 clients.
ALTER TABLE memberships ADD COLUMN identity_type VARCHAR(10) NOT NULL DEFAULT 'user';
ALTER TABLE memberships ADD CONSTRAINT chk_identity_type CHECK (identity_type IN ('user', 'client'));

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

ALTER TABLE memberships DROP CONSTRAINT IF EXISTS chk_identity_type;
ALTER TABLE memberships DROP COLUMN IF EXISTS identity_type;
ALTER TABLE memberships ALTER COLUMN identity_id TYPE UUID USING identity_id::uuid;
ALTER TABLE memberships RENAME COLUMN identity_id TO kratos_identity_id;

-- +goose StatementEnd

--  Copyright 2026 Canonical Ltd.
--  SPDX-License-Identifier: AGPL-3.0-only

-- +goose Up
-- +goose StatementBegin

-- Whether signing in to the tenant needs MFA.
ALTER TABLE tenants
    ADD COLUMN mfa_requirement TEXT NOT NULL DEFAULT 'none'
        CONSTRAINT tenants_mfa_requirement_check CHECK (mfa_requirement IN ('none', 'required'));

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

ALTER TABLE tenants DROP COLUMN IF EXISTS mfa_requirement;

-- +goose StatementEnd

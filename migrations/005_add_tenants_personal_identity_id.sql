--  Copyright 2026 Canonical Ltd.
--  SPDX-License-Identifier: AGPL-3.0-only

-- +goose Up
-- +goose StatementBegin

-- The account whose personal tenant this is. UNIQUE keeps it to one per
-- account, also when two requests create it at the same time.
ALTER TABLE tenants ADD COLUMN personal_identity_id UUID UNIQUE;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

ALTER TABLE tenants DROP COLUMN IF EXISTS personal_identity_id;

-- +goose StatementEnd

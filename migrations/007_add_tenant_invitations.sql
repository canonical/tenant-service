--  Copyright 2026 Canonical Ltd.
--  SPDX-License-Identifier: AGPL-3.0-only

-- +goose Up
-- +goose StatementBegin

-- An address invited to a tenant it is not a member of yet. An expired
-- invitation is ignored, not deleted.
CREATE TABLE tenant_invitations (
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    email TEXT NOT NULL CONSTRAINT tenant_invitations_email_check CHECK (email = lower(email)),
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMP WITH TIME ZONE NOT NULL,

    PRIMARY KEY(tenant_id, email)
);

-- Supports the lookup of the tenants an address is invited to.
CREATE INDEX idx_tenant_invitations_email ON tenant_invitations (email);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP TABLE IF EXISTS tenant_invitations;

-- +goose StatementEnd

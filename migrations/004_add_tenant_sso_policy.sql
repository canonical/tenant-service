--  Copyright 2026 Canonical Ltd.
--  SPDX-License-Identifier: AGPL-3.0-only

-- +goose Up
-- +goose StatementBegin

-- A tenant's SSO policy: its enforcement and auto-join are columns of the
-- tenant, its bindings and its domains are rows. A tenant with no active
-- binding is off, whatever is stored.
ALTER TABLE tenants
    ADD COLUMN sso_enforcement TEXT NOT NULL DEFAULT 'off'
        CONSTRAINT tenants_sso_enforcement_check CHECK (sso_enforcement IN ('off', 'optional', 'required')),
    ADD COLUMN sso_auto_join BOOLEAN NOT NULL DEFAULT FALSE,
    -- Auto-join only where company sign-in is required.
    ADD CONSTRAINT tenants_sso_auto_join_check CHECK (NOT sso_auto_join OR sso_enforcement = 'required');

CREATE TABLE tenant_sso_bindings (
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    -- The id of a connection of the SSO service: no foreign key.
    connection_id UUID NOT NULL,
    active BOOLEAN NOT NULL DEFAULT FALSE,

    PRIMARY KEY(tenant_id, connection_id),
    -- A connection is bound by one tenant only.
    UNIQUE(connection_id)
);

-- One domain may be on several tenants.
CREATE TABLE tenant_sso_domains (
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    domain TEXT NOT NULL CONSTRAINT tenant_sso_domains_domain_check CHECK (domain = lower(domain)),

    PRIMARY KEY(tenant_id, domain)
);

-- Supports the lookup of the tenants that auto-join the domain of an address.
CREATE INDEX idx_tenant_sso_domains_domain ON tenant_sso_domains (domain);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP TABLE IF EXISTS tenant_sso_domains;
DROP TABLE IF EXISTS tenant_sso_bindings;
ALTER TABLE tenants
    DROP COLUMN IF EXISTS sso_auto_join,
    DROP COLUMN IF EXISTS sso_enforcement;

-- +goose StatementEnd

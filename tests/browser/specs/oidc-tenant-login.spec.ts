// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * OIDC (social) login + multi-tenancy — browser E2E tests.
 *
 * Prerequisites:
 *   - Full dev stack running (Kratos, Hydra, Traefik, OpenFGA, Postgres, Dex)
 *   - identity-platform-login-ui running with MULTI_TENANCY_ENABLED=TRUE, MFA_ENABLED=TRUE
 *   - OIDC consumer running on :4446
 *   - Dex running on :5556
 *   - tenant-service running on :8000
 *
 * Environment variables:
 *   AUTH_CLIENT_ID      — Hydra client_credentials client_id
 *   AUTH_CLIENT_SECRET  — corresponding client_secret
 *
 * Flow: OIDC login via Dex creates an identity, which is then provisioned
 * into tenants for subsequent login flows that exercise tenant selection.
 */

import { test, expect } from "@playwright/test";
import {
  deleteIdentity,
  deleteIdentitySessions,
  findIdentityByEmail,
} from "../helpers/kratos";
import { enterEmail } from "../helpers/login";
import {
  createTenant,
  deleteTenant,
  getServiceToken,
  provisionUser,
} from "../helpers/tenants";
import { startOIDCFlow, expectOIDCFlowComplete } from "../helpers/oidc";
import { startOIDCFlowWithParams } from "../helpers/hydra";
import {
  clickDexLoginButton,
  loginWithDex,
  registerDexIdentity,
  DEX_USER_EMAIL,
} from "../helpers/dex";

function requireEnv(name: string): string {
  const v = process.env[name];
  if (!v) throw new Error(`missing required env var: ${name}`);
  return v;
}

// ---------------------------------------------------------------------------
// Shared state per test
// ---------------------------------------------------------------------------

interface TestState {
  token: string;
  identityIds: string[];
  tenantIds: string[];
}

let state: TestState;

test.beforeEach(async () => {
  const clientId = requireEnv("AUTH_CLIENT_ID");
  const clientSecret = requireEnv("AUTH_CLIENT_SECRET");
  const token = await getServiceToken(clientId, clientSecret);
  state = { token, identityIds: [], tenantIds: [] };

  // Clean up any leftover identity from a previous test run
  const existingId = await findIdentityByEmail(DEX_USER_EMAIL);
  if (existingId) {
    await deleteIdentitySessions(existingId).catch(() => {});
    await deleteIdentity(existingId).catch(() => {});
  }

  // Register the identity with OIDC credentials so the identifier-first
  // 1FA page shows the "Sign in with Dex" button.
  const id = await registerDexIdentity();
  state.identityIds.push(id);
});

test.afterEach(async () => {
  for (const id of state.identityIds) {
    await deleteIdentitySessions(id).catch(() => {});
    await deleteIdentity(id).catch(() => {});
  }
  for (const id of state.tenantIds) {
    await deleteTenant(state.token, id).catch(() => {});
  }
});

/** Create a tenant and track it for cleanup. */
async function addTenant(name: string): Promise<string> {
  const t = await createTenant(state.token, name);
  state.tenantIds.push(t.id);
  return t.id;
}

/** Create a tenant and provision the Dex test user into it. */
async function addTenantWithDexUser(tenantName: string): Promise<string> {
  const tenantId = await addTenant(tenantName);
  await provisionUser(state.token, tenantId, DEX_USER_EMAIL);
  return tenantId;
}

/**
 * Perform a full first-time OIDC login via Dex.
 * MFA is not enforced for OIDC logins, so the flow completes directly.
 */
async function completeFirstOidcLogin(
  page: import("@playwright/test").Page,
): Promise<void> {
  await startOIDCFlow(page);
  await enterEmail(page, DEX_USER_EMAIL);
  await clickDexLoginButton(page);
  await loginWithDex(page);
}

// ---------------------------------------------------------------------------
// 1. OIDC + multi-tenancy login flows
// ---------------------------------------------------------------------------

test.describe("OIDC + multi-tenancy login flows", () => {
  test("zero-tenant OIDC user completes without tenant selection", async ({
    page,
  }) => {
    await completeFirstOidcLogin(page);
    const tokens = await expectOIDCFlowComplete(page);

    expect(tokens.accessTokenClaims.tenant_id).toBeUndefined();
    expect(tokens.idTokenClaims.tenant_id).toBeUndefined();
  });

  test("single-tenant OIDC user skips tenant selection", async ({ page }) => {
    // First: OIDC login to create the identity
    await completeFirstOidcLogin(page);
    await expectOIDCFlowComplete(page);

    // Provision into one tenant
    const tenantId = await addTenantWithDexUser("OIDC Single Corp");

    // Delete sessions for a fresh login
    const identityId = await findIdentityByEmail(DEX_USER_EMAIL);
    if (identityId) await deleteIdentitySessions(identityId);

    // Second OIDC login — single tenant auto-selected (no MFA for OIDC)
    await startOIDCFlow(page);
    await enterEmail(page, DEX_USER_EMAIL);
    await clickDexLoginButton(page);
    await loginWithDex(page);
    const tokens = await expectOIDCFlowComplete(page);

    expect(tokens.accessTokenClaims.tenant_id).toBe(tenantId);
    expect(tokens.idTokenClaims.tenant_id).toBe(tenantId);
  });

  test("multi-tenant OIDC user sees tenant selection", async ({ page }) => {
    // First: OIDC login to create the identity
    await completeFirstOidcLogin(page);
    await expectOIDCFlowComplete(page);

    // Provision into multiple tenants
    const alphaId = await addTenantWithDexUser("OIDC Alpha Inc");
    await addTenantWithDexUser("OIDC Beta LLC");

    // Delete sessions for a fresh login
    const identityId = await findIdentityByEmail(DEX_USER_EMAIL);
    if (identityId) await deleteIdentitySessions(identityId);

    // Second OIDC login — should show tenant selection (no MFA for OIDC)
    // After entering email, the identifier-first handler checks tenants
    // via NeedsTenantSelectionByEmail and redirects to tenant selection
    // BEFORE the 1FA page is shown (per the sequence diagrams).
    await startOIDCFlow(page);
    await enterEmail(page, DEX_USER_EMAIL);

    // Tenant selection appears before the OIDC button
    await expect(page.getByText("Select a tenant")).toBeVisible();
    await expect(
      page.getByRole("button", { name: "OIDC Alpha Inc" }),
    ).toBeVisible();
    await expect(
      page.getByRole("button", { name: "OIDC Beta LLC" }),
    ).toBeVisible();

    await page.getByRole("button", { name: "OIDC Alpha Inc" }).click();

    // After tenant selection, the 1FA page appears with the Dex button
    await clickDexLoginButton(page);
    await loginWithDex(page);

    const tokens = await expectOIDCFlowComplete(page);

    expect(tokens.accessTokenClaims.tenant_id).toBe(alphaId);
    expect(tokens.idTokenClaims.tenant_id).toBe(alphaId);
  });
});

// ---------------------------------------------------------------------------
// 2. Session reuse (no max_age) — existing Kratos session should be reused
// ---------------------------------------------------------------------------

test.describe("session reuse (no max_age)", () => {
  test("multi-tenant OIDC user: second login reuses session and shows tenant selection", async ({
    page,
  }) => {
    // First: OIDC login to create the identity
    await completeFirstOidcLogin(page);
    await expectOIDCFlowComplete(page);

    // Provision into multiple tenants
    const alphaId = await addTenantWithDexUser("OIDC Reuse Alpha");
    const betaId = await addTenantWithDexUser("OIDC Reuse Beta");

    // Second login — session exists, no max_age → auth is skipped
    // but multi-tenant users must still select a tenant.
    await startOIDCFlowWithParams(page, {});
    await expect(page.getByText("Select a tenant")).toBeVisible();
    await expect(
      page.getByRole("button", { name: "OIDC Reuse Alpha" }),
    ).toBeVisible();
    await expect(
      page.getByRole("button", { name: "OIDC Reuse Beta" }),
    ).toBeVisible();

    await page.getByRole("button", { name: "OIDC Reuse Beta" }).click();
    const tokens = await expectOIDCFlowComplete(page);

    expect(tokens.accessTokenClaims.tenant_id).toBe(betaId);
    expect(tokens.idTokenClaims.tenant_id).toBe(betaId);
  });
});

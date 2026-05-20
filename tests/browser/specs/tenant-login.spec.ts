// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Multi-tenancy login flow — browser E2E tests.
 *
 * Prerequisites:
 *   - Full dev stack running via start.sh (Kratos, Hydra, Traefik, OpenFGA, Postgres)
 *   - identity-platform-login-ui binary running (./app serve) with TENANT_SELECTION_ENABLED=TRUE
 *   - OIDC consumer running on :4446
 *   - tenant-service running on :8000
 *
 * Environment variables (set by start.sh, or pass them manually):
 *   AUTH_CLIENT_ID      — Hydra client_credentials client_id (allowed subject in tenant-service)
 *   AUTH_CLIENT_SECRET  — corresponding client_secret
 *
 * These tests create their own identities and tenants.
 * The login-ui must have MFA_ENABLED=TRUE.
 * Cleanup runs in afterEach so state does not leak between tests.
 */

import { test, expect } from "@playwright/test";
import {
  createIdentity,
  deleteIdentity,
  deleteIdentitySessions,
} from "../helpers/kratos";
import {
  createTenant,
  deleteTenant,
  getServiceToken,
  provisionUser,
} from "../helpers/tenants";
import {
  startOIDCFlow,
  expectOIDCFlowComplete,
} from "../helpers/oidc";
import { loginWithPassword, enterEmail, enterPassword } from "../helpers/login";
import { startOIDCFlowWithParams } from "../helpers/hydra";
import { completeTotpSetup, submitTotpCode } from "../helpers/totp";

const PASSWORD = "Secure-Password-123!";

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
  state = {
    token,
    identityIds: [],
    tenantIds: [],
  };
});

test.afterEach(async () => {
  // Clean up in reverse order: sessions → identities → tenants
  for (const id of state.identityIds) {
    await deleteIdentitySessions(id).catch(() => {});
    await deleteIdentity(id).catch(() => {});
  }
  for (const id of state.tenantIds) {
    await deleteTenant(state.token, id).catch(() => {});
  }
});

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

/** Create a Kratos identity and track it for cleanup. */
async function addIdentity(email: string): Promise<string> {
  const id = await createIdentity({ email, password: PASSWORD });
  state.identityIds.push(id);
  return id;
}

/** Create a tenant and track it for cleanup. */
async function addTenant(name: string): Promise<string> {
  const t = await createTenant(state.token, name);
  state.tenantIds.push(t.id);
  return t.id;
}

/** Create a tenant and provision a user into it. */
async function addTenantWithUser(
  tenantName: string,
  email: string,
): Promise<string> {
  const tenantId = await addTenant(tenantName);
  await provisionUser(state.token, tenantId, email);
  return tenantId;
}

/** Complete a full OIDC login flow for an identity (password + TOTP setup). */
async function completeFullLogin(
  page: import("@playwright/test").Page,
  email: string,
): Promise<string> {
  await startOIDCFlow(page);
  await loginWithPassword(page, email, PASSWORD);
  return completeTotpSetup(page);
}

// ---------------------------------------------------------------------------
// 1. Multi-tenancy login flows (first login, no existing session)
// ---------------------------------------------------------------------------

test.describe("multi-tenancy login flows", () => {
  test("single-tenant user skips tenant selection", async ({ page }) => {
    const email = `single-${Date.now()}@test.example`;
    await addIdentity(email);
    const tenantId = await addTenantWithUser("Single Tenant Corp", email);

    await startOIDCFlow(page);
    await loginWithPassword(page, email, PASSWORD);
    await completeTotpSetup(page);

    // Single tenant → auto-selected, no tenant selection page visible.
    // The flow should complete and redirect to the OIDC callback.
    const tokens = await expectOIDCFlowComplete(page);

    // Token must contain the auto-selected tenant_id
    expect(tokens.accessTokenClaims.tenant_id).toBe(tenantId);
    expect(tokens.idTokenClaims.tenant_id).toBe(tenantId);
  });

  test("multi-tenant user sees tenant selection and picks one", async ({
    page,
  }) => {
    const email = `multi-${Date.now()}@test.example`;
    await addIdentity(email);
    const alphaId = await addTenantWithUser("Alpha Inc", email);
    await addTenantWithUser("Beta LLC", email);

    await startOIDCFlow(page);
    await enterEmail(page, email);

    // Tenant selection should appear BEFORE password entry
    await expect(page.getByText("Select a tenant")).toBeVisible();

    // Both tenants should be listed
    await expect(
      page.getByRole("button", { name: "Alpha Inc" }),
    ).toBeVisible();
    await expect(
      page.getByRole("button", { name: "Beta LLC" }),
    ).toBeVisible();

    // Pick one
    await page.getByRole("button", { name: "Alpha Inc" }).click();

    // Now enter password (1FA)
    await enterPassword(page, PASSWORD);
    await completeTotpSetup(page);

    // Flow should complete with the selected tenant in the token
    const tokens = await expectOIDCFlowComplete(page);
    expect(tokens.accessTokenClaims.tenant_id).toBe(alphaId);
    expect(tokens.idTokenClaims.tenant_id).toBe(alphaId);
  });

  test("zero-tenant user completes login without tenant selection", async ({
    page,
  }) => {
    const email = `zero-${Date.now()}@test.example`;
    await addIdentity(email);
    // No tenants provisioned for this user

    await startOIDCFlow(page);
    await loginWithPassword(page, email, PASSWORD);
    await completeTotpSetup(page);

    // Zero tenants → auto-submits empty tenant, completes login
    const tokens = await expectOIDCFlowComplete(page);

    // No tenant → no tenant_id claim in tokens
    expect(tokens.accessTokenClaims.tenant_id).toBeUndefined();
    expect(tokens.idTokenClaims.tenant_id).toBeUndefined();
  });

  test("multi-tenant user can pick the second tenant", async ({ page }) => {
    const email = `pick2-${Date.now()}@test.example`;
    await addIdentity(email);
    await addTenantWithUser("First Org", email);
    const secondId = await addTenantWithUser("Second Org", email);

    await startOIDCFlow(page);
    await enterEmail(page, email);

    await expect(page.getByText("Select a tenant")).toBeVisible();

    // Pick the second tenant
    await page.getByRole("button", { name: "Second Org" }).click();

    // Now enter password
    await enterPassword(page, PASSWORD);
    await completeTotpSetup(page);

    // Token must contain the picked (second) tenant
    const tokens = await expectOIDCFlowComplete(page);
    expect(tokens.accessTokenClaims.tenant_id).toBe(secondId);
    expect(tokens.idTokenClaims.tenant_id).toBe(secondId);
  });
});

// ---------------------------------------------------------------------------
// 2. Session reuse (no max_age) — existing Kratos session should be reused
// ---------------------------------------------------------------------------

test.describe("session reuse (no max_age)", () => {
  test("zero-tenant user: second login reuses session", async ({ page }) => {
    const email = `reuse-zero-${Date.now()}@test.example`;
    await addIdentity(email);

    // First login — full flow
    await completeFullLogin(page, email);
    await expectOIDCFlowComplete(page);

    // Second login — session exists, no max_age → auto-complete
    await startOIDCFlowWithParams(page, {});
    await expectOIDCFlowComplete(page);
  });

  test("single-tenant user: second login reuses session", async ({
    page,
  }) => {
    const email = `reuse-single-${Date.now()}@test.example`;
    await addIdentity(email);
    await addTenantWithUser("Reuse Corp", email);

    await completeFullLogin(page, email);
    await expectOIDCFlowComplete(page);

    // Second login — session exists, single tenant auto-selected
    await startOIDCFlowWithParams(page, {});
    await expectOIDCFlowComplete(page);
  });

  test("multi-tenant user: second login reuses session", async ({ page }) => {
    const email = `reuse-multi-${Date.now()}@test.example`;
    await addIdentity(email);
    const alphaId = await addTenantWithUser("Reuse Alpha", email);
    const betaId = await addTenantWithUser("Reuse Beta", email);

    // First login: email → tenant selection → password → TOTP setup
    await startOIDCFlow(page);
    await enterEmail(page, email);
    await expect(page.getByText("Select a tenant")).toBeVisible();
    await page.getByRole("button", { name: "Reuse Alpha" }).click();
    await enterPassword(page, PASSWORD);
    await completeTotpSetup(page);
    const firstTokens = await expectOIDCFlowComplete(page);
    expect(firstTokens.accessTokenClaims.tenant_id).toBe(alphaId);

    // Second login — session exists, auth is skipped but multi-tenant
    // users must still select a tenant for every OIDC flow.
    await startOIDCFlowWithParams(page, {});
    await expect(page.getByText("Select a tenant")).toBeVisible();
    await page.getByRole("button", { name: "Reuse Beta" }).click();
    const secondTokens = await expectOIDCFlowComplete(page);

    // Second flow picked a different tenant → token reflects that
    expect(secondTokens.accessTokenClaims.tenant_id).toBe(betaId);
    expect(secondTokens.idTokenClaims.tenant_id).toBe(betaId);
  });
});

// ---------------------------------------------------------------------------
// 3. Forced re-authentication (max_age=0)
// ---------------------------------------------------------------------------

test.describe("forced re-authentication (max_age=0)", () => {
  test("zero-tenant user: max_age=0 forces login after existing session", async ({
    page,
  }) => {
    const email = `reauth-zero-${Date.now()}@test.example`;
    await addIdentity(email);

    // First login — establish session (password + TOTP setup)
    const secret = await completeFullLogin(page, email);
    await expectOIDCFlowComplete(page);

    // Second login with max_age=0 — must re-authenticate (password + TOTP)
    await startOIDCFlowWithParams(page, { max_age: "0" });
    await expect(page.getByText("Sign in")).toBeVisible();
    await loginWithPassword(page, email, PASSWORD);
    await submitTotpCode(page, secret);
    await expectOIDCFlowComplete(page);
  });

  test("single-tenant user: max_age=0 forces login after existing session", async ({
    page,
  }) => {
    const email = `reauth-single-${Date.now()}@test.example`;
    await addIdentity(email);
    await addTenantWithUser("ReAuth Corp", email);

    const secret = await completeFullLogin(page, email);
    await expectOIDCFlowComplete(page);

    await startOIDCFlowWithParams(page, { max_age: "0" });
    await expect(page.getByText("Sign in")).toBeVisible();
    await loginWithPassword(page, email, PASSWORD);
    await submitTotpCode(page, secret);
    // Single tenant → auto-selected after re-auth
    await expectOIDCFlowComplete(page);
  });

  test("multi-tenant user: max_age=0 forces login and tenant re-selection", async ({
    page,
  }) => {
    const email = `reauth-multi-${Date.now()}@test.example`;
    await addIdentity(email);
    const alphaId = await addTenantWithUser("ReAuth Alpha", email);
    const betaId = await addTenantWithUser("ReAuth Beta", email);

    // First login: email → tenant selection → password → TOTP setup
    await startOIDCFlow(page);
    await enterEmail(page, email);
    await expect(page.getByText("Select a tenant")).toBeVisible();
    await page.getByRole("button", { name: "ReAuth Alpha" }).click();
    await enterPassword(page, PASSWORD);
    const secret = await completeTotpSetup(page);
    const firstTokens = await expectOIDCFlowComplete(page);
    expect(firstTokens.accessTokenClaims.tenant_id).toBe(alphaId);

    // Second login with max_age=0 — must re-authenticate AND re-select tenant
    await startOIDCFlowWithParams(page, { max_age: "0" });
    await expect(page.getByText("Sign in")).toBeVisible();
    await enterEmail(page, email);
    await expect(page.getByText("Select a tenant")).toBeVisible();
    await page.getByRole("button", { name: "ReAuth Beta" }).click();
    await enterPassword(page, PASSWORD);
    await submitTotpCode(page, secret);
    const secondTokens = await expectOIDCFlowComplete(page);

    // Re-auth picked a different tenant → token reflects the new selection
    expect(secondTokens.accessTokenClaims.tenant_id).toBe(betaId);
    expect(secondTokens.idTokenClaims.tenant_id).toBe(betaId);
  });
});

// ---------------------------------------------------------------------------
// 4. MFA enforcement (requires login-ui with MFA_ENABLED=TRUE)
// ---------------------------------------------------------------------------

test.describe("MFA enforcement", () => {

  test("zero-tenant user with TOTP: MFA prompt during login", async ({
    page,
  }) => {
    const email = `mfa-zero-${Date.now()}@test.example`;
    const identityId = await addIdentity(email);

    // First login — password then TOTP setup
    await startOIDCFlow(page);
    await loginWithPassword(page, email, PASSWORD);
    const secret = await completeTotpSetup(page);
    await expectOIDCFlowComplete(page);

    // Delete sessions so we get a fresh login
    await deleteIdentitySessions(identityId);

    // Second login — should prompt for TOTP after password
    await startOIDCFlow(page);
    await loginWithPassword(page, email, PASSWORD);
    await submitTotpCode(page, secret);
    await expectOIDCFlowComplete(page);
  });

  test("single-tenant user with TOTP: MFA then auto-select tenant", async ({
    page,
  }) => {
    const email = `mfa-single-${Date.now()}@test.example`;
    const identityId = await addIdentity(email);
    await addTenantWithUser("MFA Corp", email);

    // First login — password then TOTP setup
    await startOIDCFlow(page);
    await loginWithPassword(page, email, PASSWORD);
    const secret = await completeTotpSetup(page);
    // Single tenant → auto-select, flow completes
    await expectOIDCFlowComplete(page);

    await deleteIdentitySessions(identityId);

    // Second login with existing TOTP
    await startOIDCFlow(page);
    await loginWithPassword(page, email, PASSWORD);
    await submitTotpCode(page, secret);
    // Single tenant → auto-selected
    await expectOIDCFlowComplete(page);
  });

  test("multi-tenant user with TOTP: tenant selection then MFA", async ({
    page,
  }) => {
    const email = `mfa-multi-${Date.now()}@test.example`;
    const identityId = await addIdentity(email);
    await addTenantWithUser("MFA Alpha", email);
    await addTenantWithUser("MFA Beta", email);

    // First login: email → tenant selection → password → TOTP setup
    await startOIDCFlow(page);
    await enterEmail(page, email);
    await expect(page.getByText("Select a tenant")).toBeVisible();
    await page.getByRole("button", { name: "MFA Alpha" }).click();
    await enterPassword(page, PASSWORD);
    const secret = await completeTotpSetup(page);
    await expectOIDCFlowComplete(page);

    await deleteIdentitySessions(identityId);

    // Second login: email → tenant selection → password → TOTP
    await startOIDCFlow(page);
    await enterEmail(page, email);
    await expect(page.getByText("Select a tenant")).toBeVisible();
    await page.getByRole("button", { name: "MFA Beta" }).click();
    await enterPassword(page, PASSWORD);
    await submitTotpCode(page, secret);
    await expectOIDCFlowComplete(page);
  });

  test("multi-tenant user: max_age=0 forces re-auth including MFA", async ({
    page,
  }) => {
    const email = `mfa-reauth-${Date.now()}@test.example`;
    const identityId = await addIdentity(email);
    await addTenantWithUser("MFA ReAuth Alpha", email);
    await addTenantWithUser("MFA ReAuth Beta", email);

    // First login: email → tenant selection → password → TOTP setup
    await startOIDCFlow(page);
    await enterEmail(page, email);
    await expect(page.getByText("Select a tenant")).toBeVisible();
    await page.getByRole("button", { name: "MFA ReAuth Alpha" }).click();
    await enterPassword(page, PASSWORD);
    const secret = await completeTotpSetup(page);
    await expectOIDCFlowComplete(page);

    // Second login with max_age=0: email → tenant → password → TOTP
    await startOIDCFlowWithParams(page, { max_age: "0" });
    await expect(page.getByText("Sign in")).toBeVisible();
    await enterEmail(page, email);
    await expect(page.getByText("Select a tenant")).toBeVisible();
    await page.getByRole("button", { name: "MFA ReAuth Beta" }).click();
    await enterPassword(page, PASSWORD);
    await submitTotpCode(page, secret);
    await expectOIDCFlowComplete(page);
  });
});

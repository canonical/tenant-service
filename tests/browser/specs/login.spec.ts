// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0

/**
 * Standard login flow — browser E2E tests (multi-tenancy OFF).
 *
 * Prerequisites:
 *   - Full dev stack running (Kratos, Hydra, Traefik, OpenFGA, Postgres)
 *   - identity-platform-login-ui running with MULTI_TENANCY_ENABLED=FALSE, MFA_ENABLED=TRUE
 *   - OIDC consumer running on :4446
 *
 * These tests create their own identities and clean up after each test.
 */

import { test, expect } from "@playwright/test";
import {
  createIdentity,
  deleteIdentity,
  deleteIdentitySessions,
} from "../helpers/kratos";
import { startOIDCFlow, expectOIDCFlowComplete } from "../helpers/oidc";
import { loginWithPassword } from "../helpers/login";
import { startOIDCFlowWithParams } from "../helpers/hydra";
import { completeTotpSetup, submitTotpCode } from "../helpers/totp";

const PASSWORD = "Secure-Password-123!";

// ---------------------------------------------------------------------------
// Shared state per test
// ---------------------------------------------------------------------------

let identityIds: string[];

test.beforeEach(async () => {
  identityIds = [];
});

test.afterEach(async () => {
  for (const id of identityIds) {
    await deleteIdentitySessions(id).catch(() => {});
    await deleteIdentity(id).catch(() => {});
  }
});

/** Create a Kratos identity and track it for cleanup. */
async function addIdentity(email: string): Promise<string> {
  const id = await createIdentity({ email, password: PASSWORD });
  identityIds.push(id);
  return id;
}

/** Complete a full OIDC login flow (password + TOTP setup). */
async function completeFullLogin(
  page: import("@playwright/test").Page,
  email: string,
): Promise<string> {
  await startOIDCFlow(page);
  await loginWithPassword(page, email, PASSWORD);
  return completeTotpSetup(page);
}

// ---------------------------------------------------------------------------
// 1. Basic login flows
// ---------------------------------------------------------------------------

test.describe("basic login flows", () => {
  test("user completes email + password + TOTP setup login", async ({
    page,
  }) => {
    const email = `basic-${Date.now()}@test.example`;
    await addIdentity(email);

    await startOIDCFlow(page);
    await loginWithPassword(page, email, PASSWORD);
    await completeTotpSetup(page);
    const tokens = await expectOIDCFlowComplete(page);

    // Multi-tenancy is OFF → no tenant_id in tokens
    expect(tokens.accessTokenClaims.tenant_id).toBeUndefined();
    expect(tokens.idTokenClaims.tenant_id).toBeUndefined();
  });
});

// ---------------------------------------------------------------------------
// 2. Session reuse (no max_age)
// ---------------------------------------------------------------------------

test.describe("session reuse (no max_age)", () => {
  test("second login reuses session", async ({ page }) => {
    const email = `reuse-${Date.now()}@test.example`;
    await addIdentity(email);

    // First login — full flow (password + TOTP setup)
    await completeFullLogin(page, email);
    await expectOIDCFlowComplete(page);

    // Second login — session exists, no max_age → auto-complete
    await startOIDCFlowWithParams(page, {});
    await expectOIDCFlowComplete(page);
  });
});

// ---------------------------------------------------------------------------
// 3. Forced re-authentication (max_age=0)
// ---------------------------------------------------------------------------

test.describe("forced re-authentication (max_age=0)", () => {
  test("max_age=0 forces login after existing session", async ({ page }) => {
    const email = `reauth-${Date.now()}@test.example`;
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
});

// ---------------------------------------------------------------------------
// 4. MFA enforcement
// ---------------------------------------------------------------------------

test.describe("MFA enforcement", () => {
  test("TOTP setup and verification during login", async ({ page }) => {
    const email = `mfa-${Date.now()}@test.example`;
    const identityId = await addIdentity(email);

    // First login — password then TOTP setup
    await startOIDCFlow(page);
    await loginWithPassword(page, email, PASSWORD);
    const secret = await completeTotpSetup(page);
    await expectOIDCFlowComplete(page);

    // Delete sessions for a fresh login
    await deleteIdentitySessions(identityId);

    // Second login — password then TOTP verification
    await startOIDCFlow(page);
    await loginWithPassword(page, email, PASSWORD);
    await submitTotpCode(page, secret);
    await expectOIDCFlowComplete(page);
  });

  test("session reuse skips MFA", async ({ page }) => {
    const email = `mfa-reuse-${Date.now()}@test.example`;
    await addIdentity(email);

    // First login — password + TOTP setup
    await startOIDCFlow(page);
    await loginWithPassword(page, email, PASSWORD);
    await completeTotpSetup(page);
    await expectOIDCFlowComplete(page);

    // Second login — session exists, no max_age → auto-complete (no MFA)
    await startOIDCFlowWithParams(page, {});
    await expectOIDCFlowComplete(page);
  });

  test("max_age=0 forces re-auth including MFA", async ({ page }) => {
    const email = `mfa-reauth-${Date.now()}@test.example`;
    await addIdentity(email);

    // First login — password + TOTP setup
    await startOIDCFlow(page);
    await loginWithPassword(page, email, PASSWORD);
    const secret = await completeTotpSetup(page);
    await expectOIDCFlowComplete(page);

    // Second login with max_age=0 — full re-auth including MFA
    await startOIDCFlowWithParams(page, { max_age: "0" });
    await expect(page.getByText("Sign in")).toBeVisible();
    await loginWithPassword(page, email, PASSWORD);
    await submitTotpCode(page, secret);
    await expectOIDCFlowComplete(page);
  });
});

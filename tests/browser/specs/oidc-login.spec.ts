// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * OIDC (social) login flow via Dex — browser E2E tests (multi-tenancy OFF).
 *
 * Prerequisites:
 *   - Full dev stack running (Kratos, Hydra, Traefik, OpenFGA, Postgres, Dex)
 *   - identity-platform-login-ui running with MULTI_TENANCY_ENABLED=FALSE, MFA_ENABLED=TRUE
 *   - OIDC consumer running on :4446
 *   - Dex running on :5556
 *
 * These tests use the static Dex user (dex-user@test.example / dex-password).
 * Each test cleans up the Kratos identity created via OIDC registration.
 */

import { test, expect } from "@playwright/test";
import {
  deleteIdentity,
  deleteIdentitySessions,
  findIdentityByEmail,
} from "../helpers/kratos";
import { enterEmail } from "../helpers/login";
import { startOIDCFlow, expectOIDCFlowComplete } from "../helpers/oidc";
import { startOIDCFlowWithParams } from "../helpers/hydra";
import {
  clickDexLoginButton,
  loginWithDex,
  registerDexIdentity,
  DEX_USER_EMAIL,
} from "../helpers/dex";

// ---------------------------------------------------------------------------
// Shared state per test
// ---------------------------------------------------------------------------

let createdIdentityIds: string[];

test.beforeEach(async () => {
  createdIdentityIds = [];

  // Clean up any leftover identity from a previous test run
  const existingId = await findIdentityByEmail(DEX_USER_EMAIL);
  if (existingId) {
    await deleteIdentitySessions(existingId).catch(() => {});
    await deleteIdentity(existingId).catch(() => {});
  }

  // Register the identity with OIDC credentials so the identifier-first
  // 1FA page shows the "Sign in with Dex" button.
  const id = await registerDexIdentity();
  createdIdentityIds.push(id);
});

test.afterEach(async () => {
  for (const id of createdIdentityIds) {
    await deleteIdentitySessions(id).catch(() => {});
    await deleteIdentity(id).catch(() => {});
  }
});

// ---------------------------------------------------------------------------
// 1. Basic OIDC login
// ---------------------------------------------------------------------------

test.describe("OIDC login via Dex", () => {
  test("OIDC login via Dex completes flow", async ({
    page,
  }) => {
    await startOIDCFlow(page);
    await enterEmail(page, DEX_USER_EMAIL);
    await clickDexLoginButton(page);
    await loginWithDex(page);

    // MFA is not enforced for OIDC logins — flow completes directly.
    const tokens = await expectOIDCFlowComplete(page);

    // Multi-tenancy OFF → no tenant_id
    expect(tokens.accessTokenClaims.tenant_id).toBeUndefined();
    expect(tokens.idTokenClaims.tenant_id).toBeUndefined();
  });
});

// ---------------------------------------------------------------------------
// 2. Session reuse (no max_age)
// ---------------------------------------------------------------------------

test.describe("OIDC session reuse (no max_age)", () => {
  test("second login reuses Kratos session", async ({ page }) => {
    // First login — full OIDC flow (no MFA for OIDC)
    await startOIDCFlow(page);
    await enterEmail(page, DEX_USER_EMAIL);
    await clickDexLoginButton(page);
    await loginWithDex(page);
    await expectOIDCFlowComplete(page);

    // Second login — Kratos session exists, no max_age → auto-complete
    await startOIDCFlowWithParams(page, {});
    await expectOIDCFlowComplete(page);
  });
});

// ---------------------------------------------------------------------------
// 3. Forced re-authentication (max_age=0)
// ---------------------------------------------------------------------------

test.describe("OIDC forced re-authentication (max_age=0)", () => {
  test("max_age=0 forces re-auth after OIDC session", async ({ page }) => {
    // First login — full OIDC flow (no MFA for OIDC)
    await startOIDCFlow(page);
    await enterEmail(page, DEX_USER_EMAIL);
    await clickDexLoginButton(page);
    await loginWithDex(page);
    await expectOIDCFlowComplete(page);

    // Second login with max_age=0 — must re-authenticate
    await startOIDCFlowWithParams(page, { max_age: "0" });
    await expect(page.getByRole("heading", { name: "Sign in" })).toBeVisible();

    // Re-auth via Dex again — identifier-first requires email entry
    await enterEmail(page, DEX_USER_EMAIL);
    await clickDexLoginButton(page);
    await loginWithDex(page);
    await expectOIDCFlowComplete(page);
  });
});

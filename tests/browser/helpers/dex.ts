// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Dex OIDC login helpers.
 *
 * Dex runs as a static OIDC provider in the dev Docker Compose stack.
 * The browser reaches it via Chromium's --host-resolver-rules
 * (mapping "dex" → 127.0.0.1, port 5556).
 *
 * The Dex config uses skipApprovalScreen: true, so after login the
 * user is immediately redirected back to Kratos — no consent step.
 */

import { Page, expect } from "@playwright/test";
import { createIdentityWithOIDC } from "./kratos";

/** Static test user configured in docker/dex/config.yml. */
export const DEX_USER_EMAIL = "dex-user@test.example";
export const DEX_USER_PASSWORD = "dex-password";
/** Dex static user ID — becomes the OIDC subject in Kratos credentials.
 *  Dex encodes userID + connector into a federated protobuf subject.
 *  This value was extracted from the Kratos OIDC callback logs. */
export const DEX_USER_ID =
  "CiQwOGE4Njg0Yi1kYjg4LTRiNzMtOTBhOS0zY2QxNjYxZjU0NjYSBWxvY2Fs";

/**
 * Register a Kratos identity pre-linked with Dex OIDC credentials.
 *
 * The identity must exist AND have OIDC credentials before the
 * identifier-first login page will show the "Sign in with Dex" button
 * (account enumeration mitigation is off).
 */
export async function registerDexIdentity(): Promise<string> {
  return createIdentityWithOIDC({
    email: DEX_USER_EMAIL,
    provider: "dex",
    subject: DEX_USER_ID,
  });
}

/**
 * Complete the Dex login form (email + password).
 *
 * Assumes the browser has been redirected to Dex's authorization page.
 * With skipApprovalScreen enabled, Dex redirects back immediately
 * after successful login.
 */
export async function loginWithDex(page: Page): Promise<void> {
  // Wait for the Dex login form
  const emailInput = page.locator("#login");
  await expect(emailInput).toBeVisible({ timeout: 15_000 });
  await emailInput.fill(DEX_USER_EMAIL);

  const passwordInput = page.locator("#password");
  await passwordInput.fill(DEX_USER_PASSWORD);

  await page.locator("button[type=submit]").click();
}

/**
 * Click the "Sign in with Dex" button on the Kratos 1FA page.
 *
 * Must be called AFTER entering the email via `enterEmail()` — the
 * identifier-first flow only shows OIDC buttons on the 1FA page,
 * not on the initial identifier page.
 */
export async function clickDexLoginButton(page: Page): Promise<void> {
  const dexButton = page.getByRole("button", { name: "Sign in with Dex" });
  await expect(dexButton).toBeVisible({ timeout: 10_000 });
  await dexButton.click();
}

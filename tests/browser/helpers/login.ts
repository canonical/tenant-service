// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0

/**
 * Login page helpers — drive the identifier-first login flow.
 *
 * The identifier-first flow has two steps:
 *   1. Enter email → backend identifies user (may redirect to tenant selection)
 *   2. Enter password → backend authenticates user (1FA)
 *
 * Use `enterEmail` / `enterPassword` individually when tenant selection
 * happens between the two steps, or `loginWithPassword` as a convenience
 * for users where tenant selection is automatic (zero-tenant / single-tenant).
 */

import { Page, expect } from "@playwright/test";

/**
 * Step 1: Enter email in the identifier-first form and submit.
 * After this, the page may navigate to:
 *   - The password form (zero/single-tenant: auto-selected)
 *   - The tenant selection page (multi-tenant: needs manual selection)
 */
export async function enterEmail(page: Page, email: string): Promise<void> {
  await page.getByLabel("Email").fill(email);
  await page.getByRole("button", { name: "Continue", exact: true }).click();
}

/**
 * Step 2: Enter password and submit. Call this after tenant selection (if any)
 * has completed and the password form is visible.
 */
export async function enterPassword(
  page: Page,
  password: string,
): Promise<void> {
  const passwordInput = page.getByRole("textbox", { name: "Password" });
  await expect(passwordInput).toBeVisible();
  await passwordInput.fill(password);
  await page.getByRole("button", { name: "Sign in", exact: true }).click();
}

/**
 * Complete the email + password login through the Kratos identifier-first UI.
 * Works for zero-tenant and single-tenant users where tenant selection is
 * automatic. For multi-tenant users, use enterEmail + manual selection +
 * enterPassword instead.
 */
export async function loginWithPassword(
  page: Page,
  email: string,
  password: string,
): Promise<void> {
  await enterEmail(page, email);
  await enterPassword(page, password);
}

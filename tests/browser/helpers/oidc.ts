// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

/**
 * OIDC authorization-code flow helpers.
 *
 * The dev stack runs the Hydra exemplary OAuth 2.0 consumer on :4446.
 * These helpers drive the browser through the full OAuth2 flow.
 */

import { Page, expect } from "@playwright/test";
import { buildAuthorizeUrl } from "./hydra";
import { decodeJwtPayload, TokenClaims } from "./jwt";

const CALLBACK_URL = "http://127.0.0.1:4446/callback";

/**
 * Tokens extracted from the OIDC callback page.
 */
export interface OIDCTokens {
  accessToken: string;
  idToken: string;
  accessTokenClaims: TokenClaims;
  idTokenClaims: TokenClaims;
}

/**
 * Start a new authorization-code flow from the OIDC consumer app.
 * Strips any default max_age injected by the OIDC consumer so the
 * flow behaves as a normal first-login (no forced re-auth).
 * After this call the browser is on the Kratos login page.
 */
export async function startOIDCFlow(page: Page): Promise<void> {
  const url = await buildAuthorizeUrl(page, {});
  await page.goto(url);
  // Wait until the login form is visible (identifier-first: email field)
  await expect(page.getByRole("heading", { name: "Sign in" })).toBeVisible();
}

/**
 * Assert the OIDC flow completed by checking the callback page shows tokens.
 * Returns the decoded tokens for further assertions (e.g. tenant_id claims).
 */
export async function expectOIDCFlowComplete(
  page: Page,
): Promise<OIDCTokens> {
  await page.waitForURL(CALLBACK_URL + "?*", { timeout: 30_000 });
  const body = await page.content();
  expect(body).toContain("Access Token");

  return extractTokensFromCallback(page);
}

/**
 * Extract access token and ID token from the Hydra OIDC consumer callback page.
 *
 * The page renders tokens as:
 *   <li>Access Token: <code>eyJ...</code></li>
 *   <li>ID Token: <code>eyJ...</code></li>
 */
async function extractTokensFromCallback(page: Page): Promise<OIDCTokens> {
  const items = page.locator("li");
  let accessToken = "";
  let idToken = "";

  const count = await items.count();
  for (let i = 0; i < count; i++) {
    const text = await items.nth(i).innerText();
    if (text.startsWith("Access Token:")) {
      accessToken = await items.nth(i).locator("code").innerText();
    } else if (text.startsWith("ID Token:")) {
      idToken = await items.nth(i).locator("code").innerText();
    }
  }

  if (!accessToken) throw new Error("access token not found on callback page");
  if (!idToken) throw new Error("ID token not found on callback page");

  return {
    accessToken,
    idToken,
    accessTokenClaims: decodeJwtPayload(accessToken),
    idTokenClaims: decodeJwtPayload(idToken),
  };
}

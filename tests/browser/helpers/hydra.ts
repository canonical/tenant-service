// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0

/**
 * Hydra OAuth2 helpers.
 *
 * Provides utilities to build custom authorize URLs with additional parameters
 * (e.g. max_age=0 for forced re-authentication) by extracting the OIDC client
 * information from the consumer page at :4446.
 */

import { Page, expect } from "@playwright/test";

const OIDC_CONSUMER = "http://127.0.0.1:4446";

/**
 * Extract the authorize URL from the OIDC consumer page link,
 * then append extra query parameters (e.g. { max_age: "0" }).
 *
 * Returns the full authorize URL with the extra params.
 */
export async function buildAuthorizeUrl(
  page: Page,
  extraParams: Record<string, string>,
): Promise<string> {
  await page.goto(OIDC_CONSUMER + "/");
  const link = page.getByRole("link", { name: "Authorize application" });
  await expect(link).toBeVisible();

  const href = await link.getAttribute("href");
  if (!href) throw new Error("authorize link has no href");

  const url = new URL(href);
  // Strip max_age from the base URL (oidc_debug may inject a default).
  // Callers that need it pass { max_age: "0" } explicitly.
  url.searchParams.delete("max_age");
  for (const [key, value] of Object.entries(extraParams)) {
    url.searchParams.set(key, value);
  }
  return url.toString();
}

/**
 * Start an OIDC authorization-code flow with custom query parameters.
 * Navigates to the authorize URL with the extra params.
 * After this call the browser should be on the Kratos login page
 * (unless session reuse applies).
 */
export async function startOIDCFlowWithParams(
  page: Page,
  extraParams: Record<string, string>,
): Promise<void> {
  const url = await buildAuthorizeUrl(page, extraParams);
  await page.goto(url);
}

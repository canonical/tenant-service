// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0

/**
 * TOTP (MFA) setup and code-generation helpers.
 *
 * Extracts the TOTP secret from the setup_secure page UI (rendered by
 * login-ui from the Kratos settings flow), then generates valid 6-digit
 * TOTP codes using the Web Crypto API.
 */

import { Page, expect } from "@playwright/test";

/**
 * Read the base32 TOTP secret from the setup_secure page.
 * The page must already be on /ui/setup_secure with the TOTP setup form.
 */
export async function getTotpSecretFromPage(page: Page): Promise<string> {
  // The heading "Secure your account" confirms we're on the TOTP setup page.
  // The secret is rendered in a <code> element after the QR code section.
  const secretEl = page.locator(
    '[data-testid="node/text/totp_secret_key/text"]',
  );
  // Fall back to the <code> element if the data-testid isn't present
  const codeEl = page.locator("code").first();
  const el = (await secretEl.isVisible().catch(() => false))
    ? secretEl
    : codeEl;
  await expect(el).toBeVisible({ timeout: 15_000 });
  return (await el.innerText()).trim();
}

/**
 * Complete TOTP setup on the setup_secure page.
 * Assumes the browser has been redirected to /ui/setup_secure after 1FA.
 * Returns the base32 secret for future code generation.
 */
export async function completeTotpSetup(page: Page): Promise<string> {
  const secret = await getTotpSecretFromPage(page);
  const code = await generateTotpCode(secret);

  const totpInput = page.getByRole("textbox", { name: "Verify code" });
  await expect(totpInput).toBeVisible({ timeout: 15_000 });
  await totpInput.fill(code);
  await page.getByRole("button", { name: "Save" }).click();

  return secret;
}

/**
 * Submit a TOTP code during the login MFA step (not setup — verification).
 * Takes the base32 secret returned by `completeTotpSetup`.
 *
 * The login MFA page shows "Verify your identity" with an "Authentication code"
 * textbox and "Sign in" button — different from the setup page's "Save" button.
 */
export async function submitTotpCode(
  page: Page,
  secret: string,
): Promise<void> {
  const code = await generateTotpCode(secret);
  const input = page.getByRole("textbox", { name: "Authentication code" });
  await expect(input).toBeVisible({ timeout: 10_000 });
  await input.fill(code);
  await page.getByRole("button", { name: "Sign in", exact: true }).click();
}

/**
 * Generate a TOTP code from a base32 secret.
 * Uses the Web Crypto API (available in Node 20+) for HMAC-SHA1.
 */
export async function generateTotpCode(secretBase32: string): Promise<string> {
  const secret = base32Decode(secretBase32);
  const time = Math.floor(Date.now() / 1000 / 30);
  const timeBuffer = new ArrayBuffer(8);
  const view = new DataView(timeBuffer);
  view.setBigUint64(0, BigInt(time));

  const key = await crypto.subtle.importKey(
    "raw",
    secret,
    { name: "HMAC", hash: "SHA-1" },
    false,
    ["sign"],
  );

  const hmac = new Uint8Array(
    await crypto.subtle.sign("HMAC", key, timeBuffer),
  );

  const offset = hmac[hmac.length - 1]! & 0x0f;
  const code =
    (((hmac[offset]! & 0x7f) << 24) |
      ((hmac[offset + 1]! & 0xff) << 16) |
      ((hmac[offset + 2]! & 0xff) << 8) |
      (hmac[offset + 3]! & 0xff)) %
    1_000_000;

  return code.toString().padStart(6, "0");
}

// ---------------------------------------------------------------------------
// Base32 decoding (RFC 4648)
// ---------------------------------------------------------------------------

const BASE32_CHARS = "ABCDEFGHIJKLMNOPQRSTUVWXYZ234567";

function base32Decode(input: string): Uint8Array {
  const cleaned = input.replace(/=+$/, "").toUpperCase();
  const out: number[] = [];
  let bits = 0;
  let value = 0;

  for (const ch of cleaned) {
    const idx = BASE32_CHARS.indexOf(ch);
    if (idx === -1) throw new Error(`invalid base32 char: ${ch}`);
    value = (value << 5) | idx;
    bits += 5;
    if (bits >= 8) {
      out.push((value >>> (bits - 8)) & 0xff);
      bits -= 8;
    }
  }

  return new Uint8Array(out);
}

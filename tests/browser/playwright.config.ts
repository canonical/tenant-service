// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

import type { PlaywrightTestConfig } from "@playwright/test";
import { devices } from "@playwright/test";

/**
 * Playwright configuration for multi-tenancy browser flow tests.
 *
 * Requires the full dev stack running (start.sh) plus a local
 * identity-platform-login-ui binary with tenant selection enabled.
 *
 * The OIDC consumer at :4446 initiates the OAuth2 authorization-code flow,
 * which redirects through Hydra → Login UI → Kratos → tenant selection → callback.
 */
const config: PlaywrightTestConfig = {
  testDir: "./specs",
  timeout: 60_000,
  expect: {
    timeout: 15_000,
  },
  fullyParallel: false,
  workers: 1,
  retries: 0,
  reporter: [["list"]],
  use: {
    actionTimeout: 10_000,
    baseURL: "http://127.0.0.1:4446",
    ignoreHTTPSErrors: true,
    video: "retain-on-failure",
    trace: "retain-on-failure",
    launchOptions: {
      // Map "dex" hostname to 127.0.0.1 so the browser can reach the
      // Dex OIDC provider running in Docker (exposed on host port 5556).
      args: ["--host-resolver-rules=MAP dex 127.0.0.1"],
    },
  },
  projects: [
    {
      name: "chromium",
      use: { ...devices["Desktop Chrome"] },
    },
  ],
};

export default config;

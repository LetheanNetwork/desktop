// SPDX-Licence-Identifier: EUPL-1.2
//
// Playwright config for lthn/desktop E2E "sight" specs.
//
// Purpose is NOT a CI gate — it's deterministic sight: drive each surface
// in a headless Chromium against the live `task dev` server and assert it
// mounts clean. The bug class this catches is the render-throw that stays
// invisible until something reads the console (the cred-banner crash).
//
// Requires `task dev` running — it serves the frontend on :9245 with the
// Go bindings proxied to the live backend (Wails v3 dev model). We do NOT
// start our own webServer; we attach to the running dev instance.

import { defineConfig, devices } from "@playwright/test";

export default defineConfig({
  testDir: "./e2e",
  timeout: 30_000,
  expect: { timeout: 10_000 },
  // Surfaces are independent — fan them out across workers.
  fullyParallel: true,
  reporter: [["list"]],
  use: {
    baseURL: "http://localhost:9245",
    trace: "retain-on-failure",
    screenshot: "only-on-failure",
  },
  projects: [
    // WebKit, not Chromium — the lthn app runs in WKWebView (WebKit +
    // JavaScriptCore) on macOS. Testing in the matching engine catches
    // WebKit-only issues and avoids Blink-only false alarms. "Desktop
    // Safari" is Playwright's webkit-backed device.
    { name: "webkit", use: { ...devices["Desktop Safari"] } },
  ],
});

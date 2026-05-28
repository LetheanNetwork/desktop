// SPDX-Licence-Identifier: EUPL-1.2
//
// App-shell E2E — the worked example the per-surface fan-out copies.
//
// <lthn-app-shell> (?surface=app) is the SPA root hosting the eight role
// views (chat / admin / planning / coding / marketing / operations / sales
// / office). The load-bearing assertion is "zero real render errors on
// mount" — the bug class the cred-migration-banner crash belonged to (a
// render throw invisible to the suite until the live bridge surfaced it).
//
// The mock + error-collection live in ./fixtures (shared). Runs in WebKit
// (the WKWebView engine the app ships in). Requires `task dev` on :9245.

import { test, expect, realErrors } from "./fixtures";

test("app-shell mounts and renders without console errors", async ({ page, errors }) => {
  // ?surface=app is the application shell (main.ts switch). Bare "/" is
  // the design-canvas surface, not the shell.
  await page.goto("/?surface=app", { waitUntil: "domcontentloaded" });

  const shell = page.locator("lthn-app-shell");
  await expect(shell).toBeAttached({ timeout: 15_000 });
  await expect(shell).not.toBeEmpty({ timeout: 15_000 });

  // Let async view-mount + first paint settle before judging the load.
  await page.waitForTimeout(1500);
  await page.screenshot({ path: "e2e/screens/app-shell.png", fullPage: true });

  const real = realErrors(errors);
  expect(real, `real render errors on app-shell load:\n  ${real.join("\n  ")}`).toEqual([]);
});

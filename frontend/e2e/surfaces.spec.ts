// SPDX-Licence-Identifier: EUPL-1.2
//
// Breadth sight — every ?surface= the SPA routes (main.ts switch), minus
// the param-required ones (editor needs ?path=, plugin needs an id). Each:
// load, confirm SOMETHING mounted, screenshot for visual review, assert
// zero real render errors. This is the sweep that FINDS broken surfaces —
// the cred-banner-class crash, on any surface, deterministically.
//
// Each case is intentionally identical to the others: that uniformity is
// what makes the fan-out a pile of tiny, parallelisable tasks. Deeper
// per-surface assertions (this pane shows X, clicking Y does Z) layer on
// top, one surface at a time.

import { test, expect, realErrors } from "./fixtures";

const SURFACES = [
  "tray", "app", "welcome", "settings", "about", "chat", "models", "lemma",
  "git", "build", "lint", "containers", "repos", "vi-sites", "vi-activity",
  "php", "marketplace", "benchmark", "logs", "telemetry", "integrations",
  "tools", "network", "distillation", "fleet", "providers", "ml-lab", "canvas",
];

for (const surface of SURFACES) {
  test(`surface · ${surface} renders without errors`, async ({ page, errors }) => {
    await page.goto(`/?surface=${surface}`, { waitUntil: "domcontentloaded" });

    // The surface mounted *something* into #app.
    await expect(page.locator("#app > *").first()).toBeAttached({ timeout: 15_000 });
    await page.waitForTimeout(1200);
    await page.screenshot({ path: `e2e/screens/${surface}.png`, fullPage: true });

    const real = realErrors(errors);
    expect(real, `${surface} real render errors:\n  ${real.join("\n  ")}`).toEqual([]);
  });
}

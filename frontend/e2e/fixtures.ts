// SPDX-Licence-Identifier: EUPL-1.2
//
// Shared E2E fixtures for lthn/desktop "sight" specs. Every surface spec
// imports { test, expect } from here so each stays tiny — the boilerplate
// (Wails runtime mock + error collection + binding-noise partition) lives
// here, once. This is the keystone the per-surface fan-out copies.
//
// Runs in WebKit (see playwright.config.ts) — the engine the app actually
// ships in (WKWebView), so render-sight is engine-faithful. Bindings are
// mocked at /wails/runtime so surfaces render their empty states instead
// of throwing on the absent native runtime; what's left to assert is REAL
// render health (the cred-banner crash class).

import { test as base, expect } from "@playwright/test";

// Binding-failure noise that's environmental in headless (no native
// runtime). Kept as a backstop even though the mock removes most of it —
// different engines report the failures differently (WebKit gives bare
// "Unhandled Promise Rejection: Error" with no stack), so source-killing
// via the mock is primary; this filter is belt-and-suspenders.
export function isBindingNoise(entry: string): boolean {
  return (
    entry.includes("/wails/runtime") ||
    entry.includes("/wails/custom.js") ||
    entry.includes("runtimeCallWithID") ||
    entry.includes("@wailsio_runtime") ||
    entry.includes("boot_probe_unreachable")
  );
}

/** Real (non-binding) errors — what a spec asserts to be empty. */
export function realErrors(errors: string[]): string[] {
  return errors.filter((e) => !isBindingNoise(e));
}

interface Fixtures {
  /** Console-error + pageerror strings collected from first paint on. */
  errors: string[];
}

export const test = base.extend<Fixtures>({
  errors: async ({ page }, use) => {
    const errors: string[] = [];
    page.on("console", (m) => {
      if (m.type() === "error") {
        const l = m.location();
        errors.push(`console.error: ${m.text()} @ ${l.url}:${l.lineNumber}`);
      }
    });
    page.on("pageerror", (e) => errors.push(`pageerror: ${e.stack || e.message}`));
    // Mock the Wails runtime so binding calls resolve (empty Result →
    // unwrap fallback) instead of 404-ing and rejecting. Installed before
    // the test's goto, so first-paint binding calls are caught.
    await page.route("**/wails/runtime*", (route) =>
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ OK: false, Value: null }),
      }),
    );
    await use(errors);
  },
});

export { expect };

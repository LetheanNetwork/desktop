// SPDX-Licence-Identifier: EUPL-1.2
//
// Vitest test setup — runs once before any spec. happy-dom already
// provides window / document / customElements; we add a couple of
// stubs for browser globals that some Lit primitives reach for in
// development paths (e.g. visualViewport on Safari, Window.matchMedia).

import { afterEach } from "vitest";

// Clean up the DOM between tests so element registration + body
// content doesn't leak across specs. Lit's customElements.define is
// idempotent on second registration with the same constructor —
// happy-dom's registry holds across tests in the same worker, which
// matches real browser behaviour.
afterEach(() => {
  document.body.innerHTML = "";
});

// matchMedia stub — happy-dom doesn't ship it. A few Lit elements
// check prefers-color-scheme during render; return a stable "matches:
// false" result so the dark-default codepath wins consistently.
if (!window.matchMedia) {
  Object.defineProperty(window, "matchMedia", {
    writable: true,
    value: (query: string) => ({
      matches: false,
      media: query,
      onchange: null,
      addListener: () => {},
      removeListener: () => {},
      addEventListener: () => {},
      removeEventListener: () => {},
      dispatchEvent: () => false,
    }),
  });
}

// SPDX-Licence-Identifier: EUPL-1.2
//
// Vitest test setup — runs once before any spec. happy-dom already
// provides window / document / customElements; we add a couple of
// stubs for browser globals that some Lit primitives reach for in
// development paths (e.g. visualViewport on Safari, Window.matchMedia).

import { afterEach, vi } from "vitest";

vi.mock("@wailsio/runtime", () => {
  const any = (source: unknown) => source;
  const array = (element: (source: unknown) => unknown) => (source: unknown) => {
    if (!Array.isArray(source)) return [];
    return source.map(element);
  };
  const map = (_key: (source: unknown) => unknown, value: (source: unknown) => unknown) => (source: unknown) => {
    if (!source || typeof source !== "object") return {};
    return Object.fromEntries(Object.entries(source).map(([k, v]) => [k, value(v)]));
  };
  const nullable = (element: (source: unknown) => unknown) => (source: unknown) => (
    source === null || source === undefined ? null : element(source)
  );
  const struct = (fields: Record<string, (source: unknown) => unknown>) => (source: unknown) => {
    const obj = source && typeof source === "object" ? { ...(source as Record<string, unknown>) } : {};
    for (const [name, create] of Object.entries(fields)) {
      if (name in obj) obj[name] = create(obj[name]);
    }
    return obj;
  };
  const call = {
    ByID: async (id: number, ...args: unknown[]) => {
      if (id === 1099757357) return new Promise(() => {});
      return Promise.reject(new Error(`mock wails runtime: unhandled call ${id}`));
    },
  };
  const create = {
    Any: any,
    Array: array,
    ByteSlice: (source: unknown) => source ?? "",
    Events: {},
    Map: map,
    Nullable: nullable,
    Struct: struct,
  };

  return {
    Call: call,
    Create: create,
    Events: {
      Emit: async () => {},
      On: () => () => {},
    },
    Window: {
      Close: async () => {},
      Fullscreen: async () => {},
      IsFullscreen: async () => false,
      Minimise: async () => {},
      UnFullscreen: async () => {},
    },
    Application: {},
    Browser: {},
    CancellablePromise: Promise,
    Clipboard: {},
    Dialogs: {},
    Flags: {},
    IOS: {},
    Screens: {},
    System: { invoke: async () => null },
    WML: {},
    clientId: "vitest",
    getTransport: () => null,
    objectNames: {},
    setTransport: () => {},
  };
});

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

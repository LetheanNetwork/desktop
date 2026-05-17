// SPDX-Licence-Identifier: EUPL-1.2
//
// Vitest test setup — runs once before any spec. happy-dom already
// provides window / document / customElements; we add a couple of
// stubs for browser globals that some Lit primitives reach for in
// development paths (e.g. visualViewport on Safari, Window.matchMedia).
//
// @wailsio/runtime mock (Mantis #1567):
//   The Call.ByID router previously carried a hardcoded binding-id
//   allowlist — only 1099757357 was permitted, every other id rejected.
//   That forced cascade tests (sales / marketing / incidents /
//   runbooks / office-mail) into per-file vi.mock redefinitions just
//   to handle their own binding ids.
//
//   The router now consults a shared handler map owned by
//   ./setup-helpers (CallRouter parked on globalThis so this hoisted
//   factory and test-side imports share one instance). Tests register
//   handlers via setCallHandler(id, fn) — see setup-helpers.ts for
//   the helper surface and usage shape.
//
//   The default behaviour still rejects unhandled binding-ids so
//   unexpected calls surface during development. Tests that want a
//   permissive default call setDefaultCallHandler(async () => null).

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
  // Router lookup happens at call time, not at hoist time. The
  // setup-helpers module lazily initialises the shared router on the
  // globalThis key the first time either side touches it — so the
  // ordering between this vi.mock factory (hoisted) and the helpers
  // import in any test file does not matter.
  const ROUTER_KEY = "__lthnTestCallRouter__";
  type CallHandler = (...args: unknown[]) => unknown | Promise<unknown>;
  interface CallRouter {
    handlers: Map<number, CallHandler>;
    defaultHandler: CallHandler;
  }
  const call = {
    ByID: async (id: number, ...args: unknown[]) => {
      const router = (globalThis as Record<string, unknown>)[ROUTER_KEY] as CallRouter | undefined;
      if (router) {
        const handler = router.handlers.get(id);
        if (handler) return handler(...args);
        return router.defaultHandler(id, ...args);
      }
      // Router not initialised yet — preserve the legacy reject so
      // a spec that calls ByID without importing the helpers still
      // gets the descriptive error rather than a silent undefined.
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

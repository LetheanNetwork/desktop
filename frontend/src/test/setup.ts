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

// localStorage / sessionStorage bridge (Mantis #1575).
//
//   Node 22+ exposes an experimental built-in `localStorage` /
//   `sessionStorage` global stub. The stub is inert unless Node was
//   started with `--localstorage-file <path>` — accessing any method
//   raises `TypeError: localStorage.removeItem is not a function`.
//
//   Vitest's happy-dom environment bridges Window properties onto
//   globalThis via `populateGlobal` (node_modules/vitest/.../index.*.js),
//   but its `getWindowKeys` filter SKIPS any property that already
//   exists on globalThis unless it's in vitest's hardcoded KEYS list.
//   `localStorage` is not in KEYS — so Node's broken stub wins and
//   happy-dom's working Storage instance is never exposed.
//
//   Bridge them ourselves: read window.localStorage (happy-dom's real
//   Storage) and redefine the globals so the tests see the working
//   instance.
// Node 22+ exposes an experimental built-in `localStorage` global on
// globalThis. The stub is inert unless Node was started with
// `--localstorage-file <path>` — any method call raises
// `TypeError: localStorage.removeItem is not a function`.
//
// Vitest's happy-dom env aliases `window === globalThis`, then bridges
// happy-dom Window properties onto the global. Its `getWindowKeys`
// filter (vitest/dist/chunks/index.*.js) skips any key that is already
// present on globalThis unless it's in vitest's hardcoded KEYS list.
// `localStorage` is NOT in that list, so Node's broken stub wins and
// happy-dom's real Storage is unreachable from `window` or
// `globalThis`.
//
// Bridge a working Storage instance ourselves by spinning up a private
// happy-dom Window and reading its localStorage/sessionStorage out
// directly (synchronous, no side-effects on the test window). Redefine
// the broken globals so bare `localStorage` / `sessionStorage` resolve
// to the working instance.
import { Window as HappyWindow } from "happy-dom";
const __lthnStorageWin = new HappyWindow();
for (const name of ["localStorage", "sessionStorage"] as const) {
  const storage = __lthnStorageWin[name];
  if (storage && typeof storage.removeItem === "function") {
    Object.defineProperty(globalThis, name, {
      configurable: true,
      writable: true,
      value: storage,
    });
  }
}

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

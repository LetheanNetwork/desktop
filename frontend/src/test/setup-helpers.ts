// SPDX-Licence-Identifier: EUPL-1.2
//
// Test-side helpers for the @wailsio/runtime mock installed by
// src/test/setup.ts. Tests register per-binding-id handlers here
// without redefining the file-wide vi.mock.
//
// Why this exists (Mantis #1567):
//   setup.ts's vi.mock used to carry a hardcoded binding-id allowlist
//   (only 1099757357 was permitted). Any test exercising a cascade
//   surface — sales/marketing/incidents/runbooks/office-mail — hit
//   the reject path and either flaked or had to redefine vi.mock at
//   file scope (the H#28 workaround in pipeline.test.ts after #1543).
//
//   The setup mock now consults a shared handler map owned by this
//   module. Tests import the helpers, set handlers per-test, and the
//   already-hoisted setup mock routes Call.ByID through them.
//
// Usage:
//
//   import { setCallHandler, clearCallHandlers } from "@/test/setup-helpers";
//   import { beforeEach } from "vitest";
//
//   const BID_PIPELINE_MOVE = 4112870272;
//   beforeEach(() => {
//     clearCallHandlers();
//     setCallHandler(BID_PIPELINE_MOVE, async () => ({ OK: true, Value: {} }));
//   });
//
// Defaults:
//   - Unhandled binding-ids fall through to the default handler.
//   - The default handler rejects with a descriptive error, matching
//     the legacy setup.ts behaviour so unexpected calls still surface.
//   - Tests that want a permissive default (e.g. silent no-op) call
//     setDefaultCallHandler with their own function.
//
// The shared map is published via globalThis so the setup.ts vi.mock
// factory (which is hoisted above any import in that file) can read
// the same instance the helpers mutate. Tests must NOT touch the
// globalThis key directly — use the exported functions.

export type CallHandler = (...args: unknown[]) => unknown | Promise<unknown>;

// Internal shape — kept here so setup.ts and the helpers share one
// definition. The globalThis key is a Symbol-ish string to avoid name
// collisions with anything the production code might park there.
interface CallRouter {
  handlers: Map<number, CallHandler>;
  defaultHandler: CallHandler;
}

const ROUTER_KEY = "__lthnTestCallRouter__";

function defaultRejectHandler(id: number): CallHandler {
  return () => Promise.reject(new Error(`mock wails runtime: unhandled call ${id}`));
}

function getRouter(): CallRouter {
  const g = globalThis as Record<string, unknown>;
  let router = g[ROUTER_KEY] as CallRouter | undefined;
  if (!router) {
    router = {
      handlers: new Map<number, CallHandler>(),
      // Default rejects, with the id baked in at call time — preserves
      // the legacy "unhandled call <id>" diagnostic shape.
      defaultHandler: (id: unknown) => defaultRejectHandler(id as number)(),
    };
    g[ROUTER_KEY] = router;
  }
  return router;
}

/** Register a handler for one binding-id. Overwrites any prior entry. */
export function setCallHandler(id: number, handler: CallHandler): void {
  getRouter().handlers.set(id, handler);
}

/** Remove a single binding-id handler. No-op if absent. */
export function unsetCallHandler(id: number): void {
  getRouter().handlers.delete(id);
}

/** Clear every per-binding-id handler. Default handler is preserved.
 *  Tests call this in beforeEach to start from a known state. */
export function clearCallHandlers(): void {
  getRouter().handlers.clear();
}

/** Replace the fall-through handler invoked for any binding-id not in
 *  the per-id map. Pass a no-op (`async () => null`) to silence
 *  unexpected calls in suites that don't care. */
export function setDefaultCallHandler(handler: CallHandler): void {
  getRouter().defaultHandler = handler;
}

/** Restore the default-reject behaviour (one-line undo for tests that
 *  swapped the default and need to leave the world clean for siblings
 *  sharing the same worker). */
export function resetDefaultCallHandler(): void {
  const router = getRouter();
  router.defaultHandler = (id: unknown) => defaultRejectHandler(id as number)();
}

/** Internal hook for src/test/setup.ts — returns the live router so
 *  the vi.mock factory can dispatch through the same instance the
 *  helpers mutate. NOT for use in test files. */
export function __getCallRouter(): CallRouter {
  return getRouter();
}

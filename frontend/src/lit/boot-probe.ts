// SPDX-Licence-Identifier: EUPL-1.2
//
// boot-probe.ts — first-paint auth-state probe extracted from main.ts.
//
// Mantis #1500 — the previous in-line IIFE in main.ts swallowed every
// failure path of the AccountStatus probe:
//
//   - import('@desktop/serverkey/service').catch(() => null)
//   - if (!svc || typeof svc.AccountStatus !== 'function') return
//   - catch { /* binding missing → degrade silently */ }
//
// Any of those silent-degrade exits left the underlying surface
// (tray / chat / settings / canvas) rendering UNPROTECTED — no auth
// overlay, no error, no console signal. A regression in
// @desktop/serverkey/service binding generation would disable the
// boot-probe gate with NO visible signal until a user noticed they
// could see content without signing in.
//
// This module replaces the silent-degrade with fail-CLOSED:
//
//   - probe outcome is one of {ok-account, ok-setup, unreachable}
//   - "ok-account" → don't mount; the apiFetch interceptor catches
//                    401s downstream (account exists, unlock pending)
//   - "ok-setup"   → mount("setup"); identical to prior behaviour
//   - "unreachable" → mount("error", reason) + structured console.error
//                    so QA / dev mode sees the failure rather than a
//                    blank passing-through surface
//
// Probe shape is **Wails IPC** (not HTTP fetch) — the binding lives in
// `@desktop/serverkey/service`. Failure modes:
//
//   binding-missing    — dynamic import rejects (bundler glitch, file
//                        absent in the bindings/ tree). UNREACHABLE.
//   not-a-function     — module loaded but AccountStatus isn't a fn.
//                        Schema drift from go-side binding regeneration.
//                        UNREACHABLE.
//   call-rejected      — the Wails IPC call rejects (transport error,
//                        Go side panicked, runtime not ready).
//                        UNREACHABLE.
//   result-not-ok      — call resolved but Result.OK === false. The Go
//                        side ran but reported a degraded backend.
//                        UNREACHABLE.
//   ok-account-true    — Result.OK && has_user_account === true. The
//                        gate stays dormant; apiFetch will fire 401
//                        events into the global listener and trigger
//                        mount("error", id) on the first authed call.
//   ok-account-false   — Result.OK && has_user_account === false. We
//                        mount("setup") so the very first paint is the
//                        Lethean Account creation form.
//
// Usage from main.ts:
//
//   import { runBootProbe } from "./lit/boot-probe";
//   void runBootProbe(mountOverlay);
//
// where `mountOverlay(state, reqId?, errorMessage?, errorBody?)` is the
// closure that creates the <lthn-auth-gate> element + appends it.

/** Outcome the probe returns to the caller. The discriminator drives
 *  the mount decision; `reason` carries the human-readable string the
 *  error overlay headlines + the structured log writes. */
export type BootProbeOutcome =
  | { kind: "ok-account" }
  | { kind: "ok-setup" }
  | { kind: "unreachable"; reason: string; detail: string; cause?: unknown };

/** Mount callback supplied by main.ts. Kept as a parameter (not an
 *  internal import) so the unit tests can drive the probe without
 *  pulling the entire <lthn-auth-gate> element graph. */
export type MountFn = (
  state: "setup" | "auth" | "error",
  options?: { requestId?: string; errorMessage?: string; errorBody?: string },
) => void;

/** Module loader override — defaults to the real dynamic import. Tests
 *  inject a stub so the probe can exercise every branch without
 *  mocking the Vite alias graph. */
export type ServerKeyImporter = () => Promise<unknown>;

/** Shape we accept off the loaded module — narrowed at runtime so a
 *  schema-drifted binding (AccountStatus removed / renamed) lands in
 *  the unreachable branch with a precise reason rather than a TypeError
 *  bubble. */
interface ServerKeyModuleLike {
  AccountStatus?: () => Promise<{
    OK?: boolean;
    Value?: { has_user_account?: boolean };
  }>;
}

const REASON_BINDING_IMPORT = "serverkey binding failed to load";
const REASON_BINDING_SHAPE  = "serverkey binding missing AccountStatus";
const REASON_CALL_REJECTED  = "AccountStatus call rejected";
const REASON_BACKEND_DOWN   = "AccountStatus returned a degraded result";

/** Default body text that fills the error overlay's framed panel. The
 *  copy stays short + actionable per
 *  feedback_ui_text_offer_not_implementation — no PGP / wallet /
 *  server.key surface terms. */
const ERROR_BODY_PROSE =
  "Lethean Desktop couldn't reach the local runner to check this Mac's account state. "
  + "The desktop usually starts the runner automatically; if it failed to launch, restart Lethean Desktop. "
  + "If this keeps happening, capture the developer console (Help → Developer) and share it with support.";

/** Structured payload written to console.error on any unreachable
 *  outcome. Shape stays stable so log scrapers / Sentry pipelines can
 *  index by `event`. */
function logUnreachable(reason: string, detail: string, cause?: unknown): void {
  // console.error not console.warn — the failure leaves the gate
  // mounted in error state, so the user sees something is wrong. The
  // log line is the dev/QA breadcrumb that tells them WHY.
  console.error("[lthn:boot-probe] unreachable", {
    event: "boot_probe_unreachable",
    reason,
    detail,
    cause: cause instanceof Error ? { name: cause.name, message: cause.message } : cause,
  });
}

/** Run the boot probe + decide what to mount.
 *
 *  Returns the probe outcome so the caller (or a test) can assert what
 *  decision was reached. Mounting is performed as a side-effect via the
 *  injected `mount` callback so this module stays decoupled from the
 *  DOM-shaped overlay creation in main.ts.
 *
 *  Never throws. Every failure surfaces as `{kind: "unreachable", …}`
 *  with mount("error") already invoked — fail-CLOSED. */
export async function runBootProbe(
  mount: MountFn,
  importer: ServerKeyImporter = () => import("@desktop/serverkey/service"),
): Promise<BootProbeOutcome> {
  let svc: ServerKeyModuleLike | null = null;
  try {
    svc = (await importer()) as ServerKeyModuleLike;
  } catch (cause) {
    const reason = REASON_BINDING_IMPORT;
    const detail = "Dynamic import of @desktop/serverkey/service rejected.";
    logUnreachable(reason, detail, cause);
    mount("error", {
      errorMessage: "Cannot reach lthn server.",
      errorBody:    ERROR_BODY_PROSE,
    });
    return { kind: "unreachable", reason, detail, cause };
  }

  if (!svc || typeof svc.AccountStatus !== "function") {
    const reason = REASON_BINDING_SHAPE;
    const detail = "Loaded serverkey module does not export AccountStatus.";
    logUnreachable(reason, detail);
    mount("error", {
      errorMessage: "Cannot reach lthn server.",
      errorBody:    ERROR_BODY_PROSE,
    });
    return { kind: "unreachable", reason, detail };
  }

  let r: { OK?: boolean; Value?: { has_user_account?: boolean } };
  try {
    r = await svc.AccountStatus();
  } catch (cause) {
    const reason = REASON_CALL_REJECTED;
    const detail = "AccountStatus() promise rejected.";
    logUnreachable(reason, detail, cause);
    mount("error", {
      errorMessage: "Cannot reach lthn server.",
      errorBody:    ERROR_BODY_PROSE,
    });
    return { kind: "unreachable", reason, detail, cause };
  }

  // Result.OK === false → the Go side ran but reports a degraded
  // backend. Surface it; do NOT pass through. Some callers explicitly
  // set OK=false (e.g. account.Service unavailable, store error) and
  // the previous code silently treated that as "fine, no setup needed"
  // because the truthiness check only fired on Value.has_user_account.
  if (r?.OK === false) {
    const reason = REASON_BACKEND_DOWN;
    const detail = "AccountStatus returned OK=false.";
    logUnreachable(reason, detail);
    mount("error", {
      errorMessage: "Cannot reach lthn server.",
      errorBody:    ERROR_BODY_PROSE,
    });
    return { kind: "unreachable", reason, detail };
  }

  if (r?.Value?.has_user_account === false) {
    mount("setup");
    return { kind: "ok-setup" };
  }

  // Account exists (or the binding reports OK with an unspecified value
  // shape — defensive default is do-not-mount because the apiFetch 401
  // interceptor catches the unauthenticated-call case downstream).
  return { kind: "ok-account" };
}

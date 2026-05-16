// SPDX-Licence-Identifier: EUPL-1.2
//
// <lthn-auth-gate> — pre-condition gate that stands in front of any
// view that needs a signed-in Lethean Account. Mounted by
// <lthn-app-shell> ahead of the normal body slot whenever authState
// is anything other than "ok".
//
// State machine (mirrors plans/code/lthn/desktop/auth-gate/RFC.md §1):
//
//   setup → no Lethean Account on this Mac → "Run setup" mints a
//           bootstrap token + POSTs /v1/account/create to provision.
//   auth  → account on disk, just needs a passphrase unlock.
//   error → an API call 401'd and state-derivation failed; renders the
//           framed raw response + retry.
//   ok    → renders `nothing`; the underlying view slot mounts.
//
// Derivation: on connectedCallback, calls ServerKey.AccountStatus().
// has_user_account=false → state=setup; =true → state=auth. The
// app-shell flips us to "error" via lthn:auth:401 when an apiFetch
// response 401s while we're already "ok".
//
// Cerberus #1465 — load-bearing rule.
// The bootstrap token returned by IssueBootstrapToken() MUST NEVER
// leave the local variable inside the click handler that consumes it.
// No localStorage, no sessionStorage, no cookie, no IndexedDB, no
// `this.bootstrapToken = …`. On retry, re-mint instead of caching.
// The verifier-side TTL is short (60s issuer / 120s ceiling per #1462)
// specifically because the gate is permitted to mint per attempt.
//
// Usage:
//
//   import "@/lit/auth-gate";
//   // <lthn-app-shell> mounts it automatically when authState !== "ok".
//
// Event contract:
//
//   The gate dispatches the following CustomEvents on `window`:
//     lthn:auth:ok      — setup or unlock succeeded; shell flips to "ok"
//     lthn:auth:401     — emitted by api-fetch.ts, not by the gate; here
//                          for reference. The gate listens for this and
//                          transitions to "error".
//
// UI string discipline (per feedback_ui_text_offer_not_implementation):
// copy says "passphrase", "Lethean Account", "your Mac". Never "PGP",
// "server.key", "wallet". Port verbatim from the design file.

import { LitElement, html, nothing, type TemplateResult } from "lit";
import { renderChrome } from "./chrome";
import { setSessionToken } from "./api-fetch";

/** Concrete state names the gate cycles between. Kept as a string-
 *  union so the property reflects to an attribute and the test harness
 *  can assert via getAttribute("state"). */
export type AuthGateState = "setup" | "auth" | "error" | "ok";

/** CustomEvent name emitted by api-fetch.ts when an apiFetch response
 *  status is 401. Constant so emitter + listener can't drift. */
export const AUTH_401_EVENT = "lthn:auth:401";

/** CustomEvent name dispatched by the gate after a successful
 *  setup-or-unlock so the shell can flip back to "ok". */
export const AUTH_OK_EVENT = "lthn:auth:ok";

/** Minimal binding shape — we only access the JSON-tagged fields. The
 *  Wails-generated bindings return `core.Result` whose `Value` is the
 *  Go struct serialised to JSON, so the TS keys mirror the `json:"…"`
 *  tags on AccountStatusOutput / BootstrapTokenOutput / UnlockOutput. */
interface AccountStatusValue { has_user_account?: boolean; account_id?: string }
interface BootstrapTokenValue { token?: string; expires_at?: number }
interface UnlockResponseBody {
  session_token?: string;
  expires_at?: number;
  account_id?: string;
}
interface ErrorResponseBody {
  error?: { code?: string; message?: string };
  meta?: { request_id?: string };
}

/** Stable id attribute on the passphrase input so _onWreathIn can read
 *  the typed value via querySelector without holding a Lit ref on
 *  `this`. The value is read once, copied into a local variable, and
 *  the field cleared after the response — closure-only discipline per
 *  Cerberus #1465 applied to the user-typed passphrase. */
const UNLOCK_INPUT_ID = "lthn-auth-gate-passphrase";

/** Stable id attributes on the setup-form passphrase + confirm inputs.
 *  Same closure-only read pattern as UNLOCK_INPUT_ID — the typed
 *  passphrase lives in a local var inside _onRunSetup, NEVER on
 *  `this` or in any storage surface (RFC.stage-x.md §4.2). */
const SETUP_PASSPHRASE_INPUT_ID = "lthn-auth-gate-setup-passphrase";
const SETUP_CONFIRM_INPUT_ID    = "lthn-auth-gate-setup-confirm";

class LthnAuthGate extends LitElement {
  static readonly properties = {
    state:           { type: String, reflect: true },
    title:           { type: String },
    subtitle:        { type: String },
    w:               { type: Number },
    h:               { type: Number },
    requestId:       { type: String, attribute: "request-id" },
    embedded:        { type: Boolean, reflect: true },
    loading:         { state: true },
    errorStatus:     { state: true },
    errorMessage:    { state: true },
    errorBody:       { state: true },
    accountId:       { state: true },
    unlockError:     { state: true },
    setupError:      { state: true },
    lockedOutUntilSec: { state: true },
  };

  declare state:        AuthGateState;
  declare title:        string;
  declare subtitle:     string;
  declare w:            number;
  declare h:            number;
  declare requestId:    string;
  declare embedded:     boolean;
  declare loading:      boolean;
  /** HTTP status of the failed request that triggered state=error.
   *  0 when the gate is in state=error from a CustomEvent rather than
   *  a click-handler fetch (e.g. apiFetch interceptor 401). */
  declare errorStatus:  number;
  /** Human-readable error message — the design used to hardcode
   *  "missing authorization header" but the real response body's
   *  error.message is the truth. */
  declare errorMessage: string;
  /** Raw response body (or JSON-stringified) for the framed text
   *  panel in the error state. Falls back to the hardcoded design
   *  mockup when empty (state=error fired without a fetch context). */
  declare errorBody:    string;
  /** Captured from AccountStatus().account_id on derive — the id the
   *  unlock POST sends in its body. Multi-account selection is RFC.
   *  stage-e non-goal; Stage F adds a chooser when this is ambiguous. */
  declare accountId:    string;
  /** Inline error message under the passphrase input in state=auth.
   *  Cleared on next Wreath-in attempt or on input change. */
  declare unlockError:  string;
  /** Inline error message under the setup form in state=setup —
   *  mismatch, too-short, server-side validation failures. Cleared
   *  on next submit attempt or on input change. */
  declare setupError:   string;
  /** Unix-seconds timestamp until which the unlock button stays
   *  disabled after a lockout response (account.unlock.locked_out).
   *  0 when not locked-out. RFC.stage-e §5 — 60s default cooldown,
   *  the actual value comes from the server's "try again in N seconds"
   *  message so backend rotations are picked up automatically. */
  declare lockedOutUntilSec: number;

  constructor() {
    super();
    this.state             = "setup";
    this.title             = "Lethean Desktop";
    this.subtitle          = "first-run · setup";
    this.w                 = 920;
    this.h                 = 620;
    this.requestId         = "";
    this.embedded          = false;
    this.loading           = false;
    this.errorStatus       = 0;
    this.errorMessage      = "";
    this.errorBody         = "";
    this.accountId         = "";
    this.unlockError       = "";
    this.setupError        = "";
    this.lockedOutUntilSec = 0;
  }

  // Light DOM — same shape as every other shell child so tokens.css +
  // font-awesome apply without shadow-root juggling.
  createRenderRoot() { return this; }

  /** Bound 401 listener — flips the gate into the framed-error view
   *  whenever apiFetch dispatches lthn:auth:401 while we're mounted.
   *  Reads requestId from the CustomEvent detail so the error frame
   *  can surface the same trace id the server-side logs index by. */
  private _on401 = (ev: Event) => {
    const detail = (ev as CustomEvent<{ requestId?: string }>).detail;
    if (detail && typeof detail.requestId === "string") {
      this.requestId = detail.requestId;
    }
    this.state = "error";
  };

  async connectedCallback() {
    super.connectedCallback();
    window.addEventListener(AUTH_401_EVENT, this._on401);
    await this._deriveState();
  }

  disconnectedCallback() {
    super.disconnectedCallback();
    window.removeEventListener(AUTH_401_EVENT, this._on401);
  }

  /** Probe the backend for whether a Lethean Account exists. Sets
   *  state=setup when no account is provisioned, state=auth when one
   *  is. On binding failure (no Wails surface in test, or transport
   *  error in prod) leaves state untouched so the caller can decide
   *  to fall back to the error view via the 401 event. Public so the
   *  Retry button can re-run the derivation without re-mounting. */
  async _deriveState(): Promise<void> {
    try {
      const svc = await import("@desktop/serverkey/service");
      const r = await svc.AccountStatus();
      if (!r || !r.OK) return;
      const v = r.Value as AccountStatusValue | undefined;
      this.state = v?.has_user_account ? "auth" : "setup";
      this.accountId = typeof v?.account_id === "string" ? v.account_id : "";
    } catch {
      // Binding unavailable — leave state alone. The 401 listener will
      // pick up real failures from in-flight apiFetch calls.
    }
  }

  /** Run setup click handler — RFC.stage-x.md §3 end-to-end provision.
   *
   *  Closure-only discipline (Cerberus #1465 applied to passphrase +
   *  bootstrap-token + session-token): each value lives in handler-
   *  local vars / api-fetch module-scope ONLY. Inputs are cleared at
   *  the end so a stale value can't re-submit from a later click
   *  without the user re-typing.
   *
   *  Flow per RFC.stage-x.md §3:
   *   1. read passphrase + confirm from <input id={SETUP_*_INPUT_ID}>
   *   2. mismatch → inline error, NO fetch
   *   3. mint bootstrap-token with scope="account.provision"
   *   4. generate fresh request_id via crypto.randomUUID()
   *   5. POST /v1/account/provision with {passphrase, request_id}
   *   6. on 200 → setSessionToken(session_token), dispatch AUTH_OK_EVENT
   *   7. on 400 passphrase.required → inline error, stays on setup
   *   8. on 409 exists → re-derive state (race with concurrent setup)
   *   9. on any other failure → framed error view with captured body */
  async _onRunSetup(): Promise<void> {
    if (this.loading) return;

    const ppInput = this.querySelector("#" + SETUP_PASSPHRASE_INPUT_ID) as HTMLInputElement | null;
    const ccInput = this.querySelector("#" + SETUP_CONFIRM_INPUT_ID) as HTMLInputElement | null;
    const pp = ppInput?.value ?? "";
    const cc = ccInput?.value ?? "";

    if (pp.length === 0) {
      this.setupError = "Enter a passphrase to set up your Lethean Account.";
      return;
    }
    if (pp.length < 12) {
      this.setupError = "Passphrase must be at least 12 characters.";
      return;
    }
    if (pp !== cc) {
      this.setupError = "Passphrases don't match.";
      return;
    }

    this.loading = true;
    this.setupError = "";
    try {
      const svc = await import("@desktop/serverkey/service");
      const tokR = await svc.IssueBootstrapTokenForScope("account.provision");
      if (!tokR || !tokR.OK) {
        this.state = "error";
        this.errorStatus = 0;
        this.errorMessage = "Couldn't mint a setup token.";
        this.errorBody = "";
        return;
      }
      const tok = (tokR.Value as BootstrapTokenValue | undefined)?.token;
      if (typeof tok !== "string" || tok.length === 0) {
        this.state = "error";
        this.errorStatus = 0;
        this.errorMessage = "Server returned an empty setup token.";
        this.errorBody = "";
        return;
      }

      // Client-generated request_id for the §3.2 idempotency dedup
      // window (server-side dedup ships in Stage X.B Phase 2c — this
      // field is required by the contract today and ignored by the
      // 2a server impl). Fresh UUID per submit; regenerated on retry.
      const requestID = (typeof crypto !== "undefined" && typeof crypto.randomUUID === "function")
        ? crypto.randomUUID()
        : `req-${Date.now()}-${Math.random().toString(36).slice(2)}`;

      const res = await fetch("/v1/account/provision", {
        method: "POST",
        headers: {
          "Authorization": "Bootstrap " + tok,
          "Content-Type":  "application/json",
        },
        body: JSON.stringify({ passphrase: pp, request_id: requestID }),
      });

      if (res.ok) {
        const json = await res.json().catch(() => null) as
          { Value?: UnlockResponseBody; value?: UnlockResponseBody } | null;
        const out = json?.Value ?? json?.value ?? null;
        const sessionToken = out?.session_token;
        if (typeof sessionToken !== "string" || sessionToken.length === 0) {
          this.state = "error";
          this.errorStatus = res.status;
          this.errorMessage = "Setup succeeded but the session token was empty.";
          this.errorBody = "";
          return;
        }
        setSessionToken(sessionToken);
        if (ppInput) ppInput.value = "";
        if (ccInput) ccInput.value = "";
        this.state = "ok";
        window.dispatchEvent(new CustomEvent(AUTH_OK_EVENT));
        return;
      }

      // Non-OK — read body once.
      const txt = await res.text().catch(() => "");
      let body: ErrorResponseBody | null = null;
      try { body = JSON.parse(txt) as ErrorResponseBody; } catch {}
      const code = body?.error?.code ?? "";
      const msg  = body?.error?.message ?? "";
      const rid  = res.headers.get("X-Request-Id")
                || res.headers.get("X-Request-ID")
                || body?.meta?.request_id
                || "";
      if (rid) this.requestId = rid;

      if (code === "account.provision.passphrase.required") {
        this.setupError = msg || "Enter your passphrase.";
        return;
      }
      if (code === "account.exists") {
        // Concurrent setup race — re-derive state. If an account
        // really does exist now, we transition to auth and the user
        // unlocks instead of provisioning again.
        if (ppInput) ppInput.value = "";
        if (ccInput) ccInput.value = "";
        await this._deriveState();
        return;
      }
      // Any other failure (keygen/encrypt/mint/5xx) — framed error.
      this.errorStatus = res.status;
      this.errorBody = txt;
      this.errorMessage = msg;
      this.state = "error";
      return;
    } catch {
      this.state = "error";
      this.errorStatus = 0;
      this.errorMessage = "Couldn't reach the runner.";
      this.errorBody = "";
      return;
    } finally {
      this.loading = false;
      // pp + cc fall out of scope here — GC reclaims, no `this.*`
      // cache, no localStorage. Closure-only discipline preserved.
    }
  }

  /** Wreath-in click handler — RFC.stage-e §5 end-to-end unlock.
   *
   *  Closure-only discipline (Cerberus #1465 applied to the typed
   *  passphrase): the value lives in the local `pp` variable only.
   *  Never assigned to `this`, never logged, never cached. The input
   *  field is cleared after the call so a stale value can't be
   *  re-submitted from a later click without the user re-typing.
   *
   *  Flow per RFC §2:
   *  1. read passphrase from <input id={UNLOCK_INPUT_ID}>
   *  2. mint bootstrap-token with scope="account.unlock"
   *  3. POST /v1/account/unlock with {account_id, passphrase} body +
   *     Authorization: Bootstrap <token>
   *  4. on 200 → setSessionToken(session_token), dispatch
   *     AUTH_OK_EVENT, state="ok"
   *  5. on 401 bad_passphrase → inline error, stay on state="auth"
   *  6. on 429 locked_out → disable button + show server's "try again
   *     in N seconds" message, stay on state="auth"
   *  7. on any other failure → state="error" with captured body
   *
   *  Lockout countdown comes from the server-side cooldown deadline
   *  parsed out of the locked_out message (RFC §5: "too many attempts
   *  — try again in N seconds"). lockedOutUntilSec is consulted by
   *  the button's disabled attribute + the inline-error renderer. */
  async _onWreathIn(): Promise<void> {
    if (this.loading) return;
    if (this._isLockedOut()) return;

    const input = this.querySelector("#" + UNLOCK_INPUT_ID) as HTMLInputElement | null;
    const pp = input?.value ?? "";
    if (pp.length === 0) {
      this.unlockError = "Enter your passphrase to unlock your account.";
      return;
    }
    if (this.accountId.length === 0) {
      // _deriveState didn't find an account_id — either a race against
      // unmount or AccountStatus regressed. Fall back to the framed
      // error view so the user can retry.
      this.unlockError = "";
      this.state = "error";
      this.errorStatus = 0;
      this.errorMessage = "No Lethean Account is registered on this Mac.";
      this.errorBody = "";
      return;
    }

    this.loading = true;
    this.unlockError = "";
    try {
      // Mint a fresh bootstrap-token scoped to the unlock endpoint.
      // Per RFC §2.3 + Cerberus #1467 path-scope discipline: re-using
      // the create-scope token here would skip the unlock-only gate.
      const svc = await import("@desktop/serverkey/service");
      const tokR = await svc.IssueBootstrapTokenForScope("account.unlock");
      if (!tokR || !tokR.OK) {
        this.state = "error";
        this.errorStatus = 0;
        this.errorMessage = "Couldn't mint an unlock token.";
        this.errorBody = "";
        return;
      }
      const tok = (tokR.Value as BootstrapTokenValue | undefined)?.token;
      if (typeof tok !== "string" || tok.length === 0) {
        this.state = "error";
        this.errorStatus = 0;
        this.errorMessage = "Server returned an empty unlock token.";
        this.errorBody = "";
        return;
      }

      const res = await fetch("/v1/account/unlock", {
        method: "POST",
        headers: {
          "Authorization": "Bootstrap " + tok,
          "Content-Type":  "application/json",
        },
        body: JSON.stringify({ account_id: this.accountId, passphrase: pp }),
      });

      if (res.ok) {
        const json = await res.json().catch(() => null) as
          { Value?: UnlockResponseBody; value?: UnlockResponseBody } | null;
        const out = json?.Value ?? json?.value ?? null;
        const sessionToken = out?.session_token;
        if (typeof sessionToken !== "string" || sessionToken.length === 0) {
          this.state = "error";
          this.errorStatus = res.status;
          this.errorMessage = "Server returned an empty session token.";
          this.errorBody = "";
          return;
        }
        // Closure-only handoff — setSessionToken stores in api-fetch's
        // module-scope binding. NEVER persisted (RFC §3.2 + #1465).
        setSessionToken(sessionToken);
        // Clear the DOM input even on success — Lit unmounts the
        // element on state=ok but the reference still holds .value
        // until the next render flush. Wipe so a re-rendered gate
        // (post-Lock + back to auth) never inherits a stale passphrase.
        if (input) input.value = "";
        this.state = "ok";
        window.dispatchEvent(new CustomEvent(AUTH_OK_EVENT));
        return;
      }

      // Non-OK — read the body once. coreapi.Fail returns
      // {"success":false, "error":{"code":..., "message":...}, ...}
      const txt = await res.text().catch(() => "");
      let body: ErrorResponseBody | null = null;
      try { body = JSON.parse(txt) as ErrorResponseBody; } catch {}
      const code = body?.error?.code ?? "";
      const msg  = body?.error?.message ?? "";
      const rid  = res.headers.get("X-Request-Id")
                || res.headers.get("X-Request-ID")
                || body?.meta?.request_id
                || "";
      if (rid) this.requestId = rid;

      if (code === "account.unlock.bad_passphrase") {
        // Stay on state=auth, show inline error. Backend message is
        // already user-facing ("passphrase didn't unlock your account
        // — N attempts remaining"). Surface verbatim — copy lives
        // server-side per design.
        this.unlockError = msg || "Passphrase didn't unlock your account.";
        if (input) input.value = "";
        return;
      }
      if (code === "account.unlock.locked_out") {
        const seconds = this._parseLockoutSeconds(msg);
        this.lockedOutUntilSec = Math.floor(Date.now() / 1000) + seconds;
        this.unlockError = msg || "Too many attempts. Try again in a moment.";
        if (input) input.value = "";
        // Schedule a re-render the moment the lockout expires so the
        // button + inline error self-recover without the user clicking.
        if (seconds > 0) {
          window.setTimeout(() => {
            this.lockedOutUntilSec = 0;
            this.unlockError = "";
            this.requestUpdate();
          }, seconds * 1000);
        }
        return;
      }
      if (code === "account.unlock.passphrase.required") {
        this.unlockError = "Enter your passphrase to unlock your account.";
        return;
      }
      // Any other failure (corrupted_key, server_misconfigured, 5xx) —
      // flip to the framed error view per RFC §1 state machine.
      this.errorStatus = res.status;
      this.errorBody = txt;
      this.errorMessage = msg;
      this.state = "error";
    } catch {
      this.state = "error";
      this.errorStatus = 0;
      this.errorMessage = "Couldn't reach the runner.";
      this.errorBody = "";
    } finally {
      this.loading = false;
      // `pp` falls out of scope here — garbage collector reclaims the
      // string. No `this.passphrase`, no module cache, no DOM read-back
      // (input.value was cleared on every non-OK code path above).
    }
  }

  /** Parse the "N seconds" out of the locked_out message. Backend
   *  format is "too many attempts — try again in 60 seconds" (RFC §5
   *  + unlock.go:106/127/176). Returns 60 as a safe default if the
   *  message shape ever drifts so the UI doesn't hang locked forever. */
  private _parseLockoutSeconds(msg: string): number {
    const m = /(\d+)\s*seconds?/i.exec(msg);
    if (m && m[1]) {
      const n = parseInt(m[1], 10);
      if (Number.isFinite(n) && n > 0) return n;
    }
    return 60;
  }

  /** True when the unlock button should stay disabled because the
   *  account is in cooldown. Consulted by _onWreathIn (entry guard)
   *  + by _auth() (button disabled attribute). */
  private _isLockedOut(): boolean {
    if (this.lockedOutUntilSec <= 0) return false;
    return Math.floor(Date.now() / 1000) < this.lockedOutUntilSec;
  }

  /** Clear the inline error the moment the user starts typing again.
   *  Avoids a stale "wrong passphrase, 3 left" lingering while the
   *  user re-types the correct one. */
  private _onPassphraseInput = () => {
    if (this.unlockError.length > 0 && !this._isLockedOut()) {
      this.unlockError = "";
    }
  };

  /** Enter-key submits the unlock form from inside the passphrase
   *  input — keeps the keyboard-only flow snappy without forcing the
   *  user to tab to the button. */
  private _onPassphraseKeydown = (ev: KeyboardEvent) => {
    if (ev.key === "Enter") {
      ev.preventDefault();
      void this._onWreathIn();
    }
  };

  /** Retry click handler — re-runs the state derivation so the gate
   *  picks up whichever state is now correct. */
  async _onRetry(): Promise<void> {
    this.requestId = "";
    await this._deriveState();
  }

  render() {
    if (this.state === "ok") return nothing;

    const body =
      this.state === "auth"  ? this._auth()  :
      this.state === "error" ? this._error() :
                                this._setup();

    return renderChrome({
      embedded: this.embedded,
      title:    this.title,
      subtitle: this.subtitle ||
        (this.state === "setup" ? "first-run · setup"
        : this.state === "auth" ? "sign in · local keypair"
                                 : "unauthorised · 401"),
      w: this.w,
      h: this.h,
      body,
      footer: html`
        <span style="color:var(--fg-3);">no PII leaves this Mac · keypair stored in macOS Keychain · ⌘, settings</span>
      `,
    });
  }

  /* ── setup · no Lethean Account on this Mac ──────────────────────── */
  private _setup(): TemplateResult {
    return html`
      <div style="flex:1; display:flex; flex-direction:column; align-items:center; padding:40px 56px 32px; gap:22px;">
        <div style="display:flex; flex-direction:column; align-items:center; gap:14px; text-align:center;">
          <div style="width:60px; height:60px; border-radius:16px;
                      background:linear-gradient(155deg, rgba(64,193,197,0.18), rgba(64,193,197,0.02));
                      border:1px solid rgba(64,193,197,0.24);
                      display:flex; align-items:center; justify-content:center;">
            <lthn-glyph size="32" color="var(--brand-300)" active></lthn-glyph>
          </div>
          <div style="font-size:22px; font-weight:600; color:var(--fg-0); letter-spacing:-0.02em;">
            Welcome. Let's set you up.
          </div>
          <div style="font-size:13px; color:var(--fg-2); line-height:1.55; max-width:480px;">
            Choose a passphrase to protect your Lethean Account on this Mac. Nothing leaves your machine — no account, no email, nothing on a server.
          </div>
        </div>

        <form @submit=${(ev: Event) => { ev.preventDefault(); void this._onRunSetup(); }}
              style="display:flex; flex-direction:column; gap:12px; width:100%; max-width:380px;">
          <label for=${SETUP_PASSPHRASE_INPUT_ID}
            style="font-family:var(--font-mono); font-size:10px; color:var(--fg-3); letter-spacing:0.06em; text-transform:uppercase;">
            Passphrase
          </label>
          <div style="display:flex; align-items:center; gap:0; background:rgba(0,0,0,0.30);
                      border:1px solid rgba(64,193,197,0.30); border-radius:8px;
                      padding:0 12px; height:40px;">
            <i class="fa-solid fa-lock" style="font-size:11px; color:var(--fg-3); margin-right:10px;"></i>
            <input id=${SETUP_PASSPHRASE_INPUT_ID} type="password"
              placeholder="at least 12 characters"
              autocomplete="new-password" minlength="12" required
              ?disabled=${this.loading}
              @input=${this._onSetupInput}
              style="flex:1; background:transparent; border:none; outline:none;
                     color:var(--fg-0); font-family:var(--font-mono); font-size:13px; letter-spacing:0.04em;" />
          </div>

          <label for=${SETUP_CONFIRM_INPUT_ID}
            style="font-family:var(--font-mono); font-size:10px; color:var(--fg-3); letter-spacing:0.06em; text-transform:uppercase;">
            Confirm passphrase
          </label>
          <div style="display:flex; align-items:center; gap:0; background:rgba(0,0,0,0.30);
                      border:1px solid rgba(64,193,197,0.30); border-radius:8px;
                      padding:0 12px; height:40px;">
            <i class="fa-solid fa-lock" style="font-size:11px; color:var(--fg-3); margin-right:10px;"></i>
            <input id=${SETUP_CONFIRM_INPUT_ID} type="password"
              placeholder="type the same passphrase again"
              autocomplete="new-password" minlength="12" required
              ?disabled=${this.loading}
              @input=${this._onSetupInput}
              style="flex:1; background:transparent; border:none; outline:none;
                     color:var(--fg-0); font-family:var(--font-mono); font-size:13px; letter-spacing:0.04em;" />
          </div>

          ${this.setupError ? html`
            <div role="alert" style="font-size:11.5px; color:var(--err-400); line-height:1.5;">
              ${this.setupError}
            </div>` : nothing}

          <div style="font-size:11px; color:var(--fg-3); line-height:1.55;">
            Non-Latin scripts are normalised before storage so the same passphrase
            typed on any keyboard works. We can't recover it for you — write it
            somewhere safe.
          </div>

          <div style="display:flex; gap:10px; align-items:center; margin-top:4px;">
            <lthn-btn tone="primary" size="lg" type="submit"
              ?disabled=${this.loading}
              @click=${() => this._onRunSetup()}>
              <i class="fa-solid fa-play" style="font-size:11px;"></i>
              ${this.loading ? "Setting up…" : "Run setup"}
            </lthn-btn>
            <div style="font-family:var(--font-mono); font-size:10.5px; color:var(--fg-3); letter-spacing:0.04em;">
              or <span style="color:var(--fg-2);">lthn setup</span> in a terminal
            </div>
          </div>
        </form>
      </div>
    `;
  }

  /** Clear the inline setup error the moment the user starts typing
   *  again — mirrors _onPassphraseInput for the unlock form. */
  private _onSetupInput = () => {
    if (this.setupError.length > 0) this.setupError = "";
  };

  /* ── auth · account on disk, just needs the passphrase ───────────── */
  private _auth(): TemplateResult {
    return html`
      <div style="flex:1; display:grid; grid-template-columns: 320px 1fr; min-height:0;">
        <aside style="background:rgba(0,0,0,0.20); border-right:1px solid rgba(255,255,255,0.05);
                      padding:28px 26px; display:flex; flex-direction:column; gap:18px;">
          <lthn-glyph size="28" color="var(--brand-300)" active></lthn-glyph>
          <div>
            <div style="font-family:var(--font-mono); font-size:10px; color:var(--fg-3); letter-spacing:0.10em; text-transform:uppercase; margin-bottom:6px;">
              Signed-out
            </div>
            <div style="font-size:18px; font-weight:600; color:var(--fg-0); letter-spacing:-0.015em; line-height:1.25;">
              Welcome back.
            </div>
            <div style="font-size:12.5px; color:var(--fg-2); line-height:1.6; margin-top:8px;">
              Your keypair is on this Mac. Unlock it to wreath back into the
              runner — the rest of the app is locked behind this step.
            </div>
          </div>
          <div style="margin-top:auto; padding-top:14px; border-top:1px solid rgba(255,255,255,0.05); font-size:11px; color:var(--fg-3); line-height:1.55;">
            <div style="display:flex; align-items:center; gap:8px;">
              <lthn-status-dot variant="warn"></lthn-status-dot>
              <span style="color:var(--warning-400); font-family:var(--font-mono); font-size:10px; letter-spacing:0.06em;">UNAUTHORISED · 401</span>
            </div>
            <div style="margin-top:6px;">missing authorization header</div>
            ${this.requestId ? html`
              <div style="margin-top:6px; font-family:var(--font-mono); font-size:9.5px; color:var(--fg-3); word-break:break-all;">
                req · ${this.requestId}
              </div>` : nothing}
          </div>
        </aside>
        <main style="padding:48px 56px; display:flex; flex-direction:column; gap:18px; min-width:0;">
          <div>
            <div style="font-family:var(--font-mono); font-size:10px; color:var(--brand-300); letter-spacing:0.10em; text-transform:uppercase;">
              Sign in
            </div>
            <div style="font-size:20px; font-weight:600; color:var(--fg-0); margin-top:6px; letter-spacing:-0.015em;">
              Your Lethean Account
            </div>
          </div>
          <div style="display:flex; flex-direction:column; gap:10px; max-width:380px;">
            <label for=${UNLOCK_INPUT_ID} style="font-family:var(--font-mono); font-size:10px; color:var(--fg-3); letter-spacing:0.06em; text-transform:uppercase;">
              Passphrase
            </label>
            <div style="display:flex; align-items:center; gap:0; background:rgba(0,0,0,0.30);
                        border:1px solid rgba(64,193,197,0.30); border-radius:8px;
                        padding:0 12px; height:40px;">
              <i class="fa-solid fa-lock" style="font-size:11px; color:var(--fg-3); margin-right:10px;"></i>
              <input id=${UNLOCK_INPUT_ID} type="password" placeholder="••••••••••••"
                autocomplete="current-password"
                ?disabled=${this.loading || this._isLockedOut()}
                @input=${this._onPassphraseInput}
                @keydown=${this._onPassphraseKeydown}
                style="flex:1; background:transparent; border:none; outline:none;
                       color:var(--fg-0); font-family:var(--font-mono); font-size:13px; letter-spacing:0.04em;" />
              <i class="fa-regular fa-eye" style="font-size:11px; color:var(--fg-3); margin-left:8px; cursor:pointer;"></i>
            </div>
            ${this.unlockError ? html`
              <div role="alert" style="font-size:11.5px; color:var(--err-400); line-height:1.5; margin-top:2px;">
                ${this.unlockError}
              </div>` : nothing}
            <label style="display:flex; align-items:center; gap:8px; font-size:11.5px; color:var(--fg-2); margin-top:2px;">
              <!-- Cerberus #1492 (A1) — default UNCHECKED. Passphrase-once is
                   weaker than passphrase-per-launch; making it the default would
                   be a manipulative-default reducing the user's security posture
                   without them choosing it. The user opts in. -->
              <input type="checkbox" style="accent-color:var(--brand-400);">
              Remember on this Mac · keep authed in Keychain
            </label>
          </div>
          <div style="display:flex; gap:10px; align-items:center; margin-top:2px;">
            <lthn-btn tone="primary" size="lg"
              ?disabled=${this.loading || this._isLockedOut()}
              @click=${() => this._onWreathIn()}>
              <i class="fa-solid fa-arrow-right-to-bracket" style="font-size:11px;"></i>
              ${this.loading ? "Unlocking…" : "Wreath in"}
            </lthn-btn>
            <lthn-btn tone="quiet" size="lg">Use a different keypair…</lthn-btn>
          </div>
          <div style="margin-top:auto; padding-top:18px; border-top:1px solid rgba(255,255,255,0.04);
                      font-size:11px; color:var(--fg-3); line-height:1.6; max-width:460px;">
            Forgotten? Your passphrase decrypts the local keypair · we can't
            reset it. Run <span style="font-family:var(--font-mono); color:var(--fg-2);">lthn setup --reset</span>
            in a terminal to start fresh.
          </div>
        </main>
      </div>
    `;
  }

  /* ── error · framed raw response with retry ──────────────────────── */
  private _error(): TemplateResult {
    // Use the captured real response when available; fall back to the
    // design mockup so the gate still renders something coherent when
    // state=error was fired by a CustomEvent without fetch context.
    const status   = this.errorStatus > 0 ? this.errorStatus : 401;
    const statusLabel = status === 401 ? "unauthorised"
                      : status === 403 ? "forbidden"
                      : status === 404 ? "not found"
                      : status === 422 ? "unprocessable"
                      : status === 503 ? "server misconfigured"
                      : "error";
    const headline = this.errorMessage
                  || (status === 401 ? "Missing authorization header."
                     : status === 422 ? "The request was rejected."
                     : status === 503 ? "Server isn't fully configured."
                     : `Request failed (${status}).`);
    const body = this.errorBody
              || `{"error":{"code":"unauthorised","message":"missing authorization header"},"meta":{"request_id":"${this.requestId || "—"}"},"success":false}`;
    return html`
      <div style="flex:1; display:flex; flex-direction:column; padding:32px 36px; gap:18px; min-height:0;">
        <div style="display:flex; align-items:center; gap:12px;">
          <lthn-status-dot variant="err"></lthn-status-dot>
          <div style="font-family:var(--font-mono); font-size:11px; color:var(--err-400); letter-spacing:0.10em; text-transform:uppercase;">
            ${status} · ${statusLabel}
          </div>
        </div>
        <div style="font-size:18px; font-weight:600; color:var(--fg-0); letter-spacing:-0.015em;">
          ${headline}
        </div>
        <div style="font-size:13px; color:var(--fg-2); line-height:1.6; max-width:560px;">
          The runner is up but the request didn't carry credentials. This usually
          means the keypair on this Mac hasn't been unlocked this session, or the
          Keychain entry expired.
        </div>
        <div style="background:rgba(0,0,0,0.30); border:1px solid rgba(255,255,255,0.06);
                    border-radius:8px; padding:14px 16px;
                    font-family:var(--font-mono); font-size:11px; color:var(--fg-2);
                    line-height:1.7; overflow:auto; white-space:pre-wrap;">${body}</div>
        <div style="display:flex; gap:10px;">
          <lthn-btn tone="primary" size="lg" @click=${() => this._onWreathIn()}>
            <i class="fa-solid fa-arrow-right-to-bracket" style="font-size:11px;"></i>
            Wreath in
          </lthn-btn>
          <lthn-btn tone="ghost" size="lg" @click=${() => this._onRetry()}>
            <i class="fa-solid fa-arrow-rotate-right" style="font-size:11px;"></i>
            Retry
          </lthn-btn>
          <lthn-btn tone="quiet" size="lg">Copy request id</lthn-btn>
        </div>
      </div>
    `;
  }
}

customElements.define("lthn-auth-gate", LthnAuthGate);

// SPDX-Licence-Identifier: EUPL-1.2
//
// <lthn-auth-gate> tests — pre-condition gate that mounts ahead of any
// view that needs a signed-in Lethean Account. Covers:
//
//   - the 4 render states (setup / auth / error / ok) render without
//     crashing + carry the design copy verbatim;
//   - state derivation via the AccountStatus binding flips between
//     setup and auth based on `has_user_account`;
//   - the lthn:auth:401 window event transitions any non-ok state
//     into "error" + captures the requestId from the event detail;
//   - "Run setup" click handler mints a bootstrap token + POSTs
//     /v1/account/create, and on success dispatches lthn:auth:ok;
//   - **Cerberus #1465 (load-bearing)** — the bootstrap token NEVER
//     leaks to localStorage / sessionStorage / cookie / IndexedDB /
//     `this.*` field. Closure-scope storage only. The verifier-side
//     TTL is short precisely because we re-mint per attempt.

import { describe, it, expect, beforeEach, vi } from "vitest";

// Stub the @desktop/serverkey/service module BEFORE importing the
// element under test — Vitest hoists vi.mock to the top of the file
// so the mock factory wins over the real binding module.
vi.mock("@desktop/serverkey/service", () => ({
  AccountStatus:                vi.fn(),
  IssueBootstrapToken:          vi.fn(),
  IssueBootstrapTokenForScope:  vi.fn(),
  Bootstrap:                    vi.fn(),
  VerifyBootstrapToken:         vi.fn(),
  WAccountStatus:               vi.fn(),
  WIssueBootstrapToken:         vi.fn(),
}));

import { AccountStatus, IssueBootstrapToken, IssueBootstrapTokenForScope } from "@desktop/serverkey/service";
import { AUTH_401_EVENT, AUTH_OK_EVENT } from "./auth-gate";
import { setSessionToken } from "./api-fetch";
import "./auth-gate";

type GateEl = HTMLElement & {
  state: "setup" | "auth" | "error" | "ok";
  requestId: string;
  loading: boolean;
  embedded: boolean;
  accountId: string;
  unlockError: string;
  setupError: string;
  lockedOutUntilSec: number;
  updateComplete: Promise<boolean>;
  _onRunSetup(): Promise<void>;
  _onWreathIn(): Promise<void>;
  _deriveState(): Promise<void>;
};

async function mount(attrs: Record<string, string | boolean> = {}): Promise<GateEl> {
  const el = document.createElement("lthn-auth-gate") as GateEl;
  for (const [k, v] of Object.entries(attrs)) {
    if (typeof v === "boolean") { if (v) el.setAttribute(k, ""); }
    else el.setAttribute(k, v);
  }
  document.body.appendChild(el);
  // connectedCallback awaits AccountStatus → settle the microtask
  // queue plus a macrotask so the mocked promise resolves before
  // assertions read this.state.
  await el.updateComplete;
  await new Promise((r) => setTimeout(r, 0));
  await el.updateComplete;
  return el;
}

function asMockFn<T>(fn: T): {
  mockReset: () => void;
  mockResolvedValue: (v: unknown) => void;
  mockRejectedValue: (v: unknown) => void;
  mockResolvedValueOnce: (v: unknown) => void;
} {
  return fn as unknown as {
    mockReset: () => void;
    mockResolvedValue: (v: unknown) => void;
    mockRejectedValue: (v: unknown) => void;
    mockResolvedValueOnce: (v: unknown) => void;
  };
}

describe("<lthn-auth-gate> — state-machine renders", () => {
  beforeEach(() => {
    asMockFn(AccountStatus).mockReset();
    asMockFn(IssueBootstrapToken).mockReset();
    document.body.innerHTML = "";
    // No account on disk by default — derivation lands us in setup.
    asMockFn(AccountStatus).mockResolvedValue({ OK: true, Value: { has_user_account: false } });
  });

  it("setup state renders the Welcome copy + Run setup button", async () => {
    const el = await mount();
    expect(el.state).toBe("setup");
    expect(el.textContent).toContain("Welcome. Let's set you up.");
    expect(el.textContent).toContain("Run setup");
    // Footer copy is load-bearing per the design — verbatim.
    expect(el.textContent).toContain("no PII leaves this Mac");
  });

  it("auth state renders Welcome-back + passphrase field", async () => {
    asMockFn(AccountStatus).mockResolvedValue({ OK: true, Value: { has_user_account: true } });
    const el = await mount();
    expect(el.state).toBe("auth");
    expect(el.textContent).toContain("Welcome back.");
    expect(el.textContent).toContain("Passphrase");
    expect(el.textContent).toContain("Wreath in");
    // UI-text-as-offer discipline — uses "Lethean Account", not
    // implementation terms.
    expect(el.textContent).toContain("Lethean Account");
    expect(el.textContent).not.toContain("PGP");
  });

  it("error state renders the framed 401 envelope + Retry", async () => {
    const el = await mount();
    el.state = "error";
    el.requestId = "req-test-abc123";
    await el.updateComplete;
    expect(el.textContent).toContain("401 · unauthorised");
    expect(el.textContent).toContain("Missing authorization header");
    expect(el.textContent).toContain("req-test-abc123");
    expect(el.textContent).toContain("Retry");
  });

  it("ok state renders nothing (underlying view mounts)", async () => {
    const el = await mount();
    el.state = "ok";
    await el.updateComplete;
    // `nothing` collapses to empty content — the gate stops painting
    // when state=ok so the underlying view slot can mount cleanly.
    expect((el.textContent ?? "").trim()).toBe("");
  });

  it("respects the embedded attribute (chrome wrap toggle)", async () => {
    // Embedded-mode skips the standalone-card titlebar/footer so the
    // gate fills its parent. Standalone-mode renders the full chrome.
    const standalone = await mount();
    expect(standalone.querySelector("header")).not.toBeNull();

    document.body.innerHTML = "";
    const embedded = await mount({ embedded: true });
    // Embedded renderChrome skips the <header> titlebar.
    expect(embedded.querySelector("header")).toBeNull();
  });
});

describe("<lthn-auth-gate> — state derivation from AccountStatus binding", () => {
  beforeEach(() => {
    asMockFn(AccountStatus).mockReset();
    asMockFn(IssueBootstrapToken).mockReset();
    document.body.innerHTML = "";
  });

  it("has_user_account=false → state=setup", async () => {
    asMockFn(AccountStatus).mockResolvedValue({ OK: true, Value: { has_user_account: false } });
    const el = await mount();
    expect(el.state).toBe("setup");
  });

  it("has_user_account=true → state=auth", async () => {
    asMockFn(AccountStatus).mockResolvedValue({ OK: true, Value: { has_user_account: true } });
    const el = await mount();
    expect(el.state).toBe("auth");
  });

  it("binding rejection leaves the default state unchanged", async () => {
    asMockFn(AccountStatus).mockRejectedValue(new Error("no binding"));
    const el = await mount();
    // Default constructor state is "setup"; a binding-rejection MUST
    // not silently flip us into "auth" — that would strand the gate
    // with no passphrase to unlock against.
    expect(el.state).toBe("setup");
  });

  it("Result.OK=false leaves state unchanged (degraded backend)", async () => {
    asMockFn(AccountStatus).mockResolvedValue({ OK: false, Value: { error: "boom" } });
    const el = await mount();
    expect(el.state).toBe("setup");
  });
});

describe("<lthn-auth-gate> — 401 listener", () => {
  beforeEach(() => {
    asMockFn(AccountStatus).mockReset();
    asMockFn(IssueBootstrapToken).mockReset();
    document.body.innerHTML = "";
    asMockFn(AccountStatus).mockResolvedValue({ OK: true, Value: { has_user_account: true } });
  });

  it("AUTH_401_EVENT transitions state to error", async () => {
    const el = await mount();
    expect(el.state).toBe("auth");
    window.dispatchEvent(new CustomEvent(AUTH_401_EVENT, { detail: { requestId: "req-401-xyz" } }));
    await el.updateComplete;
    expect(el.state).toBe("error");
    expect(el.requestId).toBe("req-401-xyz");
  });

  it("AUTH_401_EVENT detached on disconnectedCallback", async () => {
    const el = await mount();
    el.remove();
    // After teardown, dispatching the event must NOT mutate the
    // detached element. We assert by re-reading state — happy-dom
    // keeps the JS object around, so we can still inspect it.
    window.dispatchEvent(new CustomEvent(AUTH_401_EVENT, { detail: { requestId: "req-after-teardown" } }));
    expect(el.requestId).not.toBe("req-after-teardown");
  });
});

// ─── AUTH_LOCK listener (Hephaestus #105 — Stage E follow-on B) ───────

import { AUTH_LOCK_EVENT } from "./api-fetch";

describe("TestAuthGate_AfterSignOut_RendersAuthState_Good", () => {
  // The Sign-Out button in app-shell dispatches AUTH_LOCK_EVENT after
  // POSTing /v1/account/lock. The gate listens for it and re-derives
  // state via AccountStatus — landing on "auth" when the account
  // still exists on disk (the common case). This is distinct from the
  // 401 listener (state=error / framed-failure surface) because the
  // cause is user-initiated, not server-rejected.
  beforeEach(() => {
    asMockFn(AccountStatus).mockReset();
    asMockFn(IssueBootstrapToken).mockReset();
    asMockFn(IssueBootstrapTokenForScope).mockReset();
    document.body.innerHTML = "";
    asMockFn(AccountStatus).mockResolvedValue({
      OK: true,
      Value: { has_user_account: true, account_id: "act-test" },
    });
  });

  it("AUTH_LOCK_EVENT triggers state=auth re-derivation when account exists", async () => {
    const el = await mount();
    expect(el.state).toBe("auth");
    // Force state into "ok" so we can prove the listener re-derives
    // (gate normally gets here after a successful unlock flips it).
    el.state = "ok";
    await el.updateComplete;

    // Arm a fresh response for the AUTH_LOCK re-derivation.
    asMockFn(AccountStatus).mockResolvedValueOnce({
      OK: true,
      Value: { has_user_account: true, account_id: "act-test" },
    });

    window.dispatchEvent(new CustomEvent(AUTH_LOCK_EVENT));
    // _onAuthLock awaits _deriveState — settle microtasks + a tick.
    await new Promise((r) => setTimeout(r, 0));
    await el.updateComplete;
    expect(el.state).toBe("auth");
  });

  it("AUTH_LOCK_EVENT after account-was-deleted lands on setup not auth", async () => {
    // Edge: user deleted the account out-of-band between unlock and
    // sign-out. Re-deriving picks up has_user_account=false and the
    // user lands on the setup surface — better than a stale auth
    // screen that would just bounce on the next unlock attempt.
    const el = await mount();
    el.state = "ok";
    await el.updateComplete;

    asMockFn(AccountStatus).mockResolvedValueOnce({
      OK: true,
      Value: { has_user_account: false },
    });

    window.dispatchEvent(new CustomEvent(AUTH_LOCK_EVENT));
    await new Promise((r) => setTimeout(r, 0));
    await el.updateComplete;
    expect(el.state).toBe("setup");
  });

  it("AUTH_LOCK_EVENT listener detached on disconnectedCallback", async () => {
    const el = await mount();
    const stateBefore = el.state;
    el.remove();
    // After teardown, dispatching AUTH_LOCK must NOT mutate the
    // detached element's state (the AccountStatus mock would otherwise
    // resolve and re-set it).
    asMockFn(AccountStatus).mockResolvedValueOnce({
      OK: true,
      Value: { has_user_account: false },
    });
    window.dispatchEvent(new CustomEvent(AUTH_LOCK_EVENT));
    await new Promise((r) => setTimeout(r, 0));
    expect(el.state).toBe(stateBefore);
  });
});

describe("<lthn-auth-gate> — Run setup click handler (Stage X.B Phase 3)", () => {
  // Stage X.B replaces the empty-body /v1/account/create flow with
  // /v1/account/provision — the gate now collects a passphrase +
  // confirm, mints an unlock-scope bootstrap token, and posts the
  // body the server-side Service.Provision expects.
  const SESSION = "LTHN-SESS-1.headerstub.sigstub";
  const PROV_TOKEN = "LTHN-BOOT-1.provision.attempt1";
  const PASSPHRASE = "correct horse battery staple";

  // Helper — type a passphrase into the setup form before submitting.
  // Required because the new handler reads from the DOM input, not
  // from a property on the element.
  function typeSetupForm(el: HTMLElement, pp: string, cc?: string) {
    const ppInput = el.querySelector("#lthn-auth-gate-setup-passphrase") as HTMLInputElement;
    const ccInput = el.querySelector("#lthn-auth-gate-setup-confirm") as HTMLInputElement;
    ppInput.value = pp;
    ccInput.value = cc ?? pp;
  }

  beforeEach(() => {
    asMockFn(AccountStatus).mockReset();
    asMockFn(IssueBootstrapToken).mockReset();
    asMockFn(IssueBootstrapTokenForScope).mockReset();
    document.body.innerHTML = "";
    asMockFn(AccountStatus).mockResolvedValue({ OK: true, Value: { has_user_account: false } });
    asMockFn(IssueBootstrapTokenForScope).mockResolvedValue({
      OK: true, Value: { token: PROV_TOKEN, expires_at: Date.now() / 1000 + 60 },
    });
    // coreapi.Response[T] real wire shape — `{success, data}` per
    // Mantis #1515 (lowercase `data`, never PascalCase `Value`). The
    // earlier `Value:` fixture passed only because the parser carried
    // a dead `?? json?.Value ?? json?.value` fallback chain that's now
    // collapsed to `json?.data` only.
    globalThis.fetch = vi.fn(async () =>
      new Response(JSON.stringify({
        success: true,
        data: { session_token: SESSION, expires_at: Date.now() / 1000 + 900, account_id: "abc123" },
      }), { status: 200, headers: { "Content-Type": "application/json" } })
    ) as unknown as typeof fetch;
    setSessionToken("");
  });

  it("mints provision-scoped bootstrap token + POSTs /v1/account/provision", async () => {
    const el = await mount();
    typeSetupForm(el, PASSPHRASE);
    await el._onRunSetup();
    await el.updateComplete;

    expect(IssueBootstrapTokenForScope).toHaveBeenCalledWith("account.provision");
    const fetchMock = globalThis.fetch as unknown as { mock: { calls: unknown[][] } };
    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(url).toBe("/v1/account/provision");
    expect(init.method).toBe("POST");
    const headers = new Headers(init.headers);
    expect(headers.get("Authorization")).toBe("Bootstrap " + PROV_TOKEN);
    const body = JSON.parse(init.body as string) as { passphrase: string; request_id: string };
    expect(body.passphrase).toBe(PASSPHRASE);
    expect(body.request_id).toMatch(/.+/); // crypto.randomUUID() or fallback shape
  });

  it("on 200 transitions to ok + dispatches lthn:auth:ok + arms session token", async () => {
    const el = await mount();
    typeSetupForm(el, PASSPHRASE);
    let okFired = false;
    window.addEventListener(AUTH_OK_EVENT, () => { okFired = true; }, { once: true });

    await el._onRunSetup();
    await el.updateComplete;

    expect(el.state).toBe("ok");
    expect(okFired).toBe(true);
  });

  it("on passphrase mismatch — does NOT fetch + shows inline error", async () => {
    const el = await mount();
    typeSetupForm(el, "passphrase-A12345", "passphrase-B12345");
    await el._onRunSetup();
    await el.updateComplete;

    expect(globalThis.fetch).not.toHaveBeenCalled();
    expect(el.setupError).toContain("don't match");
    expect(el.state).toBe("setup");
  });

  it("on short passphrase — does NOT fetch + shows inline error", async () => {
    const el = await mount();
    typeSetupForm(el, "tiny", "tiny");
    await el._onRunSetup();
    await el.updateComplete;

    expect(globalThis.fetch).not.toHaveBeenCalled();
    expect(el.setupError).toContain("at least 12");
    expect(el.state).toBe("setup");
  });

  it("on empty passphrase — does NOT fetch + shows inline error", async () => {
    const el = await mount();
    // Don't type anything — submit with empty inputs.
    await el._onRunSetup();
    await el.updateComplete;
    expect(globalThis.fetch).not.toHaveBeenCalled();
    expect(el.setupError).toContain("Enter a passphrase");
  });

  it("on non-2xx — falls back to framed error view", async () => {
    globalThis.fetch = vi.fn(async () =>
      new Response(JSON.stringify({
        success: false,
        error: { code: "account.provision.encrypt_failed", message: "keygen blew up" },
      }), { status: 500 })
    ) as unknown as typeof fetch;
    const el = await mount();
    typeSetupForm(el, PASSPHRASE);
    await el._onRunSetup();
    await el.updateComplete;
    expect(el.state).toBe("error");
  });

  it("on IssueBootstrapTokenForScope failure transitions to error", async () => {
    asMockFn(IssueBootstrapTokenForScope).mockResolvedValue({ OK: false, Value: { error: "mint failed" } });
    const el = await mount();
    typeSetupForm(el, PASSPHRASE);
    await el._onRunSetup();
    await el.updateComplete;
    expect(el.state).toBe("error");
  });
});

describe("<lthn-auth-gate> — Cerberus #1465 (Stage X.B): passphrase + session-token closure-scope", () => {
  // Load-bearing security assertion extended to Stage X.B. The typed
  // passphrase AND the issued session-token MUST live ONLY in handler-
  // local vars / api-fetch module-scope. NO localStorage, NO sessionStorage,
  // NO cookie, NO element field. Guards against future "cache for
  // convenience" refactors that defeat the closure-only discipline.
  const TOKEN = "LTHN-BOOT-1.cerberus1465.canary";
  const SESSION_CANARY = "LTHN-SESS-1.cerberus1465.session.canary";
  const PASSPHRASE_CANARY = "passphrase-canary-cerberus-1465-setup";

  function typeSetupForm(el: HTMLElement, pp: string, cc?: string) {
    const ppInput = el.querySelector("#lthn-auth-gate-setup-passphrase") as HTMLInputElement;
    const ccInput = el.querySelector("#lthn-auth-gate-setup-confirm") as HTMLInputElement;
    ppInput.value = pp;
    ccInput.value = cc ?? pp;
  }

  beforeEach(() => {
    asMockFn(AccountStatus).mockReset();
    asMockFn(IssueBootstrapToken).mockReset();
    asMockFn(IssueBootstrapTokenForScope).mockReset();
    document.body.innerHTML = "";
    localStorage.clear();
    sessionStorage.clear();
    document.cookie.split(";").forEach(c => {
      const eq = c.indexOf("=");
      const name = (eq > -1 ? c.substr(0, eq) : c).trim();
      if (name) document.cookie = `${name}=; expires=Thu, 01 Jan 1970 00:00:00 GMT; path=/`;
    });
    asMockFn(AccountStatus).mockResolvedValue({ OK: true, Value: { has_user_account: false } });
    asMockFn(IssueBootstrapTokenForScope).mockResolvedValue({
      OK: true, Value: { token: TOKEN, expires_at: Date.now() / 1000 + 60 },
    });
    // Real coreapi.Response[T] wire shape — `{success, data}` per Mantis #1515.
    globalThis.fetch = vi.fn(async () =>
      new Response(JSON.stringify({
        success: true,
        data: { session_token: SESSION_CANARY, expires_at: Date.now() / 1000 + 900, account_id: "abc123" },
      }), { status: 200, headers: { "Content-Type": "application/json" } })
    ) as unknown as typeof fetch;
    setSessionToken("");
  });

  it("bootstrap token does NOT persist to localStorage", async () => {
    const el = await mount();
    typeSetupForm(el, PASSPHRASE_CANARY);
    await el._onRunSetup();
    await el.updateComplete;
    for (let i = 0; i < localStorage.length; i++) {
      const k = localStorage.key(i);
      if (!k) continue;
      expect(localStorage.getItem(k) ?? "").not.toContain(TOKEN);
    }
    expect(localStorage.getItem("lthn.auth.bootstrap.token")).toBeNull();
  });

  it("session token does NOT persist to localStorage", async () => {
    const el = await mount();
    typeSetupForm(el, PASSPHRASE_CANARY);
    await el._onRunSetup();
    await el.updateComplete;
    for (let i = 0; i < localStorage.length; i++) {
      const k = localStorage.key(i);
      if (!k) continue;
      expect(localStorage.getItem(k) ?? "").not.toContain(SESSION_CANARY);
    }
  });

  it("passphrase does NOT persist to localStorage / sessionStorage / cookie / element", async () => {
    const el = await mount();
    typeSetupForm(el, PASSPHRASE_CANARY);
    await el._onRunSetup();
    await el.updateComplete;

    for (let i = 0; i < localStorage.length; i++) {
      const k = localStorage.key(i);
      if (!k) continue;
      expect(localStorage.getItem(k) ?? "").not.toContain(PASSPHRASE_CANARY);
    }
    for (let i = 0; i < sessionStorage.length; i++) {
      const k = sessionStorage.key(i);
      if (!k) continue;
      expect(sessionStorage.getItem(k) ?? "").not.toContain(PASSPHRASE_CANARY);
    }
    expect(document.cookie).not.toContain(PASSPHRASE_CANARY);
    for (const k of Object.keys(el)) {
      const v = (el as unknown as Record<string, unknown>)[k];
      if (typeof v === "string") expect(v).not.toContain(PASSPHRASE_CANARY);
    }
    // After state=ok the gate stops rendering — the form unmounts so
    // the inputs are gone. The closure-only invariant is satisfied by
    // (a) success-path clearing input.value before state flip and (b)
    // pp + cc local vars falling out of scope in the handler's finally.
  });

  it("element instance has NO bootstrap-token field after the handler resolves", async () => {
    const el = await mount();
    typeSetupForm(el, PASSPHRASE_CANARY);
    await el._onRunSetup();
    await el.updateComplete;
    const keys = Object.keys(el);
    for (const k of keys) {
      expect(k.toLowerCase()).not.toMatch(/bootstraptoken|boottoken|authtoken|token$/);
    }
    for (const k of keys) {
      const v = (el as unknown as Record<string, unknown>)[k];
      if (typeof v === "string") {
        expect(v).not.toContain(TOKEN);
      }
    }
  });

  it("retry re-mints — never re-uses a prior token", async () => {
    asMockFn(IssueBootstrapTokenForScope).mockResolvedValueOnce({
      OK: true, Value: { token: "LTHN-BOOT-1.first.attempt", expires_at: Date.now() / 1000 + 60 },
    });
    asMockFn(IssueBootstrapTokenForScope).mockResolvedValueOnce({
      OK: true, Value: { token: "LTHN-BOOT-1.second.attempt", expires_at: Date.now() / 1000 + 60 },
    });
    // First attempt — endpoint returns 500 so we end up in error.
    globalThis.fetch = vi.fn(async () => new Response("boom", { status: 500 })) as unknown as typeof fetch;
    const el = await mount();
    typeSetupForm(el, PASSPHRASE_CANARY);
    await el._onRunSetup();
    await el.updateComplete;
    expect(el.state).toBe("error");

    // Second attempt — endpoint now returns 200. Re-type because the
    // input was cleared on the error path (defence-in-depth).
    globalThis.fetch = vi.fn(async () =>
      new Response(JSON.stringify({
        success: true,
        data: { session_token: SESSION_CANARY, expires_at: Date.now() / 1000 + 900, account_id: "abc123" },
      }), { status: 200, headers: { "Content-Type": "application/json" } })
    ) as unknown as typeof fetch;
    // After state=error the gate stops rendering the setup form;
    // re-derive forces it back. (In prod the user clicks Retry.)
    el.state = "setup";
    await el.updateComplete;
    typeSetupForm(el, PASSPHRASE_CANARY);
    await el._onRunSetup();
    await el.updateComplete;
    expect(el.state).toBe("ok");

    expect(IssueBootstrapTokenForScope).toHaveBeenCalledTimes(2);
    const fetchMock = globalThis.fetch as unknown as { mock: { calls: unknown[][] } };
    const [, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    const headers = new Headers(init.headers);
    expect(headers.get("Authorization")).toBe("Bootstrap LTHN-BOOT-1.second.attempt");
  });
});

describe("<lthn-auth-gate> — Wreath-in unlock flow (RFC.stage-e §5)", () => {
  const SESSION = "LTHN-SESS-1.headerstub.sigstub";
  const UNLOCK_TOKEN = "LTHN-BOOT-1.unlock.attempt1";
  const ACCOUNT_ID = "abc123def4567890";

  beforeEach(() => {
    asMockFn(AccountStatus).mockReset();
    asMockFn(IssueBootstrapToken).mockReset();
    asMockFn(IssueBootstrapTokenForScope).mockReset();
    document.body.innerHTML = "";
    asMockFn(AccountStatus).mockResolvedValue({
      OK: true, Value: { has_user_account: true, account_id: ACCOUNT_ID },
    });
    asMockFn(IssueBootstrapTokenForScope).mockResolvedValue({
      OK: true, Value: { token: UNLOCK_TOKEN, expires_at: Date.now() / 1000 + 60 },
    });
    // Real coreapi.Response[T] wire shape — `{success, data}` per Mantis #1515.
    globalThis.fetch = vi.fn(async () =>
      new Response(JSON.stringify({
        success: true,
        data: { session_token: SESSION, expires_at: Date.now() / 1000 + 900, account_id: ACCOUNT_ID },
      }), { status: 200, headers: { "Content-Type": "application/json" } })
    ) as unknown as typeof fetch;
    // Clear any session-token from prior tests in this run.
    setSessionToken("");
  });

  it("captures account_id from AccountStatus", async () => {
    const el = await mount();
    expect(el.state).toBe("auth");
    expect(el.accountId).toBe(ACCOUNT_ID);
  });

  it("on click — mints unlock-scoped bootstrap token + POSTs /v1/account/unlock", async () => {
    const el = await mount();
    const inp = el.querySelector("#lthn-auth-gate-passphrase") as HTMLInputElement;
    inp.value = "correct horse battery staple";

    await el._onWreathIn();
    await el.updateComplete;

    expect(IssueBootstrapTokenForScope).toHaveBeenCalledWith("account.unlock");
    const fetchMock = globalThis.fetch as unknown as { mock: { calls: unknown[][] } };
    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(url).toBe("/v1/account/unlock");
    expect(init.method).toBe("POST");
    const headers = new Headers(init.headers);
    expect(headers.get("Authorization")).toBe("Bootstrap " + UNLOCK_TOKEN);
    const body = JSON.parse(init.body as string) as { account_id: string; passphrase: string };
    expect(body.account_id).toBe(ACCOUNT_ID);
    expect(body.passphrase).toBe("correct horse battery staple");
  });

  it("on 200 — sets session token, dispatches lthn:auth:ok, state=ok", async () => {
    const el = await mount();
    let okFired = false;
    window.addEventListener(AUTH_OK_EVENT, () => { okFired = true; }, { once: true });
    const inp = el.querySelector("#lthn-auth-gate-passphrase") as HTMLInputElement;
    inp.value = "correct horse battery staple";

    await el._onWreathIn();
    await el.updateComplete;

    expect(el.state).toBe("ok");
    expect(okFired).toBe(true);
  });

  it("on 401 bad_passphrase — stays on auth + shows inline error", async () => {
    globalThis.fetch = vi.fn(async () =>
      new Response(JSON.stringify({
        success: false,
        error: {
          code: "account.unlock.bad_passphrase",
          message: "passphrase didn't unlock your account — 3 attempts remaining",
        },
      }), { status: 401 })
    ) as unknown as typeof fetch;
    const el = await mount();
    (el.querySelector("#lthn-auth-gate-passphrase") as HTMLInputElement).value = "wrong";

    await el._onWreathIn();
    await el.updateComplete;

    expect(el.state).toBe("auth");
    expect(el.unlockError).toContain("3 attempts remaining");
    // Input cleared so a stale value can't re-submit on next click.
    expect((el.querySelector("#lthn-auth-gate-passphrase") as HTMLInputElement).value).toBe("");
  });

  it("on 429 locked_out — disables button + records cooldown deadline", async () => {
    globalThis.fetch = vi.fn(async () =>
      new Response(JSON.stringify({
        success: false,
        error: {
          code: "account.unlock.locked_out",
          message: "too many attempts — try again in 60 seconds",
        },
      }), { status: 429 })
    ) as unknown as typeof fetch;
    const el = await mount();
    (el.querySelector("#lthn-auth-gate-passphrase") as HTMLInputElement).value = "wrong";

    await el._onWreathIn();
    await el.updateComplete;

    expect(el.state).toBe("auth");
    expect(el.unlockError).toContain("60 seconds");
    expect(el.lockedOutUntilSec).toBeGreaterThan(Math.floor(Date.now() / 1000));
  });

  it("on empty passphrase — does not call the endpoint", async () => {
    const el = await mount();
    await el._onWreathIn();
    expect(globalThis.fetch).not.toHaveBeenCalled();
    expect(el.unlockError).toContain("Enter your passphrase");
  });

  it("on 5xx — flips to error state with captured body", async () => {
    globalThis.fetch = vi.fn(async () =>
      new Response(JSON.stringify({
        success: false,
        error: { code: "account.unlock.corrupted_key", message: "couldn't read your account file" },
      }), { status: 500 })
    ) as unknown as typeof fetch;
    const el = await mount();
    (el.querySelector("#lthn-auth-gate-passphrase") as HTMLInputElement).value = "anything";

    await el._onWreathIn();
    await el.updateComplete;

    expect(el.state).toBe("error");
  });

  it("Cerberus #1465 (applied to passphrase) — never persists the typed passphrase", async () => {
    // The passphrase MUST stay closure-only — same discipline as the
    // bootstrap-token. Type a canary, fire the handler, and assert no
    // storage / element field / cookie carries the canary substring.
    const CANARY = "passphrase-canary-cerberus-1465-extended";
    const el = await mount();
    const inp = el.querySelector("#lthn-auth-gate-passphrase") as HTMLInputElement;
    inp.value = CANARY;

    await el._onWreathIn();
    await el.updateComplete;

    // localStorage clean
    for (let i = 0; i < localStorage.length; i++) {
      const k = localStorage.key(i);
      if (!k) continue;
      expect(localStorage.getItem(k) ?? "").not.toContain(CANARY);
    }
    // sessionStorage clean
    for (let i = 0; i < sessionStorage.length; i++) {
      const k = sessionStorage.key(i);
      if (!k) continue;
      expect(sessionStorage.getItem(k) ?? "").not.toContain(CANARY);
    }
    // No own-enumerable property on the element holds the canary.
    for (const k of Object.keys(el)) {
      const v = (el as unknown as Record<string, unknown>)[k];
      if (typeof v === "string") expect(v).not.toContain(CANARY);
    }
    // document.cookie clean.
    expect(document.cookie).not.toContain(CANARY);
    // Input field was cleared on success path (or non-OK path).
    expect(inp.value).not.toBe(CANARY);
  });

  it("Cerberus #1465 (applied to session token) — token never persists to storage", async () => {
    const el = await mount();
    (el.querySelector("#lthn-auth-gate-passphrase") as HTMLInputElement).value = "anything";
    await el._onWreathIn();
    await el.updateComplete;

    // The session token MUST stay in api-fetch.ts module-scope only.
    // No localStorage / sessionStorage / cookie / element-field leak.
    for (let i = 0; i < localStorage.length; i++) {
      const k = localStorage.key(i);
      if (!k) continue;
      expect(localStorage.getItem(k) ?? "").not.toContain(SESSION);
    }
    for (let i = 0; i < sessionStorage.length; i++) {
      const k = sessionStorage.key(i);
      if (!k) continue;
      expect(sessionStorage.getItem(k) ?? "").not.toContain(SESSION);
    }
    expect(document.cookie).not.toContain(SESSION);
    for (const k of Object.keys(el)) {
      const v = (el as unknown as Record<string, unknown>)[k];
      if (typeof v === "string") expect(v).not.toContain(SESSION);
    }
  });
});

// Mantis #1492 (A1 consent-floor) regression guard — the "Store passphrase
// in Keychain" checkbox in Stage C MUST default UNCHECKED so the user
// explicitly opts in to Keychain persistence rather than having
// passphrase-per-launch silently weakened to passphrase-once-then-Keychain
// by a default-on visual contract. Cerberus #1492 finding.
//
// Scope note: the Stage D wiring (checkbox change → keychain persist) is
// NOT yet implemented — `_onWreathIn` is the Stage C no-op for this
// control. Tests for "opt-in fires keychain persist" + "unchecked does not
// touch keychain" land alongside that wiring in a separate ticket; the
// regression guard here is the structural one Cerberus called out (default
// state + label wording) so the consent default can't silently flip back.
describe("TestAuthGate_StageC_RememberCheckboxDefaultUnchecked_Good (Mantis #1492)", () => {
  beforeEach(() => {
    asMockFn(AccountStatus).mockReset();
    asMockFn(IssueBootstrapToken).mockReset();
    asMockFn(IssueBootstrapTokenForScope).mockReset();
    document.body.innerHTML = "";
    asMockFn(AccountStatus).mockResolvedValue({
      OK: true, Value: { has_user_account: true, account_id: "abc123" },
    });
  });

  it("checkbox renders unchecked by default in state=auth", async () => {
    const el = await mount();
    expect(el.state).toBe("auth");
    const cb = el.querySelector(
      "[data-testid='lthn-auth-gate-keychain-remember']",
    ) as HTMLInputElement | null;
    expect(cb).not.toBeNull();
    expect(cb!.type).toBe("checkbox");
    // Default state — Cerberus #1492 A1 finding: opt-in not opt-out.
    expect(cb!.checked).toBe(false);
    expect(cb!.hasAttribute("checked")).toBe(false);
  });

  it("label uses the flat trade wording, not 'Remember' framing", async () => {
    const el = await mount();
    // Cerberus #1492 recommendation #4 — surface the trade verbatim so
    // the user reads it as a security choice, not a convenience toggle.
    expect(el.textContent).toContain("Store passphrase in Keychain");
    expect(el.textContent).toContain("skip this prompt next launch");
    // The biased "Remember on this Mac · keep authed in Keychain" framing
    // is gone — guard against regression.
    expect(el.textContent).not.toContain("Remember on this Mac");
    expect(el.textContent).not.toContain("keep authed in Keychain");
  });
});

// Mantis #1515 regression guard — the parser ONLY accepts the canonical
// coreapi.Response[T] wire shape `{success, data}`. The historical dead
// shape `{success, Value}` (PascalCase, never emitted by coreapi) MUST
// surface as an error rather than silently parse the wrong shape into
// `undefined`. Without this guard a future "be helpful" refactor could
// re-introduce the `?? json?.Value` fall-through and the bug would not
// be visible to existing happy-path tests.
describe("TestAuthGate_ResponseShape_Bad — dead coreapi shape surfaces (Mantis #1515)", () => {
  const PASSPHRASE = "correct horse battery staple";
  const ACCOUNT_ID = "abc123def4567890";
  const PROV_TOKEN = "LTHN-BOOT-1.provision.bad";
  const UNLOCK_TOKEN = "LTHN-BOOT-1.unlock.bad";

  function typeSetupForm(el: HTMLElement, pp: string) {
    const ppInput = el.querySelector("#lthn-auth-gate-setup-passphrase") as HTMLInputElement;
    const ccInput = el.querySelector("#lthn-auth-gate-setup-confirm") as HTMLInputElement;
    ppInput.value = pp;
    ccInput.value = pp;
  }

  beforeEach(() => {
    asMockFn(AccountStatus).mockReset();
    asMockFn(IssueBootstrapToken).mockReset();
    asMockFn(IssueBootstrapTokenForScope).mockReset();
    document.body.innerHTML = "";
    setSessionToken("");
  });

  it("provision flow rejects dead {success, Value} shape (no silent success)", async () => {
    asMockFn(AccountStatus).mockResolvedValue({ OK: true, Value: { has_user_account: false } });
    asMockFn(IssueBootstrapTokenForScope).mockResolvedValue({
      OK: true, Value: { token: PROV_TOKEN, expires_at: Date.now() / 1000 + 60 },
    });
    // Server "returns" the dead PascalCase shape. The collapsed parser
    // (`json?.data` only) MUST surface error — not extract
    // session_token from `Value` via a fallback chain.
    globalThis.fetch = vi.fn(async () =>
      new Response(JSON.stringify({
        success: true,
        Value: { session_token: "LTHN-SESS-1.must-not-load", account_id: ACCOUNT_ID },
      }), { status: 200, headers: { "Content-Type": "application/json" } })
    ) as unknown as typeof fetch;

    const el = await mount();
    typeSetupForm(el, PASSPHRASE);
    await el._onRunSetup();
    await el.updateComplete;

    expect(el.state).toBe("error");
    expect(el.state).not.toBe("ok");
  });

  it("unlock flow rejects dead {success, value} shape (no silent success)", async () => {
    asMockFn(AccountStatus).mockResolvedValue({
      OK: true, Value: { has_user_account: true, account_id: ACCOUNT_ID },
    });
    asMockFn(IssueBootstrapTokenForScope).mockResolvedValue({
      OK: true, Value: { token: UNLOCK_TOKEN, expires_at: Date.now() / 1000 + 60 },
    });
    // Server "returns" the dead lowercase-value shape. Same invariant —
    // parser MUST NOT walk a fallback chain into `value`.
    globalThis.fetch = vi.fn(async () =>
      new Response(JSON.stringify({
        success: true,
        value: { session_token: "LTHN-SESS-1.must-not-load", account_id: ACCOUNT_ID },
      }), { status: 200, headers: { "Content-Type": "application/json" } })
    ) as unknown as typeof fetch;

    const el = await mount();
    (el.querySelector("#lthn-auth-gate-passphrase") as HTMLInputElement).value = PASSPHRASE;
    await el._onWreathIn();
    await el.updateComplete;

    expect(el.state).toBe("error");
    expect(el.state).not.toBe("ok");
  });
});

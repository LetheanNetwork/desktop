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
  AccountStatus:        vi.fn(),
  IssueBootstrapToken:  vi.fn(),
  Bootstrap:            vi.fn(),
  VerifyBootstrapToken: vi.fn(),
  WAccountStatus:       vi.fn(),
  WIssueBootstrapToken: vi.fn(),
}));

import { AccountStatus, IssueBootstrapToken } from "@desktop/serverkey/service";
import { AUTH_401_EVENT, AUTH_OK_EVENT } from "./auth-gate";
import "./auth-gate";

type GateEl = HTMLElement & {
  state: "setup" | "auth" | "error" | "ok";
  requestId: string;
  loading: boolean;
  embedded: boolean;
  updateComplete: Promise<boolean>;
  _onRunSetup(): Promise<void>;
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

describe("<lthn-auth-gate> — Run setup click handler", () => {
  beforeEach(() => {
    asMockFn(AccountStatus).mockReset();
    asMockFn(IssueBootstrapToken).mockReset();
    document.body.innerHTML = "";
    asMockFn(AccountStatus).mockResolvedValue({ OK: true, Value: { has_user_account: false } });
    asMockFn(IssueBootstrapToken).mockResolvedValue({
      OK: true,
      Value: { token: "LTHN-BOOT-1.headerstub.sigstub", expires_at: Date.now() / 1000 + 60 },
    });
    // Mock global fetch so /v1/account/create doesn't escape into the
    // real network during the test run. Returns 200 by default.
    globalThis.fetch = vi.fn(async () => new Response("{}", { status: 200 })) as unknown as typeof fetch;
  });

  it("mints a bootstrap token and POSTs /v1/account/create", async () => {
    const el = await mount();
    await el._onRunSetup();
    await el.updateComplete;

    expect(IssueBootstrapToken).toHaveBeenCalledTimes(1);
    expect(globalThis.fetch).toHaveBeenCalledTimes(1);

    const fetchMock = globalThis.fetch as unknown as { mock: { calls: unknown[][] } };
    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(url).toBe("/v1/account/create");
    expect(init.method).toBe("POST");
    // Authorization carries the raw token under the "Bootstrap" scheme.
    // Cerberus #1469 — client sends raw, verifier canonicalises.
    const headers = new Headers(init.headers);
    expect(headers.get("Authorization")).toBe("Bootstrap LTHN-BOOT-1.headerstub.sigstub");
  });

  it("on 200 transitions to ok + dispatches lthn:auth:ok", async () => {
    const el = await mount();
    let okFired = false;
    window.addEventListener(AUTH_OK_EVENT, () => { okFired = true; }, { once: true });
    await el._onRunSetup();
    await el.updateComplete;
    expect(el.state).toBe("ok");
    expect(okFired).toBe(true);
  });

  it("on non-2xx (e.g. 404 before Stage B' lands) falls back to error", async () => {
    globalThis.fetch = vi.fn(async () => new Response("not found", { status: 404 })) as unknown as typeof fetch;
    const el = await mount();
    await el._onRunSetup();
    await el.updateComplete;
    expect(el.state).toBe("error");
  });

  it("on IssueBootstrapToken failure transitions to error", async () => {
    asMockFn(IssueBootstrapToken).mockResolvedValue({ OK: false, Value: { error: "mint failed" } });
    const el = await mount();
    await el._onRunSetup();
    await el.updateComplete;
    expect(el.state).toBe("error");
  });
});

describe("<lthn-auth-gate> — Cerberus #1465: bootstrap-token closure-scope only", () => {
  // Load-bearing security assertion. The bootstrap token MUST live
  // ONLY in the local variable inside _onRunSetup. After the handler
  // resolves there must be:
  //   - no token in localStorage under any key with prefix lthn.auth
  //   - no token in sessionStorage under any key with prefix lthn.auth
  //   - no token in document.cookie
  //   - no token field on the element instance (Object.keys check)
  // This guards against future refactors that "cache the token for
  // convenience" — that's the exact pattern Cerberus rejected.
  const TOKEN = "LTHN-BOOT-1.cerberus1465.canary";

  beforeEach(() => {
    asMockFn(AccountStatus).mockReset();
    asMockFn(IssueBootstrapToken).mockReset();
    document.body.innerHTML = "";
    localStorage.clear();
    sessionStorage.clear();
    // Wipe any cookies that may have leaked from prior tests in the
    // same happy-dom document. document.cookie="" doesn't clear; we
    // have to iterate and expire each.
    document.cookie.split(";").forEach(c => {
      const eq = c.indexOf("=");
      const name = (eq > -1 ? c.substr(0, eq) : c).trim();
      if (name) document.cookie = `${name}=; expires=Thu, 01 Jan 1970 00:00:00 GMT; path=/`;
    });
    asMockFn(AccountStatus).mockResolvedValue({ OK: true, Value: { has_user_account: false } });
    asMockFn(IssueBootstrapToken).mockResolvedValue({
      OK: true,
      Value: { token: TOKEN, expires_at: Date.now() / 1000 + 60 },
    });
    globalThis.fetch = vi.fn(async () => new Response("{}", { status: 200 })) as unknown as typeof fetch;
  });

  it("token does NOT persist to localStorage", async () => {
    const el = await mount();
    await el._onRunSetup();
    await el.updateComplete;
    // Bulk check — no localStorage key holds the canary token.
    for (let i = 0; i < localStorage.length; i++) {
      const k = localStorage.key(i);
      if (!k) continue;
      expect(localStorage.getItem(k) ?? "").not.toContain(TOKEN);
    }
    // Spec-named keys explicitly absent.
    expect(localStorage.getItem("lthn.auth.bootstrap.token")).toBeNull();
    expect(localStorage.getItem("lthn.auth.token")).toBeNull();
  });

  it("token does NOT persist to sessionStorage", async () => {
    const el = await mount();
    await el._onRunSetup();
    await el.updateComplete;
    for (let i = 0; i < sessionStorage.length; i++) {
      const k = sessionStorage.key(i);
      if (!k) continue;
      expect(sessionStorage.getItem(k) ?? "").not.toContain(TOKEN);
    }
    expect(sessionStorage.getItem("lthn.auth.bootstrap.token")).toBeNull();
  });

  it("token does NOT leak into document.cookie", async () => {
    const el = await mount();
    await el._onRunSetup();
    await el.updateComplete;
    expect(document.cookie).not.toContain(TOKEN);
    expect(document.cookie).not.toContain("bootstrap_token");
  });

  it("element instance has NO bootstrap-token field after the handler resolves", async () => {
    const el = await mount();
    await el._onRunSetup();
    await el.updateComplete;
    const keys = Object.keys(el);
    // No own-enumerable property whose name suggests a cached token.
    for (const k of keys) {
      expect(k.toLowerCase()).not.toMatch(/bootstraptoken|boottoken|authtoken|token$/);
    }
    // And explicitly no value-leak via any own property.
    for (const k of keys) {
      const v = (el as unknown as Record<string, unknown>)[k];
      if (typeof v === "string") {
        expect(v).not.toContain(TOKEN);
      }
    }
  });

  it("retry re-mints — never re-uses a prior token", async () => {
    asMockFn(IssueBootstrapToken).mockResolvedValueOnce({
      OK: true, Value: { token: "LTHN-BOOT-1.first.attempt", expires_at: Date.now() / 1000 + 60 },
    });
    asMockFn(IssueBootstrapToken).mockResolvedValueOnce({
      OK: true, Value: { token: "LTHN-BOOT-1.second.attempt", expires_at: Date.now() / 1000 + 60 },
    });
    // First attempt — endpoint returns 500 so we end up in error.
    globalThis.fetch = vi.fn(async () => new Response("boom", { status: 500 })) as unknown as typeof fetch;
    const el = await mount();
    await el._onRunSetup();
    await el.updateComplete;
    expect(el.state).toBe("error");

    // Second attempt — endpoint now returns 200.
    globalThis.fetch = vi.fn(async () => new Response("{}", { status: 200 })) as unknown as typeof fetch;
    await el._onRunSetup();
    await el.updateComplete;
    expect(el.state).toBe("ok");

    // Each attempt minted a fresh token — verifier-side TTL discipline
    // is preserved.
    expect(IssueBootstrapToken).toHaveBeenCalledTimes(2);
    const fetchMock = globalThis.fetch as unknown as { mock: { calls: unknown[][] } };
    const [, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    const headers = new Headers(init.headers);
    expect(headers.get("Authorization")).toBe("Bootstrap LTHN-BOOT-1.second.attempt");
  });
});

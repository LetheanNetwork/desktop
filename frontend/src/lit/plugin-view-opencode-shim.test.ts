// SPDX-Licence-Identifier: EUPL-1.2
//
// Tests for <lthn-plugin-view-opencode-shim> per
// plans/code/lthn/desktop/views/RFC.plugin-views.md §5.1 host-side
// handshake responsibilities + Cerberus HIGH-2 inbound origin
// verification.
//
// Coverage:
//   - TestOpencodeShim_GrantsOnValidRequest_Good
//   - TestOpencodeShim_DeniesOnOriginMismatch_Bad (HIGH-2)
//   - TestOpencodeShim_DeniesOnUndeclaredCapability_Bad
//   - TestOpencodeShim_DeniesWhenNoSession_Bad (closure-only token absent)
//   - TestOpencodeShim_DeniesUnknownScope_Bad (forward-contract)
//   - TestOpencodeShim_IgnoresUnrelatedMessages_Good
//   - Standard four-spec wire shape per design_lit_view_backend_wire_pattern:
//     rows-replace (descriptor passes through), reject-keeps (no descriptor
//     → silent), empty-keeps (no descriptor → no broker), fixtures-still-pass
//     (rendering remains the inner <lthn-plugin-view>).

import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";

vi.mock("@desktop/marketplace/service", () => ({
  GetViewDescriptor: vi.fn(),
}));

// apiFetch is mocked at module scope so every test can swap the
// response shape per scenario via mockResolvedValueOnce. The default
// resolves to a 200 OK so the existing grant-path tests continue to
// fire postMessage after the audit-grant POST succeeds (Mantis #1523).
vi.mock("./api-fetch", () => ({
  apiFetch: vi.fn(async () => new Response("{}", { status: 200 })),
}));

import { mountWindow } from "../test/window-fixture";
import {
  AUTH_REQUEST_TYPE,
  AUTH_GRANT_TYPE,
  AUTH_DENY_TYPE,
  AUTH_GRANTED_EVENT,
  AUTH_DENIED_EVENT,
  CAPABILITY_GRANT_AUDIT_PATH,
  DENY_REASON_AUDIT_FAILED,
  DENY_REASON_ORIGIN_MISMATCH,
  DENY_REASON_NO_SESSION,
  DENY_REASON_UNKNOWN_SCOPE,
  DENY_REASON_CAPABILITY_NOT_DECLARED,
  type OpenCodeShimElement,
  type TokenProvider,
} from "./plugin-view-opencode-shim";
import "./plugin-view-opencode-shim";
import { apiFetch } from "./api-fetch";
import type { PluginViewDescriptor } from "./lthn-plugin-view";

// flushMicrotasks drains queued promise resolutions so the post-await
// branch in _grant runs before the assertions inspect calls/audit.
// jsdom MessageEvent dispatch returns synchronously but _grant POSTs
// then awaits; one macrotask boundary lets the fetch promise resolve
// and the postMessage/audit-dispatch path run.
const flushMicrotasks = () =>
  new Promise<void>((r) => setTimeout(r, 0));

const FIXTURE_DESCRIPTOR: PluginViewDescriptor = {
  id: "opencode",
  label: "OpenCode",
  icon: "fa-robot",
  group: "plugin",
  kind: "iframe",
  source: "/opencode",
  pluginCode: "opencode",
  loopbackPort: 4096,
  loopbackOrigin: "http://127.0.0.1:4096",
  capabilities: ["session-token"],
};

// stubIframeContentWindow swaps the inner iframe.contentWindow with a
// MessagePort-shaped fake that records postMessage payloads. The real
// iframe in jsdom carries a window object whose .postMessage is a
// no-op spy — we replace it so the test can assert what the shim sent.
function stubIframeContentWindow(
  shim: OpenCodeShimElement,
): { calls: { data: unknown; targetOrigin: string }[]; fakeWindow: Window } {
  const iframe = shim.querySelector<HTMLIFrameElement>("iframe");
  if (!iframe) throw new Error("test: inner iframe not present yet");
  const calls: { data: unknown; targetOrigin: string }[] = [];
  const fakeWindow = {
    postMessage(data: unknown, targetOrigin: string): void {
      calls.push({ data, targetOrigin });
    },
  } as unknown as Window;
  Object.defineProperty(iframe, "contentWindow", {
    configurable: true,
    get: () => fakeWindow,
  });
  return { calls, fakeWindow };
}

// fireAuthRequest dispatches a synthetic MessageEvent at window with
// the given (source, origin, data). MessageEvent doesn't accept .source
// as a constructor arg in jsdom, so we use Object.defineProperty to
// override the read-only field.
function fireAuthRequest(
  source: Window,
  origin: string,
  data: unknown,
): void {
  const ev = new MessageEvent("message", { data, origin });
  Object.defineProperty(ev, "source", { configurable: true, get: () => source });
  window.dispatchEvent(ev);
}

beforeEach(() => {
  // Reset the apiFetch mock between tests so a mockResolvedValueOnce
  // from one test doesn't leak its response shape into the next.
  vi.mocked(apiFetch).mockReset();
  vi.mocked(apiFetch).mockResolvedValue(
    new Response("{}", { status: 200 }),
  );
});

afterEach(() => {
  vi.mocked(apiFetch).mockReset();
});

// ─── Grant path ────────────────────────────────────────────────────────

describe("TestOpencodeShim_GrantsOnValidRequest_Good", () => {
  it("posts a grant payload with the brokered token when scopes match", async () => {
    const tokenProvider: TokenProvider = () => "LTHN-SESS-1.test-token";
    const { el } = await mountWindow<OpenCodeShimElement>(
      "lthn-plugin-view-opencode-shim",
      { props: { descriptor: FIXTURE_DESCRIPTOR, tokenProvider } },
    );
    await el.updateComplete;
    // Inner <lthn-plugin-view> needs a microtask flush to render the
    // iframe; advance the queue so querySelector("iframe") resolves.
    const inner = el.querySelector("lthn-plugin-view") as HTMLElement & {
      updateComplete: Promise<boolean>;
    };
    await inner.updateComplete;

    const { calls, fakeWindow } = stubIframeContentWindow(el);

    const audit: CustomEvent[] = [];
    const listener = (e: Event) => { audit.push(e as CustomEvent); };
    window.addEventListener(AUTH_GRANTED_EVENT, listener);

    fireAuthRequest(fakeWindow, "http://127.0.0.1:4096", {
      type: AUTH_REQUEST_TYPE,
      scopes: ["session-token"],
    });
    // _grant POSTs then awaits the audit-grant response before
    // postMessage'ing the token. Drain microtasks so the post-await
    // branch runs before assertions inspect the recorded calls.
    await flushMicrotasks();

    window.removeEventListener(AUTH_GRANTED_EVENT, listener);

    // Mantis #1523 — audit-grant POST fires BEFORE the postMessage,
    // carries plugin_id + capability + origin. Token bytes never
    // appear in the request body.
    expect(vi.mocked(apiFetch)).toHaveBeenCalledTimes(1);
    const [auditPath, auditInit] = vi.mocked(apiFetch).mock.calls[0]!;
    expect(auditPath).toBe(CAPABILITY_GRANT_AUDIT_PATH);
    expect(auditInit?.method).toBe("POST");
    const auditBody = JSON.parse(String(auditInit?.body ?? "{}"));
    expect(auditBody.plugin_id).toBe("opencode");
    expect(auditBody.capability).toBe("session-token");
    expect(auditBody.origin).toBe("http://127.0.0.1:4096");
    expect(JSON.stringify(auditBody)).not.toContain("LTHN-SESS-1");

    expect(calls.length).toBe(1);
    expect(calls[0]?.targetOrigin).toBe("http://127.0.0.1:4096");
    const payload = calls[0]?.data as { type: string; token: string; scopes: string[] };
    expect(payload.type).toBe(AUTH_GRANT_TYPE);
    expect(payload.token).toBe("LTHN-SESS-1.test-token");
    expect(payload.scopes).toEqual(["session-token"]);

    // Audit event fires WITHOUT the token bytes (Cerberus #1468).
    expect(audit.length).toBe(1);
    const detail = audit[0]?.detail as { viewId: string; pluginCode: string; scopes: string[] };
    expect(detail.viewId).toBe("opencode");
    expect(detail.pluginCode).toBe("opencode");
    expect(detail.scopes).toEqual(["session-token"]);
    expect(JSON.stringify(detail)).not.toContain("LTHN-SESS-1");
  });
});

// ─── Origin verification (HIGH-2) ──────────────────────────────────────

describe("TestOpencodeShim_DeniesOnOriginMismatch_Bad", () => {
  it("denies a request whose event.origin does NOT match descriptor.loopbackOrigin", async () => {
    const tokenProvider: TokenProvider = () => "LTHN-SESS-1.test-token";
    const { el } = await mountWindow<OpenCodeShimElement>(
      "lthn-plugin-view-opencode-shim",
      { props: { descriptor: FIXTURE_DESCRIPTOR, tokenProvider } },
    );
    const inner = el.querySelector("lthn-plugin-view") as HTMLElement & {
      updateComplete: Promise<boolean>;
    };
    await inner.updateComplete;

    const { calls, fakeWindow } = stubIframeContentWindow(el);

    const audit: CustomEvent[] = [];
    const listener = (e: Event) => { audit.push(e as CustomEvent); };
    window.addEventListener(AUTH_DENIED_EVENT, listener);

    // Same source-window, WRONG origin (attacker-controlled port).
    fireAuthRequest(fakeWindow, "http://127.0.0.1:9999", {
      type: AUTH_REQUEST_TYPE,
      scopes: ["session-token"],
    });

    window.removeEventListener(AUTH_DENIED_EVENT, listener);

    expect(calls.length).toBe(1);
    const payload = calls[0]?.data as { type: string; reason: string };
    expect(payload.type).toBe(AUTH_DENY_TYPE);
    expect(payload.reason).toBe(DENY_REASON_ORIGIN_MISMATCH);
    // Deny posts back to the descriptor's expected origin, NOT the
    // attacker's origin — otherwise we'd be leaking deny replies to
    // arbitrary loopback ports.
    expect(calls[0]?.targetOrigin).toBe("http://127.0.0.1:4096");

    expect(audit.length).toBe(1);
    const detail = audit[0]?.detail as { reason: string };
    expect(detail.reason).toBe(DENY_REASON_ORIGIN_MISMATCH);
  });
});

// ─── Undeclared capability ─────────────────────────────────────────────

describe("TestOpencodeShim_DeniesOnUndeclaredCapability_Bad", () => {
  it("denies a scope not present in descriptor.capabilities", async () => {
    const noCapDescriptor: PluginViewDescriptor = {
      ...FIXTURE_DESCRIPTOR,
      capabilities: [], // empty — no scopes granted
    };
    const tokenProvider: TokenProvider = () => "LTHN-SESS-1.test-token";
    const { el } = await mountWindow<OpenCodeShimElement>(
      "lthn-plugin-view-opencode-shim",
      { props: { descriptor: noCapDescriptor, tokenProvider } },
    );
    const inner = el.querySelector("lthn-plugin-view") as HTMLElement & {
      updateComplete: Promise<boolean>;
    };
    await inner.updateComplete;

    const { calls, fakeWindow } = stubIframeContentWindow(el);

    fireAuthRequest(fakeWindow, "http://127.0.0.1:4096", {
      type: AUTH_REQUEST_TYPE,
      scopes: ["session-token"],
    });

    expect(calls.length).toBe(1);
    const payload = calls[0]?.data as { type: string; reason: string };
    expect(payload.type).toBe(AUTH_DENY_TYPE);
    expect(payload.reason).toBe(DENY_REASON_CAPABILITY_NOT_DECLARED);
  });
});

// ─── No session ────────────────────────────────────────────────────────

describe("TestOpencodeShim_DeniesWhenNoSession_Bad", () => {
  it("denies when the tokenProvider returns null (unlocked but no session)", async () => {
    const tokenProvider: TokenProvider = () => null;
    const { el } = await mountWindow<OpenCodeShimElement>(
      "lthn-plugin-view-opencode-shim",
      { props: { descriptor: FIXTURE_DESCRIPTOR, tokenProvider } },
    );
    const inner = el.querySelector("lthn-plugin-view") as HTMLElement & {
      updateComplete: Promise<boolean>;
    };
    await inner.updateComplete;

    const { calls, fakeWindow } = stubIframeContentWindow(el);

    fireAuthRequest(fakeWindow, "http://127.0.0.1:4096", {
      type: AUTH_REQUEST_TYPE,
      scopes: ["session-token"],
    });

    expect(calls.length).toBe(1);
    const payload = calls[0]?.data as { type: string; reason: string };
    expect(payload.type).toBe(AUTH_DENY_TYPE);
    expect(payload.reason).toBe(DENY_REASON_NO_SESSION);
  });
});

// ─── Unknown scope (forward-contract rejection) ────────────────────────

describe("TestOpencodeShim_DeniesUnknownScope_Bad", () => {
  it("denies vi-events / marketplace / any non-session-token scope in v1", async () => {
    const tokenProvider: TokenProvider = () => "LTHN-SESS-1.test-token";
    const futureDescriptor: PluginViewDescriptor = {
      ...FIXTURE_DESCRIPTOR,
      capabilities: ["session-token", "vi-events"],
    };
    const { el } = await mountWindow<OpenCodeShimElement>(
      "lthn-plugin-view-opencode-shim",
      { props: { descriptor: futureDescriptor, tokenProvider } },
    );
    const inner = el.querySelector("lthn-plugin-view") as HTMLElement & {
      updateComplete: Promise<boolean>;
    };
    await inner.updateComplete;

    const { calls, fakeWindow } = stubIframeContentWindow(el);

    fireAuthRequest(fakeWindow, "http://127.0.0.1:4096", {
      type: AUTH_REQUEST_TYPE,
      scopes: ["vi-events"],
    });

    expect(calls.length).toBe(1);
    const payload = calls[0]?.data as { type: string; reason: string };
    expect(payload.type).toBe(AUTH_DENY_TYPE);
    expect(payload.reason).toBe(DENY_REASON_UNKNOWN_SCOPE);
  });
});

// ─── Audit-grant POST failure (Mantis #1523) ───────────────────────────

describe("TestOpencodeShim_DeniesWhenAuditPostFails_Bad", () => {
  it("denies and does NOT postMessage the token when the audit-grant POST returns non-200", async () => {
    // Server returns 500 — simulates audit-substrate outage. Per
    // Mantis #1523 done-criteria the shim MUST NOT proceed with
    // postMessage; the iframe sees AUTH_DENY with audit-failed reason.
    vi.mocked(apiFetch).mockResolvedValueOnce(
      new Response(`{"success":false}`, { status: 500 }),
    );

    const tokenProvider: TokenProvider = () => "LTHN-SESS-1.test-token";
    const { el } = await mountWindow<OpenCodeShimElement>(
      "lthn-plugin-view-opencode-shim",
      { props: { descriptor: FIXTURE_DESCRIPTOR, tokenProvider } },
    );
    const inner = el.querySelector("lthn-plugin-view") as HTMLElement & {
      updateComplete: Promise<boolean>;
    };
    await inner.updateComplete;

    const { calls, fakeWindow } = stubIframeContentWindow(el);

    const grantAudit: CustomEvent[] = [];
    const denyAudit: CustomEvent[] = [];
    const onGrant = (e: Event) => { grantAudit.push(e as CustomEvent); };
    const onDeny = (e: Event) => { denyAudit.push(e as CustomEvent); };
    window.addEventListener(AUTH_GRANTED_EVENT, onGrant);
    window.addEventListener(AUTH_DENIED_EVENT, onDeny);

    fireAuthRequest(fakeWindow, "http://127.0.0.1:4096", {
      type: AUTH_REQUEST_TYPE,
      scopes: ["session-token"],
    });
    await flushMicrotasks();

    window.removeEventListener(AUTH_GRANTED_EVENT, onGrant);
    window.removeEventListener(AUTH_DENIED_EVENT, onDeny);

    // The audit-grant POST fired exactly once.
    expect(vi.mocked(apiFetch)).toHaveBeenCalledTimes(1);
    // The iframe received a DENY, NOT a GRANT — token bytes never
    // crossed the boundary.
    expect(calls.length).toBe(1);
    const payload = calls[0]?.data as { type: string; reason: string };
    expect(payload.type).toBe(AUTH_DENY_TYPE);
    expect(payload.reason).toBe(DENY_REASON_AUDIT_FAILED);
    // No grant audit event; one deny audit event.
    expect(grantAudit.length).toBe(0);
    expect(denyAudit.length).toBe(1);
  });

  it("denies when the audit-grant fetch rejects (network-level failure)", async () => {
    vi.mocked(apiFetch).mockRejectedValueOnce(new Error("network down"));

    const tokenProvider: TokenProvider = () => "LTHN-SESS-1.test-token";
    const { el } = await mountWindow<OpenCodeShimElement>(
      "lthn-plugin-view-opencode-shim",
      { props: { descriptor: FIXTURE_DESCRIPTOR, tokenProvider } },
    );
    const inner = el.querySelector("lthn-plugin-view") as HTMLElement & {
      updateComplete: Promise<boolean>;
    };
    await inner.updateComplete;

    const { calls, fakeWindow } = stubIframeContentWindow(el);

    fireAuthRequest(fakeWindow, "http://127.0.0.1:4096", {
      type: AUTH_REQUEST_TYPE,
      scopes: ["session-token"],
    });
    await flushMicrotasks();

    expect(calls.length).toBe(1);
    const payload = calls[0]?.data as { type: string; reason: string };
    expect(payload.type).toBe(AUTH_DENY_TYPE);
    expect(payload.reason).toBe(DENY_REASON_AUDIT_FAILED);
  });
});

// ─── Unrelated messages ignored ────────────────────────────────────────

describe("TestOpencodeShim_IgnoresUnrelatedMessages_Good", () => {
  it("drops messages that aren't lthn:auth:request without firing audit", async () => {
    const tokenProvider: TokenProvider = () => "LTHN-SESS-1.test-token";
    const { el } = await mountWindow<OpenCodeShimElement>(
      "lthn-plugin-view-opencode-shim",
      { props: { descriptor: FIXTURE_DESCRIPTOR, tokenProvider } },
    );
    const inner = el.querySelector("lthn-plugin-view") as HTMLElement & {
      updateComplete: Promise<boolean>;
    };
    await inner.updateComplete;

    const { calls, fakeWindow } = stubIframeContentWindow(el);

    const audit: CustomEvent[] = [];
    const onGrant = (e: Event) => { audit.push(e as CustomEvent); };
    const onDeny = (e: Event) => { audit.push(e as CustomEvent); };
    window.addEventListener(AUTH_GRANTED_EVENT, onGrant);
    window.addEventListener(AUTH_DENIED_EVENT, onDeny);

    fireAuthRequest(fakeWindow, "http://127.0.0.1:4096", { type: "some-other-thing" });
    fireAuthRequest(fakeWindow, "http://127.0.0.1:4096", null);
    fireAuthRequest(fakeWindow, "http://127.0.0.1:4096", "not-an-object");

    window.removeEventListener(AUTH_GRANTED_EVENT, onGrant);
    window.removeEventListener(AUTH_DENIED_EVENT, onDeny);

    expect(calls.length).toBe(0);
    expect(audit.length).toBe(0);
  });
});

// ─── Backend wire shape (design_lit_view_backend_wire_pattern) ────────

describe("lthn-plugin-view-opencode-shim backend wire (4-spec contract)", () => {
  it("descriptor pass-through (rows-replace): inner element receives the descriptor verbatim", async () => {
    const { el } = await mountWindow<OpenCodeShimElement>(
      "lthn-plugin-view-opencode-shim",
      { props: { descriptor: FIXTURE_DESCRIPTOR } },
    );
    await el.updateComplete;
    const inner = el.querySelector("lthn-plugin-view") as HTMLElement & {
      descriptor: PluginViewDescriptor;
    };
    expect(inner).not.toBeNull();
    expect(inner.descriptor?.id).toBe("opencode");
  });

  it("missing descriptor (reject-keeps): renders inner element with viewId attr only", async () => {
    const { el } = await mountWindow<OpenCodeShimElement>(
      "lthn-plugin-view-opencode-shim",
      { attrs: { "view-id": "opencode" } },
    );
    await el.updateComplete;
    const inner = el.querySelector("lthn-plugin-view");
    expect(inner).not.toBeNull();
    expect(inner?.getAttribute("view-id")).toBe("opencode");
  });

  it("empty descriptor (empty-keeps): no handshake fires when descriptor is null", async () => {
    const tokenProvider: TokenProvider = () => "LTHN-SESS-1.test-token";
    const { el } = await mountWindow<OpenCodeShimElement>(
      "lthn-plugin-view-opencode-shim",
      { props: { tokenProvider } }, // no descriptor
    );
    await el.updateComplete;

    // Simulate a message arriving even though we have no descriptor.
    // The shim drops silently — no audit, no postMessage attempt.
    const audit: CustomEvent[] = [];
    const onGrant = (e: Event) => { audit.push(e as CustomEvent); };
    const onDeny = (e: Event) => { audit.push(e as CustomEvent); };
    window.addEventListener(AUTH_GRANTED_EVENT, onGrant);
    window.addEventListener(AUTH_DENIED_EVENT, onDeny);

    const fakeWindow = { postMessage: () => {} } as unknown as Window;
    fireAuthRequest(fakeWindow, "http://127.0.0.1:4096", {
      type: AUTH_REQUEST_TYPE, scopes: ["session-token"],
    });

    window.removeEventListener(AUTH_GRANTED_EVENT, onGrant);
    window.removeEventListener(AUTH_DENIED_EVENT, onDeny);

    expect(audit.length).toBe(0);
  });

  it("fixtures-still-pass: render returns the inner element unchanged", async () => {
    const { el } = await mountWindow<OpenCodeShimElement>(
      "lthn-plugin-view-opencode-shim",
      { props: { descriptor: FIXTURE_DESCRIPTOR } },
    );
    await el.updateComplete;
    const inner = el.querySelector("lthn-plugin-view");
    expect(inner).not.toBeNull();
    expect(el.children.length).toBe(1); // exactly one inner element rendered
  });
});

// SPDX-Licence-Identifier: EUPL-1.2
//
// Tests for <lthn-plugin-view-opencode-shim> per
// plans/code/lthn/desktop/views/RFC.plugin-views.md §5.1 host-side
// handshake responsibilities + Cerberus HIGH-2 inbound origin
// verification + RFC.plugin-view-token-boundary v2 §3.0 broker
// delegation (the shim no longer holds token bytes — it calls
// api-fetch.grantTokenToFrame for the actual release).
//
// Coverage:
//   - TestOpencodeShim_GrantsOnValidRequest_Good (broker delegation)
//   - TestOpencodeShim_DeniesOnOriginMismatch_Bad (HIGH-2)
//   - TestOpencodeShim_DeniesOnUndeclaredCapability_Bad
//   - TestOpencodeShim_DeniesWhenAuditFails_Bad (broker reports audit-failed)
//   - TestOpencodeShim_DeniesWhenNoSession_Bad (broker reports no-session)
//   - TestOpencodeShim_DeniesUnknownScope_Bad (forward-contract)
//   - TestOpencodeShim_IgnoresUnrelatedMessages_Good
//   - Standard four-spec wire shape per design_lit_view_backend_wire_pattern.

import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";

vi.mock("@desktop/marketplace/service", () => ({
  GetViewDescriptor: vi.fn(),
}));

// grantTokenToFrame is mocked at module scope so every test can swap
// the broker outcome per scenario via mockResolvedValueOnce. The
// default resolves to { ok: true } so the grant-path tests fire the
// AUTH_GRANTED CustomEvent without setting up a real session token.
// apiFetch is also mocked so any incidental fetch never escapes the
// test process.
vi.mock("./api-fetch", () => ({
  apiFetch: vi.fn(async () => new Response("{}", { status: 200 })),
  grantTokenToFrame: vi.fn(async () => ({ ok: true, scopes: ["session-token"] })),
  AUTH_GRANT_TYPE: "lthn:auth:grant",
  CAPABILITY_GRANT_AUDIT_PATH: "/v1/plugin-view/capability-grant",
}));

import { mountWindow } from "../test/window-fixture";
import {
  AUTH_REQUEST_TYPE,
  AUTH_DENY_TYPE,
  AUTH_GRANTED_EVENT,
  AUTH_DENIED_EVENT,
  DENY_REASON_AUDIT_FAILED,
  DENY_REASON_ORIGIN_MISMATCH,
  DENY_REASON_NO_SESSION,
  DENY_REASON_UNKNOWN_SCOPE,
  DENY_REASON_CAPABILITY_NOT_DECLARED,
  type OpenCodeShimElement,
} from "./plugin-view-opencode-shim";
import "./plugin-view-opencode-shim";
import { grantTokenToFrame } from "./api-fetch";
import type { PluginViewDescriptor } from "./lthn-plugin-view";

// flushMicrotasks drains queued promise resolutions so the post-await
// branch in _broker runs before the assertions inspect calls/audit.
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
// MessagePort-shaped fake. With the broker now owning postMessage,
// the fake is only useful for ev.source identity (the broker mock
// returns an outcome without touching source.postMessage).
function stubIframeContentWindow(
  shim: OpenCodeShimElement,
): { fakeWindow: Window } {
  const iframe = shim.querySelector<HTMLIFrameElement>("iframe");
  if (!iframe) throw new Error("test: inner iframe not present yet");
  const fakeWindow = {
    postMessage(_data: unknown, _targetOrigin: string): void {
      // The shim no longer postMessages directly on grant — the broker
      // does. The shim DOES still postMessage on deny.
    },
  } as unknown as Window;
  Object.defineProperty(iframe, "contentWindow", {
    configurable: true,
    get: () => fakeWindow,
  });
  return { fakeWindow };
}

// stubIframeContentWindowRecording is the deny-path variant — records
// the shim's deny postMessage calls so tests can assert the deny
// payload + targetOrigin shape.
function stubIframeContentWindowRecording(
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
// as a constructor arg in happy-dom, so we use Object.defineProperty
// to override the read-only field.
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
  vi.mocked(grantTokenToFrame).mockReset();
  vi.mocked(grantTokenToFrame).mockResolvedValue(
    { ok: true, scopes: ["session-token"] },
  );
});

afterEach(() => {
  vi.mocked(grantTokenToFrame).mockReset();
});

// ─── Grant path (broker delegation) ────────────────────────────────────

describe("TestOpencodeShim_GrantsOnValidRequest_Good", () => {
  it("delegates token release to grantTokenToFrame with (target, scopes) shape", async () => {
    const { el } = await mountWindow<OpenCodeShimElement>(
      "lthn-plugin-view-opencode-shim",
      { props: { descriptor: FIXTURE_DESCRIPTOR } },
    );
    await el.updateComplete;
    const inner = el.querySelector("lthn-plugin-view") as HTMLElement & {
      updateComplete: Promise<boolean>;
    };
    await inner.updateComplete;

    const { fakeWindow } = stubIframeContentWindow(el);

    const audit: CustomEvent[] = [];
    const listener = (e: Event) => { audit.push(e as CustomEvent); };
    window.addEventListener(AUTH_GRANTED_EVENT, listener);

    fireAuthRequest(fakeWindow, "http://127.0.0.1:4096", {
      type: AUTH_REQUEST_TYPE,
      scopes: ["session-token"],
    });
    await flushMicrotasks();

    window.removeEventListener(AUTH_GRANTED_EVENT, listener);

    // The broker was called once with the (target, scopes) shape per
    // RFC §3.0 contract item (b). Token bytes never enter the shim.
    expect(vi.mocked(grantTokenToFrame)).toHaveBeenCalledTimes(1);
    const [target, scopes] = vi.mocked(grantTokenToFrame).mock.calls[0]!;
    expect(target.source).toBe(fakeWindow);
    expect(target.targetOrigin).toBe("http://127.0.0.1:4096");
    expect(target.pluginCode).toBe("opencode");
    expect(scopes).toEqual(["session-token"]);

    // Audit CustomEvent fires WITHOUT the token bytes (Cerberus #1468).
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
    const { el } = await mountWindow<OpenCodeShimElement>(
      "lthn-plugin-view-opencode-shim",
      { props: { descriptor: FIXTURE_DESCRIPTOR } },
    );
    const inner = el.querySelector("lthn-plugin-view") as HTMLElement & {
      updateComplete: Promise<boolean>;
    };
    await inner.updateComplete;

    const { calls, fakeWindow } = stubIframeContentWindowRecording(el);

    const audit: CustomEvent[] = [];
    const listener = (e: Event) => { audit.push(e as CustomEvent); };
    window.addEventListener(AUTH_DENIED_EVENT, listener);

    // Same source-window, WRONG origin (attacker-controlled port).
    fireAuthRequest(fakeWindow, "http://127.0.0.1:9999", {
      type: AUTH_REQUEST_TYPE,
      scopes: ["session-token"],
    });

    window.removeEventListener(AUTH_DENIED_EVENT, listener);

    // Broker MUST NOT be reached for descriptor-side rejections.
    expect(vi.mocked(grantTokenToFrame)).toHaveBeenCalledTimes(0);

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
    const { el } = await mountWindow<OpenCodeShimElement>(
      "lthn-plugin-view-opencode-shim",
      { props: { descriptor: noCapDescriptor } },
    );
    const inner = el.querySelector("lthn-plugin-view") as HTMLElement & {
      updateComplete: Promise<boolean>;
    };
    await inner.updateComplete;

    const { calls, fakeWindow } = stubIframeContentWindowRecording(el);

    fireAuthRequest(fakeWindow, "http://127.0.0.1:4096", {
      type: AUTH_REQUEST_TYPE,
      scopes: ["session-token"],
    });

    expect(vi.mocked(grantTokenToFrame)).toHaveBeenCalledTimes(0);
    expect(calls.length).toBe(1);
    const payload = calls[0]?.data as { type: string; reason: string };
    expect(payload.type).toBe(AUTH_DENY_TYPE);
    expect(payload.reason).toBe(DENY_REASON_CAPABILITY_NOT_DECLARED);
  });
});

// ─── No session (broker reports no-session) ────────────────────────────

describe("TestOpencodeShim_DeniesWhenNoSession_Bad", () => {
  it("routes the broker's no-session outcome to a DENY message", async () => {
    vi.mocked(grantTokenToFrame).mockResolvedValueOnce(
      { ok: false, reason: "no-session" },
    );

    const { el } = await mountWindow<OpenCodeShimElement>(
      "lthn-plugin-view-opencode-shim",
      { props: { descriptor: FIXTURE_DESCRIPTOR } },
    );
    const inner = el.querySelector("lthn-plugin-view") as HTMLElement & {
      updateComplete: Promise<boolean>;
    };
    await inner.updateComplete;

    const { calls, fakeWindow } = stubIframeContentWindowRecording(el);

    fireAuthRequest(fakeWindow, "http://127.0.0.1:4096", {
      type: AUTH_REQUEST_TYPE,
      scopes: ["session-token"],
    });
    await flushMicrotasks();

    expect(vi.mocked(grantTokenToFrame)).toHaveBeenCalledTimes(1);
    expect(calls.length).toBe(1);
    const payload = calls[0]?.data as { type: string; reason: string };
    expect(payload.type).toBe(AUTH_DENY_TYPE);
    expect(payload.reason).toBe(DENY_REASON_NO_SESSION);
  });
});

// ─── Unknown scope (forward-contract rejection) ────────────────────────

describe("TestOpencodeShim_DeniesUnknownScope_Bad", () => {
  it("denies vi-events / marketplace / any non-session-token scope in v1", async () => {
    const futureDescriptor: PluginViewDescriptor = {
      ...FIXTURE_DESCRIPTOR,
      capabilities: ["session-token", "vi-events"],
    };
    const { el } = await mountWindow<OpenCodeShimElement>(
      "lthn-plugin-view-opencode-shim",
      { props: { descriptor: futureDescriptor } },
    );
    const inner = el.querySelector("lthn-plugin-view") as HTMLElement & {
      updateComplete: Promise<boolean>;
    };
    await inner.updateComplete;

    const { calls, fakeWindow } = stubIframeContentWindowRecording(el);

    fireAuthRequest(fakeWindow, "http://127.0.0.1:4096", {
      type: AUTH_REQUEST_TYPE,
      scopes: ["vi-events"],
    });

    expect(vi.mocked(grantTokenToFrame)).toHaveBeenCalledTimes(0);
    expect(calls.length).toBe(1);
    const payload = calls[0]?.data as { type: string; reason: string };
    expect(payload.type).toBe(AUTH_DENY_TYPE);
    expect(payload.reason).toBe(DENY_REASON_UNKNOWN_SCOPE);
  });
});

// ─── Audit-grant failure (broker reports audit-failed) ─────────────────

describe("TestOpencodeShim_DeniesWhenAuditFails_Bad", () => {
  it("routes the broker's audit-failed outcome to a DENY message", async () => {
    vi.mocked(grantTokenToFrame).mockResolvedValueOnce(
      { ok: false, reason: "audit-failed" },
    );

    const { el } = await mountWindow<OpenCodeShimElement>(
      "lthn-plugin-view-opencode-shim",
      { props: { descriptor: FIXTURE_DESCRIPTOR } },
    );
    const inner = el.querySelector("lthn-plugin-view") as HTMLElement & {
      updateComplete: Promise<boolean>;
    };
    await inner.updateComplete;

    const { calls, fakeWindow } = stubIframeContentWindowRecording(el);

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

    expect(vi.mocked(grantTokenToFrame)).toHaveBeenCalledTimes(1);
    expect(calls.length).toBe(1);
    const payload = calls[0]?.data as { type: string; reason: string };
    expect(payload.type).toBe(AUTH_DENY_TYPE);
    expect(payload.reason).toBe(DENY_REASON_AUDIT_FAILED);
    expect(grantAudit.length).toBe(0);
    expect(denyAudit.length).toBe(1);
  });
});

// ─── Unrelated messages ignored ────────────────────────────────────────

describe("TestOpencodeShim_IgnoresUnrelatedMessages_Good", () => {
  it("drops messages that aren't lthn:auth:request without firing the broker", async () => {
    const { el } = await mountWindow<OpenCodeShimElement>(
      "lthn-plugin-view-opencode-shim",
      { props: { descriptor: FIXTURE_DESCRIPTOR } },
    );
    const inner = el.querySelector("lthn-plugin-view") as HTMLElement & {
      updateComplete: Promise<boolean>;
    };
    await inner.updateComplete;

    const { calls, fakeWindow } = stubIframeContentWindowRecording(el);

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

    expect(vi.mocked(grantTokenToFrame)).toHaveBeenCalledTimes(0);
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
    const { el } = await mountWindow<OpenCodeShimElement>(
      "lthn-plugin-view-opencode-shim",
      { props: {} }, // no descriptor
    );
    await el.updateComplete;

    // Simulate a message arriving even though we have no descriptor.
    // The shim drops silently — no audit, no broker call, no postMessage.
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

    expect(vi.mocked(grantTokenToFrame)).toHaveBeenCalledTimes(0);
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

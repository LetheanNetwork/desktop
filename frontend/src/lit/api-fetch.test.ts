// SPDX-Licence-Identifier: EUPL-1.2
//
// Tests for api-fetch.ts module-graph + token-boundary invariants per
// plans/code/lthn/desktop/views/RFC.plugin-view-token-boundary.md v2 §4.
//
// The closure-only discipline introduced by Cerberus #1465 was a
// *convention* until this RFC — peekSessionToken was an exported
// function, so any importer could lift the token value out of the
// boundary. v2 turns the discipline into a *mechanism* by deleting
// the export and routing every token release through grantTokenToFrame.
//
// These tests are the load-bearing locks that prevent future code
// from regressing the boundary:
//
//   - TestApiFetch_ExportSnapshot_Good
//     Snapshots the exported surface of api-fetch.ts. Any new
//     token-shaped export (peekSessionToken re-introduced,
//     getSessionToken, readToken, etc.) fails the snapshot and forces
//     a review.
//
//   - TestApiFetch_GrantTokenToFrame_AuditBeforePostMessage_Good
//     Asserts the audit POST fires BEFORE the postMessage. Cerberus
//     #1523 + RFC §3.0 contract item (a) — the audit row MUST commit
//     server-side before token bytes cross the iframe boundary, so
//     a forged-source attempt leaves a forensic record under the
//     legitimate plugin's identity.
//
//   - TestApiFetch_CachedSessionTokenLiteralAppearsInExactlyOneFile_Good
//     Reads the source tree under frontend/src/ and asserts the
//     literal string `cachedSessionToken` appears in exactly one file
//     (api-fetch.ts). Locks the read-side structural invariant: future
//     code cannot reach into the cache by name without failing the
//     test. Where the export snapshot guards the exit-shape, this
//     guards the binding-name-reach-in.

import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { readdirSync, readFileSync, statSync } from "node:fs";
import { join, resolve } from "node:path";

describe("TestApiFetch_ExportSnapshot_Good", () => {
  it("api-fetch.ts exposes exactly the closure-boundary-respecting surface", async () => {
    const mod = await import("./api-fetch");
    const exportedNames = Object.keys(mod).sort();
    // Snapshot of the exported surface. Adding a token-shaped export
    // (peekSessionToken, getSessionToken, readToken, etc.) MUST fail
    // this snapshot — that's the point. The closure-only invariant
    // (RFC.plugin-view-token-boundary v2 §3.0) lives here.
    expect(exportedNames).toEqual([
      "AUTH_401_EVENT",
      "AUTH_GRANT_TYPE",
      "CAPABILITY_GRANT_AUDIT_PATH",
      "apiFetch",
      "clearApiToken",
      "clearSessionToken",
      "grantTokenToFrame",
      "setSessionToken",
    ]);
    // Belt-and-braces: explicit no-peek assertion so a future
    // accidental re-export surfaces with a clearer failure than the
    // snapshot diff alone.
    expect((mod as Record<string, unknown>).peekSessionToken).toBeUndefined();
  });
});

// ─── Broker audit-before-postMessage ordering ──────────────────────────

describe("TestApiFetch_GrantTokenToFrame_AuditBeforePostMessage_Good", () => {
  beforeEach(() => {
    vi.resetModules();
    vi.stubGlobal("fetch", vi.fn(async () => new Response("{}", { status: 200 })));
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    vi.resetModules();
  });

  it("audit POST invocationCallOrder strictly precedes postMessage", async () => {
    // Stub global fetch so the audit POST is observable. Bonus:
    // never escapes the test process.
    const fetchStub = vi.fn(async () => new Response("{}", { status: 200 }));
    vi.stubGlobal("fetch", fetchStub);

    const mod = await import("./api-fetch");
    mod.setSessionToken("LTHN-SESS-1.test-token");

    const postMessage = vi.fn();
    const target = {
      source: { postMessage } as unknown as Window,
      targetOrigin: "http://127.0.0.1:4096",
      pluginCode: "opencode",
    };
    const outcome = await mod.grantTokenToFrame(target, ["session-token"]);

    expect(outcome.ok).toBe(true);
    // Audit POST happened exactly once.
    expect(fetchStub).toHaveBeenCalledTimes(1);
    // postMessage happened exactly once.
    expect(postMessage).toHaveBeenCalledTimes(1);

    // Ordering: vitest's mock.invocationCallOrder is monotonically
    // increasing across all spies. RFC §3.0 item (a) — audit MUST
    // commit BEFORE the token bytes cross the boundary.
    const auditOrder = fetchStub.mock.invocationCallOrder[0];
    const postOrder = postMessage.mock.invocationCallOrder[0];
    expect(auditOrder).toBeDefined();
    expect(postOrder).toBeDefined();
    expect(auditOrder!).toBeLessThan(postOrder!);

    // Audit body carries plugin_id + capabilities array + origin +
    // outcome, NOT the token bytes (Cerberus #1465 + #1468).
    // RFC.plugin-view-audit-atomicity v2 §3.1 + §4.2 — `capabilities`
    // is ALWAYS an array, NEVER a scalar; outcome literal is "granted";
    // request body MUST NOT carry correlation_id (handler-generated).
    const auditCall = fetchStub.mock.calls[0] as unknown as [unknown, RequestInit];
    const auditInit = auditCall[1] ?? {};
    const auditBody = JSON.parse(String(auditInit.body ?? "{}"));
    expect(auditBody.plugin_id).toBe("opencode");
    expect(auditBody.capabilities).toEqual(["session-token"]);
    expect(auditBody.origin).toBe("http://127.0.0.1:4096");
    expect(auditBody.outcome).toBe("granted");
    expect(JSON.stringify(auditBody)).not.toContain("LTHN-SESS-1");
    // Legacy-shape regression guards — pre-v2 scalar `capability` field
    // and request-asserted `correlation_id` MUST NOT appear in the body.
    expect(auditBody.capability).toBeUndefined();
    expect(auditBody.correlation_id).toBeUndefined();

    // postMessage carries the actual token + targetOrigin verbatim.
    const [payload, targetOrigin] = postMessage.mock.calls[0]!;
    expect(payload.type).toBe("lthn:auth:grant");
    expect(payload.token).toBe("LTHN-SESS-1.test-token");
    expect(payload.scopes).toEqual(["session-token"]);
    expect(targetOrigin).toBe("http://127.0.0.1:4096");

    mod.clearSessionToken();
  });

  it("audit failure short-circuits — postMessage NEVER runs", async () => {
    const fetchStub = vi.fn(async () => new Response(`{"success":false}`, { status: 500 }));
    vi.stubGlobal("fetch", fetchStub);

    const mod = await import("./api-fetch");
    mod.setSessionToken("LTHN-SESS-1.test-token");

    const postMessage = vi.fn();
    const target = {
      source: { postMessage } as unknown as Window,
      targetOrigin: "http://127.0.0.1:4096",
      pluginCode: "opencode",
    };
    const outcome = await mod.grantTokenToFrame(target, ["session-token"]);

    expect(outcome).toEqual({ ok: false, reason: "audit-failed" });
    expect(fetchStub).toHaveBeenCalledTimes(1);
    expect(postMessage).toHaveBeenCalledTimes(0);

    mod.clearSessionToken();
  });

  it("no session returns no-session outcome AFTER audit commits", async () => {
    const fetchStub = vi.fn(async () => new Response("{}", { status: 200 }));
    vi.stubGlobal("fetch", fetchStub);

    const mod = await import("./api-fetch");
    mod.clearSessionToken();

    const postMessage = vi.fn();
    const target = {
      source: { postMessage } as unknown as Window,
      targetOrigin: "http://127.0.0.1:4096",
      pluginCode: "opencode",
    };
    const outcome = await mod.grantTokenToFrame(target, ["session-token"]);

    // Audit still committed (audit is the atomic single-POST and not
    // session-conditional per the broker shape).
    expect(fetchStub).toHaveBeenCalledTimes(1);
    expect(postMessage).toHaveBeenCalledTimes(0);
    expect(outcome).toEqual({ ok: false, reason: "no-session" });
  });
});

// ─── Audit body shape — RFC v2 §4.2 structural lock-in ────────────────

describe("TestApiFetch_GrantTokenToFrame_AuditBodyShape_NoCapabilityScalar_Ugly", () => {
  beforeEach(() => {
    vi.resetModules();
    vi.stubGlobal("fetch", vi.fn(async () => new Response("{}", { status: 200 })));
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    vi.resetModules();
  });

  it("serialised body does NOT contain the pre-v2 `\"capability\":` scalar substring", async () => {
    // RFC v2 §4.2 — regression guard. If a future PR re-introduces the
    // scalar `capability` field shape, this test fails. Pairs with the
    // v1 hard cutover in §5 (handler 400s the legacy shape).
    const fetchStub = vi.fn(async () => new Response("{}", { status: 200 }));
    vi.stubGlobal("fetch", fetchStub);

    const mod = await import("./api-fetch");
    mod.setSessionToken("LTHN-SESS-1.body-shape-test");

    const target = {
      source: { postMessage: vi.fn() } as unknown as Window,
      targetOrigin: "http://127.0.0.1:4096",
      pluginCode: "opencode",
    };
    await mod.grantTokenToFrame(target, ["session-token"]);

    expect(fetchStub).toHaveBeenCalledTimes(1);
    const auditCall = fetchStub.mock.calls[0] as unknown as [unknown, RequestInit];
    const serialisedBody = String(auditCall[1]?.body ?? "");
    // Substring assertion — `"capability":` (scalar) must not appear.
    // `"capabilities":` (array) MUST appear.
    expect(serialisedBody).not.toContain('"capability":');
    expect(serialisedBody).toContain('"capabilities":');
    // Also ensure the request body does not carry correlation_id
    // (handler is the sole authority per §3.1).
    expect(serialisedBody).not.toContain('"correlation_id":');

    mod.clearSessionToken();
  });
});

// ─── Correlation-id capture from response — RFC v2 §3.1 echo path ─────

describe("TestApiFetch_GrantTokenToFrame_CapturesCorrelationIdFromResponse_Good", () => {
  beforeEach(() => {
    vi.resetModules();
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    vi.resetModules();
  });

  it("broker logs the handler-generated correlation_id without leaking the token", async () => {
    // Handler echoes correlation_id in the response body. Broker reads
    // it for forensic-reconstruction logging. Token bytes MUST NOT
    // appear in the debug log line.
    const correlationID = "abcdef01-2345-4678-89ab-cdef01234567";
    const responseBody = JSON.stringify({
      success: true,
      data: { correlation_id: correlationID },
    });
    const fetchStub = vi.fn(async () => new Response(responseBody, {
      status: 200,
      headers: { "Content-Type": "application/json" },
    }));
    vi.stubGlobal("fetch", fetchStub);

    const debugSpy = vi.spyOn(console, "debug").mockImplementation(() => {});

    const mod = await import("./api-fetch");
    mod.setSessionToken("LTHN-SESS-1.correlation-log-test");

    const target = {
      source: { postMessage: vi.fn() } as unknown as Window,
      targetOrigin: "http://127.0.0.1:4096",
      pluginCode: "opencode",
    };
    const outcome = await mod.grantTokenToFrame(target, ["session-token"]);

    expect(outcome.ok).toBe(true);
    // Debug log emitted exactly once with the correlation_id payload.
    const matchingCalls = debugSpy.mock.calls.filter(
      (c) => c[0] === "lthn:audit:correlation_id",
    );
    expect(matchingCalls.length).toBe(1);
    const logPayload = matchingCalls[0]![1] as Record<string, unknown>;
    expect(logPayload.correlation_id).toBe(correlationID);
    expect(logPayload.plugin_id).toBe("opencode");
    expect(logPayload.origin).toBe("http://127.0.0.1:4096");
    // Token bytes MUST NOT appear anywhere in the log line.
    expect(JSON.stringify(logPayload)).not.toContain("LTHN-SESS-1");

    debugSpy.mockRestore();
    mod.clearSessionToken();
  });

  it("response without correlation_id is still treated as audit-committed", async () => {
    // A 200 with no parseable correlation_id is still a committed audit
    // row from the handler's perspective. Broker proceeds to postMessage.
    const fetchStub = vi.fn(async () => new Response("{}", { status: 200 }));
    vi.stubGlobal("fetch", fetchStub);

    const mod = await import("./api-fetch");
    mod.setSessionToken("LTHN-SESS-1.no-correlation-test");

    const postMessage = vi.fn();
    const target = {
      source: { postMessage } as unknown as Window,
      targetOrigin: "http://127.0.0.1:4096",
      pluginCode: "opencode",
    };
    const outcome = await mod.grantTokenToFrame(target, ["session-token"]);

    expect(outcome.ok).toBe(true);
    expect(postMessage).toHaveBeenCalledTimes(1);

    mod.clearSessionToken();
  });
});

// ─── postMessage failure propagation ──────────────────────────────────

describe("TestApiFetch_GrantTokenToFrame_PostMessageThrowReturnsNotOk_Bad", () => {
  beforeEach(() => {
    vi.resetModules();
    vi.stubGlobal("fetch", vi.fn(async () => new Response("{}", { status: 200 })));
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    vi.resetModules();
  });

  it("postMessage throw returns ok:false reason:postmessage-failed without dispatching", async () => {
    // Cerberus #17 / Mantis #1585. When target.source.postMessage throws
    // (target window closed, cross-origin throw, non-browser harness),
    // the audit row has already committed showing outcome:granted, but
    // no token bytes actually crossed. The caller MUST observe failure
    // so dispatchGranted does not fire on an empty delivery. Per RFC §10
    // row-IS-grant discipline-floor, the function reports reality.
    const fetchStub = vi.fn(async () => new Response("{}", { status: 200 }));
    vi.stubGlobal("fetch", fetchStub);

    const mod = await import("./api-fetch");
    mod.setSessionToken("LTHN-SESS-1.throw-test-token");

    const throwingPostMessage = vi.fn(() => {
      throw new Error("target window closed");
    });
    const target = {
      source: { postMessage: throwingPostMessage } as unknown as Window,
      targetOrigin: "http://127.0.0.1:4096",
      pluginCode: "opencode",
    };

    // Regression check: no caller-side window event should fire as a
    // side-effect of the failed grant. The boundary returns failure;
    // the caller chooses to dispatch (or not). Spy on global window
    // dispatchEvent so any accidental event-emission inside the boundary
    // surfaces.
    const dispatchSpy = vi.spyOn(window, "dispatchEvent");

    const outcome = await mod.grantTokenToFrame(target, ["session-token"]);

    expect(outcome.ok).toBe(false);
    // Discriminator narrows: TS won't let .reason read on ok:true.
    if (!outcome.ok) {
      expect(outcome.reason).toBe("postmessage-failed");
    }
    // Audit POST still fired exactly once (audit is pre-postMessage).
    expect(fetchStub).toHaveBeenCalledTimes(1);
    // postMessage was attempted exactly once and threw.
    expect(throwingPostMessage).toHaveBeenCalledTimes(1);
    // No window event emitted as a side-effect of the failed grant.
    expect(dispatchSpy).not.toHaveBeenCalled();

    dispatchSpy.mockRestore();
    mod.clearSessionToken();
  });
});

// ─── cachedSessionToken literal lives in exactly one file ──────────────

describe("TestApiFetch_CachedSessionTokenLiteralAppearsInExactlyOneFile_Good", () => {
  it("only api-fetch.ts references the binding by name", () => {
    // Resolve frontend/src/ relative to the vitest cwd. Vitest runs
    // from frontend/ (per package.json scripts), so src/ is the
    // tree root. happy-dom's import.meta.url returns a synthetic
    // pathname that doesn't resolve on the host fs — use cwd
    // instead, which IS the frontend/ root.
    const srcRoot = resolve(process.cwd(), "src");

    const hits: string[] = [];
    const walk = (dir: string): void => {
      for (const entry of readdirSync(dir)) {
        const full = join(dir, entry);
        const st = statSync(full);
        if (st.isDirectory()) {
          if (entry === "node_modules" || entry === "test") continue;
          walk(full);
          continue;
        }
        if (!entry.endsWith(".ts")) continue;
        // Skip THIS test file — it names the literal as a string the
        // grep would otherwise count.
        if (full.endsWith("api-fetch.test.ts")) continue;
        const body = readFileSync(full, "utf-8");
        if (body.includes("cachedSessionToken")) hits.push(full);
      }
    };
    walk(srcRoot);

    // Expected: exactly one file (api-fetch.ts). Any other file that
    // names `cachedSessionToken` is a closure-boundary breach.
    expect(hits.length).toBe(1);
    expect(hits[0]).toMatch(/api-fetch\.ts$/);
  });
});

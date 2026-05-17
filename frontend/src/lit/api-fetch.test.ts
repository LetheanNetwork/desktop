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

    // Audit body carries plugin_id + capability + origin, NOT the
    // token bytes (Cerberus #1465 + #1468).
    const auditCall = fetchStub.mock.calls[0] as unknown as [unknown, RequestInit];
    const auditInit = auditCall[1] ?? {};
    const auditBody = JSON.parse(String(auditInit.body ?? "{}"));
    expect(auditBody.plugin_id).toBe("opencode");
    expect(auditBody.capability).toBe("session-token");
    expect(auditBody.origin).toBe("http://127.0.0.1:4096");
    expect(JSON.stringify(auditBody)).not.toContain("LTHN-SESS-1");

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

    // Audit still committed (audit is per-scope and not session-
    // conditional per the broker shape).
    expect(fetchStub).toHaveBeenCalledTimes(1);
    expect(postMessage).toHaveBeenCalledTimes(0);
    expect(outcome).toEqual({ ok: false, reason: "no-session" });
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

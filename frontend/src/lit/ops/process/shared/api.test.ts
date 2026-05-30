// SPDX-Licence-Identifier: EUPL-1.2
//
// Unit tests for ProcessApi — the typed fetch wrapper over the
// /v1/api/process/* REST surface. ProcessApi dynamically imports the
// api-fetch broker; we mock it so each test drives the response shape
// (coreapi { success, data } envelope, raw body, error envelope, HTTP
// failure) without a live server.

import { describe, it, expect, beforeEach, vi } from "vitest";
import { ProcessApi } from "./api";
import type { DaemonEntry, ProcessInfo } from "./api";

// Shared mock for the api-fetch broker. ProcessApi calls
// `import('../../../api-fetch')` → resolves to src/lit/api-fetch. The
// mock path must match the specifier ProcessApi uses.
const apiFetchMock = vi.fn();
vi.mock("../../../api-fetch", () => ({
  apiFetch: (...args: unknown[]) => apiFetchMock(...args),
}));

// Build a fetch Response double. `body` is what res.json() yields;
// `ok`/`status`/`statusText` drive the error path. `text` backs the
// non-ok branch's diagnostic.
function fakeResponse(body: unknown, opts: { ok?: boolean; status?: number; statusText?: string } = {}) {
  return {
    ok: opts.ok ?? true,
    status: opts.status ?? 200,
    statusText: opts.statusText ?? "OK",
    json: async () => body,
    text: async () => (typeof body === "string" ? body : JSON.stringify(body)),
  };
}

// coreapi.Response[T] success envelope.
const enveloped = (data: unknown) => fakeResponse({ success: true, data });

beforeEach(() => {
  apiFetchMock.mockReset();
});

describe("ProcessApi — envelope parsing", () => {
  it("unwraps the coreapi { success, data } envelope to data", async () => {
    apiFetchMock.mockResolvedValue(enveloped({ id: "p1", command: "go" } as ProcessInfo));
    const api = new ProcessApi();
    const out = await api.getProcess("p1");
    expect(out).toEqual({ id: "p1", command: "go" });
  });

  it("throws the envelope error.message when success is false", async () => {
    apiFetchMock.mockResolvedValue(fakeResponse({ success: false, error: { message: "no such process" } }));
    const api = new ProcessApi();
    await expect(api.getProcess("ghost")).rejects.toThrow("no such process");
  });

  it("throws a generic 'Request failed' when success is false and no error message", async () => {
    apiFetchMock.mockResolvedValue(fakeResponse({ success: false }));
    const api = new ProcessApi();
    await expect(api.getProcess("x")).rejects.toThrow("Request failed");
  });

  it("returns a raw (non-enveloped) body unchanged — plain-string handlers", async () => {
    apiFetchMock.mockResolvedValue(fakeResponse("combined output text"));
    const api = new ProcessApi();
    const out = await api.getProcessOutput("p1");
    expect(out).toBe("combined output text");
  });

  it("throws on a non-ok HTTP response with status + body", async () => {
    apiFetchMock.mockResolvedValue(fakeResponse("internal boom", { ok: false, status: 500, statusText: "Server Error" }));
    const api = new ProcessApi();
    await expect(api.getProcess("p1")).rejects.toThrow(/500 Server Error/);
  });
});

describe("ProcessApi — list coercion to []", () => {
  it("listDaemons coerces a null body to an empty array", async () => {
    apiFetchMock.mockResolvedValue(enveloped(null));
    const api = new ProcessApi();
    expect(await api.listDaemons()).toEqual([]);
  });

  it("listDaemons coerces a {} body to an empty array", async () => {
    apiFetchMock.mockResolvedValue(enveloped({}));
    const api = new ProcessApi();
    expect(await api.listDaemons()).toEqual([]);
  });

  it("listDaemons passes through a populated array", async () => {
    const daemons: DaemonEntry[] = [{ code: "c1", daemon: "d1", pid: 7, started: "now" }];
    apiFetchMock.mockResolvedValue(enveloped(daemons));
    const api = new ProcessApi();
    expect(await api.listDaemons()).toEqual(daemons);
  });

  it("listProcesses coerces a non-array body to []", async () => {
    apiFetchMock.mockResolvedValue(enveloped(null));
    const api = new ProcessApi();
    expect(await api.listProcesses()).toEqual([]);
  });
});

describe("ProcessApi — URL + method construction", () => {
  it("uses the default base prefix /v1/api/process", async () => {
    apiFetchMock.mockResolvedValue(enveloped([]));
    await new ProcessApi().listDaemons();
    expect(apiFetchMock).toHaveBeenCalledWith("/v1/api/process/daemons", undefined);
  });

  it("honours a custom base prefix", async () => {
    apiFetchMock.mockResolvedValue(enveloped([]));
    await new ProcessApi("/custom/mount").listDaemons();
    expect(apiFetchMock).toHaveBeenCalledWith("/custom/mount/daemons", undefined);
  });

  it("appends ?runningOnly=true when listProcesses(true)", async () => {
    apiFetchMock.mockResolvedValue(enveloped([]));
    await new ProcessApi().listProcesses(true);
    expect(apiFetchMock).toHaveBeenCalledWith("/v1/api/process/processes?runningOnly=true", undefined);
  });

  it("POSTs the start payload as JSON to /processes", async () => {
    apiFetchMock.mockResolvedValue(enveloped({ id: "new" } as ProcessInfo));
    await new ProcessApi().startProcess({ command: "go", args: ["test"] });
    const [path, opts] = apiFetchMock.mock.calls[0];
    expect(path).toBe("/v1/api/process/processes");
    expect((opts as RequestInit).method).toBe("POST");
    expect(JSON.parse((opts as RequestInit).body as string)).toEqual({ command: "go", args: ["test"] });
  });

  it("killProcess POSTs to the /kill sub-path", async () => {
    apiFetchMock.mockResolvedValue(enveloped({ killed: true }));
    await new ProcessApi().killProcess("p9");
    expect(apiFetchMock).toHaveBeenCalledWith(
      "/v1/api/process/processes/p9/kill",
      expect.objectContaining({ method: "POST" }),
    );
  });

  it("signalProcess stringifies a numeric signal in the body", async () => {
    apiFetchMock.mockResolvedValue(enveloped({ signalled: true }));
    await new ProcessApi().signalProcess("p1", 9);
    const [, opts] = apiFetchMock.mock.calls[0];
    expect(JSON.parse((opts as RequestInit).body as string)).toEqual({ signal: "9" });
  });

  it("runPipeline POSTs mode + specs to /pipelines/run", async () => {
    apiFetchMock.mockResolvedValue(enveloped({ results: [], duration: 0, passed: 0, failed: 0, skipped: 0, success: true }));
    await new ProcessApi().runPipeline("sequential", [{ name: "build", command: "go" }]);
    const [path, opts] = apiFetchMock.mock.calls[0];
    expect(path).toBe("/v1/api/process/pipelines/run");
    expect(JSON.parse((opts as RequestInit).body as string)).toEqual({
      mode: "sequential",
      specs: [{ name: "build", command: "go" }],
    });
  });
});

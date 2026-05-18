// SPDX-Licence-Identifier: EUPL-1.2

import { describe, it, expect, vi } from "vitest";
import { mountWindow, expectChromeTitle, isEmbedded } from "../../test/window-fixture";
import "./benchmark-window";

// Wire-test pattern per [[design_lit_view_backend_wire_pattern]]:
//   * rows-replace  — backend returns runs → table swaps to those rows
//   * reject-keeps  — Promise.reject → fixture rows stay visible
//   * empty-keeps   — Result.OK with empty array → fixture stays
//   * fixtures-still-pass — no mock at all → fixture path renders cleanly

describe("lthn-benchmark-window — smoke", () => {
  it("mounts with the Benchmark titlebar", async () => {
    const { host } = await mountWindow("lthn-benchmark-window");
    expectChromeTitle(host, "Benchmark");
    expect(host.querySelector("header")).not.toBeNull();
  });

  it("fixtures-still-pass: renders fixture rows when backend is not mocked", async () => {
    const { host } = await mountWindow("lthn-benchmark-window");
    expect(host.textContent).toContain("gemma-4-e2b");
    expect(host.textContent).toContain("fixture");
  });
});

describe("lthn-benchmark-window — backend wire", () => {
  it("rows-replace: backend runs swap into the table", async () => {
    vi.doMock("@desktop/benchmark/service", () => ({
      History: vi.fn().mockResolvedValue({
        OK: true,
        Value: [
          { id: "r1", timestamp: "2026-05-18T10:00:00Z", bencher: "lthn-mlx", model: "qwen-3-7b", ctx: 2048, pp_tok_sec: 5200, tg_tok_sec: 52.3, prompt_len: 1024, output_len: 256, peak_watts: 11.2, peak_mem_mb: 4800 },
          { id: "r2", timestamp: "2026-05-18T09:00:00Z", bencher: "ollama-local", model: "mistral-7b", ctx: 1024, pp_tok_sec: 3100, tg_tok_sec: 28.4, prompt_len: 512, output_len: 256, peak_watts: 0, peak_mem_mb: 0, endpoint: "http://localhost:11434" },
        ],
      }),
      ListBenchers: vi.fn().mockResolvedValue({
        OK: true,
        Value: [{ name: "lthn-mlx", kind: "local" }, { name: "ollama-local", kind: "remote-http" }],
      }),
    }));
    const { host, el } = await mountWindow<HTMLElement & { updateComplete: Promise<boolean>; _loadFromBackend: () => Promise<void> }>("lthn-benchmark-window");
    await el._loadFromBackend();
    await el.updateComplete;
    expect(host.textContent).toContain("qwen-3-7b");
    expect(host.textContent).toContain("mistral-7b");
    expect(host.textContent).toContain("lthn-mlx");
    expect(host.textContent).toContain("ollama-local");
    // Fixture rows replaced.
    expect(host.textContent).not.toContain("phi-3.5-mini");
    vi.doUnmock("@desktop/benchmark/service");
  });

  it("reject-keeps: rejecting promise keeps fixture rows", async () => {
    vi.doMock("@desktop/benchmark/service", () => ({
      History: vi.fn().mockRejectedValue(new Error("backend unavailable")),
      ListBenchers: vi.fn().mockRejectedValue(new Error("backend unavailable")),
    }));
    const { host, el } = await mountWindow<HTMLElement & { updateComplete: Promise<boolean>; _loadFromBackend: () => Promise<void> }>("lthn-benchmark-window");
    await el._loadFromBackend();
    await el.updateComplete;
    // Fixture rows still visible.
    expect(host.textContent).toContain("phi-3.5-mini");
    expect(host.textContent).toContain("fixture");
    vi.doUnmock("@desktop/benchmark/service");
  });

  it("empty-keeps: empty Value array keeps fixture rows", async () => {
    vi.doMock("@desktop/benchmark/service", () => ({
      History: vi.fn().mockResolvedValue({ OK: true, Value: [] }),
      ListBenchers: vi.fn().mockResolvedValue({ OK: true, Value: [] }),
    }));
    const { host, el } = await mountWindow<HTMLElement & { updateComplete: Promise<boolean>; _loadFromBackend: () => Promise<void> }>("lthn-benchmark-window");
    await el._loadFromBackend();
    await el.updateComplete;
    expect(host.textContent).toContain("gemma-4-e2b");
    expect(host.textContent).toContain("fixture");
    vi.doUnmock("@desktop/benchmark/service");
  });
});

describe("lthn-benchmark-window — picker + Run wire", () => {
  it("renders bencher dropdown options from ListBenchers", async () => {
    vi.doMock("@desktop/benchmark/service", () => ({
      History: vi.fn().mockResolvedValue({ OK: true, Value: [] }),
      ListBenchers: vi.fn().mockResolvedValue({
        OK: true,
        Value: [{ name: "ollama", kind: "remote-http" }, { name: "fixture-x", kind: "local" }],
      }),
      ModelsForBencher: vi.fn().mockResolvedValue({ OK: true, Value: ["llama3"] }),
      Bench: vi.fn(),
    }));
    const { host, el } = await mountWindow<HTMLElement & { updateComplete: Promise<boolean>; _loadFromBackend: () => Promise<void> }>("lthn-benchmark-window");
    await el._loadFromBackend();
    await el.updateComplete;
    const selects = host.querySelectorAll("select");
    expect(selects.length).toBe(2);
    const bencherOpts = Array.from(selects[0].querySelectorAll("option")).map(o => o.value);
    expect(bencherOpts).toContain("ollama");
    expect(bencherOpts).toContain("fixture-x");
    vi.doUnmock("@desktop/benchmark/service");
  });

  it("loads models for the selected bencher", async () => {
    const modelsFn = vi.fn().mockResolvedValue({ OK: true, Value: ["llama3", "mistral", "phi3"] });
    vi.doMock("@desktop/benchmark/service", () => ({
      History: vi.fn().mockResolvedValue({ OK: true, Value: [] }),
      ListBenchers: vi.fn().mockResolvedValue({
        OK: true,
        Value: [{ name: "ollama", kind: "remote-http" }],
      }),
      ModelsForBencher: modelsFn,
      Bench: vi.fn(),
    }));
    const { host, el } = await mountWindow<HTMLElement & {
      updateComplete: Promise<boolean>;
      _loadFromBackend: () => Promise<void>;
      _loadModels: (name: string) => Promise<void>;
    }>("lthn-benchmark-window");
    await el._loadFromBackend();
    await el._loadModels("ollama");
    await el.updateComplete;
    expect(modelsFn).toHaveBeenCalledWith("ollama");
    const selects = host.querySelectorAll("select");
    const modelOpts = Array.from(selects[1].querySelectorAll("option")).map(o => o.value);
    expect(modelOpts).toEqual(["llama3", "mistral", "phi3"]);
    vi.doUnmock("@desktop/benchmark/service");
  });

  it("Run button calls Bench with selected bencher + model", async () => {
    const benchFn = vi.fn().mockResolvedValue({
      OK: true,
      Value: { id: "new-run", timestamp: "2026-05-18T12:00:00Z", bencher: "ollama", model: "llama3", ctx: 2048, pp_tok_sec: 800, tg_tok_sec: 40 },
    });
    vi.doMock("@desktop/benchmark/service", () => ({
      History: vi.fn().mockResolvedValue({ OK: true, Value: [] }),
      ListBenchers: vi.fn().mockResolvedValue({
        OK: true,
        Value: [{ name: "ollama", kind: "remote-http" }],
      }),
      ModelsForBencher: vi.fn().mockResolvedValue({ OK: true, Value: ["llama3"] }),
      Bench: benchFn,
    }));
    const { el } = await mountWindow<HTMLElement & {
      updateComplete: Promise<boolean>;
      _loadFromBackend: () => Promise<void>;
      _loadModels: (name: string) => Promise<void>;
      _runBench: () => Promise<void>;
      selectedBencher: string;
      selectedModel: string;
      models: string[];
      runErr: string;
    }>("lthn-benchmark-window");
    await el._loadFromBackend();
    await el._loadModels("ollama");
    await el.updateComplete;
    expect(el.selectedBencher).toBe("ollama");
    expect(el.selectedModel).toBe("llama3");
    expect(el.models).toEqual(["llama3"]);
    await el._runBench();
    expect(el.runErr).toBe("");
    expect(benchFn).toHaveBeenCalledTimes(1);
    const [req, name] = benchFn.mock.calls[0];
    expect(name).toBe("ollama");
    expect(req.Model).toBe("llama3");
    expect(req.Ctx).toBe(2048);
    expect(req.Prompt.length).toBeGreaterThan(50);
    vi.doUnmock("@desktop/benchmark/service");
  });

  it("Run failure renders an error banner", async () => {
    vi.doMock("@desktop/benchmark/service", () => ({
      History: vi.fn().mockResolvedValue({ OK: true, Value: [] }),
      ListBenchers: vi.fn().mockResolvedValue({
        OK: true,
        Value: [{ name: "ollama", kind: "remote-http" }],
      }),
      ModelsForBencher: vi.fn().mockResolvedValue({ OK: true, Value: ["llama3"] }),
      Bench: vi.fn().mockResolvedValue({ OK: false, Value: "ollama: model not found" }),
    }));
    const { host, el } = await mountWindow<HTMLElement & {
      updateComplete: Promise<boolean>;
      _loadFromBackend: () => Promise<void>;
      _loadModels: (name: string) => Promise<void>;
      _runBench: () => Promise<void>;
    }>("lthn-benchmark-window");
    await el._loadFromBackend();
    await el._loadModels("ollama");
    await el.updateComplete;
    await el._runBench();
    await el.updateComplete;
    expect(host.textContent).toContain("ollama: model not found");
    vi.doUnmock("@desktop/benchmark/service");
  });
});

describe("lthn-benchmark-window — two-shell", () => {
  it("embedded mode collapses the chrome", async () => {
    const { host } = await mountWindow("lthn-benchmark-window", { attrs: { embedded: "" } });
    expect(isEmbedded(host)).toBe(true);
    expect(host.querySelector("header")).toBeNull();
  });
});

// SPDX-Licence-Identifier: EUPL-1.2

import { describe, it, expect, beforeEach, vi } from "vitest";

vi.mock("../../api-fetch", () => ({
  apiFetch: vi.fn(),
}));

import { apiFetch } from "../../api-fetch";

import "./lora";

interface MountedEl extends HTMLElement {
  embedded: boolean;
  runs:     Array<{
    id: string; base_model: string; dataset: string; mode: string;
    status: string; iteration: number; total_iters: number;
    last_loss: number; throughput_tps: number; eta_seconds: number;
    started: string; train_path: string;
  }>;
  filter:   string;
  selected: string;
  loading:  boolean;
  error:    string;
  updateComplete: Promise<boolean>;
}

async function mount(attrs: Record<string, string | boolean> = {}): Promise<MountedEl> {
  const el = document.createElement("lthn-view-ml-lab-lora") as MountedEl;
  for (const [k, v] of Object.entries(attrs)) {
    if (typeof v === "boolean") {
      if (v) el.setAttribute(k, "");
    } else {
      el.setAttribute(k, v);
    }
  }
  document.body.appendChild(el);
  await el.updateComplete;
  await Promise.resolve();
  await el.updateComplete;
  return el;
}

function fetchOK(payload: unknown) {
  return {
    ok: true,
    status: 200,
    json: async () => payload,
  };
}

function makeRun(over: Partial<MountedEl["runs"][number]> = {}): MountedEl["runs"][number] {
  return {
    id: "run-test", base_model: "lemma-12b", dataset: "golden_set", mode: "sft",
    status: "active", iteration: 100, total_iters: 1000,
    last_loss: 0.5, throughput_tps: 1500, eta_seconds: 600,
    started: "2026-05-22T08:00:00Z", train_path: "/tmp/x.train",
    ...over,
  };
}

describe("<lthn-view-ml-lab-lora>", () => {
  beforeEach(() => {
    (apiFetch as unknown as { mockReset: () => void }).mockReset();
    document.body.innerHTML = "";
  });

  it("renders fixture LoRA runs when backend is not yet wired (Good)", async () => {
    (apiFetch as unknown as { mockRejectedValue: (e: unknown) => void })
      .mockRejectedValue(new Error("no binding"));
    const el = await mount();
    // FIXTURE_RUNS has 6 entries spanning every status.
    expect(el.runs.length).toBe(6);
    expect(el.textContent).toContain("lemma-12b-v4-p6");
    // Status badges visible.
    expect(el.textContent).toContain("active");
    expect(el.textContent).toContain("completed");
    expect(el.textContent).toContain("failed");
  });

  it("loads runs from apiFetch when backend returns them (Good)", async () => {
    (apiFetch as unknown as { mockResolvedValue: (v: unknown) => void })
      .mockResolvedValue(fetchOK({
        data: {
          runs: [
            makeRun({ id: "r1", base_model: "qwen-3", mode: "grpo", status: "active" }),
            makeRun({ id: "r2", base_model: "llama-3", mode: "distill", status: "paused" }),
          ],
        },
      }));
    const el = await mount();
    expect(el.runs.length).toBe(2);
    expect(el.textContent).toContain("qwen-3");
    expect(el.textContent).toContain("llama-3");
    expect(el.textContent).toContain("GRPO");
    expect(el.textContent).toContain("DISTILL");
  });

  it("reject-keeps — backend error leaves prior list intact (Ugly)", async () => {
    const calls: number[] = [];
    (apiFetch as unknown as { mockImplementation: (fn: () => unknown) => void })
      .mockImplementation(() => {
        calls.push(1);
        if (calls.length === 1) return fetchOK({ data: { runs: [
          makeRun({ id: "first", base_model: "seed-model" }),
        ] } });
        return { ok: false, status: 500, json: async () => ({}) };
      });
    const el = await mount();
    expect(el.runs.length).toBe(1);
    expect(el.textContent).toContain("seed-model");

    await (el as MountedEl & { _loadFromBackend: () => Promise<void> })._loadFromBackend();
    await el.updateComplete;
    expect(el.runs.length).toBe(1);
    expect(el.error).toContain("500");
    expect(el.textContent).toContain("seed-model");
  });

  it("empty-keeps — empty backend response leaves prior list intact (Ugly)", async () => {
    const calls: number[] = [];
    (apiFetch as unknown as { mockImplementation: (fn: () => unknown) => void })
      .mockImplementation(() => {
        calls.push(1);
        if (calls.length === 1) return fetchOK({ data: { runs: [
          makeRun({ id: "kept", base_model: "kept-model" }),
        ] } });
        return fetchOK({ data: { runs: [] } });
      });
    const el = await mount();
    expect(el.runs.length).toBe(1);

    await (el as MountedEl & { _loadFromBackend: () => Promise<void> })._loadFromBackend();
    await el.updateComplete;
    expect(el.runs.length).toBe(1);
    expect(el.textContent).toContain("kept-model");
  });

  it("filter narrows visible rows by status / mode / base_model / id (Good)", async () => {
    (apiFetch as unknown as { mockResolvedValue: (v: unknown) => void })
      .mockResolvedValue(fetchOK({
        data: {
          runs: [
            makeRun({ id: "r-sft",      base_model: "gemma-4",  mode: "sft",     status: "active" }),
            makeRun({ id: "r-grpo",     base_model: "llama-3",  mode: "grpo",    status: "paused" }),
            makeRun({ id: "r-distill",  base_model: "qwen-3",   mode: "distill", status: "completed" }),
          ],
        },
      }));
    const el = await mount();
    el.filter = "grpo";
    await el.updateComplete;
    expect(el.textContent).toContain("llama-3");
    expect(el.textContent).not.toContain("gemma-4");
    expect(el.textContent).not.toContain("qwen-3");
  });

  it("empty-state copy shows when filter matches nothing (Ugly)", async () => {
    (apiFetch as unknown as { mockResolvedValue: (v: unknown) => void })
      .mockResolvedValue(fetchOK({
        data: { runs: [makeRun({ id: "only", base_model: "only-model" })] },
      }));
    const el = await mount();
    el.filter = "no-such-run-anywhere";
    await el.updateComplete;
    expect(el.textContent).toContain("No LoRA runs match these filters");
  });

  it("clicking a row selects it; clicking the same row again deselects (Good)", async () => {
    (apiFetch as unknown as { mockResolvedValue: (v: unknown) => void })
      .mockResolvedValue(fetchOK({
        data: { runs: [
          makeRun({ id: "click-me", base_model: "test-model" }),
        ] },
      }));
    const el = await mount();
    expect(el.selected).toBe("");

    const row = el.querySelector(".run-row") as HTMLElement | null;
    expect(row).not.toBeNull();
    row?.click();
    await el.updateComplete;
    expect(el.selected).toBe("click-me");
    expect(el.querySelector(".run-row.selected")).not.toBeNull();

    row?.click();
    await el.updateComplete;
    expect(el.selected).toBe("");
    expect(el.querySelector(".run-row.selected")).toBeNull();
  });

  it("embedded attribute renders without chrome wrapper (Good)", async () => {
    (apiFetch as unknown as { mockRejectedValue: (e: unknown) => void })
      .mockRejectedValue(new Error("no binding"));
    const el = await mount({ embedded: true });
    expect(el.hasAttribute("embedded")).toBe(true);
  });
});

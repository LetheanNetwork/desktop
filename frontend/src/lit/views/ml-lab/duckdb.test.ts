// SPDX-Licence-Identifier: EUPL-1.2

import { describe, it, expect, beforeEach, vi } from "vitest";

vi.mock("../../api-fetch", () => ({
  apiFetch: vi.fn(),
}));

import { apiFetch } from "../../api-fetch";
import "./duckdb";

interface MountedEl extends HTMLElement {
  sql: string;
  result: {
    columns: Array<{ name: string; kind: string }>;
    rows: unknown[][];
    row_count_total: number;
    pagination_cursor: string;
  };
  loading: boolean;
  error: string;
  updateComplete: Promise<boolean>;
}

async function mount(): Promise<MountedEl> {
  const el = document.createElement("lthn-view-ml-lab-duckdb") as MountedEl;
  document.body.appendChild(el);
  await el.updateComplete;
  await Promise.resolve();
  await el.updateComplete;
  return el;
}

function fetchOK(payload: unknown) {
  return { ok: true, status: 200, json: async () => payload, text: async () => "" };
}

describe("<lthn-view-ml-lab-duckdb>", () => {
  beforeEach(() => {
    (apiFetch as unknown as { mockReset: () => void }).mockReset();
    document.body.innerHTML = "";
  });

  it("renders fixture rows when backend is offline (Good)", async () => {
    (apiFetch as unknown as { mockRejectedValue: (e: unknown) => void })
      .mockRejectedValue(new Error("no binding"));
    const el = await mount();
    expect(el.result.rows.length).toBe(3);
    expect(el.sql).toContain("FROM lora_runs");
    // Schema browser shows known tables.
    expect(el.textContent).toContain("lora_runs");
    expect(el.textContent).toContain("lab_saved_queries");
  });

  it("loads backend result when /v1/ml-lab/duckdb returns rows (Good)", async () => {
    (apiFetch as unknown as { mockResolvedValue: (v: unknown) => void })
      .mockResolvedValue(fetchOK({
        columns: [{ name: "n", kind: "int" }],
        rows: [[1], [2], [3], [4]],
        row_count_total: 4,
        pagination_cursor: "",
      }));
    const el = await mount();
    expect(el.result.rows.length).toBe(4);
    expect(el.result.row_count_total).toBe(4);
  });

  it("surfaces backend error string without losing prior rows (Ugly)", async () => {
    (apiFetch as unknown as { mockResolvedValue: (v: unknown) => void })
      .mockResolvedValue({
        ok: false,
        status: 400,
        json: async () => ({}),
        text: async () => "rejected: mutating SQL forbidden",
      });
    const el = await mount();
    expect(el.error).toContain("HTTP 400");
    // Prior fixture remains.
    expect(el.result.rows.length).toBe(3);
  });
});

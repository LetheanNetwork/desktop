// SPDX-Licence-Identifier: EUPL-1.2

import { describe, it, expect, beforeEach, vi } from "vitest";

vi.mock("../../api-fetch", () => ({
  apiFetch: vi.fn(),
}));

import { apiFetch } from "../../api-fetch";
import "./influx";

interface MountedEl extends HTMLElement {
  body: string;
  result: { columns: Array<{ name: string; kind: string }>; rows: unknown[][]; chart_hint: string };
  loading: boolean;
  error: string;
  updateComplete: Promise<boolean>;
}

async function mount(): Promise<MountedEl> {
  const el = document.createElement("lthn-view-ml-lab-influx") as MountedEl;
  document.body.appendChild(el);
  await el.updateComplete;
  await Promise.resolve();
  await el.updateComplete;
  return el;
}

function fetchOK(payload: unknown) {
  return { ok: true, status: 200, json: async () => payload };
}

describe("<lthn-view-ml-lab-influx>", () => {
  beforeEach(() => {
    (apiFetch as unknown as { mockReset: () => void }).mockReset();
    document.body.innerHTML = "";
  });

  it("renders fixture time-series when backend is offline (Good)", async () => {
    (apiFetch as unknown as { mockRejectedValue: (e: unknown) => void })
      .mockRejectedValue(new Error("no binding"));
    const el = await mount();
    expect(el.result.rows.length).toBe(8);
    expect(el.result.chart_hint).toBe("line");
    // Editor seeded with the default Flux body.
    expect(el.body).toContain("from(bucket:");
  });

  it("loads backend result when /v1/ml-lab/influx returns rows (Good)", async () => {
    (apiFetch as unknown as { mockResolvedValue: (v: unknown) => void })
      .mockResolvedValue(fetchOK({
        columns: [{ name: "ts", kind: "time" }, { name: "loss", kind: "float" }],
        rows: [["2026-05-24T10:00:00Z", 0.42], ["2026-05-24T10:01:00Z", 0.38]],
        chart_hint: "line",
      }));
    const el = await mount();
    expect(el.result.rows.length).toBe(2);
    expect(el.result.columns[1].name).toBe("loss");
  });

  it("reject-keeps fixture — backend error leaves prior result intact (Ugly)", async () => {
    (apiFetch as unknown as { mockRejectedValue: (e: unknown) => void })
      .mockRejectedValue(new Error("network down"));
    const el = await mount();
    // Fixture survives; error string surfaces inline.
    expect(el.result.rows.length).toBe(8);
    expect(el.error).toBeTruthy();
  });
});

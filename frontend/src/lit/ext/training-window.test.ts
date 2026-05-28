// SPDX-Licence-Identifier: EUPL-1.2

import { describe, it, expect, vi } from "vitest";
import { mountWindow, expectChromeTitle, isEmbedded } from "../../test/window-fixture";
import "./training-window";

describe("lthn-training-window — smoke", () => {
  it("mounts with the Train titlebar", async () => {
    const { host } = await mountWindow("lthn-training-window");
    expectChromeTitle(host, "Train");
    expect(host.querySelector("header")).not.toBeNull();
  });

  it("renders the training header strip with model + tier + status", async () => {
    const { host } = await mountWindow("lthn-training-window");
    const strip = host.querySelector(".training-header");
    expect(strip).not.toBeNull();
    expect(strip?.textContent).toContain("gemma4-e2b-it-q4");
    expect(strip?.textContent).toContain("E2B");
    expect(strip?.textContent).toContain("european");
  });

  it("renders all seven fork-tree subjects in the rotation queue", async () => {
    const { host } = await mountWindow("lthn-training-window");
    const rows = host.querySelectorAll(".training-rotation__row");
    expect(rows.length).toBe(7);
    const text = Array.from(rows).map(r => r.textContent ?? "").join(" ");
    for (const subject of ["English", "European", "Latam", "Russian", "Middle East", "Chinese", "African"]) {
      expect(text).toContain(subject);
    }
  });

  it("renders the loss-curve SVG with at least one peak marker", async () => {
    const { host } = await mountWindow("lthn-training-window");
    const svg = host.querySelector(".training-loss svg");
    expect(svg).not.toBeNull();
    const peaks = host.querySelectorAll(".training-loss__peak");
    expect(peaks.length).toBeGreaterThan(0);
  });

  it("renders at least one CL-BPL event row", async () => {
    const { host } = await mountWindow("lthn-training-window");
    const rows = host.querySelectorAll(".training-events__row");
    expect(rows.length).toBeGreaterThan(0);
  });

  it("renders the resource cells (tok/s, memory, ETA)", async () => {
    const { host } = await mountWindow("lthn-training-window");
    const cells = host.querySelectorAll(".training-resource__cell");
    expect(cells.length).toBe(3);
    const text = host.querySelector(".training-resource")?.textContent ?? "";
    expect(text).toMatch(/tok\/s/);
    expect(text).toMatch(/memory/);
    expect(text).toMatch(/ETA/);
  });
});

describe("lthn-training-window — two-shell", () => {
  it("embedded mode collapses the chrome", async () => {
    const { host } = await mountWindow("lthn-training-window", { attrs: { embedded: "" } });
    expect(isEmbedded(host)).toBe(true);
    expect(host.querySelector("header")).toBeNull();
  });
});

describe("lthn-training-window — Ugly inputs (defensive)", () => {
  it("tolerates being mounted without prior backend wire", async () => {
    // _loadFromBackend hits the three wailsservice imports; in vitest
    // they reject (no Wails runtime) so the fixture is authoritative.
    const { host } = await mountWindow("lthn-training-window");
    // Confirm rendering completed (rotation list is the deepest sub-tree).
    expect(host.querySelectorAll(".training-rotation__row").length).toBe(7);
  });
});

// Wire-test pattern per [[design_lit_view_backend_wire_pattern]]:
//   * wire fallback   — every binding rejects → fixture stays authoritative
//   * wire success    — every binding resolves shaped → live data swaps in
//   * partial failure — only Training fails → fixture header + live curve / counts
//   * r1 drives count — ListProbes shape feeds per-subject r1Count
describe("lthn-training-window — backend wire", () => {
  it("wire fallback — backend unreachable keeps fixture authoritative", async () => {
    vi.doMock("@desktop/training/wailsservice", () => ({
      Status: vi.fn().mockRejectedValue(new Error("not wired")),
    }));
    vi.doMock("@desktop/clbpl/wailsservice", () => ({
      Peaks: vi.fn().mockRejectedValue(new Error("not wired")),
    }));
    vi.doMock("@desktop/r1/wailsservice", () => ({
      ListModels:   vi.fn().mockRejectedValue(new Error("not wired")),
      ListSubjects: vi.fn().mockRejectedValue(new Error("not wired")),
      ListProbes:   vi.fn().mockRejectedValue(new Error("not wired")),
    }));
    const { host, el } = await mountWindow<HTMLElement & {
      updateComplete: Promise<boolean>;
      _loadFromBackend: () => Promise<void>;
    }>("lthn-training-window");
    await el._loadFromBackend();
    await el.updateComplete;
    // Fixture rotation count, fixture header, fixture events — all
    // survive the every-call-rejected case.
    expect(host.querySelectorAll(".training-rotation__row").length).toBe(7);
    expect(host.textContent).toContain("gemma4-e2b-it-q4");
    expect(host.textContent).toContain("european");
    expect(host.querySelectorAll(".training-events__row").length).toBeGreaterThan(0);
    vi.doUnmock("@desktop/training/wailsservice");
    vi.doUnmock("@desktop/clbpl/wailsservice");
    vi.doUnmock("@desktop/r1/wailsservice");
  });

  it("wire success — Training.Status returns live data, render uses it", async () => {
    vi.doMock("@desktop/training/wailsservice", () => ({
      Status: vi.fn().mockResolvedValue({
        OK: true,
        Value: {
          running: true,
          active_subject: "russian",
          active_probe: "P03",
          step: 412,
          peak_count: 7,
          groked_subjects: ["english", "european"],
          completed_runs: 2,
        },
      }),
    }));
    vi.doMock("@desktop/clbpl/wailsservice", () => ({
      Peaks: vi.fn().mockResolvedValue({
        OK: true,
        Value: [
          { step: 50,  loss: 4.20 },
          { step: 110, loss: 3.40 },
          { step: 220, loss: 2.55 },
        ],
      }),
    }));
    vi.doMock("@desktop/r1/wailsservice", () => ({
      ListModels:   vi.fn().mockResolvedValue({ OK: true, Value: ["gemma4-e2b-it-q4"] }),
      ListSubjects: vi.fn().mockResolvedValue({ OK: true, Value: ["english", "european", "russian"] }),
      ListProbes:   vi.fn().mockResolvedValue({ OK: true, Value: ["P01", "P02"] }),
    }));
    const { host, el } = await mountWindow<HTMLElement & {
      updateComplete: Promise<boolean>;
      _loadFromBackend: () => Promise<void>;
    }>("lthn-training-window");
    await el._loadFromBackend();
    await el.updateComplete;
    // Header subject reflects the Status snapshot (russian, not the
    // fixture's european).
    const header = host.querySelector(".training-header");
    expect(header?.textContent).toContain("russian");
    // Loss curve replaced — fixture had 11 samples + step 200; live
    // mock returned exactly 3 peak samples, all of which render as
    // peak markers.
    const peakMarkers = host.querySelectorAll(".training-loss__peak");
    expect(peakMarkers.length).toBe(3);
    // Rotation: english + european flipped to groked, russian flipped
    // to active, others to queued. Check by inspecting the row class
    // suffix.
    const rows = host.querySelectorAll(".training-rotation__row");
    const englishRow  = Array.from(rows).find(r => r.textContent?.includes("English"));
    const europeanRow = Array.from(rows).find(r => r.textContent?.includes("European"));
    const russianRow  = Array.from(rows).find(r => r.textContent?.includes("Russian"));
    expect(englishRow?.className).toContain("training-rotation__row--groked");
    expect(europeanRow?.className).toContain("training-rotation__row--groked");
    expect(russianRow?.className).toContain("training-rotation__row--active");
    vi.doUnmock("@desktop/training/wailsservice");
    vi.doUnmock("@desktop/clbpl/wailsservice");
    vi.doUnmock("@desktop/r1/wailsservice");
  });

  it("wire partial failure — only Training.Status fails", async () => {
    vi.doMock("@desktop/training/wailsservice", () => ({
      Status: vi.fn().mockRejectedValue(new Error("training service down")),
    }));
    vi.doMock("@desktop/clbpl/wailsservice", () => ({
      Peaks: vi.fn().mockResolvedValue({
        OK: true,
        Value: [{ step: 30, loss: 3.99 }, { step: 90, loss: 3.10 }],
      }),
    }));
    vi.doMock("@desktop/r1/wailsservice", () => ({
      ListModels:   vi.fn().mockResolvedValue({ OK: true, Value: ["gemma4-e2b-it-q4"] }),
      ListSubjects: vi.fn().mockResolvedValue({ OK: true, Value: ["english"] }),
      ListProbes:   vi.fn().mockResolvedValue({ OK: true, Value: ["P01", "P02", "P03", "P04", "P05"] }),
    }));
    const { host, el } = await mountWindow<HTMLElement & {
      updateComplete: Promise<boolean>;
      _loadFromBackend: () => Promise<void>;
    }>("lthn-training-window");
    await el._loadFromBackend();
    await el.updateComplete;
    // Header subject stays on the fixture value because Status failed.
    const header = host.querySelector(".training-header");
    expect(header?.textContent).toContain("european");
    expect(header?.textContent).toContain("gemma4-e2b-it-q4");
    // Loss curve swapped to the live (two-sample) shape — both
    // samples render as peak markers.
    const peakMarkers = host.querySelectorAll(".training-loss__peak");
    expect(peakMarkers.length).toBe(2);
    // English row picks up the live r1 count (5 probes) — overrides
    // the fixture's 312.
    const englishRow = Array.from(host.querySelectorAll(".training-rotation__row"))
      .find(r => r.textContent?.includes("English"));
    expect(englishRow?.textContent).toContain("5 R₁");
    vi.doUnmock("@desktop/training/wailsservice");
    vi.doUnmock("@desktop/clbpl/wailsservice");
    vi.doUnmock("@desktop/r1/wailsservice");
  });

  it("wire success — previewChunks populated from seeds + contentshield", async () => {
    vi.doMock("@desktop/training/wailsservice", () => ({
      Status: vi.fn().mockRejectedValue(new Error("not wired")),
    }));
    vi.doMock("@desktop/clbpl/wailsservice", () => ({
      Peaks: vi.fn().mockRejectedValue(new Error("not wired")),
    }));
    vi.doMock("@desktop/r1/wailsservice", () => ({
      ListModels:   vi.fn().mockRejectedValue(new Error("not wired")),
      ListSubjects: vi.fn().mockRejectedValue(new Error("not wired")),
      ListProbes:   vi.fn().mockRejectedValue(new Error("not wired")),
    }));
    vi.doMock("@desktop/seeds/wailsservice", () => ({
      ListProbes: vi.fn().mockResolvedValue({ OK: true, Value: ["P11", "P03", "P52"] }),
      Read: vi.fn().mockImplementation((_subject: string, id: string) => {
        const texts: Record<string, string> = {
          P11: "calm appropriate response without flattery",
          P03: "you are correct, and that is a wonderful observation",
          P52: "what an extraordinary insight you've just brought into the light",
        };
        return Promise.resolve({ OK: true, Value: texts[id] ?? "" });
      }),
    }));
    vi.doMock("@desktop/contentshield/wailsservice", () => ({
      Score: vi.fn().mockImplementation((text: string) => {
        if (text.includes("extraordinary")) {
          return Promise.resolve({ OK: true, Value: {
            sycophancy: { tier: 2, label: "hollow-flattery", composite: 71 },
            suggestions: [{ type: "tone", note: "soften" }],
          } });
        }
        if (text.includes("wonderful")) {
          return Promise.resolve({ OK: true, Value: {
            sycophancy: { tier: 1, label: "soft-agreement", composite: 42 },
            suggestions: [],
          } });
        }
        return Promise.resolve({ OK: true, Value: {
          sycophancy: { tier: 0, label: "appropriate-empathy", composite: 8 },
          suggestions: [],
        } });
      }),
    }));
    const { host, el } = await mountWindow<HTMLElement & {
      updateComplete: Promise<boolean>;
      _loadFromBackend: () => Promise<void>;
    }>("lthn-training-window");
    await el._loadFromBackend();
    await el.updateComplete;
    const rows = host.querySelectorAll(".training-preview__row");
    expect(rows.length).toBe(3);
    const flatteryRows = host.querySelectorAll(".training-preview__row--hollow-flattery");
    expect(flatteryRows.length).toBe(1);
    const text = Array.from(rows).map(r => r.textContent ?? "").join(" ");
    expect(text).toContain("P11");
    expect(text).toContain("P03");
    expect(text).toContain("P52");
    vi.doUnmock("@desktop/training/wailsservice");
    vi.doUnmock("@desktop/clbpl/wailsservice");
    vi.doUnmock("@desktop/r1/wailsservice");
    vi.doUnmock("@desktop/seeds/wailsservice");
    vi.doUnmock("@desktop/contentshield/wailsservice");
  });

  it("wire partial failure — seeds.Read fails for one probe", async () => {
    vi.doMock("@desktop/training/wailsservice", () => ({
      Status: vi.fn().mockRejectedValue(new Error("not wired")),
    }));
    vi.doMock("@desktop/clbpl/wailsservice", () => ({
      Peaks: vi.fn().mockRejectedValue(new Error("not wired")),
    }));
    vi.doMock("@desktop/r1/wailsservice", () => ({
      ListModels:   vi.fn().mockRejectedValue(new Error("not wired")),
      ListSubjects: vi.fn().mockRejectedValue(new Error("not wired")),
      ListProbes:   vi.fn().mockRejectedValue(new Error("not wired")),
    }));
    vi.doMock("@desktop/seeds/wailsservice", () => ({
      ListProbes: vi.fn().mockResolvedValue({ OK: true, Value: ["P01", "P02", "P03"] }),
      Read: vi.fn().mockImplementation((_subject: string, id: string) => {
        if (id === "P02") return Promise.reject(new Error("read failed"));
        return Promise.resolve({ OK: true, Value: `text for ${id}` });
      }),
    }));
    vi.doMock("@desktop/contentshield/wailsservice", () => ({
      Score: vi.fn().mockResolvedValue({ OK: true, Value: {
        sycophancy: { tier: 0, label: "appropriate-empathy", composite: 5 },
        suggestions: [],
      } }),
    }));
    const { host, el } = await mountWindow<HTMLElement & {
      updateComplete: Promise<boolean>;
      _loadFromBackend: () => Promise<void>;
    }>("lthn-training-window");
    await el._loadFromBackend();
    await el.updateComplete;
    const rows = host.querySelectorAll(".training-preview__row");
    expect(rows.length).toBe(2);
    const text = Array.from(rows).map(r => r.textContent ?? "").join(" ");
    expect(text).toContain("P01");
    expect(text).toContain("P03");
    expect(text).not.toContain("P02");
    vi.doUnmock("@desktop/training/wailsservice");
    vi.doUnmock("@desktop/clbpl/wailsservice");
    vi.doUnmock("@desktop/r1/wailsservice");
    vi.doUnmock("@desktop/seeds/wailsservice");
    vi.doUnmock("@desktop/contentshield/wailsservice");
  });

  it("wire fallback — seeds binding unreachable keeps fixture authoritative", async () => {
    vi.doMock("@desktop/training/wailsservice", () => ({
      Status: vi.fn().mockRejectedValue(new Error("not wired")),
    }));
    vi.doMock("@desktop/clbpl/wailsservice", () => ({
      Peaks: vi.fn().mockRejectedValue(new Error("not wired")),
    }));
    vi.doMock("@desktop/r1/wailsservice", () => ({
      ListModels:   vi.fn().mockRejectedValue(new Error("not wired")),
      ListSubjects: vi.fn().mockRejectedValue(new Error("not wired")),
      ListProbes:   vi.fn().mockRejectedValue(new Error("not wired")),
    }));
    vi.doMock("@desktop/seeds/wailsservice", () => ({
      ListProbes: vi.fn().mockRejectedValue(new Error("seeds not wired")),
      Read:       vi.fn().mockRejectedValue(new Error("seeds not wired")),
    }));
    vi.doMock("@desktop/contentshield/wailsservice", () => ({
      Score: vi.fn().mockRejectedValue(new Error("contentshield not wired")),
    }));
    const { host, el } = await mountWindow<HTMLElement & {
      updateComplete: Promise<boolean>;
      _loadFromBackend: () => Promise<void>;
    }>("lthn-training-window");
    await el._loadFromBackend();
    await el.updateComplete;
    // Fixture has 4 preview rows; all 4 survive when seeds reject.
    const rows = host.querySelectorAll(".training-preview__row");
    expect(rows.length).toBe(4);
    const text = host.querySelector(".training-preview")?.textContent ?? "";
    expect(text).toContain("EU01_REGRET");
    expect(text).toContain("EU04_SUBMIT");
    vi.doUnmock("@desktop/training/wailsservice");
    vi.doUnmock("@desktop/clbpl/wailsservice");
    vi.doUnmock("@desktop/r1/wailsservice");
    vi.doUnmock("@desktop/seeds/wailsservice");
    vi.doUnmock("@desktop/contentshield/wailsservice");
  });

  it("text truncation — textPreview adds … when source > 80 chars", async () => {
    const longText = "x".repeat(200);
    vi.doMock("@desktop/training/wailsservice", () => ({
      Status: vi.fn().mockRejectedValue(new Error("not wired")),
    }));
    vi.doMock("@desktop/clbpl/wailsservice", () => ({
      Peaks: vi.fn().mockRejectedValue(new Error("not wired")),
    }));
    vi.doMock("@desktop/r1/wailsservice", () => ({
      ListModels:   vi.fn().mockRejectedValue(new Error("not wired")),
      ListSubjects: vi.fn().mockRejectedValue(new Error("not wired")),
      ListProbes:   vi.fn().mockRejectedValue(new Error("not wired")),
    }));
    vi.doMock("@desktop/seeds/wailsservice", () => ({
      ListProbes: vi.fn().mockResolvedValue({ OK: true, Value: ["PLONG"] }),
      Read:       vi.fn().mockResolvedValue({ OK: true, Value: longText }),
    }));
    vi.doMock("@desktop/contentshield/wailsservice", () => ({
      Score: vi.fn().mockResolvedValue({ OK: true, Value: {
        sycophancy: { tier: 0, label: "appropriate-empathy", composite: 0 },
        suggestions: [],
      } }),
    }));
    const { host, el } = await mountWindow<HTMLElement & {
      updateComplete: Promise<boolean>;
      _loadFromBackend: () => Promise<void>;
    }>("lthn-training-window");
    await el._loadFromBackend();
    await el.updateComplete;
    const textNode = host.querySelector(".training-preview__text");
    expect(textNode).not.toBeNull();
    const rendered = textNode?.textContent ?? "";
    expect(rendered.endsWith("…")).toBe(true);
    expect(rendered.length).toBeLessThanOrEqual(83);
    vi.doUnmock("@desktop/training/wailsservice");
    vi.doUnmock("@desktop/clbpl/wailsservice");
    vi.doUnmock("@desktop/r1/wailsservice");
    vi.doUnmock("@desktop/seeds/wailsservice");
    vi.doUnmock("@desktop/contentshield/wailsservice");
  });

  it("tier→class mapping — sycophancy label \"submission\" maps to submission CSS class", async () => {
    vi.doMock("@desktop/training/wailsservice", () => ({
      Status: vi.fn().mockRejectedValue(new Error("not wired")),
    }));
    vi.doMock("@desktop/clbpl/wailsservice", () => ({
      Peaks: vi.fn().mockRejectedValue(new Error("not wired")),
    }));
    vi.doMock("@desktop/r1/wailsservice", () => ({
      ListModels:   vi.fn().mockRejectedValue(new Error("not wired")),
      ListSubjects: vi.fn().mockRejectedValue(new Error("not wired")),
      ListProbes:   vi.fn().mockRejectedValue(new Error("not wired")),
    }));
    vi.doMock("@desktop/seeds/wailsservice", () => ({
      ListProbes: vi.fn().mockResolvedValue({ OK: true, Value: ["PSUB"] }),
      Read:       vi.fn().mockResolvedValue({ OK: true, Value: "whatever you want, I defer" }),
    }));
    vi.doMock("@desktop/contentshield/wailsservice", () => ({
      Score: vi.fn().mockResolvedValue({ OK: true, Value: {
        sycophancy: { tier: 3, label: "submission", composite: 91 },
        suggestions: [],
      } }),
    }));
    const { host, el } = await mountWindow<HTMLElement & {
      updateComplete: Promise<boolean>;
      _loadFromBackend: () => Promise<void>;
    }>("lthn-training-window");
    await el._loadFromBackend();
    await el.updateComplete;
    const row = host.querySelector(".training-preview__row");
    expect(row).not.toBeNull();
    expect(row?.className).toContain("training-preview__row--submission");
    vi.doUnmock("@desktop/training/wailsservice");
    vi.doUnmock("@desktop/clbpl/wailsservice");
    vi.doUnmock("@desktop/r1/wailsservice");
    vi.doUnmock("@desktop/seeds/wailsservice");
    vi.doUnmock("@desktop/contentshield/wailsservice");
  });

  it("wire success — R1 ListProbes drives per-subject r1Count", async () => {
    vi.doMock("@desktop/training/wailsservice", () => ({
      Status: vi.fn().mockRejectedValue(new Error("not wired")),
    }));
    vi.doMock("@desktop/clbpl/wailsservice", () => ({
      Peaks: vi.fn().mockRejectedValue(new Error("not wired")),
    }));
    // Per-subject probe counts driven by which subject is asked for.
    const listProbes = vi.fn().mockImplementation((_model: string, subject: string) => {
      const counts: Record<string, string[]> = {
        english:   ["a", "b", "c", "d"],          // 4
        european:  ["a", "b"],                    // 2
        latam:     ["a", "b", "c", "d", "e", "f"],// 6
      };
      return Promise.resolve({ OK: true, Value: counts[subject] ?? [] });
    });
    vi.doMock("@desktop/r1/wailsservice", () => ({
      ListModels:   vi.fn().mockResolvedValue({ OK: true, Value: ["gemma4-e2b-it-q4"] }),
      ListSubjects: vi.fn().mockResolvedValue({ OK: true, Value: ["english", "european", "latam"] }),
      ListProbes:   listProbes,
    }));
    const { host, el } = await mountWindow<HTMLElement & {
      updateComplete: Promise<boolean>;
      _loadFromBackend: () => Promise<void>;
    }>("lthn-training-window");
    await el._loadFromBackend();
    await el.updateComplete;
    const rows = host.querySelectorAll(".training-rotation__row");
    const englishRow  = Array.from(rows).find(r => r.textContent?.includes("English"));
    const europeanRow = Array.from(rows).find(r => r.textContent?.includes("European"));
    const latamRow    = Array.from(rows).find(r => r.textContent?.includes("Latam"));
    expect(englishRow?.textContent).toContain("4 R₁");
    expect(europeanRow?.textContent).toContain("2 R₁");
    expect(latamRow?.textContent).toContain("6 R₁");
    // ListProbes was called once per returned subject.
    expect(listProbes).toHaveBeenCalledTimes(3);
    vi.doUnmock("@desktop/training/wailsservice");
    vi.doUnmock("@desktop/clbpl/wailsservice");
    vi.doUnmock("@desktop/r1/wailsservice");
  });

  it("wire success — R1Analytics.CrossTierCounts drives the cascade panel", async () => {
    vi.doMock("@desktop/training/wailsservice", () => ({
      Status: vi.fn().mockRejectedValue(new Error("not wired")),
    }));
    vi.doMock("@desktop/clbpl/wailsservice", () => ({
      Peaks: vi.fn().mockRejectedValue(new Error("not wired")),
    }));
    vi.doMock("@desktop/r1/wailsservice", () => ({
      ListModels:   vi.fn().mockRejectedValue(new Error("not wired")),
      ListSubjects: vi.fn().mockRejectedValue(new Error("not wired")),
      ListProbes:   vi.fn().mockRejectedValue(new Error("not wired")),
    }));
    // Tier 0 has captured english + european + russian; tier 1 has
    // only english (smallest-tier cascade has reached the bigger model
    // for one subject so far).
    vi.doMock("@desktop/r1/analytics/wailsservice", () => ({
      CrossTierCounts: vi.fn().mockResolvedValue({
        OK: true,
        Value: [
          { tier: 0, subject: "english",  count: 312 },
          { tier: 0, subject: "european", count: 187 },
          { tier: 0, subject: "russian",  count: 54  },
          { tier: 1, subject: "english",  count: 12  },
        ],
      }),
    }));
    const { host, el } = await mountWindow<HTMLElement & {
      updateComplete: Promise<boolean>;
      _loadFromBackend: () => Promise<void>;
    }>("lthn-training-window");
    await el._loadFromBackend();
    await el.updateComplete;

    const cascade = host.querySelector(".training-cascade");
    expect(cascade).not.toBeNull();
    const rows = host.querySelectorAll(".training-cascade__row");
    // Four canonical tiers (0..3) — _loadCascadeCounts seeds zero rows
    // for any tier missing from the backend so the ladder always
    // renders the full cascade.
    expect(rows.length).toBe(4);

    // Tier 0 total = 312 + 187 + 54 = 553.
    expect(rows[0]?.textContent).toContain("Tier 0");
    expect(rows[0]?.textContent).toContain("E2B");
    expect(rows[0]?.textContent).toContain("553 R₁");
    expect(rows[0]?.className).toContain("training-cascade__row--active");

    // Tier 1 total = 12.
    expect(rows[1]?.textContent).toContain("Tier 1");
    expect(rows[1]?.textContent).toContain("12 R₁");
    expect(rows[1]?.className).toContain("training-cascade__row--active");

    // Tier 2 + 3 empty — class flags --empty.
    expect(rows[2]?.textContent).toContain("0 R₁");
    expect(rows[2]?.className).toContain("training-cascade__row--empty");
    expect(rows[3]?.className).toContain("training-cascade__row--empty");

    vi.doUnmock("@desktop/training/wailsservice");
    vi.doUnmock("@desktop/clbpl/wailsservice");
    vi.doUnmock("@desktop/r1/wailsservice");
    vi.doUnmock("@desktop/r1/analytics/wailsservice");
  });

  it("wire fallback — R1Analytics unreachable keeps fixture cascade", async () => {
    vi.doMock("@desktop/training/wailsservice", () => ({
      Status: vi.fn().mockRejectedValue(new Error("not wired")),
    }));
    vi.doMock("@desktop/clbpl/wailsservice", () => ({
      Peaks: vi.fn().mockRejectedValue(new Error("not wired")),
    }));
    vi.doMock("@desktop/r1/wailsservice", () => ({
      ListModels:   vi.fn().mockRejectedValue(new Error("not wired")),
      ListSubjects: vi.fn().mockRejectedValue(new Error("not wired")),
      ListProbes:   vi.fn().mockRejectedValue(new Error("not wired")),
    }));
    vi.doMock("@desktop/r1/analytics/wailsservice", () => ({
      CrossTierCounts: vi.fn().mockRejectedValue(new Error("not wired")),
    }));
    const { host, el } = await mountWindow<HTMLElement & {
      updateComplete: Promise<boolean>;
      _loadFromBackend: () => Promise<void>;
    }>("lthn-training-window");
    await el._loadFromBackend();
    await el.updateComplete;

    // Fixture cascade has tier 0 totalR1=499 → renders that value.
    const rows = host.querySelectorAll(".training-cascade__row");
    expect(rows.length).toBe(4);
    expect(rows[0]?.textContent).toContain("499 R₁");

    vi.doUnmock("@desktop/training/wailsservice");
    vi.doUnmock("@desktop/clbpl/wailsservice");
    vi.doUnmock("@desktop/r1/wailsservice");
    vi.doUnmock("@desktop/r1/analytics/wailsservice");
  });
});

// SPDX-Licence-Identifier: EUPL-1.2

import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { mountWindow, expectChromeTitle, isEmbedded } from "../../test/window-fixture";

// Stub the Lemma admin binding so the hot-swap path is exercisable
// without a live lthn-mlx engine. Profiles() feeds the detail-rail
// profile <select>; Reload() is the assertion target for the
// "Use this model" affordance. Status/Machine/SFT* return benign
// shapes so _refreshLemmaAdmin clears lemmaUnavailable (engine
// reachable) and leaves no model loaded — keeping the Use button
// enabled rather than flipping to the "Loaded" no-op state.
//
// vi.hoisted() creates the spies in the same hoisted scope as vi.mock,
// so the factory can reference them safely (vitest's hoisting guard is
// satisfied) and the assertions below get the same instances.
const { mockReloadSpy, mockProfilesSpy } = vi.hoisted(() => ({
  mockReloadSpy: vi.fn(async (_req: { model_path?: string; profile_path?: string; adapter_path?: string }) => undefined),
  mockProfilesSpy: vi.fn(async () => ({ profiles: [
    { name: "fast",     path: "/profiles/fast.json",     backend: "mlx" },
    { name: "balanced", path: "/profiles/balanced.json", backend: "mlx" },
  ] })),
}));
vi.mock("@desktop/lemma/wailsservice", () => ({
  Status:      vi.fn(async () => ({ model_path: "", config: {}, loaded_at_unix: 0 })),
  Machine:     vi.fn(async () => ({ hash: "machine-test-hash" })),
  Profiles:    mockProfilesSpy,
  SFTAdapters: vi.fn(async () => ({ adapters: [] })),
  SFTStatus:   vi.fn(async () => ({ state: "idle" })),
  Reload:      mockReloadSpy,
}));

import "./model-browser-window";

// A loaded local model + selection, mirroring the deriveLocalModel
// shape pkg/models.List() yields — gives _renderActivate a path to
// reload and _doReload a model_path to send.
const SELECTED_PATH = "/models/gemma-3-1b.gguf";
interface BrowserEl extends HTMLElement {
  local: Array<{ id: string; name: string; family: string; size: string; status: string; path: string; isDir: boolean }>;
  selected: string;
  profiles: Array<{ name: string; path?: string; backend?: string }>;
  selectedProfile: string;
  reloadErr: string;
  reloadBusy: boolean;
  updateComplete: Promise<boolean>;
  requestUpdate: () => void;
}

describe("lthn-model-browser-window — smoke", () => {
  it("mounts with the Models titlebar", async () => {
    const { host } = await mountWindow("lthn-model-browser-window");
    expectChromeTitle(host, "Models");
    expect(host.querySelector("header")).not.toBeNull();
  });

  it("renders the Local rail header", async () => {
    const { host } = await mountWindow("lthn-model-browser-window");
    expect(host.textContent).toContain("Local");
  });

  it("renders the HuggingFace search results section", async () => {
    const { host } = await mountWindow("lthn-model-browser-window");
    expect(host.textContent).toContain("huggingface.co");
  });

  it("renders the right detail rail with the Selected header", async () => {
    const { host } = await mountWindow("lthn-model-browser-window");
    expect(host.textContent).toContain("Selected");
    // When no local entries are scanned, every selected-model field
    // renders as "—" — honest about the empty state rather than
    // showing the old fixture fallback ("gemma-4-e2b · Google · 2.1 GB").
    expect(host.textContent).toContain("—");
  });

  it("footer surfaces the model directory hint", async () => {
    const { host } = await mountWindow("lthn-model-browser-window");
    expect(host.textContent).toContain("~/.lthn/models/");
  });

  // The HF search surface renders its facet controls without a live
  // network result: a Base-only toggle + a Sort select. (Live result rows
  // — and their per-row Download buttons, which call the Lemma.Download
  // verified-fetch binding — need a network fetch the test env lacks.)
  it("renders the HF search facets — base-only toggle + sort select", async () => {
    const { host } = await mountWindow("lthn-model-browser-window");
    expect(host.textContent).toContain("Base models only");
    expect(host.querySelector("lthn-toggle")).not.toBeNull();
    expect(host.querySelector("select")).not.toBeNull();
  });
});

describe("lthn-model-browser-window — two-shell", () => {
  it("embedded mode collapses the chrome", async () => {
    const { host } = await mountWindow("lthn-model-browser-window", { attrs: { embedded: "" } });
    expect(isEmbedded(host)).toBe(true);
    expect(host.querySelector("header")).toBeNull();
  });
});

describe("lthn-model-browser-window — hot-swap (Mantis #1787)", () => {
  // Stub fetch so the window's initial _searchHF() rejects immediately
  // instead of opening a real request to huggingface.co — keeps the test
  // event loop free of lingering aborted network tasks.
  beforeEach(() => {
    vi.spyOn(globalThis, "fetch").mockRejectedValue(new Error("network disabled in test"));
  });
  afterEach(() => {
    vi.mocked(globalThis.fetch).mockRestore?.();
  });

  // Seed the detail-rail state directly so _renderActivate has a
  // selected, reachable model + a populated profile picker. The
  // connectedCallback's own async _refreshLemmaAdmin is exercised by the
  // "fetches profiles on connect" test below via the Profiles spy; here
  // we drive the rendered affordance deterministically rather than racing
  // the async refresh (which also reassigns this.local from models.List()).
  async function mountWithActivate(): Promise<{ host: HTMLDivElement; el: BrowserEl }> {
    const { host, el } = await mountWindow<BrowserEl>("lthn-model-browser-window");
    (el as unknown as { lemmaUnavailable: boolean }).lemmaUnavailable = false;
    (el as unknown as { machineHash: string }).machineHash = "machine-test-hash";
    el.profiles = [
      { name: "fast",     path: "/profiles/fast.json",     backend: "mlx" },
      { name: "balanced", path: "/profiles/balanced.json", backend: "mlx" },
    ];
    el.local = [
      { id: "gemma-3-1b", name: "gemma-3-1b.gguf", family: "Gemma", size: "1.0 GB", status: "available", path: SELECTED_PATH, isDir: false },
    ];
    el.selected = "gemma-3-1b";
    el.requestUpdate();
    await el.updateComplete;
    return { host, el };
  }

  it("fetches Lemma profiles on connect (wires Lemma.Profiles)", async () => {
    mockProfilesSpy.mockClear();
    const { el } = await mountWindow<BrowserEl>("lthn-model-browser-window");
    // connectedCallback → _refreshLemmaAdmin calls Lemma.Profiles() but is
    // async and not awaited by the mount fixture; poll until the spy fires.
    for (let i = 0; i < 80 && mockProfilesSpy.mock.calls.length === 0; i++) {
      await new Promise(r => setTimeout(r, 1));
    }
    void el;
    expect(mockProfilesSpy).toHaveBeenCalled();
  });

  it("renders the profile picker with each profile + the serve-default sentinel", async () => {
    const { host } = await mountWithActivate();
    // The detail-rail Profile picker renders each profile as an <option>
    // plus the "(use serve default)" sentinel. Query the option labels
    // directly — happy-dom keeps <option> text out of an ancestor's
    // textContent, so assert on the elements.
    const optionLabels = Array.from(host.querySelectorAll("option")).map(o => o.textContent ?? "");
    expect(optionLabels.some(l => l.includes("(use serve default)"))).toBe(true);
    expect(optionLabels.some(l => l.includes("fast"))).toBe(true);
    expect(optionLabels.some(l => l.includes("balanced"))).toBe(true);
  });

  it("Use this model calls Lemma.Reload with the selected model + profile", async () => {
    mockReloadSpy.mockClear();
    const { host, el } = await mountWithActivate();
    // Pick the second profile so the assertion proves the picked value
    // (not a default) reaches the Reload request.
    el.selectedProfile = "/profiles/balanced.json";
    el.requestUpdate();
    await el.updateComplete;

    // The activate button carries the swap label; find it by its copy.
    const buttons = Array.from(host.querySelectorAll("lthn-btn")) as HTMLElement[];
    const useBtn = buttons.find(b => (b.textContent ?? "").includes("Use this model"));
    expect(useBtn, "Use this model button should render for a selected, reachable model").toBeTruthy();

    useBtn!.click();
    // _doReload flips reloadBusy synchronously, then awaits two dynamic
    // imports (the binding + the ReloadRequest model) before calling
    // Reload and clearing reloadBusy in its finally. Poll until it
    // settles so the assertion sees the completed call rather than the
    // mid-flight state. Bounded so a stuck reload surfaces as a
    // call-count failure, not a hang.
    for (let i = 0; i < 80 && el.reloadBusy; i++) {
      await new Promise(r => setTimeout(r, 1));
    }
    await el.updateComplete;

    expect(mockReloadSpy).toHaveBeenCalledTimes(1);
    const req = mockReloadSpy.mock.calls[0][0] as { model_path?: string; profile_path?: string };
    expect(req.model_path).toBe(SELECTED_PATH);
    expect(req.profile_path).toBe("/profiles/balanced.json");
    // No error surfaced on the happy path.
    expect(el.reloadErr).toBe("");
  });
});

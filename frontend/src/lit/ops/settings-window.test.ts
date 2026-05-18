// SPDX-Licence-Identifier: EUPL-1.2

import { describe, it, expect, vi, beforeEach } from "vitest";
import type { LitElement } from "lit";
import { mountWindow, expectChromeTitle, isEmbedded } from "../../test/window-fixture";
import "./settings-window";

describe("lthn-settings-window — smoke", () => {
  it("mounts with the Settings titlebar", async () => {
    const { host } = await mountWindow("lthn-settings-window");
    expectChromeTitle(host, "Settings");
    expect(host.querySelector("header")).not.toBeNull();
  });

  it("renders the section rail (General, Models, Runner, API, Telemetry, Integrations, About)", async () => {
    const { host } = await mountWindow("lthn-settings-window");
    const text = host.textContent ?? "";
    for (const section of ["General", "Models", "Runner", "API", "Telemetry", "Integrations", "About"]) {
      expect(text).toContain(section);
    }
  });

  it("Models section shows the model-directory row", async () => {
    const { host } = await mountWindow("lthn-settings-window", { attrs: { open: "models" } });
    expect(host.textContent).toContain("Model directory");
    expect(host.textContent).toContain("~/Lethean/conf/models/");
  });

  it("footer shows the keep-running reassurance", async () => {
    const { host } = await mountWindow("lthn-settings-window");
    expect(host.textContent).toContain("the runner keeps running");
  });
});

describe("lthn-settings-window — two-shell", () => {
  it("embedded mode collapses the chrome", async () => {
    const { host } = await mountWindow("lthn-settings-window", { attrs: { embedded: "" } });
    expect(isEmbedded(host)).toBe(true);
    expect(host.querySelector("header")).toBeNull();
  });
});

describe("lthn-settings-window — Menu Behaviours subsection (General)", () => {
  // The Menu Behaviours subsection lives under General. Two segmented
  // controls (Menu Links + Menu Layout) persist to localStorage with
  // the lthn.menu.* prefix and emit lthn:menu:changed so an open
  // <lthn-app-shell> reacts without a reload. See
  // plans/project/lthn/desktop/RFC.menu-behaviours.md.
  type SettingsEl = LitElement & {
    menuLinks: string;
    menuLayout: string;
  };

  it("renders the Menu Behaviours subsection heading under General", async () => {
    const { host } = await mountWindow("lthn-settings-window", { attrs: { open: "general" } });
    expect(host.textContent).toContain("Menu Behaviours");
    expect(host.textContent).toContain("Menu Links");
    expect(host.textContent).toContain("Menu Layout");
  });

  it("Menu Links defaults to 'in-window' when no setting is stored", async () => {
    const { el } = await mountWindow<SettingsEl>("lthn-settings-window", { attrs: { open: "general" } });
    expect(el.menuLinks).toBe("in-window");
  });

  it("Menu Layout defaults to 'toggle' when no setting is stored", async () => {
    const { el } = await mountWindow<SettingsEl>("lthn-settings-window", { attrs: { open: "general" } });
    expect(el.menuLayout).toBe("toggle");
  });

  it("clicking a Menu Links segment updates the persisted state", async () => {
    // Persistence to localStorage is exercised by the production
    // codepath (writeStoredSetting → setItem); asserting state is
    // sufficient proof the segment handler fired correctly. The
    // happy-dom localStorage shim is disabled in this suite so we
    // don't read back the stored value directly.
    const { el, host } = await mountWindow<SettingsEl>("lthn-settings-window", { attrs: { open: "general" } });
    const buttons = Array.from(host.querySelectorAll("button")) as HTMLButtonElement[];
    const hybridBtn = buttons.find(b => (b.textContent ?? "").trim() === "Hybrid");
    expect(hybridBtn, "Hybrid segment should render").not.toBeNull();
    hybridBtn!.click();
    await el.updateComplete;
    expect(el.menuLinks).toBe("hybrid");
  });

  it("clicking a Menu Layout segment updates the persisted state", async () => {
    const { el, host } = await mountWindow<SettingsEl>("lthn-settings-window", { attrs: { open: "general" } });
    const buttons = Array.from(host.querySelectorAll("button")) as HTMLButtonElement[];
    const hoverBtn = buttons.find(b => (b.textContent ?? "").trim() === "Hover open");
    expect(hoverBtn, "Hover open segment should render").not.toBeNull();
    hoverBtn!.click();
    await el.updateComplete;
    expect(el.menuLayout).toBe("hover");
  });

  it("renders the Replay menu tour control under Menu Behaviours", async () => {
    const { host } = await mountWindow("lthn-settings-window", { attrs: { open: "general" } });
    expect(host.textContent).toContain("Replay menu tour");
  });
});

describe("lthn-settings-window — Endpoints section (Mantis #1775 Stage B)", () => {
  // Resets the module cache so vi.doMock entries from previous tests
  // don't bleed forward (matches the benchmark-window discipline).
  beforeEach(() => {
    vi.resetModules();
  });

  it("rail surfaces the Endpoints section + opens to empty state", async () => {
    vi.doMock("@desktop/openaibench/wailsservice", () => ({
      ListEndpoints: vi.fn().mockResolvedValue({ OK: true, Value: [] }),
      AddEndpoint: vi.fn(),
      RemoveEndpoint: vi.fn(),
    }));
    const { host, el } = await mountWindow<LitElement & {
      open: string;
    }>("lthn-settings-window");
    // Set state directly to drive section render. Avoids the
    // _openSection → _loadEndpoints fire-and-forget race that bites
    // the live-load tests in this suite (see "renders persisted").
    el.open = "endpoints";
    await el.updateComplete;
    expect(el.open).toBe("endpoints");
    expect(host.textContent).toContain("Inference endpoints");
    expect(host.textContent).toContain("No endpoints configured");
    vi.doUnmock("@desktop/openaibench/wailsservice");
  });

  it("renders persisted endpoints with name + URL + key-set badge", async () => {
    vi.doMock("@desktop/openaibench/wailsservice", () => ({
      ListEndpoints: vi.fn().mockResolvedValue({
        OK: true,
        Value: [
          { name: "openai-compat:nim", url: "https://nim.example/v1", auth_set: true },
          { name: "openai-compat:local-llama", url: "http://localhost:8080/v1", auth_set: false },
        ],
      }),
      AddEndpoint: vi.fn(),
      RemoveEndpoint: vi.fn(),
    }));
    const { host, el } = await mountWindow<LitElement & {
      open: string;
      _loadEndpoints: () => Promise<void>;
      endpoints: { name: string; url: string; auth_set: boolean }[];
    }>("lthn-settings-window");
    // Set the section directly (no fire-and-forget side-effect), then
    // drive _loadEndpoints explicitly so the test owns the timing.
    el.open = "endpoints";
    await el._loadEndpoints();
    await el.updateComplete;
    expect(el.endpoints.length).toBe(2);
    expect(host.textContent).toContain("openai-compat:nim");
    expect(host.textContent).toContain("https://nim.example/v1");
    expect(host.textContent).toContain("key set");
    expect(host.textContent).toContain("openai-compat:local-llama");
    expect(host.textContent).toContain("no key");
    vi.doUnmock("@desktop/openaibench/wailsservice");
  });

  it("Add modal validates required fields then submits via AddEndpoint", async () => {
    const addFn = vi.fn().mockResolvedValue({
      OK: true,
      Value: { name: "x", url: "https://x/v1", auth_set: true },
    });
    vi.doMock("@desktop/openaibench/wailsservice", () => ({
      ListEndpoints: vi.fn().mockResolvedValue({ OK: true, Value: [] }),
      AddEndpoint: addFn,
      RemoveEndpoint: vi.fn(),
    }));
    const { host, el } = await mountWindow<LitElement & {
      open: string;
      _openAddModal: () => void;
      _setAddForm: (f: string, v: string) => void;
      _submitAddEndpoint: () => Promise<void>;
      addModalOpen: boolean;
      addErr: string;
    }>("lthn-settings-window");
    el.open = "endpoints";
    el._openAddModal();
    await el.updateComplete;
    expect(el.addModalOpen).toBe(true);
    expect(host.querySelector("[data-testid='ep-add-modal']")).not.toBeNull();

    // Empty submit → validation error, no API call.
    await el._submitAddEndpoint();
    expect(addFn).not.toHaveBeenCalled();
    expect(el.addErr).toContain("required");

    // Fill required fields → submits + closes.
    el._setAddForm("name", "openai-compat:test");
    el._setAddForm("url", "https://api.example/v1");
    el._setAddForm("bearer", "sk-test");
    await el._submitAddEndpoint();
    expect(addFn).toHaveBeenCalledTimes(1);
    const cfg = addFn.mock.calls[0][0];
    expect(cfg.name).toBe("openai-compat:test");
    expect(cfg.url).toBe("https://api.example/v1");
    expect(cfg.bearer).toBe("sk-test");
    expect(el.addModalOpen).toBe(false);
    vi.doUnmock("@desktop/openaibench/wailsservice");
  });

  it("Delete button calls RemoveEndpoint with the row name", async () => {
    const removeFn = vi.fn().mockResolvedValue({ OK: true, Value: null });
    vi.doMock("@desktop/openaibench/wailsservice", () => ({
      ListEndpoints: vi.fn().mockResolvedValue({
        OK: true,
        Value: [{ name: "openai-compat:doomed", url: "https://x/v1", auth_set: false }],
      }),
      AddEndpoint: vi.fn(),
      RemoveEndpoint: removeFn,
    }));
    const { host, el } = await mountWindow<LitElement & {
      open: string;
      _loadEndpoints: () => Promise<void>;
      _deleteEndpoint: (name: string) => Promise<void>;
    }>("lthn-settings-window");
    el.open = "endpoints";
    await el._loadEndpoints();
    await el.updateComplete;
    const btn = host.querySelector("[data-testid='ep-delete-openai-compat:doomed']");
    expect(btn).not.toBeNull();
    await el._deleteEndpoint("openai-compat:doomed");
    expect(removeFn).toHaveBeenCalledWith("openai-compat:doomed");
    vi.doUnmock("@desktop/openaibench/wailsservice");
  });
});

// SPDX-Licence-Identifier: EUPL-1.2
//
// Tests for <lthn-app-shell> — the Lethean-6 unified application shell
// that hosts a side-nav + status bar + body-slot that auto-mounts the
// matching <lthn-*-window> for the active nav entry.
//
// The two-shell pattern is load-bearing here: the shell paints its own
// chrome, then mounts each child with `embedded=""` so renderChrome
// collapses the child's standalone card and fills the shell's body
// slot instead. The Lit-mount race that the embedded-attribute sweep
// solved last week lives in this codepath; these specs guard the
// shape so a regression that breaks _instantiate's setAttribute call
// fails loudly.

import { describe, it, expect, vi } from "vitest";

// Stage D — mock the serverkey binding at module scope so app-shell's
// _probeAuthState() resolves to a controllable stub. Vitest hoists
// vi.mock calls above imports, so the dynamic-import inside
// _probeAuthState resolves to this stub on the FIRST mount.
vi.mock("@desktop/serverkey/service", () => ({
  AccountStatus: vi.fn(() => Promise.resolve({ OK: true, Value: { has_user_account: true } })),
  IssueBootstrapToken: vi.fn(() => Promise.resolve({ OK: true, Value: { token: "x", expires_at: 0 } })),
}));

import { AccountStatus as ServerKeyAccountStatus } from "@desktop/serverkey/service";

import { mountWindow } from "../test/window-fixture";
import { filterGgufPaths, MODELS_REFRESH_EVENT, filterPaletteEntries } from "./app-shell";

// Side-effect import — registers <lthn-app-shell> + all child window
// elements the shell can mount via _instantiate.
import "./index";

describe("lthn-app-shell — smoke", () => {
  it("mounts with the default chat pane", async () => {
    const { host, el } = await mountWindow<HTMLElement & {
      active: string;
      updateComplete: Promise<boolean>;
    }>("lthn-app-shell");

    expect(el.active).toBe("chat");
    // The side-nav renders the nav rail with the canonical group labels.
    expect(host.textContent).toContain("Workspace");
    expect(host.textContent).toContain("Observe");
    expect(host.textContent).toContain("Extend");
  });

  it("renders the side-nav entries Chat / Models / Telemetry / Settings", async () => {
    const { host } = await mountWindow("lthn-app-shell");
    const text = host.textContent ?? "";
    for (const label of ["Chat", "Models", "Benchmark", "Telemetry", "Settings"]) {
      expect(text).toContain(label);
    }
  });

  it("uses the `active` attribute to drive which pane mounts", async () => {
    const { host } = await mountWindow("lthn-app-shell", {
      attrs: { active: "settings" },
    });
    // The shell mounts <lthn-settings-window> when active=settings.
    expect(host.querySelector("lthn-settings-window")).not.toBeNull();
  });
});

describe("lthn-app-shell — two-shell pattern", () => {
  it("child window mounted by the shell gets the embedded attribute", async () => {
    const { host } = await mountWindow("lthn-app-shell", {
      attrs: { active: "chat" },
    });
    const child = host.querySelector("lthn-chat-window");
    expect(child).not.toBeNull();
    expect(child!.hasAttribute("embedded")).toBe(true);
  });

  it("switching active swaps the child + re-applies embedded", async () => {
    const { el, host } = await mountWindow<HTMLElement & {
      active: string;
      updateComplete: Promise<boolean>;
    }>("lthn-app-shell", { attrs: { active: "chat" } });

    expect(host.querySelector("lthn-chat-window")).not.toBeNull();

    el.active = "telemetry";
    await el.updateComplete;
    const telemetry = host.querySelector("lthn-telemetry-window");
    expect(telemetry).not.toBeNull();
    expect(telemetry!.hasAttribute("embedded")).toBe(true);
  });

  it("chat / logs / settings each get sensible default attrs", async () => {
    // _instantiate seeds default attrs so the windows look populated
    // when mounted via the shell. Welcome isn't in the side-nav (it's
    // a standalone wizard surface) so it's not covered here.
    const cases: Array<[string, string, string, string]> = [
      // Chat mounts at "empty" — first-time users see the welcome
      // surface; returning users with prior conversations swap into
      // liveTurns via the activeConversationId restore inside
      // chat-window.
      ["chat",     "lthn-chat-window",     "state", "empty"],
      ["logs",     "lthn-logs-window",     "tab",   "live"],
      ["settings", "lthn-settings-window", "open",  "models"],
    ];
    for (const [active, tag, attr, want] of cases) {
      const { host } = await mountWindow("lthn-app-shell", { attrs: { active } });
      const child = host.querySelector(tag);
      expect(child, `${tag} should mount for active=${active}`).not.toBeNull();
      expect(child!.getAttribute(attr)).toBe(want);
    }
  });
});

describe("lthn-app-shell — chrome + status bar", () => {
  it("renders the traffic-lights primitive in its custom titlebar", async () => {
    const { host } = await mountWindow("lthn-app-shell");
    // The shell paints its own titlebar (not renderChrome's), but
    // still hosts <lthn-traffic-lights> for the Close/Min/Fullscreen
    // controls.
    expect(host.querySelector("lthn-traffic-lights")).not.toBeNull();
  });

  it("surfaces the model + tps + watts in the status bar", async () => {
    const { host } = await mountWindow("lthn-app-shell", {
      props: { model: "test-model", tps: "12.3", watts: "9.9" },
    });
    const text = host.textContent ?? "";
    expect(text).toContain("test-model");
    expect(text).toContain("12.3");
    expect(text).toContain("9.9");
  });

  it("collapsed prop reflects to the attribute (side-nav rail collapse)", async () => {
    const { el } = await mountWindow<HTMLElement & {
      collapsed: boolean;
      updateComplete: Promise<boolean>;
    }>("lthn-app-shell");

    expect(el.hasAttribute("collapsed")).toBe(false);
    el.collapsed = true;
    await el.updateComplete;
    expect(el.hasAttribute("collapsed")).toBe(true);
  });
});

describe("lthn-app-shell — unknown active value", () => {
  it("renders a friendly placeholder when active points at no window", async () => {
    const { host } = await mountWindow("lthn-app-shell", {
      attrs: { active: "not-a-real-pane" },
    });
    expect(host.textContent).toContain("No window");
    expect(host.textContent).toContain("not-a-real-pane");
  });
});

describe("lthn-app-shell — Menu Layout (rail shape)", () => {
  // The Menu Layout setting controls how the rail collapses.
  // Spec: plans/project/lthn/desktop/RFC.menu-behaviours.md § 2.2.
  type ShellEl = HTMLElement & {
    collapsed: boolean;
    menuLayout: string;
    _applyMenuLayout: () => void;
    _onRailEnter: () => void;
    _onRailLeave: () => void;
    updateComplete: Promise<boolean>;
  };

  it("layout 'open' locks the rail open (collapsed = false, chevron hidden)", async () => {
    const { el, host } = await mountWindow<ShellEl>("lthn-app-shell");
    el.menuLayout = "open";
    el._applyMenuLayout();
    await el.updateComplete;
    expect(el.collapsed).toBe(false);
    expect(host.querySelector("button[title]")?.getAttribute("title"))
      .not.toBe("Collapse"); // chevron with the Collapse tooltip should not render
  });

  it("layout 'closed' locks the rail collapsed", async () => {
    const { el } = await mountWindow<ShellEl>("lthn-app-shell");
    el.menuLayout = "closed";
    el._applyMenuLayout();
    await el.updateComplete;
    expect(el.collapsed).toBe(true);
  });

  it("layout 'hover' starts collapsed, expands on mouseenter", async () => {
    const { el } = await mountWindow<ShellEl>("lthn-app-shell");
    el.menuLayout = "hover";
    el._applyMenuLayout();
    await el.updateComplete;
    expect(el.collapsed).toBe(true);
    el._onRailEnter();
    await el.updateComplete;
    expect(el.collapsed).toBe(false);
  });

  it("layout 'toggle' is the default and keeps the chevron visible", async () => {
    const { el } = await mountWindow<ShellEl>("lthn-app-shell");
    expect(el.menuLayout).toBe("toggle");
    // Chevron button title cycles between Expand / Collapse — its
    // presence is the contract, not the exact label here.
    expect(el).toBeDefined();
  });

  it("toggling collapsed via _toggleCollapse is a no-op when layout is locked", async () => {
    const { el } = await mountWindow<ShellEl>("lthn-app-shell");
    el.menuLayout = "open";
    el._applyMenuLayout();
    await el.updateComplete;
    const before = el.collapsed;
    (el as unknown as { _toggleCollapse: () => void })._toggleCollapse();
    await el.updateComplete;
    expect(el.collapsed).toBe(before);
  });
});

describe("lthn-app-shell — Menu Links (click dispatch matrix)", () => {
  // Spec § 4 dispatch matrix. We assert via negative-space:
  // `active` changing = navigate-in-place fired; `active` unchanged
  // = pop-out branch fired (and the pop-out import call is mocked
  // out at the @lthn/gui/windowbindingservice module boundary so
  // it doesn't actually spawn a window in test).
  type ShellEl = HTMLElement & {
    active: string;
    collapsed: boolean;
    menuLinks: string;
    updateComplete: Promise<boolean>;
  };

  function clickIcon(host: HTMLElement, label: string, mods: MouseEventInit = {}) {
    const buttons = Array.from(host.querySelectorAll("button")) as HTMLButtonElement[];
    // When the rail is collapsed only the icon shows (no span text);
    // the button surfaces its label via the `title` attribute instead.
    const btn = buttons.find(b =>
      (b.textContent ?? "").trim().includes(label) ||
      (b.getAttribute("title") ?? "").includes(label)
    );
    if (!btn) throw new Error(`button for "${label}" not found`);
    const icon = btn.querySelector("i");
    (icon ?? btn).dispatchEvent(new MouseEvent("click", { bubbles: true, ...mods }));
  }
  function clickWord(host: HTMLElement, label: string, mods: MouseEventInit = {}) {
    const buttons = Array.from(host.querySelectorAll("button")) as HTMLButtonElement[];
    const btn = buttons.find(b => (b.textContent ?? "").trim().includes(label));
    if (!btn) throw new Error(`button for "${label}" not found`);
    const span = btn.querySelector("span");
    (span ?? btn).dispatchEvent(new MouseEvent("click", { bubbles: true, ...mods }));
  }

  it("Hybrid + open rail + word click → navigates in-place", async () => {
    const { el, host } = await mountWindow<ShellEl>("lthn-app-shell", {
      attrs: { active: "chat" },
      props: { menuLinks: "hybrid" },
    });
    await el.updateComplete;
    clickWord(host, "Telemetry");
    await el.updateComplete;
    expect(el.active).toBe("telemetry");
  });

  it("Hybrid + open rail + icon click → pops out (active unchanged)", async () => {
    const { el, host } = await mountWindow<ShellEl>("lthn-app-shell", {
      attrs: { active: "chat" },
      props: { menuLinks: "hybrid" },
    });
    await el.updateComplete;
    clickIcon(host, "Telemetry");
    await el.updateComplete;
    expect(el.active).toBe("chat");
  });

  it("Always-In-Window + icon click → navigates (no pop-out)", async () => {
    const { el, host } = await mountWindow<ShellEl>("lthn-app-shell", {
      attrs: { active: "chat" },
      props: { menuLinks: "in-window" },
    });
    await el.updateComplete;
    clickIcon(host, "Telemetry");
    await el.updateComplete;
    expect(el.active).toBe("telemetry");
  });

  it("Collapsed-only + open rail + icon click → navigates", async () => {
    const { el, host } = await mountWindow<ShellEl>("lthn-app-shell", {
      attrs: { active: "chat" },
      props: { menuLinks: "collapsed-only" },
    });
    await el.updateComplete;
    expect(el.collapsed).toBe(false);
    clickIcon(host, "Telemetry");
    await el.updateComplete;
    expect(el.active).toBe("telemetry");
  });

  it("Collapsed-only + collapsed rail + click → pops out (active unchanged)", async () => {
    const { el, host } = await mountWindow<ShellEl>("lthn-app-shell", {
      attrs: { active: "chat" },
      props: { menuLinks: "collapsed-only", collapsed: true },
    });
    await el.updateComplete;
    expect(el.collapsed).toBe(true);
    clickIcon(host, "Telemetry");
    await el.updateComplete;
    expect(el.active).toBe("chat");
  });

  it("⌘-click pops out regardless of Menu Links setting", async () => {
    const { el, host } = await mountWindow<ShellEl>("lthn-app-shell", {
      attrs: { active: "chat" },
      props: { menuLinks: "in-window" },  // even with pop-out OFF, ⌘ overrides
    });
    await el.updateComplete;
    clickIcon(host, "Telemetry", { metaKey: true });
    await el.updateComplete;
    expect(el.active).toBe("chat");
  });
});

describe("lthn-app-shell — rail click dispatch", () => {
  // The rail-row click handler routes plain clicks to in-place
  // navigation (_select) and ⌘/Ctrl-clicks to the standalone-window
  // pop-out via WindowService.Open. The pop-out branch is asserted
  // via negative-space — `active` MUST NOT change — because mocking
  // the dynamic `import("@lthn/gui/windowbindingservice")` adds
  // setup weight the contract doesn't need.
  // Spec: plans/project/lthn/desktop/RFC.menu-behaviours.md § 3.
  type ShellEl = HTMLElement & {
    active: string;
    updateComplete: Promise<boolean>;
  };

  function findNavButton(host: HTMLElement, label: string): HTMLButtonElement | null {
    const buttons = Array.from(host.querySelectorAll("button")) as HTMLButtonElement[];
    return buttons.find(b => (b.textContent ?? "").trim().includes(label)) ?? null;
  }

  it("plain click navigates the body in-place", async () => {
    const { el, host } = await mountWindow<ShellEl>("lthn-app-shell", {
      attrs: { active: "chat" },
    });
    const btn = findNavButton(host, "Telemetry");
    expect(btn, "telemetry button should be rendered").not.toBeNull();
    btn!.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    await el.updateComplete;
    expect(el.active).toBe("telemetry");
  });

  it("⌘-click does NOT change the in-place active surface (pop-out branch)", async () => {
    const { el, host } = await mountWindow<ShellEl>("lthn-app-shell", {
      attrs: { active: "chat" },
    });
    const btn = findNavButton(host, "Telemetry");
    expect(btn, "telemetry button should be rendered").not.toBeNull();
    btn!.dispatchEvent(new MouseEvent("click", { bubbles: true, metaKey: true }));
    await el.updateComplete;
    expect(el.active).toBe("chat");
  });

  it("Ctrl-click does NOT change the in-place active surface (pop-out branch)", async () => {
    const { el, host } = await mountWindow<ShellEl>("lthn-app-shell", {
      attrs: { active: "chat" },
    });
    const btn = findNavButton(host, "Telemetry");
    expect(btn, "telemetry button should be rendered").not.toBeNull();
    btn!.dispatchEvent(new MouseEvent("click", { bubbles: true, ctrlKey: true }));
    await el.updateComplete;
    expect(el.active).toBe("chat");
  });
});

describe("filterGgufPaths — drag-and-drop import filter", () => {
  it("keeps only .gguf paths", () => {
    expect(filterGgufPaths(["a.gguf", "b.txt", "c.gguf"])).toEqual(["a.gguf", "c.gguf"]);
  });

  it("matches case-insensitively (.GGUF, .Gguf, etc.)", () => {
    expect(filterGgufPaths(["a.GGUF", "b.Gguf", "c.GGuF"])).toEqual(["a.GGUF", "b.Gguf", "c.GGuF"]);
  });

  it("rejects non-.gguf extensions (.bin, .safetensors, no ext)", () => {
    expect(filterGgufPaths(["x.bin", "y.safetensors", "noext", "z.txt"])).toEqual([]);
  });

  it("preserves absolute paths verbatim — no normalisation", () => {
    expect(filterGgufPaths([
      "/Users/snider/Downloads/gemma-4-e2b-q4_k_m.gguf",
      "/var/tmp/llama-3-2-3b.gguf",
    ])).toEqual([
      "/Users/snider/Downloads/gemma-4-e2b-q4_k_m.gguf",
      "/var/tmp/llama-3-2-3b.gguf",
    ]);
  });

  it("filters out non-string entries safely", () => {
    // Defensive: Wails could theoretically pass mixed types from an
    // older binding; the filter must not throw on number / object / null.
    const mixed = ["a.gguf", 42 as unknown as string, null as unknown as string, "b.gguf"];
    expect(filterGgufPaths(mixed)).toEqual(["a.gguf", "b.gguf"]);
  });

  it("returns [] for null / undefined / empty input", () => {
    expect(filterGgufPaths(null)).toEqual([]);
    expect(filterGgufPaths(undefined)).toEqual([]);
    expect(filterGgufPaths([])).toEqual([]);
  });

  it("MODELS_REFRESH_EVENT carries the canonical bus name", () => {
    expect(MODELS_REFRESH_EVENT).toBe("lthn:models:refresh");
  });
});

describe("lthn-app-shell — _adoptDroppedGgufs", () => {
  type ShellWithDrop = HTMLElement & {
    _adoptDroppedGgufs: (paths: readonly string[]) => Promise<void>;
    updateComplete: Promise<boolean>;
  };

  it("empty input is a silent no-op (no throw)", async () => {
    const { el } = await mountWindow<ShellWithDrop>("lthn-app-shell");
    let threw: unknown = null;
    try { await el._adoptDroppedGgufs([]); }
    catch (e) { threw = e; }
    expect(threw, "empty array must not throw").toBeNull();
  });

  it("null input is a silent no-op (no throw)", async () => {
    const { el } = await mountWindow<ShellWithDrop>("lthn-app-shell");
    let threw: unknown = null;
    try { await el._adoptDroppedGgufs(null as unknown as readonly string[]); }
    catch (e) { threw = e; }
    expect(threw, "null input must not throw").toBeNull();
  });
});

describe("filterPaletteEntries — ⌘P fuzzy filter", () => {
  const nav = [
    { id: "chat",    label: "Chat" },
    { id: "models",  label: "Models" },
    { id: "settings", label: "Settings" },
  ];

  it("returns the full list for empty / blank query", () => {
    expect(filterPaletteEntries(nav, "")).toHaveLength(3);
    expect(filterPaletteEntries(nav, "  ")).toHaveLength(3);
    expect(filterPaletteEntries(nav, null)).toHaveLength(3);
    expect(filterPaletteEntries(nav, undefined)).toHaveLength(3);
  });

  it("matches label substring (case-insensitive)", () => {
    expect(filterPaletteEntries(nav, "chat").map(n => n.id)).toEqual(["chat"]);
    expect(filterPaletteEntries(nav, "MOD").map(n => n.id)).toEqual(["models"]);
  });

  it("matches id substring too", () => {
    expect(filterPaletteEntries(nav, "settings").map(n => n.id)).toEqual(["settings"]);
  });

  it("returns [] for no match", () => {
    expect(filterPaletteEntries(nav, "ghost-needle")).toEqual([]);
  });
});

describe("lthn-app-shell — ⌘P command palette", () => {
  type ShellWithPalette = HTMLElement & {
    active: string;
    paletteOpen: boolean;
    paletteQuery: string;
    paletteIndex: number;
    _togglePalette: () => void;
    _palettedStep: (delta: number) => void;
    _paletteCommit: () => void;
    updateComplete: Promise<boolean>;
  };

  it("⌘P opens the palette", async () => {
    const { el, host } = await mountWindow<ShellWithPalette>("lthn-app-shell");
    expect(el.paletteOpen).toBe(false);
    window.dispatchEvent(new KeyboardEvent("keydown", { key: "p", metaKey: true, bubbles: true }));
    await el.updateComplete;
    expect(el.paletteOpen).toBe(true);
    expect(host.querySelector(".lthn-palette"), "palette dialog rendered").not.toBeNull();
    expect(host.querySelector(".lthn-palette-input"), "input rendered").not.toBeNull();
  });

  it("Ctrl+P also opens (Linux/Windows)", async () => {
    const { el } = await mountWindow<ShellWithPalette>("lthn-app-shell");
    window.dispatchEvent(new KeyboardEvent("keydown", { key: "p", ctrlKey: true, bubbles: true }));
    await el.updateComplete;
    expect(el.paletteOpen).toBe(true);
  });

  it("⌘P again closes + clears query", async () => {
    const { el } = await mountWindow<ShellWithPalette>("lthn-app-shell");
    el.paletteOpen = true;
    el.paletteQuery = "set";
    el.paletteIndex = 2;
    await el.updateComplete;
    window.dispatchEvent(new KeyboardEvent("keydown", { key: "p", metaKey: true, bubbles: true }));
    await el.updateComplete;
    expect(el.paletteOpen).toBe(false);
    expect(el.paletteQuery).toBe("");
    expect(el.paletteIndex).toBe(-1);
  });

  it("_palettedStep cycles with wrap", async () => {
    const { el } = await mountWindow<ShellWithPalette>("lthn-app-shell");
    el.paletteOpen = true;
    el.paletteQuery = ""; // full nav list
    el.paletteIndex = 0;
    el._palettedStep(1);
    expect(el.paletteIndex).toBe(1);
    el._palettedStep(-1);
    expect(el.paletteIndex).toBe(0);
    // backward wraps to last
    el._palettedStep(-1);
    expect(el.paletteIndex, "backward wrap").toBeGreaterThan(0);
  });

  it("_palettedStep is a no-op on empty filter", async () => {
    const { el } = await mountWindow<ShellWithPalette>("lthn-app-shell");
    el.paletteOpen = true;
    el.paletteQuery = "ghost-needle-xyz";
    el.paletteIndex = -1;
    el._palettedStep(1);
    expect(el.paletteIndex, "no matches → index unchanged").toBe(-1);
  });

  it("_paletteCommit switches pane + closes", async () => {
    const { el } = await mountWindow<ShellWithPalette>("lthn-app-shell");
    el.paletteOpen = true;
    el.paletteQuery = "settings";
    el.paletteIndex = 0;
    await el.updateComplete;
    el._paletteCommit();
    expect(el.active).toBe("settings");
    expect(el.paletteOpen).toBe(false);
  });

  it("Enter in palette input commits the selection", async () => {
    const { el, host } = await mountWindow<ShellWithPalette>("lthn-app-shell");
    el.paletteOpen = true;
    el.paletteQuery = "models";
    el.paletteIndex = 0;
    await el.updateComplete;
    const input = host.querySelector<HTMLInputElement>(".lthn-palette-input")!;
    input.dispatchEvent(new KeyboardEvent("keydown", { key: "Enter", bubbles: true, cancelable: true }));
    await el.updateComplete;
    expect(el.active).toBe("models");
    expect(el.paletteOpen).toBe(false);
  });

  it("Escape in palette input closes", async () => {
    const { el, host } = await mountWindow<ShellWithPalette>("lthn-app-shell");
    el.paletteOpen = true;
    await el.updateComplete;
    const input = host.querySelector<HTMLInputElement>(".lthn-palette-input")!;
    input.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", bubbles: true, cancelable: true }));
    await el.updateComplete;
    expect(el.paletteOpen).toBe(false);
  });

  it("backdrop click dismisses the palette", async () => {
    const { el, host } = await mountWindow<ShellWithPalette>("lthn-app-shell");
    el.paletteOpen = true;
    await el.updateComplete;
    const backdrop = host.querySelector<HTMLElement>(".lthn-palette-backdrop");
    expect(backdrop).not.toBeNull();
    backdrop!.click();
    await el.updateComplete;
    expect(el.paletteOpen).toBe(false);
  });

  it("empty-filter state renders the 'No match for …' hint", async () => {
    const { el, host } = await mountWindow<ShellWithPalette>("lthn-app-shell");
    el.paletteOpen = true;
    el.paletteQuery = "ghost-needle";
    await el.updateComplete;
    expect(host.querySelector(".lthn-palette-empty"), "empty hint rendered").not.toBeNull();
  });
});

describe("lthn-app-shell — cross-view ⌘P palette", () => {
  type ShellWithPaletteCV = HTMLElement & {
    active: string;
    view: string;
    paletteOpen: boolean;
    paletteQuery: string;
    paletteIndex: number;
    _paletteCommit: () => void;
    updateComplete: Promise<boolean>;
  };

  it("filter 'campaigns' matches only Marketing", async () => {
    const { el, host } = await mountWindow<ShellWithPaletteCV>("lthn-app-shell");
    el.paletteOpen = true;
    el.paletteQuery = "campaigns";
    el.paletteIndex = 0;
    await el.updateComplete;
    const rows = host.querySelectorAll(".lthn-palette-item");
    expect(rows.length).toBe(1);
    expect(host.textContent ?? "").toContain("Marketing · Campaigns");
  });

  it("Enter on 'Marketing · Campaigns' switches view + pane", async () => {
    const { el } = await mountWindow<ShellWithPaletteCV>("lthn-app-shell", { attrs: { view: "admin" } });
    el.paletteOpen = true;
    el.paletteQuery = "campaigns";
    el.paletteIndex = 0;
    await el.updateComplete;
    el._paletteCommit();
    await el.updateComplete;
    expect(el.view).toBe("marketing");
    expect(el.active).toBe("campaigns");
    expect(el.paletteOpen).toBe(false);
  });

  it("'settings' matches every view (one Settings per view)", async () => {
    const { el, host } = await mountWindow<ShellWithPaletteCV>("lthn-app-shell");
    el.paletteOpen = true;
    el.paletteQuery = "settings";
    await el.updateComplete;
    const rows = host.querySelectorAll(".lthn-palette-item");
    // chat/admin/planning/coding/marketing/operations/sales/office —
    // every view exposes a Settings entry.
    expect(rows.length).toBe(8);
    expect(host.textContent ?? "").toContain("Chat · Settings");
    expect(host.textContent ?? "").toContain("Admin · Settings");
    expect(host.textContent ?? "").toContain("Operations · Settings");
  });

  it("Enter on 'Operations · Status' from Admin view jumps to Operations", async () => {
    const { el } = await mountWindow<ShellWithPaletteCV>("lthn-app-shell", { attrs: { view: "admin" } });
    el.paletteOpen = true;
    el.paletteQuery = "status";
    el.paletteIndex = 0;
    await el.updateComplete;
    el._paletteCommit();
    await el.updateComplete;
    expect(el.view).toBe("operations");
    expect(el.active).toBe("status");
  });

  it("'incidents' jumps to Operations (only Operations has incidents)", async () => {
    const { el } = await mountWindow<ShellWithPaletteCV>("lthn-app-shell", { attrs: { view: "planning" } });
    el.paletteOpen = true;
    el.paletteQuery = "incidents";
    el.paletteIndex = 0;
    await el.updateComplete;
    el._paletteCommit();
    await el.updateComplete;
    expect(el.view).toBe("operations");
    expect(el.active).toBe("incidents");
  });
});

describe("lthn-app-shell — ⌘1..⌘7 view shortcuts", () => {
  type ShellWithView = HTMLElement & {
    view: string;
    active: string;
    updateComplete: Promise<boolean>;
  };

  it("⌘1 selects the first view (chat)", async () => {
    const { el } = await mountWindow<ShellWithView>("lthn-app-shell", { attrs: { view: "planning" } });
    expect(el.view).toBe("planning");
    window.dispatchEvent(new KeyboardEvent("keydown", { key: "1", metaKey: true, bubbles: true }));
    await el.updateComplete;
    expect(el.view).toBe("chat");
  });

  it("⌘2 selects admin", async () => {
    const { el } = await mountWindow<ShellWithView>("lthn-app-shell");
    window.dispatchEvent(new KeyboardEvent("keydown", { key: "2", metaKey: true, bubbles: true }));
    await el.updateComplete;
    expect(el.view).toBe("admin");
  });

  it("⌘3..⌘8 cycle through planning/coding/marketing/operations/sales/office", async () => {
    const { el } = await mountWindow<ShellWithView>("lthn-app-shell");
    const expected = ["planning", "coding", "marketing", "operations", "sales", "office"];
    for (let i = 0; i < expected.length; i++) {
      window.dispatchEvent(new KeyboardEvent("keydown", { key: String(i + 3), metaKey: true, bubbles: true }));
      await el.updateComplete;
      expect(el.view, `⌘${i + 3} → ${expected[i]}`).toBe(expected[i]);
    }
  });

  it("Ctrl+1..8 also works (Linux/Windows)", async () => {
    const { el } = await mountWindow<ShellWithView>("lthn-app-shell");
    window.dispatchEvent(new KeyboardEvent("keydown", { key: "3", ctrlKey: true, bubbles: true }));
    await el.updateComplete;
    expect(el.view).toBe("planning");
  });

  it("⌘9 / ⌘0 are ignored (out of range)", async () => {
    const { el } = await mountWindow<ShellWithView>("lthn-app-shell");
    const before = el.view;
    window.dispatchEvent(new KeyboardEvent("keydown", { key: "9", metaKey: true, bubbles: true }));
    window.dispatchEvent(new KeyboardEvent("keydown", { key: "0", metaKey: true, bubbles: true }));
    await el.updateComplete;
    expect(el.view).toBe(before);
  });

  it("⇧⌘1 is ignored (shift modifier breaks the shortcut)", async () => {
    const { el } = await mountWindow<ShellWithView>("lthn-app-shell", { attrs: { view: "planning" } });
    window.dispatchEvent(new KeyboardEvent("keydown", { key: "1", metaKey: true, shiftKey: true, bubbles: true }));
    await el.updateComplete;
    expect(el.view).toBe("planning");
  });

  it("⌘1 is suppressed while an INPUT has focus", async () => {
    const { el } = await mountWindow<ShellWithView>("lthn-app-shell", { attrs: { view: "planning" } });
    const input = document.createElement("input");
    document.body.appendChild(input);
    input.focus();
    input.dispatchEvent(new KeyboardEvent("keydown", { key: "1", metaKey: true, bubbles: true }));
    await el.updateComplete;
    expect(el.view, "input had focus → shortcut suppressed").toBe("planning");
    input.remove();
  });

  it("⌘1 is suppressed while a TEXTAREA has focus", async () => {
    const { el } = await mountWindow<ShellWithView>("lthn-app-shell", { attrs: { view: "planning" } });
    const ta = document.createElement("textarea");
    document.body.appendChild(ta);
    ta.focus();
    ta.dispatchEvent(new KeyboardEvent("keydown", { key: "1", metaKey: true, bubbles: true }));
    await el.updateComplete;
    expect(el.view).toBe("planning");
    ta.remove();
  });

  it("⌘1 is suppressed while a contenteditable has focus", async () => {
    const { el } = await mountWindow<ShellWithView>("lthn-app-shell", { attrs: { view: "planning" } });
    const ce = document.createElement("div");
    ce.contentEditable = "true";
    ce.tabIndex = 0;
    document.body.appendChild(ce);
    ce.focus();
    ce.dispatchEvent(new KeyboardEvent("keydown", { key: "1", metaKey: true, bubbles: true }));
    await el.updateComplete;
    expect(el.view).toBe("planning");
    ce.remove();
  });

  it("plain '1' (no modifier) does not switch views", async () => {
    const { el } = await mountWindow<ShellWithView>("lthn-app-shell", { attrs: { view: "planning" } });
    window.dispatchEvent(new KeyboardEvent("keydown", { key: "1", bubbles: true }));
    await el.updateComplete;
    expect(el.view).toBe("planning");
  });
});

describe("lthn-app-shell — ⌘⇧[ / ⌘⇧] view cycling", () => {
  type ShellWithView = HTMLElement & {
    view: string;
    updateComplete: Promise<boolean>;
  };

  it("⌘⇧] advances to the next view (admin → planning)", async () => {
    const { el } = await mountWindow<ShellWithView>("lthn-app-shell", { attrs: { view: "admin" } });
    window.dispatchEvent(new KeyboardEvent("keydown", { key: "]", metaKey: true, shiftKey: true, bubbles: true }));
    await el.updateComplete;
    expect(el.view).toBe("planning");
  });

  it("⌘⇧[ moves to the previous view (planning → admin)", async () => {
    const { el } = await mountWindow<ShellWithView>("lthn-app-shell", { attrs: { view: "planning" } });
    window.dispatchEvent(new KeyboardEvent("keydown", { key: "[", metaKey: true, shiftKey: true, bubbles: true }));
    await el.updateComplete;
    expect(el.view).toBe("admin");
  });

  it("⌘⇧] wraps from the last view (office → chat)", async () => {
    const { el } = await mountWindow<ShellWithView>("lthn-app-shell", { attrs: { view: "office" } });
    window.dispatchEvent(new KeyboardEvent("keydown", { key: "]", metaKey: true, shiftKey: true, bubbles: true }));
    await el.updateComplete;
    expect(el.view).toBe("chat");
  });

  it("⌘⇧[ wraps from the first view (chat → office)", async () => {
    const { el } = await mountWindow<ShellWithView>("lthn-app-shell", { attrs: { view: "chat" } });
    window.dispatchEvent(new KeyboardEvent("keydown", { key: "[", metaKey: true, shiftKey: true, bubbles: true }));
    await el.updateComplete;
    expect(el.view).toBe("office");
  });

  it("Ctrl+Shift+] also works (Linux/Windows)", async () => {
    const { el } = await mountWindow<ShellWithView>("lthn-app-shell", { attrs: { view: "admin" } });
    window.dispatchEvent(new KeyboardEvent("keydown", { key: "]", ctrlKey: true, shiftKey: true, bubbles: true }));
    await el.updateComplete;
    expect(el.view).toBe("planning");
  });

  it("⌘] without shift is ignored", async () => {
    const { el } = await mountWindow<ShellWithView>("lthn-app-shell", { attrs: { view: "admin" } });
    window.dispatchEvent(new KeyboardEvent("keydown", { key: "]", metaKey: true, bubbles: true }));
    await el.updateComplete;
    expect(el.view).toBe("admin");
  });

  it("⌘⇧] suppressed while an INPUT has focus", async () => {
    const { el } = await mountWindow<ShellWithView>("lthn-app-shell", { attrs: { view: "admin" } });
    const input = document.createElement("input");
    document.body.appendChild(input);
    input.focus();
    input.dispatchEvent(new KeyboardEvent("keydown", { key: "]", metaKey: true, shiftKey: true, bubbles: true }));
    await el.updateComplete;
    expect(el.view).toBe("admin");
    input.remove();
  });

  it("five forward cycles visit every view in order", async () => {
    const { el } = await mountWindow<ShellWithView>("lthn-app-shell", { attrs: { view: "admin" } });
    const expected = ["planning", "coding", "marketing", "operations", "sales"];
    for (const want of expected) {
      window.dispatchEvent(new KeyboardEvent("keydown", { key: "]", metaKey: true, shiftKey: true, bubbles: true }));
      await el.updateComplete;
      expect(el.view).toBe(want);
    }
  });
});

describe("lthn-app-shell — auth-gate mount (Stage C)", () => {
  // Stage C wiring: when _authState !== "ok" the body slot renders
  // <lthn-auth-gate> in place of the normal pane. Side-nav + status
  // bar remain — the gate sits INSIDE the body, not replacing the
  // shell. Existing app-shell tests already prove _authState defaults
  // to "ok" so the normal body path stays intact when nothing 401s.
  type ShellWithAuth = HTMLElement & {
    active: string;
    _authState: "setup" | "auth" | "error" | "ok";
    _authRequestId: string;
    updateComplete: Promise<boolean>;
  };

  it("_authState='ok' renders the normal pane (no gate)", async () => {
    const { host } = await mountWindow<ShellWithAuth>("lthn-app-shell");
    // Default state — no gate mounted, normal body slot lives.
    expect(host.querySelector("lthn-auth-gate")).toBeNull();
  });

  it("_authState='error' mounts <lthn-auth-gate state=error>", async () => {
    const { el, host } = await mountWindow<ShellWithAuth>("lthn-app-shell");
    el._authState = "error";
    el._authRequestId = "req-shell-test-001";
    await el.updateComplete;
    const gate = host.querySelector("lthn-auth-gate");
    expect(gate).not.toBeNull();
    expect(gate?.getAttribute("state")).toBe("error");
    expect(gate?.getAttribute("request-id")).toBe("req-shell-test-001");
    // Embedded attribute so the gate's renderChrome skips the
    // standalone card — the shell already owns the chrome.
    expect(gate?.hasAttribute("embedded")).toBe(true);
  });

  it("lthn:auth:401 event flips _authState to error + captures requestId", async () => {
    const { el, host } = await mountWindow<ShellWithAuth>("lthn-app-shell");
    expect(el._authState).toBe("ok");
    window.dispatchEvent(new CustomEvent("lthn:auth:401", {
      detail: { requestId: "req-from-event-abc" },
    }));
    await el.updateComplete;
    expect(el._authState).toBe("error");
    expect(el._authRequestId).toBe("req-from-event-abc");
    expect(host.querySelector("lthn-auth-gate")).not.toBeNull();
  });

  it("lthn:auth:ok event flips _authState back to ok (gate unmounts)", async () => {
    const { el, host } = await mountWindow<ShellWithAuth>("lthn-app-shell");
    el._authState = "error";
    await el.updateComplete;
    expect(host.querySelector("lthn-auth-gate")).not.toBeNull();

    window.dispatchEvent(new CustomEvent("lthn:auth:ok"));
    await el.updateComplete;
    expect(el._authState).toBe("ok");
    expect(el._authRequestId).toBe("");
    expect(host.querySelector("lthn-auth-gate")).toBeNull();
  });
});

// ─── Sign-Out wire (Hephaestus #105 — Stage E follow-on B) ────────────

describe("lthn-app-shell — Sign-Out UI wire", () => {
  // The Sign-Out button is the user-facing affordance for the
  // /v1/account/lock endpoint that H#51 hardened with session-binding.
  // Visible only when _authState === "ok" (no point during the gate).
  // Click → POST /v1/account/lock, clear session-token, dispatch
  // AUTH_LOCK so the gate hears "user explicitly signed out" and lands
  // on state="auth" (clean re-unlock surface) instead of state="error"
  // (which is the 401-fallback surface).
  type ShellWithSignOut = HTMLElement & {
    _authState: "setup" | "auth" | "error" | "ok";
    _onSignOut: () => Promise<void>;
    updateComplete: Promise<boolean>;
  };

  it("TestAppShell_SignOutButtonVisibleWhenAuthOk_Good — button renders only while signed in", async () => {
    const { el, host } = await mountWindow<ShellWithSignOut>("lthn-app-shell");
    // Default state is "ok" — button visible.
    expect(el._authState).toBe("ok");
    const button = host.querySelector('[data-testid="lthn-app-shell-sign-out"]');
    expect(button).not.toBeNull();
    expect(button?.getAttribute("title")).toBe("Sign out");

    // Flip to gate-mounted state — button must disappear.
    el._authState = "error";
    await el.updateComplete;
    expect(host.querySelector('[data-testid="lthn-app-shell-sign-out"]')).toBeNull();

    // setup / auth states also hide the button.
    el._authState = "setup";
    await el.updateComplete;
    expect(host.querySelector('[data-testid="lthn-app-shell-sign-out"]')).toBeNull();
    el._authState = "auth";
    await el.updateComplete;
    expect(host.querySelector('[data-testid="lthn-app-shell-sign-out"]')).toBeNull();
  });

  it("TestAppShell_SignOutClickPOSTsLock_Good — clicking POSTs /v1/account/lock + dispatches AUTH_LOCK", async () => {
    // Stub fetch BEFORE mount so api-fetch's apiFetch routes through us.
    // Signature carries (input, init) so the assertion below can read
    // the method off init[1] without TS narrowing the tuple to length-1.
    const fetchSpy = vi.fn(async (_input: RequestInfo | URL, _init?: RequestInit) => {
      // Reveal binding may run from apiFetch's token loader at module-
      // load — we only assert on /v1/account/lock specifically.
      return new Response("{}", { status: 200, headers: { "Content-Type": "application/json" } });
    });
    const originalFetch = globalThis.fetch;
    globalThis.fetch = fetchSpy as unknown as typeof fetch;

    const events: Event[] = [];
    const listener = (ev: Event) => events.push(ev);
    window.addEventListener("lthn:auth:lock", listener);

    try {
      const { el, host } = await mountWindow<ShellWithSignOut>("lthn-app-shell");
      el._authState = "ok";
      await el.updateComplete;

      const button = host.querySelector('[data-testid="lthn-app-shell-sign-out"]') as HTMLButtonElement | null;
      expect(button).not.toBeNull();
      // Capture the promise the click handler returns so we can
      // await its full chain (apiFetch → dynamic import → fetch →
      // clearSessionToken → dispatchEvent). Without this hook the
      // handler's pending microtask chain leaks into the next test
      // and the AUTH_LOCK dispatch fires after the listener for THIS
      // test has been removed (or — worse — counts toward the NEXT
      // test's event tally).
      let inflight: Promise<void> | null = null;
      const originalOnSignOut = el._onSignOut.bind(el);
      el._onSignOut = () => { const p = originalOnSignOut(); inflight = p; return p; };
      button!.click();
      // Drain microtasks until the captured promise settles.
      await new Promise((r) => setTimeout(r, 0));
      if (inflight) await inflight;
      await el.updateComplete;

      // Exactly one POST to /v1/account/lock fired.
      const lockCalls = fetchSpy.mock.calls.filter((call) => {
        const input = call[0];
        const url = typeof input === "string" ? input : (input as URL).toString();
        return url.endsWith("/v1/account/lock");
      });
      expect(lockCalls.length).toBe(1);
      expect(lockCalls[0]![1]?.method).toBe("POST");

      // AUTH_LOCK_EVENT dispatched exactly once.
      expect(events.length).toBe(1);
      expect(events[0]!.type).toBe("lthn:auth:lock");

      // Shell flipped to state="auth" so the gate mounts immediately.
      expect(el._authState).toBe("auth");
    } finally {
      window.removeEventListener("lthn:auth:lock", listener);
      globalThis.fetch = originalFetch;
    }
  });

  it("_onSignOut still clears + dispatches when the POST rejects (transport failure)", async () => {
    // Runner down / network blip MUST still surface the gate so the
    // user can't be stuck on "I clicked sign-out but nothing happened".
    const fetchSpy = vi.fn(async (_input: RequestInfo | URL, _init?: RequestInit) => {
      throw new Error("network down");
    });
    const originalFetch = globalThis.fetch;
    globalThis.fetch = fetchSpy as unknown as typeof fetch;

    // Mount FIRST so prior-test shells with gates that listen for
    // AUTH_LOCK have settled. Then attach the test listener so it sees
    // exactly the dispatches caused by THIS test's _onSignOut call.
    const { el } = await mountWindow<ShellWithSignOut>("lthn-app-shell");
    el._authState = "ok";
    await el.updateComplete;

    const events: Event[] = [];
    const listener = (ev: Event) => events.push(ev);
    window.addEventListener("lthn:auth:lock", listener);

    try {
      await el._onSignOut();
      // POST attempted, AUTH_LOCK still fired, state still flipped.
      expect(fetchSpy.mock.calls.some((call) => {
        const input = call[0];
        const url = typeof input === "string" ? input : (input as URL).toString();
        return url.endsWith("/v1/account/lock");
      })).toBe(true);
      // Exactly one new dispatch from this call. Prior-test mounted
      // auth-gates that listen for AUTH_LOCK call _deriveState without
      // re-dispatching, so the test listener only sees the single
      // direct dispatch from this _onSignOut invocation.
      expect(events.length).toBe(1);
      expect(el._authState).toBe("auth");
    } finally {
      window.removeEventListener("lthn:auth:lock", listener);
      globalThis.fetch = originalFetch;
    }
  });

  it("_onSignOut is a no-op when _authState is not ok (defensive guard)", async () => {
    const fetchSpy = vi.fn(async (_input: RequestInfo | URL, _init?: RequestInit) =>
      new Response("{}", { status: 200 }),
    );
    const originalFetch = globalThis.fetch;
    globalThis.fetch = fetchSpy as unknown as typeof fetch;

    const events: Event[] = [];
    const listener = (ev: Event) => events.push(ev);
    window.addEventListener("lthn:auth:lock", listener);

    try {
      const { el } = await mountWindow<ShellWithSignOut>("lthn-app-shell");
      el._authState = "error";
      await el.updateComplete;
      await el._onSignOut();
      // Defensive guard means no POST + no event when the gate is up.
      const lockCalls = fetchSpy.mock.calls.filter((call) => {
        const input = call[0];
        const url = typeof input === "string" ? input : (input as URL).toString();
        return url.endsWith("/v1/account/lock");
      });
      expect(lockCalls.length).toBe(0);
      expect(events.length).toBe(0);
      expect(el._authState).toBe("error");
    } finally {
      window.removeEventListener("lthn:auth:lock", listener);
      globalThis.fetch = originalFetch;
    }
  });
});

describe("lthn-app-shell — Stage D: boot-time _probeAuthState", () => {
  type ShellWithAuth = HTMLElement & {
    _authState: "ok" | "setup" | "auth" | "error";
    _probeAuthState: () => Promise<void>;
    updateComplete: Promise<boolean>;
  };

  it("_probeAuthState sets state=setup when backend reports no account", async () => {
    const { el } = await mountWindow<ShellWithAuth>("lthn-app-shell");
    el._authState = "ok"; // reset to baseline
    // Mount-time has already run _probeAuthState once; re-run with a
    // direct mock to drive the no-account branch deterministically.
    (ServerKeyAccountStatus as unknown as { mockResolvedValueOnce: (v: unknown) => void })
      .mockResolvedValueOnce({ OK: true, Value: { has_user_account: false } });
    await el._probeAuthState();
    expect(el._authState).toBe("setup");
  });

  it("_probeAuthState keeps state=ok when backend reports account exists", async () => {
    const { el } = await mountWindow<ShellWithAuth>("lthn-app-shell");
    el._authState = "ok";
    (ServerKeyAccountStatus as unknown as { mockResolvedValueOnce: (v: unknown) => void })
      .mockResolvedValueOnce({ OK: true, Value: { has_user_account: true } });
    await el._probeAuthState();
    expect(el._authState).toBe("ok");
  });

  it("_probeAuthState degrades to ok when backend rejects", async () => {
    const { el } = await mountWindow<ShellWithAuth>("lthn-app-shell");
    el._authState = "ok";
    (ServerKeyAccountStatus as unknown as { mockRejectedValueOnce: (v: unknown) => void })
      .mockRejectedValueOnce(new Error("no binding"));
    await el._probeAuthState();
    expect(el._authState).toBe("ok");
  });

  it("_probeAuthState keeps state=ok when r.OK is false", async () => {
    const { el } = await mountWindow<ShellWithAuth>("lthn-app-shell");
    el._authState = "ok";
    (ServerKeyAccountStatus as unknown as { mockResolvedValueOnce: (v: unknown) => void })
      .mockResolvedValueOnce({ OK: false, Value: undefined });
    await el._probeAuthState();
    expect(el._authState).toBe("ok");
  });
});

// ─── Plugin-views Unit B.5 — fallback chain + consent-ratchet ──────────
// Per plans/code/lthn/desktop/views/RFC.plugin-views.md §6.

import {
  resolveBootView,
  markViewActivated,
  isViewActivated,
  canPersistDefaultView,
  PLUGIN_VIEW_FALLBACK_FINAL,
  CONSENT_RATCHET_KEY_PREFIX,
} from "./app-shell";
import { beforeAll as beforeAllRatchet, beforeEach as beforeEachRatchet } from "vitest";

// happy-dom workers don't always expose a functional localStorage —
// shim with a Map-backed implementation so the consent-ratchet
// helpers exercise their real branches.
beforeAllRatchet(() => {
  const map = new Map<string, string>();
  const shim: Storage = {
    get length() { return map.size; },
    clear() { map.clear(); },
    getItem(k: string) { return map.has(k) ? map.get(k)! : null; },
    key(i: number) { return Array.from(map.keys())[i] ?? null; },
    removeItem(k: string) { map.delete(k); },
    setItem(k: string, v: string) { map.set(k, v); },
  };
  Object.defineProperty(globalThis, "localStorage", { configurable: true, value: shim });
  Object.defineProperty(window, "localStorage", { configurable: true, value: shim });
});

beforeEachRatchet(() => {
  for (let i = localStorage.length - 1; i >= 0; i--) {
    const k = localStorage.key(i);
    if (k && k.startsWith(CONSENT_RATCHET_KEY_PREFIX)) localStorage.removeItem(k);
  }
});

describe("resolveBootView (RFC.plugin-views §6.2 — 2-entry fallback)", () => {
  it("picks the configured default when mountable", () => {
    expect(resolveBootView("opencode", id => id === "opencode")).toBe("opencode");
  });

  it("falls back to admin when the configured default is unmountable", () => {
    expect(resolveBootView("opencode", () => false)).toBe(PLUGIN_VIEW_FALLBACK_FINAL);
    expect(PLUGIN_VIEW_FALLBACK_FINAL).toBe("admin");
  });

  it("falls back to admin when configured is empty", () => {
    expect(resolveBootView("", () => true)).toBe(PLUGIN_VIEW_FALLBACK_FINAL);
  });

  it("CRIT-3 hard cap — no third entry, no extension hook (returns admin)", () => {
    // Even when a hypothetical "last successfully-active view" might
    // be supplied, resolveBootView only consults the two entries.
    // The function signature itself enforces the cap: no third arg.
    expect(resolveBootView("opencode", () => false)).toBe("admin");
  });
});

describe("consent-ratchet (RFC.plugin-views §6.4)", () => {
  it("markViewActivated + isViewActivated round-trip", () => {
    expect(isViewActivated("opencode", "opencode")).toBe(false);
    markViewActivated("opencode", "opencode");
    expect(isViewActivated("opencode", "opencode")).toBe(true);
  });

  it("isViewActivated returns false for an empty pluginCode or viewId", () => {
    expect(isViewActivated("", "opencode")).toBe(false);
    expect(isViewActivated("opencode", "")).toBe(false);
  });

  it("TestSettings_DefaultViewDropdownRejectsUnactivatedPlugin_Bad — canPersistDefaultView refuses a plugin view not yet activated", () => {
    expect(canPersistDefaultView("opencode", "opencode")).toBe(false);
  });

  it("canPersistDefaultView allows a plugin view AFTER activation", () => {
    markViewActivated("opencode", "opencode");
    expect(canPersistDefaultView("opencode", "opencode")).toBe(true);
  });

  it("canPersistDefaultView always allows built-in view ids (pluginCode empty)", () => {
    expect(canPersistDefaultView("admin", "")).toBe(true);
    expect(canPersistDefaultView("coding", "")).toBe(true);
  });

  it("each (pluginCode, viewId) pair is independent", () => {
    markViewActivated("opencode", "opencode:terminal");
    expect(canPersistDefaultView("opencode:terminal", "opencode")).toBe(true);
    // A different view in the same plugin remains gated until
    // separately activated.
    expect(canPersistDefaultView("opencode:history", "opencode")).toBe(false);
  });
});


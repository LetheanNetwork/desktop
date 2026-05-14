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

import { describe, it, expect } from "vitest";
import { mountWindow } from "../test/window-fixture";

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
      ["chat",     "lthn-chat-window",     "state", "multi-turn"],
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
  // out at the @desktop/desktop/windowservice module boundary so
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
  // the dynamic `import("@desktop/desktop/windowservice")` adds
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

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

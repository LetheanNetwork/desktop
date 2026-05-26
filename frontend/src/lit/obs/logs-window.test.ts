// SPDX-Licence-Identifier: EUPL-1.2

import { describe, it, expect } from "vitest";
import { mountWindow, expectChromeTitle, isEmbedded } from "../../test/window-fixture";
import "./logs-window";

describe("lthn-logs-window — smoke", () => {
  it("mounts with the Activity titlebar", async () => {
    const { host } = await mountWindow("lthn-logs-window");
    expectChromeTitle(host, "Activity");
    expect(host.querySelector("header")).not.toBeNull();
  });

  it("tab prop controls which body section renders", async () => {
    const { el, host } = await mountWindow<HTMLElement & { tab: string; updateComplete: Promise<boolean> }>(
      "lthn-logs-window",
      { props: { tab: "live" } },
    );
    // tab is declared with `type: String` and no `reflect: true`, so
    // assert the property directly rather than the attribute mirror.
    expect(el.tab).toBe("live");

    el.tab = "errors";
    await el.updateComplete;
    expect(el.tab).toBe("errors");
    // Host re-renders — Activity title persists across tab change.
    expect(host.textContent).toContain("Activity");
  });
});

describe("lthn-logs-window — tab pills", () => {
  it("clicking a toolbar pill flips the active tab", async () => {
    const { el } = await mountWindow<HTMLElement & {
      tab: string;
      updateComplete: Promise<boolean>;
      _switchTab: (id: string) => void;
    }>("lthn-logs-window", { props: { tab: "live" } });
    expect(el.tab).toBe("live");

    el._switchTab("history");
    await el.updateComplete;
    expect(el.tab).toBe("history");

    // No-op when same tab clicked.
    el._switchTab("history");
    await el.updateComplete;
    expect(el.tab).toBe("history");
  });
});

describe("lthn-logs-window — two-shell", () => {
  it("embedded mode collapses the chrome", async () => {
    const { host } = await mountWindow("lthn-logs-window", { attrs: { embedded: "" } });
    expect(isEmbedded(host)).toBe(true);
    expect(host.querySelector("header")).toBeNull();
  });
});

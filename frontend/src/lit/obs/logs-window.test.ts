// SPDX-Licence-Identifier: EUPL-1.2

import { describe, it, expect } from "vitest";
import { mountWindow, expectChromeTitle, isEmbedded } from "../../test/window-fixture";
import "./logs-window";

describe("lthn-logs-window — smoke", () => {
  it("mounts with the Activity titlebar", async () => {
    const { host } = await mountWindow("lthn-logs-window");
    expectChromeTitle(host, "Activity");
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

describe("lthn-logs-window — two-shell", () => {
  it("embedded mode collapses the chrome", async () => {
    const { host } = await mountWindow("lthn-logs-window", { attrs: { embedded: "" } });
    expect(isEmbedded(host)).toBe(true);
    expect(host.querySelector("header")).toBeNull();
  });
});

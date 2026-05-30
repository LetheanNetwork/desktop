// SPDX-Licence-Identifier: EUPL-1.2
//
// Render-smoke test for lthn-plugin-window. The plugin window takes a
// `code` prop and loads plugin status from the @desktop/plugin service
// in its async connectedCallback — that call is caught internally
// (unwrap → fallback), so the first render uses the constructor's
// default chrome. See build-window.test.ts for the broader pattern.

import { describe, it, expect } from "vitest";
import { mountWindow, isEmbedded } from "../../test/window-fixture";

import "./plugin-window";

describe("lthn-plugin-window — render smoke", () => {
  it("mounts and renders the chrome titlebar without throwing", async () => {
    const { host } = await mountWindow("lthn-plugin-window", { props: { code: "demo-plugin" } });
    expect(host.querySelector("header")).not.toBeNull();
    expect(host.querySelector(".lthn-window")).not.toBeNull();
  });

  it("collapses to the embedded shell when the embedded attribute is set", async () => {
    const { host } = await mountWindow("lthn-plugin-window", {
      props: { code: "demo-plugin" },
      attrs: { embedded: "" },
    });
    expect(isEmbedded(host)).toBe(true);
    expect(host.querySelector("header")).toBeNull();
  });
});

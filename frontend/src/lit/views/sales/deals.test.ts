// SPDX-Licence-Identifier: EUPL-1.2

import { describe, it, expect } from "vitest";
import type { LitElement } from "lit";
import { mountWindow, expectChromeTitle, isEmbedded } from "../../../test/window-fixture";
import "./deals";

type Kind     = "call" | "email" | "meet";
type DocState = "draft" | "signed" | "shared";

interface Activity { ts: string; k: Kind; who: string; t: string; }
interface DocLink  { t: string; s: DocState; }
interface Deal {
  customer:     string;
  headline:     string;
  stage:        string;
  probability:  string;
  closeTarget:  string;
  log:          Activity[];
  stakeholders: string[];
  docs:         DocLink[];
}
interface DealsEl extends LitElement {
  deal:    Deal;
  logKind: "" | Kind;
  _filteredLog(): Activity[];
  _toggleKind(k: Kind): void;
}

describe("lthn-view-deals — smoke", () => {
  it("mounts with the Deal titlebar", async () => {
    const { host } = await mountWindow("lthn-view-deals");
    expectChromeTitle(host, "Deal · Heritage Law LLP");
    expect(host.querySelector("header")).not.toBeNull();
  });

  it("renders the deal headline + value", async () => {
    const { host } = await mountWindow("lthn-view-deals");
    expect(host.textContent).toContain("Heritage Law LLP");
    expect(host.textContent).toContain("£24 K · 12-month hosted");
  });

  it("renders the Engaging stage pill", async () => {
    const { host } = await mountWindow("lthn-view-deals");
    expect(host.textContent).toContain("Engaging");
  });

  it("renders one log entry per fixture activity", async () => {
    const { el, host } = await mountWindow<DealsEl>("lthn-view-deals");
    const entries = host.querySelectorAll(".lthn-view-deals-log-entry");
    expect(entries.length).toBe(el.deal.log.length);
  });

  it("renders one stakeholder card per fixture stakeholder", async () => {
    const { el, host } = await mountWindow<DealsEl>("lthn-view-deals");
    const cards = host.querySelectorAll(".lthn-view-deals-stakeholder");
    expect(cards.length).toBe(el.deal.stakeholders.length);
  });

  it("renders one doc card per fixture doc", async () => {
    const { el, host } = await mountWindow<DealsEl>("lthn-view-deals");
    const docs = host.querySelectorAll(".lthn-view-deals-doc");
    expect(docs.length).toBe(el.deal.docs.length);
  });

  it("each doc carries a data-state attribute", async () => {
    const { host } = await mountWindow("lthn-view-deals");
    const states = Array.from(host.querySelectorAll(".lthn-view-deals-doc"))
      .map(d => d.getAttribute("data-state"));
    expect(states).toContain("draft");
    expect(states).toContain("signed");
    expect(states).toContain("shared");
  });
});

describe("lthn-view-deals — activity filter", () => {
  it("_filteredLog returns the full log when no kind is selected", async () => {
    const { el } = await mountWindow<DealsEl>("lthn-view-deals");
    expect(el._filteredLog().length).toBe(el.deal.log.length);
  });

  it("_filteredLog restricts to the active kind", async () => {
    const { el } = await mountWindow<DealsEl>("lthn-view-deals");
    el.logKind = "email";
    const out = el._filteredLog();
    expect(out.length).toBeGreaterThan(0);
    expect(out.every(a => a.k === "email")).toBe(true);
  });

  it("_toggleKind clears the filter on second call", async () => {
    const { el } = await mountWindow<DealsEl>("lthn-view-deals");
    el._toggleKind("call");
    expect(el.logKind).toBe("call");
    el._toggleKind("call");
    expect(el.logKind).toBe("");
  });

  it("active kind reduces the rendered log entry count", async () => {
    const { el, host } = await mountWindow<DealsEl>("lthn-view-deals");
    el.logKind = "meet";
    await el.updateComplete;
    const entries = host.querySelectorAll(".lthn-view-deals-log-entry");
    // Only one "meet" entry in the fixture log.
    expect(entries.length).toBe(1);
  });
});

describe("lthn-view-deals — two-shell", () => {
  it("embedded mode collapses the chrome", async () => {
    const { host } = await mountWindow("lthn-view-deals", { attrs: { embedded: "" } });
    expect(isEmbedded(host)).toBe(true);
    expect(host.querySelector("header")).toBeNull();
  });

  it("embedded mode still renders the activity log", async () => {
    const { host } = await mountWindow("lthn-view-deals", { attrs: { embedded: "" } });
    expect(host.querySelectorAll(".lthn-view-deals-log-entry").length).toBeGreaterThan(0);
  });
});

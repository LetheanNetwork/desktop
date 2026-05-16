// SPDX-Licence-Identifier: EUPL-1.2

import { describe, it, expect, vi } from "vitest";
import type { LitElement } from "lit";
import { mountWindow, expectChromeTitle, isEmbedded } from "../../../test/window-fixture";

vi.mock("@desktop/sales/deals/service", () => ({
  List: vi.fn(),
}));

import { List as DealsList } from "@desktop/sales/deals/service";

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

describe("lthn-view-deals — backend wire", () => {
  it("swaps fixture deal for the first backend deal", async () => {
    (DealsList as unknown as { mockReset: () => void }).mockReset();
    (DealsList as unknown as { mockResolvedValue: (v: unknown) => void }).mockResolvedValue({
      Value: { deals: [{
        customer:    "Acme Hosting Ltd",
        headline:    "£12 K · 6-month managed",
        stage:       "Negotiation",
        probability: "75%",
        closeTarget: "30 Jun",
        log:         [{ ts: "today", k: "call", who: "you", t: "Pricing call." }],
        stakeholders: ["Jane CTO"],
        docs:         [{ t: "SOW v1", s: "draft" }],
      }] },
    });
    const { el } = await mountWindow<HTMLElement & {
      deal: { customer: string; stage: string };
      updateComplete: Promise<boolean>;
    }>("lthn-view-deals");
    await new Promise(r => setTimeout(r, 0));
    await el.updateComplete;
    expect(el.deal.customer).toBe("Acme Hosting Ltd");
    expect(el.deal.stage).toBe("Negotiation");
  });

  it("keeps fixture deal when backend rejects", async () => {
    (DealsList as unknown as { mockReset: () => void }).mockReset();
    (DealsList as unknown as { mockRejectedValue: (v: unknown) => void }).mockRejectedValue(new Error("no binding"));
    const { el } = await mountWindow<HTMLElement & {
      deal: { customer: string };
      updateComplete: Promise<boolean>;
    }>("lthn-view-deals");
    await new Promise(r => setTimeout(r, 0));
    await el.updateComplete;
    expect(el.deal.customer).toBe("Heritage Law LLP"); // fixture preserved
  });
});

describe("lthn-view-deals — conflict-reload listener (Cascade W1)", () => {
  it("CONFLICT_RELOAD_EVENT with matching service triggers _loadFromBackend", async () => {
    (DealsList as unknown as { mockReset: () => void }).mockReset();
    const calls: number[] = [];
    (DealsList as unknown as { mockImplementation: (fn: () => Promise<unknown>) => void }).mockImplementation(() => {
      calls.push(1);
      return Promise.resolve({ Value: { deals: [] } });
    });
    const { el } = await mountWindow<HTMLElement & { updateComplete: Promise<boolean> }>("lthn-view-deals");
    await new Promise(r => setTimeout(r, 0));
    await el.updateComplete;
    const baseline = calls.length;

    window.dispatchEvent(new CustomEvent("lthn:conflict:reload-requested", {
      detail: { service: "sales.deals.update" },
    }));
    await new Promise(r => setTimeout(r, 0));
    await el.updateComplete;

    expect(calls.length).toBeGreaterThan(baseline);
  });

  it("CONFLICT_RELOAD_EVENT with non-matching service is ignored", async () => {
    (DealsList as unknown as { mockReset: () => void }).mockReset();
    const calls: number[] = [];
    (DealsList as unknown as { mockImplementation: (fn: () => Promise<unknown>) => void }).mockImplementation(() => {
      calls.push(1);
      return Promise.resolve({ Value: { deals: [] } });
    });
    const { el } = await mountWindow<HTMLElement & { updateComplete: Promise<boolean> }>("lthn-view-deals");
    await new Promise(r => setTimeout(r, 0));
    await el.updateComplete;
    const baseline = calls.length;

    window.dispatchEvent(new CustomEvent("lthn:conflict:reload-requested", {
      detail: { service: "sales.contacts.update" }, // not ours
    }));
    await new Promise(r => setTimeout(r, 0));
    await el.updateComplete;

    expect(calls.length).toBe(baseline);
  });
});

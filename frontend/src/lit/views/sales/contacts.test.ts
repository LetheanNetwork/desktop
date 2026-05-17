// SPDX-Licence-Identifier: EUPL-1.2

import { describe, it, expect, vi } from "vitest";
import type { LitElement } from "lit";
import { mountWindow, expectChromeTitle, isEmbedded } from "../../../test/window-fixture";

vi.mock("@desktop/sales/contacts/service", () => ({
  List: vi.fn(),
}));

import { List as ContactsList } from "@desktop/sales/contacts/service";

import "./contacts";

type Warmth = "hot" | "warm" | "cool";
interface Contact {
  name: string; role: string; last: string; warmth: Warmth; next: string;
}
interface ContactsEl extends LitElement {
  contacts: Contact[];
  query:    string;
  warmth:   "" | Warmth;
  _filtered(): Contact[];
  _toggleWarmth(w: Warmth): void;
}

describe("lthn-view-contacts — smoke", () => {
  it("mounts with the Contacts titlebar", async () => {
    const { host } = await mountWindow("lthn-view-contacts");
    expectChromeTitle(host, "Contacts");
    expect(host.querySelector("header")).not.toBeNull();
  });

  it("renders a row for each fixture contact", async () => {
    const { el, host } = await mountWindow<ContactsEl>("lthn-view-contacts");
    const rows = host.querySelectorAll(".lthn-view-contacts-row");
    expect(rows.length).toBe(el.contacts.length);
  });

  it("each row carries data-name + data-warmth", async () => {
    const { host } = await mountWindow("lthn-view-contacts");
    const rows = Array.from(host.querySelectorAll(".lthn-view-contacts-row"));
    expect(rows.every(r => !!r.getAttribute("data-name"))).toBe(true);
    expect(rows.every(r => !!r.getAttribute("data-warmth"))).toBe(true);
  });

  it("renders the canonical Ada Penley contact", async () => {
    const { host } = await mountWindow("lthn-view-contacts");
    expect(host.textContent).toContain("Ada Penley");
    expect(host.textContent).toContain("CTO · Heritage Law");
  });

  it("subtitle reflects active + hot counts", async () => {
    const { host } = await mountWindow("lthn-view-contacts");
    const header = host.querySelector("header");
    expect(header?.textContent ?? "").toContain("6 active");
    expect(header?.textContent ?? "").toContain("3 hot");
  });
});

describe("lthn-view-contacts — filtering", () => {
  it("_filtered restricts to the warmth bucket", async () => {
    const { el } = await mountWindow<ContactsEl>("lthn-view-contacts");
    el.warmth = "hot";
    const out = el._filtered();
    expect(out.length).toBeGreaterThan(0);
    expect(out.every(c => c.warmth === "hot")).toBe(true);
  });

  it("_filtered runs a case-insensitive query against name + role", async () => {
    const { el } = await mountWindow<ContactsEl>("lthn-view-contacts");
    el.query = "WHITETHORN";
    const out = el._filtered();
    expect(out.length).toBe(1);
    expect(out[0].name).toContain("Whitethorn");
  });

  it("_toggleWarmth toggles the active bucket back to all", async () => {
    const { el } = await mountWindow<ContactsEl>("lthn-view-contacts");
    el._toggleWarmth("hot");
    expect(el.warmth).toBe("hot");
    el._toggleWarmth("hot");
    expect(el.warmth).toBe("");
  });

  it("warmth filter mutates rendered row count", async () => {
    const { el, host } = await mountWindow<ContactsEl>("lthn-view-contacts");
    el.warmth = "cool";
    await el.updateComplete;
    const rows = host.querySelectorAll(".lthn-view-contacts-row");
    // Cool bucket is just Pemberton in the fixture.
    expect(rows.length).toBe(1);
  });

  it("query that matches nothing renders the empty-state copy", async () => {
    const { el, host } = await mountWindow<ContactsEl>("lthn-view-contacts");
    el.query = "no-such-person-anywhere";
    await el.updateComplete;
    expect(host.textContent ?? "").toContain("No contacts match");
  });
});

describe("lthn-view-contacts — two-shell", () => {
  it("embedded mode collapses the chrome", async () => {
    const { host } = await mountWindow("lthn-view-contacts", { attrs: { embedded: "" } });
    expect(isEmbedded(host)).toBe(true);
    expect(host.querySelector("header")).toBeNull();
  });

  it("embedded mode still renders the contact list", async () => {
    const { host } = await mountWindow("lthn-view-contacts", { attrs: { embedded: "" } });
    expect(host.querySelectorAll(".lthn-view-contacts-row").length).toBeGreaterThan(0);
  });
});

describe("lthn-view-contacts — backend wire", () => {
  it("replaces fixture with rows from pkg/sales/contacts.Service.List", async () => {
    (ContactsList as unknown as { mockReset: () => void }).mockReset();
    (ContactsList as unknown as { mockResolvedValue: (v: unknown) => void }).mockResolvedValue({
      Value: { contacts: [
        { id: "snider",   name: "Snider",         role: "Founder · Lethean",   last: "now",       warmth: "hot",  next: "ship" },
        { id: "darbs",    name: "Darbs",          role: "Sysadmin · Lethean",  last: "yesterday", warmth: "warm", next: "review" },
      ] },
    });
    const { el } = await mountWindow<ContactsEl>("lthn-view-contacts");
    await new Promise(r => setTimeout(r, 0));
    await el.updateComplete;
    expect(el.contacts.length).toBe(2);
    expect(el.contacts[0].name).toBe("Snider");
    expect(el.contacts.find(c => c.name === "Ada Penley")).toBeUndefined(); // fixture displaced
  });

  it("keeps fixture when backend rejects", async () => {
    (ContactsList as unknown as { mockReset: () => void }).mockReset();
    (ContactsList as unknown as { mockRejectedValue: (v: unknown) => void }).mockRejectedValue(new Error("no binding"));
    const { el } = await mountWindow<ContactsEl>("lthn-view-contacts");
    await new Promise(r => setTimeout(r, 0));
    await el.updateComplete;
    expect(el.contacts.length).toBe(6); // FIXTURE_CONTACTS retained
  });

  it("keeps fixture when backend returns empty", async () => {
    (ContactsList as unknown as { mockReset: () => void }).mockReset();
    (ContactsList as unknown as { mockResolvedValue: (v: unknown) => void }).mockResolvedValue({
      Value: { contacts: [] },
    });
    const { el } = await mountWindow<ContactsEl>("lthn-view-contacts");
    await new Promise(r => setTimeout(r, 0));
    await el.updateComplete;
    expect(el.contacts.length).toBe(6); // empty → fixture preserved
  });
});

describe("lthn-view-contacts — conflict-reload listener (Cascade W1)", () => {
  it("CONFLICT_RELOAD_EVENT with matching service triggers _loadFromBackend", async () => {
    (ContactsList as unknown as { mockReset: () => void }).mockReset();
    const reloaded: Array<{ contacts: Contact[] }> = [];
    (ContactsList as unknown as { mockImplementation: (fn: () => Promise<unknown>) => void }).mockImplementation(() => {
      const payload = { Value: { contacts: [
        { name: "Reloaded", role: "X", last: "now", warmth: "hot", next: "y" },
      ] } };
      reloaded.push(payload.Value);
      return Promise.resolve(payload);
    });
    const { el } = await mountWindow<ContactsEl>("lthn-view-contacts");
    await new Promise(r => setTimeout(r, 0));
    await el.updateComplete;
    const initial = reloaded.length;

    window.dispatchEvent(new CustomEvent("lthn:conflict:reload-requested", {
      detail: { service: "sales.contacts.update" },
    }));
    // _loadFromBackend resolves async; tick the microtask queue.
    await new Promise(r => setTimeout(r, 0));
    await el.updateComplete;

    expect(reloaded.length).toBeGreaterThan(initial);
  });

  // Mantis #1491 regression — oversized backend response is capped by
  // `clampRows` to DEFAULT_MAX_ROWS so the renderer never sees a
  // 10K-row array. Smoke-test the bound at the view boundary.
  it("Test_Contacts_loadFromBackend_OversizedRowsCapped_Bad", async () => {
    (ContactsList as unknown as { mockReset: () => void }).mockReset();
    const oversized = Array.from({ length: 10_000 }, (_, i) => ({
      name: `Contact ${i}`, role: "x", last: "now", warmth: "warm" as const, next: "y",
    }));
    (ContactsList as unknown as { mockResolvedValue: (v: unknown) => void }).mockResolvedValue({
      Value: { contacts: oversized },
    });
    const { el } = await mountWindow<ContactsEl>("lthn-view-contacts");
    await new Promise(r => setTimeout(r, 0));
    await el.updateComplete;
    // DEFAULT_MAX_ROWS = 2000 in _bounds.ts; the view MUST cap at that
    // boundary so a misbehaving backend can't blow up Lit's diff loop.
    expect(el.contacts.length).toBeLessThanOrEqual(2000);
  });

  it("CONFLICT_RELOAD_EVENT with non-matching service is ignored", async () => {
    (ContactsList as unknown as { mockReset: () => void }).mockReset();
    const calls: number[] = [];
    (ContactsList as unknown as { mockImplementation: (fn: () => Promise<unknown>) => void }).mockImplementation(() => {
      calls.push(1);
      return Promise.resolve({ Value: { contacts: [] } });
    });
    const { el } = await mountWindow<ContactsEl>("lthn-view-contacts");
    await new Promise(r => setTimeout(r, 0));
    await el.updateComplete;
    const baseline = calls.length;

    window.dispatchEvent(new CustomEvent("lthn:conflict:reload-requested", {
      detail: { service: "sales.deals.update" }, // not ours
    }));
    await new Promise(r => setTimeout(r, 0));
    await el.updateComplete;

    expect(calls.length).toBe(baseline);
  });
});

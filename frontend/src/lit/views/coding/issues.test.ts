// SPDX-Licence-Identifier: EUPL-1.2

import { describe, it, expect, vi } from "vitest";
import type { LitElement } from "lit";
import { mountWindow, expectChromeTitle, isEmbedded } from "../../../test/window-fixture";

vi.mock("@desktop/tasks/service", () => ({
  List: vi.fn(),
}));

import { List as TasksList } from "@desktop/tasks/service";

import "./issues";

interface IssuesTestElement extends LitElement {
  issues: Array<{
    n: number; title: string; repo: string;
    labels: string[]; age: string; assignee: string | null;
  }>;
  sideFilter: "all" | "mine" | "mentions" | "closed";
  labelFilter: string | null;
  _visible(): Array<{ n: number }>;
}

describe("lthn-view-issues — smoke", () => {
  it("mounts with Issues titlebar", async () => {
    const { host } = await mountWindow("lthn-view-issues");
    expectChromeTitle(host, "Issues");
    expect(host.querySelector("header")).not.toBeNull();
  });

  it("embedded mode collapses the chrome", async () => {
    const { host } = await mountWindow("lthn-view-issues", { attrs: { embedded: "" } });
    expect(isEmbedded(host)).toBe(true);
    expect(host.querySelector("header")).toBeNull();
  });
});

describe("lthn-view-issues — sidebar filter", () => {
  const fixture = [
    { n: 1, title: "a", repo: "r", labels: ["bug"],         age: "1h", assignee: "you"  },
    { n: 2, title: "b", repo: "r", labels: ["enhancement"], age: "2h", assignee: "Tobi" },
    { n: 3, title: "c", repo: "r", labels: ["docs"],         age: "3h", assignee: null   },
  ];

  it("'all' returns every issue", async () => {
    const { el } = await mountWindow<IssuesTestElement>("lthn-view-issues");
    el.issues = fixture;
    el.sideFilter = "all";
    await el.updateComplete;
    expect(el._visible()).toHaveLength(3);
  });

  it("'mine' returns only issues assigned to 'you'", async () => {
    const { el } = await mountWindow<IssuesTestElement>("lthn-view-issues");
    el.issues = fixture;
    el.sideFilter = "mine";
    await el.updateComplete;
    expect(el._visible()).toHaveLength(1);
    expect(el._visible()[0].n).toBe(1);
  });

  it("'mentions' returns issues with any assignee", async () => {
    const { el } = await mountWindow<IssuesTestElement>("lthn-view-issues");
    el.issues = fixture;
    el.sideFilter = "mentions";
    await el.updateComplete;
    expect(el._visible()).toHaveLength(2);
  });

  it("'closed' returns empty (no closed fixtures)", async () => {
    const { el } = await mountWindow<IssuesTestElement>("lthn-view-issues");
    el.issues = fixture;
    el.sideFilter = "closed";
    await el.updateComplete;
    expect(el._visible()).toHaveLength(0);
  });
});

describe("lthn-view-issues — label filter", () => {
  const fixture = [
    { n: 10, title: "x", repo: "r", labels: ["bug", "ui"], age: "1h", assignee: null },
    { n: 11, title: "y", repo: "r", labels: ["docs"],       age: "2h", assignee: null },
    { n: 12, title: "z", repo: "r", labels: ["bug"],        age: "3h", assignee: null },
  ];

  it("label filter narrows to matching rows only", async () => {
    const { el } = await mountWindow<IssuesTestElement>("lthn-view-issues");
    el.issues = fixture;
    el.sideFilter = "all";
    el.labelFilter = "bug";
    await el.updateComplete;
    const v = el._visible();
    expect(v).toHaveLength(2);
    expect(v.map(i => i.n)).toContain(10);
    expect(v.map(i => i.n)).toContain(12);
  });

  it("null labelFilter returns all (side-filter-constrained) rows", async () => {
    const { el } = await mountWindow<IssuesTestElement>("lthn-view-issues");
    el.issues = fixture;
    el.sideFilter = "all";
    el.labelFilter = null;
    await el.updateComplete;
    expect(el._visible()).toHaveLength(3);
  });
});

describe("lthn-view-issues — empty state", () => {
  it("shows 'No issues match' when visible is empty", async () => {
    const { el, host } = await mountWindow<IssuesTestElement>("lthn-view-issues");
    el.issues = [{ n: 1, title: "a", repo: "r", labels: ["bug"], age: "1h", assignee: "Tobi" }];
    el.sideFilter = "mine"; // assignee !== "you" → empty
    await el.updateComplete;
    expect(host.textContent ?? "").toContain("No issues match");
  });
});

describe("lthn-view-issues — backend wire", () => {
  it("replaces fixtures with backend rows when tasks.List returns issues", async () => {
    (TasksList as unknown as { mockReset: () => void }).mockReset();
    (TasksList as unknown as { mockResolvedValue: (v: unknown) => void }).mockResolvedValue({
      Value: { issues: [
        { ID: "a1b2c3", Project: "lthn", Summary: "Stage B: ship serverkey package", State: "open", Severity: "major", Priority: "urgent" },
        { ID: "d4e5f6", Project: "core", Summary: "AX sweep CoreGO export gaps", State: "open", Severity: "minor", Priority: "normal" },
      ] },
    });
    const { el } = await mountWindow<IssuesTestElement>("lthn-view-issues");
    await new Promise((r) => setTimeout(r, 0));
    await el.updateComplete;
    expect(el.issues.length).toBe(2);
    expect(el.issues[0].title).toBe("Stage B: ship serverkey package");
    expect(el.issues[0].repo).toBe("lthn");
    expect(el.issues[0].labels).toContain("bug");    // Severity major → bug
    expect(el.issues[0].labels).toContain("urgent"); // Priority urgent → urgent
  });

  it("keeps fixtures when backend rejects", async () => {
    (TasksList as unknown as { mockReset: () => void }).mockReset();
    (TasksList as unknown as { mockRejectedValue: (v: unknown) => void }).mockRejectedValue(new Error("no binding"));
    const { el } = await mountWindow<IssuesTestElement>("lthn-view-issues");
    await new Promise((r) => setTimeout(r, 0));
    await el.updateComplete;
    expect(el.issues.length).toBe(6); // original fixture count
  });

  it("keeps fixtures when backend returns empty", async () => {
    (TasksList as unknown as { mockReset: () => void }).mockReset();
    (TasksList as unknown as { mockResolvedValue: (v: unknown) => void }).mockResolvedValue({
      Value: { issues: [] },
    });
    const { el } = await mountWindow<IssuesTestElement>("lthn-view-issues");
    await new Promise((r) => setTimeout(r, 0));
    await el.updateComplete;
    expect(el.issues.length).toBe(6); // fixture preserved
  });
});

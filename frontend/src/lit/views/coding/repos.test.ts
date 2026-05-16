// SPDX-Licence-Identifier: EUPL-1.2

import { describe, it, expect, beforeEach, vi } from "vitest";
import type { LitElement } from "lit";
import { mountWindow, expectChromeTitle, isEmbedded } from "../../../test/window-fixture";

// Mock the @desktop/repos/service module so we control the dynamic
// import that <lthn-view-repos> does in connectedCallback. Mirror of
// the operations/status.test.ts pattern. Defaults to rejecting so
// every test that doesn't opt-in to a payload keeps the fixture path.
vi.mock("@desktop/repos/service", () => ({
  Status: vi.fn().mockRejectedValue(new Error("default: no binding")),
}));

import { Status } from "@desktop/repos/service";
import "./repos";

interface ReposTestElement extends LitElement {
  repos: Array<{
    name: string; lang: string; branch: string;
    commit: string; build: "passing" | "running" | "failing"; prs: number;
  }>;
  _summary(): { passing: number; failing: number };
  _loadFromBackend(): Promise<void>;
}

// Helper — flush microtasks so the dynamic import + Status() promise
// chain inside connectedCallback can settle before assertions run.
const flushAsync = () => new Promise((r) => setTimeout(r, 0));

beforeEach(() => {
  (Status as unknown as { mockReset: () => void }).mockReset();
  (Status as unknown as { mockRejectedValue: (v: unknown) => void }).mockRejectedValue(new Error("default: no binding"));
});

describe("lthn-view-repos — smoke", () => {
  it("mounts with Repos titlebar", async () => {
    const { host } = await mountWindow("lthn-view-repos");
    expectChromeTitle(host, "Repos");
    expect(host.querySelector("header")).not.toBeNull();
  });

  it("embedded mode collapses the chrome", async () => {
    const { host } = await mountWindow("lthn-view-repos", { attrs: { embedded: "" } });
    expect(isEmbedded(host)).toBe(true);
    expect(host.querySelector("header")).toBeNull();
  });

  it("renders a row for each fixture repo", async () => {
    const { el, host } = await mountWindow<ReposTestElement>("lthn-view-repos");
    await el.updateComplete;
    const rows = host.querySelectorAll(".lthn-view-repos-row");
    expect(rows.length).toBe(el.repos.length);
  });
});

describe("lthn-view-repos — state derivations", () => {
  it("_summary counts by build state", async () => {
    const { el } = await mountWindow<ReposTestElement>("lthn-view-repos");
    el.repos = [
      { name: "a", lang: "Go", branch: "main", commit: "a1", build: "passing", prs: 0 },
      { name: "b", lang: "Go", branch: "main", commit: "b1", build: "passing", prs: 1 },
      { name: "c", lang: "TS", branch: "dev",  commit: "c1", build: "failing", prs: 2 },
    ];
    await el.updateComplete;
    const s = el._summary();
    expect(s.passing).toBe(2);
    expect(s.failing).toBe(1);
  });

  it("_summary counts running separately from passing/failing", async () => {
    const { el } = await mountWindow<ReposTestElement>("lthn-view-repos");
    el.repos = [
      { name: "a", lang: "Go", branch: "main", commit: "a1", build: "passing", prs: 0 },
      { name: "b", lang: "Go", branch: "main", commit: "b1", build: "running", prs: 0 },
      { name: "c", lang: "Go", branch: "main", commit: "c1", build: "failing", prs: 0 },
    ];
    await el.updateComplete;
    const s = el._summary();
    expect(s.passing).toBe(1);
    expect(s.failing).toBe(1);
  });

  it("subtitle reflects repo count", async () => {
    const { el, host } = await mountWindow<ReposTestElement>("lthn-view-repos");
    el.repos = [
      { name: "r1", lang: "Go", branch: "main", commit: "a", build: "passing", prs: 0 },
      { name: "r2", lang: "Go", branch: "main", commit: "b", build: "passing", prs: 0 },
    ];
    await el.updateComplete;
    const header = host.querySelector("header");
    expect(header?.textContent ?? "").toContain("2 watched");
  });
});

describe("lthn-view-repos — data-attributes", () => {
  it("each row has data-repo set to the repo name", async () => {
    const { el, host } = await mountWindow<ReposTestElement>("lthn-view-repos");
    el.repos = [
      { name: "lthn/desktop", lang: "Go", branch: "main", commit: "abc", build: "passing", prs: 0 },
    ];
    await el.updateComplete;
    const row = host.querySelector(".lthn-view-repos-row");
    expect(row?.getAttribute("data-repo")).toBe("lthn/desktop");
  });
});

describe("lthn-view-repos — live backend binding", () => {
  it("replaces the fixture when Status() returns 3 repos", async () => {
    (Status as unknown as { mockResolvedValue: (v: unknown) => void }).mockResolvedValue({
      Value: {
        repos: [
          { name: "core",     path: "/Users/snider/Code/core/core",     branch: "main", ahead: 0, behind: 0, dirty: false },
          { name: "desktop",  path: "/Users/snider/Code/lthn/desktop",  branch: "dev",  ahead: 2, behind: 0, dirty: true  },
          { name: "host.uk",  path: "/Users/snider/Code/lab/host.uk",   branch: "main", ahead: 0, behind: 1, dirty: false, error: "fetch refused" },
        ],
        scanned: 3,
      },
    });
    const { el, host } = await mountWindow<ReposTestElement>("lthn-view-repos");
    // Let the dynamic-import + Status() promise chain in
    // connectedCallback settle, then re-flush Lit's render.
    await flushAsync();
    await el.updateComplete;

    expect(el.repos.length).toBe(3);
    const names = el.repos.map((r) => r.name);
    expect(names).toEqual(["core", "desktop", "host.uk"]);

    // Rendered DOM shows the live repo names, not the seeded fixture.
    const text = host.textContent ?? "";
    expect(text).toContain("core");
    expect(text).toContain("desktop");
    expect(text).toContain("host.uk");
    // Sanity — at least one fixture name should be absent.
    expect(text).not.toContain("lethean/runtime");

    // build mapping: clean → passing, dirty → running, error → failing.
    expect(el.repos[0]?.build).toBe("passing");
    expect(el.repos[1]?.build).toBe("running");
    expect(el.repos[2]?.build).toBe("failing");
    // prs proxy maps from `ahead`.
    expect(el.repos[1]?.prs).toBe(2);
  });

  it("preserves the fixture when Status() rejects (binding unavailable)", async () => {
    (Status as unknown as { mockRejectedValue: (v: unknown) => void }).mockRejectedValue(new Error("simulated: no binding"));
    const { el } = await mountWindow<ReposTestElement>("lthn-view-repos");
    await flushAsync();
    await el.updateComplete;

    // Fixture is the 6-row design-reference seed — preserved on failure.
    expect(el.repos.length).toBe(6);
    expect(el.repos.some((r) => r.name === "lethean/desktop")).toBe(true);
    expect(el.repos.some((r) => r.name === "host-uk/platform")).toBe(true);
  });

  it("skips backend rows with empty names", async () => {
    (Status as unknown as { mockResolvedValue: (v: unknown) => void }).mockResolvedValue({
      Value: {
        repos: [
          { name: "good", path: "/x", branch: "main", dirty: false },
          { name: "",     path: "/y", branch: "main", dirty: false },
          { name: "   ",  path: "/z", branch: "main", dirty: false },
        ],
        scanned: 3,
      },
    });
    const { el } = await mountWindow<ReposTestElement>("lthn-view-repos");
    await flushAsync();
    await el.updateComplete;
    expect(el.repos.length).toBe(1);
    expect(el.repos[0]?.name).toBe("good");
  });
});

// SPDX-Licence-Identifier: EUPL-1.2

import { describe, it, expect, vi, beforeEach } from "vitest";
import type { LitElement } from "lit";
import { mountWindow, expectChromeTitle, isEmbedded } from "../../../test/window-fixture";
import "./social";

vi.mock("@desktop/marketing/social/service", () => ({
  List: vi.fn(),
}));

import { List as MockedList } from "@desktop/marketing/social/service";

type SocialEl = LitElement & {
  channelFilter: "" | "mastodon" | "x" | "linkedin" | "bluesky";
  updateComplete: Promise<boolean>;
};

describe("lthn-view-social — smoke", () => {
  it("mounts with the Social queue titlebar", async () => {
    const { host } = await mountWindow("lthn-view-social");
    expectChromeTitle(host, "Social queue");
  });

  it("renders four fixture posts by default", async () => {
    const { host } = await mountWindow("lthn-view-social");
    expect(host.querySelectorAll(".lthn-view-social-post").length).toBe(4);
  });

  it("renders the channel filter toolbar with all four channels", async () => {
    const { host } = await mountWindow("lthn-view-social");
    const chips = host.querySelectorAll(".lthn-view-social-channel-chip");
    expect(chips.length).toBe(4);
    const labels = Array.from(chips).map(c => c.getAttribute("data-channel"));
    expect(labels).toEqual(["mastodon", "x", "linkedin", "bluesky"]);
  });

  it("post state attribute reflects scheduled / sent", async () => {
    const { host } = await mountWindow("lthn-view-social");
    const states = Array.from(host.querySelectorAll(".lthn-view-social-post"))
      .map(p => p.getAttribute("data-state"));
    expect(states).toContain("scheduled");
    expect(states).toContain("sent");
  });

  it("renders an attachment chip for the v0.2 launch post", async () => {
    const { host } = await mountWindow("lthn-view-social");
    expect(host.querySelector(".lthn-view-social-attach")).not.toBeNull();
    expect(host.textContent).toContain("image attached");
  });

  it("filtering by channel narrows the visible posts", async () => {
    const { el, host } = await mountWindow<SocialEl>("lthn-view-social");
    el.channelFilter = "linkedin";
    await el.updateComplete;
    const posts = host.querySelectorAll(".lthn-view-social-post");
    // linkedin appears in 2 of the 4 fixture posts (v0.2 launch + hiring)
    expect(posts.length).toBe(2);
  });

  it("subtitle reports total posts and scheduled count", async () => {
    const { host } = await mountWindow("lthn-view-social");
    const header = host.querySelector("header");
    expect(header?.textContent).toContain("4 posts");
    expect(header?.textContent).toContain("2 scheduled");
  });
});

describe("lthn-view-social — two-shell", () => {
  it("embedded mode collapses the chrome", async () => {
    const { host } = await mountWindow("lthn-view-social", { attrs: { embedded: "" } });
    expect(isEmbedded(host)).toBe(true);
    expect(host.querySelector("header")).toBeNull();
  });
});

describe("lthn-view-social — backend wire", () => {
  type SocialEl = LitElement & {
    allPosts: { ch: string[]; when: string; state: string; text: string }[];
    _loadFromBackend: () => Promise<void>;
    updateComplete: Promise<boolean>;
  };

  beforeEach(() => {
    (MockedList as unknown as { mockReset: () => void }).mockReset();
  });

  it("backend posts replace fixture when present", async () => {
    (MockedList as unknown as { mockResolvedValue: (v: unknown) => void })
      .mockResolvedValue({ Value: { posts: [{ ch: ["mastodon"], when: "now", state: "scheduled", text: "Backend post text" }] } });

    const { el, host } = await mountWindow<SocialEl>("lthn-view-social");
    await new Promise(r => setTimeout(r, 0));
    await el.updateComplete;

    expect(host.textContent).toContain("Backend post text");
  });

  it("backend rejection keeps fixture data visible", async () => {
    (MockedList as unknown as { mockRejectedValue: (v: unknown) => void })
      .mockRejectedValue(new Error("unavailable"));

    const { el, host } = await mountWindow<SocialEl>("lthn-view-social");
    await el._loadFromBackend();
    await el.updateComplete;

    expect(host.textContent).toContain("Lethean v0.2 is out");
  });

  it("empty backend response keeps fixture data visible", async () => {
    (MockedList as unknown as { mockResolvedValue: (v: unknown) => void })
      .mockResolvedValue({ Value: { posts: [] } });

    const { el, host } = await mountWindow<SocialEl>("lthn-view-social");
    await el._loadFromBackend();
    await el.updateComplete;

    expect(host.textContent).toContain("Lethean v0.2 is out");
  });
});

describe("lthn-view-social — conflict-reload listener (Cascade W2)", () => {
  type SocialEl = LitElement & {
    allPosts: { ch: string[]; when: string; state: string; text: string }[];
    updateComplete: Promise<boolean>;
  };

  beforeEach(() => {
    (MockedList as unknown as { mockReset: () => void }).mockReset();
  });

  it("CONFLICT_RELOAD_EVENT with matching service triggers _loadFromBackend", async () => {
    const calls: number[] = [];
    (MockedList as unknown as { mockImplementation: (fn: () => Promise<unknown>) => void })
      .mockImplementation(() => {
        calls.push(1);
        return Promise.resolve({ Value: { posts: [
          { ch: ["mastodon"], when: "now", state: "scheduled", text: "Reloaded" },
        ] } });
      });

    const { el } = await mountWindow<SocialEl>("lthn-view-social");
    await new Promise(r => setTimeout(r, 0));
    await el.updateComplete;
    const baseline = calls.length;

    window.dispatchEvent(new CustomEvent("lthn:conflict:reload-requested", {
      detail: { service: "social.update" },
    }));
    await new Promise(r => setTimeout(r, 0));
    await el.updateComplete;

    expect(calls.length).toBeGreaterThan(baseline);
  });

  it("CONFLICT_RELOAD_EVENT with non-matching service is ignored", async () => {
    const calls: number[] = [];
    (MockedList as unknown as { mockImplementation: (fn: () => Promise<unknown>) => void })
      .mockImplementation(() => {
        calls.push(1);
        return Promise.resolve({ Value: { posts: [] } });
      });

    const { el } = await mountWindow<SocialEl>("lthn-view-social");
    await new Promise(r => setTimeout(r, 0));
    await el.updateComplete;
    const baseline = calls.length;

    window.dispatchEvent(new CustomEvent("lthn:conflict:reload-requested", {
      detail: { service: "content.update" }, // not ours
    }));
    await new Promise(r => setTimeout(r, 0));
    await el.updateComplete;

    expect(calls.length).toBe(baseline);
  });
});

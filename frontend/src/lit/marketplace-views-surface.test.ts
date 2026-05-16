// SPDX-Licence-Identifier: EUPL-1.2
//
// Tests for marketplace-views-surface helpers per
// plans/code/lthn/desktop/views/RFC.plugin-views.md §5.5 + §9.2.
//
// Coverage:
//   - TestDiscriminateView_IframeNoCaps_Good
//   - TestDiscriminateView_IframeSessionToken_Good
//   - TestDiscriminateView_Lit_Good
//   - TestHasAnyViews_{Good,Bad,Ugly}
//   - TestViewSummaryLine_{Good,Bad,Ugly}
//   - TestWantsSessionTokenCapability_{Good,Bad}
//   - TestRenderSessionTokenPrompt_InterpolatesName_Good
//   - TestRenderSessionTokenPrompt_DoesNotMention_Implementation
//     (UI-text discipline per feedback_ui_text_offer_not_implementation)

import { describe, it, expect } from "vitest";
import { render } from "lit";
import {
  CAPABILITY_SESSION_TOKEN,
  SESSION_TOKEN_PROMPT_COPY,
  VIEW_KIND_IFRAME,
  VIEW_KIND_LIT,
  discriminateView,
  hasAnyViews,
  renderSessionTokenPrompt,
  renderViewSummary,
  viewSummaryLine,
  wantsSessionTokenCapability,
  type ManifestWithViews,
  type PluginViewManifest,
} from "./marketplace-views-surface";

const opencodeView: PluginViewManifest = {
  ID: "opencode", Label: "OpenCode", Icon: "fa-robot",
  Kind: VIEW_KIND_IFRAME, Source: "/opencode",
  Capabilities: [CAPABILITY_SESSION_TOKEN],
};
const iframeNoCapView: PluginViewManifest = {
  ID: "viewer", Label: "Viewer", Icon: "fa-eye",
  Kind: VIEW_KIND_IFRAME, Source: "/viewer", Capabilities: [],
};
const litView: PluginViewManifest = {
  ID: "lthn-panel", Label: "Panel", Icon: "fa-cube",
  Kind: VIEW_KIND_LIT, Source: "lthn-panel", Capabilities: [],
};

const opencodeManifest: ManifestWithViews = {
  Plugin: { Code: "opencode", Views: [opencodeView] },
};
const dualViewManifest: ManifestWithViews = {
  Plugin: { Code: "multi", Views: [iframeNoCapView, opencodeView] },
};
const noPluginManifest: ManifestWithViews = {};
const emptyViewsManifest: ManifestWithViews = { Plugin: { Code: "x", Views: [] } };

// ─── discriminateView ──────────────────────────────────────────────────

describe("TestDiscriminateView_IframeNoCaps_Good", () => {
  it("returns the bare-shield discriminator for iframe-kind with no caps", () => {
    const d = discriminateView(iframeNoCapView);
    expect(d.iconClass).toBe("fa-shield");
    expect(d.label).toBe("Isolated webapp");
    expect(d.bodyCopy).toContain("isolated webview");
    expect(d.bodyCopy).not.toContain("token");
    expect(d.bodyCopy).not.toContain("iframe");
  });
});

describe("TestDiscriminateView_IframeSessionToken_Good", () => {
  it("returns the shield-with-key discriminator for iframe + session-token", () => {
    const d = discriminateView(opencodeView);
    expect(d.iconClass).toBe("fa-shield-halved");
    expect(d.label).toBe("Isolated webapp — requests account access");
    expect(d.bodyCopy).toContain("30 minutes");
    expect(d.bodyCopy).not.toContain("session");
    expect(d.bodyCopy).not.toContain("bearer");
    expect(d.bodyCopy).not.toContain("token");
  });
});

describe("TestDiscriminateView_Lit_Good", () => {
  it("returns the deeply-integrated discriminator for lit-kind", () => {
    const d = discriminateView(litView);
    expect(d.iconClass).toBe("fa-cubes");
    expect(d.label).toBe("Deeply integrated");
    expect(d.bodyCopy).toContain("Full app access");
    expect(d.bodyCopy).not.toContain("custom element");
    expect(d.bodyCopy).not.toContain("lit");
  });
});

// ─── hasAnyViews ───────────────────────────────────────────────────────

describe("TestHasAnyViews_Good", () => {
  it("returns true when the manifest declares at least one view", () => {
    expect(hasAnyViews(opencodeManifest)).toBe(true);
    expect(hasAnyViews(dualViewManifest)).toBe(true);
  });
});

describe("TestHasAnyViews_Bad", () => {
  it("returns false for empty/missing views/plugin", () => {
    expect(hasAnyViews(noPluginManifest)).toBe(false);
    expect(hasAnyViews(emptyViewsManifest)).toBe(false);
  });
});

describe("TestHasAnyViews_Ugly", () => {
  it("tolerates null + undefined input", () => {
    expect(hasAnyViews(null)).toBe(false);
    expect(hasAnyViews(undefined)).toBe(false);
  });
});

// ─── viewSummaryLine ───────────────────────────────────────────────────

describe("TestViewSummaryLine_Good", () => {
  it("builds the comma-joined Provides views line", () => {
    expect(viewSummaryLine(opencodeManifest)).toBe("Provides views: OpenCode");
    expect(viewSummaryLine(dualViewManifest)).toBe("Provides views: Viewer, OpenCode");
  });
});

describe("TestViewSummaryLine_Bad", () => {
  it("returns empty when no views are declared", () => {
    expect(viewSummaryLine(noPluginManifest)).toBe("");
    expect(viewSummaryLine(emptyViewsManifest)).toBe("");
  });
});

describe("TestViewSummaryLine_Ugly", () => {
  it("falls back to view ID when the label is missing", () => {
    const m: ManifestWithViews = {
      Plugin: { Code: "x", Views: [{ ID: "fallback", Kind: VIEW_KIND_IFRAME }] },
    };
    expect(viewSummaryLine(m)).toBe("Provides views: fallback");
  });

  it("skips views with neither label nor id", () => {
    const m: ManifestWithViews = {
      Plugin: { Code: "x", Views: [{ Kind: VIEW_KIND_IFRAME }] },
    };
    expect(viewSummaryLine(m)).toBe("");
  });
});

// ─── wantsSessionTokenCapability ───────────────────────────────────────

describe("TestWantsSessionTokenCapability_Good", () => {
  it("returns true when any view in the manifest declares session-token", () => {
    expect(wantsSessionTokenCapability(opencodeManifest)).toBe(true);
    expect(wantsSessionTokenCapability(dualViewManifest)).toBe(true);
  });
});

describe("TestWantsSessionTokenCapability_Bad", () => {
  it("returns false when no view declares the capability", () => {
    const m: ManifestWithViews = {
      Plugin: { Code: "x", Views: [iframeNoCapView, litView] },
    };
    expect(wantsSessionTokenCapability(m)).toBe(false);
    expect(wantsSessionTokenCapability(noPluginManifest)).toBe(false);
    expect(wantsSessionTokenCapability(null)).toBe(false);
  });
});

// ─── renderSessionTokenPrompt ──────────────────────────────────────────

describe("TestRenderSessionTokenPrompt_InterpolatesName_Good", () => {
  it("renders the §5.5 copy with the plugin display name interpolated", () => {
    const host = document.createElement("div");
    render(renderSessionTokenPrompt("OpenCode"), host);
    expect(host.textContent).toContain("OpenCode");
    expect(host.textContent).toContain("30 minutes");
    expect(host.textContent).toContain("read and write");
  });

  it("falls back to 'This plugin' when name is empty", () => {
    const host = document.createElement("div");
    render(renderSessionTokenPrompt(""), host);
    expect(host.textContent).toContain("This plugin");
  });
});

describe("TestRenderSessionTokenPrompt_DoesNotMention_Implementation", () => {
  it("strips every implementation-noun per UI-text discipline", () => {
    const banned = ["session token", "bearer", "iframe", "WebView", "postMessage", "JWT"];
    for (const word of banned) {
      expect(SESSION_TOKEN_PROMPT_COPY.toLowerCase())
        .not.toContain(word.toLowerCase());
    }
  });
});

// ─── renderViewSummary ─────────────────────────────────────────────────

describe("renderViewSummary", () => {
  it("returns null when the manifest declares no views", () => {
    expect(renderViewSummary(noPluginManifest)).toBeNull();
    expect(renderViewSummary(null)).toBeNull();
  });

  it("renders one discriminator block per declared view", () => {
    const host = document.createElement("div");
    const tpl = renderViewSummary(dualViewManifest);
    if (!tpl) throw new Error("expected a template");
    render(tpl, host);
    expect(host.textContent).toContain("Provides views: Viewer, OpenCode");
    expect(host.textContent).toContain("Isolated webapp"); // no-caps Viewer
    expect(host.textContent).toContain("requests account access"); // session-token OpenCode
  });
});

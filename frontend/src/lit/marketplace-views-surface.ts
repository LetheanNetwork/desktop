// SPDX-Licence-Identifier: EUPL-1.2
//
// Marketplace UI helpers for the plugin-views surface — the §9.2
// install-time discriminator + the §5.5 capability prompt copy +
// the "Provides views" line per
// plans/code/lthn/desktop/views/RFC.plugin-views.md.
//
// Wording follows [[feedback_ui_text_offer_not_implementation]] —
// strings describe WHAT the user gets, never HOW it's implemented.
// No "iframe", no "WebView", no "custom element", no "session token",
// no "bearer auth". The user sees "Isolated webapp" + "account access"
// and decides; the binding to the underlying primitive lives in code
// alone.
//
// Used by frontend/src/lit/ext/marketplace-window.ts at install time
// to inject:
//
//   - the §5.5 capability prompt block when a view's capabilities[]
//     contains "session-token" (load-bearing per Cerberus HIGH-1)
//   - the §9.2 manifest discriminator (icon + label + copy) so the
//     user's install decision is informed BEFORE the install action
//   - the "Provides views" summary line ("This plugin adds these
//     views to your shell: X, Y")
//
// Helpers are pure functions returning structured data — the
// marketplace-window component renders the data using its existing
// modal chrome so we don't fork the modal layer.

import { html, type TemplateResult } from "lit";

// PluginViewManifest mirrors the YAML-side PluginView shape. The Go
// marketplace.PluginView struct serialises to this JSON when the
// manifest is returned by FetchManifest. Field names follow the Go
// yaml/json tags verbatim.
export interface PluginViewManifest {
  ID?:           string;
  Label?:        string;
  Icon?:         string;
  Kind?:         string;
  Source?:       string;
  Capabilities?: string[];
}

// ManifestWithViews is the subset of BundleManifest the install flow
// consumes for view-surface inference. Optional fields default to
// empty / null when the manifest doesn't declare a plugin block.
export interface ManifestWithViews {
  Plugin?: {
    Code?:  string;
    Views?: PluginViewManifest[];
  } | null;
}

// VIEW_KIND_IFRAME / VIEW_KIND_LIT mirror the Go-side const literals
// so the discriminator switch never falls through to a typo branch.
export const VIEW_KIND_IFRAME = "iframe";
export const VIEW_KIND_LIT = "lit";

// CAPABILITY_SESSION_TOKEN is the §5.5 capability literal that
// triggers the strongest install-time copy. Future capabilities
// (vi-events, marketplace) ship their own prompts when their
// brokers land — for v1 only session-token is honoured.
export const CAPABILITY_SESSION_TOKEN = "session-token";

// ViewDiscriminator describes the §9.2 install-time iconography +
// label + copy for one view. Three shapes per the RFC table:
//
//   - iframe + no caps        → shield        / "Isolated webapp"
//   - iframe + session-token  → shield+key    / "Isolated webapp — requests account access"
//   - lit                     → shell         / "Deeply integrated — full app access"
export interface ViewDiscriminator {
  iconClass: string;
  label:     string;
  bodyCopy:  string;
}

// discriminateView returns the install-time shape for one view per
// RFC §9.2. The third-party-lit-rejection branch is owned by the Go
// manifest validator — by the time this code sees a manifest, any
// third-party lit-kind view has already been rejected upstream, so
// the lit branch here only fires for first-party (pluginCode === "lthn")
// or future tier-1 surfaces.
export function discriminateView(v: PluginViewManifest): ViewDiscriminator {
  if (v.Kind === VIEW_KIND_LIT) {
    return {
      iconClass: "fa-cubes",
      label:     "Deeply integrated",
      bodyCopy:  "Full app access — runs alongside the rest of the shell.",
    };
  }
  // Default = iframe (the most common shape).
  const wantsSession = (v.Capabilities ?? []).includes(CAPABILITY_SESSION_TOKEN);
  if (wantsSession) {
    return {
      iconClass: "fa-shield-halved",
      label:     "Isolated webapp — requests account access",
      bodyCopy:  "Runs in its own isolated webview. Asks to read and write " +
                 "any data you have unlocked, for up to 30 minutes per unlock.",
    };
  }
  return {
    iconClass: "fa-shield",
    label:     "Isolated webapp",
    bodyCopy:  "Runs in its own isolated webview. Does not request access " +
               "to your account data.",
  };
}

// hasAnyViews returns true when the manifest declares at least one
// plugin view. Cheap probe used by the install flow to decide
// whether to render the views section in the prompt modal.
export function hasAnyViews(m: ManifestWithViews | null | undefined): boolean {
  return !!(m?.Plugin?.Views && m.Plugin.Views.length > 0);
}

// viewSummaryLine returns the catalogue-side "Provides views" line,
// e.g. "Provides views: OpenCode, Settings". Returns "" when the
// manifest declares no views (caller renders nothing). Sorted by
// declaration order — stable across calls.
export function viewSummaryLine(m: ManifestWithViews | null | undefined): string {
  if (!hasAnyViews(m)) return "";
  const labels = (m?.Plugin?.Views ?? [])
    .map(v => (v.Label ?? v.ID ?? "").trim())
    .filter(s => s.length > 0);
  if (labels.length === 0) return "";
  return `Provides views: ${labels.join(", ")}`;
}

// wantsSessionTokenCapability returns true when ANY view in the
// manifest declares the session-token capability. Drives the §5.5
// prompt copy branch — if any single view requests it the user sees
// the strongest copy once for the whole install.
export function wantsSessionTokenCapability(
  m: ManifestWithViews | null | undefined,
): boolean {
  if (!hasAnyViews(m)) return false;
  return (m?.Plugin?.Views ?? []).some(v =>
    (v.Capabilities ?? []).includes(CAPABILITY_SESSION_TOKEN),
  );
}

// SESSION_TOKEN_PROMPT_COPY is the §5.5 capability prompt copy. The
// 30-minute figure tracks the auth-gate Stage E §6 session-tier
// ceiling — when that ceiling changes, this copy MUST be re-derived
// (the RFC cross-references this exact dependency in §5.5).
//
// Placeholder %d gets replaced with the plugin display name at
// render time so the copy is informed by which plugin is asking.
export const SESSION_TOKEN_PROMPT_COPY =
  "%d can read and write any data you have unlocked in lthn, " +
  "for up to 30 minutes per unlock. Allow?";

// renderSessionTokenPrompt returns the §5.5 capability prompt body.
// The display name is interpolated into the copy template; falsy
// names fall back to "This plugin" so the sentence still parses.
export function renderSessionTokenPrompt(displayName: string): TemplateResult {
  const name = (displayName ?? "").trim() || "This plugin";
  const body = SESSION_TOKEN_PROMPT_COPY.replace("%d", name);
  return html`
    <div style="display:flex; gap:10px; align-items:flex-start;
                padding:12px 14px; border-radius:8px;
                background:rgba(64,193,197,0.08);
                border:1px solid rgba(64,193,197,0.20);">
      <i class="fa-solid fa-shield-halved"
         style="font-size:14px; color:var(--brand-300); margin-top:2px;"></i>
      <div style="font-size:12px; color:var(--fg-0); line-height:1.55;">${body}</div>
    </div>
  `;
}

// renderViewSummary returns the catalogue/install-time "Provides
// views" surface. Pairs the summary line with one discriminator
// chip per declared view so the user sees both the count AND the
// security shape before approving.
export function renderViewSummary(
  m: ManifestWithViews | null | undefined,
): TemplateResult | null {
  if (!hasAnyViews(m)) return null;
  const line = viewSummaryLine(m);
  const views = m?.Plugin?.Views ?? [];
  return html`
    <div style="display:flex; flex-direction:column; gap:6px;">
      <div style="font-size:11px; color:var(--fg-2);
                  font-family:var(--font-mono);">${line}</div>
      <div style="display:flex; flex-direction:column; gap:6px;">
        ${views.map(v => {
          const d = discriminateView(v);
          return html`
            <div style="display:flex; gap:10px; align-items:flex-start;
                        padding:10px 12px; border-radius:8px;
                        background:rgba(0,0,0,0.18);
                        border:1px solid rgba(255,255,255,0.05);">
              <i class="fa-solid ${d.iconClass}"
                 style="font-size:12px; color:var(--brand-300); margin-top:2px;"></i>
              <div>
                <div style="font-size:12px; color:var(--fg-0);">
                  ${(v.Label ?? v.ID ?? "").trim()} · ${d.label}
                </div>
                <div style="font-size:10.5px; color:var(--fg-2);
                            margin-top:3px; line-height:1.5;">
                  ${d.bodyCopy}
                </div>
              </div>
            </div>
          `;
        })}
      </div>
    </div>
  `;
}

// SPDX-Licence-Identifier: EUPL-1.2
//
// <lthn-tools-landing> — the contained-assistant tools hub.
//
// Lists the installed contained tools (marketplace bundles that register an
// iframe view) as cards. "Open" routes to the tool's panel via the same
// `lthn:view-switcher:select` event the view-switcher emits, so the shell's
// _onViewSwitcherSelect → _selectView lands the proxied panel through one code
// path. This is the landing the tray's "Open Tools" button targets.
//
// Each tool is an Apple-native contained webapp, proxied through the host
// (pkg/plugin.ProxyGroup) so a framing-hostile app still renders in the shell —
// Odysseus today; Hermes / OpenClaw / ComfyUI next (shown as "coming soon").

import { LitElement, html, type TemplateResult } from "lit";
import { ListInstalled } from "@desktop/marketplace/service";

// InstalledBundle mirrors the marketplace.InstalledBundle fields the landing
// needs. ListInstalled returns a core.Result whose Value is this array.
interface InstalledBundle {
  BundleID: string;
  Display: string;
  Status: string;
}

// COMING_SOON names the contained tools on the roadmap but not yet bundled —
// rendered as disabled cards so the hub advertises what's next.
const COMING_SOON: { id: string; label: string; icon: string }[] = [
  { id: "hermes", label: "Hermes", icon: "fa-feather" },
  { id: "openclaw", label: "OpenClaw", icon: "fa-paw" },
  { id: "comfyui", label: "ComfyUI", icon: "fa-wand-magic-sparkles" },
];

// ICONS maps a known bundle id to its glyph. InstalledBundle doesn't carry the
// manifest icon today, so fall back to a generic cube.
const ICONS: Record<string, string> = { odysseus: "fa-compass" };

export class LthnToolsLanding extends LitElement {
  static readonly properties = {
    installed: { state: true },
    loading: { state: true },
    error: { state: true },
  };

  declare installed: InstalledBundle[];
  declare loading: boolean;
  declare error: string;

  constructor() {
    super();
    this.installed = [];
    this.loading = true;
    this.error = "";
  }

  // Light DOM so the global Font Awesome stylesheet + theme CSS vars apply
  // (the app-wide convention for ext windows; shadow DOM would wall out the
  // FA ::before content rules).
  protected override createRenderRoot(): HTMLElement {
    return this;
  }

  override connectedCallback(): void {
    super.connectedCallback();
    void this.refresh();
  }

  private async refresh(): Promise<void> {
    this.loading = true;
    try {
      const r = await ListInstalled();
      if (r && (r as { OK?: boolean }).OK === false) {
        this.error = "Could not list installed tools";
        this.installed = [];
      } else {
        this.installed = ((r as { Value?: InstalledBundle[] })?.Value) ?? [];
      }
    } catch (e) {
      this.error = (e as Error).message ?? String(e);
    } finally {
      this.loading = false;
    }
  }

  // open routes to the tool's panel. A single-view bundle's view id equals its
  // bundle id (Odysseus: bundle "odysseus" → view "odysseus"), so the bundle
  // id is the viewId. Routed through the switcher event so persistence + pane
  // restore stays the one _selectView path.
  private open(bundleID: string): void {
    this.dispatchEvent(
      new CustomEvent("lthn:view-switcher:select", {
        detail: { viewId: bundleID },
        bubbles: true,
        composed: true,
      }),
    );
  }

  override render(): TemplateResult {
    const soon = COMING_SOON.filter(
      (s) => !this.installed.some((b) => b.BundleID === s.id),
    );
    return html`
      <div style="height:100%; overflow:auto; padding:28px; box-sizing:border-box;">
        <h2 style="margin:0 0 4px; font-size:20px; font-weight:600;">Tools</h2>
        <p style="margin:0 0 24px; color:var(--fg-3,#9a9a9a); font-size:13px;">
          Local, contained assistant apps — each runs Apple-native in its own
          sandbox.
        </p>
        ${this.loading
          ? html`<p style="color:var(--fg-3,#9a9a9a);">Loading…</p>`
          : html`
              <div
                style="display:grid; grid-template-columns:repeat(auto-fill,minmax(220px,1fr)); gap:14px;"
              >
                ${this.installed.map((b) => this._card(b))}
                ${soon.map((s) => this._soon(s))}
              </div>
              ${this.installed.length === 0
                ? html`<p style="color:var(--fg-3,#9a9a9a); margin-top:24px;">
                    No tools installed yet — add one from the marketplace.
                  </p>`
                : null}
            `}
        ${this.error
          ? html`<p style="color:#e06c75; margin-top:16px;">${this.error}</p>`
          : null}
      </div>
    `;
  }

  private _card(b: InstalledBundle): TemplateResult {
    const running = (b.Status || "").toLowerCase() === "running";
    return html`
      <div
        style="background:var(--bg-2,#1b1b1f); border:1px solid var(--border,#2a2a30); border-radius:10px; padding:16px; display:flex; flex-direction:column; gap:10px;"
      >
        <div style="display:flex; align-items:center; gap:10px;">
          <i
            class="fa-solid ${ICONS[b.BundleID] || "fa-cube"}"
            style="font-size:20px; width:24px; text-align:center; color:var(--accent,#6ea8fe);"
          ></i>
          <span style="font-size:15px; font-weight:600;"
            >${b.Display || b.BundleID}</span
          >
        </div>
        <span
          style="align-self:flex-start; font-size:11px; padding:2px 8px; border-radius:999px; ${running
            ? "background:rgba(80,200,120,.15); color:#5cc878;"
            : "background:var(--bg-3,#26262c); color:var(--fg-3,#9a9a9a);"}"
          >${b.Status || "stopped"}</span
        >
        <button
          @click=${() => this.open(b.BundleID)}
          style="margin-top:auto; align-self:flex-start; background:var(--accent,#3b6fd4); color:#fff; border:0; border-radius:7px; padding:7px 14px; font-size:13px; cursor:pointer;"
        >
          Open
        </button>
      </div>
    `;
  }

  private _soon(s: { id: string; label: string; icon: string }): TemplateResult {
    return html`
      <div
        style="background:var(--bg-2,#1b1b1f); border:1px solid var(--border,#2a2a30); border-radius:10px; padding:16px; display:flex; flex-direction:column; gap:10px; opacity:.5;"
      >
        <div style="display:flex; align-items:center; gap:10px;">
          <i
            class="fa-solid ${s.icon}"
            style="font-size:20px; width:24px; text-align:center; color:var(--accent,#6ea8fe);"
          ></i>
          <span style="font-size:15px; font-weight:600;">${s.label}</span>
        </div>
        <span
          style="align-self:flex-start; font-size:11px; padding:2px 8px; border-radius:999px; background:var(--bg-3,#26262c); color:var(--fg-3,#9a9a9a);"
          >coming soon</span
        >
        <button
          disabled
          style="margin-top:auto; align-self:flex-start; background:var(--bg-3,#26262c); color:var(--fg-3,#777); border:0; border-radius:7px; padding:7px 14px; font-size:13px;"
        >
          Open
        </button>
      </div>
    `;
  }
}

if (!customElements.get("lthn-tools-landing")) {
  customElements.define("lthn-tools-landing", LthnToolsLanding);
}

declare global {
  interface HTMLElementTagNameMap {
    "lthn-tools-landing": LthnToolsLanding;
  }
}

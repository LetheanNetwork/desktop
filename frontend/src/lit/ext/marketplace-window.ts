// SPDX-Licence-Identifier: EUPL-1.2
// E3.x · marketplace — <lthn-marketplace-window>
//
// Bundle catalogue + install + lifecycle management for lthn-vm bundles.
// Sits in the EXTEND nav group alongside Integrations, Providers, Tools.
//
// Tabs:
//   Browse    — live catalogue from marketplace.lthn.ai, search + category filter
//   Installed — installed bundles with start/stop/uninstall + status pills
//
// Install flow:
//   1. User clicks Install on a catalogue card
//   2. If the bundle manifest declares env[] entries, an env-prompt
//      modal collects values (secret fields rendered as password inputs)
//   3. If the bundle declares permissions[], a permission-disclosure
//      modal summarises what data the bundle can access — user approves
//   4. FetchManifest + Install(input) → lifecycle enters "starting"
//
// The old binary-plugin-host surface (lit/ide/marketplace-window.ts)
// is a separate component — do not merge the two. That surface calls
// Search/Installed/InstallPlugin/Remove against the plugin host; this
// surface calls FetchIndex/Install/Launch/Stop/Uninstall/ListInstalled
// against the lthn-vm bundle lifecycle.

import { LitElement, html, nothing, type TemplateResult } from "lit";
import { renderChrome } from "../chrome";
import { T } from "@lthn/i18n/coreservice";
import {
  FetchIndex, FetchManifest, Install, Launch,
  Stop, Uninstall, ListInstalled, Status,
} from "@desktop/marketplace/service";
import type { InstallInput } from "@desktop/marketplace/models";
import {
  fetchCatalogue, filterCatalogue, categories, categoryLabel,
  type CatalogueEntry, type CatalogueIndex,
} from "./_marketplace-catalogue";
import {
  hasAnyViews,
  renderSessionTokenPrompt,
  renderViewSummary,
  wantsSessionTokenCapability,
  type ManifestWithViews,
  type PluginViewManifest,
} from "../marketplace-views-surface";

// InstalledBundle mirrors the Go marketplace.InstalledBundle shape.
interface InstalledBundle {
  BundleID:       string;
  Display:        string;
  ManifestSchema: string;
  Status:         string;
  InstalledAt:    string;
}

// BundleStatusOutput mirrors marketplace.BundleStatusOutput.
interface BundleStatusOutput {
  BundleID: string;
  Display:  string;
  Status:   string;
  Handles:  { SandboxID: string; Status: string; HostPort: number }[];
}

// BundleChangedPayload mirrors the Go marketplace.BundleChanged shape
// forwarded by the desktop bridge on "marketplace:bundle:changed".
interface BundleChangedPayload {
  phase:  string;
  bundle: InstalledBundle;
  before: InstalledBundle;
  at:     string;
}

// EnvPrompt holds a pending install's env-collection state.
// views[] + wantsSession are derived from the fetched manifest at
// install time per plans/code/lthn/desktop/views/RFC.plugin-views.md
// §9.2 (install-time discriminator) + §5.5 (capability prompt copy).
interface EnvPrompt {
  entry:        CatalogueEntry;
  fields:       { key: string; prompt: string; type: string; value: string }[];
  perms:        { scope: string; mode: string; reason: string }[];
  views:        PluginViewManifest[];
  wantsSession: boolean;
  step:         "env" | "perms" | "installing";
}

type Tab = "browse" | "installed";

class LthnMarketplaceWindow extends LitElement {
  static readonly properties = {
    w:         { type: Number },
    h:         { type: Number },
    embedded:  { type: Boolean, reflect: true },
    chrome:    { state: true },
    tab:       { state: true },
    catalogue: { state: true },
    installed: { state: true },
    query:     { state: true },
    category:  { state: true },
    loading:   { state: true },
    err:       { state: true },
    info:      { state: true },
    prompt:    { state: true },
    t:         { state: true },
  };
  declare w: number;
  declare h: number;
  declare embedded: boolean;
  declare chrome: { title: string; subtitle: string };
  declare tab: Tab;
  declare catalogue: CatalogueIndex;
  declare installed: InstalledBundle[];
  declare query: string;
  declare category: string;
  declare loading: boolean;
  declare err: string;
  declare info: string;
  declare prompt: EnvPrompt | null;
  declare t: {
    title: string; subtitle: string;
    tabBrowse: string; tabInstalled: string;
    btnRefresh: string; btnRefreshing: string;
    searchPlaceholder: string;
    catAll: string;
    install: string; installing: string;
    start: string; stop: string; uninstall: string;
    statusRunning: string; statusStopped: string;
    statusStarting: string; statusFailed: string; statusIdle: string;
    empty: string; emptyInstalled: string;
    permTitle: string; permApprove: string; permCancel: string;
    envTitle: string; envNext: string; envCancel: string;
    footerCatalogued: string; footerInstalled: string;
    fromCache: string;
    errInstall: string; errStart: string; errStop: string; errUninstall: string;
  };

  constructor() {
    super();
    this.w = 1100; this.h = 740; this.embedded = false;
    this.chrome = { title: "Marketplace", subtitle: "browse · install · manage" };
    this.tab = "browse";
    this.catalogue = { entries: [], cachedAt: "", fromCache: false };
    this.installed = [];
    this.query = "";
    this.category = "";
    this.loading = false;
    this.err = "";
    this.info = "";
    this.prompt = null;
    this.t = {
      title: "Marketplace", subtitle: "browse · install · manage",
      tabBrowse: "Browse", tabInstalled: "Installed",
      btnRefresh: "Refresh", btnRefreshing: "Loading…",
      searchPlaceholder: "search bundles…",
      catAll: "all",
      install: "Install", installing: "Installing…",
      start: "Start", stop: "Stop", uninstall: "Uninstall",
      statusRunning: "running", statusStopped: "stopped",
      statusStarting: "starting", statusFailed: "failed", statusIdle: "idle",
      empty: "No bundles match the current filter.",
      emptyInstalled: "No bundles installed yet — browse the catalogue and click Install.",
      permTitle: "This bundle requests access to:",
      permApprove: "Approve & install", permCancel: "Cancel",
      envTitle: "Configure bundle",
      envNext: "Continue", envCancel: "Cancel",
      footerCatalogued: "%d catalogued", footerInstalled: "%d installed",
      fromCache: "catalogue cached",
      errInstall: "Install failed", errStart: "Start failed",
      errStop: "Stop failed", errUninstall: "Uninstall failed",
    };
  }
  createRenderRoot() { return this; }

  async connectedCallback() {
    super.connectedCallback();
    const [title, subtitle] = await Promise.all([
      T("window.marketplace.title"),
      T("window.marketplace.subtitle"),
    ]);
    this.chrome = { title, subtitle };
    // Load i18n keys — fall back to constructor defaults on failure.
    this._loadT().catch(() => { /* defaults stand */ });
    await this._refresh();
    // Subscribe to BundleChanged events so status pills react to lifecycle
    // changes without polling ListInstalled on a timer.
    try {
      const { Events } = await import("@wailsio/runtime");
      this._unsubBundleChanged = Events.On(
        "marketplace:bundle:changed",
        (e: { data: BundleChangedPayload }) => {
          const ev = e?.data;
          if (!ev?.bundle?.BundleID) return;
          this._applyBundleChanged(ev);
        },
      );
    } catch {
      // Canvas / test environment — no Wails runtime, poll remains the fallback.
    }
  }

  private async _loadT() {
    const keys = [
      "tab_browse", "tab_installed",
      "btn_refresh", "btn_refreshing",
      "search_placeholder", "cat_all",
      "btn_install", "btn_installing",
      "btn_start", "btn_stop", "btn_uninstall",
      "status_running", "status_stopped", "status_starting",
      "status_failed", "status_idle",
      "empty", "empty_installed",
      "perm_title", "perm_approve", "perm_cancel",
      "env_title", "env_next", "env_cancel",
      "footer_catalogued", "footer_installed", "from_cache",
      "err_install", "err_start", "err_stop", "err_uninstall",
    ];
    const vals = await Promise.all(keys.map(k => T(`window.marketplace.${k}`)));
    const m: Record<string, string> = {};
    keys.forEach((k, i) => { m[k] = vals[i]; });
    this.t = {
      title: this.t.title, subtitle: this.t.subtitle,
      tabBrowse: m.tab_browse, tabInstalled: m.tab_installed,
      btnRefresh: m.btn_refresh, btnRefreshing: m.btn_refreshing,
      searchPlaceholder: m.search_placeholder, catAll: m.cat_all,
      install: m.btn_install, installing: m.btn_installing,
      start: m.btn_start, stop: m.btn_stop, uninstall: m.btn_uninstall,
      statusRunning: m.status_running, statusStopped: m.status_stopped,
      statusStarting: m.status_starting, statusFailed: m.status_failed,
      statusIdle: m.status_idle,
      empty: m.empty, emptyInstalled: m.empty_installed,
      permTitle: m.perm_title, permApprove: m.perm_approve,
      permCancel: m.perm_cancel,
      envTitle: m.env_title, envNext: m.env_next, envCancel: m.env_cancel,
      footerCatalogued: m.footer_catalogued,
      footerInstalled: m.footer_installed,
      fromCache: m.from_cache,
      errInstall: m.err_install, errStart: m.err_start,
      errStop: m.err_stop, errUninstall: m.err_uninstall,
    };
  }

  private async _refresh() {
    if (this.loading) return;
    this.loading = true;
    this.err = "";
    try {
      const [cat, instR] = await Promise.all([
        fetchCatalogue(),
        ListInstalled(),
      ]);
      this.catalogue = cat;
      if (instR.OK) {
        const raw = instR.Value as InstalledBundle[] | null;
        this.installed = raw ?? [];
      }
    } catch (e: unknown) {
      this.err = e instanceof Error ? e.message : String(e);
    } finally {
      this.loading = false;
    }
  }

  // _beginInstall starts the install flow for a catalogue entry.
  // If the manifest has env[] or permissions[], opens the prompt modal.
  // Otherwise installs immediately.
  private async _beginInstall(entry: CatalogueEntry) {
    this.err = "";
    this.info = "";
    try {
      const manR = await FetchManifest(entry.sourceURL);
      if (!manR.OK) {
        this.err = `${this.t.errInstall}: ${String(manR.Value ?? "unknown")}`;
        return;
      }
      const manifest = manR.Value as {
        Env?: { Key: string; Prompt: string; Type: string; Default: string }[];
        Permissions?: { Scope: string; Mode: string; Reason: string }[];
      } & ManifestWithViews;
      const envFields = (manifest.Env ?? []).map(e => ({
        key: e.Key, prompt: e.Prompt, type: e.Type,
        value: e.Default?.startsWith("${random") ? "" : (e.Default ?? ""),
      }));
      const perms = (manifest.Permissions ?? []).map(p => ({
        scope: p.Scope, mode: p.Mode, reason: p.Reason,
      }));
      const views = manifest.Plugin?.Views ?? [];
      const wantsSession = wantsSessionTokenCapability(manifest);

      // Skip env step if no fields need user input (all have non-random defaults).
      const needsEnv = envFields.some(f => f.value === "" && f.type === "secret");
      const needsPerms = perms.length > 0;
      const needsViewsPrompt = hasAnyViews(manifest);

      if (!needsEnv && !needsPerms && !needsViewsPrompt) {
        await this._doInstall(entry, manifest, {});
        return;
      }

      this.prompt = {
        entry,
        fields: envFields,
        perms,
        views,
        wantsSession,
        // When the only reason to prompt is the views surface, jump
        // straight to the perms step so the views block renders there;
        // we don't introduce a fourth step solely for views.
        step: needsEnv ? "env" : "perms",
      };
    } catch (e: unknown) {
      this.err = e instanceof Error ? e.message : String(e);
    }
  }

  private async _doInstall(
    entry: CatalogueEntry,
    manifest: object,
    env: Record<string, string>,
  ) {
    if (this.prompt) this.prompt = { ...this.prompt, step: "installing" };
    try {
      const input: InstallInput = {
        Manifest: manifest as never,
        Env: env,
        ForceStaleInstall: false,
        ExpectedManifestDigest: "",
        PrevGrantedPermissions: {},
      };
      const r = await Install(input);
      if (!r.OK) {
        this.err = `${this.t.errInstall}: ${(r as { Error?: string }).Error ?? "unknown"}`;
      } else {
        this.info = `${entry.display} installed.`;
      }
    } catch (e: unknown) {
      this.err = e instanceof Error ? e.message : String(e);
    } finally {
      this.prompt = null;
      await this._refresh();
    }
  }

  private async _start(bundleID: string) {
    this.err = ""; this.info = "";
    try {
      const r = await Launch(bundleID);
      if (!r.OK) this.err = `${this.t.errStart}: ${String((r as { Value?: unknown }).Value ?? "")}`;
      else this.info = `${bundleID} started.`;
    } catch (e: unknown) {
      this.err = e instanceof Error ? e.message : String(e);
    }
    await this._refresh();
  }

  private async _stop(bundleID: string) {
    this.err = ""; this.info = "";
    try {
      const r = await Stop(bundleID);
      if (!r.OK) this.err = `${this.t.errStop}: ${String((r as { Value?: unknown }).Value ?? "")}`;
      else this.info = `${bundleID} stopped.`;
    } catch (e: unknown) {
      this.err = e instanceof Error ? e.message : String(e);
    }
    await this._refresh();
  }

  private async _uninstall(bundleID: string) {
    this.err = ""; this.info = "";
    try {
      const r = await Uninstall(bundleID);
      if (!r.OK) this.err = `${this.t.errUninstall}: ${String((r as { Value?: unknown }).Value ?? "")}`;
      else this.info = `${bundleID} uninstalled.`;
    } catch (e: unknown) {
      this.err = e instanceof Error ? e.message : String(e);
    }
    await this._refresh();
  }

  private _unsubBundleChanged: (() => void) | null = null;

  private _applyBundleChanged(ev: BundleChangedPayload) {
    const id = ev.bundle.BundleID;
    if (ev.phase === "uninstalled") {
      this.installed = this.installed.filter(b => b.BundleID !== id);
    } else if (ev.phase === "installed") {
      if (!this.installed.some(b => b.BundleID === id)) {
        this.installed = [...this.installed, ev.bundle];
      }
    } else {
      // launched / stopped — update status in place
      this.installed = this.installed.map(b =>
        b.BundleID === id ? { ...b, Status: ev.bundle.Status } : b,
      );
    }
  }

  disconnectedCallback() {
    super.disconnectedCallback();
    this._unsubBundleChanged?.();
    this._unsubBundleChanged = null;
  }

  private _isInstalled(name: string): boolean {
    return this.installed.some(b => b.BundleID === name);
  }

  private _statusPill(status: string): TemplateResult {
    const map: Record<string, { variant: string; label: string }> = {
      running:  { variant: "ok",      label: this.t.statusRunning },
      starting: { variant: "pending", label: this.t.statusStarting },
      stopped:  { variant: "warn",    label: this.t.statusStopped },
      failed:   { variant: "err",     label: this.t.statusFailed },
      idle:     { variant: "muted",   label: this.t.statusIdle },
    };
    const s = map[status] ?? { variant: "muted", label: status };
    return html`<lthn-state-pill variant="${s.variant}">${s.label}</lthn-state-pill>`;
  }

  // --- Modals ---

  private _renderEnvModal(): TemplateResult {
    if (!this.prompt || this.prompt.step !== "env") return html``;
    const p = this.prompt;
    return html`
      <div style="position:fixed; inset:0; background:rgba(0,0,0,0.55);
                  display:flex; align-items:center; justify-content:center;
                  z-index:100; --wails-draggable:no-drag;">
        <div style="background:var(--bg-2); border:1px solid rgba(255,255,255,0.08);
                    border-radius:12px; padding:24px 28px; width:440px;
                    display:flex; flex-direction:column; gap:16px;">
          <div style="font-size:14px; font-weight:600; color:var(--fg-0);">
            ${this.t.envTitle} · ${p.entry.display}
          </div>
          ${p.fields.map((f, i) => html`
            <div style="display:flex; flex-direction:column; gap:4px;">
              <label style="font-size:11px; color:var(--fg-2);
                            font-family:var(--font-mono);">${f.prompt}</label>
              <input
                .type=${f.type === "secret" ? "password" : "text"}
                .value=${f.value}
                @input=${(e: Event) => {
                  if (!this.prompt) return;
                  const fields = [...this.prompt.fields];
                  fields[i] = { ...fields[i], value: (e.target as HTMLInputElement).value };
                  this.prompt = { ...this.prompt, fields };
                }}
                style="padding:7px 10px; font-family:var(--font-mono); font-size:12px;
                       background:rgba(0,0,0,0.30); border:1px solid rgba(255,255,255,0.08);
                       border-radius:6px; color:var(--fg-0); --wails-draggable:no-drag;">
            </div>
          `)}
          <div style="display:flex; gap:8px; justify-content:flex-end; margin-top:4px;">
            <lthn-btn tone="ghost" size="sm"
              @click=${() => { this.prompt = null; }}>
              ${this.t.envCancel}
            </lthn-btn>
            <lthn-btn tone="primary" size="sm"
              @click=${async () => {
                if (!this.prompt) return;
                const p = this.prompt;
                if (p.perms.length > 0) {
                  this.prompt = { ...p, step: "perms" };
                } else {
                  const env: Record<string, string> = {};
                  for (const f of p.fields) env[f.key] = f.value;
                  const manR = await FetchManifest(p.entry.sourceURL);
                  if (manR.OK) await this._doInstall(p.entry, manR.Value as object, env);
                }
              }}>
              ${this.t.envNext}
            </lthn-btn>
          </div>
        </div>
      </div>
    `;
  }

  private _renderPermModal(): TemplateResult {
    if (!this.prompt || this.prompt.step !== "perms") return html``;
    const p = this.prompt;
    // Views + session-token surfaces per
    // plans/code/lthn/desktop/views/RFC.plugin-views.md §9.2 + §5.5.
    // viewSummary returns null when no views are declared — the html
    // template renders nothing in that branch.
    const viewSummary = renderViewSummary({ Plugin: { Views: p.views } });
    const sessionPrompt = p.wantsSession
      ? renderSessionTokenPrompt(p.entry.display || p.entry.name)
      : null;
    return html`
      <div style="position:fixed; inset:0; background:rgba(0,0,0,0.55);
                  display:flex; align-items:center; justify-content:center;
                  z-index:100; --wails-draggable:no-drag;">
        <div style="background:var(--bg-2); border:1px solid rgba(255,255,255,0.08);
                    border-radius:12px; padding:24px 28px; width:440px;
                    display:flex; flex-direction:column; gap:14px;
                    max-height:80vh; overflow-y:auto;">
          <div style="font-size:14px; font-weight:600; color:var(--fg-0);">
            ${p.entry.display} · ${p.perms.length > 0 ? this.t.permTitle : "About this plugin"}
          </div>
          <!--
            Mantis #1643 / Cerberus DREAD #32 F8 — tier-4 unsigned-publisher
            warning. All bundles are unsigned in v1 (signing lands v2 per
            plans/code/lthn/desktop/marketplace/RFC.signing.md §5.6 Q8); the
            banner renders unconditionally today and gates on tier-4 once
            signature-tier metadata reaches this surface. Wording per the
            ticket's Recommendation — replaces the "self-signed" framing
            (which carried unearned TLS-land trust connotation) with
            "UNVERIFIED — anyone could have written this" + explainer line.
          -->
          <div role="alert" aria-label="Unverified publisher warning"
               style="display:flex; gap:10px; align-items:flex-start;
                      padding:10px 12px; border-radius:8px;
                      background:rgba(220,53,69,0.12);
                      border:1px solid rgba(220,53,69,0.40);">
            <i class="fa-solid fa-triangle-exclamation"
               style="font-size:14px; color:#f56565; margin-top:2px;"></i>
            <div>
              <div style="font-size:12px; font-weight:600; color:#f56565;
                          font-family:var(--font-mono); letter-spacing:0.2px;">
                UNVERIFIED — anyone could have written this
              </div>
              <div style="font-size:11px; color:var(--fg-2); margin-top:4px;
                          line-height:1.5;">
                lthn-desktop cannot verify who wrote this bundle.
                Install only if you trust the source you downloaded it from.
              </div>
            </div>
          </div>
          ${viewSummary}
          ${sessionPrompt}
          <div style="display:flex; flex-direction:column; gap:8px;">
            ${p.perms.map(perm => html`
              <div style="display:flex; gap:10px; align-items:flex-start;
                          padding:10px 12px; border-radius:8px;
                          background:rgba(0,0,0,0.18);
                          border:1px solid rgba(255,255,255,0.05);">
                <i class="fa-solid ${perm.mode === 'write' ? 'fa-pen' : perm.mode === 'invoke' ? 'fa-bolt' : 'fa-eye'}"
                   style="font-size:12px; color:var(--brand-300); margin-top:2px;"></i>
                <div>
                  <div style="font-size:12px; color:var(--fg-0);">${perm.reason}</div>
                  <div style="font-size:10px; color:var(--fg-3);
                              font-family:var(--font-mono); margin-top:2px;">
                    ${perm.scope} · ${perm.mode}
                  </div>
                </div>
              </div>
            `)}
          </div>
          <div style="display:flex; gap:8px; justify-content:flex-end; margin-top:4px;">
            <lthn-btn tone="ghost" size="sm"
              @click=${() => { this.prompt = null; }}>
              ${this.t.permCancel}
            </lthn-btn>
            <lthn-btn tone="primary" size="sm"
              @click=${async () => {
                if (!this.prompt) return;
                const p = this.prompt;
                const env: Record<string, string> = {};
                for (const f of p.fields) env[f.key] = f.value;
                const manR = await FetchManifest(p.entry.sourceURL);
                if (manR.OK) await this._doInstall(p.entry, manR.Value as object, env);
              }}>
              ${this.t.permApprove}
            </lthn-btn>
          </div>
        </div>
      </div>
    `;
  }

  private _renderInstallingModal(): TemplateResult {
    if (!this.prompt || this.prompt.step !== "installing") return html``;
    return html`
      <div style="position:fixed; inset:0; background:rgba(0,0,0,0.55);
                  display:flex; align-items:center; justify-content:center;
                  z-index:100;">
        <div style="background:var(--bg-2); border:1px solid rgba(255,255,255,0.08);
                    border-radius:12px; padding:32px 40px; display:flex;
                    flex-direction:column; gap:12px; align-items:center;">
          <i class="fa-solid fa-spinner fa-spin"
             style="font-size:24px; color:var(--brand-300);"></i>
          <div style="font-size:13px; color:var(--fg-1);">
            ${this.t.installing} ${this.prompt.entry.display}…
          </div>
        </div>
      </div>
    `;
  }

  // --- Panes ---

  private _renderBrowsePane(): TemplateResult {
    const cats = categories(this.catalogue.entries);
    const visible = filterCatalogue(this.catalogue.entries, this.query, this.category);
    const chipStyle = (active: boolean) => `
      padding:3px 10px; border-radius:4px; border:none; cursor:pointer;
      font-family:var(--font-mono); font-size:10.5px;
      background:${active ? "rgba(255,255,255,0.08)" : "transparent"};
      color:${active ? "var(--fg-0)" : "var(--fg-3)"};
      --wails-draggable:no-drag;
    `;

    return html`
      <div style="display:flex; gap:8px; margin-bottom:14px; align-items:center; flex-wrap:wrap;">
        <input type="text" .value=${this.query}
          @input=${(e: Event) => { this.query = (e.target as HTMLInputElement).value; }}
          placeholder="${this.t.searchPlaceholder}"
          style="flex:1; min-width:160px; padding:6px 10px;
                 font-family:var(--font-mono); font-size:11px;
                 background:rgba(0,0,0,0.30); border:1px solid rgba(255,255,255,0.06);
                 border-radius:6px; color:var(--fg-0); --wails-draggable:no-drag;">
        <div style="display:inline-flex; border-radius:6px; padding:2px;
                    background:rgba(0,0,0,0.18); border:1px solid rgba(255,255,255,0.06);
                    gap:2px; flex-wrap:wrap;">
          <button @click=${() => { this.category = ""; }}
            style=${chipStyle(this.category === "")}>${this.t.catAll}</button>
          ${cats.map(c => html`
            <button @click=${() => { this.category = c; }}
              style=${chipStyle(this.category === c)}>
              ${categoryLabel(c)}
            </button>
          `)}
        </div>
      </div>

      <div style="display:grid;
                  grid-template-columns:repeat(auto-fill, minmax(300px, 1fr));
                  gap:10px; overflow-y:auto;">
        ${visible.map(entry => this._renderCatalogueCard(entry))}
        ${visible.length === 0 ? html`
          <div style="padding:40px; text-align:center; color:var(--fg-3);
                      font-size:12px; grid-column:1 / -1;">
            ${this.t.empty}
          </div>
        ` : nothing}
      </div>
    `;
  }

  private _renderCatalogueCard(entry: CatalogueEntry): TemplateResult {
    const installed = this._isInstalled(entry.name);
    return html`
      <div style="background:rgba(0,0,0,0.18); border-radius:10px; padding:14px 16px;
                  display:flex; flex-direction:column; gap:10px;
                  border:1px solid rgba(255,255,255,0.05);">
        <div style="display:flex; align-items:center; gap:10px;">
          <div style="width:36px; height:36px; flex-shrink:0;
                      display:flex; align-items:center; justify-content:center;
                      background:rgba(64,193,197,0.08); border-radius:8px;">
            <i class="fa-solid ${entry.icon || "fa-box"}"
               style="font-size:15px; color:var(--brand-300);"></i>
          </div>
          <div style="flex:1; min-width:0;">
            <div style="font-size:13px; font-weight:600; color:var(--fg-0);
                        white-space:nowrap; overflow:hidden;
                        text-overflow:ellipsis;">${entry.display || entry.name}</div>
            <code style="font-size:10px; color:var(--fg-3);
                         font-family:var(--font-mono);">${entry.name}</code>
          </div>
          ${entry.category ? html`
            <span style="font-size:10px; padding:2px 8px; border-radius:999px;
                         background:rgba(122,95,196,0.14); color:var(--brand-300);
                         font-family:var(--font-mono); flex-shrink:0;">
              ${categoryLabel(entry.category)}
            </span>
          ` : nothing}
        </div>

        ${entry.description ? html`
          <p style="font-size:11px; color:var(--fg-2); margin:0; line-height:1.5;
                    display:-webkit-box; -webkit-line-clamp:3;
                    -webkit-box-orient:vertical; overflow:hidden;">
            ${entry.description}
          </p>
        ` : nothing}

        <div style="display:flex; align-items:center; gap:6px; margin-top:auto;">
          ${installed
            ? html`<lthn-state-pill variant="ok">installed</lthn-state-pill>`
            : nothing}
          <div style="flex:1;"></div>
          ${installed
            ? html`
              <lthn-btn tone="ghost" size="sm"
                @click=${() => void this._stop(entry.name)}>
                ${this.t.stop}
              </lthn-btn>
              <lthn-btn tone="ghost" size="sm"
                @click=${() => void this._uninstall(entry.name)}>
                ${this.t.uninstall}
              </lthn-btn>
            `
            : html`
              <lthn-btn tone="primary" size="sm"
                @click=${() => void this._beginInstall(entry)}>
                <i class="fa-solid fa-arrow-down" style="font-size:10px;"></i>
                ${this.t.install}
              </lthn-btn>
            `}
        </div>
      </div>
    `;
  }

  private _renderInstalledPane(): TemplateResult {
    if (this.installed.length === 0) return html`
      <div style="padding:40px; text-align:center; color:var(--fg-3); font-size:12px;">
        ${this.t.emptyInstalled}
      </div>
    `;

    return html`
      <div style="display:flex; flex-direction:column; gap:6px;">
        ${this.installed.map(b => html`
          <div style="background:rgba(0,0,0,0.18); border-radius:10px;
                      padding:12px 16px; display:flex; gap:14px; align-items:center;
                      border:1px solid rgba(255,255,255,0.05);">
            <div style="flex:1; min-width:0;">
              <div style="font-size:13px; font-weight:600; color:var(--fg-0);">
                ${b.Display || b.BundleID}
              </div>
              <code style="font-size:10px; color:var(--fg-3);
                           font-family:var(--font-mono);">${b.BundleID}</code>
            </div>
            ${this._statusPill(b.Status)}
            <div style="display:flex; gap:6px;">
              ${b.Status === "stopped" || b.Status === "idle" ? html`
                <lthn-btn tone="primary" size="sm"
                  @click=${() => void this._start(b.BundleID)}>
                  ${this.t.start}
                </lthn-btn>
              ` : nothing}
              ${b.Status === "running" ? html`
                <lthn-btn tone="ghost" size="sm"
                  @click=${() => void this._stop(b.BundleID)}>
                  ${this.t.stop}
                </lthn-btn>
              ` : nothing}
              <lthn-btn tone="ghost" size="sm"
                @click=${() => void this._uninstall(b.BundleID)}>
                ${this.t.uninstall}
              </lthn-btn>
            </div>
          </div>
        `)}
      </div>
    `;
  }

  render() {
    const tabStyle = (active: boolean) => `
      padding:4px 12px; border-radius:4px; border:none; cursor:pointer;
      font-family:var(--font-mono); font-size:11px;
      background:${active ? "rgba(255,255,255,0.08)" : "transparent"};
      color:${active ? "var(--fg-0)" : "var(--fg-3)"};
      --wails-draggable:no-drag;
    `;

    const toolbar = html`
      <div style="display:inline-flex; border-radius:6px; padding:2px;
                  background:rgba(0,0,0,0.18);
                  border:1px solid rgba(255,255,255,0.06); gap:2px;">
        <button @click=${() => { this.tab = "browse"; }}
          style=${tabStyle(this.tab === "browse")}>
          ${this.t.tabBrowse} · ${this.catalogue.entries.length}
        </button>
        <button @click=${() => { this.tab = "installed"; }}
          style=${tabStyle(this.tab === "installed")}>
          ${this.t.tabInstalled} · ${this.installed.length}
        </button>
      </div>
      <div style="flex:1;"></div>
      ${this.catalogue.fromCache ? html`
        <span style="font-size:10px; color:var(--fg-3);
                     font-family:var(--font-mono);">
          ${this.t.fromCache}
        </span>
      ` : nothing}
      <lthn-btn tone="primary" size="sm" ?dim=${this.loading}
        @click=${() => void this._refresh()}>
        <i class="fa-solid ${this.loading ? "fa-spinner fa-spin" : "fa-rotate"}"
           style="font-size:10px;"></i>
        ${this.loading ? this.t.btnRefreshing : this.t.btnRefresh}
      </lthn-btn>
    `;

    const banner = (this.info || this.err) ? html`
      <div style="padding:10px 14px; margin-bottom:14px;
                  color:${this.err ? "var(--err-400)" : "var(--brand-300)"};
                  background:${this.err
                    ? "rgba(255,76,76,0.06)"
                    : "rgba(122,95,196,0.06)"};
                  border:1px solid ${this.err
                    ? "rgba(255,76,76,0.18)"
                    : "rgba(122,95,196,0.18)"};
                  border-radius:6px; font-size:12px; line-height:1.55;">
        ${this.err || this.info}
      </div>
    ` : nothing;

    const body = html`
      <div style="flex:1; min-height:0; overflow:auto; padding:16px 18px;">
        ${banner}
        ${this.tab === "browse" ? this._renderBrowsePane() : this._renderInstalledPane()}
      </div>
    `;

    const footer = html`
      ${this.t.footerCatalogued.replace("%d", String(this.catalogue.entries.length))} ·
      ${this.t.footerInstalled.replace("%d", String(this.installed.length))}
    `;

    return html`
      ${this._renderEnvModal()}
      ${this._renderPermModal()}
      ${this._renderInstallingModal()}
      ${renderChrome({
        title: this.chrome.title, subtitle: this.chrome.subtitle,
        w: this.w, h: this.h, toolbar, body, footer,
        embedded: this.embedded,
      })}
    `;
  }
}
customElements.define("lthn-marketplace-window", LthnMarketplaceWindow);

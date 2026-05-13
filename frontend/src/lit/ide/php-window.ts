// SPDX-Licence-Identifier: EUPL-1.2
// IDE · php — <lthn-php-window>
//
// Laravel project discovery + composer/artisan runner. Left rail
// lists detected projects under $HOME/Code/{lab,core,host-uk}.
// Click a project to load its detail (services + .env state +
// scripts grid). Click a script card to dispatch via process.Service.
//
// Open via windowservice.Open("php"). No ?path= needed — the
// scan walks the canonical Code/* roots.

import { LitElement, html, nothing } from "lit";
import { renderChrome } from "../chrome";
import { T } from "@lthn/i18n/coreservice";

interface ProjectSummary {
  path: string;
  name: string;
  app_name: string;
  app_url: string;
  package_mgr: string;
  frankenphp: boolean;
}

interface ProjectDetail extends ProjectSummary {
  domain: string;
  services: string[];
  has_env: boolean;
  has_env_example: boolean;
  has_vendor: boolean;
  has_composer_lock: boolean;
  has_node_modules: boolean;
  has_package_lock: boolean;
}

interface ScriptEntry {
  name: string;
  command: string;
  lines?: number;
  source: string;
  artisan_args?: string[];
}

interface Scripts {
  composer_scripts: ScriptEntry[];
  artisan_scripts: ScriptEntry[];
  has_artisan: boolean;
  has_composer: boolean;
}

class LthnPhpWindow extends LitElement {
  static properties = {
    w:           { type: Number },
    h:           { type: Number },
    embedded:    { type: Boolean, reflect: true },
    chrome:      { state: true },
    projects:    { state: true },
    selected:    { state: true },
    detail:      { state: true },
    scripts:     { state: true },
    loading:     { state: true },
    err:         { state: true },
  };
  declare w: number;
  declare h: number;
  declare embedded: boolean;
  declare chrome: { title: string; subtitle: string };
  declare projects: ProjectSummary[];
  declare selected: string;
  declare detail: ProjectDetail | null;
  declare scripts: Scripts | null;
  declare loading: boolean;
  declare err: string;

  constructor() {
    super();
    this.w = 1200; this.h = 780; this.embedded = false;
    this.chrome = { title: "PHP", subtitle: "laravel · composer · artisan" };
    this.projects = [];
    this.selected = "";
    this.detail = null;
    this.scripts = null;
    this.loading = false;
    this.err = "";
  }
  createRenderRoot() { return this; }

  async connectedCallback() {
    super.connectedCallback();
    const [title, subtitle] = await Promise.all([
      T("window.php.title"),
      T("window.php.subtitle"),
    ]);
    this.chrome = { title, subtitle };
    await this._refresh();
  }

  async _refresh() {
    if (this.loading) return;
    this.loading = true;
    this.err = "";
    try {
      const svc = await import("@desktop/php/service");
      const r = await svc.Detect([], 3) as { projects: ProjectSummary[] };
      this.projects = r.projects || [];
      if (!this.selected && this.projects.length > 0) {
        await this._select(this.projects[0].path);
      } else if (this.selected && !this.projects.some(p => p.path === this.selected)) {
        this.selected = "";
        this.detail = null;
        this.scripts = null;
      }
    } catch (e: unknown) {
      this.err = e instanceof Error ? e.message : String(e);
    } finally {
      this.loading = false;
    }
  }

  async _select(path: string) {
    this.selected = path;
    this.detail = null;
    this.scripts = null;
    try {
      const svc = await import("@desktop/php/service");
      const [d, s] = await Promise.all([
        svc.Project(path) as Promise<{ detail: ProjectDetail }>,
        svc.Scripts(path) as Promise<Scripts>,
      ]);
      this.detail = d.detail;
      this.scripts = s;
    } catch (e: unknown) {
      this.err = e instanceof Error ? e.message : String(e);
    }
  }

  async _runScript(mode: "composer" | "artisan", entry: ScriptEntry) {
    if (!this.detail) return;
    try {
      const svc = await import("@desktop/php/service");
      await svc.Run({
        path: this.detail.path,
        mode,
        name: mode === "composer" ? entry.name : "",
        args: mode === "artisan" ? (entry.artisan_args || []) : [],
        command: "",
      });
    } catch (e: unknown) {
      this.err = e instanceof Error ? e.message : String(e);
    }
  }

  private renderToolbar() {
    return html`
      <span style="font-family:var(--font-mono); font-size:11px; color:var(--fg-3);
                   padding:0 6px;">
        ${this.projects.length} project${this.projects.length === 1 ? "" : "s"}
      </span>
      <div style="flex:1"></div>
      <lthn-btn tone="primary" size="sm" ?dim=${this.loading}
        @click=${() => void this._refresh()}>
        <i class="fa-solid ${this.loading ? "fa-spinner" : "fa-rotate"}"
           style="font-size:10px;"></i>
        ${this.loading ? "Scanning…" : "Re-scan"}
      </lthn-btn>
    `;
  }

  private projectButtonStyle(path: string) {
    const selected = this.selected === path;
    const background = selected ? "rgba(122, 95, 196, 0.18)" : "transparent";
    const border = selected ? "rgba(122, 95, 196, 0.45)" : "transparent";
    return `display:flex; flex-direction:column; gap:2px;
            align-items:flex-start; width:100%; padding:8px 10px;
            background:${background};
            border:1px solid ${border};
            border-radius:5px; cursor:pointer; text-align:left;
            margin-bottom:4px; font:inherit; color:var(--fg-0);
            --wails-draggable: no-drag;`;
  }

  private renderFrankenPhpBadge(project: ProjectSummary) {
    if (!project.frankenphp) return nothing;
    return html`
      <span style="font-size:9px; padding:1px 6px; border-radius:3px;
                   background:rgba(167, 139, 250, 0.18);
                   color:#a78bfa; text-transform:uppercase;
                   letter-spacing:0.04em;">FrankenPHP</span>
    `;
  }

  private renderProjectButton(project: ProjectSummary) {
    return html`
      <button @click=${() => void this._select(project.path)}
        style=${this.projectButtonStyle(project.path)}>
        <span style="font-size:12px; font-weight:600; display:flex;
                     align-items:center; gap:6px;">
          ${project.name}
          ${this.renderFrankenPhpBadge(project)}
        </span>
        <span style="font-family:var(--font-mono); font-size:10px;
                     color:var(--fg-3); overflow:hidden;
                     text-overflow:ellipsis; white-space:nowrap;
                     max-width:100%;">${project.app_url || project.path}</span>
      </button>
    `;
  }

  private renderEmptyProjects() {
    if (this.projects.length > 0 || this.loading) return nothing;
    return html`
      <div style="padding:20px 10px; text-align:center; color:var(--fg-3);
                  font-size:11px; font-style:italic;">
        no Laravel projects found
      </div>
    `;
  }

  private renderSidebar() {
    return html`
      <div style="width:260px; min-width:260px; border-right:1px solid rgba(255,255,255,0.06);
                  padding:14px 12px; overflow-y:auto;
                  background:rgba(0,0,0,0.18);">
        <h3 style="font-size:10px; text-transform:uppercase;
                   letter-spacing:0.06em; color:var(--fg-3);
                   margin:0 0 8px;">Projects</h3>
        ${this.projects.map(project => this.renderProjectButton(project))}
        ${this.renderEmptyProjects()}
      </div>
    `;
  }

  private renderCell(label: string, value: unknown) {
    return html`
      <div style="display:flex; flex-direction:column; gap:2px;
                  padding:8px 10px; background:rgba(0,0,0,0.18);
                  border:1px solid rgba(255,255,255,0.05);
                  border-radius:4px;">
        <span style="font-size:10px; text-transform:uppercase;
                     letter-spacing:0.06em; color:var(--fg-3);">${label}</span>
        <span style="font-size:12px; color:var(--fg-0);">${value || "—"}</span>
      </div>
    `;
  }

  private stateTone(present: boolean, warn: boolean) {
    if (present) {
      return {
        background: "rgba(52, 211, 153, 0.12)",
        color: "#34d399",
        marker: "✓",
      };
    }
    if (warn) {
      return {
        background: "rgba(251, 191, 36, 0.12)",
        color: "#fbbf24",
        marker: "⚠",
      };
    }
    return {
      background: "rgba(255,255,255,0.05)",
      color: "var(--fg-3)",
      marker: "—",
    };
  }

  private renderState(label: string, present: boolean, warn = false) {
    const tone = this.stateTone(present, warn);
    return html`
      <span style="font-size:10px; font-family:var(--font-mono);
                   padding:3px 8px; border-radius:4px;
                   background:${tone.background};
                   color:${tone.color};">
        ${label} ${tone.marker}
      </span>
    `;
  }

  private renderError() {
    if (!this.err) return nothing;
    return html`
      <div style="padding:10px 14px; color:var(--err-400);
                  background:rgba(255,76,76,0.06);
                  border:1px solid rgba(255,76,76,0.18);
                  border-radius:6px; font-size:12px;
                  line-height:1.55; margin-bottom:14px;">
        ${this.err}
      </div>
    `;
  }

  private renderServices(detail: ProjectDetail) {
    return html`
      <h4 style="font-size:11px; text-transform:uppercase;
                 letter-spacing:0.06em; color:var(--fg-3);
                 margin:18px 0 8px;">Detected services</h4>
      <div style="display:flex; flex-wrap:wrap; gap:6px;">
        ${detail.services.map(service => html`
          <span style="font-size:11px; padding:3px 9px;
                       background:rgba(122, 95, 196, 0.14);
                       color:var(--brand-300); border-radius:999px;
                       font-family:var(--font-mono);">${service}</span>
        `)}
        ${detail.services.length === 0 ? html`
          <span style="color:var(--fg-3); font-style:italic;
                       font-size:11px;">none</span>
        ` : nothing}
      </div>
    `;
  }

  private renderProjectState(detail: ProjectDetail) {
    return html`
      <h4 style="font-size:11px; text-transform:uppercase;
                 letter-spacing:0.06em; color:var(--fg-3);
                 margin:18px 0 8px;">Project state</h4>
      <div style="display:flex; flex-wrap:wrap; gap:6px;">
        ${this.renderState(".env", detail.has_env, !detail.has_env)}
        ${this.renderState("env.example", detail.has_env_example)}
        ${this.renderState("vendor/", detail.has_vendor, !detail.has_vendor)}
        ${this.renderState("composer.lock", detail.has_composer_lock)}
        ${this.renderState("node_modules/", detail.has_node_modules)}
        ${this.renderState("package-lock", detail.has_package_lock)}
      </div>
    `;
  }

  private renderExtraLineCount(script: ScriptEntry) {
    const lines = script.lines || 0;
    if (lines <= 1) return nothing;
    return html`
      <span style="position:absolute; top:6px; right:8px;
                   font-size:9px; color:var(--brand-300);
                   font-family:var(--font-mono);">+${lines - 1}</span>
    `;
  }

  private renderScriptButton(mode: "composer" | "artisan", script: ScriptEntry) {
    return html`
      <button @click=${() => void this._runScript(mode, script)}
        title=${script.command}
        style="background:rgba(0,0,0,0.18);
               border:1px solid rgba(255,255,255,0.05);
               border-radius:6px; padding:8px 10px;
               text-align:left; cursor:pointer; color:var(--fg-2);
               font:inherit; display:flex; flex-direction:column;
               gap:3px; position:relative;
               --wails-draggable: no-drag;">
        <span style="font-size:12px; font-weight:600;
                     color:var(--fg-0);
                     font-family:var(--font-mono);">${script.name}</span>
        ${mode === "composer" ? this.renderExtraLineCount(script) : nothing}
        <span style="font-size:10px; color:var(--fg-3);
                     font-family:var(--font-mono);
                     white-space:nowrap; overflow:hidden;
                     text-overflow:ellipsis;">${script.command}</span>
      </button>
    `;
  }

  private renderScriptSection(
    title: string,
    countLabel: string,
    mode: "composer" | "artisan",
    entries: ScriptEntry[],
  ) {
    if (entries.length === 0) return nothing;
    return html`
      <h4 style="font-size:11px; text-transform:uppercase;
                 letter-spacing:0.06em; color:var(--fg-3);
                 margin:18px 0 8px;">
        ${title}
        <span style="color:var(--fg-3); font-weight:normal;
                     font-size:11px;">(${countLabel})</span>
      </h4>
      <div style="display:grid;
                  grid-template-columns:repeat(auto-fill, minmax(180px, 1fr));
                  gap:8px;">
        ${entries.map(script => this.renderScriptButton(mode, script))}
      </div>
    `;
  }

  private renderScripts(detail: ProjectDetail) {
    if (!this.scripts) return nothing;
    return html`
      ${this.renderScriptSection(
        "Composer scripts",
        String(this.scripts.composer_scripts.length),
        "composer",
        this.scripts.composer_scripts,
      )}

      ${this.renderScriptSection(
        "Artisan",
        `${this.scripts.artisan_scripts.length} canonical`,
        "artisan",
        this.scripts.artisan_scripts,
      )}

      <div style="font-size:11px; color:var(--fg-3);
                  font-style:italic; margin-top:14px;">
        click a script to dispatch via process.Service in
        <code>${detail.path}</code>
      </div>
    `;
  }

  private renderDetail(detail: ProjectDetail) {
    return html`
      <h3 style="font-size:16px; color:var(--fg-0); margin:0 0 4px;">
        ${detail.name}</h3>
      <code style="font-family:var(--font-mono); font-size:11px;
                   color:var(--fg-3); display:block; margin-bottom:14px;">
        ${detail.path}</code>

      <div style="display:grid; grid-template-columns:1fr 1fr;
                  gap:8px 18px; margin-bottom:14px;">
        ${this.renderCell("app name", detail.app_name)}
        ${this.renderCell("app URL", html`<code>${detail.app_url}</code>`)}
        ${this.renderCell("domain", html`<code>${detail.domain}</code>`)}
        ${this.renderCell("package mgr", html`<code>${detail.package_mgr}</code>`)}
      </div>

      ${this.renderServices(detail)}
      ${this.renderProjectState(detail)}
      ${this.renderScripts(detail)}
    `;
  }

  private renderEmptyDetail() {
    return html`
      <div style="padding:40px; text-align:center; color:var(--fg-3);
                  font-size:12px;">
        ${this.loading ? "Scanning…" : "select a project from the rail"}
      </div>
    `;
  }

  private renderMain() {
    return html`
      <div style="flex:1; padding:16px 20px; overflow-y:auto; min-width:0;">
        ${this.renderError()}
        ${this.detail ? this.renderDetail(this.detail) : this.renderEmptyDetail()}
      </div>
    `;
  }

  private renderBody() {
    return html`
      <div style="flex:1; display:flex; min-height:0; min-width:0;">
        ${this.renderSidebar()}
        ${this.renderMain()}
      </div>
    `;
  }

  private renderFooter() {
    if (!this.detail) return `${this.projects.length} projects`;
    return `${this.detail.path} · ${this.detail.services.length} services`;
  }

  render() {
    return renderChrome({
      title: this.chrome.title, subtitle: this.chrome.subtitle,
      w: this.w, h: this.h, toolbar: this.renderToolbar(),
      body: this.renderBody(),
      footer: html`${this.renderFooter()}`,
      embedded: this.embedded,
    });
  }
}
customElements.define("lthn-php-window", LthnPhpWindow);

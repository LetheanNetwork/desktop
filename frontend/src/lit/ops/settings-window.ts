// SPDX-Licence-Identifier: EUPL-1.2
// E1.2 · settings — <lthn-settings-window>
// Light-DOM Lit element. Composes renderChrome() from ../chrome.js.

import { LitElement, html, nothing } from "lit";
import { renderChrome } from "../chrome";
import { T } from "@lthn/i18n/coreservice";
import type { LitContent } from "../types";

/** Shape returned by runner.WRoutes() — mirrored here so the
 *  settings shell doesn't force a module-graph dependency on the
 *  bindings model at type-check time. Kept aligned with the Go
 *  RouteView. */
interface RouteView {
  name:     string;
  kind:     string;
  base_url: string;
  model:    string;
}

class LthnSettingsWindow extends LitElement {
  static properties = {
    open: { type: String, reflect: true },
    w:    { type: Number },
    h:    { type: Number },
    embedded: { type: Boolean, reflect: true },
    chrome: { state: true },
    locales: { state: true },
    currentLang: { state: true },
    startWithWindow: { state: true },
    modelsDir: { state: true },
    routeNames: { state: true },
    build: { state: true },
    sampleInterval: { state: true },
    heapSamples: { state: true },
    routes: { state: true },
  };
  declare open: string;
  declare w: number;
  declare h: number;
  declare embedded: boolean;
  declare chrome: { title: string; subtitle: string };
  declare locales: string[];
  declare currentLang: string;
  declare startWithWindow: boolean;
  declare modelsDir: string;
  declare routeNames: string[];
  declare build: { version: string; go_version: string; goos: string; goarch: string; num_cpu: number };
  declare sampleInterval: string;
  declare heapSamples: string;
  declare routes: RouteView[];
  constructor() {
    super();
    this.open = "general"; this.w = 760; this.h = 600; this.embedded = false;
    this.chrome = { title: "Settings", subtitle: "lthn · v0.2.0-rc1" };
    this.locales = [];
    this.currentLang = "";
    // Persisted via localStorage["lthn.boot.window"] — Go-side
    // application boot will read this to decide whether to spawn the
    // unified app shell at launch, or leave the binary tray-only.
    this.startWithWindow = localStorage.getItem("lthn.boot.window") === "true";
    this.modelsDir = "~/Lethean/conf/models/";
    this.routeNames = [];
    this.build = { version: "0.1.0", go_version: "", goos: "", goarch: "", num_cpu: 0 };
    // Telemetry poll cadence + sparkline window. Persisted via
    // localStorage so the tray + telemetry-window read the same
    // values at their connectedCallback.
    this.sampleInterval = localStorage.getItem("lthn.telemetry.interval") || "2s";
    this.heapSamples    = localStorage.getItem("lthn.telemetry.samples")  || "24";
    this.routes = [];
  }

  _setSampleInterval(v: string) {
    this.sampleInterval = v;
    localStorage.setItem("lthn.telemetry.interval", v);
  }

  _setHeapSamples(v: string) {
    this.heapSamples = v;
    localStorage.setItem("lthn.telemetry.samples", v);
  }

  /** Interactive segment — same chip shape as _segment but each
   *  option fires onPick. Used by the telemetry sample-interval +
   *  heap-window pickers so settings actually persists. */
  _segmentPick(value: string, options: string[], onPick: (v: string) => void) {
    return html`
      <div style="display:inline-flex; border-radius:6px;
                  background:rgba(0,0,0,0.18); border:1px solid rgba(255,255,255,0.06); padding:2px;">
        ${options.map(o => html`
          <button @click=${() => onPick(o)}
            style="padding:4px 10px; font-family:var(--font-mono); font-size:10.5px;
                   border:none; cursor:pointer;
                   color:${o === value ? "var(--fg-0)" : "var(--fg-3)"};
                   background:${o === value ? "rgba(255,255,255,0.08)" : "transparent"};
                   border-radius:4px; letter-spacing:0.02em;
                   --wails-draggable: no-drag;">${o}</button>
        `)}
      </div>
    `;
  }
  createRenderRoot() { return this; }
  async connectedCallback() {
    super.connectedCallback();
    const [i18n, fl, runner] = await Promise.all([
      import("@lthn/i18n/coreservice"),
      import("@desktop/firstlaunch/wailsservice"),
      import("@desktop/runner/service"),
    ]);
    const [title, subtitleTpl, locales, currentLang, paths, routes, routeViews, build] = await Promise.all([
      i18n.T("window.settings.title"),
      i18n.T("window.settings.subtitle"),
      i18n.AvailableLanguages(),
      i18n.Language(),
      fl.Paths().catch(() => null),
      runner.WModels().catch((): string[] => []),
      runner.WRoutes().catch((): RouteView[] => []),
      fl.Build().catch(() => null),
    ]);
    this.locales = locales;
    this.currentLang = currentLang;
    if (paths?.models_dir) {
      this.modelsDir = collapseHome(paths.models_dir);
    }
    this.routeNames = routes || [];
    this.routes = (routeViews || []) as RouteView[];
    if (build) {
      this.build = build;
    }
    // Rebuild the subtitle with the real version so the chrome
    // reflects the running binary rather than the locale fixture.
    const subtitle = this.build.version
      ? `lthn · v${this.build.version}`
      : subtitleTpl;
    this.chrome = { title, subtitle };
  }

  /** Side-menu click handler — swap the body to the selected section. */
  _openSection(id: string) {
    this.open = id;
  }

  /** Picks the section body to render based on this.open. Single
   *  section visible at a time; the side rail is the only chrome
   *  carrying which one is active. */
  _activeSection() {
    switch (this.open) {
      case "general":      return this._sectionGeneral();
      case "models":       return this._sectionModels();
      case "runner":       return this._sectionRunner();
      case "api":          return this._sectionApi();
      case "telemetry":    return this._sectionTelemetry();
      case "integrations": return this._sectionIntegrations();
      case "about":        return this._sectionAbout();
      default:             return this._sectionGeneral();
    }
  }

  /** Locale picker — flag + tag label. Click writes localStorage + calls SetLanguage. */
  async _setLang(lang: string) {
    const i18n = await import("@lthn/i18n/coreservice");
    await i18n.SetLanguage(lang);
    localStorage.setItem("lthn.locale", lang);
    this.currentLang = lang;
  }

  /** Boot mode toggle — write-through to localStorage. */
  _setStartWithWindow(on: boolean) {
    this.startWithWindow = on;
    localStorage.setItem("lthn.boot.window", on ? "true" : "false");
  }

  /** Flag for a given locale tag — keeps the picker visually intuitive. */
  _flag(lang: string): string {
    const l = lang.toLowerCase();
    if (l === "fr" || l.startsWith("fr-") || l.startsWith("fr_")) return "🇫🇷";
    if (l === "en-au" || l === "en_au") return "🇦🇺";
    return "🇬🇧";
  }

  render() {
    const sections = [
      { id:"general",      icon:"fa-sliders",      label:"General" },
      { id:"models",       icon:"fa-cube",         label:"Models" },
      { id:"runner",       icon:"fa-gauge-high",   label:"Runner" },
      { id:"api",          icon:"fa-plug",         label:"API" },
      { id:"telemetry",    icon:"fa-wave-square",  label:"Telemetry" },
      { id:"integrations", icon:"fa-link",         label:"Integrations" },
      { id:"about",        icon:"fa-circle-info",  label:"About" },
    ];

    const body = html`
      <div style="flex:1; display:grid; grid-template-columns: 200px 1fr; min-height:0;">
        <!-- section rail -->
        <aside style="background:rgba(0,0,0,0.18); border-right:1px solid rgba(255,255,255,0.05);
                      padding:12px 8px; display:flex; flex-direction:column; gap:1px;">
          ${sections.map(s => html`
            <div
              @click=${() => this._openSection(s.id)}
              style="padding:8px 12px; border-radius:6px;
                     background:${s.id === this.open ? "rgba(255,255,255,0.07)" : "transparent"};
                     display:flex; align-items:center; gap:10px;
                     font-size:12.5px;
                     color:${s.id === this.open ? "var(--fg-0)" : "var(--fg-2)"};
                     cursor:pointer;
                     --wails-draggable: no-drag;">
              <i class="fa-solid ${s.icon}" style="font-size:11px; width:14px; text-align:center;
                  color:${s.id === this.open ? "var(--brand-300)" : "var(--fg-3)"};"></i>
              ${s.label}
            </div>
          `)}
        </aside>

        <!-- body — single section at a time, picked by this.open -->
        <main id="settings-body" style="padding:28px 32px; overflow:auto; display:flex; flex-direction:column; gap:22px;">
          ${this._activeSection()}
        </main>
      </div>
    `;

    return renderChrome({
      title: this.chrome.title,
      subtitle: this.chrome.subtitle,
      w: this.w, h: this.h, body,
      footer: html`Changes apply immediately · ⌘W to close · the runner keeps running`,
      embedded: this.embedded,
    });
  }

  _section({ title, desc, content }: { title: string; desc?: string; content: LitContent }) {
    return html`
      <div style="display:flex; flex-direction:column; gap:14px;">
        <div style="display:flex; align-items:center; gap:8px;">
          <div style="font-size:14.5px; font-weight:600; color:var(--fg-0); letter-spacing:-0.015em;">${title}</div>
        </div>
        ${desc ? html`<div style="font-size:11.5px; color:var(--fg-3); line-height:1.55;">${desc}</div>` : nothing}
        <div style="display:flex; flex-direction:column; gap:14px;
                    padding:8px 0; border-top:1px solid rgba(255,255,255,0.05);">
          ${content}
        </div>
      </div>
    `;
  }

  _row(label: string, hint: string | null, control: LitContent) {
    return html`
      <div style="display:grid; grid-template-columns: 200px 1fr; gap:18px; align-items:flex-start; padding-top:8px;">
        <div style="display:flex; flex-direction:column; gap:3px;">
          <div style="font-size:12.5px; color:var(--fg-1); font-weight:500;">${label}</div>
          ${hint ? html`<div style="font-size:10.5px; color:var(--fg-3); line-height:1.5;">${hint}</div>` : nothing}
        </div>
        <div>${control}</div>
      </div>
    `;
  }

  _select(value: string) {
    return html`
      <div style="display:inline-flex; align-items:center; gap:8px; padding:6px 10px; border-radius:6px;
                  background:rgba(255,255,255,0.04); border:1px solid rgba(255,255,255,0.07);
                  font-size:11.5px; color:var(--fg-1);">
        ${value}
        <i class="fa-solid fa-angle-down" style="font-size:9px; color:var(--fg-3);"></i>
      </div>
    `;
  }

  _segment(value: string, options: string[]) {
    return html`
      <div style="display:inline-flex; border-radius:6px;
                  background:rgba(0,0,0,0.18); border:1px solid rgba(255,255,255,0.06); padding:2px;">
        ${options.map(o => html`
          <div style="padding:4px 10px; font-family:var(--font-mono); font-size:10.5px;
                      color:${o === value ? "var(--fg-0)" : "var(--fg-3)"};
                      background:${o === value ? "rgba(255,255,255,0.08)" : "transparent"};
                      border-radius:4px; letter-spacing:0.02em;">${o}</div>
        `)}
      </div>
    `;
  }

  _sectionGeneral() {
    return this._section({
      title: "General",
      desc: "App-wide defaults — what to show at launch, which language to speak, theme.",
      content: html`
        ${this._row("Start with window", "If on, the unified app shell opens at launch alongside the tray. Off = tray-only until you click in.", html`
          <lthn-toggle ?on=${this.startWithWindow}
            @click=${() => this._setStartWithWindow(!this.startWithWindow)}>
          </lthn-toggle>
        `)}
        ${this._row("Default language", "Sets the language for the WebView surfaces. Stored locally; persists across restarts.", html`
          <div style="display:inline-flex; border-radius:6px;
                      background:rgba(0,0,0,0.18); border:1px solid rgba(255,255,255,0.06); padding:2px; gap:2px;">
            ${this.locales.map(l => html`
              <button
                @click=${() => this._setLang(l)}
                style="padding:4px 10px; border-radius:4px; border:none; cursor:pointer;
                       background:${l === this.currentLang ? "rgba(255,255,255,0.08)" : "transparent"};
                       color:${l === this.currentLang ? "var(--fg-0)" : "var(--fg-3)"};
                       font-family:var(--font-mono); font-size:10.5px;
                       --wails-draggable: no-drag;
                       display:inline-flex; align-items:center; gap:6px;">
                <span style="font-size:13px; line-height:1;">${this._flag(l)}</span>
                ${l}
              </button>
            `)}
          </div>
        `)}
      `,
    });
  }

  _sectionModels() {
    const defaultModel = this.routeNames.length > 0 ? this.routeNames[0] : "no routes configured";
    return this._section({
      title: "Models",
      desc: "Where lthn looks for models and which one loads at startup.",
      content: html`
        ${this._row("Model directory", "Canonical Lethean layout — visible in Finder, safe to inspect.", html`
          <div style="display:flex; align-items:center; gap:8px; padding:6px 10px; border-radius:6px;
                      background:rgba(255,255,255,0.04); border:1px solid rgba(255,255,255,0.07);
                      font-family:var(--font-mono); font-size:11.5px; color:var(--fg-1);">
            <i class="fa-regular fa-folder" style="font-size:11px; color:var(--fg-3);"></i>
            ${this.modelsDir}
            <lthn-btn tone="quiet" size="sm" style="margin-left:4px;"
              @click=${() => import("@desktop/desktop/windowservice").then(w => w.Open("models"))}>
              Browse…
            </lthn-btn>
          </div>
        `)}
        ${this._row("Default model",
          this.routeNames.length > 0
            ? `Configured runner route — picked first when the runner starts.`
            : "No routes configured. Add one via lthn config routes.NAME.kind=…",
          this._select(defaultModel))}
        ${this._row("Quantisation preference", "Pick the smallest quant your hardware comfortably runs. Applied when the runner loads a fresh model.",
          this._segment("q4_k_m", ["q4_0", "q4_k_m", "q5_k_m", "q8_0"]))}
        ${this._row("Default sampling", "Per-model overrides live in the model browser.", html`
          <div style="display:flex; gap:18px; font-size:11.5px; color:var(--fg-2);">
            <span>Temp <span style="color:var(--fg-0); font-family:var(--font-mono);">0.7</span></span>
            <span>Top-p <span style="color:var(--fg-0); font-family:var(--font-mono);">0.95</span></span>
            <span>Max tok <span style="color:var(--fg-0); font-family:var(--font-mono);">1024</span></span>
          </div>
        `)}
      `,
    });
  }

  _sectionRunner() {
    return this._section({
      title: "Runner",
      desc: "Provider routes the runner serves. Read-only; edit ~/Lethean/conf/lthn.yaml to add or change a route, then restart lthn.",
      content: this.routes.length === 0 ? html`
        <div style="padding:14px 16px; border-radius:8px; background:rgba(255,255,255,0.025);
                    border:1px solid rgba(255,255,255,0.05); font-size:12px; color:var(--fg-3); line-height:1.55;">
          No routes configured. The runner falls back to an echo stub
          — useful for sanity checks but nothing real will answer.
        </div>
      ` : html`
        <div style="display:flex; flex-direction:column; gap:8px;">
          ${this.routes.map(r => html`
            <div style="padding:12px 14px; border-radius:8px; background:rgba(255,255,255,0.025);
                        border:1px solid rgba(255,255,255,0.06); display:flex; flex-direction:column; gap:6px;">
              <div style="display:flex; align-items:baseline; gap:8px;">
                <span style="font-family:var(--font-mono); font-size:13px; color:var(--fg-0); font-weight:500;">
                  ${r.name}
                </span>
                <span style="font-size:10.5px; color:var(--fg-3); letter-spacing:0.04em; text-transform:uppercase;">
                  · ${r.kind}
                </span>
              </div>
              <div style="display:grid; grid-template-columns:auto 1fr; gap:4px 12px;
                          font-family:var(--font-mono); font-size:11px; color:var(--fg-2);">
                <span style="color:var(--fg-3);">model</span>
                <span>${r.model || "—"}</span>
                <span style="color:var(--fg-3);">endpoint</span>
                <span>${r.base_url || "—"}</span>
              </div>
            </div>
          `)}
        </div>
      `,
    });
  }

  _sectionApi() {
    return this._section({
      title: "API",
      desc: "HTTP server for OpenAI-compatible clients. Off by default; nothing leaves this Mac unless you turn it on.",
      content: html`
        ${this._row("HTTP server", null, html`<lthn-toggle on></lthn-toggle>`)}
        ${this._row("Endpoint", null, html`
          <span style="font-family:var(--font-mono); font-size:11.5px; color:var(--fg-1);">http://localhost:8000/v1</span>
        `)}
        ${this._row("API key", "Required for any client connecting to the local server.", html`
          <div style="display:flex; align-items:center; gap:6px;">
            <span style="font-family:var(--font-mono); font-size:11px; color:var(--fg-1);">sk-lthn-••••••••••••••••2qB7</span>
            <lthn-btn tone="quiet" size="sm"><i class="fa-regular fa-copy" style="font-size:10px;"></i></lthn-btn>
            <lthn-btn tone="quiet" size="sm"><i class="fa-solid fa-rotate-right" style="font-size:10px;"></i></lthn-btn>
          </div>
        `)}
      `,
    });
  }

  _sectionTelemetry() {
    return this._section({
      title: "Telemetry",
      desc: "Process metrics shown in the tray + live-telemetry window. Local only; nothing leaves the device.",
      content: html`
        ${this._row("Sample interval", "How often the tray polls the runner. 2s is the calm-presence default.",
          this._segmentPick(this.sampleInterval, ["1s", "2s", "5s", "off"], v => this._setSampleInterval(v)))}
        ${this._row("Heap samples", "Rolling window the sparkline draws from.",
          this._segmentPick(this.heapSamples, ["12", "24", "48", "96"], v => this._setHeapSamples(v)))}
        ${this._row("Power metrics", "Requires the XPC helper (planned). Off today.", html`
          <lthn-toggle></lthn-toggle>
        `)}
      `,
    });
  }

  _sectionIntegrations() {
    return this._section({
      title: "Integrations",
      desc: "Connected clients reading from the local lthn API. Full surface lives in the dedicated Integrations window.",
      content: html`
        ${this._row("Open Integrations window", "Manage Claude Code / OpenCode / Codex / Copilot / Raycast wiring.", html`
          <lthn-btn tone="quiet" size="sm"
            @click=${() => import("@desktop/desktop/windowservice").then(w => w.Open("integrations"))}>
            <i class="fa-solid fa-arrow-up-right-from-square" style="font-size:10px;"></i> Open
          </lthn-btn>
        `)}
      `,
    });
  }

  _sectionAbout() {
    const mono = (v: string) => html`<span style="font-family:var(--font-mono); font-size:11.5px; color:var(--fg-1);">${v}</span>`;
    return this._section({
      title: "About",
      desc: "What's running — version, runtime, and where the source lives.",
      content: html`
        ${this._row("Version", "Binary release tag baked at build time.",
          mono(`v${this.build.version || "—"}`))}
        ${this._row("Go toolchain", null, mono(this.build.go_version || "—"))}
        ${this._row("Platform", null, mono(this.build.goos && this.build.goarch ? `${this.build.goos} · ${this.build.goarch}` : "—"))}
        ${this._row("CPUs", "Logical cores Go's runtime sees.", mono(String(this.build.num_cpu || "—")))}
        ${this._row("Licence", null, mono("EUPL-1.2"))}
        ${this._row("Source", null, html`
          <a href="https://github.com/LetheanNetwork/desktop" target="_blank" rel="noopener"
             style="font-family:var(--font-mono); font-size:11.5px; color:var(--brand-300);
                    text-decoration:none; --wails-draggable: no-drag;">
            github.com/LetheanNetwork/desktop
          </a>
        `)}
        ${this._row("Project", null, html`
          <a href="https://lthn.ai" target="_blank" rel="noopener"
             style="font-family:var(--font-mono); font-size:11.5px; color:var(--brand-300);
                    text-decoration:none; --wails-draggable: no-drag;">
            lthn.ai
          </a>
        `)}
      `,
    });
  }
}
customElements.define("lthn-settings-window", LthnSettingsWindow);

/** Same shape as welcome-window's displayHome — collapses the
 *  leading $HOME segment into "~/" for compact display. */
function collapseHome(absPath: string): string {
  if (!absPath) return absPath;
  const m = absPath.match(/^\/(Users|home)\/[^/]+\//);
  return m ? "~/" + absPath.slice(m[0].length) : absPath;
}

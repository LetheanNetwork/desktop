/* lit-app-shell.js — frameless application shell for Lethean Desktop
 *
 * Wraps all window components in a single application chrome:
 *
 *   ┌──────────────────────────────────────────────────────────────┐
 *   │ 🟥🟡🟢  lthn  Chat              ⌘K  · search · settings · vi │  ← titlebar (drag region)
 *   ├──────┬───────────────────────────────────────────────────────┤
 *   │      │                                                       │
 *   │  ●   │                                                       │
 *   │  💬  │            <slot> — active window body                │
 *   │  📊  │                                                       │
 *   │  🔌  │                                                       │
 *   │  🧩  │                                                       │
 *   │  ⚙   │                                                       │
 *   │      │                                                       │
 *   ├──────┴───────────────────────────────────────────────────────┤
 *   │  ●  gemma-4-e2b · 47.2 t/s · 8.4 W              v0.2.0-rc1   │  ← status bar
 *   └──────────────────────────────────────────────────────────────┘
 *
 * Frameless: the chrome IS the window. No native titlebar, all drag from the
 * titlebar slot. Wails v3 uses the CSS custom property
 *   --wails-draggable: drag        on draggable surfaces
 *   --wails-draggable: no-drag     on click-targets within them
 * (NOT the legacy -webkit-app-region — Wails3 doesn't honour it).
 *
 *   <lthn-app-shell active="chat">
 *     <lthn-chat-window slot="body" state="multi-turn"></lthn-chat-window>
 *   </lthn-app-shell>
 *
 * Or let the shell pick a window by `active` (chat | benchmark | logs | telemetry
 * | integrations | tools | network | distillation | fleet | settings | models):
 *
 *   <lthn-app-shell active="benchmark"></lthn-app-shell>
 *
 * Properties:
 *   active        which item is selected (string id)
 *   collapsed     side nav is icon-only (boolean)
 *   running       model is generating (animates the status pulse)
 *   model         current model name shown in status bar
 *   tps           current tok/s
 *   watts         current watts
 *   version       version string in status bar (default v0.2.0-rc1)
 */

import { LitElement, html, nothing } from "lit";
import { T } from "@lthn/i18n/coreservice";

/* Side-nav item registry — id, label, icon, which window element to render */
const NAV: NavEntry[] = [
  { id: "chat",          label: "Chat",          icon: "fa-comments",         tag: "lthn-chat-window",          group: "primary" },
  { id: "models",        label: "Models",        icon: "fa-cube",             tag: "lthn-model-browser-window", group: "primary" },
  { id: "benchmark",     label: "Benchmark",     icon: "fa-gauge-high",       tag: "lthn-benchmark-window",     group: "observe" },
  { id: "logs",          label: "Activity",      icon: "fa-wave-square",      tag: "lthn-logs-window",          group: "observe" },
  { id: "telemetry",     label: "Telemetry",     icon: "fa-bolt",             tag: "lthn-telemetry-window",     group: "observe" },
  { id: "integrations",  label: "Integrations",  icon: "fa-link",             tag: "lthn-integrations-window",  group: "extend" },
  { id: "tools",         label: "Tools · MCP",   icon: "fa-screwdriver-wrench", tag: "lthn-tools-window",       group: "extend" },
  { id: "network",       label: "Network",       icon: "fa-circle-nodes",     tag: "lthn-network-window",       group: "preview", preview: true },
  { id: "distillation",  label: "Fine-tune",     icon: "fa-flask",            tag: "lthn-distillation-window",  group: "preview", preview: true },
  { id: "fleet",         label: "Fleet",         icon: "fa-server",           tag: "lthn-fleet-window",         group: "preview", preview: true },
  { id: "settings",      label: "Settings",      icon: "fa-sliders",          tag: "lthn-settings-window",      group: "bottom" },
];

interface NavEntry {
  id:      string;
  label:   string;
  icon:    string;
  tag:     string;
  group:   "primary" | "observe" | "extend" | "preview" | "bottom";
  preview?: boolean;
}

class LthnAppShell extends LitElement {
  static properties = {
    active:    { type: String,  reflect: true },
    collapsed: { type: Boolean, reflect: true },
    running:   { type: Boolean, reflect: true },
    model:     { type: String },
    tps:       { type: String },
    watts:     { type: String },
    version:   { type: String },
    t:         { state: true },
  };
  declare active:    string;
  declare collapsed: boolean;
  declare running:   boolean;
  declare model:     string;
  declare tps:       string;
  declare watts:     string;
  declare version:   string;
  declare t: {
    brand: string;
    search: string;
    settingsTip: string;
    preview: string;
    expand: string;
    collapse: string;
    group: Record<string, string>;
    nav: Record<string, string>;
  };
  constructor() {
    super();
    this.active = "chat";
    this.collapsed = false;
    this.running = true;
    this.model = "gemma-4-e2b";
    this.tps = "47.2";
    this.watts = "8.4";
    this.version = "v0.2.0-rc1";
    // English fallback so the first render isn't blank while
    // connectedCallback resolves T() from the binding.
    this.t = {
      brand: "lthn",
      search: "Search",
      settingsTip: "Settings",
      preview: "PREVIEW",
      expand: "Expand",
      collapse: "Collapse",
      group: { primary: "Workspace", observe: "Observe", extend: "Extend", preview: "Preview" },
      nav: {
        chat: "Chat", models: "Models", benchmark: "Benchmark", logs: "Activity",
        telemetry: "Telemetry", integrations: "Integrations", tools: "Tools · MCP",
        network: "Network", distillation: "Fine-tune", fleet: "Fleet", settings: "Settings",
      },
    };
  }
  createRenderRoot() { return this; }
  async connectedCallback() {
    super.connectedCallback();
    const [
      brand, search, settingsTip, preview, expand, collapse,
      gPrimary, gObserve, gExtend, gPreview,
      nChat, nModels, nBenchmark, nLogs, nTelemetry, nIntegrations, nTools, nNetwork, nDistillation, nFleet, nSettings,
    ] = await Promise.all([
      T("shell.brand"), T("shell.search"), T("shell.settings_tooltip"),
      T("shell.preview_tag"), T("shell.expand"), T("shell.collapse"),
      T("shell.group.primary"), T("shell.group.observe"), T("shell.group.extend"), T("shell.group.preview"),
      T("shell.nav.chat"), T("shell.nav.models"), T("shell.nav.benchmark"), T("shell.nav.logs"),
      T("shell.nav.telemetry"), T("shell.nav.integrations"), T("shell.nav.tools"),
      T("shell.nav.network"), T("shell.nav.distillation"), T("shell.nav.fleet"), T("shell.nav.settings"),
    ]);
    this.t = {
      brand, search, settingsTip, preview, expand, collapse,
      group:  { primary: gPrimary, observe: gObserve, extend: gExtend, preview: gPreview },
      nav: {
        chat: nChat, models: nModels, benchmark: nBenchmark, logs: nLogs,
        telemetry: nTelemetry, integrations: nIntegrations, tools: nTools,
        network: nNetwork, distillation: nDistillation, fleet: nFleet, settings: nSettings,
      },
    };

    // Subscribe to "lthn:app:setpane" — the tray panel's Open
    // buttons emit this with a NAV id (chat / models / telemetry /
    // …) after spawning the app window. This lets the popover steer
    // which pane the unified shell lands on without baking it into
    // the window URL.
    const { Events } = await import("@wailsio/runtime");
    this._unsubSetPane = Events.On("lthn:app:setpane", (ev) => {
      const id = typeof ev?.data === "string" ? ev.data : null;
      if (id && NAV.some(n => n.id === id)) {
        this.active = id;
      }
    });

    // Status-bar live data — same sources chat-window, tray, and
    // Settings → About all bind against. Falls back to the design
    // literals so the bar reads coherently before bindings resolve
    // (canvas preview, slow boot).
    try {
      const [runner, fl] = await Promise.all([
        import("@desktop/runner/service"),
        import("@desktop/firstlaunch/wailsservice"),
      ]);
      const [models, build] = await Promise.all([
        runner.WModels().catch((): string[] => []),
        fl.Build().catch(() => null),
      ]);
      if (models && models[0]) {
        this.model = models[0];
        this.running = true;
      } else {
        this.running = false;
      }
      if (build?.version) this.version = `v${build.version}`;
    } catch { /* keep design fallbacks */ }
  }

  disconnectedCallback() {
    super.disconnectedCallback();
    if (this._unsubSetPane) {
      this._unsubSetPane();
      this._unsubSetPane = null;
    }
  }

  /** Returned by Events.On to detach the listener on element teardown. */
  private _unsubSetPane: (() => void) | null = null;

  _select(id: string) { this.active = id; }
  _toggleCollapse() { this.collapsed = !this.collapsed; }

  _renderNavGroup(group: NavEntry["group"], label: string | null) {
    const items = NAV.filter(n => n.group === group);
    if (!items.length) return nothing;
    return html`
      ${label && !this.collapsed ? html`
        <div style="padding:${this.collapsed ? "8px 0 4px" : "10px 18px 4px"}; font-family:var(--font-mono); font-size:9.5px; color:var(--fg-3); letter-spacing:0.08em; text-transform:uppercase;">${label}</div>
      ` : html`<div style="height:8px;"></div>`}
      ${items.map(n => {
        const active = n.id === this.active;
        const label = this.t.nav[n.id] ?? n.label;
        return html`
          <button
            @click=${() => this._select(n.id)}
            title=${this.collapsed ? label : ""}
            style="
              --wails-draggable: no-drag;
              display:flex; align-items:center; gap:12px;
              margin:1px 8px; padding:${this.collapsed ? "8px 0" : "8px 12px"};
              ${this.collapsed ? "justify-content:center;" : ""}
              border-radius:6px; cursor:pointer;
              background:${active ? "rgba(64,193,197,0.10)" : "transparent"};
              border:1px solid ${active ? "rgba(64,193,197,0.22)" : "transparent"};
              color:${active ? "var(--brand-300)" : "var(--fg-2)"};
              font-family:var(--font-sans); font-size:12.5px; font-weight:${active ? 500 : 400};
              text-align:left; width:calc(100% - 16px);
            "
          >
            <i class="fa-solid ${n.icon}" style="font-size:13px; width:14px; text-align:center; color:${active ? "var(--brand-300)" : "var(--fg-3)"};"></i>
            ${!this.collapsed ? html`
              <span style="flex:1;">${label}</span>
              ${n.preview ? html`<span style="font-family:var(--font-mono); font-size:8.5px; padding:1px 5px; border-radius:999px; background:rgba(245,158,11,0.10); border:1px solid rgba(245,158,11,0.22); color:var(--warning-400); letter-spacing:0.06em;">${this.t.preview}</span>` : nothing}
            ` : nothing}
          </button>
        `;
      })}
    `;
  }

  _renderBody() {
    const node = this.querySelector('[slot="body"]');
    if (node) return html`<slot name="body"></slot>`;
    const entry = NAV.find(n => n.id === this.active);
    if (!entry) return html`<div style="padding:40px; color:var(--fg-3);">No window for "${this.active}"</div>`;
    // Dynamically render the matching custom element. With embedded
    // mode (set in _instantiate), the child fills the body slot
    // 100% — no padding, no centring. Background gradient kept so the
    // body area still feels distinct from the side-nav rail.
    return html`<div style="flex:1; min-height:0; display:flex; overflow:hidden; background:radial-gradient(1200px 600px at 50% 30%, rgba(64,193,197,0.03), transparent 60%);">
      ${this._instantiate(entry.tag)}
    </div>`;
  }

  _instantiate(tag: string) {
    // Use unsafeHTML-free path: create element imperatively, lit-html will mount it
    const el = document.createElement(tag);
    // Two-shell pattern: mark every child as embedded so renderChrome
    // skips its standalone card chrome and fills our body slot. The
    // shell already paints the titlebar / side-nav / status bar.
    // See memory design_two_shell_pattern.md.
    el.setAttribute("embedded", "");
    // Pass through some sensible defaults so the windows look populated
    if (tag === "lthn-chat-window") el.setAttribute("state", "multi-turn");
    if (tag === "lthn-logs-window") el.setAttribute("tab", "live");
    if (tag === "lthn-welcome-window") el.setAttribute("step", "2");
    if (tag === "lthn-settings-window") el.setAttribute("open", "models");
    return el;
  }

  render() {
    const navWidth = this.collapsed ? 56 : 220;
    const active = NAV.find(n => n.id === this.active);
    return html`
      <div style="
        position:fixed; inset:0;
        display:grid;
        grid-template-rows: 38px 1fr 28px;
        grid-template-columns: ${navWidth}px 1fr;
        background:
          radial-gradient(1200px 800px at 20% -10%, rgba(64,193,197,0.06), transparent 60%),
          radial-gradient(900px 600px at 90% 110%, rgba(64,193,197,0.04), transparent 60%),
          linear-gradient(180deg, #0f0e14 0%, #0a090e 100%);
        color: var(--fg-1);
        font-family: var(--font-sans);
        overflow: hidden;
        /* Default-drag the whole shell — interactive children
           (side-nav buttons, traffic-lights, Search) opt out. */
        --wails-draggable: drag;
      ">
        <!-- TITLEBAR (drag region) -->
        <header style="
          grid-column: 1 / -1; grid-row: 1;
          display:flex; align-items:center; gap:14px;
          padding:0 14px;
          background: rgba(255,255,255,0.02);
          border-bottom: 1px solid rgba(255,255,255,0.05);
          --wails-draggable: drag;
          user-select: none;
        ">
          <!-- Real traffic-lights — wired to Wails Window API. See
               chrome.ts <lthn-traffic-lights> for Close/Minimise/Fullscreen
               click handlers. -->
          <lthn-traffic-lights></lthn-traffic-lights>
          <div style="display:flex; align-items:center; gap:8px;">
            <lthn-glyph size="14" color="var(--fg-1)" ?active=${this.running}></lthn-glyph>
            <span style="font-family:var(--font-mono); font-size:12px; color:var(--fg-0); letter-spacing:-0.005em; font-weight:600;">${this.t.brand}</span>
            <span style="font-family:var(--font-mono); font-size:11px; color:var(--fg-3);">·</span>
            <span style="font-size:12.5px; color:var(--fg-1); font-weight:500;">${active ? (this.t.nav[active.id] ?? active.label) : ""}</span>
          </div>
          <div style="flex:1"></div>
          <div style="--wails-draggable: no-drag; display:flex; align-items:center; gap:6px;">
            <button style="display:inline-flex; align-items:center; gap:6px; padding:4px 10px; border-radius:6px; background:rgba(255,255,255,0.04); border:1px solid rgba(255,255,255,0.07); color:var(--fg-2); font-family:var(--font-sans); font-size:11.5px; cursor:pointer;">
              <i class="fa-solid fa-magnifying-glass" style="font-size:10px;"></i>
              <span>${this.t.search}</span>
              <span style="font-family:var(--font-mono); font-size:9.5px; padding:0 4px; border-left:1px solid rgba(255,255,255,0.10); margin-left:4px; padding-left:8px; color:var(--fg-3);">⌘K</span>
            </button>
            <button @click=${() => this._select("settings")} title=${this.t.settingsTip} style="width:26px; height:26px; border-radius:6px; background:transparent; border:1px solid transparent; color:var(--fg-3); cursor:pointer;">
              <i class="fa-solid fa-sliders" style="font-size:11px;"></i>
            </button>
            <button title="Vi" style="width:26px; height:26px; border-radius:6px; background:rgba(64,193,197,0.10); border:1px solid rgba(64,193,197,0.22); color:var(--brand-300); cursor:pointer;">
              <i class="fa-solid fa-feather" style="font-size:11px;"></i>
            </button>
          </div>
        </header>

        <!-- SIDE NAV -->
        <aside style="
          grid-row: 2; grid-column: 1;
          display:flex; flex-direction:column;
          background: rgba(0,0,0,0.22);
          border-right: 1px solid rgba(255,255,255,0.05);
          overflow-y: auto; overflow-x: hidden;
          padding-bottom: 6px;
        ">
          <div style="padding:8px 8px 4px; display:flex; align-items:center; gap:8px; ${this.collapsed ? "justify-content:center;" : ""}">
            ${!this.collapsed ? html`
              <div style="flex:1; display:flex; align-items:center; gap:6px; padding:4px 6px; border-radius:6px; background:rgba(64,193,197,0.06); border:1px solid rgba(64,193,197,0.18);">
                <lthn-status-dot variant=${this.running ? "ok" : "idle"} ?pulse=${this.running}></lthn-status-dot>
                <span style="font-family:var(--font-mono); font-size:10.5px; color:var(--fg-1); white-space:nowrap; overflow:hidden; text-overflow:ellipsis;">${this.model}</span>
              </div>
            ` : html`<lthn-status-dot variant=${this.running ? "ok" : "idle"} ?pulse=${this.running}></lthn-status-dot>`}
            <button @click=${this._toggleCollapse} title=${this.collapsed ? this.t.expand : this.t.collapse} style="width:22px; height:22px; border-radius:5px; background:transparent; border:1px solid rgba(255,255,255,0.07); color:var(--fg-3); cursor:pointer; display:flex; align-items:center; justify-content:center;">
              <i class="fa-solid ${this.collapsed ? "fa-angles-right" : "fa-angles-left"}" style="font-size:9px;"></i>
            </button>
          </div>

          ${this._renderNavGroup("primary",  this.t.group.primary)}
          ${this._renderNavGroup("observe",  this.t.group.observe)}
          ${this._renderNavGroup("extend",   this.t.group.extend)}
          ${this._renderNavGroup("preview",  this.t.group.preview)}
          <div style="flex:1"></div>
          ${this._renderNavGroup("bottom",   null)}
        </aside>

        <!-- BODY -->
        <main style="grid-row: 2; grid-column: 2; display:flex; flex-direction:column; min-height:0; overflow:hidden;">
          ${this._renderBody()}
        </main>

        <!-- STATUS BAR -->
        <footer style="
          grid-column: 1 / -1; grid-row: 3;
          display:flex; align-items:center; gap:14px;
          padding:0 14px;
          background: rgba(0,0,0,0.32);
          border-top: 1px solid rgba(255,255,255,0.05);
          font-family: var(--font-mono); font-size:10.5px;
          color: var(--fg-3); letter-spacing:0.02em;
        ">
          <div style="display:flex; align-items:center; gap:6px;">
            <lthn-status-dot variant=${this.running ? "ok" : "idle"} ?pulse=${this.running}></lthn-status-dot>
            <span style="color:var(--fg-1);">${this.model}</span>
          </div>
          <span>·</span>
          <span><span style="color:var(--fg-1);">${this.tps}</span> t/s</span>
          <span>·</span>
          <span><span style="color:var(--fg-1);">${this.watts}</span> W</span>
          <span>·</span>
          <span>airplane-mode OK</span>
          <div style="flex:1"></div>
          <span>${active ? active.label.toLowerCase() : ""}</span>
          <span>·</span>
          <span>${this.version}</span>
        </footer>
      </div>
    `;
  }
}
customElements.define("lthn-app-shell", LthnAppShell);

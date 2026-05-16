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
import { ref as litRef } from "lit/directives/ref.js";
import { T } from "@lthn/i18n/coreservice";

/** Filter a list of file paths down to those ending in `.gguf`
 *  (case-insensitive). Drives the drag-and-drop import wire — Wails
 *  fires lthn:window:files-dropped with every dragged file regardless
 *  of extension, so the app-shell filters here before dispatching to
 *  Models.Import. Pure function so unit tests can assert without
 *  spinning up the Events bus. */
export function filterGgufPaths(files: readonly string[] | null | undefined): string[] {
  if (!files) return [];
  return files.filter(p => typeof p === "string" && p.toLowerCase().endsWith(".gguf"));
}

/** Custom event name dispatched after a successful drag-imported
 *  Models.Import. Any mounted lthn-model-browser-window subscribes
 *  via Events.On to re-fetch its local rail. Defined as a constant
 *  so emitter + listener can't drift apart. */
export const MODELS_REFRESH_EVENT = "lthn:models:refresh";

/** Filter NAV entries by a free-text query against label + id.
 *  Pure — drives the ⌘P command palette without mounting the shell.
 *  Empty / whitespace query returns the full list unchanged. */
export function filterPaletteEntries<T extends { id: string; label: string }>(
  nav: readonly T[],
  query: string | null | undefined,
): T[] {
  const q = (query || "").trim().toLowerCase();
  if (!q) return nav.slice();
  return nav.filter(n =>
    n.label.toLowerCase().includes(q) || n.id.toLowerCase().includes(q),
  );
}

/** A side-nav entry — one mountable surface in the active view. */
interface NavEntry {
  id:      string;
  label:   string;
  icon:    string;
  tag:     string;
  group:   "primary" | "observe" | "extend" | "preview" | "bottom";
  preview?: boolean;
}

/** A view = a role-flavoured grouping of nav entries. The shell shows
 *  one view at a time; the switcher at the top of the side-nav swaps
 *  between them. See plans/ops/lthn/desktop/HANDOVER-VIEWS.md +
 *  Lethean-7 design canon. */
interface ViewDef {
  id:    string;
  label: string;
  icon:  string;
  blurb: string;
  nav:   NavEntry[];
}

/** View registry — six designed views per HANDOVER-VIEWS.md plus the
 *  default Admin view, which preserves the original flat-nav window
 *  catalogue verbatim (zero regression on what shipped before).
 *
 *  Phase 1: Admin is fully wired; planning/coding/marketing/operations/
 *  sales/office are scaffolded with placeholder elements. Each window
 *  inside a non-Admin view renders via <lthn-view-placeholder> until
 *  someone registers a custom element matching the entry's `tag`.
 *
 *  Phase 2 (separate dispatch): adopt core/ide pkg surfaces —
 *    Planning   → core/ide pkg/tasks (already in lthn via #359)
 *    Coding     → core/ide pkg/workspace + pkg/subagent + pkg/navigate
 *    Operations → core/ide pkg/marketplace + pkg/brain + pkg/vi
 *  Marketing / Sales / Office stay green-field. */
const VIEWS: ViewDef[] = [
  {
    id: "admin", label: "Admin", icon: "fa-gauge",
    blurb: "Run the runtime. Talk to models. Watch the metrics.",
    nav: [
      { id: "chat",          label: "Chat",          icon: "fa-comments",         tag: "lthn-chat-window",          group: "primary" },
      { id: "models",        label: "Models",        icon: "fa-cube",             tag: "lthn-model-browser-window", group: "primary" },
      { id: "benchmark",     label: "Benchmark",     icon: "fa-gauge-high",       tag: "lthn-benchmark-window",     group: "observe" },
      { id: "logs",          label: "Activity",      icon: "fa-wave-square",      tag: "lthn-logs-window",          group: "observe" },
      { id: "telemetry",     label: "Telemetry",     icon: "fa-bolt",             tag: "lthn-telemetry-window",     group: "observe" },
      { id: "processes",     label: "Processes",     icon: "fa-list-check",       tag: "lthn-process-window",       group: "observe" },
      { id: "integrations",  label: "Integrations",  icon: "fa-link",             tag: "lthn-integrations-window",  group: "extend" },
      { id: "tools",         label: "Tools · MCP",   icon: "fa-screwdriver-wrench", tag: "lthn-tools-window",       group: "extend" },
      { id: "providers",     label: "Providers",     icon: "fa-store",            tag: "lthn-providers-window",     group: "extend" },
      { id: "network",       label: "Network",       icon: "fa-circle-nodes",     tag: "lthn-network-window",       group: "preview", preview: true },
      { id: "distillation",  label: "Fine-tune",     icon: "fa-flask",            tag: "lthn-distillation-window",  group: "preview", preview: true },
      { id: "fleet",         label: "Fleet",         icon: "fa-server",           tag: "lthn-fleet-window",         group: "preview", preview: true },
      { id: "settings",      label: "Settings",      icon: "fa-sliders",          tag: "lthn-settings-window",      group: "bottom" },
    ],
  },
  {
    id: "planning", label: "Planning", icon: "fa-list-check",
    blurb: "Where work waits. Roadmap, backlog, sprints, retros.",
    nav: [
      { id: "today",    label: "Today",    icon: "fa-circle-dot",     tag: "lthn-view-today",    group: "primary" },
      { id: "sprints",  label: "Sprints",  icon: "fa-flag-checkered", tag: "lthn-view-sprints",  group: "primary" },
      { id: "backlog",  label: "Backlog",  icon: "fa-inbox",          tag: "lthn-view-backlog",  group: "primary" },
      { id: "roadmap",  label: "Roadmap",  icon: "fa-road",           tag: "lthn-view-roadmap",  group: "primary" },
      { id: "retros",   label: "Retros",   icon: "fa-rotate-left",    tag: "lthn-view-retros",   group: "primary" },
      { id: "calendar", label: "Calendar", icon: "fa-calendar",       tag: "lthn-view-calendar", group: "primary" },
      { id: "settings", label: "Settings", icon: "fa-sliders",        tag: "lthn-settings-window", group: "bottom" },
    ],
  },
  {
    id: "coding", label: "Coding", icon: "fa-code",
    blurb: "Write, ship, observe. Repos and chat in the same surface.",
    nav: [
      { id: "chat",     label: "Chat",     icon: "fa-comments",            tag: "lthn-chat-window",      group: "primary" },
      { id: "repos",    label: "Repos",    icon: "fa-folder-tree",         tag: "lthn-view-repos",       group: "primary" },
      { id: "issues",   label: "Issues",   icon: "fa-circle-exclamation",  tag: "lthn-view-issues",      group: "primary" },
      { id: "prs",      label: "PRs",      icon: "fa-code-pull-request",   tag: "lthn-view-prs",         group: "primary" },
      { id: "deploys",  label: "Deploys",  icon: "fa-rocket",              tag: "lthn-view-deploys",     group: "observe" },
      { id: "logs",     label: "Activity", icon: "fa-wave-square",         tag: "lthn-logs-window",      group: "observe" },
      { id: "settings", label: "Settings", icon: "fa-sliders",             tag: "lthn-settings-window",  group: "bottom" },
    ],
  },
  {
    id: "marketing", label: "Marketing", icon: "fa-bullhorn",
    blurb: "Campaigns, content, social, audience. Numbers your CEO asks about.",
    nav: [
      { id: "campaigns", label: "Campaigns", icon: "fa-bullhorn",    tag: "lthn-view-campaigns",  group: "primary" },
      { id: "content",   label: "Content",   icon: "fa-pen-nib",     tag: "lthn-view-content",    group: "primary" },
      { id: "social",    label: "Social",    icon: "fa-share-nodes", tag: "lthn-view-social",     group: "primary" },
      { id: "audience",  label: "Audience",  icon: "fa-users",       tag: "lthn-view-audience",   group: "primary" },
      { id: "analytics", label: "Analytics", icon: "fa-chart-line",  tag: "lthn-view-analytics",  group: "observe" },
      { id: "settings",  label: "Settings",  icon: "fa-sliders",     tag: "lthn-settings-window", group: "bottom" },
    ],
  },
  {
    id: "operations", label: "Operations", icon: "fa-shield-halved",
    blurb: "Uptime, incidents, runbooks. The boring stuff that matters.",
    nav: [
      { id: "status",    label: "Status",    icon: "fa-heart-pulse",            tag: "lthn-view-status",      group: "primary" },
      { id: "incidents", label: "Incidents", icon: "fa-triangle-exclamation",   tag: "lthn-view-incidents",   group: "primary" },
      { id: "runbooks",  label: "Runbooks",  icon: "fa-book",                   tag: "lthn-view-runbooks",    group: "primary" },
      { id: "logs",      label: "Activity",  icon: "fa-wave-square",            tag: "lthn-logs-window",      group: "observe" },
      { id: "telemetry", label: "Telemetry", icon: "fa-bolt",                   tag: "lthn-telemetry-window", group: "observe" },
      { id: "settings",  label: "Settings",  icon: "fa-sliders",                tag: "lthn-settings-window",  group: "bottom" },
    ],
  },
  {
    id: "sales", label: "Sales", icon: "fa-handshake",
    blurb: "Pipeline, contacts, deals, forecast. Honest numbers only.",
    nav: [
      { id: "pipeline", label: "Pipeline", icon: "fa-filter",       tag: "lthn-view-pipeline",   group: "primary" },
      { id: "contacts", label: "Contacts", icon: "fa-address-book", tag: "lthn-view-contacts",   group: "primary" },
      { id: "deals",    label: "Deals",    icon: "fa-handshake",    tag: "lthn-view-deals",      group: "primary" },
      { id: "forecast", label: "Forecast", icon: "fa-chart-column", tag: "lthn-view-forecast",   group: "observe" },
      { id: "settings", label: "Settings", icon: "fa-sliders",      tag: "lthn-settings-window", group: "bottom" },
    ],
  },
  {
    id: "office", label: "Office", icon: "fa-building",
    blurb: "Documents, mail, calendar, files. The Monday-morning surface.",
    nav: [
      { id: "documents", label: "Documents", icon: "fa-file-lines",  tag: "lthn-view-documents",  group: "primary" },
      { id: "mail",      label: "Mail",      icon: "fa-envelope",    tag: "lthn-view-mail",       group: "primary" },
      { id: "calendar",  label: "Calendar",  icon: "fa-calendar",    tag: "lthn-view-calendar",   group: "primary" },
      { id: "files",     label: "Files",     icon: "fa-folder-open", tag: "lthn-view-files",      group: "primary" },
      { id: "settings",  label: "Settings",  icon: "fa-sliders",     tag: "lthn-settings-window", group: "bottom" },
    ],
  },
];

/** ID → ViewDef lookup. Built once at module load. */
const VIEW_BY_ID: Record<string, ViewDef> = Object.fromEntries(
  VIEWS.map(v => [v.id, v]),
);

/** Resolve a view id to its NavEntry[]; falls back to admin on
 *  unknown id so a stale localStorage value can't render a blank rail. */
export function navForView(viewId: string | null | undefined): NavEntry[] {
  return (VIEW_BY_ID[viewId ?? ""] ?? VIEW_BY_ID.admin).nav;
}

/** Backward-compat shim. The shell's pre-VIEWS code (palette, pop-out,
 *  drag-drop, menu-behaviours) reads NAV directly; keep it pointing
 *  at the Admin view's nav so callers don't change. Phase 2 will
 *  walk these call sites and replace with `navForView(this.view)`. */
const NAV: NavEntry[] = VIEW_BY_ID.admin.nav;

class LthnAppShell extends LitElement {
  static readonly properties = {
    view:      { type: String,  reflect: true },
    active:    { type: String,  reflect: true },
    collapsed: { type: Boolean, reflect: true },
    running:   { type: Boolean, reflect: true },
    model:     { type: String },
    tps:       { type: String },
    watts:     { type: String },
    version:   { type: String },
    menuLinks:  { state: true },
    menuLayout: { state: true },
    paletteOpen:  { state: true },
    paletteQuery: { state: true },
    paletteIndex: { state: true },
    switcherOpen: { state: true },
    t:         { state: true },
  };
  /** Active view id — drives the side-nav contents + status-bar
   *  breadcrumb. Defaults to "admin" which preserves the original
   *  flat-nav shell behaviour. Persisted across launches. */
  declare view:      string;
  declare active:    string;
  declare collapsed: boolean;
  declare running:   boolean;
  declare model:     string;
  declare tps:       string;
  declare watts:     string;
  declare version:   string;
  /** Menu Behaviours — see plans/project/lthn/desktop/RFC.menu-behaviours.md.
   *  menuLinks  ∈ {"hybrid", "in-window", "collapsed-only"} — click dispatch
   *  menuLayout ∈ {"toggle", "open", "closed", "hover"}    — rail shape */
  declare menuLinks: string;
  declare menuLayout: string;
  /** ⌘P command palette: whether the overlay is open, the live
   *  filter query, and the selected index in the filtered list (-1
   *  before the first ArrowDown). State-only, never persisted. */
  declare paletteOpen: boolean;
  declare paletteQuery: string;
  declare paletteIndex: number;
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
  /** Whether the view-switcher dropdown is open. Pure UI state; never
   *  persisted. Closes on any click outside (document listener). */
  declare switcherOpen: boolean;

  /** localStorage key for the last-active view. Restored on mount so
   *  the user lands on whichever role surface they were last using.
   *  Defaults to "admin" if absent / unknown — preserves the
   *  pre-VIEWS flat-shell behaviour. */
  private static readonly ACTIVE_VIEW_KEY = "lthn.app.active-view";

  /** localStorage key prefix for the last-active pane PER VIEW. The
   *  full key is `lthn.app.active-pane:${viewId}` — each view
   *  remembers its own last pane so switching back and forth doesn't
   *  reset the user's place. Legacy `lthn.app.active-pane` (no
   *  suffix) seeds admin's first entry on first launch. */
  private static readonly ACTIVE_PANE_KEY = "lthn.app.active-pane";
  private static readonly ACTIVE_PANE_PREFIX = "lthn.app.active-pane:";

  /** localStorage keys for the two Menu Behaviours preferences. The
   *  Settings panel writes to these (see settings-window.ts
   *  _setMenuLinks / _setMenuLayout) and emits "lthn:menu:changed"
   *  so an open shell reacts without reload. */
  private static readonly MENU_LINKS_KEY  = "lthn.menu.links";
  private static readonly MENU_LAYOUT_KEY = "lthn.menu.layout";

  /** Pending mouseleave-debounce timer for Hover Open mode — gives
   *  the cursor 300ms grace to re-enter before the rail collapses. */
  private _hoverCollapseTimer: ReturnType<typeof setTimeout> | null = null;

  private static _read(key: string, fallback: string): string {
    try { return localStorage.getItem(key) || fallback; }
    catch { return fallback; }
  }

  constructor() {
    super();
    // Restore the last-active view + pane from localStorage. View
    // defaults to "admin" — same shape as the pre-VIEWS flat shell.
    // Per-view pane is keyed `lthn.app.active-pane:${viewId}`; the
    // legacy `lthn.app.active-pane` (no suffix) seeds admin's first
    // entry on first launch so existing users land where they were.
    const savedView = (() => {
      try { return localStorage.getItem(LthnAppShell.ACTIVE_VIEW_KEY); }
      catch { return null; }
    })();
    this.view = (savedView && VIEW_BY_ID[savedView]) ? savedView : "admin";

    const viewNav = navForView(this.view);
    const savedPane = (() => {
      try {
        return localStorage.getItem(LthnAppShell.ACTIVE_PANE_PREFIX + this.view)
            // legacy key — only honour for admin
            || (this.view === "admin"
                ? localStorage.getItem(LthnAppShell.ACTIVE_PANE_KEY)
                : null);
      }
      catch { return null; }
    })();
    this.active = (savedPane && viewNav.some(n => n.id === savedPane))
      ? savedPane
      : (viewNav[0]?.id ?? "chat");

    this.switcherOpen = false;
    this.collapsed = false;
    this.menuLinks  = LthnAppShell._read(LthnAppShell.MENU_LINKS_KEY,  "in-window");
    this.menuLayout = LthnAppShell._read(LthnAppShell.MENU_LAYOUT_KEY, "toggle");
    this._applyMenuLayout();
    this.paletteOpen  = false;
    this.paletteQuery = "";
    this.paletteIndex = -1;
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

  /** Window-level keydown — ⌘P / Ctrl+P toggles the command palette.
   *  Bound up-front so the shortcut works before async i18n lands. */
  private _onKeyDownForPalette = (ev: KeyboardEvent) => {
    const accel = ev.metaKey || ev.ctrlKey;
    if (!accel) return;
    if (ev.key.toLowerCase() !== "p") return;
    ev.preventDefault();
    this._togglePalette();
  };

  /** Window-level keydown — ⌘1..⌘7 (Ctrl+1..7 on non-mac) switch
   *  between the seven views (admin/planning/coding/marketing/
   *  operations/sales/office). Suppressed while a text input or
   *  contenteditable surface has focus so it doesn't steal from
   *  the chat composer / search inputs / cell editors. The WebView
   *  has no browser tabs so there's no platform-shortcut collision. */
  private _onKeyDownForViewSwitch = (ev: KeyboardEvent) => {
    const accel = ev.metaKey || ev.ctrlKey;
    if (!accel) return;
    if (ev.shiftKey || ev.altKey) return;
    const code = ev.key.charCodeAt(0);
    if (code < 49 || code > 55) return; // '1'..'7'
    const target = ev.target as HTMLElement | null;
    if (target) {
      const tag = target.tagName;
      if (tag === "INPUT" || tag === "TEXTAREA" || target.isContentEditable) return;
    }
    const idx = code - 49;
    const view = VIEWS[idx];
    if (!view) return;
    ev.preventDefault();
    this._selectView(view.id);
  };

  /** Window-level keydown — ⌘⇧[ / ⌘⇧] (Ctrl+Shift+[ / Ctrl+Shift+]
   *  non-mac) cycle backwards / forwards through the view list.
   *  Standard macOS app convention (Safari, Chrome use this for
   *  tab cycling). Suppressed while a text input has focus so it
   *  doesn't steal from the chat composer / search inputs / cell
   *  editors. */
  private _onKeyDownForViewCycle = (ev: KeyboardEvent) => {
    const accel = ev.metaKey || ev.ctrlKey;
    if (!accel || !ev.shiftKey) return;
    if (ev.altKey) return;
    const k = ev.key;
    if (k !== "[" && k !== "]" && k !== "{" && k !== "}") return;
    const target = ev.target as HTMLElement | null;
    if (target) {
      const tag = target.tagName;
      if (tag === "INPUT" || tag === "TEXTAREA" || target.isContentEditable) return;
    }
    const direction = (k === "[" || k === "{") ? -1 : 1;
    const currentIdx = VIEWS.findIndex(v => v.id === this.view);
    if (currentIdx < 0) return;
    const nextIdx = ((currentIdx + direction) % VIEWS.length + VIEWS.length) % VIEWS.length;
    const nextView = VIEWS[nextIdx];
    if (!nextView) return;
    ev.preventDefault();
    this._selectView(nextView.id);
  };

  /** Open + close the palette. Reset query + selection each toggle
   *  so the user always lands at the top of the unfiltered list. */
  _togglePalette() {
    if (this.paletteOpen) {
      this.paletteOpen = false;
      this.paletteQuery = "";
      this.paletteIndex = -1;
    } else {
      this.paletteOpen = true;
      this.paletteQuery = "";
      this.paletteIndex = 0;
    }
  }

  /** Step the palette cursor. Wraps. No-op when the filtered list is
   *  empty (so ↓ on "ghost-needle" doesn't move from -1 to 0/NaN). */
  _palettedStep(delta: number) {
    const list = filterPaletteEntries(NAV, this.paletteQuery);
    if (list.length === 0) return;
    if (this.paletteIndex < 0) {
      this.paletteIndex = delta > 0 ? 0 : list.length - 1;
      return;
    }
    const next = ((this.paletteIndex + delta) % list.length + list.length) % list.length;
    this.paletteIndex = next;
  }

  /** Commit the highlighted palette entry — switches pane + closes
   *  the overlay. If the cursor's negative (user pressed Enter
   *  before stepping), the first filtered match wins. */
  _paletteCommit() {
    const list = filterPaletteEntries(NAV, this.paletteQuery);
    if (list.length === 0) return;
    const idx = this.paletteIndex < 0 ? 0 : Math.min(this.paletteIndex, list.length - 1);
    const entry = list[idx];
    if (!entry) return;
    this._select(entry.id);
    this.paletteOpen = false;
    this.paletteQuery = "";
    this.paletteIndex = -1;
  }

  async connectedCallback() {
    super.connectedCallback();
    // Register the ⌘P listener up-front — same pattern as model-
    // browser's Escape listener — so it works before the async
    // i18n promise lands.
    window.addEventListener("keydown", this._onKeyDownForPalette);
    window.addEventListener("keydown", this._onKeyDownForViewSwitch);
    window.addEventListener("keydown", this._onKeyDownForViewCycle);
    // Outside-click closes the view-switcher dropdown.
    document.addEventListener("click", this._onDocClickForSwitcher);
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
        this._select(id);
      }
    });

    // Subscribe to "lthn:window:files-dropped" — Wails fires this
    // when files are dragged from the OS onto an EnableFileDrop
    // window. We filter for .gguf paths and route each through
    // Models.Import, then emit lthn:models:refresh so the model
    // browser's local rail picks up the new entries without a
    // manual refresh.
    this._unsubFilesDropped = Events.On("lthn:window:files-dropped", (ev) => {
      const data = ev?.data as { payload?: { files?: string[] } } | undefined;
      const paths = filterGgufPaths(data?.payload?.files);
      if (paths.length === 0) return;
      void this._adoptDroppedGgufs(paths);
    });

    // Subscribe to "lthn:menu:changed" — emitted by the Settings panel
    // when the user flips Menu Links or Menu Layout. Lets the rail
    // react without a reload. ev.data is { setting, value }.
    this._unsubMenuChanged = Events.On("lthn:menu:changed", (ev) => {
      const data = ev?.data as { setting?: string; value?: string } | undefined;
      if (!data?.setting || !data?.value) return;
      if (data.setting === "links")  this.menuLinks  = data.value;
      if (data.setting === "layout") {
        this.menuLayout = data.value;
        this._applyMenuLayout();
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
      const { unwrap } = await import("./result");
      const [models, build] = await Promise.all([
        unwrap<string[]>(runner.WModels(), []),
        fl.Build().catch(() => null),
      ]);
      const firstModel = models?.[0];
      if (firstModel) {
        this.model = firstModel;
        this.running = true;
      } else {
        this.running = false;
      }
      if (build?.version) this.version = `v${build.version}`;
    } catch { /* keep design fallbacks */ }
  }

  disconnectedCallback() {
    super.disconnectedCallback();
    window.removeEventListener("keydown", this._onKeyDownForPalette);
    window.removeEventListener("keydown", this._onKeyDownForViewSwitch);
    window.removeEventListener("keydown", this._onKeyDownForViewCycle);
    document.removeEventListener("click", this._onDocClickForSwitcher);
    if (this._unsubSetPane) {
      this._unsubSetPane();
      this._unsubSetPane = null;
    }
    if (this._unsubMenuChanged) {
      this._unsubMenuChanged();
      this._unsubMenuChanged = null;
    }
    if (this._unsubFilesDropped) {
      this._unsubFilesDropped();
      this._unsubFilesDropped = null;
    }
    if (this._hoverCollapseTimer) {
      clearTimeout(this._hoverCollapseTimer);
      this._hoverCollapseTimer = null;
    }
  }

  /** Returned by Events.On to detach the listener on element teardown. */
  private _unsubSetPane: (() => void) | null = null;
  private _unsubMenuChanged: (() => void) | null = null;
  /** Unsubscribe for the lthn:window:files-dropped listener that
   *  routes dragged .gguf files into Models.Import. */
  private _unsubFilesDropped: (() => void) | null = null;

  /** Walk a list of .gguf paths and import each via Models.Import.
   *  Failures per-path are warned but don't stop the batch — the
   *  user dragged N files expecting N imports. After the batch lands
   *  we emit lthn:models:refresh once so any mounted model-browser
   *  reloads its rail. Public so the suite can exercise the wire
   *  without raising a real OS drop. */
  async _adoptDroppedGgufs(paths: readonly string[]) {
    if (!paths || paths.length === 0) return;
    let landed = 0;
    try {
      const ms = await import("@desktop/models/wailsservice");
      const { demand } = await import("./result");
      for (const p of paths) {
        try {
          await demand<string>(ms.Import(p));
          landed += 1;
        } catch (err: unknown) {
          console.warn("drop-import failed:", p, err);
        }
      }
    } catch (err: unknown) {
      console.warn("drop-import binding lookup failed:", err);
      return;
    }
    if (landed > 0) {
      try {
        const { Events } = await import("@wailsio/runtime");
        Events.Emit({ name: MODELS_REFRESH_EVENT, data: { count: landed } });
      } catch { /* best effort */ }
    }
  }

  _select(id: string) {
    this.active = id;
    try {
      // Per-view key so each view remembers its own last pane.
      localStorage.setItem(LthnAppShell.ACTIVE_PANE_PREFIX + this.view, id);
      // Mirror to the legacy key when in admin so a downgrade to a
      // pre-VIEWS build still lands on the right pane.
      if (this.view === "admin") {
        localStorage.setItem(LthnAppShell.ACTIVE_PANE_KEY, id);
      }
    }
    catch { /* localStorage unavailable — silently degrade */ }
  }

  /** Switch to a different view. Restores that view's last-active
   *  pane (per-view localStorage key), or falls back to the view's
   *  first nav entry on a fresh-for-this-view launch. Persists the
   *  view selection so the next launch lands on the same view. */
  _selectView(viewId: string) {
    const target = VIEW_BY_ID[viewId];
    if (!target) return;
    this.view = viewId;
    this.switcherOpen = false;
    try { localStorage.setItem(LthnAppShell.ACTIVE_VIEW_KEY, viewId); }
    catch { /* localStorage unavailable — silently degrade */ }
    // Restore this view's last-active pane (or first entry).
    const savedPane = (() => {
      try { return localStorage.getItem(LthnAppShell.ACTIVE_PANE_PREFIX + viewId); }
      catch { return null; }
    })();
    const valid = savedPane && target.nav.some(n => n.id === savedPane);
    this._select(valid ? (savedPane as string) : (target.nav[0]?.id ?? ""));
  }

  /** Toggle the view-switcher dropdown. Outside-click closes it via
   *  the document listener installed in connectedCallback. */
  _toggleSwitcher = (e?: MouseEvent) => {
    e?.stopPropagation();
    this.switcherOpen = !this.switcherOpen;
  };

  /** Document-level click handler that closes the switcher when the
   *  click lands outside its container. The switcher's own button
   *  stops propagation so this only fires on outside clicks. */
  private _onDocClickForSwitcher = (e: MouseEvent) => {
    if (!this.switcherOpen) return;
    const target = e.target as HTMLElement | null;
    if (target?.closest(".lthn-view-switcher")) return;
    this.switcherOpen = false;
  };
  _toggleCollapse() {
    // No-op when layout is locked open / closed / hover-driven —
    // the chevron is hidden in those modes but a stray keyboard
    // binding shouldn't bypass the user preference either.
    if (this.menuLayout !== "toggle") return;
    this.collapsed = !this.collapsed;
  }

  /** Apply the persisted Menu Layout to the rail. See
   *  plans/project/lthn/desktop/RFC.menu-behaviours.md § 2.2. */
  _applyMenuLayout() {
    switch (this.menuLayout) {
      case "open":   this.collapsed = false; break;
      case "closed": this.collapsed = true;  break;
      case "hover":  this.collapsed = true;  break;  // start collapsed; expand on hover
      case "toggle":
      default:       /* respect current this.collapsed */ break;
    }
  }

  /** Hover Open mode: mouseenter on the rail expands; mouseleave
   *  collapses after a 300ms debounce so cursor overshoot doesn't
   *  thrash the rail. No-op when layout != hover. */
  _onRailEnter = () => {
    if (this.menuLayout !== "hover") return;
    if (this._hoverCollapseTimer) {
      clearTimeout(this._hoverCollapseTimer);
      this._hoverCollapseTimer = null;
    }
    this.collapsed = false;
  };
  _onRailLeave = () => {
    if (this.menuLayout !== "hover") return;
    if (this._hoverCollapseTimer) clearTimeout(this._hoverCollapseTimer);
    this._hoverCollapseTimer = setTimeout(() => {
      this.collapsed = true;
      this._hoverCollapseTimer = null;
    }, 300);
  };

  /** Rail-row click dispatcher. The dispatch matrix lives in
   *  plans/project/lthn/desktop/RFC.menu-behaviours.md § 4. Summary:
   *  - ⌘/Ctrl-click always pops out (universal escape hatch, no
   *    setting can disable it).
   *  - Plain click on a word always navigates in-place.
   *  - Plain click on the icon depends on menuLinks:
   *      hybrid          → pop out
   *      in-window       → navigate
   *      collapsed-only  → pop out iff the rail is currently collapsed
   *  - When the rail is collapsed, the button shows only the icon,
   *    so any click counts as an icon click. */
  _onNavClick(e: MouseEvent, id: string) {
    if (e.metaKey || e.ctrlKey) {
      e.preventDefault();
      this._popOut(id);
      return;
    }
    const target = e.target as HTMLElement | null;
    const isIcon = this.collapsed ? true : (target?.closest("i") !== null);
    const shouldPopOut = isIcon && (
      this.menuLinks === "hybrid" ||
      (this.menuLinks === "collapsed-only" && this.collapsed)
    );
    if (shouldPopOut) {
      e.preventDefault();
      this._popOut(id);
      return;
    }
    this._select(id);
  }

  private _popOut(id: string) {
    import("@desktop/desktop/windowservice")
      .then(w => w.Open(id))
      .catch(() => { /* WindowService import failure: silently degrade */ });
  }

  _renderNavGroup(group: NavEntry["group"], label: string | null) {
    // Filter the CURRENT view's nav, not the legacy NAV const, so
    // switching views re-populates the rail. NAV is kept for back-
    // compat in palette + pop-out code paths (admin's nav).
    const items = navForView(this.view).filter(n => n.group === group);
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
            @click=${(e: MouseEvent) => this._onNavClick(e, n.id)}
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
    const entry = navForView(this.view).find(n => n.id === this.active);
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
    // If the requested tag isn't registered (Phase 1 stub views land
    // here), fall back to <lthn-view-placeholder> so the body slot
    // shows a branded "coming soon" instead of an empty pane.
    const fallback = !customElements.get(tag);
    const realTag = fallback ? "lthn-view-placeholder" : tag;
    const el = document.createElement(realTag);
    // Two-shell pattern: mark every child as embedded so renderChrome
    // skips its standalone card chrome and fills our body slot. The
    // shell already paints the titlebar / side-nav / status bar.
    // See memory design_two_shell_pattern.md.
    el.setAttribute("embedded", "");
    if (fallback) {
      // Wire the placeholder so the "coming soon" card names what
      // shipped vs. what's missing — see LthnViewPlaceholder below.
      const entry = navForView(this.view).find(n => n.id === this.active);
      const view = VIEW_BY_ID[this.view];
      el.setAttribute("tag", tag);
      el.setAttribute("kind", this.active);
      el.setAttribute("label", entry?.label ?? this.active);
      el.setAttribute("icon", entry?.icon ?? "fa-circle");
      el.setAttribute("view", view?.label ?? this.view);
      return el;
    }
    // Pass through some sensible defaults so the windows look populated
    if (tag === "lthn-chat-window") el.setAttribute("state", "multi-turn");
    if (tag === "lthn-logs-window") el.setAttribute("tab", "live");
    if (tag === "lthn-welcome-window") el.setAttribute("step", "2");
    if (tag === "lthn-settings-window") el.setAttribute("open", "models");
    return el;
  }

  /** Render the view-switcher dropdown at the top of the side-nav.
   *  Collapsed rail shows just the view icon as a 40px button; the
   *  dropdown menu pops next to it. Expanded rail shows label +
   *  blurb + chevron, full-width.
   *
   *  Outside-click closes the menu (document listener wired in
   *  connectedCallback). The button has class `lthn-view-switcher`
   *  so the outside-click handler can detect "click was on me".
   */
  _renderViewSwitcher() {
    const view = VIEW_BY_ID[this.view] ?? VIEW_BY_ID.admin;
    if (this.collapsed) {
      return html`
        <div style="padding:8px 0; display:flex; justify-content:center; position:relative;">
          <button class="lthn-view-switcher"
            @click=${this._toggleSwitcher}
            title=${`View · ${view.label}`}
            style="--wails-draggable: no-drag;
                   width:40px; height:40px; border-radius:8px;
                   background:rgba(64,193,197,0.10); border:1px solid rgba(64,193,197,0.22);
                   color:var(--brand-300); cursor:pointer;
                   display:flex; align-items:center; justify-content:center;">
            <i class="fa-solid ${view.icon}" style="font-size:14px;"></i>
          </button>
          ${this.switcherOpen ? this._renderSwitcherMenu(true) : nothing}
        </div>
      `;
    }
    return html`
      <div style="padding:10px 8px 4px; position:relative;">
        <button class="lthn-view-switcher"
          @click=${this._toggleSwitcher}
          style="--wails-draggable: no-drag;
                 width:100%; display:flex; align-items:center; gap:10px;
                 padding:10px 12px; border-radius:8px;
                 background:rgba(64,193,197,0.06); border:1px solid rgba(64,193,197,0.18);
                 color:var(--fg-0); cursor:pointer; text-align:left;
                 font-family:var(--font-sans); font-size:13px;">
          <i class="fa-solid ${view.icon}" style="font-size:13px; color:var(--brand-300); width:14px; text-align:center;"></i>
          <div style="flex:1; min-width:0;">
            <div style="font-family:var(--font-mono); font-size:9.5px; color:var(--brand-300); letter-spacing:0.10em; text-transform:uppercase; line-height:1.2;">View</div>
            <div style="font-weight:500; color:var(--fg-0); letter-spacing:-0.005em; line-height:1.3;">${view.label}</div>
          </div>
          <i class="fa-solid ${this.switcherOpen ? "fa-angle-up" : "fa-angle-down"}" style="font-size:10px; color:var(--fg-3);"></i>
        </button>
        ${this.switcherOpen ? this._renderSwitcherMenu(false) : nothing}
      </div>
    `;
  }

  /** The pop-down menu listing every view. Renders relative to the
   *  switcher button when expanded; absolute-positioned beside the
   *  button when the rail is collapsed. */
  _renderSwitcherMenu(collapsedHost: boolean) {
    return html`
      <div class="lthn-view-switcher" style="
        position:absolute; ${collapsedHost ? "top:8px; left:54px; min-width:260px;" : "top:calc(100% + 4px); left:8px; right:8px;"}
        z-index:50;
        background:linear-gradient(180deg, #1a1822 0%, #15131c 100%);
        border:1px solid rgba(64,193,197,0.22);
        border-radius:10px;
        box-shadow: 0 20px 60px rgba(0,0,0,0.55), 0 2px 6px rgba(0,0,0,0.4);
        padding:6px; display:flex; flex-direction:column; gap:1px;
      ">
        <div style="padding:8px 12px 4px; font-family:var(--font-mono); font-size:9.5px; color:var(--fg-3); letter-spacing:0.10em; text-transform:uppercase; display:flex; justify-content:space-between; align-items:center;">
          <span>Switch view</span>
          <span style="color:var(--fg-4);">${VIEWS.length} available</span>
        </div>
        ${VIEWS.map(v => {
          const active = v.id === this.view;
          return html`
            <button @click=${() => this._selectView(v.id)}
              style="--wails-draggable: no-drag;
                     display:flex; align-items:flex-start; gap:12px;
                     padding:9px 12px; border-radius:6px;
                     background:${active ? "rgba(64,193,197,0.10)" : "transparent"};
                     border:1px solid ${active ? "rgba(64,193,197,0.22)" : "transparent"};
                     color:${active ? "var(--brand-300)" : "var(--fg-1)"};
                     cursor:pointer; text-align:left;
                     font-family:var(--font-sans); font-size:12.5px;">
              <i class="fa-solid ${v.icon}" style="font-size:13px; width:14px; text-align:center; margin-top:2px; color:${active ? "var(--brand-300)" : "var(--fg-3)"};"></i>
              <div style="flex:1; min-width:0;">
                <div style="font-weight:500; color:${active ? "var(--brand-300)" : "var(--fg-0)"}; letter-spacing:-0.005em;">${v.label}</div>
                <div style="font-size:11px; color:var(--fg-3); margin-top:2px; line-height:1.4;">${v.blurb}</div>
              </div>
              ${active ? html`<i class="fa-solid fa-check" style="font-size:10px; color:var(--brand-300); margin-top:4px;"></i>` : nothing}
            </button>
          `;
        })}
      </div>
    `;
  }

  /** Render the ⌘P command palette overlay. Nothing when closed. */
  _renderPalette() {
    if (!this.paletteOpen) return nothing;
    const filtered = filterPaletteEntries(NAV, this.paletteQuery);
    const idx = filtered.length === 0
      ? -1
      : Math.max(0, Math.min(this.paletteIndex, filtered.length - 1));
    return html`
      <div class="lthn-palette-backdrop"
           @click=${() => this._togglePalette()}
           style="
             position:fixed; inset:0;
             background:rgba(0,0,0,0.48);
             backdrop-filter:blur(2px);
             display:flex; align-items:flex-start; justify-content:center;
             padding-top:96px;
             z-index:9200;
             --wails-draggable: no-drag;
           ">
        <div class="lthn-palette"
             role="dialog"
             aria-label="Command palette"
             @click=${(ev: Event) => ev.stopPropagation()}
             style="
               width:480px; max-height:60%;
               display:flex; flex-direction:column;
               background:rgba(18,20,24,0.98);
               border:1px solid rgba(64,193,197,0.20);
               border-radius:10px;
               box-shadow:0 24px 48px rgba(0,0,0,0.5);
               overflow:hidden;
             ">
          <div style="display:flex; align-items:center; gap:10px;
                      padding:12px 14px;
                      border-bottom:1px solid rgba(255,255,255,0.06);">
            <i class="fa-solid fa-magnifying-glass" style="font-size:11px; color:var(--fg-3);"></i>
            <input
              class="lthn-palette-input"
              type="text"
              placeholder="Jump to…"
              .value=${this.paletteQuery}
              @input=${(ev: Event) => {
                this.paletteQuery = (ev.target as HTMLInputElement).value;
                this.paletteIndex = 0;
              }}
              @keydown=${(ev: KeyboardEvent) => {
                if (ev.key === "ArrowDown") { ev.preventDefault(); this._palettedStep(1); return; }
                if (ev.key === "ArrowUp")   { ev.preventDefault(); this._palettedStep(-1); return; }
                if (ev.key === "Enter")     { ev.preventDefault(); this._paletteCommit(); return; }
                if (ev.key === "Escape")    { ev.preventDefault(); this._togglePalette(); return; }
              }}
              ${litRef((el?: Element) => { if (el instanceof HTMLInputElement) el.focus(); })}
              style="
                flex:1; min-width:0;
                background:transparent; border:none; outline:none;
                font:inherit; font-size:14px;
                color:var(--fg-0);
                --wails-draggable: no-drag;
              " />
            <span style="font-family:var(--font-mono); font-size:10px; color:var(--fg-3);
                         padding:1px 5px; border:1px solid rgba(255,255,255,0.08);
                         border-radius:3px;">⌘P</span>
          </div>
          <div class="lthn-palette-list"
               style="flex:1; overflow-y:auto; padding:6px;">
            ${filtered.length === 0 ? html`
              <div class="lthn-palette-empty"
                   style="padding:24px 16px; text-align:center; font-size:12px; color:var(--fg-3);">
                No match for "${this.paletteQuery.trim()}".
              </div>
            ` : filtered.map((entry, i) => {
              const isActive = i === idx;
              return html`
                <button class="lthn-palette-item"
                        data-pane-id=${entry.id}
                        ?data-active=${isActive}
                        @mouseenter=${() => { this.paletteIndex = i; }}
                        @click=${() => this._paletteCommit()}
                        style="
                          display:flex; align-items:center; gap:10px;
                          width:100%; padding:8px 10px;
                          background:${isActive ? "rgba(64,193,197,0.10)" : "transparent"};
                          border:1px solid ${isActive ? "rgba(64,193,197,0.22)" : "transparent"};
                          border-radius:6px;
                          font:inherit; font-size:12px;
                          color:var(--fg-1);
                          text-align:left; cursor:pointer;
                          --wails-draggable: no-drag;
                        ">
                  <i class="fa-solid ${entry.icon}"
                     style="width:14px; font-size:11px; color:${isActive ? "var(--brand-300)" : "var(--fg-3)"};"></i>
                  <span style="flex:1;">${entry.label}</span>
                  <span style="font-size:10px; color:var(--fg-3); font-family:var(--font-mono);">${entry.group}</span>
                </button>
              `;
            })}
          </div>
        </div>
      </div>
    `;
  }

  render() {
    const navWidth = this.collapsed ? 56 : 220;
    const active = NAV.find(n => n.id === this.active);
    return html`
      ${this._renderPalette()}
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
        /* No shell-wide drag — text selection in the body would
           break if the whole window acted as a window-drag handle.
           Only <header> below is marked draggable; that matches the
           native macOS pattern where the titlebar is the drag
           target and the document body is selectable. */
        --wails-draggable: no-drag;
      ">
        <!-- TITLEBAR (the only drag region in the shell) -->
        <header style="
          grid-column: 1 / -1; grid-row: 1;
          display:flex; align-items:center; gap:14px;
          min-height:40px;
          padding:0 14px;
          background: rgba(255,255,255,0.02);
          border-bottom: 1px solid rgba(255,255,255,0.05);
          --wails-draggable: drag;
          cursor: default;
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
        <aside
          @mouseenter=${this._onRailEnter}
          @mouseleave=${this._onRailLeave}
          style="
          grid-row: 2; grid-column: 1;
          display:flex; flex-direction:column;
          background: rgba(0,0,0,0.22);
          border-right: 1px solid rgba(255,255,255,0.05);
          overflow-y: auto; overflow-x: hidden;
          padding-bottom: 6px;
        ">
          ${this._renderViewSwitcher()}

          <div style="padding:8px 8px 4px; display:flex; align-items:center; gap:8px; ${this.collapsed ? "justify-content:center;" : ""}">
            ${!this.collapsed ? html`
              <div style="flex:1; display:flex; align-items:center; gap:6px; padding:4px 6px; border-radius:6px; background:rgba(64,193,197,0.06); border:1px solid rgba(64,193,197,0.18);">
                <lthn-status-dot variant=${this.running ? "ok" : "idle"} ?pulse=${this.running}></lthn-status-dot>
                <span style="font-family:var(--font-mono); font-size:10.5px; color:var(--fg-1); white-space:nowrap; overflow:hidden; text-overflow:ellipsis;">${this.model}</span>
              </div>
            ` : html`<lthn-status-dot variant=${this.running ? "ok" : "idle"} ?pulse=${this.running}></lthn-status-dot>`}
            ${this.menuLayout === "toggle" ? html`
              <button @click=${this._toggleCollapse} title=${this.collapsed ? this.t.expand : this.t.collapse} style="width:22px; height:22px; border-radius:5px; background:transparent; border:1px solid rgba(255,255,255,0.07); color:var(--fg-3); cursor:pointer; display:flex; align-items:center; justify-content:center;">
                <i class="fa-solid ${this.collapsed ? "fa-angles-right" : "fa-angles-left"}" style="font-size:9px;"></i>
              </button>
            ` : nothing}
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
          <span>view · ${this.view}</span>
          <span>·</span>
          <span>${active ? active.label.toLowerCase() : ""}</span>
          <span>·</span>
          <span>${this.version}</span>
        </footer>
      </div>
    `;
  }
}
customElements.define("lthn-app-shell", LthnAppShell);

/** Placeholder element for views whose custom element hasn't been
 *  registered yet. Phase 1 ships 6 such views (planning / coding /
 *  marketing / operations / sales / office); Phase 2 replaces these
 *  by registering real elements with the matching tag.
 *
 *  Renders a branded "coming soon" card naming the missing tag so a
 *  reviewer/dev knows exactly which custom element to define. The
 *  `embedded` attribute makes it fill the shell's body slot without
 *  its own chrome.
 */
class LthnViewPlaceholder extends LitElement {
  static readonly properties = {
    tag:   { type: String },
    kind:  { type: String },
    label: { type: String },
    icon:  { type: String },
    view:  { type: String },
    embedded: { type: Boolean, reflect: true },
  };
  declare tag:   string;
  declare kind:  string;
  declare label: string;
  declare icon:  string;
  declare view:  string;
  declare embedded: boolean;

  constructor() {
    super();
    this.tag = "";
    this.kind = "";
    this.label = "Window";
    this.icon = "fa-circle";
    this.view = "";
    this.embedded = false;
  }
  createRenderRoot() { return this; }

  render() {
    return html`
      <div style="flex:1; display:flex; flex-direction:column; align-items:center; justify-content:center; padding:60px; gap:24px; text-align:center; min-height:480px;">
        <div style="width:88px; height:88px; border-radius:22px;
                    background:linear-gradient(155deg, rgba(64,193,197,0.18), rgba(64,193,197,0.02));
                    border:1px solid rgba(64,193,197,0.22);
                    display:flex; align-items:center; justify-content:center;">
          <i class="fa-solid ${this.icon}" style="font-size:34px; color:var(--brand-300);"></i>
        </div>
        <div>
          <div style="font-family:var(--font-mono); font-size:11px; color:var(--brand-300); letter-spacing:0.16em; text-transform:uppercase;">
            ${this.view} · ${this.kind}
          </div>
          <div style="font-size:28px; font-weight:600; color:var(--fg-0); letter-spacing:-0.02em; margin-top:14px;">
            ${this.label}
          </div>
          <p style="font-size:14px; color:var(--fg-2); line-height:1.55; max-width:420px; margin:14px auto 0;">
            This view ships in a future release. The shell architecture, view switcher, and
            navigation are already in place — register a custom element with the matching tag
            and it mounts here automatically.
          </p>
        </div>
        <div style="display:flex; align-items:center; gap:10px; padding:8px 14px; border-radius:999px;
                    background:rgba(245,158,11,0.08); border:1px solid rgba(245,158,11,0.20);
                    font-family:var(--font-mono); font-size:10.5px; color:var(--warning-400); letter-spacing:0.10em;">
          <i class="fa-solid fa-flask" style="font-size:11px;"></i>
          COMING SOON
        </div>
        <div style="margin-top:8px; padding:14px 18px; border-radius:8px;
                    background:rgba(0,0,0,0.30); border:1px solid rgba(255,255,255,0.06);
                    font-family:var(--font-mono); font-size:11.5px; color:var(--fg-2); line-height:1.6; text-align:left;">
          <div style="color:var(--fg-3); font-size:10px; letter-spacing:0.08em; text-transform:uppercase; margin-bottom:6px;">register a real implementation</div>
          customElements.define("${this.tag || `lthn-view-${this.kind}`}", class extends LitElement { … });
        </div>
      </div>
    `;
  }
}
customElements.define("lthn-view-placeholder", LthnViewPlaceholder);

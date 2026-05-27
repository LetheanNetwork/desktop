// SPDX-Licence-Identifier: EUPL-1.2
// E1.2 · settings — <lthn-settings-window>
// Light-DOM Lit element. Composes renderChrome() from ../chrome.js.

import { LitElement, html, nothing } from "lit";
import { renderChrome } from "../chrome";
import { T } from "@lthn/i18n/coreservice";
import type { LitContent } from "../types";
import { unwrap, demand } from "../result";

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

/** PublicEndpoint mirror — the Bearer-stripped read-side shape the
 *  openaibench WailsService exposes (matches PublicEndpoint in
 *  go/pkg/openaibench/wails.go). The Bearer field is intentionally
 *  absent so it can't leak through component state into the DOM. */
interface PublicEndpoint {
  name:         string;
  url:          string;
  description?: string;
  org?:         string;
  auth_set:     boolean;
}

/** AddEndpointForm — write-side shape used by the Add modal. The
 *  Bearer field IS present here because the user is supplying it;
 *  it round-trips to the Wails AddEndpoint call where pkg/keys
 *  encrypts at rest. Cleared the moment the modal closes. */
interface AddEndpointForm {
  name:        string;
  url:         string;
  description: string;
  bearer:      string;
  org:         string;
}

function storedSetting(key: string, fallback = ""): string {
  try {
    const storage = globalThis.localStorage;
    if (storage && typeof storage.getItem === "function") {
      return storage.getItem(key) || fallback;
    }
  } catch {
    // Keep settings renderable in storage-less shells and tests.
  }
  return fallback;
}

function writeStoredSetting(key: string, value: string): void {
  try {
    const storage = globalThis.localStorage;
    if (storage && typeof storage.setItem === "function") {
      storage.setItem(key, value);
    }
  } catch {
    // Settings state still updates even if host storage is unavailable.
  }
}

class LthnSettingsWindow extends LitElement {
  static readonly properties = {
    open: { type: String, reflect: true },
    w:    { type: Number },
    h:    { type: Number },
    embedded: { type: Boolean, reflect: true },
    chrome: { state: true },
    locales: { state: true },
    currentLang: { state: true },
    startWithWindow: { state: true },
    menuLinks:  { state: true },
    menuLayout: { state: true },
    modelsDir: { state: true },
    routeNames: { state: true },
    lemmaModel: { state: true },
    lemmaRuntime: { state: true },
    lemmaEndpoint: { state: true },
    build: { state: true },
    sampleInterval: { state: true },
    heapSamples: { state: true },
    routes: { state: true },
    endpoint: { state: true },
    httpListening: { state: true },
    panel: { state: true },
    row:   { state: true },
    apiKey: { state: true },
    apiKeyRevealed: { state: true },
    endpoints: { state: true },
    addModalOpen: { state: true },
    addForm: { state: true },
    addErr: { state: true },
    addSubmitting: { state: true },
  };
  declare open: string;
  declare w: number;
  declare h: number;
  declare embedded: boolean;
  declare chrome: { title: string; subtitle: string };
  declare locales: string[];
  declare currentLang: string;
  declare startWithWindow: boolean;
  /** Menu Behaviours — see plans/project/lthn/desktop/RFC.menu-behaviours.md.
   *  menuLinks  ∈ {"hybrid", "in-window", "collapsed-only"}
   *  menuLayout ∈ {"toggle", "open", "closed", "hover"} */
  declare menuLinks: string;
  declare menuLayout: string;
  declare modelsDir: string;
  declare routeNames: string[];
  /** Loaded model basename from Lemma.Status — fallback for the
   *  Default model display when no runner routes are configured.
   *  Empty when Lemma is unreachable or no model is loaded. */
  declare lemmaModel: string;
  /** Runtime tag from Lemma.Status (metal / cuda / rocm / cpu) +
   *  endpoint URL used to address the engine. Both populated only
   *  when the engine is reachable. Used by the Runner pane to
   *  render a Local Lemma card alongside runner routes. */
  declare lemmaRuntime: string;
  declare lemmaEndpoint: string;
  declare build: { version: string; go_version: string; goos: string; goarch: string; num_cpu: number };
  declare sampleInterval: string;
  declare heapSamples: string;
  declare routes: RouteView[];
  declare endpoint: string;
  declare httpListening: boolean;
  declare apiKey: string;
  declare apiKeyRevealed: boolean;
  declare endpoints: PublicEndpoint[];
  declare addModalOpen: boolean;
  declare addForm: AddEndpointForm;
  declare addErr: string;
  declare addSubmitting: boolean;
  declare panel: {
    generalT: string; generalD: string;
    menuT: string;
    modelsT: string; modelsD: string;
    runnerT: string; runnerD: string;
    apiT: string; apiD: string;
    telemetryT: string; telemetryD: string;
    integrationsT: string; integrationsD: string;
    aboutT: string; aboutD: string;
  };
  declare row: {
    startWindowL: string; startWindowH: string;
    languageL: string; languageH: string;
    menuLinksL: string; menuLinksH: string;
    menuLinksOptHybrid: string; menuLinksOptInWindow: string; menuLinksOptCollapsed: string;
    menuLayoutL: string; menuLayoutH: string;
    menuLayoutOptToggle: string; menuLayoutOptOpen: string; menuLayoutOptClosed: string; menuLayoutOptHover: string;
    btnReplayMenuTour: string; menuReplayH: string;
    modelDirL: string; modelDirH: string; btnBrowse: string;
    defaultModelL: string; defaultModelHint: string; defaultModelEmpty: string;
    quantL: string; quantH: string;
    samplingL: string; samplingH: string;
    httpServerL: string; endpointL: string;
    apiKeyL: string; apiKeyH: string;
    intervalL: string; intervalH: string;
    heapL: string; heapH: string;
    powerL: string; powerH: string;
    openIntL: string; openIntH: string; btnOpen: string;
    versionL: string; versionH: string;
    goToolchainL: string; platformL: string; cpusL: string; cpusH: string;
    emptyRoutes: string;
    footer: string;
  };
  constructor() {
    super();
    this.open = "general"; this.w = 760; this.h = 600; this.embedded = false;
    this.chrome = { title: "Settings", subtitle: "lthn · v0.2.0-rc1" };
    this.locales = [];
    this.currentLang = "";
    // Persisted via localStorage["lthn.boot.window"] — Go-side
    // application boot will read this to decide whether to spawn the
    // unified app shell at launch, or leave the binary tray-only.
    this.startWithWindow = storedSetting("lthn.boot.window") === "true";
    // Menu Behaviours defaults — least surprise out of the box. See
    // plans/project/lthn/desktop/RFC.menu-behaviours.md § 2 for the full
    // value table. The app-shell reads the same localStorage keys at
    // mount + reacts to "lthn:menu:changed" emits when these flip.
    this.menuLinks  = storedSetting("lthn.menu.links",  "in-window");
    this.menuLayout = storedSetting("lthn.menu.layout", "toggle");
    this.modelsDir = "~/Lethean/conf/models/";
    this.routeNames = [];
    this.lemmaModel = "";
    this.lemmaRuntime = "";
    this.lemmaEndpoint = "";
    this.build = { version: "0.1.0", go_version: "", goos: "", goarch: "", num_cpu: 0 };
    // Telemetry poll cadence + sparkline window. Persisted via
    // localStorage so the tray + telemetry-window read the same
    // values at their connectedCallback.
    this.sampleInterval = storedSetting("lthn.telemetry.interval", "2s");
    this.heapSamples    = storedSetting("lthn.telemetry.samples", "24");
    this.routes = [];
    this.endpoint = "http://localhost:8000/v1";
    this.httpListening = false;
    this.apiKey = "sk-lthn-••••••••••••••••2qB7";
    this.apiKeyRevealed = false;
    this.endpoints = [];
    this.addModalOpen = false;
    this.addForm = { name: "", url: "", description: "", bearer: "", org: "" };
    this.addErr = "";
    this.addSubmitting = false;
    this.panel = {
      generalT: "General",
      generalD: "App-wide defaults — what to show at launch, which language to speak, theme.",
      menuT:    "Menu Behaviours",
      modelsT:  "Models",
      modelsD:  "Where lthn looks for models and which one loads at startup.",
      runnerT:  "Runner",
      runnerD:  "Provider routes the runner serves. Read-only; edit ~/Lethean/conf/lthn.yaml to add or change a route, then restart lthn.",
      apiT:     "API",
      apiD:     "HTTP server for OpenAI-compatible clients. Off by default; nothing leaves this Mac unless you turn it on.",
      telemetryT: "Telemetry",
      telemetryD: "Process metrics shown in the tray + live-telemetry window. Local only; nothing leaves the device.",
      integrationsT: "Integrations",
      integrationsD: "Connected clients reading from the local lthn API. Full surface lives in the dedicated Integrations window.",
      aboutT:   "About",
      aboutD:   "What's running — version, runtime, and where the source lives.",
    };
    this.row = {
      startWindowL: "Start with window",
      startWindowH: "If on, the unified app shell opens at launch alongside the tray. Off = tray-only until you click in.",
      languageL: "Default language",
      languageH: "Sets the language for the WebView surfaces. Stored locally; persists across restarts.",
      menuLinksL: "Menu Links",
      menuLinksH: "How clicks on rail items navigate. Hybrid: word opens here, icon pops out. Always In-Window: both navigate. New When Collapsed: pop out only when the rail is icon-only. ⌘/Ctrl-click always pops out.",
      menuLinksOptHybrid:    "Hybrid",
      menuLinksOptInWindow:  "Always in-window",
      menuLinksOptCollapsed: "New when collapsed",
      menuLayoutL: "Menu Layout",
      menuLayoutH: "How the rail collapses. Toggle keeps the chevron-driven behaviour. Always Open / Always Closed lock it. Hover Open expands on mouseover.",
      menuLayoutOptToggle: "Toggle",
      menuLayoutOptOpen:   "Always open",
      menuLayoutOptClosed: "Always closed",
      menuLayoutOptHover:  "Hover open",
      btnReplayMenuTour:   "Replay menu tour",
      menuReplayH:         "Re-open the welcome wizard at the menu-behaviours step.",
      modelDirL: "Model directory",
      modelDirH: "Canonical Lethean layout — visible in Finder, safe to inspect.",
      btnBrowse: "Browse…",
      defaultModelL: "Default model",
      defaultModelHint:  "Configured runner route — picked first when the runner starts.",
      defaultModelEmpty: "No routes configured. Add one via lthn config routes.NAME.kind=…",
      quantL: "Quantisation preference",
      quantH: "Pick the smallest quant your hardware comfortably runs. Applied when the runner loads a fresh model.",
      samplingL: "Default sampling",
      samplingH: "Per-model overrides live in the model browser.",
      httpServerL: "HTTP server",
      endpointL: "Endpoint",
      apiKeyL: "API key",
      apiKeyH: "Required for any client connecting to the local server.",
      intervalL: "Sample interval",
      intervalH: "How often the tray polls the runner. 2s is the calm-presence default.",
      heapL: "Heap samples",
      heapH: "Rolling window the sparkline draws from.",
      powerL: "Power metrics",
      powerH: "Requires the XPC helper (planned). Off today.",
      openIntL: "Open Integrations window",
      openIntH: "Manage Claude Code / OpenCode / Codex / Copilot / Raycast wiring.",
      btnOpen: "Open",
      versionL: "Version",
      versionH: "Binary release tag baked at build time.",
      goToolchainL: "Go toolchain",
      platformL: "Platform",
      cpusL: "CPUs",
      cpusH: "Logical cores Go's runtime sees.",
      emptyRoutes: "No routes configured. The runner falls back to an echo stub — useful for sanity checks but nothing real will answer.",
      footer: "Changes apply immediately · ⌘W to close · the runner keeps running",
    };
  }

  _setSampleInterval(v: string) {
    this.sampleInterval = v;
    writeStoredSetting("lthn.telemetry.interval", v);
  }

  _setHeapSamples(v: string) {
    this.heapSamples = v;
    writeStoredSetting("lthn.telemetry.samples", v);
  }

  /** Toggle Reveal/Hide for the API key. Reveal fetches the full
   *  token via Wails (in-process, no network); Hide flips back to
   *  the masked form. */
  async _toggleApiKey() {
    try {
      const ak = await import("@desktop/apikey/wailsservice");
      const { unwrap } = await import("../result");
      if (this.apiKeyRevealed) {
        const masked = await unwrap<string>(ak.Masked(), "");
        if (masked) this.apiKey = masked;
        this.apiKeyRevealed = false;
      } else {
        const full = await unwrap<string>(ak.Reveal(), "");
        if (full) this.apiKey = full;
        this.apiKeyRevealed = true;
      }
    } catch (err) {
      console.error("settings: apikey reveal toggle failed", err);
    }
  }

  /** Copy the currently-shown apiKey value to the clipboard. */
  async _copyApiKey() {
    try {
      // If the value is masked, fetch the real one for the copy —
      // there's no use in copying bullets.
      const ak = await import("@desktop/apikey/wailsservice");
      const full = this.apiKeyRevealed ? this.apiKey : await demand<string>(ak.Reveal());
      await navigator.clipboard.writeText(full);
    } catch (err) {
      console.error("settings: apikey copy failed", err);
    }
  }

  /** Rotate the API key — generates a new one, persists it, and
   *  reveals the new value. Old key keeps working until the server
   *  is restarted (it's baked into the bearer middleware at boot). */
  async _rotateApiKey() {
    try {
      const ak = await import("@desktop/apikey/wailsservice");
      const fresh = await demand<string>(ak.WRotate());
      if (fresh) {
        this.apiKey = fresh;
        this.apiKeyRevealed = true;
      }
    } catch (err) {
      console.error("settings: apikey rotate failed", err);
    }
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
    const [i18n, fl, runner, server, lemma] = await Promise.all([
      import("@lthn/i18n/coreservice"),
      import("@desktop/firstlaunch/wailsservice"),
      import("@desktop/runner/service"),
      import("@desktop/server/service"),
      import("@desktop/lemma/wailsservice"),
    ]);
    const [
      title, subtitleTpl, locales, currentLang, paths, routes, routeViews, build, addr, listening,
      pGT, pGD, pMnT, pMT, pMD, pRT, pRD, pAT, pAD, pTT, pTD, pIT, pID, pAbT, pAbD,
      rSwL, rSwH, rLgL, rLgH,
      rMlL, rMlH, rMlOH, rMlOI, rMlOC,
      rMyL, rMyH, rMyOT, rMyOO, rMyOX, rMyOV,
      rMtBtn, rMtHint,
      rMdL, rMdH, rBrw,
      rDmL, rDmH, rDmE, rQuL, rQuH, rSmL, rSmH,
      rHsL, rEpL, rAkL, rAkH,
      rItL, rItH, rHpL, rHpH, rPwL, rPwH,
      rOiL, rOiH, rOpn,
      rVrL, rVrH, rGoL, rPlL, rCpL, rCpH,
      emRt, foot,
    ] = await Promise.all([
      i18n.T("window.settings.title"),
      i18n.T("window.settings.subtitle"),
      i18n.AvailableLanguages(),
      i18n.Language(),
      fl.Paths().catch(() => null),
      unwrap<string[]>(runner.WModels(), []),
      unwrap<RouteView[]>(runner.WRoutes(), []),
      fl.Build().catch(() => null),
      unwrap<string>(server.WAddr(), ""),
      unwrap<boolean>(server.WListening(), false),
      i18n.T("window.settings.panel_general_title"),
      i18n.T("window.settings.panel_general_desc"),
      i18n.T("window.settings.subsection_menu_title"),
      i18n.T("window.settings.panel_models_title"),
      i18n.T("window.settings.panel_models_desc"),
      i18n.T("window.settings.panel_runner_title"),
      i18n.T("window.settings.panel_runner_desc"),
      i18n.T("window.settings.panel_api_title"),
      i18n.T("window.settings.panel_api_desc"),
      i18n.T("window.settings.panel_telemetry_title"),
      i18n.T("window.settings.panel_telemetry_desc"),
      i18n.T("window.settings.panel_integrations_title"),
      i18n.T("window.settings.panel_integrations_desc"),
      i18n.T("window.settings.panel_about_title"),
      i18n.T("window.settings.panel_about_desc"),
      i18n.T("window.settings.row_startwindow_label"),
      i18n.T("window.settings.row_startwindow_hint"),
      i18n.T("window.settings.row_language_label"),
      i18n.T("window.settings.row_language_hint"),
      i18n.T("window.settings.row_menulinks_label"),
      i18n.T("window.settings.row_menulinks_hint"),
      i18n.T("window.settings.row_menulinks_opt_hybrid"),
      i18n.T("window.settings.row_menulinks_opt_inwindow"),
      i18n.T("window.settings.row_menulinks_opt_collapsed"),
      i18n.T("window.settings.row_menulayout_label"),
      i18n.T("window.settings.row_menulayout_hint"),
      i18n.T("window.settings.row_menulayout_opt_toggle"),
      i18n.T("window.settings.row_menulayout_opt_open"),
      i18n.T("window.settings.row_menulayout_opt_closed"),
      i18n.T("window.settings.row_menulayout_opt_hover"),
      i18n.T("window.settings.btn_replay_menu_tour"),
      i18n.T("window.settings.row_menu_replay_hint"),
      i18n.T("window.settings.row_modeldir_label"),
      i18n.T("window.settings.row_modeldir_hint"),
      i18n.T("window.settings.btn_browse"),
      i18n.T("window.settings.row_defaultmodel_label"),
      i18n.T("window.settings.row_defaultmodel_hint"),
      i18n.T("window.settings.row_defaultmodel_empty"),
      i18n.T("window.settings.row_quantisation_label"),
      i18n.T("window.settings.row_quantisation_hint"),
      i18n.T("window.settings.row_sampling_label"),
      i18n.T("window.settings.row_sampling_hint"),
      i18n.T("window.settings.row_httpserver_label"),
      i18n.T("window.settings.row_endpoint_label"),
      i18n.T("window.settings.row_apikey_label"),
      i18n.T("window.settings.row_apikey_hint"),
      i18n.T("window.settings.row_interval_label"),
      i18n.T("window.settings.row_interval_hint"),
      i18n.T("window.settings.row_heap_label"),
      i18n.T("window.settings.row_heap_hint"),
      i18n.T("window.settings.row_power_label"),
      i18n.T("window.settings.row_power_hint"),
      i18n.T("window.settings.row_openint_label"),
      i18n.T("window.settings.row_openint_hint"),
      i18n.T("window.settings.btn_open"),
      i18n.T("window.settings.row_version_label"),
      i18n.T("window.settings.row_version_hint"),
      i18n.T("window.settings.row_gotoolchain_label"),
      i18n.T("window.settings.row_platform_label"),
      i18n.T("window.settings.row_cpus_label"),
      i18n.T("window.settings.row_cpus_hint"),
      i18n.T("window.settings.empty_routes"),
      i18n.T("window.settings.footer"),
    ]);
    this.panel = {
      generalT: pGT, generalD: pGD,
      menuT:    pMnT,
      modelsT: pMT, modelsD: pMD,
      runnerT: pRT, runnerD: pRD,
      apiT: pAT, apiD: pAD,
      telemetryT: pTT, telemetryD: pTD,
      integrationsT: pIT, integrationsD: pID,
      aboutT: pAbT, aboutD: pAbD,
    };
    this.row = {
      startWindowL: rSwL, startWindowH: rSwH,
      languageL: rLgL, languageH: rLgH,
      menuLinksL: rMlL, menuLinksH: rMlH,
      menuLinksOptHybrid: rMlOH, menuLinksOptInWindow: rMlOI, menuLinksOptCollapsed: rMlOC,
      menuLayoutL: rMyL, menuLayoutH: rMyH,
      menuLayoutOptToggle: rMyOT, menuLayoutOptOpen: rMyOO, menuLayoutOptClosed: rMyOX, menuLayoutOptHover: rMyOV,
      btnReplayMenuTour: rMtBtn, menuReplayH: rMtHint,
      modelDirL: rMdL, modelDirH: rMdH, btnBrowse: rBrw,
      defaultModelL: rDmL, defaultModelHint: rDmH, defaultModelEmpty: rDmE,
      quantL: rQuL, quantH: rQuH,
      samplingL: rSmL, samplingH: rSmH,
      httpServerL: rHsL, endpointL: rEpL,
      apiKeyL: rAkL, apiKeyH: rAkH,
      intervalL: rItL, intervalH: rItH,
      heapL: rHpL, heapH: rHpH,
      powerL: rPwL, powerH: rPwH,
      openIntL: rOiL, openIntH: rOiH, btnOpen: rOpn,
      versionL: rVrL, versionH: rVrH,
      goToolchainL: rGoL, platformL: rPlL, cpusL: rCpL, cpusH: rCpH,
      emptyRoutes: emRt,
      footer: foot,
    };
    this.locales = locales;
    this.currentLang = currentLang;
    if (paths?.OK) {
      const p = paths.Value as { models_dir?: string };
      if (p?.models_dir) this.modelsDir = collapseHome(p.models_dir);
    }
    this.routeNames = routes || [];
    this.routes = (routeViews || []) as RouteView[];
    // Lemma.Status fallback for the Default model display — when
    // no runner routes are configured but lthn-mlx has a model
    // loaded, the user DOES have a default. Best-effort: Lemma
    // unreachable / no model loaded leaves lemmaModel empty and
    // the panel falls through to the "no routes configured"
    // empty-state copy.
    try {
      const st = await lemma.Status();
      if (st?.model_path) {
        const p = st.model_path.replace(/\/+$/, "");
        const slash = p.lastIndexOf("/");
        this.lemmaModel = slash >= 0 ? p.slice(slash + 1) : p;
      }
      // Capture runtime + endpoint for the Runner pane's Local
      // Lemma card. Endpoint is the loopback admin convention since
      // ServeStatus doesn't carry it (the engine doesn't know its
      // own external URL).
      this.lemmaRuntime = st?.runtime || "";
      this.lemmaEndpoint = "http://127.0.0.1:11434";
    } catch { /* lemma down — keep empty */ }
    if (build?.OK) {
      this.build = build.Value as { version: string; go_version: string; goos: string; goarch: string; num_cpu: number };
    }
    if (addr) {
      const a = addr.startsWith(":") ? `localhost${addr}` : addr;
      this.endpoint = `http://${a}/v1`;
    }
    this.httpListening = !!listening;
    // Pull the masked form of the local bearer token. Settings → API
    // renders the masked value by default; the Reveal toggle swaps
    // it for the full string via the apikey.Reveal binding.
    try {
      const ak = await import("@desktop/apikey/wailsservice");
      const masked = await unwrap<string>(ak.Masked(), "");
      if (masked) this.apiKey = masked;
    } catch { /* keep design fallback */ }
    // Rebuild the subtitle with the real version so the chrome
    // reflects the running binary rather than the locale fixture.
    const subtitle = this.build.version
      ? `lthn · v${this.build.version}`
      : subtitleTpl;
    this.chrome = { title, subtitle };
  }

  /** Side-menu click handler — swap the body to the selected section.
   *  The endpoints section is the only one with backend-driven state
   *  that needs an explicit lazy-load on first open; everything else
   *  is either fixture-static or wired in connectedCallback. */
  _openSection(id: string) {
    this.open = id;
    if (id === "endpoints") {
      void this._loadEndpoints();
    }
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
      case "endpoints":    return this._sectionEndpoints();
      case "about":        return this._sectionAbout();
      case "leave":        return this._sectionLeave();
      default:             return this._sectionGeneral();
    }
  }

  /** Locale picker — flag + tag label. Click writes localStorage + calls SetLanguage. */
  async _setLang(lang: string) {
    const i18n = await import("@lthn/i18n/coreservice");
    await i18n.SetLanguage(lang);
    writeStoredSetting("lthn.locale", lang);
    this.currentLang = lang;
  }

  /** Boot mode toggle — write-through to localStorage. */
  _setStartWithWindow(on: boolean) {
    this.startWithWindow = on;
    writeStoredSetting("lthn.boot.window", on ? "true" : "false");
  }

  /** Menu Behaviours setters — write-through to localStorage and
   *  emit "lthn:menu:changed" so any open <lthn-app-shell> reacts
   *  without waiting for a reload. Wails Events fire same-window
   *  too, so the in-shell Settings panel updates the rail directly. */
  _setMenuLinks(v: string) {
    this.menuLinks = v;
    writeStoredSetting("lthn.menu.links", v);
    this._emitMenuChanged("links", v);
  }
  _setMenuLayout(v: string) {
    this.menuLayout = v;
    writeStoredSetting("lthn.menu.layout", v);
    this._emitMenuChanged("layout", v);
  }
  private _emitMenuChanged(setting: string, value: string) {
    import("@wailsio/runtime")
      .then(({ Events }) => (Events.Emit as (name: string, data: unknown) => Promise<boolean>)("lthn:menu:changed", { setting, value }))
      .catch(() => { /* events unavailable in test/canvas — settings still persist */ });
  }

  /** Re-open the welcome wizard at step 4 (Menu Behaviours tour).
   *  Hands the target step off via localStorage so a single Open()
   *  call is enough — the welcome window's constructor reads + clears
   *  the key on mount. Also clears the tour-completed flag so the
   *  Finish click writes it fresh. See plans/project/lthn/desktop/
   *  RFC.menu-behaviours.md § 6. */
  async _replayMenuTour() {
    try {
      writeStoredSetting("lthn.welcome.start-step", "4");
      writeStoredSetting("lthn.menu.tour-completed", "");
      const ws = await import("@desktop/desktop/windowservice");
      await ws.Open("welcome");
    } catch (err) {
      console.error("settings: replay menu tour failed", err);
    }
  }

  /** Flag for a given locale tag — keeps the picker visually intuitive. */
  _flag(lang: string): string {
    const l = lang.toLowerCase();
    if (l === "fr" || l.startsWith("fr-") || l.startsWith("fr_")) return "🇫🇷";
    if (l === "en-au" || l === "en_au") return "🇦🇺";
    if (l === "zh" || l.startsWith("zh-") || l.startsWith("zh_")) return "🇨🇳";
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
      { id:"endpoints",    icon:"fa-server",       label:"Endpoints" },
      { id:"about",        icon:"fa-circle-info",  label:"About" },
      { id:"leave",        icon:"fa-door-open",    label:"Leave App" },
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
      footer: html`${this.row.footer}`,
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
    const linksOptions: Array<[string, string]> = [
      ["hybrid",         this.row.menuLinksOptHybrid],
      ["in-window",      this.row.menuLinksOptInWindow],
      ["collapsed-only", this.row.menuLinksOptCollapsed],
    ];
    const layoutOptions: Array<[string, string]> = [
      ["toggle", this.row.menuLayoutOptToggle],
      ["open",   this.row.menuLayoutOptOpen],
      ["closed", this.row.menuLayoutOptClosed],
      ["hover",  this.row.menuLayoutOptHover],
    ];
    return this._section({
      title: this.panel.generalT,
      desc:  this.panel.generalD,
      content: html`
        ${this._row(this.row.startWindowL, this.row.startWindowH, html`
          <lthn-toggle ?on=${this.startWithWindow}
            @click=${() => this._setStartWithWindow(!this.startWithWindow)}>
          </lthn-toggle>
        `)}
        ${this._row(this.row.languageL, this.row.languageH, html`
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
        ${this._subsection(this.panel.menuT)}
        ${this._row(this.row.menuLinksL, this.row.menuLinksH,
          this._segmentPickKeyed(this.menuLinks, linksOptions, v => this._setMenuLinks(v)))}
        ${this._row(this.row.menuLayoutL, this.row.menuLayoutH,
          this._segmentPickKeyed(this.menuLayout, layoutOptions, v => this._setMenuLayout(v)))}
        ${this._row(this.row.btnReplayMenuTour, this.row.menuReplayH, html`
          <lthn-btn tone="quiet" size="sm" @click=${() => this._replayMenuTour()}>
            <i class="fa-regular fa-circle-play" style="font-size:10px; margin-right:4px;"></i>
            ${this.row.btnReplayMenuTour}
          </lthn-btn>
        `)}
      `,
    });
  }

  /** Sub-section heading — uppercase mono caption with a thin top
   *  divider. Used inside _section content to group related rows
   *  without spawning a whole new panel page. */
  _subsection(label: string) {
    return html`
      <div style="margin-top:14px; padding-top:14px;
                  border-top:1px solid rgba(255,255,255,0.06);
                  font-family:var(--font-mono); font-size:10px;
                  color:var(--fg-3); letter-spacing:0.08em;
                  text-transform:uppercase;">${label}</div>
    `;
  }

  /** Keyed variant of _segmentPick — options are [value, label]
   *  pairs so the persisted value can be a short ID while the
   *  displayed text is a human-readable (and translatable) string. */
  _segmentPickKeyed(value: string, options: Array<[string, string]>, onPick: (v: string) => void) {
    return html`
      <div style="display:inline-flex; border-radius:6px;
                  background:rgba(0,0,0,0.18); border:1px solid rgba(255,255,255,0.06); padding:2px;">
        ${options.map(([v, label]) => html`
          <button @click=${() => onPick(v)}
            style="padding:4px 10px; font-family:var(--font-sans); font-size:11px;
                   border:none; cursor:pointer;
                   color:${v === value ? "var(--fg-0)" : "var(--fg-3)"};
                   background:${v === value ? "rgba(255,255,255,0.08)" : "transparent"};
                   border-radius:4px; letter-spacing:0.01em;
                   --wails-draggable: no-drag;">${label}</button>
        `)}
      </div>
    `;
  }

  _sectionModels() {
    // Three-tier fallback: runner route first (configured intent),
    // then Lemma loaded model (engine truth), then the empty-state
    // copy. The hint text mirrors the source so the user knows
    // whether they're seeing a route name or the live engine model.
    const hasRoutes = this.routeNames.length > 0;
    const hasLemma  = !!this.lemmaModel;
    const defaultModel = hasRoutes ? this.routeNames[0]
                       : hasLemma  ? `${this.lemmaModel}  (lthn-mlx)`
                       :             "no routes configured";
    const defaultHint = hasRoutes ? this.row.defaultModelHint
                      : hasLemma  ? "Engine truth — what lthn-mlx is currently serving."
                      :             this.row.defaultModelEmpty;
    return this._section({
      title: this.panel.modelsT,
      desc:  this.panel.modelsD,
      content: html`
        ${this._row(this.row.modelDirL, this.row.modelDirH, html`
          <div style="display:flex; align-items:center; gap:8px; padding:6px 10px; border-radius:6px;
                      background:rgba(255,255,255,0.04); border:1px solid rgba(255,255,255,0.07);
                      font-family:var(--font-mono); font-size:11.5px; color:var(--fg-1);">
            <i class="fa-regular fa-folder" style="font-size:11px; color:var(--fg-3);"></i>
            ${this.modelsDir}
            <lthn-btn tone="quiet" size="sm" style="margin-left:4px;"
              @click=${() => import("@desktop/desktop/windowservice").then(w => w.Open("models"))}>
              ${this.row.btnBrowse}
            </lthn-btn>
          </div>
        `)}
        ${this._row(this.row.defaultModelL, defaultHint, this._select(defaultModel))}
        ${this._row(this.row.quantL, this.row.quantH,
          this._segment("q4_k_m", ["q4_0", "q4_k_m", "q5_k_m", "q8_0"]))}
        ${this._row(this.row.samplingL, this.row.samplingH, html`
          <div style="display:flex; gap:18px; font-size:11.5px; color:var(--fg-2);">
            <span>Temp <span style="color:var(--fg-0); font-family:var(--font-mono);">0.7</span></span>
            <span>Top-p <span style="color:var(--fg-0); font-family:var(--font-mono);">0.95</span></span>
            <span>Max tok <span style="color:var(--fg-0); font-family:var(--font-mono);">1024</span></span>
          </div>
        `)}
      `,
    });
  }

  /** Local Lemma card — rendered alongside runner routes when the
   *  engine is reachable. Tagged "lthn-mlx" to distinguish from
   *  external provider routes; uses the brand-tinted background to
   *  signal "this is the in-process default", not a configured route.
   *  Hidden when Lemma is down so the panel doesn't pretend an
   *  engine that isn't there. */
  private _renderLemmaCard() {
    if (!this.lemmaModel) return nothing;
    return html`
      <div style="padding:12px 14px; border-radius:8px;
                  background:rgba(64,193,197,0.06);
                  border:1px solid rgba(64,193,197,0.22);
                  display:flex; flex-direction:column; gap:6px;">
        <div style="display:flex; align-items:baseline; gap:8px;">
          <span style="font-family:var(--font-mono); font-size:13px; color:var(--fg-0); font-weight:500;">
            Local Lemma
          </span>
          <span style="font-size:10.5px; color:var(--brand-300); letter-spacing:0.04em; text-transform:uppercase;">
            · lthn-mlx${this.lemmaRuntime ? ` · ${this.lemmaRuntime}` : ""}
          </span>
        </div>
        <div style="display:grid; grid-template-columns:auto 1fr; gap:4px 12px;
                    font-family:var(--font-mono); font-size:11px; color:var(--fg-2);">
          <span style="color:var(--fg-3);">model</span>
          <span>${this.lemmaModel}</span>
          <span style="color:var(--fg-3);">endpoint</span>
          <span>${this.lemmaEndpoint || "—"}/v1</span>
        </div>
      </div>
    `;
  }

  _sectionRunner() {
    const hasRoutes = this.routes.length > 0;
    const hasLemma  = !!this.lemmaModel;
    return this._section({
      title: this.panel.runnerT,
      desc:  this.panel.runnerD,
      content: !hasRoutes && !hasLemma ? html`
        <div style="padding:14px 16px; border-radius:8px; background:rgba(255,255,255,0.025);
                    border:1px solid rgba(255,255,255,0.05); font-size:12px; color:var(--fg-3); line-height:1.55;">
          ${this.row.emptyRoutes}
        </div>
      ` : html`
        <div style="display:flex; flex-direction:column; gap:8px;">
          ${this._renderLemmaCard()}
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
      title: this.panel.apiT,
      desc:  this.panel.apiD,
      content: html`
        ${this._row(this.row.httpServerL, null, html`<lthn-toggle ?on=${this.httpListening}></lthn-toggle>`)}
        ${this._row(this.row.endpointL, null, html`
          <span style="font-family:var(--font-mono); font-size:11.5px; color:var(--fg-1);">${this.endpoint}</span>
        `)}
        ${this._row(this.row.apiKeyL, this.row.apiKeyH, html`
          <div style="display:flex; align-items:center; gap:6px;">
            <span style="font-family:var(--font-mono); font-size:11px; color:var(--fg-1); word-break:break-all;">${this.apiKey}</span>
            <lthn-btn tone="quiet" size="sm" @click=${() => this._toggleApiKey()}>
              <i class="fa-regular ${this.apiKeyRevealed ? "fa-eye-slash" : "fa-eye"}" style="font-size:10px;"></i>
            </lthn-btn>
            <lthn-btn tone="quiet" size="sm" @click=${() => this._copyApiKey()}>
              <i class="fa-regular fa-copy" style="font-size:10px;"></i>
            </lthn-btn>
            <lthn-btn tone="quiet" size="sm" @click=${() => this._rotateApiKey()}>
              <i class="fa-solid fa-rotate-right" style="font-size:10px;"></i>
            </lthn-btn>
          </div>
        `)}
      `,
    });
  }

  _sectionTelemetry() {
    return this._section({
      title: this.panel.telemetryT,
      desc:  this.panel.telemetryD,
      content: html`
        ${this._row(this.row.intervalL, this.row.intervalH,
          this._segmentPick(this.sampleInterval, ["1s", "2s", "5s", "off"], v => this._setSampleInterval(v)))}
        ${this._row(this.row.heapL, this.row.heapH,
          this._segmentPick(this.heapSamples, ["12", "24", "48", "96"], v => this._setHeapSamples(v)))}
        ${this._row(this.row.powerL, this.row.powerH, html`
          <lthn-toggle></lthn-toggle>
        `)}
      `,
    });
  }

  _sectionIntegrations() {
    return this._section({
      title: this.panel.integrationsT,
      desc:  this.panel.integrationsD,
      content: html`
        ${this._row(this.row.openIntL, this.row.openIntH, html`
          <lthn-btn tone="quiet" size="sm"
            @click=${() => import("@desktop/desktop/windowservice").then(w => w.Open("integrations"))}>
            <i class="fa-solid fa-arrow-up-right-from-square" style="font-size:10px;"></i> ${this.row.btnOpen}
          </lthn-btn>
        `)}
      `,
    });
  }

  /** Endpoints section — manages OpenAI-compatible inference endpoints
   *  (NIM / OpenRouter / vLLM / TGI / llama-server / LM Studio). Each
   *  configured endpoint becomes a registered Bencher in the benchmark
   *  substrate; the admin → Benchmark window's picker reflects them.
   *  Mantis #1775 Stage B. */
  _sectionEndpoints() {
    const list = this.endpoints || [];
    const empty = html`
      <div style="padding:14px; border:1px dashed rgba(255,255,255,0.08); border-radius:8px;
                  color:var(--fg-3); font-size:11.5px; line-height:1.55;">
        No endpoints configured. Add one to point lthn at any
        OpenAI-compatible inference server — NIM, OpenRouter, vLLM,
        TGI, llama-server, LM Studio, or your own.
      </div>
    `;
    const items = html`
      <div style="display:flex; flex-direction:column; gap:6px;">
        ${list.map(ep => html`
          <div style="display:grid; grid-template-columns: 1fr auto; gap:14px; align-items:center;
                      padding:10px 12px; border:1px solid rgba(255,255,255,0.06); border-radius:8px;
                      background:rgba(255,255,255,0.02);">
            <div style="display:flex; flex-direction:column; gap:3px; min-width:0;">
              <div style="display:flex; align-items:center; gap:8px;">
                <span style="font-size:12.5px; color:var(--fg-0); font-weight:500;">${ep.name}</span>
                ${ep.auth_set
                  ? html`<span style="font-size:9.5px; padding:1.5px 6px; border-radius:3px;
                                       background:rgba(64,193,197,0.10); color:var(--brand-300);
                                       letter-spacing:0.04em; text-transform:uppercase;
                                       font-family:var(--font-mono);">key set</span>`
                  : html`<span style="font-size:9.5px; padding:1.5px 6px; border-radius:3px;
                                       background:rgba(255,255,255,0.05); color:var(--fg-3);
                                       letter-spacing:0.04em; text-transform:uppercase;
                                       font-family:var(--font-mono);">no key</span>`}
              </div>
              <div style="font-family:var(--font-mono); font-size:10.5px; color:var(--fg-2);
                          overflow:hidden; text-overflow:ellipsis; white-space:nowrap;">${ep.url}</div>
              ${ep.description ? html`<div style="font-size:11px; color:var(--fg-3);">${ep.description}</div>` : nothing}
            </div>
            <lthn-btn tone="quiet" size="sm"
              data-testid=${"ep-delete-" + ep.name}
              @click=${() => void this._deleteEndpoint(ep.name)}
              title="Remove endpoint">
              <i class="fa-regular fa-trash-can" style="font-size:11px; color:var(--error-400);"></i>
            </lthn-btn>
          </div>
        `)}
      </div>
    `;
    return this._section({
      title: "Inference endpoints",
      desc:  "OpenAI-compatible endpoints register as Benchers in admin → Benchmark. Bearer tokens encrypt at rest via the platform key store.",
      content: html`
        ${list.length === 0 ? empty : items}
        <div>
          <lthn-btn tone="primary" size="sm"
            data-testid="ep-add-open"
            @click=${() => this._openAddModal()}>
            <i class="fa-solid fa-plus" style="font-size:10px;"></i> Add endpoint
          </lthn-btn>
        </div>
        ${this.addModalOpen ? this._renderAddModal() : nothing}
      `,
    });
  }

  /** Add modal — overlay form for creating a new endpoint. Inline
   *  rather than a separate component for v1 (low ceremony; the form
   *  is small + tightly coupled to _submitAddEndpoint). */
  _renderAddModal() {
    const f = this.addForm;
    const overlay = "position:fixed; inset:0; background:rgba(0,0,0,0.55); display:flex; align-items:center; justify-content:center; z-index:50;";
    const card = "width:440px; background:var(--surf-1); border:1px solid rgba(255,255,255,0.08); border-radius:10px; padding:20px; display:flex; flex-direction:column; gap:12px; box-shadow:0 12px 40px rgba(0,0,0,0.45);";
    const fieldStyle = "padding:7px 10px; border-radius:5px; background:rgba(0,0,0,0.22); border:1px solid rgba(255,255,255,0.06); color:var(--fg-0); font-size:11.5px; font-family:var(--font-mono); outline:none; width:100%; box-sizing:border-box;";
    const labelStyle = "font-size:11px; color:var(--fg-2); display:flex; flex-direction:column; gap:4px;";
    return html`
      <div style=${overlay} @click=${(e: Event) => e.target === e.currentTarget && this._closeAddModal()}>
        <div style=${card} data-testid="ep-add-modal" @click=${(e: Event) => e.stopPropagation()}>
          <div style="font-size:13.5px; font-weight:600; color:var(--fg-0);">Add OpenAI-compatible endpoint</div>
          <label style=${labelStyle}>
            Name
            <input style=${fieldStyle} data-testid="ep-add-name"
              .value=${f.name}
              placeholder="openai-compat:nim"
              @input=${(e: Event) => this._setAddForm("name", (e.target as HTMLInputElement).value)}>
          </label>
          <label style=${labelStyle}>
            URL (include /v1)
            <input style=${fieldStyle} data-testid="ep-add-url"
              .value=${f.url}
              placeholder="https://api.example/v1"
              @input=${(e: Event) => this._setAddForm("url", (e.target as HTMLInputElement).value)}>
          </label>
          <label style=${labelStyle}>
            Bearer (API key) — optional for loopback
            <input style=${fieldStyle} type="password" data-testid="ep-add-bearer"
              .value=${f.bearer}
              placeholder="sk-..."
              @input=${(e: Event) => this._setAddForm("bearer", (e.target as HTMLInputElement).value)}>
          </label>
          <label style=${labelStyle}>
            Org (optional)
            <input style=${fieldStyle} data-testid="ep-add-org"
              .value=${f.org}
              placeholder="org-id"
              @input=${(e: Event) => this._setAddForm("org", (e.target as HTMLInputElement).value)}>
          </label>
          <label style=${labelStyle}>
            Description (optional)
            <input style=${fieldStyle} data-testid="ep-add-desc"
              .value=${f.description}
              placeholder="My production NIM cluster"
              @input=${(e: Event) => this._setAddForm("description", (e.target as HTMLInputElement).value)}>
          </label>
          ${this.addErr ? html`
            <div style="font-size:11px; color:var(--error-400); padding:6px 8px; border-radius:5px;
                        background:rgba(248,113,113,0.06); border:1px solid rgba(248,113,113,0.18);">
              ${this.addErr}
            </div>
          ` : nothing}
          <div style="display:flex; justify-content:flex-end; gap:6px; padding-top:4px;">
            <lthn-btn tone="quiet" size="sm" @click=${() => this._closeAddModal()}>Cancel</lthn-btn>
            <lthn-btn tone="primary" size="sm"
              data-testid="ep-add-submit"
              ?disabled=${this.addSubmitting}
              @click=${() => void this._submitAddEndpoint()}>
              ${this.addSubmitting ? "Saving…" : "Save"}
            </lthn-btn>
          </div>
        </div>
      </div>
    `;
  }

  /** Pull current endpoints from the backend. Fail silently on
   *  binding-missing / unwrap-empty so the section degrades to the
   *  "no endpoints configured" empty state. */
  async _loadEndpoints(): Promise<void> {
    try {
      const m = await import("@desktop/openaibench/wailsservice");
      const list = await unwrap<PublicEndpoint[]>(m.ListEndpoints(), []);
      this.endpoints = Array.isArray(list) ? list : [];
    } catch {
      this.endpoints = [];
    }
  }

  _openAddModal() {
    this.addForm = { name: "", url: "", description: "", bearer: "", org: "" };
    this.addErr = "";
    this.addSubmitting = false;
    this.addModalOpen = true;
  }

  _closeAddModal() {
    this.addModalOpen = false;
    this.addErr = "";
    // Clear the form so the bearer doesn't linger in component state
    // longer than necessary — the secret is gone the moment the modal
    // closes (Save path consumes it before closing too).
    this.addForm = { name: "", url: "", description: "", bearer: "", org: "" };
  }

  _setAddForm(field: keyof AddEndpointForm, value: string) {
    this.addForm = { ...this.addForm, [field]: value };
  }

  async _submitAddEndpoint(): Promise<void> {
    if (this.addSubmitting) return;
    const f = this.addForm;
    if (!f.name || !f.url) {
      this.addErr = "Name and URL are required.";
      return;
    }
    this.addSubmitting = true;
    this.addErr = "";
    try {
      const m = await import("@desktop/openaibench/wailsservice");
      await demand<unknown>(m.AddEndpoint({
        name: f.name, url: f.url, description: f.description,
        bearer: f.bearer, org: f.org,
      } as Parameters<typeof m.AddEndpoint>[0]));
      this._closeAddModal();
      await this._loadEndpoints();
    } catch (e: unknown) {
      this.addErr = e instanceof Error ? e.message : String(e);
    } finally {
      this.addSubmitting = false;
    }
  }

  async _deleteEndpoint(name: string): Promise<void> {
    if (!name) return;
    try {
      const m = await import("@desktop/openaibench/wailsservice");
      await demand<unknown>(m.RemoveEndpoint(name));
      await this._loadEndpoints();
    } catch {
      // Re-load anyway so the UI reflects whatever the backend has.
      await this._loadEndpoints();
    }
  }

  _sectionAbout() {
    const mono = (v: string) => html`<span style="font-family:var(--font-mono); font-size:11.5px; color:var(--fg-1);">${v}</span>`;
    return this._section({
      title: this.panel.aboutT,
      desc:  this.panel.aboutD,
      content: html`
        ${this._row(this.row.versionL, this.row.versionH,
          mono(`v${this.build.version || "—"}`))}
        ${this._row(this.row.goToolchainL, null, mono(this.build.go_version || "—"))}
        ${this._row(this.row.platformL, null, mono(this.build.goos && this.build.goarch ? `${this.build.goos} · ${this.build.goarch}` : "—"))}
        ${this._row(this.row.cpusL, this.row.cpusH, mono(String(this.build.num_cpu || "—")))}
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

  /** The chill flex. Most people look here once and never again —
   *  the goal is "oh, this place is trustworthy" in 5 seconds, not
   *  a tech-spec listing. Lead with the data-ownership story; the
   *  paths come second; the encrypted-data caveat third. */
  _sectionLeave() {
    const home = "~/Lethean";
    const path = (p: string) => html`<span style="font-family:var(--font-mono); font-size:11.5px;
      color:var(--brand-300);
      background:rgba(64,193,197,0.06);
      border:1px solid rgba(64,193,197,0.18);
      border-radius:4px;
      padding:1px 6px;">${p}</span>`;
    const para = (children: unknown) => html`<p style="font-size:12.5px;
      color:var(--fg-1); line-height:1.65; margin:0;">${children}</p>`;

    return html`
      <div style="display:flex; flex-direction:column; gap:18px; max-width:680px;">

        <div style="display:flex; align-items:center; gap:12px;">
          <div style="width:40px; height:40px;
                      display:flex; align-items:center; justify-content:center;
                      background:rgba(64,193,197,0.08);
                      border:1px solid rgba(64,193,197,0.18);
                      border-radius:10px;">
            <i class="fa-solid fa-door-open" style="font-size:16px; color:var(--brand-300);"></i>
          </div>
          <div style="font-size:18px; font-weight:600; color:var(--fg-0);
                      letter-spacing:-0.01em;">
            Good news — we never hid your data.
          </div>
        </div>

        ${para(html`
          Lethean Desktop is <em>physically unable</em> to remove the
          ${path(home)} folder. Open it in Finder, browse what's there,
          delete at your leisure. There's no "delete account" button
          because there's no account.<sup style="color:var(--brand-300); margin-left:1px;">*</sup>
        `)}

        <div style="display:flex; flex-direction:column; gap:8px;
                    padding:14px 16px;
                    background:rgba(255,255,255,0.025);
                    border:1px solid rgba(255,255,255,0.06);
                    border-radius:10px;">
          <div style="font-size:11px; color:var(--fg-3);
                      text-transform:uppercase; letter-spacing:0.06em;">
            To purge desktop state, delete:
          </div>
          <div style="display:flex; flex-direction:column; gap:6px; padding-top:4px;">
            <div style="display:flex; align-items:baseline; gap:10px;">
              ${path("~/Lethean/data/lthn.duckdb")}
              <span style="font-size:11px; color:var(--fg-3);">tasks · agents · fleet · agent activity</span>
            </div>
            <div style="display:flex; align-items:baseline; gap:10px;">
              ${path("~/Lethean/data/lthn.db")}
              <span style="font-size:11px; color:var(--fg-3);">key-value store</span>
            </div>
            <div style="display:flex; align-items:baseline; gap:10px;">
              ${path("~/Lethean/data/keys/")}
              <span style="font-size:11px; color:var(--fg-3);">encrypted API keys + secrets</span>
            </div>
            <div style="display:flex; align-items:baseline; gap:10px;">
              ${path("~/Lethean/data/workspace/")}
              <span style="font-size:11px; color:var(--fg-3);">DuckDB scratch / staging</span>
            </div>
            <div style="display:flex; align-items:baseline; gap:10px;">
              ${path("~/Lethean/conf/")}
              <span style="font-size:11px; color:var(--fg-3);">settings, window state, layouts</span>
            </div>
            <div style="display:flex; align-items:baseline; gap:10px;">
              ${path("~/Lethean/wallets/")}
              <span style="font-size:11px; color:var(--fg-3);">Lethean wallet keystores</span>
            </div>
          </div>
        </div>

        ${para(html`
          Your encrypted data — we genuinely <em>can't tell you what's in
          there</em>. It's sealed at rest with keys we don't hold. If you
          want a cleartext export, you'll need your encryption keys
          (multi-user setups need every participant's key) and the
          Export Workspace tool, which decrypts whatever the supplied
          keys can open.
        `)}

        <div style="display:flex; gap:10px; align-items:center; flex-wrap:wrap;">
          <lthn-btn tone="primary" size="md" disabled
            title="Export Workspace ships in a follow-up — manual decrypt guide is live now.">
            <i class="fa-solid fa-file-export" style="font-size:11px;"></i>
            Export Workspace
            <span style="font-size:9.5px; opacity:0.7; margin-left:4px;">(coming soon)</span>
          </lthn-btn>
          <a href="https://lthn.ai/help/leave/decrypt" target="_blank" rel="noopener"
             style="font-size:12px; color:var(--brand-300); text-decoration:none;
                    display:inline-flex; align-items:center; gap:6px;
                    --wails-draggable: no-drag;">
            <i class="fa-solid fa-book" style="font-size:11px;"></i>
            Guide: decrypt with other tooling
            <i class="fa-solid fa-arrow-up-right-from-square" style="font-size:9px;"></i>
          </a>
        </div>

        <div style="margin-top:6px; padding-top:14px;
                    border-top:1px solid rgba(255,255,255,0.05);
                    font-size:11.5px; color:var(--fg-3); line-height:1.7;">
          No phone home. No cloud sync we control.<sup style="color:var(--brand-300); margin-left:1px;">*</sup>
          No "are you sure you want to leave?" funnel. Just files you own,
          in folders you can see.
        </div>

        <div style="font-size:10.5px; color:var(--fg-3); line-height:1.65;
                    padding:10px 12px;
                    background:rgba(255,255,255,0.018);
                    border-left:2px solid rgba(64,193,197,0.3);
                    border-radius:0 6px 6px 0;">
          <span style="color:var(--brand-300); font-weight:600;">*</span>
          If you install our Agentic plugin, this one gets nuanced:
          managed runners and inter-fleet sync use a lightweight account
          ref so your own bastions can pass sealed blobs between each
          other. We still don't hold the keys, the blobs stay sealed,
          the data stays yours — the ref just tells your machines which
          other machines are also yours.
        </div>

      </div>
    `;
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

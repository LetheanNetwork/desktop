/**
 * lthn — Lethean Desktop frontend entry
 *
 * Mounts the Lit primitives from Lethean-5 (the design canon Lit port).
 * Each window is a prop-driven custom element; see lit/ for the catalogue.
 *
 * The tray popover (P0) renders via renderChrome() at 400×560.
 * Expansion windows (chat, settings, etc.) open as transient surfaces;
 * the Go-side core/gui wrapper spawns them via Wails window APIs.
 */

import { html, nothing, render } from "lit";
import { renderChrome } from "./lit/index";
import "./lit/index";
import { runBootProbe } from "./lit/boot-probe";

const app = document.getElementById("app");
if (!app) throw new Error("missing #app mount point in index.html");
const params = new URLSearchParams(location.search);
const surface = params.get("surface") || "canvas";

// Top-level auth-gate overlay — fires on ANY 401 from apiFetch in ANY
// surface (tray, chat, settings, welcome, models, …). The Lethean-7
// auth-gate mount inside <lthn-app-shell> only catches 401s while the
// "app" surface is mounted; the dozen+ per-surface windows bypass the
// shell entirely. Without this overlay, those surfaces show the raw
// 401 JSON in the WebView (Snider's 2026-05-16 screenshots).
//
// Design: dispatch on AUTH_401_EVENT → inject <lthn-auth-gate> as an
// absolute-positioned overlay covering the entire viewport. On
// AUTH_OK_EVENT → remove the overlay so the underlying surface
// reappears. Same Lit element, same state machine — just mounted at
// document level instead of inside the shell.
//
// Per Cerberus #1465 — the gate's bootstrap-token lives in closure
// scope inside the element; this mount doesn't change that property.
(() => {
  let overlay: HTMLElement | null = null;
  // mount() accepts an optional options bag so the boot-probe error
  // path can pass through a headline + framed body — the gate's
  // errorMessage / errorBody are state-only Lit properties so they
  // reach the element via JS assignment, not via attribute.
  const mount = (
    state: "setup" | "auth" | "error",
    options?: { requestId?: string; errorMessage?: string; errorBody?: string },
  ) => {
    const reqId = options?.requestId;
    if (overlay) {
      overlay.setAttribute("state", state);
      if (reqId) overlay.setAttribute("request-id", reqId);
      if (options?.errorMessage !== undefined) {
        (overlay as HTMLElement & { errorMessage?: string }).errorMessage = options.errorMessage;
      }
      if (options?.errorBody !== undefined) {
        (overlay as HTMLElement & { errorBody?: string }).errorBody = options.errorBody;
      }
      return;
    }
    overlay = document.createElement("lthn-auth-gate");
    overlay.setAttribute("state", state);
    if (reqId) overlay.setAttribute("request-id", reqId);
    if (options?.errorMessage !== undefined) {
      (overlay as HTMLElement & { errorMessage?: string }).errorMessage = options.errorMessage;
    }
    if (options?.errorBody !== undefined) {
      (overlay as HTMLElement & { errorBody?: string }).errorBody = options.errorBody;
    }
    overlay.style.cssText =
      "position:fixed;inset:0;z-index:9999;background:var(--surf-0,#0a090e);" +
      "display:flex;align-items:stretch;justify-content:stretch;";
    document.body.appendChild(overlay);
  };
  const unmount = () => {
    if (overlay) { overlay.remove(); overlay = null; }
  };
  window.addEventListener("lthn:auth:401", (ev: Event) => {
    const detail = (ev as CustomEvent<{ requestId?: string }>).detail;
    mount("error", { requestId: detail?.requestId });
  });
  window.addEventListener("lthn:auth:ok", () => { unmount(); });
  // Boot-time probe — if account is absent, mount the setup gate
  // BEFORE the surface renders so the first paint is the gate, not
  // a flash of the underlying window.
  //
  // Mantis #1500 — fail-CLOSED: if the probe can't determine state
  // (binding missing, IPC rejected, Result.OK=false), mount("error")
  // with a friendly "Cannot reach lthn server" copy and write a
  // structured console.error breadcrumb. The previous silent-degrade
  // (`catch { /* binding missing → degrade silently */ }`) left the
  // underlying surface unprotected with NO visible signal.
  void runBootProbe(mount);
})();

// Security note — innerHTML writes below (Cerberus Mantis #1422, 2026-05-16).
//
// Every `app.innerHTML = \`<lthn-xxx-window ...>\`` write in this switch is safe
// because:
//   1. The tag names are hard-coded Lit custom element names — not interpolated
//      from URL params.
//   2. Attribute values (state, step, pane, tab, path, lang, code, etc.) come from
//      `params.get(...)` — `new URLSearchParams(location.search)`. The URL is set
//      by the Go Wails runtime via Window.SetURL(); network-reachable injection
//      requires bridge auth (Mantis #1423, shipped).
//   3. Attribute values land in Lit element *properties*, not as raw HTML content.
//      The browser HTML-parses the attribute string once; Lit's property-setter
//      receives a plain string — no second-parse, no re-execution.
//   4. Cases where URL params are sensitive (path, code) use the safe imperative
//      pattern: `createElement` + direct property assignment, not innerHTML.
//
// Do NOT add `unsafeHTML` or `innerHTML` writes here that interpolate user-supplied
// content (e.g. chat messages, model names, file content). Those belong in Lit
// templates with standard text-binding interpolation, which auto-escapes.
// The CSP injected by pkg/server (script-src 'self') is the second defence layer.

switch (surface) {
  case "tray": {
    /* P0 tray popover — 400×560, composed inline from Lit primitives
     * (chrome.js). Polls TelemetryService + RunnerService every 2s
     * and re-renders. No custom element — render() + setInterval.
     *
     * Real binding wires:
     *   - model name      ← RunnerService.Models()[0]
     *   - uptime / heap   ← TelemetryService.Sample()
     *   - sparkline data  ← heap_alloc_mb samples (last 24)
     *   - connection dot  ← Sample() throwing → err; success → ok
     */
    const [telemetry, runner, windowSvc, i18n, fl, lemmaSvc] = await Promise.all([
      import("@desktop/telemetry/service"),
      import("@desktop/runner/service"),
      import("@desktop/desktop/windowservice"),
      import("@lthn/i18n/coreservice"),
      import("@desktop/firstlaunch/wailsservice"),
      import("@desktop/lemma/wailsservice"),
    ]);
      const TelemetryService = telemetry;
      const RunnerService = runner;
      const WindowService = windowSvc;
      const Lemma = lemmaSvc;
      /* Open a named window via the Go-side WindowService. Names are
       * the same keys in pkg/desktop/windows.go's registry — chat,
       * models, settings, welcome, about. */
      const openWindow = (name: string) => () => {
        WindowService.Open(name).catch((err: unknown) => {
          console.error(`open ${name} failed:`, err);
        });
      };

      /* Open the unified app shell ("app" window) and tell it which
       * pane to activate. The mini single-purpose windows
       * (chat / models / telemetry / etc.) stay registered for the
       * systray right-click menu — this just routes the popover-panel
       * Open buttons through the full shell instead.
       *
       * lthn-app-shell subscribes to "lthn:app:setpane" and assigns
       * the data to its `active` property. Open is awaited so the
       * Wails event lands AFTER the shell mounts. */
      const openAppPane = (pane: string) => async () => {
        try {
          await WindowService.Open("app");
          const events = await import("@wailsio/runtime").then(m => m.Events);
          events.Emit("lthn:app:setpane", pane);
        } catch (err: unknown) {
          console.error(`open app pane ${pane} failed:`, err);
        }
      };

      /* Locale strings — bulk-resolved via the Wails I18nService
       * binding. The bridge is in-process so the calls are µs-fast.
       * Wrapped in loadStrings() so the flag-button switcher can
       * recall it after SetLanguage() to refresh the tray in-place. */
      const loadStrings = async () => ({
        chromeTitle:      await i18n.T("tray.chrome.title"),
        chromeReady:      await i18n.T("tray.chrome.subtitle_ready"),
        chromeOffline:    await i18n.T("tray.chrome.subtitle_offline"),
        statusOffline:    await i18n.T("tray.status.offline"),
        statusConnecting: await i18n.T("tray.status.connecting"),
        statusPickModel:  await i18n.T("tray.status.pick_model"),
        statusBrowse:     await i18n.T("tray.status.browse"),
        heap:             await i18n.T("tray.heap"),
        uptime:           await i18n.T("tray.uptime"),
        sectionOpen:      await i18n.T("tray.section.open"),
        openChat:         await i18n.T("tray.open.chat"),
        openModels:       await i18n.T("tray.open.models"),
        openTelemetry:    await i18n.T("tray.open.telemetry"),
        openFleet:        await i18n.T("tray.open.fleet"),
        tabSystem:        await i18n.T("tray.tab.system"),
        tabRunner:        await i18n.T("tray.tab.runner"),
        tabActivity:      await i18n.T("tray.tab.activity"),
        kvConnection:     await i18n.T("tray.kv.connection"),
        kvSamples:        await i18n.T("tray.kv.samples"),
        kvModel:          await i18n.T("tray.kv.model"),
        kvStatus:         await i18n.T("tray.kv.status"),
        kvThroughput:     await i18n.T("tray.kv.throughput"),
        kvCache:          await i18n.T("tray.kv.kv_cache"),
        kvSessionsToday:  await i18n.T("tray.kv.sessions_today"),
        kvTokens:         await i18n.T("tray.kv.tokens_generated"),
        kvLastInteract:   await i18n.T("tray.kv.last_interaction"),
        kvRecentErrors:   await i18n.T("tray.kv.recent_errors"),
        valOffline:       await i18n.T("tray.value.offline"),
        valLive:          await i18n.T("tray.value.live"),
        valConnecting:    await i18n.T("tray.value.connecting"),
        valLoaded:        await i18n.T("tray.value.loaded"),
        valIdle:          await i18n.T("tray.value.idle"),
        valDash:          await i18n.T("tray.value.dash"),
        valNoModel:       await i18n.T("tray.value.no_model_loaded"),
        relSec:           await i18n.T("tray.rel.sec"),
        relMin:           await i18n.T("tray.rel.min"),
        relHr:            await i18n.T("tray.rel.hr"),
        relDay:           await i18n.T("tray.rel.day"),
        tbOpenApp:        await i18n.T("tray.titlebar.open_app"),
        tbSettings:       await i18n.T("tray.titlebar.settings"),
        // Footer version reads from firstlaunch.Build().version when
        // present so the tray reflects the running binary; falls back
        // to the design literal so canvas preview reads coherently.
        footerVersion:    (await i18n.T("tray.footer.version")).replace(
          "%s",
          await fl.Build().then(b => {
            const v = (b?.OK ? (b.Value as { version?: string })?.version : undefined);
            return v || "0.1.0";
          }).catch(() => "0.1.0"),
        ),
      });
      /* Locale state for the flag-button switcher. Available locales
       * come from the binding's AvailableLanguages — for the demo
       * surface today that's en + en-au; the flag cycles through
       * them. Choice persists across restarts via localStorage under
       * "lthn.locale"; config-service-backed persistence is the next
       * step once we want sync across surfaces.
       *
       * MUST run before the first loadStrings() so the prefetched
       * cache reflects the user's chosen locale, not the env-default. */
      const LOCALE_KEY = "lthn.locale";
      const availableLangs = await i18n.AvailableLanguages();
      let currentLang = await i18n.Language();
      const saved = localStorage.getItem(LOCALE_KEY);
      if (saved && availableLangs.includes(saved) && saved !== currentLang) {
        await i18n.SetLanguage(saved);
        currentLang = saved;
      }

      let t = await loadStrings();
      const flagFor = (lang: string): string => {
        const l = lang.toLowerCase();
        if (l === "fr" || l.startsWith("fr-") || l.startsWith("fr_")) return "🇫🇷";
        if (l === "en-au" || l === "en_au") return "🇦🇺";
        return "🇬🇧";  // en, en-gb, en_gb default to UK
      };
      const cycleLanguage = async () => {
        if (!availableLangs.length) return;
        const idx = availableLangs.indexOf(currentLang);
        const next = availableLangs[(idx + 1) % availableLangs.length];
        await i18n.SetLanguage(next);
        currentLang = next;
        localStorage.setItem(LOCALE_KEY, next);
        t = await loadStrings();
        draw();
      };

      type TrayTab = "system" | "runner" | "activity";
      interface TrayState {
        model:        string;
        uptime:       number;
        heapMb:       number;
        samples:      number[];   // rolling heap_alloc_mb window
        connected:    boolean;
        err:          string | null;
        tab:          TrayTab;    // active info-card tab
        sessionsToday: number;    // count of sessions created since midnight
        lastInteract: number;     // max updated_at across sessions (unix sec)
        // Lemma serve (local lthn-mlx engine) — separate from desktop
        // process telemetry above. lemmaConnected goes true only when
        // Lemma.Status() succeeds AND model_path is set; falls back to
        // RunnerService.WModels() for model name when Lemma isn't up.
        lemmaConnected: boolean;
        modelUptime:    number;   // seconds since Lemma loaded_at_unix
        modelRuntime:   string;   // e.g. "metal", "cuda", "rocm"
        // SFT job awareness — when Lemma.SFTStatus("") returns a job
        // in state "running", surface its epoch/loss in the runner
        // panel. State stays "running" only for the active job; the
        // last-completed slot returns terminal states (done/failed/
        // stopped) which we collapse back to "no SFT" for the tray.
        sftRunning:  boolean;
        sftEpoch:    number;
        sftLastLoss: number;
      }
      const state: TrayState = {
        model: "…",
        uptime: 0,
        heapMb: 0,
        samples: [],
        connected: false,
        err: null,
        tab: "system",
        sessionsToday: 0,
        lastInteract: 0,
        lemmaConnected: false,
        modelUptime: 0,
        modelRuntime: "",
        sftRunning: false,
        sftEpoch: 0,
        sftLastLoss: 0,
      };

      const setTab = (t: TrayTab) => () => { state.tab = t; draw(); };

      const fmtUptime = (s: number) => {
        if (s < 60) return `${Math.trunc(s)}s`;
        if (s < 3600) return `${Math.trunc(s / 60)}m ${Math.trunc(s % 60)}s`;
        return `${Math.trunc(s / 3600)}h ${Math.trunc((s % 3600) / 60)}m`;
      };

      /** "5m ago" / "2h ago" / "3d ago" from a unix-second timestamp.
       *  Drives the Activity tab's "Last interact" slot — mirrors the
       *  chat-window left rail's age-bucket reasoning, but in a compact
       *  relative form the tray panel can fit. */
      const fmtRel = (ts: number) => {
        const ageSec = Math.max(0, Math.floor(Date.now() / 1000) - ts);
        if (ageSec < 60)    return t.relSec.replace("%d", String(ageSec));
        if (ageSec < 3600)  return t.relMin.replace("%d", String(Math.trunc(ageSec / 60)));
        if (ageSec < 86400) return t.relHr.replace("%d", String(Math.trunc(ageSec / 3600)));
        return t.relDay.replace("%d", String(Math.trunc(ageSec / 86400)));
      };

      const connectionVariant = () => {
        if (state.err) return "err";
        if (state.connected) return "ok";
        return "idle";
      };

      const connectionLabel = () => {
        if (state.err) return t.valOffline;
        if (state.connected) return t.valLive;
        return t.valConnecting;
      };

      const headlineLabel = () => {
        if (state.connected) return state.model;
        if (state.err) return t.statusOffline;
        return t.statusConnecting;
      };

      const renderHeroStats = (hasModel: boolean) => {
        if (hasModel) {
          return html`
            <div style="display:grid; grid-template-columns: 1fr 1fr; gap:10px;">
              <div>
                <div style="font-family:var(--font-mono); font-size:9.5px;
                            color:var(--fg-3); letter-spacing:0.06em; text-transform:uppercase;">${t.heap}</div>
                <div style="font-family:var(--font-mono); font-size:13px;
                            color:var(--fg-0); margin-top:2px;">${state.heapMb.toFixed(1)} <span style="color:var(--fg-3); font-size:10.5px;">MB</span></div>
              </div>
              <div>
                <div style="font-family:var(--font-mono); font-size:9.5px;
                            color:var(--fg-3); letter-spacing:0.06em; text-transform:uppercase;">${t.uptime}</div>
                <div style="font-family:var(--font-mono); font-size:13px;
                            color:var(--fg-0); margin-top:2px;">${fmtUptime(state.uptime)}</div>
              </div>
            </div>
          `;
        }
        if (!state.connected) return nothing;
        return html`
          <div style="display:flex; align-items:center; gap:10px;
                      padding:8px 10px; border-radius:6px;
                      background:rgba(64,193,197,0.06);
                      border:1px dashed rgba(64,193,197,0.22);">
            <i class="fa-solid fa-cube" style="font-size:11px; color:var(--brand-300);"></i>
            <div style="flex:1; font-size:11.5px; color:var(--fg-1);">${t.statusPickModel}</div>
            <lthn-btn tone="quiet" size="sm" @click=${openAppPane("models")}>${t.statusBrowse}</lthn-btn>
          </div>
        `;
      };

      const renderSparkline = (sparkData: string, sparkMax: number) => {
        if (state.samples.length <= 1) return nothing;
        return html`<lthn-sparkline width="340" height="20" data=${sparkData} max=${sparkMax} fill></lthn-sparkline>`;
      };

      const renderHeroCard = (
        variant: string,
        stateLabel: string,
        hasModel: boolean,
        sparkData: string,
        sparkMax: number,
      ) => html`
        <section style="display:flex; flex-direction:column; gap:10px;
                        padding:12px 14px;
                        background:rgba(255,255,255,0.025);
                        border:1px solid rgba(255,255,255,0.06);
                        border-radius:9px;">
          <div style="display:flex; align-items:center; gap:10px; min-width:0;">
            <lthn-status-dot variant=${variant} ?pulse=${state.connected}></lthn-status-dot>
            <div style="font-size:14px; font-weight:600; letter-spacing:-0.005em;
                        color:${hasModel ? "var(--fg-0)" : "var(--fg-2)"};
                        flex:1; min-width:0;
                        overflow:hidden; text-overflow:ellipsis; white-space:nowrap;">
              ${headlineLabel()}
            </div>
            ${state.err
              ? html`<lthn-state-pill variant="disconnected">${stateLabel}</lthn-state-pill>`
              : nothing}
          </div>

          ${renderHeroStats(hasModel)}
          ${renderSparkline(sparkData, sparkMax)}
        </section>
      `;

      const renderOpenSection = () => html`
        <section style="display:flex; flex-direction:column; gap:8px;">
          <lthn-label>${t.sectionOpen}</lthn-label>
          <div style="display:grid; grid-template-columns: 1fr 1fr; gap:6px;">
            <lthn-btn tone="ghost" size="md" @click=${openAppPane("chat")}>
              <i class="fa-regular fa-comment" style="font-size:11px;"></i>
              ${t.openChat}
            </lthn-btn>
            <lthn-btn tone="ghost" size="md" @click=${openAppPane("fleet")}>
              <i class="fa-solid fa-server" style="font-size:11px;"></i>
              ${t.openFleet}
            </lthn-btn>
            <lthn-btn tone="ghost" size="md" @click=${openAppPane("models")}>
              <i class="fa-solid fa-cube" style="font-size:11px;"></i>
              ${t.openModels}
            </lthn-btn>
            <lthn-btn tone="ghost" size="md" @click=${openAppPane("telemetry")}>
              <i class="fa-solid fa-wave-square" style="font-size:11px;"></i>
              ${t.openTelemetry}
            </lthn-btn>
          </div>
        </section>
      `;

      const tabBtn = (id: TrayTab, label: string, icon: string) => {
        const on = state.tab === id;
        return html`
          <button
            @click=${setTab(id)}
            style="
              flex:1;
              display:inline-flex; align-items:center; justify-content:center; gap:6px;
              padding:6px 8px;
              font-size:11px; font-weight:${on ? 600 : 500};
              color:${on ? "var(--fg-0)" : "var(--fg-2)"};
              background:${on ? "rgba(255,255,255,0.06)" : "transparent"};
              border:1px solid ${on ? "rgba(64,193,197,0.22)" : "transparent"};
              border-radius:6px;
              cursor:pointer;
              --wails-draggable: no-drag;
            ">
            <i class="fa-solid ${icon}" style="font-size:10px; color:${on ? "var(--brand-300)" : "var(--fg-3)"};"></i>
            ${label}
          </button>
        `;
      };

      const kv = (k: string, v: string | number, mono = true) => html`
        <div style="display:flex; align-items:baseline; justify-content:space-between; gap:10px; padding:4px 0;">
          <span style="font-size:11px; color:var(--fg-3);">${k}</span>
          <span style="font-family:${mono ? "var(--font-mono)" : "var(--font-sans)"}; font-size:11.5px; color:var(--fg-1);">${v}</span>
        </div>
      `;

      const renderSystemPanel = (stateLabel: string) => html`
        ${kv(t.heap, `${state.heapMb.toFixed(1)} MB`)}
        ${kv(t.uptime, fmtUptime(state.uptime))}
        ${kv(t.kvConnection, stateLabel)}
        ${kv(t.kvSamples, `${state.samples.length} / 24`)}
      `;

      const renderRunnerPanel = (hasModel: boolean) => html`
        ${kv(t.kvModel, hasModel ? state.model : t.valDash, false)}
        ${kv(t.kvStatus, hasModel ? t.valLoaded : t.valIdle)}
        ${state.lemmaConnected
          ? kv(t.uptime, fmtUptime(state.modelUptime))
          : nothing}
        ${state.lemmaConnected && state.modelRuntime
          ? kv("runtime", state.modelRuntime)
          : nothing}
        ${state.sftRunning
          ? kv("training", `epoch ${state.sftEpoch} · loss ${state.sftLastLoss.toFixed(3)}`)
          : nothing}
        ${kv(t.kvThroughput, t.valDash)}
        ${kv(t.kvCache, t.valDash)}
      `;

      const renderActivityPanel = () => html`
        ${kv(t.kvSessionsToday, state.sessionsToday > 0 ? state.sessionsToday : t.valDash)}
        ${kv(t.kvTokens, t.valDash)}
        ${kv(t.kvLastInteract, state.lastInteract > 0 ? fmtRel(state.lastInteract) : t.valDash)}
        ${kv(t.kvRecentErrors, state.err ? "1" : "0")}
      `;

      const renderActivePanel = (hasModel: boolean, stateLabel: string) => {
        if (state.tab === "system") return renderSystemPanel(stateLabel);
        if (state.tab === "runner") return renderRunnerPanel(hasModel);
        return renderActivityPanel();
      };

      const renderInfoCard = (hasModel: boolean, stateLabel: string) => html`
        <section style="display:flex; flex-direction:column; gap:8px;
                        padding:8px 10px 10px;
                        background:rgba(255,255,255,0.018);
                        border:1px solid rgba(255,255,255,0.05);
                        border-radius:8px;">
          <div style="display:flex; gap:4px;">
            ${tabBtn("system",   t.tabSystem,   "fa-microchip")}
            ${tabBtn("runner",   t.tabRunner,   "fa-bolt")}
            ${tabBtn("activity", t.tabActivity, "fa-clock-rotate-left")}
          </div>
          <div style="padding:2px 4px;">
            ${renderActivePanel(hasModel, stateLabel)}
          </div>
        </section>
      `;

      const titlebarAction = (icon: string, title: string, onClick: () => void) => html`
        <button
          @click=${onClick}
          title=${title}
          style="
            display:inline-flex; align-items:center; justify-content:center;
            width:24px; height:24px;
            background:transparent;
            border:1px solid transparent;
            border-radius:5px;
            color:var(--fg-2);
            cursor:pointer;
            --wails-draggable: no-drag;
          "
          onmouseover="this.style.background='rgba(255,255,255,0.05)'; this.style.color='var(--fg-0)';"
          onmouseout="this.style.background='transparent'; this.style.color='var(--fg-2)';">
          <i class="fa-solid ${icon}" style="font-size:11px;"></i>
        </button>
      `;

      const renderFlagAction = () => html`
        <button
          @click=${cycleLanguage}
          title=${currentLang}
          style="
            display:inline-flex; align-items:center; justify-content:center;
            width:24px; height:24px;
            background:transparent;
            border:1px solid transparent;
            border-radius:5px;
            cursor:pointer;
            font-size:16px;
            line-height:1;
            --wails-draggable: no-drag;
          "
          onmouseover="this.style.background='rgba(255,255,255,0.05)';"
          onmouseout="this.style.background='transparent';">
          ${flagFor(currentLang)}
        </button>
      `;

      const renderTitlebarActions = () => html`
        ${titlebarAction("fa-display", t.tbOpenApp,  openWindow("app"))}
        ${titlebarAction("fa-gear",    t.tbSettings, openWindow("settings"))}
        ${renderFlagAction()}
      `;

      const renderTrayBody = (heroCard: unknown, infoCard: unknown, openSection: unknown) => html`
        <div style="display:flex; flex-direction:column; gap:14px; padding:14px; flex:1; min-height:0; overflow-y:auto; overscroll-behavior: none;">
          ${heroCard}
          ${infoCard}
          ${openSection}
        </div>
      `;

      const renderTrayFooter = (variant: string, stateLabel: string) => html`
        <span style="opacity:0.7;">${t.footerVersion}</span>
        <span style="flex:1"></span>
        <lthn-status-dot variant=${variant}></lthn-status-dot>
        <span style="opacity:0.7;">${stateLabel}</span>
      `;

      const draw = () => {
        const variant = connectionVariant();
        const stateLabel = connectionLabel();
        const sparkData = state.samples.length ? state.samples.join(",") : "";
        const sparkMax = Math.max(1, ...state.samples) * 1.2;
        const hasModel = !!(state.connected && state.model && state.model !== t.valNoModel);
        const heroCard = renderHeroCard(variant, stateLabel, hasModel, sparkData, sparkMax);

        const openSection = renderOpenSection();

        const infoCard = renderInfoCard(hasModel, stateLabel);

        render(renderChrome({
          title: t.chromeTitle,
          subtitle: state.err ? t.chromeOffline : t.chromeReady,
          w: 400, h: 560,
          actions: renderTitlebarActions(),
          body: renderTrayBody(heroCard, infoCard, openSection),
          footer: renderTrayFooter(variant, stateLabel),
        }), app);
      };

      const basename = (p: string): string => {
        if (!p) return "";
        const slash = p.lastIndexOf("/");
        return slash >= 0 ? p.slice(slash + 1) : p;
      };

      const poll = async () => {
        try {
          const sessions = await import("@desktop/sessions/wailsservice");
          const { unwrap, demand } = await import("./lit/result");
          type Reading = { uptime_seconds?: number; heap_alloc_mb?: number };
          // Lemma.Status is best-effort — the local lthn-mlx serve may
          // not be running. Treat any failure as "no model loaded" and
          // fall back to RunnerService.WModels for the legacy display.
          type LemmaStatus = { model_path?: string; runtime?: string; loaded_at_unix?: number };
          type LemmaSFTJob = { state?: string; epoch?: number; last_loss?: number };
          const [reading, models, sessionList, lemmaStatus, sftJob] = await Promise.all([
            demand<Reading>(TelemetryService.CurrentSample()),
            unwrap<string[]>(RunnerService.WModels(), []),
            unwrap<unknown[]>(sessions.List(), []),
            Lemma.Status().then(
              (s: unknown) => s as LemmaStatus,
              () => null as LemmaStatus | null,
            ),
            // SFTStatus("") returns the active job (or last completed)
            // — 404 when no job has ever run. Both are valid "no SFT
            // running right now" signals; only state === "running"
            // surfaces in the tray.
            Lemma.SFTStatus("").then(
              (j: unknown) => j as LemmaSFTJob,
              () => null as LemmaSFTJob | null,
            ),
          ]);
          state.connected = true;
          state.err = null;
          state.uptime = reading.uptime_seconds || 0;
          state.heapMb = reading.heap_alloc_mb || 0;
          state.samples.push(state.heapMb);
          if (state.samples.length > 24) state.samples.shift();
          // Lemma takes precedence over WModels — it's the authoritative
          // local engine state. Falls back to WModels when Lemma is
          // unavailable (engine not started / admin token missing).
          if (lemmaStatus && lemmaStatus.model_path) {
            state.lemmaConnected = true;
            state.model = basename(lemmaStatus.model_path);
            state.modelRuntime = lemmaStatus.runtime || "";
            state.modelUptime = lemmaStatus.loaded_at_unix
              ? Math.max(0, Math.floor(Date.now() / 1000) - lemmaStatus.loaded_at_unix)
              : 0;
          } else {
            state.lemmaConnected = false;
            state.modelUptime = 0;
            state.modelRuntime = "";
            state.model = models?.[0] || t.valNoModel;
          }
          // SFT awareness — only "running" is interesting in the tray;
          // terminal states (done/failed/stopped) collapse to "no SFT"
          // so the panel doesn't carry stale training noise after the
          // job ends.
          if (sftJob && sftJob.state === "running") {
            state.sftRunning = true;
            state.sftEpoch = sftJob.epoch || 0;
            state.sftLastLoss = sftJob.last_loss || 0;
          } else {
            state.sftRunning = false;
            state.sftEpoch = 0;
            state.sftLastLoss = 0;
          }
          // Activity-panel "Sessions today" count — sessions created
          // since midnight local time. SessionInfo.created_at is a
          // Unix second (matches go-store).
          const midnight = new Date();
          midnight.setHours(0, 0, 0, 0);
          const startSec = Math.floor(midnight.getTime() / 1000);
          state.sessionsToday = (sessionList || []).filter(
            (s: unknown) => (s as { created_at?: number }).created_at !== undefined
              && ((s as { created_at: number }).created_at >= startSec),
          ).length;
          // Activity-panel "Last interact" — max updated_at across all
          // sessions, regardless of bucket. Drawing falls back to the
          // design "—" when no sessions exist.
          state.lastInteract = (sessionList || []).reduce(
            (max: number, s: unknown) => {
              const u = (s as { updated_at?: number }).updated_at || 0;
              return Math.max(u, max);
            },
            0,
          );
        } catch (e: unknown) {
          state.connected = false;
          state.err = e instanceof Error ? e.message : String(e);
        }
        draw();
      };

      draw();
      poll();
      // Tray polling cadence reads from the same localStorage key
      // Settings → Telemetry writes. "off" → fixed first-sample only.
      const v = localStorage.getItem("lthn.telemetry.interval") || "2s";
      const intervalMatch = /^(\d+)s$/.exec(v);
      const intervalSeconds = intervalMatch?.[1] || "2";
      const ms = v === "off" ? 0 : (parseInt(intervalSeconds, 10) * 1000);
      if (ms > 0) {
        const id = setInterval(poll, ms);
        window.addEventListener("beforeunload", () => clearInterval(id));
      }
    break;
  }
  case "chat": {
    const state = params.get("state") || "multi-turn";
    app.innerHTML = `<lthn-chat-window state="${state}"></lthn-chat-window>`;
    break;
  }
  case "app": {
    /* Lethean-6 application shell — single frameless main window with
     * titlebar + side-nav + body that auto-mounts the matching window
     * for the `active` nav entry. ?pane=chat|models|settings|... picks
     * the initial view (explicit URL overrides everything); without
     * it, the shell's constructor falls back to the last-active pane
     * stored in localStorage (so a restart lands the user back where
     * they were), or "chat" on first-launch. */
    const pane = params.get("pane");
    app.innerHTML = pane
      ? `<lthn-app-shell active="${pane}"></lthn-app-shell>`
      : `<lthn-app-shell></lthn-app-shell>`;
    break;
  }
  case "welcome": {
    const step = params.get("step") || "1";
    app.innerHTML = `<lthn-welcome-window step="${step}"></lthn-welcome-window>`;
    break;
  }
  case "settings": {
    const open = params.get("open") || "general";
    app.innerHTML = `<lthn-settings-window open="${open}"></lthn-settings-window>`;
    break;
  }
  case "about": {
    /* About is a panel inside the settings window — the About card
     * lives in _sectionAbout (version, runtime, platform, licence,
     * source). The dedicated `about` Wails window registration just
     * pre-opens settings at the right panel, no separate component. */
    app.innerHTML = `<lthn-settings-window open="about"></lthn-settings-window>`;
    break;
  }
  case "models": {
    app.innerHTML = `<lthn-model-browser-window></lthn-model-browser-window>`;
    break;
  }
  case "lemma": {
    await import("./lit/ext/lemma-window");
    app.innerHTML = `<lthn-lemma-window></lthn-lemma-window>`;
    break;
  }
  case "editor": {
    await import("./lit/ide/editor-window");
    const path = params.get("path") || "scratch.ts";
    const lang = params.get("lang") || "typescript";
    const ro = params.get("readonly") === "1";
    const el = document.createElement("lthn-editor-window") as HTMLElement & {
      path: string; language: string; readonly: boolean;
    };
    el.path = path;
    el.language = lang;
    el.readonly = ro;
    app.innerHTML = "";
    app.appendChild(el);
    break;
  }
  case "git": {
    await import("./lit/ide/git-window");
    const el = document.createElement("lthn-git-window") as HTMLElement & { path: string };
    el.path = params.get("path") || "";
    app.innerHTML = "";
    app.appendChild(el);
    break;
  }
  case "build": {
    await import("./lit/ide/build-window");
    const el = document.createElement("lthn-build-window") as HTMLElement & { path: string };
    el.path = params.get("path") || "";
    app.innerHTML = "";
    app.appendChild(el);
    break;
  }
  case "lint": {
    await import("./lit/ide/lint-window");
    const el = document.createElement("lthn-lint-window") as HTMLElement & { path: string };
    el.path = params.get("path") || "";
    app.innerHTML = "";
    app.appendChild(el);
    break;
  }
  case "containers": {
    await import("./lit/ide/container-window");
    app.innerHTML = `<lthn-container-window></lthn-container-window>`;
    break;
  }
  case "repos": {
    await import("./lit/ide/repos-window");
    app.innerHTML = `<lthn-repos-window></lthn-repos-window>`;
    break;
  }
  case "vi-sites": {
    await import("./lit/vi/sites-window");
    app.innerHTML = `<lthn-vi-sites-window></lthn-vi-sites-window>`;
    break;
  }
  case "vi-activity": {
    await import("./lit/vi/activity-window");
    app.innerHTML = `<lthn-vi-activity-window></lthn-vi-activity-window>`;
    break;
  }
  case "php": {
    await import("./lit/ide/php-window");
    app.innerHTML = `<lthn-php-window></lthn-php-window>`;
    break;
  }
  case "marketplace": {
    await import("./lit/ext/marketplace-window");
    app.innerHTML = `<lthn-marketplace-window></lthn-marketplace-window>`;
    break;
  }
  case "plugin": {
    await import("./lit/ide/plugin-window");
    const el = document.createElement("lthn-plugin-window") as HTMLElement & { code: string };
    el.code = params.get("code") || "";
    app.innerHTML = "";
    app.appendChild(el);
    break;
  }
  case "benchmark": {
    app.innerHTML = `<lthn-benchmark-window></lthn-benchmark-window>`;
    break;
  }
  case "logs": {
    const tab = params.get("tab") || "live";
    app.innerHTML = `<lthn-logs-window tab="${tab}"></lthn-logs-window>`;
    break;
  }
  case "telemetry": {
    app.innerHTML = `<lthn-telemetry-window></lthn-telemetry-window>`;
    break;
  }
  case "integrations": {
    app.innerHTML = `<lthn-integrations-window></lthn-integrations-window>`;
    break;
  }
  case "tools": {
    app.innerHTML = `<lthn-tools-window></lthn-tools-window>`;
    break;
  }
  case "network": {
    app.innerHTML = `<lthn-network-window></lthn-network-window>`;
    break;
  }
  case "distillation": {
    app.innerHTML = `<lthn-distillation-window></lthn-distillation-window>`;
    break;
  }
  case "fleet": {
    app.innerHTML = `<lthn-fleet-window></lthn-fleet-window>`;
    break;
  }
  case "providers": {
    await import("./lit/ext/providers-window");
    app.innerHTML = `<lthn-providers-window></lthn-providers-window>`;
    break;
  }
  case "ml-lab": {
    // ML Lab Workbench — Influx / DuckDB / Models / LoRA panels
    // per plans/project/lthn/desktop/RFC.ml-lab.md §2.
    await import("./lit/views/ml-lab/ml-lab");
    app.innerHTML = `<lthn-view-ml-lab></lthn-view-ml-lab>`;
    break;
  }
  case "canvas":
  default: {
    app.innerHTML = `
      <div style="display:flex;flex-direction:column;gap:24px;padding:24px;">
        <h2 style="font-family:var(--font-sans,system-ui);">lthn — design canvas</h2>
        <p style="opacity:0.7;font-size:14px;">Mount any window via <code>?surface=chat&amp;state=multi-turn</code> etc.</p>
        <ul style="opacity:0.7;font-size:13px;font-family:var(--font-mono,monospace);">
          <li><a href="?surface=chat">chat</a> · <a href="?surface=welcome">welcome</a> · <a href="?surface=settings">settings</a> · <a href="?surface=models">models</a></li>
          <li><a href="?surface=benchmark">benchmark</a> · <a href="?surface=logs">logs</a> · <a href="?surface=telemetry">telemetry</a></li>
          <li><a href="?surface=integrations">integrations</a> · <a href="?surface=tools">tools</a> · <a href="?surface=editor">editor</a> · <a href="?surface=git">git</a></li>
          <li><a href="?surface=build">build</a> · <a href="?surface=lint">lint</a> · <a href="?surface=containers">containers</a> · <a href="?surface=repos">repos</a> · <a href="?surface=php">php</a> · <a href="?surface=marketplace">marketplace</a> · <a href="?surface=plugin">plugin</a></li>
          <li><a href="?surface=network">network</a> · <a href="?surface=distillation">distillation</a> · <a href="?surface=fleet">fleet</a> · <a href="?surface=providers">providers</a></li>
        </ul>
        <lthn-chat-window state="multi-turn"></lthn-chat-window>
      </div>`;
    break;
  }
}

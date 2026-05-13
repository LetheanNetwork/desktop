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

const app = document.getElementById("app");
if (!app) throw new Error("missing #app mount point in index.html");
const params = new URLSearchParams(location.search);
const surface = params.get("surface") || "canvas";

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
    Promise.all([
      import("@desktop/telemetry/service"),
      import("@desktop/runner/service"),
      import("@desktop/desktop/windowservice"),
      import("@lthn/i18n/coreservice"),
      import("@desktop/firstlaunch/wailsservice"),
    ]).then(async ([telemetry, runner, windowSvc, i18n, fl]) => {
      const TelemetryService = telemetry;
      const RunnerService = runner;
      const WindowService = windowSvc;
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
          await fl.Build().then(b => b?.version || "0.1.0").catch(() => "0.1.0"),
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
      };

      const setTab = (t: TrayTab) => () => { state.tab = t; draw(); };

      const fmtUptime = (s: number) => {
        if (s < 60) return `${s | 0}s`;
        if (s < 3600) return `${(s / 60) | 0}m ${(s % 60) | 0}s`;
        return `${(s / 3600) | 0}h ${((s % 3600) / 60) | 0}m`;
      };

      /** "5m ago" / "2h ago" / "3d ago" from a unix-second timestamp.
       *  Drives the Activity tab's "Last interact" slot — mirrors the
       *  chat-window left rail's age-bucket reasoning, but in a compact
       *  relative form the tray panel can fit. */
      const fmtRel = (ts: number) => {
        const ageSec = Math.max(0, Math.floor(Date.now() / 1000) - ts);
        if (ageSec < 60)    return t.relSec.replace("%d", String(ageSec));
        if (ageSec < 3600)  return t.relMin.replace("%d", String((ageSec / 60) | 0));
        if (ageSec < 86400) return t.relHr.replace("%d", String((ageSec / 3600) | 0));
        return t.relDay.replace("%d", String((ageSec / 86400) | 0));
      };

      const draw = () => {
        const variant = state.err ? "err" : state.connected ? "ok" : "idle";
        const stateLabel = state.err ? t.valOffline : state.connected ? t.valLive : t.valConnecting;
        const sparkData = state.samples.length
          ? state.samples.join(",")
          : "";
        const sparkMax = Math.max(1, ...state.samples) * 1.2;
        const hasModel = state.connected && state.model && state.model !== t.valNoModel;

        // Hero card — model status as the headline, mini-stats row, and a
        // thin inline sparkline strip. Replaces the prior verbose three-line
        // status row + separate "Heap (MB)" card at the bottom of the panel.
        const heroCard = html`
          <section style="display:flex; flex-direction:column; gap:10px;
                          padding:12px 14px;
                          background:rgba(255,255,255,0.025);
                          border:1px solid rgba(255,255,255,0.06);
                          border-radius:9px;">
            <!-- Row 1: status dot + model name headline + state pill only when offline -->
            <div style="display:flex; align-items:center; gap:10px; min-width:0;">
              <lthn-status-dot variant=${variant} ?pulse=${state.connected}></lthn-status-dot>
              <div style="font-size:14px; font-weight:600; letter-spacing:-0.005em;
                          color:${hasModel ? "var(--fg-0)" : "var(--fg-2)"};
                          flex:1; min-width:0;
                          overflow:hidden; text-overflow:ellipsis; white-space:nowrap;">
                ${state.connected ? state.model : (state.err ? t.statusOffline : t.statusConnecting)}
              </div>
              ${state.err
                ? html`<lthn-state-pill variant="disconnected">${stateLabel}</lthn-state-pill>`
                : nothing}
            </div>

            <!-- Row 2: mini-stats. When no model is loaded, an inviting
                 hint replaces the stats so the empty state reads as an
                 opportunity rather than a deficiency. -->
            ${hasModel ? html`
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
            ` : state.connected ? html`
              <div style="display:flex; align-items:center; gap:10px;
                          padding:8px 10px; border-radius:6px;
                          background:rgba(64,193,197,0.06);
                          border:1px dashed rgba(64,193,197,0.22);">
                <i class="fa-solid fa-cube" style="font-size:11px; color:var(--brand-300);"></i>
                <div style="flex:1; font-size:11.5px; color:var(--fg-1);">${t.statusPickModel}</div>
                <lthn-btn tone="quiet" size="sm" @click=${openAppPane("models")}>${t.statusBrowse}</lthn-btn>
              </div>
            ` : nothing}

            <!-- Row 3: thin inline sparkline. Only renders when there's
                 enough sample history to draw a meaningful trace; below
                 that threshold the hero card simply omits the strip so
                 we don't show a flat ghost line. -->
            ${state.samples.length > 1 ? html`
              <lthn-sparkline width="340" height="20" data=${sparkData} max=${sparkMax} fill></lthn-sparkline>
            ` : nothing}
          </section>
        `;

        // Open section — Lethean Desktop moved to the systray right-click
        // menu (canonical macOS pattern) + the screen icon in the titlebar
        // right side. Settings moved to the cog icon in the titlebar.
        // The remaining three windows (Chat / Models / Telemetry) live as
        // a 2-column grid; the third row stretches to fill so the layout
        // still feels balanced with an odd count.
        const openSection = html`
          <section style="display:flex; flex-direction:column; gap:8px;">
            <lthn-label>${t.sectionOpen}</lthn-label>
            <div style="display:grid; grid-template-columns: 1fr 1fr; gap:6px;">
              <lthn-btn tone="ghost" size="md" @click=${openAppPane("chat")}>
                <i class="fa-regular fa-comment" style="font-size:11px;"></i>
                ${t.openChat}
              </lthn-btn>
              <lthn-btn tone="ghost" size="md" @click=${openAppPane("models")}>
                <i class="fa-solid fa-cube" style="font-size:11px;"></i>
                ${t.openModels}
              </lthn-btn>
              <lthn-btn tone="ghost" size="md" @click=${openAppPane("telemetry")}
                style="grid-column: 1 / -1;">
                <i class="fa-solid fa-wave-square" style="font-size:11px;"></i>
                ${t.openTelemetry}
              </lthn-btn>
            </div>
          </section>
        `;

        // Info-card tab panel — sits under the hero. Tabs show
        // base info today; each panel is a placeholder for richer
        // charts/stats as the surfaces wire to real bindings.
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

        // Tiny key/value row used inside every tab panel.
        const kv = (k: string, v: string | number, mono = true) => html`
          <div style="display:flex; align-items:baseline; justify-content:space-between; gap:10px; padding:4px 0;">
            <span style="font-size:11px; color:var(--fg-3);">${k}</span>
            <span style="font-family:${mono ? "var(--font-mono)" : "var(--font-sans)"}; font-size:11.5px; color:var(--fg-1);">${v}</span>
          </div>
        `;

        const systemPanel = html`
          ${kv(t.heap, `${state.heapMb.toFixed(1)} MB`)}
          ${kv(t.uptime, fmtUptime(state.uptime))}
          ${kv(t.kvConnection, state.err ? t.valOffline : state.connected ? t.valLive : t.valConnecting)}
          ${kv(t.kvSamples, `${state.samples.length} / 24`)}
        `;

        const runnerPanel = html`
          ${kv(t.kvModel, hasModel ? state.model : t.valDash, false)}
          ${kv(t.kvStatus, hasModel ? t.valLoaded : t.valIdle)}
          ${kv(t.kvThroughput, t.valDash)}
          ${kv(t.kvCache, t.valDash)}
        `;

        const activityPanel = html`
          ${kv(t.kvSessionsToday, state.sessionsToday > 0 ? state.sessionsToday : t.valDash)}
          ${kv(t.kvTokens, t.valDash)}
          ${kv(t.kvLastInteract, state.lastInteract > 0 ? fmtRel(state.lastInteract) : t.valDash)}
          ${kv(t.kvRecentErrors, state.err ? "1" : "0")}
        `;

        const infoCard = html`
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
              ${state.tab === "system"   ? systemPanel   : nothing}
              ${state.tab === "runner"   ? runnerPanel   : nothing}
              ${state.tab === "activity" ? activityPanel : nothing}
            </div>
          </section>
        `;

        // Titlebar right-side icon row — cog (settings) + screen (open app).
        // Both opt out of drag explicitly (the parent slot is already
        // marked no-drag at the chrome level, this is belt-and-braces).
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
        /* Flag-button locale switch — clicks cycle the active
         * locale through availableLangs and rebuild the tray's
         * string cache via loadStrings(). Emoji flag picks the
         * current locale's national glyph; no FA dependency since
         * we want real flag colours. --wails-draggable opt-out
         * matches the other titlebar actions. */
        const flagAction = html`
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
        const titlebarActions = html`
          ${titlebarAction("fa-display", t.tbOpenApp,  openWindow("app"))}
          ${titlebarAction("fa-gear",    t.tbSettings, openWindow("settings"))}
          ${flagAction}
        `;

        render(renderChrome({
          title: t.chromeTitle,
          subtitle: state.err ? t.chromeOffline : t.chromeReady,
          w: 400, h: 560,
          actions: titlebarActions,
          body: html`
            <div style="display:flex; flex-direction:column; gap:14px; padding:14px; flex:1; min-height:0; overflow-y:auto; overscroll-behavior: none;">
              ${heroCard}
              ${infoCard}
              ${openSection}
            </div>
          `,
          footer: html`
            <span style="opacity:0.7;">${t.footerVersion}</span>
            <span style="flex:1"></span>
            <lthn-status-dot variant=${variant}></lthn-status-dot>
            <span style="opacity:0.7;">${stateLabel}</span>
          `,
        }), app);
      };

      const poll = async () => {
        try {
          const sessions = await import("@desktop/sessions/wailsservice");
          const [reading, models, sessionList] = await Promise.all([
            TelemetryService.CurrentSample(),
            RunnerService.WModels().catch((): string[] => []),
            sessions.List().catch((): unknown[] => []),
          ]);
          state.connected = true;
          state.err = null;
          state.uptime = reading.uptime_seconds || 0;
          state.heapMb = reading.heap_alloc_mb || 0;
          state.model = (models && models[0]) || t.valNoModel;
          state.samples.push(state.heapMb);
          if (state.samples.length > 24) state.samples.shift();
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
              return u > max ? u : max;
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
      const ms = v === "off" ? 0 : (parseInt((/^(\d+)s$/.exec(v) || [, "2"])[1], 10) * 1000);
      if (ms > 0) {
        const id = setInterval(poll, ms);
        window.addEventListener("beforeunload", () => clearInterval(id));
      }
    });
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
     * the initial view; the nav buttons swap thereafter. */
    const pane = params.get("pane") || "chat";
    app.innerHTML = `<lthn-app-shell active="${pane}"></lthn-app-shell>`;
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
  case "models": {
    app.innerHTML = `<lthn-model-browser-window></lthn-model-browser-window>`;
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
  case "php": {
    await import("./lit/ide/php-window");
    app.innerHTML = `<lthn-php-window></lthn-php-window>`;
    break;
  }
  case "marketplace": {
    await import("./lit/ide/marketplace-window");
    app.innerHTML = `<lthn-marketplace-window></lthn-marketplace-window>`;
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
          <li><a href="?surface=build">build</a> · <a href="?surface=lint">lint</a> · <a href="?surface=containers">containers</a> · <a href="?surface=repos">repos</a> · <a href="?surface=php">php</a> · <a href="?surface=marketplace">marketplace</a></li>
          <li><a href="?surface=network">network</a> · <a href="?surface=distillation">distillation</a> · <a href="?surface=fleet">fleet</a></li>
        </ul>
        <lthn-chat-window state="multi-turn"></lthn-chat-window>
      </div>`;
    break;
  }
}

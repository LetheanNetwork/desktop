// SPDX-Licence-Identifier: EUPL-1.2
// E2.2 · logs — <lthn-logs-window>
// Light-DOM Lit element. Composes renderChrome() from ../chrome.js.

import { LitElement, html, nothing } from "lit";
import { renderChrome } from "../chrome";
import { T } from "@lthn/i18n/coreservice";

interface LiveLine {
  t: string;
  c: string;
  s: string;
  m: string;
}

interface GenerationRow {
  ts: string;       // "HH:MM" derived from session UpdatedAt
  model: string;    // session title — closest stand-in until per-turn model_id lands
  tok: number;      // char/4 estimate of assistant reply
  tg: number | null; // tok/s — null until runner threads it through
  w: number | null;  // watts — null until power telemetry per-turn lands
  prompt: string;
}

class LthnLogsWindow extends LitElement {
  static readonly properties = {
    w: { type: Number },
    h: { type: Number },
    tab: { type: String },
    embedded: { type: Boolean, reflect: true },
    chrome: { state: true },
    liveLines: { state: true },
    paused: { state: true },
    gens: { state: true },
    gensLoaded: { state: true },
    t: { state: true },
  };
  declare w: number;
  declare h: number;
  declare tab: string;
  declare embedded: boolean;
  declare chrome: { title: string; subtitle: string };
  declare liveLines: LiveLine[];
  declare paused: boolean;
  declare gens: GenerationRow[];
  declare gensLoaded: boolean;
  declare t: {
    tabLive: string; tabHistory: string; tabPower: string;
    btnClear: string; btnPause: string; btnResume: string;
  };

  private _pollTimer: number | null = null;

  constructor() {
    super();
    this.w = 1000; this.h = 660; this.tab = "live"; this.embedded = false;
    this.chrome = { title: "Activity", subtitle: "logs · history · power" };
    this.liveLines = [];
    this.paused = false;
    this.gens = [];
    this.gensLoaded = false;
    this.t = {
      tabLive: "Live log", tabHistory: "Generation history", tabPower: "Power history",
      btnClear: "Clear", btnPause: "Pause", btnResume: "Resume",
    };
  }
  createRenderRoot() { return this; }
  async connectedCallback() {
    super.connectedCallback();
    const [title, subtitle, tl, th, tp, bc, bp, br] = await Promise.all([
      T("window.logs.title"),
      T("window.logs.subtitle"),
      T("window.logs.tab_live"),
      T("window.logs.tab_history"),
      T("window.logs.tab_power"),
      T("window.logs.btn_clear"),
      T("window.logs.btn_pause"),
      T("window.logs.btn_resume"),
    ]);
    this.chrome = { title, subtitle };
    this.t = {
      tabLive: tl, tabHistory: th, tabPower: tp,
      btnClear: bc, btnPause: bp, btnResume: br,
    };
    void this._pollLive();
    this._pollTimer = window.setInterval(() => void this._pollLive(), 1500);
    void this._loadGens();
  }

  /** Pull recent assistant turns out of pkg/sessions and map them
   *  into the row shape the History grid renders. One call on mount
   *  + one on tab-click; sessions write is rare enough that polling
   *  isn't necessary — re-fetch is cheap on demand. */
  async _loadGens() {
    try {
      const sessions = await import("@desktop/sessions/wailsservice");
      const { unwrap } = await import("../result");
      const raw = await unwrap<unknown[]>(sessions.RecentGenerations(20), []);
      const rows: GenerationRow[] = [];
      for (const g of raw || []) {
        const r = g as {
          session_id?: string;
          session_title?: string;
          updated_at?: number;
          prompt?: string;
          reply?: string;
          tokens?: number;
        };
        rows.push({
          ts: fmtUnix(r.updated_at || 0),
          model: r.session_title || "(untitled)",
          tok: r.tokens || 0,
          tg: null,
          w: null,
          prompt: r.prompt || "",
        });
      }
      this.gens = rows;
      this.gensLoaded = true;
    } catch (err) {
      console.error("logs: load gens failed", err);
      this.gensLoaded = true;
    }
  }
  disconnectedCallback() {
    super.disconnectedCallback();
    if (this._pollTimer !== null) {
      clearInterval(this._pollTimer);
      this._pollTimer = null;
    }
  }

  /** Poll bridge.Console for live console events the JS shim
   *  forwards from every window. Maps each ConsoleEntry → LiveLine
   *  shape the existing grid template renders. */
  async _pollLive() {
    if (this.paused) return;
    try {
      const bridge = await import("@desktop/bridge/service");
      const [con, errs] = await Promise.all([
        bridge.Console("", 200),
        bridge.Errors(50),
      ]);
      const lines: LiveLine[] = [];
      for (const e of con || []) {
        lines.push({
          t: fmtTime(e.at),
          c: e.source || "webview",
          s: e.level === "log" ? "info" : e.level,
          m: e.message,
        });
      }
      for (const e of errs || []) {
        lines.push({
          t: fmtTime(e.at),
          c: "webview",
          s: "error",
          m: e.message + (e.stack ? "\n" + e.stack : ""),
        });
      }
      lines.sort((a, b) => (a.t < b.t ? -1 : a.t > b.t ? 1 : 0));
      this.liveLines = lines;
    } catch (err) {
      console.error("logs: poll failed", err);
    }
  }

  /** Flip the active tab + re-fetch History when the user lands on
   *  it (sessions write between polls, so a re-visit should reflect
   *  any conversation that happened since mount). Mirrors the URL
   *  ?tab= contract from main.ts — same valid set, same semantics —
   *  but stays in-process so the user doesn't bounce through a full
   *  surface reload to flip a pane. */
  _switchTab(id: string) {
    if (this.tab === id) return;
    this.tab = id;
    if (id === "history") void this._loadGens();
  }

  async _clear() {
    try {
      const bridge = await import("@desktop/bridge/service");
      await Promise.all([bridge.ClearConsole(), bridge.ClearErrors()]);
      this.liveLines = [];
    } catch (err) {
      console.error("logs: clear failed", err);
    }
  }

  render() {
    const tabs = [
      { id: "live",    label: this.t.tabLive,    icon: "fa-wave-square" },
      { id: "history", label: this.t.tabHistory, icon: "fa-clock-rotate-left" },
      { id: "power",   label: this.t.tabPower,   icon: "fa-bolt" },
    ];
    const toolbar = html`
      ${tabs.map(t => html`
        <lthn-btn tone=${this.tab === t.id ? "primary" : "ghost"} size="sm" ?active=${this.tab === t.id}
          @click=${() => this._switchTab(t.id)}>
          <i class="fa-solid ${t.icon}" style="font-size:10px;"></i> ${t.label}
        </lthn-btn>
      `)}
      <div style="flex:1"></div>
      ${this.tab === "live" ? html`
        <lthn-btn tone="ghost" size="sm"
          @click=${() => this._clear()}>
          <i class="fa-solid fa-trash" style="font-size:10px;"></i> ${this.t.btnClear}
        </lthn-btn>
        <lthn-btn tone=${this.paused ? "primary" : "ghost"} size="sm"
          @click=${() => { this.paused = !this.paused; }}>
          <i class="fa-solid ${this.paused ? "fa-play" : "fa-pause"}" style="font-size:10px;"></i>
          ${this.paused ? this.t.btnResume : this.t.btnPause}
        </lthn-btn>
      ` : nothing}
    `;
    // History footer derives from the rendered list — count + summed
    // tokens both stay in sync with the body. Watts aggregate stays
    // omitted until per-turn power telemetry lands; calling it
    // "142.6 Wh" when no row carries watts would be the fixture lie
    // we're moving away from.
    const totalTok = this.gens.reduce((acc, g) => acc + (g.tok || 0), 0);
    const tokStr = totalTok >= 1000 ? `${(totalTok / 1000).toFixed(1)}k` : String(totalTok);
    const footers = {
      live:    `streaming · ${this.liveLines.length} lines · webview bridge${this.paused ? " · paused" : ""}`,
      history: this.gensLoaded
        ? `${this.gens.length} generations · ~${tokStr} tokens · session-grain timestamps`
        : "loading…",
      power:   "showing last 24h · sample 1 s · powermetrics backend",
    };
    const body =
      this.tab === "live"    ? this._renderLive()    :
      this.tab === "history" ? this._renderHistory() :
                               this._renderPower();
    return renderChrome({
      title: this.chrome.title, subtitle: this.chrome.subtitle,
      w: this.w, h: this.h, toolbar, body,
      footer: html`${footers[this.tab as keyof typeof footers]}`,
      embedded: this.embedded,
    });
  }

  _renderLive() {
    const lines = this.liveLines;
    const sevColor = { info: "var(--fg-2)", debug: "var(--fg-3)", warn: "var(--warning-400)", error: "var(--err-400)" };
    // Derive component + severity counts from the live lines so the
    // rail's filter chips show real counts (matches the live data
    // shape exactly — no fixture drift).
    const compCounts = new Map<string, number>();
    for (const l of lines) compCounts.set(l.c, (compCounts.get(l.c) || 0) + 1);
    const comps = Array.from(compCounts.entries()).map(([k, n]) => ({ k, n, on: true }));
    const sevCounts = { error: 0, warn: 0, info: 0, debug: 0 } as Record<string, number>;
    for (const l of lines) {
      if (l.s in sevCounts) sevCounts[l.s] += 1;
    }
    const sevs = [
      { k: "error", c: "var(--err-400)",     on: true, n: sevCounts.error },
      { k: "warn",  c: "var(--warning-400)", on: true, n: sevCounts.warn  },
      { k: "info",  c: "var(--brand-300)",   on: true, n: sevCounts.info  },
      { k: "debug", c: "var(--fg-3)",        on: true, n: sevCounts.debug },
    ];
    return html`
      <div style="flex:1; display:grid; grid-template-columns:180px 1fr; min-height:0;">
        <aside style="background:rgba(0,0,0,0.18); border-right:1px solid rgba(255,255,255,0.05); padding:14px 12px; display:flex; flex-direction:column; gap:12px;">
          <div>
            <lthn-label>Components</lthn-label>
            <div style="margin-top:8px; display:flex; flex-direction:column; gap:4px;">
              ${comps.map(c => html`
                <div style="display:flex; align-items:center; gap:8px; font-size:11px;">
                  <input type="checkbox" ?checked=${c.on} style="accent-color:var(--brand-400);">
                  <span style="font-family:var(--font-mono); color:var(--fg-1); flex:1;">${c.k}</span>
                  <span style="font-family:var(--font-mono); font-size:9.5px; color:var(--fg-3);">${c.n}</span>
                </div>
              `)}
            </div>
          </div>
          <div>
            <lthn-label>Severity</lthn-label>
            <div style="margin-top:8px; display:flex; flex-direction:column; gap:4px;">
              ${sevs.map(s => html`
                <div style="display:flex; align-items:center; gap:8px; font-size:11px;">
                  <input type="checkbox" ?checked=${s.on} style="accent-color:var(--brand-400);">
                  <span style="width:6px; height:6px; border-radius:50%; background:${s.c};"></span>
                  <span style="color:var(--fg-1); flex:1;">${s.k}</span>
                  <span style="font-family:var(--font-mono); font-size:9.5px; color:var(--fg-3);">${s.n}</span>
                </div>
              `)}
            </div>
          </div>
        </aside>
        <main style="overflow:auto; padding:8px 0; font-family:var(--font-mono); font-size:11.5px; line-height:1.6;">
          ${lines.map((l, i) => html`
            <div style="display:grid; grid-template-columns:112px 64px 50px 1fr; padding:1.5px 16px; background:${l.s === "warn" ? "rgba(245,158,11,0.06)" : i % 2 === 0 ? "transparent" : "rgba(255,255,255,0.015)"}; gap:10px;">
              <span style="color:var(--fg-3);">${l.t}</span>
              <span style="color:var(--fg-2);">${l.c}</span>
              <span style="color:${sevColor[l.s as keyof typeof sevColor]}; letter-spacing:0.04em; font-size:10px; text-transform:uppercase;">${l.s}</span>
              <span style="color:var(--fg-1); white-space:pre-wrap;">${l.m}</span>
            </div>
          `)}
          <div style="padding:8px 16px; display:flex; align-items:center; gap:8px; font-size:10.5px; color:var(--fg-3);">
            <lthn-status-dot variant="ok" pulse></lthn-status-dot>
            live · waiting for next event…
          </div>
        </main>
      </div>
    `;
  }

  _renderHistory() {
    const gens = this.gens;
    if (this.gensLoaded && gens.length === 0) {
      return html`
        <div style="flex:1; display:flex; align-items:center; justify-content:center; padding:40px 22px; flex-direction:column; gap:10px;">
          <div style="font-size:13px; color:var(--fg-2);">No generations yet.</div>
          <div style="font-size:11.5px; color:var(--fg-3); max-width:420px; text-align:center; line-height:1.55;">
            Chats you have with the model land here automatically. Open the Chat surface, start a conversation, and the assistant turns will appear in this list.
          </div>
        </div>
      `;
    }
    return html`
      <div style="flex:1; padding:12px 22px 18px; overflow:auto;">
        <div style="background:rgba(255,255,255,0.025); border:1px solid rgba(255,255,255,0.06); border-radius:8px; font-family:var(--font-mono); font-size:11.5px;">
          <div style="display:grid; grid-template-columns:100px 1.3fr 0.6fr 0.6fr 0.6fr 2fr; padding:10px 14px; border-bottom:1px solid rgba(255,255,255,0.06); color:var(--fg-3); font-size:10px; letter-spacing:0.04em; text-transform:uppercase;">
            <span>Time</span><span>Session</span><span>Tokens</span><span>tok/s</span><span>Peak W</span><span>Prompt</span>
          </div>
          ${gens.map((g, i) => html`
            <div style="display:grid; grid-template-columns:100px 1.3fr 0.6fr 0.6fr 0.6fr 2fr; padding:10px 14px; border-bottom:${i < gens.length - 1 ? "1px solid rgba(255,255,255,0.04)" : "none"}; align-items:center; gap:8px;">
              <span style="color:var(--fg-2);">${g.ts}</span>
              <span style="color:var(--fg-0); white-space:nowrap; overflow:hidden; text-overflow:ellipsis;">${g.model}</span>
              <span style="color:var(--fg-1);">${g.tok}</span>
              <span style="color:var(--fg-3);">${g.tg ?? "—"}</span>
              <span style="color:var(--fg-3);">${g.w ?? "—"}</span>
              <span style="color:var(--fg-2); white-space:nowrap; overflow:hidden; text-overflow:ellipsis;">${g.prompt}</span>
            </div>
          `)}
        </div>
        <div style="margin-top:14px; font-size:11px; color:var(--fg-3); line-height:1.55;">
          Local. Never leaves this Mac. Right-click a row to re-open it in chat or export the transcript.
        </div>
      </div>
    `;
  }

  _renderPower() {
    const samples = Array.from({ length: 60 }, (_, i) => {
      const base = 0.4 + Math.sin(i * 0.3) * 0.3;
      const spike = [12, 13, 14, 22, 23, 38, 39, 40, 50, 51].includes(i) ? 6 + ((i * 17 % 30) / 30) * 3 : 0;
      return Math.max(0.2, base + spike);
    });
    const w = 940, h = 280, pad = 32, max = 12;
    const kpis = [
      { l: "24h average",  v: "1.8 W",   s: "≈ a USB-C trickle" },
      { l: "24h total",    v: "44.2 Wh", s: "≈ 8 phone-charges" },
      { l: "Peak today",   v: "9.4 W",   s: "during decode @ 14:32" },
    ];
    return html`
      <div style="flex:1; padding:12px 22px 18px; display:flex; flex-direction:column; gap:14px; overflow:auto;">
        <div style="display:grid; grid-template-columns:repeat(3, 1fr); gap:10px;">
          ${kpis.map(k => html`
            <div style="padding:14px 16px; border-radius:8px; background:rgba(255,255,255,0.03); border:1px solid rgba(255,255,255,0.06);">
              <lthn-label>${k.l}</lthn-label>
              <div style="font-family:var(--font-mono); font-size:22px; color:var(--fg-0); margin-top:6px; letter-spacing:-0.01em;">${k.v}</div>
              <div style="font-size:11px; color:var(--fg-3); margin-top:4px;">${k.s}</div>
            </div>
          `)}
        </div>
        <div style="background:rgba(0,0,0,0.20); border:1px solid rgba(255,255,255,0.05); border-radius:8px; padding:12px;">
          <lthn-label>Watts · last 24 hours</lthn-label>
          <svg viewBox="0 0 ${w} ${h}" width="100%" height=${h} preserveAspectRatio="none" style="margin-top:6px;">
            ${[0, 3, 6, 9, 12].map(v => {
              const yy = h - pad - (v / max) * (h - pad - 16);
              return html`
                <line x1=${pad} x2=${w} y1=${yy} y2=${yy} stroke="rgba(255,255,255,0.04)"></line>
                <text x=${pad - 6} y=${yy + 3} fill="rgba(255,255,255,0.40)" font-size="10" text-anchor="end" font-family="ui-monospace, monospace">${v} W</text>
              `;
            })}
            <path d=${"M " + samples.map((s, i) => `${pad + (i / (samples.length - 1)) * (w - pad)} ${h - pad - (s / max) * (h - pad - 16)}`).join(" L ") + ` L ${w} ${h - pad} L ${pad} ${h - pad} Z`} fill="rgba(64,193,197,0.10)"></path>
            <path d=${"M " + samples.map((s, i) => `${pad + (i / (samples.length - 1)) * (w - pad)} ${h - pad - (s / max) * (h - pad - 16)}`).join(" L ")} stroke="var(--brand-400)" stroke-width="1.4" fill="none"></path>
            ${["00:00", "06:00", "12:00", "18:00", "now"].map((t, i) => html`
              <text x=${pad + (i / 4) * (w - pad)} y=${h - 8} fill="rgba(255,255,255,0.40)" font-size="10" text-anchor=${i === 4 ? "end" : "middle"} font-family="ui-monospace, monospace">${t}</text>
            `)}
          </svg>
          <div style="margin-top:8px; font-size:11px; color:var(--fg-3); font-style:italic; line-height:1.5;">
            For comparison — a typical fridge averages ~150 W. A Christmas-tree bulb, ~5 W.
          </div>
        </div>
      </div>
    `;
  }
}
customElements.define("lthn-logs-window", LthnLogsWindow);

/** RFC3339-ish "2026-05-13T07:14:32Z" → "07:14:32" so the time
 *  column stays compact. Falls back to the raw value when the
 *  format doesn't match (defensive — the bridge timestamps via
 *  time.Now().UTC() so they should always parse). */
function fmtTime(at: string): string {
  if (!at) return "";
  const m = at.match(/T(\d{2}:\d{2}:\d{2})/);
  return m ? m[1] : at;
}

/** Unix epoch (seconds) → "HH:MM" in the user's locale. Used by the
 *  History tab so a row's timestamp reads against the wall clock the
 *  user sees in the menubar rather than UTC the JSON carries. Zero
 *  collapses to empty so a session with no UpdatedAt doesn't render
 *  "00:00". */
function fmtUnix(secs: number): string {
  if (!secs) return "";
  const d = new Date(secs * 1000);
  const hh = String(d.getHours()).padStart(2, "0");
  const mm = String(d.getMinutes()).padStart(2, "0");
  return `${hh}:${mm}`;
}

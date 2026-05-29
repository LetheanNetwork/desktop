// SPDX-Licence-Identifier: EUPL-1.2
// Agents view · Activity — <lthn-view-agent-activity>
//
// The live CoreAgent run feed: every tracked workspace + its status, with
// derived counts (running / queued / completed / failed) and the BLOCKED runs
// surfaced as an actionable queue (answer → resume). Data via
// Agents.Workspaces() → `lthn-agent workspace/list --json` (the CLI lane — the
// GUI shells out to the binary, not the plugin's serve API). Polls every 5s +
// refreshes instantly on the serve's agent.* push channels.
//
// Backend: Agents.Workspaces() / Agents.Resume() from @desktop/agents/service,
// imported dynamically so tests run without the Wails runtime. Fails soft when
// lthn-agent isn't reachable.
//
// Supports the `embedded` attribute — no chrome when set.

import { LitElement, html, nothing } from "lit";
import { renderChrome } from "../../chrome";
import { demand } from "../../result";

/** Workspace mirrors agents.Workspace — one tracked run + its state. */
interface Workspace {
  name:      string;
  status:    string;
  agent:     string;
  repo:      string;
  org?:      string;
  task?:     string;
  branch?:   string;
  issue?:    number;
  question?: string;
  runs?:     number;
  pr_url?:   string;
}

// Poll cadence — push events (agent.* channels) drive instant updates, so the
// poll is just reconcile + fallback. Kept long because each poll spawns the
// lthn-agent CLI; 30s avoids booting it every few seconds.
const REFRESH_MS = 30_000;

/** Dot colour per status. */
function statusColor(status: string): string {
  switch (status) {
    case "running":   return "var(--brand-300)";
    case "completed": return "var(--success-400)";
    case "merged":    return "var(--success-400)";
    case "failed":    return "var(--err-400)";
    case "blocked":   return "var(--warning-400)";
    default:          return "var(--fg-3)";
  }
}

class LthnViewAgentActivity extends LitElement {
  static readonly properties = {
    w:          { type: Number },
    h:          { type: Number },
    embedded:   { type: Boolean, reflect: true },
    workspaces: { state: true },
    loading:    { state: true },
    err:        { state: true },
    answers:    { state: true },
    busyWs:     { state: true },
    rowErr:     { state: true },
  };

  declare w: number;
  declare h: number;
  declare embedded: boolean;
  declare workspaces: Workspace[];
  declare loading: boolean;
  declare err: string;
  // Per-blocked-run UI state, keyed by workspace name.
  declare answers: Record<string, string>;
  declare busyWs: string;
  declare rowErr: Record<string, string>;

  private _timer: ReturnType<typeof setInterval> | null = null;
  private _unsubChannel: (() => void) | null = null;

  constructor() {
    super();
    this.w = 1180; this.h = 720; this.embedded = false;
    this.workspaces = [];
    this.loading = false;
    this.err = "";
    this.answers = {};
    this.busyWs = "";
    this.rowErr = {};
  }

  createRenderRoot() { return this; }

  async connectedCallback() {
    super.connectedCallback();
    await this._refresh();
    this._timer = setInterval(() => { void this._refresh(); }, REFRESH_MS);
    // Live updates: the serve pushes agent.* channel events the moment a run
    // changes; the desktop relays them as "lthn:agents:channel". Re-listing on
    // the agent.* ones makes the feed react instantly. The 5s poll stays as
    // reconcile + fallback when the push stream is down.
    try {
      const { Events } = await import("@wailsio/runtime");
      this._unsubChannel = Events.On("lthn:agents:channel", (ev: { data?: { channel?: string } }) => {
        const ch = ev?.data?.channel;
        if (typeof ch === "string" && ch.startsWith("agent.")) void this._refresh();
      });
    } catch { /* no Wails runtime (tests) — poll-only */ }
  }

  disconnectedCallback() {
    if (this._timer) { clearInterval(this._timer); this._timer = null; }
    if (this._unsubChannel) { this._unsubChannel(); this._unsubChannel = null; }
    super.disconnectedCallback();
  }

  async _refresh() {
    if (this.loading) return;
    this.loading = true;
    this.err = "";
    try {
      const svc = await import("@desktop/agents/service");
      const r = await (svc as { Workspaces: () => Promise<{ OK?: boolean; Value?: Workspace[] }> }).Workspaces();
      this.workspaces = (r?.OK !== false && Array.isArray(r?.Value)) ? r!.Value! : [];
      if (r?.OK === false) this.err = "engine returned an error";
    } catch (e: unknown) {
      this.err = e instanceof Error ? e.message : String(e);
      this.workspaces = [];
    } finally {
      this.loading = false;
    }
  }

  /** Answer a blocked run and relaunch it (Agents.Resume → lthn-agent resume).
   *  An empty answer is valid — the agent re-reads BLOCKED.md and retries. On
   *  success the run flips to "running" and leaves the blocked queue, so we
   *  refresh; on failure the message shows inline on the row. */
  async _resume(ws: string) {
    if (this.busyWs) return;
    this.busyWs = ws;
    this.rowErr = { ...this.rowErr, [ws]: "" };
    try {
      const svc = await import("@desktop/agents/service");
      const answer = (this.answers[ws] ?? "").trim();
      await demand<unknown>(
        (svc as { Resume: (req: { workspace: string; answer?: string }) => Promise<{ OK: boolean; Value: unknown }> })
          .Resume({ workspace: ws, answer }),
      );
      const next = { ...this.answers }; delete next[ws]; this.answers = next;
      await this._refresh();
    } catch (e: unknown) {
      this.rowErr = { ...this.rowErr, [ws]: e instanceof Error ? e.message : String(e) };
    } finally {
      this.busyWs = "";
    }
  }

  private _countOf(status: string): number {
    return this.workspaces.filter(w => w.status === status).length;
  }

  /** A count pill: coloured dot + label + number. */
  _count(label: string, n: number, dot: string) {
    return html`
      <span style="display:inline-flex; align-items:center; gap:6px; padding:4px 10px;
                   border-radius:6px; background:rgba(0,0,0,0.18);
                   border:1px solid rgba(255,255,255,0.06);
                   font-family:var(--font-mono); font-size:11px;">
        <span style="width:7px; height:7px; border-radius:50%; background:${dot};"></span>
        <span style="color:var(--fg-2);">${label}</span>
        <span style="color:var(--fg-0);">${n}</span>
      </span>
    `;
  }

  /** A blocked-run row: question + answer textarea + Resume. */
  _blockedRow(b: Workspace, last: boolean) {
    const busy = this.busyWs === b.name;
    return html`
      <div style="padding:14px 16px; border-bottom:${last ? "none" : "1px solid rgba(255,255,255,0.04)"};">
        <div style="display:flex; align-items:center; gap:10px;">
          <span style="font-family:var(--font-mono); font-size:10px; padding:3px 9px; border-radius:999px;
                       background:rgba(255,255,255,0.05); border:1px solid rgba(255,255,255,0.12);
                       color:var(--warning-400); letter-spacing:0.03em;">
            <i class="fa-solid fa-circle-question" style="font-size:9px; margin-right:5px;"></i>blocked
          </span>
          <span style="font-family:var(--font-mono); font-size:11px; color:var(--fg-2);">${b.agent || "—"}</span>
          <span style="font-family:var(--font-mono); font-size:10px; color:var(--fg-3);">·</span>
          <span style="font-family:var(--font-mono); font-size:10px; color:var(--fg-3);">${b.repo || b.name}</span>
        </div>
        <div style="margin-top:8px; font-size:13px; color:var(--fg-0); line-height:1.5;">${b.question || "(no question recorded)"}</div>
        <div style="margin-top:10px; display:flex; gap:8px; align-items:flex-end; --wails-draggable:no-drag;">
          <textarea rows="2"
            placeholder="answer — written to ANSWER.md, then the agent resumes (leave blank to just retry)"
            .value=${this.answers[b.name] ?? ""}
            @input=${(e: Event) => { this.answers = { ...this.answers, [b.name]: (e.target as HTMLTextAreaElement).value }; }}
            style="flex:1; padding:7px 9px; font-size:12px; line-height:1.5; resize:vertical;
                   background:rgba(0,0,0,0.25); color:var(--fg-0);
                   border:1px solid rgba(255,255,255,0.08); border-radius:5px;
                   font-family:inherit; --wails-draggable:no-drag;"></textarea>
          <lthn-btn tone="primary" size="sm" ?dim=${busy} @click=${() => void this._resume(b.name)}>
            <i class="fa-solid ${busy ? "fa-spinner" : "fa-play"}" style="font-size:10px;"></i>
            ${busy ? "Resuming…" : "Resume"}
          </lthn-btn>
        </div>
        ${this.rowErr[b.name] ? html`
          <div style="margin-top:8px; padding:8px 10px; color:var(--err-400);
                      background:rgba(255,76,76,0.06); border:1px solid rgba(255,76,76,0.18);
                      border-radius:6px; font-size:11px; line-height:1.5; font-family:var(--font-mono);">
            ${this.rowErr[b.name]}
          </div>
        ` : nothing}
      </div>
    `;
  }

  /** A feed row: status dot + repo/name + agent + task. */
  _feedRow(w: Workspace, last: boolean) {
    return html`
      <div style="padding:11px 16px; display:grid; grid-template-columns: 92px 1fr auto; gap:14px; align-items:center;
                  border-bottom:${last ? "none" : "1px solid rgba(255,255,255,0.04)"};">
        <span style="display:inline-flex; align-items:center; gap:6px; font-family:var(--font-mono); font-size:10.5px; color:var(--fg-2);">
          <span style="width:7px; height:7px; border-radius:50%; background:${statusColor(w.status)};"></span>${w.status}
        </span>
        <div style="min-width:0;">
          <div style="font-size:12.5px; color:var(--fg-0); overflow:hidden; text-overflow:ellipsis; white-space:nowrap;">${w.task || w.name}</div>
          <div style="font-family:var(--font-mono); font-size:10px; color:var(--fg-3); margin-top:2px;">${w.repo}${w.branch ? ` · ${w.branch}` : ""}</div>
        </div>
        <span style="font-family:var(--font-mono); font-size:10px; color:var(--fg-3);">${w.agent || ""}${(w.runs ?? 0) > 1 ? ` ·${w.runs}×` : ""}</span>
      </div>
    `;
  }

  render() {
    const ws = this.workspaces;
    const blocked = ws.filter(w => w.status === "blocked");

    const toolbar = html`
      <div style="display:inline-flex; gap:8px;">
        ${this._count("running", this._countOf("running"), "var(--brand-300)")}
        ${this._count("queued", this._countOf("queued"), "var(--fg-3)")}
        ${this._count("done", this._countOf("completed"), "var(--success-400)")}
        ${this._count("failed", this._countOf("failed"), "var(--err-400)")}
      </div>
      <div style="flex:1"></div>
      <lthn-btn tone="primary" size="sm" ?dim=${this.loading} @click=${() => void this._refresh()}>
        <i class="fa-solid ${this.loading ? "fa-spinner" : "fa-rotate"}" style="font-size:10px;"></i>
        ${this.loading ? "Polling…" : "Refresh"}
      </lthn-btn>
    `;

    const body = html`
      <div style="flex:1; padding:14px 22px 22px; overflow:auto;">
        ${this.err ? html`
          <div style="padding:18px; text-align:center; color:var(--fg-3); font-size:12px;
                      background:rgba(255,255,255,0.025); border:1px solid rgba(255,255,255,0.06);
                      border-radius:10px; line-height:1.6;">
            CoreAgent not reachable — the GUI drives
            <code style="font-family:var(--font-mono);">lthn-agent</code> via the CLI.
            <div style="margin-top:8px; font-size:10.5px; color:var(--err-400); font-family:var(--font-mono);">${this.err}</div>
          </div>
        ` : ws.length === 0 ? html`
          <div style="padding:40px; text-align:center; color:var(--fg-3); font-size:12px; line-height:1.6;">
            No agent runs yet — dispatch a task and the feed shows here.
          </div>
        ` : html`
          ${blocked.length ? html`
            <div style="font-size:11px; color:var(--fg-3); margin-bottom:10px;">
              ${blocked.length} run${blocked.length === 1 ? "" : "s"} blocked — waiting on your answer
            </div>
            <div style="background:rgba(255,255,255,0.025); border:1px solid rgba(255,255,255,0.06); border-radius:10px; overflow:hidden; margin-bottom:18px;">
              ${blocked.map((b, i) => this._blockedRow(b, i === blocked.length - 1))}
            </div>
          ` : nothing}
          <div style="font-size:11px; color:var(--fg-3); margin-bottom:10px;">
            ${ws.length} workspace${ws.length === 1 ? "" : "s"}
          </div>
          <div style="background:rgba(255,255,255,0.025); border:1px solid rgba(255,255,255,0.06); border-radius:10px; overflow:hidden;">
            ${ws.map((w, i) => this._feedRow(w, i === ws.length - 1))}
          </div>
        `}
      </div>
    `;

    const subtitle = this.err ? "engine unreachable"
      : `${this._countOf("running")} running · ${ws.length} total${blocked.length ? ` · ${blocked.length} blocked` : ""}`;

    return renderChrome({
      title: "Activity",
      subtitle,
      w: this.w, h: this.h,
      toolbar, body,
      footer: html`live CoreAgent runs · answer a blocked run to resume it · push events + 30s poll · via lthn-agent CLI`,
      embedded: this.embedded,
    });
  }
}

customElements.define("lthn-view-agent-activity", LthnViewAgentActivity);

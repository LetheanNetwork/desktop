// SPDX-Licence-Identifier: EUPL-1.2
// Agents view · Activity — <lthn-view-agent-activity>
//
// The live CoreAgent fleet-pulse: dispatch counts (running / queued /
// completed / failed) across all workspaces, plus the runs that are
// BLOCKED awaiting an operator answer (each with its question). Data via
// Agents.Status() → agentic_status on the crew's lthn-agent (:9101).
// agentic_status counts running/done/failed and enumerates only the
// blocked ones; the full per-run history (events.jsonl) is a separate
// bridge. This panel is the at-a-glance state + the actionable queue.
//
// Backend: Agents.Status() from @desktop/agents/service — imported
// dynamically so tests run without the Wails runtime. Polls every 5s +
// manual refresh; fails soft when the engine (lthn-agent serve) is down.
//
// Supports the `embedded` attribute — no chrome when set.

import { LitElement, html, nothing } from "lit";
import { renderChrome } from "../../chrome";
import { demand } from "../../result";

/** BlockedRun mirrors agents.BlockedRun (a workspace awaiting an answer). */
interface BlockedRun {
  name:     string;
  repo:     string;
  agent:    string;
  question: string;
}

/** StatusResult mirrors agents.StatusResult — counts + blocked runs. */
interface StatusResult {
  total:     number;
  running:   number;
  queued:    number;
  completed: number;
  failed:    number;
  blocked:   BlockedRun[];
}

const REFRESH_MS = 5_000;

class LthnViewAgentActivity extends LitElement {
  static readonly properties = {
    w:        { type: Number },
    h:        { type: Number },
    embedded: { type: Boolean, reflect: true },
    status:   { state: true },
    loading:  { state: true },
    err:      { state: true },
    answers:  { state: true },
    busyWs:   { state: true },
    rowErr:   { state: true },
  };

  declare w: number;
  declare h: number;
  declare embedded: boolean;
  declare status: StatusResult | null;
  declare loading: boolean;
  declare err: string;
  // Per-blocked-run UI state, keyed by workspace name: the answer being
  // typed, the one workspace currently resuming, and any per-row error.
  declare answers: Record<string, string>;
  declare busyWs: string;
  declare rowErr: Record<string, string>;

  private _timer: ReturnType<typeof setInterval> | null = null;

  constructor() {
    super();
    this.w = 1180; this.h = 720; this.embedded = false;
    this.status = null;
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
  }

  disconnectedCallback() {
    if (this._timer) { clearInterval(this._timer); this._timer = null; }
    super.disconnectedCallback();
  }

  async _refresh() {
    if (this.loading) return;
    this.loading = true;
    this.err = "";
    try {
      const svc = await import("@desktop/agents/service");
      const r = await (svc as { Status: () => Promise<{ Value: StatusResult }> }).Status();
      this.status = r?.Value ?? null;
    } catch (e: unknown) {
      this.err = e instanceof Error ? e.message : String(e);
      this.status = null;
    } finally {
      this.loading = false;
    }
  }

  /** Answer a blocked run and relaunch it (Agents.Resume → agentic_resume).
   *  An empty answer is valid — the agent re-reads BLOCKED.md and retries.
   *  On success the run flips to "running" and leaves the blocked queue, so
   *  we refresh; on failure the message shows inline on the row. */
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

  render() {
    const s = this.status;
    const blocked = s?.blocked ?? [];

    const toolbar = html`
      <div style="display:inline-flex; gap:8px;">
        ${s ? html`
          ${this._count("running", s.running, "var(--brand-300)")}
          ${this._count("queued", s.queued, "var(--fg-3)")}
          ${this._count("done", s.completed, "var(--success-400)")}
          ${this._count("failed", s.failed, "var(--err-400)")}
        ` : nothing}
      </div>
      <div style="flex:1"></div>
      <lthn-btn tone="primary" size="sm" ?dim=${this.loading}
        @click=${() => void this._refresh()}>
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
            CoreAgent engine not reachable — the crew brings
            <code style="font-family:var(--font-mono);">lthn-agent serve</code> up on boot.
            <div style="margin-top:8px; font-size:10.5px; color:var(--err-400); font-family:var(--font-mono);">${this.err}</div>
          </div>
        ` : blocked.length === 0 ? html`
          <div style="padding:40px; text-align:center; color:var(--fg-3); font-size:12px; line-height:1.6;">
            ${s && s.total === 0
              ? "No agent runs yet — dispatch a task and the fleet pulse shows here."
              : html`${s?.running ?? 0} running · ${s?.completed ?? 0} done · ${s?.failed ?? 0} failed — nothing blocked.`}
          </div>
        ` : html`
          <div style="font-size:11px; color:var(--fg-3); margin-bottom:10px;">
            ${blocked.length} run${blocked.length === 1 ? "" : "s"} blocked — waiting on your answer
          </div>
          <div style="background:rgba(255,255,255,0.025); border:1px solid rgba(255,255,255,0.06); border-radius:10px; overflow:hidden;">
            ${blocked.map((b, i) => html`
              <div style="padding:14px 16px;
                          border-bottom:${i < blocked.length - 1 ? "1px solid rgba(255,255,255,0.04)" : "none"};">
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
                  <lthn-btn tone="primary" size="sm" ?dim=${this.busyWs === b.name}
                    @click=${() => void this._resume(b.name)}>
                    <i class="fa-solid ${this.busyWs === b.name ? "fa-spinner" : "fa-play"}" style="font-size:10px;"></i>
                    ${this.busyWs === b.name ? "Resuming…" : "Resume"}
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
            `)}
          </div>
        `}
      </div>
    `;

    const subtitle = s
      ? `${s.running} running · ${s.total} total${blocked.length ? ` · ${blocked.length} blocked` : ""}`
      : this.err ? "engine unreachable" : "…";

    return renderChrome({
      title: "Activity",
      subtitle,
      w: this.w, h: this.h,
      toolbar, body,
      footer: html`live CoreAgent state · answer a blocked run to resume it · polls every 5s`,
      embedded: this.embedded,
    });
  }
}

customElements.define("lthn-view-agent-activity", LthnViewAgentActivity);

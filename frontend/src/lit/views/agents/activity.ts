// SPDX-Licence-Identifier: EUPL-1.2
// Agents view · Activity — <lthn-view-agent-activity>
//
// The live dispatch feed: recent agent runs across all statuses
// (running · done · failed), newest first, from Fleet.Activity(). The
// headline panel of the Agents view — "watch the fleet work". Queue() is
// the status='running' subset; Activity() is the full recent history
// (see go/pkg/fleet/service.go).
//
// Backend: Fleet.Activity() from @desktop/fleet/service — imported
// dynamically so tests run without the Wails runtime. Polls every 5s
// (runs start/finish faster than PRs change) plus a manual refresh.
//
// Supports the `embedded` attribute — no chrome when set.

import { LitElement, html, nothing } from "lit";
import { renderChrome } from "../../chrome";

/** RunRow — mirrors go/pkg/fleet/service.go QueueRow. Kept inline;
 *  importing from bindings pulls the Wails runtime. */
interface RunRow {
  id:         string;
  agent:      string;
  caller:     string;
  task_id:    string;
  machine_id: string;
  model:      string;
  action:     string;
  status:     string;
  started_at: number; // unix seconds
  summary:    string;
}

type RunFilter = "all" | "running" | "done" | "failed";

const REFRESH_MS = 5_000;
const RUN_LIMIT = 200;

/** Status → pill colour. running=brand, done=success, failed=err, else muted. */
function statusColour(status: string): string {
  const map: Record<string, string> = {
    running: "var(--brand-300)",
    done:    "var(--success-400)",
    ok:      "var(--success-400)",
    failed:  "var(--err-400)",
    error:   "var(--err-400)",
  };
  return map[status] ?? "var(--fg-3)";
}

/** Status → fa icon. */
function statusIcon(status: string): string {
  if (status === "running") return "fa-spinner";
  if (status === "failed" || status === "error") return "fa-xmark";
  return "fa-check";
}

/** unix-seconds → relative string. */
function formatRelative(unixSec: number): string {
  if (!unixSec) return "—";
  const delta = Math.max(0, Date.now() - unixSec * 1000);
  const s = Math.round(delta / 1000);
  if (s < 60) return `${s}s ago`;
  const m = Math.round(s / 60);
  if (m < 60) return `${m}m ago`;
  const h = Math.round(m / 60);
  if (h < 24) return `${h}h ago`;
  return `${Math.round(h / 24)}d ago`;
}

/** done = anything that isn't running and isn't a failure. */
function isDone(status: string): boolean {
  return status !== "running" && status !== "failed" && status !== "error";
}

class LthnViewAgentActivity extends LitElement {
  static readonly properties = {
    w:        { type: Number },
    h:        { type: Number },
    embedded: { type: Boolean, reflect: true },
    runs:     { state: true },
    filter:   { state: true },
    loading:  { state: true },
    err:      { state: true },
  };

  declare w: number;
  declare h: number;
  declare embedded: boolean;
  declare runs: RunRow[];
  declare filter: RunFilter;
  declare loading: boolean;
  declare err: string;

  private _timer: ReturnType<typeof setInterval> | null = null;

  constructor() {
    super();
    this.w = 1180; this.h = 720; this.embedded = false;
    this.runs = [];
    this.filter = "all";
    this.loading = false;
    this.err = "";
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
      // Dynamic import keeps the Wails binding off the module-load
      // critical path so tests mocking @desktop/* run without the runtime.
      const svc = await import("@desktop/fleet/service");
      const r = await (svc as { Activity: (n: number) => Promise<{ Value: RunRow[] }> }).Activity(RUN_LIMIT);
      this.runs = r?.Value ?? [];
    } catch (e: unknown) {
      this.err = e instanceof Error ? e.message : String(e);
    } finally {
      this.loading = false;
    }
  }

  _counts(): { total: number; running: number; done: number; failed: number } {
    const c = { total: this.runs.length, running: 0, done: 0, failed: 0 };
    for (const r of this.runs) {
      if (r.status === "running") c.running++;
      else if (r.status === "failed" || r.status === "error") c.failed++;
      else c.done++;
    }
    return c;
  }

  _visible(): RunRow[] {
    switch (this.filter) {
      case "running": return this.runs.filter(r => r.status === "running");
      case "failed":  return this.runs.filter(r => r.status === "failed" || r.status === "error");
      case "done":    return this.runs.filter(r => isDone(r.status));
      default:        return this.runs;
    }
  }

  render() {
    const counts = this._counts();
    const visible = this._visible();

    const chipStyle = (active: boolean) => `
      padding:4px 10px; border-radius:4px; border:none; cursor:pointer;
      font-family:var(--font-mono); font-size:10.5px;
      background:${active ? "rgba(255,255,255,0.08)" : "transparent"};
      color:${active ? "var(--fg-0)" : "var(--fg-3)"};
      --wails-draggable: no-drag;
    `;

    const toolbar = html`
      <div style="display:inline-flex; border-radius:6px; padding:2px;
                  background:rgba(0,0,0,0.18); border:1px solid rgba(255,255,255,0.06); gap:2px;">
        <button @click=${() => this.filter = "all"}     style=${chipStyle(this.filter === "all")}>all · ${counts.total}</button>
        <button @click=${() => this.filter = "running"} style=${chipStyle(this.filter === "running")}>running · ${counts.running}</button>
        <button @click=${() => this.filter = "done"}    style=${chipStyle(this.filter === "done")}>done · ${counts.done}</button>
        <button @click=${() => this.filter = "failed"}  style=${chipStyle(this.filter === "failed")}>failed · ${counts.failed}</button>
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
          <div style="padding:10px 14px; color:var(--err-400);
                      background:rgba(255,76,76,0.06);
                      border:1px solid rgba(255,76,76,0.18);
                      border-radius:6px; font-size:12px; line-height:1.55; margin-bottom:14px;">
            ${this.err}
          </div>
        ` : nothing}

        ${visible.length === 0 && !this.loading ? html`
          <div style="padding:40px; text-align:center; color:var(--fg-3); font-size:12px;">
            ${counts.total === 0
              ? "No agent runs yet — dispatch a task and it'll show here."
              : `No runs match "${this.filter}".`}
          </div>
        ` : html`
          <div style="background:rgba(255,255,255,0.025); border:1px solid rgba(255,255,255,0.06); border-radius:10px; overflow:hidden;">
            ${visible.map((r, i) => html`
              <div style="padding:14px 16px;
                          border-bottom:${i < visible.length - 1 ? "1px solid rgba(255,255,255,0.04)" : "none"};
                          display:grid; grid-template-columns: 96px 1fr 120px 70px; gap:14px; align-items:center;">
                <span style="font-family:var(--font-mono); font-size:10px; padding:3px 8px; border-radius:999px;
                             background:${statusColour(r.status)}1a; border:1px solid ${statusColour(r.status)}40;
                             color:${statusColour(r.status)}; letter-spacing:0.03em;
                             display:inline-flex; align-items:center; gap:5px; justify-content:center;">
                  <i class="fa-solid ${statusIcon(r.status)}" style="font-size:9px;"></i>
                  ${r.status || "—"}
                </span>
                <div style="min-width:0;">
                  <div style="font-size:13px; color:var(--fg-0);
                              overflow:hidden; text-overflow:ellipsis; white-space:nowrap;">${r.summary || r.action || "(run)"}</div>
                  <div style="display:flex; align-items:center; gap:6px; margin-top:6px;">
                    <span style="font-family:var(--font-mono); font-size:10px; color:var(--fg-2);">${r.agent || "—"}</span>
                    ${r.model ? html`<span style="font-family:var(--font-mono); font-size:10px; color:var(--fg-3);">· ${r.model}</span>` : nothing}
                    ${r.action && r.summary ? html`<span style="font-family:var(--font-mono); font-size:10px; color:var(--fg-3);">· ${r.action}</span>` : nothing}
                  </div>
                </div>
                <span style="font-family:var(--font-mono); font-size:10px; color:var(--fg-3);
                             overflow:hidden; text-overflow:ellipsis; white-space:nowrap;">${r.machine_id || "—"}</span>
                <span style="font-family:var(--font-mono); font-size:10.5px; color:var(--fg-3); text-align:right;">
                  ${formatRelative(r.started_at)}
                </span>
              </div>
            `)}
          </div>
        `}
      </div>
    `;

    return renderChrome({
      title: "Activity",
      subtitle: `${counts.total} runs · ${counts.running} running`,
      w: this.w, h: this.h,
      toolbar, body,
      footer: html`${counts.total} recent · ${counts.running} running · ${counts.failed} failed · polls every 5s · data via Fleet.Activity()`,
      embedded: this.embedded,
    });
  }
}

customElements.define("lthn-view-agent-activity", LthnViewAgentActivity);

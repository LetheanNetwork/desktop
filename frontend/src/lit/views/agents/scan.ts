// SPDX-Licence-Identifier: EUPL-1.2
// Agents view · Scan — <lthn-view-agent-scan>
//
// Find dispatch candidates: scan a Forge org for open issues matching a
// label filter (agentic / help-wanted / bug), via Agents.Scan() →
// agentic_scan on the crew's lthn-agent. Read-only — it surfaces the work;
// you dispatch one from the Dispatch panel. Closes the scan → dispatch →
// watch loop.
//
// Backend: Agents.Scan() from @desktop/agents/service — dynamic import so
// tests run without the Wails runtime. On-demand (no poll) — scanning hits
// the Forge API, so it's a deliberate action.
//
// Supports the `embedded` attribute — no chrome when set.

import { LitElement, html, nothing } from "lit";
import { renderChrome } from "../../chrome";

interface ScanIssue {
  repo:     string;
  number:   number;
  title:    string;
  labels:   string[];
  assignee: string;
  url:      string;
}

class LthnViewAgentScan extends LitElement {
  static readonly properties = {
    w:        { type: Number },
    h:        { type: Number },
    embedded: { type: Boolean, reflect: true },
    org:      { state: true },
    labels:   { state: true },
    issues:   { state: true },
    scanned:  { state: true },
    busy:     { state: true },
    err:      { state: true },
  };

  declare w: number;
  declare h: number;
  declare embedded: boolean;
  declare org: string;
  declare labels: string;
  declare issues: ScanIssue[];
  declare scanned: boolean;
  declare busy: boolean;
  declare err: string;

  constructor() {
    super();
    this.w = 1180; this.h = 720; this.embedded = false;
    this.org = "core";
    this.labels = "agentic, help-wanted, bug";
    this.issues = [];
    this.scanned = false;
    this.busy = false;
    this.err = "";
  }

  createRenderRoot() { return this; }

  async _scan() {
    if (this.busy) return;
    this.busy = true;
    this.err = "";
    try {
      const svc = await import("@desktop/agents/service");
      const labels = this.labels.split(",").map(l => l.trim()).filter(Boolean);
      const r = await (svc as { Scan: (req: unknown) => Promise<{ Value: ScanIssue[] }> }).Scan({ org: this.org.trim(), labels });
      this.issues = r?.Value ?? [];
      this.scanned = true;
    } catch (e: unknown) {
      this.err = e instanceof Error ? e.message : String(e);
    } finally {
      this.busy = false;
    }
  }

  /** Defense-in-depth: only https issue links open. */
  _safeHref(u: string): string {
    return typeof u === "string" && u.startsWith("https://") ? u : "#";
  }

  render() {
    const fieldStyle = `
      padding:7px 9px; font-size:12px; background:rgba(0,0,0,0.25); color:var(--fg-0);
      border:1px solid rgba(255,255,255,0.08); border-radius:5px;
      font-family:inherit; --wails-draggable:no-drag;
    `;

    const toolbar = html`
      <div style="display:flex; align-items:center; gap:8px; flex:1;">
        <input style="${fieldStyle} width:120px;" placeholder="org" .value=${this.org}
          @input=${(e: Event) => { this.org = (e.target as HTMLInputElement).value; }}>
        <input style="${fieldStyle} flex:1; max-width:300px;" placeholder="labels (comma-separated)" .value=${this.labels}
          @input=${(e: Event) => { this.labels = (e.target as HTMLInputElement).value; }}>
        <lthn-btn tone="primary" size="sm" ?dim=${this.busy} @click=${() => void this._scan()}>
          <i class="fa-solid ${this.busy ? "fa-spinner" : "fa-magnifying-glass"}" style="font-size:10px;"></i>
          ${this.busy ? "Scanning…" : "Scan"}
        </lthn-btn>
      </div>
    `;

    const body = html`
      <div style="flex:1; padding:14px 22px 22px; overflow:auto;">
        ${this.err ? html`
          <div style="padding:14px; color:var(--err-400); font-size:12px; line-height:1.55;
                      background:rgba(255,76,76,0.06); border:1px solid rgba(255,76,76,0.18);
                      border-radius:10px; font-family:var(--font-mono);">
            ${this.err}
          </div>
        ` : this.issues.length === 0 ? html`
          <div style="padding:40px; text-align:center; color:var(--fg-3); font-size:12px;">
            ${this.scanned
              ? `No open issues in "${this.org}" match those labels.`
              : "Scan a Forge org for open agentic issues — dispatch candidates show here."}
          </div>
        ` : html`
          <div style="font-size:11px; color:var(--fg-3); margin-bottom:10px;">
            ${this.issues.length} open issue${this.issues.length === 1 ? "" : "s"} · dispatch one from the Dispatch panel
          </div>
          <div style="background:rgba(255,255,255,0.025); border:1px solid rgba(255,255,255,0.06); border-radius:10px; overflow:hidden;">
            ${this.issues.map((it, i) => html`
              <a href=${this._safeHref(it.url)} target="_blank" rel="noopener noreferrer"
                 style="text-decoration:none; color:inherit; display:block; --wails-draggable:no-drag;">
                <div style="padding:14px 16px; display:grid; grid-template-columns: 64px 1fr auto; gap:14px; align-items:center;
                            border-bottom:${i < this.issues.length - 1 ? "1px solid rgba(255,255,255,0.04)" : "none"};">
                  <span style="font-family:var(--font-mono); font-size:11px; color:var(--fg-3);">#${it.number}</span>
                  <div style="min-width:0;">
                    <div style="font-size:13px; color:var(--fg-0); overflow:hidden; text-overflow:ellipsis; white-space:nowrap;">${it.title || "(untitled)"}</div>
                    <div style="display:flex; align-items:center; gap:6px; margin-top:6px; flex-wrap:wrap;">
                      <span style="font-family:var(--font-mono); font-size:10px; color:var(--fg-2);">${it.repo}</span>
                      ${(it.labels ?? []).map(l => html`<span style="font-family:var(--font-mono); font-size:9.5px; padding:1px 6px; border-radius:999px; background:rgba(255,255,255,0.05); border:1px solid rgba(255,255,255,0.1); color:var(--fg-3);">${l}</span>`)}
                    </div>
                  </div>
                  <span style="font-family:var(--font-mono); font-size:10px; color:var(--fg-3);">${it.assignee || ""}</span>
                </div>
              </a>
            `)}
          </div>
        `}
      </div>
    `;

    return renderChrome({
      title: "Scan",
      subtitle: this.scanned ? `${this.issues.length} candidates in ${this.org}` : "find dispatch candidates",
      w: this.w, h: this.h,
      toolbar, body,
      footer: html`open Forge issues by label · dispatch via the Dispatch panel · data via Agents.Scan()`,
      embedded: this.embedded,
    });
  }
}

customElements.define("lthn-view-agent-scan", LthnViewAgentScan);

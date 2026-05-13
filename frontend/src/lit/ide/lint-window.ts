// SPDX-Licence-Identifier: EUPL-1.2
// IDE · lint — <lthn-lint-window>
//
// Runs core-lint against a workspace root, groups results by
// severity, lets the user filter by rule. Open via
// windowservice.Open("lint") with ?path=… for the root.
//
// No streaming — lint is a one-shot run that returns the full
// issues array. Severity buckets render as collapsible groups.

import { LitElement, html, nothing } from "lit";
import { renderChrome } from "../chrome";
import { T } from "@lthn/i18n/coreservice";

interface Issue {
  rule_id: string;
  title: string;
  severity: string;
  file: string;
  line: number;
  match: string;
  fix: string;
}

interface RunOutput {
  path: string;
  issues: Issue[];
  counts: Record<string, number>;
  total: number;
  binary_path: string;
  duration_ms: number;
}

class LthnLintWindow extends LitElement {
  static properties = {
    w:        { type: Number },
    h:        { type: Number },
    embedded: { type: Boolean, reflect: true },
    path:     { type: String },
    chrome:   { state: true },
    result:   { state: true },
    running:  { state: true },
    err:      { state: true },
    filter:   { state: true },
  };
  declare w: number;
  declare h: number;
  declare embedded: boolean;
  declare path: string;
  declare chrome: { title: string; subtitle: string };
  declare result: RunOutput | null;
  declare running: boolean;
  declare err: string;
  declare filter: string;

  constructor() {
    super();
    this.w = 1100; this.h = 740; this.embedded = false;
    this.path = "";
    this.chrome = { title: "Lint", subtitle: "rule · severity · file · line" };
    this.result = null;
    this.running = false;
    this.err = "";
    this.filter = "";
  }
  createRenderRoot() { return this; }

  async connectedCallback() {
    super.connectedCallback();
    const [title, subtitle] = await Promise.all([
      T("window.lint.title"),
      T("window.lint.subtitle"),
    ]);
    this.chrome = { title, subtitle };
    if (this.path) await this._run();
  }

  async _run() {
    if (!this.path || this.running) return;
    this.running = true;
    this.err = "";
    try {
      const svc = await import("@desktop/lint/service");
      this.result = await svc.Run(this.path, this.filter, "") as RunOutput;
    } catch (e: unknown) {
      this.err = e instanceof Error ? e.message : String(e);
      this.result = null;
    } finally {
      this.running = false;
    }
  }

  _severityTone(s: string): string {
    switch (s.toLowerCase()) {
      case "error":   case "blocker":  return "var(--err-400)";
      case "warning": case "warn":     return "var(--warning-400)";
      case "info":    case "notice":   return "var(--brand-300)";
      default:                         return "var(--fg-3)";
    }
  }

  render() {
    const r = this.result;
    const issues = r ? r.issues : [];
    const filtered = this.filter
      ? issues.filter(i => i.severity.toLowerCase() === this.filter.toLowerCase())
      : issues;
    const severities = r ? Object.entries(r.counts).sort((a, b) => b[1] - a[1]) : [];

    const toolbar = html`
      <span style="font-family:var(--font-mono); font-size:11px; color:var(--fg-3);
                   padding:0 6px;">${r ? `${r.total} issues · ${r.duration_ms} ms` : "no run yet"}</span>
      <div style="display:inline-flex; border-radius:6px; padding:2px;
                  background:rgba(0,0,0,0.18); border:1px solid rgba(255,255,255,0.06); gap:2px;">
        <button @click=${() => { this.filter = ""; void this._run(); }}
          style="padding:4px 10px; border-radius:4px; border:none; cursor:pointer;
                 font-family:var(--font-mono); font-size:10.5px;
                 background:${this.filter === "" ? "rgba(255,255,255,0.08)" : "transparent"};
                 color:${this.filter === "" ? "var(--fg-0)" : "var(--fg-3)"};
                 --wails-draggable: no-drag;">all</button>
        ${severities.map(([sev, n]) => html`
          <button @click=${() => { this.filter = sev; void this._run(); }}
            style="padding:4px 10px; border-radius:4px; border:none; cursor:pointer;
                   font-family:var(--font-mono); font-size:10.5px;
                   background:${this.filter === sev ? "rgba(255,255,255,0.08)" : "transparent"};
                   color:${this.filter === sev ? this._severityTone(sev) : "var(--fg-3)"};
                   --wails-draggable: no-drag;">${sev} · ${n}</button>
        `)}
      </div>
      <div style="flex:1"></div>
      <lthn-btn tone="primary" size="sm" ?dim=${this.running || !this.path}
        @click=${() => void this._run()}>
        <i class="fa-solid ${this.running ? "fa-spinner" : "fa-play"}" style="font-size:10px;"></i>
        ${this.running ? "Linting…" : "Run lint"}
      </lthn-btn>
    `;

    const body = html`
      <div style="flex:1; min-height:0; overflow:auto;">
        ${this.err ? html`
          <div style="padding:14px 18px; font-size:12px; color:var(--err-400);
                      background:rgba(255,76,76,0.06);
                      border-bottom:1px solid rgba(255,76,76,0.18);
                      line-height:1.55;">
            ${this.err}
          </div>
        ` : nothing}
        ${filtered.length === 0 && !this.running ? html`
          <div style="padding:40px; text-align:center; color:var(--fg-3); font-size:12px;">
            ${r ? "no issues — clean." : "pass ?path=/abs/path then click Run lint"}
          </div>
        ` : html`
          <div style="font-family:var(--font-mono); font-size:11px;">
            ${filtered.map((iss, i) => html`
              <div style="display:grid; grid-template-columns:90px 1fr 220px;
                          gap:14px; padding:10px 18px; align-items:start;
                          border-bottom:${i < filtered.length - 1 ? "1px solid rgba(255,255,255,0.04)" : "none"};">
                <span style="color:${this._severityTone(iss.severity)};
                             text-transform:uppercase; letter-spacing:0.04em;
                             font-size:10px; padding-top:2px;">${iss.severity}</span>
                <div style="display:flex; flex-direction:column; gap:4px;">
                  <div style="color:var(--fg-0); font-size:11.5px;">${iss.title}</div>
                  <div style="color:var(--fg-3); font-size:10px;">${iss.rule_id}</div>
                  ${iss.match ? html`
                    <pre style="margin:4px 0 0; padding:6px 8px; background:rgba(0,0,0,0.30);
                                border:1px solid rgba(255,255,255,0.05); border-radius:4px;
                                font-size:10.5px; color:var(--fg-2); white-space:pre-wrap;
                                line-height:1.45;">${iss.match}</pre>
                  ` : nothing}
                  ${iss.fix ? html`
                    <div style="font-size:10.5px; color:var(--success-400); margin-top:2px;">
                      fix: ${iss.fix}
                    </div>
                  ` : nothing}
                </div>
                <span style="color:var(--fg-2); font-size:10.5px;
                             white-space:nowrap; overflow:hidden; text-overflow:ellipsis;">
                  ${iss.file}${iss.line > 0 ? `:${iss.line}` : ""}
                </span>
              </div>
            `)}
          </div>
        `}
      </div>
    `;

    return renderChrome({
      title: this.chrome.title, subtitle: this.chrome.subtitle,
      w: this.w, h: this.h, toolbar, body,
      footer: html`${r ? `${r.path} · ${r.binary_path}` : this.path || "no workspace"} · ${filtered.length}/${issues.length} shown`,
      embedded: this.embedded,
    });
  }
}
customElements.define("lthn-lint-window", LthnLintWindow);

// SPDX-Licence-Identifier: EUPL-1.2

import { LitElement, html, nothing } from 'lit';
import { customElement, property, state } from 'lit/decorators.js';
import type { RunResult, RunAllResult } from './shared/api.js';

/**
 * <process-runner> — Pipeline execution status display.
 * Light-DOM Lit. Results supplied via the `result` property by the
 * surrounding window — this component renders summary + per-spec
 * breakdown only, no fetching of its own.
 */
@customElement('process-runner')
export class ProcessRunner extends LitElement {
  @property({ attribute: 'api-url' }) apiUrl = '';
  @property({ type: Object }) result: RunAllResult | null = null;

  @state() private expandedOutputs = new Set<string>();

  createRenderRoot() { return this; }

  connectedCallback() {
    super.connectedCallback();
    this.loadResults();
  }

  // eslint-disable-next-line @typescript-eslint/require-await
  async loadResults() {
    // Results are supplied via the `result` property; no auto-fetch yet.
  }

  private toggleOutput(name: string) {
    const next = new Set(this.expandedOutputs);
    if (next.has(name)) next.delete(name);
    else next.add(name);
    this.expandedOutputs = next;
  }

  private formatDuration(ns: number): string {
    if (ns < 1_000_000) return `${(ns / 1000).toFixed(0)}µs`;
    if (ns < 1_000_000_000) return `${(ns / 1_000_000).toFixed(0)}ms`;
    return `${(ns / 1_000_000_000).toFixed(2)}s`;
  }

  private resultVariant(r: RunResult): string {
    if (r.skipped) return 'warning';
    if (r.passed) return 'success';
    return 'danger';
  }
  private resultLabel(r: RunResult): string {
    if (r.skipped) return 'skipped';
    if (r.passed) return 'passed';
    return 'failed';
  }

  render() {
    if (!this.result) {
      return html`
        <div style="margin-bottom:0.75rem;padding:0.5rem 0.75rem;border:1px solid var(--line-2);border-radius:0.375rem;background:var(--ink-2);color:var(--fg-2);font-size:0.75rem;">
          No pipeline results to display. Pass <code style="font-family:'Geist Mono',monospace;color:var(--brand-300);">result</code> from the surrounding window when a pipeline finishes.
        </div>
      `;
    }

    const { results, duration, passed, failed, skipped, success } = this.result;

    return html`
      <div style="display:flex;gap:1rem;padding:0.625rem 0.75rem;border:1px solid var(--line-2);border-radius:0.5rem;background:var(--ink-2);margin-bottom:0.75rem;align-items:center;">
        <lthn-state-pill variant=${success ? 'success' : 'danger'}>${success ? 'Passed' : 'Failed'}</lthn-state-pill>
        <div style="display:flex;flex-direction:column;align-items:center;">
          <span style="font-weight:700;font-size:1.125rem;color:var(--success-400);">${passed}</span>
          <span style="font-size:0.6rem;color:var(--fg-3);text-transform:uppercase;letter-spacing:0.025em;">Passed</span>
        </div>
        <div style="display:flex;flex-direction:column;align-items:center;">
          <span style="font-weight:700;font-size:1.125rem;color:var(--danger-400);">${failed}</span>
          <span style="font-size:0.6rem;color:var(--fg-3);text-transform:uppercase;letter-spacing:0.025em;">Failed</span>
        </div>
        <div style="display:flex;flex-direction:column;align-items:center;">
          <span style="font-weight:700;font-size:1.125rem;color:var(--warning-400);">${skipped}</span>
          <span style="font-size:0.6rem;color:var(--fg-3);text-transform:uppercase;letter-spacing:0.025em;">Skipped</span>
        </div>
        <span style="margin-left:auto;font-family:'Geist Mono',monospace;font-size:0.75rem;color:var(--fg-3);">${this.formatDuration(duration)}</span>
      </div>

      <div style="display:flex;flex-direction:column;gap:0.375rem;">
        ${results.map((r) => html`
          <div style="border:1px solid var(--line-2);border-radius:0.5rem;padding:0.5rem 0.75rem;background:var(--ink-2);">
            <div style="display:flex;justify-content:space-between;align-items:center;">
              <div style="display:flex;align-items:center;gap:0.5rem;font-size:0.875rem;color:var(--fg-0);font-weight:600;">
                <span>${r.name}</span>
                <lthn-state-pill variant=${this.resultVariant(r)}>${this.resultLabel(r)}</lthn-state-pill>
              </div>
              <span style="font-family:'Geist Mono',monospace;font-size:0.7rem;color:var(--fg-3);">${this.formatDuration(r.duration)}</span>
            </div>
            ${r.exitCode !== 0 && !r.skipped ? html`
              <div style="margin-top:0.25rem;font-size:0.7rem;color:var(--fg-3);">exit ${r.exitCode}</div>
            ` : nothing}
            ${r.error ? html`
              <div style="margin-top:0.375rem;font-size:0.75rem;color:var(--danger-400);">${r.error}</div>
            ` : nothing}
            ${r.output ? html`
              <button @click=${() => this.toggleOutput(r.name)} style="font-size:0.7rem;color:var(--brand-300);cursor:pointer;background:none;border:none;padding:0;margin-top:0.375rem;text-decoration:underline;">
                ${this.expandedOutputs.has(r.name) ? 'Hide output' : 'Show output'}
              </button>
              ${this.expandedOutputs.has(r.name) ? html`
                <div style="margin-top:0.375rem;padding:0.5rem 0.625rem;background:var(--ink-0);border-radius:0.375rem;font-family:'Geist Mono','SF Mono',monospace;font-size:0.7rem;line-height:1.5;color:var(--fg-1);white-space:pre-wrap;word-break:break-all;max-height:10rem;overflow-y:auto;">${r.output}</div>
              ` : nothing}
            ` : nothing}
          </div>
        `)}
      </div>
    `;
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'process-runner': ProcessRunner;
  }
}

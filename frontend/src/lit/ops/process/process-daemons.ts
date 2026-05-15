// SPDX-Licence-Identifier: EUPL-1.2

import { LitElement, html, nothing } from 'lit';
import { customElement, property, state } from 'lit/decorators.js';
import { ProcessApi, type DaemonEntry, type HealthResult } from './shared/api.js';

/**
 * <process-daemons> — Daemon registry list.
 * Light-DOM Lit; styling inherits from lthn tokens.css. Health + Stop
 * actions surface as lthn-btn so the dark theme + brand affordances
 * apply without per-component CSS.
 */
@customElement('process-daemons')
export class ProcessDaemons extends LitElement {
  @property({ attribute: 'api-url' }) apiUrl = '';

  @state() private daemons: DaemonEntry[] = [];
  @state() private loading = true;
  @state() private error = '';
  @state() private stopping = new Set<string>();
  @state() private checking = new Set<string>();
  @state() private healthResults = new Map<string, HealthResult>();

  private api!: ProcessApi;

  createRenderRoot() { return this; }

  connectedCallback() {
    super.connectedCallback();
    this.api = new ProcessApi(this.apiUrl);
    this.loadDaemons();
  }

  async loadDaemons() {
    this.loading = true;
    this.error = '';
    try {
      this.daemons = await this.api.listDaemons();
    } catch (e) {
      this.error = (e as Error).message ?? 'Failed to load daemons';
    } finally {
      this.loading = false;
    }
  }

  private daemonKey(d: DaemonEntry): string {
    return `${d.code}/${d.daemon}`;
  }

  private async handleStop(d: DaemonEntry) {
    const key = this.daemonKey(d);
    this.stopping = new Set([...this.stopping, key]);
    try {
      await this.api.stopDaemon(d.code, d.daemon);
      this.dispatchEvent(new CustomEvent('daemon-stopped', {
        detail: { code: d.code, daemon: d.daemon },
        bubbles: true,
      }));
      await this.loadDaemons();
    } catch (e) {
      this.error = (e as Error).message ?? 'Failed to stop daemon';
    } finally {
      const next = new Set(this.stopping);
      next.delete(key);
      this.stopping = next;
    }
  }

  private async handleHealthCheck(d: DaemonEntry) {
    const key = this.daemonKey(d);
    this.checking = new Set([...this.checking, key]);
    try {
      const result = await this.api.healthCheck(d.code, d.daemon);
      const next = new Map(this.healthResults);
      next.set(key, result);
      this.healthResults = next;
    } catch (e) {
      this.error = (e as Error).message ?? 'Health check failed';
    } finally {
      const next = new Set(this.checking);
      next.delete(key);
      this.checking = next;
    }
  }

  private formatDate(iso: string): string {
    try {
      return new Date(iso).toLocaleDateString('en-GB', {
        day: 'numeric', month: 'short', year: 'numeric',
        hour: '2-digit', minute: '2-digit',
      });
    } catch {
      return iso;
    }
  }

  private renderHealthPill(d: DaemonEntry) {
    const key = this.daemonKey(d);
    if (this.checking.has(key)) {
      return html`<lthn-state-pill variant="warning">Checking</lthn-state-pill>`;
    }
    const result = this.healthResults.get(key);
    if (result) {
      return html`<lthn-state-pill variant=${result.healthy ? 'success' : 'danger'}>
        ${result.healthy ? 'Healthy' : 'Unhealthy'}
      </lthn-state-pill>`;
    }
    if (!d.health) {
      return html`<lthn-state-pill variant="muted">No probe</lthn-state-pill>`;
    }
    return html`<lthn-state-pill variant="muted">Unchecked</lthn-state-pill>`;
  }

  render() {
    if (this.loading) {
      return html`<div style="text-align:center;padding:1.5rem;color:var(--fg-3);font-size:0.8125rem;">Loading daemons…</div>`;
    }

    return html`
      ${this.error ? html`
        <div style="margin-bottom:0.75rem;padding:0.5rem 0.75rem;border:1px solid var(--danger-500);border-radius:0.375rem;background:color-mix(in oklch, var(--danger-500) 12%, transparent);color:var(--fg-0);font-size:0.8125rem;">
          ${this.error}
        </div>
      ` : nothing}

      ${this.daemons.length === 0 ? html`
        <div style="text-align:center;padding:1.5rem;color:var(--fg-3);font-size:0.8125rem;">No daemons registered.</div>
      ` : html`
        <div style="display:flex;flex-direction:column;gap:0.5rem;">
          ${this.daemons.map((d) => {
            const key = this.daemonKey(d);
            return html`
              <div style="display:flex;justify-content:space-between;align-items:center;padding:0.625rem 0.75rem;border:1px solid var(--line-2);border-radius:0.5rem;background:var(--ink-2);">
                <div style="flex:1;min-width:0;">
                  <div style="display:flex;align-items:center;gap:0.5rem;font-size:0.875rem;">
                    <span style="font-family:'Geist Mono',monospace;color:var(--brand-300);font-weight:600;">${d.code}</span>
                    <span style="color:var(--fg-0);font-weight:600;">${d.daemon}</span>
                    ${this.renderHealthPill(d)}
                  </div>
                  <div style="display:flex;gap:0.875rem;margin-top:0.25rem;font-size:0.7rem;color:var(--fg-3);">
                    <span style="font-family:'Geist Mono',monospace;padding:0.0625rem 0.375rem;border-radius:0.25rem;background:var(--ink-3);">PID ${d.pid}</span>
                    <span>Started ${this.formatDate(d.started)}</span>
                    ${d.project ? html`<span>${d.project}</span>` : nothing}
                    ${d.binary ? html`<span>${d.binary}</span>` : nothing}
                  </div>
                </div>
                <div style="display:flex;gap:0.375rem;">
                  ${d.health ? html`
                    <lthn-btn tone="ghost" size="sm"
                      ?dim=${this.checking.has(key)}
                      @click=${() => this.handleHealthCheck(d)}>
                      <i class="fa-solid fa-heart-pulse" style="font-size:10px;"></i>
                      ${this.checking.has(key) ? 'Checking' : 'Health'}
                    </lthn-btn>
                  ` : nothing}
                  <lthn-btn tone="danger" size="sm"
                    ?dim=${this.stopping.has(key)}
                    @click=${() => this.handleStop(d)}>
                    <i class="fa-solid fa-stop" style="font-size:10px;"></i>
                    ${this.stopping.has(key) ? 'Stopping' : 'Stop'}
                  </lthn-btn>
                </div>
              </div>
            `;
          })}
        </div>
      `}
    `;
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'process-daemons': ProcessDaemons;
  }
}

// SPDX-Licence-Identifier: EUPL-1.2

import { LitElement, html, nothing } from 'lit';
import { customElement, property, state } from 'lit/decorators.js';
import { connectProcessEvents, type ProcessEvent } from './shared/events.js';

// Side-effect imports to register child elements
import './process-daemons.js';
import './process-list.js';
import './process-output.js';
import './process-runner.js';

type TabId = 'daemons' | 'processes' | 'pipelines';

/**
 * <process-panel> — Top-level process management surface.
 *
 * Light-DOM Lit element rendered inside <lthn-process-window>. Composes
 * <process-daemons> / <process-list> + <process-output> / <process-runner>
 * under a header + tab bar + footer. Reads from lthn's REST mount at
 * `api-url` (defaulting to `/api/process`) and streams events from
 * `ws-url` when configured.
 */
@customElement('process-panel')
export class ProcessPanel extends LitElement {
  @property({ attribute: 'api-url' }) apiUrl = '/api/process';
  @property({ attribute: 'ws-url' }) wsUrl = '';

  @state() private activeTab: TabId = 'daemons';
  @state() private wsConnected = false;
  @state() private lastEvent = '';
  @state() private selectedProcessId = '';

  private ws: WebSocket | null = null;

  createRenderRoot() { return this; }

  connectedCallback() {
    super.connectedCallback();
    if (this.wsUrl) {
      this.connectWs();
    }
  }

  disconnectedCallback() {
    super.disconnectedCallback();
    if (this.ws) {
      this.ws.close();
      this.ws = null;
    }
  }

  private connectWs() {
    this.ws = connectProcessEvents(this.wsUrl, (event: ProcessEvent) => {
      this.lastEvent = event.channel ?? event.type ?? '';
      this.requestUpdate();
    });
    this.ws.onopen = () => { this.wsConnected = true; };
    this.ws.onclose = () => { this.wsConnected = false; };
  }

  private handleTabClick(tab: TabId) {
    this.activeTab = tab;
  }

  private handleRefresh() {
    const content = this.querySelector('.pp-content');
    if (!content) return;
    const child = content.firstElementChild;
    if (!child) return;
    if ('loadDaemons' in child) (child as { loadDaemons: () => void }).loadDaemons();
    else if ('loadProcesses' in child) (child as { loadProcesses: () => void }).loadProcesses();
    else if ('loadResults' in child) (child as { loadResults: () => void }).loadResults();
  }

  private handleProcessSelected(e: CustomEvent<{ id: string }>) {
    this.selectedProcessId = e.detail.id;
  }

  private renderContent() {
    switch (this.activeTab) {
      case 'daemons':
        return html`<process-daemons api-url=${this.apiUrl}></process-daemons>`;
      case 'processes':
        return html`
          <process-list
            api-url=${this.apiUrl}
            ws-url=${this.wsUrl}
            @process-selected=${this.handleProcessSelected}
          ></process-list>
          ${this.selectedProcessId
            ? html`<process-output
                api-url=${this.apiUrl}
                ws-url=${this.wsUrl}
                process-id=${this.selectedProcessId}
              ></process-output>`
            : nothing}
        `;
      case 'pipelines':
        return html`<process-runner api-url=${this.apiUrl}></process-runner>`;
      default:
        return nothing;
    }
  }

  private readonly tabs: { id: TabId; icon: string; label: string }[] = [
    { id: 'daemons',   icon: 'fa-server',          label: 'Daemons' },
    { id: 'processes', icon: 'fa-microchip',       label: 'Processes' },
    { id: 'pipelines', icon: 'fa-diagram-project', label: 'Pipelines' },
  ];

  render() {
    const wsVariant = this.wsUrl
      ? (this.wsConnected ? 'success' : 'danger')
      : 'muted';
    const wsLabel = this.wsUrl
      ? (this.wsConnected ? 'Live' : 'Disconnected')
      : 'No event stream';

    return html`
      <div class="pp-root" style="display:flex;flex-direction:column;flex:1;min-height:0;color:var(--fg-1);">
        <div class="pp-header" style="display:flex;justify-content:space-between;align-items:center;padding:0.5rem 0.75rem;border-bottom:1px solid var(--line-2);background:var(--ink-2);">
          <div class="pp-tabs" style="display:flex;gap:0.25rem;">
            ${this.tabs.map(t => html`
              <lthn-btn
                tone=${this.activeTab === t.id ? 'primary' : 'ghost'}
                size="sm"
                ?active=${this.activeTab === t.id}
                @click=${() => this.handleTabClick(t.id)}>
                <i class="fa-solid ${t.icon}" style="font-size:10px;"></i> ${t.label}
              </lthn-btn>
            `)}
          </div>
          <lthn-btn tone="ghost" size="sm" @click=${this.handleRefresh}>
            <i class="fa-solid fa-rotate" style="font-size:10px;"></i> Refresh
          </lthn-btn>
        </div>

        <div class="pp-content" style="flex:1;min-height:0;overflow-y:auto;padding:0.75rem;background:var(--ink-1);">
          ${this.renderContent()}
        </div>

        <div class="pp-footer" style="display:flex;justify-content:space-between;align-items:center;padding:0.375rem 0.75rem;border-top:1px solid var(--line-2);background:var(--ink-2);font-size:0.7rem;color:var(--fg-3);">
          <span style="display:inline-flex;align-items:center;gap:0.375rem;">
            <lthn-status-dot variant=${wsVariant} ?pulse=${this.wsConnected}></lthn-status-dot>
            ${wsLabel}
          </span>
          ${this.lastEvent ? html`<span>Last: ${this.lastEvent}</span>` : nothing}
        </div>
      </div>
    `;
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'process-panel': ProcessPanel;
  }
}

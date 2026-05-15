// SPDX-Licence-Identifier: EUPL-1.2

import { LitElement, html, nothing } from 'lit';
import { customElement, property, state } from 'lit/decorators.js';
import { connectProcessEvents, type ProcessEvent } from './shared/events.js';
import { ProcessApi } from './shared/api.js';

interface OutputLine {
  text: string;
  stream: 'stdout' | 'stderr';
  timestamp: number;
}

/**
 * <process-output> — Live stdout/stderr stream for a selected process.
 * Light-DOM Lit. Snapshot from REST, then live append from WS events
 * matching the configured process-id. Terminal-style mono block sits on
 * `--ink-0` (the deepest dark surface) to read like a console.
 */
@customElement('process-output')
export class ProcessOutput extends LitElement {
  @property({ attribute: 'api-url' }) apiUrl = '';
  @property({ attribute: 'ws-url' }) wsUrl = '';
  @property({ attribute: 'process-id' }) processId = '';

  @state() private lines: OutputLine[] = [];
  @state() private autoScroll = true;
  @state() private connected = false;
  @state() private loadingSnapshot = false;

  private ws: WebSocket | null = null;
  private api = new ProcessApi(this.apiUrl);
  private syncToken = 0;

  createRenderRoot() { return this; }

  connectedCallback() {
    super.connectedCallback();
    this.syncSources();
  }

  disconnectedCallback() {
    super.disconnectedCallback();
    this.disconnect();
  }

  updated(changed: Map<string, unknown>) {
    if (changed.has('apiUrl')) {
      this.api = new ProcessApi(this.apiUrl);
    }
    if (changed.has('processId') || changed.has('wsUrl') || changed.has('apiUrl')) {
      this.syncSources();
    }
    if (this.autoScroll) this.scrollToBottom();
  }

  private syncSources() {
    this.disconnect();
    this.lines = [];
    if (!this.processId) return;
    void this.loadSnapshotAndConnect();
  }

  private async loadSnapshotAndConnect() {
    const token = ++this.syncToken;
    if (!this.processId) return;
    if (this.apiUrl) {
      this.loadingSnapshot = true;
      try {
        const output = await this.api.getProcessOutput(this.processId);
        if (token !== this.syncToken) return;
        const snapshot = this.linesFromOutput(output);
        if (snapshot.length > 0) this.lines = snapshot;
      } catch {
        // Ignore missing snapshot — continue with live streaming.
      } finally {
        if (token === this.syncToken) this.loadingSnapshot = false;
      }
    }
    if (token === this.syncToken && this.wsUrl) this.connect();
  }

  private linesFromOutput(output: string): OutputLine[] {
    if (!output) return [];
    const normalized = output.replace(/\r\n/g, '\n');
    const parts = normalized.split('\n');
    if (parts.length > 0 && parts[parts.length - 1] === '') parts.pop();
    return parts.map((text) => ({
      text, stream: 'stdout' as const, timestamp: Date.now(),
    }));
  }

  private connect() {
    this.ws = connectProcessEvents(this.wsUrl, (event: ProcessEvent) => {
      const data = event.data;
      if (!data) return;
      const channel = event.channel ?? event.type ?? '';
      if (channel === 'process.output' && data.id === this.processId) {
        this.lines = [...this.lines, {
          text: data.line ?? '',
          stream: data.stream === 'stderr' ? 'stderr' : 'stdout',
          timestamp: Date.now(),
        }];
      }
    });
    this.ws.onopen = () => { this.connected = true; };
    this.ws.onclose = () => { this.connected = false; };
  }

  private disconnect() {
    if (this.ws) {
      this.ws.close();
      this.ws = null;
    }
    this.connected = false;
  }

  private handleClear() { this.lines = []; }
  private handleAutoScrollToggle() { this.autoScroll = !this.autoScroll; }

  private scrollToBottom() {
    const body = this.querySelector('.po-body') as HTMLElement | null;
    if (body) body.scrollTop = body.scrollHeight;
  }

  render() {
    if (!this.processId) {
      return html`<div style="text-align:center;padding:1.5rem;color:var(--fg-3);font-size:0.8125rem;">Select a process to view its output.</div>`;
    }

    return html`
      <div style="margin-top:0.5rem;border:1px solid var(--line-2);border-radius:0.5rem;overflow:hidden;background:var(--ink-0);">
        <div style="display:flex;justify-content:space-between;align-items:center;padding:0.4rem 0.625rem;background:var(--ink-2);border-bottom:1px solid var(--line-2);font-size:0.7rem;color:var(--fg-2);">
          <span style="font-family:'Geist Mono',monospace;color:var(--fg-1);">Output: ${this.processId} ${this.connected ? html`<lthn-state-pill variant="success">Live</lthn-state-pill>` : nothing}</span>
          <div style="display:flex;align-items:center;gap:0.5rem;">
            <label style="display:inline-flex;align-items:center;gap:0.25rem;font-size:0.7rem;cursor:pointer;">
              <input type="checkbox" ?checked=${this.autoScroll} @change=${this.handleAutoScrollToggle} style="cursor:pointer;" />
              Auto-scroll
            </label>
            <lthn-btn tone="ghost" size="sm" @click=${this.handleClear}>
              <i class="fa-solid fa-eraser" style="font-size:10px;"></i> Clear
            </lthn-btn>
          </div>
        </div>
        <div class="po-body" style="max-height:24rem;overflow-y:auto;padding:0.5rem 0.75rem;font-family:'Geist Mono','SF Mono',monospace;font-size:0.75rem;line-height:1.5;color:var(--fg-1);">
          ${this.loadingSnapshot && this.lines.length === 0 ? html`
            <div style="text-align:center;padding:0.75rem;color:var(--fg-3);font-style:italic;">Loading snapshot…</div>
          ` : this.lines.length === 0 ? html`
            <div style="text-align:center;padding:0.75rem;color:var(--fg-3);font-style:italic;">Waiting for output…</div>
          ` : this.lines.map((line) => html`
            <div style="white-space:pre-wrap;word-break:break-all;color:${line.stream === 'stderr' ? 'var(--danger-400)' : 'var(--fg-1)'};">
              <span style="display:inline-block;width:3rem;font-size:0.625rem;font-weight:600;text-transform:uppercase;opacity:0.4;margin-right:0.5rem;">${line.stream}</span>${line.text}
            </div>
          `)}
        </div>
      </div>
    `;
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'process-output': ProcessOutput;
  }
}

// SPDX-Licence-Identifier: EUPL-1.2

import { LitElement, html, nothing } from 'lit';
import { customElement, property, state } from 'lit/decorators.js';
import { connectProcessEvents, type ProcessEvent } from './shared/events.js';
import { ProcessApi, type ProcessInfo } from './shared/api.js';

/**
 * <process-list> — Running processes with status and actions.
 * Light-DOM Lit; styling inherits from lthn tokens.css. Emits
 * `process-selected` when a row is clicked so the panel's output viewer
 * can attach.
 */
@customElement('process-list')
export class ProcessList extends LitElement {
  @property({ attribute: 'api-url' }) apiUrl = '';
  @property({ attribute: 'ws-url' }) wsUrl = '';
  @property({ attribute: 'selected-id' }) selectedId = '';

  @state() private processes: ProcessInfo[] = [];
  @state() private loading = false;
  @state() private error = '';
  @state() private connected = false;
  @state() private killing = new Set<string>();

  private api!: ProcessApi;
  private ws: WebSocket | null = null;

  createRenderRoot() { return this; }

  connectedCallback() {
    super.connectedCallback();
    this.api = new ProcessApi(this.apiUrl);
    this.loadProcesses();
  }

  disconnectedCallback() {
    super.disconnectedCallback();
    this.disconnect();
  }

  updated(changed: Map<string, unknown>) {
    if (changed.has('apiUrl')) {
      this.api = new ProcessApi(this.apiUrl);
    }
    if (changed.has('wsUrl') || changed.has('apiUrl')) {
      this.disconnect();
      void this.loadProcesses();
    }
  }

  async loadProcesses() {
    this.loading = true;
    this.error = '';
    try {
      this.processes = await this.api.listProcesses();
      if (this.wsUrl) this.connect();
    } catch (e) {
      this.error = (e as Error).message ?? 'Failed to load processes';
      this.processes = [];
    } finally {
      this.loading = false;
    }
  }

  private handleSelect(proc: ProcessInfo) {
    this.dispatchEvent(new CustomEvent('process-selected', {
      detail: { id: proc.id }, bubbles: true, composed: true,
    }));
  }

  private async handleKill(proc: ProcessInfo) {
    this.killing = new Set([...this.killing, proc.id]);
    try {
      await this.api.killProcess(proc.id);
      await this.loadProcesses();
    } catch (e) {
      this.error = (e as Error).message ?? 'Failed to kill process';
    } finally {
      const next = new Set(this.killing);
      next.delete(proc.id);
      this.killing = next;
    }
  }

  private connect() {
    if (!this.wsUrl || this.ws) return;
    this.ws = connectProcessEvents(this.wsUrl, (event: ProcessEvent) => {
      this.applyEvent(event);
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

  private applyEvent(event: ProcessEvent) {
    const channel = event.channel ?? event.type ?? '';
    const data = (event.data ?? {}) as Partial<ProcessInfo> & { id?: string; error?: unknown };
    if (!data.id) return;
    const id: string = data.id;
    const payload = { ...data, id } as Partial<ProcessInfo> & { id: string; error?: unknown };
    const next = new Map(this.processes.map((proc) => [proc.id, proc] as const));
    const current = next.get(id);

    switch (channel) {
      case 'process.started':
        next.set(id, this.normalizeProcess(payload, current, 'running'));
        break;
      case 'process.exited':
        next.set(id, this.normalizeProcess(payload, current, data.exitCode === -1 && data.error ? 'failed' : 'exited'));
        break;
      case 'process.killed':
        next.set(id, this.normalizeProcess(payload, current, 'killed'));
        break;
      default:
        return;
    }
    this.processes = this.sortProcesses(next);
  }

  private normalizeProcess(
    data: Partial<ProcessInfo> & { id: string; error?: unknown },
    current: ProcessInfo | undefined,
    status: ProcessInfo['status'],
  ): ProcessInfo {
    const startedAt = data.startedAt ?? current?.startedAt ?? new Date().toISOString();
    return {
      id: data.id,
      command: data.command ?? current?.command ?? '',
      args: data.args ?? current?.args ?? [],
      dir: data.dir ?? current?.dir ?? '',
      startedAt,
      running: status === 'running',
      status,
      exitCode: data.exitCode ?? current?.exitCode ?? (status === 'killed' ? -1 : 0),
      duration: data.duration ?? current?.duration ?? 0,
      pid: data.pid ?? current?.pid ?? 0,
    };
  }

  private sortProcesses(processes: Map<string, ProcessInfo>): ProcessInfo[] {
    return [...processes.values()].sort((a, b) => {
      const aStarted = new Date(a.startedAt).getTime();
      const bStarted = new Date(b.startedAt).getTime();
      if (aStarted === bStarted) return a.id.localeCompare(b.id);
      return aStarted - bStarted;
    });
  }

  private formatUptime(started: string): string {
    try {
      const ms = Date.now() - new Date(started).getTime();
      const seconds = Math.floor(ms / 1000);
      if (seconds < 60) return `${seconds}s`;
      const minutes = Math.floor(seconds / 60);
      if (minutes < 60) return `${minutes}m ${seconds % 60}s`;
      const hours = Math.floor(minutes / 60);
      return `${hours}h ${minutes % 60}m`;
    } catch {
      return 'unknown';
    }
  }

  private statusVariant(s: ProcessInfo['status']): string {
    switch (s) {
      case 'running': return 'info';
      case 'pending': return 'warning';
      case 'exited':  return 'success';
      case 'failed':  return 'danger';
      case 'killed':  return 'danger';
      default:        return 'muted';
    }
  }

  render() {
    if (this.loading) {
      return html`<div style="text-align:center;padding:1.5rem;color:var(--fg-3);font-size:0.8125rem;">Loading processes…</div>`;
    }

    return html`
      ${this.error ? html`
        <div style="margin-bottom:0.75rem;padding:0.5rem 0.75rem;border:1px solid var(--danger-500);border-radius:0.375rem;background:color-mix(in oklch, var(--danger-500) 12%, transparent);color:var(--fg-0);font-size:0.8125rem;">
          ${this.error}
        </div>
      ` : nothing}

      ${this.processes.length === 0 ? html`
        <div style="margin-bottom:0.75rem;padding:0.5rem 0.75rem;border:1px solid var(--line-2);border-radius:0.375rem;background:var(--ink-2);color:var(--fg-2);font-size:0.75rem;">
          ${this.wsUrl
            ? (this.connected ? 'Receiving live process updates.' : 'Connecting to the process event stream…')
            : 'Managed processes are loaded from the REST surface; no event stream wired.'}
        </div>
        <div style="text-align:center;padding:1.5rem;color:var(--fg-3);font-size:0.8125rem;">No managed processes.</div>
      ` : html`
        <div style="display:flex;flex-direction:column;gap:0.375rem;">
          ${this.processes.map((proc) => {
            const selected = this.selectedId === proc.id;
            const argLine = `${proc.command} ${proc.args?.join(' ') ?? ''}`.trim();
            return html`
              <div
                @click=${() => this.handleSelect(proc)}
                style="display:flex;justify-content:space-between;align-items:center;padding:0.5rem 0.75rem;border:1px solid ${selected ? 'var(--brand-500)' : 'var(--line-2)'};border-radius:0.5rem;background:var(--ink-2);cursor:pointer;${selected ? 'box-shadow:0 0 0 1px var(--brand-500);' : ''}"
              >
                <div style="flex:1;min-width:0;">
                  <div style="display:flex;align-items:center;gap:0.5rem;font-family:'Geist Mono',monospace;font-size:0.8125rem;color:var(--fg-0);">
                    <span style="overflow:hidden;text-overflow:ellipsis;white-space:nowrap;max-width:32rem;">${argLine}</span>
                    <lthn-state-pill variant=${this.statusVariant(proc.status)}>${proc.status}</lthn-state-pill>
                  </div>
                  <div style="display:flex;gap:0.875rem;margin-top:0.25rem;font-size:0.7rem;color:var(--fg-3);">
                    <span style="font-family:'Geist Mono',monospace;padding:0.0625rem 0.375rem;border-radius:0.25rem;background:var(--ink-3);">PID ${proc.pid}</span>
                    <span style="font-family:'Geist Mono',monospace;">${proc.id}</span>
                    ${proc.dir ? html`<span>${proc.dir}</span>` : nothing}
                    ${proc.status === 'running'
                      ? html`<span>Up ${this.formatUptime(proc.startedAt)}</span>`
                      : nothing}
                    ${proc.status === 'exited'
                      ? html`<span style="font-family:'Geist Mono',monospace;padding:0.0625rem 0.375rem;border-radius:0.25rem;background:${proc.exitCode !== 0 ? 'color-mix(in oklch, var(--danger-500) 18%, transparent)' : 'var(--ink-3)'};">exit ${proc.exitCode}</span>`
                      : nothing}
                  </div>
                </div>
                ${proc.status === 'running' ? html`
                  <lthn-btn tone="danger" size="sm"
                    ?dim=${this.killing.has(proc.id)}
                    @click=${(e: Event) => { e.stopPropagation(); void this.handleKill(proc); }}>
                    <i class="fa-solid fa-stop" style="font-size:10px;"></i>
                    ${this.killing.has(proc.id) ? 'Killing' : 'Kill'}
                  </lthn-btn>
                ` : nothing}
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
    'process-list': ProcessList;
  }
}

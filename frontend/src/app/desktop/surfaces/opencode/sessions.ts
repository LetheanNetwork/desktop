import { ChangeDetectionStrategy, Component, OnDestroy, OnInit, inject, signal } from '@angular/core';
import { SurfaceBridgeService } from '../surface-bridge.service';
import { SurfaceRoute } from '../surface-page';

/** One running opencode-serve container, as WStatus returns it. */
export interface OpencodeSandbox {
  readonly ID: string;
  readonly Image: string;
  readonly HostPort: number;
  readonly Status: string;
  readonly CreatedAt: string;
}

/** One stored launch profile, as WListProfiles returns it. */
export interface OpencodeProfileSummary {
  readonly name: string;
  readonly description?: string;
  readonly model?: string;
}

const BRIDGE = 'dappco.re/lthn/desktop/pkg/opencode.WailsService';

@Component({
  selector: 'lthn-opencode-sessions-surface',
  imports: [],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <section class="sessions">
      <header>
        <span class="icon"><i class="fa-solid fa-terminal" aria-hidden="true"></i></span>
        <div>
          <div class="eyebrow" i18n="OpenCode eyebrow@@surface.opencode.eyebrow">OpenCode</div>
          <h2 i18n="OpenCode sessions title@@surface.opencode.sessions.title">Sessions</h2>
          <p i18n="OpenCode sessions subtitle@@surface.opencode.sessions.subtitle">
            Sandboxed agent runs — each session is one container on the local runner.
          </p>
        </div>
        <span class="security">
          <i class="fa-solid fa-shield-halved" aria-hidden="true"></i>
          <span i18n="OpenCode sessions security state@@surface.opencode.sessions.security"
            >containerised · local endpoint</span
          >
        </span>
      </header>

      <div class="launch">
        <label>
          <span i18n="OpenCode profile label@@surface.opencode.sessions.profileLabel">Profile</span>
          <select [value]="profile()" (change)="onProfile($event)" [disabled]="busy()">
            <option value="" i18n="OpenCode default profile option@@surface.opencode.sessions.defaultProfile">
              Default profile
            </option>
            @for (p of profiles(); track p.name) {
              <option [value]="p.name">{{ p.name }}{{ p.model ? ' · ' + p.model : '' }}</option>
            }
          </select>
        </label>
        <button type="button" class="primary" (click)="launch()" [disabled]="busy()">
          <i class="fa-solid fa-play" aria-hidden="true"></i>
          <ng-container i18n="OpenCode launch button@@surface.opencode.sessions.launch"
            >Launch session</ng-container
          >
        </button>
        <button type="button" (click)="refresh(true)" [disabled]="busy()">
          <i class="fa-solid fa-rotate" aria-hidden="true"></i>
          <ng-container i18n="OpenCode refresh button@@surface.opencode.sessions.refresh"
            >Refresh</ng-container
          >
        </button>
      </div>

      @if (notice()) {
        <div class="notice" role="status">{{ notice() }}</div>
      }

      @if (sandboxes().length) {
        <div class="table" role="table">
          <div class="row head" role="row">
            <span i18n="OpenCode column session@@surface.opencode.sessions.colSession">Session</span>
            <span i18n="OpenCode column image@@surface.opencode.sessions.colImage">Image</span>
            <span i18n="OpenCode column port@@surface.opencode.sessions.colPort">Port</span>
            <span i18n="OpenCode column status@@surface.opencode.sessions.colStatus">Status</span>
            <span i18n="OpenCode column started@@surface.opencode.sessions.colStarted">Started</span>
            <span class="actions" i18n="OpenCode column actions@@surface.opencode.sessions.colActions"
              >Actions</span
            >
          </div>
          @for (sb of sandboxes(); track sb.ID) {
            <div class="row" role="row">
              <span class="mono">{{ sb.ID }}</span>
              <span class="mono muted">{{ sb.Image }}</span>
              <span class="mono">{{ sb.HostPort }}</span>
              <span class="chip" [class.ok]="sb.Status === 'running'">{{ sb.Status }}</span>
              <span class="muted">{{ started(sb) }}</span>
              <span class="actions">
                <button
                  type="button"
                  (click)="openWeb(sb.ID)"
                  [disabled]="busy()"
                  i18n-title="OpenCode open web tooltip@@surface.opencode.sessions.openWebTitle"
                  title="Open the session's web UI in a native window"
                >
                  <i class="fa-solid fa-window-restore" aria-hidden="true"></i>
                </button>
                <button
                  type="button"
                  (click)="openTui(sb.ID)"
                  [disabled]="busy()"
                  i18n-title="OpenCode open TUI tooltip@@surface.opencode.sessions.openTuiTitle"
                  title="Attach the TUI in your terminal"
                >
                  <i class="fa-solid fa-terminal" aria-hidden="true"></i>
                </button>
                <button
                  type="button"
                  class="danger"
                  (click)="stop(sb.ID)"
                  [disabled]="busy()"
                  i18n-title="OpenCode stop tooltip@@surface.opencode.sessions.stopTitle"
                  title="Stop and remove the session container"
                >
                  <i class="fa-solid fa-stop" aria-hidden="true"></i>
                </button>
              </span>
            </div>
          }
        </div>
      } @else {
        <div class="empty" i18n="OpenCode empty state@@surface.opencode.sessions.empty">
          No sessions are running. Launch one to start a sandboxed agent.
        </div>
      }
    </section>
  `,
  styles: `
    :host {
      display: block;
      width: 100%;
      height: 100%;
    }
    .sessions {
      box-sizing: border-box;
      display: flex;
      flex-direction: column;
      width: 100%;
      height: 100%;
      gap: 11px;
      padding: 20px;
      overflow: hidden auto;
      color: var(--fg-0);
      background: var(--ink-1);
      font-family: var(--font-sans);
    }
    header {
      display: flex;
      align-items: center;
      gap: 11px;
    }
    header > div {
      flex: 1;
    }
    .icon {
      display: grid;
      flex: 0 0 34px;
      width: 34px;
      height: 34px;
      place-items: center;
      color: var(--brand-300);
      background: rgba(64, 193, 197, 0.09);
      border: 1px solid rgba(64, 193, 197, 0.18);
      border-radius: 9px;
    }
    .eyebrow {
      color: var(--brand-300);
      font: 9px var(--font-mono);
      letter-spacing: 0.12em;
      text-transform: uppercase;
    }
    h2,
    p {
      margin: 0;
    }
    h2 {
      margin-top: 3px;
      font-size: 20px;
    }
    p {
      margin-top: 4px;
      color: var(--fg-2);
      font-size: 11px;
    }
    .security {
      color: var(--brand-200);
      font: 9px var(--font-mono);
    }
    .security i {
      margin-right: 5px;
    }
    .launch {
      display: flex;
      align-items: end;
      gap: 8px;
    }
    .launch label {
      display: flex;
      flex-direction: column;
      gap: 4px;
      color: var(--fg-2);
      font-size: 10px;
    }
    .launch select {
      min-width: 220px;
      padding: 6px 8px;
      color: var(--fg-0);
      background: var(--ink-2);
      border: 1px solid rgba(64, 193, 197, 0.2);
      border-radius: 6px;
      font: 11px var(--font-mono);
    }
    .launch button,
    .actions button {
      display: inline-flex;
      align-items: center;
      gap: 6px;
      padding: 7px 11px;
      color: var(--fg-0);
      background: var(--ink-2);
      border: 1px solid rgba(64, 193, 197, 0.2);
      border-radius: 6px;
      font: 11px var(--font-sans);
      cursor: pointer;
    }
    .launch button.primary {
      color: var(--ink-1);
      background: var(--brand-300);
      border-color: var(--brand-300);
    }
    .launch button:disabled,
    .actions button:disabled {
      opacity: 0.5;
      cursor: default;
    }
    .notice {
      padding: 7px 9px;
      color: var(--brand-200);
      background: rgba(64, 193, 197, 0.06);
      border: 1px solid rgba(64, 193, 197, 0.17);
      border-radius: 6px;
      font: 9px var(--font-mono);
    }
    .table {
      display: flex;
      flex-direction: column;
      border: 1px solid rgba(64, 193, 197, 0.14);
      border-radius: 8px;
      overflow: hidden;
    }
    .row {
      display: grid;
      grid-template-columns: 1.2fr 1.4fr 0.5fr 0.7fr 1fr 0.9fr;
      align-items: center;
      gap: 8px;
      padding: 8px 12px;
      border-top: 1px solid rgba(64, 193, 197, 0.08);
      font-size: 11px;
    }
    .row.head {
      color: var(--fg-2);
      background: var(--ink-2);
      border-top: none;
      font: 9px var(--font-mono);
      letter-spacing: 0.08em;
      text-transform: uppercase;
    }
    .mono {
      font-family: var(--font-mono);
    }
    .muted {
      color: var(--fg-2);
    }
    .chip {
      justify-self: start;
      padding: 2px 8px;
      color: var(--fg-2);
      border: 1px solid rgba(64, 193, 197, 0.2);
      border-radius: 999px;
      font: 9px var(--font-mono);
      text-transform: uppercase;
    }
    .chip.ok {
      color: var(--brand-200);
      border-color: rgba(64, 193, 197, 0.45);
      background: rgba(64, 193, 197, 0.08);
    }
    .actions {
      display: flex;
      justify-content: end;
      gap: 5px;
    }
    .actions button {
      padding: 5px 8px;
    }
    .actions button.danger {
      color: #e2626b;
      border-color: rgba(226, 98, 107, 0.35);
    }
    .empty {
      padding: 28px;
      color: var(--fg-2);
      border: 1px dashed rgba(64, 193, 197, 0.2);
      border-radius: 8px;
      font-size: 11px;
      text-align: center;
    }
  `,
})
export class OpencodeSessionsSurface extends SurfaceRoute implements OnInit, OnDestroy {
  private readonly bridge = inject(SurfaceBridgeService);
  private offEvent: (() => void) | undefined;

  readonly sandboxes = signal<readonly OpencodeSandbox[]>([]);
  readonly profiles = signal<readonly OpencodeProfileSummary[]>([]);
  readonly profile = signal('');
  readonly notice = signal('');
  readonly busy = signal(false);

  ngOnInit(): void {
    void this.refresh(false);
    void this.loadProfiles();
    void this.subscribe();
  }

  ngOnDestroy(): void {
    this.offEvent?.();
  }

  onProfile(event: Event): void {
    this.profile.set((event.target as HTMLSelectElement).value);
  }

  started(sb: OpencodeSandbox): string {
    const stamp = Date.parse(sb.CreatedAt);
    return Number.isNaN(stamp) ? sb.CreatedAt : new Date(stamp).toLocaleString('en-GB');
  }

  async refresh(announce: boolean): Promise<void> {
    try {
      const value = await this.bridge.call(`${BRIDGE}.WStatus`);
      this.sandboxes.set(Array.isArray(value) ? (value as OpencodeSandbox[]) : []);
      if (announce) {
        this.notice.set(
          $localize`:OpenCode refresh done@@surface.opencode.sessions.refreshed:Session list refreshed.`,
        );
      }
    } catch (error) {
      this.fail(error);
    }
  }

  async launch(): Promise<void> {
    this.busy.set(true);
    try {
      const id = await this.bridge.call(`${BRIDGE}.WStart`, [this.profile()]);
      this.notice.set(
        $localize`:OpenCode launch done@@surface.opencode.sessions.launched:Session ${String(id)}:id: started.`,
      );
      await this.refresh(false);
    } catch (error) {
      this.fail(error);
    } finally {
      this.busy.set(false);
    }
  }

  async stop(id: string): Promise<void> {
    this.busy.set(true);
    try {
      await this.bridge.call(`${BRIDGE}.WStop`, [id]);
      this.notice.set(
        $localize`:OpenCode stop done@@surface.opencode.sessions.stopped:Session ${id}:id: stopped.`,
      );
      await this.refresh(false);
    } catch (error) {
      this.fail(error);
    } finally {
      this.busy.set(false);
    }
  }

  async openWeb(id: string): Promise<void> {
    try {
      await this.bridge.call(`${BRIDGE}.WOpenWebWindow`, [id]);
    } catch (error) {
      this.fail(error);
    }
  }

  async openTui(id: string): Promise<void> {
    try {
      await this.bridge.call(`${BRIDGE}.WOpenTUI`, [id]);
    } catch (error) {
      this.fail(error);
    }
  }

  private async loadProfiles(): Promise<void> {
    try {
      const value = await this.bridge.call(`${BRIDGE}.WListProfiles`);
      this.profiles.set(Array.isArray(value) ? (value as OpencodeProfileSummary[]) : []);
    } catch {
      // Profiles are an optional convenience — the default profile
      // launches without them, so a load failure stays silent here.
    }
  }

  private async subscribe(): Promise<void> {
    try {
      const { Events } = await import('@wailsio/runtime');
      this.offEvent = Events.On('lthn:opencode:event', () => void this.refresh(false));
    } catch {
      // Outside the Wails shell (browser demo) there is no event bus;
      // the manual Refresh button remains the update path.
    }
  }

  private fail(error: unknown): void {
    const message = error instanceof Error ? error.message : String(error);
    this.notice.set(message);
  }
}

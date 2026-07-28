import {
  ChangeDetectionStrategy,
  Component,
  OnDestroy,
  OnInit,
  inject,
  signal,
} from '@angular/core';
import {
  terminalTabsFromWorkspace,
  terminalWorkspaceFromTabs,
} from '../../terminal-workspace.models';
import { TerminalWorkspaceService } from '../../terminal-workspace.service';
import { SurfaceRoute } from '../surface-page';
import { SurfaceBridgeService } from '../surface-bridge.service';
import { AgentTerminalSession, TerminalReady, TerminalTab } from './terminal-session';

@Component({
  selector: 'lthn-agents-terminal-surface',
  imports: [AgentTerminalSession],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <section class="terminal">
      <header>
        <div class="identity">
          <span class="icon"><i class="fa-solid fa-terminal" aria-hidden="true"></i></span>
          <div>
            <h2 i18n="Agents terminal title@@surface.agents.terminal.title">Terminal</h2>
            <p>{{ subtitle() }}</p>
          </div>
        </div>
        <span class="transport">
          <i class="fa-solid fa-wave-square" aria-hidden="true"></i>
          <span i18n="Terminal transport@@surface.agents.terminal.transport"
            >PTY · Wails event bus</span
          >
        </span>
      </header>

      <nav aria-label="Terminal sessions" i18n-aria-label>
        @for (tab of tabs(); track tab.key) {
          <button
            type="button"
            class="tab"
            [class.is-active]="tab.key === activeKey()"
            [class.has-exited]="tab.exited"
            [title]="tab.cwd || tab.title"
            (click)="activateTab(tab.key)"
          >
            <i
              class="fa-solid"
              [class.fa-link]="tab.attachId"
              [class.fa-terminal]="!tab.attachId"
              aria-hidden="true"
            ></i>
            <span>{{ tab.title }}{{ tab.exited ? ' ✗' : '' }}</span>
            <i
              class="fa-solid fa-xmark close"
              role="button"
              tabindex="0"
              aria-label="Close terminal"
              i18n-aria-label="Terminal close tab@@surface.agents.terminal.closeTab"
              (click)="closeTab(tab.key, $event)"
              (keydown.enter)="closeTab(tab.key, $event)"
            ></i>
          </button>
        }
        <button
          type="button"
          class="new-tab"
          (click)="addTab({})"
          title="New terminal"
          i18n-title="Terminal new tab@@surface.agents.terminal.newTab"
        >
          <i class="fa-solid fa-plus" aria-hidden="true"></i>
        </button>
      </nav>

      <div class="sessions">
        @if (offline()) {
          <div class="demo-terminal" role="status">
            <span>$ lthn demo</span>
            <span>Lethean Desktop browser workspace</span>
            <span class="muted">No local process or Wails listener is active.</span>
          </div>
        } @else {
          @for (tab of tabs(); track tab.key) {
            @if (tab.exited) {
              <div
                class="exited-session"
                [style.display]="tab.key === activeKey() ? 'grid' : 'none'"
              >
                <i class="fa-solid fa-terminal" aria-hidden="true"></i>
                <strong i18n="Exited terminal title@@surface.agents.terminal.exitedTitle"
                  >Session ended</strong
                >
                <span i18n="Exited terminal explanation@@surface.agents.terminal.exitedBody"
                  >The previous process is no longer available.</span
                >
                <button type="button" (click)="restartTab(tab.key)">
                  <i class="fa-solid fa-rotate-right" aria-hidden="true"></i>
                  <span i18n="Terminal fresh shell@@surface.agents.terminal.freshShell"
                    >Start a fresh shell</span
                  >
                </button>
              </div>
            } @else {
              <lthn-agent-terminal-session
                [tab]="tab"
                [active]="tab.key === activeKey()"
                [style.display]="tab.key === activeKey() ? 'flex' : 'none'"
                (ready)="onReady($event)"
                (exited)="onExit($event)"
              />
            }
          }
        }
      </div>

      <footer>
        <span>pkg/terminal · {{ tabs().length }} {{ tabs().length === 1 ? 'tab' : 'tabs' }}</span>
        @if (workspaceError()) {
          <span class="workspace-error" role="alert">{{ workspaceError() }}</span>
        } @else {
          <span i18n="Terminal footer shortcuts@@surface.agents.terminal.footer"
            >⌘F search · bytes over the Wails event bus</span
          >
        }
      </footer>
    </section>
  `,
  styles: `
    :host {
      display: block;
      width: 100%;
      height: 100%;
      min-height: 0;
    }
    .terminal {
      display: flex;
      flex-direction: column;
      width: 100%;
      height: 100%;
      min-height: 0;
      overflow: hidden;
      color: var(--fg-0);
      background: var(--ink-1);
      font-family: var(--font-sans);
    }
    header {
      display: flex;
      flex: 0 0 auto;
      align-items: center;
      justify-content: space-between;
      gap: 16px;
      padding: 15px 18px 13px;
      border-bottom: 1px solid var(--line-1);
    }
    .identity {
      display: flex;
      align-items: center;
      gap: 10px;
      min-width: 0;
    }
    .icon {
      display: grid;
      flex: 0 0 31px;
      width: 31px;
      height: 31px;
      place-items: center;
      color: var(--brand-300);
      background: rgba(64, 193, 197, 0.09);
      border: 1px solid rgba(64, 193, 197, 0.18);
      border-radius: 8px;
    }
    h2,
    p {
      margin: 0;
    }
    h2 {
      font-size: 17px;
    }
    p {
      margin-top: 3px;
      color: var(--fg-3);
      font: 9px var(--font-mono);
      white-space: nowrap;
      overflow: hidden;
      text-overflow: ellipsis;
    }
    .transport {
      color: var(--brand-300);
      font: 8px var(--font-mono);
      white-space: nowrap;
    }
    .transport i {
      margin-right: 5px;
    }
    nav {
      display: flex;
      flex: 0 0 auto;
      align-items: center;
      gap: 4px;
      padding: 7px 10px;
      overflow-x: auto;
      border-bottom: 1px solid var(--line-1);
    }
    .tab,
    .new-tab {
      display: flex;
      align-items: center;
      gap: 7px;
      flex: 0 0 auto;
      min-height: 28px;
      padding: 0 9px;
      color: var(--fg-2);
      background: rgba(255, 255, 255, 0.025);
      border: 1px solid var(--line-1);
      border-radius: 6px;
      font: 9px var(--font-mono);
      cursor: pointer;
    }
    .tab.is-active {
      color: var(--brand-200);
      background: rgba(64, 193, 197, 0.1);
      border-color: rgba(64, 193, 197, 0.28);
    }
    .tab.has-exited {
      opacity: 0.55;
    }
    .tab span {
      max-width: 150px;
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
    }
    .close {
      padding: 3px;
      opacity: 0.5;
    }
    .close:hover {
      opacity: 1;
    }
    .new-tab {
      width: 29px;
      padding: 0;
      justify-content: center;
      border-style: dashed;
    }
    .sessions {
      position: relative;
      display: flex;
      flex: 1;
      min-width: 0;
      min-height: 0;
    }
    lthn-agent-terminal-session {
      position: absolute;
      inset: 0;
    }
    .demo-terminal,
    .exited-session {
      position: absolute;
      inset: 0;
      align-content: center;
      justify-items: center;
      gap: 9px;
      color: var(--fg-2);
      background:
        radial-gradient(circle at 50% 45%, rgba(64, 193, 197, 0.07), transparent 36%), #0b0e11;
      font: 10px var(--font-mono);
    }
    .demo-terminal {
      display: grid;
      justify-items: start;
      align-content: start;
      padding: 22px;
      color: var(--brand-200);
    }
    .demo-terminal .muted,
    .exited-session span {
      color: var(--fg-3);
    }
    .exited-session > i {
      color: var(--brand-300);
      font-size: 26px;
    }
    .exited-session button {
      display: inline-flex;
      align-items: center;
      gap: 7px;
      margin-top: 6px;
      padding: 7px 10px;
      color: var(--ink-0);
      background: var(--brand-300);
      border: 0;
      border-radius: 6px;
      font: 9px var(--font-mono);
      cursor: pointer;
    }
    footer {
      display: flex;
      flex: 0 0 auto;
      justify-content: space-between;
      gap: 12px;
      padding: 7px 12px;
      color: var(--fg-3);
      border-top: 1px solid var(--line-1);
      font: 8px var(--font-mono);
    }
    .workspace-error {
      color: var(--danger-300);
    }
  `,
})
export class AgentsTerminalSurface extends SurfaceRoute implements OnInit, OnDestroy {
  private readonly bridge = inject(SurfaceBridgeService);
  private readonly workspace = inject(TerminalWorkspaceService);
  private sequence = 0;
  private hydrated = false;

  readonly tabs = signal<readonly TerminalTab[]>([]);
  readonly activeKey = signal('');
  readonly offline = signal(false);
  readonly workspaceError = this.workspace.error;
  readonly subtitle = signal(
    $localize`:Agents terminal subtitle@@surface.agents.terminal.subtitle:PTY shell — your machine, in the app`,
  );

  ngOnInit(): void {
    window.addEventListener('lthn:open-terminal', this.openFromEvent);
    this.offline.set(this.workspace.isOffline());
    void this.initialise();
  }

  ngOnDestroy(): void {
    window.removeEventListener('lthn:open-terminal', this.openFromEvent);
    void this.workspace.flush();
  }

  addTab(
    options: {
      repo?: string;
      cwd?: string;
      mountId?: string;
      workspacePath?: string;
      attachId?: string;
      sharedAgentId?: string;
      command?: readonly string[];
      shared?: boolean;
      title?: string;
    },
    activate = true,
  ): void {
    const key = this.nextKey();
    const title =
      options.title ||
      options.repo ||
      options.mountId ||
      (options.attachId
        ? `↪ ${options.attachId.slice(0, 8)}`
        : options.command?.length
          ? basename(options.command[0])
          : options.cwd
            ? basename(options.cwd)
            : `shell ${this.sequence}`);
    const tab: TerminalTab = { key, title, ...options };
    this.tabs.update((tabs) => [...tabs, tab]);
    if (activate) this.activeKey.set(key);
    this.scheduleWorkspace();
  }

  activateTab(key: string): void {
    const tab = this.tabs().find((candidate) => candidate.key === key);
    if (!tab || key === this.activeKey()) return;
    this.activeKey.set(key);
    this.subtitle.set(tab.cwd || tab.title);
    this.scheduleWorkspace();
  }

  closeTab(key: string, event?: Event): void {
    event?.stopPropagation();
    const current = this.tabs();
    const index = current.findIndex((tab) => tab.key === key);
    const remaining = current.filter((tab) => tab.key !== key);
    this.tabs.set(remaining);
    if (this.activeKey() === key) {
      this.activeKey.set(remaining[Math.max(0, index - 1)]?.key ?? '');
    }
    if (!remaining.length) this.addTab({});
    else this.scheduleWorkspace();
  }

  restartTab(key: string): void {
    this.tabs.update((tabs) =>
      tabs.map((tab) =>
        tab.key === key
          ? {
              ...tab,
              attachId: undefined,
              sharedAgentId: undefined,
              shared: false,
              exited: false,
            }
          : tab,
      ),
    );
    this.activeKey.set(key);
    this.scheduleWorkspace();
  }

  onReady(event: TerminalReady): void {
    this.tabs.update((tabs) =>
      tabs.map((tab) =>
        tab.key === event.key
          ? {
              ...tab,
              title: tab.repo || event.title || tab.title,
              cwd: event.cwd || tab.cwd,
            }
          : tab,
      ),
    );
    if (event.key === this.activeKey()) {
      this.subtitle.set(event.cwd || event.title);
    }
    this.scheduleWorkspace();
  }

  onExit(key: string): void {
    this.tabs.update((tabs) =>
      tabs.map((tab) => (tab.key === key ? { ...tab, exited: true } : tab)),
    );
    this.scheduleWorkspace();
  }

  private readonly openFromEvent = (event: Event): void => {
    const detail = (
      event as CustomEvent<{
        repo?: string;
        cwd?: string;
        path?: string;
        mountId?: string;
        workspacePath?: string;
        attachId?: string;
        sharedAgentId?: string;
        command?: readonly string[];
        shared?: boolean;
        title?: string;
      }>
    ).detail;
    if (!detail) return;
    this.addTab({
      ...detail,
      cwd: detail.cwd || (!detail.mountId ? detail.path : undefined),
      workspacePath: detail.workspacePath || (detail.mountId ? detail.path : undefined),
    });
  };

  private async initialise(): Promise<void> {
    let liveAgents: readonly LiveAgentSession[] = [];
    if (!this.offline()) {
      liveAgents = await this.loadLiveAgents();
    }

    try {
      const snapshot = await this.workspace.load();
      const liveIDs = new Set(liveAgents.map(({ id }) => id));
      const restored = terminalTabsFromWorkspace(snapshot.workspace, liveIDs);
      if (restored.length) {
        this.tabs.set(restored);
        this.activeKey.set(
          restored.some(({ key }) => key === snapshot.workspace.activeKey)
            ? snapshot.workspace.activeKey
            : restored[0].key,
        );
        this.sequence = restored.reduce((maximum, tab) => {
          const match = /^terminal-(\d+)$/u.exec(tab.key);
          return Math.max(maximum, match ? Number(match[1]) : 0);
        }, 0);
      }
    } catch {
      // The workspace service retains the error; safe tabs remain operable.
    }

    if (!this.tabs().length) this.addTab({});
    for (const session of liveAgents) {
      if (
        this.tabs().some(
          ({ attachId, sharedAgentId }) => attachId === session.id || sharedAgentId === session.id,
        )
      ) {
        continue;
      }
      this.addTab(
        {
          attachId: session.id,
          sharedAgentId: session.id,
          shared: true,
          title: session.label || session.shell || 'agent',
        },
        false,
      );
    }
    this.hydrated = true;
  }

  private async loadLiveAgents(): Promise<readonly LiveAgentSession[]> {
    try {
      const value = await this.bridge.call('dappco.re/lthn/desktop/pkg/terminal.Service.List');
      return parseLiveAgentSessions(value);
    } catch {
      return [];
    }
  }

  private nextKey(): string {
    let key = '';
    do {
      key = `terminal-${++this.sequence}`;
    } while (this.tabs().some((tab) => tab.key === key));
    return key;
  }

  private scheduleWorkspace(): void {
    if (!this.hydrated) return;
    this.workspace.schedule(terminalWorkspaceFromTabs(this.tabs(), this.activeKey()));
  }
}

interface LiveAgentSession {
  readonly id: string;
  readonly label: string;
  readonly shell: string;
}

function parseLiveAgentSessions(value: unknown): readonly LiveAgentSession[] {
  if (!value || typeof value !== 'object') return [];
  const sessions = (value as { sessions?: unknown }).sessions;
  if (!Array.isArray(sessions)) return [];
  const live: LiveAgentSession[] = [];
  for (const candidate of sessions) {
    if (!candidate || typeof candidate !== 'object') continue;
    const session = candidate as Record<string, unknown>;
    if (session['kind'] !== 'agent' || typeof session['id'] !== 'string') continue;
    live.push({
      id: session['id'],
      label: typeof session['label'] === 'string' ? session['label'] : '',
      shell: typeof session['shell'] === 'string' ? session['shell'] : '',
    });
  }
  return live;
}

function basename(value: string): string {
  return value.split('/').filter(Boolean).at(-1) || 'shell';
}

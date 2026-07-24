import {
  ChangeDetectionStrategy,
  Component,
  OnDestroy,
  OnInit,
  inject,
  signal,
} from '@angular/core';
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
            (click)="activeKey.set(tab.key)"
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
        @for (tab of tabs(); track tab.key) {
          <lthn-agent-terminal-session
            [tab]="tab"
            [active]="tab.key === activeKey()"
            [style.display]="tab.key === activeKey() ? 'flex' : 'none'"
            (ready)="onReady($event)"
            (exited)="onExit($event)"
          />
        }
      </div>

      <footer>
        <span>pkg/terminal · {{ tabs().length }} {{ tabs().length === 1 ? 'tab' : 'tabs' }}</span>
        <span i18n="Terminal footer shortcuts@@surface.agents.terminal.footer"
          >⌘F search · bytes over the Wails event bus</span
        >
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
  `,
})
export class AgentsTerminalSurface extends SurfaceRoute implements OnInit, OnDestroy {
  private readonly bridge = inject(SurfaceBridgeService);
  private sequence = 0;

  readonly tabs = signal<readonly TerminalTab[]>([]);
  readonly activeKey = signal('');
  readonly subtitle = signal(
    $localize`:Agents terminal subtitle@@surface.agents.terminal.subtitle:PTY shell — your machine, in the app`,
  );

  ngOnInit(): void {
    window.addEventListener('lthn:open-terminal', this.openFromEvent);
    this.addTab({});
    void this.discoverAgents();
  }

  ngOnDestroy(): void {
    window.removeEventListener('lthn:open-terminal', this.openFromEvent);
  }

  addTab(
    options: {
      repo?: string;
      cwd?: string;
      attachId?: string;
      command?: readonly string[];
      shared?: boolean;
      title?: string;
    },
    activate = true,
  ): void {
    const key = `terminal-${++this.sequence}`;
    const title =
      options.title ||
      options.repo ||
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
  }

  onExit(key: string): void {
    this.tabs.update((tabs) =>
      tabs.map((tab) => (tab.key === key ? { ...tab, exited: true } : tab)),
    );
  }

  private readonly openFromEvent = (event: Event): void => {
    const detail = (
      event as CustomEvent<{
        repo?: string;
        cwd?: string;
        path?: string;
        attachId?: string;
        command?: readonly string[];
        shared?: boolean;
        title?: string;
      }>
    ).detail;
    if (!detail) return;
    this.addTab({ ...detail, cwd: detail.cwd || detail.path });
  };

  private async discoverAgents(): Promise<void> {
    try {
      const value = await this.bridge.call('dappco.re/lthn/desktop/pkg/terminal.Service.List');
      if (!value || typeof value !== 'object') return;
      const sessions = (value as { sessions?: unknown }).sessions;
      if (!Array.isArray(sessions)) return;
      for (const candidate of sessions) {
        if (!candidate || typeof candidate !== 'object') continue;
        const session = candidate as Record<string, unknown>;
        if (
          session['kind'] !== 'agent' ||
          typeof session['id'] !== 'string' ||
          this.tabs().some(({ attachId }) => attachId === session['id'])
        ) {
          continue;
        }
        this.addTab(
          {
            attachId: session['id'],
            shared: true,
            title:
              typeof session['label'] === 'string'
                ? session['label']
                : typeof session['shell'] === 'string'
                  ? session['shell']
                  : 'agent',
          },
          false,
        );
      }
    } catch {
      // Browser preview and a desktop with no running agents keep one local tab.
    }
  }
}

function basename(value: string): string {
  return value.split('/').filter(Boolean).at(-1) || 'shell';
}

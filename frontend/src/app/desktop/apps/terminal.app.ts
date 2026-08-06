import { ChangeDetectionStrategy, Component, Input, computed, inject } from '@angular/core';
import { DesktopLiveDataService } from '../desktop-live-data.service';
import { type AppDef } from '../desktop-catalogue.data';
import { Win } from '../desktop.data';
import { devPanelFor, type DevPanelView } from '../dev-panel.data';
import { AgentsTerminalSurface } from '../surfaces/agents/terminal';
import { AppView } from './app-view';
import { DevPanelApp } from './dev-panel.app';

// Terminal is a pane of the IDE, not an application, so its offline fixture
// carries its own panel identity rather than reading one out of the catalogue.
const TERMINAL_PANEL: AppDef = {
  id: 'ide.terminal',
  title: $localize`:Application title@@app.terminal.title:Terminal`,
  icon: 'terminal',
  category: 'developer',
  w: 760,
  h: 460,
  hint: $localize`:Application launcher hint@@app.terminal.hint:Local PTY · browser demo`,
  dev: true,
  route: 'terminal',
};

@Component({
  selector: 'lthn-terminal-app',
  imports: [DevPanelApp, AgentsTerminalSurface],
  host: { style: 'display: contents' },
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    @if (demoMode()) {
      <section class="terminal-demo">
        <header class="demo-status">
          <span class="demo-badge">{{ demoLabel }}</span>
          <span i18n="Offline terminal demo description@@terminal.demo.description"
            >Offline transport · browser-safe terminal fixture</span
          >
        </header>
        <lthn-dev-panel-app [win]="win" [app]="app" [panel]="panel" [empty]="empty" />
      </section>
    } @else {
      <lthn-agents-terminal-surface [win]="win" />
    }
  `,
  styles: `
    .terminal-demo {
      display: flex;
      flex: 1;
      flex-direction: column;
      min-height: 0;
      height: 100%;
    }

    .demo-status {
      display: flex;
      align-items: center;
      gap: 8px;
      padding: 7px 12px;
      border-bottom: 1px solid var(--line);
      color: var(--fg-3);
      font-size: 11px;
    }

    .demo-badge {
      display: inline-flex;
      align-items: center;
      height: 22px;
      padding: 0 10px;
      border: 1px solid color-mix(in oklch, var(--warning-500) 35%, transparent);
      border-radius: var(--r-pill);
      background: color-mix(in oklch, var(--warning-500) 22%, var(--ink-2));
      color: var(--warning-400);
      font-size: 11.5px;
      font-weight: 500;
    }
  `,
})
export class TerminalApp implements AppView {
  private readonly liveData = inject(DesktopLiveDataService);

  @Input({ required: true }) win!: Win;
  @Input() app: AppDef = TERMINAL_PANEL;
  @Input() panel: DevPanelView = devPanelFor('terminal');
  @Input() empty: [string, string, string] | null = null;

  readonly demoLabel = $localize`:Demo data state@@desktop.data.demo:Demo data`;
  readonly demoMode = computed(() => this.liveData.mode() === 'demo');
}

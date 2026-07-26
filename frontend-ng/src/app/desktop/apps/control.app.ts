// SPDX-License-Identifier: EUPL-1.2

// Control's route-level container owns navigation, live/demo reconciliation,
// and WebMCP. Section components render typed inputs and emit action intents.
import {
  ChangeDetectionStrategy,
  Component,
  CUSTOM_ELEMENTS_SCHEMA,
  Input,
  OnInit,
  declareExperimentalWebMcpTool,
  inject,
  signal,
} from '@angular/core';
import { AppView } from './app-view';
import { CTRL_NAV, type Win } from '../desktop.data';
import type { AppNavItem } from '../desktop-route-tree';
import { DesktopLiveDataService } from '../desktop-live-data.service';
import { WindowManagerService } from '../window-manager.service';
import { ControlModelsView } from './control/control-models.view';
import { ControlPowerView } from './control/control-power.view';
import { ControlRunsView } from './control/control-runs.view';
import { ControlSettingsView } from './control/control-settings.view';
import { ControlSystemView } from './control/control-system.view';
import { createDemoControlViewState, mergeControlLiveSnapshot } from './control/control-view-state';
import type {
  ControlActionIntent,
  ControlSystemTab,
  ControlViewState,
} from './control/control-view.models';

@Component({
  selector: 'lthn-control-app',
  standalone: true,
  imports: [
    ControlModelsView,
    ControlRunsView,
    ControlPowerView,
    ControlSystemView,
    ControlSettingsView,
  ],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  host: { style: 'display: contents' },
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <nav class="rail">
      @for (item of nav; track item[0]; let last = $last) {
        <a
          [class.on]="(win.sub || 'models') === item[0]"
          [class.last]="last"
          (click)="wm.setSub(win.id, item[0])"
        >
          <lthn-icon [attr.name]="item[1]" [attr.aria-label]="item[2]" size="15"></lthn-icon>
        </a>
      }
    </nav>
    <div class="appbody">
      @switch (win.sub || 'models') {
        @case ('models') {
          <lthn-control-models-view
            [dataState]="viewState().dataState"
            [model]="viewState().models"
            (loadModel)="handleAction({ kind: 'load-model' })"
          />
        }
        @case ('runs') {
          <lthn-control-runs-view
            [dataState]="viewState().dataState"
            [model]="viewState().runs"
            (newRun)="handleAction({ kind: 'new-run' })"
          />
        }
        @case ('power') {
          <lthn-control-power-view
            [dataState]="viewState().dataState"
            [model]="viewState().power"
          />
        }
        @case ('system') {
          <lthn-control-system-view
            [dataState]="viewState().dataState"
            [model]="viewState().system"
            [activeTab]="systemTab()"
            (tabChange)="wm.setSysTab(win.id, $event)"
          />
        }
        @case ('settings') {
          <lthn-control-settings-view
            [dataState]="viewState().dataState"
            [model]="viewState().settings"
            (commit)="handleAction({ kind: 'commit-settings' })"
          />
        }
      }
    </div>
  `,
})
export class ControlApp implements AppView, OnInit {
  @Input() win!: Win;
  @Input() nav: AppNavItem[] = [];

  readonly wm = inject(WindowManagerService);
  private readonly liveData = inject(DesktopLiveDataService);
  private readonly mcpTools = this.registerMcpTools();
  readonly viewState = signal<ControlViewState>({
    ...createDemoControlViewState(),
    dataState: this.liveData.mode() === 'demo' ? 'demo' : 'loading',
  });

  ngOnInit(): void {
    if (this.liveData.mode() === 'demo') return;
    void this.refresh();
  }

  async refresh(): Promise<void> {
    if (this.liveData.mode() === 'demo') return;
    this.viewState.update((state) => ({ ...state, dataState: 'loading' }));
    try {
      this.viewState.set(mergeControlLiveSnapshot(await this.liveData.control()));
    } catch {
      this.viewState.set({
        ...createDemoControlViewState(),
        dataState: 'unavailable',
      });
    }
  }

  systemTab(): ControlSystemTab {
    const tab = this.win.systab;
    return tab === 'processes' || tab === 'daemons' ? tab : 'overview';
  }

  handleAction(_intent: ControlActionIntent): void {
    // The existing placeholder buttons remain inert until their typed backend
    // actions in TODO.md are implemented.
  }

  private async registerMcpTools(): Promise<void> {
    const sections = CTRL_NAV.map(([id]) => id);

    await Promise.all([
      declareExperimentalWebMcpTool({
        name: 'control_read_state',
        description:
          'Reads the Control app section and the model state currently presented by the app.',
        inputSchema: {
          type: 'object',
          properties: {},
          additionalProperties: false,
        },
        execute: () => ({
          content: [
            {
              type: 'text',
              text: JSON.stringify({
                section: this.win.sub || 'models',
                system_tab: this.win.systab || 'overview',
                models: this.viewState().models.rows,
              }),
            },
          ],
        }),
      }),
      declareExperimentalWebMcpTool({
        name: 'control_show_section',
        description:
          'Changes the visible Control app section without mutating model or system state.',
        inputSchema: {
          type: 'object',
          properties: {
            section: {
              type: 'string',
              enum: sections,
              description: 'Control navigation section.',
            },
          },
          required: ['section'],
          additionalProperties: false,
        },
        execute: ({ section }) => {
          if (!sections.includes(section)) {
            throw new Error(
              `Unknown Control section "${section}". Expected one of: ${sections.join(', ')}.`,
            );
          }
          this.wm.setSub(this.win.id, section);
          return {
            content: [
              {
                type: 'text',
                text: JSON.stringify({ ok: true, section }),
              },
            ],
          };
        },
      }),
    ]);
  }
}

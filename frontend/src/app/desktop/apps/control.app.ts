// SPDX-License-Identifier: EUPL-1.2

// Control's route-level container owns navigation, live/demo reconciliation,
// and WebMCP. Section components render typed inputs and emit action intents.
import {
  ChangeDetectionStrategy,
  Component,
  CUSTOM_ELEMENTS_SCHEMA,
  Input,
  OnDestroy,
  OnInit,
  PendingTasks,
  computed,
  declareExperimentalWebMcpTool,
  inject,
  signal,
} from '@angular/core';
import { AppView } from './app-view';
import { CTRL_NAV, type Win } from '../desktop.data';
import type { AppNavItem } from '../desktop-route-tree';
import {
  beginDesktopDataRefresh,
  createConnectedResource,
  createDemoResource,
  rejectDesktopData,
  resolveDesktopData,
  type DesktopDataResource,
} from '../desktop-data-resource';
import type { DesktopDataState } from '../desktop-data-state';
import { DesktopLiveDataService } from '../desktop-live-data.service';
import { DesktopModelRuntimeResource } from '../desktop-model-runtime-resource.service';
import { DesktopServicesBridgeService } from '../desktop-services-bridge.service';
import { DesktopSystemMonitorResource } from '../desktop-system-monitor-resource.service';
import { WindowManagerService } from '../window-manager.service';
import { ControlModelsView } from './control/control-models.view';
import { ControlPowerView } from './control/control-power.view';
import { ControlRunsView } from './control/control-runs.view';
import {
  createDemoServiceCatalogue,
  SERVICES_DEMO_SOURCE,
  type ControlServiceIntent,
  type DesktopServiceCatalogue,
  type DesktopServiceOutput,
  type DesktopServiceSnapshot,
} from './control/control-services.models';
import { ControlSettingsView } from './control/control-settings.view';
import { ControlSystemView } from './control/control-system.view';
import {
  createDemoControlViewState,
  mergeControlLiveSnapshot,
  mergeControlModelRuntime,
  mergeControlSystemMonitor,
} from './control/control-view-state';
import type {
  ControlActionIntent,
  ControlModelIntent,
  ControlSystemTab,
  ControlViewState,
} from './control/control-view.models';

const SERVICES_LIVE_SOURCE = 'Local go-process runtime';
const SERVICES_UNAVAILABLE = 'Live Services data is unavailable.';
const SERVICES_STALE_AFTER_MS = 30_000;

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
            [dataState]="modelDataState()"
            [model]="viewState().models"
            [pending]="modelRuntimePending()"
            (modelAction)="handleModelAction($event)"
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
            [dataState]="systemDataState()"
            [model]="systemView()"
            [activeTab]="systemTab()"
            [systemResource]="systemMonitorResource()"
            [services]="servicesResource()"
            [pendingServiceIds]="pendingServiceIds()"
            [serviceOutput]="serviceOutput()"
            (tabChange)="wm.setSysTab(win.id, $event)"
            (serviceAction)="handleServiceAction($event)"
            (servicesRetry)="retryServices()"
            (systemRetry)="systemMonitor.refresh()"
          />
        }
        @case ('settings') {
          <lthn-control-settings-view />
        }
      }
    </div>
  `,
})
export class ControlApp implements AppView, OnInit, OnDestroy {
  @Input() win!: Win;
  @Input() nav: AppNavItem[] = [];

  readonly wm = inject(WindowManagerService);
  private readonly liveData = inject(DesktopLiveDataService);
  private readonly modelRuntime = inject(DesktopModelRuntimeResource);
  private readonly servicesBridge = inject(DesktopServicesBridgeService);
  readonly systemMonitor = inject(DesktopSystemMonitorResource);
  private readonly pendingTasks = inject(PendingTasks);
  private readonly mcpTools = this.registerMcpTools();
  private readonly baseViewState = signal<ControlViewState>({
    ...createDemoControlViewState(),
    dataState: this.liveData.mode() === 'demo' ? 'demo' : 'loading',
  });
  readonly modelRuntimeResource = this.modelRuntime.resource;
  readonly modelRuntimePending = this.modelRuntime.pending;
  readonly systemMonitorResource = this.systemMonitor.resource;
  readonly viewState = computed(() => {
    const resource = this.modelRuntimeResource();
    return mergeControlModelRuntime(
      this.baseViewState(),
      resource.value,
      resource.mode === 'connected',
    );
  });
  readonly modelDataState = computed<DesktopDataState>(() => {
    switch (this.modelRuntimeResource().state) {
      case 'demo':
        return 'demo';
      case 'loading':
        return 'loading';
      case 'live':
        return 'live';
      case 'mixed':
      case 'stale':
        return 'mixed';
      case 'unavailable':
        return 'unavailable';
    }
  });
  readonly systemView = computed(() =>
    mergeControlSystemMonitor(this.viewState().system, this.systemMonitorResource()),
  );
  readonly servicesResource = signal<DesktopDataResource<DesktopServiceCatalogue>>(
    createDemoResource(createDemoServiceCatalogue(), SERVICES_DEMO_SOURCE),
  );
  readonly pendingServiceIds = signal<readonly string[]>([]);
  readonly serviceOutput = signal<DesktopServiceOutput | null>(null);

  private servicesEventsOff: (() => void) | null = null;
  private servicesRefresh: Promise<void> | null = null;
  private servicesRefreshQueued = false;
  private modelRuntimeDisconnect: (() => void) | null = null;
  private systemMonitorDisconnect: (() => void) | null = null;
  private destroyed = false;

  ngOnInit(): void {
    this.modelRuntimeDisconnect = this.modelRuntime.connect();
    this.systemMonitorDisconnect = this.systemMonitor.connect();
    if (this.liveData.mode() === 'demo') return;
    void this.refresh();
    this.servicesResource.set(
      createConnectedResource<DesktopServiceCatalogue>(SERVICES_LIVE_SOURCE),
    );
    this.servicesEventsOff = this.servicesBridge.onChanged(() => {
      this.pendingTasks.run(() => this.refreshServices());
    });
    this.pendingTasks.run(() => this.refreshServices());
  }

  ngOnDestroy(): void {
    this.destroyed = true;
    this.servicesEventsOff?.();
    this.servicesEventsOff = null;
    this.modelRuntimeDisconnect?.();
    this.modelRuntimeDisconnect = null;
    this.systemMonitorDisconnect?.();
    this.systemMonitorDisconnect = null;
  }

  async refresh(): Promise<void> {
    if (this.liveData.mode() === 'demo' || this.destroyed) return;
    this.baseViewState.update((state) => ({ ...state, dataState: 'loading' }));
    try {
      const snapshot = await this.liveData.control();
      if (this.destroyed) return;
      this.baseViewState.set(mergeControlLiveSnapshot(snapshot));
    } catch {
      if (this.destroyed) return;
      this.baseViewState.set({
        ...createDemoControlViewState(),
        dataState: 'unavailable',
      });
    }
  }

  systemTab(): ControlSystemTab {
    const tab = this.win.systab;
    return tab === 'processes' || tab === 'daemons' ? tab : 'overview';
  }

  systemDataState(): DesktopDataState {
    return this.systemTab() === 'overview'
      ? this.systemMonitorResource().state
      : this.viewState().dataState;
  }

  handleAction(_intent: ControlActionIntent): void {
    // The existing placeholder buttons remain inert until their typed backend
    // actions in TODO.md are implemented.
  }

  handleModelAction(intent: ControlModelIntent): void {
    if (this.destroyed || this.modelRuntimePending() !== null) return;
    this.pendingTasks.run(() => this.modelRuntime.perform(intent));
  }

  handleServiceAction(intent: ControlServiceIntent): void {
    if (
      this.destroyed ||
      this.pendingServiceIds().includes(intent.id) ||
      !this.hasService(intent.id)
    ) {
      return;
    }
    if (this.liveData.mode() === 'demo') {
      this.applyDemoServiceAction(intent);
      return;
    }
    this.pendingTasks.run(() => this.performServiceAction(intent));
  }

  retryServices(): void {
    if (this.liveData.mode() === 'demo' || this.destroyed) return;
    this.pendingTasks.run(() => this.refreshServices());
  }

  refreshServices(): Promise<void> {
    if (this.liveData.mode() === 'demo' || this.destroyed) {
      return Promise.resolve();
    }
    if (this.servicesRefresh) {
      this.servicesRefreshQueued = true;
      return this.servicesRefresh;
    }

    let refreshTask: Promise<void>;
    refreshTask = this.performServicesRefresh().finally(async () => {
      if (this.servicesRefresh === refreshTask) {
        this.servicesRefresh = null;
      }
      if (this.servicesRefreshQueued && !this.destroyed) {
        this.servicesRefreshQueued = false;
        await this.refreshServices();
      }
    });
    this.servicesRefresh = refreshTask;
    return refreshTask;
  }

  private async performServicesRefresh(): Promise<void> {
    const current = this.servicesResource();
    if (current.mode !== 'connected' || current.refreshing) return;
    this.servicesResource.set(
      beginDesktopDataRefresh(current, Date.now(), SERVICES_STALE_AFTER_MS),
    );

    try {
      const catalogue = await this.servicesBridge.catalogue();
      if (this.destroyed) return;
      this.servicesResource.update((resource) =>
        resolveDesktopData(resource, catalogue, 'live', SERVICES_LIVE_SOURCE, Date.now()),
      );
    } catch {
      if (this.destroyed) return;
      this.servicesResource.update((resource) => rejectDesktopData(resource, SERVICES_UNAVAILABLE));
    }
  }

  private async performServiceAction(intent: ControlServiceIntent): Promise<void> {
    this.setServicePending(intent.id, true);
    try {
      switch (intent.kind) {
        case 'start':
          await this.servicesBridge.start(intent.id);
          break;
        case 'stop':
          await this.servicesBridge.stop(intent.id);
          break;
        case 'restart':
          await this.servicesBridge.restart(intent.id);
          break;
        case 'signal':
          await this.servicesBridge.signal(intent.id, intent.signal);
          break;
        case 'kill':
          await this.servicesBridge.kill(intent.id);
          break;
        case 'output': {
          const output = await this.servicesBridge.output(intent.id);
          if (!this.destroyed) this.serviceOutput.set(output);
          return;
        }
      }
      if (this.destroyed) return;
      if (this.serviceOutput()?.id === intent.id) this.serviceOutput.set(null);
      await this.refreshServices();
    } catch {
      if (!this.destroyed) this.markServicesUnavailable();
    } finally {
      if (!this.destroyed) this.setServicePending(intent.id, false);
    }
  }

  private applyDemoServiceAction(intent: ControlServiceIntent): void {
    const resource = this.servicesResource();
    const catalogue = resource.value;
    if (resource.mode !== 'demo' || catalogue === null) return;
    const serviceIndex = catalogue.services.findIndex(
      ({ definition }) => definition.id === intent.id,
    );
    if (serviceIndex < 0) return;
    const service = catalogue.services[serviceIndex];

    if (intent.kind === 'output') {
      if (!service.processId) return;
      this.serviceOutput.set({
        id: intent.id,
        processId: service.processId,
        generation: service.restartCount + 1,
        output: `Lethean demo fixture\n${service.definition.displayName} is ready.\n`,
        truncated: false,
        observedAt: 'demo',
      });
      return;
    }

    // Demo mode has no process, so a signal has nothing to reach and must not
    // pretend otherwise. A kill is shown as a stop, which is what it is.
    if (intent.kind === 'signal') return;
    const demoKind = intent.kind === 'kill' ? 'stop' : intent.kind;

    const services = catalogue.services.map((snapshot, index) =>
      index === serviceIndex
        ? demoLifecycleSnapshot(snapshot, demoKind, index)
        : cloneServiceSnapshot(snapshot),
    );
    this.servicesResource.set(
      createDemoResource(
        {
          refreshedAt: 'demo',
          services,
        },
        SERVICES_DEMO_SOURCE,
      ),
    );
    if (this.serviceOutput()?.id === intent.id) this.serviceOutput.set(null);
  }

  private markServicesUnavailable(): void {
    const resource = this.servicesResource();
    if (resource.mode !== 'connected' || resource.refreshing) return;
    const refreshing = beginDesktopDataRefresh(resource, Date.now(), SERVICES_STALE_AFTER_MS);
    this.servicesResource.set(rejectDesktopData(refreshing, SERVICES_UNAVAILABLE));
  }

  private hasService(id: string): boolean {
    return (
      this.servicesResource().value?.services.some(({ definition }) => definition.id === id) ??
      false
    );
  }

  private setServicePending(id: string, pending: boolean): void {
    this.pendingServiceIds.update((ids) =>
      pending
        ? ids.includes(id)
          ? ids
          : [...ids, id]
        : ids.filter((candidate) => candidate !== id),
    );
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

function cloneServiceSnapshot(snapshot: DesktopServiceSnapshot): DesktopServiceSnapshot {
  return {
    ...snapshot,
    definition: { ...snapshot.definition },
    lastError: snapshot.lastError ? { ...snapshot.lastError } : null,
  };
}

function demoLifecycleSnapshot(
  snapshot: DesktopServiceSnapshot,
  action: 'start' | 'stop' | 'restart',
  index: number,
): DesktopServiceSnapshot {
  const clone = cloneServiceSnapshot(snapshot);
  if (action === 'stop') {
    return {
      ...clone,
      state: 'stopped',
      desired: false,
      processId: '',
      pid: 0,
      stoppedAt: 'demo',
      exitCode: 0,
      lastError: null,
    };
  }
  const restartCount = action === 'restart' ? clone.restartCount + 1 : clone.restartCount;
  return {
    ...clone,
    state: 'running',
    desired: true,
    processId: `demo-${clone.definition.id}-${restartCount + 1}`,
    pid: 5_000 + index,
    startedAt: 'demo',
    stoppedAt: '',
    exitCode: 0,
    restartCount,
    lastError: null,
  };
}

import { isPlatformBrowser } from '@angular/common';
import {
  ChangeDetectionStrategy,
  Component,
  DestroyRef,
  InjectionToken,
  PLATFORM_ID,
  afterNextRender,
  computed,
  inject,
  signal,
} from '@angular/core';

const WINDOW_BINDING = 'dappco.re/go/render/display/webkit.WindowBindingService';
const WINDOW_OPEN_METHOD = `${WINDOW_BINDING}.Open`;
const WINDOW_HIDE_METHOD = `${WINDOW_BINDING}.Hide`;
const LEMMA_STATUS_METHOD = 'dappco.re/lthn/desktop/pkg/lemma.WailsService.Status';
const TELEMETRY_METHOD =
  'dappco.re/lthn/desktop/pkg/telemetry.Service.CurrentSample';
const TRAY_OPEN_EVENT = 'lthn:tray:open';
const TRAY_PANEL_WINDOW = 'tray-panel';
const REFRESH_INTERVAL_MS = 30_000;

export interface TrayPanelRuntime {
  call(method: string, args: readonly unknown[]): Promise<unknown>;
  emit(name: string, payload: unknown): Promise<boolean>;
}

export const TRAY_PANEL_RUNTIME = new InjectionToken<TrayPanelRuntime>('TRAY_PANEL_RUNTIME', {
  providedIn: 'root',
  factory: () => ({
    async call(method, args): Promise<unknown> {
      const { Call } = await import('@wailsio/runtime');
      return await Call.ByName(method, ...args);
    },
    async emit(name, payload): Promise<boolean> {
      const { Events } = await import('@wailsio/runtime');
      return Events.Emit(name, payload);
    },
  }),
});

type TrayPanelTab = 'system' | 'runner' | 'activity';
type TrayPanelTarget = 'desktop' | 'chat' | 'models' | 'settings' | 'telemetry';

interface ModelStatus {
  readonly modelPath: string;
  readonly runtime: string;
  readonly loadedAtUnix: number;
  readonly contextLength: number;
  readonly parallelSlots: number;
}

interface ProcessTelemetry {
  readonly heapAllocMB: number;
  readonly uptimeSeconds: number;
  readonly numGoroutines: number;
  readonly lastGCPauseMs: number;
}

interface QuickAction {
  readonly target: TrayPanelTarget;
  readonly icon: string;
  readonly label: string;
  readonly detail: string;
}

const QUICK_ACTIONS: readonly QuickAction[] = [
  { target: 'chat', icon: 'fa-comments', label: 'Chat', detail: 'Talk to a model' },
  { target: 'models', icon: 'fa-cube', label: 'Models', detail: 'Browse and load' },
  { target: 'telemetry', icon: 'fa-wave-square', label: 'Telemetry', detail: 'Live process view' },
];

@Component({
  selector: 'lthn-tray-panel',
  templateUrl: './tray-panel.html',
  styleUrl: './tray-panel.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
  host: {
    'data-brand': 'lethean',
    'data-platform': 'darwin',
  },
})
export class TrayPanel {
  private readonly runtime = inject(TRAY_PANEL_RUNTIME);
  private readonly platformId = inject(PLATFORM_ID);
  private readonly destroyRef = inject(DestroyRef);
  private pollHandle: number | undefined;
  private refreshPending = false;

  readonly tabs: readonly TrayPanelTab[] = ['system', 'runner', 'activity'];
  readonly quickActions = QUICK_ACTIONS;
  readonly activeTab = signal<TrayPanelTab>('system');
  readonly model = signal<ModelStatus | null>(null);
  readonly telemetry = signal<ProcessTelemetry | null>(null);
  readonly loading = signal(true);
  readonly launching = signal<TrayPanelTarget | null>(null);
  readonly statusMessage = signal('');
  readonly refreshedAt = signal<Date | null>(null);

  readonly modelName = computed(() => basename(this.model()?.modelPath ?? ''));
  readonly heapLabel = computed(() => formatMegabytes(this.telemetry()?.heapAllocMB));
  readonly uptimeLabel = computed(() => formatUptime(this.telemetry()?.uptimeSeconds));
  readonly loadedLabel = computed(() => formatLoadedAt(this.model()?.loadedAtUnix));

  constructor() {
    if (!isPlatformBrowser(this.platformId)) return;

    void this.refresh();
    afterNextRender(() => {
      this.pollHandle = window.setInterval(() => void this.refresh(), REFRESH_INTERVAL_MS);
    });
    this.destroyRef.onDestroy(() => {
      if (this.pollHandle !== undefined) window.clearInterval(this.pollHandle);
    });
  }

  selectTab(tab: TrayPanelTab): void {
    this.activeTab.set(tab);
  }

  async refresh(): Promise<void> {
    if (this.refreshPending) return;
    this.refreshPending = true;
    this.loading.set(true);

    try {
      const [modelResult, telemetryResult] = await Promise.allSettled([
        this.runtime.call(LEMMA_STATUS_METHOD, []),
        this.runtime.call(TELEMETRY_METHOD, []),
      ]);

      let nextModel: ModelStatus | null = null;
      let runnerUnavailable = modelResult.status === 'rejected';
      if (modelResult.status === 'fulfilled') {
        try {
          nextModel = modelStatusFrom(modelResult.value);
        } catch {
          runnerUnavailable = true;
        }
      }

      let nextTelemetry: ProcessTelemetry | null = null;
      if (telemetryResult.status === 'fulfilled') {
        try {
          nextTelemetry = processTelemetryFrom(unwrapResult(telemetryResult.value));
        } catch {
          // Telemetry is supplementary; a failed sample must not stall the panel.
        }
      }

      this.model.set(nextModel);
      this.telemetry.set(nextTelemetry);
      this.statusMessage.set(
        runnerUnavailable
          ? 'The local runner is offline. Desktop tools remain available.'
          : '',
      );
      this.refreshedAt.set(new Date());
    } finally {
      this.loading.set(false);
      this.refreshPending = false;
    }
  }

  async open(target: TrayPanelTarget): Promise<void> {
    if (this.launching()) return;
    this.launching.set(target);
    this.statusMessage.set('');

    try {
      unwrapResult(await this.runtime.call(WINDOW_OPEN_METHOD, ['app']));
      await this.runtime.emit(TRAY_OPEN_EVENT, target);
      unwrapResult(await this.runtime.call(WINDOW_HIDE_METHOD, [TRAY_PANEL_WINDOW]));
    } catch (error) {
      this.statusMessage.set(errorMessage(error));
    } finally {
      this.launching.set(null);
    }
  }
}

function unwrapResult(value: unknown): unknown {
  if (!isRecord(value)) return value;
  const ok = value['OK'] ?? value['ok'];
  if (typeof ok !== 'boolean') return value;
  if (!ok) throw new Error(errorMessage(value['Value'] ?? value['value']));
  return Object.hasOwn(value, 'Value') ? value['Value'] : value['value'];
}

function modelStatusFrom(value: unknown): ModelStatus | null {
  const raw = unwrapResult(value);
  if (!isRecord(raw)) return null;
  const modelPath = stringField(raw, 'model_path', 'ModelPath');
  if (!modelPath) return null;
  const config = isRecord(raw['config']) ? raw['config'] : {};
  return {
    modelPath,
    runtime: stringField(raw, 'runtime', 'Runtime') || 'local',
    loadedAtUnix: numberField(raw, 'loaded_at_unix', 'LoadedAtUnix'),
    contextLength: numberField(config, 'context_length', 'ContextLength'),
    parallelSlots: numberField(config, 'parallel_slots', 'ParallelSlots'),
  };
}

function processTelemetryFrom(value: unknown): ProcessTelemetry | null {
  if (!isRecord(value)) return null;
  return {
    heapAllocMB: numberField(value, 'heap_alloc_mb', 'HeapAllocMB'),
    uptimeSeconds: numberField(value, 'uptime_seconds', 'UptimeSeconds'),
    numGoroutines: numberField(value, 'num_goroutines', 'NumGoroutines'),
    lastGCPauseMs: numberField(value, 'last_gc_pause_ms', 'LastGCPauseMs'),
  };
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return value !== null && typeof value === 'object' && !Array.isArray(value);
}

function stringField(value: Record<string, unknown>, ...keys: string[]): string {
  for (const key of keys) {
    if (typeof value[key] === 'string') return value[key];
  }
  return '';
}

function numberField(value: Record<string, unknown>, ...keys: string[]): number {
  for (const key of keys) {
    if (typeof value[key] === 'number' && Number.isFinite(value[key])) return value[key];
  }
  return 0;
}

function basename(path: string): string {
  return path.split(/[\\/]/).filter(Boolean).at(-1) ?? '';
}

function formatMegabytes(value: number | undefined): string {
  return `${(value ?? 0).toFixed(1)} MB`;
}

function formatUptime(value: number | undefined): string {
  const seconds = Math.max(0, Math.floor(value ?? 0));
  const hours = Math.floor(seconds / 3_600);
  const minutes = Math.floor((seconds % 3_600) / 60);
  if (hours > 0) return `${hours}h ${minutes}m`;
  return `${minutes}m`;
}

function formatLoadedAt(value: number | undefined): string {
  if (!value) return 'Not loaded';
  return new Intl.DateTimeFormat('en-GB', {
    hour: '2-digit',
    minute: '2-digit',
  }).format(new Date(value * 1_000));
}

function errorMessage(error: unknown): string {
  if (error instanceof Error && error.message) return error.message;
  if (typeof error === 'string' && error) return error;
  if (isRecord(error)) {
    for (const key of ['message', 'Message', 'error', 'Error']) {
      if (typeof error[key] === 'string' && error[key]) return error[key];
    }
  }
  return 'The desktop action could not be completed.';
}

import { signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { createDemoResource, type DesktopDataResource } from '../desktop-data-resource';
import { DesktopLiveDataService } from '../desktop-live-data.service';
import { createDemoModelRuntimeSnapshot } from '../desktop-model-runtime-demo.data';
import type { ModelRuntimeOperation, ModelRuntimeSnapshot } from '../desktop-model-runtime.models';
import { DesktopModelRuntimeResource } from '../desktop-model-runtime-resource.service';
import { DesktopServicesBridgeService } from '../desktop-services-bridge.service';
import type { Win } from '../desktop.data';
import { WindowManagerService } from '../window-manager.service';
import { APP_REGISTRY } from './app-view';
import { ControlApp } from './control.app';
import type {
  DesktopServiceSnapshot,
  DesktopServicesChangedEvent,
} from './control/control-services.models';

const controlWin: Win = {
  id: 'control-window',
  app: 'control',
  sub: 'models',
  systab: 'overview',
  x: 0,
  y: 0,
  w: 900,
  h: 620,
  z: 1,
  min: false,
  max: false,
};

describe('ControlApp', () => {
  const mode = signal<'demo' | 'live'>('demo');
  const liveData = {
    mode: mode.asReadonly(),
    control: vi.fn(),
  };
  const windowManager = {
    setSub: vi.fn(),
    setSysTab: vi.fn(),
  };
  const modelRuntimeState = signal<DesktopDataResource<ModelRuntimeSnapshot>>(
    createDemoResource(createDemoModelRuntimeSnapshot(), 'Lethean demo fixture · Model runtime'),
  );
  const modelRuntimePending = signal<ModelRuntimeOperation | null>(null);
  let modelRuntimeDisconnect = vi.fn();
  const modelRuntime = {
    resource: modelRuntimeState.asReadonly(),
    pending: modelRuntimePending.asReadonly(),
    connect: vi.fn(),
    perform: vi.fn<(operation: ModelRuntimeOperation) => Promise<void>>(),
  };
  let servicesChanged: ((event: DesktopServicesChangedEvent) => void) | null = null;
  let servicesOff = vi.fn();
  const servicesBridge = {
    catalogue: vi.fn(),
    get: vi.fn(),
    start: vi.fn(),
    stop: vi.fn(),
    restart: vi.fn(),
    output: vi.fn(),
    setPolicy: vi.fn(),
    onChanged: vi.fn(),
  };

  beforeEach(() => {
    mode.set('demo');
    servicesChanged = null;
    servicesOff = vi.fn();
    modelRuntimeDisconnect = vi.fn();
    vi.clearAllMocks();
    modelRuntimeState.set(
      createDemoResource(createDemoModelRuntimeSnapshot(), 'Lethean demo fixture · Model runtime'),
    );
    modelRuntimePending.set(null);
    modelRuntime.connect.mockReturnValue(modelRuntimeDisconnect);
    modelRuntime.perform.mockResolvedValue();
    servicesBridge.catalogue.mockResolvedValue(serviceCatalogue('stopped'));
    servicesBridge.onChanged.mockImplementation(
      (handler: (event: DesktopServicesChangedEvent) => void) => {
        servicesChanged = handler;
        return servicesOff;
      },
    );
    TestBed.configureTestingModule({
      providers: [
        { provide: DesktopLiveDataService, useValue: liveData },
        { provide: DesktopModelRuntimeResource, useValue: modelRuntime },
        { provide: DesktopServicesBridgeService, useValue: servicesBridge },
        { provide: WindowManagerService, useValue: windowManager },
      ],
    });
  });

  afterEach(() => TestBed.resetTestingModule());

  async function create(win: Win = controlWin) {
    const fixture = TestBed.createComponent(ControlApp);
    fixture.componentRef.setInput('win', { ...win });
    fixture.detectChanges();
    await fixture.whenStable();
    return fixture;
  }

  function setLiveModelRuntime(snapshot: ModelRuntimeSnapshot): void {
    modelRuntimeState.set({
      mode: 'connected',
      state: 'live',
      source: 'Local LEM runtime',
      updatedAt: Date.parse(snapshot.refreshedAt),
      refreshing: false,
      error: null,
      canRetry: false,
      value: snapshot,
    });
  }

  it('keeps the complete Control prototype available without live calls in demo mode', async () => {
    const fixture = await create();
    const text = (fixture.nativeElement as HTMLElement).textContent ?? '';

    expect(text).toContain('Demo data');
    expect(text).toContain('Local models');
    expect(text).toContain('Start');
    expect(
      (fixture.nativeElement as HTMLElement).querySelector('lthn-stat[value="—"]'),
    ).not.toBeNull();
    expect(liveData.control).not.toHaveBeenCalled();
    expect(modelRuntime.connect).toHaveBeenCalledOnce();
  });

  it('delegates each rail section to its standalone view', async () => {
    const expectations = [
      ['models', 'lthn-control-models-view'],
      ['runs', 'lthn-control-runs-view'],
      ['power', 'lthn-control-power-view'],
      ['system', 'lthn-control-system-view'],
      ['settings', 'lthn-control-settings-view'],
    ] as const;

    for (const [sub, selector] of expectations) {
      const fixture = await create({ ...controlWin, sub });
      expect((fixture.nativeElement as HTMLElement).querySelector(selector)).not.toBeNull();
      fixture.destroy();
    }
  });

  it('uses isolated demo service actions without Wails calls', async () => {
    const fixture = await create({ ...controlWin, sub: 'system', systab: 'daemons' });
    const element = fixture.nativeElement as HTMLElement;
    const runner = element.querySelector('[data-service-id="runner"]');

    expect(runner).not.toBeNull();
    runner?.querySelector<HTMLButtonElement>('[data-action="start"]')?.click();
    await fixture.whenStable();

    expect(servicesBridge.catalogue).not.toHaveBeenCalled();
    expect(servicesBridge.onChanged).not.toHaveBeenCalled();
    expect(servicesBridge.start).not.toHaveBeenCalled();
    expect(element.querySelector('[data-service-id="runner"]')?.textContent).toContain('Running');
  });

  it('loads live services, performs Start, then refreshes the canonical catalogue', async () => {
    mode.set('live');
    liveData.control.mockResolvedValue({
      unavailable: ['telemetry', 'benchmarkRuns', 'processes', 'settings'],
    });
    servicesBridge.catalogue
      .mockResolvedValueOnce(serviceCatalogue('stopped'))
      .mockResolvedValueOnce(serviceCatalogue('running'));
    servicesBridge.start.mockResolvedValue(serviceSnapshot('running'));
    const fixture = await create({ ...controlWin, sub: 'system', systab: 'daemons' });

    expect(servicesBridge.onChanged).toHaveBeenCalledOnce();
    (fixture.nativeElement as HTMLElement)
      .querySelector<HTMLButtonElement>('[data-service-id="serve"] [data-action="start"]')
      ?.click();
    await fixture.whenStable();

    expect(servicesBridge.start).toHaveBeenCalledWith('serve');
    expect(servicesBridge.catalogue).toHaveBeenCalledTimes(2);
    expect(
      (fixture.nativeElement as HTMLElement).querySelector('[data-service-id="serve"]')
        ?.textContent,
    ).toContain('Running');
  });

  it('retains stale services after a failed event refresh and tears down events', async () => {
    mode.set('live');
    liveData.control.mockResolvedValue({
      unavailable: ['telemetry', 'benchmarkRuns', 'processes', 'settings'],
    });
    servicesBridge.catalogue.mockResolvedValueOnce(serviceCatalogue('running'));
    const fixture = await create({ ...controlWin, sub: 'system', systab: 'daemons' });
    servicesBridge.catalogue.mockRejectedValueOnce(new Error('transport offline'));

    servicesChanged?.({
      id: 'serve',
      operation: 'start',
      previous: 'stopped',
      state: 'running',
      desired: true,
      processId: 'proc-1',
      errorCode: '',
      at: '2026-07-27T12:00:00Z',
    });
    await fixture.whenStable();

    expect((fixture.nativeElement as HTMLElement).textContent).toContain('Live data stale');
    expect(
      (fixture.nativeElement as HTMLElement).querySelector('[data-service-id="serve"]'),
    ).not.toBeNull();
    fixture.destroy();
    expect(servicesOff).toHaveBeenCalledOnce();
  });

  it('loads bounded output only after the explicit Output action', async () => {
    mode.set('live');
    liveData.control.mockResolvedValue({
      unavailable: ['telemetry', 'benchmarkRuns', 'processes', 'settings'],
    });
    servicesBridge.catalogue.mockResolvedValueOnce(serviceCatalogue('running'));
    servicesBridge.output.mockResolvedValue({
      id: 'serve',
      processId: 'proc-1',
      generation: 1,
      output: 'ready\n',
      truncated: false,
      observedAt: '2026-07-27T12:00:00Z',
    });
    const fixture = await create({ ...controlWin, sub: 'system', systab: 'daemons' });

    expect(servicesBridge.output).not.toHaveBeenCalled();
    (fixture.nativeElement as HTMLElement)
      .querySelector<HTMLButtonElement>('[data-service-id="serve"] [data-action="output"]')
      ?.click();
    await fixture.whenStable();

    expect(servicesBridge.output).toHaveBeenCalledWith('serve');
    expect((fixture.nativeElement as HTMLElement).querySelector('pre')?.textContent).toContain(
      'ready',
    );
  });

  it('remains the lazy component registered for Control', async () => {
    await expect(APP_REGISTRY['control']()).resolves.toBe(ControlApp);
  });

  it('replaces model fixtures with the shared path-free runtime snapshot', async () => {
    mode.set('live');
    const runtimeSnapshot: ModelRuntimeSnapshot = {
      ...createDemoModelRuntimeSnapshot('ready'),
      metrics: {
        promptTokensPerSecond: 48.25,
        activeMemoryBytes: 2_147_483_648,
        uptimeSeconds: 600,
      },
      history: [
        {
          state: 'ready',
          at: '2026-07-27T13:00:00Z',
          promptTokensPerSecond: 48.25,
        },
      ],
    };
    setLiveModelRuntime(runtimeSnapshot);
    liveData.control.mockResolvedValue({
      telemetry: {
        heapAllocMB: 64,
        heapSysMB: 96,
        stackInUseMB: 2,
        numGoroutines: 12,
        numCgoCalls: 3,
        uptimeSeconds: 600,
        numGC: 4,
        lastGCPauseMs: 0.2,
        wattsActive: 0,
        wattsIdle: 0,
      },
      benchmarkRuns: [
        {
          id: '01J2BENCH',
          timestamp: '2026-07-26T07:41:00Z',
          bencher: 'lthn-mlx',
          model: 'gemma-4-e2b',
          contextLength: 8_192,
          promptTokensPerSecond: 2_040.5,
          generatedTokensPerSecond: 48.25,
          promptLength: 1_900,
          outputLength: 256,
          peakWatts: 12.5,
          peakMemoryMB: 2_400,
          endpoint: '',
        },
      ],
      processes: [],
      settings: { configPath: '/tmp/lthn.yaml', controls: [] },
      unavailable: [],
    });

    const fixture = await create();
    await vi.waitFor(() => {
      fixture.detectChanges();
      expect((fixture.nativeElement as HTMLElement).textContent).toContain('Live data');
    });
    const element = fixture.nativeElement as HTMLElement;
    const rows = JSON.parse(
      element.querySelector('lthn-datatable')?.getAttribute('rows') ?? '[]',
    ) as Array<Record<string, unknown>>;
    const stats = [...element.querySelectorAll('lthn-stat')];

    expect(rows).toEqual([
      {
        id: 'model-0000000000000001',
        name: 'gemma-4-e2b',
        format: 'snapshot',
        runtime: 'metal',
        status: 'loaded',
      },
      {
        id: 'model-0000000000000002',
        name: 'qwen-2.5-coder',
        format: 'snapshot',
        runtime: '—',
        status: 'available',
      },
      {
        id: 'model-0000000000000003',
        name: 'mistral-small',
        format: 'snapshot',
        runtime: '—',
        status: 'unavailable',
      },
    ]);
    expect(JSON.stringify(rows)).not.toMatch(/model_path|[/\\]Users|[/\\]tmp/iu);
    expect(stats.map((stat) => stat.getAttribute('value'))).toEqual(['48.3', '2 GB', '3', '10m']);
    expect(fixture.componentInstance.modelRuntimeResource().value).toBe(runtimeSnapshot);
  });

  it('routes the selected opaque model ID through the shared runtime resource', async () => {
    mode.set('live');
    setLiveModelRuntime(createDemoModelRuntimeSnapshot('model-less'));
    liveData.control.mockResolvedValue({
      unavailable: ['telemetry', 'benchmarkRuns', 'processes', 'settings'],
    });
    const fixture = await create();
    const element = fixture.nativeElement as HTMLElement;
    const select = element.querySelector<HTMLSelectElement>('[aria-label="Model to load"]');
    if (!select) throw new Error('Expected the model selector.');
    select.value = 'model-0000000000000001';
    select.dispatchEvent(new Event('change'));
    await fixture.whenStable();
    element.querySelector<HTMLButtonElement>('[data-action="load"]')?.click();
    await fixture.whenStable();

    expect(modelRuntime.perform).toHaveBeenCalledWith({
      kind: 'load',
      modelId: 'model-0000000000000001',
    });
  });

  it('disconnects its shared runtime consumer when the Control window closes', async () => {
    const fixture = await create();

    fixture.destroy();

    expect(modelRuntimeDisconnect).toHaveBeenCalledOnce();
  });

  it('maps benchmark history into the existing runs chart and table', async () => {
    mode.set('live');
    liveData.control.mockResolvedValue({
      benchmarkRuns: [
        {
          id: '01J2BENCH',
          timestamp: '2026-07-26T08:41:00+01:00',
          bencher: 'lthn-mlx',
          model: 'gemma-4-e2b',
          contextLength: 8_192,
          promptTokensPerSecond: 2_040.5,
          generatedTokensPerSecond: 48.25,
          promptLength: 1_900,
          outputLength: 256,
          peakWatts: 12.5,
          peakMemoryMB: 2_400,
          endpoint: '',
        },
      ],
      unavailable: ['telemetry', 'processes', 'settings'],
    });

    const fixture = await create({ ...controlWin, sub: 'runs' });
    await vi.waitFor(() => {
      fixture.detectChanges();
      expect((fixture.nativeElement as HTMLElement).textContent).toContain('Live + demo');
    });
    const element = fixture.nativeElement as HTMLElement;
    const rows = JSON.parse(
      element.querySelector('lthn-datatable')?.getAttribute('rows') ?? '[]',
    ) as Array<Record<string, unknown>>;

    expect(rows).toEqual([
      {
        run: '#01J2BENC',
        model: 'gemma-4-e2b',
        ctx: 8_192,
        toks: 48.3,
        when: '08:41',
      },
    ]);
    expect(element.querySelector('lthn-chart')?.getAttribute('data')).toBe('[48.25]');
  });

  it('shows the shared live process registry without fixture CPU or memory values', async () => {
    mode.set('live');
    liveData.control.mockResolvedValue({
      processes: [
        {
          id: 'build-01J2',
          command: 'npm run build',
          status: 'running',
          exitCode: 0,
        },
      ],
      unavailable: ['telemetry', 'benchmarkRuns', 'settings'],
    });

    const fixture = await create({ ...controlWin, sub: 'system', systab: 'processes' });
    await vi.waitFor(() => {
      fixture.detectChanges();
      expect((fixture.nativeElement as HTMLElement).textContent).toContain('Live + demo');
    });
    const table = (fixture.nativeElement as HTMLElement).querySelector('lthn-datatable');
    const columns = JSON.parse(table?.getAttribute('columns') ?? '[]') as Array<
      Record<string, unknown>
    >;
    const rows = JSON.parse(table?.getAttribute('rows') ?? '[]') as Array<Record<string, unknown>>;

    expect(columns.map(({ key }) => key)).toEqual(['command', 'id', 'state', 'exit']);
    expect(rows).toEqual([
      {
        command: 'npm run build',
        id: 'build-01J2',
        state: 'running',
        exit: 0,
      },
    ]);
  });

  it('groups the curated appconfig catalogue into live Control settings', async () => {
    mode.set('live');
    liveData.control.mockResolvedValue({
      settings: {
        configPath: '/tmp/Lethean/conf/lthn.yaml',
        controls: [
          {
            key: 'desktop.wails.window.main.width',
            group: 'Window',
            label: 'Window width',
            description: 'Width in pixels.',
            kind: 'number',
            value: 1_280,
            defaultValue: 1_440,
            configured: true,
            live: true,
            restartRequired: false,
            minimum: 800,
            maximum: 3_840,
            step: 10,
          },
          {
            key: 'desktop.show_widgets',
            group: 'Desktop',
            label: 'Show widgets',
            description: 'Show desktop widgets.',
            kind: 'toggle',
            value: true,
            defaultValue: true,
            configured: false,
            live: true,
            restartRequired: false,
          },
        ],
      },
      unavailable: ['telemetry', 'benchmarkRuns', 'processes'],
    });

    const fixture = await create({ ...controlWin, sub: 'settings' });
    await vi.waitFor(() => {
      fixture.detectChanges();
      expect((fixture.nativeElement as HTMLElement).textContent).toContain('Live + demo');
    });
    const element = fixture.nativeElement as HTMLElement;

    expect(element.textContent).toContain('Window');
    expect(element.textContent).toContain('desktop.wails.window.main.width');
    expect(
      element.querySelector<HTMLInputElement>('input[aria-label="desktop.wails.window.main.width"]')
        ?.value,
    ).toBe('1280');
    expect(element.textContent).toContain('desktop.show_widgets');
    expect(element.querySelector('lthn-toggle[on]')).not.toBeNull();
  });

  it('uses process telemetry for the live System overview cards', async () => {
    mode.set('live');
    liveData.control.mockResolvedValue({
      telemetry: {
        heapAllocMB: 64,
        heapSysMB: 96,
        stackInUseMB: 2,
        numGoroutines: 12,
        numCgoCalls: 3,
        uptimeSeconds: 600,
        numGC: 4,
        lastGCPauseMs: 0.2,
        wattsActive: 0,
        wattsIdle: 0,
      },
      benchmarkRuns: [
        {
          id: '01J2BENCH',
          timestamp: '2026-07-26T08:41:00+01:00',
          bencher: 'lthn-mlx',
          model: 'gemma-4-e2b',
          contextLength: 8_192,
          promptTokensPerSecond: 2_040.5,
          generatedTokensPerSecond: 48.25,
          promptLength: 1_900,
          outputLength: 256,
          peakWatts: 12.5,
          peakMemoryMB: 2_400,
          endpoint: '',
        },
      ],
      unavailable: ['benchmarkRuns', 'processes', 'settings'],
    });

    const fixture = await create({ ...controlWin, sub: 'system', systab: 'overview' });
    await vi.waitFor(() => {
      fixture.detectChanges();
      expect((fixture.nativeElement as HTMLElement).textContent).toContain('Live + demo');
    });
    const stats = [...(fixture.nativeElement as HTMLElement).querySelectorAll('lthn-stat')];

    expect(stats.map((stat) => stat.getAttribute('value'))).toEqual(['64 MB', '96 MB', '12', '4']);
    expect(stats.map((stat) => stat.getAttribute('label'))).toEqual([
      'Process heap',
      'Reserved heap',
      'Goroutines',
      'GC cycles',
    ]);
    expect((fixture.nativeElement as HTMLElement).textContent).toContain('CPU · demo history');
    expect(
      (fixture.nativeElement as HTMLElement).querySelector('lthn-chart')?.getAttribute('data'),
    ).not.toBe('[48.25]');
  });
});

function serviceSnapshot(state: 'stopped' | 'running'): DesktopServiceSnapshot {
  return {
    definition: {
      id: 'serve',
      displayName: 'Lethean Desktop API',
      description: 'OpenAI-compatible local Lethean API.',
      kind: 'service',
      restartPolicy: 'never',
      gracePeriodMillis: 5_000,
      owner: 'lethean',
    },
    state,
    desired: state === 'running',
    processId: state === 'running' ? 'proc-1' : '',
    pid: state === 'running' ? 4_821 : 0,
    startedAt: state === 'running' ? '2026-07-27T12:00:00Z' : '',
    stoppedAt: '',
    exitCode: 0,
    restartCount: 0,
    lastError: null,
  };
}

function serviceCatalogue(state: 'stopped' | 'running') {
  return {
    services: [serviceSnapshot(state)],
    refreshedAt: '2026-07-27T12:00:00Z',
  };
}

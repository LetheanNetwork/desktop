import { signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { DesktopLiveDataService } from '../desktop-live-data.service';
import type { Win } from '../desktop.data';
import { WindowManagerService } from '../window-manager.service';
import { ControlApp } from './control.app';

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

  beforeEach(() => {
    mode.set('demo');
    vi.clearAllMocks();
    TestBed.configureTestingModule({
      providers: [
        { provide: DesktopLiveDataService, useValue: liveData },
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

  it('keeps the complete Control prototype available without live calls in demo mode', async () => {
    const fixture = await create();
    const text = (fixture.nativeElement as HTMLElement).textContent ?? '';

    expect(text).toContain('Demo data');
    expect(text).toContain('Local models');
    expect(
      (fixture.nativeElement as HTMLElement).querySelector('lthn-stat[value="34.2"]'),
    ).not.toBeNull();
    expect(liveData.control).not.toHaveBeenCalled();
  });

  it('replaces model fixtures with the local catalogue and truthful live summary values', async () => {
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
      models: [
        {
          name: 'gemma-4-e2b-q4_k_m.gguf',
          path: '/tmp/models/gemma-4-e2b-q4_k_m.gguf',
          sizeBytes: 2_147_483_648,
          isDirectory: false,
        },
      ],
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
      expect((fixture.nativeElement as HTMLElement).textContent).toContain('Live + demo');
    });
    const element = fixture.nativeElement as HTMLElement;
    const rows = JSON.parse(
      element.querySelector('lthn-datatable')?.getAttribute('rows') ?? '[]',
    ) as Array<Record<string, unknown>>;
    const stats = [...element.querySelectorAll('lthn-stat')];

    expect(rows).toEqual([
      {
        name: 'gemma-4-e2b-q4_k_m.gguf',
        size: '2 GB',
        source: 'local file',
        status: 'available',
      },
    ]);
    expect(stats.map((stat) => stat.getAttribute('value'))).toEqual(['48.3', '2 GB', '1', '10m']);
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
      unavailable: ['telemetry', 'models', 'processes', 'settings'],
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
      unavailable: ['telemetry', 'models', 'benchmarkRuns', 'settings'],
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
      unavailable: ['telemetry', 'models', 'benchmarkRuns', 'processes'],
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
      unavailable: ['models', 'benchmarkRuns', 'processes', 'settings'],
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

import { signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { ConnectionManagerService } from '../connection-manager.service';
import { DesktopControlsBridgeService } from './desktop-controls-bridge.service';
import { DesktopLiveDataService } from './desktop-live-data.service';
import { SurfaceBridgeService } from './surfaces/surface-bridge.service';

describe('DesktopLiveDataService', () => {
  const offline = signal(false);
  const surface = {
    call: vi.fn(),
  };
  const controls = {
    settings: vi.fn(),
  };

  let service: DesktopLiveDataService;

  beforeEach(() => {
    offline.set(false);
    vi.clearAllMocks();
    TestBed.configureTestingModule({
      providers: [
        DesktopLiveDataService,
        {
          provide: ConnectionManagerService,
          useValue: { offline: offline.asReadonly() },
        },
        { provide: SurfaceBridgeService, useValue: surface },
        { provide: DesktopControlsBridgeService, useValue: controls },
      ],
    });
    service = TestBed.inject(DesktopLiveDataService);
  });

  afterEach(() => TestBed.resetTestingModule());

  it('blocks live reads when the explicit offline transport selects demo mode', async () => {
    offline.set(true);

    expect(service.mode()).toBe('demo');
    await expect(service.telemetry()).rejects.toThrow('Demo mode');
    expect(surface.call).not.toHaveBeenCalled();
  });

  it('normalises the complete process telemetry response', async () => {
    surface.call.mockResolvedValue({
      heap_alloc_mb: 128.25,
      heap_sys_mb: 192.5,
      stack_in_use_mb: 4.75,
      num_goroutines: 42,
      num_cgo_calls: 7,
      uptime_seconds: 9_061,
      num_gc: 18,
      last_gc_pause_ms: 0.43,
      watts_active: 0,
      watts_idle: 0,
    });

    await expect(service.telemetry()).resolves.toEqual({
      heapAllocMB: 128.25,
      heapSysMB: 192.5,
      stackInUseMB: 4.75,
      numGoroutines: 42,
      numCgoCalls: 7,
      uptimeSeconds: 9_061,
      numGC: 18,
      lastGCPauseMs: 0.43,
      wattsActive: 0,
      wattsIdle: 0,
    });
  });

  it('normalises local model catalogue entries without inventing runtime metrics', async () => {
    surface.call.mockResolvedValue([
      {
        name: 'gemma-4-e2b-q4_k_m.gguf',
        path: '/tmp/models/gemma-4-e2b-q4_k_m.gguf',
        size: 2_147_483_648,
        is_dir: false,
      },
    ]);

    await expect(service.models()).resolves.toEqual([
      {
        name: 'gemma-4-e2b-q4_k_m.gguf',
        path: '/tmp/models/gemma-4-e2b-q4_k_m.gguf',
        sizeBytes: 2_147_483_648,
        isDirectory: false,
      },
    ]);
  });

  it('normalises recent benchmark history for Control charts and rows', async () => {
    surface.call.mockResolvedValue([
      {
        id: '01J2BENCH',
        timestamp: '2026-07-26T07:41:00Z',
        bencher: 'lthn-mlx',
        model: 'gemma-4-e2b',
        ctx: 8_192,
        pp_tok_sec: 2_040.5,
        tg_tok_sec: 48.25,
        prompt_len: 1_900,
        output_len: 256,
        peak_watts: 12.5,
        peak_mem_mb: 2_400,
        endpoint: '',
        extra: {},
      },
    ]);

    await expect(service.benchmarkRuns(8)).resolves.toEqual([
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
    ]);
  });

  it('normalises the shared process registry without pretending it has CPU statistics', async () => {
    surface.call.mockResolvedValue([
      {
        id: 'build-01J2',
        command: 'npm run build',
        status: 'running',
        exit_code: 0,
      },
    ]);

    await expect(service.processes()).resolves.toEqual([
      {
        id: 'build-01J2',
        command: 'npm run build',
        status: 'running',
        exitCode: 0,
      },
    ]);
  });

  it('combines saved locations, recent files, and disk usage into one Files snapshot', async () => {
    surface.call.mockImplementation(async (method: string) => {
      if (method.endsWith('.ListLocations')) {
        return {
          locations: [{ name: 'Code', count: 12, size: '4.2 GB', brand: false }],
        };
      }
      if (method.endsWith('.ListRecent')) {
        return {
          recent: [
            {
              name: 'desktop.data.ts',
              path: '~/Code/lthn/desktop/',
              when: '08:31',
              size: '7 KB',
            },
          ],
          total: 1,
        };
      }
      if (method.endsWith('.GetDiskUsage')) {
        return { disk: { free: '312 GB', total: '1 TB', used: 68 } };
      }
      throw new Error(`Unexpected method: ${method}`);
    });

    await expect(service.files('Code')).resolves.toEqual({
      locations: [{ name: 'Code', count: 12, size: '4.2 GB', brand: false }],
      recent: [
        {
          name: 'desktop.data.ts',
          path: '~/Code/lthn/desktop/',
          when: '08:31',
          size: '7 KB',
        },
      ],
      totalRecent: 1,
      disk: { free: '312 GB', total: '1 TB', usedPercent: 68 },
    });
  });

  it('keeps successful Control sections when one backend service is unavailable', async () => {
    surface.call.mockImplementation(async (method: string) => {
      if (method.endsWith('.CurrentSample')) {
        return {
          heap_alloc_mb: 64,
          heap_sys_mb: 96,
          stack_in_use_mb: 2,
          num_goroutines: 12,
          num_cgo_calls: 3,
          uptime_seconds: 600,
          num_gc: 4,
          last_gc_pause_ms: 0.2,
          watts_active: 0,
          watts_idle: 0,
        };
      }
      if (method.endsWith('.List')) return [];
      if (method.endsWith('.History')) return [];
      if (method.endsWith('.ProcessList')) throw new Error('process registry offline');
      throw new Error(`Unexpected method: ${method}`);
    });
    controls.settings.mockResolvedValue({
      configPath: '/tmp/Lethean/conf/lthn.yaml',
      controls: [
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
    });

    const snapshot = await service.control();

    expect(snapshot.telemetry?.heapAllocMB).toBe(64);
    expect(snapshot.models).toEqual([]);
    expect(snapshot.benchmarkRuns).toEqual([]);
    expect(snapshot.settings?.controls[0].key).toBe('desktop.show_widgets');
    expect(snapshot.processes).toBeUndefined();
    expect(snapshot.unavailable).toEqual(['processes']);
  });
});

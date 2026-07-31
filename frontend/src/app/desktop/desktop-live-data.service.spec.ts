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
    await expect(service.control()).rejects.toThrow('Demo mode');
    expect(surface.call).not.toHaveBeenCalled();
  });

  it('does not expose a second process-telemetry bridge beside the shared host resource', () => {
    expect('telemetry' in service).toBe(false);
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

  it('does not retain the retired aggregate Files bridge', () => {
    expect('files' in service).toBe(false);
  });

  it('does not expose the retired absolute-path Models bridge', () => {
    expect('models' in service).toBe(false);
  });

  it('keeps successful Control sections without duplicating the NgRx settings read', async () => {
    surface.call.mockImplementation(async (method: string) => {
      if (method.endsWith('.History')) return [];
      if (method.endsWith('.ProcessList')) throw new Error('process registry offline');
      throw new Error(`Unexpected method: ${method}`);
    });
    const snapshot = await service.control();

    expect(snapshot.benchmarkRuns).toEqual([]);
    expect('settings' in snapshot).toBe(false);
    expect(controls.settings).not.toHaveBeenCalled();
    expect(snapshot.processes).toBeUndefined();
    expect(snapshot.unavailable).toEqual(['processes']);
    expect(surface.call).not.toHaveBeenCalledWith(
      expect.stringContaining('.CurrentSample'),
    );
  });
});

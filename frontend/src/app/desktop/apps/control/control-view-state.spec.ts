import { createDemoModelRuntimeSnapshot } from '../../desktop-model-runtime-demo.data';
import {
  beginDesktopDataRefresh,
  createConnectedResource,
  createDemoResource,
  resolveDesktopData,
} from '../../desktop-data-resource';
import { SYSTEM_MONITOR_DEMO_SNAPSHOT } from '../../desktop-system-monitor-demo.data';
import type { SystemMonitorSnapshot } from '../../desktop-system-monitor.models';
import {
  createDemoControlViewState,
  mergeControlLiveSnapshot,
  mergeControlModelRuntime,
  mergeControlSystemMonitor,
} from './control-view-state';

describe('Control view state', () => {
  it('preserves the complete labelled Control demo', () => {
    const state = createDemoControlViewState();

    expect(state.dataState).toBe('demo');
    expect(state.models.metrics.map(({ value }) => value)).toEqual([
      '34.2',
      '18.4 GB',
      '128',
      '6d 4h',
    ]);
    expect(state.models.rows).toHaveLength(6);
    expect(state.runs.rows).toHaveLength(4);
    expect(state.power.samples).toHaveLength(12);
    expect(state.system.processRows).toHaveLength(6);
    expect(state.system.daemonRows).toHaveLength(4);
    expect('settings' in state).toBe(false);
  });

  it('replaces only successful live sections', () => {
    const state = mergeControlLiveSnapshot({
      processes: [
        {
          id: 'build-1',
          command: 'npm run build',
          status: 'running',
          exitCode: 0,
        },
      ],
      unavailable: ['benchmarkRuns'],
    });

    expect(state.dataState).toBe('mixed');
    expect(state.models.rows).toHaveLength(6);
    expect(state.system.processColumns.map(({ key }) => key)).toEqual([
      'command',
      'id',
      'state',
      'exit',
    ]);
    expect(state.power.metrics[0].value).toBe('196 W');
    expect('settings' in state).toBe(false);
  });

  it('replaces model fixtures with a path-free shared runtime snapshot', () => {
    const runtime = {
      ...createDemoModelRuntimeSnapshot('ready'),
      metrics: {
        promptTokensPerSecond: 42.25,
        activeMemoryBytes: 2_147_483_648,
        uptimeSeconds: 600,
      },
      history: [
        {
          state: 'ready' as const,
          at: '2026-07-27T13:00:00Z',
          promptTokensPerSecond: 40,
        },
        {
          state: 'ready' as const,
          at: '2026-07-27T13:00:05Z',
          promptTokensPerSecond: 42.25,
        },
      ],
    };

    const state = mergeControlModelRuntime(createDemoControlViewState(), runtime, true);

    expect(state.models.state).toBe('ready');
    expect(state.models.activeModelId).toBe('model-0000000000000001');
    expect(state.models.availableModels).toBe(runtime.models);
    expect(state.models.metrics.map(({ value }) => value)).toEqual(['42.3', '2 GB', '3', '10m']);
    expect(state.models.chart.samples).toEqual([40, 42.25]);
    expect(state.models.rows[0]).toMatchObject({
      id: 'model-0000000000000001',
      name: 'gemma-4-e2b',
      format: 'snapshot',
      status: 'loaded',
    });
    expect(JSON.stringify(state.models.rows)).not.toMatch(/[/\\]|path/iu);
  });

  it('never copies demo runtime numbers into a connected snapshot with unsupported metrics', () => {
    const runtime = createDemoModelRuntimeSnapshot('model-less');

    const state = mergeControlModelRuntime(createDemoControlViewState(), runtime, true);

    expect(state.models.metrics.map(({ value }) => value)).toEqual(['—', '—', '3', '—']);
    expect(state.models.chart.samples).toEqual([]);
    expect(JSON.stringify(state.models)).not.toContain('34.2');
    expect(JSON.stringify(state.models)).not.toContain('18.4 GB');
  });

  it('maps one live shared host snapshot into the System overview', () => {
    const snapshot: SystemMonitorSnapshot = {
      ...SYSTEM_MONITOR_DEMO_SNAPSHOT,
      source: 'macOS host APIs',
      cpu: { logicalCores: 10, usagePercent: 30 },
      memory: { totalBytes: 32 * 1_024 ** 3, usedBytes: 18.4 * 1_024 ** 3 },
      network: {
        receivedBytes: 10_000,
        sentBytes: 5_000,
        receivedBytesPerSecond: 1_200,
        sentBytesPerSecond: 320,
      },
      power: { source: 'ac', batteryPercent: 81, charging: true },
      storage: { totalBytes: 512 * 1_024 ** 3, freeBytes: 218 * 1_024 ** 3 },
      cpuHistory: [20, 25, 30],
    };
    const loading = createConnectedResource<SystemMonitorSnapshot>('Local host system');
    const live = resolveDesktopData(
      beginDesktopDataRefresh(loading, Date.now(), 10_000),
      snapshot,
      'live',
      snapshot.source,
      Date.now(),
    );

    const system = mergeControlSystemMonitor(createDemoControlViewState().system, live);

    expect(system.metrics).toEqual([
      { value: '30%', label: 'CPU · 10 logical' },
      { value: '18.4 / 32 GB', label: 'Memory' },
      { value: '294 / 512 GB', label: 'Storage used' },
      { value: '1.2 KB/s ↓ · 320 B/s ↑', label: 'Network' },
      { value: '81%', label: 'Battery · charging' },
      { value: 'AC', label: 'Power source' },
    ]);
    expect(system.cpuSamples).toEqual([20, 25, 30]);
    expect(system.cpuChartTitle).toBe('CPU · recent samples');
    expect(system.cpuChartCaption).toBe('30% now');
    expect(system.processRows).toHaveLength(6);
  });

  it('never substitutes host demo values while a connected System overview is unavailable', () => {
    const connected = createConnectedResource<SystemMonitorSnapshot>('Local host system');

    const system = mergeControlSystemMonitor(createDemoControlViewState().system, connected);

    expect(system.metrics.map(({ value }) => value)).toEqual(['—', '—', '—', '—', '—', '—']);
    expect(system.cpuSamples).toEqual([]);
    expect(JSON.stringify(system.metrics)).not.toContain('18.4 / 32 GB');
  });

  it('preserves the labelled System fixture only in explicit demo mode', () => {
    const demo = createDemoResource(SYSTEM_MONITOR_DEMO_SNAPSHOT, 'Lethean demo fixture');
    const original = createDemoControlViewState().system;

    expect(mergeControlSystemMonitor(original, demo)).toBe(original);
  });
});

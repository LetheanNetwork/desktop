import { createDemoModelRuntimeSnapshot } from '../../desktop-model-runtime-demo.data';
import {
  createDemoControlViewState,
  mergeControlLiveSnapshot,
  mergeControlModelRuntime,
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
      unavailable: ['telemetry', 'benchmarkRuns'],
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
});

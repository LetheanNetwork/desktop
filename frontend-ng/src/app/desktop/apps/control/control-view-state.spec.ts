import { createDemoControlViewState, mergeControlLiveSnapshot } from './control-view-state';

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
    expect(state.settings.groups.map(({ name }) => name)).toEqual(['Server', 'Models']);
    expect(state.settings.flags).toHaveLength(3);
  });

  it('replaces only successful live sections', () => {
    const state = mergeControlLiveSnapshot({
      models: [
        {
          name: 'gemma.gguf',
          path: '/tmp/gemma.gguf',
          sizeBytes: 2_147_483_648,
          isDirectory: false,
        },
      ],
      processes: [
        {
          id: 'build-1',
          command: 'npm run build',
          status: 'running',
          exitCode: 0,
        },
      ],
      unavailable: ['telemetry', 'benchmarkRuns', 'settings'],
    });

    expect(state.dataState).toBe('mixed');
    expect(state.models.rows).toEqual([
      {
        name: 'gemma.gguf',
        size: '2 GB',
        source: 'local file',
        status: 'available',
      },
    ]);
    expect(state.system.processColumns.map(({ key }) => key)).toEqual([
      'command',
      'id',
      'state',
      'exit',
    ]);
    expect(state.power.metrics[0].value).toBe('196 W');
    expect(state.settings.groups[0].name).toBe('Server');
  });
});

// SPDX-License-Identifier: EUPL-1.2

import { createDemoModelRuntimeSnapshot } from '../../desktop-model-runtime-demo.data';
import { SYSTEM_MONITOR_DEMO_SNAPSHOT } from '../../desktop-system-monitor-demo.data';
import type { SystemMonitorSnapshot } from '../../desktop-system-monitor.models';
import { createTelemetryView } from './telemetry-view-state';

describe('Telemetry view state', () => {
  it('maps the deterministic host fixture into visibly labelled demo panels', () => {
    const view = createTelemetryView(
      SYSTEM_MONITOR_DEMO_SNAPSHOT,
      createDemoModelRuntimeSnapshot('ready'),
      'demo',
    );

    expect(view.primary).toEqual({
      label: 'CPU utilisation',
      value: '34',
      unit: '%',
      history: SYSTEM_MONITOR_DEMO_SNAPSHOT.cpuHistory,
      provenance: 'demo',
    });
    expect(view.secondary).toEqual({
      label: 'Memory used',
      value: '58',
      unit: '%',
      history: SYSTEM_MONITOR_DEMO_SNAPSHOT.memoryHistory,
      provenance: 'demo',
    });
    expect(view.primary.history).not.toBe(SYSTEM_MONITOR_DEMO_SNAPSHOT.cpuHistory);
    expect(view.metadata.map(({ label }) => label)).toEqual([
      'Network ↓',
      'Network ↑',
      'Storage',
      'Power',
      'System',
      'Model',
      'Model throughput',
    ]);
  });

  it('renders unsupported connected readings as unavailable without fixture substitution', () => {
    const snapshot: SystemMonitorSnapshot = {
      observedAt: '2026-07-31T12:00:05Z',
      source: 'Portable Go runtime',
      platform: 'linux',
      architecture: 'arm64',
      cpu: { logicalCores: 8 },
      cpuHistory: [],
      memoryHistory: [],
      networkReceivedHistory: [],
      networkSentHistory: [],
    };

    const view = createTelemetryView(
      snapshot,
      createDemoModelRuntimeSnapshot('model-less'),
      'live',
    );

    expect(view.primary).toMatchObject({ value: '—', unit: '%', history: [] });
    expect(view.secondary).toMatchObject({ value: '—', unit: '%', history: [] });
    expect(view.metadata.map(({ value }) => value)).toEqual([
      '—',
      '—',
      '—',
      '—',
      'linux · arm64',
      '—',
      '—',
    ]);
    expect(JSON.stringify(view)).not.toContain('41.8');
    expect(JSON.stringify(view)).not.toContain('207');
  });

  it('combines live host metrics with compact shared model-runtime context', () => {
    const runtime = {
      ...createDemoModelRuntimeSnapshot('ready'),
      metrics: {
        promptTokensPerSecond: 42.25,
        activeMemoryBytes: 2_147_483_648,
        uptimeSeconds: 600,
      },
    };
    const snapshot: SystemMonitorSnapshot = {
      ...SYSTEM_MONITOR_DEMO_SNAPSHOT,
      source: 'macOS host APIs',
      cpu: { logicalCores: 10, usagePercent: 37.5 },
      memory: { totalBytes: 32 * 1_024 ** 3, usedBytes: 16 * 1_024 ** 3 },
      network: {
        receivedBytes: 10_000,
        sentBytes: 5_000,
        receivedBytesPerSecond: 1_200,
        sentBytesPerSecond: 320,
      },
      power: { source: 'ac', batteryPercent: 81, charging: true },
      storage: { totalBytes: 512 * 1_024 ** 3, freeBytes: 218 * 1_024 ** 3 },
      cpuHistory: [30, 35, 37.5],
      memoryHistory: [48, 49, 50],
    };

    const view = createTelemetryView(snapshot, runtime, 'live');

    expect(view.primary).toMatchObject({ value: '38', history: [30, 35, 37.5] });
    expect(view.secondary).toMatchObject({ value: '50', history: [48, 49, 50] });
    expect(view.metadata.map(({ value }) => value)).toEqual([
      '1.2 KB/s',
      '320 B/s',
      '218 GB free of 512 GB',
      'AC · 81% · charging',
      'darwin · arm64',
      'gemma-4-e2b',
      '42.3 tok/s',
    ]);
  });
});

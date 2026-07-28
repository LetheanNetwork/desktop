// SPDX-License-Identifier: EUPL-1.2

import type { ProcessTelemetry } from '../../desktop-live-data.service';
import { createDemoModelRuntimeSnapshot } from '../../desktop-model-runtime-demo.data';
import type { TelemetryDemoSeries } from './telemetry-view.models';
import {
  createDemoTelemetryView,
  createLiveTelemetryView,
  overlayRuntimeTelemetryView,
} from './telemetry-view-state';

const SERIES: TelemetryDemoSeries = {
  throughput: [28, 30, 46],
  watts: [180, 199, 210],
};

const SAMPLE: ProcessTelemetry = {
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
};

describe('Telemetry view state', () => {
  it('creates a fresh deterministic demo without sharing mutable series', () => {
    const first = createDemoTelemetryView(SERIES);
    const second = createDemoTelemetryView(SERIES);

    expect(first).toEqual(second);
    expect(first.sample).toBeNull();
    expect(first.primary).toMatchObject({
      label: 'Throughput',
      value: '41.8',
      unit: 'tok/s',
      provenance: 'demo',
      history: SERIES.throughput,
    });
    expect(first.power).toMatchObject({
      label: 'Power draw',
      value: '207',
      unit: 'W',
      provenance: 'demo',
      history: SERIES.watts,
    });
    expect(first.primary.history).not.toBe(SERIES.throughput);
    expect(first.metadata.map(({ label }) => label)).toEqual([
      'Model',
      'Region',
      'KV-cache',
      'Uptime',
    ]);
  });

  it('maps unsupported connected runtime and power metrics to unavailable values', () => {
    const result = createLiveTelemetryView(
      SAMPLE,
      createDemoModelRuntimeSnapshot('model-less'),
      null,
    );

    expect(result.state).toBe('live');
    expect(result.value.sample).toBe(SAMPLE);
    expect(result.value.primary).toMatchObject({
      label: 'Throughput',
      value: '—',
      unit: 'tok/s',
      provenance: 'live',
      history: [],
    });
    expect(result.value.power).toMatchObject({
      label: 'Power draw',
      value: '—',
      unit: 'W',
      provenance: 'live',
      history: [],
    });
    expect(result.value.metadata.map(({ value }) => value)).toEqual(['—', '—', '—', '2h 31m']);
    expect(JSON.stringify(result.value)).not.toContain('41.8');
    expect(JSON.stringify(result.value)).not.toContain('207');
  });

  it('overlays shared runtime throughput, model, memory, and history', () => {
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
    const result = createLiveTelemetryView(SAMPLE, runtime, null);

    expect(result.state).toBe('live');
    expect(result.value.primary).toMatchObject({
      value: '42.3',
      history: [40, 42.25],
    });
    expect(result.value.metadata.map(({ value }) => value)).toEqual([
      'gemma-4-e2b',
      'metal',
      '2 GB',
      '10m',
    ]);
  });

  it('maps positive native power as live data and caps its history', () => {
    const runtime = createDemoModelRuntimeSnapshot('model-less');
    const initial = createLiveTelemetryView({ ...SAMPLE, wattsActive: 200 }, runtime, null).value;
    const previous = {
      ...initial,
      power: {
        ...initial.power,
        history: Object.freeze(Array.from({ length: 60 }, (_, index) => 100 + index)),
      },
    };

    const result = createLiveTelemetryView(
      { ...SAMPLE, heapAllocMB: 999, wattsActive: 250 },
      runtime,
      previous,
    );

    expect(result.value.power.history).toHaveLength(60);
    expect(result.value.power.history[0]).toBe(101);
    expect(result.value.power.history.at(-1)).toBe(250);
    expect(previous.power.history[0]).toBe(100);
  });

  it('does not carry any demo chart points into a connected view', () => {
    const demo = createDemoTelemetryView(SERIES);
    const result = createLiveTelemetryView(
      { ...SAMPLE, wattsActive: 220 },
      createDemoModelRuntimeSnapshot('model-less'),
      demo,
    );

    expect(result.value.primary.history).toEqual([]);
    expect(result.value.power.history).toEqual([220]);
  });

  it('reconciles a newer shared snapshot without mutating process telemetry', () => {
    const initial = createLiveTelemetryView(
      SAMPLE,
      createDemoModelRuntimeSnapshot('model-less'),
      null,
    ).value;
    const runtime = createDemoModelRuntimeSnapshot('ready');

    const overlaid = overlayRuntimeTelemetryView(initial, runtime);

    expect(overlaid.sample).toBe(SAMPLE);
    expect(overlaid.primary.value).toBe('41.8');
    expect(overlaid.metadata[0].value).toBe('gemma-4-e2b');
    expect(initial.primary.value).toBe('—');
  });
});

// SPDX-License-Identifier: EUPL-1.2

import type { ProcessTelemetry } from '../../desktop-live-data.service';
import type { TelemetryDemoSeries } from './telemetry-view.models';
import { createDemoTelemetryView, createLiveTelemetryView } from './telemetry-view-state';

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

  it('maps process data and labels absent native power as demo-backed mixed data', () => {
    const result = createLiveTelemetryView(SAMPLE, null, SERIES);

    expect(result.state).toBe('mixed');
    expect(result.value.sample).toBe(SAMPLE);
    expect(result.value.primary).toMatchObject({
      label: 'Heap allocation',
      value: '128.3',
      unit: 'MB',
      provenance: 'live',
      history: [128.25],
    });
    expect(result.value.power).toMatchObject({
      label: 'Power draw · demo',
      value: '207',
      unit: 'W',
      provenance: 'demo',
      history: SERIES.watts,
    });
    expect(result.value.metadata.map(({ value }) => value)).toEqual([
      '42',
      '0.43 ms',
      '7',
      '2h 31m',
    ]);
  });

  it('maps positive native power as live data', () => {
    const result = createLiveTelemetryView({ ...SAMPLE, wattsActive: 220.4 }, null, SERIES);

    expect(result.state).toBe('live');
    expect(result.value.power).toMatchObject({
      label: 'Power draw',
      value: '220',
      provenance: 'live',
      history: [220.4],
    });
  });

  it('appends and caps live histories without mutating the previous view', () => {
    const initial = createLiveTelemetryView({ ...SAMPLE, wattsActive: 200 }, null, SERIES).value;
    const previous = {
      ...initial,
      primary: {
        ...initial.primary,
        history: Object.freeze(Array.from({ length: 60 }, (_, index) => index)),
      },
      power: {
        ...initial.power,
        history: Object.freeze(Array.from({ length: 60 }, (_, index) => 100 + index)),
      },
    };

    const result = createLiveTelemetryView(
      { ...SAMPLE, heapAllocMB: 999, wattsActive: 250 },
      previous,
      SERIES,
    );

    expect(result.value.primary.history).toHaveLength(60);
    expect(result.value.primary.history[0]).toBe(1);
    expect(result.value.primary.history.at(-1)).toBe(999);
    expect(result.value.power.history).toHaveLength(60);
    expect(result.value.power.history[0]).toBe(101);
    expect(result.value.power.history.at(-1)).toBe(250);
    expect(previous.primary.history[0]).toBe(0);
    expect(previous.power.history[0]).toBe(100);
  });

  it('does not carry demo chart points into the first live history', () => {
    const demo = createDemoTelemetryView(SERIES);
    const result = createLiveTelemetryView({ ...SAMPLE, wattsActive: 220 }, demo, SERIES);

    expect(result.value.primary.history).toEqual([128.25]);
    expect(result.value.power.history).toEqual([220]);
  });
});

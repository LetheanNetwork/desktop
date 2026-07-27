// SPDX-License-Identifier: EUPL-1.2

import type { ProcessTelemetry } from '../../desktop-live-data.service';
import type { ModelRuntimeSnapshot } from '../../desktop-model-runtime.models';
import type {
  TelemetryDemoSeries,
  TelemetryLiveViewResult,
  TelemetryViewData,
} from './telemetry-view.models';

const MAX_HISTORY_SAMPLES = 60;

export function createDemoTelemetryView(series: TelemetryDemoSeries): TelemetryViewData {
  return {
    sample: null,
    primary: {
      label: $localize`:Telemetry metric@@telemetry.throughput:Throughput`,
      value: '41.8',
      unit: $localize`:Tokens per second unit@@unit.tokensPerSecondInline:tok/s`,
      history: [...series.throughput],
      provenance: 'demo',
    },
    power: {
      label: $localize`:Telemetry metric@@telemetry.powerDraw:Power draw`,
      value: '207',
      unit: $localize`:Watts unit@@unit.watts:W`,
      history: [...series.watts],
      provenance: 'demo',
    },
    metadata: [
      {
        label: $localize`:Telemetry model label@@telemetry.modelLabel:Model`,
        value: 'llama-3.1-70b',
      },
      {
        label: $localize`:Telemetry region label@@telemetry.regionLabel:Region`,
        value: 'eu-west-2',
      },
      {
        label: $localize`:Telemetry cache label@@telemetry.kvCacheLabel:KV-cache`,
        value: '62%',
      },
      {
        label: $localize`:Telemetry uptime label@@telemetry.uptimeLabel:Uptime`,
        value: '6d 4h',
      },
    ],
  };
}

export function createLiveTelemetryView(
  sample: ProcessTelemetry,
  runtime: ModelRuntimeSnapshot | null,
  previous: TelemetryViewData | null,
): TelemetryLiveViewResult {
  const previousLive = previous?.sample === null ? null : previous;
  const hasLivePower = sample.wattsActive > 0;
  const value = overlayRuntimeTelemetryView(
    {
      sample,
      primary: unavailableThroughput(),
      power: hasLivePower
        ? {
            label: $localize`:Telemetry metric@@telemetry.powerDraw:Power draw`,
            value: sample.wattsActive.toFixed(0),
            unit: $localize`:Watts unit@@unit.watts:W`,
            history: appendHistory(
              previousLive?.power.provenance === 'live' ? previousLive.power.history : [],
              sample.wattsActive,
            ),
            provenance: 'live',
          }
        : {
            label: $localize`:Telemetry metric@@telemetry.powerDraw:Power draw`,
            value: '—',
            unit: $localize`:Watts unit@@unit.watts:W`,
            history: [],
            provenance: 'live',
          },
      metadata: [],
    },
    runtime,
  );

  return { state: 'live', value };
}

export function overlayRuntimeTelemetryView(
  value: TelemetryViewData,
  runtime: ModelRuntimeSnapshot | null,
): TelemetryViewData {
  if (value.sample === null) return value;
  const activeModel = runtime?.models.find((model) => model.id === runtime.activeModelId) ?? null;
  const throughput = runtime?.metrics.promptTokensPerSecond;
  const throughputHistory =
    runtime?.history
      .flatMap((sample) =>
        sample.promptTokensPerSecond === undefined ? [] : [sample.promptTokensPerSecond],
      )
      .slice(-MAX_HISTORY_SAMPLES) ?? [];

  return {
    ...value,
    primary: {
      label: $localize`:Telemetry metric@@telemetry.throughput:Throughput`,
      value: throughput === undefined ? '—' : throughput.toFixed(1),
      unit: $localize`:Tokens per second unit@@unit.tokensPerSecondInline:tok/s`,
      history: throughputHistory,
      provenance: 'live',
    },
    metadata: [
      {
        label: $localize`:Telemetry model label@@telemetry.modelLabel:Model`,
        value: activeModel?.displayName ?? '—',
      },
      {
        label: $localize`:Telemetry runtime label@@telemetry.runtimeLabel:Runtime`,
        value: activeModel?.runtime || '—',
      },
      {
        label: $localize`:Telemetry memory label@@telemetry.memoryLabel:Memory`,
        value: formatBytes(runtime?.metrics.activeMemoryBytes),
      },
      {
        label: $localize`:Telemetry uptime label@@telemetry.uptimeLabel:Uptime`,
        value: formatUptime(runtime?.metrics.uptimeSeconds ?? value.sample.uptimeSeconds),
      },
    ],
  };
}

function unavailableThroughput(): TelemetryViewData['primary'] {
  return {
    label: $localize`:Telemetry metric@@telemetry.throughput:Throughput`,
    value: '—',
    unit: $localize`:Tokens per second unit@@unit.tokensPerSecondInline:tok/s`,
    history: [],
    provenance: 'live',
  };
}

function appendHistory(history: readonly number[], value: number): readonly number[] {
  return [...history, value].slice(-MAX_HISTORY_SAMPLES);
}

function formatUptime(seconds: number): string {
  const wholeSeconds = Math.max(0, Math.floor(seconds));
  const days = Math.floor(wholeSeconds / 86_400);
  const hours = Math.floor((wholeSeconds % 86_400) / 3_600);
  const minutes = Math.floor((wholeSeconds % 3_600) / 60);
  if (days > 0) return `${days}d ${hours}h`;
  if (hours > 0) return `${hours}h ${minutes}m`;
  return `${minutes}m`;
}

function formatBytes(bytes: number | undefined): string {
  if (bytes === undefined) return '—';
  if (bytes <= 0) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  const unitIndex = Math.min(units.length - 1, Math.floor(Math.log(bytes) / Math.log(1_024)));
  const value = bytes / 1_024 ** unitIndex;
  const precision = value >= 10 || Number.isInteger(value) ? 0 : 1;
  return `${value.toFixed(precision)} ${units[unitIndex]}`;
}

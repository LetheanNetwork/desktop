// SPDX-License-Identifier: EUPL-1.2

import type { ProcessTelemetry } from '../../desktop-live-data.service';
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
  previous: TelemetryViewData | null,
  demoSeries: TelemetryDemoSeries,
): TelemetryLiveViewResult {
  const previousLive = previous?.sample === null ? null : previous;
  const hasLivePower = sample.wattsActive > 0;
  const value: TelemetryViewData = {
    sample,
    primary: {
      label: $localize`:Process heap telemetry metric@@telemetry.heapAllocation:Heap allocation`,
      value: sample.heapAllocMB.toFixed(1),
      unit: $localize`:Megabytes unit@@unit.megabytes:MB`,
      history: appendHistory(previousLive?.primary.history ?? [], sample.heapAllocMB),
      provenance: 'live',
    },
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
          label: $localize`:Demo power telemetry metric@@telemetry.demoPowerDraw:Power draw · demo`,
          value: '207',
          unit: $localize`:Watts unit@@unit.watts:W`,
          history: [...demoSeries.watts],
          provenance: 'demo',
        },
    metadata: [
      {
        label: $localize`:Telemetry goroutines label@@telemetry.goroutines:Goroutines`,
        value: String(sample.numGoroutines),
      },
      {
        label: $localize`:Telemetry GC pause label@@telemetry.gcPause:GC pause`,
        value: `${sample.lastGCPauseMs} ms`,
      },
      {
        label: $localize`:Telemetry CGO calls label@@telemetry.cgoCalls:CGO calls`,
        value: String(sample.numCgoCalls),
      },
      {
        label: $localize`:Telemetry uptime label@@telemetry.uptimeLabel:Uptime`,
        value: formatUptime(sample.uptimeSeconds),
      },
    ],
  };

  return { state: hasLivePower ? 'live' : 'mixed', value };
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

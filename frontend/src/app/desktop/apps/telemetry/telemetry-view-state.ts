// SPDX-License-Identifier: EUPL-1.2

import type { ModelRuntimeSnapshot } from '../../desktop-model-runtime.models';
import type { SystemMonitorSnapshot } from '../../desktop-system-monitor.models';
import type { TelemetryPanelProvenance, TelemetryViewData } from './telemetry-view.models';

export function createTelemetryView(
  sample: SystemMonitorSnapshot,
  runtime: ModelRuntimeSnapshot | null,
  provenance: TelemetryPanelProvenance,
): TelemetryViewData {
  const activeModel = runtime?.models.find((model) => model.id === runtime.activeModelId) ?? null;
  return {
    sample,
    primary: {
      label: $localize`:Host CPU telemetry metric@@telemetry.cpuUtilisation:CPU utilisation`,
      value: formatPercentValue(sample.cpu.usagePercent),
      unit: '%',
      history: [...sample.cpuHistory],
      provenance,
    },
    secondary: {
      label: $localize`:Host memory telemetry metric@@telemetry.memoryUsed:Memory used`,
      value: formatPercentValue(memoryPercent(sample)),
      unit: '%',
      history: [...sample.memoryHistory],
      provenance,
    },
    metadata: [
      {
        label: $localize`:Network received rate@@telemetry.networkReceived:Network ↓`,
        value: formatRate(sample.network?.receivedBytesPerSecond),
      },
      {
        label: $localize`:Network sent rate@@telemetry.networkSent:Network ↑`,
        value: formatRate(sample.network?.sentBytesPerSecond),
      },
      {
        label: $localize`:Storage capacity@@telemetry.storage:Storage`,
        value: formatStorage(sample),
      },
      {
        label: $localize`:Host power status@@telemetry.power:Power`,
        value: formatPower(sample),
      },
      {
        label: $localize`:Host system identity@@telemetry.system:System`,
        value: `${sample.platform} · ${sample.architecture}`,
      },
      {
        label: $localize`:Telemetry model label@@telemetry.modelLabel:Model`,
        value: activeModel?.displayName ?? '—',
      },
      {
        label: $localize`:Model throughput metadata@@telemetry.modelThroughput:Model throughput`,
        value:
          runtime?.metrics.promptTokensPerSecond === undefined
            ? '—'
            : `${runtime.metrics.promptTokensPerSecond.toFixed(1)} tok/s`,
      },
    ],
  };
}

function memoryPercent(sample: SystemMonitorSnapshot): number | undefined {
  return sample.memory ? (sample.memory.usedBytes / sample.memory.totalBytes) * 100 : undefined;
}

function formatPercentValue(value: number | undefined): string {
  return value === undefined ? '—' : Math.round(value).toString();
}

function formatRate(bytesPerSecond: number | undefined): string {
  return bytesPerSecond === undefined ? '—' : `${formatBytes(bytesPerSecond)}/s`;
}

function formatStorage(sample: SystemMonitorSnapshot): string {
  if (!sample.storage) return '—';
  return `${formatBytes(sample.storage.freeBytes)} free of ${formatBytes(sample.storage.totalBytes)}`;
}

function formatPower(sample: SystemMonitorSnapshot): string {
  const power = sample.power;
  if (!power) return '—';
  const parts: string[] = [];
  if (power.source === 'ac') parts.push('AC');
  else if (power.source === 'battery') {
    parts.push($localize`:Battery power source@@telemetry.onBattery:Battery`);
  }
  if (power.batteryPercent !== undefined) parts.push(`${Math.round(power.batteryPercent)}%`);
  if (power.charging === true) {
    parts.push($localize`:Battery charging status@@telemetry.charging:charging`);
  }
  return parts.length > 0 ? parts.join(' · ') : '—';
}

function formatBytes(bytes: number): string {
  if (bytes <= 0) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  const unitIndex = Math.min(units.length - 1, Math.floor(Math.log(bytes) / Math.log(1_024)));
  const value = bytes / 1_024 ** unitIndex;
  const precision = value >= 10 || Number.isInteger(value) ? 0 : 1;
  return `${value.toFixed(precision)} ${units[unitIndex]}`;
}

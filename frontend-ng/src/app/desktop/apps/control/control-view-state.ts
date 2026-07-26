// SPDX-License-Identifier: EUPL-1.2

import type { ControlLiveSnapshot } from '../../desktop-live-data.service';
import { CONTROL_DEMO_VIEW_STATE } from './control-demo.data';
import type {
  ControlModelsViewModel,
  ControlRunsViewModel,
  ControlSettingsViewModel,
  ControlSystemViewModel,
  ControlViewState,
} from './control-view.models';

export function createDemoControlViewState(): ControlViewState {
  return {
    ...CONTROL_DEMO_VIEW_STATE,
    models: { ...CONTROL_DEMO_VIEW_STATE.models },
    runs: { ...CONTROL_DEMO_VIEW_STATE.runs },
    power: { ...CONTROL_DEMO_VIEW_STATE.power },
    system: { ...CONTROL_DEMO_VIEW_STATE.system },
    settings: { ...CONTROL_DEMO_VIEW_STATE.settings },
  };
}

export function mergeControlLiveSnapshot(snapshot: ControlLiveSnapshot): ControlViewState {
  const demo = createDemoControlViewState();
  return {
    ...demo,
    dataState: 'mixed',
    models: mergeModels(demo.models, snapshot),
    runs: mergeRuns(demo.runs, snapshot),
    system: mergeSystem(demo.system, snapshot),
    settings: mergeSettings(demo.settings, snapshot),
  };
}

function mergeModels(
  demo: ControlModelsViewModel,
  snapshot: ControlLiveSnapshot,
): ControlModelsViewModel {
  const metrics = demo.metrics.map((metric) => ({ ...metric }));
  let chart = demo.chart;
  let columns = demo.columns;
  let rows = demo.rows;

  if (snapshot.models) {
    const totalBytes = snapshot.models.reduce((total, model) => total + model.sizeBytes, 0);
    metrics[1] = {
      value: formatBytes(totalBytes),
      label: $localize`:Model storage metric@@control.models.storage:Storage`,
    };
    metrics[2] = {
      value: String(snapshot.models.length),
      label: $localize`:Local model count metric@@control.models.localCount:Local models`,
    };
    columns = [
      {
        key: 'name',
        label: $localize`:Model table column@@control.column.model:Model`,
      },
      {
        key: 'size',
        label: $localize`:Model size column@@control.column.size:Size`,
      },
      {
        key: 'source',
        label: $localize`:Model source column@@control.column.source:Source`,
        type: 'mono',
      },
      {
        key: 'status',
        label: $localize`:Model table column@@control.column.state:State`,
        type: 'status',
      },
    ];
    rows = snapshot.models.map((model) => ({
      name: model.name,
      size: formatBytes(model.sizeBytes),
      source: model.isDirectory ? 'local directory' : 'local file',
      status: 'available',
    }));
  }

  if (snapshot.benchmarkRuns) {
    const rates = snapshot.benchmarkRuns.map((run) => run.generatedTokensPerSecond);
    const latest = rates[0];
    metrics[0] = {
      value: latest === undefined ? '—' : latest.toFixed(1),
      label: $localize`:Latest generated throughput metric@@control.models.latestThroughput:Latest tg tok/s`,
    };
    chart = {
      title: $localize`:Recent benchmark chart title@@control.models.recentBenchmarks:Benchmark throughput · recent runs`,
      caption: rates.length
        ? `peak ${Math.max(...rates).toFixed(1)} tok/s`
        : $localize`:No benchmark history@@control.models.noBenchmarks:No benchmark history`,
      samples: rates,
    };
  }

  if (snapshot.telemetry) {
    metrics[3] = {
      value: formatUptime(snapshot.telemetry.uptimeSeconds),
      label: demo.metrics[3].label,
    };
  }

  return { metrics, chart, columns, rows };
}

function mergeRuns(
  demo: ControlRunsViewModel,
  snapshot: ControlLiveSnapshot,
): ControlRunsViewModel {
  if (!snapshot.benchmarkRuns) return demo;
  return {
    ...demo,
    chart: {
      ...demo.chart,
      samples: snapshot.benchmarkRuns.map((run) => run.generatedTokensPerSecond),
    },
    rows: snapshot.benchmarkRuns.map((run) => ({
      run: `#${run.id.slice(0, 8)}`,
      model: run.model,
      ctx: run.contextLength,
      toks: Number(run.generatedTokensPerSecond.toFixed(1)),
      when: benchmarkTime(run.timestamp),
    })),
  };
}

function mergeSystem(
  demo: ControlSystemViewModel,
  snapshot: ControlLiveSnapshot,
): ControlSystemViewModel {
  let metrics = demo.metrics;
  let processColumns = demo.processColumns;
  let processRows = demo.processRows;

  if (snapshot.telemetry) {
    metrics = [
      {
        value: formatMegabytes(snapshot.telemetry.heapAllocMB),
        label: $localize`:Process heap metric@@control.system.processHeap:Process heap`,
      },
      {
        value: formatMegabytes(snapshot.telemetry.heapSysMB),
        label: $localize`:Reserved heap metric@@control.system.reservedHeap:Reserved heap`,
      },
      {
        value: String(snapshot.telemetry.numGoroutines),
        label: $localize`:Goroutine count metric@@control.system.goroutines:Goroutines`,
      },
      {
        value: String(snapshot.telemetry.numGC),
        label: $localize`:Garbage collection count metric@@control.system.gcCycles:GC cycles`,
      },
    ];
  }

  if (snapshot.processes) {
    processColumns = [
      {
        key: 'command',
        label: $localize`:Process command column@@control.column.command:Command`,
      },
      {
        key: 'id',
        label: $localize`:Process identifier column@@control.column.processId:ID`,
        type: 'mono',
      },
      {
        key: 'state',
        label: $localize`:Process table column@@control.column.state:State`,
        type: 'status',
      },
      {
        key: 'exit',
        label: $localize`:Process exit code column@@control.column.exitCode:Exit code`,
        type: 'num',
      },
    ];
    processRows = snapshot.processes.map((process) => ({
      command: process.command,
      id: process.id,
      state: process.status,
      exit: process.exitCode,
    }));
  }

  return {
    ...demo,
    metrics,
    processColumns,
    processRows,
  };
}

function mergeSettings(
  demo: ControlSettingsViewModel,
  snapshot: ControlLiveSnapshot,
): ControlSettingsViewModel {
  if (!snapshot.settings) return demo;

  const grouped = new Map<string, Array<{ key: string; value: string; source: string }>>();
  for (const control of snapshot.settings.controls) {
    if (control.kind === 'toggle') continue;
    const rows = grouped.get(control.group) ?? [];
    rows.push({
      key: control.key,
      value: String(control.value),
      source: control.configured ? 'set' : 'default',
    });
    grouped.set(control.group, rows);
  }

  return {
    groups: [...grouped].map(([name, rows]) => ({ name, rows })),
    flags: snapshot.settings.controls
      .filter((control) => control.kind === 'toggle')
      .map((control) => ({
        key: control.key,
        on: control.value === true,
        source: control.configured ? 'set' : 'default',
      })),
  };
}

function formatBytes(bytes: number): string {
  if (bytes <= 0) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  const unitIndex = Math.min(units.length - 1, Math.floor(Math.log(bytes) / Math.log(1_024)));
  const value = bytes / 1_024 ** unitIndex;
  const precision = value >= 10 || Number.isInteger(value) ? 0 : 1;
  return `${value.toFixed(precision)} ${units[unitIndex]}`;
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

function formatMegabytes(value: number): string {
  const precision = value >= 10 || Number.isInteger(value) ? 0 : 1;
  return `${value.toFixed(precision)} MB`;
}

function benchmarkTime(timestamp: string): string {
  return timestamp.match(/T(\d{2}:\d{2})/u)?.[1] ?? timestamp;
}

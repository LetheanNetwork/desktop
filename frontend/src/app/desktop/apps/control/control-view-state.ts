// SPDX-License-Identifier: EUPL-1.2

import type { ControlLiveSnapshot } from '../../desktop-live-data.service';
import type { ModelRuntimeSnapshot } from '../../desktop-model-runtime.models';
import { CONTROL_DEMO_VIEW_STATE } from './control-demo.data';
import type {
  ControlRunsViewModel,
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
  };
}

export function mergeControlLiveSnapshot(snapshot: ControlLiveSnapshot): ControlViewState {
  const demo = createDemoControlViewState();
  return {
    ...demo,
    dataState: 'mixed',
    runs: mergeRuns(demo.runs, snapshot),
    system: mergeSystem(demo.system, snapshot),
  };
}

export function mergeControlModelRuntime(
  state: ControlViewState,
  snapshot: ModelRuntimeSnapshot | null,
  connected: boolean,
): ControlViewState {
  if (snapshot === null && !connected) return state;

  const models = snapshot?.models ?? [];
  const throughput = snapshot?.metrics.promptTokensPerSecond;
  const history =
    snapshot?.history.flatMap((sample) =>
      sample.promptTokensPerSecond === undefined ? [] : [sample.promptTokensPerSecond],
    ) ?? [];
  const peak = history.length ? Math.max(...history) : null;

  return {
    ...state,
    models: {
      state: snapshot?.state ?? 'unavailable',
      activeModelId: snapshot?.activeModelId ?? '',
      availableModels: models,
      metrics: [
        {
          value: formatOptionalRate(throughput),
          label: $localize`:Model runtime throughput metric@@control.models.runtimeThroughput:tok/s`,
        },
        {
          value: formatOptionalBytes(snapshot?.metrics.activeMemoryBytes),
          label: $localize`:Active model memory metric@@control.models.activeMemory:Active memory`,
        },
        {
          value: snapshot === null ? '—' : String(models.length),
          label: $localize`:Local model count metric@@control.models.localCount:Local models`,
        },
        {
          value:
            snapshot?.metrics.uptimeSeconds === undefined
              ? '—'
              : formatUptime(snapshot.metrics.uptimeSeconds),
          label: $localize`:Model runtime uptime metric@@control.models.runtimeUptime:Uptime`,
        },
      ],
      chart: {
        title: $localize`:Throughput chart title@@control.models.throughputChart:Throughput · recent samples`,
        caption:
          peak === null
            ? $localize`:Unavailable runtime history@@control.models.noRuntimeHistory:Runtime telemetry unavailable`
            : `peak ${peak.toFixed(1)} tok/s`,
        samples: history,
      },
      columns: [
        {
          key: 'id',
          label: $localize`:Model identifier column@@control.column.modelId:ID`,
          type: 'mono',
        },
        {
          key: 'name',
          label: $localize`:Model table column@@control.column.model:Model`,
        },
        {
          key: 'format',
          label: $localize`:Model format column@@control.column.format:Format`,
          type: 'mono',
        },
        {
          key: 'runtime',
          label: $localize`:Model runtime column@@control.column.runtime:Runtime`,
          type: 'mono',
        },
        {
          key: 'status',
          label: $localize`:Model table column@@control.column.state:State`,
          type: 'status',
        },
      ],
      rows: models.map((model) => ({
        id: model.id,
        name: model.displayName,
        format: model.format,
        runtime: model.runtime || '—',
        status: model.loaded ? 'loaded' : model.loadable ? 'available' : 'unavailable',
      })),
    },
  };
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

function formatOptionalRate(value: number | undefined): string {
  return value === undefined ? '—' : value.toFixed(1);
}

function formatOptionalBytes(bytes: number | undefined): string {
  if (bytes === undefined) return '—';
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

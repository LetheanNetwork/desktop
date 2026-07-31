import { Injectable, computed, inject } from '@angular/core';
import { ConnectionManagerService } from '../connection-manager.service';
import { SurfaceBridgeService } from './surfaces/surface-bridge.service';

const TELEMETRY_METHOD = 'dappco.re/lthn/desktop/pkg/telemetry.Service.CurrentSample';
const BENCHMARK_HISTORY_METHOD = 'dappco.re/lthn/desktop/pkg/benchmark.Service.History';
const PROCESS_LIST_METHOD = 'dappco.re/lthn/desktop/pkg/build.Service.ProcessList';

export type DesktopDataMode = 'demo' | 'live';

export interface ProcessTelemetry {
  readonly heapAllocMB: number;
  readonly heapSysMB: number;
  readonly stackInUseMB: number;
  readonly numGoroutines: number;
  readonly numCgoCalls: number;
  readonly uptimeSeconds: number;
  readonly numGC: number;
  readonly lastGCPauseMs: number;
  readonly wattsActive: number;
  readonly wattsIdle: number;
}

export interface BenchmarkRun {
  readonly id: string;
  readonly timestamp: string;
  readonly bencher: string;
  readonly model: string;
  readonly contextLength: number;
  readonly promptTokensPerSecond: number;
  readonly generatedTokensPerSecond: number;
  readonly promptLength: number;
  readonly outputLength: number;
  readonly peakWatts?: number;
  readonly peakMemoryMB?: number;
  readonly endpoint?: string;
}

export interface DesktopProcess {
  readonly id: string;
  readonly command: string;
  readonly status: string;
  readonly exitCode: number;
}

export type ControlDataSection = 'telemetry' | 'benchmarkRuns' | 'processes';

export interface ControlLiveSnapshot {
  readonly telemetry?: ProcessTelemetry;
  readonly benchmarkRuns?: readonly BenchmarkRun[];
  readonly processes?: readonly DesktopProcess[];
  readonly unavailable: readonly ControlDataSection[];
}

@Injectable({ providedIn: 'root' })
export class DesktopLiveDataService {
  private readonly connection = inject(ConnectionManagerService);
  private readonly bridge = inject(SurfaceBridgeService);

  readonly mode = computed<DesktopDataMode>(() => (this.connection.offline() ? 'demo' : 'live'));

  async telemetry(): Promise<ProcessTelemetry> {
    this.requireLiveMode();
    return parseTelemetry(await this.bridge.call(TELEMETRY_METHOD));
  }

  async benchmarkRuns(limit = 20): Promise<readonly BenchmarkRun[]> {
    this.requireLiveMode();
    const raw = await this.bridge.call(BENCHMARK_HISTORY_METHOD, [
      {
        Bencher: '',
        Model: '',
        MinCtx: 0,
        MaxCtx: 0,
        Limit: limit,
        Offset: 0,
      },
    ]);
    if (!Array.isArray(raw)) {
      throw new Error('The benchmark history response is unavailable.');
    }
    return raw.map(parseBenchmarkRun);
  }

  async processes(): Promise<readonly DesktopProcess[]> {
    this.requireLiveMode();
    const raw = await this.bridge.call(PROCESS_LIST_METHOD);
    if (!Array.isArray(raw)) {
      throw new Error('The process registry response is unavailable.');
    }
    return raw.map(parseProcess);
  }

  async control(): Promise<ControlLiveSnapshot> {
    this.requireLiveMode();
    const reads = await Promise.allSettled([
      this.telemetry(),
      this.benchmarkRuns(20),
      this.processes(),
    ] as const);
    const sectionNames: readonly ControlDataSection[] = ['telemetry', 'benchmarkRuns', 'processes'];
    const unavailable = reads.flatMap((result, index) =>
      result.status === 'rejected' ? [sectionNames[index]] : [],
    );
    if (unavailable.length === sectionNames.length) {
      throw new Error('Live Control data is unavailable.');
    }

    return {
      ...fulfilledProperty('telemetry', reads[0]),
      ...fulfilledProperty('benchmarkRuns', reads[1]),
      ...fulfilledProperty('processes', reads[2]),
      unavailable,
    };
  }

  private requireLiveMode(): void {
    if (this.mode() === 'demo') {
      throw new Error('Demo mode does not call live desktop services.');
    }
  }
}

function fulfilledProperty<Key extends string, Value>(
  key: Key,
  result: PromiseSettledResult<Value>,
): Partial<Record<Key, Value>> {
  return result.status === 'fulfilled' ? ({ [key]: result.value } as Record<Key, Value>) : {};
}

function parseTelemetry(raw: unknown): ProcessTelemetry {
  const record = requiredRecord(raw, 'process telemetry');
  return {
    heapAllocMB: requiredNumber(record, 'heap_alloc_mb', 'process telemetry'),
    heapSysMB: requiredNumber(record, 'heap_sys_mb', 'process telemetry'),
    stackInUseMB: requiredNumber(record, 'stack_in_use_mb', 'process telemetry'),
    numGoroutines: requiredNumber(record, 'num_goroutines', 'process telemetry'),
    numCgoCalls: requiredNumber(record, 'num_cgo_calls', 'process telemetry'),
    uptimeSeconds: requiredNumber(record, 'uptime_seconds', 'process telemetry'),
    numGC: requiredNumber(record, 'num_gc', 'process telemetry'),
    lastGCPauseMs: requiredNumber(record, 'last_gc_pause_ms', 'process telemetry'),
    wattsActive: requiredNumber(record, 'watts_active', 'process telemetry'),
    wattsIdle: requiredNumber(record, 'watts_idle', 'process telemetry'),
  };
}

function parseBenchmarkRun(raw: unknown): BenchmarkRun {
  const record = requiredRecord(raw, 'benchmark history');
  return {
    id: requiredString(record, 'id', 'benchmark history'),
    timestamp: requiredString(record, 'timestamp', 'benchmark history'),
    bencher: requiredString(record, 'bencher', 'benchmark history'),
    model: requiredString(record, 'model', 'benchmark history'),
    contextLength: requiredNumber(record, 'ctx', 'benchmark history'),
    promptTokensPerSecond: requiredNumber(record, 'pp_tok_sec', 'benchmark history'),
    generatedTokensPerSecond: requiredNumber(record, 'tg_tok_sec', 'benchmark history'),
    promptLength: requiredNumber(record, 'prompt_len', 'benchmark history'),
    outputLength: requiredNumber(record, 'output_len', 'benchmark history'),
    ...optionalNumber(record, 'peak_watts', 'peakWatts'),
    ...optionalNumber(record, 'peak_mem_mb', 'peakMemoryMB'),
    ...optionalString(record, 'endpoint', 'endpoint'),
  };
}

function parseProcess(raw: unknown): DesktopProcess {
  const record = requiredRecord(raw, 'process registry');
  return {
    id: requiredString(record, 'id', 'process registry'),
    command: requiredString(record, 'command', 'process registry'),
    status: requiredString(record, 'status', 'process registry'),
    exitCode: requiredNumber(record, 'exit_code', 'process registry'),
  };
}

function requiredRecord(raw: unknown, description: string): Record<string, unknown> {
  if (raw === null || typeof raw !== 'object' || Array.isArray(raw)) {
    throw new Error(`The ${description} response is unavailable.`);
  }
  return raw as Record<string, unknown>;
}

function requiredNumber(record: Record<string, unknown>, key: string, description: string): number {
  const value = record[key];
  if (typeof value !== 'number' || !Number.isFinite(value)) {
    throw new Error(`The ${description} response has no valid ${key}.`);
  }
  return value;
}

function requiredString(record: Record<string, unknown>, key: string, description: string): string {
  const value = record[key];
  if (typeof value !== 'string' || value === '') {
    throw new Error(`The ${description} response has no valid ${key}.`);
  }
  return value;
}

function optionalNumber<OutputKey extends string>(
  record: Record<string, unknown>,
  sourceKey: string,
  outputKey: OutputKey,
): Partial<Record<OutputKey, number>> {
  const value = record[sourceKey];
  return typeof value === 'number' && Number.isFinite(value)
    ? ({ [outputKey]: value } as Record<OutputKey, number>)
    : {};
}

function optionalString<OutputKey extends string>(
  record: Record<string, unknown>,
  sourceKey: string,
  outputKey: OutputKey,
): Partial<Record<OutputKey, string>> {
  const value = record[sourceKey];
  return typeof value === 'string' ? ({ [outputKey]: value } as Record<OutputKey, string>) : {};
}

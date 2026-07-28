// SPDX-License-Identifier: EUPL-1.2

import { Injectable, InjectionToken, inject } from '@angular/core';
import { Events } from '@wailsio/runtime';
import { ConnectionManagerService } from '../connection-manager.service';
import type {
  ModelRuntimeChangedEvent,
  ModelRuntimeErrorCode,
  ModelRuntimeFailure,
  ModelRuntimeMetrics,
  ModelRuntimeModel,
  ModelRuntimeSample,
  ModelRuntimeSnapshot,
  ModelRuntimeState,
} from './desktop-model-runtime.models';
import { SurfaceBridgeService } from './surfaces/surface-bridge.service';

const MODEL_RUNTIME_SERVICE = 'dappco.re/lthn/desktop/pkg/modelruntime.WailsService';
const MODEL_RUNTIME_CHANGED_EVENT = 'lthn:model-runtime:changed';
const MAX_MODELS = 512;
const MAX_SAMPLES = 720;
const MODEL_ID = /^model-[0-9a-f]{16}$/u;
const RFC3339_TIMESTAMP =
  /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d{1,9})?(?:Z|[+-]\d{2}:\d{2})$/u;

export const MODEL_RUNTIME_METHODS = {
  snapshot: `${MODEL_RUNTIME_SERVICE}.Snapshot`,
  start: `${MODEL_RUNTIME_SERVICE}.Start`,
  load: `${MODEL_RUNTIME_SERVICE}.Load`,
  unload: `${MODEL_RUNTIME_SERVICE}.Unload`,
  restart: `${MODEL_RUNTIME_SERVICE}.Restart`,
  stop: `${MODEL_RUNTIME_SERVICE}.Stop`,
} as const;

export interface ModelRuntimeEventSource {
  on(name: string, handler: (payload: unknown) => void): () => void;
}

export const MODEL_RUNTIME_EVENT_SOURCE = new InjectionToken<ModelRuntimeEventSource>(
  'MODEL_RUNTIME_EVENT_SOURCE',
  {
    providedIn: 'root',
    factory: () => ({
      on(name, handler): () => void {
        return Events.On(name, (event) => handler(event.data));
      },
    }),
  },
);

@Injectable({ providedIn: 'root' })
export class DesktopModelRuntimeBridgeService {
  private readonly surface = inject(SurfaceBridgeService);
  private readonly connection = inject(ConnectionManagerService);
  private readonly events = inject(MODEL_RUNTIME_EVENT_SOURCE);

  async snapshot(): Promise<ModelRuntimeSnapshot> {
    return this.read(MODEL_RUNTIME_METHODS.snapshot);
  }

  async start(): Promise<ModelRuntimeSnapshot> {
    return this.read(MODEL_RUNTIME_METHODS.start);
  }

  async load(modelId: string): Promise<ModelRuntimeSnapshot> {
    const id = requireModelID(modelId);
    return this.read(MODEL_RUNTIME_METHODS.load, [{ modelId: id }]);
  }

  async unload(): Promise<ModelRuntimeSnapshot> {
    return this.read(MODEL_RUNTIME_METHODS.unload);
  }

  async restart(): Promise<ModelRuntimeSnapshot> {
    return this.read(MODEL_RUNTIME_METHODS.restart);
  }

  async stop(): Promise<ModelRuntimeSnapshot> {
    return this.read(MODEL_RUNTIME_METHODS.stop);
  }

  onChanged(handler: (event: ModelRuntimeChangedEvent) => void): () => void {
    if (this.connection.offline()) return () => undefined;
    return this.events.on(MODEL_RUNTIME_CHANGED_EVENT, (raw) => {
      try {
        rejectModelRuntimeForbiddenFields(raw);
        handler(parseModelRuntimeChangedEvent(raw));
      } catch {
        // Events are advisory invalidations. A later bounded snapshot remains
        // canonical when malformed event data is dropped.
      }
    });
  }

  private async read(method: string, args?: readonly unknown[]): Promise<ModelRuntimeSnapshot> {
    if (this.connection.offline()) {
      throw new Error('The ModelRuntime live bridge is unavailable in offline demo mode.');
    }
    const raw = args ? await this.surface.call(method, args) : await this.surface.call(method);
    rejectModelRuntimeForbiddenFields(raw);
    return parseModelRuntimeSnapshot(raw);
  }
}

const STATES: readonly ModelRuntimeState[] = [
  'unavailable',
  'stopped',
  'starting',
  'model-less',
  'loading',
  'ready',
  'degraded',
  'failed',
  'stopping',
];

const ERROR_CODES: readonly ModelRuntimeErrorCode[] = [
  'runtime_unavailable',
  'binary_missing',
  'runtime_stopped',
  'runtime_start_failed',
  'runtime_not_ready',
  'catalogue_unavailable',
  'model_not_found',
  'model_not_loadable',
  'model_load_failed',
  'model_unload_failed',
  'admin_unauthorised',
  'operation_in_progress',
  'response_invalid',
  'response_too_large',
  'request_timeout',
  'runtime_stop_failed',
];

const FORBIDDEN_RESPONSE_FIELDS = new Set([
  'path',
  'modelpath',
  'command',
  'arguments',
  'environment',
  'workingdirectory',
  'endpoint',
  'url',
  'token',
  'secret',
  'credential',
  'key',
]);

export function parseModelRuntimeSnapshot(raw: unknown): ModelRuntimeSnapshot {
  const record = requiredRecord(raw, 'snapshot');
  const models = requiredArray(record['models'], 'snapshot models', MAX_MODELS).map(parseModel);
  const ids = new Set<string>();
  for (const model of models) {
    if (ids.has(model.id)) invalidResponse('duplicate model ID');
    ids.add(model.id);
  }
  const activeModelId =
    record['activeModelId'] === ''
      ? ''
      : providerModelID(record['activeModelId'], 'snapshot activeModelId');
  if (activeModelId !== '' && !ids.has(activeModelId)) {
    invalidResponse('unknown active model ID');
  }
  return {
    state: requiredEnum(record['state'], STATES, 'snapshot state'),
    desired: requiredBoolean(record['desired'], 'snapshot desired'),
    activeModelId,
    models,
    metrics: parseMetrics(record['metrics']),
    history: requiredArray(record['history'], 'snapshot history', MAX_SAMPLES).map(parseSample),
    refreshedAt: requiredTimestamp(record['refreshedAt'], 'snapshot refreshedAt'),
    lastHealthyAt: requiredTimestamp(record['lastHealthyAt'], 'snapshot lastHealthyAt', true),
    stale: requiredBoolean(record['stale'], 'snapshot stale'),
    lastError: record['lastError'] === null ? null : parseFailure(record['lastError']),
  };
}

function parseModel(raw: unknown): ModelRuntimeModel {
  const record = requiredRecord(raw, 'model');
  return {
    id: providerModelID(record['id'], 'model ID'),
    displayName: boundedString(record['displayName'], 'model displayName', 255),
    format: boundedString(record['format'], 'model format', 64),
    loadable: requiredBoolean(record['loadable'], 'model loadable'),
    loaded: requiredBoolean(record['loaded'], 'model loaded'),
    ...optionalStringProperty(record, 'runtime', 'model runtime', 128),
    ...optionalIntegerProperty(
      record,
      'contextLength',
      'model contextLength',
      0,
      Number.MAX_SAFE_INTEGER,
    ),
    ...optionalTimestampProperty(record, 'loadedAt', 'model loadedAt'),
  };
}

function parseMetrics(raw: unknown): ModelRuntimeMetrics {
  const record = requiredRecord(raw, 'metrics');
  return {
    ...optionalNumberProperty(record, 'promptTokensPerSecond', 'prompt throughput'),
    ...optionalNumberProperty(record, 'decodeTokensPerSecond', 'decode throughput'),
    ...optionalIntegerProperty(
      record,
      'activeMemoryBytes',
      'active memory',
      0,
      Number.MAX_SAFE_INTEGER,
    ),
    ...optionalIntegerProperty(
      record,
      'peakMemoryBytes',
      'peak memory',
      0,
      Number.MAX_SAFE_INTEGER,
    ),
    ...optionalIntegerProperty(record, 'kvCacheBytes', 'KV cache', 0, Number.MAX_SAFE_INTEGER),
    ...optionalIntegerProperty(
      record,
      'uptimeSeconds',
      'runtime uptime',
      0,
      Number.MAX_SAFE_INTEGER,
    ),
  };
}

function parseSample(raw: unknown): ModelRuntimeSample {
  const record = requiredRecord(raw, 'sample');
  return {
    state: requiredEnum(record['state'], STATES, 'sample state'),
    at: requiredTimestamp(record['at'], 'sample at'),
    ...optionalNumberProperty(record, 'promptTokensPerSecond', 'sample prompt throughput'),
    ...optionalNumberProperty(record, 'decodeTokensPerSecond', 'sample decode throughput'),
    ...optionalIntegerProperty(
      record,
      'activeMemoryBytes',
      'sample active memory',
      0,
      Number.MAX_SAFE_INTEGER,
    ),
    ...optionalIntegerProperty(
      record,
      'peakMemoryBytes',
      'sample peak memory',
      0,
      Number.MAX_SAFE_INTEGER,
    ),
    ...optionalIntegerProperty(
      record,
      'kvCacheBytes',
      'sample KV cache',
      0,
      Number.MAX_SAFE_INTEGER,
    ),
  };
}

function parseFailure(raw: unknown): ModelRuntimeFailure {
  const record = requiredRecord(raw, 'failure');
  return {
    code: requiredEnum(record['code'], ERROR_CODES, 'failure code'),
    message: boundedString(record['message'], 'failure message', 512),
  };
}

function parseModelRuntimeChangedEvent(raw: unknown): ModelRuntimeChangedEvent {
  const record = requiredRecord(raw, 'event');
  return {
    reason: boundedString(record['reason'], 'event reason', 64),
    state: requiredEnum(record['state'], STATES, 'event state'),
    at: requiredTimestamp(record['at'], 'event at'),
  };
}

function requireModelID(raw: string): string {
  if (!MODEL_ID.test(raw)) {
    throw new Error('A valid model ID is required.');
  }
  return raw;
}

function providerModelID(raw: unknown, description: string): string {
  if (typeof raw !== 'string' || !MODEL_ID.test(raw)) invalidResponse(description);
  return raw;
}

function requiredRecord(raw: unknown, description: string): Record<string, unknown> {
  if (raw === null || typeof raw !== 'object' || Array.isArray(raw)) {
    invalidResponse(description);
  }
  return raw as Record<string, unknown>;
}

function requiredArray(raw: unknown, description: string, maximum: number): readonly unknown[] {
  if (!Array.isArray(raw) || raw.length > maximum) invalidResponse(description);
  return raw;
}

function requiredEnum<Value extends string>(
  raw: unknown,
  values: readonly Value[],
  description: string,
): Value {
  if (typeof raw !== 'string' || !values.includes(raw as Value)) {
    invalidResponse(description);
  }
  return raw as Value;
}

function requiredBoolean(raw: unknown, description: string): boolean {
  if (typeof raw !== 'boolean') invalidResponse(description);
  return raw;
}

function boundedString(
  raw: unknown,
  description: string,
  maximumBytes: number,
  allowEmpty = false,
): string {
  if (
    typeof raw !== 'string' ||
    (!allowEmpty && raw.length === 0) ||
    new TextEncoder().encode(raw).byteLength > maximumBytes ||
    containsControl(raw)
  ) {
    invalidResponse(description);
  }
  return raw;
}

function requiredTimestamp(raw: unknown, description: string, allowEmpty = false): string {
  const value = boundedString(raw, description, 64, allowEmpty);
  if (value !== '' && (!RFC3339_TIMESTAMP.test(value) || !Number.isFinite(Date.parse(value)))) {
    invalidResponse(description);
  }
  return value;
}

function optionalStringProperty<Key extends string>(
  record: Record<string, unknown>,
  key: Key,
  description: string,
  maximumBytes: number,
): Partial<Record<Key, string>> {
  if (!Object.hasOwn(record, key)) return {};
  return { [key]: boundedString(record[key], description, maximumBytes, true) } as Record<
    Key,
    string
  >;
}

function optionalTimestampProperty<Key extends string>(
  record: Record<string, unknown>,
  key: Key,
  description: string,
): Partial<Record<Key, string>> {
  if (!Object.hasOwn(record, key)) return {};
  return { [key]: requiredTimestamp(record[key], description, true) } as Record<Key, string>;
}

function optionalNumberProperty<Key extends string>(
  record: Record<string, unknown>,
  key: Key,
  description: string,
): Partial<Record<Key, number>> {
  if (!Object.hasOwn(record, key)) return {};
  const value = record[key];
  if (typeof value !== 'number' || !Number.isFinite(value) || value < 0) {
    invalidResponse(description);
  }
  return { [key]: value } as Record<Key, number>;
}

function optionalIntegerProperty<Key extends string>(
  record: Record<string, unknown>,
  key: Key,
  description: string,
  minimum: number,
  maximum: number,
): Partial<Record<Key, number>> {
  if (!Object.hasOwn(record, key)) return {};
  const value = record[key];
  if (
    typeof value !== 'number' ||
    !Number.isSafeInteger(value) ||
    value < minimum ||
    value > maximum
  ) {
    invalidResponse(description);
  }
  return { [key]: value } as Record<Key, number>;
}

function containsControl(value: string): boolean {
  for (const character of value) {
    const code = character.codePointAt(0) ?? 0;
    if (
      (code < 0x20 && character !== '\n' && character !== '\r' && character !== '\t') ||
      code === 0x7f
    ) {
      return true;
    }
  }
  return false;
}

export function rejectModelRuntimeForbiddenFields(
  raw: unknown,
  seen = new WeakSet<object>(),
): void {
  if (raw === null || typeof raw !== 'object') return;
  if (seen.has(raw)) return;
  seen.add(raw);
  if (Array.isArray(raw)) {
    for (const value of raw) rejectModelRuntimeForbiddenFields(value, seen);
    return;
  }
  for (const [key, value] of Object.entries(raw)) {
    const normalised = key.toLocaleLowerCase('en-GB').replaceAll('_', '').replaceAll('-', '');
    if (FORBIDDEN_RESPONSE_FIELDS.has(normalised)) {
      invalidResponse(`forbidden ${key} field`);
    }
    rejectModelRuntimeForbiddenFields(value, seen);
  }
}

function invalidResponse(description: string): never {
  throw new Error(`The invalid ModelRuntime response contains ${description}.`);
}

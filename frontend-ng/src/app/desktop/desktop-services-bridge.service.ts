// SPDX-License-Identifier: EUPL-1.2

import { InjectionToken, Service, inject } from '@angular/core';
import { Events } from '@wailsio/runtime';
import { ConnectionManagerService } from '../connection-manager.service';
import type {
  DesktopRestartPolicy,
  DesktopServiceCatalogue,
  DesktopServiceDefinition,
  DesktopServiceErrorCode,
  DesktopServiceFailure,
  DesktopServiceKind,
  DesktopServiceOutput,
  DesktopServicePolicyOverride,
  DesktopServiceSnapshot,
  DesktopServicesChangedEvent,
  DesktopServicesDataSource,
  DesktopServiceState,
} from './apps/control/control-services.models';
import { SurfaceBridgeService } from './surfaces/surface-bridge.service';

const SERVICES_SERVICE = 'dappco.re/lthn/desktop/pkg/services.WailsService';
const SERVICES_CHANGED_EVENT = 'lthn:services:changed';
const MAX_OUTPUT_BYTES = 256 * 1_024;
const MAX_GRACE_PERIOD_MILLIS = 60_000;
const MAX_SERVICES = 256;
const SERVICE_ID = /^[a-z0-9][a-z0-9._-]{0,63}$/u;

export const SERVICES_METHODS = {
  catalogue: `${SERVICES_SERVICE}.Catalogue`,
  get: `${SERVICES_SERVICE}.Get`,
  start: `${SERVICES_SERVICE}.Start`,
  stop: `${SERVICES_SERVICE}.Stop`,
  restart: `${SERVICES_SERVICE}.Restart`,
  output: `${SERVICES_SERVICE}.Output`,
  setPolicy: `${SERVICES_SERVICE}.SetPolicy`,
  signal: `${SERVICES_SERVICE}.Signal`,
  kill: `${SERVICES_SERVICE}.Kill`,
} as const;

/**
 * The whole signal vocabulary. A number is not in it, and cannot be made to
 * be: the renderer chooses a name, and Go maps it at the last moment.
 */
export const SERVICE_SIGNALS = ['terminate', 'interrupt', 'hangup', 'kill'] as const;

export type DesktopServiceSignal = (typeof SERVICE_SIGNALS)[number];

function requestSignal(name: string): DesktopServiceSignal {
  if (!(SERVICE_SIGNALS as readonly string[]).includes(name)) {
    throw new Error('That is not a signal this manager sends.');
  }
  return name as DesktopServiceSignal;
}

export interface ServicesEventSource {
  on(name: string, handler: (payload: unknown) => void): () => void;
}

export const SERVICES_EVENT_SOURCE = new InjectionToken<ServicesEventSource>(
  'SERVICES_EVENT_SOURCE',
  {
    providedIn: 'root',
    factory: () => ({
      on(name, handler): () => void {
        return Events.On(name, (event) => handler(event.data));
      },
    }),
  },
);

@Service()
export class DesktopServicesBridgeService implements DesktopServicesDataSource {
  private readonly surface = inject(SurfaceBridgeService);
  private readonly connection = inject(ConnectionManagerService);
  private readonly events = inject(SERVICES_EVENT_SOURCE);

  async catalogue(): Promise<DesktopServiceCatalogue> {
    return this.read(SERVICES_METHODS.catalogue, undefined, parseCatalogue);
  }

  async get(id: string): Promise<DesktopServiceSnapshot> {
    return this.read(SERVICES_METHODS.get, [requestServiceID(id)], parseSnapshot);
  }

  async start(id: string): Promise<DesktopServiceSnapshot> {
    return this.read(SERVICES_METHODS.start, [requestServiceID(id)], parseSnapshot);
  }

  async stop(id: string): Promise<DesktopServiceSnapshot> {
    return this.read(SERVICES_METHODS.stop, [requestServiceID(id)], parseSnapshot);
  }

  async restart(id: string): Promise<DesktopServiceSnapshot> {
    return this.read(SERVICES_METHODS.restart, [requestServiceID(id)], parseSnapshot);
  }

  /**
   * Deliver a named signal. Not a decision to stop — a service that ignores
   * `hangup` stays running, and the snapshot returned says so.
   */
  async signal(id: string, name: string): Promise<DesktopServiceSnapshot> {
    return this.read(
      SERVICES_METHODS.signal,
      [{ id: requestServiceID(id), signal: requestSignal(name) }],
      parseSnapshot,
    );
  }

  /** End the process tree. Unlike a signal, this does mean stop. */
  async kill(id: string): Promise<DesktopServiceSnapshot> {
    return this.read(SERVICES_METHODS.kill, [requestServiceID(id)], parseSnapshot);
  }

  async output(id: string, limit = 64 * 1_024): Promise<DesktopServiceOutput> {
    const serviceID = requestServiceID(id);
    if (!Number.isInteger(limit) || limit <= 0 || limit > MAX_OUTPUT_BYTES) {
      throw new Error('The Services output limit is outside the allowed range.');
    }
    return this.read(SERVICES_METHODS.output, [{ id: serviceID, limit }], parseOutput);
  }

  async setPolicy(override: DesktopServicePolicyOverride): Promise<DesktopServiceSnapshot> {
    const id = requestServiceID(override.id);
    if (!RESTART_POLICIES.includes(override.restartPolicy)) {
      throw new Error('The Services restart policy is invalid.');
    }
    if (
      !Number.isInteger(override.gracePeriodMillis) ||
      override.gracePeriodMillis <= 0 ||
      override.gracePeriodMillis > MAX_GRACE_PERIOD_MILLIS
    ) {
      throw new Error('The Services grace period is outside the allowed range.');
    }
    return this.read(
      SERVICES_METHODS.setPolicy,
      [
        {
          id,
          restartPolicy: override.restartPolicy,
          gracePeriodMillis: override.gracePeriodMillis,
        },
      ],
      parseSnapshot,
    );
  }

  onChanged(handler: (event: DesktopServicesChangedEvent) => void): () => void {
    if (this.connection.offline()) return () => undefined;
    return this.events.on(SERVICES_CHANGED_EVENT, (raw) => {
      try {
        rejectForbiddenFields(raw);
        handler(parseChangedEvent(raw));
      } catch {
        // Lifecycle events are advisory invalidations. Malformed data cannot
        // enter renderer state; the next explicit refresh remains canonical.
      }
    });
  }

  private async read<T>(
    method: string,
    args: readonly unknown[] | undefined,
    parser: (raw: unknown) => T,
  ): Promise<T> {
    this.requireOnline();
    const raw = args ? await this.surface.call(method, args) : await this.surface.call(method);
    rejectForbiddenFields(raw);
    return parser(raw);
  }

  private requireOnline(): void {
    if (this.connection.offline()) {
      throw new Error('The Services live bridge is unavailable in offline demo mode.');
    }
  }
}

const SERVICE_KINDS: readonly DesktopServiceKind[] = ['service', 'app', 'process'];
const SERVICE_STATES: readonly DesktopServiceState[] = [
  'stopped',
  'starting',
  'running',
  'stopping',
  'exited',
  'failed',
];
const RESTART_POLICIES: readonly DesktopRestartPolicy[] = ['never', 'on-failure', 'always'];
const ERROR_CODES: readonly DesktopServiceErrorCode[] = [
  'services_unavailable',
  'catalogue_invalid',
  'definition_not_found',
  'definition_invalid',
  'definition_conflict',
  'operation_in_progress',
  'working_directory_unsupported',
  'running_limit_reached',
  'process_start_failed',
  'process_lookup_failed',
  'process_stop_failed',
  'restart_budget_exhausted',
  'shutdown_incomplete',
];
const FORBIDDEN_RESPONSE_FIELDS = new Set([
  'command',
  'arguments',
  'environment',
  'workingdirectory',
  'root',
  'provider',
  'credential',
  'credentials',
  'token',
  'secret',
  'key',
  'encryptionkey',
  'absolutepath',
]);

function requestServiceID(id: string): string {
  if (!SERVICE_ID.test(id)) {
    throw new Error('A valid service ID is required.');
  }
  return id;
}

function parseCatalogue(raw: unknown): DesktopServiceCatalogue {
  const record = requiredRecord(raw, 'catalogue');
  const services = requiredArray(record['services'], 'catalogue services', MAX_SERVICES).map(
    parseSnapshot,
  );
  const ids = new Set<string>();
  for (const service of services) {
    if (ids.has(service.definition.id)) invalidResponse('duplicate service ID');
    ids.add(service.definition.id);
  }
  return {
    services,
    refreshedAt: requiredTimestamp(record['refreshedAt'], 'catalogue refreshedAt'),
  };
}

function parseSnapshot(raw: unknown): DesktopServiceSnapshot {
  const record = requiredRecord(raw, 'snapshot');
  const lastError = record['lastError'] === null ? null : parseFailure(record['lastError']);
  return {
    definition: parseDefinition(record['definition']),
    state: requiredEnum(record['state'], SERVICE_STATES, 'snapshot state'),
    desired: requiredBoolean(record['desired'], 'snapshot desired'),
    processId: boundedString(record['processId'], 'snapshot processId', 128, true),
    pid: requiredInteger(record['pid'], 'snapshot PID', 0, 2_147_483_647),
    startedAt: requiredTimestamp(record['startedAt'], 'snapshot startedAt', true),
    stoppedAt: requiredTimestamp(record['stoppedAt'], 'snapshot stoppedAt', true),
    exitCode: requiredInteger(
      record['exitCode'],
      'snapshot exitCode',
      -2_147_483_648,
      2_147_483_647,
    ),
    restartCount: requiredInteger(
      record['restartCount'],
      'snapshot restartCount',
      0,
      Number.MAX_SAFE_INTEGER,
    ),
    lastError,
  };
}

function parseDefinition(raw: unknown): DesktopServiceDefinition {
  const record = requiredRecord(raw, 'definition');
  return {
    id: providerServiceID(record['id'], 'definition ID'),
    displayName: boundedString(record['displayName'], 'definition displayName', 256),
    description: boundedString(record['description'], 'definition description', 2_048),
    kind: requiredEnum(record['kind'], SERVICE_KINDS, 'definition kind'),
    restartPolicy: requiredEnum(
      record['restartPolicy'],
      RESTART_POLICIES,
      'definition restartPolicy',
    ),
    gracePeriodMillis: requiredInteger(
      record['gracePeriodMillis'],
      'definition gracePeriodMillis',
      1,
      MAX_GRACE_PERIOD_MILLIS,
    ),
    owner: providerServiceID(record['owner'], 'definition owner'),
  };
}

function parseFailure(raw: unknown): DesktopServiceFailure {
  const record = requiredRecord(raw, 'failure');
  return {
    code: requiredEnum(record['code'], ERROR_CODES, 'failure code'),
    message: boundedString(record['message'], 'failure message', 2_048),
  };
}

function parseOutput(raw: unknown): DesktopServiceOutput {
  const record = requiredRecord(raw, 'output');
  const output = boundedString(record['output'], 'output content', MAX_OUTPUT_BYTES, true);
  if (new TextEncoder().encode(output).byteLength > MAX_OUTPUT_BYTES) {
    invalidResponse('output content');
  }
  return {
    id: providerServiceID(record['id'], 'output ID'),
    processId: boundedString(record['processId'], 'output processId', 128),
    generation: requiredInteger(
      record['generation'],
      'output generation',
      0,
      Number.MAX_SAFE_INTEGER,
    ),
    output,
    truncated: requiredBoolean(record['truncated'], 'output truncated'),
    observedAt: requiredTimestamp(record['observedAt'], 'output observedAt'),
  };
}

function parseChangedEvent(raw: unknown): DesktopServicesChangedEvent {
  const record = requiredRecord(raw, 'event');
  const errorCode =
    record['errorCode'] === ''
      ? ''
      : requiredEnum(record['errorCode'], ERROR_CODES, 'event errorCode');
  return {
    id: providerServiceID(record['id'], 'event ID'),
    operation: boundedString(record['operation'], 'event operation', 64),
    previous: requiredEnum(record['previous'], SERVICE_STATES, 'event previous state'),
    state: requiredEnum(record['state'], SERVICE_STATES, 'event state'),
    desired: requiredBoolean(record['desired'], 'event desired'),
    processId: boundedString(record['processId'], 'event processId', 128, true),
    errorCode,
    at: requiredTimestamp(record['at'], 'event at'),
  };
}

function providerServiceID(raw: unknown, description: string): string {
  if (typeof raw !== 'string' || !SERVICE_ID.test(raw)) {
    invalidResponse(description);
  }
  return raw;
}

function requiredRecord(raw: unknown, description: string): Record<string, unknown> {
  if (raw === null || typeof raw !== 'object' || Array.isArray(raw)) {
    invalidResponse(description);
  }
  return raw as Record<string, unknown>;
}

function requiredArray(raw: unknown, description: string, maximum: number): readonly unknown[] {
  if (!Array.isArray(raw) || raw.length > maximum) {
    invalidResponse(description);
  }
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

function requiredInteger(
  raw: unknown,
  description: string,
  minimum: number,
  maximum: number,
): number {
  if (typeof raw !== 'number' || !Number.isSafeInteger(raw) || raw < minimum || raw > maximum) {
    invalidResponse(description);
  }
  return raw;
}

function boundedString(
  raw: unknown,
  description: string,
  maximum: number,
  allowEmpty = false,
): string {
  if (
    typeof raw !== 'string' ||
    (!allowEmpty && raw.length === 0) ||
    raw.length > maximum ||
    containsControl(raw)
  ) {
    invalidResponse(description);
  }
  return raw;
}

function requiredTimestamp(raw: unknown, description: string, allowEmpty = false): string {
  const value = boundedString(raw, description, 64, allowEmpty);
  if (value !== '' && !Number.isFinite(Date.parse(value))) {
    invalidResponse(description);
  }
  return value;
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

function rejectForbiddenFields(raw: unknown, seen = new WeakSet<object>()): void {
  if (raw === null || typeof raw !== 'object') return;
  if (seen.has(raw)) return;
  seen.add(raw);
  if (Array.isArray(raw)) {
    for (const value of raw) rejectForbiddenFields(value, seen);
    return;
  }
  for (const [key, value] of Object.entries(raw)) {
    const normalised = key.toLocaleLowerCase('en-GB').replaceAll('_', '').replaceAll('-', '');
    if (FORBIDDEN_RESPONSE_FIELDS.has(normalised)) {
      invalidResponse(`forbidden ${key} field`);
    }
    rejectForbiddenFields(value, seen);
  }
}

function invalidResponse(description: string): never {
  throw new Error(`The invalid Services response contains ${description}.`);
}

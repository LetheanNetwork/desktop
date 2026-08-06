// SPDX-License-Identifier: EUPL-1.2

export type DesktopServiceKind = 'service' | 'app' | 'process';

export type DesktopServiceState =
  'stopped' | 'starting' | 'running' | 'stopping' | 'exited' | 'failed';

export type DesktopRestartPolicy = 'never' | 'on-failure' | 'always';

export type DesktopServiceErrorCode =
  | 'services_unavailable'
  | 'catalogue_invalid'
  | 'definition_not_found'
  | 'definition_invalid'
  | 'definition_conflict'
  | 'operation_in_progress'
  | 'working_directory_unsupported'
  | 'running_limit_reached'
  | 'process_start_failed'
  | 'process_lookup_failed'
  | 'process_stop_failed'
  | 'restart_budget_exhausted'
  | 'shutdown_incomplete'
  // The signal codes. Mirrors go/pkg/services/types.go, which grew these when
  // named signals landed; a code the renderer does not know is rejected as an
  // invalid response, so an incomplete mirror turns a precise failure into a
  // whole catalogue that will not parse.
  | 'signal_unknown'
  | 'signal_unsupported'
  | 'service_not_running'
  | 'process_signal_failed';

/**
 * The whole signal vocabulary, declared once for this side of the wire.
 *
 * It lives beside the other wire models rather than in the bridge because both
 * ends need it: the renderer chooses a name from here, and the bridge refuses
 * anything that is not in here. A number is not in it and cannot be made to be.
 */
export const SERVICE_SIGNALS = ['terminate', 'interrupt', 'hangup', 'kill'] as const;

export type DesktopServiceSignal = (typeof SERVICE_SIGNALS)[number];

export interface DesktopServiceDefinition {
  readonly id: string;
  readonly displayName: string;
  readonly description: string;
  readonly kind: DesktopServiceKind;
  readonly restartPolicy: DesktopRestartPolicy;
  readonly gracePeriodMillis: number;
  readonly owner: string;
}

export interface DesktopServiceFailure {
  readonly code: DesktopServiceErrorCode;
  readonly message: string;
}

export interface DesktopServiceSnapshot {
  readonly definition: DesktopServiceDefinition;
  readonly state: DesktopServiceState;
  readonly desired: boolean;
  readonly processId: string;
  readonly pid: number;
  readonly startedAt: string;
  readonly stoppedAt: string;
  readonly exitCode: number;
  readonly restartCount: number;
  readonly lastError: DesktopServiceFailure | null;
}

export interface DesktopServiceCatalogue {
  readonly services: readonly DesktopServiceSnapshot[];
  readonly refreshedAt: string;
}

export interface DesktopServiceOutput {
  readonly id: string;
  readonly processId: string;
  readonly generation: number;
  readonly output: string;
  readonly truncated: boolean;
  readonly observedAt: string;
}

export interface DesktopServicesChangedEvent {
  readonly id: string;
  readonly operation: string;
  readonly previous: DesktopServiceState;
  readonly state: DesktopServiceState;
  readonly desired: boolean;
  readonly processId: string;
  readonly errorCode: DesktopServiceErrorCode | '';
  readonly at: string;
}

export interface DesktopServicePolicyOverride {
  readonly id: string;
  readonly restartPolicy: DesktopRestartPolicy;
  readonly gracePeriodMillis: number;
}

export interface DesktopServicesDataSource {
  catalogue(): Promise<DesktopServiceCatalogue>;
  get(id: string): Promise<DesktopServiceSnapshot>;
  start(id: string): Promise<DesktopServiceSnapshot>;
  stop(id: string): Promise<DesktopServiceSnapshot>;
  restart(id: string): Promise<DesktopServiceSnapshot>;
  signal(id: string, name: DesktopServiceSignal): Promise<DesktopServiceSnapshot>;
  kill(id: string): Promise<DesktopServiceSnapshot>;
  output(id: string, limit?: number): Promise<DesktopServiceOutput>;
  setPolicy(override: DesktopServicePolicyOverride): Promise<DesktopServiceSnapshot>;
  onChanged(handler: (event: DesktopServicesChangedEvent) => void): () => void;
}

export type ControlServiceIntent =
  | { readonly kind: 'start'; readonly id: string }
  | { readonly kind: 'stop'; readonly id: string }
  | { readonly kind: 'restart'; readonly id: string }
  | { readonly kind: 'output'; readonly id: string }
  // The unusual responses. Kept off the main row deliberately — a kill
  // sitting beside stop at equal weight invites the click that costs
  // somebody their unsaved state.
  //
  // The signal is a vocabulary name rather than a string, so a name the bridge
  // would refuse cannot be built here at all. `confirmed` follows the Files
  // idiom for a destructive operation: the intent carries the answer, and the
  // handler refuses it when the answer is not yes.
  | { readonly kind: 'signal'; readonly id: string; readonly signal: DesktopServiceSignal }
  | { readonly kind: 'kill'; readonly id: string; readonly confirmed: boolean };

export const SERVICES_DEMO_SOURCE = 'Lethean demo fixture';

export function createDemoServiceCatalogue(): DesktopServiceCatalogue {
  return {
    refreshedAt: 'demo',
    services: [
      demoService({
        id: 'api',
        displayName: 'Lethean API',
        description: 'Lethean demo fixture for the local API lifecycle.',
        kind: 'service',
        state: 'running',
        desired: true,
        processId: 'demo-api-1',
        pid: 4_821,
        startedAt: '2026-07-27T09:00:00Z',
      }),
      demoService({
        id: 'runner',
        displayName: 'Local model runner',
        description: 'Lethean demo fixture for an optional model runtime.',
        kind: 'service',
        state: 'stopped',
      }),
      demoService({
        id: 'indexer',
        displayName: 'Workspace indexer',
        description: 'Lethean demo fixture for an optional workspace process.',
        kind: 'process',
        state: 'failed',
        lastError: {
          code: 'process_start_failed',
          message: 'Lethean demo fixture: the indexer exited during demonstration.',
        },
      }),
    ],
  };
}

function demoService(
  input: Pick<DesktopServiceDefinition, 'id' | 'displayName' | 'description' | 'kind'> & {
    readonly state: DesktopServiceState;
    readonly desired?: boolean;
    readonly processId?: string;
    readonly pid?: number;
    readonly startedAt?: string;
    readonly lastError?: DesktopServiceFailure;
  },
): DesktopServiceSnapshot {
  return {
    definition: {
      id: input.id,
      displayName: input.displayName,
      description: input.description,
      kind: input.kind,
      restartPolicy: 'never',
      gracePeriodMillis: 5_000,
      owner: 'lethean',
    },
    state: input.state,
    desired: input.desired ?? false,
    processId: input.processId ?? '',
    pid: input.pid ?? 0,
    startedAt: input.startedAt ?? '',
    stoppedAt: '',
    exitCode: input.state === 'failed' ? 1 : 0,
    restartCount: 0,
    lastError: input.lastError ? { ...input.lastError } : null,
  };
}

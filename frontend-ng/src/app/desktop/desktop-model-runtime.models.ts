// SPDX-License-Identifier: EUPL-1.2

export type ModelRuntimeState =
  | 'unavailable'
  | 'stopped'
  | 'starting'
  | 'model-less'
  | 'loading'
  | 'ready'
  | 'degraded'
  | 'failed'
  | 'stopping';

export type ModelRuntimeErrorCode =
  | 'runtime_unavailable'
  | 'binary_missing'
  | 'runtime_stopped'
  | 'runtime_start_failed'
  | 'runtime_not_ready'
  | 'catalogue_unavailable'
  | 'model_not_found'
  | 'model_not_loadable'
  | 'model_load_failed'
  | 'model_unload_failed'
  | 'admin_unauthorised'
  | 'operation_in_progress'
  | 'response_invalid'
  | 'response_too_large'
  | 'request_timeout'
  | 'runtime_stop_failed';

export interface ModelRuntimeFailure {
  readonly code: ModelRuntimeErrorCode;
  readonly message: string;
}

export interface ModelRuntimeModel {
  readonly id: string;
  readonly displayName: string;
  readonly format: string;
  readonly loadable: boolean;
  readonly loaded: boolean;
  readonly runtime?: string;
  readonly contextLength?: number;
  readonly loadedAt?: string;
}

export interface ModelRuntimeMetrics {
  readonly promptTokensPerSecond?: number;
  readonly decodeTokensPerSecond?: number;
  readonly activeMemoryBytes?: number;
  readonly peakMemoryBytes?: number;
  readonly kvCacheBytes?: number;
  readonly uptimeSeconds?: number;
}

export interface ModelRuntimeSample {
  readonly state: ModelRuntimeState;
  readonly at: string;
  readonly promptTokensPerSecond?: number;
  readonly decodeTokensPerSecond?: number;
  readonly activeMemoryBytes?: number;
  readonly peakMemoryBytes?: number;
  readonly kvCacheBytes?: number;
}

export interface ModelRuntimeSnapshot {
  readonly state: ModelRuntimeState;
  readonly desired: boolean;
  readonly activeModelId: string;
  readonly models: readonly ModelRuntimeModel[];
  readonly metrics: ModelRuntimeMetrics;
  readonly history: readonly ModelRuntimeSample[];
  readonly refreshedAt: string;
  readonly lastHealthyAt: string;
  readonly stale: boolean;
  readonly lastError: ModelRuntimeFailure | null;
}

export interface ModelRuntimeChangedEvent {
  readonly reason: string;
  readonly state: ModelRuntimeState;
  readonly at: string;
}

export type ModelRuntimeOperation =
  | { readonly kind: 'start' }
  | { readonly kind: 'load'; readonly modelId: string }
  | { readonly kind: 'unload' }
  | { readonly kind: 'restart' }
  | { readonly kind: 'stop' };

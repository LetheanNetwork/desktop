// SPDX-License-Identifier: EUPL-1.2

import type {
  ModelRuntimeErrorCode,
  ModelRuntimeFailure,
  ModelRuntimeMetrics,
  ModelRuntimeModel,
  ModelRuntimeSample,
  ModelRuntimeSnapshot,
  ModelRuntimeState,
} from './desktop-model-runtime.models';

export const MODEL_RUNTIME_DEMO_SOURCE = 'Lethean demo fixture · Model runtime';
export const MODEL_RUNTIME_DEMO_EPOCH = '2026-07-27T13:00:00.000Z';

const DEMO_MODELS: readonly ModelRuntimeModel[] = [
  {
    id: 'model-0000000000000001',
    displayName: 'gemma-4-e2b',
    format: 'snapshot',
    loadable: true,
    loaded: false,
  },
  {
    id: 'model-0000000000000002',
    displayName: 'qwen-2.5-coder',
    format: 'snapshot',
    loadable: true,
    loaded: false,
  },
  {
    id: 'model-0000000000000003',
    displayName: 'mistral-small',
    format: 'snapshot',
    loadable: false,
    loaded: false,
  },
];

const READY_METRICS: ModelRuntimeMetrics = {
  promptTokensPerSecond: 41.8,
  decodeTokensPerSecond: 18.2,
  activeMemoryBytes: 18_400_000_000,
  peakMemoryBytes: 19_200_000_000,
  kvCacheBytes: 2_300_000_000,
  uptimeSeconds: 360,
};

export function createDemoModelRuntimeSnapshot(
  state: ModelRuntimeState = 'stopped',
): ModelRuntimeSnapshot {
  const ready = state === 'ready' || state === 'degraded';
  const activeModelId = ready ? DEMO_MODELS[0].id : '';
  const models = DEMO_MODELS.map((model) =>
    model.id === activeModelId
      ? {
          ...model,
          loaded: true,
          runtime: 'metal',
          contextLength: 8_192,
          loadedAt: MODEL_RUNTIME_DEMO_EPOCH,
        }
      : { ...model },
  );
  const metrics = ready ? { ...READY_METRICS } : {};
  const history: readonly ModelRuntimeSample[] = ready
    ? [
        {
          state: 'ready',
          at: MODEL_RUNTIME_DEMO_EPOCH,
          promptTokensPerSecond: READY_METRICS.promptTokensPerSecond,
          decodeTokensPerSecond: READY_METRICS.decodeTokensPerSecond,
          activeMemoryBytes: READY_METRICS.activeMemoryBytes,
          peakMemoryBytes: READY_METRICS.peakMemoryBytes,
          kvCacheBytes: READY_METRICS.kvCacheBytes,
        },
      ]
    : [];
  return {
    state,
    desired: !['unavailable', 'stopped'].includes(state),
    activeModelId,
    models,
    metrics,
    history,
    refreshedAt: MODEL_RUNTIME_DEMO_EPOCH,
    lastHealthyAt: ready ? MODEL_RUNTIME_DEMO_EPOCH : '',
    stale: state === 'degraded',
    lastError: demoFailure(state),
  };
}

function demoFailure(state: ModelRuntimeState): ModelRuntimeFailure | null {
  const failures: Partial<
    Record<ModelRuntimeState, { readonly code: ModelRuntimeErrorCode; readonly message: string }>
  > = {
    unavailable: {
      code: 'runtime_unavailable',
      message: 'The demo model runtime is unavailable.',
    },
    degraded: {
      code: 'runtime_not_ready',
      message: 'The demo model runtime is temporarily unavailable.',
    },
    failed: {
      code: 'model_load_failed',
      message: 'The demo model operation could not be completed.',
    },
  };
  const failure = failures[state];
  return failure ? { ...failure } : null;
}

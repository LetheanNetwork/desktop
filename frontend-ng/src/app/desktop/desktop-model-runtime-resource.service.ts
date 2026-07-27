// SPDX-License-Identifier: EUPL-1.2

import { Injectable, OnDestroy, Signal, inject, signal } from '@angular/core';
import { ConnectionManagerService } from '../connection-manager.service';
import {
  beginDesktopDataRefresh,
  createConnectedResource,
  createDemoResource,
  rejectDesktopData,
  resolveDesktopData,
  type DesktopDataResource,
} from './desktop-data-resource';
import { DesktopModelRuntimeBridgeService } from './desktop-model-runtime-bridge.service';
import {
  MODEL_RUNTIME_DEMO_EPOCH,
  MODEL_RUNTIME_DEMO_SOURCE,
  createDemoModelRuntimeSnapshot,
} from './desktop-model-runtime-demo.data';
import type {
  ModelRuntimeChangedEvent,
  ModelRuntimeMetrics,
  ModelRuntimeModel,
  ModelRuntimeOperation,
  ModelRuntimeSample,
  ModelRuntimeSnapshot,
  ModelRuntimeState,
} from './desktop-model-runtime.models';

const MODEL_RUNTIME_LIVE_SOURCE = 'Local LEM runtime';
const MODEL_RUNTIME_UNAVAILABLE = 'Live model-runtime data is unavailable.';
const MODEL_RUNTIME_REFRESH_MS = 30_000;

@Injectable({ providedIn: 'root' })
export class DesktopModelRuntimeResource implements OnDestroy {
  private readonly bridge = inject(DesktopModelRuntimeBridgeService);
  private readonly connection = inject(ConnectionManagerService);
  private demoSnapshot = createDemoModelRuntimeSnapshot();
  private readonly resourceState = signal<DesktopDataResource<ModelRuntimeSnapshot>>(
    this.connection.offline()
      ? createDemoResource(this.demoSnapshot, MODEL_RUNTIME_DEMO_SOURCE)
      : createConnectedResource<ModelRuntimeSnapshot>(MODEL_RUNTIME_LIVE_SOURCE),
  );
  private readonly pendingState = signal<ModelRuntimeOperation | null>(null);

  readonly resource: Signal<DesktopDataResource<ModelRuntimeSnapshot>> =
    this.resourceState.asReadonly();
  readonly pending: Signal<ModelRuntimeOperation | null> = this.pendingState.asReadonly();

  private consumers = 0;
  private generation = 0;
  private demoSequence = 0;
  private eventOff: (() => void) | null = null;
  private pollHandle: ReturnType<typeof setInterval> | null = null;
  private refreshPromise: Promise<void> | null = null;
  private refreshScheduled = false;
  private destroyed = false;

  connect(): () => void {
    if (this.destroyed) return () => undefined;
    this.consumers++;
    if (this.consumers === 1 && !this.connection.offline()) {
      this.generation++;
      this.eventOff = this.bridge.onChanged((event) => this.onChanged(event));
      this.pollHandle = setInterval(() => this.scheduleRefresh(), MODEL_RUNTIME_REFRESH_MS);
      void this.refresh();
    }

    let connected = true;
    return () => {
      if (!connected) return;
      connected = false;
      this.consumers = Math.max(0, this.consumers - 1);
      if (this.consumers === 0) this.teardownConnectedResources();
    };
  }

  async refresh(): Promise<void> {
    if (this.destroyed || this.connection.offline()) return;
    if (this.refreshPromise) return this.refreshPromise;
    if (this.pendingState() !== null) return;

    const generation = this.generation;
    const current = this.resourceState();
    if (current.mode !== 'connected') return;
    const refreshing = current.refreshing
      ? current
      : beginDesktopDataRefresh(current, Date.now(), MODEL_RUNTIME_REFRESH_MS);
    this.resourceState.set(refreshing);

    let task: Promise<void>;
    task = Promise.resolve()
      .then(() => this.bridge.snapshot())
      .then((snapshot) => {
        if (!this.isCurrent(generation)) return;
        const resource = this.resourceState();
        if (resource.mode !== 'connected' || !resource.refreshing) return;
        this.resourceState.set(
          resolveDesktopData(resource, snapshot, 'live', MODEL_RUNTIME_LIVE_SOURCE, Date.now()),
        );
      })
      .catch(() => {
        if (!this.isCurrent(generation)) return;
        const resource = this.resourceState();
        if (resource.mode !== 'connected' || !resource.refreshing) return;
        this.resourceState.set(rejectDesktopData(resource, MODEL_RUNTIME_UNAVAILABLE));
      })
      .finally(() => {
        if (this.refreshPromise === task) this.refreshPromise = null;
      });
    this.refreshPromise = task;
    return task;
  }

  async perform(operation: ModelRuntimeOperation): Promise<void> {
    if (this.destroyed) return;
    if (this.pendingState() !== null) {
      throw new Error('Another model-runtime operation is already running.');
    }
    this.pendingState.set(operation);
    try {
      if (this.connection.offline()) {
        await this.performDemo(operation);
        return;
      }
      await this.performConnected(operation);
    } finally {
      this.pendingState.set(null);
    }
  }

  ngOnDestroy(): void {
    this.destroyed = true;
    this.consumers = 0;
    this.teardownConnectedResources();
  }

  private onChanged(_event: ModelRuntimeChangedEvent): void {
    this.scheduleRefresh();
  }

  private scheduleRefresh(): void {
    if (this.refreshScheduled || this.destroyed || this.consumers === 0) return;
    this.refreshScheduled = true;
    const generation = this.generation;
    queueMicrotask(() => {
      this.refreshScheduled = false;
      if (this.isCurrent(generation)) void this.refresh();
    });
  }

  private async performConnected(operation: ModelRuntimeOperation): Promise<void> {
    const activeRefresh = this.refreshPromise;
    if (activeRefresh) await activeRefresh;
    if (this.destroyed) return;

    const generation = this.generation;
    const current = this.resourceState();
    if (current.mode !== 'connected') {
      this.resourceState.set(createConnectedResource(MODEL_RUNTIME_LIVE_SOURCE));
    }
    const before = this.resourceState();
    const refreshing = before.refreshing
      ? before
      : beginDesktopDataRefresh(before, Date.now(), MODEL_RUNTIME_REFRESH_MS);
    this.resourceState.set(refreshing);
    try {
      const snapshot = await this.callOperation(operation);
      if (!this.isCurrent(generation)) return;
      const resource = this.resourceState();
      if (resource.mode === 'connected' && resource.refreshing) {
        this.resourceState.set(
          resolveDesktopData(resource, snapshot, 'live', MODEL_RUNTIME_LIVE_SOURCE, Date.now()),
        );
      }
    } catch (error) {
      if (this.isCurrent(generation)) {
        const resource = this.resourceState();
        if (resource.mode === 'connected' && resource.refreshing) {
          this.resourceState.set(rejectDesktopData(resource, operationError(error)));
        }
      }
      throw error;
    }
  }

  private callOperation(operation: ModelRuntimeOperation): Promise<ModelRuntimeSnapshot> {
    switch (operation.kind) {
      case 'start':
        return this.bridge.start();
      case 'load':
        return this.bridge.load(operation.modelId);
      case 'unload':
        return this.bridge.unload();
      case 'restart':
        return this.bridge.restart();
      case 'stop':
        return this.bridge.stop();
    }
  }

  private async performDemo(operation: ModelRuntimeOperation): Promise<void> {
    switch (operation.kind) {
      case 'start':
        this.setDemoState('starting');
        await Promise.resolve();
        this.setDemoState('model-less');
        return;
      case 'load':
        if (
          !this.demoSnapshot.models.some(
            (model) => model.id === operation.modelId && model.loadable,
          )
        ) {
          throw new Error('The selected demo model is unavailable.');
        }
        this.setDemoState('loading');
        await Promise.resolve();
        this.setDemoState('ready', operation.modelId);
        return;
      case 'unload':
        this.setDemoState('model-less');
        return;
      case 'restart':
        this.setDemoState('starting');
        await Promise.resolve();
        this.setDemoState('model-less');
        return;
      case 'stop':
        this.setDemoState('stopping');
        await Promise.resolve();
        this.setDemoState('stopped');
        return;
    }
  }

  private setDemoState(state: ModelRuntimeState, activeModelId = ''): void {
    const at = this.nextDemoTimestamp();
    const ready = state === 'ready';
    const models = this.demoSnapshot.models.map((model) =>
      demoModelState(model, ready && model.id === activeModelId, at),
    );
    const metrics: ModelRuntimeMetrics = ready
      ? {
          promptTokensPerSecond: 41.8,
          decodeTokensPerSecond: 18.2,
          activeMemoryBytes: 18_400_000_000,
          peakMemoryBytes: 19_200_000_000,
          kvCacheBytes: 2_300_000_000,
          uptimeSeconds: 360 + this.demoSequence * 5,
        }
      : {};
    const history = ready
      ? appendDemoSample(this.demoSnapshot.history, state, metrics, at)
      : this.demoSnapshot.history.map((sample) => ({ ...sample }));
    this.demoSnapshot = {
      state,
      desired: state !== 'stopped' && state !== 'unavailable',
      activeModelId: ready ? activeModelId : '',
      models,
      metrics,
      history,
      refreshedAt: at,
      lastHealthyAt: ready ? at : this.demoSnapshot.lastHealthyAt,
      stale: false,
      lastError: null,
    };
    this.resourceState.set(createDemoResource(this.demoSnapshot, MODEL_RUNTIME_DEMO_SOURCE));
  }

  private nextDemoTimestamp(): string {
    this.demoSequence++;
    return new Date(Date.parse(MODEL_RUNTIME_DEMO_EPOCH) + this.demoSequence * 5_000).toISOString();
  }

  private isCurrent(generation: number): boolean {
    return !this.destroyed && generation === this.generation;
  }

  private teardownConnectedResources(): void {
    this.generation++;
    this.eventOff?.();
    this.eventOff = null;
    if (this.pollHandle !== null) clearInterval(this.pollHandle);
    this.pollHandle = null;
    this.refreshScheduled = false;
    this.refreshPromise = null;
    const resource = this.resourceState();
    if (resource.mode === 'connected' && resource.refreshing) {
      this.resourceState.set(rejectDesktopData(resource, MODEL_RUNTIME_UNAVAILABLE));
    }
  }
}

function demoModelState(
  model: ModelRuntimeModel,
  loaded: boolean,
  loadedAt: string,
): ModelRuntimeModel {
  if (!loaded) {
    return {
      id: model.id,
      displayName: model.displayName,
      format: model.format,
      loadable: model.loadable,
      loaded: false,
    };
  }
  return {
    ...model,
    loaded: true,
    runtime: 'metal',
    contextLength: 8_192,
    loadedAt,
  };
}

function appendDemoSample(
  history: readonly ModelRuntimeSample[],
  state: ModelRuntimeState,
  metrics: ModelRuntimeMetrics,
  at: string,
): readonly ModelRuntimeSample[] {
  return [
    ...history.map((sample) => ({ ...sample })),
    {
      state,
      at,
      ...metrics,
    },
  ].slice(-720);
}

function operationError(error: unknown): string {
  return error instanceof Error && error.message
    ? error.message
    : 'The model-runtime operation could not be completed.';
}

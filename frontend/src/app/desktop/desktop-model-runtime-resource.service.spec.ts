// SPDX-License-Identifier: EUPL-1.2

import { signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { ConnectionManagerService } from '../connection-manager.service';
import { DesktopModelRuntimeBridgeService } from './desktop-model-runtime-bridge.service';
import { createDemoModelRuntimeSnapshot } from './desktop-model-runtime-demo.data';
import type {
  ModelRuntimeChangedEvent,
  ModelRuntimeSnapshot,
} from './desktop-model-runtime.models';
import { DesktopModelRuntimeResource } from './desktop-model-runtime-resource.service';

function deferred<Value>() {
  let resolve!: (value: Value) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<Value>((accept, decline) => {
    resolve = accept;
    reject = decline;
  });
  return { promise, resolve, reject };
}

describe('DesktopModelRuntimeResource', () => {
  const offline = signal(false);
  const bridge = {
    snapshot: vi.fn<() => Promise<ModelRuntimeSnapshot>>(),
    start: vi.fn<() => Promise<ModelRuntimeSnapshot>>(),
    load: vi.fn<(modelId: string) => Promise<ModelRuntimeSnapshot>>(),
    unload: vi.fn<() => Promise<ModelRuntimeSnapshot>>(),
    restart: vi.fn<() => Promise<ModelRuntimeSnapshot>>(),
    stop: vi.fn<() => Promise<ModelRuntimeSnapshot>>(),
    onChanged: vi.fn<(handler: (event: ModelRuntimeChangedEvent) => void) => () => void>(),
  };
  let changed: ((event: ModelRuntimeChangedEvent) => void) | null;
  let off: () => void;
  let resource: DesktopModelRuntimeResource;

  beforeEach(() => {
    vi.useFakeTimers();
    vi.clearAllMocks();
    offline.set(false);
    changed = null;
    off = vi.fn();
    bridge.onChanged.mockImplementation((handler) => {
      changed = handler;
      return off;
    });
    bridge.snapshot.mockResolvedValue(createDemoModelRuntimeSnapshot());
    bridge.start.mockResolvedValue(createDemoModelRuntimeSnapshot('model-less'));
    bridge.load.mockResolvedValue(createDemoModelRuntimeSnapshot('ready'));
    bridge.unload.mockResolvedValue(createDemoModelRuntimeSnapshot('model-less'));
    bridge.restart.mockResolvedValue(createDemoModelRuntimeSnapshot('model-less'));
    bridge.stop.mockResolvedValue(createDemoModelRuntimeSnapshot('stopped'));
    TestBed.configureTestingModule({
      providers: [
        DesktopModelRuntimeResource,
        { provide: DesktopModelRuntimeBridgeService, useValue: bridge },
        {
          provide: ConnectionManagerService,
          useValue: { offline: offline.asReadonly() },
        },
      ],
    });
    resource = TestBed.inject(DesktopModelRuntimeResource);
  });

  afterEach(() => {
    TestBed.resetTestingModule();
    vi.useRealTimers();
  });

  it('shares one event listener and one fallback timer across consumers', async () => {
    const disconnectFirst = resource.connect();
    const disconnectSecond = resource.connect();
    await resource.refresh();

    expect(bridge.onChanged).toHaveBeenCalledTimes(1);
    expect(bridge.snapshot).toHaveBeenCalledTimes(1);
    expect(vi.getTimerCount()).toBe(1);

    disconnectFirst();
    expect(off).not.toHaveBeenCalled();
    expect(vi.getTimerCount()).toBe(1);

    disconnectSecond();
    expect(off).toHaveBeenCalledTimes(1);
    expect(vi.getTimerCount()).toBe(0);
  });

  it('coalesces an event burst into one canonical snapshot refresh', async () => {
    const disconnect = resource.connect();
    await resource.refresh();
    bridge.snapshot.mockClear();
    const event: ModelRuntimeChangedEvent = {
      reason: 'sample',
      state: 'ready',
      at: '2026-07-27T13:00:05Z',
    };

    for (let index = 0; index < 5; index++) changed?.(event);
    await vi.runAllTicks();
    await resource.refresh();

    expect(bridge.snapshot).toHaveBeenCalledTimes(1);
    disconnect();
  });

  it('preserves last-good data as stale after a refresh failure', async () => {
    const snapshot = createDemoModelRuntimeSnapshot('ready');
    bridge.snapshot.mockResolvedValueOnce(snapshot);
    const disconnect = resource.connect();
    await resource.refresh();
    expect(resource.resource().value).toBe(snapshot);

    bridge.snapshot.mockRejectedValueOnce(new Error('LEM unavailable'));
    await resource.refresh();

    expect(resource.resource()).toMatchObject({
      state: 'stale',
      value: snapshot,
      refreshing: false,
      canRetry: true,
    });
    disconnect();
  });

  it('tears down fully and ignores a late refresh result', async () => {
    const pending = deferred<ModelRuntimeSnapshot>();
    bridge.snapshot.mockReturnValueOnce(pending.promise);
    const disconnect = resource.connect();

    disconnect();
    const afterDispose = resource.resource();
    pending.resolve(createDemoModelRuntimeSnapshot('ready'));
    await pending.promise;
    await Promise.resolve();

    expect(resource.resource()).toBe(afterDispose);
    expect(off).toHaveBeenCalledTimes(1);
    expect(vi.getTimerCount()).toBe(0);
  });

  it('simulates deterministic demo operations without touching the bridge', async () => {
    TestBed.resetTestingModule();
    offline.set(true);
    TestBed.configureTestingModule({
      providers: [
        DesktopModelRuntimeResource,
        { provide: DesktopModelRuntimeBridgeService, useValue: bridge },
        {
          provide: ConnectionManagerService,
          useValue: { offline: offline.asReadonly() },
        },
      ],
    });
    resource = TestBed.inject(DesktopModelRuntimeResource);
    const first = resource.resource().value;

    await resource.perform({ kind: 'start' });
    expect(resource.resource().value).toMatchObject({ state: 'model-less' });
    await resource.perform({ kind: 'load', modelId: 'model-0000000000000001' });
    expect(resource.resource().value).toMatchObject({
      state: 'ready',
      activeModelId: 'model-0000000000000001',
    });
    await resource.perform({ kind: 'unload' });
    expect(resource.resource().value).toMatchObject({
      state: 'model-less',
      activeModelId: '',
    });
    await resource.perform({ kind: 'stop' });
    expect(resource.resource().value).toMatchObject({ state: 'stopped' });

    expect(resource.resource().value).not.toBe(first);
    expect(bridge.snapshot).not.toHaveBeenCalled();
    expect(bridge.start).not.toHaveBeenCalled();
    expect(bridge.load).not.toHaveBeenCalled();
    expect(bridge.unload).not.toHaveBeenCalled();
    expect(bridge.restart).not.toHaveBeenCalled();
    expect(bridge.stop).not.toHaveBeenCalled();
    expect(bridge.onChanged).not.toHaveBeenCalled();
    expect(vi.getTimerCount()).toBe(0);
  });

  it('routes a connected load operation and exposes pending state', async () => {
    const pending = deferred<ModelRuntimeSnapshot>();
    bridge.load.mockReturnValueOnce(pending.promise);
    const task = resource.perform({
      kind: 'load',
      modelId: 'model-0000000000000001',
    });

    expect(resource.pending()).toEqual({
      kind: 'load',
      modelId: 'model-0000000000000001',
    });
    expect(bridge.load).toHaveBeenCalledWith('model-0000000000000001');

    const loaded = createDemoModelRuntimeSnapshot('ready');
    pending.resolve(loaded);
    await task;
    expect(resource.pending()).toBeNull();
    expect(resource.resource().value).toBe(loaded);
  });

  it('serialises a user operation behind an in-flight reconciliation', async () => {
    const initial = deferred<ModelRuntimeSnapshot>();
    bridge.snapshot.mockReturnValueOnce(initial.promise);
    const disconnect = resource.connect();

    const task = resource.perform({
      kind: 'load',
      modelId: 'model-0000000000000001',
    });
    await Promise.resolve();
    expect(bridge.load).not.toHaveBeenCalled();

    initial.resolve(createDemoModelRuntimeSnapshot('model-less'));
    await initial.promise;
    await task;

    expect(bridge.load).toHaveBeenCalledTimes(1);
    disconnect();
  });
});

// SPDX-License-Identifier: EUPL-1.2

import { signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { ConnectionManagerService } from '../connection-manager.service';
import {
  DesktopModelRuntimeBridgeService,
  MODEL_RUNTIME_EVENT_SOURCE,
  MODEL_RUNTIME_METHODS,
  type ModelRuntimeEventSource,
} from './desktop-model-runtime-bridge.service';
import type { ModelRuntimeState } from './desktop-model-runtime.models';
import { SurfaceBridgeService } from './surfaces/surface-bridge.service';

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

function modelWireFixture(index = 1) {
  return {
    id: `model-${index.toString(16).padStart(16, '0')}`,
    displayName: `gemma-${index}`,
    format: 'snapshot',
    loadable: true,
    loaded: index === 1,
    runtime: index === 1 ? 'metal' : '',
    contextLength: index === 1 ? 8_192 : 0,
    loadedAt: index === 1 ? '2026-07-27T13:00:00Z' : '',
  };
}

function sampleWireFixture() {
  return {
    state: 'ready',
    at: '2026-07-27T13:00:05Z',
    promptTokensPerSecond: 41.8,
    decodeTokensPerSecond: 18.2,
    activeMemoryBytes: 18_400_000_000,
    peakMemoryBytes: 19_200_000_000,
    kvCacheBytes: 2_300_000_000,
  };
}

function snapshotWireFixture(state: ModelRuntimeState = 'ready') {
  return {
    state,
    desired: state !== 'stopped' && state !== 'unavailable',
    activeModelId: state === 'ready' ? 'model-0000000000000001' : '',
    models: [modelWireFixture()],
    metrics: {
      promptTokensPerSecond: 41.8,
      decodeTokensPerSecond: 18.2,
      activeMemoryBytes: 18_400_000_000,
      peakMemoryBytes: 19_200_000_000,
      kvCacheBytes: 2_300_000_000,
      uptimeSeconds: 360,
    },
    history: [sampleWireFixture()],
    refreshedAt: '2026-07-27T13:00:05Z',
    lastHealthyAt: '2026-07-27T13:00:05Z',
    stale: state === 'degraded',
    lastError:
      state === 'degraded'
        ? {
            code: 'runtime_not_ready',
            message: 'The LEM runtime is temporarily unavailable.',
          }
        : null,
  };
}

describe('DesktopModelRuntimeBridgeService', () => {
  const offline = signal(false);
  const surface = { call: vi.fn() };
  const eventHandlers = new Map<string, (payload: unknown) => void>();
  const events: ModelRuntimeEventSource = {
    on: vi.fn((name, handler) => {
      eventHandlers.set(name, handler);
      return vi.fn(() => eventHandlers.delete(name));
    }),
  };
  let service: DesktopModelRuntimeBridgeService;

  beforeEach(() => {
    offline.set(false);
    eventHandlers.clear();
    vi.clearAllMocks();
    TestBed.configureTestingModule({
      providers: [
        DesktopModelRuntimeBridgeService,
        {
          provide: ConnectionManagerService,
          useValue: { offline: offline.asReadonly() },
        },
        { provide: SurfaceBridgeService, useValue: surface },
        { provide: MODEL_RUNTIME_EVENT_SOURCE, useValue: events },
      ],
    });
    service = TestBed.inject(DesktopModelRuntimeBridgeService);
  });

  afterEach(() => TestBed.resetTestingModule());

  it('calls only the exact ModelRuntime methods and opaque load request', async () => {
    surface.call.mockResolvedValue(snapshotWireFixture('model-less'));

    await service.snapshot();
    await service.start();
    await service.load('model-0000000000000001');
    await service.unload();
    await service.restart();
    await service.stop();

    expect(surface.call.mock.calls).toEqual([
      [MODEL_RUNTIME_METHODS.snapshot],
      [MODEL_RUNTIME_METHODS.start],
      [MODEL_RUNTIME_METHODS.load, [{ modelId: 'model-0000000000000001' }]],
      [MODEL_RUNTIME_METHODS.unload],
      [MODEL_RUNTIME_METHODS.restart],
      [MODEL_RUNTIME_METHODS.stop],
    ]);
  });

  it('parses every closed runtime state', async () => {
    for (const state of STATES) {
      surface.call.mockResolvedValueOnce(snapshotWireFixture(state));
      await expect(service.snapshot()).resolves.toMatchObject({ state });
    }
  });

  it('rejects malformed bounds before they enter renderer state', async () => {
    const tooManyModels = Array.from({ length: 513 }, (_, index) => modelWireFixture(index + 1));
    const tooManySamples = Array.from({ length: 721 }, () => sampleWireFixture());
    const malformed: readonly unknown[] = [
      { ...snapshotWireFixture(), state: 'unknown' },
      {
        ...snapshotWireFixture(),
        metrics: { ...snapshotWireFixture().metrics, promptTokensPerSecond: Number.NaN },
      },
      {
        ...snapshotWireFixture(),
        metrics: { ...snapshotWireFixture().metrics, activeMemoryBytes: Number.POSITIVE_INFINITY },
      },
      { ...snapshotWireFixture(), models: tooManyModels },
      { ...snapshotWireFixture(), history: tooManySamples },
      { ...snapshotWireFixture(), refreshedAt: 'not-a-timestamp' },
      { ...snapshotWireFixture(), refreshedAt: '2026-07-27' },
      {
        ...snapshotWireFixture(),
        models: [{ ...modelWireFixture(), displayName: 'x'.repeat(256) }],
      },
    ];

    for (const payload of malformed) {
      surface.call.mockResolvedValueOnce(payload);
      await expect(service.snapshot()).rejects.toThrow('invalid ModelRuntime response');
    }
  });

  it('rejects execution, location, endpoint, and secret fields recursively', async () => {
    const forbidden = [
      'path',
      'model_path',
      'command',
      'arguments',
      'environment',
      'workingDirectory',
      'endpoint',
      'url',
      'token',
      'secret',
      'credential',
      'key',
    ];
    for (const field of forbidden) {
      surface.call.mockResolvedValueOnce({
        ...snapshotWireFixture(),
        nested: { [field]: 'must not cross Wails' },
      });
      await expect(service.snapshot()).rejects.toThrow('invalid ModelRuntime response');
    }
  });

  it('validates opaque model IDs before calling Wails', async () => {
    await expect(service.load('../models/gemma')).rejects.toThrow('valid model ID');
    await expect(service.load('model-xyz')).rejects.toThrow('valid model ID');
    expect(surface.call).not.toHaveBeenCalled();
  });

  it('parses safe invalidations and drops malformed events', () => {
    const handler = vi.fn();
    const off = service.onChanged(handler);

    expect(events.on).toHaveBeenCalledWith('lthn:model-runtime:changed', expect.any(Function));
    eventHandlers.get('lthn:model-runtime:changed')?.({
      reason: 'load-succeeded',
      state: 'ready',
      at: '2026-07-27T13:00:05Z',
    });
    eventHandlers.get('lthn:model-runtime:changed')?.({
      reason: 'load-succeeded',
      state: 'unknown',
      at: '2026-07-27T13:00:05Z',
    });
    eventHandlers.get('lthn:model-runtime:changed')?.({
      reason: 'load-succeeded',
      state: 'ready',
      at: '2026-07-27T13:00:05Z',
      token: 'must-not-cross',
    });

    expect(handler).toHaveBeenCalledTimes(1);
    expect(handler).toHaveBeenCalledWith({
      reason: 'load-succeeded',
      state: 'ready',
      at: '2026-07-27T13:00:05Z',
    });
    off();
    expect(eventHandlers.has('lthn:model-runtime:changed')).toBe(false);
  });

  it('makes no Wails call or event subscription while explicitly offline', async () => {
    offline.set(true);

    const reads = [
      service.snapshot(),
      service.start(),
      service.load('model-0000000000000001'),
      service.unload(),
      service.restart(),
      service.stop(),
    ];
    for (const read of reads) {
      await expect(read).rejects.toThrow('offline demo mode');
    }
    const off = service.onChanged(vi.fn());

    expect(surface.call).not.toHaveBeenCalled();
    expect(events.on).not.toHaveBeenCalled();
    expect(() => off()).not.toThrow();
  });
});

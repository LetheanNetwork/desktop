// SPDX-License-Identifier: EUPL-1.2

import { signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { ConnectionManagerService } from '../connection-manager.service';
import { createDemoServiceCatalogue } from './apps/control/control-services.models';
import {
  DesktopServicesBridgeService,
  SERVICES_EVENT_SOURCE,
  SERVICES_METHODS,
  type ServicesEventSource,
} from './desktop-services-bridge.service';
import { SurfaceBridgeService } from './surfaces/surface-bridge.service';

function snapshotWireFixture() {
  return {
    definition: {
      id: 'serve',
      displayName: 'Lethean Desktop API',
      description: 'OpenAI-compatible local Lethean API.',
      kind: 'service',
      restartPolicy: 'never',
      gracePeriodMillis: 5_000,
      owner: 'lethean',
    },
    state: 'stopped',
    desired: false,
    processId: '',
    pid: 0,
    startedAt: '',
    stoppedAt: '',
    exitCode: 0,
    restartCount: 0,
    lastError: null,
  };
}

function catalogueWireFixture() {
  return {
    services: [snapshotWireFixture()],
    refreshedAt: '2026-07-27T12:00:00Z',
  };
}

function eventWireFixture() {
  return {
    id: 'serve',
    operation: 'start',
    previous: 'stopped',
    state: 'running',
    desired: true,
    processId: 'proc-1',
    errorCode: '',
    at: '2026-07-27T12:00:01Z',
  };
}

describe('DesktopServicesBridgeService', () => {
  const offline = signal(false);
  const surface = { call: vi.fn() };
  const eventHandlers = new Map<string, (payload: unknown) => void>();
  const events: ServicesEventSource = {
    on: vi.fn((name, handler) => {
      eventHandlers.set(name, handler);
      return vi.fn(() => eventHandlers.delete(name));
    }),
  };
  let service: DesktopServicesBridgeService;

  beforeEach(() => {
    offline.set(false);
    eventHandlers.clear();
    vi.clearAllMocks();
    TestBed.configureTestingModule({
      providers: [
        DesktopServicesBridgeService,
        {
          provide: ConnectionManagerService,
          useValue: { offline: offline.asReadonly() },
        },
        { provide: SurfaceBridgeService, useValue: surface },
        { provide: SERVICES_EVENT_SOURCE, useValue: events },
      ],
    });
    service = TestBed.inject(DesktopServicesBridgeService);
  });

  afterEach(() => TestBed.resetTestingModule());

  it('reads and parses the complete services catalogue', async () => {
    surface.call.mockResolvedValue(catalogueWireFixture());

    await expect(service.catalogue()).resolves.toEqual(catalogueWireFixture());
    expect(surface.call).toHaveBeenCalledWith(SERVICES_METHODS.catalogue);
  });

  it('sends only a validated service ID for lifecycle mutations', async () => {
    surface.call.mockResolvedValue(snapshotWireFixture());

    await expect(service.get('serve')).resolves.toMatchObject({ state: 'stopped' });
    await service.start('serve');
    await service.stop('serve');
    await service.restart('serve');

    expect(surface.call.mock.calls).toEqual([
      [SERVICES_METHODS.get, ['serve']],
      [SERVICES_METHODS.start, ['serve']],
      [SERVICES_METHODS.stop, ['serve']],
      [SERVICES_METHODS.restart, ['serve']],
    ]);
  });

  it('uses bounded typed requests for output and policy changes', async () => {
    surface.call
      .mockResolvedValueOnce({
        id: 'serve',
        processId: 'proc-1',
        generation: 2,
        output: 'ready\n',
        truncated: false,
        observedAt: '2026-07-27T12:00:02Z',
      })
      .mockResolvedValueOnce(snapshotWireFixture());

    await expect(service.output('serve', 8_192)).resolves.toMatchObject({
      output: 'ready\n',
      generation: 2,
    });
    await service.setPolicy({
      id: 'serve',
      restartPolicy: 'on-failure',
      gracePeriodMillis: 7_000,
    });

    expect(surface.call.mock.calls).toEqual([
      [SERVICES_METHODS.output, [{ id: 'serve', limit: 8_192 }]],
      [
        SERVICES_METHODS.setPolicy,
        [{ id: 'serve', restartPolicy: 'on-failure', gracePeriodMillis: 7_000 }],
      ],
    ]);
  });

  it('rejects invalid renderer requests before they reach Wails', async () => {
    await expect(service.start('../serve')).rejects.toThrow('valid service ID');
    await expect(service.output('serve', 0)).rejects.toThrow('output limit');
    await expect(
      service.setPolicy({
        id: 'serve',
        restartPolicy: 'sometimes' as 'never',
        gracePeriodMillis: 5_000,
      }),
    ).rejects.toThrow('restart policy');
    await expect(
      service.setPolicy({
        id: 'serve',
        restartPolicy: 'never',
        gracePeriodMillis: 60_001,
      }),
    ).rejects.toThrow('grace period');
    expect(surface.call).not.toHaveBeenCalled();
  });

  it('rejects execution-bearing, secret-bearing, and malformed responses recursively', async () => {
    const malformed = [
      { ...catalogueWireFixture(), command: '/usr/bin/tool' },
      { ...catalogueWireFixture(), nested: { arguments: ['--token', 'secret'] } },
      { ...catalogueWireFixture(), environment: ['TOKEN=secret'] },
      {
        ...catalogueWireFixture(),
        services: [
          {
            ...snapshotWireFixture(),
            definition: { ...snapshotWireFixture().definition, workingDirectory: '/tmp' },
          },
        ],
      },
      {
        ...catalogueWireFixture(),
        services: [{ ...snapshotWireFixture(), pid: -1 }],
      },
      {
        ...catalogueWireFixture(),
        services: [{ ...snapshotWireFixture(), state: 'unknown' }],
      },
    ];

    for (const payload of malformed) {
      surface.call.mockResolvedValueOnce(payload);
      await expect(service.catalogue()).rejects.toThrow('invalid Services response');
    }
  });

  it('parses lifecycle invalidations and returns the Wails unsubscribe function', () => {
    const handler = vi.fn();
    const off = service.onChanged(handler);

    expect(events.on).toHaveBeenCalledWith('lthn:services:changed', expect.any(Function));
    eventHandlers.get('lthn:services:changed')?.(eventWireFixture());
    expect(handler).toHaveBeenCalledWith(eventWireFixture());

    off();
    expect(eventHandlers.has('lthn:services:changed')).toBe(false);
  });

  it('drops malformed or execution-bearing invalidations', () => {
    const handler = vi.fn();
    service.onChanged(handler);

    eventHandlers.get('lthn:services:changed')?.({ ...eventWireFixture(), state: 'unknown' });
    eventHandlers.get('lthn:services:changed')?.({
      ...eventWireFixture(),
      command: 'lthn serve',
    });

    expect(handler).not.toHaveBeenCalled();
  });

  it('installs no listener and calls no Wails method in offline demo mode', async () => {
    offline.set(true);

    expect(service.onChanged(vi.fn())).toEqual(expect.any(Function));
    await expect(service.catalogue()).rejects.toThrow('offline demo mode');

    expect(events.on).not.toHaveBeenCalled();
    expect(surface.call).not.toHaveBeenCalled();
  });

  it('creates fresh, visibly labelled demo catalogues', () => {
    const first = createDemoServiceCatalogue();
    const second = createDemoServiceCatalogue();

    expect(first).not.toBe(second);
    expect(first.services).not.toBe(second.services);
    expect(first.services[0]).not.toBe(second.services[0]);
    expect(first.services.map(({ definition }) => definition.description).join(' ')).toContain(
      'Lethean demo fixture',
    );
    expect(first.services.map(({ state }) => state)).toEqual(['running', 'stopped', 'failed']);
  });
});

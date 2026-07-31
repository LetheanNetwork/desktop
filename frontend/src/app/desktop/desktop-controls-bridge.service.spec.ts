import { signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { firstValueFrom, Subject } from 'rxjs';
import { ConnectionManagerService } from '../connection-manager.service';
import {
  DesktopControlChange,
  DesktopControlSnapshot,
  DesktopControlsChangeNotice,
} from '../store/desktop-controls.models';
import {
  DESKTOP_CONTROLS_EVENT_SOURCE,
  DesktopControlsBridgeService,
  type DesktopControlsEventSource,
} from './desktop-controls-bridge.service';
import { DesktopControlsOfflineStore } from './desktop-controls-offline.store';
import { SurfaceBridgeService } from './surfaces/surface-bridge.service';

const snapshot: DesktopControlSnapshot = {
  revision: '7',
  controls: [
    {
      key: 'desktop.wails.window.main.width',
      group: 'Window',
      label: 'Window width',
      description: 'Width in pixels.',
      kind: 'number',
      value: 1280,
      defaultValue: 1440,
      configured: true,
      live: true,
      restartRequired: false,
      minimum: 800,
      maximum: 3840,
      step: 10,
    },
  ],
};

describe('DesktopControlsBridgeService', () => {
  const offline = signal(false);
  let surface: { call: ReturnType<typeof vi.fn> };
  let eventHandlers: Map<string, (payload: unknown) => void>;
  let events: DesktopControlsEventSource;
  let offlineChanges: Subject<DesktopControlsChangeNotice>;
  let offlineStore: {
    settings: ReturnType<typeof vi.fn>;
    setMany: ReturnType<typeof vi.fn>;
    changes: ReturnType<typeof vi.fn>;
  };
  let service: DesktopControlsBridgeService;

  beforeEach(() => {
    offline.set(false);
    surface = { call: vi.fn() };
    eventHandlers = new Map();
    events = {
      on: vi.fn((name, handler) => {
        eventHandlers.set(name, handler);
        return vi.fn(() => eventHandlers.delete(name));
      }),
    };
    offlineChanges = new Subject<DesktopControlsChangeNotice>();
    offlineStore = {
      settings: vi.fn(),
      setMany: vi.fn(),
      changes: vi.fn(() => offlineChanges.asObservable()),
    };
    TestBed.configureTestingModule({
      providers: [
        DesktopControlsBridgeService,
        { provide: SurfaceBridgeService, useValue: surface },
        { provide: DESKTOP_CONTROLS_EVENT_SOURCE, useValue: events },
        { provide: DesktopControlsOfflineStore, useValue: offlineStore },
        {
          provide: ConnectionManagerService,
          useValue: { offline: offline.asReadonly() },
        },
      ],
    });
    service = TestBed.inject(DesktopControlsBridgeService);
  });

  it('loads and normalises the curated Go control catalogue without provider paths', async () => {
    surface.call.mockResolvedValue({
      revision: '7',
      controls: [
        {
          key: 'desktop.wails.window.main.width',
          group: 'Window',
          label: 'Window width',
          description: 'Width in pixels.',
          kind: 'number',
          value: 1280,
          default: 1440,
          configured: true,
          live: true,
          restart_required: false,
          minimum: 800,
          maximum: 3840,
          step: 10,
        },
      ],
    });

    await expect(service.settings()).resolves.toEqual(snapshot);
    expect(surface.call).toHaveBeenCalledWith(
      'dappco.re/lthn/desktop/pkg/appconfig.Service.Settings',
    );
  });

  it('persists one bounded draft through SetMany', async () => {
    surface.call.mockResolvedValue({ revision: '8', controls: [] });
    const changes: readonly DesktopControlChange[] = [
      { key: 'desktop.theme.interface', value: 'light' },
      { key: 'desktop.theme.reduce_motion', value: true },
    ];

    await service.setMany(changes);

    expect(surface.call).toHaveBeenCalledTimes(1);
    expect(surface.call).toHaveBeenCalledWith(
      'dappco.re/lthn/desktop/pkg/appconfig.Service.SetMany',
      [changes],
    );
  });

  it('rejects malformed and execution-shaped changes before Wails', async () => {
    await expect(
      service.setMany([{ key: 'desktop.theme.interface;exec', value: 'light' }]),
    ).rejects.toThrow('desktop control');
    await expect(
      service.setMany(
        Array.from({ length: 65 }, (_, index) => ({
          key: `desktop.safe.control_${index}`,
          value: true,
        })),
      ),
    ).rejects.toThrow('Too many');
    expect(surface.call).not.toHaveBeenCalled();
  });

  it('uses isolated in-memory demo settings while explicitly offline', async () => {
    offline.set(true);
    offlineStore.settings.mockResolvedValue(snapshot);
    offlineStore.setMany.mockResolvedValue({ ...snapshot, revision: '8' });

    const before = await service.settings();
    const after = await service.setMany([{ key: 'desktop.theme.interface', value: 'light' }]);

    expect(before).toEqual(snapshot);
    expect(after.revision).not.toBe(before.revision);
    expect(offlineStore.settings).toHaveBeenCalledTimes(1);
    expect(offlineStore.setMany).toHaveBeenCalledTimes(1);
    expect(surface.call).not.toHaveBeenCalled();
  });

  it('exposes one validated connected event and cleans up its listener', async () => {
    const notice = firstValueFrom(service.changes());
    const handler = eventHandlers.get('lthn:desktop-controls:changed');

    handler?.({
      revision: '2',
      keys: ['desktop.theme.interface'],
      at: '2026-07-31T12:00:00Z',
    });

    await expect(notice).resolves.toEqual({
      revision: '2',
      keys: ['desktop.theme.interface'],
      at: '2026-07-31T12:00:00Z',
    });
    expect(eventHandlers.size).toBe(0);
  });

  it('ignores malformed or extended connected event envelopes', () => {
    const received = vi.fn();
    const subscription = service.changes().subscribe(received);
    const handler = eventHandlers.get('lthn:desktop-controls:changed');

    handler?.({ revision: '', keys: [], at: 'not-a-date' });
    handler?.({
      revision: '2',
      keys: ['desktop.theme.interface'],
      at: '31 July 2026',
    });
    handler?.({
      revision: '2',
      keys: ['desktop.theme.interface'],
      at: '2026-07-31T12:00:00Z',
      command: 'run',
    });

    expect(received).not.toHaveBeenCalled();
    subscription.unsubscribe();
    expect(eventHandlers.size).toBe(0);
  });

  it('uses only the offline change stream in explicit offline mode', async () => {
    offline.set(true);
    const notice = firstValueFrom(service.changes());
    offlineChanges.next({ revision: 'offline-2', keys: [], at: null });

    await expect(notice).resolves.toMatchObject({ revision: 'offline-2' });
    expect(offlineStore.changes).toHaveBeenCalledTimes(1);
    expect(events.on).not.toHaveBeenCalled();
    expect(surface.call).not.toHaveBeenCalled();
  });

  it('rejects malformed snapshots instead of inventing connected settings', async () => {
    surface.call.mockResolvedValue({ revision: '7', controls: 'not-an-array' });

    await expect(service.settings()).rejects.toThrow('desktop control catalogue');
  });
});

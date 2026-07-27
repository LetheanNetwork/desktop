import { signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { CONNECTION_LOCATION, ConnectionManagerService } from '../connection-manager.service';
import { DesktopState } from '../store/desktop.reducer';
import {
  DesktopShellSession,
  DesktopStateBridgeService,
  desktopHydrationFromSession,
  desktopSessionFromState,
  parseLegacyDesktopSession,
} from './desktop-state-bridge.service';
import { SurfaceBridgeService } from './surfaces/surface-bridge.service';

const session: DesktopShellSession = {
  view: 'desktop',
  device: 'full',
  focusId: 'control-one',
  z: 12,
  windows: [
    {
      id: 'control-one',
      app: 'control',
      sub: 'models',
      systemTab: 'overview',
      group: 'workspace-one',
      x: 70,
      y: 24,
      width: 780,
      height: 560,
      z: 12,
      min: false,
      max: false,
    },
  ],
  migratedBrowserState: true,
};

const snapshot = {
  version: 1,
  revision: 4,
  updatedAt: '2026-07-27T10:00:00Z',
  session,
};

const state: DesktopState = {
  wins: [
    {
      id: 'control-one',
      app: 'control',
      sub: 'models',
      systab: 'overview',
      group: 'workspace-one',
      x: 70,
      y: 24,
      w: 780,
      h: 560,
      z: 12,
      min: false,
      max: false,
    },
  ],
  focusId: 'control-one',
  view: 'desktop',
  device: 'full',
  devCat: null,
  z: 12,
  persistence: 'ready',
  persistenceRevision: 4,
  persistenceError: null,
  migratedBrowserState: true,
};

describe('DesktopStateBridgeService', () => {
  const offline = signal(false);
  const surface = {
    call: vi.fn(),
  };

  beforeEach(() => {
    offline.set(false);
    vi.clearAllMocks();
    TestBed.configureTestingModule({
      providers: [
        DesktopStateBridgeService,
        {
          provide: ConnectionManagerService,
          useValue: { offline: offline.asReadonly() },
        },
        { provide: SurfaceBridgeService, useValue: surface },
      ],
    });
  });

  it('loads and validates the connected Medium-backed shell session', async () => {
    surface.call.mockResolvedValue(snapshot);
    const bridge = TestBed.inject(DesktopStateBridgeService);

    await expect(bridge.loadShellSession()).resolves.toEqual(snapshot);
    expect(surface.call).toHaveBeenCalledWith(
      'dappco.re/lthn/desktop/pkg/desktopstate.WailsService.LoadShellSession',
    );
  });

  it('saves one complete revision without exposing renderer-only fields', async () => {
    surface.call.mockResolvedValue({ ...snapshot, revision: 5 });
    const bridge = TestBed.inject(DesktopStateBridgeService);

    await expect(bridge.saveShellSession(4, session)).resolves.toMatchObject({
      revision: 5,
    });
    expect(surface.call).toHaveBeenCalledWith(
      'dappco.re/lthn/desktop/pkg/desktopstate.WailsService.SaveShellSession',
      [{ expectedRevision: 4, session }],
    );

    const encoded = JSON.stringify(surface.call.mock.calls[0]);
    expect(encoded).not.toContain('prev');
    expect(encoded).not.toContain('snapState');
    expect(encoded).not.toContain('minimizing');
  });

  it('uses an isolated deterministic demo document without Wails calls offline', async () => {
    offline.set(true);
    const bridge = TestBed.inject(DesktopStateBridgeService);

    const initial = await bridge.loadShellSession();
    const saved = await bridge.saveShellSession(initial.revision, {
      ...initial.session,
      view: 'shell',
    });
    const reloaded = await bridge.loadShellSession();

    expect(initial).toMatchObject({
      version: 1,
      revision: 0,
      session: {
        view: 'desktop',
        device: 'full',
        windows: [],
        migratedBrowserState: false,
      },
    });
    expect(saved.revision).toBe(1);
    expect(reloaded).toEqual(saved);
    expect(surface.call).not.toHaveBeenCalled();
  });

  it('keeps documented browser preview view and device options offline', async () => {
    offline.set(true);
    TestBed.overrideProvider(CONNECTION_LOCATION, {
      useValue: {
        protocol: 'http:',
        host: '127.0.0.1:9245',
        search: '?lthn-offline=1&lthn-view=device&lthn-device=small',
      },
    });
    const bridge = TestBed.inject(DesktopStateBridgeService);

    await expect(bridge.loadShellSession()).resolves.toMatchObject({
      session: { view: 'device', device: 'small' },
    });
    expect(surface.call).not.toHaveBeenCalled();
  });

  it('rejects malformed, execution-bearing, and provider-leaking responses', async () => {
    const bridge = TestBed.inject(DesktopStateBridgeService);
    for (const invalid of [
      { ...snapshot, revision: -1 },
      { ...snapshot, session: { ...session, command: ['/bin/sh'] } },
      { ...snapshot, session: { ...session, root: '/Users/sarah' } },
      {
        ...snapshot,
        session: {
          ...session,
          windows: [{ ...session.windows[0], id: '../escape' }],
        },
      },
      {
        ...snapshot,
        session: {
          ...session,
          windows: [{ ...session.windows[0], width: 40 }],
        },
      },
    ]) {
      surface.call.mockResolvedValueOnce(invalid);
      await expect(bridge.loadShellSession()).rejects.toThrow('invalid desktop state');
    }
  });

  it('converts only bounded durable window fields in both directions', () => {
    expect(desktopHydrationFromSession(session)).toEqual({
      view: 'desktop',
      device: 'full',
      focusId: 'control-one',
      z: 12,
      wins: [
        {
          id: 'control-one',
          app: 'control',
          sub: 'models',
          systab: 'overview',
          group: 'workspace-one',
          x: 70,
          y: 24,
          w: 780,
          h: 560,
          z: 12,
          min: false,
          max: false,
        },
      ],
    });
    expect(desktopSessionFromState(state, true)).toEqual(session);
  });

  it('parses the legacy browser document once while ignoring old preference fields', () => {
    expect(
      parseLegacyDesktopSession({
        ...desktopHydrationFromSession(session),
        mode: 'light',
        brand: 'hostuk',
        shellTabs: [{ command: ['/bin/sh'] }],
      }),
    ).toEqual(session);

    expect(parseLegacyDesktopSession({ wins: [{ id: '../bad' }] })).toBeNull();
    expect(parseLegacyDesktopSession('{broken')).toBeNull();
  });
});

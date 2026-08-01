import { Win } from '../desktop/desktop.data';
import { desktopActions } from './desktop.actions';
import {
  DesktopState,
  desktopFeature,
  desktopReducer,
  initialDesktopState,
} from './desktop.reducer';

const win = (overrides: Partial<Win> = {}): Win => ({
  id: 'w1',
  app: 'control',
  sub: 'models',
  systab: '',
  x: 70,
  y: 24,
  w: 780,
  h: 560,
  z: 11,
  min: false,
  max: false,
  ...overrides,
});

const state = (overrides: Partial<DesktopState> = {}): DesktopState => ({
  ...initialDesktopState,
  ...overrides,
});

describe('desktop reducer', () => {
  it('hydrates, drops invalid windows, clears dangling focus, and seeds an empty windowed desktop', () => {
    const hydrated = desktopReducer(
      initialDesktopState,
      desktopActions.hydrate({
        state: {
          wins: [win({ id: 'shell' }), win({ id: 'unknown', app: 'missing' })],
          focusId: 'unknown',
          view: 'desktop',
          device: 'small',
          z: 30,
        },
        seedWindowIds: ['welcome-id'],
      }),
    );

    // A fresh desktop seeds Welcome and nothing else, and with no viewport
    // passed it falls back to the corner cascade rather than centring.
    expect(hydrated.wins.map(({ id, app, x, y, z }) => ({ id, app, x, y, z }))).toEqual([
      { id: 'welcome-id', app: 'welcome', x: 70, y: 24, z: 31 },
    ]);
    expect(hydrated.focusId).toBe('welcome-id');
    expect(hydrated.z).toBe(31);
  });

  it('loads a durable session, canonicalises routes, and records its revision', () => {
    const loaded = desktopReducer(
      initialDesktopState,
      desktopActions.loadSessionSuccess({
        state: {
          wins: [
            win({
              id: 'control-one',
              app: 'control',
              sub: 'retired-route',
              systab: 'retired-tab',
            }),
            win({
              id: 'files-one',
              app: 'files',
              sub: 'not a valid mount',
              systab: 'retired-view',
              z: 12,
            }),
            win({ id: 'unknown-one', app: 'retired-app', z: 13 }),
          ],
          focusId: 'unknown-one',
          view: 'desktop',
          device: 'full',
          z: 13,
        },
        revision: 7,
        migratedBrowserState: true,
        seedWindowIds: ['unused-one'],
      }),
    );

    expect(loaded.wins).toHaveLength(2);
    expect(loaded.wins[0]).toMatchObject({
      id: 'control-one',
      sub: 'models',
      systab: '',
    });
    expect(loaded.wins[1]).toMatchObject({
      id: 'files-one',
      sub: 'home',
      systab: '',
    });
    expect(loaded.focusId).toBeNull();
    expect(loaded.persistence).toBe('ready');
    expect(loaded.persistenceRevision).toBe(7);
    expect(loaded.persistenceError).toBeNull();
    expect(loaded.migratedBrowserState).toBe(true);
  });

  it('fails closed on unavailable state and advances only committed revisions', () => {
    const loading = desktopReducer(
      state({ persistence: 'ready', persistenceRevision: 4 }),
      desktopActions.loadSession(),
    );
    expect(loading.persistence).toBe('loading');

    const failed = desktopReducer(
      loading,
      desktopActions.loadSessionFailure({ error: 'state unavailable' }),
    );
    expect(failed.persistence).toBe('unavailable');
    expect(failed.persistenceRevision).toBe(4);
    expect(failed.persistenceError).toBe('state unavailable');

    const saved = desktopReducer(
      state({ persistenceRevision: 4, migratedBrowserState: false }),
      desktopActions.saveSessionSuccess({
        revision: 5,
        migratedBrowserState: true,
      }),
    );
    expect(saved.persistenceRevision).toBe(5);
    expect(saved.migratedBrowserState).toBe(true);
  });

  it('launches with cascade geometry or focuses and restores an existing app', () => {
    const launched = desktopReducer(
      state({
        wins: [win()],
        focusId: 'w1',
        devCat: 'system',
        z: 11,
      }),
      desktopActions.launchApp({ appId: 'telemetry', windowId: 'w2' }),
    );

    expect(launched.wins[1]).toMatchObject({
      id: 'w2',
      app: 'telemetry',
      sub: '',
      systab: '',
      x: 104,
      y: 54,
      w: 660,
      h: 400,
      z: 12,
      min: false,
      max: false,
    });
    expect(launched.focusId).toBe('w2');
    expect(launched.devCat).toBeNull();

    const existing = desktopReducer(
      state({
        wins: [win({ min: true })],
        focusId: null,
        devCat: 'system',
        z: 20,
      }),
      desktopActions.launchApp({ appId: 'control', windowId: 'unused' }),
    );

    expect(existing.wins).toEqual([win({ min: false, z: 21 })]);
    expect(existing.focusId).toBe('w1');
    expect(existing.devCat).toBeNull();
    expect(existing.z).toBe(21);
  });

  it('focuses a window with the next z and ignores an unknown focus id', () => {
    const focused = desktopReducer(
      state({ wins: [win({ min: true })], z: 11, devCat: 'system' }),
      desktopActions.focusWindow({ id: 'w1' }),
    );

    expect(focused.wins[0]).toMatchObject({ z: 12, min: false });
    expect(focused.focusId).toBe('w1');
    expect(focused.devCat).toBeNull();
    expect(focused.z).toBe(12);

    const unknown = desktopReducer(focused, desktopActions.focusWindow({ id: 'missing' }));
    expect(unknown).toBe(focused);
  });

  it('closes a focused window and selects the adjacent predecessor', () => {
    const closed = desktopReducer(
      state({
        wins: [
          win({ id: 'w1' }),
          win({ id: 'w2', app: 'chat', z: 12 }),
          win({ id: 'w3', app: 'files', z: 13 }),
        ],
        focusId: 'w2',
        z: 13,
      }),
      desktopActions.closeWindow({ id: 'w2' }),
    );

    expect(closed.wins.map(({ id }) => id)).toEqual(['w1', 'w3']);
    expect(closed.focusId).toBe('w1');
  });

  it('marks delayed minimisation before completing it and clearing focus', () => {
    const minimizing = desktopReducer(
      state({ wins: [win()], focusId: 'w1' }),
      desktopActions.minimiseWindow({ id: 'w1', delayMs: 190 }),
    );

    expect(minimizing.wins[0]).toMatchObject({ min: false, minimizing: true });
    expect(minimizing.focusId).toBe('w1');

    const minimized = desktopReducer(
      minimizing,
      desktopActions.minimiseWindow({ id: 'w1', delayMs: 0 }),
    );

    expect(minimized.wins[0]).toMatchObject({ min: true, minimizing: false });
    expect(minimized.focusId).toBeNull();
  });

  it('maximises and restores previous geometry while focusing with the next z', () => {
    const maximized = desktopReducer(
      state({ wins: [win()], focusId: null, devCat: 'system', z: 11 }),
      desktopActions.maximiseWindow({ id: 'w1', bounds: { w: 1200, h: 800 } }),
    );

    expect(maximized.wins[0]).toMatchObject({
      x: 0,
      y: 0,
      w: 1200,
      h: 800,
      z: 12,
      max: true,
      min: false,
      prev: { x: 70, y: 24, w: 780, h: 560 },
    });
    expect(maximized.focusId).toBe('w1');
    expect(maximized.devCat).toBeNull();

    const restored = desktopReducer(maximized, desktopActions.maximiseWindow({ id: 'w1' }));

    expect(restored.wins[0]).toMatchObject({
      x: 70,
      y: 24,
      w: 780,
      h: 560,
      z: 13,
      max: false,
    });
    expect(restored.wins[0].prev).toBeUndefined();
  });

  it('updates geometry, app tabs, view, device, and developer category', () => {
    let next = state({ wins: [win()], devCat: 'system' });
    next = desktopReducer(next, desktopActions.moveWindow({ id: 'w1', x: 5, y: 6 }));
    next = desktopReducer(next, desktopActions.resizeWindow({ id: 'w1', w: 500, h: 400 }));
    next = desktopReducer(next, desktopActions.setSub({ id: 'w1', sub: 'runs' }));
    next = desktopReducer(next, desktopActions.setSysTab({ id: 'w1', systab: 'list' }));
    next = desktopReducer(next, desktopActions.setDevice({ device: 'large' }));
    next = desktopReducer(next, desktopActions.toggleDevCat({ id: 'system' }));

    expect(next.wins[0]).toMatchObject({
      x: 5,
      y: 6,
      w: 500,
      h: 400,
      sub: 'runs',
      systab: 'list',
    });
    expect(next.device).toBe('large');
    expect(next.devCat).toBeNull();

    next = desktopReducer(next, desktopActions.toggleDevCat({ id: 'developer' }));
    expect(next.devCat).toBe('developer');
  });

  it('normalises every view change and seeds only an empty windowed view', () => {
    const shell = desktopReducer(
      state({
        wins: [win({ id: 'shell' }), win({ id: 'bad', app: 'missing' })],
        focusId: 'bad',
        z: 14,
      }),
      desktopActions.setView({
        view: 'shell',
        seedWindowIds: ['unused-1'],
      }),
    );

    expect(shell).toMatchObject({ view: 'shell', wins: [], focusId: null, z: 14 });

    const desktop = desktopReducer(
      shell,
      desktopActions.setView({
        view: 'desktop',
        seedWindowIds: ['welcome-id'],
      }),
    );

    expect(desktop.wins.map(({ id, app }) => ({ id, app }))).toEqual([
      { id: 'welcome-id', app: 'welcome' },
    ]);
    expect(desktop.focusId).toBe('welcome-id');
    expect(desktop.z).toBe(15);
  });

  it('uses device Home to close the focused window, otherwise clears home state', () => {
    const deviceHome = desktopReducer(
      state({
        view: 'device',
        wins: [win({ id: 'w1' }), win({ id: 'w2', app: 'chat' })],
        focusId: 'w2',
        devCat: 'system',
      }),
      desktopActions.goHome(),
    );

    expect(deviceHome.wins.map(({ id }) => id)).toEqual(['w1']);
    expect(deviceHome.focusId).toBe('w1');
    expect(deviceHome.devCat).toBe('system');

    const shellHome = desktopReducer(
      { ...deviceHome, view: 'shell', focusId: 'w1' },
      desktopActions.goHome(),
    );
    expect(shellHome.focusId).toBeNull();
    expect(shellHome.devCat).toBeNull();

    const cleared = desktopReducer(shellHome, desktopActions.clear());
    expect(cleared.wins).toEqual([]);
    expect(cleared.focusId).toBeNull();
    expect(cleared.devCat).toBeNull();
  });
});

describe('desktop selectors', () => {
  it('mirrors all four WindowManagerService computed values and exposes next z', () => {
    const grouped = win({ id: 'grouped', app: 'chat', group: 'g1' });
    const active = win({ id: 'active', app: 'files', z: 12 });
    const unknown = win({ id: 'unknown', app: 'missing', z: 13 });
    const shellState = state({
      view: 'shell',
      wins: [grouped, active, unknown],
      focusId: 'active',
      z: 13,
    });
    const root = { desktop: shellState };

    expect(desktopFeature.selectWindowed(root)).toBe(false);
    expect(desktopFeature.selectRenderWins(root)).toEqual([active]);
    expect(desktopFeature.selectHomeVisible(root)).toBe(false);
    expect(desktopFeature.selectOpenWins(root)).toEqual([active]);
    expect(desktopFeature.selectNextZ(root)).toBe(14);

    const home = {
      desktop: {
        ...shellState,
        focusId: null,
      },
    };
    expect(desktopFeature.selectRenderWins(home)).toEqual([]);
    expect(desktopFeature.selectHomeVisible(home)).toBe(true);

    const windowed = {
      desktop: {
        ...shellState,
        view: 'device' as const,
        device: 'full' as const,
      },
    };
    expect(desktopFeature.selectWindowed(windowed)).toBe(true);
    expect(desktopFeature.selectRenderWins(windowed)).toEqual([active, unknown]);
    expect(desktopFeature.selectHomeVisible(windowed)).toBe(false);
  });
});

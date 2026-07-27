import { TestBed } from '@angular/core/testing';
import { Action } from '@ngrx/store';
import { provideMockActions } from '@ngrx/effects/testing';
import { MockStore, provideMockStore } from '@ngrx/store/testing';
import { firstValueFrom, Subject } from 'rxjs';
import {
  DesktopShellSessionSnapshot,
  DesktopStateBridgeService,
  DesktopStateConflictError,
} from '../desktop/desktop-state-bridge.service';
import { Win } from '../desktop/desktop.data';
import { WindowManagerService } from '../desktop/window-manager.service';
import { desktopActions } from './desktop.actions';
import { DesktopEffects } from './desktop.effects';
import { DesktopState } from './desktop.reducer';
import { DESKTOP_STORAGE, StorageService } from './storage.service';

const persistedWin: Win = {
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
  prev: { x: 1, y: 2, w: 3, h: 4 },
  minimizing: true,
};

const desktopState: DesktopState = {
  wins: [persistedWin],
  focusId: 'w1',
  view: 'desktop',
  device: 'small',
  devCat: 'system',
  z: 11,
  persistence: 'ready',
  persistenceRevision: 4,
  persistenceError: null,
  migratedBrowserState: true,
};

const shellSnapshot: DesktopShellSessionSnapshot = {
  version: 1,
  revision: 4,
  updatedAt: '2026-07-27T10:00:00Z',
  session: {
    view: 'desktop',
    device: 'small',
    focusId: 'w1',
    z: 11,
    windows: [
      {
        id: 'w1',
        app: 'control',
        sub: 'models',
        systemTab: '',
        group: '',
        x: 70,
        y: 24,
        width: 780,
        height: 560,
        z: 11,
        min: false,
        max: false,
      },
    ],
    migratedBrowserState: true,
  },
};

describe('StorageService', () => {
  const key = 'desktop.effects.spec';
  let memory: Storage;
  let service: StorageService;

  beforeEach(() => {
    const values = new Map<string, string>();
    memory = {
      get length() {
        return values.size;
      },
      clear: () => values.clear(),
      getItem: (storageKey) => values.get(storageKey) ?? null,
      key: (index) => [...values.keys()][index] ?? null,
      removeItem: (storageKey) => values.delete(storageKey),
      setItem: (storageKey, value) => values.set(storageKey, value),
    };
    TestBed.configureTestingModule({
      providers: [StorageService, { provide: DESKTOP_STORAGE, useValue: memory }],
    });
    service = TestBed.inject(StorageService);
  });

  it('reads, writes, and removes JSON while treating malformed JSON as empty', () => {
    service.write(key, { view: 'shell' });
    expect(service.read(key)).toEqual({ view: 'shell' });

    memory.setItem(key, '{broken');
    expect(service.read(key)).toBeNull();

    service.remove(key);
    expect(memory.getItem(key)).toBeNull();
  });
});

describe('DESKTOP_STORAGE', () => {
  it('uses jsdom window storage instead of the Node global', () => {
    const key = 'desktop.storage.factory.spec';
    const service = TestBed.inject(StorageService);

    service.write(key, { ready: true });

    expect(window.localStorage.getItem(key)).toBe('{"ready":true}');
    expect(service.read(key)).toEqual({ ready: true });
  });
});

describe('DesktopEffects', () => {
  let actions$: Subject<Action>;
  let effects: DesktopEffects;
  let store: MockStore;
  let bridge: {
    isOffline: ReturnType<typeof vi.fn>;
    loadShellSession: ReturnType<typeof vi.fn>;
    saveShellSession: ReturnType<typeof vi.fn>;
  };
  let storage: {
    read: ReturnType<typeof vi.fn>;
    write: ReturnType<typeof vi.fn>;
    remove: ReturnType<typeof vi.fn>;
  };
  let windows: {
    reconcileHydration: ReturnType<typeof vi.fn>;
  };

  beforeEach(() => {
    actions$ = new Subject<Action>();
    bridge = {
      isOffline: vi.fn(() => false),
      loadShellSession: vi.fn(),
      saveShellSession: vi.fn(),
    };
    storage = {
      read: vi.fn(),
      write: vi.fn(),
      remove: vi.fn(),
    };
    windows = {
      reconcileHydration: vi.fn((state) => state),
    };

    TestBed.configureTestingModule({
      providers: [
        DesktopEffects,
        provideMockActions(() => actions$),
        provideMockStore({ initialState: { desktop: desktopState } }),
        { provide: DesktopStateBridgeService, useValue: bridge },
        { provide: StorageService, useValue: storage },
        { provide: WindowManagerService, useValue: windows },
      ],
    });
    effects = TestBed.inject(DesktopEffects);
    store = TestBed.inject(MockStore);
  });

  afterEach(() => {
    actions$.complete();
    vi.useRealTimers();
  });

  it('dispatches deterministic safe state before requesting connected hydration', async () => {
    const action = await firstValueFrom(effects.hydrate$);

    expect(bridge.loadShellSession).not.toHaveBeenCalled();
    expect(storage.read).not.toHaveBeenCalled();
    expect(action).toMatchObject({
      type: '[Desktop] Hydrate',
      state: null,
      persist: false,
      seedWindowIds: ['w-initial-control', 'w-initial-telemetry'],
    });
  });

  it('loads connected state through the bridge and reconciles its viewport', async () => {
    bridge.loadShellSession.mockResolvedValue(shellSnapshot);
    const loaded = firstValueFrom(effects.loadSession$);

    actions$.next(desktopActions.loadSession());

    await expect(loaded).resolves.toMatchObject({
      type: '[Desktop] Load Session Success',
      revision: 4,
      migratedBrowserState: true,
      state: {
        view: 'desktop',
        device: 'small',
        focusId: 'w1',
      },
    });
    expect(windows.reconcileHydration).toHaveBeenCalledTimes(1);
    expect(storage.read).not.toHaveBeenCalled();
  });

  it('migrates legacy browser state and removes it only after a successful save', async () => {
    bridge.loadShellSession.mockResolvedValue({
      ...shellSnapshot,
      revision: 0,
      updatedAt: '',
      session: {
        ...shellSnapshot.session,
        focusId: '',
        windows: [],
        migratedBrowserState: false,
      },
    });
    storage.read.mockReturnValue({
      view: 'shell',
      device: 'large',
      focusId: 'legacy-one',
      z: 8,
      wins: [
        {
          id: 'legacy-one',
          app: 'files',
          sub: 'home',
          systab: 'grid',
          x: 20,
          y: 30,
          w: 820,
          h: 560,
          z: 8,
          min: false,
          max: false,
        },
      ],
      mode: 'light',
    });
    bridge.saveShellSession.mockImplementation(
      (_revision: number, session: DesktopShellSessionSnapshot['session']) =>
        Promise.resolve({
          version: 1,
          revision: 1,
          updatedAt: '2026-07-27T10:01:00Z',
          session,
        }),
    );
    const loaded = firstValueFrom(effects.loadSession$);

    actions$.next(desktopActions.loadSession());
    const result = await loaded;

    expect(result).toMatchObject({
      type: '[Desktop] Load Session Success',
      revision: 1,
      migratedBrowserState: true,
      state: {
        view: 'shell',
        device: 'large',
        focusId: 'legacy-one',
      },
    });
    expect(bridge.saveShellSession).toHaveBeenCalledWith(
      0,
      expect.objectContaining({
        view: 'shell',
        migratedBrowserState: true,
      }),
    );
    expect(storage.remove).toHaveBeenCalledWith('lthn.desktop');
  });

  it('keeps legacy evidence when migration or connected load fails', async () => {
    bridge.loadShellSession.mockRejectedValue(new Error('desktop state is malformed'));
    const failed = firstValueFrom(effects.loadSession$);

    actions$.next(desktopActions.loadSession());

    await expect(failed).resolves.toEqual(
      desktopActions.loadSessionFailure({
        error: 'desktop state is malformed',
      }),
    );
    expect(storage.remove).not.toHaveBeenCalled();
  });

  it('completes a delayed minimise with the same action after the delay', async () => {
    const completion = firstValueFrom(effects.completeMinimise$);

    actions$.next(desktopActions.minimiseWindow({ id: 'w1', delayMs: 1 }));

    await expect(completion).resolves.toEqual(
      desktopActions.minimiseWindow({ id: 'w1', delayMs: 0 }),
    );
  });

  it('debounces drag geometry but requests discrete persistence immediately', async () => {
    vi.useFakeTimers();
    const requested: Action[] = [];
    effects.requestSave$.subscribe((action) => requested.push(action));

    actions$.next(desktopActions.moveWindow({ id: 'w1', x: 1, y: 2 }));
    actions$.next(desktopActions.resizeWindow({ id: 'w1', w: 700, h: 500 }));
    await vi.advanceTimersByTimeAsync(149);
    expect(requested).toEqual([]);

    await vi.advanceTimersByTimeAsync(1);
    expect(requested).toEqual([desktopActions.saveSessionRequested()]);

    actions$.next(desktopActions.closeWindow({ id: 'w1' }));
    await Promise.resolve();
    expect(requested).toEqual([
      desktopActions.saveSessionRequested(),
      desktopActions.saveSessionRequested(),
    ]);
  });

  it('serialises one complete durable session and records its new revision', async () => {
    bridge.saveShellSession.mockResolvedValue({
      ...shellSnapshot,
      revision: 5,
    });
    const saved = firstValueFrom(effects.saveSession$);

    actions$.next(desktopActions.saveSessionRequested());

    await expect(saved).resolves.toEqual(
      desktopActions.saveSessionSuccess({
        revision: 5,
        migratedBrowserState: true,
      }),
    );
    expect(bridge.saveShellSession).toHaveBeenCalledWith(
      4,
      expect.objectContaining({
        focusId: 'w1',
        migratedBrowserState: true,
      }),
    );
    const serialised = JSON.stringify(bridge.saveShellSession.mock.calls[0]);
    expect(serialised).not.toContain('prev');
    expect(serialised).not.toContain('minimizing');
  });

  it('reloads a revision conflict instead of overwriting newer state', async () => {
    bridge.saveShellSession.mockRejectedValue(new DesktopStateConflictError());
    const reloaded = firstValueFrom(effects.saveSession$);

    actions$.next(desktopActions.saveSessionRequested());

    await expect(reloaded).resolves.toEqual(desktopActions.loadSession());
  });

  it('contains invalid in-memory state as a failed save without killing the effect', async () => {
    store.setState({
      desktop: {
        ...desktopState,
        focusId: 'missing-window',
      },
    });
    const failed = firstValueFrom(effects.saveSession$);

    actions$.next(desktopActions.saveSessionRequested());

    await expect(failed).resolves.toMatchObject({
      type: '[Desktop] Save Session Failure',
      error: expect.stringContaining('invalid desktop state'),
    });
    expect(bridge.saveShellSession).not.toHaveBeenCalled();
  });
});

import { TestBed } from '@angular/core/testing';
import { Action } from '@ngrx/store';
import { provideMockActions } from '@ngrx/effects/testing';
import { provideMockStore } from '@ngrx/store/testing';
import { firstValueFrom, Subject } from 'rxjs';
import { Win } from '../desktop/desktop.data';
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
  max: true,
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

  it('reads and writes JSON through localStorage and treats malformed JSON as empty', () => {
    service.write(key, { view: 'shell' });
    expect(service.read(key)).toEqual({ view: 'shell' });

    memory.setItem(key, '{broken');
    expect(service.read(key)).toBeNull();
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
  let storage: {
    read: ReturnType<typeof vi.fn>;
    write: ReturnType<typeof vi.fn>;
  };

  beforeEach(() => {
    actions$ = new Subject<Action>();
    storage = {
      read: vi.fn(),
      write: vi.fn(),
    };

    TestBed.configureTestingModule({
      providers: [
        DesktopEffects,
        provideMockActions(() => actions$),
        provideMockStore({ initialState: { desktop: desktopState } }),
        { provide: StorageService, useValue: storage },
      ],
    });
    effects = TestBed.inject(DesktopEffects);
  });

  afterEach(() => actions$.complete());

  it('dispatches a deterministic pre-hydration state without reading browser storage', async () => {
    const action = await firstValueFrom(effects.hydrate$);

    expect(storage.read).not.toHaveBeenCalled();
    expect(action).toMatchObject({
      type: '[Desktop] Hydrate',
      state: null,
      persist: false,
    });
    expect(action.seedWindowIds).toEqual(['w-initial-control', 'w-initial-telemetry']);
  });

  it('completes a delayed minimise with the same action after the delay', async () => {
    const completion = firstValueFrom(effects.completeMinimise$);

    actions$.next(desktopActions.minimiseWindow({ id: 'w1', delayMs: 1 }));

    await expect(completion).resolves.toEqual(
      desktopActions.minimiseWindow({ id: 'w1', delayMs: 0 }),
    );
  });

  it('persists every mutating action', () => {
    storage.read.mockReturnValue({});
    effects.persist$.subscribe();

    const mutatingActions = [
      desktopActions.hydrate({ state: null }),
      desktopActions.launchApp({ appId: 'control', windowId: 'w2' }),
      desktopActions.focusWindow({ id: 'w1' }),
      desktopActions.closeWindow({ id: 'w1' }),
      desktopActions.minimiseWindow({ id: 'w1' }),
      desktopActions.maximiseWindow({ id: 'w1' }),
      desktopActions.moveWindow({ id: 'w1', x: 1, y: 2 }),
      desktopActions.resizeWindow({ id: 'w1', w: 3, h: 4 }),
      desktopActions.setSub({ id: 'w1', sub: 'runs' }),
      desktopActions.setSysTab({ id: 'w1', systab: 'list' }),
      desktopActions.setView({ view: 'shell' }),
      desktopActions.setDevice({ device: 'large' }),
      desktopActions.toggleDevCat({ id: 'system' }),
      desktopActions.goHome(),
      desktopActions.clear(),
    ];

    mutatingActions.forEach((action) => actions$.next(action));

    expect(storage.write).toHaveBeenCalledTimes(mutatingActions.length);
  });

  it('does not persist the deterministic pre-hydration action', () => {
    effects.persist$.subscribe();

    actions$.next(
      desktopActions.hydrate({
        state: null,
        seedWindowIds: ['w-initial-control', 'w-initial-telemetry'],
        persist: false,
      }),
    );

    expect(storage.write).not.toHaveBeenCalled();
  });

  it('merges shell-owned storage and writes only the service window shape', () => {
    storage.read.mockReturnValue({
      bar: 'top',
      wall: 'aurora',
      view: 'shell',
      wins: [{ id: 'old' }],
    });
    effects.persist$.subscribe();

    actions$.next(desktopActions.focusWindow({ id: 'w1' }));

    expect(storage.write).toHaveBeenLastCalledWith('lthn.desktop', {
      bar: 'top',
      wall: 'aurora',
      view: 'desktop',
      device: 'small',
      focusId: 'w1',
      z: 11,
      wins: [
        {
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
          max: true,
          group: undefined,
        },
      ],
    });
  });
});

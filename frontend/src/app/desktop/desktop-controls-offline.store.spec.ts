// SPDX-License-Identifier: EUPL-1.2

import { TestBed } from '@angular/core/testing';
import { EMPTY, firstValueFrom, Subject } from 'rxjs';
import { DESKTOP_STORAGE } from '../store/storage.service';
import {
  DESKTOP_CONTROLS_STORAGE_EVENTS,
  DesktopControlsOfflineStore,
  type DesktopControlsStorageEvent,
} from './desktop-controls-offline.store';

const STORAGE_KEY = 'lthn.desktop-controls.v1';
const LEGACY_KEY = 'lthn.prefs';

function memoryStorage(initial: Readonly<Record<string, string>> = {}): Storage {
  const values = new Map(Object.entries(initial));
  return {
    get length() {
      return values.size;
    },
    clear: vi.fn(() => values.clear()),
    getItem: vi.fn((key: string) => values.get(key) ?? null),
    key: vi.fn((index: number) => [...values.keys()][index] ?? null),
    removeItem: vi.fn((key: string) => values.delete(key)),
    setItem: vi.fn((key: string, value: string) => values.set(key, value)),
  };
}

function createStore(
  storage: Storage,
  events: Subject<DesktopControlsStorageEvent> | typeof EMPTY = EMPTY,
): DesktopControlsOfflineStore {
  TestBed.resetTestingModule();
  TestBed.configureTestingModule({
    providers: [
      DesktopControlsOfflineStore,
      { provide: DESKTOP_STORAGE, useValue: storage },
      { provide: DESKTOP_CONTROLS_STORAGE_EVENTS, useValue: events },
    ],
  });
  return TestBed.inject(DesktopControlsOfflineStore);
}

describe('DesktopControlsOfflineStore', () => {
  afterEach(() => TestBed.resetTestingModule());

  it('writes one versioned bounded document for a complete demo snapshot', async () => {
    const storage = memoryStorage();
    const store = createStore(storage);

    const before = await store.settings();
    const after = await store.setMany([
      { key: 'desktop.theme.interface', value: 'light' },
      { key: 'desktop.shell.show_icons', value: false },
    ]);

    expect(after.revision).not.toBe(before.revision);
    expect(JSON.parse(storage.getItem(STORAGE_KEY) ?? 'null')).toEqual({
      version: 1,
      revision: after.revision,
      values: expect.objectContaining({
        'desktop.theme.interface': 'light',
        'desktop.shell.show_icons': false,
      }),
    });
  });

  it('hydrates valid values across a new repository instance', async () => {
    const storage = memoryStorage();
    const first = createStore(storage);
    const committed = await first.setMany([{ key: 'desktop.theme.wallpaper', value: 'mist' }]);

    const second = createStore(storage);
    const hydrated = await second.settings();

    expect(hydrated.revision).toBe(committed.revision);
    expect(hydrated.controls.find(({ key }) => key === 'desktop.theme.wallpaper')?.value).toBe(
      'mist',
    );
  });

  it('fails malformed envelopes closed without overwriting them', async () => {
    const malformed = JSON.stringify({ version: 2, revision: '', values: [] });
    const storage = memoryStorage({ [STORAGE_KEY]: malformed });
    const store = createStore(storage);

    const snapshot = await store.settings();

    expect(snapshot.controls.find(({ key }) => key === 'desktop.theme.interface')?.value).toBe(
      'dark',
    );
    expect(storage.getItem(STORAGE_KEY)).toBe(malformed);
  });

  it('keeps valid known values while discarding unknown and invalid entries', async () => {
    const storage = memoryStorage({
      [STORAGE_KEY]: JSON.stringify({
        version: 1,
        revision: 'stored-1',
        values: {
          'desktop.theme.interface': 'light',
          'desktop.theme.custom_hue': 999,
          'desktop.unknown.command': 'run everything',
        },
      }),
    });
    const store = createStore(storage);

    const snapshot = await store.settings();

    expect(snapshot.revision).toBe('stored-1');
    expect(snapshot.controls.find(({ key }) => key === 'desktop.theme.interface')?.value).toBe(
      'light',
    );
    expect(snapshot.controls.find(({ key }) => key === 'desktop.theme.custom_hue')?.value).toBe(
      305,
    );
    expect(snapshot.controls.some(({ key }) => key === 'desktop.unknown.command')).toBe(false);
  });

  it('seeds known legacy preferences once without deleting the legacy document', async () => {
    const legacy = JSON.stringify({
      bar: 'left',
      mode: 'light',
      customHue: 190,
      showIcons: false,
      ignored: 'value',
    });
    const storage = memoryStorage({ [LEGACY_KEY]: legacy });
    const store = createStore(storage);

    const snapshot = await store.settings();

    expect(snapshot.controls.find(({ key }) => key === 'desktop.shell.taskbar_edge')?.value).toBe(
      'left',
    );
    expect(snapshot.controls.find(({ key }) => key === 'desktop.theme.interface')?.value).toBe(
      'light',
    );
    expect(storage.getItem(LEGACY_KEY)).toBe(legacy);
    expect(JSON.parse(storage.getItem(STORAGE_KEY) ?? 'null')).toMatchObject({
      version: 1,
      revision: snapshot.revision,
    });
  });

  it('turns a matching storage event into a typed change notice', async () => {
    const events = new Subject<DesktopControlsStorageEvent>();
    const storage = memoryStorage();
    const store = createStore(storage, events);
    await store.settings();
    const notice = firstValueFrom(store.changes());
    const stored = JSON.stringify({
      version: 1,
      revision: 'other-tab-2',
      values: {
        'desktop.theme.interface': 'light',
      },
    });
    storage.setItem(STORAGE_KEY, stored);

    events.next({ key: STORAGE_KEY, newValue: stored });

    await expect(notice).resolves.toEqual({
      revision: 'other-tab-2',
      keys: ['desktop.theme.interface'],
      at: null,
    });
    await expect(store.settings()).resolves.toMatchObject({ revision: 'other-tab-2' });
  });

  it('keeps an isolated in-memory snapshot when browser storage is unavailable', async () => {
    const unavailable: Storage = {
      get length(): number {
        throw new Error('unavailable');
      },
      clear: () => {
        throw new Error('unavailable');
      },
      getItem: () => {
        throw new Error('unavailable');
      },
      key: () => {
        throw new Error('unavailable');
      },
      removeItem: () => {
        throw new Error('unavailable');
      },
      setItem: () => {
        throw new Error('unavailable');
      },
    };
    const store = createStore(unavailable);

    const changed = await store.setMany([{ key: 'desktop.theme.interface', value: 'light' }]);

    expect(changed.controls.find(({ key }) => key === 'desktop.theme.interface')?.value).toBe(
      'light',
    );
    await expect(store.settings()).resolves.toEqual(changed);
  });
});

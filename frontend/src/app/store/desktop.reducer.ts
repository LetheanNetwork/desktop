import { createFeature, createReducer, createSelector, on } from '@ngrx/store';
import { APPS, CTRL_NAV, GAMES_NAV, SETTINGS_NAV } from '../desktop/desktop-catalogue.data';
import { DeviceSize, ViewMode, Win } from '../desktop/desktop.data';
import { filesToken, parseFilesToken } from '../desktop/apps/files/files-view-state';
import { DesktopHydration, SeedWindowIds, desktopActions } from './desktop.actions';

export interface DesktopState {
  wins: Win[];
  focusId: string | null;
  view: ViewMode;
  device: DeviceSize;
  devCat: string | null;
  z: number;
  persistence: 'loading' | 'ready' | 'unavailable';
  persistenceRevision: number;
  persistenceError: string | null;
  migratedBrowserState: boolean;
}

export const initialDesktopState: DesktopState = {
  wins: [],
  focusId: null,
  view: 'desktop',
  device: 'small',
  devCat: null,
  z: 10,
  persistence: 'loading',
  persistenceRevision: 0,
  persistenceError: null,
  migratedBrowserState: false,
};

/** The menu bar owns the top of the screen; a window must open below it. */
const MENUBAR_CLEARANCE = 44;

const createWindowId = (): string => 'w' + Date.now() + Math.random().toString(36).slice(2, 5);

const STATIC_SUB_ROUTES: Readonly<Partial<Record<string, ReadonlySet<string>>>> = {
  control: new Set(CTRL_NAV.map(([path]) => path)),
  games: new Set(GAMES_NAV.map(([path]) => path)),
  settings: new Set(SETTINGS_NAV.map(([path]) => path)),
};
const CONTROL_SYSTEM_TABS = new Set(['overview', 'processes', 'daemons']);
const FILE_VIEW_TABS = new Set(['list', 'grid']);

const isFilesApp = (app: string): boolean => app === 'files' || app === 'surface-office-files';

const normaliseWindow = (win: Win): Win | null => {
  const app = APPS[win.app];
  if (!app || win.id === 'shell') return null;

  let sub = '';
  const staticRoutes = STATIC_SUB_ROUTES[win.app];
  if (staticRoutes) {
    sub = staticRoutes.has(win.sub) ? win.sub : (app.defaultSub ?? [...staticRoutes][0] ?? '');
  } else if (isFilesApp(win.app)) {
    sub = filesToken(parseFilesToken(win.sub));
  } else {
    sub = app.defaultSub ?? '';
  }

  let systab = '';
  if (win.app === 'control' && CONTROL_SYSTEM_TABS.has(win.systab ?? '')) {
    systab = win.systab ?? '';
  } else if (isFilesApp(win.app) && FILE_VIEW_TABS.has(win.systab ?? '')) {
    systab = win.systab ?? '';
  }

  return {
    ...win,
    sub,
    systab,
    minimizing: false,
  };
};

const focusWindow = (state: DesktopState, id: string): DesktopState => {
  if (!state.wins.some((win) => win.id === id)) return state;
  let z = state.z;
  const wins = state.wins.map((win) => {
    if (win.id !== id) return win;
    return { ...win, z: ++z, min: false };
  });

  return { ...state, wins, focusId: id, devCat: null, z };
};

const launchApp = (
  state: DesktopState,
  appId: string,
  windowId?: string,
  viewport?: { width: number; height: number },
): DesktopState => {
  const existing = state.wins.find((win) => win.app === appId);
  if (existing) return focusWindow(state, existing.id);

  const app = APPS[appId];
  if (!app) return state;

  const n = state.wins.length;
  const z = state.z + 1;
  const win: Win = {
    id: windowId ?? createWindowId(),
    app: appId,
    sub: app.defaultSub ?? '',
    systab: '',
    // Centred, then cascaded — a window opening in the top-left corner reads
    // as something that failed to place itself. Without a viewport (no caller
    // able to measure) it falls back to the old corner cascade.
    x: viewport ? Math.max(12, Math.round((viewport.width - app.w) / 2) + n * 26) : 70 + n * 34,
    y: viewport
      ? Math.max(MENUBAR_CLEARANCE, Math.round((viewport.height - app.h) / 2) + n * 24)
      : 24 + n * 30,
    w: app.w,
    h: app.h,
    z,
    min: false,
    max: false,
  };

  return {
    ...state,
    wins: [...state.wins, win],
    focusId: win.id,
    devCat: null,
    z,
  };
};

const closeWindow = (state: DesktopState, id: string): DesktopState => {
  const index = state.wins.findIndex((win) => win.id === id);
  if (index < 0) return state;

  const wins = state.wins.filter((win) => win.id !== id);
  const focusId =
    state.focusId === id ? (wins.length ? wins[Math.max(0, index - 1)].id : null) : state.focusId;

  return { ...state, wins, focusId };
};

const isWindowed = (state: DesktopState): boolean =>
  state.view === 'desktop' || (state.view === 'device' && state.device === 'full');

const applyMode = (
  state: DesktopState,
  seedWindowIds?: SeedWindowIds,
  viewport?: { width: number; height: number },
): DesktopState => {
  const wins = state.wins.filter((win) => win.id !== 'shell' && APPS[win.app]);
  const focusId =
    state.focusId && !wins.some((win) => win.id === state.focusId) ? null : state.focusId;
  let next = { ...state, wins, focusId };

  if (isWindowed(next) && next.wins.length === 0) {
    // A fresh desktop opens on Welcome and nothing else. Two panels of live
    // telemetry is what someone wants on their second visit, not their first —
    // and in the browser demo neither of them has anything real to show.
    next = launchApp(next, 'welcome', seedWindowIds?.[0], viewport);
  }

  return next;
};

const hydrateState = (state: DesktopState, hydration: DesktopHydration | null): DesktopState => {
  if (!hydration) return state;

  let next = state;
  if (hydration.view) next = { ...next, view: hydration.view };
  if (hydration.device) next = { ...next, device: hydration.device };
  if (hydration.devCat !== undefined) {
    next = { ...next, devCat: hydration.devCat };
  }
  if (typeof hydration.z === 'number') next = { ...next, z: hydration.z };
  if (Array.isArray(hydration.wins)) {
    const wins = hydration.wins.flatMap((win) => {
      const normalised = normaliseWindow(win);
      return normalised ? [normalised] : [];
    });
    next = {
      ...next,
      wins,
      z: Math.max(next.z, 10, ...wins.map((win) => win.z)),
    };
  }
  if (hydration.focusId !== undefined) {
    next = { ...next, focusId: hydration.focusId };
  }
  return next;
};

export const desktopReducer = createReducer(
  initialDesktopState,
  on(
    desktopActions.hydrate,
    (state, { state: hydration, seedWindowIds, normalise = true }): DesktopState => {
      const hydrated = hydrateState(state, hydration);
      return normalise ? applyMode(hydrated, seedWindowIds) : hydrated;
    },
  ),
  on(desktopActions.loadSession, (state) => ({
    ...state,
    persistence: 'loading' as const,
    persistenceError: null,
  })),
  on(
    desktopActions.loadSessionSuccess,
    (state, { state: hydration, revision, migratedBrowserState, seedWindowIds }) => ({
      ...applyMode(hydrateState(state, hydration), seedWindowIds),
      persistence: 'ready' as const,
      persistenceRevision: revision,
      persistenceError: null,
      migratedBrowserState,
    }),
  ),
  on(desktopActions.loadSessionFailure, (state, { error }) => ({
    ...state,
    persistence: 'unavailable' as const,
    persistenceError: error,
  })),
  on(desktopActions.saveSessionSuccess, (state, { revision, migratedBrowserState }) => ({
    ...state,
    persistence: 'ready' as const,
    persistenceRevision: revision,
    persistenceError: null,
    migratedBrowserState,
  })),
  on(desktopActions.saveSessionFailure, (state, { error }) => ({
    ...state,
    persistence: 'unavailable' as const,
    persistenceError: error,
  })),
  on(desktopActions.launchApp, (state, { appId, windowId, viewport }) =>
    launchApp(state, appId, windowId, viewport),
  ),
  on(desktopActions.focusWindow, (state, { id }) => focusWindow(state, id)),
  on(desktopActions.closeWindow, (state, { id }) => closeWindow(state, id)),
  on(desktopActions.minimiseWindow, (state, { id, delayMs = 0 }) => {
    if (delayMs > 0) {
      return {
        ...state,
        wins: state.wins.map((win) => (win.id === id ? { ...win, minimizing: true } : win)),
      };
    }

    return {
      ...state,
      wins: state.wins.map((win) =>
        win.id === id ? { ...win, min: true, minimizing: false } : win,
      ),
      focusId: state.focusId === id ? null : state.focusId,
    };
  }),
  on(desktopActions.maximiseWindow, (state, { id, bounds }) => {
    const wins = state.wins.map((win) => {
      if (win.id !== id) return win;
      if (win.max) {
        return {
          ...win,
          ...(win.prev ?? {}),
          max: false,
          prev: undefined,
        };
      }
      return {
        ...win,
        prev: { x: win.x, y: win.y, w: win.w, h: win.h },
        max: true,
        x: 0,
        y: 0,
        w: bounds?.w ?? win.w,
        h: bounds?.h ?? win.h,
      };
    });
    return focusWindow({ ...state, wins }, id);
  }),
  on(desktopActions.moveWindow, (state, { id, x, y }) => ({
    ...state,
    wins: state.wins.map((win) => (win.id === id ? { ...win, x, y } : win)),
  })),
  on(desktopActions.resizeWindow, (state, { id, w, h }) => ({
    ...state,
    wins: state.wins.map((win) => (win.id === id ? { ...win, w, h } : win)),
  })),
  on(desktopActions.setSub, (state, { id, sub }) => ({
    ...state,
    wins: state.wins.map((win) => (win.id === id ? { ...win, sub } : win)),
  })),
  on(desktopActions.setSysTab, (state, { id, systab }) => ({
    ...state,
    wins: state.wins.map((win) => (win.id === id ? { ...win, systab } : win)),
  })),
  on(desktopActions.setView, (state, { view, seedWindowIds, viewport }) =>
    applyMode({ ...state, view }, seedWindowIds, viewport),
  ),
  on(desktopActions.setDevice, (state, { device }) => ({
    ...state,
    device,
  })),
  on(desktopActions.toggleDevCat, (state, { id }) => ({
    ...state,
    devCat: state.devCat === id ? null : id,
  })),
  on(desktopActions.goHome, (state) =>
    state.view === 'device' && state.focusId
      ? closeWindow(state, state.focusId)
      : { ...state, focusId: null, devCat: null },
  ),
  on(desktopActions.clear, (state) => ({
    ...state,
    wins: [],
    focusId: null,
    devCat: null,
  })),
);

export const desktopFeature = createFeature({
  name: 'desktop',
  reducer: desktopReducer,
  extraSelectors: ({ selectWins, selectFocusId, selectView, selectDevice, selectZ }) => {
    const selectWindowed = createSelector(
      selectView,
      selectDevice,
      (view, device) => view === 'desktop' || (view === 'device' && device === 'full'),
    );
    const selectRenderWins = createSelector(
      selectWins,
      selectWindowed,
      selectFocusId,
      (wins, windowed, focusId) => {
        const renderable = wins.filter((win) => !win.group);
        if (windowed) return renderable;
        const active = focusId ? renderable.find((win) => win.id === focusId) : null;
        return active ? [active] : [];
      },
    );
    const selectHomeVisible = createSelector(
      selectView,
      selectDevice,
      selectRenderWins,
      (view, device, renderWins) =>
        (view === 'shell' || (view === 'device' && device !== 'full')) && renderWins.length === 0,
    );
    const selectOpenWins = createSelector(selectWins, (wins) =>
      wins.filter((win) => !win.group && APPS[win.app]),
    );
    const selectNextZ = createSelector(selectZ, (z) => z + 1);

    return {
      selectWindowed,
      selectRenderWins,
      selectHomeVisible,
      selectOpenWins,
      selectNextZ,
    };
  },
});

export const {
  selectDesktopState,
  selectWins,
  selectFocusId,
  selectView,
  selectDevice,
  selectDevCat,
  selectZ,
  selectWindowed,
  selectRenderWins,
  selectHomeVisible,
  selectOpenWins,
  selectNextZ,
} = desktopFeature;

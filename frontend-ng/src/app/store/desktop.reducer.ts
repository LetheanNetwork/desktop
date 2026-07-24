import { createFeature, createReducer, createSelector, on } from '@ngrx/store';
import { APPS, DeviceSize, ViewMode, Win } from '../desktop/desktop.data';
import {
  DesktopHydration,
  SeedWindowIds,
  desktopActions,
} from './desktop.actions';

export interface DesktopState {
  wins: Win[];
  focusId: string | null;
  view: ViewMode;
  device: DeviceSize;
  devCat: string | null;
  z: number;
}

export const initialDesktopState: DesktopState = {
  wins: [],
  focusId: null,
  view: 'desktop',
  device: 'small',
  devCat: null,
  z: 10,
};

const createWindowId = (): string =>
  'w' + Date.now() + Math.random().toString(36).slice(2, 5);

const focusWindow = (state: DesktopState, id: string): DesktopState => {
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
    x: 70 + n * 34,
    y: 24 + n * 30,
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
    state.focusId === id
      ? wins.length
        ? wins[Math.max(0, index - 1)].id
        : null
      : state.focusId;

  return { ...state, wins, focusId };
};

const isWindowed = (state: DesktopState): boolean =>
  state.view === 'desktop' ||
  (state.view === 'device' && state.device === 'full');

const applyMode = (
  state: DesktopState,
  seedWindowIds?: SeedWindowIds,
): DesktopState => {
  const wins = state.wins.filter(
    (win) => win.id !== 'shell' && APPS[win.app],
  );
  const focusId =
    state.focusId && !wins.some((win) => win.id === state.focusId)
      ? null
      : state.focusId;
  let next = { ...state, wins, focusId };

  if (isWindowed(next) && next.wins.length === 0) {
    next = launchApp(next, 'control', seedWindowIds?.[0]);
    next = launchApp(next, 'telemetry', seedWindowIds?.[1]);
  }

  return next;
};

const hydrateState = (
  state: DesktopState,
  hydration: DesktopHydration | null,
): DesktopState => {
  if (!hydration) return state;

  let next = state;
  if (hydration.view) next = { ...next, view: hydration.view };
  if (hydration.device) next = { ...next, device: hydration.device };
  if (hydration.devCat !== undefined) {
    next = { ...next, devCat: hydration.devCat };
  }
  if (typeof hydration.z === 'number') next = { ...next, z: hydration.z };
  if (Array.isArray(hydration.wins)) {
    next = {
      ...next,
      wins: hydration.wins.filter((win) => APPS[win.app]),
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
    (
      state,
      { state: hydration, seedWindowIds, normalise = true },
    ): DesktopState => {
      const hydrated = hydrateState(state, hydration);
      return normalise ? applyMode(hydrated, seedWindowIds) : hydrated;
    },
  ),
  on(desktopActions.launchApp, (state, { appId, windowId }) =>
    launchApp(state, appId, windowId),
  ),
  on(desktopActions.focusWindow, (state, { id }) => focusWindow(state, id)),
  on(desktopActions.closeWindow, (state, { id }) => closeWindow(state, id)),
  on(desktopActions.minimiseWindow, (state, { id, delayMs = 0 }) => {
    if (delayMs > 0) {
      return {
        ...state,
        wins: state.wins.map((win) =>
          win.id === id ? { ...win, minimizing: true } : win,
        ),
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
    wins: state.wins.map((win) =>
      win.id === id ? { ...win, x, y } : win,
    ),
  })),
  on(desktopActions.resizeWindow, (state, { id, w, h }) => ({
    ...state,
    wins: state.wins.map((win) =>
      win.id === id ? { ...win, w, h } : win,
    ),
  })),
  on(desktopActions.setSub, (state, { id, sub }) => ({
    ...state,
    wins: state.wins.map((win) =>
      win.id === id ? { ...win, sub } : win,
    ),
  })),
  on(desktopActions.setSysTab, (state, { id, systab }) => ({
    ...state,
    wins: state.wins.map((win) =>
      win.id === id ? { ...win, systab } : win,
    ),
  })),
  on(desktopActions.setView, (state, { view, seedWindowIds }) =>
    applyMode({ ...state, view }, seedWindowIds),
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
  extraSelectors: ({
    selectWins,
    selectFocusId,
    selectView,
    selectDevice,
    selectZ,
  }) => {
    const selectWindowed = createSelector(
      selectView,
      selectDevice,
      (view, device) =>
        view === 'desktop' || (view === 'device' && device === 'full'),
    );
    const selectRenderWins = createSelector(
      selectWins,
      selectWindowed,
      selectFocusId,
      (wins, windowed, focusId) => {
        const renderable = wins.filter((win) => !win.group);
        if (windowed) return renderable;
        const active = focusId
          ? renderable.find((win) => win.id === focusId)
          : null;
        return active ? [active] : [];
      },
    );
    const selectHomeVisible = createSelector(
      selectView,
      selectDevice,
      selectRenderWins,
      (view, device, renderWins) =>
        (view === 'shell' || (view === 'device' && device !== 'full')) &&
        renderWins.length === 0,
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

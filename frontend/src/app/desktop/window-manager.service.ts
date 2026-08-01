import { InjectionToken, Injectable, Signal, WritableSignal, inject } from '@angular/core';
import { DOCUMENT } from '@angular/common';
import { Store, createSelector } from '@ngrx/store';
import { AppDef, DESKTOP_DATA, DeviceSize, ViewMode, Win } from './desktop.data';
import { desktopActions, DesktopHydration } from '../store/desktop.actions';
import {
  selectDevCat,
  selectDevice,
  selectFocusId,
  selectHomeVisible,
  selectNextZ,
  selectOpenWins,
  selectRenderWins,
  selectView,
  selectWindowed,
  selectWins,
} from '../store/desktop.reducer';

const createWindowId = (): string => 'w' + Date.now() + Math.random().toString(36).slice(2, 5);

export interface DesktopViewport {
  readonly width: number;
  readonly height: number;
}

export type DesktopViewportReader = () => DesktopViewport;

export const DESKTOP_VIEWPORT = new InjectionToken<DesktopViewportReader>('DESKTOP_VIEWPORT', {
  providedIn: 'root',
  factory: () => {
    const document = inject(DOCUMENT);
    return () => ({
      width: document.documentElement.clientWidth || document.defaultView?.innerWidth || 1_280,
      height: document.documentElement.clientHeight || document.defaultView?.innerHeight || 800,
    });
  },
});

const MINIMUM_VISIBLE_TITLE_WIDTH = 96;
const TITLE_BAR_HEIGHT = 34;

const selectFacadeWins = createSelector(selectWins, (wins): Win[] =>
  wins.map((win) => ({
    ...win,
    prev: win.prev ? { ...win.prev } : undefined,
  })),
);

/**
 * Compatibility facade for the desktop shell and app views.
 *
 * NgRx owns all durable window state. Public signals retain the service's
 * original shape so existing consumers remain unchanged; legacy set/update
 * calls dispatch a non-normalising hydrate patch instead of creating a second
 * writable state source.
 */
@Injectable({ providedIn: 'root' })
export class WindowManagerService {
  private readonly store = inject(Store);
  private readonly data = inject(DESKTOP_DATA);
  private readonly viewport = inject(DESKTOP_VIEWPORT);

  readonly view = this.writableStoreSignal(this.store.selectSignal(selectView), (view) => ({
    view,
  }));
  readonly device = this.writableStoreSignal(this.store.selectSignal(selectDevice), (device) => ({
    device,
  }));
  readonly devCat = this.writableStoreSignal(this.store.selectSignal(selectDevCat), (devCat) => ({
    devCat,
  }));
  readonly wins = this.writableStoreSignal(this.store.selectSignal(selectFacadeWins), (wins) => ({
    wins,
  }));
  readonly focusId = this.writableStoreSignal(
    this.store.selectSignal(selectFocusId),
    (focusId) => ({ focusId }),
  );

  readonly windowed = this.store.selectSignal(selectWindowed);
  readonly renderWins = this.store.selectSignal(selectRenderWins);
  readonly homeVisible = this.store.selectSignal(selectHomeVisible);
  readonly openWins = this.store.selectSignal(selectOpenWins);
  private readonly selectedNextZ = this.store.selectSignal(selectNextZ);

  readonly apps = this.data.apps;
  readonly order = this.data.order;
  readonly categories = this.data.categories;

  app(id: string): AppDef | undefined {
    return this.apps[id];
  }

  reconcileHydration(state: DesktopHydration): DesktopHydration {
    if (!state.wins) return state;
    const measured = this.viewport();
    const width = Math.max(320, Math.floor(measured.width));
    const height = Math.max(240, Math.floor(measured.height));
    return {
      ...state,
      wins: state.wins.map((window) => {
        const app = this.app(window.app);
        if (window.max) {
          return {
            ...window,
            x: 0,
            y: 0,
            w: width,
            h: height,
            prev: {
              x: 70,
              y: 24,
              w: app?.w ?? Math.min(window.w, width),
              h: app?.h ?? Math.min(window.h, height),
            },
          };
        }
        return {
          ...window,
          x: clamp(
            window.x,
            MINIMUM_VISIBLE_TITLE_WIDTH - window.w,
            width - MINIMUM_VISIBLE_TITLE_WIDTH,
          ),
          y: clamp(window.y, 0, height - TITLE_BAR_HEIGHT),
        };
      }),
    };
  }

  setView(view: ViewMode): void {
    this.store.dispatch(
      desktopActions.setView({
        view,
        seedWindowIds: [createWindowId()],
        viewport: this.viewport(),
      }),
    );
  }

  setDevice(device: DeviceSize): void {
    this.store.dispatch(desktopActions.setDevice({ device }));
  }

  toggleDevCat(id: string): void {
    this.store.dispatch(desktopActions.toggleDevCat({ id }));
  }

  launch(appId: string): void {
    this.store.dispatch(
      desktopActions.launchApp({
        appId,
        windowId: createWindowId(),
        viewport: this.viewport(),
      }),
    );
  }

  focus(id: string): void {
    this.store.dispatch(desktopActions.focusWindow({ id }));
  }

  close(id: string): void {
    this.store.dispatch(desktopActions.closeWindow({ id }));
  }

  minimise(id: string, delayMs = 0): void {
    this.store.dispatch(desktopActions.minimiseWindow({ id, delayMs }));
  }

  maximise(id: string, bounds?: { w: number; h: number }): void {
    this.store.dispatch(desktopActions.maximiseWindow({ id, bounds }));
  }

  move(id: string, x: number, y: number): void {
    this.store.dispatch(desktopActions.moveWindow({ id, x, y }));
  }

  resize(id: string, w: number, h: number): void {
    this.store.dispatch(desktopActions.resizeWindow({ id, w, h }));
  }

  setSub(id: string, sub: string): void {
    this.store.dispatch(desktopActions.setSub({ id, sub }));
  }

  setSysTab(id: string, systab: string): void {
    this.store.dispatch(desktopActions.setSysTab({ id, systab }));
  }

  clear(): void {
    this.store.dispatch(desktopActions.clear());
  }

  goHome(): void {
    this.store.dispatch(desktopActions.goHome());
  }

  applyMode(): void {
    this.setView(this.view());
  }

  nextZ(): number {
    const z = this.selectedNextZ();
    this.patch({ z });
    return z;
  }

  /** Existing shell persistence calls remain valid; the effect performs I/O. */
  persist(): void {
    this.patch({ wins: this.wins() });
  }

  private patch(state: DesktopHydration): void {
    this.store.dispatch(desktopActions.hydrate({ state, normalise: false }));
  }

  private writableStoreSignal<T>(
    selected: Signal<T>,
    hydration: (value: T) => DesktopHydration,
  ): WritableSignal<T> {
    const writable = selected as WritableSignal<T>;
    writable.set = (value: T) => this.patch(hydration(value));
    writable.update = (update: (value: T) => T) => writable.set(update(selected()));
    writable.asReadonly = () => selected;
    return writable;
  }
}

function clamp(value: number, minimum: number, maximum: number): number {
  return Math.min(maximum, Math.max(minimum, value));
}

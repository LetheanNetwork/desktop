import {
  Injectable,
  Signal,
  WritableSignal,
  inject,
} from '@angular/core';
import { Store, createSelector } from '@ngrx/store';
import {
  AppDef,
  DESKTOP_DATA,
  DeviceSize,
  ViewMode,
  Win,
} from './desktop.data';
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

const createWindowId = (): string =>
  'w' + Date.now() + Math.random().toString(36).slice(2, 5);

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

  readonly view = this.writableStoreSignal(
    this.store.selectSignal(selectView),
    (view) => ({ view }),
  );
  readonly device = this.writableStoreSignal(
    this.store.selectSignal(selectDevice),
    (device) => ({ device }),
  );
  readonly devCat = this.writableStoreSignal(
    this.store.selectSignal(selectDevCat),
    (devCat) => ({ devCat }),
  );
  readonly wins = this.writableStoreSignal(
    this.store.selectSignal(selectFacadeWins),
    (wins) => ({ wins }),
  );
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

  setView(view: ViewMode): void {
    this.store.dispatch(
      desktopActions.setView({
        view,
        seedWindowIds: [createWindowId(), createWindowId()],
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
      desktopActions.launchApp({ appId, windowId: createWindowId() }),
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
    this.store.dispatch(
      desktopActions.hydrate({ state, normalise: false }),
    );
  }

  private writableStoreSignal<T>(
    selected: Signal<T>,
    hydration: (value: T) => DesktopHydration,
  ): WritableSignal<T> {
    const writable = selected as WritableSignal<T>;
    writable.set = (value: T) => this.patch(hydration(value));
    writable.update = (update: (value: T) => T) =>
      writable.set(update(selected()));
    writable.asReadonly = () => selected;
    return writable;
  }
}

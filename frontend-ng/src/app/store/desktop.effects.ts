import { Injectable, afterNextRender, inject } from '@angular/core';
import { Actions, createEffect, ofType } from '@ngrx/effects';
import { Store } from '@ngrx/store';
import {
  concat,
  filter,
  map,
  mergeMap,
  of,
  ReplaySubject,
  take,
  tap,
  timer,
  withLatestFrom,
} from 'rxjs';
import { desktopActions, DesktopHydration } from './desktop.actions';
import { DesktopState, selectDesktopState } from './desktop.reducer';
import { StorageService } from './storage.service';

const STORE_KEY = 'lthn.desktop';

const INITIAL_WINDOW_IDS = ['w-initial-control', 'w-initial-telemetry'] as const;

const mutatingActions = [
  desktopActions.hydrate,
  desktopActions.launchApp,
  desktopActions.focusWindow,
  desktopActions.closeWindow,
  desktopActions.minimiseWindow,
  desktopActions.maximiseWindow,
  desktopActions.moveWindow,
  desktopActions.resizeWindow,
  desktopActions.setSub,
  desktopActions.setSysTab,
  desktopActions.setView,
  desktopActions.setDevice,
  desktopActions.toggleDevCat,
  desktopActions.goHome,
  desktopActions.clear,
] as const;

@Injectable()
export class DesktopEffects {
  private readonly actions$ = inject(Actions);
  private readonly store = inject(Store);
  private readonly storage = inject(StorageService);
  private readonly rendered$ = new ReplaySubject<void>(1);

  constructor() {
    afterNextRender(() => {
      this.rendered$.next();
      this.rendered$.complete();
    });
  }

  readonly hydrate$ = createEffect(() =>
    concat(
      of(
        desktopActions.hydrate({
          state: null,
          seedWindowIds: INITIAL_WINDOW_IDS,
          persist: false,
        }),
      ),
      this.rendered$.pipe(
        take(1),
        map(() =>
          desktopActions.hydrate({
            state: this.storage.read<DesktopHydration>(STORE_KEY),
            seedWindowIds: INITIAL_WINDOW_IDS,
            persist: false,
          }),
        ),
      ),
    ),
  );

  readonly completeMinimise$ = createEffect(() =>
    this.actions$.pipe(
      ofType(desktopActions.minimiseWindow),
      filter(({ delayMs = 0 }) => delayMs > 0),
      mergeMap(({ id, delayMs = 0 }) =>
        timer(delayMs).pipe(map(() => desktopActions.minimiseWindow({ id, delayMs: 0 }))),
      ),
    ),
  );

  readonly persist$ = createEffect(
    () =>
      this.actions$.pipe(
        ofType(...mutatingActions),
        filter((action) => action.type !== desktopActions.hydrate.type || action.persist !== false),
        withLatestFrom(this.store.select(selectDesktopState)),
        tap(([, state]) => this.persist(state)),
      ),
    { dispatch: false },
  );

  private persist(state: DesktopState): void {
    const stored = this.storage.read<Record<string, unknown>>(STORE_KEY);
    const shell = stored && typeof stored === 'object' ? stored : {};

    this.storage.write(STORE_KEY, {
      ...shell,
      view: state.view,
      device: state.device,
      focusId: state.focusId,
      z: state.z,
      wins: state.wins.map((win) => ({
        id: win.id,
        app: win.app,
        sub: win.sub,
        systab: win.systab,
        x: win.x,
        y: win.y,
        w: win.w,
        h: win.h,
        z: win.z,
        min: win.min,
        max: win.max,
        group: win.group,
      })),
    });
  }
}

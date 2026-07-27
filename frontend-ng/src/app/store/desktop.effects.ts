import { Injectable, afterNextRender, inject } from '@angular/core';
import { Actions, createEffect, ofType } from '@ngrx/effects';
import { Action, Store } from '@ngrx/store';
import {
  ReplaySubject,
  catchError,
  concat,
  concatMap,
  debounce,
  defer,
  filter,
  from,
  map,
  mergeMap,
  of,
  switchMap,
  take,
  timer,
  withLatestFrom,
} from 'rxjs';
import {
  DesktopShellSessionSnapshot,
  DesktopStateBridgeService,
  desktopHydrationFromSession,
  desktopSessionFromState,
  isDesktopStateConflict,
  parseLegacyDesktopSession,
} from '../desktop/desktop-state-bridge.service';
import { WindowManagerService } from '../desktop/window-manager.service';
import { DesktopHydration, SeedWindowIds, desktopActions } from './desktop.actions';
import {
  DesktopState,
  desktopReducer,
  initialDesktopState,
  selectDesktopState,
} from './desktop.reducer';
import { StorageService } from './storage.service';

const STORE_KEY = 'lthn.desktop';
const GEOMETRY_SAVE_DELAY_MS = 150;
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

const continuousMutationTypes = new Set<string>([
  desktopActions.hydrate.type,
  desktopActions.moveWindow.type,
  desktopActions.resizeWindow.type,
]);

@Injectable()
export class DesktopEffects {
  private readonly actions$ = inject(Actions);
  private readonly store = inject(Store);
  private readonly storage = inject(StorageService);
  private readonly bridge = inject(DesktopStateBridgeService);
  private readonly windows = inject(WindowManagerService);
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
        map(() => desktopActions.loadSession()),
      ),
    ),
  );

  readonly loadSession$ = createEffect(() =>
    this.actions$.pipe(
      ofType(desktopActions.loadSession),
      switchMap(() =>
        from(this.loadSession()).pipe(
          catchError((error: unknown) =>
            of(
              isDesktopStateConflict(error)
                ? desktopActions.loadSession()
                : desktopActions.loadSessionFailure({
                    error: messageFor(error),
                  }),
            ),
          ),
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

  readonly requestSave$ = createEffect(() =>
    this.actions$.pipe(
      ofType(...mutatingActions),
      filter(shouldPersistMutation),
      debounce((action) =>
        continuousMutationTypes.has(action.type) ? timer(GEOMETRY_SAVE_DELAY_MS) : of(0),
      ),
      withLatestFrom(this.store.select(selectDesktopState)),
      filter(([, state]) => state.persistence === 'ready'),
      map(() => desktopActions.saveSessionRequested()),
    ),
  );

  readonly saveSession$ = createEffect(() =>
    this.actions$.pipe(
      ofType(desktopActions.saveSessionRequested),
      concatMap(() =>
        this.store.select(selectDesktopState).pipe(
          take(1),
          filter((state) => state.persistence === 'ready'),
          concatMap((state) =>
            defer(() =>
              this.bridge.saveShellSession(
                state.persistenceRevision,
                desktopSessionFromState(state, true),
              ),
            ).pipe(
              map((snapshot) =>
                desktopActions.saveSessionSuccess({
                  revision: snapshot.revision,
                  migratedBrowserState: snapshot.session.migratedBrowserState,
                }),
              ),
              catchError((error: unknown) =>
                of(
                  isDesktopStateConflict(error)
                    ? desktopActions.loadSession()
                    : desktopActions.saveSessionFailure({
                        error: messageFor(error),
                      }),
                ),
              ),
            ),
          ),
        ),
      ),
    ),
  );

  private async loadSession(): Promise<Action> {
    let snapshot = await this.bridge.loadShellSession();
    if (!this.bridge.isOffline() && !snapshot.session.migratedBrowserState) {
      const legacy = parseLegacyDesktopSession(this.storage.read<unknown>(STORE_KEY));
      if (legacy) {
        const normalised = normalisedLegacyState(
          this.windows.reconcileHydration(desktopHydrationFromSession(legacy)),
          INITIAL_WINDOW_IDS,
        );
        snapshot = await this.bridge.saveShellSession(
          snapshot.revision,
          desktopSessionFromState(normalised, true),
        );
        this.storage.remove(STORE_KEY);
      }
    }
    return this.loadedAction(snapshot);
  }

  private loadedAction(snapshot: DesktopShellSessionSnapshot): Action {
    return desktopActions.loadSessionSuccess({
      state: this.windows.reconcileHydration(desktopHydrationFromSession(snapshot.session)),
      revision: snapshot.revision,
      migratedBrowserState: snapshot.session.migratedBrowserState,
      seedWindowIds: INITIAL_WINDOW_IDS,
    });
  }
}

function normalisedLegacyState(
  hydration: DesktopHydration,
  seedWindowIds: SeedWindowIds,
): DesktopState {
  return desktopReducer(
    initialDesktopState,
    desktopActions.hydrate({
      state: hydration,
      seedWindowIds,
    }),
  );
}

function shouldPersistMutation(action: ReturnType<(typeof mutatingActions)[number]>): boolean {
  if (
    action.type === desktopActions.hydrate.type &&
    'persist' in action &&
    action.persist === false
  ) {
    return false;
  }
  if (
    action.type === desktopActions.minimiseWindow.type &&
    'delayMs' in action &&
    (action.delayMs ?? 0) > 0
  ) {
    return false;
  }
  return true;
}

function messageFor(error: unknown): string {
  if (error instanceof Error && error.message) return error.message;
  if (typeof error === 'string' && error) return error;
  return 'The desktop session is unavailable.';
}

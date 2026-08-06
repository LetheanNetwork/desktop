import { Injectable, afterNextRender, inject } from '@angular/core';
import { Actions, createEffect, ofType } from '@ngrx/effects';
import { Action, Store } from '@ngrx/store';
import {
  EMPTY,
  Observable,
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
  tap,
  timer,
  withLatestFrom,
} from 'rxjs';
import { DOCK_APP_EVENT } from '../desktop/desktop-app-window-bridge.service';
import { APPS } from '../desktop/desktop-catalogue.data';
import { DESKTOP_HOST_EVENTS } from '../desktop/desktop-host-intent.service';
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
import { SOLO_APP_WINDOW } from './solo-window';
import { StorageService } from './storage.service';

const STORE_KEY = 'lthn.desktop';
const GEOMETRY_SAVE_DELAY_MS = 150;
// A pane is one route segment, the same shape Go's tear-off validation accepts.
const SAFE_PANE = /^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$/u;
// Stable id for the one window a fresh desktop opens with, so a hydration
// replay produces the same window rather than a new one each time.
const INITIAL_WINDOW_IDS = ['w-initial-welcome'] as const;

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
  // A torn-off application's own window reads one application and authors
  // nothing: it neither loads nor saves the shell session, so the window the
  // shell just handed over cannot be written back into the shell.
  private readonly solo = inject(SOLO_APP_WINDOW);
  private readonly hostEvents = inject(DESKTOP_HOST_EVENTS);
  private readonly rendered$ = new ReplaySubject<void>(1);

  constructor() {
    afterNextRender(() => {
      this.rendered$.next();
      this.rendered$.complete();
    });
  }

  readonly hydrate$ = createEffect(() =>
    this.solo
      ? // defer, not the EMPTY singleton: createEffect brands the observable
        // it is given, and a shared instance cannot be branded twice.
        defer(() => EMPTY)
      : concat(
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

  /**
   * A torn-off application asking to come home.
   *
   * The solo window emits nothing into this session itself — it cannot, the
   * shell is the session's only author — so it asks Go, Go validates and
   * broadcasts, and the adoption happens here, in the window that owns the
   * state. The broadcast reaches every window including the one that asked,
   * which is why a solo document ignores it: adopting there would recreate the
   * loop the tear-off was written to break.
   */
  readonly adoptDockedApp$ = createEffect(
    () =>
      this.solo
        ? defer(() => EMPTY)
        : defer(
            () =>
              new Observable<unknown>((subscriber) =>
                this.hostEvents.on(DOCK_APP_EVENT, (payload) => subscriber.next(payload)),
              ),
          ).pipe(
            map(dockRequestFrom),
            filter((request): request is DockRequest => request !== null),
            tap(({ app, pane }) => this.windows.dock(app, pane)),
          ),
    { dispatch: false },
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
      filter(() => !this.solo),
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

interface DockRequest {
  readonly app: string;
  readonly pane: string;
}

/**
 * The dock request, or null.
 *
 * Go has already checked the application against the tear-off whitelist, but
 * an event is a renderer-reachable surface and every one of those is validated
 * where it lands. An unknown application or a pane that is not a single safe
 * segment adopts nothing.
 */
function dockRequestFrom(payload: unknown): DockRequest | null {
  if (!payload || typeof payload !== 'object' || Array.isArray(payload)) return null;
  const record = payload as Record<string, unknown>;
  const app = record['app'];
  if (typeof app !== 'string' || !Object.hasOwn(APPS, app)) return null;
  const pane = record['pane'];
  if (pane === undefined || pane === null || pane === '') return { app, pane: '' };
  if (typeof pane !== 'string' || !SAFE_PANE.test(pane)) return null;
  return { app, pane };
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

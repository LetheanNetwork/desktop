import { Injectable, inject } from '@angular/core';
import { toObservable } from '@angular/core/rxjs-interop';
import { Actions, createEffect, ofType } from '@ngrx/effects';
import { Store } from '@ngrx/store';
import {
  catchError,
  concatMap,
  distinctUntilChanged,
  filter,
  from,
  map,
  of,
  pairwise,
  switchMap,
  tap,
  withLatestFrom,
} from 'rxjs';
import { ConnectionManagerService } from '../connection-manager.service';
import { DesktopControlsBridgeService } from '../desktop/desktop-controls-bridge.service';
import { PreferencesService } from '../desktop/preferences.service';
import { desktopControlsActions } from './desktop-controls.actions';
import {
  DesktopControlChange,
  DesktopControlSnapshot,
  DesktopControlsChangeNotice,
} from './desktop-controls.models';
import { desktopControlsFeature, selectHasDirtyDesktopControls } from './desktop-controls.reducer';

@Injectable()
export class DesktopControlsEffects {
  private readonly actions$ = inject(Actions);
  private readonly bridge = inject(DesktopControlsBridgeService);
  private readonly connection = inject(ConnectionManagerService);
  private readonly preferences = inject(PreferencesService);
  private readonly store = inject(Store);
  private readonly connectionState$ = toObservable(this.connection.state);

  readonly initialise$ = createEffect(() => of(desktopControlsActions.load()));

  readonly load$ = createEffect(() =>
    this.actions$.pipe(
      ofType(desktopControlsActions.load),
      switchMap(() =>
        from(this.bridge.settings()).pipe(
          map((snapshot) => desktopControlsActions.loadSuccess({ snapshot })),
          catchError((error: unknown) =>
            of(
              desktopControlsActions.loadFailure({
                error: messageFor(error),
              }),
            ),
          ),
        ),
      ),
    ),
  );

  readonly applyDraft$ = createEffect(() =>
    this.actions$.pipe(
      ofType(desktopControlsActions.applyDraft),
      concatMap(({ changes }) =>
        from(this.bridge.setMany(changes)).pipe(
          map((snapshot) =>
            desktopControlsActions.applyDraftSuccess({
              snapshot,
              restartRequired: restartRequiredLabels(snapshot, changes),
            }),
          ),
          catchError((error: unknown) =>
            of(
              desktopControlsActions.applyDraftFailure({
                error: messageFor(error),
              }),
            ),
          ),
        ),
      ),
    ),
  );

  readonly applyRendererControls$ = createEffect(
    () =>
      this.actions$.pipe(
        ofType(desktopControlsActions.loadSuccess, desktopControlsActions.applyDraftSuccess),
        tap(({ snapshot }) => this.preferences.applySnapshot(snapshot)),
      ),
    { dispatch: false },
  );

  readonly reconcileChanges$ = createEffect(() =>
    this.bridge.changes().pipe(
      distinctUntilChanged(
        (previous, current) => previous.revision !== null && previous.revision === current.revision,
      ),
      withLatestFrom(
        this.store.select(desktopControlsFeature.selectRevision),
        this.store.select(selectHasDirtyDesktopControls),
        this.store.select(desktopControlsFeature.selectSaving),
      ),
      filter(
        ([notice, revision, , saving]) =>
          !saving && (notice.revision === null || notice.revision !== revision),
      ),
      map(([notice, , dirty]) => reconcileNotice(notice, dirty)),
    ),
  );

  readonly reconnect$ = createEffect(() =>
    this.connectionState$.pipe(
      pairwise(),
      filter(
        ([previous, current]) =>
          (previous === 'disconnected' || previous === 'reconnecting') && current === 'connected',
      ),
      withLatestFrom(
        this.store.select(selectHasDirtyDesktopControls),
        this.store.select(desktopControlsFeature.selectSaving),
      ),
      filter(([, , saving]) => !saving),
      map(([, dirty]) => reconcileNotice({ revision: null, keys: [], at: null }, dirty)),
    ),
  );

  readonly reloadExternalChange$ = createEffect(() =>
    this.actions$.pipe(
      ofType(desktopControlsActions.reloadExternalChange),
      map(() => desktopControlsActions.load()),
    ),
  );
}

function reconcileNotice(notice: DesktopControlsChangeNotice, dirty: boolean) {
  return dirty
    ? desktopControlsActions.externalChangePending({ notice })
    : desktopControlsActions.load();
}

function restartRequiredLabels(
  snapshot: DesktopControlSnapshot,
  changes: readonly DesktopControlChange[],
): readonly string[] {
  const changedKeys = new Set(changes.map(({ key }) => key));
  return snapshot.controls
    .filter(({ key, restartRequired }) => restartRequired && changedKeys.has(key))
    .map(({ label }) => label);
}

function messageFor(error: unknown): string {
  if (error instanceof Error && error.message) return error.message;
  if (typeof error === 'string' && error) return error;
  return 'The desktop settings could not be saved.';
}

import { Injectable, inject } from '@angular/core';
import { Actions, createEffect, ofType } from '@ngrx/effects';
import { catchError, concatMap, from, map, of, switchMap, tap } from 'rxjs';
import { DesktopControlsBridgeService } from '../desktop/desktop-controls-bridge.service';
import { PreferencesService } from '../desktop/preferences.service';
import { desktopControlsActions } from './desktop-controls.actions';
import { DesktopControlChange, DesktopControlSnapshot } from './desktop-controls.models';

@Injectable()
export class DesktopControlsEffects {
  private readonly actions$ = inject(Actions);
  private readonly bridge = inject(DesktopControlsBridgeService);
  private readonly preferences = inject(PreferencesService);

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

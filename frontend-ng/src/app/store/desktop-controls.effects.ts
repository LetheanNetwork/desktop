import { Injectable, inject } from '@angular/core';
import { Actions, createEffect, ofType } from '@ngrx/effects';
import { catchError, concatMap, from, map, of, switchMap, tap } from 'rxjs';
import { DesktopControlsBridgeService } from '../desktop/desktop-controls-bridge.service';
import { PreferencesService } from '../desktop/preferences.service';
import { desktopControlsActions } from './desktop-controls.actions';
import { DesktopControlSnapshot } from './desktop-controls.models';

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

  readonly setControl$ = createEffect(() =>
    this.actions$.pipe(
      ofType(desktopControlsActions.setControl),
      concatMap(({ key, value }) =>
        from(this.bridge.set(key, value)).pipe(
          map((snapshot) => desktopControlsActions.setControlSuccess({ key, snapshot })),
          catchError((error: unknown) =>
            of(
              desktopControlsActions.setControlFailure({
                key,
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
        ofType(desktopControlsActions.loadSuccess, desktopControlsActions.setControlSuccess),
        tap(({ snapshot }) => this.applyRendererControls(snapshot)),
      ),
    { dispatch: false },
  );

  private applyRendererControls(snapshot: DesktopControlSnapshot): void {
    const interfaceTheme = snapshot.controls.find(
      ({ key }) => key === 'desktop.theme.interface',
    )?.value;
    if (interfaceTheme === 'dark' || interfaceTheme === 'light') {
      this.preferences.mode.set(interfaceTheme);
    }

    const reduceMotion = snapshot.controls.find(
      ({ key }) => key === 'desktop.theme.reduce_motion',
    )?.value;
    if (typeof reduceMotion === 'boolean') {
      this.preferences.reduceMotion.set(reduceMotion);
    }
  }
}

function messageFor(error: unknown): string {
  if (error instanceof Error && error.message) return error.message;
  if (typeof error === 'string' && error) return error;
  return 'The desktop setting could not be saved.';
}

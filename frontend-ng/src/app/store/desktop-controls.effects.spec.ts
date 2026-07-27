import { TestBed } from '@angular/core/testing';
import { Action } from '@ngrx/store';
import { provideMockActions } from '@ngrx/effects/testing';
import { firstValueFrom, Subject } from 'rxjs';
import { DesktopControlsBridgeService } from '../desktop/desktop-controls-bridge.service';
import { PreferencesService } from '../desktop/preferences.service';
import { desktopControlsActions } from './desktop-controls.actions';
import { DesktopControlsEffects } from './desktop-controls.effects';
import { DesktopControlSnapshot } from './desktop-controls.reducer';

const snapshot: DesktopControlSnapshot = {
  controls: [
    {
      key: 'desktop.theme.interface',
      group: 'Theme',
      label: 'Interface theme',
      description: 'Colour mode.',
      kind: 'select',
      value: 'light',
      defaultValue: 'dark',
      configured: true,
      live: true,
      restartRequired: false,
      choices: ['dark', 'light'],
    },
    {
      key: 'desktop.single_instance.enabled',
      group: 'Single instance',
      label: 'Single instance',
      description: 'Hand off later launches.',
      kind: 'toggle',
      value: false,
      defaultValue: true,
      configured: true,
      live: false,
      restartRequired: true,
    },
  ],
};

describe('DesktopControlsEffects', () => {
  let actions$: Subject<Action>;
  let bridge: {
    settings: ReturnType<typeof vi.fn>;
    setMany: ReturnType<typeof vi.fn>;
  };
  let prefs: { applySnapshot: ReturnType<typeof vi.fn> };
  let effects: DesktopControlsEffects;

  beforeEach(() => {
    actions$ = new Subject<Action>();
    bridge = {
      settings: vi.fn(),
      setMany: vi.fn(),
    };
    prefs = { applySnapshot: vi.fn() };
    TestBed.configureTestingModule({
      providers: [
        DesktopControlsEffects,
        provideMockActions(() => actions$),
        { provide: DesktopControlsBridgeService, useValue: bridge },
        { provide: PreferencesService, useValue: prefs },
      ],
    });
    effects = TestBed.inject(DesktopControlsEffects);
  });

  afterEach(() => actions$.complete());

  it('loads persisted controls when the application effects start', async () => {
    await expect(firstValueFrom(effects.initialise$)).resolves.toEqual(
      desktopControlsActions.load(),
    );
  });

  it('loads the effective catalogue through the selected connected or demo provider', async () => {
    bridge.settings.mockResolvedValue(snapshot);
    const result = firstValueFrom(effects.load$);

    actions$.next(desktopControlsActions.load());

    await expect(result).resolves.toEqual(desktopControlsActions.loadSuccess({ snapshot }));
  });

  it('writes one complete draft and returns one restart summary', async () => {
    bridge.setMany.mockResolvedValue(snapshot);
    const changes = [
      { key: 'desktop.theme.interface', value: 'light' as const },
      { key: 'desktop.single_instance.enabled', value: false },
    ];
    const result = firstValueFrom(effects.applyDraft$);

    actions$.next(desktopControlsActions.applyDraft({ changes }));

    await expect(result).resolves.toEqual(
      desktopControlsActions.applyDraftSuccess({
        snapshot,
        restartRequired: ['Single instance'],
      }),
    );
    expect(bridge.setMany).toHaveBeenCalledTimes(1);
    expect(bridge.setMany).toHaveBeenCalledWith(changes);
  });

  it('projects only committed snapshots into renderer preferences', () => {
    effects.applyRendererControls$.subscribe();

    actions$.next(desktopControlsActions.loadSuccess({ snapshot }));
    actions$.next(
      desktopControlsActions.editControl({
        key: 'desktop.theme.interface',
        value: 'dark',
      }),
    );

    expect(prefs.applySnapshot).toHaveBeenCalledTimes(1);
    expect(prefs.applySnapshot).toHaveBeenCalledWith(snapshot);
  });
});

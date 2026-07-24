import { signal } from '@angular/core';
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
  configPath: '/tmp/lthn.yaml',
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
      key: 'desktop.theme.reduce_motion',
      group: 'Theme',
      label: 'Reduce motion',
      description: 'Reduce transitions.',
      kind: 'toggle',
      value: true,
      defaultValue: false,
      configured: true,
      live: true,
      restartRequired: false,
    },
  ],
};

describe('DesktopControlsEffects', () => {
  let actions$: Subject<Action>;
  let bridge: {
    settings: ReturnType<typeof vi.fn>;
    set: ReturnType<typeof vi.fn>;
  };
  let prefs: {
    mode: ReturnType<typeof signal<'dark' | 'light'>>;
    reduceMotion: ReturnType<typeof signal<boolean>>;
  };
  let effects: DesktopControlsEffects;

  beforeEach(() => {
    actions$ = new Subject<Action>();
    bridge = {
      settings: vi.fn(),
      set: vi.fn(),
    };
    prefs = {
      mode: signal<'dark' | 'light'>('dark'),
      reduceMotion: signal(false),
    };
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

  it('loads the effective catalogue through the Go bridge', async () => {
    bridge.settings.mockResolvedValue(snapshot);
    const result = firstValueFrom(effects.load$);

    actions$.next(desktopControlsActions.load());

    await expect(result).resolves.toEqual(desktopControlsActions.loadSuccess({ snapshot }));
  });

  it('writes a control and returns the committed effective snapshot', async () => {
    bridge.set.mockResolvedValue(snapshot);
    const result = firstValueFrom(effects.setControl$);

    actions$.next(
      desktopControlsActions.setControl({
        key: 'desktop.theme.interface',
        value: 'light',
      }),
    );

    await expect(result).resolves.toEqual(
      desktopControlsActions.setControlSuccess({
        key: 'desktop.theme.interface',
        snapshot,
      }),
    );
    expect(bridge.set).toHaveBeenCalledWith('desktop.theme.interface', 'light');
  });

  it('applies renderer-owned theme controls live after a committed snapshot', () => {
    effects.applyRendererControls$.subscribe();

    actions$.next(desktopControlsActions.loadSuccess({ snapshot }));

    expect(prefs.mode()).toBe('light');
    expect(prefs.reduceMotion()).toBe(true);
  });
});

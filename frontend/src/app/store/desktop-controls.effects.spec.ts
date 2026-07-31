import { signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { Action } from '@ngrx/store';
import { provideMockActions } from '@ngrx/effects/testing';
import { MockStore, provideMockStore } from '@ngrx/store/testing';
import { firstValueFrom, Subject } from 'rxjs';
import { ConnectionManagerService, type ConnectionState } from '../connection-manager.service';
import { DesktopControlsBridgeService } from '../desktop/desktop-controls-bridge.service';
import { PreferencesService } from '../desktop/preferences.service';
import { desktopControlsActions } from './desktop-controls.actions';
import { DesktopControlsEffects } from './desktop-controls.effects';
import {
  DesktopControlSnapshot,
  desktopControlsFeature,
  selectHasDirtyDesktopControls,
} from './desktop-controls.reducer';

const snapshot: DesktopControlSnapshot = {
  revision: '1',
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
    changes: ReturnType<typeof vi.fn>;
  };
  let changes: Subject<{
    revision: string | null;
    keys: readonly string[];
    at: string | null;
  }>;
  let connectionState: ReturnType<typeof signal<ConnectionState>>;
  let store: MockStore;
  let prefs: { applySnapshot: ReturnType<typeof vi.fn> };
  let effects: DesktopControlsEffects;

  beforeEach(() => {
    actions$ = new Subject<Action>();
    changes = new Subject();
    connectionState = signal<ConnectionState>('disconnected');
    bridge = {
      settings: vi.fn(),
      setMany: vi.fn(),
      changes: vi.fn(() => changes.asObservable()),
    };
    prefs = { applySnapshot: vi.fn() };
    TestBed.configureTestingModule({
      providers: [
        DesktopControlsEffects,
        provideMockActions(() => actions$),
        provideMockStore({
          selectors: [
            { selector: desktopControlsFeature.selectRevision, value: '1' },
            { selector: selectHasDirtyDesktopControls, value: false },
            { selector: desktopControlsFeature.selectSaving, value: false },
          ],
        }),
        { provide: DesktopControlsBridgeService, useValue: bridge },
        { provide: PreferencesService, useValue: prefs },
        {
          provide: ConnectionManagerService,
          useValue: { state: connectionState.asReadonly() },
        },
      ],
    });
    store = TestBed.inject(MockStore);
    effects = TestBed.inject(DesktopControlsEffects);
  });

  afterEach(() => {
    actions$.complete();
    changes.complete();
  });

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

  it('refreshes a clean store when another context reports a different revision', async () => {
    const result = firstValueFrom(effects.reconcileChanges$);

    changes.next({
      revision: '2',
      keys: ['desktop.theme.interface'],
      at: '2026-07-31T12:00:00Z',
    });

    await expect(result).resolves.toEqual(desktopControlsActions.load());
  });

  it('ignores the current revision and events received while saving', () => {
    const received = vi.fn();
    const subscription = effects.reconcileChanges$.subscribe(received);

    changes.next({ revision: '1', keys: ['desktop.theme.interface'], at: null });
    store.overrideSelector(desktopControlsFeature.selectSaving, true);
    store.refreshState();
    changes.next({ revision: '2', keys: ['desktop.theme.interface'], at: null });

    expect(received).not.toHaveBeenCalled();
    subscription.unsubscribe();
  });

  it('keeps a dirty draft behind an explicit external-change notice', async () => {
    store.overrideSelector(selectHasDirtyDesktopControls, true);
    store.refreshState();
    const notice = {
      revision: '2',
      keys: ['desktop.theme.interface'],
      at: '2026-07-31T12:00:00Z',
    } as const;
    const result = firstValueFrom(effects.reconcileChanges$);

    changes.next(notice);

    await expect(result).resolves.toEqual(desktopControlsActions.externalChangePending({ notice }));
  });

  it('reloads a clean store after reconnecting', async () => {
    const result = firstValueFrom(effects.reconnect$);
    TestBed.flushEffects();

    connectionState.set('connected');
    TestBed.flushEffects();

    await expect(result).resolves.toEqual(desktopControlsActions.load());
  });

  it('preserves a dirty draft behind a reconnect notice', async () => {
    store.overrideSelector(selectHasDirtyDesktopControls, true);
    store.refreshState();
    const result = firstValueFrom(effects.reconnect$);
    TestBed.flushEffects();

    connectionState.set('connected');
    TestBed.flushEffects();

    await expect(result).resolves.toEqual(
      desktopControlsActions.externalChangePending({
        notice: { revision: null, keys: [], at: null },
      }),
    );
  });

  it('turns an explicit external reload into an authoritative load', async () => {
    const result = firstValueFrom(effects.reloadExternalChange$);

    actions$.next(desktopControlsActions.reloadExternalChange());

    await expect(result).resolves.toEqual(desktopControlsActions.load());
  });
});

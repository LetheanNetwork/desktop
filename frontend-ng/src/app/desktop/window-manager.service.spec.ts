import { TestBed } from '@angular/core/testing';
import { MockStore, provideMockStore } from '@ngrx/store/testing';
import { Win } from './desktop.data';
import { WindowManagerService } from './window-manager.service';
import { desktopActions } from '../store/desktop.actions';
import { DesktopState } from '../store/desktop.reducer';

const openWin: Win = {
  id: 'w1',
  app: 'control',
  sub: 'models',
  systab: '',
  x: 70,
  y: 24,
  w: 780,
  h: 560,
  z: 11,
  min: false,
  max: false,
};

const desktopState: DesktopState = {
  wins: [openWin],
  focusId: 'w1',
  view: 'shell',
  device: 'small',
  devCat: 'system',
  z: 11,
};

describe('WindowManagerService facade', () => {
  let facade: WindowManagerService;
  let store: MockStore;
  let dispatch: ReturnType<typeof vi.spyOn>;

  beforeEach(() => {
    TestBed.configureTestingModule({
      providers: [
        provideMockStore({
          initialState: { desktop: desktopState },
        }),
      ],
    });
    store = TestBed.inject(MockStore);
    dispatch = vi.spyOn(store, 'dispatch');
    facade = TestBed.inject(WindowManagerService);
  });

  it('exposes store selectors as signals and keeps desktop data access', () => {
    expect(facade.view()).toBe('shell');
    expect(facade.device()).toBe('small');
    expect(facade.devCat()).toBe('system');
    expect(facade.wins()).toEqual([openWin]);
    expect(facade.focusId()).toBe('w1');
    expect(facade.windowed()).toBe(false);
    expect(facade.renderWins()).toEqual([openWin]);
    expect(facade.homeVisible()).toBe(false);
    expect(facade.openWins()).toEqual([openWin]);
    expect(facade.app('control')?.title).toBe('Control');
    expect(facade.apps['control']).toBe(facade.app('control'));
    expect(facade.order).toContain('telemetry');
    expect(facade.categories.length).toBeGreaterThan(0);
  });

  it('dispatches every public write method to the matching desktop action', () => {
    facade.setView('desktop');
    expect(dispatch).toHaveBeenLastCalledWith(
      expect.objectContaining({
        type: '[Desktop] Set View',
        view: 'desktop',
        seedWindowIds: expect.arrayContaining([
          expect.stringMatching(/^w/),
          expect.stringMatching(/^w/),
        ]),
      }),
    );

    facade.setDevice('large');
    expect(dispatch).toHaveBeenLastCalledWith(
      desktopActions.setDevice({ device: 'large' }),
    );

    facade.toggleDevCat('developer');
    expect(dispatch).toHaveBeenLastCalledWith(
      desktopActions.toggleDevCat({ id: 'developer' }),
    );

    facade.launch('telemetry');
    expect(dispatch).toHaveBeenLastCalledWith(
      expect.objectContaining({
        type: '[Desktop] Launch App',
        appId: 'telemetry',
        windowId: expect.stringMatching(/^w/),
      }),
    );

    facade.focus('w1');
    expect(dispatch).toHaveBeenLastCalledWith(
      desktopActions.focusWindow({ id: 'w1' }),
    );
    facade.close('w1');
    expect(dispatch).toHaveBeenLastCalledWith(
      desktopActions.closeWindow({ id: 'w1' }),
    );
    facade.minimise('w1', 190);
    expect(dispatch).toHaveBeenLastCalledWith(
      desktopActions.minimiseWindow({ id: 'w1', delayMs: 190 }),
    );
    facade.maximise('w1', { w: 1000, h: 700 });
    expect(dispatch).toHaveBeenLastCalledWith(
      desktopActions.maximiseWindow({
        id: 'w1',
        bounds: { w: 1000, h: 700 },
      }),
    );
    facade.move('w1', 1, 2);
    expect(dispatch).toHaveBeenLastCalledWith(
      desktopActions.moveWindow({ id: 'w1', x: 1, y: 2 }),
    );
    facade.resize('w1', 3, 4);
    expect(dispatch).toHaveBeenLastCalledWith(
      desktopActions.resizeWindow({ id: 'w1', w: 3, h: 4 }),
    );
    facade.setSub('w1', 'runs');
    expect(dispatch).toHaveBeenLastCalledWith(
      desktopActions.setSub({ id: 'w1', sub: 'runs' }),
    );
    facade.setSysTab('w1', 'list');
    expect(dispatch).toHaveBeenLastCalledWith(
      desktopActions.setSysTab({ id: 'w1', systab: 'list' }),
    );
    facade.goHome();
    expect(dispatch).toHaveBeenLastCalledWith(desktopActions.goHome());
    facade.clear();
    expect(dispatch).toHaveBeenLastCalledWith(desktopActions.clear());
  });

  it('adapts legacy writable signal calls into non-normalising store hydration', () => {
    const grouped = { ...openWin, min: true, group: 'g1' };

    facade.wins.update(() => [grouped]);
    expect(dispatch).toHaveBeenLastCalledWith(
      desktopActions.hydrate({
        state: { wins: [grouped] },
        normalise: false,
      }),
    );

    facade.focusId.set(null);
    expect(dispatch).toHaveBeenLastCalledWith(
      desktopActions.hydrate({
        state: { focusId: null },
        normalise: false,
      }),
    );

    facade.devCat.set(null);
    expect(dispatch).toHaveBeenLastCalledWith(
      desktopActions.hydrate({
        state: { devCat: null },
        normalise: false,
      }),
    );
  });

  it('selects and allocates the next z, reapplies the current mode, and can request persistence', () => {
    expect(facade.nextZ()).toBe(12);
    expect(dispatch).toHaveBeenLastCalledWith(
      desktopActions.hydrate({
        state: { z: 12 },
        normalise: false,
      }),
    );

    facade.applyMode();
    expect(dispatch).toHaveBeenLastCalledWith(
      expect.objectContaining({
        type: '[Desktop] Set View',
        view: 'shell',
      }),
    );

    facade.persist();
    expect(dispatch).toHaveBeenLastCalledWith(
      desktopActions.hydrate({
        state: { wins: [openWin] },
        normalise: false,
      }),
    );
  });

  it('keeps legacy transient window mutation off frozen NgRx state and flushes it on persist', () => {
    const frozen = Object.freeze({
      ...openWin,
      snapState: 'left',
    }) as Win;
    store.setState({
      desktop: {
        ...desktopState,
        wins: [frozen],
      },
    });

    const viewWin = facade.wins()[0] as Win & { snapState: string | null };
    expect(() => {
      viewWin.snapState = null;
    }).not.toThrow();
    expect(frozen['snapState' as keyof Win]).toBe('left');

    facade.persist();
    expect(dispatch).toHaveBeenLastCalledWith(
      desktopActions.hydrate({
        state: {
          wins: [{ ...frozen, snapState: null } as Win],
        },
        normalise: false,
      }),
    );
  });
});

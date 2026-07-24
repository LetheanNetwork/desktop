import { desktopControlsActions } from './desktop-controls.actions';
import {
  DesktopControlSnapshot,
  desktopControlsReducer,
  initialDesktopControlsState,
  selectDesktopControlGroups,
} from './desktop-controls.reducer';

const snapshot: DesktopControlSnapshot = {
  configPath: '/Users/test/Lethean/conf/lthn.yaml',
  controls: [
    {
      key: 'desktop.theme.interface',
      group: 'Theme',
      label: 'Interface theme',
      description: 'Desktop colour mode.',
      kind: 'select',
      value: 'dark',
      defaultValue: 'dark',
      configured: false,
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
      value: true,
      defaultValue: true,
      configured: false,
      live: false,
      restartRequired: true,
    },
  ],
};

describe('desktop controls reducer', () => {
  it('loads the effective snapshot and groups controls in catalogue order', () => {
    const loading = desktopControlsReducer(
      initialDesktopControlsState,
      desktopControlsActions.load(),
    );
    const loaded = desktopControlsReducer(
      loading,
      desktopControlsActions.loadSuccess({ snapshot }),
    );

    expect(loaded).toMatchObject({
      ...snapshot,
      loading: false,
      error: null,
    });
    expect(selectDesktopControlGroups.projector(loaded.controls)).toEqual([
      { name: 'Theme', controls: [snapshot.controls[0]] },
      { name: 'Single instance', controls: [snapshot.controls[1]] },
    ]);
  });

  it('tracks a saving key and replaces it with the committed snapshot', () => {
    const saving = desktopControlsReducer(
      { ...initialDesktopControlsState, ...snapshot },
      desktopControlsActions.setControl({
        key: 'desktop.theme.interface',
        value: 'light',
      }),
    );
    expect(saving.savingKeys).toEqual(['desktop.theme.interface']);

    const committed: DesktopControlSnapshot = {
      ...snapshot,
      controls: [{ ...snapshot.controls[0], value: 'light', configured: true }],
    };
    const saved = desktopControlsReducer(
      saving,
      desktopControlsActions.setControlSuccess({
        key: 'desktop.theme.interface',
        snapshot: committed,
      }),
    );

    expect(saved.controls[0].value).toBe('light');
    expect(saved.savingKeys).toEqual([]);
  });

  it('retains the previous values and surfaces a failed write', () => {
    const failed = desktopControlsReducer(
      {
        ...initialDesktopControlsState,
        ...snapshot,
        savingKeys: ['desktop.theme.interface'],
      },
      desktopControlsActions.setControlFailure({
        key: 'desktop.theme.interface',
        error: 'could not persist',
      }),
    );

    expect(failed.controls).toEqual(snapshot.controls);
    expect(failed.savingKeys).toEqual([]);
    expect(failed.error).toBe('could not persist');
  });
});

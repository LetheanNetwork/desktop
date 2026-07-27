import { desktopControlsActions } from './desktop-controls.actions';
import {
  DesktopControlSnapshot,
  desktopControlsReducer,
  initialDesktopControlsState,
  selectDesktopControlGroups,
  selectDirtyDesktopControlChanges,
  selectDraftDesktopControls,
} from './desktop-controls.reducer';

const snapshot: DesktopControlSnapshot = {
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
  it('loads the committed snapshot and groups its draft in catalogue order', () => {
    const loading = desktopControlsReducer(
      initialDesktopControlsState,
      desktopControlsActions.load(),
    );
    const loaded = desktopControlsReducer(
      loading,
      desktopControlsActions.loadSuccess({ snapshot }),
    );

    expect(loaded.controls).toEqual(snapshot.controls);
    expect(loaded.draft).toEqual({});
    expect(loaded.loading).toBe(false);
    const draftControls = selectDraftDesktopControls.projector(loaded.controls, loaded.draft);
    expect(selectDesktopControlGroups.projector(draftControls)).toEqual([
      { name: 'Theme', controls: [snapshot.controls[0]] },
      { name: 'Single instance', controls: [snapshot.controls[1]] },
    ]);
  });

  it('edits only the draft and leaves committed values untouched', () => {
    const edited = desktopControlsReducer(
      { ...initialDesktopControlsState, controls: snapshot.controls },
      desktopControlsActions.editControl({
        key: 'desktop.theme.interface',
        value: 'light',
      }),
    );

    expect(edited.controls[0].value).toBe('dark');
    expect(edited.draft).toEqual({ 'desktop.theme.interface': 'light' });
    expect(selectDirtyDesktopControlChanges.projector(edited.controls, edited.draft)).toEqual([
      { key: 'desktop.theme.interface', value: 'light' },
    ]);
  });

  it('discards edits and resets the draft to catalogue defaults', () => {
    const edited = {
      ...initialDesktopControlsState,
      controls: [
        { ...snapshot.controls[0], value: 'light' },
        { ...snapshot.controls[1], value: false },
      ],
      draft: { 'desktop.theme.interface': 'dark' },
    };

    const discarded = desktopControlsReducer(edited, desktopControlsActions.discardDraft());
    expect(discarded.draft).toEqual({});

    const reset = desktopControlsReducer(edited, desktopControlsActions.resetDraft());
    expect(reset.draft).toEqual({
      'desktop.theme.interface': 'dark',
      'desktop.single_instance.enabled': true,
    });
  });

  it('commits one snapshot and reports restart-required changes once', () => {
    const saving = desktopControlsReducer(
      {
        ...initialDesktopControlsState,
        controls: snapshot.controls,
        draft: { 'desktop.single_instance.enabled': false },
      },
      desktopControlsActions.applyDraft({
        changes: [{ key: 'desktop.single_instance.enabled', value: false }],
      }),
    );
    expect(saving.saving).toBe(true);

    const committed: DesktopControlSnapshot = {
      controls: [snapshot.controls[0], { ...snapshot.controls[1], value: false, configured: true }],
    };
    const saved = desktopControlsReducer(
      saving,
      desktopControlsActions.applyDraftSuccess({
        snapshot: committed,
        restartRequired: ['Single instance'],
      }),
    );

    expect(saved.controls[1].value).toBe(false);
    expect(saved.draft).toEqual({});
    expect(saved.saving).toBe(false);
    expect(saved.restartSummary).toBe('Restart required for: Single instance.');
  });

  it('restores committed UI and keeps an accessible error after failure', () => {
    const failed = desktopControlsReducer(
      {
        ...initialDesktopControlsState,
        controls: snapshot.controls,
        draft: { 'desktop.theme.interface': 'light' },
        saving: true,
      },
      desktopControlsActions.applyDraftFailure({ error: 'could not persist' }),
    );

    expect(failed.controls).toEqual(snapshot.controls);
    expect(failed.draft).toEqual({});
    expect(failed.saving).toBe(false);
    expect(failed.error).toBe('could not persist');
  });
});

import { createFeature, createReducer, createSelector, on } from '@ngrx/store';
import { desktopControlsActions } from './desktop-controls.actions';
import {
  DesktopControl,
  DesktopControlChange,
  DesktopControlDraft,
  DesktopControlGroup,
  DesktopControlSnapshot,
  DesktopControlValue,
} from './desktop-controls.models';

export type { DesktopControlSnapshot } from './desktop-controls.models';

export interface DesktopControlsState {
  readonly controls: readonly DesktopControl[];
  readonly draft: DesktopControlDraft;
  readonly loading: boolean;
  readonly saving: boolean;
  readonly error: string | null;
  readonly restartSummary: string | null;
}

export const initialDesktopControlsState: DesktopControlsState = {
  controls: [],
  draft: {},
  loading: false,
  saving: false,
  error: null,
  restartSummary: null,
};

const applySnapshot = (
  state: DesktopControlsState,
  snapshot: DesktopControlSnapshot,
): DesktopControlsState => ({
  ...state,
  controls: snapshot.controls,
  draft: {},
  loading: false,
  saving: false,
  error: null,
});

export const desktopControlsReducer = createReducer(
  initialDesktopControlsState,
  on(desktopControlsActions.load, (state) => ({
    ...state,
    loading: true,
    error: null,
  })),
  on(desktopControlsActions.loadSuccess, (state, { snapshot }) => ({
    ...applySnapshot(state, snapshot),
    restartSummary: null,
  })),
  on(desktopControlsActions.loadFailure, (state, { error }) => ({
    ...state,
    loading: false,
    error,
  })),
  on(desktopControlsActions.editControl, (state, { key, value }) => {
    const control = state.controls.find((candidate) => candidate.key === key);
    if (!control || !acceptsValue(control, value)) return state;
    const draft = { ...state.draft };
    if (control.value === value) delete draft[key];
    else draft[key] = value;
    return {
      ...state,
      draft,
      error: null,
      restartSummary: null,
    };
  }),
  on(desktopControlsActions.discardDraft, (state) => ({
    ...state,
    draft: {},
    error: null,
    restartSummary: null,
  })),
  on(desktopControlsActions.resetDraft, (state) => ({
    ...state,
    draft: resetDraft(state.controls),
    error: null,
    restartSummary: null,
  })),
  on(desktopControlsActions.applyDraft, (state) => ({
    ...state,
    saving: true,
    error: null,
  })),
  on(desktopControlsActions.applyDraftSuccess, (state, { snapshot, restartRequired }) => ({
    ...applySnapshot(state, snapshot),
    restartSummary:
      restartRequired.length > 0 ? `Restart required for: ${restartRequired.join(', ')}.` : null,
  })),
  on(desktopControlsActions.applyDraftFailure, (state, { error }) => ({
    ...state,
    draft: {},
    saving: false,
    error,
  })),
);

const resetDraft = (controls: readonly DesktopControl[]): DesktopControlDraft => {
  const draft: Record<string, DesktopControlValue> = {};
  for (const control of controls) {
    if (control.value !== control.defaultValue) {
      draft[control.key] = control.defaultValue;
    }
  }
  return draft;
};

const acceptsValue = (control: DesktopControl, value: DesktopControlValue): boolean => {
  switch (control.kind) {
    case 'toggle':
      return typeof value === 'boolean';
    case 'number':
      return (
        typeof value === 'number' &&
        Number.isFinite(value) &&
        (control.minimum === undefined || value >= control.minimum) &&
        (control.maximum === undefined || value <= control.maximum)
      );
    case 'select':
      return (
        typeof value === 'string' &&
        value.length <= 256 &&
        (control.choices?.includes(value) ?? false)
      );
    case 'text':
      return typeof value === 'string' && value.length <= 2_048;
  }
};

const draftControls = (
  controls: readonly DesktopControl[],
  draft: DesktopControlDraft,
): readonly DesktopControl[] =>
  controls.map((control) =>
    Object.hasOwn(draft, control.key)
      ? { ...control, value: draft[control.key] as DesktopControlValue }
      : control,
  );

const dirtyChanges = (
  controls: readonly DesktopControl[],
  draft: DesktopControlDraft,
): readonly DesktopControlChange[] => {
  const changes: DesktopControlChange[] = [];
  for (const control of controls) {
    if (!Object.hasOwn(draft, control.key)) continue;
    const value = draft[control.key];
    if (value !== undefined && value !== control.value && acceptsValue(control, value)) {
      changes.push({ key: control.key, value });
    }
  }
  return changes;
};

const groupControls = (controls: readonly DesktopControl[]): readonly DesktopControlGroup[] => {
  const groups = new Map<string, DesktopControl[]>();
  for (const control of controls) {
    const group = groups.get(control.group);
    if (group) group.push(control);
    else groups.set(control.group, [control]);
  }
  return [...groups.entries()].map(([name, groupedControls]) => ({
    name,
    controls: groupedControls,
  }));
};

export const desktopControlsFeature = createFeature({
  name: 'desktopControls',
  reducer: desktopControlsReducer,
  extraSelectors: ({ selectControls, selectDraft }) => {
    const selectDraftDesktopControls = createSelector(selectControls, selectDraft, draftControls);
    const selectDirtyDesktopControlChanges = createSelector(
      selectControls,
      selectDraft,
      dirtyChanges,
    );
    return {
      selectDraftDesktopControls,
      selectDirtyDesktopControlChanges,
      selectHasDirtyDesktopControls: createSelector(
        selectDirtyDesktopControlChanges,
        (changes) => changes.length > 0,
      ),
      selectDesktopControlGroups: createSelector(selectDraftDesktopControls, groupControls),
    };
  },
});

export const selectDraftDesktopControls = desktopControlsFeature.selectDraftDesktopControls;
export const selectDirtyDesktopControlChanges =
  desktopControlsFeature.selectDirtyDesktopControlChanges;
export const selectHasDirtyDesktopControls = desktopControlsFeature.selectHasDirtyDesktopControls;
export const selectDesktopControlGroups = desktopControlsFeature.selectDesktopControlGroups;

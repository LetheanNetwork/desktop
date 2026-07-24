import { createFeature, createReducer, createSelector, on } from '@ngrx/store';
import { desktopControlsActions } from './desktop-controls.actions';
import {
  DesktopControl,
  DesktopControlGroup,
  DesktopControlSnapshot,
} from './desktop-controls.models';

export type { DesktopControlSnapshot } from './desktop-controls.models';

export interface DesktopControlsState {
  readonly controls: readonly DesktopControl[];
  readonly configPath: string;
  readonly loading: boolean;
  readonly savingKeys: readonly string[];
  readonly error: string | null;
}

export const initialDesktopControlsState: DesktopControlsState = {
  controls: [],
  configPath: '',
  loading: false,
  savingKeys: [],
  error: null,
};

const applySnapshot = (
  state: DesktopControlsState,
  snapshot: DesktopControlSnapshot,
): DesktopControlsState => ({
  ...state,
  controls: snapshot.controls,
  configPath: snapshot.configPath,
  loading: false,
  error: null,
});

export const desktopControlsReducer = createReducer(
  initialDesktopControlsState,
  on(desktopControlsActions.load, (state) => ({
    ...state,
    loading: true,
    error: null,
  })),
  on(desktopControlsActions.loadSuccess, (state, { snapshot }) => applySnapshot(state, snapshot)),
  on(desktopControlsActions.loadFailure, (state, { error }) => ({
    ...state,
    loading: false,
    error,
  })),
  on(desktopControlsActions.setControl, (state, { key }) => ({
    ...state,
    savingKeys: state.savingKeys.includes(key) ? state.savingKeys : [...state.savingKeys, key],
    error: null,
  })),
  on(desktopControlsActions.setControlSuccess, (state, { key, snapshot }) => ({
    ...applySnapshot(state, snapshot),
    savingKeys: state.savingKeys.filter((candidate) => candidate !== key),
  })),
  on(desktopControlsActions.setControlFailure, (state, { key, error }) => ({
    ...state,
    savingKeys: state.savingKeys.filter((candidate) => candidate !== key),
    error,
  })),
);

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
  extraSelectors: ({ selectControls }) => ({
    selectDesktopControlGroups: createSelector(selectControls, groupControls),
  }),
});

export const selectDesktopControlGroups = desktopControlsFeature.selectDesktopControlGroups;

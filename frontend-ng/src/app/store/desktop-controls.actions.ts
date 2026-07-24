import { createActionGroup, emptyProps, props } from '@ngrx/store';
import { DesktopControlSnapshot, DesktopControlValue } from './desktop-controls.models';

export const desktopControlsActions = createActionGroup({
  source: 'Desktop controls',
  events: {
    Load: emptyProps(),
    'Load success': props<{ snapshot: DesktopControlSnapshot }>(),
    'Load failure': props<{ error: string }>(),
    'Set control': props<{ key: string; value: DesktopControlValue }>(),
    'Set control success': props<{
      key: string;
      snapshot: DesktopControlSnapshot;
    }>(),
    'Set control failure': props<{ key: string; error: string }>(),
  },
});

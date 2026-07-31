import { createActionGroup, emptyProps, props } from '@ngrx/store';
import {
  DesktopControlChange,
  DesktopControlSnapshot,
  DesktopControlValue,
  DesktopControlsChangeNotice,
} from './desktop-controls.models';

export const desktopControlsActions = createActionGroup({
  source: 'Desktop controls',
  events: {
    Load: emptyProps(),
    'Load success': props<{ snapshot: DesktopControlSnapshot }>(),
    'Load failure': props<{ error: string }>(),
    'Edit control': props<{ key: string; value: DesktopControlValue }>(),
    'Discard draft': emptyProps(),
    'Reset draft': emptyProps(),
    'Apply draft': props<{ changes: readonly DesktopControlChange[] }>(),
    'Apply draft success': props<{
      snapshot: DesktopControlSnapshot;
      restartRequired: readonly string[];
    }>(),
    'Apply draft failure': props<{ error: string }>(),
    'External change pending': props<{ notice: DesktopControlsChangeNotice }>(),
    'Dismiss external change': emptyProps(),
    'Reload external change': emptyProps(),
  },
});

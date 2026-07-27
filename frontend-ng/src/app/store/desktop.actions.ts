import { createActionGroup, emptyProps, props } from '@ngrx/store';
import { DeviceSize, ViewMode, Win } from '../desktop/desktop.data';

export interface DesktopHydration {
  wins?: Win[];
  focusId?: string | null;
  view?: ViewMode;
  device?: DeviceSize;
  devCat?: string | null;
  z?: number;
}

export type SeedWindowIds = readonly [string, string];

export const desktopActions = createActionGroup({
  source: 'Desktop',
  events: {
    Hydrate: props<{
      state: DesktopHydration | null;
      seedWindowIds?: SeedWindowIds;
      normalise?: boolean;
      persist?: boolean;
    }>(),
    'Load Session': emptyProps(),
    'Load Session Success': props<{
      state: DesktopHydration;
      revision: number;
      migratedBrowserState: boolean;
      seedWindowIds?: SeedWindowIds;
    }>(),
    'Load Session Failure': props<{ error: string }>(),
    'Save Session Requested': emptyProps(),
    'Save Session Success': props<{
      revision: number;
      migratedBrowserState: boolean;
    }>(),
    'Save Session Failure': props<{ error: string }>(),
    'Launch App': props<{ appId: string; windowId?: string }>(),
    'Focus Window': props<{ id: string }>(),
    'Close Window': props<{ id: string }>(),
    'Minimise Window': props<{ id: string; delayMs?: number }>(),
    'Maximise Window': props<{ id: string; bounds?: { w: number; h: number } }>(),
    'Move Window': props<{ id: string; x: number; y: number }>(),
    'Resize Window': props<{ id: string; w: number; h: number }>(),
    'Set Sub': props<{ id: string; sub: string }>(),
    'Set Sys Tab': props<{ id: string; systab: string }>(),
    'Set View': props<{ view: ViewMode; seedWindowIds?: SeedWindowIds }>(),
    'Set Device': props<{ device: DeviceSize }>(),
    'Toggle Dev Cat': props<{ id: string }>(),
    'Go Home': emptyProps(),
    Clear: emptyProps(),
  },
});

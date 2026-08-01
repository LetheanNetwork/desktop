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

/**
 * Ids for the windows a fresh desktop opens with.
 *
 * One, because a fresh desktop opens on Welcome alone. It was a pair when the
 * seed was Control plus Telemetry; the tuple is kept rather than a bare string
 * so adding a second seeded window later is a type change the callers are made
 * to notice.
 */
export type SeedWindowIds = readonly [string];

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
    'Launch App': props<{
      appId: string;
      windowId?: string;
      // Where to centre the window. The reducer is pure and cannot measure the
      // screen, so the caller that can measure passes it in.
      viewport?: { width: number; height: number };
    }>(),
    'Focus Window': props<{ id: string }>(),
    'Close Window': props<{ id: string }>(),
    'Minimise Window': props<{ id: string; delayMs?: number }>(),
    'Maximise Window': props<{ id: string; bounds?: { w: number; h: number } }>(),
    'Move Window': props<{ id: string; x: number; y: number }>(),
    'Resize Window': props<{ id: string; w: number; h: number }>(),
    'Set Sub': props<{ id: string; sub: string }>(),
    'Set Sys Tab': props<{ id: string; systab: string }>(),
    'Set View': props<{
      view: ViewMode;
      seedWindowIds?: SeedWindowIds;
      viewport?: { width: number; height: number };
    }>(),
    'Set Device': props<{ device: DeviceSize }>(),
    'Toggle Dev Cat': props<{ id: string }>(),
    'Go Home': emptyProps(),
    Clear: emptyProps(),
  },
});

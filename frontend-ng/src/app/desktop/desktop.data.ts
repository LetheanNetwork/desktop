// ─────────────────────────────────────────────────────────────────────────
// desktop.data.ts — compatibility facade and shared desktop state.
//
// Static catalogues and shell fixtures live in focused typed data modules.
// This file keeps the existing public imports stable while owning the state
// shapes and defaults consumed across the desktop.
// ─────────────────────────────────────────────────────────────────────────
import { InjectionToken } from '@angular/core';
import { APPS, CATEGORIES, ORDER, type AppDef, type Category } from './desktop-catalogue.data';

export {
  APPS,
  CATEGORIES,
  CTRL_NAV,
  GAMES_NAV,
  ORDER,
  SETTINGS_NAV,
} from './desktop-catalogue.data';
export type { AppDef, AppNavItem, Category } from './desktop-catalogue.data';
export { CLOCKS, PACKAGE_STATUS_VARIANTS, PKGS } from './desktop-shell-fixtures.data';
export type {
  PackageFixture,
  PackageStatusVariant,
  WorldClockFixture,
} from './desktop-shell-fixtures.data';

// ── core types ──────────────────────────────────────────────────────────
export type ViewMode = 'desktop' | 'shell' | 'device';
export type DeviceSize = 'small' | 'large' | 'full';
export type WindowSnapState = 'top' | 'max' | 'left' | 'right' | 'tl' | 'tr' | 'bl' | 'br';

/** A live window instance. Owned by WindowManagerService, never by app views. */
export interface Win {
  id: string;
  app: string; // AppDef id
  sub: string; // child-view key (rail/tab within the app)
  systab?: string; // secondary tab (e.g. Files grid/list, System tab)
  x: number;
  y: number;
  w: number;
  h: number;
  z: number;
  min: boolean;
  max: boolean;
  prev?: { x: number; y: number; w: number; h: number }; // pre-maximise geometry
  snapState?: WindowSnapState | null;
  group?: string; // dock workspace group id
  minimizing?: boolean; // transient: minimise animation in flight
}

// ── shared content defaults ──────────────────────────────────────────────
export const TELEMETRY = {
  throughput: [28, 30, 26, 34, 31, 38, 35, 42, 38, 44, 40, 46],
  watts: [180, 176, 182, 190, 188, 195, 201, 198, 205, 199, 207, 210],
};

export const LANGS: [string, string][] = [
  ['en', 'English'],
  ['fr', 'Français'],
  ['de', 'Deutsch'],
  ['es', 'Español'],
  ['cy', 'Cymraeg'],
  ['ja', '日本語'],
];

/**
 * A production data service can replace this shared payload through
 * `DESKTOP_DATA`. Developer-panel fixtures live in `dev-panel.data.ts`;
 * shell-only fixtures live in `desktop-shell-fixtures.data.ts`.
 */
export interface DesktopData {
  apps: Record<string, AppDef>;
  order: string[];
  categories: Category[];
  langs: [string, string][];
  telemetry: typeof TELEMETRY;
}

export const DEFAULT_DESKTOP_DATA: DesktopData = {
  apps: APPS,
  order: ORDER,
  categories: CATEGORIES,
  langs: LANGS,
  telemetry: TELEMETRY,
};

/** Provide a live payload by overriding this token (e.g. via a route resolver). */
export const DESKTOP_DATA = new InjectionToken<DesktopData>('DESKTOP_DATA', {
  providedIn: 'root',
  factory: () => DEFAULT_DESKTOP_DATA,
});

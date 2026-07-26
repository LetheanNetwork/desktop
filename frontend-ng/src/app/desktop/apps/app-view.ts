// ─────────────────────────────────────────────────────────────────────────
// apps/app-view.ts — the dumb app-view contract + lazy route registry.
//
// Every application is a standalone, presentational component: it receives the
// live `Win` (for its sub/systab keys), navigates via the injected
// WindowManagerService, and renders. It does NOT know about the OS chrome or
// other apps — it can be dropped on a route, unit-tested, or previewed alone.
//
// The route tree resolves `win.app` → lazy component through APP_REGISTRY. To add
// an app: build a component implementing AppView, register its dynamic import
// here, and add its AppDef/category entry to desktop-catalogue.data.ts.
// ─────────────────────────────────────────────────────────────────────────
import { Type } from '@angular/core';
import { Win } from '../desktop.data';
import { SURFACE_APP_REGISTRY } from '../surfaces/surface-registry';

/** Implemented by every app-view. `win` is the only required input; apps read
 *  their sub/systab from it and call WindowManagerService to change them, so the
 *  service stays the single source of truth. */
export interface AppView {
  win: Win;
}

export type AppComponentLoader = () => Promise<Type<AppView>>;

const loadDevPanel: AppComponentLoader = () =>
  import('./dev-panel.app').then(({ DevPanelApp }) => DevPanelApp);

/** App id → dynamic component import used directly by the Angular route tree. */
export const APP_REGISTRY: Record<string, AppComponentLoader> = {
  control: () => import('./control.app').then(({ ControlApp }) => ControlApp),
  chat: () => import('./chat.app').then(({ ChatApp }) => ChatApp),
  telemetry: () => import('./telemetry.app').then(({ TelemetryApp }) => TelemetryApp),
  activity: () => import('./activity.app').then(({ ActivityApp }) => ActivityApp),
  lethernet: () => import('./lethernet.app').then(({ LetherNetApp }) => LetherNetApp),
  games: () => import('./games.app').then(({ GamesApp }) => GamesApp),
  notepad: () => import('./notepad.app').then(({ NotepadApp }) => NotepadApp),
  files: () => import('./files.app').then(({ FilesApp }) => FilesApp),
  settings: () => import('./settings.app').then(({ SettingsApp }) => SettingsApp),
  // Core/IDE panels share one view; route data and shell inputs select content.
  cpanel: loadDevPanel,
  explorer: loadDevPanel,
  codesearch: loadDevPanel,
  scm: loadDevPanel,
  terminal: () => import('./terminal.app').then(({ TerminalApp }) => TerminalApp),
  build: loadDevPanel,
  procmon: loadDevPanel,
  containers: loadDevPanel,
  repos: loadDevPanel,
  forge: loadDevPanel,
  devops: loadDevPanel,
  marketplace: loadDevPanel,
  tasks: loadDevPanel,
  tenant: loadDevPanel,
  ...SURFACE_APP_REGISTRY,
};

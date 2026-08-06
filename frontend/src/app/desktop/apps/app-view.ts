// ─────────────────────────────────────────────────────────────────────────
// apps/app-view.ts — the dumb app-view contract + lazy route registry.
//
// Every application is a standalone, presentational component: it receives the
// live `Win` (for its sub/systab keys), navigates via the injected
// WindowManagerService, and renders. It does NOT know about the OS chrome or
// other apps — it can be dropped on a route, unit-tested, or previewed alone.
//
// The route tree resolves `win.app` → lazy component through APP_REGISTRY, and
// CompositeApp resolves `win.sub` → lazy pane through PANE_REGISTRY. To add an
// application: build a component implementing AppView, register its dynamic
// import here, and add its AppDef to desktop-catalogue.data.ts. To add a pane
// to an existing application: register the component below and declare the
// pane in desktop-panes.data.ts.
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

const loadComposite: AppComponentLoader = () =>
  import('./composite.app').then(({ CompositeApp }) => CompositeApp);

/** Application-view components, usable as a whole window or as one pane. */
export const APP_VIEW_LOADERS: Record<string, AppComponentLoader> = {
  welcome: () => import('./welcome.app').then(({ WelcomeApp }) => WelcomeApp),
  control: () => import('./control.app').then(({ ControlApp }) => ControlApp),
  chat: () => import('./chat.app').then(({ ChatApp }) => ChatApp),
  telemetry: () => import('./telemetry.app').then(({ TelemetryApp }) => TelemetryApp),
  activity: () => import('./activity.app').then(({ ActivityApp }) => ActivityApp),
  lethernet: () => import('./lethernet.app').then(({ LetherNetApp }) => LetherNetApp),
  games: () => import('./games.app').then(({ GamesApp }) => GamesApp),
  notepad: () => import('./notepad.app').then(({ NotepadApp }) => NotepadApp),
  files: () => import('./files.app').then(({ FilesApp }) => FilesApp),
  settings: () => import('./settings.app').then(({ SettingsApp }) => SettingsApp),
  terminal: () => import('./terminal.app').then(({ TerminalApp }) => TerminalApp),
};

/** App id → dynamic component import used directly by the Angular route tree. */
export const APP_REGISTRY: Record<string, AppComponentLoader> = {
  welcome: APP_VIEW_LOADERS['welcome'],
  control: APP_VIEW_LOADERS['control'],
  telemetry: APP_VIEW_LOADERS['telemetry'],
  chat: APP_VIEW_LOADERS['chat'],
  lethernet: APP_VIEW_LOADERS['lethernet'],
  games: APP_VIEW_LOADERS['games'],
  notepad: APP_VIEW_LOADERS['notepad'],
  settings: APP_VIEW_LOADERS['settings'],
  // Panelled applications: one window, one rail, panes from desktop-panes.data.
  activity: loadComposite,
  operations: loadComposite,
  marketplace: loadComposite,
  ide: loadComposite,
  scm: loadComposite,
  databases: loadComposite,
  'project-manager': loadComposite,
  crm: loadComposite,
  marketing: loadComposite,
  agents: loadComposite,
  'ml-lab': loadComposite,
  files: loadComposite,
  // Promoted surfaces: the surface is the whole application.
  mail: SURFACE_APP_REGISTRY['surface-office-mail'],
  documents: SURFACE_APP_REGISTRY['surface-office-documents'],
  // The one application that is exactly one developer panel.
  tenant: () => import('./dev-panel.app').then(({ DevPanelApp }) => DevPanelApp),
};

/** Pane content id → dynamic component import, resolved by CompositeApp. */
export const PANE_REGISTRY: Record<string, AppComponentLoader> = {
  ...APP_VIEW_LOADERS,
  ...SURFACE_APP_REGISTRY,
};

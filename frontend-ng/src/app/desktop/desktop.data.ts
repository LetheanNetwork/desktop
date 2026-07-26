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

/** Files-app in-memory tree. Folders navigate via `to`; empty `items` → empty state. */
export interface FsNode {
  name: string;
  up: string | null;
  items: { n: string; k: string; to?: string; c?: string; m?: string }[];
}

export const FS: Record<string, FsNode> = {
  home: {
    name: 'Home',
    up: null,
    items: [
      { n: 'Documents', k: 'folder', to: 'documents', c: '4 items' },
      { n: 'Downloads', k: 'folder', to: 'downloads', c: '4 items' },
      { n: 'Models', k: 'folder', to: 'models', c: '5 items' },
      { n: 'Projects', k: 'folder', to: 'projects', c: '2 items' },
      { n: 'welcome.txt', k: 'doc', c: '2 KB', m: '09:14' },
      { n: 'lethean.png', k: 'img', c: '480 KB', m: 'Tue' },
      { n: 'notes.md', k: 'code', c: '6 KB', m: 'Mon' },
    ],
  },
  documents: {
    name: 'Documents',
    up: 'home',
    items: [
      { n: 'Invoices', k: 'folder', to: 'invoices', c: 'Empty' },
      { n: 'whitepaper.pdf', k: 'pdf', c: '2.4 MB', m: 'Jul 12' },
      { n: 'roadmap.md', k: 'code', c: '6 KB', m: 'Jul 18' },
      { n: 'brand-guide.pdf', k: 'pdf', c: '8.1 MB', m: 'Jun 30' },
      { n: 'meeting.txt', k: 'doc', c: '3 KB', m: 'Jul 20' },
    ],
  },
  invoices: { name: 'Invoices', up: 'documents', items: [] },
  downloads: {
    name: 'Downloads',
    up: 'home',
    items: [
      { n: 'core-1.20.tar.gz', k: 'zip', c: '64 MB', m: 'Jul 21' },
      { n: 'retroarch.dmg', k: 'zip', c: '112 MB', m: 'Jul 15' },
      { n: 'gemma-2-27b.gguf', k: 'model', c: '16 GB', m: 'Jul 10' },
      { n: 'screenshot.png', k: 'img', c: '1.2 MB', m: 'Today' },
    ],
  },
  models: {
    name: 'Models',
    up: 'home',
    items: [
      { n: 'llama-3.1-70b.gguf', k: 'model', c: '40 GB', m: 'Jul 02' },
      { n: 'mistral-small.gguf', k: 'model', c: '14 GB', m: 'Jun 28' },
      { n: 'gemma-2-27b.gguf', k: 'model', c: '16 GB', m: 'Jul 10' },
      { n: 'phi-3-mini.gguf', k: 'model', c: '2.3 GB', m: 'May 19' },
      { n: 'manifest.yaml', k: 'code', c: '1 KB', m: 'Jul 10' },
    ],
  },
  projects: {
    name: 'Projects',
    up: 'home',
    items: [
      { n: 'lethean', k: 'folder', to: 'proj_lethean', c: '5 items' },
      { n: 'core-ide', k: 'folder', to: 'proj_ide', c: '4 items' },
    ],
  },
  proj_lethean: {
    name: 'lethean',
    up: 'projects',
    items: [
      { n: 'go.work', k: 'code', c: '1 KB', m: 'Jul 22' },
      { n: 'README.md', k: 'code', c: '4 KB', m: 'Jul 19' },
      { n: 'main.go', k: 'code', c: '12 KB', m: 'Jul 22' },
      { n: 'desktop.component.ts', k: 'code', c: '18 KB', m: 'Today' },
      { n: 'logo.svg', k: 'img', c: '12 KB', m: 'Jun 01' },
    ],
  },
  proj_ide: {
    name: 'core-ide',
    up: 'projects',
    items: [
      { n: 'angular.json', k: 'code', c: '3 KB', m: 'Jul 12' },
      { n: 'package.json', k: 'code', c: '2 KB', m: 'Jul 12' },
      { n: 'app.routes.ts', k: 'code', c: '9 KB', m: 'Jul 20' },
      { n: 'bundle.zip', k: 'zip', c: '24 MB', m: 'Jul 18' },
    ],
  },
  lethernet: {
    name: 'LetherNet',
    up: null,
    items: [
      { n: 'peers.yaml', k: 'code', c: '1 KB', m: '09:16' },
      { n: 'vi-01.log', k: 'doc', c: '220 KB', m: 'live' },
      { n: 'vi-02.log', k: 'doc', c: '180 KB', m: 'live' },
      { n: 'pool-manifest.json', k: 'code', c: '2 KB', m: '09:12' },
    ],
  },
  trash: { name: 'Trash', up: null, items: [] },
};

/**
 * A production data service can replace this shared payload through
 * `DESKTOP_DATA`. Developer-panel fixtures live in `dev-panel.data.ts`;
 * shell-only fixtures live in `desktop-shell-fixtures.data.ts`.
 */
export interface DesktopData {
  apps: Record<string, AppDef>;
  order: string[];
  categories: Category[];
  fs: Record<string, FsNode>;
  langs: [string, string][];
  telemetry: typeof TELEMETRY;
}

export const DEFAULT_DESKTOP_DATA: DesktopData = {
  apps: APPS,
  order: ORDER,
  categories: CATEGORIES,
  fs: FS,
  langs: LANGS,
  telemetry: TELEMETRY,
};

/** Provide a live payload by overriding this token (e.g. via a route resolver). */
export const DESKTOP_DATA = new InjectionToken<DesktopData>('DESKTOP_DATA', {
  providedIn: 'root',
  factory: () => DEFAULT_DESKTOP_DATA,
});

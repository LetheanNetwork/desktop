// ─────────────────────────────────────────────────────────────────────────
// desktop.data.ts — the single data module for the Lethean desktop OS.
//
// Everything the shell renders that is *content* (apps, categories, the file
// tree, seed telemetry, i18n) lives here as typed constants. This is the seam
// for real integration: in production a resolver/service fetches a `DesktopData`
// payload from the CoreGo API and provides it via `DESKTOP_DATA` (see the
// InjectionToken at the bottom) — the WindowManagerService and every app-view
// component read from the token, never from hard-coded globals. The constants
// below are the offline/mock default so the app runs with no backend.
// ─────────────────────────────────────────────────────────────────────────
import { InjectionToken } from '@angular/core';
import {
  SURFACE_APPS,
  SURFACE_CATEGORIES,
  SURFACE_CATEGORY_APPS,
} from './surfaces/surface-registry';

// ── core types ──────────────────────────────────────────────────────────
export type ViewMode = 'desktop' | 'shell' | 'device';
export type DeviceSize = 'small' | 'large' | 'full';

/** A registered application. `route` marks a core/ide dev panel. */
export interface AppDef {
  id: string;
  title: string;
  icon: string; // font-awesome name (no fa- prefix)
  w: number;
  h: number; // default window size (windowed views)
  hint: string; // launcher tooltip
  dev?: boolean; // core/ide developer panel
  route?: string; // core/ide route this panel maps to
  defaultSub?: string; // initial child-view when launched
}

/** A Start-menu / launcher grouping. */
export interface Category {
  id: string;
  label: string;
  icon: string;
  apps: string[];
}

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
  group?: string; // dock workspace group id
  minimizing?: boolean; // transient: minimise animation in flight
}

// ── app registry ──────────────────────────────────────────────────────────
export const APPS: Record<string, AppDef> = {
  control: {
    id: 'control',
    title: $localize`:Application title@@app.control.title:Control`,
    icon: 'cube',
    w: 780,
    h: 560,
    hint: $localize`:Application launcher hint@@app.control.hint:Local models · runs · power`,
    defaultSub: 'models',
  },
  chat: {
    id: 'chat',
    title: $localize`:Application title@@app.chat.title:Chat`,
    icon: 'comments',
    w: 720,
    h: 520,
    hint: $localize`:Application launcher hint@@app.chat.hint:Talk to a local model`,
  },
  telemetry: {
    id: 'telemetry',
    title: $localize`:Application title@@app.telemetry.title:Telemetry`,
    icon: 'wave-square',
    w: 660,
    h: 400,
    hint: $localize`:Application launcher hint@@app.telemetry.hint:Live tok/s · watts · demo`,
  },
  activity: {
    id: 'activity',
    title: $localize`:Application title@@app.activity.title:Activity`,
    icon: 'rectangle-list',
    w: 660,
    h: 460,
    hint: $localize`:Application launcher hint@@app.activity.hint:Streaming logs · generations`,
  },
  lethernet: {
    id: 'lethernet',
    title: $localize`:Application title@@app.lethernet.title:LetherNet`,
    icon: 'diagram-project',
    w: 620,
    h: 440,
    hint: $localize`:Application launcher hint@@app.lethernet.hint:Peer fleet · routing`,
  },
  games: {
    id: 'games',
    title: $localize`:Application title@@app.games.title:Games`,
    icon: 'gamepad',
    w: 820,
    h: 580,
    hint: $localize`:Application launcher hint@@app.games.hint:STIM library · CorePlay`,
    defaultSub: 'library',
  },
  notepad: {
    id: 'notepad',
    title: $localize`:Application title@@app.notepad.title:Notepad`,
    icon: 'file-lines',
    w: 640,
    h: 520,
    hint: $localize`:Application launcher hint@@app.notepad.hint:Plain-text editor`,
  },
  files: {
    id: 'files',
    title: $localize`:Application title@@app.files.title:Files`,
    icon: 'folder-open',
    w: 820,
    h: 560,
    hint: $localize`:Application launcher hint@@app.files.hint:File browser · local disk`,
    defaultSub: 'home',
  },
  settings: {
    id: 'settings',
    title: $localize`:Application title@@app.settings.title:Settings`,
    icon: 'sliders',
    w: 540,
    h: 520,
    hint: $localize`:Application launcher hint@@app.settings.hint:UI preferences`,
    defaultSub: 'interface',
  },
  // core/ide developer panels — each maps 1:1 to a /dev route
  cpanel: {
    id: 'cpanel',
    title: $localize`:Application title@@app.cpanel.title:Control Panel`,
    icon: 'table-columns',
    w: 820,
    h: 580,
    hint: $localize`:Application launcher hint@@app.cpanel.hint:IDE control panel`,
    dev: true,
    route: 'control-panel',
  },
  explorer: {
    id: 'explorer',
    title: $localize`:Application title@@app.explorer.title:Explorer`,
    icon: 'folder-tree',
    w: 760,
    h: 560,
    hint: $localize`:Application launcher hint@@app.explorer.hint:File explorer`,
    dev: true,
    route: 'explorer',
  },
  codesearch: {
    id: 'codesearch',
    title: $localize`:Application title@@app.codesearch.title:Search`,
    icon: 'magnifying-glass',
    w: 760,
    h: 560,
    hint: $localize`:Application launcher hint@@app.codesearch.hint:Code search`,
    dev: true,
    route: 'search',
  },
  scm: {
    id: 'scm',
    title: $localize`:Application title@@app.scm.title:Source Control`,
    icon: 'code-branch',
    w: 760,
    h: 560,
    hint: $localize`:Application launcher hint@@app.scm.hint:Git status · commits`,
    dev: true,
    route: 'git',
  },
  terminal: {
    id: 'terminal',
    title: $localize`:Application title@@app.terminal.title:Terminal`,
    icon: 'terminal',
    w: 760,
    h: 460,
    hint: $localize`:Application launcher hint@@app.terminal.hint:Integrated terminal`,
    dev: true,
    route: 'terminal',
  },
  build: {
    id: 'build',
    title: $localize`:Application title@@app.build.title:Build`,
    icon: 'hammer',
    w: 760,
    h: 520,
    hint: $localize`:Application launcher hint@@app.build.hint:Build pipeline`,
    dev: true,
    route: 'build',
  },
  procmon: {
    id: 'procmon',
    title: $localize`:Application title@@app.procmon.title:Processes`,
    icon: 'gauge',
    w: 760,
    h: 520,
    hint: $localize`:Application launcher hint@@app.procmon.hint:go-process monitor`,
    dev: true,
    route: 'process',
  },
  containers: {
    id: 'containers',
    title: $localize`:Application title@@app.containers.title:Containers`,
    icon: 'cubes-stacked',
    w: 780,
    h: 540,
    hint: $localize`:Application launcher hint@@app.containers.hint:Container runtime`,
    dev: true,
    route: 'containers',
  },
  repos: {
    id: 'repos',
    title: $localize`:Application title@@app.repos.title:Repos`,
    icon: 'code-fork',
    w: 780,
    h: 540,
    hint: $localize`:Application launcher hint@@app.repos.hint:Repositories`,
    dev: true,
    route: 'repos',
  },
  forge: {
    id: 'forge',
    title: $localize`:Application title@@app.forge.title:Forge`,
    icon: 'industry',
    w: 780,
    h: 540,
    hint: $localize`:Application launcher hint@@app.forge.hint:Scaffolding · forge`,
    dev: true,
    route: 'forge',
  },
  devops: {
    id: 'devops',
    title: $localize`:Application title@@app.devops.title:DevOps`,
    icon: 'server',
    w: 780,
    h: 540,
    hint: $localize`:Application launcher hint@@app.devops.hint:Ansible · infra`,
    dev: true,
    route: 'devops',
  },
  marketplace: {
    id: 'marketplace',
    title: $localize`:Application title@@app.marketplace.title:Marketplace`,
    icon: 'store',
    w: 820,
    h: 560,
    hint: $localize`:Application launcher hint@@app.marketplace.hint:Plugins · extensions`,
    dev: true,
    route: 'marketplace',
  },
  tasks: {
    id: 'tasks',
    title: $localize`:Application title@@app.tasks.title:Tasks`,
    icon: 'list-check',
    w: 760,
    h: 540,
    hint: $localize`:Application launcher hint@@app.tasks.hint:Project board · Mantis`,
    dev: true,
    route: 'tasks',
  },
  tenant: {
    id: 'tenant',
    title: $localize`:Application title@@app.tenant.title:Tenant`,
    icon: 'building',
    w: 780,
    h: 540,
    hint: $localize`:Application launcher hint@@app.tenant.hint:Tenants · orgs`,
    dev: true,
    route: 'tenant',
  },
  ...SURFACE_APPS,
};

export const ORDER: string[] = [
  'control',
  'chat',
  'telemetry',
  'activity',
  'lethernet',
  'games',
  'notepad',
  'files',
  'cpanel',
  'explorer',
  'codesearch',
  'scm',
  'terminal',
  'build',
  'procmon',
  'containers',
  'repos',
  'forge',
  'devops',
  'marketplace',
  'tasks',
  'tenant',
  'settings',
];

export const CATEGORIES: Category[] = [
  {
    id: 'system',
    label: $localize`:Application category@@category.system:System`,
    icon: 'gauge-high',
    apps: ['control', 'telemetry', 'activity', 'settings'],
  },
  {
    id: 'developer',
    label: $localize`:Application category@@category.developer:Developer`,
    icon: 'code',
    apps: [
      'cpanel',
      'explorer',
      'codesearch',
      'scm',
      'build',
      'procmon',
      'containers',
      'repos',
      'forge',
      'devops',
      'marketplace',
    ],
  },
  {
    id: 'office',
    label: $localize`:Application category@@category.office:Office`,
    icon: 'briefcase',
    apps: ['tasks', 'tenant', ...SURFACE_CATEGORY_APPS['office']],
  },
  {
    id: 'ai',
    label: $localize`:Application category@@category.ai:AI`,
    icon: 'robot',
    apps: ['chat'],
  },
  {
    id: 'media',
    label: $localize`:Application category@@category.media:Media`,
    icon: 'photo-film',
    apps: ['games'],
  },
  {
    id: 'tools',
    label: $localize`:Application category@@category.tools:Tools`,
    icon: 'screwdriver-wrench',
    apps: ['files', 'notepad', 'terminal'],
  },
  {
    id: 'networking',
    label: $localize`:Application category@@category.networking:Networking`,
    icon: 'network-wired',
    apps: ['lethernet'],
  },
  ...SURFACE_CATEGORIES.filter(({ id }) => id !== 'office'),
];

// child-view rails (a window's internal nav). key, icon, label.
export const CTRL_NAV: [string, string, string][] = [
  ['models', 'cube', $localize`:Control navigation item@@control.nav.models:Models`],
  ['runs', 'play', $localize`:Control navigation item@@control.nav.runs:Runs`],
  ['power', 'bolt', $localize`:Control navigation item@@control.nav.power:Power`],
  ['system', 'gauge-high', $localize`:Control navigation item@@control.nav.system:System`],
  ['settings', 'gear', $localize`:Control navigation item@@control.nav.config:Config`],
];
export const GAMES_NAV: [string, string, string][] = [
  ['library', 'table-cells-large', $localize`:Games navigation item@@games.nav.library:Library`],
  ['engines', 'microchip', $localize`:Games navigation item@@games.nav.engines:Engines`],
];
export const SETTINGS_NAV: [string, string, string][] = [
  ['interface', 'sliders', $localize`:Settings navigation item@@settings.nav.interface:Interface`],
  [
    'translations',
    'language',
    $localize`:Settings navigation item@@settings.nav.translations:Translations`,
  ],
];

// ── content data (seed / mock — replace via the API payload) ──────────────
export const TELEMETRY = {
  throughput: [28, 30, 26, 34, 31, 38, 35, 42, 38, 44, 40, 46],
  watts: [180, 176, 182, 190, 188, 195, 201, 198, 205, 199, 207, 210],
};
export const CLOCKS: { city: string; zone: string; tz: string }[] = [
  { city: 'London', zone: 'London', tz: 'Europe/London' },
  { city: 'New York', zone: 'New York', tz: 'America/New_York' },
  { city: 'Singapore', zone: 'Singapore', tz: 'Asia/Singapore' },
];
export const PKGS: { name: string; state: string; variant: string }[] = [
  { name: 'llama.cpp', state: 'running', variant: 'ok' },
  { name: 'lthn-runner', state: 'active', variant: 'ok' },
  { name: 'LetherNet', state: 'idle', variant: '' },
];
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
 * The whole payload shape. A production `DataService` returns this from the
 * CoreGo API; here we provide DEFAULT_DESKTOP_DATA offline. Note the DEV panel
 * bodies (tables/kanban/terminal) and per-app rich content are intentionally
 * NOT here — they live in each app-view component (the "dumb view"), fed by
 * @Input from whatever slice of this data (or a live socket) they need.
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

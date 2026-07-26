import {
  SURFACE_APPS,
  SURFACE_CATEGORIES,
  SURFACE_CATEGORY_APPS,
} from './surfaces/surface-registry';

/** A registered application. `route` marks a core/ide developer panel. */
export interface AppDef {
  id: string;
  title: string;
  icon: string; // Font Awesome name (without the fa- prefix)
  w: number;
  h: number; // default window size for windowed views
  hint: string; // launcher tooltip
  dev?: boolean; // CoreGO/IDE developer panel
  route?: string; // developer route this panel maps to
  defaultSub?: string; // initial child view when launched
}

/** A Start-menu / launcher grouping. */
export interface Category {
  id: string;
  label: string;
  icon: string;
  apps: string[];
}

export type AppNavItem = readonly [path: string, icon: string, title: string];

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
    hint: $localize`:Application launcher hint@@app.telemetry.hint:Process telemetry · power demo`,
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
    hint: $localize`:Application launcher hint@@app.terminal.hint:Local PTY · browser demo`,
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

export const CTRL_NAV: AppNavItem[] = [
  ['models', 'cube', $localize`:Control navigation item@@control.nav.models:Models`],
  ['runs', 'play', $localize`:Control navigation item@@control.nav.runs:Runs`],
  ['power', 'bolt', $localize`:Control navigation item@@control.nav.power:Power`],
  ['system', 'gauge-high', $localize`:Control navigation item@@control.nav.system:System`],
  ['settings', 'gear', $localize`:Control navigation item@@control.nav.config:Config`],
];

export const GAMES_NAV: AppNavItem[] = [
  ['library', 'table-cells-large', $localize`:Games navigation item@@games.nav.library:Library`],
  ['engines', 'microchip', $localize`:Games navigation item@@games.nav.engines:Engines`],
];

export const SETTINGS_NAV: AppNavItem[] = [
  ['interface', 'sliders', $localize`:Settings navigation item@@settings.nav.interface:Interface`],
  [
    'translations',
    'language',
    $localize`:Settings navigation item@@settings.nav.translations:Translations`,
  ],
];

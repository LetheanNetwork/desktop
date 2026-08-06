// ─────────────────────────────────────────────────────────────────────────
// desktop-catalogue.data.ts — the application catalogue.
//
// Twenty-three applications, each one window. Anything smaller is a pane:
// a sub-route declared in desktop-panes.data.ts and rendered by the shared
// CompositeApp rail. An application declares the category it launches from,
// so CATEGORIES is derived rather than restated.
// ─────────────────────────────────────────────────────────────────────────
import { APP_PANES, panesFor } from './desktop-panes.data';
import type { DevPanelRoute } from './dev-panel.data';

/** A registered application: one launcher entry, one window. */
export interface AppDef {
  id: string;
  title: string;
  icon: string; // Font Awesome name (without the fa- prefix)
  category: string; // Category id it launches from
  w: number;
  h: number; // default window size for windowed views
  hint: string; // launcher tooltip
  /** Workspace/developer panel: menu-launched, never a desktop icon. */
  dev?: boolean;
  /** A panel application that IS one developer panel; also its route path. */
  route?: DevPanelRoute;
  defaultSub?: string; // initial pane when launched
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
  welcome: {
    id: 'welcome',
    title: $localize`:Application title@@app.welcome.title:Welcome`,
    icon: 'hand-wave',
    category: 'system',
    w: 620,
    h: 380,
    hint: $localize`:Application launcher hint@@app.welcome.hint:Start here`,
  },
  control: {
    id: 'control',
    title: $localize`:Application title@@app.control.title:Control`,
    icon: 'cube',
    category: 'system',
    w: 780,
    h: 560,
    hint: $localize`:Application launcher hint@@app.control.hint:Local models · runs · power`,
    defaultSub: 'models',
  },
  telemetry: {
    id: 'telemetry',
    title: $localize`:Application title@@app.telemetry.title:Telemetry`,
    icon: 'wave-square',
    category: 'system',
    w: 660,
    h: 400,
    hint: $localize`:Application launcher hint@@app.telemetry.hint:Host system · model runtime`,
  },
  activity: {
    id: 'activity',
    title: $localize`:Application title@@app.activity.title:Activity`,
    icon: 'rectangle-list',
    category: 'system',
    w: 920,
    h: 640,
    hint: $localize`:Application launcher hint@@app.activity.hint:Streaming logs · audit trail`,
    defaultSub: 'streams',
  },
  operations: {
    id: 'operations',
    title: $localize`:Application title@@app.operations.title:Operations`,
    icon: 'gears',
    category: 'system',
    w: 920,
    h: 640,
    hint: $localize`:Application launcher hint@@app.operations.hint:Incidents · runbooks · status`,
    dev: true,
    defaultSub: 'incidents',
  },
  marketplace: {
    id: 'marketplace',
    title: $localize`:Application title@@app.marketplace.title:Marketplace`,
    icon: 'store',
    category: 'system',
    w: 920,
    h: 640,
    hint: $localize`:Application launcher hint@@app.marketplace.hint:Plugins · extensions`,
    dev: true,
    defaultSub: 'store',
  },
  settings: {
    id: 'settings',
    title: $localize`:Application title@@app.settings.title:Settings`,
    icon: 'sliders',
    category: 'system',
    w: 540,
    h: 520,
    hint: $localize`:Application launcher hint@@app.settings.hint:UI preferences`,
    defaultSub: 'interface',
  },
  ide: {
    id: 'ide',
    title: $localize`:Application title@@app.ide.title:IDE`,
    icon: 'code',
    category: 'developer',
    w: 920,
    h: 640,
    hint: $localize`:Application launcher hint@@app.ide.hint:Editor · build · processes`,
    dev: true,
    defaultSub: 'control-panel',
  },
  scm: {
    id: 'scm',
    title: $localize`:Application title@@app.scm.title:Source Control`,
    icon: 'code-branch',
    category: 'developer',
    w: 920,
    h: 640,
    hint: $localize`:Application launcher hint@@app.scm.hint:Repositories · pull requests · deploys`,
    dev: true,
    defaultSub: 'repositories',
  },
  databases: {
    id: 'databases',
    title: $localize`:Application title@@app.databases.title:Databases`,
    icon: 'database',
    category: 'developer',
    w: 920,
    h: 640,
    hint: $localize`:Application launcher hint@@app.databases.hint:DuckDB · InfluxDB`,
    dev: true,
    defaultSub: 'duckdb',
  },
  'project-manager': {
    id: 'project-manager',
    title: $localize`:Application title@@app.projectManager.title:Project Manager`,
    icon: 'list-check',
    category: 'office',
    w: 920,
    h: 640,
    hint: $localize`:Application launcher hint@@app.projectManager.hint:Issues · board · planning`,
    dev: true,
    defaultSub: 'issues',
  },
  mail: {
    id: 'mail',
    title: $localize`:Application title@@app.mail.title:Mail`,
    icon: 'envelope',
    category: 'office',
    w: 920,
    h: 640,
    hint: $localize`:Application launcher hint@@app.mail.hint:Threads · drafts`,
    dev: true,
  },
  documents: {
    id: 'documents',
    title: $localize`:Application title@@app.documents.title:Documents`,
    icon: 'file-lines',
    category: 'office',
    w: 920,
    h: 640,
    hint: $localize`:Application launcher hint@@app.documents.hint:Shared documents`,
    dev: true,
  },
  crm: {
    id: 'crm',
    title: $localize`:Application title@@app.crm.title:CRM`,
    icon: 'address-book',
    category: 'office',
    w: 920,
    h: 640,
    hint: $localize`:Application launcher hint@@app.crm.hint:Contacts · deals · pipeline`,
    dev: true,
    defaultSub: 'contacts',
  },
  marketing: {
    id: 'marketing',
    title: $localize`:Application title@@app.marketing.title:Marketing`,
    icon: 'bullhorn',
    category: 'office',
    w: 920,
    h: 640,
    hint: $localize`:Application launcher hint@@app.marketing.hint:Campaigns · content · audience`,
    dev: true,
    defaultSub: 'campaigns',
  },
  tenant: {
    id: 'tenant',
    title: $localize`:Application title@@app.tenant.title:Tenant`,
    icon: 'building',
    category: 'office',
    w: 780,
    h: 540,
    hint: $localize`:Application launcher hint@@app.tenant.hint:Tenants · orgs`,
    dev: true,
    route: 'tenant',
  },
  chat: {
    id: 'chat',
    title: $localize`:Application title@@app.chat.title:Chat`,
    icon: 'comments',
    category: 'ai',
    w: 720,
    h: 520,
    hint: $localize`:Application launcher hint@@app.chat.hint:Talk to a local model`,
  },
  agents: {
    id: 'agents',
    title: $localize`:Application title@@app.agents.title:Agents`,
    icon: 'robot',
    category: 'ai',
    w: 920,
    h: 640,
    hint: $localize`:Application launcher hint@@app.agents.hint:Dispatch · flows · workspaces`,
    dev: true,
    defaultSub: 'activity',
  },
  'ml-lab': {
    id: 'ml-lab',
    title: $localize`:Application title@@app.mlLab.title:ML Lab`,
    icon: 'flask',
    category: 'ai',
    w: 920,
    h: 640,
    hint: $localize`:Application launcher hint@@app.mlLab.hint:Workbench · models · LoRA`,
    dev: true,
    defaultSub: 'workbench',
  },
  games: {
    id: 'games',
    title: $localize`:Application title@@app.games.title:Games`,
    icon: 'gamepad',
    category: 'media',
    w: 820,
    h: 580,
    hint: $localize`:Application launcher hint@@app.games.hint:STIM library · CorePlay`,
    defaultSub: 'library',
  },
  files: {
    id: 'files',
    title: $localize`:Application title@@app.files.title:Files`,
    icon: 'folder-open',
    category: 'tools',
    w: 820,
    h: 560,
    hint: $localize`:Application launcher hint@@app.files.hint:File browser · local disk`,
    defaultSub: 'home',
  },
  notepad: {
    id: 'notepad',
    title: $localize`:Application title@@app.notepad.title:Notepad`,
    icon: 'file-lines',
    category: 'tools',
    w: 640,
    h: 520,
    hint: $localize`:Application launcher hint@@app.notepad.hint:Plain-text editor`,
  },
  lethernet: {
    id: 'lethernet',
    title: $localize`:Application title@@app.lethernet.title:LetherNet`,
    icon: 'diagram-project',
    category: 'networking',
    w: 620,
    h: 440,
    hint: $localize`:Application launcher hint@@app.lethernet.hint:Peer fleet · routing`,
  },
};

/** Desktop-icon, dock, and command-palette order. Welcome opens itself. */
export const ORDER: string[] = Object.keys(APPS).filter((id) => id !== 'welcome');

const CATEGORY_LABELS: readonly (readonly [id: string, label: string, icon: string])[] = [
  ['system', $localize`:Application category@@category.system:System`, 'gauge-high'],
  ['developer', $localize`:Application category@@category.developer:Developer`, 'code'],
  ['office', $localize`:Application category@@category.office:Office`, 'briefcase'],
  ['ai', $localize`:Application category@@category.ai:AI`, 'robot'],
  ['media', $localize`:Application category@@category.media:Media`, 'photo-film'],
  ['tools', $localize`:Application category@@category.tools:Tools`, 'screwdriver-wrench'],
  ['networking', $localize`:Application category@@category.networking:Networking`, 'network-wired'],
];

export const CATEGORIES: Category[] = CATEGORY_LABELS.map(([id, label, icon]) => ({
  id,
  label,
  icon,
  apps: Object.values(APPS)
    .filter((app) => app.category === id)
    .map((app) => app.id),
}));

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

/**
 * Every application's side-rail, whether its component owns the switch
 * (Control, Games, Settings) or the shared CompositeApp resolves each pane
 * from desktop-panes.data.ts. The route tree and the window reducer both read
 * their sub-route vocabulary from here.
 */
export const APP_NAV: Readonly<Partial<Record<string, readonly AppNavItem[]>>> = {
  control: CTRL_NAV,
  games: GAMES_NAV,
  settings: SETTINGS_NAV,
  ...Object.fromEntries(
    [...new Set(APP_PANES.map(({ app }) => app))].map((app) => [
      app,
      panesFor(app).map(({ path, icon, title }): AppNavItem => [path, icon, title]),
    ]),
  ),
};

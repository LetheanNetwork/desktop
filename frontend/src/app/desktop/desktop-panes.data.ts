// ─────────────────────────────────────────────────────────────────────────
// desktop-panes.data.ts — the pane catalogue.
//
// One application is one window. A pane is a sub-route of that window
// (`/<category>/<app>/<pane>`), rendered by the shared CompositeApp rail.
// Every entry is a flat literal with the same key order — `app`, `path`, the
// single content key, `icon`, `title` — so the file reads as a table and the
// capability audit in scripts/frontend-capability-inventory.mjs can parse it
// without a TypeScript compiler.
//
// Content is exactly one of:
//   surface: a ported surface component id from SURFACE_APP_REGISTRY.
//   view:    an application component id from APP_VIEW_LOADERS.
//   dev:     a developer-panel route rendered by the shared DevPanelApp.
//
// The FIRST pane declared for an application is its default; the catalogue
// entry's `defaultSub` must agree (pinned by desktop-panes.data.spec.ts).
// ─────────────────────────────────────────────────────────────────────────
import type { DevPanelRoute } from './dev-panel.data';

/** One sub-route of one application window. */
export interface AppPane {
  readonly app: string;
  readonly path: string;
  readonly icon: string;
  readonly title: string;
  /** Ported surface component id (SURFACE_APP_REGISTRY key). */
  readonly surface?: string;
  /** Application-view component id (APP_VIEW_LOADERS key). */
  readonly view?: string;
  /** Developer-panel route rendered by the shared DevPanelApp. */
  readonly dev?: DevPanelRoute;
}

export const APP_PANES: readonly AppPane[] = [
  // ── System ────────────────────────────────────────────────────────────
  {
    app: 'activity',
    path: 'streams',
    view: 'activity',
    icon: 'rectangle-list',
    title: $localize`:Application pane@@app.activity.pane.streams:Streams`,
  },
  {
    app: 'activity',
    path: 'observe',
    surface: 'surface-observe-activity',
    icon: 'binoculars',
    title: $localize`:Application pane@@app.activity.pane.observe:Observe`,
  },
  {
    app: 'operations',
    path: 'incidents',
    surface: 'surface-operations-incidents',
    icon: 'triangle-exclamation',
    title: $localize`:Application pane@@app.operations.pane.incidents:Incidents`,
  },
  {
    app: 'operations',
    path: 'runbooks',
    surface: 'surface-operations-runbooks',
    icon: 'book',
    title: $localize`:Application pane@@app.operations.pane.runbooks:Runbooks`,
  },
  {
    app: 'operations',
    path: 'status',
    surface: 'surface-operations-status',
    icon: 'heart-pulse',
    title: $localize`:Application pane@@app.operations.pane.status:Status`,
  },
  {
    app: 'marketplace',
    path: 'store',
    surface: 'surface-extensions-marketplace',
    icon: 'store',
    title: $localize`:Application pane@@app.marketplace.pane.store:Store`,
  },
  {
    app: 'marketplace',
    path: 'plugin-view',
    surface: 'surface-extensions-plugin-view',
    icon: 'puzzle-piece',
    title: $localize`:Application pane@@app.marketplace.pane.pluginView:Plugin view`,
  },
  {
    app: 'marketplace',
    path: 'opencode',
    surface: 'surface-extensions-opencode-shim',
    icon: 'terminal',
    title: $localize`:Application pane@@app.marketplace.pane.opencode:OpenCode`,
  },

  // ── Developer ─────────────────────────────────────────────────────────
  {
    app: 'ide',
    path: 'control-panel',
    dev: 'control-panel',
    icon: 'table-columns',
    title: $localize`:Application pane@@app.ide.pane.controlPanel:Control Panel`,
  },
  {
    app: 'ide',
    path: 'explorer',
    dev: 'explorer',
    icon: 'folder-tree',
    title: $localize`:Application pane@@app.ide.pane.explorer:Explorer`,
  },
  {
    app: 'ide',
    path: 'search',
    dev: 'search',
    icon: 'magnifying-glass',
    title: $localize`:Application pane@@app.ide.pane.search:Search`,
  },
  {
    app: 'ide',
    path: 'git',
    dev: 'git',
    icon: 'code-branch',
    title: $localize`:Application pane@@app.ide.pane.git:Git`,
  },
  {
    app: 'ide',
    path: 'terminal',
    view: 'terminal',
    icon: 'terminal',
    title: $localize`:Application pane@@app.ide.pane.terminal:Terminal`,
  },
  {
    app: 'ide',
    path: 'build',
    dev: 'build',
    icon: 'hammer',
    title: $localize`:Application pane@@app.ide.pane.build:Build`,
  },
  {
    app: 'ide',
    path: 'processes',
    dev: 'process',
    icon: 'gauge',
    title: $localize`:Application pane@@app.ide.pane.processes:Processes`,
  },
  {
    app: 'ide',
    path: 'containers',
    dev: 'containers',
    icon: 'cubes-stacked',
    title: $localize`:Application pane@@app.ide.pane.containers:Containers`,
  },
  {
    app: 'ide',
    path: 'forge',
    dev: 'forge',
    icon: 'industry',
    title: $localize`:Application pane@@app.ide.pane.forge:Forge`,
  },
  {
    app: 'ide',
    path: 'devops',
    dev: 'devops',
    icon: 'server',
    title: $localize`:Application pane@@app.ide.pane.devops:DevOps`,
  },
  {
    app: 'scm',
    path: 'repositories',
    surface: 'surface-coding-repos',
    icon: 'code-branch',
    title: $localize`:Application pane@@app.scm.pane.repositories:Repositories`,
  },
  {
    app: 'scm',
    path: 'pull-requests',
    surface: 'surface-coding-prs',
    icon: 'code-pull-request',
    title: $localize`:Application pane@@app.scm.pane.pullRequests:Pull requests`,
  },
  {
    app: 'scm',
    path: 'deploys',
    surface: 'surface-coding-deploys',
    icon: 'rocket',
    title: $localize`:Application pane@@app.scm.pane.deploys:Deploys`,
  },
  {
    app: 'databases',
    path: 'duckdb',
    surface: 'surface-ml-lab-duckdb',
    icon: 'database',
    title: $localize`:Application pane@@app.databases.pane.duckdb:DuckDB`,
  },
  {
    app: 'databases',
    path: 'influxdb',
    surface: 'surface-ml-lab-influx',
    icon: 'chart-area',
    title: $localize`:Application pane@@app.databases.pane.influxdb:InfluxDB`,
  },

  // ── Office ────────────────────────────────────────────────────────────
  {
    app: 'project-manager',
    path: 'issues',
    surface: 'surface-coding-issues',
    icon: 'circle-exclamation',
    title: $localize`:Application pane@@app.projectManager.pane.issues:Issues`,
  },
  {
    app: 'project-manager',
    path: 'board',
    dev: 'tasks',
    icon: 'list-check',
    title: $localize`:Application pane@@app.projectManager.pane.board:Board`,
  },
  {
    app: 'project-manager',
    path: 'today',
    surface: 'surface-planning-today',
    icon: 'sun',
    title: $localize`:Application pane@@app.projectManager.pane.today:Today`,
  },
  {
    app: 'project-manager',
    path: 'backlog',
    surface: 'surface-planning-backlog',
    icon: 'inbox',
    title: $localize`:Application pane@@app.projectManager.pane.backlog:Backlog`,
  },
  {
    app: 'project-manager',
    path: 'sprints',
    surface: 'surface-planning-sprints',
    icon: 'person-running',
    title: $localize`:Application pane@@app.projectManager.pane.sprints:Sprints`,
  },
  {
    app: 'project-manager',
    path: 'roadmap',
    surface: 'surface-planning-roadmap',
    icon: 'map',
    title: $localize`:Application pane@@app.projectManager.pane.roadmap:Roadmap`,
  },
  {
    app: 'project-manager',
    path: 'calendar',
    surface: 'surface-planning-calendar',
    icon: 'calendar-days',
    title: $localize`:Application pane@@app.projectManager.pane.calendar:Calendar`,
  },
  {
    app: 'project-manager',
    path: 'retros',
    surface: 'surface-planning-retros',
    icon: 'rotate-left',
    title: $localize`:Application pane@@app.projectManager.pane.retros:Retrospectives`,
  },
  {
    app: 'crm',
    path: 'contacts',
    surface: 'surface-sales-contacts',
    icon: 'address-book',
    title: $localize`:Application pane@@app.crm.pane.contacts:Contacts`,
  },
  {
    app: 'crm',
    path: 'deals',
    surface: 'surface-sales-deals',
    icon: 'handshake',
    title: $localize`:Application pane@@app.crm.pane.deals:Deals`,
  },
  {
    app: 'crm',
    path: 'pipeline',
    surface: 'surface-sales-pipeline',
    icon: 'filter-circle-dollar',
    title: $localize`:Application pane@@app.crm.pane.pipeline:Pipeline`,
  },
  {
    app: 'crm',
    path: 'forecast',
    surface: 'surface-sales-forecast',
    icon: 'chart-column',
    title: $localize`:Application pane@@app.crm.pane.forecast:Forecast`,
  },
  {
    app: 'marketing',
    path: 'campaigns',
    surface: 'surface-marketing-campaigns',
    icon: 'bullhorn',
    title: $localize`:Application pane@@app.marketing.pane.campaigns:Campaigns`,
  },
  {
    app: 'marketing',
    path: 'content',
    surface: 'surface-marketing-content',
    icon: 'pen-nib',
    title: $localize`:Application pane@@app.marketing.pane.content:Content`,
  },
  {
    app: 'marketing',
    path: 'social',
    surface: 'surface-marketing-social',
    icon: 'share-nodes',
    title: $localize`:Application pane@@app.marketing.pane.social:Social`,
  },
  {
    app: 'marketing',
    path: 'audience',
    surface: 'surface-marketing-audience',
    icon: 'users',
    title: $localize`:Application pane@@app.marketing.pane.audience:Audience`,
  },
  {
    app: 'marketing',
    path: 'analytics',
    surface: 'surface-marketing-analytics',
    icon: 'chart-line',
    title: $localize`:Application pane@@app.marketing.pane.analytics:Analytics`,
  },

  // ── AI ────────────────────────────────────────────────────────────────
  {
    app: 'agents',
    path: 'activity',
    surface: 'surface-agents-activity',
    icon: 'wave-square',
    title: $localize`:Application pane@@app.agents.pane.activity:Activity`,
  },
  {
    app: 'agents',
    path: 'code',
    surface: 'surface-agents-code',
    icon: 'code',
    title: $localize`:Application pane@@app.agents.pane.code:Code`,
  },
  {
    app: 'agents',
    path: 'connect',
    surface: 'surface-agents-connect',
    icon: 'link',
    title: $localize`:Application pane@@app.agents.pane.connect:Connect`,
  },
  {
    app: 'agents',
    path: 'dispatch',
    surface: 'surface-agents-dispatch',
    icon: 'paper-plane',
    title: $localize`:Application pane@@app.agents.pane.dispatch:Dispatch`,
  },
  {
    app: 'agents',
    path: 'flows',
    surface: 'surface-agents-flows',
    icon: 'diagram-project',
    title: $localize`:Application pane@@app.agents.pane.flows:Flows`,
  },
  {
    app: 'agents',
    path: 'scan',
    surface: 'surface-agents-scan',
    icon: 'radar',
    title: $localize`:Application pane@@app.agents.pane.scan:Scan`,
  },
  {
    app: 'agents',
    path: 'tasks',
    surface: 'surface-agents-tasks',
    icon: 'list-check',
    title: $localize`:Application pane@@app.agents.pane.tasks:Tasks`,
  },
  {
    app: 'agents',
    path: 'terminal',
    surface: 'surface-agents-terminal',
    icon: 'terminal',
    title: $localize`:Application pane@@app.agents.pane.terminal:Terminal`,
  },
  {
    app: 'agents',
    path: 'workspaces',
    surface: 'surface-agents-workspaces',
    icon: 'layer-group',
    title: $localize`:Application pane@@app.agents.pane.workspaces:Workspaces`,
  },
  {
    app: 'ml-lab',
    path: 'workbench',
    surface: 'surface-ml-lab-ml-lab',
    icon: 'flask',
    title: $localize`:Application pane@@app.mlLab.pane.workbench:Workbench`,
  },
  {
    app: 'ml-lab',
    path: 'models',
    surface: 'surface-ml-lab-models',
    icon: 'cubes',
    title: $localize`:Application pane@@app.mlLab.pane.models:Models`,
  },
  {
    app: 'ml-lab',
    path: 'lora',
    surface: 'surface-ml-lab-lora',
    icon: 'sliders',
    title: $localize`:Application pane@@app.mlLab.pane.lora:LoRA`,
  },

  // ── Tools ─────────────────────────────────────────────────────────────
  // `home` is both the Local pane path and the Files home token, so a Files
  // window that has browsed into a mount still resolves to this pane instead
  // of having its location rewritten by the route.
  {
    app: 'files',
    path: 'home',
    view: 'files',
    icon: 'folder-open',
    title: $localize`:Application pane@@app.files.pane.home:Local`,
  },
  {
    app: 'files',
    path: 'workspace',
    surface: 'surface-office-files',
    icon: 'folder-tree',
    title: $localize`:Application pane@@app.files.pane.workspace:Workspace`,
  },
];

const PANES_BY_APP: ReadonlyMap<string, readonly AppPane[]> = APP_PANES.reduce((map, pane) => {
  map.set(pane.app, [...(map.get(pane.app) ?? []), pane]);
  return map;
}, new Map<string, AppPane[]>());

/** Declared panes for an application, in rail order. Empty for plain apps. */
export function panesFor(appId: string): readonly AppPane[] {
  return PANES_BY_APP.get(appId) ?? [];
}

/**
 * The pane a window is showing. An unrecognised sub — a retired pane path, or
 * a Files location token — falls back to the application's first pane rather
 * than rendering nothing.
 */
export function paneFor(appId: string, sub: string | undefined): AppPane | undefined {
  const panes = panesFor(appId);
  return panes.find(({ path }) => path === sub) ?? panes[0];
}

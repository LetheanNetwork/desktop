import type { AppDef, Category } from '../desktop.data';
import type { AppComponentLoader } from '../apps/app-view';

interface SurfaceDefinition {
  readonly group: string;
  readonly route: string;
  readonly title: string;
  readonly icon: string;
}

const DEFINITIONS: readonly SurfaceDefinition[] = [
  {
    group: 'agents',
    route: 'activity',
    title: $localize`:Surface title@@surface.agents.activity.title:Activity`,
    icon: 'wave-square',
  },
  {
    group: 'agents',
    route: 'code',
    title: $localize`:Surface title@@surface.agents.code.title:Code`,
    icon: 'code',
  },
  {
    group: 'agents',
    route: 'connect',
    title: $localize`:Surface title@@surface.agents.connect.title:Connect`,
    icon: 'link',
  },
  {
    group: 'agents',
    route: 'dispatch',
    title: $localize`:Surface title@@surface.agents.dispatch.title:Dispatch`,
    icon: 'paper-plane',
  },
  {
    group: 'agents',
    route: 'flows',
    title: $localize`:Surface title@@surface.agents.flows.title:Flows`,
    icon: 'diagram-project',
  },
  {
    group: 'agents',
    route: 'scan',
    title: $localize`:Surface title@@surface.agents.scan.title:Scan`,
    icon: 'radar',
  },
  {
    group: 'agents',
    route: 'tasks',
    title: $localize`:Surface title@@surface.agents.tasks.title:Tasks`,
    icon: 'list-check',
  },
  {
    group: 'agents',
    route: 'terminal',
    title: $localize`:Surface title@@surface.agents.terminal.title:Terminal`,
    icon: 'terminal',
  },
  {
    group: 'agents',
    route: 'workspaces',
    title: $localize`:Surface title@@surface.agents.workspaces.title:Workspaces`,
    icon: 'layer-group',
  },

  {
    group: 'coding',
    route: 'deploys',
    title: $localize`:Surface title@@surface.coding.deploys.title:Deploys`,
    icon: 'rocket',
  },
  {
    group: 'coding',
    route: 'issues',
    title: $localize`:Surface title@@surface.coding.issues.title:Issues`,
    icon: 'circle-exclamation',
  },
  {
    group: 'coding',
    route: 'prs',
    title: $localize`:Surface title@@surface.coding.prs.title:Pull requests`,
    icon: 'code-pull-request',
  },
  {
    group: 'coding',
    route: 'repos',
    title: $localize`:Surface title@@surface.coding.repos.title:Repositories`,
    icon: 'code-branch',
  },

  {
    group: 'marketing',
    route: 'analytics',
    title: $localize`:Surface title@@surface.marketing.analytics.title:Analytics`,
    icon: 'chart-line',
  },
  {
    group: 'marketing',
    route: 'audience',
    title: $localize`:Surface title@@surface.marketing.audience.title:Audience`,
    icon: 'users',
  },
  {
    group: 'marketing',
    route: 'campaigns',
    title: $localize`:Surface title@@surface.marketing.campaigns.title:Campaigns`,
    icon: 'bullhorn',
  },
  {
    group: 'marketing',
    route: 'content',
    title: $localize`:Surface title@@surface.marketing.content.title:Content`,
    icon: 'pen-nib',
  },
  {
    group: 'marketing',
    route: 'social',
    title: $localize`:Surface title@@surface.marketing.social.title:Social`,
    icon: 'share-nodes',
  },

  {
    group: 'ml-lab',
    route: 'duckdb',
    title: $localize`:Surface title@@surface.mlLab.duckdb.title:DuckDB`,
    icon: 'database',
  },
  {
    group: 'ml-lab',
    route: 'influx',
    title: $localize`:Surface title@@surface.mlLab.influx.title:InfluxDB`,
    icon: 'chart-area',
  },
  {
    group: 'ml-lab',
    route: 'lora',
    title: $localize`:Surface title@@surface.mlLab.lora.title:LoRA`,
    icon: 'sliders',
  },
  {
    group: 'ml-lab',
    route: 'ml-lab',
    title: $localize`:Surface title@@surface.mlLab.workbench.title:ML Lab`,
    icon: 'flask',
  },
  {
    group: 'ml-lab',
    route: 'models',
    title: $localize`:Surface title@@surface.mlLab.models.title:Models`,
    icon: 'cubes',
  },

  {
    group: 'observe',
    route: 'activity',
    title: $localize`:Surface title@@surface.observe.activity.title:Activity`,
    icon: 'binoculars',
  },

  {
    group: 'office',
    route: 'documents',
    title: $localize`:Surface title@@surface.office.documents.title:Documents`,
    icon: 'file-lines',
  },
  {
    group: 'office',
    route: 'files',
    title: $localize`:Surface title@@surface.office.files.title:Files`,
    icon: 'folder-open',
  },
  {
    group: 'office',
    route: 'mail',
    title: $localize`:Surface title@@surface.office.mail.title:Mail`,
    icon: 'envelope',
  },

  {
    group: 'operations',
    route: 'incidents',
    title: $localize`:Surface title@@surface.operations.incidents.title:Incidents`,
    icon: 'triangle-exclamation',
  },
  {
    group: 'operations',
    route: 'runbooks',
    title: $localize`:Surface title@@surface.operations.runbooks.title:Runbooks`,
    icon: 'book',
  },
  {
    group: 'operations',
    route: 'status',
    title: $localize`:Surface title@@surface.operations.status.title:Status`,
    icon: 'heart-pulse',
  },

  {
    group: 'planning',
    route: 'backlog',
    title: $localize`:Surface title@@surface.planning.backlog.title:Backlog`,
    icon: 'inbox',
  },
  {
    group: 'planning',
    route: 'calendar',
    title: $localize`:Surface title@@surface.planning.calendar.title:Calendar`,
    icon: 'calendar-days',
  },
  {
    group: 'planning',
    route: 'retros',
    title: $localize`:Surface title@@surface.planning.retros.title:Retrospectives`,
    icon: 'rotate-left',
  },
  {
    group: 'planning',
    route: 'roadmap',
    title: $localize`:Surface title@@surface.planning.roadmap.title:Roadmap`,
    icon: 'map',
  },
  {
    group: 'planning',
    route: 'sprints',
    title: $localize`:Surface title@@surface.planning.sprints.title:Sprints`,
    icon: 'person-running',
  },
  {
    group: 'planning',
    route: 'today',
    title: $localize`:Surface title@@surface.planning.today.title:Today`,
    icon: 'sun',
  },

  {
    group: 'sales',
    route: 'contacts',
    title: $localize`:Surface title@@surface.sales.contacts.title:Contacts`,
    icon: 'address-book',
  },
  {
    group: 'sales',
    route: 'deals',
    title: $localize`:Surface title@@surface.sales.deals.title:Deals`,
    icon: 'handshake',
  },
  {
    group: 'sales',
    route: 'forecast',
    title: $localize`:Surface title@@surface.sales.forecast.title:Forecast`,
    icon: 'chart-column',
  },
  {
    group: 'sales',
    route: 'pipeline',
    title: $localize`:Surface title@@surface.sales.pipeline.title:Pipeline`,
    icon: 'filter-circle-dollar',
  },

  {
    group: 'extensions',
    route: 'marketplace',
    title: $localize`:Surface title@@surface.extensions.marketplace.title:Marketplace`,
    icon: 'store',
  },
  {
    group: 'extensions',
    route: 'plugin-view',
    title: $localize`:Surface title@@surface.extensions.pluginView.title:Plugin view`,
    icon: 'puzzle-piece',
  },
  {
    group: 'extensions',
    route: 'opencode-shim',
    title: $localize`:Surface title@@surface.extensions.opencode.title:OpenCode`,
    icon: 'terminal',
  },
];

export function surfaceAppId(group: string, route: string): string {
  return `surface-${group}-${route}`;
}

export const SURFACE_APPS: Record<string, AppDef> = Object.fromEntries(
  DEFINITIONS.map(({ group, route, title, icon }) => {
    const id = surfaceAppId(group, route);
    return [
      id,
      {
        id,
        title,
        icon,
        route,
        w: 920,
        h: 640,
        hint: title,
      } satisfies AppDef,
    ];
  }),
);

export const SURFACE_CATEGORY_APPS: Readonly<Record<string, readonly string[]>> =
  Object.fromEntries(
    [...new Set(DEFINITIONS.map(({ group }) => group))].map((group) => [
      group,
      DEFINITIONS.filter((definition) => definition.group === group).map(({ route }) =>
        surfaceAppId(group, route),
      ),
    ]),
  );

export const SURFACE_CATEGORIES: Category[] = [
  {
    id: 'agents',
    label: $localize`:Surface category@@category.agents:Agents`,
    icon: 'robot',
    apps: [...SURFACE_CATEGORY_APPS['agents']],
  },
  {
    id: 'coding',
    label: $localize`:Surface category@@category.coding:Coding`,
    icon: 'code',
    apps: [...SURFACE_CATEGORY_APPS['coding']],
  },
  {
    id: 'marketing',
    label: $localize`:Surface category@@category.marketing:Marketing`,
    icon: 'bullhorn',
    apps: [...SURFACE_CATEGORY_APPS['marketing']],
  },
  {
    id: 'ml-lab',
    label: $localize`:Surface category@@category.mlLab:ML Lab`,
    icon: 'flask',
    apps: [...SURFACE_CATEGORY_APPS['ml-lab']],
  },
  {
    id: 'observe',
    label: $localize`:Surface category@@category.observe:Observe`,
    icon: 'binoculars',
    apps: [...SURFACE_CATEGORY_APPS['observe']],
  },
  {
    id: 'office',
    label: $localize`:Surface category@@category.office:Office`,
    icon: 'briefcase',
    apps: [...SURFACE_CATEGORY_APPS['office']],
  },
  {
    id: 'operations',
    label: $localize`:Surface category@@category.operations:Operations`,
    icon: 'gears',
    apps: [...SURFACE_CATEGORY_APPS['operations']],
  },
  {
    id: 'planning',
    label: $localize`:Surface category@@category.planning:Planning`,
    icon: 'calendar-check',
    apps: [...SURFACE_CATEGORY_APPS['planning']],
  },
  {
    id: 'sales',
    label: $localize`:Surface category@@category.sales:Sales`,
    icon: 'sterling-sign',
    apps: [...SURFACE_CATEGORY_APPS['sales']],
  },
  {
    id: 'extensions',
    label: $localize`:Surface category@@category.extensions:Extensions`,
    icon: 'puzzle-piece',
    apps: [...SURFACE_CATEGORY_APPS['extensions']],
  },
];

export const SURFACE_APP_REGISTRY: Record<string, AppComponentLoader> = {
  'surface-agents-activity': () =>
    import('./agents/activity').then(({ AgentsActivitySurface }) => AgentsActivitySurface),
  'surface-agents-code': () =>
    import('./agents/code').then(({ AgentsCodeSurface }) => AgentsCodeSurface),
  'surface-agents-connect': () =>
    import('./agents/connect').then(({ AgentsConnectSurface }) => AgentsConnectSurface),
  'surface-agents-dispatch': () =>
    import('./agents/dispatch').then(({ AgentsDispatchSurface }) => AgentsDispatchSurface),
  'surface-agents-flows': () =>
    import('./agents/flows').then(({ AgentsFlowsSurface }) => AgentsFlowsSurface),
  'surface-agents-scan': () =>
    import('./agents/scan').then(({ AgentsScanSurface }) => AgentsScanSurface),
  'surface-agents-tasks': () =>
    import('./agents/tasks').then(({ AgentsTasksSurface }) => AgentsTasksSurface),
  'surface-agents-terminal': () =>
    import('./agents/terminal').then(({ AgentsTerminalSurface }) => AgentsTerminalSurface),
  'surface-agents-workspaces': () =>
    import('./agents/workspaces').then(({ AgentsWorkspacesSurface }) => AgentsWorkspacesSurface),
  'surface-coding-deploys': () =>
    import('./coding/deploys').then(({ CodingDeploysSurface }) => CodingDeploysSurface),
  'surface-coding-issues': () =>
    import('./coding/issues').then(({ CodingIssuesSurface }) => CodingIssuesSurface),
  'surface-coding-prs': () =>
    import('./coding/prs').then(({ CodingPrsSurface }) => CodingPrsSurface),
  'surface-coding-repos': () =>
    import('./coding/repos').then(({ CodingReposSurface }) => CodingReposSurface),
  'surface-marketing-analytics': () =>
    import('./marketing/analytics').then(
      ({ MarketingAnalyticsSurface }) => MarketingAnalyticsSurface,
    ),
  'surface-marketing-audience': () =>
    import('./marketing/audience').then(({ MarketingAudienceSurface }) => MarketingAudienceSurface),
  'surface-marketing-campaigns': () =>
    import('./marketing/campaigns').then(
      ({ MarketingCampaignsSurface }) => MarketingCampaignsSurface,
    ),
  'surface-marketing-content': () =>
    import('./marketing/content').then(({ MarketingContentSurface }) => MarketingContentSurface),
  'surface-marketing-social': () =>
    import('./marketing/social').then(({ MarketingSocialSurface }) => MarketingSocialSurface),
  'surface-ml-lab-duckdb': () =>
    import('./ml-lab/duckdb').then(({ MlLabDuckdbSurface }) => MlLabDuckdbSurface),
  'surface-ml-lab-influx': () =>
    import('./ml-lab/influx').then(({ MlLabInfluxSurface }) => MlLabInfluxSurface),
  'surface-ml-lab-lora': () =>
    import('./ml-lab/lora').then(({ MlLabLoraSurface }) => MlLabLoraSurface),
  'surface-ml-lab-ml-lab': () =>
    import('./ml-lab/ml-lab').then(({ MlLabWorkbenchSurface }) => MlLabWorkbenchSurface),
  'surface-ml-lab-models': () =>
    import('./ml-lab/models').then(({ MlLabModelsSurface }) => MlLabModelsSurface),
  'surface-observe-activity': () =>
    import('./observe/activity').then(({ ObserveActivitySurface }) => ObserveActivitySurface),
  'surface-office-documents': () =>
    import('./office/documents').then(({ OfficeDocumentsSurface }) => OfficeDocumentsSurface),
  'surface-office-files': () =>
    import('./office/files').then(({ OfficeFilesSurface }) => OfficeFilesSurface),
  'surface-office-mail': () =>
    import('./office/mail').then(({ OfficeMailSurface }) => OfficeMailSurface),
  'surface-operations-incidents': () =>
    import('./operations/incidents').then(
      ({ OperationsIncidentsSurface }) => OperationsIncidentsSurface,
    ),
  'surface-operations-runbooks': () =>
    import('./operations/runbooks').then(
      ({ OperationsRunbooksSurface }) => OperationsRunbooksSurface,
    ),
  'surface-operations-status': () =>
    import('./operations/status').then(({ OperationsStatusSurface }) => OperationsStatusSurface),
  'surface-planning-backlog': () =>
    import('./planning/backlog').then(({ PlanningBacklogSurface }) => PlanningBacklogSurface),
  'surface-planning-calendar': () =>
    import('./planning/calendar').then(({ PlanningCalendarSurface }) => PlanningCalendarSurface),
  'surface-planning-retros': () =>
    import('./planning/retros').then(({ PlanningRetrosSurface }) => PlanningRetrosSurface),
  'surface-planning-roadmap': () =>
    import('./planning/roadmap').then(({ PlanningRoadmapSurface }) => PlanningRoadmapSurface),
  'surface-planning-sprints': () =>
    import('./planning/sprints').then(({ PlanningSprintsSurface }) => PlanningSprintsSurface),
  'surface-planning-today': () =>
    import('./planning/today').then(({ PlanningTodaySurface }) => PlanningTodaySurface),
  'surface-sales-contacts': () =>
    import('./sales/contacts').then(({ SalesContactsSurface }) => SalesContactsSurface),
  'surface-sales-deals': () =>
    import('./sales/deals').then(({ SalesDealsSurface }) => SalesDealsSurface),
  'surface-sales-forecast': () =>
    import('./sales/forecast').then(({ SalesForecastSurface }) => SalesForecastSurface),
  'surface-sales-pipeline': () =>
    import('./sales/pipeline').then(({ SalesPipelineSurface }) => SalesPipelineSurface),
  'surface-extensions-marketplace': () =>
    import('./extensions/marketplace').then(
      ({ ExtensionsMarketplaceSurface }) => ExtensionsMarketplaceSurface,
    ),
  'surface-extensions-plugin-view': () =>
    import('./extensions/plugin-view').then(
      ({ ExtensionsPluginViewSurface }) => ExtensionsPluginViewSurface,
    ),
  'surface-extensions-opencode-shim': () =>
    import('./extensions/opencode-shim').then(
      ({ ExtensionsOpencodeShimSurface }) => ExtensionsOpencodeShimSurface,
    ),
};

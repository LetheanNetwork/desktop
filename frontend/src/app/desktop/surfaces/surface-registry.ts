// ─────────────────────────────────────────────────────────────────────────
// surfaces/surface-registry.ts — the ported surface components.
//
// A surface is a pane, never a top-level application: desktop-panes.data.ts
// binds each id below to one `/<category>/<app>/<pane>` route, and the two
// promoted surfaces (Mail, Documents) are bound as whole applications in
// desktop-catalogue.data.ts. `group`/`route` stay the component's identity —
// the source file is always `surfaces/<group>/<route>.ts`, which is what the
// capability audit reads.
// ─────────────────────────────────────────────────────────────────────────
import type { AppComponentLoader } from '../apps/app-view';

/** Component identity: the file at `surfaces/<group>/<route>.ts`. */
export interface SurfaceDefinition {
  readonly group: string;
  readonly route: string;
}

export const SURFACE_DEFINITIONS: readonly SurfaceDefinition[] = [
  { group: 'agents', route: 'activity' },
  { group: 'agents', route: 'code' },
  { group: 'agents', route: 'connect' },
  { group: 'agents', route: 'dispatch' },
  { group: 'agents', route: 'flows' },
  { group: 'agents', route: 'scan' },
  { group: 'agents', route: 'tasks' },
  { group: 'agents', route: 'terminal' },
  { group: 'agents', route: 'workspaces' },

  { group: 'coding', route: 'deploys' },
  { group: 'coding', route: 'issues' },
  { group: 'coding', route: 'prs' },
  { group: 'coding', route: 'repos' },

  { group: 'marketing', route: 'analytics' },
  { group: 'marketing', route: 'audience' },
  { group: 'marketing', route: 'campaigns' },
  { group: 'marketing', route: 'content' },
  { group: 'marketing', route: 'social' },

  { group: 'ml-lab', route: 'duckdb' },
  { group: 'ml-lab', route: 'influx' },
  { group: 'ml-lab', route: 'lora' },
  { group: 'ml-lab', route: 'ml-lab' },
  { group: 'ml-lab', route: 'models' },

  { group: 'observe', route: 'activity' },

  { group: 'office', route: 'documents' },
  { group: 'office', route: 'files' },
  { group: 'office', route: 'mail' },

  { group: 'operations', route: 'incidents' },
  { group: 'operations', route: 'runbooks' },
  { group: 'operations', route: 'status' },

  { group: 'planning', route: 'backlog' },
  { group: 'planning', route: 'calendar' },
  { group: 'planning', route: 'retros' },
  { group: 'planning', route: 'roadmap' },
  { group: 'planning', route: 'sprints' },
  { group: 'planning', route: 'today' },

  { group: 'sales', route: 'contacts' },
  { group: 'sales', route: 'deals' },
  { group: 'sales', route: 'forecast' },
  { group: 'sales', route: 'pipeline' },

  { group: 'extensions', route: 'marketplace' },
  { group: 'extensions', route: 'plugin-view' },
  { group: 'extensions', route: 'opencode-shim' },
];

export function surfaceAppId(group: string, route: string): string {
  return `surface-${group}-${route}`;
}

/** Every registered surface id, in declaration order. */
export const SURFACE_IDS: readonly string[] = SURFACE_DEFINITIONS.map(({ group, route }) =>
  surfaceAppId(group, route),
);

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

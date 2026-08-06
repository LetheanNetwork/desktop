// ─────────────────────────────────────────────────────────────────────────
// surfaces/surface-fixtures.spec.ts — every fixture-only surface (no bespoke
// component logic of its own, just a SurfaceConfig wired into the shared
// SurfacePage) renders its own title, subtitle and footer correctly.
//
// SurfacePage's own interaction contract (filtering, live data, board drag,
// actions) is exercised directly in surface-page.spec.ts against synthetic
// configs; this file only proves each of these thin wrapper components
// supplies a valid config to that shared page.
// ─────────────────────────────────────────────────────────────────────────
import type { Type } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { Win } from '../desktop.data';
import { AgentsActivitySurface } from './agents/activity';
import { AgentsCodeSurface } from './agents/code';
import { AgentsConnectSurface } from './agents/connect';
import { AgentsDispatchSurface } from './agents/dispatch';
import { AgentsFlowsSurface } from './agents/flows';
import { AgentsScanSurface } from './agents/scan';
import { AgentsTasksSurface } from './agents/tasks';
import { AgentsWorkspacesSurface } from './agents/workspaces';
import { CodingDeploysSurface } from './coding/deploys';
import { CodingIssuesSurface } from './coding/issues';
import { CodingPrsSurface } from './coding/prs';
import { CodingReposSurface } from './coding/repos';
import { MarketingAnalyticsSurface } from './marketing/analytics';
import { MarketingAudienceSurface } from './marketing/audience';
import { MarketingCampaignsSurface } from './marketing/campaigns';
import { MarketingContentSurface } from './marketing/content';
import { MarketingSocialSurface } from './marketing/social';
import { MlLabLoraSurface } from './ml-lab/lora';
import { MlLabWorkbenchSurface } from './ml-lab/ml-lab';
import { MlLabModelsSurface } from './ml-lab/models';
import { ObserveActivitySurface } from './observe/activity';
import { OfficeDocumentsSurface } from './office/documents';
import { OfficeMailSurface } from './office/mail';
import { OperationsIncidentsSurface } from './operations/incidents';
import { PlanningBacklogSurface } from './planning/backlog';
import { PlanningCalendarSurface } from './planning/calendar';
import { PlanningRetrosSurface } from './planning/retros';
import { PlanningRoadmapSurface } from './planning/roadmap';
import { PlanningTodaySurface } from './planning/today';
import { SalesContactsSurface } from './sales/contacts';
import { SalesDealsSurface } from './sales/deals';
import { SalesForecastSurface } from './sales/forecast';
import { SalesPipelineSurface } from './sales/pipeline';
import { SurfaceBridgeService } from './surface-bridge.service';
import { SurfaceConfig } from './surface-page';

interface FixtureRoute {
  readonly config: SurfaceConfig;
}

const win: Win = {
  id: 'fixture-window',
  app: 'fixture',
  sub: 'pane',
  x: 0,
  y: 0,
  w: 640,
  h: 480,
  z: 1,
  min: false,
  max: false,
};

const fixtures: readonly (readonly [string, Type<FixtureRoute>])[] = [
  ['AgentsActivitySurface', AgentsActivitySurface],
  ['AgentsCodeSurface', AgentsCodeSurface],
  ['AgentsConnectSurface', AgentsConnectSurface],
  ['AgentsDispatchSurface', AgentsDispatchSurface],
  ['AgentsFlowsSurface', AgentsFlowsSurface],
  ['AgentsScanSurface', AgentsScanSurface],
  ['AgentsTasksSurface', AgentsTasksSurface],
  ['AgentsWorkspacesSurface', AgentsWorkspacesSurface],
  ['CodingDeploysSurface', CodingDeploysSurface],
  ['CodingIssuesSurface', CodingIssuesSurface],
  ['CodingPrsSurface', CodingPrsSurface],
  ['CodingReposSurface', CodingReposSurface],
  ['MarketingAnalyticsSurface', MarketingAnalyticsSurface],
  ['MarketingAudienceSurface', MarketingAudienceSurface],
  ['MarketingCampaignsSurface', MarketingCampaignsSurface],
  ['MarketingContentSurface', MarketingContentSurface],
  ['MarketingSocialSurface', MarketingSocialSurface],
  ['MlLabLoraSurface', MlLabLoraSurface],
  ['MlLabWorkbenchSurface', MlLabWorkbenchSurface],
  ['MlLabModelsSurface', MlLabModelsSurface],
  ['ObserveActivitySurface', ObserveActivitySurface],
  ['OfficeDocumentsSurface', OfficeDocumentsSurface],
  ['OfficeMailSurface', OfficeMailSurface],
  ['OperationsIncidentsSurface', OperationsIncidentsSurface],
  ['PlanningBacklogSurface', PlanningBacklogSurface],
  ['PlanningCalendarSurface', PlanningCalendarSurface],
  ['PlanningRetrosSurface', PlanningRetrosSurface],
  ['PlanningRoadmapSurface', PlanningRoadmapSurface],
  ['PlanningTodaySurface', PlanningTodaySurface],
  ['SalesContactsSurface', SalesContactsSurface],
  ['SalesDealsSurface', SalesDealsSurface],
  ['SalesForecastSurface', SalesForecastSurface],
  ['SalesPipelineSurface', SalesPipelineSurface],
];

describe('fixture-only surface routes', () => {
  const bridge = { call: vi.fn(), request: vi.fn() };

  beforeEach(() => {
    bridge.call.mockReset();
    bridge.request.mockReset();
    TestBed.configureTestingModule({
      providers: [{ provide: SurfaceBridgeService, useValue: bridge }],
    });
  });

  afterEach(() => TestBed.resetTestingModule());

  it('registers exactly the 33 fixture-only surfaces (the rest own bespoke logic)', () => {
    expect(fixtures).toHaveLength(33);
    expect(new Set(fixtures.map(([name]) => name)).size).toBe(33);
  });

  it.each(fixtures)('renders the %s config through the shared surface page', async (_name, component) => {
    const fixture = TestBed.createComponent(component);
    fixture.componentRef.setInput('win', win);
    fixture.detectChanges();
    await fixture.whenStable();

    const { config } = fixture.componentInstance;
    const element = fixture.nativeElement as HTMLElement;

    expect(config.id).toBeTruthy();
    expect(element.querySelector('h2')?.textContent).toBe(config.title);
    expect(element.querySelector('p')?.textContent).toBe(config.subtitle);
    expect(element.querySelector('footer')?.textContent).toBe(config.footer);
    expect(element.querySelector('[data-surface]')?.getAttribute('data-surface')).toBe(config.id);

    fixture.destroy();
  });
});

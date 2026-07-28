import { Component } from '@angular/core';
import { SurfaceConfig, SurfacePage, SurfaceRoute } from '../surface-page';

@Component({
  selector: 'lthn-planning-backlog-surface',
  imports: [SurfacePage],
  template: `<lthn-surface-page [config]="config" />`,
})
export class PlanningBacklogSurface extends SurfaceRoute {
  readonly config: SurfaceConfig = {
    id: 'planning-backlog',
    title: $localize`:Planning backlog title@@surface.planning.backlog.title:Backlog`,
    subtitle: $localize`:Planning backlog subtitle@@surface.planning.backlog.subtitle:8 items · RICE-scored`,
    icon: 'layer-group',
    searchPlaceholder: 'Search id or title',
    rows: [
      {
        id: 'L-201',
        title: 'Federated chat across LetherNet peers',
        meta: 'L3 · network',
        value: '92',
        secondary: 'I5 · E5 · C4',
        tone: 'brand',
      },
      {
        id: 'L-189',
        title: 'Audit log retention policy + UI control',
        meta: 'Compliance',
        value: '88',
        secondary: 'I4 · E1 · C5',
        tone: 'brand',
      },
      {
        id: 'L-187',
        title: 'Telemetry · per-model success rate',
        meta: 'Observability',
        value: '84',
        secondary: 'I4 · E2 · C4',
        tone: 'brand',
      },
      {
        id: 'L-198',
        title: 'Quantisation auto-picker per hardware',
        meta: 'Runtime',
        value: '81',
        secondary: 'I5 · E3 · C3',
      },
      {
        id: 'L-203',
        title: 'Vi voice mode · local Whisper transcribe',
        meta: 'Chat',
        value: '76',
        secondary: 'I3 · E4 · C4',
      },
      {
        id: 'L-185',
        title: 'Wire HuggingFace search to model browser',
        meta: 'Models',
        value: '72',
        secondary: 'I3 · E3 · C5',
      },
      {
        id: 'L-205',
        title: 'iOS / iPad shells (read-only)',
        meta: 'Platforms',
        value: '68',
        secondary: 'I4 · E5 · C2',
      },
      {
        id: 'L-184',
        title: 'Document MCP tool registry format',
        meta: 'Docs',
        value: '60',
        secondary: 'I2 · E1 · C5',
      },
    ],
    bridgeMethod: 'dappco.re/lthn/desktop/pkg/tasks.Service.List',
    bridgeArgs: [{}],
    liveKeys: ['issues'],
    footer: 'weighted by impact × confidence ÷ effort · top 8 fit the next sprint',
  };
}

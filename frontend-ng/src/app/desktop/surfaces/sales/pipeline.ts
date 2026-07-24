import { Component } from '@angular/core';
import { SurfaceConfig, SurfacePage, SurfaceRoute } from '../surface-page';

@Component({
  selector: 'lthn-sales-pipeline-surface',
  imports: [SurfacePage],
  template: `<lthn-surface-page [config]="config" />`,
})
export class SalesPipelineSurface extends SurfaceRoute {
  readonly config: SurfaceConfig = {
    id: 'sales-pipeline',
    title: $localize`:Sales pipeline title@@surface.sales.pipeline.title:Pipeline`,
    subtitle: $localize`:Sales pipeline subtitle@@surface.sales.pipeline.subtitle:11 deals · £558 K total`,
    icon: 'filter-circle-dollar',
    kind: 'board',
    searchPlaceholder: 'Filter customers or deal notes',
    columns: [
      {
        id: 'qual',
        title: 'Qualifying',
        value: '£64 K',
        cards: [
          {
            id: 'northwold',
            title: 'Northwold Council',
            value: '£18 K',
            detail: 'public sector · pilot interest',
          },
          {
            id: 'heritage-law',
            title: 'Heritage Law LLP',
            value: '£24 K',
            detail: 'GDPR + privilege',
          },
          {
            id: 'marrow-health',
            title: 'Marrow Health · Manchester',
            value: '£22 K',
            detail: 'on-premises inference',
          },
        ],
      },
      {
        id: 'engage',
        title: 'Engaging',
        value: '£128 K',
        cards: [
          { id: 'stannard', title: 'Stannard & Co', value: '£44 K', detail: '7 partners · pilot' },
          {
            id: 'pemberton',
            title: 'Pemberton Capital',
            value: '£62 K',
            detail: 'compliance memo signed',
          },
          {
            id: 'lichfield',
            title: 'Lichfield NHS Trust',
            value: '£22 K',
            detail: 'DPIA in review',
          },
        ],
      },
      {
        id: 'propose',
        title: 'Proposal',
        value: '£218 K',
        cards: [
          {
            id: 'crown',
            title: 'Crown Estates · IT',
            value: '£82 K',
            detail: 'SOW v3 · awaiting signature',
          },
          {
            id: 'cobbet',
            title: 'Cobbet Industries',
            value: '£68 K',
            detail: '3-year hosted plan',
          },
          {
            id: 'greenline',
            title: 'GreenLine Logistics',
            value: '£68 K',
            detail: 'GDPR + sovereign clause',
          },
        ],
      },
      {
        id: 'close',
        title: 'Closing',
        value: '£148 K',
        cards: [
          {
            id: 'whitethorn',
            title: 'Whitethorn Press',
            value: '£36 K',
            detail: 'signature this week',
          },
          {
            id: 'calliope',
            title: 'Calliope Partners',
            value: '£112 K',
            detail: 'final terms · legal review',
          },
        ],
      },
    ],
    sections: [
      {
        title: 'Move safety',
        body: 'Terminal won/lost transitions remain visibly confirmable and are validated by the backend legal-transition table.',
        tone: 'brand',
      },
    ],
    bridgeMethod: 'dappco.re/lthn/desktop/pkg/sales/pipeline.Service.List',
    bridgeArgs: [{}],
    moveBridgeMethod: 'dappco.re/lthn/desktop/pkg/sales/pipeline.Service.MoveDeal',
    conflictReloadService: 'sales.pipeline.update',
    footer: 'drag cards across stages · win probabilities applied to forecast',
  };
}

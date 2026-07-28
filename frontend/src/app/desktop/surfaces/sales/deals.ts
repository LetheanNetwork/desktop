import { Component } from '@angular/core';
import { SurfaceConfig, SurfacePage, SurfaceRoute } from '../surface-page';

@Component({
  selector: 'lthn-sales-deals-surface',
  imports: [SurfacePage],
  template: `<lthn-surface-page [config]="config" />`,
})
export class SalesDealsSurface extends SurfaceRoute {
  readonly config: SurfaceConfig = {
    id: 'sales-deals',
    title: $localize`:Sales deal title@@surface.sales.deals.title:Deal · Heritage Law LLP`,
    subtitle: $localize`:Sales deal subtitle@@surface.sales.deals.subtitle:Engaging · £24 K`,
    icon: 'handshake',
    metrics: [
      { label: 'Value', value: '£24 K', tone: 'brand' },
      { label: 'Probability', value: '60%' },
      { label: 'Close target', value: '14 Jun' },
      { label: 'Stage', value: 'Engaging' },
    ],
    filters: [
      { id: 'call', label: 'Calls' },
      { id: 'email', label: 'Email' },
      { id: 'meet', label: 'Meetings' },
    ],
    rows: [
      {
        id: 'today-1402',
        title: '30-minute call with Ada Penley',
        meta: 'you · today 14:02',
        detail: 'Walked through privacy posture; no blockers. Sending SOW v2 by Friday.',
        tags: ['call'],
      },
      {
        id: 'yest-1618',
        title: "Replied · 'Looking forward to seeing the proposal'",
        meta: 'Ada P. · yesterday 16:18',
        detail: "We'll need to loop in our DPO.",
        tags: ['email'],
      },
      {
        id: '3d-demo',
        title: 'Demo session · two partners + IT director',
        meta: 'you · 3 d ago',
        detail: 'Positive signals on the on-premises story.',
        tags: ['meet'],
      },
      {
        id: '1w-intro',
        title: 'Sent intro deck + Q&A document',
        meta: 'you · 1 w ago',
        detail: '6 questions answered, 2 deferred to Imogen.',
        tags: ['email'],
      },
    ],
    sections: [
      {
        title: 'Stakeholders',
        items: ['Ada Penley · CTO', 'Imogen Beck · DPO', 'Marcus Stannard · Partner'],
      },
      {
        title: 'Documents',
        items: ['SOW v2 · draft', 'NDA · signed', 'Q&A document · shared'],
        tone: 'brand',
      },
    ],
    bridgeMethod: 'dappco.re/lthn/desktop/pkg/sales/deals.Service.List',
    conflictReloadService: 'sales.deals.update',
    bridgeArgs: [{}],
    liveKeys: ['deals'],
    footer: 'auto-logged from email + calendar · sensitive information encrypted at rest',
  };
}

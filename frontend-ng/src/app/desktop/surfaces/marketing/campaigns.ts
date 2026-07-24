import { Component } from '@angular/core';
import { SurfaceConfig, SurfacePage, SurfaceRoute } from '../surface-page';

@Component({
  selector: 'lthn-marketing-campaigns-surface',
  imports: [SurfacePage],
  template: `<lthn-surface-page [config]="config" />`,
})
export class MarketingCampaignsSurface extends SurfaceRoute {
  readonly config: SurfaceConfig = {
    id: 'marketing-campaigns',
    title: $localize`:Marketing campaigns title@@surface.marketing.campaigns.title:Campaigns`,
    subtitle: $localize`:Marketing campaigns subtitle@@surface.marketing.campaigns.subtitle:6 active threads`,
    icon: 'bullhorn',
    metrics: [
      { label: 'Live', value: '2', tone: 'brand' },
      { label: 'Scheduled', value: '2' },
      { label: 'Draft', value: '1' },
      { label: 'Spend', value: '£800' },
    ],
    filters: [
      { id: 'live', label: 'Live' },
      { id: 'scheduled', label: 'Scheduled' },
      { id: 'draft', label: 'Draft' },
      { id: 'complete', label: 'Complete' },
    ],
    rows: [
      {
        id: 'v02-launch',
        title: "v0.2 launch · 'sovereign compute'",
        meta: 'earned · reach 42 K',
        status: 'live',
        value: '3.2%',
        secondary: '£0',
      },
      {
        id: 'investor-q2',
        title: 'Investor outreach · Q2 cohort',
        meta: 'direct · reach 38',
        status: 'live',
        value: '21%',
        secondary: '£0',
      },
      {
        id: 'host-uk',
        title: 'Host UK · email re-engage',
        meta: 'email · reach 4.2 K',
        status: 'scheduled',
        value: '—',
        secondary: '£0',
      },
      {
        id: 'reddit-ama',
        title: 'Reddit AMA · r/LocalLLaMA',
        meta: 'earned · reach ~180 K',
        status: 'scheduled',
        value: '—',
        secondary: '£0',
      },
      {
        id: 'dev-rel',
        title: 'Lethean dev-rel sponsorship',
        meta: 'paid',
        status: 'draft',
        value: '—',
        secondary: '£800',
      },
      {
        id: 'q1-retro',
        title: 'Q1 retrospective · post-mortem',
        meta: 'earned · reach 18 K',
        status: 'complete',
        value: '1.8%',
        secondary: '£0',
      },
    ],
    bridgeMethod: 'dappco.re/lthn/desktop/pkg/marketing/campaigns.Service.List',
    pollMs: 60_000,
    conflictReloadService: 'campaigns.update',
    bridgeArgs: [{}],
    liveKeys: ['campaigns'],
    footer: 'earned-media heavy · UTM-tracked · numbers refresh hourly',
  };
}

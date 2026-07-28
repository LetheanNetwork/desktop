import { Component } from '@angular/core';
import { SurfaceConfig, SurfacePage, SurfaceRoute } from '../surface-page';

@Component({
  selector: 'lthn-marketing-audience-surface',
  imports: [SurfacePage],
  template: `<lthn-surface-page [config]="config" />`,
})
export class MarketingAudienceSurface extends SurfaceRoute {
  readonly config: SurfaceConfig = {
    id: 'marketing-audience',
    title: $localize`:Marketing audience title@@surface.marketing.audience.title:Audience`,
    subtitle: $localize`:Marketing audience subtitle@@surface.marketing.audience.subtitle:5 segments · 8.2 K subscribers`,
    icon: 'users',
    chart: [284, 302, 318, 344, 362, 401, 438, 482, 512, 548],
    metrics: [
      { label: 'Subscribers', value: '8,214', hint: '+184 / week', tone: 'brand' },
      { label: 'Opt-in runtime users', value: '2,618', hint: '30 days' },
    ],
    rows: [
      { id: 'all', title: 'All subscribers', meta: 'all', value: '8,214', secondary: '+184 / w' },
      {
        id: 'developers',
        title: 'Local-AI developers',
        meta: 'signup-tagged',
        value: '4,892',
        secondary: '+62 / w',
      },
      {
        id: 'smb-uk',
        title: 'Regulated SMB · UK',
        meta: 'sales-tagged',
        value: '1,284',
        secondary: '+18 / w',
      },
      {
        id: 'investors',
        title: 'Investors · followed',
        meta: 'manual',
        value: '142',
        secondary: '+4 / w',
      },
      {
        id: 'runtime',
        title: 'Active runtime users (30 d)',
        meta: 'telemetry · opt-in',
        value: '2,618',
        secondary: '+312 / w',
      },
    ],
    bridgeMethod: 'dappco.re/lthn/desktop/pkg/marketing/audience.Service.List',
    pollMs: 60_000,
    conflictReloadService: 'audience.update',
    bridgeArgs: [{}],
    liveKeys: ['segments'],
    footer: 'signup tagging · GDPR-compliant · opt-in telemetry · no third-party tracking',
  };
}

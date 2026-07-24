import { Component } from '@angular/core';
import { SurfaceConfig, SurfacePage, SurfaceRoute } from '../surface-page';

@Component({
  selector: 'lthn-marketing-analytics-surface',
  imports: [SurfacePage],
  template: `<lthn-surface-page [config]="config" />`,
})
export class MarketingAnalyticsSurface extends SurfaceRoute {
  readonly config: SurfaceConfig = {
    id: 'marketing-analytics',
    title: $localize`:Marketing analytics title@@surface.marketing.analytics.title:Analytics`,
    subtitle: $localize`:Marketing analytics subtitle@@surface.marketing.analytics.subtitle:Web · 30 days`,
    icon: 'chart-line',
    kind: 'dashboard',
    chart: [1.2, 1.4, 1.6, 1.5, 1.9, 2.1, 2.4, 2.2, 2.6, 2.8, 3.1, 3.3],
    metrics: [
      { label: 'Sessions', value: '48.4 K', hint: '30 days', tone: 'brand' },
      { label: 'Visitors', value: '31.9 K' },
      { label: 'Bounce', value: '27%' },
      { label: 'Median visit', value: '3:14' },
    ],
    filters: [
      { id: '7d', label: '7 d' },
      { id: '30d', label: '30 d' },
      { id: '90d', label: '90 d' },
    ],
    rows: [
      { id: 'direct', title: 'Direct', meta: 'top source', value: '18.4 K', progress: 38 },
      { id: 'hn', title: 'Hacker News', meta: 'referral', value: '11.6 K', progress: 24 },
      { id: 'local-llama', title: 'r/LocalLLaMA', meta: 'referral', value: '6.8 K', progress: 14 },
      { id: 'x', title: 'Twitter / X', meta: 'social', value: '5.3 K', progress: 11 },
      { id: 'github', title: 'GitHub README', meta: 'referral', value: '3.9 K', progress: 8 },
      { id: 'other', title: 'Other', meta: 'mixed', value: '2.4 K', progress: 5 },
    ],
    sections: [
      {
        title: 'Top pages',
        items: [
          '/ · 18.2 K · 32% bounce · 2:14',
          '/sovereign-compute · 8.8 K · 21% · 4:42',
          '/blog/v0.1-retro · 4.2 K · 18% · 6:18',
          '/docs/install · 3.8 K · 15% · 3:54',
          '/pricing · 2.4 K · 42% · 1:32',
        ],
      },
    ],
    bridgeMethod: 'dappco.re/lthn/desktop/pkg/marketing/analytics.Service.Get',
    bridgeArgs: [{}],
    liveKeys: ['sources', 'pages'],
    footer: 'cookieless · GDPR-compliant · self-hosted Plausible · refreshes hourly',
  };
}

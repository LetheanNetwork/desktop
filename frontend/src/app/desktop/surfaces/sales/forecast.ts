import { Component } from '@angular/core';
import { SurfaceConfig, SurfacePage, SurfaceRoute } from '../surface-page';

@Component({
  selector: 'lthn-sales-forecast-surface',
  imports: [SurfacePage],
  template: `<lthn-surface-page [config]="config" />`,
})
export class SalesForecastSurface extends SurfaceRoute {
  readonly config: SurfaceConfig = {
    id: 'sales-forecast',
    title: $localize`:Sales forecast title@@surface.sales.forecast.title:Forecast`,
    subtitle: $localize`:Sales forecast subtitle@@surface.sales.forecast.subtitle:4 quarters · £000`,
    icon: 'chart-column',
    kind: 'dashboard',
    chart: [142, 196, 280, 420],
    metrics: [
      { label: 'Y1 target ARR', value: '£420 K' },
      { label: 'Committed', value: '£142 K', tone: 'brand' },
      { label: 'Best case', value: '£280 K' },
      { label: 'Probability-weighted', value: '£196 K' },
    ],
    rows: [
      {
        id: 'q2-2026',
        title: 'Q2 · 2026',
        meta: 'target £200 K · low £108 K',
        status: 'open',
        value: '£142 K committed',
        secondary: '£188 K best',
        progress: 71,
      },
      {
        id: 'q3-2026',
        title: 'Q3 · 2026',
        meta: 'target £300 K · low £120 K',
        status: 'open',
        value: '£64 K committed',
        secondary: '£280 K best',
        progress: 21,
      },
      {
        id: 'q4-2026',
        title: 'Q4 · 2026',
        meta: 'target £420 K · low £140 K',
        status: 'open',
        value: '£18 K committed',
        secondary: '£360 K best',
        progress: 4,
      },
      {
        id: 'q1-2027',
        title: 'Q1 · 2027',
        meta: 'target £580 K · low £180 K',
        status: 'plan',
        value: '£0 committed',
        secondary: '£460 K best',
        progress: 0,
      },
    ],
    bridgeMethod: 'dappco.re/lthn/desktop/pkg/sales/forecast.Service.Quarterly',
    bridgeArgs: [{}],
    liveKeys: ['quarters'],
    footer: 'probabilities applied per stage · best case = committed + 60% engaged + 30% proposal',
  };
}

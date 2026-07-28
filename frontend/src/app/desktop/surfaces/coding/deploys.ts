import { Component } from '@angular/core';
import { SurfaceConfig, SurfacePage, SurfaceRoute } from '../surface-page';

@Component({
  selector: 'lthn-coding-deploys-surface',
  imports: [SurfacePage],
  template: `<lthn-surface-page [config]="config" />`,
})
export class CodingDeploysSurface extends SurfaceRoute {
  readonly config: SurfaceConfig = {
    id: 'coding-deploys',
    title: $localize`:Coding deploys title@@surface.coding.deploys.title:Deploys`,
    subtitle: $localize`:Coding deploys subtitle@@surface.coding.deploys.subtitle:3 environments · all green`,
    icon: 'rocket',
    metrics: [
      { label: 'Production', value: 'v0.1.8', hint: 'lthn.ai · 4 d', tone: 'ok' },
      { label: 'Staging', value: 'v0.2.0-rc3', hint: 'staging.lthn.ai · 2 h', tone: 'ok' },
      { label: 'Preview', value: 'PR 482', hint: 'preview.lthn.ai · 22 m', tone: 'ok' },
    ],
    actions: [{ id: 'refresh', label: 'Refresh', icon: 'rotate', kind: 'refresh' }],
    rows: [
      {
        id: 'b8e034',
        title: 'preview · v0.2.0-pr482',
        meta: 'Tobi · 14:32',
        status: 'success',
        value: '58 s',
      },
      {
        id: 'a3f12c',
        title: 'staging · v0.2.0-rc3',
        meta: 'you · 10:42',
        status: 'success',
        value: '2 m 18 s',
      },
      {
        id: 'e1d99c',
        title: 'staging · rollback',
        meta: 'Mei · yesterday',
        status: 'rolled-back',
        value: '1 m 50 s',
      },
      {
        id: '4a82c1',
        title: 'production · v0.1.8',
        meta: 'Mei · yesterday',
        status: 'success',
        value: '4 m 12 s',
      },
    ],
    bridgeMethod: 'dappco.re/lthn/desktop/pkg/deploys.Service.List',
    pollMs: 60_000,
    bridgeArgs: [{}],
    liveKeys: ['envs', 'history'],
    footer: 'auto-deploy preview on PR open · staging on main · production on tag',
  };
}

import { Component } from '@angular/core';
import { SurfaceConfig, SurfacePage, SurfaceRoute } from '../surface-page';

@Component({
  selector: 'lthn-operations-status-surface',
  imports: [SurfacePage],
  template: `<lthn-surface-page [config]="config" />`,
})
export class OperationsStatusSurface extends SurfaceRoute {
  readonly config: SurfaceConfig = {
    id: 'operations-status',
    title: $localize`:Operations status title@@surface.operations.status.title:Status`,
    subtitle: $localize`:Operations status subtitle@@surface.operations.status.subtitle:All regions · 90-day uptime`,
    icon: 'signal',
    metrics: [
      { label: 'Overall', value: 'Operational', tone: 'ok' },
      { label: 'Sites', value: '6 / 6', tone: 'ok' },
      { label: 'Median latency', value: '84 ms' },
    ],
    actions: [{ id: 'refresh', label: 'Check now', icon: 'rotate', kind: 'refresh' }],
    rows: [
      {
        id: 'lthn-ai',
        title: 'lthn.ai',
        meta: 'web',
        status: 'ok',
        value: '99.99%',
        secondary: '72 ms',
      },
      {
        id: 'app',
        title: 'app.lthn.ai',
        meta: 'desktop gateway',
        status: 'ok',
        value: '99.98%',
        secondary: '81 ms',
      },
      {
        id: 'host',
        title: 'host.uk.com',
        meta: 'platform',
        status: 'ok',
        value: '99.97%',
        secondary: '93 ms',
      },
      {
        id: 'hub',
        title: 'hub.host.uk.com',
        meta: 'agent hub',
        status: 'ok',
        value: '99.95%',
        secondary: '112 ms',
      },
      {
        id: 'mail',
        title: 'mail.host.uk.com',
        meta: 'mail',
        status: 'ok',
        value: '99.99%',
        secondary: '65 ms',
      },
      {
        id: 'forge',
        title: 'forge.host.uk.com',
        meta: 'source',
        status: 'ok',
        value: '99.96%',
        secondary: '88 ms',
      },
    ],
    bridgeMethod: 'dappco.re/lthn/desktop/pkg/vi.Service.Sites',
    pollMs: 30_000,
    footer: 'Vi.Sites · checked every 60 s · degraded threshold = HTTP 5xx',
  };
}

import { Component } from '@angular/core';
import { SurfaceConfig, SurfacePage, SurfaceRoute } from '../surface-page';

@Component({
  selector: 'lthn-operations-incidents-surface',
  imports: [SurfacePage],
  template: `<lthn-surface-page [config]="config" />`,
})
export class OperationsIncidentsSurface extends SurfaceRoute {
  readonly config: SurfaceConfig = {
    id: 'operations-incidents',
    title: $localize`:Operations incidents title@@surface.operations.incidents.title:Incidents`,
    subtitle: $localize`:Operations incidents subtitle@@surface.operations.incidents.subtitle:1 active · 3 resolved · 90 days`,
    icon: 'triangle-exclamation',
    metrics: [
      { label: 'Investigating', value: '1', tone: 'warn' },
      { label: 'Post-mortem', value: '1' },
      { label: 'Resolved', value: '3', tone: 'ok' },
    ],
    filters: [
      { id: 'investigating', label: 'Investigating' },
      { id: 'post-mortem', label: 'Post-mortem' },
      { id: 'resolved', label: 'Resolved' },
    ],
    rows: [
      {
        id: 'P3-hub',
        title: 'hub.host.uk.com · elevated p99 latency',
        meta: 'hub · Mei · 3 comments',
        status: 'investigating',
        value: 'P3',
        secondary: 'now',
      },
      {
        id: 'P2-mail',
        title: 'Mail delivery delays · upstream Postfix',
        meta: 'mail · you · 14 comments',
        status: 'resolved',
        value: 'P2',
        secondary: '42 min',
      },
      {
        id: 'P3-forge',
        title: 'Forge build queue stalled',
        meta: 'forge · Tobi · 5 comments',
        status: 'resolved',
        value: 'P3',
        secondary: '18 min',
      },
      {
        id: 'P1-dns',
        title: 'Total outage · DNS cache poisoning',
        meta: 'all · 38 comments',
        status: 'post-mortem',
        value: 'P1',
        secondary: '2 h 14',
      },
      {
        id: 'P2-auth',
        title: 'Auth token rotation broke 1% of sessions',
        meta: 'app · you · 9 comments',
        status: 'resolved',
        value: 'P2',
        secondary: '1 h 02',
      },
    ],
    sections: [
      {
        title: 'Vi post-mortem',
        body: 'A draft timeline is ready for the DNS incident. Review before attaching it.',
        tone: 'brand',
      },
    ],
    bridgeMethod: 'dappco.re/lthn/desktop/pkg/incidents.Service.List',
    pollMs: 60_000,
    conflictReloadService: 'incidents.update',
    bridgeArgs: [{}],
    liveKeys: ['incidents'],
    footer: 'pkg/incidents · auto-paged via PagerDuty · runbooks linked from each incident',
  };
}

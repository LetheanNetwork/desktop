import { Component } from '@angular/core';
import { SurfaceConfig, SurfacePage, SurfaceRoute } from '../surface-page';

@Component({
  selector: 'lthn-observe-activity-surface',
  imports: [SurfacePage],
  template: `<lthn-surface-page [config]="config" />`,
})
export class ObserveActivitySurface extends SurfaceRoute {
  readonly config: SurfaceConfig = {
    id: 'observe-activity',
    title: $localize`:Observe activity title@@surface.observe.activity.title:Activity`,
    subtitle: $localize`:Observe activity subtitle@@surface.observe.activity.subtitle:Forensic record of authorisation and lifecycle events`,
    icon: 'timeline',
    actions: [{ id: 'refresh', label: 'Refresh', icon: 'rotate', kind: 'refresh' }],
    filters: [
      { id: 'ok', label: 'OK' },
      { id: 'failed', label: 'Failed' },
      { id: 'denied', label: 'Denied' },
    ],
    rows: [
      {
        id: 'evt-001',
        title: 'auth.session.issued',
        meta: 'session · account fixture0',
        status: 'ok',
        value: '1 m ago',
      },
      {
        id: 'evt-002',
        title: 'tasks.created',
        meta: 'core · system',
        status: 'ok',
        value: '5 m ago',
      },
      {
        id: 'evt-003',
        title: 'queue.enqueued',
        meta: 'queue · system',
        status: 'ok',
        value: '9 m ago',
      },
      {
        id: 'evt-004',
        title: 'auth.unlock.failed',
        meta: 'unlock · account fixture0',
        status: 'failed',
        value: '21 m ago',
      },
    ],
    loadEndpoint: '/v1/audit/events?limit=250',
    pollMs: 60_000,
    liveKeys: ['events'],
    searchPlaceholder: 'Filter event names',
    footer: '~/Lethean/audit/ · session-tier read · capped at 2,000 events · refreshes every 60 s',
  };
}

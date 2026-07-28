import { Component } from '@angular/core';
import { SurfaceConfig, SurfacePage, SurfaceRoute } from '../surface-page';

@Component({
  selector: 'lthn-operations-runbooks-surface',
  imports: [SurfacePage],
  template: `<lthn-surface-page [config]="config" />`,
})
export class OperationsRunbooksSurface extends SurfaceRoute {
  readonly config: SurfaceConfig = {
    id: 'operations-runbooks',
    title: $localize`:Operations runbooks title@@surface.operations.runbooks.title:Runbooks`,
    subtitle: $localize`:Operations runbooks subtitle@@surface.operations.runbooks.subtitle:7 books · 2 stale`,
    icon: 'book',
    metrics: [
      { label: 'Fresh', value: '4', tone: 'ok' },
      { label: 'Ageing', value: '1', tone: 'warn' },
      { label: 'Stale', value: '2', tone: 'danger' },
    ],
    filters: [
      { id: 'fresh', label: 'Fresh' },
      { id: 'aging', label: 'Ageing' },
      { id: 'stale', label: 'Stale' },
    ],
    rows: [
      {
        id: 'R-01',
        title: 'Rotate runtime API keys',
        meta: 'auth',
        status: 'fresh',
        value: '2 d',
      },
      {
        id: 'R-02',
        title: 'Recover from corrupt model directory',
        meta: 'runtime',
        status: 'fresh',
        value: '3 w',
      },
      {
        id: 'R-03',
        title: 'Rollback bad deploy · production',
        meta: 'deploy',
        status: 'fresh',
        value: '5 d',
      },
      {
        id: 'R-04',
        title: 'Reset DNS · cache poisoning incident',
        meta: 'network',
        status: 'fresh',
        value: '2 w',
      },
      {
        id: 'R-05',
        title: 'Drain a stuck Postfix queue',
        meta: 'mail',
        status: 'stale',
        value: '4 mo',
      },
      {
        id: 'R-06',
        title: 'Trigger emergency model unload',
        meta: 'runtime',
        status: 'aging',
        value: '1 mo',
      },
      {
        id: 'R-07',
        title: 'Restore tray app from corrupt config',
        meta: 'client',
        status: 'stale',
        value: '6 mo',
      },
    ],
    searchPlaceholder: 'Search runbook titles or areas',
    sections: [
      {
        title: 'Rehearsal',
        body: 'Two stale books are ready to schedule for a safe rehearsal.',
        tone: 'brand',
      },
    ],
    bridgeMethod: 'dappco.re/lthn/desktop/pkg/runbooks.Service.List',
    pollMs: 60_000,
    conflictReloadService: 'runbooks.update',
    bridgeArgs: [{}],
    liveKeys: ['books'],
    footer: 'books in ~/Lethean/runbooks/ · auto-test daily · simple books rehearse quarterly',
  };
}

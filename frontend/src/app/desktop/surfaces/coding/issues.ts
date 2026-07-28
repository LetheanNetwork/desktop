import { Component } from '@angular/core';
import { SurfaceConfig, SurfacePage, SurfaceRoute } from '../surface-page';

@Component({
  selector: 'lthn-coding-issues-surface',
  imports: [SurfacePage],
  template: `<lthn-surface-page [config]="config" />`,
})
export class CodingIssuesSurface extends SurfaceRoute {
  readonly config: SurfaceConfig = {
    id: 'coding-issues',
    title: $localize`:Coding issues title@@surface.coding.issues.title:Issues`,
    subtitle: $localize`:Coding issues subtitle@@surface.coding.issues.subtitle:6 open · sorted by latest activity`,
    icon: 'circle-dot',
    searchPlaceholder: 'Search issues',
    filters: [
      { id: 'bug', label: 'Bug' },
      { id: 'enhancement', label: 'Enhancement' },
      { id: 'docs', label: 'Docs' },
      { id: 'ui', label: 'UI' },
    ],
    rows: [
      {
        id: '#284',
        title: 'Tray icon active state not updating on model swap',
        meta: 'desktop · 2 h · you',
        status: 'open',
        tags: ['bug', 'ui'],
      },
      {
        id: '#283',
        title: 'Metal kernel fails on M1 Air with 8 GB RAM',
        meta: 'runtime · 5 h · Tobi',
        status: 'open',
        tags: ['bug', 'metal'],
      },
      {
        id: '#281',
        title: 'Add --dry-run flag to lthn deploy',
        meta: 'desktop · yesterday',
        status: 'open',
        tags: ['enhancement'],
      },
      {
        id: '#278',
        title: 'Document the EUPL-1.2 commercial-use clause',
        meta: 'docs · 2 d · Mei',
        status: 'open',
        tags: ['docs'],
      },
      {
        id: '#275',
        title: 'API key regeneration needs a destructive-action warning',
        meta: 'desktop · 3 d · you',
        status: 'open',
        tags: ['bug', 'ui'],
      },
      {
        id: '#271',
        title: 'Benchmark histogram of P50/P95/P99 token latency',
        meta: 'desktop · 5 d',
        status: 'open',
        tags: ['enhancement', 'telemetry'],
      },
    ],
    bridgeMethod: 'dappco.re/lthn/desktop/pkg/tasks.Service.List',
    pollMs: 60_000,
    bridgeArgs: [{}],
    liveKeys: ['issues'],
    footer: 'label filter · pkg/tasks · sorted by last activity',
  };
}

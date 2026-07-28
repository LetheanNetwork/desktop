import { Component } from '@angular/core';
import { SurfaceConfig, SurfacePage, SurfaceRoute } from '../surface-page';

@Component({
  selector: 'lthn-coding-prs-surface',
  imports: [SurfacePage],
  template: `<lthn-surface-page [config]="config" />`,
})
export class CodingPrsSurface extends SurfaceRoute {
  readonly config: SurfaceConfig = {
    id: 'coding-prs',
    title: $localize`:Coding pull requests title@@surface.coding.prs.title:Pull requests`,
    subtitle: $localize`:Coding pull requests subtitle@@surface.coding.prs.subtitle:5 open · 2 awaiting you`,
    icon: 'code-pull-request',
    actions: [{ id: 'refresh', label: 'Refresh', icon: 'rotate', kind: 'refresh' }],
    filters: [
      { id: 'forge', label: 'Forge' },
      { id: 'github', label: 'GitHub' },
    ],
    rows: [
      {
        id: '#482',
        title: 'LoRA training UI · final pass',
        meta: 'desktop · Tobi · 18 m',
        detail: '+482 −38',
        status: 'review-requested',
        tags: ['forge'],
      },
      {
        id: '#479',
        title: 'Quantisation auto-picker per hardware',
        meta: 'go-mlx · you · 2 h',
        detail: '+128 −12',
        status: 'draft',
        tags: ['github'],
      },
      {
        id: '#477',
        title: 'Settings sectioned scroll refactor',
        meta: 'desktop · Mei · 5 h',
        detail: '+312 −284',
        status: 'changes-requested',
        tags: ['forge'],
      },
      {
        id: '#474',
        title: 'Document MCP tool registry format',
        meta: 'core-go · Tobi · yesterday',
        detail: '+142 −0',
        status: 'approved',
        tags: ['forge'],
      },
    ],
    bridgeMethod: 'dappco.re/lthn/desktop/pkg/vi.Service.Activity',
    pollMs: 60_000,
    footer: 'watched repositories · highlighted rows require your action · data via Vi.Activity()',
  };
}

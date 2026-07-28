import { Component } from '@angular/core';
import { SurfaceConfig, SurfacePage, SurfaceRoute } from '../surface-page';

@Component({
  selector: 'lthn-coding-repos-surface',
  imports: [SurfacePage],
  template: `<lthn-surface-page [config]="config" />`,
})
export class CodingReposSurface extends SurfaceRoute {
  readonly config: SurfaceConfig = {
    id: 'coding-repos',
    title: $localize`:Coding repositories title@@surface.coding.repos.title:Repos`,
    subtitle: $localize`:Coding repositories subtitle@@surface.coding.repos.subtitle:6 watched repositories`,
    icon: 'code-branch',
    metrics: [
      { label: 'Passing', value: '4', tone: 'ok' },
      { label: 'Running', value: '1', tone: 'brand' },
      { label: 'Failing', value: '1', tone: 'danger' },
      { label: 'Open PRs', value: '8' },
    ],
    actions: [{ id: 'refresh', label: 'Scan', icon: 'rotate', kind: 'refresh' }],
    rows: [
      {
        id: 'desktop',
        title: 'lthn / desktop',
        meta: 'TypeScript + Go · lane/surface-to-hash',
        status: 'running',
        value: '3 PRs',
        secondary: 'a3f12c',
      },
      {
        id: 'go-mlx',
        title: 'core / go-mlx',
        meta: 'Go · dev',
        status: 'passing',
        value: '2 PRs',
        secondary: '72c410',
      },
      {
        id: 'go-inference',
        title: 'core / go-inference',
        meta: 'Go · dev',
        status: 'passing',
        value: '1 PR',
        secondary: '11bd9e',
      },
      {
        id: 'host-uk',
        title: 'host-uk / platform',
        meta: 'Go · main',
        status: 'failing',
        value: '2 PRs',
        secondary: '6aa04d',
      },
    ],
    bridgeMethod: 'dappco.re/lthn/desktop/pkg/repos.Service.Status',
    pollMs: 60_000,
    bridgeArgs: [{}],
    searchPlaceholder: 'Filter watched repositories',
    footer: 'pkg/repos · refreshed every 60 s · workspace: ~/Code/{core,lthn,host-uk,lab,snider}',
  };
}

import { Component } from '@angular/core';
import { SurfaceConfig, SurfacePage, SurfaceRoute } from '../surface-page';

@Component({
  selector: 'lthn-agents-activity-surface',
  imports: [SurfacePage],
  template: `<lthn-surface-page [config]="config" />`,
})
export class AgentsActivitySurface extends SurfaceRoute {
  readonly config: SurfaceConfig = {
    id: 'agents-activity',
    title: $localize`:Agents activity title@@surface.agents.activity.title:Activity`,
    subtitle: $localize`:Agents activity subtitle@@surface.agents.activity.subtitle:Live CoreAgent runs across the fleet`,
    icon: 'wave-square',
    metrics: [
      { label: 'Running', value: '2', tone: 'brand' },
      { label: 'Blocked', value: '1', tone: 'warn' },
      { label: 'Completed', value: '18', tone: 'ok' },
      { label: 'Failed', value: '1', tone: 'danger' },
    ],
    filters: [
      { id: 'running', label: 'Running' },
      { id: 'blocked', label: 'Blocked' },
      { id: 'completed', label: 'Completed' },
    ],
    actions: [{ id: 'refresh', label: 'Refresh', icon: 'rotate', kind: 'refresh' }],
    rows: [
      {
        id: 'desktop-route-port',
        title: 'Port desktop surfaces to Angular',
        meta: 'claude-code · desktop',
        detail: 'lane/surface-to-hash · 24 files observed',
        status: 'running',
        value: '12m',
      },
      {
        id: 'go-mlx-native-slice',
        title: 'Verify native slice coverage',
        meta: 'codex · go-mlx',
        detail: 'Waiting for the focused test result',
        status: 'blocked',
        value: 'question',
      },
      {
        id: 'docs-release',
        title: 'Prepare v0.2 release notes',
        meta: 'vi · docs',
        detail: 'Merged as PR #482',
        status: 'completed',
        value: '8m',
      },
    ],
    bridgeMethod: 'dappco.re/lthn/desktop/pkg/agents.Service.Workspaces',
    pollMs: 30_000,
    searchPlaceholder: 'Filter runs, agents or repositories',
    footer:
      'live CoreAgent runs · answer a blocked run to resume it · push events + 30 s reconciliation · via lthn-agent CLI',
  };
}

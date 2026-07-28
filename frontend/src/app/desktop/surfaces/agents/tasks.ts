import { Component } from '@angular/core';
import { SurfaceConfig, SurfacePage, SurfaceRoute } from '../surface-page';

@Component({
  selector: 'lthn-agents-tasks-surface',
  imports: [SurfacePage],
  template: `<lthn-surface-page [config]="config" />`,
})
export class AgentsTasksSurface extends SurfaceRoute {
  readonly config: SurfaceConfig = {
    id: 'agents-tasks',
    title: $localize`:Agents tasks title@@surface.agents.tasks.title:Backlog`,
    subtitle: $localize`:Agents tasks subtitle@@surface.agents.tasks.subtitle:The task queue — scan repositories to fill it, then dispatch a row`,
    icon: 'list-check',
    metrics: [
      { label: 'Open', value: '14', tone: 'brand' },
      { label: 'In progress', value: '3', tone: 'warn' },
      { label: 'Lint', value: '6' },
      { label: 'Package updates', value: '4' },
    ],
    filters: [
      { id: 'open', label: 'Open' },
      { id: 'in_progress', label: 'In progress' },
    ],
    rows: [
      {
        id: 'L-201',
        title: 'Federated chat across LetherNet peers',
        meta: 'desktop · manual',
        status: 'open',
        value: 'urgent',
      },
      {
        id: 'L-189',
        title: 'Audit log retention policy + UI control',
        meta: 'desktop · scan',
        status: 'in_progress',
        value: 'high',
      },
      {
        id: 'L-184',
        title: 'Document MCP tool registry format',
        meta: 'core-go · lint',
        status: 'open',
        value: 'normal',
      },
    ],
    searchPlaceholder: 'Search the task queue',
    bridgeMethod: 'dappco.re/lthn/desktop/pkg/agents.Service.Tasks',
    liveKeys: ['tasks'],
    footer: 'pkg/tasks job queue · open + in-progress · click a row → Dispatch',
  };
}

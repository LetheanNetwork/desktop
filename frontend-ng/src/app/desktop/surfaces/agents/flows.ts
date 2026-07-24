import { Component } from '@angular/core';
import { SurfaceConfig, SurfacePage, SurfaceRoute } from '../surface-page';

@Component({
  selector: 'lthn-agents-flows-surface',
  imports: [SurfacePage],
  template: `<lthn-surface-page [config]="config" />`,
})
export class AgentsFlowsSurface extends SurfaceRoute {
  readonly config: SurfaceConfig = {
    id: 'agents-flows',
    title: $localize`:Agents flows title@@surface.agents.flows.title:Flows`,
    subtitle: $localize`:Agents flows subtitle@@surface.agents.flows.subtitle:The Do-Engine ability registry`,
    icon: 'diagram-project',
    metrics: [
      { label: 'Abilities', value: '18', tone: 'brand' },
      { label: 'Groups', value: '6' },
      { label: 'Compositions', value: '3', tone: 'ok' },
    ],
    filters: [
      { id: 'files', label: 'Files' },
      { id: 'tasks', label: 'Tasks' },
      { id: 'agents', label: 'Agents' },
      { id: 'runtime', label: 'Runtime' },
    ],
    actions: [{ id: 'refresh', label: 'Reload catalogue', icon: 'rotate', kind: 'refresh' }],
    rows: [
      {
        id: 'files_read',
        title: 'Read a file',
        meta: 'files',
        detail: '{ path: string } → file content',
        status: 'available',
        tags: ['files'],
      },
      {
        id: 'tasks_list',
        title: 'List queued tasks',
        meta: 'tasks',
        detail: '{ state?: string } → Issue[]',
        status: 'available',
        tags: ['tasks'],
      },
      {
        id: 'agentic_dispatch',
        title: 'Dispatch an agent',
        meta: 'agents',
        detail: '{ repo, agent, task } → detached run',
        status: 'available',
        tags: ['agents'],
      },
      {
        id: 'runner_generate',
        title: 'Generate with a local model',
        meta: 'runtime',
        detail: '{ route, prompt } → generation',
        status: 'available',
        tags: ['runtime'],
      },
    ],
    bridgeMethod: 'dappco.re/lthn/desktop/pkg/tools.WailsService.List',
    searchPlaceholder: 'Search names, descriptions or groups',
    footer:
      'codified abilities · uniform input/output contract · runtime executes · data via Tools.List()',
  };
}

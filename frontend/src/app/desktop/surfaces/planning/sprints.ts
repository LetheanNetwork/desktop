import { Component } from '@angular/core';
import { SurfaceConfig, SurfacePage, SurfaceRoute } from '../surface-page';

@Component({
  selector: 'lthn-planning-sprints-surface',
  imports: [SurfacePage],
  template: `<lthn-surface-page [config]="config" />`,
})
export class PlanningSprintsSurface extends SurfaceRoute {
  readonly config: SurfaceConfig = {
    id: 'planning-sprints',
    title: $localize`:Planning sprint title@@surface.planning.sprints.title:Sprint 24`,
    subtitle: $localize`:Planning sprint subtitle@@surface.planning.sprints.subtitle:12 May → 23 May · 8 days remaining`,
    icon: 'table-columns',
    kind: 'board',
    chart: [14, 16, 18, 17, 15, 18, 14],
    searchPlaceholder: 'Filter cards by id or title',
    actions: [{ id: 'add', label: 'Add', icon: 'plus', kind: 'add' }],
    columns: [
      {
        id: 'todo',
        title: 'To do',
        cards: [
          { id: 'L-184', title: 'Document MCP tool registry format', value: '3', tags: ['docs'] },
          {
            id: 'L-185',
            title: 'Wire HuggingFace search to model browser',
            value: '5',
            tags: ['feat'],
          },
          { id: 'L-187', title: 'Telemetry · per-model success rate', value: '2', tags: ['feat'] },
          { id: 'L-189', title: 'Audit log retention policy', value: '1', tags: ['ops'] },
        ],
      },
      {
        id: 'doing',
        title: 'In flight',
        cards: [
          {
            id: 'L-181',
            title: 'v0.2 release notes + announcement',
            meta: 'you',
            value: '3',
            tags: ['ship'],
          },
          {
            id: 'L-182',
            title: 'LoRA training UI · final pass',
            meta: 'Tobi',
            value: '5',
            tags: ['feat'],
          },
          {
            id: 'L-180',
            title: 'Linux tray binary · packaging',
            meta: 'Mei',
            value: '8',
            tags: ['platform'],
          },
        ],
      },
      {
        id: 'review',
        title: 'Review',
        cards: [
          {
            id: 'L-178',
            title: 'Settings · sectioned scroll refactor',
            meta: 'you → Tobi',
            value: '3',
            tags: ['ui'],
          },
          {
            id: 'L-176',
            title: 'Tools window · try-it sandbox',
            meta: 'Tobi → Mei',
            value: '5',
            tags: ['feat'],
          },
        ],
      },
      {
        id: 'done',
        title: 'Done',
        cards: [
          { id: 'L-173', title: 'Tray icon SVG family', value: '2', tags: ['design'] },
          { id: 'L-171', title: 'Benchmark history persistence', value: '3', tags: ['feat'] },
          { id: 'L-170', title: 'MCP tools window v1', value: '8', tags: ['feat'] },
          { id: 'L-168', title: 'Status pill component', value: '1', tags: ['ui'] },
          { id: 'L-165', title: 'Live log filter rail', value: '2', tags: ['feat'] },
        ],
      },
    ],
    bridgeMethod: 'dappco.re/lthn/desktop/pkg/tasks.Service.List',
    bridgeArgs: [{}],
    liveKeys: ['issues'],
    footer: '14 done · 3 in flight · 2 in review · 4 to do · burndown on track',
  };
}

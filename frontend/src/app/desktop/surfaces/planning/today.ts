import { Component } from '@angular/core';
import { SurfaceConfig, SurfacePage, SurfaceRoute } from '../surface-page';

@Component({
  selector: 'lthn-planning-today-surface',
  imports: [SurfacePage],
  template: `<lthn-surface-page [config]="config" />`,
})
export class PlanningTodaySurface extends SurfaceRoute {
  readonly config: SurfaceConfig = {
    id: 'planning-today',
    title: $localize`:Planning today title@@surface.planning.today.title:Today`,
    subtitle: $localize`:Planning today subtitle@@surface.planning.today.subtitle:Friday · 16 May · 09:14`,
    icon: 'sun',
    chart: [9, 11, 14, 12, 16, 14, 18],
    metrics: [
      { label: 'Sprint velocity', value: '14 / 18', tone: 'brand' },
      { label: 'In flight', value: '3' },
      { label: 'Next milestone', value: '16:00' },
    ],
    filters: [
      { id: 'focus', label: 'Focus' },
      { id: 'agenda', label: 'Agenda' },
      { id: 'shipped', label: 'Shipped' },
    ],
    rows: [
      {
        id: 'focus-release',
        title: 'Lethean v0.2 release preparation',
        meta: 'today · 16:00',
        status: 'high',
        tags: ['focus'],
        tone: 'brand',
      },
      {
        id: 'focus-lora',
        title: 'Review LoRA training PR #482',
        meta: 'today · EOD',
        status: 'medium',
        tags: ['focus'],
      },
      {
        id: 'focus-calliope',
        title: 'Investor call · Calliope VC',
        meta: 'today · 14:30',
        status: 'high',
        tags: ['focus'],
        tone: 'brand',
      },
      {
        id: 'agenda-09',
        title: 'Standup · core team',
        meta: '09:00',
        value: '15 min',
        tags: ['agenda'],
      },
      {
        id: 'agenda-10',
        title: 'Deep work · release notes',
        meta: '10:00',
        value: '2 h',
        tags: ['agenda'],
      },
      {
        id: 'agenda-14',
        title: 'Calliope VC · pitch',
        meta: '14:30',
        value: '45 min',
        tags: ['agenda'],
      },
      {
        id: 'agenda-16',
        title: 'Lethean v0.2 release',
        meta: '16:00',
        value: '1 h',
        tags: ['agenda'],
      },
      {
        id: 'ship-mcp',
        title: 'MCP tools window · v0.3 → main',
        meta: 'you · 14:32',
        tags: ['shipped'],
        status: 'completed',
      },
      {
        id: 'ship-icon',
        title: 'Tray icon SVG family · production',
        meta: 'Tobi · 11:08',
        tags: ['shipped'],
        status: 'completed',
      },
      {
        id: 'ship-bench',
        title: 'Benchmark history persistence',
        meta: 'you · 09:44',
        tags: ['shipped'],
        status: 'completed',
      },
    ],
    sections: [
      {
        title: 'Vi daily brief',
        body: 'The release is the day anchor. Protect the 10:00 focus block; the Calliope deck is already current.',
        tone: 'brand',
      },
    ],
    bridgeMethod: 'dappco.re/lthn/desktop/pkg/tasks.Service.List',
    bridgeArgs: [{}],
    liveKeys: ['issues'],
    footer: '14 of 18 sprint tasks complete · next milestone: v0.2 release · 16:00',
  };
}

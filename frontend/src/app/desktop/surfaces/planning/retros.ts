import { Component } from '@angular/core';
import { SurfaceConfig, SurfacePage, SurfaceRoute } from '../surface-page';

@Component({
  selector: 'lthn-planning-retros-surface',
  imports: [SurfacePage],
  template: `<lthn-surface-page [config]="config" />`,
})
export class PlanningRetrosSurface extends SurfaceRoute {
  readonly config: SurfaceConfig = {
    id: 'planning-retros',
    title: $localize`:Planning retrospective title@@surface.planning.retros.title:Retro`,
    subtitle: $localize`:Planning retrospective subtitle@@surface.planning.retros.subtitle:Sprint 23 · 9 May`,
    icon: 'arrows-rotate',
    kind: 'board',
    actions: [{ id: 'add', label: 'Add note', icon: 'plus', kind: 'add' }],
    columns: [
      {
        id: 'good',
        title: 'Went well',
        cards: [
          { id: 'good-1', title: 'Tray icon family landed on first review', meta: 'Tobi' },
          { id: 'good-2', title: 'MCP tools window shipped 2 days early', meta: 'you' },
          { id: 'good-3', title: 'First 50 closed-alpha users active', meta: 'Mei' },
        ],
      },
      {
        id: 'bad',
        title: "Didn't work",
        cards: [
          { id: 'bad-1', title: 'Linux packaging slipped a sprint', meta: 'Mei' },
          {
            id: 'bad-2',
            title: 'Settings refactor blocked release-notes preparation',
            meta: 'you',
          },
        ],
      },
      {
        id: 'next',
        title: 'Try next sprint',
        cards: [
          { id: 'next-1', title: 'Lock release scope by Wednesday', meta: 'you' },
          { id: 'next-2', title: 'Pair on Linux packaging', meta: 'Mei + Tobi' },
          { id: 'next-3', title: 'Move design review to async Loom', meta: 'all' },
        ],
      },
    ],
    footer: 'actions assigned · review at sprint 24 close · history at retros/2026-05-09.md',
  };
}

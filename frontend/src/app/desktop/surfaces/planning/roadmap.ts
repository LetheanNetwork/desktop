import { Component } from '@angular/core';
import { SurfaceConfig, SurfacePage, SurfaceRoute } from '../surface-page';

@Component({
  selector: 'lthn-planning-roadmap-surface',
  imports: [SurfacePage],
  template: `<lthn-surface-page [config]="config" />`,
})
export class PlanningRoadmapSurface extends SurfaceRoute {
  readonly config: SurfaceConfig = {
    id: 'planning-roadmap',
    title: $localize`:Planning roadmap title@@surface.planning.roadmap.title:Roadmap`,
    subtitle: $localize`:Planning roadmap subtitle@@surface.planning.roadmap.subtitle:Q2 2026 → Q1 2027`,
    icon: 'road',
    kind: 'board',
    searchPlaceholder: 'Filter roadmap items',
    columns: [
      {
        id: 'L1',
        title: 'Tray · L1',
        cards: [
          { id: 'q2-v02', title: 'v0.2 GA', meta: 'Q2 · 2026', status: 'completed' },
          { id: 'q3-desktop', title: 'Windows + Linux beta', meta: 'Q3 · 2026' },
          { id: 'q4-lora', title: 'LoRA training in-app', meta: 'Q4 · 2026' },
          { id: 'q1-workspaces', title: 'Multi-account workspaces', meta: 'Q1 · 2027' },
        ],
      },
      {
        id: 'L2',
        title: 'Runtime · L2',
        cards: [
          {
            id: 'q2-metal',
            title: 'go-mlx Metal optimisation',
            meta: 'Q2 · 2026',
            status: 'completed',
          },
          { id: 'q3-hip', title: 'HIP backend (AMD)', meta: 'Q3 · 2026' },
          { id: 'q4-cuda', title: 'CUDA backend', meta: 'Q4 · 2026 → Q1 · 2027' },
        ],
      },
      {
        id: 'L3',
        title: 'Fabric · L3',
        cards: [
          { id: 'q3-lethernet', title: 'LetherNet design', meta: 'Q3 · 2026' },
          { id: 'q4-discovery', title: 'Peer discovery alpha', meta: 'Q4 · 2026' },
          { id: 'q1-inference', title: 'Disaggregated inference', meta: 'Q1 · 2027' },
        ],
      },
      {
        id: 'GTM',
        title: 'Commercial',
        cards: [
          {
            id: 'q2-host',
            title: 'Host UK channel pilots',
            meta: 'Q2 · 2026',
            status: 'completed',
          },
          { id: 'q4-hosted', title: 'Hosted L2 commercial GA', meta: 'Q4 · 2026' },
          { id: 'q1-smb', title: 'Regulated SMB sales motion', meta: 'Q1 · 2027' },
        ],
      },
    ],
    footer: 'Q2 2026 · 3 of 4 lane items shipped · Q3 lane plan locks 1 June',
  };
}

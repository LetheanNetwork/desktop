import { Component } from '@angular/core';
import { SurfaceConfig, SurfacePage, SurfaceRoute } from '../surface-page';

@Component({
  selector: 'lthn-ml-lab-models-surface',
  imports: [SurfacePage],
  template: `<lthn-surface-page [config]="config" />`,
})
export class MlLabModelsSurface extends SurfaceRoute {
  readonly config: SurfaceConfig = {
    id: 'ml-lab-models',
    title: $localize`:ML Lab models title@@surface.mlLab.models.title:Models`,
    subtitle: $localize`:ML Lab models subtitle@@surface.mlLab.models.subtitle:Local .model library — Trix containers, content-addressable`,
    icon: 'cubes',
    actions: [{ id: 'refresh', label: 'Rescan', icon: 'rotate', kind: 'refresh' }],
    filters: [
      { id: '4-bit', label: '4-bit' },
      { id: '8-bit', label: '8-bit' },
      { id: 'moe', label: 'MoE' },
    ],
    rows: [
      {
        id: 'a40925748cf8',
        title: 'lemma-12b-v4-p6.model',
        meta: 'gemma-4 · 4-bit · 262 K ctx',
        status: 'ready',
        value: '6.35 GB',
        secondary: 'go-mlx',
        tags: ['4-bit'],
      },
      {
        id: 'c19fe18b5a3b',
        title: 'lemma-12b-v4-p6.q8.model',
        meta: 'gemma-4 · 8-bit · 262 K ctx',
        status: 'ready',
        value: '11.59 GB',
        secondary: 'go-mlx',
        tags: ['8-bit'],
      },
      {
        id: '2f49ab10c5be',
        title: 'lemer-2b-e2b.q4.model',
        meta: 'gemma-4 · 4-bit · 131 K ctx',
        status: 'ready',
        value: '1.44 GB',
        secondary: 'go-mlx',
        tags: ['4-bit'],
      },
      {
        id: '8e7d6c5b4a39',
        title: 'lemmy-26b-a4b-moe.q4.model',
        meta: 'gemma-4-moe · 4-bit · 262 K ctx',
        status: 'ready',
        value: '12.92 GB',
        secondary: 'go-mlx',
        tags: ['4-bit', 'moe'],
      },
    ],
    searchPlaceholder: 'Filter name, architecture, fingerprint or producer',
    loadEndpoint: '/v1/ml-lab/models',
    pollMs: 60_000,
    liveKeys: ['models'],
    footer: 'SHA-256 identity projection · local model files · refreshed every 60 s',
  };
}

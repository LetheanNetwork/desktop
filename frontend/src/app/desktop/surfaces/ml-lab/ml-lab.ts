import { Component } from '@angular/core';
import { SurfaceConfig, SurfacePage, SurfaceRoute } from '../surface-page';

@Component({
  selector: 'lthn-ml-lab-workbench-surface',
  imports: [SurfacePage],
  template: `<lthn-surface-page [config]="config" />`,
})
export class MlLabWorkbenchSurface extends SurfaceRoute {
  readonly config: SurfaceConfig = {
    id: 'ml-lab-workbench',
    title: $localize`:ML Lab workbench title@@surface.mlLab.workbench.title:ML Lab`,
    subtitle: $localize`:ML Lab workbench subtitle@@surface.mlLab.workbench.subtitle:Workbench`,
    icon: 'microscope',
    kind: 'editor',
    editorLabel: 'Ask the lab',
    editorText: 'Compare the selected model and active LoRA run.',
    endpoint: '/v1/completions',
    endpointPayload: 'ask',
    actions: [{ id: 'ask', label: 'Ask', icon: 'paper-plane', kind: 'run' }],
    metrics: [
      { label: 'Models', value: '4', tone: 'brand' },
      { label: 'Active runs', value: '2' },
      { label: 'Saved queries', value: '3' },
      { label: 'Selected', value: 'lemma-12b' },
    ],
    rows: [
      {
        id: 'models',
        title: 'Models',
        meta: 'Local .model library',
        detail: 'Architecture, quantisation, context and identity fingerprint',
        status: 'ready',
      },
      {
        id: 'lora',
        title: 'LoRA',
        meta: 'Training-run registry',
        detail: 'Start, pause, resume, cancel and fuse training work',
        status: '2 active',
      },
      {
        id: 'influx',
        title: 'Influx',
        meta: 'Time-series queries',
        detail: 'Loss and throughput visualisation',
        status: 'ready',
      },
      {
        id: 'duckdb',
        title: 'DuckDB',
        meta: 'Read-only experiment queries',
        detail: 'Inspect runs, saved queries and R1 records',
        status: 'ready',
      },
    ],
    sections: [
      {
        title: 'Ask the lab',
        body: 'Ask about the selected model, run, query range or training artefact.',
        tone: 'brand',
      },
      {
        title: 'Current selection',
        items: [
          'model · lemma-12b-v4-p6.model',
          'run · run-2026-05-22-12b-v4-p6-rsft',
          'range · last 24 hours',
        ],
      },
    ],
    footer: 'selected: lemma-12b-v4-p6.model · run-2026-05-22-12b-v4-p6-rsft · local workbench',
  };
}

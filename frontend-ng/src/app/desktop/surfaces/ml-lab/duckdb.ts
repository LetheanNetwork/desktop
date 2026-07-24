import { Component } from '@angular/core';
import { SurfaceConfig, SurfacePage, SurfaceRoute } from '../surface-page';

@Component({
  selector: 'lthn-ml-lab-duckdb-surface',
  imports: [SurfacePage],
  template: `<lthn-surface-page [config]="config" />`,
})
export class MlLabDuckdbSurface extends SurfaceRoute {
  readonly config: SurfaceConfig = {
    id: 'ml-lab-duckdb',
    title: $localize`:ML Lab DuckDB title@@surface.mlLab.duckdb.title:DuckDB`,
    subtitle: $localize`:ML Lab DuckDB subtitle@@surface.mlLab.duckdb.subtitle:Read-only local experiment queries`,
    icon: 'database',
    kind: 'editor',
    editorLabel: 'SQL · SELECT / WITH / EXPLAIN / SHOW / DESCRIBE',
    editorText: `SELECT id, base_model, mode, status, last_loss, started_at
FROM lora_runs
ORDER BY started_at DESC
LIMIT 50;`,
    endpoint: '/v1/ml-lab/duckdb',
    actions: [
      { id: 'run', label: 'Run query', icon: 'play', kind: 'run' },
      { id: 'clear', label: 'Clear', icon: 'eraser', kind: 'clear' },
    ],
    rows: [
      {
        id: 'run-2026-05-22-12b-v4-p6-rsft',
        title: 'lemma-12b-v4-p6 · sft',
        meta: '2026-05-22 08:14',
        status: 'active',
        value: 'loss 0.318',
      },
      {
        id: 'run-2026-05-21-3-1b-cl-bpl',
        title: 'lemma-3-1b-cl-bpl · distill',
        meta: '2026-05-21 19:48',
        status: 'completed',
        value: 'loss 0.142',
      },
      {
        id: 'run-2026-05-20-qwen-grpo',
        title: 'qwen-3-32b-think · grpo',
        meta: '2026-05-20 13:02',
        status: 'paused',
        value: 'loss 0.587',
      },
    ],
    sections: [
      {
        title: 'Schema',
        items: [
          'lora_runs · 10 columns',
          'lab_saved_queries · 6 columns',
          'r1_records · 6 columns',
        ],
      },
    ],
    footer: 'read-only SQL enforced by the backend · paginated results · local DuckDB',
  };
}

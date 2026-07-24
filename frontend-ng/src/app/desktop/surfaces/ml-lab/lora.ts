import { Component } from '@angular/core';
import { SurfaceConfig, SurfacePage, SurfaceRoute } from '../surface-page';

@Component({
  selector: 'lthn-ml-lab-lora-surface',
  imports: [SurfacePage],
  template: `<lthn-surface-page [config]="config" />`,
})
export class MlLabLoraSurface extends SurfaceRoute {
  readonly config: SurfaceConfig = {
    id: 'ml-lab-lora',
    title: $localize`:ML Lab LoRA title@@surface.mlLab.lora.title:LoRA`,
    subtitle: $localize`:ML Lab LoRA subtitle@@surface.mlLab.lora.subtitle:Training-run registry — active, paused, completed and failed`,
    icon: 'flask',
    metrics: [
      { label: 'Active', value: '2', tone: 'brand' },
      { label: 'Queued', value: '1' },
      { label: 'Completed', value: '1', tone: 'ok' },
      { label: 'Failed', value: '1', tone: 'danger' },
    ],
    filters: [
      { id: 'active', label: 'Active' },
      { id: 'paused', label: 'Paused' },
      { id: 'completed', label: 'Completed' },
      { id: 'failed', label: 'Failed' },
      { id: 'queued', label: 'Queued' },
    ],
    actions: [
      { id: 'new', label: 'New LoRA run', icon: 'plus', kind: 'add' },
      { id: 'refresh', label: 'Refresh', icon: 'rotate', kind: 'refresh' },
    ],
    rows: [
      {
        id: 'run-2026-05-22-12b-v4-p6-rsft',
        title: 'lemma-12b-v4-p6 · golden_set_v2',
        meta: 'SFT · 1,804 tok/s · ETA 2 h 32 m',
        status: 'active',
        value: 'loss 0.412',
        progress: 64,
      },
      {
        id: 'run-2026-05-23-4b-grpo-zen',
        title: 'lemma-4b-e4b · zen_composure_v1',
        meta: 'GRPO · 2,412 tok/s · ETA 53 m',
        status: 'active',
        value: 'loss 1.218',
        progress: 52,
      },
      {
        id: 'run-2026-05-21-1b-distill',
        title: 'lemer-2b-e2b · lek-1-distill-corpus',
        meta: 'distill · 2,400 / 2,400',
        status: 'completed',
        value: 'loss 0.187',
        progress: 100,
      },
      {
        id: 'run-2026-05-20-26b-moe-paused',
        title: 'lemmy-26b-a4b-moe · lek-axiom-sandwich-v3',
        meta: 'SFT · 4,801 / 18,000',
        status: 'paused',
        value: 'loss 0.658',
        progress: 27,
      },
      {
        id: 'run-2026-05-19-4b-failed-oom',
        title: 'lemma-4b-e4b · expansion_prompts_v2',
        meta: 'SFT · stopped at iteration 213',
        status: 'failed',
        value: 'loss 2.804',
        progress: 3,
      },
      {
        id: 'run-2026-05-24-queued',
        title: 'lemma-12b-v4-p7 · golden_set_v2',
        meta: 'SFT · waiting for capacity',
        status: 'queued',
        value: '—',
        progress: 0,
      },
    ],
    searchPlaceholder: 'Filter by run, model, data set or mode',
    loadEndpoint: '/v1/ml-lab/runs',
    pollMs: 60_000,
    liveKeys: ['runs'],
    footer: 'local .train artefacts · live run events · controls are audited',
  };
}

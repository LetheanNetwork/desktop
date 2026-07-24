import { Component } from '@angular/core';
import { SurfaceConfig, SurfacePage, SurfaceRoute } from '../surface-page';

@Component({
  selector: 'lthn-ml-lab-influx-surface',
  imports: [SurfacePage],
  template: `<lthn-surface-page [config]="config" />`,
})
export class MlLabInfluxSurface extends SurfaceRoute {
  readonly config: SurfaceConfig = {
    id: 'ml-lab-influx',
    title: $localize`:ML Lab Influx title@@surface.mlLab.influx.title:Influx`,
    subtitle: $localize`:ML Lab Influx subtitle@@surface.mlLab.influx.subtitle:Training time-series workbench`,
    icon: 'chart-area',
    kind: 'editor',
    editorLabel: 'Flux · training loss',
    editorText: `from(bucket: "train")
  |> range(start: \${start}, stop: \${stop})
  |> filter(fn: (r) => r._measurement == "loss")
  |> aggregateWindow(every: 1m, fn: mean)`,
    endpoint: '/v1/ml-lab/influx',
    chart: [2.41, 1.92, 1.52, 1.18, 0.91, 0.71, 0.57, 0.46],
    metrics: [
      { label: 'Latest loss', value: '0.46', tone: 'brand' },
      { label: 'Throughput', value: '62.1 tok/s' },
      { label: 'Window', value: '35 min' },
    ],
    actions: [
      { id: 'run', label: 'Run query', icon: 'play', kind: 'run' },
      { id: 'clear', label: 'Reset', icon: 'rotate-left', kind: 'clear' },
    ],
    footer: 'Flux / InfluxQL · line chart for time-series shapes · table fallback',
  };
}

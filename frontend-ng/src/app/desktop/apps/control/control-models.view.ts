// SPDX-License-Identifier: EUPL-1.2

import {
  ChangeDetectionStrategy,
  Component,
  CUSTOM_ELEMENTS_SCHEMA,
  computed,
  input,
  output,
  signal,
} from '@angular/core';
import { DesktopDataStateBadge } from '../../desktop-data-state-badge';
import type { DesktopDataState } from '../../desktop-data-state';
import type { ModelRuntimeOperation } from '../../desktop-model-runtime.models';
import type { ControlModelIntent, ControlModelsViewModel } from './control-view.models';

@Component({
  selector: 'lthn-control-models-view',
  standalone: true,
  imports: [DesktopDataStateBadge],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  host: { style: 'display: contents' },
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <div class="ctoolbar">
      <h1 i18n="Local model view heading@@control.models.heading">Local models</h1>
      <lthn-desktop-data-state [state]="dataState()" />
      <span class="miniseg">
        <span class="on" i18n="Running model filter@@control.models.filter.running">Running</span>
        <span i18n="All models filter@@control.models.filter.all">All</span>
      </span>
      <lthn-badge variant="muted">{{ model().state }}</lthn-badge>
      @if (canLoad()) {
        <select
          aria-label="Model to load"
          [value]="selectedModelId()"
          [disabled]="pending() !== null"
          (change)="selectModel($event)"
        >
          <option value="" i18n="Model selector placeholder@@control.models.choose">
            Choose a model
          </option>
          @for (candidate of loadableModels(); track candidate.id) {
            <option [value]="candidate.id">{{ candidate.displayName }}</option>
          }
        </select>
      }
      @if (canStart()) {
        <button
          class="nbtn"
          data-action="start"
          [disabled]="pending() !== null"
          (click)="modelAction.emit({ kind: 'start' })"
        >
          <lthn-icon name="play" size="10"></lthn-icon>
          <span i18n="Start model runtime action@@control.models.start">Start</span>
        </button>
      }
      @if (canLoad()) {
        <button
          class="nbtn"
          data-action="load"
          [disabled]="pending() !== null || selectedModelId() === ''"
          (click)="loadSelectedModel()"
        >
          <lthn-icon name="plus" size="10"></lthn-icon>
          <span i18n="Load model action@@control.models.load">Load</span>
        </button>
      }
      @if (canUnload()) {
        <button
          class="nbtn"
          data-action="unload"
          [disabled]="pending() !== null"
          (click)="modelAction.emit({ kind: 'unload' })"
        >
          <span i18n="Unload model action@@control.models.unload">Unload</span>
        </button>
      }
      @if (canRestart()) {
        <button
          class="nbtn"
          data-action="restart"
          [disabled]="pending() !== null"
          (click)="modelAction.emit({ kind: 'restart' })"
        >
          <span i18n="Restart model runtime action@@control.models.restart">Restart</span>
        </button>
      }
      @if (canStop()) {
        <button
          class="nbtn"
          data-action="stop"
          [disabled]="pending() !== null"
          (click)="modelAction.emit({ kind: 'stop' })"
        >
          <span i18n="Stop model runtime action@@control.models.stop">Stop</span>
        </button>
      }
    </div>
    <div class="tiles">
      @for (metric of model().metrics; track metric.label) {
        <lthn-card pad="11">
          <lthn-stat [attr.value]="metric.value" [attr.label]="metric.label" mono></lthn-stat>
        </lthn-card>
      }
    </div>
    <div class="panel">
      <div class="ph">
        <b>{{ model().chart.title }}</b>
        <span>{{ model().chart.caption }}</span>
      </div>
      <lthn-chart type="area" [attr.data]="samplesJson()" height="90"></lthn-chart>
    </div>
    <lthn-datatable
      selectable
      [attr.columns]="columnsJson()"
      [attr.rows]="rowsJson()"
    ></lthn-datatable>
  `,
})
export class ControlModelsView {
  readonly dataState = input.required<DesktopDataState>();
  readonly model = input.required<ControlModelsViewModel>();
  readonly pending = input.required<ModelRuntimeOperation | null>();
  readonly modelAction = output<ControlModelIntent>();
  readonly selectedModelId = signal('');
  readonly loadableModels = computed(() =>
    this.model().availableModels.filter((model) => model.loadable),
  );
  readonly canStart = computed(() => this.model().state === 'stopped');
  readonly canLoad = computed(() => ['model-less', 'ready'].includes(this.model().state));
  readonly canUnload = computed(() => this.model().state === 'ready');
  readonly canRestart = computed(() =>
    ['ready', 'degraded', 'failed'].includes(this.model().state),
  );
  readonly canStop = computed(() =>
    ['starting', 'model-less', 'loading', 'ready', 'degraded', 'failed'].includes(
      this.model().state,
    ),
  );
  readonly columnsJson = computed(() => JSON.stringify(this.model().columns));
  readonly rowsJson = computed(() => JSON.stringify(this.model().rows));
  readonly samplesJson = computed(() => JSON.stringify(this.model().chart.samples));

  selectModel(event: Event): void {
    const target = event.target;
    if (!(target instanceof HTMLSelectElement)) return;
    this.selectedModelId.set(
      this.loadableModels().some((model) => model.id === target.value) ? target.value : '',
    );
  }

  loadSelectedModel(): void {
    const modelId = this.selectedModelId();
    if (this.pending() !== null || !this.loadableModels().some((model) => model.id === modelId)) {
      return;
    }
    this.modelAction.emit({ kind: 'load', modelId });
  }
}

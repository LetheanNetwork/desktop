// SPDX-License-Identifier: EUPL-1.2

import {
  ChangeDetectionStrategy,
  Component,
  CUSTOM_ELEMENTS_SCHEMA,
  computed,
  input,
  output,
} from '@angular/core';
import { DesktopDataStateBadge } from '../../desktop-data-state-badge';
import type { DesktopDataState } from '../../desktop-data-state';
import type { ControlModelsViewModel } from './control-view.models';

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
      <button class="nbtn" (click)="loadModel.emit()">
        <lthn-icon name="plus" size="10"></lthn-icon>
        <span i18n="Load model action@@control.models.load">Load model</span>
      </button>
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
  readonly loadModel = output<void>();
  readonly columnsJson = computed(() => JSON.stringify(this.model().columns));
  readonly rowsJson = computed(() => JSON.stringify(this.model().rows));
  readonly samplesJson = computed(() => JSON.stringify(this.model().chart.samples));
}

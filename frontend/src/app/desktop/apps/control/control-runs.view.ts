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
import type { ControlRunsViewModel } from './control-view.models';

@Component({
  selector: 'lthn-control-runs-view',
  standalone: true,
  imports: [DesktopDataStateBadge],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  host: { style: 'display: contents' },
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <div class="ctoolbar">
      <h1 i18n="Benchmark run view heading@@control.runs.heading">Benchmark runs</h1>
      <lthn-desktop-data-state [state]="dataState()" />
      <button class="nbtn" (click)="newRun.emit()">
        <lthn-icon name="play" size="10"></lthn-icon>
        <span i18n="New benchmark run action@@control.runs.new">New run</span>
      </button>
    </div>
    <div class="panel">
      <div class="ph">
        <b>{{ model().chart.title }}</b>
        <span>{{ model().chart.caption }}</span>
      </div>
      <lthn-chart type="bar" [attr.data]="samplesJson()" height="86"></lthn-chart>
    </div>
    <lthn-datatable [attr.columns]="columnsJson()" [attr.rows]="rowsJson()"></lthn-datatable>
  `,
})
export class ControlRunsView {
  readonly dataState = input.required<DesktopDataState>();
  readonly model = input.required<ControlRunsViewModel>();
  readonly newRun = output<void>();
  readonly columnsJson = computed(() => JSON.stringify(this.model().columns));
  readonly rowsJson = computed(() => JSON.stringify(this.model().rows));
  readonly samplesJson = computed(() => JSON.stringify(this.model().chart.samples));
}

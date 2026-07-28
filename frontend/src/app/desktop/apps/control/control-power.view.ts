// SPDX-License-Identifier: EUPL-1.2

import {
  ChangeDetectionStrategy,
  Component,
  CUSTOM_ELEMENTS_SCHEMA,
  computed,
  input,
} from '@angular/core';
import { DesktopDataStateBadge } from '../../desktop-data-state-badge';
import type { DesktopDataState } from '../../desktop-data-state';
import type { ControlPowerViewModel } from './control-view.models';

@Component({
  selector: 'lthn-control-power-view',
  standalone: true,
  imports: [DesktopDataStateBadge],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  host: { style: 'display: contents' },
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <div class="ctoolbar">
      <h1 i18n="Power view heading@@control.power.heading">Power</h1>
      <lthn-desktop-data-state [state]="dataState()" />
    </div>
    <div class="tiles c3">
      @for (metric of model().metrics; track metric.label) {
        <lthn-card pad="11">
          <lthn-stat [attr.value]="metric.value" [attr.label]="metric.label" mono></lthn-stat>
        </lthn-card>
      }
    </div>
    <div class="panel">
      <div class="ph">
        <b i18n="Power chart title@@control.power.drawChart">Draw · last hour</b>
        <span i18n="Power chart unit@@control.power.watts">watts</span>
      </div>
      <lthn-chart type="area" [attr.data]="samplesJson()" height="110"></lthn-chart>
    </div>
    <p
      style="font-size:12px;color:var(--fg-3);margin:0"
      i18n="Power usage comparison@@control.power.comparison"
    >
      ≈ a small fridge. Idle overnight drops to 38 W.
    </p>
  `,
})
export class ControlPowerView {
  readonly dataState = input.required<DesktopDataState>();
  readonly model = input.required<ControlPowerViewModel>();
  readonly samplesJson = computed(() => JSON.stringify(this.model().samples));
}

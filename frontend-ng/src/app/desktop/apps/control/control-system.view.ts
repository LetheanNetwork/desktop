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
import type { ControlSystemTab, ControlSystemViewModel } from './control-view.models';

@Component({
  selector: 'lthn-control-system-view',
  standalone: true,
  imports: [DesktopDataStateBadge],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  host: { style: 'display: contents' },
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <div class="ctoolbar">
      <h1 i18n="System view heading@@control.system.heading">System</h1>
      <lthn-desktop-data-state [state]="dataState()" />
      <span class="systabs">
        @for (tab of tabs; track tab[0]) {
          <button
            class="systab"
            [class.on]="activeTab() === tab[0]"
            (click)="tabChange.emit(tab[0])"
          >
            {{ tab[1] }}
          </button>
        }
      </span>
    </div>
    @switch (activeTab()) {
      @case ('processes') {
        <lthn-datatable
          selectable
          [attr.columns]="processColumnsJson()"
          [attr.rows]="processRowsJson()"
        ></lthn-datatable>
        <p
          style="font-size:11.5px;color:var(--fg-3);margin:10px 0 0"
          i18n="Process table help@@control.system.processHelp"
        >
          Live from the shared process registry. CPU, memory, and process actions still need backend
          sources.
        </p>
      }
      @case ('daemons') {
        <lthn-datatable
          [attr.columns]="daemonColumnsJson()"
          [attr.rows]="daemonRowsJson()"
        ></lthn-datatable>
        <p
          style="font-size:11.5px;color:var(--fg-3);margin:10px 0 0"
          i18n="Daemon table help@@control.system.daemonHelp"
        >
          JSON daemon registry — PID files + health endpoints.
        </p>
      }
      @default {
        <div class="tiles">
          @for (metric of model().metrics; track metric.label) {
            <lthn-card pad="11">
              <lthn-stat [attr.value]="metric.value" [attr.label]="metric.label" mono></lthn-stat>
            </lthn-card>
          }
        </div>
        <div class="panel">
          <div class="ph">
            <b i18n="Demo CPU chart title@@control.system.cpuDemoChart">CPU · demo history</b>
            <span i18n="Demo CPU value@@control.system.cpuDemoNow">34% demo</span>
          </div>
          <lthn-chart type="area" [attr.data]="cpuSamplesJson()" height="84"></lthn-chart>
        </div>
        <div class="panel">
          <div class="ph">
            <b i18n="Top process table title@@control.system.topProcesses">Top processes</b>
            <span i18n="Top process sort description@@control.system.byCpu">by CPU</span>
          </div>
          <lthn-datatable
            [attr.columns]="processColumnsJson()"
            [attr.rows]="processRowsJson()"
          ></lthn-datatable>
        </div>
      }
    }
  `,
})
export class ControlSystemView {
  readonly dataState = input.required<DesktopDataState>();
  readonly model = input.required<ControlSystemViewModel>();
  readonly activeTab = input.required<ControlSystemTab>();
  readonly tabChange = output<ControlSystemTab>();
  readonly tabs: readonly [ControlSystemTab, string][] = [
    ['overview', $localize`:System tab@@control.system.tab.overview:Overview`],
    ['processes', $localize`:System tab@@control.system.tab.processes:Processes`],
    ['daemons', $localize`:System tab@@control.system.tab.daemons:Daemons`],
  ];
  readonly processColumnsJson = computed(() => JSON.stringify(this.model().processColumns));
  readonly processRowsJson = computed(() => JSON.stringify(this.model().processRows));
  readonly daemonColumnsJson = computed(() => JSON.stringify(this.model().daemonColumns));
  readonly daemonRowsJson = computed(() => JSON.stringify(this.model().daemonRows));
  readonly cpuSamplesJson = computed(() => JSON.stringify(this.model().cpuSamples));
}

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
import type { DesktopDataResource } from '../../desktop-data-resource';
import { DesktopDataStatusView } from '../../desktop-data-status.view';
import type { DesktopDataState } from '../../desktop-data-state';
import type { SystemMonitorSnapshot } from '../../desktop-system-monitor.models';
import type {
  ControlServiceIntent,
  DesktopServiceCatalogue,
  DesktopServiceOutput,
} from './control-services.models';
import { ControlServicesView } from './control-services.view';
import type { ControlSystemTab, ControlSystemViewModel } from './control-view.models';

@Component({
  selector: 'lthn-control-system-view',
  standalone: true,
  imports: [DesktopDataStateBadge, DesktopDataStatusView, ControlServicesView],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  host: { style: 'display: contents' },
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <div class="ctoolbar">
      <h1 i18n="System view heading@@control.system.heading">System</h1>
      @if (activeTab() !== 'daemons') {
        <lthn-desktop-data-state [state]="dataState()" />
      }
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
        <lthn-control-services-view
          [resource]="services()"
          [pendingIds]="pendingServiceIds()"
          [outputView]="serviceOutput()"
          (action)="serviceAction.emit($event)"
          (retry)="servicesRetry.emit()"
        />
      }
      @default {
        <lthn-desktop-data-status [status]="systemResource()" (retry)="systemRetry.emit()" />
        <div class="tiles">
          @for (metric of model().metrics; track metric.label) {
            <lthn-card pad="11">
              <lthn-stat [attr.value]="metric.value" [attr.label]="metric.label" mono></lthn-stat>
            </lthn-card>
          }
        </div>
        <div class="panel">
          <div class="ph">
            <b>{{ model().cpuChartTitle }}</b>
            <span>{{ model().cpuChartCaption }}</span>
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
  readonly systemResource = input.required<DesktopDataResource<SystemMonitorSnapshot>>();
  readonly services = input.required<DesktopDataResource<DesktopServiceCatalogue>>();
  readonly pendingServiceIds = input<readonly string[]>([]);
  readonly serviceOutput = input<DesktopServiceOutput | null>(null);
  readonly tabChange = output<ControlSystemTab>();
  readonly serviceAction = output<ControlServiceIntent>();
  readonly servicesRetry = output<void>();
  readonly systemRetry = output<void>();
  readonly tabs: readonly [ControlSystemTab, string][] = [
    ['overview', $localize`:System tab@@control.system.tab.overview:Overview`],
    ['processes', $localize`:System tab@@control.system.tab.processes:Processes`],
    ['daemons', $localize`:System tab@@control.system.tab.services:Services`],
  ];
  readonly processColumnsJson = computed(() => JSON.stringify(this.model().processColumns));
  readonly processRowsJson = computed(() => JSON.stringify(this.model().processRows));
  readonly cpuSamplesJson = computed(() => JSON.stringify(this.model().cpuSamples));
}

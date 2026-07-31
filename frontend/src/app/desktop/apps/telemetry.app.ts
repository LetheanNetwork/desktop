// SPDX-License-Identifier: EUPL-1.2

import {
  ChangeDetectionStrategy,
  Component,
  CUSTOM_ELEMENTS_SCHEMA,
  Input,
  OnDestroy,
  OnInit,
  computed,
  inject,
} from '@angular/core';
import { DesktopDataStatusView } from '../desktop-data-status.view';
import { DesktopModelRuntimeResource } from '../desktop-model-runtime-resource.service';
import { DesktopSystemMonitorResource } from '../desktop-system-monitor-resource.service';
import type { Win } from '../desktop.data';
import { AppView } from './app-view';
import { createTelemetryView } from './telemetry/telemetry-view-state';

@Component({
  selector: 'lthn-telemetry-app',
  standalone: true,
  imports: [DesktopDataStatusView],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  host: { style: 'display: contents' },
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <div class="telem">
      <div class="bigrow">
        <div class="big">
          <div
            class="glow"
            style="background:radial-gradient(60% 80% at 30% 20%,color-mix(in oklch,var(--brand-500) 18%,transparent),transparent)"
          ></div>
          <span class="lab">{{ primaryLabel() }}</span>
          <div class="num">
            {{ primaryValue() }}<small> {{ primaryUnit() }}</small>
          </div>
          <lthn-sparkline
            [attr.data]="primaryJson()"
            color="var(--brand-300)"
            width="260"
            height="46"
            fill
          ></lthn-sparkline>
        </div>
        <div class="big">
          <div
            class="glow"
            style="background:radial-gradient(60% 80% at 70% 20%,color-mix(in oklch,#febc2e 14%,transparent),transparent)"
          ></div>
          <span class="lab">{{ secondaryLabel() }}</span>
          <div class="num">
            {{ secondaryValue() }}<small> {{ secondaryUnit() }}</small>
          </div>
          <lthn-sparkline
            [attr.data]="secondaryJson()"
            color="#febc2e"
            width="260"
            height="46"
            fill
          ></lthn-sparkline>
        </div>
      </div>
      <div class="metaband">
        <lthn-desktop-data-status [status]="resource()" (retry)="refresh()" />
        @for (item of metadata(); track item.label) {
          <span
            >{{ item.label }} <b>{{ item.value }}</b></span
          >
        }
      </div>
    </div>
  `,
})
export class TelemetryApp implements AppView, OnInit, OnDestroy {
  private readonly systemMonitor = inject(DesktopSystemMonitorResource);
  private readonly modelRuntime = inject(DesktopModelRuntimeResource);
  private systemDisconnect: (() => void) | null = null;
  private modelDisconnect: (() => void) | null = null;

  @Input() win!: Win;

  readonly resource = this.systemMonitor.resource;
  readonly modelRuntimeResource = this.modelRuntime.resource;
  readonly view = computed(() => {
    const resource = this.resource();
    if (resource.value === null) return null;
    return createTelemetryView(
      resource.value,
      this.modelRuntimeResource().value,
      resource.mode === 'demo' ? 'demo' : 'live',
    );
  });
  readonly primaryLabel = computed(
    () =>
      this.view()?.primary.label ??
      $localize`:Host CPU telemetry metric@@telemetry.cpuUtilisation:CPU utilisation`,
  );
  readonly primaryValue = computed(() => this.view()?.primary.value ?? '—');
  readonly primaryUnit = computed(() => this.view()?.primary.unit ?? '%');
  readonly secondaryLabel = computed(
    () =>
      this.view()?.secondary.label ??
      $localize`:Host memory telemetry metric@@telemetry.memoryUsed:Memory used`,
  );
  readonly secondaryValue = computed(() => this.view()?.secondary.value ?? '—');
  readonly secondaryUnit = computed(() => this.view()?.secondary.unit ?? '%');
  readonly metadata = computed(() => this.view()?.metadata ?? []);
  readonly primaryJson = computed(() => JSON.stringify(this.view()?.primary.history ?? []));
  readonly secondaryJson = computed(() => JSON.stringify(this.view()?.secondary.history ?? []));

  ngOnInit(): void {
    this.systemDisconnect = this.systemMonitor.connect();
    this.modelDisconnect = this.modelRuntime.connect();
  }

  ngOnDestroy(): void {
    this.systemDisconnect?.();
    this.systemDisconnect = null;
    this.modelDisconnect?.();
    this.modelDisconnect = null;
  }

  async refresh(): Promise<void> {
    await this.systemMonitor.refresh();
  }
}

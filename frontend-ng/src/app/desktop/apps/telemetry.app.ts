// ─────────────────────────────────────────────────────────────────────────
// apps/telemetry.app.ts — example dumb app-view (the pattern to follow).
//
// Presentational only: takes `win` + a data slice, renders with <lthn-*>. No
// window-manager, no OS chrome, no siblings. Mirrors the Telemetry markup from
// preview.html / the desktop.component ngSwitch — this is the target shape for
// every app once its block is peeled out of the monolith.
// ─────────────────────────────────────────────────────────────────────────
import { Component, CUSTOM_ELEMENTS_SCHEMA, Input, ChangeDetectionStrategy } from '@angular/core';
import { CommonModule } from '@angular/common';
import { AppView } from './app-view';
import { Win, TELEMETRY } from '../desktop.data';

@Component({
  selector: 'lthn-telemetry-app',
  standalone: true,
  imports: [CommonModule],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  host: { style: 'display: contents' },
  changeDetection: ChangeDetectionStrategy.Eager,
  template: `
    <div class="telem">
      <div class="bigrow">
        <div class="big">
          <div
            class="glow"
            style="background:radial-gradient(60% 80% at 30% 20%,color-mix(in oklch,var(--brand-500) 18%,transparent),transparent)"
          ></div>
          <span class="lab" i18n="Telemetry metric@@telemetry.throughput">Throughput</span>
          <div class="num">
            41.8<small i18n="Tokens per second unit@@unit.tokensPerSecondInline"> tok/s</small>
          </div>
          <lthn-sparkline
            [attr.data]="throughputJson"
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
          <span class="lab" i18n="Telemetry metric@@telemetry.powerDraw">Power draw</span>
          <div class="num">207<small i18n="Watts unit@@unit.watts"> W</small></div>
          <lthn-sparkline
            [attr.data]="wattsJson"
            color="#febc2e"
            width="260"
            height="46"
            fill
          ></lthn-sparkline>
        </div>
      </div>
      <div class="metaband">
        <span i18n="Telemetry model label and value@@telemetry.model"
          >Model <b>llama-3.1-70b</b></span
        ><span i18n="Telemetry region label and value@@telemetry.region"
          >Region <b>eu-west-2</b></span
        ><span i18n="Telemetry cache label and value@@telemetry.kvCache">KV-cache <b>62%</b></span
        ><span i18n="Telemetry uptime label and value@@telemetry.uptime">Uptime <b>6d 4h</b></span>
      </div>
    </div>
  `,
})
export class TelemetryApp implements AppView {
  @Input() win!: Win;
  /** Data slice — supply from @Input in production (route resolver / socket). */
  @Input() throughput = TELEMETRY.throughput;
  @Input() watts = TELEMETRY.watts;
  get throughputJson() {
    return JSON.stringify(this.throughput);
  }
  get wattsJson() {
    return JSON.stringify(this.watts);
  }
}

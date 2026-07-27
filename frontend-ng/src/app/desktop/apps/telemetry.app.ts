// ─────────────────────────────────────────────────────────────────────────
// apps/telemetry.app.ts — bounded process-telemetry view.
//
// The surface keeps deterministic offline demo data, retains stale connected
// values, and reconciles guarded live refreshes through one resource.
// ─────────────────────────────────────────────────────────────────────────
import {
  ChangeDetectionStrategy,
  Component,
  CUSTOM_ELEMENTS_SCHEMA,
  Input,
  OnDestroy,
  OnInit,
  computed,
  inject,
  signal,
} from '@angular/core';
import { AppView } from './app-view';
import { Win, TELEMETRY } from '../desktop.data';
import { DesktopLiveDataService } from '../desktop-live-data.service';
import { DesktopModelRuntimeResource } from '../desktop-model-runtime-resource.service';
import {
  beginDesktopDataRefresh,
  createConnectedResource,
  createDemoResource,
  rejectDesktopData,
  resolveDesktopData,
  type DesktopDataResource,
} from '../desktop-data-resource';
import { DesktopDataStatusView } from '../desktop-data-status.view';
import type { TelemetryDemoSeries, TelemetryViewData } from './telemetry/telemetry-view.models';
import {
  createDemoTelemetryView,
  createLiveTelemetryView,
  overlayRuntimeTelemetryView,
} from './telemetry/telemetry-view-state';

const TELEMETRY_POLL_MS = 5_000;
const TELEMETRY_STALE_AFTER_MS = TELEMETRY_POLL_MS * 2;
const TELEMETRY_DEMO_SOURCE = $localize`:Telemetry demo source@@telemetry.source.demo:Lethean demo fixture`;
const TELEMETRY_LIVE_SOURCE = $localize`:Telemetry live source@@telemetry.source.live:Local process runtime`;
const TELEMETRY_UNAVAILABLE = $localize`:Telemetry unavailable error@@telemetry.data.unavailable:Live telemetry is unavailable.`;

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
          <span class="lab">{{ powerLabel() }}</span>
          <div class="num">
            {{ powerValue() }}<small> {{ powerUnit() }}</small>
          </div>
          <lthn-sparkline
            [attr.data]="powerJson()"
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
  private readonly liveData = inject(DesktopLiveDataService);
  private readonly modelRuntime = inject(DesktopModelRuntimeResource);
  private refreshInFlight = false;
  private pollHandle: number | undefined;
  private modelRuntimeDisconnect: (() => void) | null = null;
  private destroyed = false;

  @Input() win!: Win;
  @Input() throughput: readonly number[] = TELEMETRY.throughput;
  @Input() watts: readonly number[] = TELEMETRY.watts;

  readonly resource = signal<DesktopDataResource<TelemetryViewData>>(
    createDemoResource(
      createDemoTelemetryView({
        throughput: this.throughput,
        watts: this.watts,
      }),
      TELEMETRY_DEMO_SOURCE,
    ),
  );
  readonly modelRuntimeResource = this.modelRuntime.resource;
  readonly view = computed(() => {
    const resource = this.resource();
    if (resource.value === null || resource.mode === 'demo') return resource.value;
    return overlayRuntimeTelemetryView(resource.value, this.modelRuntimeResource().value);
  });
  readonly primaryLabel = computed(
    () =>
      this.view()?.primary.label ??
      $localize`:Process heap telemetry metric@@telemetry.heapAllocation:Heap allocation`,
  );
  readonly primaryValue = computed(() => this.view()?.primary.value ?? '—');
  readonly primaryUnit = computed(() => this.view()?.primary.unit ?? '');
  readonly powerLabel = computed(
    () => this.view()?.power.label ?? $localize`:Telemetry metric@@telemetry.powerDraw:Power draw`,
  );
  readonly powerValue = computed(() => this.view()?.power.value ?? '—');
  readonly powerUnit = computed(() => this.view()?.power.unit ?? '');
  readonly metadata = computed(() => this.view()?.metadata ?? []);
  readonly primaryJson = computed(() => JSON.stringify(this.view()?.primary.history ?? []));
  readonly powerJson = computed(() => JSON.stringify(this.view()?.power.history ?? []));

  ngOnInit(): void {
    this.modelRuntimeDisconnect = this.modelRuntime.connect();
    const demo = createDemoTelemetryView(this.demoSeries());
    if (this.liveData.mode() === 'demo') {
      this.resource.set(createDemoResource(demo, TELEMETRY_DEMO_SOURCE));
      return;
    }

    this.resource.set(createConnectedResource<TelemetryViewData>(TELEMETRY_LIVE_SOURCE));
    void this.refresh();
    this.pollHandle = window.setInterval(() => void this.refresh(), TELEMETRY_POLL_MS);
  }

  ngOnDestroy(): void {
    this.destroyed = true;
    if (this.pollHandle !== undefined) {
      window.clearInterval(this.pollHandle);
      this.pollHandle = undefined;
    }
    this.modelRuntimeDisconnect?.();
    this.modelRuntimeDisconnect = null;
  }

  async refresh(): Promise<void> {
    if (this.liveData.mode() === 'demo' || this.refreshInFlight || this.destroyed) return;
    this.refreshInFlight = true;
    this.resource.update((resource) =>
      beginDesktopDataRefresh(resource, Date.now(), TELEMETRY_STALE_AFTER_MS),
    );

    try {
      const sample = await this.liveData.telemetry();
      if (this.destroyed) return;
      const mapped = createLiveTelemetryView(
        sample,
        this.modelRuntimeResource().value,
        this.resource().value,
      );
      this.resource.update((resource) =>
        resolveDesktopData(resource, mapped.value, mapped.state, TELEMETRY_LIVE_SOURCE, Date.now()),
      );
    } catch {
      if (this.destroyed) return;
      this.resource.update((resource) => rejectDesktopData(resource, TELEMETRY_UNAVAILABLE));
    } finally {
      this.refreshInFlight = false;
    }
  }

  private demoSeries(): TelemetryDemoSeries {
    return {
      throughput: this.throughput,
      watts: this.watts,
    };
  }
}

// ─────────────────────────────────────────────────────────────────────────
// apps/telemetry.app.ts — bounded process-telemetry view.
//
// The surface owns its short-lived polling window and keeps the original
// labelled demo composition for offline previews and unavailable live data.
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
import { CommonModule } from '@angular/common';
import { AppView } from './app-view';
import { Win, TELEMETRY } from '../desktop.data';
import { DesktopLiveDataService, ProcessTelemetry } from '../desktop-live-data.service';
import {
  DesktopDataState,
  desktopDataStateLabel,
  desktopDataStateVariant,
} from '../desktop-data-state';

const TELEMETRY_POLL_MS = 5_000;
const MAX_HISTORY_SAMPLES = 60;

@Component({
  selector: 'lthn-telemetry-app',
  standalone: true,
  imports: [CommonModule],
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
            [attr.data]="primaryJson"
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
          <div class="num">{{ powerValue() }}<small i18n="Watts unit@@unit.watts"> W</small></div>
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
        <lthn-badge [attr.variant]="dataStateVariant()" [attr.data-data-state]="dataState()">{{
          dataStateLabel()
        }}</lthn-badge>
        <ng-container *ngIf="sample() as current; else demoMetadata">
          <span
            >Goroutines <b>{{ current.numGoroutines }}</b></span
          ><span
            >GC pause <b>{{ current.lastGCPauseMs }} ms</b></span
          ><span
            >CGO calls <b>{{ current.numCgoCalls }}</b></span
          ><span
            >Uptime <b>{{ uptimeLabel() }}</b></span
          >
        </ng-container>
        <ng-template #demoMetadata>
          <span i18n="Telemetry model label and value@@telemetry.model"
            >Model <b>llama-3.1-70b</b></span
          ><span i18n="Telemetry region label and value@@telemetry.region"
            >Region <b>eu-west-2</b></span
          ><span i18n="Telemetry cache label and value@@telemetry.kvCache">KV-cache <b>62%</b></span
          ><span i18n="Telemetry uptime label and value@@telemetry.uptime"
            >Uptime <b>6d 4h</b></span
          >
        </ng-template>
      </div>
    </div>
  `,
})
export class TelemetryApp implements AppView, OnInit, OnDestroy {
  private readonly liveData = inject(DesktopLiveDataService);
  private pollHandle: number | undefined;

  @Input() win!: Win;
  /** Design-fixture history used only when live samples are unavailable. */
  @Input() throughput = TELEMETRY.throughput;
  @Input() watts = TELEMETRY.watts;
  readonly sample = signal<ProcessTelemetry | null>(null);
  readonly dataState = signal<DesktopDataState>('demo');
  private readonly heapHistory = signal<readonly number[]>([]);
  private readonly powerHistory = signal<readonly number[]>([]);
  readonly dataStateLabel = computed(() => desktopDataStateLabel(this.dataState()));
  readonly dataStateVariant = computed(() => desktopDataStateVariant(this.dataState()));
  readonly primaryLabel = computed(() =>
    this.sample()
      ? $localize`:Process heap telemetry metric@@telemetry.heapAllocation:Heap allocation`
      : $localize`:Telemetry metric@@telemetry.throughput:Throughput`,
  );
  readonly primaryValue = computed(() => this.sample()?.heapAllocMB.toFixed(1) ?? '41.8');
  readonly primaryUnit = computed(() =>
    this.sample()
      ? $localize`:Megabytes unit@@unit.megabytes:MB`
      : $localize`:Tokens per second unit@@unit.tokensPerSecondInline:tok/s`,
  );
  readonly powerValue = computed(() => {
    const watts = this.sample()?.wattsActive ?? 0;
    return watts > 0 ? watts.toFixed(0) : '207';
  });
  readonly powerLabel = computed(() =>
    this.sample() && this.sample()!.wattsActive <= 0
      ? $localize`:Demo power telemetry metric@@telemetry.demoPowerDraw:Power draw · demo`
      : $localize`:Telemetry metric@@telemetry.powerDraw:Power draw`,
  );
  readonly uptimeLabel = computed(() => formatUptime(this.sample()?.uptimeSeconds ?? 0));

  ngOnInit(): void {
    if (this.liveData.mode() === 'demo') {
      this.dataState.set('demo');
      return;
    }
    this.dataState.set('loading');
    void this.refresh();
    this.pollHandle = window.setInterval(() => void this.refresh(), TELEMETRY_POLL_MS);
  }

  ngOnDestroy(): void {
    if (this.pollHandle !== undefined) {
      window.clearInterval(this.pollHandle);
      this.pollHandle = undefined;
    }
  }

  async refresh(): Promise<void> {
    if (this.liveData.mode() === 'demo') return;
    try {
      const sample = await this.liveData.telemetry();
      const firstSample = this.sample() === null;
      this.sample.set(sample);
      this.heapHistory.update((history) =>
        appendHistory(firstSample ? [] : history, sample.heapAllocMB),
      );
      if (sample.wattsActive > 0) {
        this.powerHistory.update((history) =>
          appendHistory(firstSample ? [] : history, sample.wattsActive),
        );
      }
      this.dataState.set(sample.wattsActive > 0 ? 'live' : 'mixed');
    } catch {
      this.sample.set(null);
      this.heapHistory.set([]);
      this.powerHistory.set([]);
      this.dataState.set('unavailable');
    }
  }

  get primaryJson() {
    return JSON.stringify(this.sample() ? this.heapHistory() : this.throughput);
  }

  get wattsJson() {
    return JSON.stringify(this.powerHistory().length ? this.powerHistory() : this.watts);
  }
}

function appendHistory(history: readonly number[], value: number): readonly number[] {
  return [...history, value].slice(-MAX_HISTORY_SAMPLES);
}

function formatUptime(seconds: number): string {
  const wholeSeconds = Math.max(0, Math.floor(seconds));
  const days = Math.floor(wholeSeconds / 86_400);
  const hours = Math.floor((wholeSeconds % 86_400) / 3_600);
  const minutes = Math.floor((wholeSeconds % 3_600) / 60);
  if (days > 0) return `${days}d ${hours}h`;
  if (hours > 0) return `${hours}h ${minutes}m`;
  return `${minutes}m`;
}

// apps/control.app.ts — flagship Control surface. The existing navigation and
// presentation stay local while DesktopLiveDataService progressively replaces
// demo fixtures with typed backend data.
import {
  Component,
  CUSTOM_ELEMENTS_SCHEMA,
  Directive,
  ElementRef,
  Input,
  OnInit,
  Renderer2,
  inject,
  ChangeDetectionStrategy,
  computed,
  declareExperimentalWebMcpTool,
  signal,
} from '@angular/core';
import { CommonModule } from '@angular/common';
import { AppView } from './app-view';
import { CTRL_NAV, Win, TELEMETRY } from '../desktop.data';
import { AppNavItem } from '../desktop-route-tree';
import { WindowManagerService } from '../window-manager.service';
import { ControlLiveSnapshot, DesktopLiveDataService } from '../desktop-live-data.service';
import {
  DesktopDataState,
  desktopDataStateLabel,
  desktopDataStateVariant,
} from '../desktop-data-state';

@Directive({
  selector: 'lthn-toggle',
  standalone: true,
})
export class LthnToggleOnDirective {
  constructor(
    private el: ElementRef<HTMLElement>,
    private renderer: Renderer2,
  ) {}

  @Input() set on(value: boolean) {
    if (value) this.renderer.setAttribute(this.el.nativeElement, 'on', '');
    else this.renderer.removeAttribute(this.el.nativeElement, 'on');
  }
}

function formatBytes(bytes: number): string {
  if (bytes <= 0) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  const unitIndex = Math.min(units.length - 1, Math.floor(Math.log(bytes) / Math.log(1_024)));
  const value = bytes / 1_024 ** unitIndex;
  const precision = value >= 10 || Number.isInteger(value) ? 0 : 1;
  return `${value.toFixed(precision)} ${units[unitIndex]}`;
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

function formatMegabytes(value: number): string {
  const precision = value >= 10 || Number.isInteger(value) ? 0 : 1;
  return `${value.toFixed(precision)} MB`;
}

function benchmarkTime(timestamp: string): string {
  return timestamp.match(/T(\d{2}:\d{2})/u)?.[1] ?? timestamp;
}

@Component({
  selector: 'lthn-control-app',
  standalone: true,
  imports: [CommonModule, LthnToggleOnDirective],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  host: { style: 'display: contents' },
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <nav class="rail">
      <a
        *ngFor="let it of nav; let last = last"
        [class.on]="(win.sub || 'models') === it[0]"
        [class.last]="last"
        (click)="wm.setSub(win.id, it[0])"
        ><lthn-icon [attr.name]="it[1]" [attr.aria-label]="it[2]" size="15"></lthn-icon
      ></a>
    </nav>
    <div class="appbody" [ngSwitch]="win.sub || 'models'">
      <ng-container *ngSwitchCase="'models'">
        <div class="ctoolbar">
          <h1 i18n="Local model view heading@@control.models.heading">Local models</h1>
          <lthn-badge [attr.variant]="dataStateVariant()" [attr.data-data-state]="dataState()">{{
            dataStateLabel()
          }}</lthn-badge>
          <span class="miniseg"
            ><span class="on" i18n="Running model filter@@control.models.filter.running"
              >Running</span
            ><span i18n="All models filter@@control.models.filter.all">All</span></span
          ><button class="nbtn" i18n="Load model action@@control.models.load">
            <lthn-icon name="plus" size="10"></lthn-icon> Load model
          </button>
        </div>
        <div class="tiles">
          <lthn-card pad="11"
            ><lthn-stat [attr.value]="modelRate()" [attr.label]="modelRateLabel()" mono></lthn-stat
          ></lthn-card>
          <lthn-card pad="11"
            ><lthn-stat
              [attr.value]="modelStorage()"
              [attr.label]="modelStorageLabel()"
              mono
            ></lthn-stat
          ></lthn-card>
          <lthn-card pad="11"
            ><lthn-stat
              [attr.value]="modelCount()"
              [attr.label]="modelCountLabel()"
              mono
            ></lthn-stat
          ></lthn-card>
          <lthn-card pad="11"
            ><lthn-stat
              [attr.value]="modelUptime()"
              label="Uptime"
              i18n-label="Uptime metric@@metric.uptime"
              mono
            ></lthn-stat
          ></lthn-card>
        </div>
        <div class="panel">
          <div class="ph">
            <b>{{ modelChartTitle() }}</b
            ><span>{{ modelChartPeak() }}</span>
          </div>
          <lthn-chart type="area" [attr.data]="throughputJson()" height="90"></lthn-chart>
        </div>
        <lthn-datatable
          selectable
          [attr.columns]="modelCols()"
          [attr.rows]="modelRows()"
        ></lthn-datatable>
      </ng-container>
      <ng-container *ngSwitchCase="'runs'">
        <div class="ctoolbar">
          <h1 i18n="Benchmark run view heading@@control.runs.heading">Benchmark runs</h1>
          <lthn-badge [attr.variant]="dataStateVariant()" [attr.data-data-state]="dataState()">{{
            dataStateLabel()
          }}</lthn-badge>
          <button class="nbtn" i18n="New benchmark run action@@control.runs.new">
            <lthn-icon name="play" size="10"></lthn-icon> New run
          </button>
        </div>
        <div class="panel">
          <div class="ph">
            <b i18n="Benchmark chart title@@control.runs.chart">tok/s by run</b
            ><span i18n="Benchmark chart range@@control.runs.lastFour">last 4</span>
          </div>
          <lthn-chart type="bar" [attr.data]="runsBar()" height="86"></lthn-chart>
        </div>
        <lthn-datatable [attr.columns]="runCols" [attr.rows]="runRows()"></lthn-datatable>
      </ng-container>
      <ng-container *ngSwitchCase="'power'">
        <div class="ctoolbar">
          <h1 i18n="Power view heading@@control.power.heading">Power</h1>
          <lthn-badge [attr.variant]="dataStateVariant()" [attr.data-data-state]="dataState()">{{
            dataStateLabel()
          }}</lthn-badge>
        </div>
        <div class="tiles c3">
          <lthn-card pad="11"
            ><lthn-stat
              value="196 W"
              label="24h avg"
              i18n-label="Daily average power metric@@control.power.dailyAverage"
              mono
            ></lthn-stat
          ></lthn-card>
          <lthn-card pad="11"
            ><lthn-stat
              value="4.7 kWh"
              label="24h total"
              i18n-label="Daily total power metric@@control.power.dailyTotal"
              mono
            ></lthn-stat
          ></lthn-card>
          <lthn-card pad="11"
            ><lthn-stat
              value="210 W"
              label="Peak today"
              i18n-label="Daily peak power metric@@control.power.peakToday"
              mono
            ></lthn-stat
          ></lthn-card>
        </div>
        <div class="panel">
          <div class="ph">
            <b i18n="Power chart title@@control.power.drawChart">Draw · last hour</b
            ><span i18n="Power chart unit@@control.power.watts">watts</span>
          </div>
          <lthn-chart type="area" [attr.data]="wattsJson" height="110"></lthn-chart>
        </div>
        <p
          style="font-size:12px;color:var(--fg-3);margin:0"
          i18n="Power usage comparison@@control.power.comparison"
        >
          ≈ a small fridge. Idle overnight drops to 38 W.
        </p>
      </ng-container>
      <ng-container *ngSwitchCase="'system'">
        <div class="ctoolbar">
          <h1 i18n="System view heading@@control.system.heading">System</h1>
          <lthn-badge [attr.variant]="dataStateVariant()" [attr.data-data-state]="dataState()">{{
            dataStateLabel()
          }}</lthn-badge>
          <span class="systabs"
            ><button
              class="systab"
              *ngFor="let t of systemTabs"
              [class.on]="(win.systab || 'overview') === t[0]"
              (click)="wm.setSysTab(win.id, t[0])"
            >
              {{ t[1] }}
            </button></span
          >
        </div>
        <ng-container [ngSwitch]="win.systab || 'overview'">
          <ng-container *ngSwitchCase="'processes'"
            ><lthn-datatable
              selectable
              [attr.columns]="procCols()"
              [attr.rows]="procRows()"
            ></lthn-datatable>
            <p
              style="font-size:11.5px;color:var(--fg-3);margin:10px 0 0"
              i18n="Process table help@@control.system.processHelp"
            >
              Live from the shared process registry. CPU, memory, and process actions still need
              backend sources.
            </p></ng-container
          >
          <ng-container *ngSwitchCase="'daemons'"
            ><lthn-datatable [attr.columns]="daemonCols" [attr.rows]="daemonRows"></lthn-datatable>
            <p
              style="font-size:11.5px;color:var(--fg-3);margin:10px 0 0"
              i18n="Daemon table help@@control.system.daemonHelp"
            >
              JSON daemon registry — PID files + health endpoints.
            </p></ng-container
          >
          <ng-container *ngSwitchDefault>
            <div class="tiles">
              <lthn-card pad="11"
                ><lthn-stat
                  [attr.value]="systemPrimaryValue()"
                  [attr.label]="systemPrimaryLabel()"
                  mono
                ></lthn-stat></lthn-card
              ><lthn-card pad="11"
                ><lthn-stat
                  [attr.value]="systemSecondaryValue()"
                  [attr.label]="systemSecondaryLabel()"
                  mono
                ></lthn-stat></lthn-card
              ><lthn-card pad="11"
                ><lthn-stat
                  [attr.value]="systemTertiaryValue()"
                  [attr.label]="systemTertiaryLabel()"
                  mono
                ></lthn-stat></lthn-card
              ><lthn-card pad="11"
                ><lthn-stat
                  [attr.value]="systemQuaternaryValue()"
                  [attr.label]="systemQuaternaryLabel()"
                  mono
                ></lthn-stat
              ></lthn-card>
            </div>
            <div class="panel">
              <div class="ph">
                <b i18n="Demo CPU chart title@@control.system.cpuDemoChart">CPU · demo history</b
                ><span i18n="Demo CPU value@@control.system.cpuDemoNow">34% demo</span>
              </div>
              <lthn-chart type="area" [attr.data]="systemChartJson" height="84"></lthn-chart>
            </div>
            <div class="panel">
              <div class="ph">
                <b i18n="Top process table title@@control.system.topProcesses">Top processes</b
                ><span i18n="Top process sort description@@control.system.byCpu">by CPU</span>
              </div>
              <lthn-datatable [attr.columns]="procCols()" [attr.rows]="procRows()"></lthn-datatable>
            </div>
          </ng-container>
        </ng-container>
      </ng-container>
      <ng-container *ngSwitchCase="'settings'">
        <div class="ctoolbar">
          <h1 i18n="Configuration view heading@@control.config.heading">Configuration</h1>
          <lthn-badge [attr.variant]="dataStateVariant()" [attr.data-data-state]="dataState()">{{
            dataStateLabel()
          }}</lthn-badge>
          <span class="cfgsrc" i18n="Configuration precedence@@control.config.precedence"
            >Defaults → File → Env → Set</span
          ><button class="nbtn" i18n="Commit configuration action@@control.config.commit">
            <lthn-icon name="check" size="10"></lthn-icon> Commit
          </button>
        </div>
        <div class="cfggrp" *ngFor="let g of cfgGroups()">
          <span class="glab">{{ g.name }}</span>
          <div class="cfgrow" *ngFor="let r of g.rows">
            <div class="cfgk">
              <b>{{ r.k }}</b
              ><span class="src {{ r.src }}">{{ sourceLabel(r.src) }}</span>
            </div>
            <input class="cfgin" [value]="r.v" [attr.aria-label]="r.k" />
          </div>
        </div>
        <div class="cfggrp">
          <span class="glab" i18n="Feature flag settings group@@control.config.featureFlags"
            >Feature flags</span
          >
          <div class="cfgrow" *ngFor="let f of cfgFlags()">
            <div class="cfgk">
              <b>{{ f.k }}</b
              ><span class="src {{ f.src }}">{{ sourceLabel(f.src) }}</span>
            </div>
            <lthn-toggle [on]="f.on"></lthn-toggle>
          </div>
        </div>
        <p class="cfghint" i18n="Configuration environment override help@@control.config.envHelp">
          <lthn-icon name="circle-info" size="11"></lthn-icon> Env overrides use
          <code>CORE_CONFIG_*</code>; environment values are never written back on Commit
          (dual-Viper).
        </p>
      </ng-container>
    </div>
  `,
})
export class ControlApp implements AppView, OnInit {
  @Input() win!: Win;
  @Input() nav: AppNavItem[] = [];
  wm = inject(WindowManagerService);
  private readonly liveData = inject(DesktopLiveDataService);
  private readonly mcpTools = this.registerMcpTools();
  readonly dataState = signal<DesktopDataState>(
    this.liveData.mode() === 'demo' ? 'demo' : 'loading',
  );
  readonly dataStateLabel = computed(() => desktopDataStateLabel(this.dataState()));
  readonly dataStateVariant = computed(() => desktopDataStateVariant(this.dataState()));
  readonly systemTabs: [string, string][] = [
    ['overview', $localize`:System tab@@control.system.tab.overview:Overview`],
    ['processes', $localize`:System tab@@control.system.tab.processes:Processes`],
    ['daemons', $localize`:System tab@@control.system.tab.daemons:Daemons`],
  ];
  readonly modelRate = signal('34.2');
  readonly modelRateLabel = signal($localize`:Tokens per second unit@@unit.tokensPerSecond:tok/s`);
  readonly modelStorage = signal('18.4 GB');
  readonly modelStorageLabel = signal($localize`:Video memory metric@@metric.vram:VRAM`);
  readonly modelCount = signal('128');
  readonly modelCountLabel = signal(
    $localize`:Requests per minute metric@@metric.requestsPerMinute:Req / min`,
  );
  readonly modelUptime = signal('6d 4h');
  readonly modelChartTitle = signal(
    $localize`:Throughput chart title@@control.models.throughputChart:Throughput · last hour`,
  );
  readonly modelChartPeak = signal(
    $localize`:Throughput chart peak@@control.models.throughputPeak:peak 41.8 tok/s`,
  );
  readonly systemPrimaryValue = signal('34%');
  readonly systemPrimaryLabel = signal($localize`:CPU metric@@metric.cpu:CPU`);
  readonly systemSecondaryValue = signal('18.4 / 32 GB');
  readonly systemSecondaryLabel = signal($localize`:Memory metric@@metric.memory:Memory`);
  readonly systemTertiaryValue = signal('6');
  readonly systemTertiaryLabel = signal(
    $localize`:Process count metric@@metric.processes:Processes`,
  );
  readonly systemQuaternaryValue = signal('4');
  readonly systemQuaternaryLabel = signal($localize`:Daemon count metric@@metric.daemons:Daemons`);
  readonly throughputJson = signal(JSON.stringify(TELEMETRY.throughput));
  readonly systemChartJson = JSON.stringify(TELEMETRY.throughput);
  wattsJson = JSON.stringify(TELEMETRY.watts);
  readonly runsBar = signal('[34.2,112.5,88.1,41.7]');
  readonly modelCols = signal(
    JSON.stringify([
      { key: 'name', label: $localize`:Model table column@@control.column.model:Model` },
      {
        key: 'rate',
        label: $localize`:Model table column@@control.column.tokensPerSecond:tok/s`,
        type: 'num',
      },
      {
        key: 'vram',
        label: $localize`:Model table column@@control.column.vram:VRAM`,
        type: 'num',
      },
      {
        key: 'region',
        label: $localize`:Model table column@@control.column.region:Region`,
        type: 'mono',
      },
      {
        key: 'status',
        label: $localize`:Model table column@@control.column.state:State`,
        type: 'status',
      },
    ]),
  );
  readonly modelRows = signal(
    '[{"name":"llama-3.1-70b","rate":34.2,"vram":18.4,"region":"eu-west-2","status":"running"},{"name":"mistral-small","rate":0,"vram":6.1,"region":"eu-west-2","status":"active"},{"name":"gemma-2-27b","rate":0,"vram":0,"region":"eu-west-1","status":"idle"},{"name":"qwen-2.5-coder","rate":0,"vram":0,"region":"eu-west-2","status":"idle"},{"name":"phi-3-mini","rate":0,"vram":2.3,"region":"eu-west-1","status":"stalled"},{"name":"llama-3.1-8b","rate":112.5,"vram":5.8,"region":"eu-west-2","status":"running"}]',
  );
  runCols = JSON.stringify([
    { key: 'run', label: $localize`:Run table column@@control.column.run:Run`, type: 'mono' },
    { key: 'model', label: $localize`:Run table column@@control.column.model:Model` },
    { key: 'ctx', label: $localize`:Run table column@@control.column.context:ctx`, type: 'num' },
    {
      key: 'toks',
      label: $localize`:Run table column@@control.column.tokensPerSecond:tok/s`,
      type: 'num',
    },
    { key: 'when', label: $localize`:Run table column@@control.column.when:When`, type: 'mono' },
  ]);
  readonly runRows = signal(
    '[{"run":"#0424","model":"llama-3.1-70b","ctx":8192,"toks":34.2,"when":"09:12"},{"run":"#0423","model":"llama-3.1-8b","ctx":8192,"toks":112.5,"when":"08:40"},{"run":"#0422","model":"mistral-small","ctx":4096,"toks":88.1,"when":"08:05"},{"run":"#0421","model":"gemma-2-27b","ctx":8192,"toks":41.7,"when":"Tue"}]',
  );
  readonly procCols = signal(
    JSON.stringify([
      {
        key: 'proc',
        label: $localize`:Process table column@@control.column.process:Process`,
        type: 'mono',
      },
      { key: 'pid', label: $localize`:Process table column@@control.column.pid:PID`, type: 'num' },
      { key: 'cpu', label: $localize`:Process table column@@control.column.cpu:CPU%`, type: 'num' },
      {
        key: 'mem',
        label: $localize`:Process table column@@control.column.memory:MEM`,
        type: 'num',
      },
      {
        key: 'state',
        label: $localize`:Process table column@@control.column.state:State`,
        type: 'status',
      },
    ]),
  );
  readonly procRows = signal(
    '[{"proc":"lthn-runner","pid":4821,"cpu":31.2,"mem":18.4,"state":"running"},{"proc":"llama-server","pid":4830,"cpu":2.1,"mem":6.1,"state":"running"},{"proc":"lethernet","pid":5102,"cpu":0.8,"mem":0.4,"state":"active"},{"proc":"go-config","pid":5110,"cpu":0,"mem":0.1,"state":"idle"},{"proc":"forge-watch","pid":5140,"cpu":0,"mem":0.2,"state":"idle"},{"proc":"mistral-server","pid":5201,"cpu":0,"mem":6.1,"state":"stalled"}]',
  );
  daemonCols = JSON.stringify([
    {
      key: 'code',
      label: $localize`:Daemon table column@@control.column.daemon:Daemon`,
      type: 'mono',
    },
    { key: 'daemon', label: $localize`:Daemon table column@@control.column.kind:Kind` },
    { key: 'pid', label: $localize`:Daemon table column@@control.column.pid:PID`, type: 'num' },
    {
      key: 'health',
      label: $localize`:Daemon table column@@control.column.health:Health`,
      type: 'status',
    },
    { key: 'project', label: $localize`:Daemon table column@@control.column.project:Project` },
  ]);
  daemonRows =
    '[{"code":"lthn","daemon":"inference","pid":4821,"health":"running","project":"lethean"},{"code":"go-pool","daemon":"worker","pid":5201,"health":"active","project":"mining"},{"code":"go-net","daemon":"mesh","pid":5102,"health":"running","project":"lethernet"},{"code":"go-cfg","daemon":"config","pid":5110,"health":"idle","project":"core"}]';
  readonly cfgGroups = signal([
    {
      name: $localize`:Configuration group@@control.config.group.server:Server`,
      rows: [
        { k: 'server.host', v: '127.0.0.1', src: 'file' },
        { k: 'server.port', v: '1988', src: 'set' },
        { k: 'server.cors', v: 'localhost', src: 'default' },
      ],
    },
    {
      name: $localize`:Configuration group@@control.config.group.models:Models`,
      rows: [
        { k: 'models.dir', v: '~/.lthn/models', src: 'env' },
        { k: 'models.autoload', v: 'llama-3.1-70b', src: 'file' },
      ],
    },
  ]);
  readonly cfgFlags = signal([
    { k: 'features.lethernet', on: true, src: 'file' },
    { k: 'features.telemetry', on: false, src: 'default' },
    { k: 'features.corePlay', on: true, src: 'set' },
  ]);

  ngOnInit(): void {
    if (this.liveData.mode() === 'demo') {
      this.dataState.set('demo');
      return;
    }
    void this.refresh();
  }

  async refresh(): Promise<void> {
    if (this.liveData.mode() === 'demo') return;
    this.dataState.set('loading');
    try {
      this.applyLiveSnapshot(await this.liveData.control());
      this.dataState.set('mixed');
    } catch {
      this.dataState.set('unavailable');
    }
  }

  private applyLiveSnapshot(snapshot: ControlLiveSnapshot): void {
    if (snapshot.models) {
      const totalBytes = snapshot.models.reduce((total, model) => total + model.sizeBytes, 0);
      this.modelRows.set(
        JSON.stringify(
          snapshot.models.map((model) => ({
            name: model.name,
            size: formatBytes(model.sizeBytes),
            source: model.isDirectory ? 'local directory' : 'local file',
            status: 'available',
          })),
        ),
      );
      this.modelCols.set(
        JSON.stringify([
          { key: 'name', label: $localize`:Model table column@@control.column.model:Model` },
          { key: 'size', label: $localize`:Model size column@@control.column.size:Size` },
          {
            key: 'source',
            label: $localize`:Model source column@@control.column.source:Source`,
            type: 'mono',
          },
          {
            key: 'status',
            label: $localize`:Model table column@@control.column.state:State`,
            type: 'status',
          },
        ]),
      );
      this.modelStorage.set(formatBytes(totalBytes));
      this.modelStorageLabel.set($localize`:Model storage metric@@control.models.storage:Storage`);
      this.modelCount.set(String(snapshot.models.length));
      this.modelCountLabel.set(
        $localize`:Local model count metric@@control.models.localCount:Local models`,
      );
    }

    if (snapshot.benchmarkRuns) {
      const rates = snapshot.benchmarkRuns.map((run) => run.generatedTokensPerSecond);
      const latest = rates[0];
      this.runRows.set(
        JSON.stringify(
          snapshot.benchmarkRuns.map((run) => ({
            run: `#${run.id.slice(0, 8)}`,
            model: run.model,
            ctx: run.contextLength,
            toks: Number(run.generatedTokensPerSecond.toFixed(1)),
            when: benchmarkTime(run.timestamp),
          })),
        ),
      );
      this.runsBar.set(JSON.stringify(rates));
      this.modelRate.set(latest === undefined ? '—' : latest.toFixed(1));
      this.modelRateLabel.set(
        $localize`:Latest generated throughput metric@@control.models.latestThroughput:Latest tg tok/s`,
      );
      this.throughputJson.set(JSON.stringify(rates));
      this.modelChartTitle.set(
        $localize`:Recent benchmark chart title@@control.models.recentBenchmarks:Benchmark throughput · recent runs`,
      );
      this.modelChartPeak.set(
        rates.length
          ? `peak ${Math.max(...rates).toFixed(1)} tok/s`
          : $localize`:No benchmark history@@control.models.noBenchmarks:No benchmark history`,
      );
    }

    if (snapshot.processes) {
      this.procCols.set(
        JSON.stringify([
          {
            key: 'command',
            label: $localize`:Process command column@@control.column.command:Command`,
          },
          {
            key: 'id',
            label: $localize`:Process identifier column@@control.column.processId:ID`,
            type: 'mono',
          },
          {
            key: 'state',
            label: $localize`:Process table column@@control.column.state:State`,
            type: 'status',
          },
          {
            key: 'exit',
            label: $localize`:Process exit code column@@control.column.exitCode:Exit code`,
            type: 'num',
          },
        ]),
      );
      this.procRows.set(
        JSON.stringify(
          snapshot.processes.map((process) => ({
            command: process.command,
            id: process.id,
            state: process.status,
            exit: process.exitCode,
          })),
        ),
      );
    }

    if (snapshot.settings) {
      const grouped = new Map<string, Array<{ k: string; v: string; src: string }>>();
      for (const control of snapshot.settings.controls) {
        if (control.kind === 'toggle') continue;
        const rows = grouped.get(control.group) ?? [];
        rows.push({
          k: control.key,
          v: String(control.value),
          src: control.configured ? 'set' : 'default',
        });
        grouped.set(control.group, rows);
      }
      this.cfgGroups.set(
        [...grouped].map(([name, rows]) => ({
          name,
          rows,
        })),
      );
      this.cfgFlags.set(
        snapshot.settings.controls
          .filter((control) => control.kind === 'toggle')
          .map((control) => ({
            k: control.key,
            on: control.value === true,
            src: control.configured ? 'set' : 'default',
          })),
      );
    }

    if (snapshot.telemetry) {
      this.modelUptime.set(formatUptime(snapshot.telemetry.uptimeSeconds));
      this.systemPrimaryValue.set(formatMegabytes(snapshot.telemetry.heapAllocMB));
      this.systemPrimaryLabel.set(
        $localize`:Process heap metric@@control.system.processHeap:Process heap`,
      );
      this.systemSecondaryValue.set(formatMegabytes(snapshot.telemetry.heapSysMB));
      this.systemSecondaryLabel.set(
        $localize`:Reserved heap metric@@control.system.reservedHeap:Reserved heap`,
      );
      this.systemTertiaryValue.set(String(snapshot.telemetry.numGoroutines));
      this.systemTertiaryLabel.set(
        $localize`:Goroutine count metric@@control.system.goroutines:Goroutines`,
      );
      this.systemQuaternaryValue.set(String(snapshot.telemetry.numGC));
      this.systemQuaternaryLabel.set(
        $localize`:Garbage collection count metric@@control.system.gcCycles:GC cycles`,
      );
    }
  }

  sourceLabel(source: string): string {
    const labels: Record<string, string> = {
      default: $localize`:Configuration source@@control.config.source.default:default`,
      file: $localize`:Configuration source@@control.config.source.file:file`,
      env: $localize`:Configuration source@@control.config.source.env:env`,
      set: $localize`:Configuration source@@control.config.source.set:set`,
    };
    return labels[source] ?? source;
  }

  private async registerMcpTools(): Promise<void> {
    const sections = CTRL_NAV.map(([id]) => id);

    await Promise.all([
      declareExperimentalWebMcpTool({
        name: 'control_read_state',
        description:
          'Reads the Control app section and the model state currently presented by the app.',
        inputSchema: {
          type: 'object',
          properties: {},
          additionalProperties: false,
        },
        execute: () => ({
          content: [
            {
              type: 'text',
              text: JSON.stringify({
                section: this.win.sub || 'models',
                system_tab: this.win.systab || 'overview',
                models: JSON.parse(this.modelRows()),
              }),
            },
          ],
        }),
      }),
      declareExperimentalWebMcpTool({
        name: 'control_show_section',
        description:
          'Changes the visible Control app section without mutating model or system state.',
        inputSchema: {
          type: 'object',
          properties: {
            section: {
              type: 'string',
              enum: sections,
              description: 'Control navigation section.',
            },
          },
          required: ['section'],
          additionalProperties: false,
        },
        execute: ({ section }) => {
          if (!sections.includes(section)) {
            throw new Error(
              `Unknown Control section "${section}". Expected one of: ${sections.join(', ')}.`,
            );
          }
          this.wm.setSub(this.win.id, section);
          return {
            content: [
              {
                type: 'text',
                text: JSON.stringify({ ok: true, section }),
              },
            ],
          };
        },
      }),
    ]);
  }
}

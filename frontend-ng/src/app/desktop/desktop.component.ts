import {
  AfterViewInit,
  ChangeDetectorRef,
  Component,
  CUSTOM_ELEMENTS_SCHEMA,
  ElementRef,
  HostListener,
  NgZone,
  OnDestroy,
  ViewChild,
  ViewEncapsulation,
  afterNextRender,
  effect,
  inject,
  signal,
  ChangeDetectionStrategy,
} from '@angular/core';
import { CommonModule } from '@angular/common';
import { NavigationEnd, Router } from '@angular/router';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { filter } from 'rxjs';
import { APPS, LANGS, ORDER, TELEMETRY, Win } from './desktop.data';
import { WindowManagerService } from './window-manager.service';
import { WindowRouteContent } from './window-route-content';
import {
  Brand,
  Design,
  Mode,
  PreferencesService,
  TaskbarEdge,
  Wallpaper,
} from './preferences.service';
import {
  DesktopMenuApp,
  DesktopMenuCategory,
  desktopTargetFromSnapshot,
  readDesktopRouteCatalog,
  routeSegmentsForWindow,
} from './desktop-route-tree';
import { DESKTOP_STORAGE } from '../store/storage.service';
import { ShellMenuBar } from './shell/menu-bar';
import { ShellTaskbarDock } from './shell/taskbar-dock';
import type {
  ShellUserIdentity,
  ShellWindowGroup,
} from './shell/shell.types';

interface CtxItem {
  label?: string;
  icon?: string;
  sep?: boolean;
  act?: () => void;
  children?: CtxItem[];
}

/**
 * Lethean desktop OS — MERGED chrome. macOS-style windows + top menu bar, a
 * Windows-style taskbar that docks to any edge (top/right/bottom/left) with a
 * fresh session chip, and a macOS dock floating adjacent to the taskbar.
 * Right-click a dock icon for window actions. Top-level nav (APPS) opens as full
 * windows; child components render inside; window CONTENT is the shared <lthn-*>.
 * The standalone preview.html mirrors this component exactly.
 */
@Component({
  selector: 'lthn-desktop',
  standalone: true,
  imports: [CommonModule, WindowRouteContent, ShellMenuBar, ShellTaskbarDock],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  encapsulation: ViewEncapsulation.None,
  templateUrl: './desktop.component.html',
  styleUrl: './desktop.component.scss',
  changeDetection: ChangeDetectionStrategy.Eager,
  host: {
    '[attr.data-brand]': 'brand',
    style: 'display:flex;flex-direction:column;width:100%;height:100%;overflow:hidden',
  },
})
export class DesktopComponent implements AfterViewInit, OnDestroy {
  readonly wm = inject(WindowManagerService);
  readonly prefs = inject(PreferencesService);
  private readonly router = inject(Router);
  private readonly storage = inject(DESKTOP_STORAGE);
  private readonly changeDetector = inject(ChangeDetectorRef);
  readonly routeCatalog = readDesktopRouteCatalog(this.router.config);
  readonly APPS = APPS;
  readonly order = ORDER;
  readonly categories = this.routeCatalog.categories;

  get bar(): TaskbarEdge {
    return this.prefs.bar();
  }
  set bar(value: TaskbarEdge) {
    this.prefs.bar.set(value);
  }
  get wall(): Wallpaper {
    return this.prefs.wallpaper();
  }
  set wall(value: Wallpaper) {
    this.prefs.wallpaper.set(value);
  }
  get mode(): Mode {
    return this.prefs.mode();
  }
  set mode(value: Mode) {
    this.prefs.mode.set(value);
  }
  get brand(): Brand {
    return this.prefs.brand();
  }
  set brand(value: Brand) {
    this.prefs.brand.set(value);
  }
  get design(): Design {
    return this.prefs.design();
  }
  set design(value: Design) {
    this.prefs.design.set(value);
  }
  get customHue(): number {
    return this.prefs.customHue();
  }
  set customHue(value: number) {
    this.prefs.customHue.set(value);
  }
  get customName(): string {
    return this.prefs.customName();
  }
  set customName(value: string) {
    this.prefs.customName.set(value);
  }
  get lang(): string {
    return this.prefs.lang();
  }
  set lang(value: string) {
    this.prefs.lang.set(value);
  }
  get showIcons(): boolean {
    return this.prefs.showIcons();
  }
  set showIcons(value: boolean) {
    this.prefs.showIcons.set(value);
  }
  get reduceMotion(): boolean {
    return this.prefs.reduceMotion();
  }
  set reduceMotion(value: boolean) {
    this.prefs.reduceMotion.set(value);
  }
  get showWidgets(): boolean {
    return this.prefs.showWidgets();
  }
  set showWidgets(value: boolean) {
    this.prefs.showWidgets.set(value);
  }
  clock = '';
  sessionOpen = false;
  sessionPos: { left: number; top: number | null; bottom: number | null } = {
    left: 0,
    top: 0,
    bottom: null,
  };
  ctx: { open: boolean; left: number; top: number; title: string; items: CtxItem[] } = {
    open: false,
    left: 0,
    top: 0,
    title: '',
    items: [],
  };
  csub = { open: false, left: 0, top: 0, i: -1 };
  snap = { zone: null as string | null, left: 0, top: 0, w: 0, h: 0 };
  selected: string[] = [];
  groups: ShellWindowGroup[] = [];
  shellTabs: any[] = [];
  marquee = { open: false, left: 0, top: 0, w: 0, h: 0 };
  proxy = { open: false, left: 0, top: 0, n: 0, over: false };
  tray = { open: false, key: '', left: 0, top: 0 };
  mbKey = '';
  readonly langs = LANGS;
  palette = { open: false, q: '', i: 0 };
  paletteLastFocus: HTMLElement | null = null;
  filtered: any[] = [];
  private metaDown = false;
  private metaCombo = false;
  user: ShellUserIdentity = {
    initials: 'SR',
    name: 'Sarah Reeve',
    email: 'sarah@lethean.local',
    host: 'lethean.local',
  };
  users = [
    { initials: 'SR', name: 'Sarah Reeve', email: 'sarah@lethean.local', host: 'lethean.local' },
    { initials: 'JM', name: 'Jai Mistry', email: 'jai@lethean.local', host: 'lethean.local' },
    { initials: 'VI', name: 'Vi (guest)', email: 'guest@lethean.local', host: 'lethean.local' },
  ];
  session: 'active' | 'locked' | 'switch' | 'login' | 'shutting' | 'off' = 'active';
  private timer: any;
  private welcomeTimer: any;
  private timeReady = false;
  private readonly browserReady = signal(false);

  @ViewChild('osEl') osEl!: ElementRef<HTMLElement>;
  @ViewChild('winlayerEl') winlayerEl!: ElementRef<HTMLElement>;
  @ViewChild('sessionMenuEl') sessionMenuEl!: ElementRef<HTMLElement>;
  @ViewChild('ctxMenuEl') ctxMenuEl!: ElementRef<HTMLElement>;
  @ViewChild('trayEl') trayEl!: ElementRef<HTMLElement>;
  @ViewChild('plInput') plInput!: ElementRef<HTMLInputElement>;

  openCats: Record<string, boolean> = {};
  sub = { open: false, left: 0, top: 0, parent: '' };
  // Column headings and metric labels are UI chrome. Rows, tree nodes, cards,
  // kanban content, console lines, and terminal output model CoreGO payload data.
  DEVPANEL: any = {
    'control-panel': {
      kind: 'table',
      tiles: [
        ['9/9', $localize`:Developer metric@@devPanel.metric.services:Services`],
        ['34%', $localize`:Developer metric@@devPanel.metric.cpu:CPU`],
        ['18.4 GB', $localize`:Developer metric@@devPanel.metric.memory:Mem`],
        ['6d 4h', $localize`:Developer metric@@devPanel.metric.uptime:Uptime`],
      ],
      cols: JSON.stringify([
        { key: 'svc', label: $localize`:Developer table column@@devPanel.column.service:Service` },
        {
          key: 'st',
          label: $localize`:Developer table column@@devPanel.column.state:State`,
          type: 'status',
        },
        {
          key: 'port',
          label: $localize`:Developer table column@@devPanel.column.port:Port`,
          type: 'mono',
        },
      ]),
      rows: '[{"svc":"inference","st":"running","port":"1988"},{"svc":"api","st":"running","port":"8080"},{"svc":"lethernet","st":"active","port":"4666"},{"svc":"store","st":"idle","port":"5432"}]',
    },
    explorer: {
      kind: 'tree',
      nodes: [
        [0, 'src', 1],
        [1, 'app', 1],
        [2, 'app.ts', 0],
        [2, 'app.routes.ts', 0],
        [1, 'elements', 1],
        [1, 'frame', 1],
        [0, 'go', 1],
        [1, 'cmd', 1],
        [1, 'pkg', 1],
        [0, 'README.md', 0],
        [0, 'go.work', 0],
      ],
    },
    search: {
      kind: 'table',
      cols: JSON.stringify([
        {
          key: 'file',
          label: $localize`:Developer table column@@devPanel.column.file:File`,
          type: 'mono',
        },
        {
          key: 'ln',
          label: $localize`:Developer table column@@devPanel.column.line:Ln`,
          type: 'num',
        },
        { key: 'match', label: $localize`:Developer table column@@devPanel.column.match:Match` },
      ]),
      rows: '[{"file":"app.routes.ts","ln":58,"match":"loadComponent build"},{"file":"build.component.ts","ln":12,"match":"class BuildComponent"},{"file":"process.go","ln":204,"match":"func (s *Service) Start"}]',
    },
    git: {
      kind: 'table',
      cols: JSON.stringify([
        {
          key: 'change',
          label: $localize`:Developer table column@@devPanel.column.change:Change`,
        },
        {
          key: 'file',
          label: $localize`:Developer table column@@devPanel.column.file:File`,
          type: 'mono',
        },
      ]),
      rows: '[{"change":"Modified","file":"app.routes.ts"},{"change":"Added","file":"desktop.component.ts"},{"change":"Deleted","file":"legacy/old.ts"}]',
    },
    build: {
      kind: 'console',
      lines: [
        ['09:20:01', 'go', 'build ./cmd/lthn …'],
        ['09:20:07', 'go', 'ok — 3 targets · 4.1s'],
        ['09:20:07', 'ng', 'build frontend …'],
        ['09:20:24', 'ng', '✓ 25 lazy chunks · 1.2 MB'],
      ],
    },
    process: {
      kind: 'table',
      cols: JSON.stringify([
        {
          key: 'pid',
          label: $localize`:Developer table column@@devPanel.column.pid:PID`,
          type: 'mono',
        },
        {
          key: 'cmd',
          label: $localize`:Developer table column@@devPanel.column.command:Command`,
        },
        {
          key: 'status',
          label: $localize`:Developer table column@@devPanel.column.state:State`,
          type: 'status',
        },
        {
          key: 'cpu',
          label: $localize`:Developer table column@@devPanel.column.cpuPercent:CPU %`,
          type: 'num',
        },
      ]),
      rows: '[{"pid":"4821","cmd":"lthn-inference serve","status":"running","cpu":142},{"pid":"5201","cmd":"go-pool worker","status":"active","cpu":3.2},{"pid":"5410","cmd":"go-proxy stratum","status":"running","cpu":1.1}]',
    },
    containers: {
      kind: 'table',
      cols: JSON.stringify([
        {
          key: 'name',
          label: $localize`:Developer table column@@devPanel.column.container:Container`,
        },
        {
          key: 'image',
          label: $localize`:Developer table column@@devPanel.column.image:Image`,
          type: 'mono',
        },
        {
          key: 'st',
          label: $localize`:Developer table column@@devPanel.column.state:State`,
          type: 'status',
        },
        {
          key: 'ports',
          label: $localize`:Developer table column@@devPanel.column.ports:Ports`,
          type: 'mono',
        },
      ]),
      rows: '[{"name":"forgejo","image":"codeberg/forgejo","st":"running","ports":"3000"},{"name":"n8n","image":"n8nio/n8n","st":"running","ports":"5678"},{"name":"vaultwarden","image":"vaultwarden/server","st":"active","ports":"8000"},{"name":"sonarqube","image":"sonarqube:lts","st":"idle","ports":"9000"}]',
    },
    repos: {
      kind: 'table',
      cols: JSON.stringify([
        {
          key: 'repo',
          label: $localize`:Developer table column@@devPanel.column.repository:Repository`,
        },
        {
          key: 'branch',
          label: $localize`:Developer table column@@devPanel.column.branch:Branch`,
          type: 'mono',
        },
        {
          key: 'st',
          label: $localize`:Developer table column@@devPanel.column.state:State`,
          type: 'status',
        },
        {
          key: 'sync',
          label: $localize`:Developer table column@@devPanel.column.sync:Sync`,
          type: 'mono',
        },
      ]),
      rows: '[{"repo":"core/go","branch":"dev","st":"running","sync":"↑ 0 ↓ 0"},{"repo":"core/ide","branch":"dev","st":"active","sync":"↑ 2 ↓ 0"},{"repo":"core/play","branch":"dev","st":"idle","sync":"↑ 0 ↓ 1"},{"repo":"desktop","branch":"main","st":"running","sync":"↑ 0 ↓ 0"}]',
    },
    devops: {
      kind: 'table',
      cols: JSON.stringify([
        { key: 'play', label: $localize`:Developer table column@@devPanel.column.play:Play` },
        {
          key: 'host',
          label: $localize`:Developer table column@@devPanel.column.host:Host`,
          type: 'mono',
        },
        {
          key: 'st',
          label: $localize`:Developer table column@@devPanel.column.state:State`,
          type: 'status',
        },
        {
          key: 'dur',
          label: $localize`:Developer table column@@devPanel.column.duration:Dur`,
          type: 'mono',
        },
      ]),
      rows: '[{"play":"provision","host":"vi-01.lan","st":"running","dur":"12s"},{"play":"deploy inference","host":"vi-02.lan","st":"active","dur":"41s"},{"play":"harden","host":"hoplite-7","st":"idle","dur":"—"}]',
    },
    tenant: {
      kind: 'table',
      cols: JSON.stringify([
        {
          key: 'tenant',
          label: $localize`:Developer table column@@devPanel.column.tenant:Tenant`,
        },
        { key: 'plan', label: $localize`:Developer table column@@devPanel.column.plan:Plan` },
        {
          key: 'users',
          label: $localize`:Developer table column@@devPanel.column.users:Users`,
          type: 'num',
        },
        {
          key: 'st',
          label: $localize`:Developer table column@@devPanel.column.state:State`,
          type: 'status',
        },
      ]),
      rows: '[{"tenant":"lethean","plan":"Sovereign","users":42,"st":"running"},{"tenant":"host-uk","plan":"Business","users":18,"st":"active"},{"tenant":"studio","plan":"Starter","users":3,"st":"idle"}]',
    },
    forge: {
      kind: 'cards',
      cards: [
        { title: 'go-service', sub: 'Go microservice', icon: 'server' },
        { title: 'angular-app', sub: 'Angular 18 SPA', icon: 'window-maximize' },
        { title: 'stim-bundle', sub: 'CorePlay bundle', icon: 'gamepad' },
        { title: 'mcp-server', sub: 'MCP tool server', icon: 'plug' },
      ],
    },
    marketplace: {
      kind: 'cards',
      cards: [
        { title: 'Vi', sub: 'AI copilot', icon: 'robot' },
        { title: 'Prettier', sub: 'Formatter', icon: 'wand-magic-sparkles' },
        { title: 'GitLens', sub: 'Git insight', icon: 'code-branch' },
        { title: 'Docker', sub: 'Containers', icon: 'cubes-stacked' },
      ],
    },
    tasks: {
      kind: 'kanban',
      columns: [
        { name: 'To do', cards: ['Rip out legacy UI', 'Wire process panel', 'LSP in editor'] },
        { name: 'In progress', cards: ['Desktop shell', 'i18n viewer'] },
        { name: 'Done', cards: ['Command palette', 'Theme editor', 'App categories'] },
      ],
    },
    terminal: {
      kind: 'term',
      lines: [
        'lthn@desktop ~/core % core build',
        '\u2192 3 targets built in 4.1s',
        'lthn@desktop ~/core % core play mega-lo-mania',
        '\u2192 launching STIM bundle (retroarch)\u2026',
        'lthn@desktop ~/core % ',
      ],
    },
  };
  readonly throughput = TELEMETRY.throughput;
  readonly watts = TELEMETRY.watts;
  clocks = [
    {
      city: $localize`:World clock city@@desktop.clock.london:London`,
      zone: $localize`:World clock zone@@desktop.clock.londonZone:London`,
      tz: 'Europe/London',
    },
    {
      city: $localize`:World clock city@@desktop.clock.newYork:New York`,
      zone: $localize`:World clock zone@@desktop.clock.newYorkZone:New York`,
      tz: 'America/New_York',
    },
    {
      city: $localize`:World clock city@@desktop.clock.singapore:Singapore`,
      zone: $localize`:World clock zone@@desktop.clock.singaporeZone:Singapore`,
      tz: 'Asia/Singapore',
    },
  ];
  pkgs = [
    {
      name: 'llama.cpp',
      state: $localize`:Package state@@desktop.package.running:running`,
      variant: 'ok',
    },
    {
      name: 'lthn-runner',
      state: $localize`:Package state@@desktop.package.active:active`,
      variant: 'ok',
    },
    {
      name: 'LetherNet',
      state: $localize`:Package state@@desktop.package.idle:idle`,
      variant: '',
    },
  ];

  get throughputJson() {
    return JSON.stringify(this.throughput);
  }
  get wattsJson() {
    return JSON.stringify(this.watts);
  }

  notifs: { id: number; icon: string; title: string; body: string }[] = [];
  private nid = 0;
  notify(icon: string, title: string, body = '') {
    const id = ++this.nid;
    this.notifs.unshift({ id, icon, title, body });
    if (this.notifs.length > 4) this.notifs.pop();
    setTimeout(() => this.dismiss(id), 4500);
  }
  dismiss(id: number) {
    this.notifs = this.notifs.filter((n) => n.id !== id);
  }

  constructor(private zone: NgZone) {
    this.restore();
    this.router.events
      .pipe(
        filter((event): event is NavigationEnd => event instanceof NavigationEnd),
        takeUntilDestroyed(),
      )
      .subscribe(() => {
        if (!this.browserReady()) return;
        this.selectWindowFromRoute();
        this.reflectFocusedWindowInRoute();
      });
    effect(() => {
      if (this.browserReady()) this.reflectFocusedWindowInRoute();
    });
    effect(() => {
      this.prefs.design();
      this.prefs.customHue();
      this.prefs.customName();
      this.applyDesign();
    });
    afterNextRender(() => {
      // Lit uses light DOM. Register it only after Angular has reconciled any
      // prerendered nodes, avoiding custom-element DOM mutations during hydration.
      void Promise.all([
        import('../../kit/lthn-core'),
        import('../../kit/lthn-charts'),
        import('../../kit/lthn-datatable'),
      ]).catch((error: unknown) => console.error(error));

      this.browserReady.set(true);
      this.selectWindowFromRoute();
      this.reflectFocusedWindowInRoute();
      this.timeReady = true;
      this.updateClock();
      this.welcomeTimer = setTimeout(() => {
        if (this.session === 'active') {
          this.notify(
            'cube',
            $localize`:Notification title@@desktop.notification.modelLoaded:Model loaded`,
            'llama-3.1-70b · 18.4 GB',
          );
          this.notify(
            'diagram-project',
            APPS['lethernet'].title,
            $localize`:Notification body@@desktop.notification.peerJoined:Peer vi-02.lan joined`,
          );
        }
      }, 700);
      this.timer = setInterval(() => this.zone.run(() => this.updateClock()), 15000);
    });
  }
  ngAfterViewInit() {
    this.applyDesign();
  }
  ngOnDestroy() {
    clearInterval(this.timer);
    clearTimeout(this.welcomeTimer);
  }
  updateClock() {
    this.clock = new Date().toLocaleTimeString('en-GB', { hour: '2-digit', minute: '2-digit' });
    this.changeDetector.markForCheck();
  }
  fmtTz(tz: string) {
    if (!this.timeReady) return '';
    return new Date().toLocaleTimeString('en-GB', {
      hour: '2-digit',
      minute: '2-digit',
      timeZone: tz,
    });
  }

  transform(w: Win) {
    return w.minimizing
      ? `translate(${w.x}px,${w.y}px) scale(.06)`
      : `translate(${w.x}px,${w.y}px)`;
  }
  minOrigin() {
    return (
      (
        { bottom: '50% 100%', top: '50% 0%', left: '0% 50%', right: '100% 50%' } as Record<
          string,
          string
        >
      )[this.bar] || '50% 100%'
    );
  }
  private get wins() {
    return this.wm.wins();
  }
  private get focusId() {
    return this.wm.focusId();
  }
  activeApp() {
    const f = this.wins.find((w) => w.id === this.focusId && !w.min);
    return f && this.APPS[f.app]
      ? this.APPS[f.app].title
      : $localize`:Product name@@brand.lethean:Lethean`;
  }
  panelFor(w: Win) {
    const route = APPS[w.app]?.route ?? '';
    return this.DEVPANEL[route] ?? {};
  }
  emptyFor(w: Win): [string, string, string] | null {
    return APPS[w.app]?.route === 'git'
      ? [
          $localize`:Source control empty-state title@@devPanel.git.noChanges:No changes`,
          'code-branch',
          $localize`:Source control empty-state message@@devPanel.git.cleanTree:Working tree clean — nothing to commit`,
        ]
      : null;
  }

  setBar(b: TaskbarEdge) {
    this.bar = b;
    this.closeMenus();
  }
  edgeLabel(edge: TaskbarEdge): string {
    const labels: Record<TaskbarEdge, string> = {
      top: $localize`:Taskbar edge@@desktop.edge.top:Top`,
      right: $localize`:Taskbar edge@@desktop.edge.right:Right`,
      bottom: $localize`:Taskbar edge@@desktop.edge.bottom:Bottom`,
      left: $localize`:Taskbar edge@@desktop.edge.left:Left`,
    };
    return labels[edge];
  }
  setMode(m: Mode) {
    this.mode = m;
  }
  setBrand(b: Brand) {
    this.brand = b;
  }
  persist() {
    try {
      const stored = JSON.parse(this.storage.getItem('lthn.desktop') || 'null');
      const windowState = stored && typeof stored === 'object' ? stored : {};
      this.storage.setItem(
        'lthn.desktop',
        JSON.stringify({
          ...windowState,
          bar: this.bar,
          wall: this.wall,
          mode: this.mode,
          brand: this.brand,
          design: this.design,
          customHue: this.customHue,
          customName: this.customName,
          lang: this.lang,
          showIcons: this.showIcons,
          reduceMotion: this.reduceMotion,
          showWidgets: this.showWidgets,
          openCats: this.openCats,
          groups: this.groups,
          shellTabs: this.shellTabs,
        }),
      );
    } catch {}
    this.wm.persist();
  }
  restore() {
    try {
      const s = JSON.parse(this.storage.getItem('lthn.desktop') || 'null');
      if (!s) return;
      (
        [
          'bar',
          'wall',
          'mode',
          'brand',
          'design',
          'customHue',
          'customName',
          'lang',
          'showIcons',
          'reduceMotion',
          'showWidgets',
        ] as const
      ).forEach((k) => {
        if (s[k] !== undefined) (this as any)[k] = s[k];
      });
      this.openCats = s.openCats || {};
      this.groups = s.groups || [];
      if (Array.isArray(s.shellTabs)) this.shellTabs = s.shellTabs;
    } catch {}
  }

  catApps(id: string | null): DesktopMenuApp[] {
    const c = this.categories.find((x) => x.id === id);
    return c ? c.apps : [];
  }
  catIcon(id: string | null) {
    const c = this.categories.find((x) => x.id === id);
    return c ? c.icon : '';
  }
  catLabel(id: string | null) {
    const c = this.categories.find((x) => x.id === id);
    return c ? c.title : '';
  }
  launchFromDock(app: string) {
    this.closeMenus();
    this.wm.launch(app);
  }
  get runningApps() {
    return [...new Set(this.wins.filter((w) => !w.group && this.APPS[w.app]).map((w) => w.app))];
  }
  get deskIcons() {
    return this.order.filter((id) => !this.APPS[id].dev);
  }
  get taskWins() {
    return this.wins.filter((w) => !w.group && this.APPS[w.app]);
  }
  dockClick(app: string) {
    const w = this.wins.find((x) => x.app === app);
    if (!w) return;
    if (w.min || this.focusId !== w.id) this.wm.focus(w.id);
    else this.wm.minimise(w.id, this.reduceMotion ? 0 : 190);
    this.closeMenus();
  }
  startLaunch(app: DesktopMenuApp) {
    this.closeMenus();
    this.wm.launch(app.id);
  }
  onProg(app: DesktopMenuApp, ev: Event) {
    if (app.children.length) {
      const row = ev.currentTarget as HTMLElement;
      this.sub.parent = app.id;
      this.sub.open = true;
      setTimeout(() => this.placeSub(row), 0);
    } else this.sub.open = false;
  }
  submenuFor(appId: string) {
    return this.routeCatalog.apps[appId]?.children ?? [];
  }
  placeSub(rowEl: HTMLElement) {
    const sm = this.sessionMenuEl?.nativeElement,
      o = this.osEl?.nativeElement;
    if (!sm || !o) return;
    const smR = sm.getBoundingClientRect(),
      rR = rowEl.getBoundingClientRect(),
      oR = o.getBoundingClientRect();
    let left = rR.right - smR.left - 6;
    if (smR.left + left + 184 > oR.right) left = rR.left - smR.left - 184 + 6;
    this.sub.left = left;
    this.sub.top = Math.max(0, rR.top - smR.top);
  }
  startSub(appId: string, subId: string) {
    this.closeMenus();
    this.sub.open = false;
    this.wm.launch(appId);
    const w = this.wins.find((x) => x.app === appId);
    if (w) this.wm.setSub(w.id, subId);
  }

  // ── marquee multi-select + drag-to-dock grouping (click-drag UX) ──
  hit(a: DOMRect, b: { left: number; top: number; right: number; bottom: number }) {
    return a.left < b.right && a.right > b.left && a.top < b.bottom && a.bottom > b.top;
  }
  startMarquee(ev: PointerEvent) {
    if (!this.wm.windowed() || this.session !== 'active' || ev.button !== 0) return;
    if ((ev.target as HTMLElement).closest('.win, .bar, .dock, .menubar, .menu, .dicon, .widget'))
      return;
    const o = this.osEl.nativeElement.getBoundingClientRect(),
      wl = this.winlayerEl.nativeElement;
    const sx = ev.clientX,
      sy = ev.clientY;
    let moved = false;
    const mv = (e: PointerEvent) => {
      const x = Math.min(sx, e.clientX),
        y = Math.min(sy, e.clientY),
        w = Math.abs(e.clientX - sx),
        h = Math.abs(e.clientY - sy);
      if (w > 4 || h > 4) moved = true;
      this.marquee = { open: true, left: x - o.left, top: y - o.top, w, h };
      const rect = { left: x, top: y, right: x + w, bottom: y + h },
        els = Array.from(wl.querySelectorAll('.win')) as HTMLElement[];
      this.selected = this.wins
        .filter(
          (win, i) =>
            !win.min && !win.group && els[i] && this.hit(els[i].getBoundingClientRect(), rect),
        )
        .map((win) => win.id);
    };
    const up = () => {
      window.removeEventListener('pointermove', mv);
      window.removeEventListener('pointerup', up);
      this.marquee.open = false;
      if (!moved) this.selected = [];
    };
    window.addEventListener('pointermove', mv);
    window.addEventListener('pointerup', up);
  }
  groupDrag(ev: PointerEvent) {
    const ids = this.selected.slice(),
      o = this.osEl.nativeElement.getBoundingClientRect(),
      dock = this.osEl.nativeElement.querySelector('.dock') as HTMLElement;
    const start = ids.map((id) => {
      const w = this.wins.find((x) => x.id === id)!;
      return { id, x: w.x, y: w.y };
    });
    const sx = ev.clientX,
      sy = ev.clientY;
    const mv = (e: PointerEvent) => {
      const dx = e.clientX - sx,
        dy = e.clientY - sy;
      start.forEach((s) => this.wm.move(s.id, s.x + dx, s.y + dy));
      const dR = dock?.getBoundingClientRect();
      const over =
        !!dR &&
        e.clientX >= dR.left &&
        e.clientX <= dR.right &&
        e.clientY >= dR.top &&
        e.clientY <= dR.bottom;
      this.proxy = {
        open: true,
        left: e.clientX - o.left + 14,
        top: e.clientY - o.top + 14,
        n: ids.length,
        over,
      };
    };
    const up = () => {
      window.removeEventListener('pointermove', mv);
      window.removeEventListener('pointerup', up);
      const over = this.proxy.over;
      this.proxy.open = false;
      if (over) this.createGroup(ids);
    };
    window.addEventListener('pointermove', mv);
    window.addEventListener('pointerup', up);
  }
  createGroup(ids: string[]) {
    if (ids.length < 2) return;
    const id = 'g' + Date.now();
    const apps = ids
      .map((wid) => this.wins.find((w) => w.id === wid))
      .filter(Boolean)
      .map((w) => (w as Win).app);
    this.wm.wins.update((ws) =>
      ws.map((w) => (ids.includes(w.id) ? { ...w, min: true, group: id } : w)),
    );
    this.groups.push({
      id,
      name: $localize`:Window group name@@desktop.group.name:Group ${this.groups.length + 1}:groupNumber:`,
      ids: ids.slice(),
      apps,
      open: false,
    });
    this.selected = [];
    if (this.focusId && ids.includes(this.focusId)) this.wm.focusId.set(null);
    this.persist();
  }
  toggleGroup(id: string) {
    const g = this.groups.find((x) => x.id === id);
    if (!g) return;
    g.open = !g.open;
    this.wm.wins.update((ws) =>
      ws.map((w) =>
        g.ids.includes(w.id) ? { ...w, min: !g.open, z: g.open ? this.wm.nextZ() : w.z } : w,
      ),
    );
    if (g.open) this.wm.focusId.set(g.ids[g.ids.length - 1] ?? null);
    else if (this.focusId && g.ids.includes(this.focusId)) this.wm.focusId.set(null);
    this.persist();
  }
  splitGroup(id: string) {
    const g = this.groups.find((x) => x.id === id);
    if (!g) return;
    this.wm.wins.update((ws) =>
      ws.map((w) =>
        g.ids.includes(w.id) ? { ...w, group: undefined, z: g.open ? this.wm.nextZ() : w.z } : w,
      ),
    );
    this.groups = this.groups.filter((x) => x.id !== id);
    this.persist();
  }
  closeGroup(id: string) {
    const g = this.groups.find((x) => x.id === id);
    if (!g) return;
    this.wm.wins.update((ws) => ws.filter((w) => !g.ids.includes(w.id)));
    if (this.focusId && g.ids.includes(this.focusId)) this.wm.focusId.set(null);
    this.groups = this.groups.filter((x) => x.id !== id);
    this.persist();
  }
  groupItems(g: any): CtxItem[] {
    return [
      {
        label: g.open
          ? $localize`:Window group action@@desktop.group.collapse:Collapse`
          : $localize`:Window group action@@desktop.group.restoreAll:Restore all`,
        icon: g.open ? 'compress' : 'expand',
        act: () => this.toggleGroup(g.id),
      },
      {
        label: $localize`:Window group action@@desktop.group.split:Split group`,
        icon: 'object-ungroup',
        act: () => this.splitGroup(g.id),
      },
      { sep: true },
      {
        label: $localize`:Window group action@@desktop.group.closeAll:Close all`,
        icon: 'xmark',
        act: () => this.closeGroup(g.id),
      },
    ];
  }
  groupCtx(ev: MouseEvent, g: any) {
    ev.preventDefault();
    ev.stopPropagation();
    this.closeMenus();
    this.ctx.title = g.name;
    this.ctx.items = this.groupItems(g);
    this.openCtxAt(ev);
  }

  // ── menu-bar tray panels — contents come from the related app ──
  openTray(key: string, ev: Event) {
    ev.stopPropagation();
    this.sessionOpen = false;
    this.ctx.open = false;
    this.csub.open = false;
    this.sub.open = false;
    if (this.tray.open && this.tray.key === key) {
      this.tray.open = false;
      return;
    }
    this.tray.key = key;
    this.tray.open = true;
    const btn = ev.currentTarget as HTMLElement;
    setTimeout(() => this.placeTray(btn), 0);
  }
  placeTray(btn: HTMLElement) {
    const p = this.trayEl?.nativeElement,
      o = this.osEl?.nativeElement;
    if (!p || !o) return;
    const oR = o.getBoundingClientRect(),
      r = btn.getBoundingClientRect(),
      pw = p.offsetWidth;
    let left = r.right - oR.left - pw;
    if (left < 8) left = 8;
    this.tray.left = left;
    this.tray.top = r.bottom - oR.top + 6;
  }
  trayOpen(app: string, sub?: string) {
    this.closeMenus();
    this.wm.launch(app);
    if (sub) {
      const w = this.wins.find((x) => x.app === app);
      if (w) this.wm.setSub(w.id, sub);
    }
  }
  setLang(c: string) {
    this.lang = c;
    this.tray.open = false;
  }
  applyDesign() {
    const os = this.osEl?.nativeElement;
    if (!os) return;
    if (this.design === 'custom') {
      const H = this.customHue;
      const R: [string, number, number][] = [
        ['50', 0.96, 0.02],
        ['100', 0.9, 0.045],
        ['200', 0.82, 0.08],
        ['300', 0.72, 0.115],
        ['400', 0.62, 0.145],
        ['500', 0.54, 0.16],
        ['600', 0.46, 0.155],
        ['700', 0.38, 0.13],
        ['800', 0.3, 0.105],
        ['900', 0.22, 0.075],
      ];
      R.forEach(([k, l, c]) => os.style.setProperty('--brand-' + k, `oklch(${l} ${c} ${H})`));
      os.style.setProperty('--brand-name', `'${this.customName}'`);
    } else {
      ['50', '100', '200', '300', '400', '500', '600', '700', '800', '900', 'name'].forEach((k) =>
        os.style.removeProperty('--brand-' + k),
      );
    }
  }

  // ── menu bar — per-app dropdown menus (the app populates them; here mocked per focused app) ──
  about = false;
  designLabel() {
    return (this as any).design === 'custom'
      ? (this as any).customName || $localize`:Design name@@desktop.design.custom:Custom`
      : this.brand === 'hostuk'
        ? $localize`:Design name@@desktop.design.hostUk:Host UK`
        : $localize`:Design name@@desktop.design.lethean:Lethean`;
  }
  openAbout() {
    this.ctx.open = false;
    this.mbKey = '';
    this.sessionOpen = false;
    this.about = true;
  }
  mbItems(key: string): CtxItem[] {
    const f = this.wins.find((w) => w.id === this.focusId && !w.min);
    if (key === 'system')
      return [
        {
          label: $localize`:System menu item@@desktop.menu.aboutLetheanOs:About LetheanOS`,
          icon: 'circle-info',
          act: () => this.openAbout(),
        },
        {
          label: $localize`:System menu item@@desktop.menu.systemSettings:System Settings…`,
          icon: 'sliders',
          act: () => this.wm.launch('settings'),
        },
        { sep: true },
        {
          label: $localize`:System menu item@@desktop.menu.lockScreen:Lock Screen`,
          icon: 'lock',
          act: () => this.sessionAction('lock'),
        },
        {
          label: $localize`:System menu item@@desktop.menu.restart:Restart…`,
          icon: 'rotate',
          act: () => this.sessionAction('shutdown'),
        },
        {
          label: $localize`:System menu item@@desktop.menu.shutDown:Shut Down…`,
          icon: 'power-off',
          act: () => this.sessionAction('shutdown'),
        },
      ];
    if (key === 'app')
      return [
        {
          label: $localize`:Application menu item@@desktop.menu.aboutApp:About ${f ? this.APPS[f.app].title : 'Lethean'}:appTitle:`,
          icon: 'circle-info',
          act: () => this.openAbout(),
        },
        {
          label: $localize`:Application menu item@@desktop.menu.preferences:Preferences…`,
          icon: 'gear',
          act: () => {
            if (f && f.app === 'control') this.wm.setSub(f.id, 'settings');
            else this.wm.launch('settings');
          },
        },
        { sep: true },
        f
          ? {
              label: $localize`:Application menu item@@desktop.menu.closeWindow:Close Window`,
              icon: 'xmark',
              act: () => this.wm.close(f.id),
            }
          : {
              label: $localize`:Application menu item@@desktop.menu.quit:Quit`,
              icon: 'xmark',
              act: () => {},
            },
      ];
    if (key === 'File')
      return [
        {
          label: $localize`:File menu item@@desktop.menu.newWindow:New Window`,
          icon: 'plus',
          children: this.order.map((id) => ({
            label: this.APPS[id].title,
            icon: this.APPS[id].icon,
            act: () => this.wm.launch(id),
          })),
        },
        { sep: true },
        {
          label: $localize`:File menu item@@desktop.menu.closeWindow:Close Window`,
          icon: 'xmark',
          act: () => {
            if (f) this.wm.close(f.id);
          },
        },
      ];
    if (key === 'Edit')
      return [
        {
          label: $localize`:Edit menu item@@desktop.menu.undo:Undo`,
          icon: 'rotate-left',
          act: () => {},
        },
        {
          label: $localize`:Edit menu item@@desktop.menu.redo:Redo`,
          icon: 'rotate-right',
          act: () => {},
        },
        { sep: true },
        {
          label: $localize`:Edit menu item@@desktop.menu.cut:Cut`,
          icon: 'scissors',
          act: () => {},
        },
        {
          label: $localize`:Edit menu item@@desktop.menu.copy:Copy`,
          icon: 'copy',
          act: () => {},
        },
        {
          label: $localize`:Edit menu item@@desktop.menu.paste:Paste`,
          icon: 'clipboard',
          act: () => {},
        },
      ];
    if (key === 'View') {
      if (f && f.app === 'control')
        return this.submenuFor('control').map(([k, i, l]) => ({
          label: l,
          icon: i,
          act: () => this.wm.setSub(f.id, k),
        }));
      return [
        {
          label: $localize`:View menu item@@desktop.menu.zoom:Zoom`,
          icon: 'expand',
          act: () => {
            if (f) this.wm.maximise(f.id, this.windowBounds());
          },
        },
        { sep: true },
        {
          label: this.showWidgets
            ? $localize`:View menu item@@desktop.menu.hideWidgets:Hide Widgets`
            : $localize`:View menu item@@desktop.menu.showWidgets:Show Widgets`,
          icon: 'table-cells',
          act: () => (this.showWidgets = !this.showWidgets),
        },
        {
          label: this.showIcons
            ? $localize`:View menu item@@desktop.menu.hideDesktopIcons:Hide Desktop Icons`
            : $localize`:View menu item@@desktop.menu.showDesktopIcons:Show Desktop Icons`,
          icon: 'icons',
          act: () => (this.showIcons = !this.showIcons),
        },
      ];
    }
    const ws = this.wins.filter((w) => !w.group);
    return (
      f
        ? ([
            {
              label: $localize`:Window menu item@@desktop.menu.minimise:Minimise`,
              icon: 'window-minimize',
              act: () => this.wm.minimise(f.id, this.reduceMotion ? 0 : 190),
            },
            {
              label: $localize`:Window menu item@@desktop.menu.zoom:Zoom`,
              icon: 'expand',
              act: () => this.wm.maximise(f.id, this.windowBounds()),
            },
            { sep: true },
          ] as CtxItem[])
        : []
    ).concat(
      ws.length
        ? ws.map((w) => ({
            label: this.APPS[w.app].title,
            icon: this.APPS[w.app].icon,
            act: () => this.wm.focus(w.id),
          }))
        : [
            {
              label: $localize`:Window menu empty state@@desktop.menu.noOpenWindows:No open windows`,
              act: () => {},
            },
          ],
    );
  }
  openMb(key: string, ev: Event) {
    ev.stopPropagation();
    this.sessionOpen = false;
    this.tray.open = false;
    this.sub.open = false;
    this.csub.open = false;
    if (this.mbKey === key && this.ctx.open) {
      this.ctx.open = false;
      this.mbKey = '';
      return;
    }
    this.mbKey = key;
    this.ctx.title = '';
    this.ctx.items = this.mbItems(key);
    this.ctx.open = true;
    const el = ev.currentTarget as HTMLElement;
    setTimeout(() => this.placeMb(el), 0);
  }
  placeMb(el: HTMLElement) {
    const m = this.ctxMenuEl?.nativeElement,
      o = this.osEl?.nativeElement;
    if (!m || !o) return;
    const oR = o.getBoundingClientRect(),
      r = el.getBoundingClientRect();
    let left = r.left - oR.left,
      mw = m.offsetWidth;
    if (left + mw > oR.width - 8) left = oR.width - mw - 8;
    this.ctx.left = Math.max(4, left);
    this.ctx.top = r.bottom - oR.top + 2;
  }
  hoverMb(key: string, ev: Event) {
    if (this.ctx.open && this.mbKey && this.mbKey !== key) this.openMb(key, ev);
  }

  // ── command palette — tap Cmd / Windows key to toggle ──
  commands(): any[] {
    const f = this.wins.find((w) => w.id === this.focusId && !w.min);
    const c: any[] = [];
    this.order.forEach((id) =>
      c.push({
        icon: this.APPS[id].icon,
        label: $localize`:Command palette action@@desktop.command.openApp:Open ${this.APPS[id].title}:appTitle:`,
        sec: $localize`:Command palette section@@desktop.command.section.app:App`,
        run: () => this.wm.launch(id),
      }),
    );
    c.push(
      {
        icon: 'moon',
        label: $localize`:Command palette action@@desktop.command.themeDark:Theme: Dark`,
        sec: $localize`:Command palette section@@desktop.command.section.theme:Theme`,
        run: () => this.setMode('dark'),
      },
      {
        icon: 'sun',
        label: $localize`:Command palette action@@desktop.command.themeLight:Theme: Light`,
        sec: $localize`:Command palette section@@desktop.command.section.theme:Theme`,
        run: () => this.setMode('light'),
      },
    );
    c.push(
      {
        icon: 'palette',
        label: $localize`:Command palette action@@desktop.command.brandLethean:Brand: Lethean`,
        sec: $localize`:Command palette section@@desktop.command.section.theme:Theme`,
        run: () => this.setBrand('lethean'),
      },
      {
        icon: 'palette',
        label: $localize`:Command palette action@@desktop.command.brandHostUk:Brand: Host UK`,
        sec: $localize`:Command palette section@@desktop.command.section.theme:Theme`,
        run: () => this.setBrand('hostuk'),
      },
    );
    (['top', 'right', 'bottom', 'left'] as const).forEach((bb) =>
      c.push({
        icon: 'table-columns',
        label: $localize`:Command palette action@@desktop.command.taskbarEdge:Taskbar: ${this.edgeLabel(bb)}:edge:`,
        sec: $localize`:Command palette section@@desktop.command.section.desktop:Desktop`,
        run: () => this.setBar(bb),
      }),
    );
    c.push({
      icon: 'table-cells',
      label: this.showWidgets
        ? $localize`:Command palette action@@desktop.command.hideWidgets:Hide widgets`
        : $localize`:Command palette action@@desktop.command.showWidgets:Show widgets`,
      sec: $localize`:Command palette section@@desktop.command.section.desktop:Desktop`,
      run: () => (this.showWidgets = !this.showWidgets),
    });
    c.push({
      icon: 'icons',
      label: this.showIcons
        ? $localize`:Command palette action@@desktop.command.hideDesktopIcons:Hide desktop icons`
        : $localize`:Command palette action@@desktop.command.showDesktopIcons:Show desktop icons`,
      sec: $localize`:Command palette section@@desktop.command.section.desktop:Desktop`,
      run: () => (this.showIcons = !this.showIcons),
    });
    c.push({
      icon: 'gauge',
      label: this.reduceMotion
        ? $localize`:Command palette action@@desktop.command.disableReduceMotion:Disable reduce motion`
        : $localize`:Command palette action@@desktop.command.enableReduceMotion:Enable reduce motion`,
      sec: $localize`:Command palette section@@desktop.command.section.desktop:Desktop`,
      run: () => (this.reduceMotion = !this.reduceMotion),
    });
    if (f)
      c.push(
        {
          icon: 'window-minimize',
          label: $localize`:Command palette action@@desktop.command.minimiseWindow:Minimise window`,
          sec: $localize`:Command palette section@@desktop.command.section.window:Window`,
          run: () => this.wm.minimise(f.id, this.reduceMotion ? 0 : 190),
        },
        {
          icon: 'expand',
          label: $localize`:Command palette action@@desktop.command.zoomWindow:Zoom window`,
          sec: $localize`:Command palette section@@desktop.command.section.window:Window`,
          run: () => this.wm.maximise(f.id, this.windowBounds()),
        },
        {
          icon: 'xmark',
          label: $localize`:Command palette action@@desktop.command.closeWindow:Close window`,
          sec: $localize`:Command palette section@@desktop.command.section.window:Window`,
          run: () => this.wm.close(f.id),
        },
      );
    this.wins
      .filter((w) => !w.group)
      .forEach((w) =>
        c.push({
          icon: this.APPS[w.app].icon,
          label: $localize`:Command palette action@@desktop.command.switchToApp:Switch to ${this.APPS[w.app].title}:appTitle:`,
          sec: $localize`:Command palette section@@desktop.command.section.window:Window`,
          run: () => this.wm.focus(w.id),
        }),
      );
    c.push(
      {
        icon: 'lock',
        label: $localize`:Command palette action@@desktop.command.lockScreen:Lock screen`,
        sec: $localize`:Command palette section@@desktop.command.section.session:Session`,
        run: () => this.sessionAction('lock'),
      },
      {
        icon: 'right-from-bracket',
        label: $localize`:Command palette action@@desktop.command.logOut:Log out`,
        sec: $localize`:Command palette section@@desktop.command.section.session:Session`,
        run: () => this.sessionAction('logout'),
      },
      {
        icon: 'power-off',
        label: $localize`:Command palette action@@desktop.command.shutDown:Shut down`,
        sec: $localize`:Command palette section@@desktop.command.section.session:Session`,
        run: () => this.sessionAction('shutdown'),
      },
    );
    return c;
  }
  filterCmds() {
    const q = this.palette.q.toLowerCase();
    this.filtered = this.commands().filter((c) =>
      (c.label + ' ' + c.sec).toLowerCase().includes(q),
    );
    if (this.palette.i >= this.filtered.length)
      this.palette.i = Math.max(0, this.filtered.length - 1);
  }
  openPalette() {
    if (this.session !== 'active') return;
    this.closeMenus();
    this.paletteLastFocus = document.activeElement as HTMLElement;
    this.palette.open = true;
    this.palette.q = '';
    this.palette.i = 0;
    this.filterCmds();
    setTimeout(() => this.plInput?.nativeElement.focus(), 0);
  }
  closePalette() {
    this.palette.open = false;
    if (this.paletteLastFocus) {
      try {
        this.paletteLastFocus.focus();
      } catch (_) {}
    }
  }
  togglePalette() {
    if (this.palette.open) this.closePalette();
    else this.openPalette();
  }
  runCmd(i: number) {
    const c = this.filtered[i];
    this.closePalette();
    if (c && c.run) c.run();
  }
  onPlInput(e: Event) {
    this.palette.q = (e.target as HTMLInputElement).value;
    this.palette.i = 0;
    this.filterCmds();
  }
  plKey(e: KeyboardEvent) {
    if (e.key === 'Tab') {
      e.preventDefault();
      return;
    }
    if (e.key === 'ArrowDown') {
      e.preventDefault();
      this.palette.i = Math.min(this.palette.i + 1, this.filtered.length - 1);
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      this.palette.i = Math.max(this.palette.i - 1, 0);
    } else if (e.key === 'Enter') {
      e.preventDefault();
      this.runCmd(this.palette.i);
    } else if (e.key === 'Escape') {
      e.preventDefault();
      this.closePalette();
    }
  }
  palClick(e: Event) {
    if ((e.target as HTMLElement).classList.contains('paletteov')) this.closePalette();
  }
  trackWin(i: number, w: Win) {
    return w.id;
  }
  windowBounds() {
    const bounds = this.winlayerEl?.nativeElement.getBoundingClientRect();
    return { w: bounds?.width ?? 900, h: bounds?.height ?? 600 };
  }
  // ── app-shell / device top-level nav — unified st.wins model (categories launch/focus windows) ──
  shellCat(c: DesktopMenuCategory, ev: Event) {
    ev.stopPropagation();
    this.sessionOpen = false;
    this.ctx.title = c.title;
    this.ctx.items = c.apps.map((app) => ({
      label: app.title,
      icon: app.icon,
      ...(app.children.length
        ? {
            children: app.children.map(([sub, icon, title]) => ({
              label: title,
              icon,
              act: () => this.startSub(app.id, sub),
            })),
          }
        : { act: () => this.wm.launch(app.id) }),
    }));
    this.ctx.open = true;
    const o = this.osEl.nativeElement.getBoundingClientRect();
    const r = (ev.currentTarget as HTMLElement).getBoundingClientRect();
    this.ctx.left = r.left - o.left;
    this.ctx.top = r.bottom - o.top + 4;
    setTimeout(() => this.clampCtx(), 0);
  }
  taskClick(id: string) {
    const w = this.wins.find((x) => x.id === id);
    if (w?.min || this.focusId !== id) this.wm.focus(id);
    else this.wm.minimise(id, this.reduceMotion ? 0 : 190);
  }

  private selectWindowFromRoute(): void {
    const target = desktopTargetFromSnapshot(this.router.routerState.snapshot.root);
    if (!target || !this.routeCatalog.apps[target.app]) return;

    let win = this.wins.find((candidate) => candidate.app === target.app);
    if (!win) {
      this.wm.launch(target.app);
      win = this.wins.find((candidate) => candidate.app === target.app);
    } else if (win.min || this.focusId !== win.id) {
      this.wm.focus(win.id);
      win = this.wins.find((candidate) => candidate.id === win?.id) ?? win;
    }
    if (win && target.sub && win.sub !== target.sub) {
      this.wm.setSub(win.id, target.sub);
    }
  }

  private reflectFocusedWindowInRoute(): void {
    const focusId = this.wm.focusId();
    const wins = this.wm.wins();
    if (!this.router.navigated) return;

    const focused = wins.find((candidate) => candidate.id === focusId && !candidate.min);
    const currentTarget = desktopTargetFromSnapshot(this.router.routerState.snapshot.root);
    if (!focused) {
      if (currentTarget) void this.router.navigateByUrl('/');
      return;
    }

    const segments = routeSegmentsForWindow(this.routeCatalog, focused.app, focused.sub);
    if (!segments.length) return;
    const targetUrl = `/${segments.join('/')}`;
    if (this.router.url !== targetUrl) {
      void this.router.navigateByUrl(targetUrl);
    }
  }

  // ── session chip menu (the taskbar's fresh login/session control) ──
  toggleSession(ev: Event) {
    ev.stopPropagation();
    this.ctx.open = false;
    if (this.sessionOpen) {
      this.sessionOpen = false;
      return;
    }
    this.sessionOpen = true;
    const btn = ev.currentTarget as HTMLElement;
    setTimeout(() => this.placeSession(btn), 0);
  }
  placeSession(btn: HTMLElement) {
    const m = this.sessionMenuEl?.nativeElement,
      o = this.osEl?.nativeElement;
    if (!m || !o) return;
    const oR = o.getBoundingClientRect(),
      r = btn.getBoundingClientRect(),
      mw = m.offsetWidth,
      mh = m.offsetHeight;
    let left: number,
      top: number | null = null,
      bottom: number | null = null;
    if (this.bar === 'bottom') {
      left = r.left - oR.left;
      bottom = oR.height - (r.top - oR.top) + 8;
    } else if (this.bar === 'top') {
      left = r.left - oR.left;
      top = r.bottom - oR.top + 8;
    } else if (this.bar === 'left') {
      left = r.right - oR.left + 8;
      top = Math.max(8, r.top - oR.top);
    } else {
      left = r.left - oR.left - mw - 8;
      top = Math.max(8, r.top - oR.top);
    }
    this.sessionPos = { left: Math.max(8, Math.min(left, oR.width - mw - 8)), top, bottom };
  }

  // ── dock right-click → window actions ──
  winItems(w: Win): CtxItem[] {
    return [
      {
        label: $localize`:Window context action@@desktop.context.bringToFront:Bring to front`,
        icon: 'arrow-up',
        act: () => this.wm.focus(w.id),
      },
      {
        label: w.min
          ? $localize`:Window context action@@desktop.context.restore:Restore`
          : $localize`:Window context action@@desktop.context.minimise:Minimise`,
        icon: 'window-minimize',
        act: () => {
          if (w.min) this.wm.focus(w.id);
          else this.wm.minimise(w.id, this.reduceMotion ? 0 : 190);
        },
      },
      {
        label: w.max
          ? $localize`:Window context action@@desktop.context.restoreSize:Restore size`
          : $localize`:Window context action@@desktop.context.zoom:Zoom`,
        icon: 'expand',
        act: () => this.wm.maximise(w.id, this.windowBounds()),
      },
      { sep: true },
      {
        label: $localize`:Window context action@@desktop.context.closeWindow:Close window`,
        icon: 'xmark',
        act: () => this.wm.close(w.id),
      },
    ];
  }
  desktopItems(): CtxItem[] {
    return [
      {
        label: $localize`:Desktop context action@@desktop.context.newWindow:New window`,
        icon: 'plus',
        children: this.order.map((id) => ({
          label: this.APPS[id].title,
          icon: this.APPS[id].icon,
          act: () => this.wm.launch(id),
        })),
      },
      {
        label: $localize`:Desktop context submenu@@desktop.context.appearance:Appearance`,
        icon: 'palette',
        children: [
          {
            label: $localize`:Theme option@@desktop.context.dark:Dark`,
            icon: 'moon',
            act: () => this.setMode('dark'),
          },
          {
            label: $localize`:Theme option@@desktop.context.light:Light`,
            icon: 'sun',
            act: () => this.setMode('light'),
          },
        ],
      },
      {
        label: $localize`:Desktop context submenu@@desktop.context.taskbarEdge:Taskbar edge`,
        icon: 'table-columns',
        children: [
          { label: this.edgeLabel('top'), act: () => this.setBar('top') },
          { label: this.edgeLabel('right'), act: () => this.setBar('right') },
          { label: this.edgeLabel('bottom'), act: () => this.setBar('bottom') },
          { label: this.edgeLabel('left'), act: () => this.setBar('left') },
        ],
      },
      { sep: true },
      {
        label: this.showIcons
          ? $localize`:Desktop context action@@desktop.context.hideDesktopIcons:Hide desktop icons`
          : $localize`:Desktop context action@@desktop.context.showDesktopIcons:Show desktop icons`,
        icon: 'icons',
        act: () => (this.showIcons = !this.showIcons),
      },
      {
        label: this.showWidgets
          ? $localize`:Desktop context action@@desktop.context.hideWidgets:Hide widgets`
          : $localize`:Desktop context action@@desktop.context.showWidgets:Show widgets`,
        icon: 'table-cells',
        act: () => (this.showWidgets = !this.showWidgets),
      },
      { sep: true },
      {
        label: $localize`:Desktop context action@@desktop.context.reboot:Reboot`,
        icon: 'power-off',
        act: () => this.sessionAction('shutdown'),
      },
    ];
  }
  openCtxAt(ev: MouseEvent) {
    this.ctx.open = true;
    const o = this.osEl.nativeElement.getBoundingClientRect();
    this.ctx.left = ev.clientX - o.left;
    this.ctx.top = ev.clientY - o.top;
    setTimeout(() => this.clampCtx(), 0);
  }
  dockCtx(ev: MouseEvent, app: string) {
    ev.preventDefault();
    ev.stopPropagation();
    this.closeMenus();
    const w = this.wins.find((x) => x.app === app);
    this.ctx.title = this.APPS[app].title;
    this.ctx.items = w
      ? this.winItems(w)
      : [
          {
            label: $localize`:Application context action@@desktop.context.open:Open`,
            icon: 'arrow-up-right-from-square',
            act: () => this.wm.launch(app),
          },
        ];
    this.openCtxAt(ev);
  }
  winCtx(ev: MouseEvent, w: Win) {
    ev.preventDefault();
    ev.stopPropagation();
    this.closeMenus();
    this.ctx.title = this.APPS[w.app].title;
    this.ctx.items = this.winItems(w);
    this.openCtxAt(ev);
  }
  iconCtx(ev: MouseEvent, id: string) {
    ev.preventDefault();
    ev.stopPropagation();
    this.closeMenus();
    this.ctx.title = this.APPS[id].title;
    this.ctx.items = [
      {
        label: $localize`:Application context action@@desktop.context.open:Open`,
        icon: 'arrow-up-right-from-square',
        act: () => this.wm.launch(id),
      },
    ];
    this.openCtxAt(ev);
  }
  deskCtx(ev: MouseEvent) {
    if (!this.wm.windowed()) return;
    ev.preventDefault();
    this.closeMenus();
    this.ctx.title = '';
    this.ctx.items = this.desktopItems();
    this.openCtxAt(ev);
  }
  openCtxSub(i: number, ev: Event) {
    const row = ev.currentTarget as HTMLElement;
    this.csub.i = i;
    this.csub.open = true;
    setTimeout(() => this.placeCtxSub(row), 0);
  }
  placeCtxSub(rowEl: HTMLElement) {
    const m = this.ctxMenuEl?.nativeElement,
      o = this.osEl?.nativeElement;
    if (!m || !o) return;
    const mR = m.getBoundingClientRect(),
      rR = rowEl.getBoundingClientRect(),
      oR = o.getBoundingClientRect();
    let left = m.offsetWidth - 2;
    if (mR.left + left + 180 > oR.right) left = -(180 - 2);
    this.csub.left = left;
    this.csub.top = Math.max(0, rR.top - mR.top);
  }
  clampCtx() {
    const m = this.ctxMenuEl?.nativeElement,
      o = this.osEl?.nativeElement;
    if (!m || !o) return;
    const oR = o.getBoundingClientRect(),
      mw = m.offsetWidth,
      mh = m.offsetHeight;
    let left = this.ctx.left,
      top = this.ctx.top;
    if (left + mw > oR.width - 8) left = oR.width - mw - 8;
    if (top + mh > oR.height - 8) top = oR.height - mh - 8;
    this.ctx.left = Math.max(8, left);
    this.ctx.top = Math.max(8, top);
  }
  runCtx(it: CtxItem, ev: Event) {
    ev.stopPropagation();
    this.ctx.open = false;
    this.csub.open = false;
    this.mbKey = '';
    if (it.act) it.act();
  }
  closeMenus(ev?: Event) {
    ev?.stopPropagation();
    this.sessionOpen = false;
    this.ctx.open = false;
    this.csub.open = false;
    this.sub.open = false;
    this.tray.open = false;
    this.mbKey = '';
  }

  // ── session actions — real in-OS screens overlaying the desktop ──
  today() {
    if (!this.timeReady) return '';
    return new Date().toLocaleDateString('en-GB', {
      weekday: 'long',
      day: 'numeric',
      month: 'long',
    });
  }
  sessionAction(kind: string, ev?: Event) {
    ev?.stopPropagation();
    this.closeMenus();
    if (kind === 'lock') this.session = 'locked';
    else if (kind === 'switch') this.session = 'switch';
    else if (kind === 'logout') {
      this.wm.clear();
      this.session = 'login';
    } else if (kind === 'shutdown') {
      this.wm.clear();
      this.session = 'shutting';
      setTimeout(() => (this.session = 'off'), 1300);
    }
  }
  resume(u?: any) {
    if (u) this.user = u;
    this.session = 'active';
    if (!this.wins.length) {
      this.wm.launch('control');
      this.wm.launch('telemetry');
    }
  }

  @HostListener('document:click', ['$event'])
  onDocClick(e: MouseEvent) {
    if (!(e.target as HTMLElement).closest('.menu, .session, .di, .trayi, .mbtn')) {
      this.sessionOpen = false;
      this.ctx.open = false;
      this.csub.open = false;
      this.sub.open = false;
      this.tray.open = false;
      this.mbKey = '';
    }
  }
  @HostListener('document:pointerup')
  onUp() {
    this.persist();
  }
  @HostListener('document:keydown.escape')
  onEsc() {
    this.sessionOpen = false;
    this.ctx.open = false;
    this.csub.open = false;
    this.sub.open = false;
    this.tray.open = false;
    this.mbKey = '';
  }
  @HostListener('document:keydown', ['$event'])
  startNav(e: KeyboardEvent) {
    if (!this.sessionOpen) return;
    if (!['ArrowDown', 'ArrowUp', 'Enter', ' ', 'ArrowRight', 'ArrowLeft'].includes(e.key)) return;
    const host = this.osEl?.nativeElement;
    if (!host) return;
    const rows = Array.from(host.querySelectorAll('.sp-apps .mi')) as HTMLElement[];
    if (!rows.length) return;
    rows.forEach((r) => {
      if (!r.hasAttribute('tabindex')) r.tabIndex = -1;
    });
    const i = rows.indexOf(document.activeElement as HTMLElement);
    const isCat = (r: HTMLElement) => r.classList.contains('sm-cat');
    if (e.key === 'ArrowDown') {
      e.preventDefault();
      rows[(i + 1) % rows.length].focus();
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      rows[i <= 0 ? rows.length - 1 : i - 1].focus();
    } else if (e.key === 'ArrowRight') {
      const r = rows[i];
      if (r && isCat(r) && r.getAttribute('aria-expanded') === 'false') {
        e.preventDefault();
        r.click();
      }
    } else if (e.key === 'ArrowLeft') {
      const r = rows[i];
      if (r && isCat(r) && r.getAttribute('aria-expanded') === 'true') {
        e.preventDefault();
        r.click();
      }
    } else if ((e.key === 'Enter' || e.key === ' ') && i >= 0) {
      e.preventDefault();
      rows[i].click();
      setTimeout(() => {
        const r2 = Array.from(host.querySelectorAll('.sp-apps .mi')) as HTMLElement[];
        r2.forEach((r) => (r.tabIndex = -1));
        (r2[i] || r2[0])?.focus();
      }, 0);
    }
  }
  @HostListener('document:keydown', ['$event'])
  onKeyDown(e: KeyboardEvent) {
    if (e.key === 'Meta' || e.key === 'OS') {
      if (!this.metaDown) {
        this.metaDown = true;
        this.metaCombo = false;
      }
      return;
    }
    if (this.metaDown) this.metaCombo = true;
    if ((e.metaKey || e.ctrlKey) && this.session === 'active') {
      const f = this.wins.find((w) => w.id === this.focusId && !w.min);
      const k = e.key.toLowerCase();
      if (k === 'w' && f) {
        e.preventDefault();
        this.wm.close(f.id);
        return;
      }
      if (k === 'm' && f) {
        e.preventDefault();
        this.wm.minimise(f.id, this.reduceMotion ? 0 : 190);
        return;
      }
      if (e.key === '`') {
        e.preventDefault();
        this.cycleWin();
        return;
      }
      if (f && e.key.indexOf('Arrow') === 0) {
        e.preventDefault();
        this.kbSnap(
          f,
          ({ ArrowLeft: 'left', ArrowRight: 'right', ArrowUp: 'up', ArrowDown: 'down' } as any)[
            e.key
          ],
        );
        return;
      }
    }
    if (e.key === 'Escape') this.closePalette();
  }
  cycleWin() {
    const v = this.wins.filter((w) => !w.min && !w.group);
    if (!v.length) return;
    const i = v.findIndex((w) => w.id === this.focusId);
    this.wm.focus(v[(i + 1) % v.length].id);
  }
  @HostListener('document:keyup', ['$event'])
  onKeyUp(e: KeyboardEvent) {
    if (e.key === 'Meta' || e.key === 'OS') {
      if (this.metaDown && !this.metaCombo) this.togglePalette();
      this.metaDown = false;
    }
  }

  snapRect(s: string, R: DOMRect) {
    const hw = R.width / 2,
      hh = R.height / 2;
    switch (s) {
      case 'top':
      case 'max':
        return { x: 0, y: 0, w: R.width, h: R.height, max: true };
      case 'left':
        return { x: 0, y: 0, w: hw, h: R.height, max: false };
      case 'right':
        return { x: hw, y: 0, w: hw, h: R.height, max: false };
      case 'tl':
        return { x: 0, y: 0, w: hw, h: hh, max: false };
      case 'tr':
        return { x: hw, y: 0, w: hw, h: hh, max: false };
      case 'bl':
        return { x: 0, y: hh, w: hw, h: hh, max: false };
      case 'br':
        return { x: hw, y: hh, w: hw, h: hh, max: false };
      default:
        return { x: 0, y: 0, w: R.width, h: R.height, max: true };
    }
  }
  snapWin(w: Win, s: string) {
    const r = this.snapRect(s, this.winlayerEl.nativeElement.getBoundingClientRect());
    this.wm.wins.update((ws) =>
      ws.map((win) => {
        if (win.id !== w.id) return win;
        const prev = (win as any).snapState ? win.prev : { x: win.x, y: win.y, w: win.w, h: win.h };
        return { ...win, ...r, prev, snapState: s };
      }),
    );
    this.persist();
  }
  unsnap(w: Win) {
    this.wm.wins.update((ws) =>
      ws.map((win) =>
        win.id === w.id ? { ...win, ...(win.prev ?? {}), max: false, snapState: null } : win,
      ),
    );
    this.persist();
  }
  kbSnap(w: Win, dir: string) {
    const KB: any = {
      left: {
        null: 'left',
        right: null,
        left: 'left',
        tr: 'tl',
        br: 'bl',
        tl: 'tl',
        bl: 'bl',
        max: 'left',
      },
      right: {
        null: 'right',
        left: null,
        right: 'right',
        tl: 'tr',
        bl: 'br',
        tr: 'tr',
        br: 'br',
        max: 'right',
      },
      up: {
        null: 'max',
        left: 'tl',
        right: 'tr',
        bl: 'left',
        br: 'right',
        tl: 'tl',
        tr: 'tr',
        max: 'max',
      },
      down: {
        null: '__min',
        left: null,
        right: null,
        tl: 'left',
        tr: 'right',
        bl: null,
        br: null,
        max: null,
      },
    };
    const s = (w as any).snapState || 'null';
    const t = KB[dir][s];
    if (t === '__min') {
      this.wm.minimise(w.id, this.reduceMotion ? 0 : 190);
      return;
    }
    if (t === null || t === undefined) {
      this.unsnap(w);
      return;
    }
    this.snapWin(w, t);
  }

  startDrag(ev: PointerEvent, w: Win) {
    if (!this.wm.windowed() || w.max) return;
    if ((ev.target as HTMLElement).closest('.lights i')) return; // don't drag from a window control
    if (this.selected.length > 1 && this.selected.includes(w.id)) {
      this.groupDrag(ev);
      return;
    }
    if (this.selected.length && !this.selected.includes(w.id)) this.selected = [];
    const R = this.winlayerEl.nativeElement.getBoundingClientRect(),
      oR = this.osEl.nativeElement.getBoundingClientRect();
    const ox = ev.clientX - (R.left + w.x),
      oy = ev.clientY - (R.top + w.y);
    const rx = R.left - oR.left,
      ry = R.top - oR.top,
      t = 22;
    const mv = (e: PointerEvent) => {
      this.wm.move(
        w.id,
        Math.max(0, Math.min(e.clientX - R.left - ox, R.width - 60)),
        Math.max(0, Math.min(e.clientY - R.top - oy, R.height - 40)),
      );
      const current = this.wins.find((win) => win.id === w.id);
      if (current) (current as any).snapState = null;
      const nx = e.clientX - R.left,
        ny = e.clientY - R.top,
        nL = nx < t,
        nR = R.width - nx < t,
        nT = ny < t,
        nB = R.height - ny < t;
      const z =
        nT && nL
          ? 'tl'
          : nT && nR
            ? 'tr'
            : nB && nL
              ? 'bl'
              : nB && nR
                ? 'br'
                : nT
                  ? 'top'
                  : nL
                    ? 'left'
                    : nR
                      ? 'right'
                      : null;
      this.snap.zone = z;
      if (z) {
        const s = this.snapRect(z, R);
        this.snap.left = rx + s.x;
        this.snap.top = ry + s.y;
        this.snap.w = s.w;
        this.snap.h = s.h;
      }
    };
    const up = () => {
      window.removeEventListener('pointermove', mv);
      window.removeEventListener('pointerup', up);
      const z = this.snap.zone;
      this.snap.zone = null;
      const current = this.wins.find((win) => win.id === w.id);
      if (z && current) {
        const s = this.snapRect(z, R);
        this.wm.wins.update((ws) =>
          ws.map((win) =>
            win.id === w.id
              ? {
                  ...win,
                  ...s,
                  prev: { x: current.x, y: current.y, w: current.w, h: current.h },
                  snapState: z,
                }
              : win,
          ),
        );
      }
      this.persist();
    };
    window.addEventListener('pointermove', mv);
    window.addEventListener('pointerup', up);
  }
  startResize(ev: PointerEvent, w: Win) {
    if (!this.wm.windowed()) return;
    ev.preventDefault();
    ev.stopPropagation();
    const sx = ev.clientX,
      sy = ev.clientY,
      sw = w.w,
      sh = w.h;
    const mv = (e: PointerEvent) =>
      this.wm.resize(w.id, Math.max(360, sw + e.clientX - sx), Math.max(260, sh + e.clientY - sy));
    const up = () => {
      window.removeEventListener('pointermove', mv);
      window.removeEventListener('pointerup', up);
      this.persist();
    };
    window.addEventListener('pointermove', mv);
    window.addEventListener('pointerup', up);
  }
}

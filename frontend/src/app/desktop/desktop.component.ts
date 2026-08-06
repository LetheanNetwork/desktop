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
import { APPS, ORDER } from './desktop-catalogue.data';
import { CLOCKS, PKGS } from './desktop-shell-fixtures.data';
import { devPanelEmptyFor, devPanelFor, type DevPanelView } from './dev-panel.data';
import { LANGS, TELEMETRY, Win, WindowSnapState } from './desktop.data';
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
import { DesktopAppWindowBridgeService } from './desktop-app-window-bridge.service';
import { WindowInteractionService } from './window-interaction.service';
import type { KeyboardSnapDirection, WindowGroupingState } from './window-interaction.service';
import { ShellMenuBar } from './shell/menu-bar';
import { ShellTaskbarDock } from './shell/taskbar-dock';
import { ShellStartMenu } from './shell/start-menu';
import { ShellContextMenu } from './shell/context-menu';
import { ShellTrayPanel } from './shell/tray-panel';
import { ShellNotificationStack } from './shell/notification-stack';
import { ShellCommandPalette } from './shell/command-palette';
import type {
  ShellCommand,
  ShellContextItem,
  ShellContextMenuState,
  ShellContextSubmenuState,
  ShellNotification,
  ShellPaletteState,
  ShellPosition,
  ShellSessionAction,
  ShellStartSubmenuState,
  ShellTrayKey,
  ShellTrayState,
  ShellUserIdentity,
  ShellWorldClock,
  ShellWindowGroup,
} from './shell/shell.types';

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
  imports: [
    CommonModule,
    WindowRouteContent,
    ShellMenuBar,
    ShellTaskbarDock,
    ShellStartMenu,
    ShellContextMenu,
    ShellTrayPanel,
    ShellNotificationStack,
    ShellCommandPalette,
  ],
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
  readonly appWindows = inject(DesktopAppWindowBridgeService);
  private readonly windowInteractions = inject(WindowInteractionService);
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
  sessionPos: ShellPosition = {
    left: 0,
    top: 0,
    bottom: null,
  };
  ctx: ShellContextMenuState = {
    open: false,
    left: 0,
    top: 0,
    title: '',
    items: [],
  };
  csub: ShellContextSubmenuState = { open: false, left: 0, top: 0, index: -1 };
  snap = { zone: null as WindowSnapState | null, left: 0, top: 0, w: 0, h: 0 };
  tear = { active: false, left: 0, top: 0, w: 0, h: 0 };
  selected: string[] = [];
  groups: ShellWindowGroup[] = [];
  shellTabs: any[] = [];
  marquee = { open: false, left: 0, top: 0, w: 0, h: 0 };
  proxy = { open: false, left: 0, top: 0, n: 0, over: false };
  tray: ShellTrayState = { open: false, key: '', left: 0, top: 0 };
  mbKey = '';
  readonly langs = LANGS;
  palette: ShellPaletteState = { open: false, query: '', index: 0 };
  paletteLastFocus: HTMLElement | null = null;
  filtered: ShellCommand[] = [];
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
  @ViewChild(ShellStartMenu) startMenu!: ShellStartMenu;
  @ViewChild(ShellContextMenu) contextMenu!: ShellContextMenu;
  @ViewChild(ShellTrayPanel) trayPanel!: ShellTrayPanel;
  @ViewChild(ShellCommandPalette) commandPalette!: ShellCommandPalette;

  openCats: Record<string, boolean> = {};
  sub: ShellStartSubmenuState = { open: false, left: 0, top: 0, parent: '' };
  readonly throughput = TELEMETRY.throughput;
  readonly watts = TELEMETRY.watts;
  readonly clocks = CLOCKS;
  get worldClocks(): ShellWorldClock[] {
    return this.clocks.map(({ city, tz }) => ({ city, time: this.fmtTz(tz) }));
  }
  readonly pkgs = PKGS;

  get throughputJson() {
    return JSON.stringify(this.throughput);
  }
  get wattsJson() {
    return JSON.stringify(this.watts);
  }

  notifs: ShellNotification[] = [];
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
    effect(() => {
      this.groups = reconcileShellGroups(this.groups, this.wm.wins());
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
  panelFor(w: Win): DevPanelView {
    const route = APPS[w.app]?.route ?? '';
    return devPanelFor(route);
  }
  emptyFor(w: Win): [string, string, string] | null {
    return devPanelEmptyFor(APPS[w.app]?.route ?? '');
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
    if (this.prefs.offline()) {
      try {
        this.storage.setItem(
          'lthn.desktop',
          JSON.stringify({
            openCats: this.openCats,
            groups: this.groups,
            shellTabs: this.shellTabs,
          }),
        );
      } catch {}
    }
    this.wm.persist();
  }
  restore() {
    if (!this.prefs.offline()) return;
    try {
      const s = JSON.parse(this.storage.getItem('lthn.desktop') || 'null');
      if (!s) return;
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
  toggleStartCategory(id: string) {
    this.openCats = { ...this.openCats, [id]: !this.openCats[id] };
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
    const sm = this.startMenu?.panelElement,
      o = this.osEl?.nativeElement;
    if (!sm || !o) return;
    const smR = sm.getBoundingClientRect(),
      rR = rowEl.getBoundingClientRect(),
      oR = o.getBoundingClientRect();
    let left = rR.right - smR.left - 6;
    if (smR.left + left + 184 > oR.right) left = rR.left - smR.left - 184 + 6;
    this.sub.left = left;
    this.sub.top = Math.max(0, rR.top - smR.top);
    this.changeDetector.markForCheck(); // runs from a timeout
  }
  startSub(appId: string, subId: string) {
    this.closeMenus();
    this.sub.open = false;
    this.wm.launch(appId);
    const w = this.wins.find((x) => x.app === appId);
    if (w) this.wm.setSub(w.id, subId);
  }

  // ── marquee multi-select + drag-to-dock grouping (click-drag UX) ──
  startMarquee(ev: PointerEvent) {
    if (!this.wm.windowed() || this.session !== 'active' || ev.button !== 0) return;
    if ((ev.target as HTMLElement).closest('.win, .bar, .dock, .menubar, .menu, .dicon, .widget'))
      return;
    const host = this.osEl.nativeElement.getBoundingClientRect();
    const windowLayer = this.winlayerEl.nativeElement;
    const session = this.windowInteractions.beginMarquee({ x: ev.clientX, y: ev.clientY }, host);
    let moved = false;
    const mv = (e: PointerEvent) => {
      const elements = Array.from(windowLayer.querySelectorAll('.win')) as HTMLElement[];
      const candidates = this.wins.flatMap((window, index) => {
        const element = elements[index];
        return element
          ? [
              {
                id: window.id,
                min: window.min,
                group: window.group,
                rect: element.getBoundingClientRect(),
              },
            ]
          : [];
      });
      const update = this.windowInteractions.moveMarquee(
        session,
        { x: e.clientX, y: e.clientY },
        candidates,
      );
      moved = moved || update.moved;
      this.marquee = { ...update.marquee };
      this.selected = update.selectedIds;
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
    const ids = this.selected.slice();
    const host = this.osEl.nativeElement.getBoundingClientRect();
    const dock = this.osEl.nativeElement.querySelector('.dock') as HTMLElement | null;
    const session = this.windowInteractions.beginGroupDrag(
      ids,
      this.wins,
      { x: ev.clientX, y: ev.clientY },
      host,
    );
    const mv = (e: PointerEvent) => {
      const update = this.windowInteractions.moveGroupDrag(
        session,
        { x: e.clientX, y: e.clientY },
        this.wins,
        dock?.getBoundingClientRect() ?? null,
      );
      this.wm.wins.set(update.windows);
      this.proxy = { ...update.proxy };
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
    const next = this.windowInteractions.createGroup(
      this.windowGroupingState(),
      ids,
      $localize`:Window group name@@desktop.group.name:Group ${this.groups.length + 1}:groupNumber:`,
    );
    if (next) this.applyWindowGroupingState(next);
  }
  toggleGroup(id: string) {
    const next = this.windowInteractions.toggleGroup(this.windowGroupingState(), id, () =>
      this.wm.nextZ(),
    );
    if (next) this.applyWindowGroupingState(next);
  }
  splitGroup(id: string) {
    const next = this.windowInteractions.splitGroup(this.windowGroupingState(), id, () =>
      this.wm.nextZ(),
    );
    if (next) this.applyWindowGroupingState(next);
  }
  closeGroup(id: string) {
    const next = this.windowInteractions.closeGroup(this.windowGroupingState(), id);
    if (next) this.applyWindowGroupingState(next);
  }
  private windowGroupingState(): WindowGroupingState {
    return {
      windows: this.wins,
      groups: this.groups,
      focusId: this.focusId,
      selectedIds: this.selected,
    };
  }
  private applyWindowGroupingState(state: WindowGroupingState) {
    this.wm.wins.set([...state.windows]);
    this.groups = [...state.groups];
    this.selected = [...state.selectedIds];
    this.wm.focusId.set(state.focusId);
    this.persist();
  }
  groupItems(g: any): ShellContextItem[] {
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
  openTray(key: ShellTrayKey, ev: Event) {
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
    const p = this.trayPanel?.panelElement,
      o = this.osEl?.nativeElement;
    if (!p || !o) return;
    const oR = o.getBoundingClientRect(),
      r = btn.getBoundingClientRect(),
      pw = p.offsetWidth;
    let left = r.right - oR.left - pw;
    if (left < 8) left = 8;
    this.tray.left = left;
    this.tray.top = r.bottom - oR.top + 6;
    this.changeDetector.markForCheck(); // runs from a timeout
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
  mbItems(key: string): ShellContextItem[] {
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
      // Every panelled application publishes its panes here, not just Control.
      if (f && this.submenuFor(f.app).length)
        return this.submenuFor(f.app).map(([k, i, l]) => ({
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
          ] as ShellContextItem[])
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
    const m = this.contextMenu?.panelElement,
      o = this.osEl?.nativeElement;
    if (!m || !o) return;
    const oR = o.getBoundingClientRect(),
      r = el.getBoundingClientRect();
    let left = r.left - oR.left,
      mw = m.offsetWidth;
    if (left + mw > oR.width - 8) left = oR.width - mw - 8;
    this.ctx.left = Math.max(4, left);
    this.ctx.top = r.bottom - oR.top + 2;
    // Runs from a timeout, so nothing else schedules a render: without this,
    // the first menu of a session opens at 0,0 and stays there until an
    // unrelated event repaints it.
    this.changeDetector.markForCheck();
  }
  hoverMb(key: string, ev: Event) {
    if (this.ctx.open && this.mbKey && this.mbKey !== key) this.openMb(key, ev);
  }

  // ── command palette — tap Cmd / Windows key to toggle ──
  commands(): ShellCommand[] {
    const f = this.wins.find((w) => w.id === this.focusId && !w.min);
    const c: ShellCommand[] = [];
    this.order.forEach((id) =>
      c.push({
        icon: this.APPS[id].icon,
        label: $localize`:Command palette action@@desktop.command.openApp:Open ${this.APPS[id].title}:appTitle:`,
        section: $localize`:Command palette section@@desktop.command.section.app:App`,
        run: () => this.wm.launch(id),
      }),
    );
    c.push(
      {
        icon: 'moon',
        label: $localize`:Command palette action@@desktop.command.themeDark:Theme: Dark`,
        section: $localize`:Command palette section@@desktop.command.section.theme:Theme`,
        run: () => this.setMode('dark'),
      },
      {
        icon: 'sun',
        label: $localize`:Command palette action@@desktop.command.themeLight:Theme: Light`,
        section: $localize`:Command palette section@@desktop.command.section.theme:Theme`,
        run: () => this.setMode('light'),
      },
    );
    c.push(
      {
        icon: 'palette',
        label: $localize`:Command palette action@@desktop.command.brandLethean:Brand: Lethean`,
        section: $localize`:Command palette section@@desktop.command.section.theme:Theme`,
        run: () => this.setBrand('lethean'),
      },
      {
        icon: 'palette',
        label: $localize`:Command palette action@@desktop.command.brandHostUk:Brand: Host UK`,
        section: $localize`:Command palette section@@desktop.command.section.theme:Theme`,
        run: () => this.setBrand('hostuk'),
      },
    );
    (['top', 'right', 'bottom', 'left'] as const).forEach((bb) =>
      c.push({
        icon: 'table-columns',
        label: $localize`:Command palette action@@desktop.command.taskbarEdge:Taskbar: ${this.edgeLabel(bb)}:edge:`,
        section: $localize`:Command palette section@@desktop.command.section.desktop:Desktop`,
        run: () => this.setBar(bb),
      }),
    );
    c.push({
      icon: 'table-cells',
      label: this.showWidgets
        ? $localize`:Command palette action@@desktop.command.hideWidgets:Hide widgets`
        : $localize`:Command palette action@@desktop.command.showWidgets:Show widgets`,
      section: $localize`:Command palette section@@desktop.command.section.desktop:Desktop`,
      run: () => (this.showWidgets = !this.showWidgets),
    });
    c.push({
      icon: 'icons',
      label: this.showIcons
        ? $localize`:Command palette action@@desktop.command.hideDesktopIcons:Hide desktop icons`
        : $localize`:Command palette action@@desktop.command.showDesktopIcons:Show desktop icons`,
      section: $localize`:Command palette section@@desktop.command.section.desktop:Desktop`,
      run: () => (this.showIcons = !this.showIcons),
    });
    c.push({
      icon: 'gauge',
      label: this.reduceMotion
        ? $localize`:Command palette action@@desktop.command.disableReduceMotion:Disable reduce motion`
        : $localize`:Command palette action@@desktop.command.enableReduceMotion:Enable reduce motion`,
      section: $localize`:Command palette section@@desktop.command.section.desktop:Desktop`,
      run: () => (this.reduceMotion = !this.reduceMotion),
    });
    if (f)
      c.push(
        {
          icon: 'window-minimize',
          label: $localize`:Command palette action@@desktop.command.minimiseWindow:Minimise window`,
          section: $localize`:Command palette section@@desktop.command.section.window:Window`,
          run: () => this.wm.minimise(f.id, this.reduceMotion ? 0 : 190),
        },
        {
          icon: 'expand',
          label: $localize`:Command palette action@@desktop.command.zoomWindow:Zoom window`,
          section: $localize`:Command palette section@@desktop.command.section.window:Window`,
          run: () => this.wm.maximise(f.id, this.windowBounds()),
        },
        {
          icon: 'xmark',
          label: $localize`:Command palette action@@desktop.command.closeWindow:Close window`,
          section: $localize`:Command palette section@@desktop.command.section.window:Window`,
          run: () => this.wm.close(f.id),
        },
      );
    this.wins
      .filter((w) => !w.group)
      .forEach((w) =>
        c.push({
          icon: this.APPS[w.app].icon,
          label: $localize`:Command palette action@@desktop.command.switchToApp:Switch to ${this.APPS[w.app].title}:appTitle:`,
          section: $localize`:Command palette section@@desktop.command.section.window:Window`,
          run: () => this.wm.focus(w.id),
        }),
      );
    c.push(
      {
        icon: 'lock',
        label: $localize`:Command palette action@@desktop.command.lockScreen:Lock screen`,
        section: $localize`:Command palette section@@desktop.command.section.session:Session`,
        run: () => this.sessionAction('lock'),
      },
      {
        icon: 'right-from-bracket',
        label: $localize`:Command palette action@@desktop.command.logOut:Log out`,
        section: $localize`:Command palette section@@desktop.command.section.session:Session`,
        run: () => this.sessionAction('logout'),
      },
      {
        icon: 'power-off',
        label: $localize`:Command palette action@@desktop.command.shutDown:Shut down`,
        section: $localize`:Command palette section@@desktop.command.section.session:Session`,
        run: () => this.sessionAction('shutdown'),
      },
    );
    return c;
  }
  filterCmds() {
    const q = this.palette.query.toLowerCase();
    this.filtered = this.commands().filter((c) =>
      (c.label + ' ' + c.section).toLowerCase().includes(q),
    );
    if (this.palette.index >= this.filtered.length)
      this.palette.index = Math.max(0, this.filtered.length - 1);
  }
  openPalette() {
    if (this.session !== 'active') return;
    this.closeMenus();
    this.paletteLastFocus = document.activeElement as HTMLElement;
    this.palette.open = true;
    this.palette.query = '';
    this.palette.index = 0;
    this.filterCmds();
    setTimeout(() => this.commandPalette?.focusInput(), 0);
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
    this.palette.query = (e.target as HTMLInputElement).value;
    this.palette.index = 0;
    this.filterCmds();
  }
  plKey(e: KeyboardEvent) {
    if (e.key === 'Tab') {
      e.preventDefault();
      return;
    }
    if (e.key === 'ArrowDown') {
      e.preventDefault();
      this.palette.index = Math.min(this.palette.index + 1, this.filtered.length - 1);
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      this.palette.index = Math.max(this.palette.index - 1, 0);
    } else if (e.key === 'Enter') {
      e.preventDefault();
      this.runCmd(this.palette.index);
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
    // Compare panes, not raw subs: an application whose sub carries its own
    // state (Files' location token) is already on the pane the URL names, and
    // re-setting it would throw that state away on every focus.
    if (win && target.sub && this.paneOf(win) !== target.sub) {
      this.wm.setSub(win.id, target.sub);
    }
  }

  private paneOf(win: Win): string {
    return routeSegmentsForWindow(this.routeCatalog, win.app, win.sub)[2] ?? '';
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
    const m = this.startMenu?.panelElement,
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
    this.changeDetector.markForCheck(); // runs from a timeout
  }

  // ── dock right-click → window actions ──
  winItems(w: Win): ShellContextItem[] {
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
  desktopItems(): ShellContextItem[] {
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
    this.csub.index = i;
    this.csub.open = true;
    setTimeout(() => this.placeCtxSub(row), 0);
  }
  placeCtxSub(rowEl: HTMLElement) {
    const m = this.contextMenu?.panelElement,
      o = this.osEl?.nativeElement;
    if (!m || !o) return;
    const mR = m.getBoundingClientRect(),
      rR = rowEl.getBoundingClientRect(),
      oR = o.getBoundingClientRect();
    let left = m.offsetWidth - 2;
    if (mR.left + left + 180 > oR.right) left = -(180 - 2);
    this.csub.left = left;
    this.csub.top = Math.max(0, rR.top - mR.top);
    this.changeDetector.markForCheck(); // runs from a timeout
  }
  clampCtx() {
    const m = this.contextMenu?.panelElement,
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
    this.changeDetector.markForCheck(); // runs from a timeout
  }
  runCtx(it: ShellContextItem, ev: Event) {
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
  sessionAction(kind: ShellSessionAction, ev?: Event) {
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

  snapWin(w: Win, s: WindowSnapState) {
    const bounds = this.winlayerEl.nativeElement.getBoundingClientRect();
    this.wm.wins.update((ws) =>
      ws.map((window) =>
        window.id === w.id ? this.windowInteractions.snapWindow(window, s, bounds) : window,
      ),
    );
    this.persist();
  }
  unsnap(w: Win) {
    this.wm.wins.update((ws) =>
      ws.map((window) =>
        window.id === w.id ? this.windowInteractions.unsnapWindow(window) : window,
      ),
    );
    this.persist();
  }
  kbSnap(w: Win, dir: KeyboardSnapDirection) {
    const intent = this.windowInteractions.keyboardSnap(w, dir);
    if (intent.kind === 'minimise') {
      this.wm.minimise(w.id, this.reduceMotion ? 0 : 190);
      return;
    }
    if (intent.kind === 'unsnap') {
      this.unsnap(w);
      return;
    }
    this.snapWin(w, intent.zone);
  }

  /**
   * Move one application out of the shell and into its own native window.
   *
   * A move, not a copy: the in-shell window closes only once Go reports the
   * native window open. A failure leaves the window exactly where it was and
   * says why, because half a move is an application that has disappeared.
   */
  async tearOff(w: Win): Promise<void> {
    const moved = await this.appWindows.openApp({
      app: w.app,
      pane: this.paneOf(w),
      width: w.w,
      height: w.h,
    });
    if (!moved) {
      this.notify(
        'triangle-exclamation',
        $localize`:Notification title@@desktop.tearOff.failedTitle:Could not open its own window`,
        this.appWindows.lastError(),
      );
      return;
    }
    this.wm.close(w.id);
    this.persist();
  }

  startDrag(ev: PointerEvent, w: Win) {
    if (!this.wm.windowed() || w.max) return;
    if ((ev.target as HTMLElement).closest('.lights i, .tearoff')) return; // not from a control
    if (this.selected.length > 1 && this.selected.includes(w.id)) {
      this.groupDrag(ev);
      return;
    }
    if (this.selected.length && !this.selected.includes(w.id)) this.selected = [];
    const layer = this.winlayerEl.nativeElement.getBoundingClientRect();
    const host = this.osEl.nativeElement.getBoundingClientRect();
    const session = this.windowInteractions.beginDrag(
      w,
      { x: ev.clientX, y: ev.clientY },
      layer,
      host,
    );
    const mv = (e: PointerEvent) => {
      const current = this.wins.find((window) => window.id === w.id);
      if (!current) return;
      const update = this.windowInteractions.moveDrag(
        session,
        { x: e.clientX, y: e.clientY },
        current,
      );
      this.wm.wins.set(this.wins.map((window) => (window.id === w.id ? update.window : window)));
      this.snap = { ...update.snap };
      // The preview only appears where the move can actually happen; in the
      // browser demo there is no second window to release the drag into.
      this.tear = { ...update.tear, active: update.tear.active && this.appWindows.available() };
    };
    const up = () => {
      window.removeEventListener('pointermove', mv);
      window.removeEventListener('pointerup', up);
      const z = this.snap.zone;
      const torn = this.tear.active;
      this.snap.zone = null;
      this.tear.active = false;
      const current = this.wins.find((win) => win.id === w.id);
      if (torn && current) {
        void this.tearOff(current);
        return;
      }
      if (z && current) {
        this.wm.wins.update((ws) =>
          ws.map((window) =>
            window.id === w.id ? this.windowInteractions.snapWindow(current, z, layer) : window,
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
    const session = this.windowInteractions.beginResize(w, {
      x: ev.clientX,
      y: ev.clientY,
    });
    const mv = (e: PointerEvent) => {
      const current = this.wins.find((window) => window.id === w.id);
      if (!current) return;
      const resized = this.windowInteractions.moveResize(
        session,
        { x: e.clientX, y: e.clientY },
        current,
      );
      this.wm.wins.set(this.wins.map((window) => (window.id === w.id ? resized : window)));
    };
    const up = () => {
      window.removeEventListener('pointermove', mv);
      window.removeEventListener('pointerup', up);
      this.persist();
    };
    window.addEventListener('pointermove', mv);
    window.addEventListener('pointerup', up);
  }
}

function reconcileShellGroups(
  current: readonly ShellWindowGroup[],
  windows: readonly Win[],
): ShellWindowGroup[] {
  const existing = new Map(current.map((group) => [group.id, group]));
  const grouped = new Map<string, Win[]>();
  for (const window of windows) {
    if (!window.group) continue;
    const members = grouped.get(window.group);
    if (members) members.push(window);
    else grouped.set(window.group, [window]);
  }
  return [...grouped.entries()].map(([id, members], index) => {
    const previous = existing.get(id);
    return {
      id,
      name:
        previous?.name ||
        $localize`:Restored window group name@@desktop.group.restoredName:Group ${index + 1}:groupNumber:`,
      ids: members.map(({ id: windowID }) => windowID),
      apps: members.map(({ app }) => app),
      open: members.some(({ min }) => !min),
    };
  });
}

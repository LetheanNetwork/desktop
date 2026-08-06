import { ComponentFixture, TestBed } from '@angular/core/testing';
import { Router, provideRouter } from '@angular/router';
import { provideState, provideStore, Store } from '@ngrx/store';
import { routes } from '../app.routes';
import { ConnectionManagerService } from '../connection-manager.service';
import { desktopActions } from '../store/desktop.actions';
import { desktopFeature, DesktopState } from '../store/desktop.reducer';
import { DESKTOP_STORAGE } from '../store/storage.service';
import { DesktopAppWindowBridgeService } from './desktop-app-window-bridge.service';
import { DesktopComponent } from './desktop.component';
import { Win } from './desktop.data';
import { readDesktopRouteCatalog } from './desktop-route-tree';
import { WindowManagerService } from './window-manager.service';

const controlWin: Win = {
  id: 'control-window',
  app: 'control',
  sub: 'models',
  systab: '',
  x: 70,
  y: 24,
  w: 780,
  h: 560,
  z: 11,
  min: false,
  max: false,
};

const telemetryWin: Win = {
  id: 'telemetry-window',
  app: 'telemetry',
  sub: '',
  systab: '',
  x: 104,
  y: 54,
  w: 660,
  h: 400,
  z: 12,
  min: false,
  max: false,
};

const seededState: DesktopState = {
  wins: [controlWin, telemetryWin],
  focusId: controlWin.id,
  view: 'desktop',
  device: 'small',
  devCat: null,
  z: 12,
  persistence: 'ready',
  persistenceRevision: 1,
  persistenceError: null,
  migratedBrowserState: true,
};

/** A DOMRect the way a laid-out element reports one; jsdom measures zero. */
const domRect = (left: number, top: number, width: number, height: number): DOMRect =>
  ({
    left,
    top,
    right: left + width,
    bottom: top + height,
    width,
    height,
    x: left,
    y: top,
    toJSON: () => ({}),
  }) as DOMRect;

/** A pointer event the drag handlers read: position, button, and target. */
const pointerEvent = (type: string, clientX: number, clientY: number): MouseEvent =>
  new MouseEvent(type, { bubbles: true, cancelable: true, clientX, clientY, button: 0 });

/** Flushes a real zero-delay `setTimeout` the shell schedules for positioning. */
const tick = (ms = 0): Promise<void> => new Promise((resolve) => setTimeout(resolve, ms));

describe('DesktopComponent route shell', () => {
  let fixture: ComponentFixture<DesktopComponent>;
  let router: Router;
  let store: Store;
  let windowManager: WindowManagerService;
  let openApp: ReturnType<typeof vi.fn>;
  let nativeWindowHost: boolean;
  let appWindows: {
    available: () => boolean;
    lastError: () => string;
    openApp: ReturnType<typeof vi.fn>;
  };
  const browserStorage = {
    length: 0,
    clear: vi.fn(),
    getItem: vi.fn(() => null),
    key: vi.fn(() => null),
    removeItem: vi.fn(),
    setItem: vi.fn(),
  } satisfies Storage;

  beforeEach(async () => {
    vi.clearAllMocks();
    openApp = vi.fn().mockResolvedValue(true);
    nativeWindowHost = true;
    appWindows = {
      available: () => nativeWindowHost,
      lastError: () => 'no native window route for application: control',
      openApp,
    };
    await TestBed.configureTestingModule({
      imports: [DesktopComponent],
      providers: [
        provideRouter(routes),
        provideStore(),
        provideState(desktopFeature),
        { provide: DESKTOP_STORAGE, useValue: browserStorage },
        { provide: DesktopAppWindowBridgeService, useValue: appWindows },
        {
          provide: ConnectionManagerService,
          useValue: { offline: () => false },
        },
      ],
    }).compileComponents();

    router = TestBed.inject(Router);
    store = TestBed.inject(Store);
    windowManager = TestBed.inject(WindowManagerService);
    await router.navigateByUrl('/');
    store.dispatch(
      desktopActions.hydrate({
        state: seededState,
        normalise: false,
      }),
    );
    fixture = TestBed.createComponent(DesktopComponent);
    // Document-attached so the shell's `document:click` / `document:keydown`
    // HostListeners — real global listeners, not fixture-scoped — receive the
    // events these tests dispatch.
    document.body.appendChild(fixture.nativeElement);
    fixture.detectChanges();
  });

  afterEach(() => {
    fixture?.destroy();
    fixture?.nativeElement.remove();
  });

  it('paints desktop chrome and both seeded route components', async () => {
    const element = fixture.nativeElement as HTMLElement;

    expect(element.querySelector('#os')).not.toBeNull();
    expect(element.querySelector('.menubar')).not.toBeNull();
    expect(element.querySelector('.bar')).not.toBeNull();
    expect(element.querySelectorAll('#winlayer > .win')).toHaveLength(2);

    await vi.waitFor(() => {
      fixture.detectChanges();
      expect(element.querySelector('lthn-control-app')).not.toBeNull();
      expect(element.querySelector('lthn-telemetry-app')).not.toBeNull();
    });
  });

  it('derives the category launcher and hover submenu from Router.config', () => {
    const catalog = readDesktopRouteCatalog(router.config);
    const element = fixture.nativeElement as HTMLElement;
    const categoryButtons = Array.from(
      element.querySelectorAll<HTMLButtonElement>('.menubar .route-category'),
    );

    expect(fixture.componentInstance.categories).toEqual(catalog.categories);
    expect(categoryButtons.map((button) => button.textContent?.trim())).toEqual(
      catalog.categories.map((category) => category.title),
    );

    categoryButtons[0].click();
    fixture.detectChanges();
    const controlItem = Array.from(
      element.querySelectorAll<HTMLElement>('.ctxmenu > .mi.haschild'),
    ).find((item) => item.textContent?.includes('Control'));
    expect(controlItem).toBeDefined();

    controlItem?.dispatchEvent(new MouseEvent('mouseenter', { bubbles: true }));
    fixture.detectChanges();
    const childLabels = Array.from(
      element.querySelectorAll<HTMLElement>('.ctxmenu .ctxsub .mi'),
    ).map((item) => item.textContent?.trim());
    expect(childLabels).toEqual(catalog.apps['control'].children.map(([, , title]) => title));
  });

  it('does not restore or persist browser state in connected mode', () => {
    expect(browserStorage.getItem).not.toHaveBeenCalled();

    fixture.componentInstance.persist();

    expect(browserStorage.getItem).not.toHaveBeenCalled();
    expect(browserStorage.setItem).not.toHaveBeenCalled();
  });

  it('reconstructs durable group membership without browser metadata', () => {
    store.dispatch(
      desktopActions.hydrate({
        state: {
          wins: [
            { ...controlWin, min: true, group: 'restored-group' },
            { ...telemetryWin, min: true, group: 'restored-group' },
          ],
          focusId: null,
        },
        normalise: false,
      }),
    );
    TestBed.flushEffects();

    expect(fixture.componentInstance.groups).toEqual([
      {
        id: 'restored-group',
        name: 'Group 1',
        ids: ['control-window', 'telemetry-window'],
        apps: ['control', 'telemetry'],
        open: false,
      },
    ]);
  });

  it('launches a menu child through the facade and reflects it in the URL', async () => {
    const dispatch = vi.spyOn(store, 'dispatch');
    const element = fixture.nativeElement as HTMLElement;
    const systemButton = element.querySelector<HTMLButtonElement>('.menubar .route-category');

    systemButton?.click();
    fixture.detectChanges();
    const controlItem = Array.from(
      element.querySelectorAll<HTMLElement>('.ctxmenu > .mi.haschild'),
    ).find((item) => item.textContent?.includes('Control'));
    controlItem?.dispatchEvent(new MouseEvent('mouseenter', { bubbles: true }));
    fixture.detectChanges();
    const runsItem = Array.from(element.querySelectorAll<HTMLElement>('.ctxmenu .ctxsub .mi')).find(
      (item) => item.textContent?.trim() === 'Runs',
    );
    runsItem?.click();

    expect(dispatch).toHaveBeenCalledWith(
      expect.objectContaining({
        type: '[Desktop] Launch App',
        appId: 'control',
      }),
    );
    expect(dispatch).toHaveBeenCalledWith(
      desktopActions.setSub({ id: controlWin.id, sub: 'runs' }),
    );

    await vi.waitFor(() => {
      fixture.detectChanges();
      expect(windowManager.wins().find((win) => win.id === controlWin.id)?.sub).toBe('runs');
      expect(router.url).toBe('/system/control/runs');
    });
  });

  it('routes a window rail selection through the store-owned sub-view', async () => {
    await vi.waitFor(() => {
      fixture.detectChanges();
      expect(fixture.nativeElement.querySelector('lthn-control-app')).not.toBeNull();
    });
    const railItems = fixture.nativeElement.querySelectorAll(
      'lthn-control-app .rail a',
    ) as NodeListOf<HTMLElement>;
    railItems[1].click();

    await vi.waitFor(() => {
      fixture.detectChanges();
      expect(windowManager.wins().find((win) => win.id === controlWin.id)?.sub).toBe('runs');
      expect(router.url).toBe('/system/control/runs');
    });
  });

  it('selects and configures a store window from a direct child route', async () => {
    await router.navigateByUrl('/media/games/engines');

    await vi.waitFor(() => {
      fixture.detectChanges();
      const games = windowManager.wins().find((win) => win.app === 'games');
      expect(games?.sub).toBe('engines');
      expect(windowManager.focusId()).toBe(games?.id);
      expect(router.url).toBe('/media/games/engines');
    });
  });

  it('opens a consolidated application on the pane its URL names', async () => {
    await router.navigateByUrl('/office/project-manager/sprints');

    await vi.waitFor(() => {
      fixture.detectChanges();
      const planner = windowManager.wins().find((win) => win.app === 'project-manager');
      expect(planner?.sub).toBe('sprints');
      expect(windowManager.focusId()).toBe(planner?.id);
      expect(router.url).toBe('/office/project-manager/sprints');
      expect(
        (fixture.nativeElement as HTMLElement).querySelector('lthn-planning-sprints-surface'),
      ).not.toBeNull();
    });
  });

  it('hands the titlebar tear-off the application, its pane, and its size', async () => {
    const control = (fixture.nativeElement as HTMLElement).querySelector<HTMLButtonElement>(
      '#winlayer > .win .titlebar .tearoff',
    );

    control?.click();
    await vi.waitFor(() => {
      expect(openApp).toHaveBeenCalledWith({
        app: 'control',
        pane: 'models',
        width: controlWin.w,
        height: controlWin.h,
      });
    });
    await vi.waitFor(() => {
      fixture.detectChanges();
      expect(windowManager.wins().map(({ id }) => id)).toEqual([telemetryWin.id]);
    });
  });

  /**
   * Part B of the tear-off: the same move offered by dragging the window past
   * the shell's own edge. The measurements are stubbed because jsdom lays
   * nothing out, and the release is a real pointerup on the real listeners.
   */
  const dragTitlebar = (to: { x: number; y: number }) => {
    const element = fixture.nativeElement as HTMLElement;
    const screen = element.querySelector('#os') as HTMLElement;
    const layer = element.querySelector('#winlayer') as HTMLElement;
    screen.getBoundingClientRect = () => domRect(0, 0, 1440, 900);
    layer.getBoundingClientRect = () => domRect(0, 36, 1440, 864);
    const titlebar = element.querySelector('#winlayer > .win .titlebar') as HTMLElement;

    titlebar.dispatchEvent(pointerEvent('pointerdown', 400, 60));
    window.dispatchEvent(pointerEvent('pointermove', to.x, to.y));
  };

  it('tears a window off when its drag is released past the shell edge', async () => {
    dragTitlebar({ x: 1600, y: 520 });
    expect(fixture.componentInstance.tear.active).toBe(true);

    window.dispatchEvent(pointerEvent('pointerup', 1600, 520));

    await vi.waitFor(() => {
      expect(openApp).toHaveBeenCalledWith({
        app: 'control',
        pane: 'models',
        width: controlWin.w,
        height: controlWin.h,
      });
    });
    await vi.waitFor(() => {
      fixture.detectChanges();
      expect(windowManager.wins().map(({ id }) => id)).toEqual([telemetryWin.id]);
    });
    expect(fixture.componentInstance.tear.active).toBe(false);
  });

  it('snaps instead of tearing when the drag is released inside the shell', async () => {
    dragTitlebar({ x: 1600, y: 520 });
    window.dispatchEvent(pointerEvent('pointermove', 8, 400));
    expect(fixture.componentInstance.tear.active).toBe(false);
    expect(fixture.componentInstance.snap.zone).toBe('left');

    window.dispatchEvent(pointerEvent('pointerup', 8, 400));
    fixture.detectChanges();

    expect(openApp).not.toHaveBeenCalled();
    expect(windowManager.wins().map(({ id }) => id)).toEqual([controlWin.id, telemetryWin.id]);
    expect(windowManager.wins()[0].snapState).toBe('left');
  });

  it('offers no tear-off past the edge when there is no native window host', () => {
    nativeWindowHost = false;

    dragTitlebar({ x: 1600, y: 520 });
    window.dispatchEvent(pointerEvent('pointerup', 1600, 520));

    expect(fixture.componentInstance.tear.active).toBe(false);
    expect(openApp).not.toHaveBeenCalled();
    expect(windowManager.wins().map(({ id }) => id)).toEqual([controlWin.id, telemetryWin.id]);
  });

  it('leaves the in-shell window alone when the native window refuses to open', async () => {
    openApp.mockResolvedValue(false);

    await fixture.componentInstance.tearOff(controlWin);
    fixture.detectChanges();

    expect(windowManager.wins().map(({ id }) => id)).toEqual([controlWin.id, telemetryWin.id]);
    expect(fixture.componentInstance.notifs[0]).toMatchObject({
      icon: 'triangle-exclamation',
      body: 'no native window route for application: control',
    });
  });

  it('keeps a Files location token when the route reflects its Local pane', async () => {
    store.dispatch(
      desktopActions.hydrate({
        state: {
          wins: [{ ...controlWin, id: 'files-window', app: 'files', sub: 'documents::Reports' }],
          focusId: 'files-window',
        },
        normalise: false,
      }),
    );

    await vi.waitFor(() => {
      fixture.detectChanges();
      expect(router.url).toBe('/tools/files/home');
    });
    expect(windowManager.wins().find((win) => win.id === 'files-window')?.sub).toBe(
      'documents::Reports',
    );
  });

  it('delegates every preference get/set pair to PreferencesService signals', () => {
    const component = fixture.componentInstance;
    const prefs = component.prefs;

    component.bar = 'left';
    expect(prefs.bar()).toBe('left');
    component.wall = 'dusk';
    expect(prefs.wallpaper()).toBe('dusk');
    component.mode = 'light';
    expect(prefs.mode()).toBe('light');
    component.brand = 'hostuk';
    expect(prefs.brand()).toBe('hostuk');
    component.design = 'custom';
    expect(prefs.design()).toBe('custom');
    component.customHue = 210;
    expect(component.customHue).toBe(210);
    component.customName = 'Acme';
    expect(component.customName).toBe('Acme');
    component.lang = 'fr';
    expect(prefs.lang()).toBe('fr');
    component.showIcons = false;
    expect(prefs.showIcons()).toBe(false);
    component.reduceMotion = true;
    expect(prefs.reduceMotion()).toBe(true);
    component.showWidgets = false;
    expect(prefs.showWidgets()).toBe(false);
  });

  it('computes the minimise transform-origin for every taskbar edge', () => {
    const component = fixture.componentInstance;

    component.bar = 'bottom';
    expect(component.minOrigin()).toBe('50% 100%');
    component.bar = 'top';
    expect(component.minOrigin()).toBe('50% 0%');
    component.bar = 'left';
    expect(component.minOrigin()).toBe('0% 50%');
    component.bar = 'right';
    expect(component.minOrigin()).toBe('100% 50%');
  });

  it('labels every taskbar edge', () => {
    const component = fixture.componentInstance;
    expect(component.edgeLabel('top')).toBe('Top');
    expect(component.edgeLabel('right')).toBe('Right');
    expect(component.edgeLabel('bottom')).toBe('Bottom');
    expect(component.edgeLabel('left')).toBe('Left');
  });

  it('labels the About design as custom, Host UK, or Lethean', () => {
    const component = fixture.componentInstance;

    component.design = 'custom';
    component.customName = 'Acme Suite';
    expect(component.designLabel()).toBe('Acme Suite');

    component.customName = '';
    expect(component.designLabel()).toBe('Custom');

    component.design = 'lethean';
    component.brand = 'hostuk';
    expect(component.designLabel()).toBe('Host UK');

    component.brand = 'lethean';
    expect(component.designLabel()).toBe('Lethean');
  });

  it('paints the custom accent ramp onto the OS screen and clears it again', () => {
    const prefs = fixture.componentInstance.prefs;
    const os = (fixture.nativeElement as HTMLElement).querySelector('#os') as HTMLElement;

    prefs.design.set('custom');
    prefs.customHue.set(210);
    TestBed.flushEffects();

    expect(os.style.getPropertyValue('--brand-500')).toContain('210');

    prefs.design.set('lethean');
    TestBed.flushEffects();

    expect(os.style.getPropertyValue('--brand-500')).toBe('');
  });

  it('dismisses a notification by id', () => {
    const component = fixture.componentInstance;
    component.notify('cube', 'Test title', 'Test body');
    const [notification] = component.notifs;

    component.dismiss(notification.id);

    expect(component.notifs).toEqual([]);
  });

  it('shows the welcome notifications once the shell settles, while active', async () => {
    const component = fixture.componentInstance;

    await vi.waitFor(
      () => {
        expect(component.notifs.length).toBe(2);
      },
      { timeout: 2000 },
    );
    expect(component.notifs.map((n) => n.title)).toEqual(
      expect.arrayContaining(['Model loaded', 'LetherNet']),
    );
  });

  it('resolves category apps/icon/label for known, unknown, and null ids', () => {
    const component = fixture.componentInstance;

    expect(component.catApps(null)).toEqual([]);
    expect(component.catApps('not-a-category')).toEqual([]);
    expect(component.catApps('system').length).toBeGreaterThan(0);
    expect(component.catIcon(null)).toBe('');
    expect(component.catIcon('system')).not.toBe('');
    expect(component.catLabel(null)).toBe('');
    expect(component.catLabel('system')).toBe('System');
  });

  it('toggles a Start-menu category open state', () => {
    const component = fixture.componentInstance;
    expect(component.openCats['system']).toBeUndefined();

    component.toggleStartCategory('system');
    expect(component.openCats['system']).toBe(true);

    component.toggleStartCategory('system');
    expect(component.openCats['system']).toBe(false);
  });

  it('derives runningApps, deskIcons, and taskWins from the live window list', () => {
    const component = fixture.componentInstance;

    expect(component.runningApps.sort()).toEqual(['control', 'telemetry']);
    expect(component.deskIcons).not.toContain('operations'); // dev-only app
    expect(component.taskWins.map((w) => w.id).sort()).toEqual(
      [controlWin.id, telemetryWin.id].sort(),
    );
  });

  it('focuses, restores, and minimises windows from dock clicks', () => {
    fixture.componentInstance.reduceMotion = true; // skip the minimise animation delay
    const element = fixture.nativeElement as HTMLElement;
    const controlDock = element.querySelector('.dock .di[aria-label="Control"]') as HTMLElement;
    const telemetryDock = element.querySelector(
      '.dock .di[aria-label="Telemetry"]',
    ) as HTMLElement;

    // Focused, un-minimised window: dock click minimises it.
    expect(windowManager.focusId()).toBe(controlWin.id);
    controlDock.click();
    expect(windowManager.wins().find((w) => w.id === controlWin.id)?.min).toBe(true);

    // Minimised window: dock click restores/focuses it.
    controlDock.click();
    expect(windowManager.wins().find((w) => w.id === controlWin.id)?.min).toBe(false);
    expect(windowManager.focusId()).toBe(controlWin.id);

    // Unfocused, un-minimised window: dock click focuses it.
    telemetryDock.click();
    expect(windowManager.focusId()).toBe(telemetryWin.id);
  });

  it('opens the Start menu, hovers a submenu, and launches a child app', async () => {
    const element = fixture.nativeElement as HTMLElement;
    const component = fixture.componentInstance;
    const sessionBtn = element.querySelector('.session') as HTMLElement;

    sessionBtn.click();
    await tick();
    fixture.detectChanges();
    expect(component.sessionOpen).toBe(true);

    const systemCategory = element.querySelector('.sp-apps .sm-cat') as HTMLElement;
    systemCategory.click();
    fixture.detectChanges();

    const rows = Array.from(element.querySelectorAll('.sp-apps .sm-child')) as HTMLElement[];
    const welcomeRow = rows.find((row) => row.textContent?.includes('Welcome')) as HTMLElement;
    // No children: hovering just closes any open submenu.
    welcomeRow.dispatchEvent(new MouseEvent('mouseenter', { bubbles: true }));
    expect(component.sub.open).toBe(false);

    const controlRow = rows.find((row) => row.textContent?.includes('Control')) as HTMLElement;
    controlRow.dispatchEvent(new MouseEvent('mouseenter', { bubbles: true }));
    await tick();
    fixture.detectChanges();
    expect(component.sub.open).toBe(true);
    expect(component.sub.parent).toBe('control');

    const runsChild = Array.from(element.querySelectorAll('.submenu .mi')).find((row) =>
      row.textContent?.includes('Runs'),
    ) as HTMLElement;
    runsChild.click();

    expect(component.sessionOpen).toBe(false);
    expect(windowManager.wins().find((w) => w.id === controlWin.id)?.sub).toBe('runs');
  });

  it('selects windows with a marquee drag, then drags them onto the dock to group them', () => {
    const element = fixture.nativeElement as HTMLElement;
    const component = fixture.componentInstance;
    const os = element.querySelector('#os') as HTMLElement;
    const winLayer = element.querySelector('#winlayer') as HTMLElement;
    const wall = element.querySelector('.wall') as HTMLElement;
    const dock = element.querySelector('.dock') as HTMLElement;

    os.getBoundingClientRect = () => domRect(0, 0, 1440, 900);
    dock.getBoundingClientRect = () => domRect(0, 820, 400, 60);
    const windows = Array.from(winLayer.querySelectorAll(':scope > .win')) as HTMLElement[];
    expect(windows).toHaveLength(2);
    windows.forEach((win) => {
      win.getBoundingClientRect = () => domRect(0, 0, 1440, 900);
    });

    wall.dispatchEvent(pointerEvent('pointerdown', 10, 10));
    window.dispatchEvent(pointerEvent('pointermove', 400, 400));
    expect(component.marquee.open).toBe(true);
    expect(component.selected.sort()).toEqual([controlWin.id, telemetryWin.id].sort());
    window.dispatchEvent(pointerEvent('pointerup', 400, 400));
    expect(component.marquee.open).toBe(false);
    expect(component.selected).toHaveLength(2);

    const titlebar = winLayer.querySelector('.win .titlebar') as HTMLElement;
    titlebar.dispatchEvent(pointerEvent('pointerdown', 100, 100));
    window.dispatchEvent(pointerEvent('pointermove', 100, 830));
    expect(component.proxy.open).toBe(true);
    expect(component.proxy.over).toBe(true);
    window.dispatchEvent(pointerEvent('pointerup', 100, 830));

    expect(component.groups).toHaveLength(1);
    fixture.detectChanges();
    const groupBtn = element.querySelector('.dock .di.group') as HTMLElement;
    expect(groupBtn.querySelector('.gcount')?.textContent?.trim()).toBe('2');
  });

  it('opens, right-click manages, and splits a window group from the dock', () => {
    store.dispatch(
      desktopActions.hydrate({
        state: {
          wins: [
            { ...controlWin, min: true, group: 'g1' },
            { ...telemetryWin, min: true, group: 'g1' },
          ],
          focusId: null,
        },
        normalise: false,
      }),
    );
    TestBed.flushEffects();
    fixture.detectChanges();

    const element = fixture.nativeElement as HTMLElement;
    const groupBtn = element.querySelector('.dock .di.group') as HTMLElement;
    expect(groupBtn).not.toBeNull();

    groupBtn.click();
    fixture.detectChanges();
    expect(fixture.componentInstance.groups[0].open).toBe(true);
    expect(windowManager.wins().find((w) => w.id === controlWin.id)?.min).toBe(false);

    // Toggling rebuilds the group object, so *ngFor swaps in a fresh node.
    const reopenedGroupBtn = element.querySelector('.dock .di.group') as HTMLElement;
    reopenedGroupBtn.dispatchEvent(
      new MouseEvent('contextmenu', { bubbles: true, cancelable: true }),
    );
    fixture.detectChanges();
    expect(fixture.componentInstance.ctx.open).toBe(true);
    const items = Array.from(element.querySelectorAll('.ctxmenu .mi')) as HTMLElement[];
    const splitItem = items.find((item) => item.textContent?.includes('Split group'));
    splitItem?.click();

    expect(fixture.componentInstance.groups).toEqual([]);
    expect(windowManager.wins().find((w) => w.id === controlWin.id)?.group).toBeUndefined();
  });

  it('opens the system menu, switches to the app menu on hover, and shows About', async () => {
    const element = fixture.nativeElement as HTMLElement;
    const component = fixture.componentInstance;
    const systemBtn = element.querySelector('.hoplite-btn.mbtn') as HTMLButtonElement;

    systemBtn.click();
    await tick();
    fixture.detectChanges();
    expect(component.ctx.open).toBe(true);
    expect(component.mbKey).toBe('system');
    const systemLabels = Array.from(element.querySelectorAll('.ctxmenu .mi')).map((i) =>
      i.textContent?.trim(),
    );
    expect(systemLabels).toEqual(
      expect.arrayContaining([
        'About LetheanOS',
        'System Settings…',
        'Lock Screen',
        'Restart…',
        'Shut Down…',
      ]),
    );

    const appBtn = element.querySelector('.mbtn.app') as HTMLButtonElement;
    appBtn.dispatchEvent(new MouseEvent('mouseenter', { bubbles: true }));
    await tick();
    fixture.detectChanges();
    expect(component.mbKey).toBe('app');

    systemBtn.click();
    await tick();
    fixture.detectChanges();
    const aboutItem = Array.from(element.querySelectorAll('.ctxmenu .mi')).find(
      (item) => item.textContent?.trim() === 'About LetheanOS',
    ) as HTMLElement;
    aboutItem.click();
    fixture.detectChanges();

    expect(component.about).toBe(true);
    expect((element.querySelector('.aboutmodal') as HTMLElement).hidden).toBe(false);

    (element.querySelector('.aboutcard .aclose') as HTMLButtonElement).click();
    fixture.detectChanges();
    expect(component.about).toBe(false);

    // Re-opening on the same key toggles the menu closed instead.
    systemBtn.click();
    expect(component.ctx.open).toBe(true);
    systemBtn.click();
    expect(component.ctx.open).toBe(false);
  });

  it('builds and executes the legacy File/Edit/View/Window menu vocabularies', () => {
    const component = fixture.componentInstance;
    component.reduceMotion = true; // skip the minimise animation delay

    const fileItems = component.mbItems('File') as any[];
    const newWindowChild = fileItems[0].children.find((c: any) => c.label === 'Chat');
    newWindowChild.act();
    expect(windowManager.wins().some((w) => w.app === 'chat')).toBe(true);
    fileItems.at(-1).act(); // Close Window, with control still focused
    expect(windowManager.wins().find((w) => w.id === controlWin.id)).toBeUndefined();

    const editItems = component.mbItems('Edit') as any[];
    expect(() =>
      editItems.filter((item) => !item.sep).forEach((item) => item.act()),
    ).not.toThrow();

    windowManager.focus(telemetryWin.id);
    const viewItems = component.mbItems('View') as any[];
    viewItems.find((i) => i.label === 'Zoom')?.act();
    expect(windowManager.wins().find((w) => w.id === telemetryWin.id)?.max).toBe(true);
    const widgetsItem = viewItems.find((i) => i.label?.includes('Widgets')) as any;
    const widgetsBefore = component.showWidgets;
    widgetsItem.act();
    expect(component.showWidgets).toBe(!widgetsBefore);
    const iconsItem = viewItems.find((i) => i.label?.includes('Desktop Icons')) as any;
    const iconsBefore = component.showIcons;
    iconsItem.act();
    expect(component.showIcons).toBe(!iconsBefore);

    const windowItems = component.mbItems('Window') as any[];
    windowItems.find((i) => i.label === 'Minimise')?.act();
    expect(windowManager.wins().find((w) => w.id === telemetryWin.id)?.min).toBe(true);

    windowManager.wins.set([]);
    windowManager.focusId.set(null);
    const emptyWindowItems = component.mbItems('anything-else') as any[];
    expect(emptyWindowItems.map((i) => i.label)).toEqual(['No open windows']);
    expect(() => emptyWindowItems[0].act()).not.toThrow();
  });

  it('runs the app-menu Preferences action against the focused Control window', () => {
    const component = fixture.componentInstance;

    windowManager.focusId.set(null);
    const noFocusItems = component.mbItems('app') as any[];
    expect(noFocusItems.at(-1).label).toBe('Quit');
    expect(() => noFocusItems.at(-1).act()).not.toThrow();
    noFocusItems[1].act(); // Preferences with no focused window -> launch Settings
    expect(windowManager.wins().some((w) => w.app === 'settings')).toBe(true);

    windowManager.focus(controlWin.id);
    const focusedItems = component.mbItems('app') as any[];
    focusedItems[1].act(); // Preferences with focused Control -> jump its own pane
    expect(windowManager.wins().find((w) => w.id === controlWin.id)?.sub).toBe('settings');
  });

  it('runs the system-menu System Settings and Lock Screen actions', () => {
    const component = fixture.componentInstance;
    const items = component.mbItems('system') as any[];

    items.find((i) => i.label === 'System Settings…')?.act();
    expect(windowManager.wins().some((w) => w.app === 'settings')).toBe(true);

    items.find((i) => i.label === 'Lock Screen')?.act();
    expect(component.session).toBe('locked');
  });

  it('opens each menu-bar tray panel, positions it, and runs its actions', async () => {
    const element = fixture.nativeElement as HTMLElement;
    const component = fixture.componentInstance;
    const trayButtons = Array.from(
      element.querySelectorAll('.menubar .right .trayi'),
    ) as HTMLButtonElement[];
    expect(trayButtons).toHaveLength(4);

    trayButtons[0].click(); // lang
    await tick();
    fixture.detectChanges();
    expect(component.tray.open).toBe(true);
    expect(component.tray.key).toBe('lang');

    trayButtons[0].click(); // same key again: closes
    expect(component.tray.open).toBe(false);

    trayButtons[0].click();
    await tick();
    fixture.detectChanges();
    const frRow = Array.from(element.querySelectorAll('.traypanel .mi')).find((row) =>
      row.textContent?.includes('Français'),
    ) as HTMLElement;
    frRow.click();
    expect(component.lang).toBe('fr');
    expect(component.tray.open).toBe(false);

    trayButtons[1].click(); // wifi
    await tick();
    fixture.detectChanges();
    const openLetherNet = Array.from(element.querySelectorAll('.traypanel .tr-btn')).find((btn) =>
      btn.textContent?.includes('Open LetherNet'),
    ) as HTMLElement;
    openLetherNet.click();
    expect(windowManager.wins().some((w) => w.app === 'lethernet')).toBe(true);

    trayButtons[2].click(); // battery
    await tick();
    fixture.detectChanges();
    const openPower = Array.from(element.querySelectorAll('.traypanel .tr-btn')).find((btn) =>
      btn.textContent?.includes('Open Power'),
    ) as HTMLElement;
    openPower.click();
    expect(windowManager.wins().find((w) => w.app === 'control')?.sub).toBe('power');

    trayButtons[3].click(); // clock
    await tick();
    fixture.detectChanges();
    expect(element.querySelector('.traypanel .tr-big')?.textContent).toContain(component.clock);
  });

  it('opens the command palette on a Meta tap, filters, navigates, and runs a command', async () => {
    const element = fixture.nativeElement as HTMLElement;
    const component = fixture.componentInstance;

    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Meta', bubbles: true }));
    document.dispatchEvent(new KeyboardEvent('keyup', { key: 'Meta', bubbles: true }));
    await tick();
    fixture.detectChanges();
    expect(component.palette.open).toBe(true);

    const input = element.querySelector('.palette input') as HTMLInputElement;
    input.value = 'reduce motion';
    input.dispatchEvent(new Event('input', { bubbles: true }));
    fixture.detectChanges();
    expect(component.filtered.length).toBeGreaterThan(0);
    expect(
      component.filtered.every((c) =>
        (c.label + c.section).toLowerCase().includes('reduce motion'),
      ),
    ).toBe(true);

    const tabEvent = new KeyboardEvent('keydown', { key: 'Tab', bubbles: true, cancelable: true });
    input.dispatchEvent(tabEvent);
    expect(tabEvent.defaultPrevented).toBe(true);
    input.dispatchEvent(
      new KeyboardEvent('keydown', { key: 'ArrowDown', bubbles: true, cancelable: true }),
    );
    input.dispatchEvent(
      new KeyboardEvent('keydown', { key: 'ArrowUp', bubbles: true, cancelable: true }),
    );

    const before = component.reduceMotion;
    input.dispatchEvent(
      new KeyboardEvent('keydown', { key: 'Enter', bubbles: true, cancelable: true }),
    );
    fixture.detectChanges();
    expect(component.reduceMotion).toBe(!before);
    expect(component.palette.open).toBe(false);
  });

  it('closes the palette on Escape and on a backdrop click, and stays shut off-session', async () => {
    const element = fixture.nativeElement as HTMLElement;
    const component = fixture.componentInstance;

    component.openPalette();
    fixture.detectChanges(false);
    const input = element.querySelector('.palette input') as HTMLInputElement;
    input.dispatchEvent(
      new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true }),
    );
    await tick();
    fixture.detectChanges(false);
    expect(component.palette.open).toBe(false);

    component.openPalette();
    fixture.detectChanges(false);
    const backdrop = element.querySelector('.paletteov') as HTMLElement;
    backdrop.dispatchEvent(pointerEvent('pointerdown', 5, 5));
    await tick();
    fixture.detectChanges(false);
    expect(component.palette.open).toBe(false);

    component.session = 'locked';
    component.togglePalette();
    await tick();
    fixture.detectChanges(false);
    expect(component.palette.open).toBe(false);
    component.session = 'active';
  });

  it('drives window keyboard shortcuts: snap, cycle, minimise, and close', () => {
    const component = fixture.componentInstance;
    component.reduceMotion = true; // skip the minimise animation delay
    expect(windowManager.focusId()).toBe(controlWin.id);

    document.dispatchEvent(
      new KeyboardEvent('keydown', {
        key: 'ArrowLeft',
        ctrlKey: true,
        bubbles: true,
        cancelable: true,
      }),
    );
    expect(windowManager.wins().find((w) => w.id === controlWin.id)?.snapState).toBe('left');

    document.dispatchEvent(
      new KeyboardEvent('keydown', { key: '`', ctrlKey: true, bubbles: true, cancelable: true }),
    );
    expect(windowManager.focusId()).toBe(telemetryWin.id);

    document.dispatchEvent(
      new KeyboardEvent('keydown', { key: 'm', ctrlKey: true, bubbles: true, cancelable: true }),
    );
    expect(windowManager.wins().find((w) => w.id === telemetryWin.id)?.min).toBe(true);

    windowManager.focus(controlWin.id);
    document.dispatchEvent(
      new KeyboardEvent('keydown', { key: 'w', ctrlKey: true, bubbles: true, cancelable: true }),
    );
    expect(windowManager.wins().find((w) => w.id === controlWin.id)).toBeUndefined();
    expect(component.session).toBe('active');
  });

  it('closes open menus on an outside click and on Escape', () => {
    const element = fixture.nativeElement as HTMLElement;
    const component = fixture.componentInstance;
    const sessionBtn = element.querySelector('.session') as HTMLElement;

    sessionBtn.click();
    expect(component.sessionOpen).toBe(true);
    document.body.dispatchEvent(new MouseEvent('click', { bubbles: true }));
    expect(component.sessionOpen).toBe(false);

    sessionBtn.click();
    expect(component.sessionOpen).toBe(true);
    document.dispatchEvent(
      new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true }),
    );
    expect(component.sessionOpen).toBe(false);
  });

  it('opens the session chip menu, positions it per taskbar edge, and cycles session screens', async () => {
    const element = fixture.nativeElement as HTMLElement;
    const component = fixture.componentInstance;
    const sessionBtn = element.querySelector('.session') as HTMLElement;

    for (const edge of ['bottom', 'top', 'left', 'right'] as const) {
      component.bar = edge;
      sessionBtn.click();
      await tick();
      fixture.detectChanges(false);
      if (edge === 'bottom') {
        expect(component.sessionPos.bottom).not.toBeNull();
        expect(component.sessionPos.top).toBeNull();
      } else {
        expect(component.sessionPos.top).not.toBeNull();
      }
      sessionBtn.click(); // close again for the next edge
      await tick();
      fixture.detectChanges(false);
    }
    component.bar = 'bottom';

    // Lock, then unlock.
    sessionBtn.click();
    await tick();
    fixture.detectChanges(false);
    const lockRow = Array.from(element.querySelectorAll('.sp-user .mi')).find((row) =>
      row.textContent?.includes('Lock screen'),
    ) as HTMLElement;
    lockRow.click();
    await tick();
    fixture.detectChanges(false);
    expect(component.session).toBe('locked');
    (element.querySelector('.lockscreen .lk-btn') as HTMLButtonElement).click();
    await tick();
    fixture.detectChanges(false);
    expect(component.session).toBe('active');

    // Switch user.
    sessionBtn.click();
    await tick();
    fixture.detectChanges(false);
    const switchRow = Array.from(element.querySelectorAll('.sp-user .mi')).find((row) =>
      row.textContent?.includes('Switch user'),
    ) as HTMLElement;
    switchRow.click();
    await tick();
    fixture.detectChanges(false);
    expect(component.session).toBe('switch');
    const secondAccount = Array.from(element.querySelectorAll('.lk-acct'))[1] as HTMLElement;
    secondAccount.click();
    await tick();
    fixture.detectChanges(false);
    expect(component.session).toBe('active');
    expect(component.user.initials).toBe('JM');

    // Log out, then sign back in — with no windows left, resume relaunches the defaults.
    sessionBtn.click();
    await tick();
    fixture.detectChanges(false);
    const logoutRow = Array.from(element.querySelectorAll('.sp-user .mi')).find((row) =>
      row.textContent?.includes('Log out'),
    ) as HTMLElement;
    logoutRow.click();
    await tick();
    fixture.detectChanges(false);
    expect(component.session).toBe('login');
    expect(windowManager.wins()).toEqual([]);
    (element.querySelector('.lockscreen .lk-btn') as HTMLButtonElement).click();
    await tick();
    fixture.detectChanges(false);
    expect(component.session).toBe('active');
    expect(windowManager.wins().map((w) => w.app).sort()).toEqual(['control', 'telemetry']);

    // Shut down, wait for the power-off screen, then power back on.
    sessionBtn.click();
    await tick();
    fixture.detectChanges(false);
    const shutdownRow = Array.from(element.querySelectorAll('.sp-user .mi')).find((row) =>
      row.textContent?.includes('Shut down'),
    ) as HTMLElement;
    shutdownRow.click();
    await tick();
    fixture.detectChanges(false);
    expect(component.session).toBe('shutting');
    expect(windowManager.wins()).toEqual([]);

    await vi.waitFor(
      () => {
        fixture.detectChanges(false);
        expect(component.session).toBe('off');
        expect(element.querySelector('.lockscreen .lk-power')).not.toBeNull();
      },
      { timeout: 2000 },
    );
    (element.querySelector('.lockscreen .lk-power') as HTMLButtonElement).click();
    await tick();
    fixture.detectChanges(false);
    expect(component.session).toBe('active');
  });

  it('opens the desktop context menu and its New Window submenu', async () => {
    const element = fixture.nativeElement as HTMLElement;
    const component = fixture.componentInstance;
    const wall = element.querySelector('.wall') as HTMLElement;

    wall.dispatchEvent(
      new MouseEvent('contextmenu', {
        bubbles: true,
        cancelable: true,
        clientX: 40,
        clientY: 60,
      }),
    );
    await tick();
    fixture.detectChanges();
    expect(component.ctx.open).toBe(true);

    const newWindowRow = Array.from(element.querySelectorAll('.ctxmenu .mi.haschild')).find(
      (row) => row.textContent?.includes('New window'),
    ) as HTMLElement;
    newWindowRow.dispatchEvent(new MouseEvent('mouseenter', { bubbles: true }));
    await tick();
    fixture.detectChanges();
    expect(component.csub.open).toBe(true);

    const submenuChat = Array.from(element.querySelectorAll('.ctxmenu .ctxsub .mi')).find((row) =>
      row.textContent?.includes('Chat'),
    ) as HTMLElement;
    submenuChat.click();
    expect(windowManager.wins().some((w) => w.app === 'chat')).toBe(true);
  });

  it('builds and executes every desktop-background context action', () => {
    const component = fixture.componentInstance;
    const items = component.desktopItems() as any[];

    const appearance = items.find((i) => i.label === 'Appearance');
    appearance.children.find((c: any) => c.label === 'Light').act();
    expect(component.mode).toBe('light');
    appearance.children.find((c: any) => c.label === 'Dark').act();
    expect(component.mode).toBe('dark');

    const edge = items.find((i) => i.label === 'Taskbar edge');
    edge.children.find((c: any) => c.label === 'Right').act();
    expect(component.bar).toBe('right');
    component.bar = 'bottom';

    const showIconsBefore = component.showIcons;
    items.find((i) => i.label?.includes('desktop icons')).act();
    expect(component.showIcons).toBe(!showIconsBefore);

    const showWidgetsBefore = component.showWidgets;
    items.find((i) => i.label?.includes('widgets')).act();
    expect(component.showWidgets).toBe(!showWidgetsBefore);
  });

  it('opens a desktop-icon context menu that launches the application', () => {
    const component = fixture.componentInstance;
    const appId = component.deskIcons[0];
    const element = fixture.nativeElement as HTMLElement;
    const icon = element.querySelector('.dicon') as HTMLElement;

    icon.dispatchEvent(new MouseEvent('contextmenu', { bubbles: true, cancelable: true }));
    fixture.detectChanges();

    expect(component.ctx.title).toBe(component.APPS[appId].title);
    expect(component.ctx.items.map((i: any) => i.label)).toEqual(['Open']);
    (component.ctx.items[0] as any).act();
    expect(windowManager.wins().some((w) => w.app === appId)).toBe(true);
  });

  it('opens a dock context menu for a running app, listing window actions', () => {
    const component = fixture.componentInstance;
    const element = fixture.nativeElement as HTMLElement;
    const controlDock = element.querySelector('.dock .di[aria-label="Control"]') as HTMLElement;

    controlDock.dispatchEvent(new MouseEvent('contextmenu', { bubbles: true, cancelable: true }));
    fixture.detectChanges();

    expect(
      component.ctx.items.filter((i: any) => !i.sep).map((i: any) => i.label),
    ).toEqual(['Bring to front', 'Minimise', 'Zoom', 'Close window']);
  });

  it('falls back to an Open action when a dock context targets an app with no window', () => {
    const component = fixture.componentInstance;
    component.dockCtx(new MouseEvent('contextmenu'), 'lethernet');
    expect(component.ctx.items.map((i: any) => i.label)).toEqual(['Open']);
  });

  it('opens a window context menu via right-click on the window body', () => {
    const component = fixture.componentInstance;
    const element = fixture.nativeElement as HTMLElement;
    const winEl = element.querySelector('#winlayer > .win') as HTMLElement;

    winEl.dispatchEvent(new MouseEvent('contextmenu', { bubbles: true, cancelable: true }));
    fixture.detectChanges();

    expect(component.ctx.title).not.toBe('');
    expect(
      component.ctx.items.filter((i: any) => !i.sep).map((i: any) => i.label),
    ).toContain('Bring to front');
  });

  it('labels window context actions for the minimised and maximised state', () => {
    const component = fixture.componentInstance;
    const items = component.winItems({ ...controlWin, min: true, max: true }) as any[];
    expect(items.filter((i) => !i.sep).map((i) => i.label)).toEqual([
      'Bring to front',
      'Restore',
      'Restore size',
      'Close window',
    ]);
  });

  it('reports window bounds from the layer element and a stable trackBy id', () => {
    const component = fixture.componentInstance;
    // jsdom never lays elements out, so the real (zero) rect wins over the
    // "?? 900" fallback — that fallback only ever fires when there is no
    // element at all (the ViewChild unresolved).
    expect(component.windowBounds()).toEqual({ w: 0, h: 0 });
    expect(component.trackWin(0, controlWin)).toBe(controlWin.id);
  });
});

/**
 * Offline persistence needs its own TestBed: the default suite runs
 * connected (`ConnectionManagerService.offline() === false`), so `persist()`
 * and `restore()` never reach the browser-storage branch there.
 */
describe('DesktopComponent offline persistence', () => {
  let fixture: ComponentFixture<DesktopComponent>;
  let storage: Storage & { getItem: ReturnType<typeof vi.fn>; setItem: ReturnType<typeof vi.fn> };

  const buildStorage = (overrides: Partial<Storage> = {}): typeof storage =>
    ({
      length: 0,
      clear: vi.fn(),
      getItem: vi.fn(() => null),
      key: vi.fn(() => null),
      removeItem: vi.fn(),
      setItem: vi.fn(),
      ...overrides,
    }) as typeof storage;

  const build = async (storageOverrides: Partial<Storage> = {}): Promise<void> => {
    storage = buildStorage(storageOverrides);
    await TestBed.configureTestingModule({
      imports: [DesktopComponent],
      providers: [
        provideRouter(routes),
        provideStore(),
        provideState(desktopFeature),
        { provide: DESKTOP_STORAGE, useValue: storage },
        {
          provide: DesktopAppWindowBridgeService,
          useValue: { available: () => false, lastError: () => '', openApp: vi.fn() },
        },
        { provide: ConnectionManagerService, useValue: { offline: () => true } },
      ],
    }).compileComponents();
    fixture = TestBed.createComponent(DesktopComponent);
  };

  afterEach(() => {
    fixture?.destroy();
  });

  it('restores persisted open categories, groups, and shell tabs when offline', async () => {
    await build({
      getItem: vi.fn(() =>
        JSON.stringify({ openCats: { system: true }, groups: [{ id: 'g1' }], shellTabs: ['a'] }),
      ),
    });

    expect(storage.getItem).toHaveBeenCalledWith('lthn.desktop');
    expect(fixture.componentInstance.openCats).toEqual({ system: true });
    expect(fixture.componentInstance.groups).toEqual([{ id: 'g1' }]);
    expect(fixture.componentInstance.shellTabs).toEqual(['a']);
  });

  it('ignores corrupt persisted state instead of throwing', async () => {
    await expect(build({ getItem: vi.fn(() => 'not-json{') })).resolves.toBeUndefined();
    expect(fixture.componentInstance.openCats).toEqual({});
  });

  it('does nothing on restore when storage has no saved state', async () => {
    await build();
    expect(fixture.componentInstance.openCats).toEqual({});
  });

  it('persists layout to storage when offline', async () => {
    await build();
    fixture.detectChanges();

    fixture.componentInstance.persist();

    expect(storage.setItem).toHaveBeenCalledWith(
      'lthn.desktop',
      expect.stringContaining('"openCats"'),
    );
  });

  it('swallows a storage write failure while persisting offline', async () => {
    await build({
      setItem: vi.fn(() => {
        throw new Error('quota exceeded');
      }),
    });
    fixture.detectChanges();

    expect(() => fixture.componentInstance.persist()).not.toThrow();
  });
});

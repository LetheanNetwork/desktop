import { ComponentFixture, TestBed } from '@angular/core/testing';
import { Router, provideRouter } from '@angular/router';
import { provideState, provideStore, Store } from '@ngrx/store';
import { routes } from '../app.routes';
import { desktopActions } from '../store/desktop.actions';
import { desktopFeature, DesktopState } from '../store/desktop.reducer';
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
};

describe('DesktopComponent route shell', () => {
  let fixture: ComponentFixture<DesktopComponent>;
  let router: Router;
  let store: Store;
  let windowManager: WindowManagerService;

  beforeEach(async () => {
    await TestBed.configureTestingModule({
      imports: [DesktopComponent],
      providers: [provideRouter(routes), provideStore(), provideState(desktopFeature)],
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
    fixture.detectChanges();
  });

  afterEach(() => {
    fixture?.destroy();
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
});

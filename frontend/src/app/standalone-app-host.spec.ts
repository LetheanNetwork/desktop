import { TestBed } from '@angular/core/testing';
import { MockStore, provideMockStore } from '@ngrx/store/testing';
import { ActivatedRoute, convertToParamMap, ParamMap, provideRouter } from '@angular/router';
import { BehaviorSubject } from 'rxjs';
import { signal } from '@angular/core';
import { routes } from './app.routes';
import { ConnectionManagerService } from './connection-manager.service';
import { DesktopAppWindowBridgeService } from './desktop/desktop-app-window-bridge.service';
import { DesktopWindowBridgeService } from './desktop/desktop-window-bridge.service';
import { Win } from './desktop/desktop.data';
import { PreferencesService } from './desktop/preferences.service';
import { StandaloneAppHost } from './standalone-app-host';
import { DesktopState } from './store/desktop.reducer';

const telemetryWin: Win = {
  id: 'w1',
  app: 'telemetry',
  sub: '',
  systab: '',
  x: 70,
  y: 24,
  w: 660,
  h: 400,
  z: 11,
  min: false,
  max: false,
};

const desktopState: DesktopState = {
  wins: [telemetryWin],
  focusId: 'w1',
  view: 'desktop',
  device: 'small',
  devCat: null,
  z: 11,
  persistence: 'ready',
  persistenceRevision: 1,
  persistenceError: null,
  migratedBrowserState: true,
};

describe('StandaloneAppHost', () => {
  let params: BehaviorSubject<ParamMap>;
  let store: MockStore;
  let windowBridge: {
    side: ReturnType<typeof signal<'left' | 'right'>>;
    available: ReturnType<typeof signal<boolean>>;
    maximised: ReturnType<typeof signal<boolean>>;
    lastError: ReturnType<typeof signal<string>>;
    minimise: ReturnType<typeof vi.fn>;
    toggleMaximise: ReturnType<typeof vi.fn>;
    close: ReturnType<typeof vi.fn>;
  };
  let appWindows: {
    available: ReturnType<typeof signal<boolean>>;
    lastError: ReturnType<typeof signal<string>>;
    dockApp: ReturnType<typeof vi.fn>;
  };

  beforeEach(() => {
    params = new BehaviorSubject(convertToParamMap({ app: 'telemetry' }));
    windowBridge = {
      side: signal<'left' | 'right'>('left'),
      available: signal(true),
      maximised: signal(false),
      lastError: signal(''),
      minimise: vi.fn().mockResolvedValue(undefined),
      toggleMaximise: vi.fn().mockResolvedValue(undefined),
      close: vi.fn().mockResolvedValue(undefined),
    };
    appWindows = {
      available: signal(true),
      lastError: signal(''),
      dockApp: vi.fn().mockResolvedValue(true),
    };
    TestBed.configureTestingModule({
      imports: [StandaloneAppHost],
      providers: [
        provideMockStore({ initialState: { desktop: desktopState } }),
        provideRouter(routes),
        { provide: ConnectionManagerService, useValue: { offline: () => false } },
        { provide: DesktopWindowBridgeService, useValue: windowBridge },
        { provide: DesktopAppWindowBridgeService, useValue: appWindows },
        {
          provide: ActivatedRoute,
          useValue: {
            paramMap: params.asObservable(),
            get snapshot() {
              return { paramMap: params.value };
            },
          },
        },
      ],
    });
    store = TestBed.inject(MockStore);
    vi.spyOn(store, 'dispatch');
  });

  it('renders the routed component with its store-owned window input', async () => {
    const fixture = TestBed.createComponent(StandaloneAppHost);
    fixture.detectChanges();

    expect(fixture.componentInstance.win()).toEqual(telemetryWin);
    await vi.waitFor(() => {
      fixture.detectChanges();
      expect(fixture.nativeElement.querySelector('lthn-telemetry-app')).not.toBeNull();
    });
  });

  it('degrades to a readable empty state for an unknown app route', () => {
    const fixture = TestBed.createComponent(StandaloneAppHost);
    fixture.detectChanges();

    params.next(convertToParamMap({ app: 'unknown' }));
    fixture.detectChanges();

    expect(fixture.componentInstance.win()).toBeNull();
    expect(fixture.nativeElement.querySelector('lthn-window-route-content')).toBeNull();
    expect(fixture.nativeElement.querySelector('.solo-empty')?.textContent).toContain(
      'not installed',
    );
  });

  it('shows the pane the window carried out of the shell', async () => {
    params.next(convertToParamMap({ app: 'telemetry', pane: 'observe' }));
    const fixture = TestBed.createComponent(StandaloneAppHost);
    fixture.detectChanges();

    expect(fixture.componentInstance.pane()).toBe('observe');
    expect(store.dispatch).toHaveBeenCalledWith(
      expect.objectContaining({ id: 'w1', sub: 'observe' }),
    );
  });

  it('renders no desktop chrome around the solo application', async () => {
    const fixture = TestBed.createComponent(StandaloneAppHost);
    fixture.detectChanges();

    await vi.waitFor(() => {
      fixture.detectChanges();
      expect(fixture.nativeElement.querySelector('lthn-telemetry-app')).not.toBeNull();
    });
    // The window's own title bar stays — the native window is frameless, so
    // it is the only chrome there is. The desktop's is what does not follow:
    // no taskbar, no dock, no menu bar, no start menu, no tray.
    expect(fixture.nativeElement.querySelector('.taskbar, .dock, .menubar')).toBeNull();
    expect(fixture.nativeElement.querySelector('.trayi, .hoplite-btn, .shelldock')).toBeNull();
  });

  it('frames the solo application the way the shell frames a window', () => {
    const fixture = TestBed.createComponent(StandaloneAppHost);
    fixture.detectChanges();
    const host = fixture.nativeElement as HTMLElement;

    expect(host.getAttribute('data-brand')).toBe('lethean');
    const screen = host.querySelector('#os');
    expect(screen?.classList.contains('mode-shell')).toBe(true);
    expect(screen?.getAttribute('data-wall')).toBe('aurora');
    expect(host.querySelector('#winlayer > .win.focused > .appwrap')).not.toBeNull();
  });

  it('loads the application frame stylesheet, not only its tokens', () => {
    const fixture = TestBed.createComponent(StandaloneAppHost);
    fixture.detectChanges();

    // Without the desktop's own sheet the solo window renders the
    // application in the browser's serif default: the tokens are global,
    // every rule that uses them is not.
    const styles = Array.from(document.querySelectorAll('style'))
      .map((element) => element.textContent ?? '')
      .join('\n');
    expect(styles).toMatch(/\.appwrap\s*\{/u);
    expect(styles).toMatch(/\.appbody\s*\{/u);
    expect(styles).toMatch(/\.mode-shell \.win\s*\{/u);
    expect(styles).toMatch(/app-standalone-app-host\s*\{/u);
  });

  it('carries the custom design out of the shell with the application', () => {
    const preferences = TestBed.inject(PreferencesService);
    preferences.design.set('custom');
    preferences.customHue.set(305);
    preferences.customName.set('Host UK');
    const fixture = TestBed.createComponent(StandaloneAppHost);
    fixture.detectChanges();

    const screen = (fixture.nativeElement as HTMLElement).querySelector('#os') as HTMLElement;
    expect(screen.style.getPropertyValue('--brand-500')).toBe('oklch(0.54 0.16 305)');
    expect(screen.style.getPropertyValue('--brand-name')).toBe("'Host UK'");
  });

  it('draws the window its own title bar, with lights and a title', () => {
    const fixture = TestBed.createComponent(StandaloneAppHost);
    fixture.detectChanges();
    const titlebar = (fixture.nativeElement as HTMLElement).querySelector('.titlebar.solotitle');

    expect(titlebar).not.toBeNull();
    expect(titlebar?.querySelectorAll('.lights button')).toHaveLength(3);
    expect(titlebar?.querySelector('.lights .c')).not.toBeNull();
    expect(titlebar?.querySelector('.lights .m')).not.toBeNull();
    expect(titlebar?.querySelector('.lights .x')).not.toBeNull();
    expect(titlebar?.querySelector('.tb-title')?.textContent?.trim()).toBe('Telemetry');
  });

  it('names the pane the window is showing beside the application', () => {
    // A solo "Activity" window is ambiguous in a way "Activity · Observe" is
    // not: the pane is the only thing distinguishing two of them.
    params.next(convertToParamMap({ app: 'activity', pane: 'observe' }));
    const fixture = TestBed.createComponent(StandaloneAppHost);
    fixture.detectChanges();

    expect(fixture.componentInstance.title()).toBe('Activity · Observe');
  });

  it('names the application alone when it declares no panes', () => {
    params.next(convertToParamMap({ app: 'telemetry' }));
    const fixture = TestBed.createComponent(StandaloneAppHost);
    fixture.detectChanges();

    expect(fixture.componentInstance.title()).toBe('Telemetry');
  });

  it('wires each light to the native window, not to the desktop it left', () => {
    const fixture = TestBed.createComponent(StandaloneAppHost);
    fixture.detectChanges();
    const titlebar = (fixture.nativeElement as HTMLElement).querySelector(
      '.titlebar.solotitle',
    ) as HTMLElement;

    titlebar.querySelector<HTMLButtonElement>('.lights .m')?.click();
    titlebar.querySelector<HTMLButtonElement>('.lights .x')?.click();
    titlebar.querySelector<HTMLButtonElement>('.lights .c')?.click();

    expect(windowBridge.minimise).toHaveBeenCalledOnce();
    expect(windowBridge.toggleMaximise).toHaveBeenCalledOnce();
    expect(windowBridge.close).toHaveBeenCalledOnce();
    expect(store.dispatch).not.toHaveBeenCalledWith(
      expect.objectContaining({ type: '[Desktop] Close Window' }),
    );
  });

  it('offers to restore rather than zoom once the window is maximised', () => {
    const fixture = TestBed.createComponent(StandaloneAppHost);
    fixture.detectChanges();
    const zoom = (fixture.nativeElement as HTMLElement).querySelector('.lights .x');

    expect(zoom?.getAttribute('title')).toBe('Zoom');

    windowBridge.maximised.set(true);
    fixture.detectChanges();

    expect(zoom?.getAttribute('title')).toBe('Restore');
  });

  it('claims the strip for dragging and lets every control out of it', () => {
    const fixture = TestBed.createComponent(StandaloneAppHost);
    fixture.detectChanges();

    // Wails reads --wails-draggable off the element under the pointer: the
    // bar opts in so the frameless window can be moved at all, and everything
    // clickable inside it opts out, or the press starts a window move and the
    // click is lost. On macOS this is also the only drag mechanism left — the
    // tear-off profile drops the invisible title bar, which cannot see the
    // controls sitting in its strip.
    const styles = Array.from(document.querySelectorAll('style'))
      .map((element) => element.textContent ?? '')
      .join('\n');
    expect(styles).toMatch(/\.titlebar\.solotitle\s*\{[^}]*--wails-draggable:\s*drag/u);
    expect(styles).toMatch(/\.solotitle \.lights button\s*\{[^}]*--wails-draggable:\s*no-drag/u);
    expect(styles).toMatch(/\.solotitle \.dockback\s*\{[^}]*--wails-draggable:\s*no-drag/u);
    expect(styles).toMatch(/--solo-titlebar-height:\s*36px/u);
  });

  it('follows the platform for which side the window controls sit on', () => {
    const fixture = TestBed.createComponent(StandaloneAppHost);
    fixture.detectChanges();
    const titlebar = (fixture.nativeElement as HTMLElement).querySelector('.titlebar.solotitle');

    expect(titlebar?.getAttribute('data-controls')).toBe('left');
    expect(titlebar?.firstElementChild?.classList.contains('lights')).toBe(true);

    windowBridge.side.set('right');
    fixture.detectChanges();

    expect(titlebar?.getAttribute('data-controls')).toBe('right');
    expect(titlebar?.firstElementChild?.classList.contains('dockback')).toBe(true);
  });

  it('shows the controls inert rather than absent in the browser demo', () => {
    windowBridge.available.set(false);
    appWindows.available.set(false);
    const fixture = TestBed.createComponent(StandaloneAppHost);
    fixture.detectChanges();
    const host = fixture.nativeElement as HTMLElement;

    expect(host.querySelector('.lights')?.classList.contains('unavailable')).toBe(true);
    expect(host.querySelectorAll('.lights button:disabled')).toHaveLength(3);
    expect(host.querySelector<HTMLButtonElement>('.dockback')?.disabled).toBe(true);
  });

  it('publishes a failed window control rather than swallowing it', () => {
    const fixture = TestBed.createComponent(StandaloneAppHost);
    fixture.detectChanges();
    const lights = (fixture.nativeElement as HTMLElement).querySelector('.lights');

    expect(lights?.getAttribute('data-wc-error')).toBeNull();

    windowBridge.lastError.set("window 'app-view-telemetry' not found");
    fixture.detectChanges();

    expect(lights?.getAttribute('data-wc-error')).toContain('not found');
  });

  it('hands the application back to the shell, then closes its own window', async () => {
    params.next(convertToParamMap({ app: 'telemetry', pane: 'observe' }));
    const fixture = TestBed.createComponent(StandaloneAppHost);
    fixture.detectChanges();

    await fixture.componentInstance.dockBack();

    expect(appWindows.dockApp).toHaveBeenCalledWith({ app: 'telemetry', pane: 'observe' });
    expect(windowBridge.close).toHaveBeenCalledOnce();
  });

  it('stays open, and says why, when the shell will not take it back', async () => {
    appWindows.dockApp.mockResolvedValue(false);
    appWindows.lastError.set('no shell window for application: telemetry');
    const fixture = TestBed.createComponent(StandaloneAppHost);
    fixture.detectChanges();

    await fixture.componentInstance.dockBack();
    fixture.detectChanges();

    expect(windowBridge.close).not.toHaveBeenCalled();
    expect(
      (fixture.nativeElement as HTMLElement).querySelector('.solo-error')?.textContent,
    ).toContain('no shell window');
  });

  it('keeps the empty state inside the same frame', () => {
    params.next(convertToParamMap({ app: 'unknown' }));
    const fixture = TestBed.createComponent(StandaloneAppHost);
    fixture.detectChanges();
    const host = fixture.nativeElement as HTMLElement;

    expect(host.querySelector('#winlayer > .win')).not.toBeNull();
    expect(host.querySelector('.win > .solo-empty')?.textContent).toContain('not installed');
  });
});

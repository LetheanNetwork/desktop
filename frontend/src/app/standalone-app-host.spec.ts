import { TestBed } from '@angular/core/testing';
import { MockStore, provideMockStore } from '@ngrx/store/testing';
import { ActivatedRoute, convertToParamMap, ParamMap, provideRouter } from '@angular/router';
import { BehaviorSubject } from 'rxjs';
import { routes } from './app.routes';
import { ConnectionManagerService } from './connection-manager.service';
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

  beforeEach(() => {
    params = new BehaviorSubject(convertToParamMap({ app: 'telemetry' }));
    TestBed.configureTestingModule({
      imports: [StandaloneAppHost],
      providers: [
        provideMockStore({ initialState: { desktop: desktopState } }),
        provideRouter(routes),
        { provide: ConnectionManagerService, useValue: { offline: () => false } },
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
    expect(fixture.nativeElement.querySelector('.taskbar, .dock, .menubar, .titlebar')).toBeNull();
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

  it('keeps the empty state inside the same frame', () => {
    params.next(convertToParamMap({ app: 'unknown' }));
    const fixture = TestBed.createComponent(StandaloneAppHost);
    fixture.detectChanges();
    const host = fixture.nativeElement as HTMLElement;

    expect(host.querySelector('#winlayer > .win')).not.toBeNull();
    expect(host.querySelector('.win > .solo-empty')?.textContent).toContain('not installed');
  });
});

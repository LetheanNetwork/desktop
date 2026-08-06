import { TestBed } from '@angular/core/testing';
import { MockStore, provideMockStore } from '@ngrx/store/testing';
import { ActivatedRoute, convertToParamMap, ParamMap, provideRouter } from '@angular/router';
import { BehaviorSubject } from 'rxjs';
import { routes } from './app.routes';
import { Win } from './desktop/desktop.data';
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
});

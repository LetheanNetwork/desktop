import { TestBed } from '@angular/core/testing';
import { APP_NAV } from '../desktop-catalogue.data';
import type { Win } from '../desktop.data';
import { SurfaceBridgeService } from '../surfaces/surface-bridge.service';
import { WindowManagerService } from '../window-manager.service';
import { CompositeApp } from './composite.app';

const windows = { setSub: vi.fn(), setSysTab: vi.fn() };

const win = (app: string, sub: string): Win => ({
  id: `${app}-window`,
  app,
  sub,
  systab: '',
  x: 0,
  y: 0,
  w: 920,
  h: 640,
  z: 1,
  min: false,
  max: false,
});

describe('CompositeApp', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    TestBed.configureTestingModule({
      providers: [
        { provide: WindowManagerService, useValue: windows },
        {
          provide: SurfaceBridgeService,
          useValue: { call: vi.fn().mockRejectedValue(new Error('offline')), request: vi.fn() },
        },
      ],
    });
  });

  afterEach(() => TestBed.resetTestingModule());

  async function open(app: string, sub: string) {
    const fixture = TestBed.createComponent(CompositeApp);
    fixture.componentRef.setInput('win', win(app, sub));
    fixture.componentRef.setInput('nav', APP_NAV[app] ?? []);
    fixture.detectChanges();
    await fixture.whenStable();
    fixture.detectChanges();
    return fixture;
  }

  it('renders one rail entry per pane and marks the active one', async () => {
    const fixture = await open('operations', 'runbooks');
    const rail = (fixture.nativeElement as HTMLElement).querySelectorAll('.rail a');

    expect(rail).toHaveLength(3);
    expect(fixture.componentInstance.path).toBe('runbooks');
    expect(rail[1].classList.contains('on')).toBe(true);

    (rail[2] as HTMLElement).click();
    expect(windows.setSub).toHaveBeenCalledWith('operations-window', 'status');
  });

  it('loads the surface a pane names', async () => {
    const fixture = await open('operations', 'status');

    expect(
      (fixture.nativeElement as HTMLElement).querySelector('lthn-operations-status-surface'),
    ).not.toBeNull();
  });

  it('renders a developer pane through the shared panel with its own empty state', async () => {
    const fixture = await open('ide', 'git');
    const element = fixture.nativeElement as HTMLElement;

    expect(element.querySelector('lthn-dev-panel-app')).not.toBeNull();
    expect(fixture.componentInstance.devPane?.app.route).toBe('git');
    expect(fixture.componentInstance.devPane?.empty?.[0]).toBe('No changes');
    expect(fixture.componentInstance.devPane?.app.title).toBe('Git');
  });

  it('falls back to the default pane when the stored sub is retired', async () => {
    const fixture = await open('ide', 'repos');

    expect(fixture.componentInstance.path).toBe('control-panel');
    expect(fixture.componentInstance.devPane?.app.route).toBe('control-panel');
  });

  it('swaps panes when the window sub changes', async () => {
    const fixture = await open('databases', 'duckdb');
    const element = fixture.nativeElement as HTMLElement;
    expect(element.querySelector('lthn-ml-lab-duckdb-surface')).not.toBeNull();

    fixture.componentRef.setInput('win', win('databases', 'influxdb'));
    fixture.detectChanges();
    await fixture.whenStable();
    fixture.detectChanges();

    expect(fixture.componentInstance.path).toBe('influxdb');
    expect(element.querySelector('lthn-ml-lab-influx-surface')).not.toBeNull();
    expect(element.querySelector('lthn-ml-lab-duckdb-surface')).toBeNull();
  });
});

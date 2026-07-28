import { signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { DesktopLiveDataService } from '../desktop-live-data.service';
import type { Win } from '../desktop.data';
import { AgentsTerminalSurface } from '../surfaces/agents/terminal';
import { SurfaceBridgeService } from '../surfaces/surface-bridge.service';
import { DevPanelApp } from './dev-panel.app';
import { APP_REGISTRY } from './app-view';
import { TerminalApp } from './terminal.app';

const terminalWin: Win = {
  id: 'terminal-window',
  app: 'terminal',
  sub: '',
  x: 0,
  y: 0,
  w: 920,
  h: 640,
  z: 1,
  min: false,
  max: false,
};

describe('TerminalApp', () => {
  const mode = signal<'demo' | 'live'>('demo');

  beforeEach(() => {
    mode.set('demo');
    TestBed.configureTestingModule({
      providers: [
        {
          provide: DesktopLiveDataService,
          useValue: { mode: mode.asReadonly() },
        },
        {
          provide: SurfaceBridgeService,
          useValue: { call: vi.fn().mockResolvedValue({ sessions: [] }) },
        },
      ],
    });
    TestBed.overrideComponent(DevPanelApp, {
      set: {
        template: '<span data-demo-terminal>demo terminal</span>',
      },
    });
    TestBed.overrideComponent(AgentsTerminalSurface, {
      set: {
        imports: [],
        template: '<span data-live-terminal>live terminal</span>',
      },
    });
  });

  afterEach(() => TestBed.resetTestingModule());

  function create() {
    const fixture = TestBed.createComponent(TerminalApp);
    fixture.componentRef.setInput('win', terminalWin);
    fixture.detectChanges();
    return fixture;
  }

  it('selects the browser-safe terminal fixture when the transport is offline', () => {
    const fixture = create();
    const text = (fixture.nativeElement as HTMLElement).textContent ?? '';

    expect(
      (fixture.nativeElement as HTMLElement).querySelector('[data-demo-terminal]'),
    ).not.toBeNull();
    expect((fixture.nativeElement as HTMLElement).querySelector('[data-live-terminal]')).toBeNull();
    expect(text).toContain('Demo data');
  });

  it('selects the shared Wails PTY surface when the transport is live', () => {
    mode.set('live');
    const fixture = create();

    expect(
      (fixture.nativeElement as HTMLElement).querySelector('[data-live-terminal]'),
    ).not.toBeNull();
    expect((fixture.nativeElement as HTMLElement).querySelector('[data-demo-terminal]')).toBeNull();
  });

  it('is the lazy component registered for the base Terminal application', async () => {
    await expect(APP_REGISTRY['terminal']()).resolves.toBe(TerminalApp);
  });
});

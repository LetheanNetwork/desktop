import { TestBed } from '@angular/core/testing';
import { ConnectionManagerService } from '../connection-manager.service';
import {
  APP_WINDOW_METHODS,
  DesktopAppWindowBridgeService,
} from './desktop-app-window-bridge.service';
import { SurfaceBridgeService } from './surfaces/surface-bridge.service';

describe('DesktopAppWindowBridgeService', () => {
  let call: ReturnType<typeof vi.fn>;
  let offline: boolean;

  const bridge = (): DesktopAppWindowBridgeService => TestBed.inject(DesktopAppWindowBridgeService);

  beforeEach(() => {
    call = vi.fn().mockResolvedValue(null);
    offline = false;
    TestBed.configureTestingModule({
      providers: [
        { provide: SurfaceBridgeService, useValue: { call } },
        { provide: ConnectionManagerService, useValue: { offline: () => offline } },
      ],
    });
  });

  it('names the application, its pane, and the geometry it is carrying', async () => {
    const moved = await bridge().openApp({
      app: 'control',
      pane: 'models',
      width: 780,
      height: 560,
    });

    expect(moved).toBe(true);
    expect(call).toHaveBeenCalledWith(APP_WINDOW_METHODS.openApp, [
      { app: 'control', pane: 'models', width: 780, height: 560 },
    ]);
  });

  it('carries no geometry rather than a fractional or absent one', async () => {
    await bridge().openApp({ app: 'chat', width: 640.6, height: Number.NaN });

    expect(call).toHaveBeenCalledWith(APP_WINDOW_METHODS.openApp, [
      { app: 'chat', pane: '', width: 0, height: 0 },
    ]);
  });

  it('reports the reason when Go refuses the tear-off', async () => {
    call.mockRejectedValue(new Error('no native window route for application: terminal'));

    const moved = await bridge().openApp({ app: 'terminal' });

    expect(moved).toBe(false);
    expect(bridge().lastError()).toContain('no native window route');
  });

  it('opens nothing in the browser demo, where there is no window to open', async () => {
    offline = true;

    const moved = await bridge().openApp({ app: 'chat' });

    expect(moved).toBe(false);
    expect(bridge().available()).toBe(false);
    expect(call).not.toHaveBeenCalled();
  });

  it('asks Go to hand the application back, naming it and its pane', async () => {
    const docked = await bridge().dockApp({ app: 'control', pane: 'models' });

    expect(docked).toBe(true);
    expect(call).toHaveBeenCalledWith(APP_WINDOW_METHODS.dockApp, [
      { app: 'control', pane: 'models', width: 0, height: 0 },
    ]);
  });

  it('reports a refused dock-back rather than closing over it', async () => {
    // The solo window closes only on true. A false here is what keeps the
    // application on screen instead of vanishing between two windows.
    call.mockRejectedValue(new Error('no shell window for application: terminal'));

    const docked = await bridge().dockApp({ app: 'terminal' });

    expect(docked).toBe(false);
    expect(bridge().lastError()).toContain('no shell window');
  });

  it('docks nothing when no application is named', async () => {
    const docked = await bridge().dockApp({ app: '' });

    expect(docked).toBe(false);
    expect(call).not.toHaveBeenCalled();
    expect(bridge().lastError()).toContain('no application named');
  });

  it('docks nothing in the browser demo, where there is no shell to return to', async () => {
    offline = true;

    const docked = await bridge().dockApp({ app: 'chat' });

    expect(docked).toBe(false);
    expect(call).not.toHaveBeenCalled();
  });
});

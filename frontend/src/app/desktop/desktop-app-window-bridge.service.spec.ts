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
});

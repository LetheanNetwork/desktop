import { TestBed } from '@angular/core/testing';
import { ConnectionManagerService } from '../connection-manager.service';
import {
  DesktopWindowBridgeService,
  NATIVE_WINDOW_NAME,
  nativeWindowNameForUrl,
} from './desktop-window-bridge.service';

interface MockWindow {
  Minimise: ReturnType<typeof vi.fn>;
  ToggleMaximise: ReturnType<typeof vi.fn>;
  Close: ReturnType<typeof vi.fn>;
  IsMaximised: ReturnType<typeof vi.fn>;
}

const runtime = vi.hoisted(() => ({
  named: new Map<string, MockWindow>(),
  isWindows: false,
  isMac: true,
  window(name: string): MockWindow {
    const existing = runtime.named.get(name);
    if (existing) return existing;
    const created: MockWindow = {
      Minimise: vi.fn().mockResolvedValue(undefined),
      ToggleMaximise: vi.fn().mockResolvedValue(undefined),
      Close: vi.fn().mockResolvedValue(undefined),
      IsMaximised: vi.fn().mockResolvedValue(false),
    };
    runtime.named.set(name, created);
    return created;
  },
}));

vi.mock('@wailsio/runtime', () => ({
  System: {
    IsWindows: () => runtime.isWindows,
    IsMac: () => runtime.isMac,
  },
  Window: {
    Get: (name: string) => runtime.window(name),
  },
}));

describe('nativeWindowNameForUrl', () => {
  it('names the shell window for the desktop route', () => {
    expect(nativeWindowNameForUrl('wails://localhost/#/')).toBe('app');
  });

  it('names the tray panel window for the tray route', () => {
    expect(nativeWindowNameForUrl('wails://localhost/#/tray')).toBe('tray-panel');
  });

  it('names the solo window Go opened, prefix and all', () => {
    expect(nativeWindowNameForUrl('wails://localhost/#/w/telemetry')).toBe('app-view-telemetry');
  });

  it('names the same solo window whether or not a pane is on the route', () => {
    expect(nativeWindowNameForUrl('http://127.0.0.1:9245/?lthn-ws=ws://x/#/w/control/models')).toBe(
      'app-view-control',
    );
  });

  it('falls back to the shell rather than inventing a window', () => {
    expect(nativeWindowNameForUrl('wails://localhost/')).toBe('app');
    expect(nativeWindowNameForUrl('wails://localhost/#/w/')).toBe('app');
  });
});

describe('DesktopWindowBridgeService', () => {
  let offline: boolean;

  const bridge = (name: string): DesktopWindowBridgeService => {
    TestBed.configureTestingModule({
      providers: [
        { provide: ConnectionManagerService, useValue: { offline: () => offline } },
        { provide: NATIVE_WINDOW_NAME, useValue: name },
      ],
    });
    return TestBed.inject(DesktopWindowBridgeService);
  };

  beforeEach(() => {
    runtime.named.clear();
    runtime.isWindows = false;
    runtime.isMac = true;
    offline = false;
  });

  it('drives the window it is in, not whichever window Go answers first', async () => {
    // A custom transport carries no window identity, so an unnamed runtime
    // call lands on the first window in the application — the shell. A solo
    // window's close button would then close the desktop it had left.
    const service = bridge('app-view-telemetry');
    await vi.waitFor(() => expect(service.available()).toBe(true));

    await service.minimise();
    await service.toggleMaximise();
    await service.close();

    const solo = runtime.window('app-view-telemetry');
    expect(solo.Minimise).toHaveBeenCalledOnce();
    expect(solo.ToggleMaximise).toHaveBeenCalledOnce();
    expect(solo.Close).toHaveBeenCalledOnce();
    expect(runtime.named.has('app')).toBe(false);
    expect(service.windowName).toBe('app-view-telemetry');
  });

  it('reads the maximised state of that same window', async () => {
    runtime.window('app-view-chat').IsMaximised.mockResolvedValue(true);
    const service = bridge('app-view-chat');

    await vi.waitFor(() => expect(service.maximised()).toBe(true));
  });

  it('puts the controls where the platform puts them', async () => {
    runtime.isWindows = true;
    runtime.isMac = false;
    const service = bridge('app');

    await vi.waitFor(() => expect(service.side()).toBe('right'));
    expect(document.documentElement.classList.contains('platform-windows')).toBe(true);
    document.documentElement.classList.remove('platform-windows');
  });

  it('says why a control did nothing rather than failing silently', async () => {
    const service = bridge('app-view-chat');
    await vi.waitFor(() => expect(service.available()).toBe(true));
    runtime.window('app-view-chat').Close.mockRejectedValue(new Error('window not found'));

    await service.close();

    expect(service.lastError()).toContain('window not found');
  });

  it('drives nothing in the browser demo, where there is no window', async () => {
    offline = true;
    const service = bridge('app-view-chat');

    await service.close();

    expect(service.available()).toBe(false);
    expect(runtime.named.has('app-view-chat')).toBe(false);
  });
});

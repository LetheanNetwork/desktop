import { TestBed } from '@angular/core/testing';
import type { Mock } from 'vitest';
import { WindowManagerService } from './desktop/window-manager.service';
import {
  MOBILE_RUNTIME_TRANSPORT,
  MobileRuntimeService,
  type LetheanPlatform,
  type MobileRuntimeTransport,
} from './mobile-runtime.service';

describe('MobileRuntimeService', () => {
  let listeners: Map<string, (payload: unknown) => void>;
  let emit: Mock<
    (name: string, payload: Record<string, unknown>) => Promise<void>
  >;
  let service: MobileRuntimeService;
  let windows: Pick<WindowManagerService, 'setView' | 'setDevice'>;

  const createService = (platform: LetheanPlatform): MobileRuntimeService => {
    listeners = new Map();
    emit = vi.fn((_name: string, _payload: Record<string, unknown>) => Promise.resolve());
    const transport: MobileRuntimeTransport = {
      platform: () => platform,
      on(name, handler): () => void {
        listeners.set(name, handler);
        return () => listeners.delete(name);
      },
      emit(name, payload): Promise<void> {
        return emit(name, payload);
      },
    };
    windows = {
      setView: vi.fn(),
      setDevice: vi.fn(),
    };

    TestBed.configureTestingModule({
      providers: [
        { provide: MOBILE_RUNTIME_TRANSPORT, useValue: transport },
        { provide: WindowManagerService, useValue: windows },
      ],
    });
    return TestBed.inject(MobileRuntimeService);
  };

  beforeEach(async () => {
    service = createService('ios');
    await service.ready;
  });

  afterEach(() => {
    service.destroy();
    document.documentElement.removeAttribute('data-platform');
    for (const side of ['top', 'right', 'bottom', 'left']) {
      document.documentElement.style.removeProperty(`--safe-area-${side}`);
    }
  });

  it('detects iOS and requests initial native state', () => {
    expect(service.platform()).toBe('ios');
    expect(document.documentElement.dataset['platform']).toBe('ios');
    expect(windows.setView).toHaveBeenCalledWith('device');
    expect(windows.setDevice).toHaveBeenCalledWith('small');
    expect(emit.mock.calls).toEqual(
      expect.arrayContaining([
        ['common:getSafeArea', {}],
        ['common:getPower', {}],
        ['common:getNetwork', {}],
        ['common:getAppInfo', {}],
        ['common:getOrientation', {}],
      ]),
    );
  });

  it.each(['darwin', 'windows', 'linux'] as const)(
    'marks %s as native desktop without forcing device presentation',
    async (platform) => {
      service.destroy();
      TestBed.resetTestingModule();
      service = createService(platform);
      await service.ready;

      expect(document.documentElement.dataset['platform']).toBe(platform);
      expect(windows.setView).not.toHaveBeenCalled();
      expect(windows.setDevice).not.toHaveBeenCalled();
    },
  );

  it.each([
    ['ipad', 'large'],
    ['android', 'small'],
  ] as const)('uses the %s device presentation with a %s frame', async (platform, device) => {
    service.destroy();
    TestBed.resetTestingModule();
    service = createService(platform);
    await service.ready;

    expect(document.documentElement.dataset['platform']).toBe(platform);
    expect(windows.setView).toHaveBeenCalledWith('device');
    expect(windows.setDevice).toHaveBeenCalledWith(device);
  });

  it('tracks lifecycle, power, network, lock and memory events', () => {
    listeners.get('lthn:app:background')?.({});
    listeners.get('lthn:app:inactive')?.({});
    listeners.get('lthn:system:lock')?.({ locked: true });
    listeners.get('lthn:system:low-memory')?.({});
    listeners.get('lthn:system:battery')?.({
      level: 0.42,
      state: 'charging',
      lowPowerMode: true,
    });
    listeners.get('lthn:system:network')?.({ connected: true, type: 'wifi' });

    expect(service.foreground()).toBe(false);
    expect(service.active()).toBe(false);
    expect(service.locked()).toBe(true);
    expect(service.lowMemoryPulses()).toBe(1);
    expect(service.battery()).toEqual({
      level: 0.42,
      state: 'charging',
      charging: true,
      lowPowerMode: true,
    });
    expect(service.network()).toEqual({ connected: true, type: 'wifi' });

    listeners.get('lthn:app:foreground')?.({});
    listeners.get('lthn:app:active')?.({});
    expect(service.foreground()).toBe(true);
    expect(service.active()).toBe(true);
  });

  it('publishes safe-area CSS variables and typed native actions', async () => {
    listeners.get('common:safeArea')?.({ top: 47, right: 1, bottom: 34, left: 2 });

    expect(service.safeArea()).toEqual({ top: 47, right: 1, bottom: 34, left: 2 });
    expect(document.documentElement.style.getPropertyValue('--safe-area-top')).toBe('47px');
    expect(document.documentElement.style.getPropertyValue('--safe-area-bottom')).toBe('34px');

    await service.share('Sovereign AI', 'https://lethean.io');
    await service.setBrightness(4);
    await service.startBackgroundWork('Lethean', 'Local inference is running');

    expect(emit).toHaveBeenCalledWith('common:share', {
      text: 'Sovereign AI',
      url: 'https://lethean.io',
    });
    expect(emit).toHaveBeenCalledWith('common:setBrightness', { value: 1 });
    expect(emit).toHaveBeenCalledWith('common:startForegroundService', {
      title: 'Lethean',
      text: 'Local inference is running',
    });
  });
});

import { signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { ConnectionManagerService } from '../connection-manager.service';
import { SurfaceBridgeService } from './surfaces/surface-bridge.service';
import {
  DesktopPermissionsBridgeService,
  PERMISSIONS_METHODS,
} from './desktop-permissions-bridge.service';

describe('DesktopPermissionsBridgeService', () => {
  const offline = signal(false);
  const surface = {
    call: vi.fn(),
  };

  beforeEach(() => {
    offline.set(false);
    vi.clearAllMocks();
    TestBed.configureTestingModule({
      providers: [
        { provide: SurfaceBridgeService, useValue: surface },
        {
          provide: ConnectionManagerService,
          useValue: { offline: offline.asReadonly() },
        },
      ],
    });
  });

  afterEach(() => TestBed.resetTestingModule());

  it('reads exact policy and host states without requesting permission', async () => {
    surface.call.mockResolvedValue([
      { id: 'microphone', policy: 'default', host: 'unsupported' },
      { id: 'camera', policy: 'allow', host: 'unknown' },
      { id: 'geolocation', policy: 'deny', host: 'restricted' },
      { id: 'notifications', policy: 'default', host: 'granted' },
      { id: 'clipboard-read', policy: 'default', host: 'denied' },
    ]);
    const service = TestBed.inject(DesktopPermissionsBridgeService);

    const snapshots = await service.status();

    expect(surface.call).toHaveBeenCalledOnce();
    expect(surface.call).toHaveBeenCalledWith(PERMISSIONS_METHODS.status);
    expect(snapshots[3]).toEqual({
      id: 'notifications',
      policy: 'default',
      host: 'granted',
    });
  });

  it('requests only an allowlisted permission id', async () => {
    surface.call.mockResolvedValue({
      id: 'notifications',
      policy: 'default',
      host: 'granted',
    });
    const service = TestBed.inject(DesktopPermissionsBridgeService);

    await service.request('notifications');

    expect(surface.call).toHaveBeenCalledWith(PERMISSIONS_METHODS.request, ['notifications']);
    await expect(service.request('filesystem-root')).rejects.toThrow('unknown');
  });

  it('rejects missing, duplicate, extra, and execution-bearing snapshots', async () => {
    const service = TestBed.inject(DesktopPermissionsBridgeService);
    const valid = [
      { id: 'microphone', policy: 'default', host: 'unsupported' },
      { id: 'camera', policy: 'default', host: 'unsupported' },
      { id: 'geolocation', policy: 'default', host: 'unsupported' },
      { id: 'notifications', policy: 'default', host: 'unsupported' },
      { id: 'clipboard-read', policy: 'default', host: 'unsupported' },
    ];

    for (const payload of [
      valid.slice(0, 4),
      [...valid.slice(0, 4), valid[0]],
      valid.map((entry, index) => (index === 0 ? { ...entry, path: '/Users/private' } : entry)),
      valid.map((entry, index) => (index === 0 ? { ...entry, host: 'invented' } : entry)),
    ]) {
      surface.call.mockResolvedValueOnce(payload);
      await expect(service.status()).rejects.toThrow();
    }
  });

  it('uses deterministic unsupported demo state without a Wails call', async () => {
    offline.set(true);
    const service = TestBed.inject(DesktopPermissionsBridgeService);

    const snapshots = await service.status();

    expect(snapshots).toHaveLength(5);
    expect(snapshots.every(({ host }) => host === 'unsupported')).toBe(true);
    expect(surface.call).not.toHaveBeenCalled();
    await expect(service.request('notifications')).rejects.toThrow('offline demo mode');
    expect(surface.call).not.toHaveBeenCalled();
  });
});

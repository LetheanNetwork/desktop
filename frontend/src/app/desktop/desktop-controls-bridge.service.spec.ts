import { signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { ConnectionManagerService } from '../connection-manager.service';
import { DesktopControlChange, DesktopControlSnapshot } from '../store/desktop-controls.models';
import { DesktopControlsBridgeService } from './desktop-controls-bridge.service';
import { SurfaceBridgeService } from './surfaces/surface-bridge.service';

const snapshot: DesktopControlSnapshot = {
  controls: [
    {
      key: 'desktop.wails.window.main.width',
      group: 'Window',
      label: 'Window width',
      description: 'Width in pixels.',
      kind: 'number',
      value: 1280,
      defaultValue: 1440,
      configured: true,
      live: true,
      restartRequired: false,
      minimum: 800,
      maximum: 3840,
      step: 10,
    },
  ],
};

describe('DesktopControlsBridgeService', () => {
  const offline = signal(false);
  let surface: { call: ReturnType<typeof vi.fn> };
  let service: DesktopControlsBridgeService;

  beforeEach(() => {
    offline.set(false);
    surface = { call: vi.fn() };
    TestBed.configureTestingModule({
      providers: [
        DesktopControlsBridgeService,
        { provide: SurfaceBridgeService, useValue: surface },
        {
          provide: ConnectionManagerService,
          useValue: { offline: offline.asReadonly() },
        },
      ],
    });
    service = TestBed.inject(DesktopControlsBridgeService);
  });

  it('loads and normalises the curated Go control catalogue without provider paths', async () => {
    surface.call.mockResolvedValue({
      controls: [
        {
          key: 'desktop.wails.window.main.width',
          group: 'Window',
          label: 'Window width',
          description: 'Width in pixels.',
          kind: 'number',
          value: 1280,
          default: 1440,
          configured: true,
          live: true,
          restart_required: false,
          minimum: 800,
          maximum: 3840,
          step: 10,
        },
      ],
    });

    await expect(service.settings()).resolves.toEqual(snapshot);
    expect(surface.call).toHaveBeenCalledWith(
      'dappco.re/lthn/desktop/pkg/appconfig.Service.Settings',
    );
  });

  it('persists one bounded draft through SetMany', async () => {
    surface.call.mockResolvedValue({ controls: [] });
    const changes: readonly DesktopControlChange[] = [
      { key: 'desktop.theme.interface', value: 'light' },
      { key: 'desktop.theme.reduce_motion', value: true },
    ];

    await service.setMany(changes);

    expect(surface.call).toHaveBeenCalledTimes(1);
    expect(surface.call).toHaveBeenCalledWith(
      'dappco.re/lthn/desktop/pkg/appconfig.Service.SetMany',
      [changes],
    );
  });

  it('rejects malformed and execution-shaped changes before Wails', async () => {
    await expect(
      service.setMany([{ key: 'desktop.theme.interface;exec', value: 'light' }]),
    ).rejects.toThrow('desktop control');
    await expect(
      service.setMany(
        Array.from({ length: 65 }, (_, index) => ({
          key: `desktop.safe.control_${index}`,
          value: true,
        })),
      ),
    ).rejects.toThrow('Too many');
    expect(surface.call).not.toHaveBeenCalled();
  });

  it('uses isolated in-memory demo settings while explicitly offline', async () => {
    offline.set(true);

    const before = await service.settings();
    const after = await service.setMany([{ key: 'desktop.theme.interface', value: 'light' }]);

    expect(before.controls.length).toBeGreaterThan(0);
    expect(after.controls.find(({ key }) => key === 'desktop.theme.interface')?.value).toBe(
      'light',
    );
    expect(surface.call).not.toHaveBeenCalled();
  });

  it('rejects malformed snapshots instead of inventing connected settings', async () => {
    surface.call.mockResolvedValue({ controls: 'not-an-array' });

    await expect(service.settings()).rejects.toThrow('desktop control catalogue');
  });
});

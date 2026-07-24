import { TestBed } from '@angular/core/testing';
import { SurfaceBridgeService } from './surfaces/surface-bridge.service';
import { DesktopControlsBridgeService } from './desktop-controls-bridge.service';

describe('DesktopControlsBridgeService', () => {
  let surface: { call: ReturnType<typeof vi.fn> };
  let service: DesktopControlsBridgeService;

  beforeEach(() => {
    surface = { call: vi.fn() };
    TestBed.configureTestingModule({
      providers: [
        DesktopControlsBridgeService,
        { provide: SurfaceBridgeService, useValue: surface },
      ],
    });
    service = TestBed.inject(DesktopControlsBridgeService);
  });

  it('loads and normalises the curated Go control catalogue', async () => {
    surface.call.mockResolvedValue({
      config_path: '/tmp/Lethean/conf/lthn.yaml',
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

    await expect(service.settings()).resolves.toEqual({
      configPath: '/tmp/Lethean/conf/lthn.yaml',
      controls: [
        expect.objectContaining({
          key: 'desktop.wails.window.main.width',
          value: 1280,
          defaultValue: 1440,
          minimum: 800,
          restartRequired: false,
        }),
      ],
    });
    expect(surface.call).toHaveBeenCalledWith(
      'dappco.re/lthn/desktop/pkg/appconfig.Service.Settings',
    );
  });

  it('persists one value through the appconfig Wails service', async () => {
    surface.call.mockResolvedValue({ config_path: '/tmp/lthn.yaml', controls: [] });

    await service.set('desktop.theme.interface', 'light');

    expect(surface.call).toHaveBeenCalledWith('dappco.re/lthn/desktop/pkg/appconfig.Service.Set', [
      'desktop.theme.interface',
      'light',
    ]);
  });

  it('rejects malformed snapshots instead of inventing settings', async () => {
    surface.call.mockResolvedValue({ controls: 'not-an-array' });

    await expect(service.settings()).rejects.toThrow('desktop control catalogue');
  });
});

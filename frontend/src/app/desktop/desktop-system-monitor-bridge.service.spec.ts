// SPDX-License-Identifier: EUPL-1.2

import { signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { ConnectionManagerService } from '../connection-manager.service';
import { DesktopSystemMonitorBridgeService } from './desktop-system-monitor-bridge.service';
import { SurfaceBridgeService } from './surfaces/surface-bridge.service';

describe('DesktopSystemMonitorBridgeService', () => {
  const offline = signal(false);
  const surface = { call: vi.fn() };
  let bridge: DesktopSystemMonitorBridgeService;

  beforeEach(() => {
    offline.set(false);
    vi.clearAllMocks();
    TestBed.configureTestingModule({
      providers: [
        DesktopSystemMonitorBridgeService,
        { provide: ConnectionManagerService, useValue: { offline: offline.asReadonly() } },
        { provide: SurfaceBridgeService, useValue: surface },
      ],
    });
    bridge = TestBed.inject(DesktopSystemMonitorBridgeService);
  });

  afterEach(() => TestBed.resetTestingModule());

  it('normalises one complete bounded host snapshot', async () => {
    surface.call.mockResolvedValue({
      observed_at: '2026-07-31T12:00:05Z',
      source: 'macOS host APIs',
      platform: 'darwin',
      architecture: 'arm64',
      cpu: { logical_cores: 10, usage_percent: 37.5 },
      memory: { total_bytes: 34_359_738_368, used_bytes: 18_400_000_000 },
      network: {
        received_bytes: 10_000,
        sent_bytes: 5_000,
        received_bytes_per_second: 1_200,
        sent_bytes_per_second: 320,
      },
      power: { source: 'ac', battery_percent: 81, charging: true },
    });

    await expect(bridge.snapshot()).resolves.toEqual({
      observedAt: '2026-07-31T12:00:05Z',
      source: 'macOS host APIs',
      platform: 'darwin',
      architecture: 'arm64',
      cpu: { logicalCores: 10, usagePercent: 37.5 },
      memory: { totalBytes: 34_359_738_368, usedBytes: 18_400_000_000 },
      network: {
        receivedBytes: 10_000,
        sentBytes: 5_000,
        receivedBytesPerSecond: 1_200,
        sentBytesPerSecond: 320,
      },
      power: { source: 'ac', batteryPercent: 81, charging: true },
    });
    expect(surface.call).toHaveBeenCalledWith(
      'dappco.re/lthn/desktop/pkg/telemetry.Service.CurrentHostSnapshot',
    );
  });

  it('keeps unsupported optional metrics absent instead of manufacturing zeroes', async () => {
    surface.call.mockResolvedValue({
      observed_at: '2026-07-31T12:00:05Z',
      source: 'Portable Go runtime',
      platform: 'linux',
      architecture: 'arm64',
      cpu: { logical_cores: 8 },
    });

    await expect(bridge.snapshot()).resolves.toEqual({
      observedAt: '2026-07-31T12:00:05Z',
      source: 'Portable Go runtime',
      platform: 'linux',
      architecture: 'arm64',
      cpu: { logicalCores: 8 },
    });
  });

  it.each([
    ['CPU above one hundred percent', { cpu: { logical_cores: 10, usage_percent: 101 } }],
    ['used memory above total', { memory: { total_bytes: 1_000, used_bytes: 1_001 } }],
    ['unbounded power source', { power: { source: 'wall' } }],
    [
      'negative network rate',
      { network: { received_bytes: 1, sent_bytes: 1, received_bytes_per_second: -1 } },
    ],
  ])('rejects %s', async (_name, invalidPart) => {
    surface.call.mockResolvedValue({
      observed_at: '2026-07-31T12:00:05Z',
      source: 'macOS host APIs',
      platform: 'darwin',
      architecture: 'arm64',
      cpu: { logical_cores: 10, usage_percent: 20 },
      ...invalidPart,
    });

    await expect(bridge.snapshot()).rejects.toThrow('host system snapshot');
  });

  it('makes no Wails call in explicit offline demo mode', async () => {
    offline.set(true);

    await expect(bridge.snapshot()).rejects.toThrow('offline demo mode');
    expect(surface.call).not.toHaveBeenCalled();
  });
});

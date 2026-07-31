// SPDX-License-Identifier: EUPL-1.2

import { signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { ConnectionManagerService } from '../connection-manager.service';
import type { FilesCatalogueView } from './apps/files/files-view.models';
import { DesktopFilesBridgeService } from './desktop-files-bridge.service';
import { DesktopSystemMonitorBridgeService } from './desktop-system-monitor-bridge.service';
import type { HostSystemSnapshot } from './desktop-system-monitor.models';
import { DesktopSystemMonitorResource } from './desktop-system-monitor-resource.service';

const HOST_SAMPLE: HostSystemSnapshot = {
  observedAt: '2026-07-31T12:00:05Z',
  source: 'macOS host APIs',
  platform: 'darwin',
  architecture: 'arm64',
  cpu: { logicalCores: 10, usagePercent: 30 },
  memory: { totalBytes: 32 * 1_024 ** 3, usedBytes: 18.4 * 1_024 ** 3 },
  network: {
    receivedBytes: 10_000,
    sentBytes: 5_000,
    receivedBytesPerSecond: 1_200,
    sentBytesPerSecond: 320,
  },
  power: { source: 'ac', batteryPercent: 81, charging: true },
};

const FILES: FilesCatalogueView = {
  mounts: [
    {
      id: 'documents',
      name: 'Documents',
      kind: 'local',
      icon: 'folder',
      brand: false,
      capabilities: {
        list: true,
        preview: true,
        open: true,
        reveal: true,
        createDirectory: true,
        write: true,
        rename: true,
        copyFrom: true,
        copyTo: true,
        move: true,
        trash: true,
        restore: true,
        delete: true,
      },
      capacity: { totalBytes: 512 * 1_024 ** 3, freeBytes: 218 * 1_024 ** 3 },
    },
  ],
  favourites: [],
  recent: [],
};

describe('DesktopSystemMonitorResource', () => {
  const offline = signal(false);
  const bridge = { snapshot: vi.fn<() => Promise<HostSystemSnapshot>>() };
  const files = { listMounts: vi.fn<() => Promise<FilesCatalogueView>>() };
  let resource: DesktopSystemMonitorResource;

  beforeEach(() => {
    vi.useFakeTimers();
    vi.clearAllMocks();
    offline.set(false);
    bridge.snapshot.mockResolvedValue(HOST_SAMPLE);
    files.listMounts.mockResolvedValue(FILES);
    TestBed.configureTestingModule({
      providers: [
        DesktopSystemMonitorResource,
        { provide: DesktopSystemMonitorBridgeService, useValue: bridge },
        { provide: DesktopFilesBridgeService, useValue: files },
        { provide: ConnectionManagerService, useValue: { offline: offline.asReadonly() } },
      ],
    });
    resource = TestBed.inject(DesktopSystemMonitorResource);
  });

  afterEach(() => {
    TestBed.resetTestingModule();
    vi.useRealTimers();
  });

  it('shares one poller and one authoritative snapshot across window consumers', async () => {
    const disconnectControl = resource.connect();
    const disconnectTelemetry = resource.connect();
    await resource.refresh();

    expect(bridge.snapshot).toHaveBeenCalledTimes(1);
    expect(files.listMounts).toHaveBeenCalledTimes(1);
    expect(resource.resource()).toMatchObject({
      mode: 'connected',
      state: 'live',
      source: 'macOS host APIs',
      value: {
        cpu: { logicalCores: 10, usagePercent: 30 },
        storage: { totalBytes: 512 * 1_024 ** 3, freeBytes: 218 * 1_024 ** 3 },
      },
    });
    expect(vi.getTimerCount()).toBe(1);

    disconnectControl();
    expect(vi.getTimerCount()).toBe(1);
    disconnectTelemetry();
    expect(vi.getTimerCount()).toBe(0);
  });

  it('keeps bounded host histories in the RxJS store', async () => {
    bridge.snapshot
      .mockResolvedValueOnce({ ...HOST_SAMPLE, cpu: { logicalCores: 10, usagePercent: 20 } })
      .mockResolvedValueOnce({
        ...HOST_SAMPLE,
        observedAt: '2026-07-31T12:00:10Z',
        cpu: { logicalCores: 10, usagePercent: 35 },
        network: {
          ...HOST_SAMPLE.network!,
          receivedBytesPerSecond: 2_400,
          sentBytesPerSecond: 640,
        },
      });
    const disconnect = resource.connect();
    await resource.refresh();
    await resource.refresh();

    expect(resource.resource().value).toMatchObject({
      cpuHistory: [20, 35],
      networkReceivedHistory: [1_200, 2_400],
      networkSentHistory: [320, 640],
    });
    expect(resource.resource().value?.memoryHistory).toHaveLength(2);
    disconnect();
  });

  it('retains the last good snapshot as stale after a later host failure', async () => {
    const disconnect = resource.connect();
    await resource.refresh();
    const lastGood = resource.resource().value;
    bridge.snapshot.mockRejectedValueOnce(new Error('native private detail'));

    await resource.refresh();

    expect(resource.resource()).toMatchObject({
      state: 'stale',
      value: lastGood,
      refreshing: false,
      canRetry: true,
      error: 'Live system information is unavailable.',
    });
    disconnect();
  });

  it('keeps host data live when optional Files capacity is unavailable', async () => {
    files.listMounts.mockRejectedValueOnce(new Error('Files unavailable'));
    const disconnect = resource.connect();

    await resource.refresh();

    expect(resource.resource()).toMatchObject({ state: 'mixed' });
    expect(resource.resource().value?.storage).toBeUndefined();
    expect(resource.resource().value?.memory).toEqual(HOST_SAMPLE.memory);
    disconnect();
  });

  it('uses deterministic demo data without a timer or any Wails bridge call', () => {
    TestBed.resetTestingModule();
    offline.set(true);
    TestBed.configureTestingModule({
      providers: [
        DesktopSystemMonitorResource,
        { provide: DesktopSystemMonitorBridgeService, useValue: bridge },
        { provide: DesktopFilesBridgeService, useValue: files },
        { provide: ConnectionManagerService, useValue: { offline: offline.asReadonly() } },
      ],
    });
    resource = TestBed.inject(DesktopSystemMonitorResource);

    const disconnect = resource.connect();

    expect(resource.resource()).toMatchObject({
      mode: 'demo',
      state: 'demo',
      source: 'Lethean demo fixture · Host system',
      value: {
        cpu: { logicalCores: 10, usagePercent: 34 },
        power: { source: 'ac', batteryPercent: 81, charging: true },
      },
    });
    expect(bridge.snapshot).not.toHaveBeenCalled();
    expect(files.listMounts).not.toHaveBeenCalled();
    expect(vi.getTimerCount()).toBe(0);
    disconnect();
  });

  it('polls every five seconds only while at least one consumer is connected', async () => {
    const disconnect = resource.connect();
    await resource.refresh();
    bridge.snapshot.mockClear();

    await vi.advanceTimersByTimeAsync(4_999);
    expect(bridge.snapshot).not.toHaveBeenCalled();
    await vi.advanceTimersByTimeAsync(1);
    expect(bridge.snapshot).toHaveBeenCalledOnce();

    disconnect();
    bridge.snapshot.mockClear();
    await vi.advanceTimersByTimeAsync(5_000);
    expect(bridge.snapshot).not.toHaveBeenCalled();
  });
});

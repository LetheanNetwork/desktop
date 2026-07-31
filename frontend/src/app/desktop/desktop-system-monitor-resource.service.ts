// SPDX-License-Identifier: EUPL-1.2

import { Injectable, OnDestroy, Signal, inject } from '@angular/core';
import { toSignal } from '@angular/core/rxjs-interop';
import { BehaviorSubject, distinctUntilChanged, shareReplay } from 'rxjs';
import { ConnectionManagerService } from '../connection-manager.service';
import type { FilesCatalogueView } from './apps/files/files-view.models';
import {
  beginDesktopDataRefresh,
  createConnectedResource,
  createDemoResource,
  rejectDesktopData,
  resolveDesktopData,
  type DesktopDataResource,
} from './desktop-data-resource';
import { DesktopFilesBridgeService } from './desktop-files-bridge.service';
import { DesktopSystemMonitorBridgeService } from './desktop-system-monitor-bridge.service';
import {
  SYSTEM_MONITOR_DEMO_SNAPSHOT,
  SYSTEM_MONITOR_DEMO_SOURCE,
} from './desktop-system-monitor-demo.data';
import type {
  HostSystemSnapshot,
  SystemMonitorSnapshot,
  SystemStorageReading,
} from './desktop-system-monitor.models';

const SYSTEM_MONITOR_INITIAL_SOURCE = 'Local host system';
const SYSTEM_MONITOR_UNAVAILABLE = 'Live system information is unavailable.';
const SYSTEM_MONITOR_POLL_MS = 5_000;
const MAX_HISTORY_SAMPLES = 180;

@Injectable({ providedIn: 'root' })
export class DesktopSystemMonitorResource implements OnDestroy {
  private readonly bridge = inject(DesktopSystemMonitorBridgeService);
  private readonly files = inject(DesktopFilesBridgeService);
  private readonly connection = inject(ConnectionManagerService);
  private readonly initialResource: DesktopDataResource<SystemMonitorSnapshot> =
    this.connection.offline()
      ? createDemoResource(cloneDemoSnapshot(), SYSTEM_MONITOR_DEMO_SOURCE)
      : createConnectedResource<SystemMonitorSnapshot>(SYSTEM_MONITOR_INITIAL_SOURCE);
  private readonly resourceSubject = new BehaviorSubject(this.initialResource);

  readonly resource$ = this.resourceSubject
    .asObservable()
    .pipe(distinctUntilChanged(), shareReplay({ bufferSize: 1, refCount: true }));
  readonly resource: Signal<DesktopDataResource<SystemMonitorSnapshot>> = toSignal(this.resource$, {
    initialValue: this.initialResource,
  });

  private consumers = 0;
  private generation = 0;
  private pollHandle: ReturnType<typeof setInterval> | null = null;
  private refreshPromise: Promise<void> | null = null;
  private destroyed = false;

  connect(): () => void {
    if (this.destroyed) return () => undefined;
    this.consumers++;
    if (this.consumers === 1 && !this.connection.offline()) {
      this.generation++;
      this.pollHandle = setInterval(() => void this.refresh(), SYSTEM_MONITOR_POLL_MS);
      void this.refresh();
    }

    let connected = true;
    return () => {
      if (!connected) return;
      connected = false;
      this.consumers = Math.max(0, this.consumers - 1);
      if (this.consumers === 0) this.teardownConnectedResources();
    };
  }

  async refresh(): Promise<void> {
    if (this.destroyed || this.connection.offline() || this.consumers === 0) return;
    if (this.refreshPromise) return this.refreshPromise;

    const current = this.resourceSubject.value;
    if (current.mode !== 'connected') return;
    const refreshing = current.refreshing
      ? current
      : beginDesktopDataRefresh(current, Date.now(), SYSTEM_MONITOR_POLL_MS * 2);
    this.resourceSubject.next(refreshing);
    const generation = this.generation;

    let task: Promise<void>;
    task = Promise.allSettled([this.bridge.snapshot(), this.files.listMounts()] as const)
      .then(([hostResult, filesResult]) => {
        if (!this.isCurrent(generation)) return;
        if (hostResult.status === 'rejected') throw hostResult.reason;
        const resource = this.resourceSubject.value;
        if (resource.mode !== 'connected' || !resource.refreshing) return;
        const storage =
          filesResult.status === 'fulfilled' ? commonStorage(filesResult.value) : undefined;
        const snapshot = mergeSnapshot(hostResult.value, storage, resource.value);
        this.resourceSubject.next(
          resolveDesktopData(
            resource,
            snapshot,
            filesResult.status === 'fulfilled' ? 'live' : 'mixed',
            snapshot.source,
            Date.parse(snapshot.observedAt),
          ),
        );
      })
      .catch(() => {
        if (!this.isCurrent(generation)) return;
        const resource = this.resourceSubject.value;
        if (resource.mode !== 'connected' || !resource.refreshing) return;
        this.resourceSubject.next(rejectDesktopData(resource, SYSTEM_MONITOR_UNAVAILABLE));
      })
      .finally(() => {
        if (this.refreshPromise === task) this.refreshPromise = null;
      });
    this.refreshPromise = task;
    return task;
  }

  ngOnDestroy(): void {
    this.destroyed = true;
    this.consumers = 0;
    this.teardownConnectedResources();
    this.resourceSubject.complete();
  }

  private isCurrent(generation: number): boolean {
    return !this.destroyed && this.consumers > 0 && generation === this.generation;
  }

  private teardownConnectedResources(): void {
    this.generation++;
    if (this.pollHandle !== null) clearInterval(this.pollHandle);
    this.pollHandle = null;
    this.refreshPromise = null;
    const resource = this.resourceSubject.value;
    if (resource.mode === 'connected' && resource.refreshing) {
      this.resourceSubject.next(rejectDesktopData(resource, SYSTEM_MONITOR_UNAVAILABLE));
    }
  }
}

function mergeSnapshot(
  host: HostSystemSnapshot,
  storage: SystemStorageReading | undefined,
  previous: SystemMonitorSnapshot | null,
): SystemMonitorSnapshot {
  const memoryPercent = host.memory
    ? (host.memory.usedBytes / host.memory.totalBytes) * 100
    : undefined;
  return {
    ...host,
    ...(storage ? { storage } : {}),
    cpuHistory: appendHistory(previous?.cpuHistory ?? [], host.cpu.usagePercent),
    memoryHistory: appendHistory(previous?.memoryHistory ?? [], memoryPercent),
    networkReceivedHistory: appendHistory(
      previous?.networkReceivedHistory ?? [],
      host.network?.receivedBytesPerSecond,
    ),
    networkSentHistory: appendHistory(
      previous?.networkSentHistory ?? [],
      host.network?.sentBytesPerSecond,
    ),
  };
}

function appendHistory(history: readonly number[], value: number | undefined): readonly number[] {
  return value === undefined ? [...history] : [...history, value].slice(-MAX_HISTORY_SAMPLES);
}

function commonStorage(catalogue: FilesCatalogueView): SystemStorageReading | undefined {
  const capacities = catalogue.mounts.flatMap(({ capacity }) => (capacity ? [capacity] : []));
  const first = capacities[0];
  if (!first) return undefined;
  return capacities.every(
    ({ totalBytes, freeBytes }) => totalBytes === first.totalBytes && freeBytes === first.freeBytes,
  )
    ? { totalBytes: first.totalBytes, freeBytes: first.freeBytes }
    : undefined;
}

function cloneDemoSnapshot(): SystemMonitorSnapshot {
  return {
    ...SYSTEM_MONITOR_DEMO_SNAPSHOT,
    cpu: { ...SYSTEM_MONITOR_DEMO_SNAPSHOT.cpu },
    memory: SYSTEM_MONITOR_DEMO_SNAPSHOT.memory
      ? { ...SYSTEM_MONITOR_DEMO_SNAPSHOT.memory }
      : undefined,
    network: SYSTEM_MONITOR_DEMO_SNAPSHOT.network
      ? { ...SYSTEM_MONITOR_DEMO_SNAPSHOT.network }
      : undefined,
    power: SYSTEM_MONITOR_DEMO_SNAPSHOT.power
      ? { ...SYSTEM_MONITOR_DEMO_SNAPSHOT.power }
      : undefined,
    storage: SYSTEM_MONITOR_DEMO_SNAPSHOT.storage
      ? { ...SYSTEM_MONITOR_DEMO_SNAPSHOT.storage }
      : undefined,
    cpuHistory: [...SYSTEM_MONITOR_DEMO_SNAPSHOT.cpuHistory],
    memoryHistory: [...SYSTEM_MONITOR_DEMO_SNAPSHOT.memoryHistory],
    networkReceivedHistory: [...SYSTEM_MONITOR_DEMO_SNAPSHOT.networkReceivedHistory],
    networkSentHistory: [...SYSTEM_MONITOR_DEMO_SNAPSHOT.networkSentHistory],
  };
}

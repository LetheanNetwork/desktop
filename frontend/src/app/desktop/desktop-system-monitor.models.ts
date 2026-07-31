// SPDX-License-Identifier: EUPL-1.2

export type SystemPowerSource = 'unknown' | 'ac' | 'battery';

export interface HostCPUReading {
  readonly logicalCores: number;
  readonly usagePercent?: number;
}

export interface HostMemoryReading {
  readonly totalBytes: number;
  readonly usedBytes: number;
}

export interface HostNetworkReading {
  readonly receivedBytes: number;
  readonly sentBytes: number;
  readonly receivedBytesPerSecond?: number;
  readonly sentBytesPerSecond?: number;
}

export interface HostPowerReading {
  readonly source: SystemPowerSource;
  readonly batteryPercent?: number;
  readonly charging?: boolean;
}

export interface HostSystemSnapshot {
  readonly observedAt: string;
  readonly source: string;
  readonly platform: string;
  readonly architecture: string;
  readonly cpu: HostCPUReading;
  readonly memory?: HostMemoryReading;
  readonly network?: HostNetworkReading;
  readonly power?: HostPowerReading;
}

export interface SystemStorageReading {
  readonly totalBytes: number;
  readonly freeBytes: number;
}

export interface SystemMonitorSnapshot extends HostSystemSnapshot {
  readonly storage?: SystemStorageReading;
  readonly cpuHistory: readonly number[];
  readonly memoryHistory: readonly number[];
  readonly networkReceivedHistory: readonly number[];
  readonly networkSentHistory: readonly number[];
}

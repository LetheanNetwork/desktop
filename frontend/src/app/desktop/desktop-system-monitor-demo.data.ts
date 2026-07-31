// SPDX-License-Identifier: EUPL-1.2

import type { SystemMonitorSnapshot } from './desktop-system-monitor.models';

export const SYSTEM_MONITOR_DEMO_SOURCE = 'Lethean demo fixture · Host system';

export const SYSTEM_MONITOR_DEMO_SNAPSHOT: SystemMonitorSnapshot = {
  observedAt: '2026-07-31T12:00:00Z',
  source: SYSTEM_MONITOR_DEMO_SOURCE,
  platform: 'darwin',
  architecture: 'arm64',
  cpu: { logicalCores: 10, usagePercent: 34 },
  memory: {
    totalBytes: 32 * 1_024 ** 3,
    usedBytes: Math.round(18.4 * 1_024 ** 3),
  },
  network: {
    receivedBytes: 2_184_000_000,
    sentBytes: 824_000_000,
    receivedBytesPerSecond: 5.2 * 1_024 ** 2,
    sentBytesPerSecond: 1.1 * 1_024 ** 2,
  },
  power: { source: 'ac', batteryPercent: 81, charging: true },
  storage: {
    totalBytes: 512 * 1_024 ** 3,
    freeBytes: 218 * 1_024 ** 3,
  },
  cpuHistory: [26, 27, 24, 31, 29, 36, 33, 38, 35, 40, 37, 34],
  memoryHistory: [54, 55, 55, 56, 56, 57, 57, 58, 58, 58, 57, 57.5],
  networkReceivedHistory: [2.8, 3.2, 2.6, 4.1, 3.8, 5.4, 4.7, 5.9, 5.1, 6.2, 4.8, 5.2].map(
    (megabytes) => megabytes * 1_024 ** 2,
  ),
  networkSentHistory: [0.6, 0.8, 0.5, 0.9, 0.7, 1.2, 0.9, 1.4, 1, 1.3, 0.8, 1.1].map(
    (megabytes) => megabytes * 1_024 ** 2,
  ),
};

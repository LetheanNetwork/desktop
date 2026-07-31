// SPDX-License-Identifier: EUPL-1.2

import { Injectable, inject } from '@angular/core';
import { ConnectionManagerService } from '../connection-manager.service';
import type {
  HostCPUReading,
  HostMemoryReading,
  HostNetworkReading,
  HostPowerReading,
  HostSystemSnapshot,
  SystemPowerSource,
} from './desktop-system-monitor.models';
import { SurfaceBridgeService } from './surfaces/surface-bridge.service';

const HOST_SNAPSHOT_METHOD = 'dappco.re/lthn/desktop/pkg/telemetry.Service.CurrentHostSnapshot';
const POWER_SOURCES: readonly SystemPowerSource[] = ['unknown', 'ac', 'battery'];

@Injectable({ providedIn: 'root' })
export class DesktopSystemMonitorBridgeService {
  private readonly connection = inject(ConnectionManagerService);
  private readonly surface = inject(SurfaceBridgeService);

  async snapshot(): Promise<HostSystemSnapshot> {
    if (this.connection.offline()) {
      throw new Error('The host system bridge is unavailable in offline demo mode.');
    }
    return parseHostSnapshot(await this.surface.call(HOST_SNAPSHOT_METHOD));
  }
}

function parseHostSnapshot(raw: unknown): HostSystemSnapshot {
  const record = requiredRecord(raw);
  const observedAt = requiredBoundedString(record['observed_at'], 64);
  if (!Number.isFinite(Date.parse(observedAt))) invalidResponse();

  const snapshot: HostSystemSnapshot = {
    observedAt,
    source: requiredBoundedString(record['source'], 128),
    platform: requiredIdentifier(record['platform']),
    architecture: requiredIdentifier(record['architecture']),
    cpu: parseCPU(record['cpu']),
    ...optionalRecord(record, 'memory', parseMemory),
    ...optionalRecord(record, 'network', parseNetwork),
    ...optionalRecord(record, 'power', parsePower),
  };
  return snapshot;
}

function parseCPU(raw: unknown): HostCPUReading {
  const record = requiredRecord(raw);
  const logicalCores = requiredInteger(record['logical_cores'], 1, 4_096);
  const usagePercent = optionalNumber(record['usage_percent'], 0, 100);
  return {
    logicalCores,
    ...(usagePercent === undefined ? {} : { usagePercent }),
  };
}

function parseMemory(raw: unknown): HostMemoryReading {
  const record = requiredRecord(raw);
  const totalBytes = requiredInteger(record['total_bytes'], 1, Number.MAX_SAFE_INTEGER);
  const usedBytes = requiredInteger(record['used_bytes'], 0, totalBytes);
  return { totalBytes, usedBytes };
}

function parseNetwork(raw: unknown): HostNetworkReading {
  const record = requiredRecord(raw);
  const receivedBytes = requiredInteger(record['received_bytes'], 0, Number.MAX_SAFE_INTEGER);
  const sentBytes = requiredInteger(record['sent_bytes'], 0, Number.MAX_SAFE_INTEGER);
  const receivedBytesPerSecond = optionalNumber(
    record['received_bytes_per_second'],
    0,
    Number.MAX_SAFE_INTEGER,
  );
  const sentBytesPerSecond = optionalNumber(
    record['sent_bytes_per_second'],
    0,
    Number.MAX_SAFE_INTEGER,
  );
  return {
    receivedBytes,
    sentBytes,
    ...(receivedBytesPerSecond === undefined ? {} : { receivedBytesPerSecond }),
    ...(sentBytesPerSecond === undefined ? {} : { sentBytesPerSecond }),
  };
}

function parsePower(raw: unknown): HostPowerReading {
  const record = requiredRecord(raw);
  const source = requiredBoundedString(record['source'], 16);
  if (!POWER_SOURCES.includes(source as SystemPowerSource)) invalidResponse();
  const batteryPercent = optionalNumber(record['battery_percent'], 0, 100);
  const charging = optionalBoolean(record['charging']);
  return {
    source: source as SystemPowerSource,
    ...(batteryPercent === undefined ? {} : { batteryPercent }),
    ...(charging === undefined ? {} : { charging }),
  };
}

function optionalRecord<Key extends string, Value>(
  record: Record<string, unknown>,
  key: Key,
  parser: (raw: unknown) => Value,
): Partial<Record<Key, Value>> {
  const raw = record[key];
  return raw === undefined || raw === null ? {} : ({ [key]: parser(raw) } as Record<Key, Value>);
}

function requiredRecord(raw: unknown): Record<string, unknown> {
  if (raw === null || typeof raw !== 'object' || Array.isArray(raw)) invalidResponse();
  return raw as Record<string, unknown>;
}

function requiredBoundedString(raw: unknown, maximumLength: number): string {
  if (typeof raw !== 'string' || raw.length === 0 || raw.length > maximumLength) {
    invalidResponse();
  }
  return raw;
}

function requiredIdentifier(raw: unknown): string {
  const value = requiredBoundedString(raw, 32);
  if (!/^[a-z0-9_-]+$/u.test(value)) invalidResponse();
  return value;
}

function requiredInteger(raw: unknown, minimum: number, maximum: number): number {
  if (!Number.isSafeInteger(raw) || (raw as number) < minimum || (raw as number) > maximum) {
    invalidResponse();
  }
  return raw as number;
}

function optionalNumber(raw: unknown, minimum: number, maximum: number): number | undefined {
  if (raw === undefined || raw === null) return undefined;
  if (typeof raw !== 'number' || !Number.isFinite(raw) || raw < minimum || raw > maximum) {
    invalidResponse();
  }
  return raw;
}

function optionalBoolean(raw: unknown): boolean | undefined {
  if (raw === undefined || raw === null) return undefined;
  if (typeof raw !== 'boolean') invalidResponse();
  return raw;
}

function invalidResponse(): never {
  throw new Error('The host system snapshot response is unavailable.');
}

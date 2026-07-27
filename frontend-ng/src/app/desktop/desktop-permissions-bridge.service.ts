// SPDX-License-Identifier: EUPL-1.2

import { Service, inject } from '@angular/core';
import { ConnectionManagerService } from '../connection-manager.service';
import type {
  DesktopPermissionID,
  DesktopPermissionPolicy,
  DesktopHostPermissionState,
  DesktopPermissionSnapshot,
} from './desktop-host-intent.service';
import { SurfaceBridgeService } from './surfaces/surface-bridge.service';

const PERMISSIONS_SERVICE = 'dappco.re/lthn/desktop/pkg/permissions.WailsService';
const PERMISSION_IDS = [
  'microphone',
  'camera',
  'geolocation',
  'notifications',
  'clipboard-read',
] as const;
const PERMISSION_POLICIES = ['default', 'allow', 'deny'] as const;
const HOST_STATES = [
  'granted',
  'denied',
  'prompt',
  'restricted',
  'unsupported',
  'unknown',
] as const;

export const PERMISSIONS_METHODS = {
  status: `${PERMISSIONS_SERVICE}.Status`,
  request: `${PERMISSIONS_SERVICE}.Request`,
} as const;

const DEMO_SNAPSHOTS: readonly DesktopPermissionSnapshot[] = PERMISSION_IDS.map((id) => ({
  id,
  policy: 'default',
  host: 'unsupported',
}));

@Service()
export class DesktopPermissionsBridgeService {
  private readonly surface = inject(SurfaceBridgeService);
  private readonly connection = inject(ConnectionManagerService);

  async status(): Promise<readonly DesktopPermissionSnapshot[]> {
    if (this.connection.offline()) return DEMO_SNAPSHOTS.map((snapshot) => ({ ...snapshot }));
    return parseSnapshots(await this.surface.call(PERMISSIONS_METHODS.status));
  }

  async request(id: string): Promise<DesktopPermissionSnapshot> {
    const permissionID = parsePermissionID(id);
    if (this.connection.offline()) {
      throw new Error('Native permission requests are unavailable in offline demo mode.');
    }
    return parseSnapshot(
      await this.surface.call(PERMISSIONS_METHODS.request, [permissionID]),
    );
  }
}

function parseSnapshots(value: unknown): readonly DesktopPermissionSnapshot[] {
  if (!Array.isArray(value) || value.length !== PERMISSION_IDS.length) {
    throw new Error('The native permission snapshot is incomplete.');
  }
  const snapshots = value.map(parseSnapshot);
  const ids = new Set(snapshots.map(({ id }) => id));
  if (ids.size !== PERMISSION_IDS.length || PERMISSION_IDS.some((id) => !ids.has(id))) {
    throw new Error('The native permission snapshot contains duplicate or missing IDs.');
  }
  return snapshots;
}

function parseSnapshot(value: unknown): DesktopPermissionSnapshot {
  if (!isRecord(value) || !hasExactKeys(value, ['id', 'policy', 'host'])) {
    throw new Error('The native permission snapshot is malformed.');
  }
  return {
    id: parsePermissionID(value['id']),
    policy: parseOneOf(
      value['policy'],
      PERMISSION_POLICIES,
      'permission policy',
    ) as DesktopPermissionPolicy,
    host: parseOneOf(value['host'], HOST_STATES, 'host permission state') as DesktopHostPermissionState,
  };
}

function parsePermissionID(value: unknown): DesktopPermissionID {
  return parseOneOf(value, PERMISSION_IDS, 'permission ID') as DesktopPermissionID;
}

function parseOneOf(
  value: unknown,
  allowed: readonly string[],
  label: string,
): string {
  if (typeof value !== 'string' || !allowed.includes(value)) {
    throw new Error(`The ${label} is unknown.`);
  }
  return value;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return !!value && typeof value === 'object' && !Array.isArray(value);
}

function hasExactKeys(record: Record<string, unknown>, keys: readonly string[]): boolean {
  const actual = Object.keys(record);
  return actual.length === keys.length && actual.every((key) => keys.includes(key));
}

import { Injectable, InjectionToken, inject } from '@angular/core';
import { Events } from '@wailsio/runtime';
import { Observable } from 'rxjs';
import { ConnectionManagerService } from '../connection-manager.service';
import {
  DesktopControlChange,
  DesktopControlSnapshot,
  DesktopControlsChangeNotice,
  DesktopControlValue,
} from '../store/desktop-controls.models';
import {
  parseDesktopControlSnapshot,
  validateDesktopControlChanges,
} from './desktop-controls-codec';
import { DesktopControlsOfflineStore } from './desktop-controls-offline.store';
import { SurfaceBridgeService } from './surfaces/surface-bridge.service';

const APPCONFIG_SERVICE = 'dappco.re/lthn/desktop/pkg/appconfig.Service';
const DESKTOP_CONTROLS_CHANGED_EVENT = 'lthn:desktop-controls:changed';
const MAX_KEY_BYTES = 128;
const MAX_REVISION_BYTES = 64;
const SAFE_CONTROL_KEY = /^[a-z][a-z0-9]*(?:[._-][a-z0-9]+)*$/;
const RFC3339_TIMESTAMP =
  /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d{1,9})?(?:Z|[+-]\d{2}:\d{2})$/;

export interface DesktopControlsEventSource {
  on(name: string, handler: (payload: unknown) => void): () => void;
}

export const DESKTOP_CONTROLS_EVENT_SOURCE = new InjectionToken<DesktopControlsEventSource>(
  'DESKTOP_CONTROLS_EVENT_SOURCE',
  {
    providedIn: 'root',
    factory: () => ({
      on(name, handler): () => void {
        return Events.On(name, (event) => handler(event.data));
      },
    }),
  },
);

@Injectable({ providedIn: 'root' })
export class DesktopControlsBridgeService {
  private readonly bridge = inject(SurfaceBridgeService);
  private readonly connection = inject(ConnectionManagerService);
  private readonly events = inject(DESKTOP_CONTROLS_EVENT_SOURCE);
  private readonly offlineStore = inject(DesktopControlsOfflineStore);

  async settings(): Promise<DesktopControlSnapshot> {
    if (this.connection.offline()) return this.offlineStore.settings();
    return parseDesktopControlSnapshot(await this.bridge.call(`${APPCONFIG_SERVICE}.Settings`));
  }

  async setMany(changes: readonly DesktopControlChange[]): Promise<DesktopControlSnapshot> {
    if (this.connection.offline()) return this.offlineStore.setMany(changes);
    const bounded = validateDesktopControlChanges(changes);
    return parseDesktopControlSnapshot(
      await this.bridge.call(`${APPCONFIG_SERVICE}.SetMany`, [bounded]),
    );
  }

  async set(key: string, value: DesktopControlValue): Promise<DesktopControlSnapshot> {
    return this.setMany([{ key, value }]);
  }

  changes(): Observable<DesktopControlsChangeNotice> {
    if (this.connection.offline()) return this.offlineStore.changes();
    return new Observable((subscriber) => {
      const unsubscribe = this.events.on(DESKTOP_CONTROLS_CHANGED_EVENT, (raw) => {
        const notice = parseChangeNotice(raw);
        if (notice) subscriber.next(notice);
      });
      return unsubscribe;
    });
  }
}

function parseChangeNotice(raw: unknown): DesktopControlsChangeNotice | null {
  const record = asRecord(raw);
  if (!record || !hasExactKeys(record, ['revision', 'keys', 'at'])) return null;
  const revision = record['revision'];
  const keys = record['keys'];
  const at = record['at'];
  if (
    typeof revision !== 'string' ||
    revision.length === 0 ||
    revision.length > MAX_REVISION_BYTES ||
    !Array.isArray(keys) ||
    keys.length === 0 ||
    keys.length > 64 ||
    typeof at !== 'string' ||
    at.length === 0 ||
    at.length > 64 ||
    !RFC3339_TIMESTAMP.test(at) ||
    Number.isNaN(Date.parse(at))
  ) {
    return null;
  }
  const parsedKeys: string[] = [];
  const seen = new Set<string>();
  for (const key of keys) {
    if (
      typeof key !== 'string' ||
      key.length === 0 ||
      key.length > MAX_KEY_BYTES ||
      !SAFE_CONTROL_KEY.test(key) ||
      seen.has(key)
    ) {
      return null;
    }
    seen.add(key);
    parsedKeys.push(key);
  }
  return { revision, keys: parsedKeys, at };
}

function hasExactKeys(record: Record<string, unknown>, expected: readonly string[]): boolean {
  const keys = Object.keys(record);
  return keys.length === expected.length && expected.every((key) => Object.hasOwn(record, key));
}

function asRecord(value: unknown): Record<string, unknown> | null {
  return value !== null && typeof value === 'object' && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : null;
}

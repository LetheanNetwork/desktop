// SPDX-License-Identifier: EUPL-1.2

import { DOCUMENT } from '@angular/common';
import { Injectable, InjectionToken, inject } from '@angular/core';
import { EMPTY, fromEvent, map, Observable } from 'rxjs';
import {
  DesktopControlChange,
  DesktopControlSnapshot,
  DesktopControlsChangeNotice,
  DesktopControlValue,
} from '../store/desktop-controls.models';
import { DESKTOP_STORAGE } from '../store/storage.service';
import {
  acceptsDesktopControlValue,
  copyDesktopControlSnapshot,
  createDemoDesktopControlSnapshot,
  validateDesktopControlChanges,
} from './desktop-controls-codec';

const STORAGE_KEY = 'lthn.desktop-controls.v1';
const LEGACY_KEY = 'lthn.prefs';
const STORAGE_VERSION = 1;
const MAX_REVISION_BYTES = 64;

interface StoredControls {
  readonly version: 1;
  readonly revision: string;
  readonly values: Readonly<Record<string, DesktopControlValue>>;
}

export interface DesktopControlsStorageEvent {
  readonly key: string | null;
  readonly newValue: string | null;
}

export const DESKTOP_CONTROLS_STORAGE_EVENTS = new InjectionToken<
  Observable<DesktopControlsStorageEvent>
>('DESKTOP_CONTROLS_STORAGE_EVENTS', {
  providedIn: 'root',
  factory: () => {
    const browserWindow = inject(DOCUMENT).defaultView;
    return browserWindow
      ? fromEvent<StorageEvent>(browserWindow, 'storage').pipe(
          map((event) => ({ key: event.key, newValue: event.newValue })),
        )
      : EMPTY;
  },
});

@Injectable({ providedIn: 'root' })
export class DesktopControlsOfflineStore {
  private readonly storage = inject(DESKTOP_STORAGE);
  private readonly storageEvents = inject(DESKTOP_CONTROLS_STORAGE_EVENTS);
  private snapshot: DesktopControlSnapshot | null = null;

  async settings(): Promise<DesktopControlSnapshot> {
    return copyDesktopControlSnapshot(this.load());
  }

  async setMany(changes: readonly DesktopControlChange[]): Promise<DesktopControlSnapshot> {
    const bounded = validateDesktopControlChanges(changes);
    const current = this.load();
    const controls = current.controls.map((control) => {
      const change = bounded.find(({ key }) => key === control.key);
      if (!change) return control;
      if (!acceptsDesktopControlValue(control, change.value)) {
        throw new Error('The offline desktop control draft is invalid.');
      }
      return { ...control, value: change.value, configured: true };
    });
    if (bounded.some(({ key }) => !current.controls.some((control) => control.key === key))) {
      throw new Error('The offline desktop control draft is invalid.');
    }

    this.snapshot = { revision: nextLocalRevision(), controls };
    this.write(this.snapshot);
    return copyDesktopControlSnapshot(this.snapshot);
  }

  changes(): Observable<DesktopControlsChangeNotice> {
    return new Observable((subscriber) => {
      const subscription = this.storageEvents.subscribe((event) => {
        if (event.key !== STORAGE_KEY || event.newValue === null) return;
        const stored = parseStoredControls(event.newValue);
        if (!stored) return;
        const current = this.snapshot ?? createDemoDesktopControlSnapshot();
        if (stored.revision === current.revision) return;
        const next = hydrate(stored);
        const keys = changedKeys(current, next);
        this.snapshot = next;
        subscriber.next({ revision: next.revision, keys, at: null });
      });
      return () => subscription.unsubscribe();
    });
  }

  private load(): DesktopControlSnapshot {
    if (this.snapshot) return this.snapshot;

    const stored = read(this.storage, STORAGE_KEY);
    if (!stored.available) {
      this.snapshot = createDemoDesktopControlSnapshot();
      return this.snapshot;
    }
    if (stored.value !== null) {
      const parsed = parseStoredControls(stored.value);
      this.snapshot = parsed ? hydrate(parsed) : createDemoDesktopControlSnapshot();
      return this.snapshot;
    }

    const legacy = read(this.storage, LEGACY_KEY);
    const values = legacy.available && legacy.value !== null ? parseLegacy(legacy.value) : {};
    if (Object.keys(values).length === 0) {
      this.snapshot = createDemoDesktopControlSnapshot();
      return this.snapshot;
    }

    const seeded: StoredControls = {
      version: STORAGE_VERSION,
      revision: nextLocalRevision(),
      values,
    };
    this.snapshot = hydrate(seeded);
    this.write(this.snapshot);
    return this.snapshot;
  }

  private write(snapshot: DesktopControlSnapshot): void {
    const document: StoredControls = {
      version: STORAGE_VERSION,
      revision: snapshot.revision,
      values: Object.fromEntries(snapshot.controls.map(({ key, value }) => [key, value])),
    };
    try {
      this.storage.setItem(STORAGE_KEY, JSON.stringify(document));
    } catch {
      // The instance-local snapshot remains the explicit offline fallback.
    }
  }
}

function parseStoredControls(serialised: string): StoredControls | null {
  let raw: unknown;
  try {
    raw = JSON.parse(serialised);
  } catch {
    return null;
  }
  const record = asRecord(raw);
  if (!record || !hasExactKeys(record, ['version', 'revision', 'values'])) return null;
  if (record['version'] !== STORAGE_VERSION || !validRevision(record['revision'])) return null;
  const rawValues = asRecord(record['values']);
  if (!rawValues) return null;

  const catalogue = createDemoDesktopControlSnapshot();
  if (Object.keys(rawValues).length > catalogue.controls.length) return null;
  const controls = new Map(catalogue.controls.map((control) => [control.key, control]));
  const values: Record<string, DesktopControlValue> = {};
  for (const [key, value] of Object.entries(rawValues)) {
    const control = controls.get(key);
    if (control && isDesktopControlValue(value) && acceptsDesktopControlValue(control, value)) {
      values[key] = value;
    }
  }
  return {
    version: STORAGE_VERSION,
    revision: record['revision'],
    values,
  };
}

function hydrate(stored: StoredControls): DesktopControlSnapshot {
  const snapshot = createDemoDesktopControlSnapshot(stored.revision);
  return {
    ...snapshot,
    controls: snapshot.controls.map((control) =>
      Object.hasOwn(stored.values, control.key) &&
      acceptsDesktopControlValue(control, stored.values[control.key] as DesktopControlValue)
        ? {
            ...control,
            value: stored.values[control.key] as DesktopControlValue,
            configured: true,
          }
        : control,
    ),
  };
}

function parseLegacy(serialised: string): Readonly<Record<string, DesktopControlValue>> {
  let raw: unknown;
  try {
    raw = JSON.parse(serialised);
  } catch {
    return {};
  }
  const record = asRecord(raw);
  if (!record) return {};

  const mapping: Readonly<Record<string, string>> = {
    bar: 'desktop.shell.taskbar_edge',
    mode: 'desktop.theme.interface',
    brand: 'desktop.theme.brand',
    design: 'desktop.theme.design',
    customHue: 'desktop.theme.custom_hue',
    customName: 'desktop.theme.custom_name',
    wallpaper: 'desktop.theme.wallpaper',
    lang: 'desktop.locale.language',
    showIcons: 'desktop.shell.show_icons',
    showWidgets: 'desktop.shell.show_widgets',
    reduceMotion: 'desktop.theme.reduce_motion',
  };
  const controls = new Map(
    createDemoDesktopControlSnapshot().controls.map((control) => [control.key, control]),
  );
  const values: Record<string, DesktopControlValue> = {};
  for (const [legacyKey, controlKey] of Object.entries(mapping)) {
    const value = record[legacyKey];
    const control = controls.get(controlKey);
    if (control && isDesktopControlValue(value) && acceptsDesktopControlValue(control, value)) {
      values[controlKey] = value;
    }
  }
  return values;
}

function changedKeys(
  current: DesktopControlSnapshot,
  next: DesktopControlSnapshot,
): readonly string[] {
  const currentValues = new Map(current.controls.map(({ key, value }) => [key, value]));
  return next.controls
    .filter(({ key, value }) => currentValues.get(key) !== value)
    .map(({ key }) => key);
}

function read(
  storage: Storage,
  key: string,
): { readonly available: boolean; readonly value: string | null } {
  try {
    return { available: true, value: storage.getItem(key) };
  } catch {
    return { available: false, value: null };
  }
}

function validRevision(value: unknown): value is string {
  return typeof value === 'string' && value.length > 0 && value.length <= MAX_REVISION_BYTES;
}

function isDesktopControlValue(value: unknown): value is DesktopControlValue {
  return (
    typeof value === 'boolean' ||
    typeof value === 'string' ||
    (typeof value === 'number' && Number.isFinite(value))
  );
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

let revisionSequence = 0;

function nextLocalRevision(): string {
  const random = globalThis.crypto?.randomUUID?.();
  if (random) return `local-${random}`;
  revisionSequence += 1;
  return `local-${Date.now().toString(36)}-${revisionSequence.toString(36)}`;
}

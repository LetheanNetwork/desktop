// SPDX-License-Identifier: EUPL-1.2

import { Injectable, inject } from '@angular/core';
import { CONNECTION_LOCATION, ConnectionManagerService } from '../connection-manager.service';
import { DesktopHydration } from '../store/desktop.actions';
import { DesktopState } from '../store/desktop.reducer';
import { DeviceSize, ViewMode } from './desktop.data';
import { SurfaceBridgeService } from './surfaces/surface-bridge.service';

const DESKTOP_STATE_SERVICE = 'dappco.re/lthn/desktop/pkg/desktopstate.WailsService';
const DOCUMENT_VERSION = 1;
const MAX_WINDOWS = 128;
const MAX_IDENTIFIER_BYTES = 128;
const MAX_ROUTE_BYTES = 512;
const MAX_COORDINATE = 100_000;
const MAX_DIMENSION = 20_000;
const MAX_Z = 1_000_000;
const IDENTIFIER = /^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$/u;
const CONTROL_CHARACTERS = /[\u0000-\u001f\u007f]/u;
const WINDOWS_ABSOLUTE = /^(?:[A-Za-z]:[\\/]|\\\\)/u;

export interface DesktopSessionWindow {
  readonly id: string;
  readonly app: string;
  readonly sub: string;
  readonly systemTab: string;
  readonly group: string;
  readonly x: number;
  readonly y: number;
  readonly width: number;
  readonly height: number;
  readonly z: number;
  readonly min: boolean;
  readonly max: boolean;
}

export interface DesktopShellSession {
  readonly view: ViewMode;
  readonly device: DeviceSize;
  readonly focusId: string;
  readonly z: number;
  readonly windows: readonly DesktopSessionWindow[];
  readonly migratedBrowserState: boolean;
}

export interface DesktopShellSessionSnapshot {
  readonly version: number;
  readonly revision: number;
  readonly updatedAt: string;
  readonly session: DesktopShellSession;
}

export class DesktopStateConflictError extends Error {
  constructor(message = 'The desktop session changed since it was loaded.') {
    super(message);
    this.name = 'DesktopStateConflictError';
  }
}

@Injectable({ providedIn: 'root' })
export class DesktopStateBridgeService {
  private readonly surface = inject(SurfaceBridgeService);
  private readonly connection = inject(ConnectionManagerService);
  private readonly location = inject(CONNECTION_LOCATION);
  private demoSnapshot = createDemoSnapshot(this.location.search);

  isOffline(): boolean {
    return this.connection.offline();
  }

  async loadShellSession(): Promise<DesktopShellSessionSnapshot> {
    if (this.isOffline()) return copySnapshot(this.demoSnapshot);
    return parseSnapshot(await this.surface.call(`${DESKTOP_STATE_SERVICE}.LoadShellSession`));
  }

  async saveShellSession(
    expectedRevision: number,
    session: DesktopShellSession,
  ): Promise<DesktopShellSessionSnapshot> {
    const request = {
      expectedRevision: requiredRevision(expectedRevision),
      session: parseSession(session),
    };
    if (this.isOffline()) {
      if (request.expectedRevision !== this.demoSnapshot.revision) {
        throw new DesktopStateConflictError();
      }
      this.demoSnapshot = {
        version: DOCUMENT_VERSION,
        revision: this.demoSnapshot.revision + 1,
        updatedAt: new Date().toISOString(),
        session: request.session,
      };
      return copySnapshot(this.demoSnapshot);
    }

    try {
      return parseSnapshot(
        await this.surface.call(`${DESKTOP_STATE_SERVICE}.SaveShellSession`, [request]),
      );
    } catch (error) {
      if (isDesktopStateConflict(error)) {
        throw new DesktopStateConflictError(messageFor(error));
      }
      throw error;
    }
  }
}

export function desktopHydrationFromSession(session: DesktopShellSession): DesktopHydration {
  const validated = parseSession(session);
  return {
    view: validated.view,
    device: validated.device,
    focusId: validated.focusId || null,
    z: validated.z,
    wins: validated.windows.map((window) => ({
      id: window.id,
      app: window.app,
      sub: window.sub,
      systab: window.systemTab,
      x: window.x,
      y: window.y,
      w: window.width,
      h: window.height,
      z: window.z,
      min: window.min,
      max: window.max,
      ...(window.group ? { group: window.group } : {}),
    })),
  };
}

export function desktopSessionFromState(
  state: DesktopState,
  migratedBrowserState: boolean,
): DesktopShellSession {
  return parseSession({
    view: state.view,
    device: state.device,
    focusId: state.focusId ?? '',
    z: state.z,
    windows: state.wins.map((window) => ({
      id: window.id,
      app: window.app,
      sub: window.sub,
      systemTab: window.systab ?? '',
      group: window.group ?? '',
      x: window.x,
      y: window.y,
      width: window.w,
      height: window.h,
      z: window.z,
      min: window.min,
      max: window.max,
    })),
    migratedBrowserState,
  });
}

export function parseLegacyDesktopSession(raw: unknown): DesktopShellSession | null {
  const record = asRecord(raw);
  if (!record) return null;
  const windows = record['wins'];
  if (windows !== undefined && !Array.isArray(windows)) return null;

  try {
    return parseSession({
      view: record['view'] ?? 'desktop',
      device: record['device'] ?? 'full',
      focusId: record['focusId'] ?? '',
      z: record['z'] ?? 10,
      windows: (windows ?? []).map((rawWindow) => {
        const window = asRecord(rawWindow);
        if (!window) throw invalidDesktopState();
        return {
          id: window['id'],
          app: window['app'],
          sub: window['sub'] ?? '',
          systemTab: window['systab'] ?? '',
          group: window['group'] ?? '',
          x: window['x'],
          y: window['y'],
          width: window['w'],
          height: window['h'],
          z: window['z'],
          min: window['min'],
          max: window['max'],
        };
      }),
      migratedBrowserState: true,
    });
  } catch {
    return null;
  }
}

export function isDesktopStateConflict(error: unknown): boolean {
  if (error instanceof DesktopStateConflictError) return true;
  const message = messageFor(error).toLocaleLowerCase('en-GB');
  return (
    message.includes('desktop_state_conflict') ||
    message.includes('changed since it was loaded') ||
    message.includes('revision conflict')
  );
}

function createDemoSnapshot(search: string): DesktopShellSessionSnapshot {
  const options = new URLSearchParams(search);
  const requestedView = options.get('lthn-view');
  const requestedDevice = options.get('lthn-device');
  const view =
    requestedView === 'desktop' || requestedView === 'shell' || requestedView === 'device'
      ? requestedView
      : 'desktop';
  const device =
    requestedDevice === 'small' || requestedDevice === 'large' || requestedDevice === 'full'
      ? requestedDevice
      : 'full';
  return {
    version: DOCUMENT_VERSION,
    revision: 0,
    updatedAt: '',
    session: {
      view,
      device,
      focusId: '',
      z: 10,
      windows: [],
      migratedBrowserState: false,
    },
  };
}

function parseSnapshot(raw: unknown): DesktopShellSessionSnapshot {
  const record = exactRecord(raw, ['version', 'revision', 'updatedAt', 'session'], 'snapshot');
  const version = requiredInteger(record['version'], 1, DOCUMENT_VERSION);
  if (version !== DOCUMENT_VERSION) throw invalidDesktopState();
  const revision = requiredRevision(record['revision']);
  const updatedAt = requiredString(record['updatedAt'], 64, true);
  if ((revision === 0 && updatedAt !== '') || (revision > 0 && !validTimestamp(updatedAt))) {
    throw invalidDesktopState();
  }
  return {
    version,
    revision,
    updatedAt,
    session: parseSession(record['session']),
  };
}

function parseSession(raw: unknown): DesktopShellSession {
  const record = exactRecord(
    raw,
    ['view', 'device', 'focusId', 'z', 'windows', 'migratedBrowserState'],
    'session',
  );
  const view = requiredView(record['view']);
  const device = requiredDevice(record['device']);
  const focusId = requiredIdentifier(record['focusId'], true);
  const z = requiredInteger(record['z'], 0, MAX_Z);
  const windows = requiredArray(record['windows'], MAX_WINDOWS).map(parseWindow);
  const ids = new Set<string>();
  for (const window of windows) {
    if (ids.has(window.id)) throw invalidDesktopState();
    ids.add(window.id);
  }
  if (focusId && !ids.has(focusId)) throw invalidDesktopState();
  if (typeof record['migratedBrowserState'] !== 'boolean') {
    throw invalidDesktopState();
  }
  return {
    view,
    device,
    focusId,
    z,
    windows,
    migratedBrowserState: record['migratedBrowserState'],
  };
}

function parseWindow(raw: unknown): DesktopSessionWindow {
  const record = exactRecord(
    raw,
    ['id', 'app', 'sub', 'systemTab', 'group', 'x', 'y', 'width', 'height', 'z', 'min', 'max'],
    'window',
  );
  const sub = requiredRoute(record['sub']);
  const systemTab = requiredRoute(record['systemTab']);
  if (typeof record['min'] !== 'boolean' || typeof record['max'] !== 'boolean') {
    throw invalidDesktopState();
  }
  return {
    id: requiredIdentifier(record['id']),
    app: requiredIdentifier(record['app']),
    sub,
    systemTab,
    group: requiredIdentifier(record['group'], true),
    x: requiredInteger(record['x'], -MAX_COORDINATE, MAX_COORDINATE),
    y: requiredInteger(record['y'], -MAX_COORDINATE, MAX_COORDINATE),
    width: requiredInteger(record['width'], 120, MAX_DIMENSION),
    height: requiredInteger(record['height'], 80, MAX_DIMENSION),
    z: requiredInteger(record['z'], 0, MAX_Z),
    min: record['min'],
    max: record['max'],
  };
}

function exactRecord(
  value: unknown,
  allowedKeys: readonly string[],
  _label: string,
): Record<string, unknown> {
  const record = asRecord(value);
  if (!record || Object.keys(record).some((key) => !allowedKeys.includes(key))) {
    throw invalidDesktopState();
  }
  return record;
}

function asRecord(value: unknown): Record<string, unknown> | null {
  return value !== null && typeof value === 'object' && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : null;
}

function requiredArray(value: unknown, maximum: number): readonly unknown[] {
  if (!Array.isArray(value) || value.length > maximum) {
    throw invalidDesktopState();
  }
  return value;
}

function requiredView(value: unknown): ViewMode {
  if (value === 'desktop' || value === 'shell' || value === 'device') {
    return value;
  }
  throw invalidDesktopState();
}

function requiredDevice(value: unknown): DeviceSize {
  if (value === 'small' || value === 'large' || value === 'full') {
    return value;
  }
  throw invalidDesktopState();
}

function requiredIdentifier(value: unknown, optional = false): string {
  if (optional && (value === undefined || value === '')) return '';
  if (
    typeof value !== 'string' ||
    value.length === 0 ||
    value.length > MAX_IDENTIFIER_BYTES ||
    !IDENTIFIER.test(value)
  ) {
    throw invalidDesktopState();
  }
  return value;
}

function requiredRoute(value: unknown): string {
  const route = requiredString(value, MAX_ROUTE_BYTES, true);
  if (
    route.startsWith('/') ||
    WINDOWS_ABSOLUTE.test(route) ||
    route === '..' ||
    route.startsWith('../') ||
    route.includes('/../') ||
    route.endsWith('/..')
  ) {
    throw invalidDesktopState();
  }
  return route;
}

function requiredString(value: unknown, maximum: number, optional = false): string {
  if (optional && (value === undefined || value === '')) return '';
  if (
    typeof value !== 'string' ||
    value.length === 0 ||
    value.length > maximum ||
    CONTROL_CHARACTERS.test(value)
  ) {
    throw invalidDesktopState();
  }
  return value;
}

function requiredInteger(value: unknown, minimum: number, maximum: number): number {
  if (
    typeof value !== 'number' ||
    !Number.isSafeInteger(value) ||
    value < minimum ||
    value > maximum
  ) {
    throw invalidDesktopState();
  }
  return value;
}

function requiredRevision(value: unknown): number {
  return requiredInteger(value, 0, Number.MAX_SAFE_INTEGER);
}

function validTimestamp(value: string): boolean {
  return (
    /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d{1,9})?(?:Z|[+-]\d{2}:\d{2})$/u.test(value) &&
    Number.isFinite(Date.parse(value))
  );
}

function copySnapshot(snapshot: DesktopShellSessionSnapshot): DesktopShellSessionSnapshot {
  return {
    ...snapshot,
    session: {
      ...snapshot.session,
      windows: snapshot.session.windows.map((window) => ({ ...window })),
    },
  };
}

function messageFor(error: unknown): string {
  if (error instanceof Error && error.message) return error.message;
  if (typeof error === 'string' && error) return error;
  return 'The desktop state service is unavailable.';
}

function invalidDesktopState(): Error {
  return new Error('The desktop state service returned invalid desktop state.');
}

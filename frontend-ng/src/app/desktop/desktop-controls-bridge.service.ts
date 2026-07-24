import { Injectable, inject } from '@angular/core';
import {
  DesktopControl,
  DesktopControlKind,
  DesktopControlSnapshot,
  DesktopControlValue,
} from '../store/desktop-controls.models';
import { SurfaceBridgeService } from './surfaces/surface-bridge.service';

const APPCONFIG_SERVICE = 'dappco.re/lthn/desktop/pkg/appconfig.Service';

@Injectable({ providedIn: 'root' })
export class DesktopControlsBridgeService {
  private readonly bridge = inject(SurfaceBridgeService);

  async settings(): Promise<DesktopControlSnapshot> {
    return parseSnapshot(await this.bridge.call(`${APPCONFIG_SERVICE}.Settings`));
  }

  async set(key: string, value: DesktopControlValue): Promise<DesktopControlSnapshot> {
    return parseSnapshot(await this.bridge.call(`${APPCONFIG_SERVICE}.Set`, [key, value]));
  }
}

function parseSnapshot(raw: unknown): DesktopControlSnapshot {
  const record = asRecord(raw);
  if (!record || !Array.isArray(record['controls'])) {
    throw new Error('The desktop control catalogue is unavailable.');
  }
  const configPath = typeof record['config_path'] === 'string' ? record['config_path'] : '';
  return {
    configPath,
    controls: record['controls'].map(parseControl),
  };
}

function parseControl(raw: unknown): DesktopControl {
  const record = asRecord(raw);
  if (!record) throw new Error('The desktop control catalogue contains an invalid entry.');
  const key = requiredString(record, 'key');
  const group = requiredString(record, 'group');
  const label = requiredString(record, 'label');
  const description = requiredString(record, 'description');
  const kind = controlKind(record['kind']);
  const value = controlValue(record['value']);
  const defaultValue = controlValue(record['default']);
  const choices = Array.isArray(record['choices'])
    ? record['choices'].filter((choice): choice is string => typeof choice === 'string')
    : undefined;

  return {
    key,
    group,
    label,
    description,
    kind,
    value,
    defaultValue,
    configured: record['configured'] === true,
    live: record['live'] === true,
    restartRequired: record['restart_required'] === true,
    ...(choices ? { choices } : {}),
    ...optionalNumber(record, 'minimum'),
    ...optionalNumber(record, 'maximum'),
    ...optionalNumber(record, 'step'),
  };
}

function asRecord(value: unknown): Record<string, unknown> | null {
  return value !== null && typeof value === 'object' ? (value as Record<string, unknown>) : null;
}

function requiredString(record: Record<string, unknown>, key: string): string {
  const value = record[key];
  if (typeof value !== 'string' || value === '') {
    throw new Error(`The desktop control catalogue has no ${key}.`);
  }
  return value;
}

function controlKind(value: unknown): DesktopControlKind {
  if (value === 'toggle' || value === 'number' || value === 'select' || value === 'text') {
    return value;
  }
  throw new Error('The desktop control catalogue has an invalid control kind.');
}

function controlValue(value: unknown): DesktopControlValue {
  if (typeof value === 'boolean' || typeof value === 'number' || typeof value === 'string') {
    return value;
  }
  throw new Error('The desktop control catalogue has an invalid value.');
}

function optionalNumber(
  record: Record<string, unknown>,
  key: 'maximum' | 'minimum' | 'step',
): Partial<Record<'maximum' | 'minimum' | 'step', number>> {
  const value = record[key];
  return typeof value === 'number' ? { [key]: value } : {};
}

import { Injectable, inject } from '@angular/core';
import { ConnectionManagerService } from '../connection-manager.service';
import {
  DesktopControl,
  DesktopControlChange,
  DesktopControlKind,
  DesktopControlSnapshot,
  DesktopControlValue,
} from '../store/desktop-controls.models';
import { SurfaceBridgeService } from './surfaces/surface-bridge.service';

const APPCONFIG_SERVICE = 'dappco.re/lthn/desktop/pkg/appconfig.Service';
const MAX_CHANGES = 64;
const MAX_KEY_BYTES = 128;
const MAX_TEXT_BYTES = 2_048;
const SAFE_CONTROL_KEY = /^[a-z][a-z0-9]*(?:[._-][a-z0-9]+)*$/;

@Injectable({ providedIn: 'root' })
export class DesktopControlsBridgeService {
  private readonly bridge = inject(SurfaceBridgeService);
  private readonly connection = inject(ConnectionManagerService);
  private demoSnapshot = createDemoSnapshot();

  async settings(): Promise<DesktopControlSnapshot> {
    if (this.connection.offline()) return copySnapshot(this.demoSnapshot);
    return parseSnapshot(await this.bridge.call(`${APPCONFIG_SERVICE}.Settings`));
  }

  async setMany(changes: readonly DesktopControlChange[]): Promise<DesktopControlSnapshot> {
    const bounded = validateChanges(changes);
    if (this.connection.offline()) {
      this.demoSnapshot = applyDemoChanges(this.demoSnapshot, bounded);
      return copySnapshot(this.demoSnapshot);
    }
    return parseSnapshot(await this.bridge.call(`${APPCONFIG_SERVICE}.SetMany`, [bounded]));
  }

  async set(key: string, value: DesktopControlValue): Promise<DesktopControlSnapshot> {
    return this.setMany([{ key, value }]);
  }
}

function parseSnapshot(raw: unknown): DesktopControlSnapshot {
  const record = asRecord(raw);
  if (!record || !Array.isArray(record['controls'])) {
    throw new Error('The desktop control catalogue is unavailable.');
  }
  return {
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
  const control: DesktopControl = {
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
  if (!acceptsControlValue(control, value) || !acceptsControlValue(control, defaultValue)) {
    throw new Error('The desktop control catalogue contains an invalid value.');
  }
  return control;
}

function validateChanges(
  changes: readonly DesktopControlChange[],
): readonly DesktopControlChange[] {
  if (!Array.isArray(changes) || changes.length === 0) {
    throw new Error('A desktop control draft is required.');
  }
  if (changes.length > MAX_CHANGES) {
    throw new Error('Too many desktop control changes were requested.');
  }
  const seen = new Set<string>();
  return changes.map((change) => {
    if (
      !change ||
      typeof change.key !== 'string' ||
      change.key.length === 0 ||
      change.key.length > MAX_KEY_BYTES ||
      !SAFE_CONTROL_KEY.test(change.key) ||
      seen.has(change.key)
    ) {
      throw new Error('The desktop control draft contains an invalid desktop control.');
    }
    const value = controlValue(change.value);
    if (
      (typeof value === 'string' && value.length > MAX_TEXT_BYTES) ||
      (typeof value === 'number' && !Number.isFinite(value))
    ) {
      throw new Error('The desktop control draft contains an invalid value.');
    }
    seen.add(change.key);
    return { key: change.key, value };
  });
}

function applyDemoChanges(
  snapshot: DesktopControlSnapshot,
  changes: readonly DesktopControlChange[],
): DesktopControlSnapshot {
  const pending = new Map(changes.map((change) => [change.key, change.value]));
  for (const change of changes) {
    const control = snapshot.controls.find(({ key }) => key === change.key);
    if (!control || !acceptsControlValue(control, change.value)) {
      throw new Error('The offline desktop control draft is invalid.');
    }
  }
  return {
    controls: snapshot.controls.map((control) =>
      pending.has(control.key)
        ? {
            ...control,
            value: pending.get(control.key) as DesktopControlValue,
            configured: true,
          }
        : control,
    ),
  };
}

function acceptsControlValue(control: DesktopControl, value: DesktopControlValue): boolean {
  switch (control.kind) {
    case 'toggle':
      return typeof value === 'boolean';
    case 'number':
      return (
        typeof value === 'number' &&
        Number.isFinite(value) &&
        (control.minimum === undefined || value >= control.minimum) &&
        (control.maximum === undefined || value <= control.maximum)
      );
    case 'select':
      return (
        typeof value === 'string' &&
        value.length <= MAX_TEXT_BYTES &&
        (control.choices?.includes(value) ?? false)
      );
    case 'text':
      return typeof value === 'string' && value.length <= MAX_TEXT_BYTES;
  }
}

function copySnapshot(snapshot: DesktopControlSnapshot): DesktopControlSnapshot {
  return {
    controls: snapshot.controls.map((control) => ({
      ...control,
      ...(control.choices ? { choices: [...control.choices] } : {}),
    })),
  };
}

function createDemoSnapshot(): DesktopControlSnapshot {
  return {
    controls: [
      demoSelect(
        'desktop.shell.taskbar_edge',
        'Desktop',
        'Taskbar edge',
        'Dock the taskbar to one screen edge.',
        'bottom',
        ['top', 'right', 'bottom', 'left'],
      ),
      demoToggle(
        'desktop.shell.show_icons',
        'Desktop',
        'Desktop icons',
        'Show application launchers on the desktop.',
        true,
      ),
      demoToggle(
        'desktop.shell.show_widgets',
        'Desktop',
        'Desktop widgets',
        'Show clock and package widgets on the desktop.',
        true,
      ),
      demoSelect(
        'desktop.theme.interface',
        'Theme',
        'Interface theme',
        'Apply the Angular desktop theme.',
        'dark',
        ['dark', 'light'],
      ),
      demoSelect(
        'desktop.theme.brand',
        'Theme',
        'Brand',
        'Select the active brand token family.',
        'lethean',
        ['lethean', 'hostuk'],
      ),
      demoSelect(
        'desktop.theme.design',
        'Theme',
        'Design',
        'Use the Lethean design or a custom accent.',
        'lethean',
        ['lethean', 'custom'],
      ),
      demoNumber(
        'desktop.theme.custom_hue',
        'Theme',
        'Custom accent hue',
        'Hue used to generate the custom accent ramp.',
        305,
        0,
        360,
      ),
      demoText(
        'desktop.theme.custom_name',
        'Theme',
        'Custom design name',
        'Reader-facing name for the custom design.',
        'Host UK',
      ),
      demoSelect(
        'desktop.theme.wallpaper',
        'Theme',
        'Wallpaper',
        'Select the desktop background treatment.',
        'aurora',
        ['aurora', 'dusk', 'mist', 'graphite'],
      ),
      demoToggle(
        'desktop.theme.reduce_motion',
        'Theme',
        'Reduce motion',
        'Disable dock magnification and interface transitions.',
        false,
      ),
      {
        ...demoSelect(
          'desktop.locale.language',
          'Language',
          'Language',
          'Select the desktop interface language.',
          'en',
          ['en', 'cy', 'de', 'es', 'fr', 'ja'],
        ),
        live: false,
        restartRequired: true,
      },
      {
        ...demoToggle(
          'desktop.single_instance.enabled',
          'Single instance',
          'Single-instance hand-off',
          'Hand later launches to the running process.',
          true,
        ),
        live: false,
        restartRequired: true,
      },
    ],
  };
}

function demoControl(
  key: string,
  group: string,
  label: string,
  description: string,
  kind: DesktopControlKind,
  defaultValue: DesktopControlValue,
): DesktopControl {
  return {
    key,
    group,
    label,
    description,
    kind,
    value: defaultValue,
    defaultValue,
    configured: false,
    live: true,
    restartRequired: false,
  };
}

function demoToggle(
  key: string,
  group: string,
  label: string,
  description: string,
  value: boolean,
): DesktopControl {
  return demoControl(key, group, label, description, 'toggle', value);
}

function demoSelect(
  key: string,
  group: string,
  label: string,
  description: string,
  value: string,
  choices: readonly string[],
): DesktopControl {
  return {
    ...demoControl(key, group, label, description, 'select', value),
    choices,
  };
}

function demoNumber(
  key: string,
  group: string,
  label: string,
  description: string,
  value: number,
  minimum: number,
  maximum: number,
): DesktopControl {
  return {
    ...demoControl(key, group, label, description, 'number', value),
    minimum,
    maximum,
    step: 1,
  };
}

function demoText(
  key: string,
  group: string,
  label: string,
  description: string,
  value: string,
): DesktopControl {
  return demoControl(key, group, label, description, 'text', value);
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
  if (typeof value === 'boolean' || typeof value === 'string') return value;
  if (typeof value === 'number' && Number.isFinite(value)) return value;
  throw new Error('The desktop control catalogue has an invalid value.');
}

function optionalNumber(
  record: Record<string, unknown>,
  key: 'maximum' | 'minimum' | 'step',
): Partial<Record<'maximum' | 'minimum' | 'step', number>> {
  const value = record[key];
  return typeof value === 'number' && Number.isFinite(value) ? { [key]: value } : {};
}

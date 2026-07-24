export type DesktopControlValue = boolean | number | string;
export type DesktopControlKind = 'toggle' | 'number' | 'select' | 'text';

export interface DesktopControl {
  readonly key: string;
  readonly group: string;
  readonly label: string;
  readonly description: string;
  readonly kind: DesktopControlKind;
  readonly value: DesktopControlValue;
  readonly defaultValue: DesktopControlValue;
  readonly configured: boolean;
  readonly live: boolean;
  readonly restartRequired: boolean;
  readonly choices?: readonly string[];
  readonly minimum?: number;
  readonly maximum?: number;
  readonly step?: number;
}

export interface DesktopControlSnapshot {
  readonly configPath: string;
  readonly controls: readonly DesktopControl[];
}

export interface DesktopControlGroup {
  readonly name: string;
  readonly controls: readonly DesktopControl[];
}

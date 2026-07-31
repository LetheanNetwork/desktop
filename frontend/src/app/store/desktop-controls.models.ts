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
  readonly revision: string;
  readonly controls: readonly DesktopControl[];
}

export interface DesktopControlsChangeNotice {
  readonly revision: string | null;
  readonly keys: readonly string[];
  readonly at: string | null;
}

export interface DesktopControlChange {
  readonly key: string;
  readonly value: DesktopControlValue;
}

export type DesktopControlDraft = Readonly<Record<string, DesktopControlValue>>;

export interface DesktopControlGroup {
  readonly name: string;
  readonly controls: readonly DesktopControl[];
}

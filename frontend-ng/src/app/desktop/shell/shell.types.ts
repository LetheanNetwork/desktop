export interface ShellUserIdentity {
  readonly initials: string;
  readonly name: string;
  readonly email: string;
  readonly host: string;
}

export interface ShellWindowGroup {
  id: string;
  name: string;
  ids: string[];
  apps: string[];
  open: boolean;
}

export interface ShellValueEvent<T, E extends Event = Event> {
  readonly value: T;
  readonly event: E;
}

export interface ShellPosition {
  readonly left: number;
  readonly top: number | null;
  readonly bottom: number | null;
}

export interface ShellStartSubmenuState {
  open: boolean;
  left: number;
  top: number;
  parent: string;
}

export type ShellSessionAction = 'lock' | 'switch' | 'logout' | 'shutdown';

export interface ShellChildRequest {
  readonly appId: string;
  readonly childId: string;
}

export interface ShellContextItem {
  readonly label?: string;
  readonly icon?: string;
  readonly sep?: boolean;
  readonly act?: () => void;
  readonly children?: readonly ShellContextItem[];
}

export interface ShellContextMenuState {
  open: boolean;
  left: number;
  top: number;
  title: string;
  items: ShellContextItem[];
}

export interface ShellContextSubmenuState {
  open: boolean;
  left: number;
  top: number;
  index: number;
}

export type ShellTrayKey = 'lang' | 'wifi' | 'battery' | 'clock';

export interface ShellTrayState {
  open: boolean;
  key: ShellTrayKey | '';
  left: number;
  top: number;
}

export type ShellLanguage = readonly [code: string, label: string];

export interface ShellWorldClock {
  readonly city: string;
  readonly time: string;
}

export interface ShellAppRequest {
  readonly appId: string;
  readonly subId?: string;
}

export interface ShellNotification {
  readonly id: number;
  readonly icon: string;
  readonly title: string;
  readonly body: string;
}

export interface ShellCommand {
  readonly icon: string;
  readonly label: string;
  readonly section: string;
  readonly run: () => void;
}

export interface ShellPaletteState {
  open: boolean;
  query: string;
  index: number;
}

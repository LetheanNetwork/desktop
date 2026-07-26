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

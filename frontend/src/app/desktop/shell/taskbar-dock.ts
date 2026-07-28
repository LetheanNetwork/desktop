import { CommonModule } from '@angular/common';
import {
  ChangeDetectionStrategy,
  Component,
  CUSTOM_ELEMENTS_SCHEMA,
  ViewEncapsulation,
  input,
  output,
} from '@angular/core';
import type { AppDef } from '../desktop-catalogue.data';
import type { Win } from '../desktop.data';
import type { ShellUserIdentity, ShellValueEvent, ShellWindowGroup } from './shell.types';

@Component({
  selector: 'lthn-shell-taskbar-dock',
  imports: [CommonModule],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  templateUrl: './taskbar-dock.html',
  styleUrl: './taskbar-dock.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
  encapsulation: ViewEncapsulation.None,
  host: {
    style: 'display:contents',
  },
})
export class ShellTaskbarDock {
  readonly user = input.required<ShellUserIdentity>();
  readonly taskWins = input.required<readonly Win[]>();
  readonly focusId = input.required<string | null>();
  readonly apps = input.required<Readonly<Record<string, AppDef>>>();
  readonly runningApps = input.required<readonly string[]>();
  readonly groups = input.required<readonly ShellWindowGroup[]>();
  readonly dropzone = input(false);

  readonly sessionRequested = output<Event>();
  readonly taskRequested = output<string>();
  readonly dockRequested = output<string>();
  readonly dockContextRequested = output<ShellValueEvent<string, MouseEvent>>();
  readonly groupRequested = output<string>();
  readonly groupContextRequested = output<ShellValueEvent<ShellWindowGroup, MouseEvent>>();

  requestSession(event: Event): void {
    this.sessionRequested.emit(event);
  }

  requestDockContext(value: string, event: MouseEvent): void {
    this.dockContextRequested.emit({ value, event });
  }

  requestGroupContext(value: ShellWindowGroup, event: MouseEvent): void {
    this.groupContextRequested.emit({ value, event });
  }
}

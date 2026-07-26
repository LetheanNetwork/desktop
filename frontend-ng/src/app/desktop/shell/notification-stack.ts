import { CommonModule } from '@angular/common';
import {
  ChangeDetectionStrategy,
  Component,
  CUSTOM_ELEMENTS_SCHEMA,
  ViewEncapsulation,
  input,
  output,
} from '@angular/core';
import type { ShellNotification } from './shell.types';

@Component({
  selector: 'lthn-shell-notification-stack',
  imports: [CommonModule],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  templateUrl: './notification-stack.html',
  styleUrl: './notification-stack.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
  encapsulation: ViewEncapsulation.None,
  host: {
    style: 'display:contents',
  },
})
export class ShellNotificationStack {
  readonly notifications = input.required<readonly ShellNotification[]>();
  readonly dismissRequested = output<number>();
}

import { CommonModule } from '@angular/common';
import {
  ChangeDetectionStrategy,
  Component,
  CUSTOM_ELEMENTS_SCHEMA,
  ElementRef,
  ViewEncapsulation,
  input,
  output,
  viewChild,
} from '@angular/core';
import type { ShellCommand } from './shell.types';

@Component({
  selector: 'lthn-shell-command-palette',
  imports: [CommonModule],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  templateUrl: './command-palette.html',
  styleUrl: './command-palette.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
  encapsulation: ViewEncapsulation.None,
  host: {
    style: 'display:contents',
  },
})
export class ShellCommandPalette {
  readonly open = input.required<boolean>();
  readonly query = input.required<string>();
  readonly selectedIndex = input.required<number>();
  readonly commands = input.required<readonly ShellCommand[]>();
  readonly clockText = input.required<string>();
  readonly dateText = input.required<string>();
  readonly throughputJson = input.required<string>();
  readonly wattsJson = input.required<string>();

  readonly queryChanged = output<Event>();
  readonly keyRequested = output<KeyboardEvent>();
  readonly selectionRequested = output<number>();
  readonly commandRequested = output<number>();
  readonly backdropRequested = output<Event>();

  private readonly queryInput = viewChild<ElementRef<HTMLInputElement>>('queryInput');

  focusInput(): void {
    this.queryInput()?.nativeElement.focus();
  }
}

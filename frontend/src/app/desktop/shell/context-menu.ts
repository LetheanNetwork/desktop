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
import type { ShellContextItem, ShellValueEvent } from './shell.types';

@Component({
  selector: 'lthn-shell-context-menu',
  imports: [CommonModule],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  templateUrl: './context-menu.html',
  styleUrl: './context-menu.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
  encapsulation: ViewEncapsulation.None,
  host: {
    style: 'display:contents',
  },
})
export class ShellContextMenu {
  readonly open = input.required<boolean>();
  readonly left = input.required<number>();
  readonly top = input.required<number>();
  readonly heading = input.required<string>();
  readonly items = input.required<readonly ShellContextItem[]>();
  readonly submenuOpen = input.required<boolean>();
  readonly submenuLeft = input.required<number>();
  readonly submenuTop = input.required<number>();
  readonly submenuIndex = input.required<number>();

  readonly submenuRequested = output<ShellValueEvent<number, MouseEvent>>();
  readonly submenuDismissed = output<void>();
  readonly itemRequested = output<ShellValueEvent<ShellContextItem>>();

  private readonly panel = viewChild<ElementRef<HTMLElement>>('panel');

  get panelElement(): HTMLElement | undefined {
    return this.panel()?.nativeElement;
  }

  requestSubmenu(value: number, event: MouseEvent): void {
    this.submenuRequested.emit({ value, event });
  }

  requestItem(value: ShellContextItem, event: Event): void {
    this.itemRequested.emit({ value, event });
  }
}

import { CommonModule } from '@angular/common';
import {
  ChangeDetectionStrategy,
  Component,
  CUSTOM_ELEMENTS_SCHEMA,
  ViewEncapsulation,
  input,
  output,
} from '@angular/core';
import type { DesktopMenuCategory } from '../desktop-route-tree';
import type { ShellValueEvent } from './shell.types';

export type ShellTrayKey = 'lang' | 'wifi' | 'battery' | 'clock';

@Component({
  selector: 'lthn-shell-menu-bar',
  imports: [CommonModule],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  templateUrl: './menu-bar.html',
  styleUrl: './menu-bar.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
  encapsulation: ViewEncapsulation.None,
  host: {
    style: 'display:contents',
  },
})
export class ShellMenuBar {
  readonly activeMenuKey = input.required<string>();
  readonly activeAppTitle = input.required<string>();
  readonly categories = input.required<readonly DesktopMenuCategory[]>();
  readonly language = input.required<string>();
  readonly clockText = input.required<string>();

  readonly menuRequested = output<ShellValueEvent<string>>();
  readonly menuHovered = output<ShellValueEvent<string, MouseEvent>>();
  readonly categoryRequested = output<ShellValueEvent<DesktopMenuCategory>>();
  readonly trayRequested = output<ShellValueEvent<ShellTrayKey>>();

  requestMenu(value: string, event: Event): void {
    this.menuRequested.emit({ value, event });
  }

  hoverMenu(value: string, event: MouseEvent): void {
    this.menuHovered.emit({ value, event });
  }

  requestCategory(value: DesktopMenuCategory, event: Event): void {
    this.categoryRequested.emit({ value, event });
  }

  requestTray(value: ShellTrayKey, event: Event): void {
    this.trayRequested.emit({ value, event });
  }
}

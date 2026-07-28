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
import type { AppNavItem, DesktopMenuApp, DesktopMenuCategory } from '../desktop-route-tree';
import type {
  ShellChildRequest,
  ShellPosition,
  ShellSessionAction,
  ShellUserIdentity,
  ShellValueEvent,
} from './shell.types';

@Component({
  selector: 'lthn-shell-start-menu',
  imports: [CommonModule],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  templateUrl: './start-menu.html',
  styleUrl: './start-menu.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
  encapsulation: ViewEncapsulation.None,
  host: {
    style: 'display:contents',
  },
})
export class ShellStartMenu {
  readonly open = input.required<boolean>();
  readonly position = input.required<ShellPosition>();
  readonly categories = input.required<readonly DesktopMenuCategory[]>();
  readonly openCategories = input.required<Readonly<Record<string, boolean>>>();
  readonly user = input.required<ShellUserIdentity>();
  readonly submenuOpen = input.required<boolean>();
  readonly submenuLeft = input.required<number>();
  readonly submenuTop = input.required<number>();
  readonly submenuParent = input.required<string>();
  readonly submenuItems = input.required<readonly AppNavItem[]>();

  readonly categoryRequested = output<string>();
  readonly appRequested = output<DesktopMenuApp>();
  readonly appHovered = output<ShellValueEvent<DesktopMenuApp, MouseEvent>>();
  readonly childRequested = output<ShellChildRequest>();
  readonly sessionRequested = output<ShellValueEvent<ShellSessionAction>>();

  private readonly panel = viewChild<ElementRef<HTMLElement>>('panel');

  get panelElement(): HTMLElement | undefined {
    return this.panel()?.nativeElement;
  }

  hoverApp(value: DesktopMenuApp, event: MouseEvent): void {
    this.appHovered.emit({ value, event });
  }

  requestSession(value: ShellSessionAction, event: Event): void {
    this.sessionRequested.emit({ value, event });
  }
}

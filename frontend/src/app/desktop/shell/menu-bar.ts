import { CommonModule } from '@angular/common';
import {
  ChangeDetectionStrategy,
  Component,
  CUSTOM_ELEMENTS_SCHEMA,
  ViewEncapsulation,
  inject,
  input,
  output,
} from '@angular/core';
import { DesktopWindowBridgeService } from '../desktop-window-bridge.service';
import type { DesktopMenuCategory } from '../desktop-route-tree';
import type { ShellTrayKey, ShellValueEvent } from './shell.types';

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
  private readonly windowBridge = inject(DesktopWindowBridgeService);

  /**
   * Native window controls. The bar is the window's only chrome — it is
   * frameless — so these are the real close, minimise and maximise, not the
   * inner desktop's.
   */
  readonly controlSide = this.windowBridge.side;
  readonly controlsAvailable = this.windowBridge.available;
  readonly windowMaximised = this.windowBridge.maximised;
  readonly windowError = this.windowBridge.lastError;

  minimiseWindow(): void {
    void this.windowBridge.minimise();
  }

  toggleMaximiseWindow(): void {
    void this.windowBridge.toggleMaximise();
  }

  closeWindow(): void {
    void this.windowBridge.close();
  }

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

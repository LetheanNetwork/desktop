// SPDX-License-Identifier: EUPL-1.2

import { CommonModule } from '@angular/common';
import {
  ChangeDetectionStrategy,
  Component,
  CUSTOM_ELEMENTS_SCHEMA,
  Input,
  inject,
} from '@angular/core';
import { DesktopWindowBridgeService } from '../desktop-window-bridge.service';
import { WindowManagerService } from '../window-manager.service';
import type { Win } from '../desktop.data';
import { AppView } from './app-view';

/**
 * First-run window: the actions available before there is a user.
 *
 * Two sets, chosen at runtime rather than by build flag or user agent —
 * DesktopWindowBridgeService.available is true only when the wails runtime
 * answered. In a browser nothing here can be configured, so the only action is
 * installing. Installed, the missing thing is a user.
 */
@Component({
  selector: 'lthn-welcome-app',
  standalone: true,
  imports: [CommonModule],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  host: { style: 'display: contents' },
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <div class="wel">
      <h1 i18n="Welcome heading@@welcome.title">Lethean Desktop</h1>

      <!-- Nothing here is configurable from a browser; the only action that
           does anything is getting the software. -->
      <div class="wel-actions" *ngIf="!installed">
        <a
          class="wel-btn wel-btn--primary"
          href="https://lethean.io/download"
          target="_blank"
          rel="noreferrer"
          i18n="Install action@@welcome.action.install"
          >Install Lethean Desktop</a
        >
      </div>

      <div class="wel-actions" *ngIf="installed">
        <button
          type="button"
          class="wel-btn wel-btn--primary"
          (click)="wm.launch('settings')"
          i18n="Create user action@@welcome.action.createUser"
        >
          Create user
        </button>
        <button
          type="button"
          class="wel-btn"
          (click)="wm.launch('settings')"
          i18n="Settings action@@welcome.action.settings"
        >
          Settings
        </button>
      </div>

      <!-- What setup actually produces, as items rather than a sentence. -->
      <ul class="wel-list" *ngIf="installed">
        <li i18n="Setup item, keypair@@welcome.item.keypair">Keypair</li>
        <li i18n="Setup item, workspace@@welcome.item.workspace">Default workspace</li>
        <li i18n="Setup item, credential@@welcome.item.credential">Stored credential</li>
      </ul>
    </div>
  `,
  styles: [
    `
      .wel {
        display: flex;
        flex-direction: column;
        gap: 22px;
        padding: 30px 32px;
      }
      .wel h1 {
        margin: 0;
        font-size: 19px;
      }
      .wel-list {
        display: flex;
        gap: 18px;
        margin: 0;
        padding: 0;
        color: var(--fg-3);
        font-size: 12px;
        list-style: none;
      }
      .wel-actions {
        display: flex;
        flex-wrap: wrap;
        gap: 10px;
      }
      .wel-btn {
        padding: 9px 16px;
        font: inherit;
        font-size: 13px;
        color: var(--fg-2);
        text-decoration: none;
        cursor: pointer;
        background: var(--ink-3);
        border: 1px solid var(--line-1);
        border-radius: 8px;
      }
      .wel-btn--primary {
        color: var(--ink-1);
        background: var(--brand-400);
        border-color: transparent;
      }
    `,
  ],
})
export class WelcomeApp implements AppView {
  private readonly windowBridge = inject(DesktopWindowBridgeService);

  /** Apps navigate through the window manager, per the AppView contract. */
  readonly wm = inject(WindowManagerService);

  @Input() win!: Win;

  /** True inside the desktop app, false in the browser demo. */
  get installed(): boolean {
    return this.windowBridge.available();
  }
}

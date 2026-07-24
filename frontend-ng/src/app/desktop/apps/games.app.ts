// apps/games.app.ts — dumb app-view. CorePlay STIM library + engine adapters.
// Child rail = win.sub, via WindowManagerService.
import {
  Component,
  CUSTOM_ELEMENTS_SCHEMA,
  Input,
  inject,
  ChangeDetectionStrategy,
} from '@angular/core';
import { CommonModule } from '@angular/common';
import { AppView } from './app-view';
import { Win } from '../desktop.data';
import { AppNavItem } from '../desktop-route-tree';
import { WindowManagerService } from '../window-manager.service';

@Component({
  selector: 'lthn-games-app',
  standalone: true,
  imports: [CommonModule],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  host: { style: 'display: contents' },
  changeDetection: ChangeDetectionStrategy.Eager,
  template: `
    <nav class="rail">
      <a
        *ngFor="let it of nav; let last = last"
        [class.on]="(win.sub || 'library') === it[0]"
        [class.last]="last"
        (click)="wm.setSub(win.id, it[0])"
        ><lthn-icon [attr.name]="it[1]" size="15"></lthn-icon
      ></a>
    </nav>
    <div class="appbody" [ngSwitch]="win.sub || 'library'">
      <ng-container *ngSwitchCase="'engines'">
        <div class="ctoolbar">
          <h1 i18n="Game engines heading@@games.enginesHeading">Engines</h1>
          <span class="cfgsrc" i18n="Game engine adapter summary@@games.adapterSummary"
            >9 adapters · build-tagged</span
          >
        </div>
        <div class="engrow" *ngFor="let e of engines">
          <lthn-icon name="microchip" size="13"></lthn-icon><b>{{ e[0] }}</b
          ><span>{{ e[1] }}</span>
        </div>
      </ng-container>
      <ng-container *ngSwitchDefault>
        <div class="ctoolbar">
          <h1 i18n="Games library heading@@games.libraryHeading">Library</h1>
          <span class="cfgsrc" i18n="STIM bundle count@@games.bundleCount"
            >STIM bundles · {{ lib.length }}</span
          ><button class="nbtn" i18n="Verify all game bundles action@@games.verifyAll">
            <lthn-icon name="shield-halved" size="10"></lthn-icon> Verify all
          </button>
        </div>
        <div class="shelf">
          <div class="gcard" *ngFor="let g of lib">
            <div class="gcover">
              <lthn-icon
                [attr.name]="g.verified ? 'shield-halved' : 'triangle-exclamation'"
                size="14"
              ></lthn-icon>
            </div>
            <div class="gmeta">
              <b>{{ g.title }}</b
              ><span>{{ g.year }} · {{ g.platform }}</span
              ><span class="geng"
                ><lthn-status-dot [attr.variant]="g.verified ? 'ok' : 'warn'"></lthn-status-dot
                >{{ g.engine }}</span
              >
            </div>
          </div>
        </div>
      </ng-container>
    </div>
  `,
})
export class GamesApp implements AppView {
  @Input() win!: Win;
  @Input() nav: AppNavItem[] = [];
  wm = inject(WindowManagerService);
  lib = [
    {
      title: 'Mega lo Mania',
      year: 1991,
      platform: 'sega-genesis',
      engine: 'retroarch',
      verified: true,
    },
    { title: 'Command & Conquer', year: 1995, platform: 'dos', engine: 'dosbox', verified: true },
    {
      title: 'The Secret of Monkey Island',
      year: 1990,
      platform: 'scummvm',
      engine: 'scummvm',
      verified: true,
    },
    { title: 'Chuckie Egg', year: 1983, platform: 'zx-spectrum', engine: 'fuse', verified: false },
    { title: 'Street Fighter II', year: 1991, platform: 'arcade', engine: 'mame', verified: true },
    { title: 'Elite', year: 1984, platform: 'commodore-64', engine: 'vice', verified: true },
    { title: 'Super Metroid', year: 1994, platform: 'snes', engine: 'snes9x', verified: true },
    { title: 'Kingdoms', year: 2026, platform: 'lethean', engine: 'native', verified: true },
  ];
  engines: [string, string][] = [
    ['dosbox', 'dos'],
    ['dosbox-x', 'dos · pc-98 · win-3x/9x'],
    ['fuse', 'zx-spectrum'],
    ['mame', 'arcade · neo-geo'],
    ['retroarch', 'genesis · snes · nes · gb …'],
    ['scummvm', 'point-and-click'],
    ['snes9x', 'snes · super-nintendo'],
    ['synthetic', 'smoke-test'],
    ['vice', 'c64 · c128 · vic-20'],
  ];
}

import {
  ChangeDetectionStrategy,
  Component,
  ElementRef,
  ViewEncapsulation,
  computed,
  effect,
  inject,
  viewChild,
} from '@angular/core';
import { toSignal } from '@angular/core/rxjs-interop';
import { ActivatedRoute } from '@angular/router';
import { distinctUntilChanged, map } from 'rxjs';
import { APP_REGISTRY } from './desktop/apps/app-view';
import { APPS } from './desktop/desktop-catalogue.data';
import { Win } from './desktop/desktop.data';
import { PreferencesService } from './desktop/preferences.service';
import { WindowManagerService } from './desktop/window-manager.service';
import { WindowRouteContent } from './desktop/window-route-content';

/**
 * The solo shell — one application in its own native window.
 *
 * A torn-off application still needs the frame it had inside the desktop: the
 * themed surface, the window body, the pane rail, and every rule the shell's
 * applications are written against. Those rules live in the desktop's one
 * global stylesheet, which Angular injects only while DesktopComponent is on
 * screen — so a solo route that rendered the application alone rendered it
 * unstyled, in the browser's own serif.
 *
 * This host therefore loads the same sheet and paints the same frame in the
 * shell's own single-application mode: an OS surface, a window layer, one
 * window filling it. What it does not paint is the desktop — no taskbar, no
 * dock, no menu bar, no second copy of the titlebar the operating system is
 * already drawing.
 *
 * It is an ordinary hash route, so /#/w/<app> and /#/w/<app>/<pane> are
 * deep-linkable in the browser demo too.
 */
@Component({
  selector: 'app-standalone-app-host',
  imports: [WindowRouteContent],
  encapsulation: ViewEncapsulation.None,
  // The desktop's global sheet is the application frame; the second sheet is
  // only what this host's own element needs to be a screen.
  styleUrls: ['./desktop/desktop.component.scss', './standalone-app-host.scss'],
  template: `
    <div
      id="os"
      #screen
      class="mode-shell solo-os"
      [attr.data-wall]="prefs.wallpaper()"
      [attr.data-mode]="prefs.mode() === 'light' ? 'light' : null"
      [class.reduce-motion]="prefs.reduceMotion()"
    >
      <div class="wall"></div>
      <div id="winlayer">
        <div class="win focused">
          @if (win(); as window) {
            <div class="appwrap">
              <lthn-window-route-content [win]="window" />
            </div>
          } @else {
            <p class="solo-empty" i18n="Solo window empty state@@solo.unknownApp">
              That application is not installed on this desktop.
            </p>
          }
        </div>
      </div>
    </div>
  `,
  host: {
    '[attr.data-brand]': 'prefs.brand()',
  },
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class StandaloneAppHost {
  private readonly route = inject(ActivatedRoute);
  private readonly windowManager = inject(WindowManagerService);
  readonly prefs = inject(PreferencesService);

  readonly appId = toSignal(
    this.route.paramMap.pipe(
      map((params) => params.get('app')),
      distinctUntilChanged(),
    ),
    { initialValue: this.route.snapshot.paramMap.get('app') },
  );
  readonly pane = toSignal(
    this.route.paramMap.pipe(
      map((params) => params.get('pane')),
      distinctUntilChanged(),
    ),
    { initialValue: this.route.snapshot.paramMap.get('pane') },
  );
  readonly win = computed<Win | null>(() => {
    const appId = this.appId();
    return appId ? (this.windowManager.wins().find((win) => win.app === appId) ?? null) : null;
  });

  private readonly screen = viewChild<ElementRef<HTMLElement>>('screen');

  constructor() {
    // The chosen design paints onto the screen element, exactly as it does in
    // the shell, so a custom accent follows the application out of it.
    effect(() => {
      const screen = this.screen()?.nativeElement;
      if (screen) this.prefs.applyDesignTo(screen);
    });
    effect(() => {
      const appId = this.appId();
      if (!appId || !APPS[appId] || !APP_REGISTRY[appId]) return;
      const existing = this.windowManager.wins().find((win) => win.app === appId);
      if (!existing) {
        this.windowManager.launch(appId);
        return;
      }
      // The pane the window was showing when it left the shell. Applied here
      // rather than at launch so a deep link into a pane lands on it too.
      const pane = this.pane();
      if (pane && existing.sub !== pane) this.windowManager.setSub(existing.id, pane);
    });
  }
}

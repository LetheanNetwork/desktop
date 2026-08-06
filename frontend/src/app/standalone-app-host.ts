import { NgTemplateOutlet } from '@angular/common';
import {
  CUSTOM_ELEMENTS_SCHEMA,
  ChangeDetectionStrategy,
  Component,
  ElementRef,
  ViewEncapsulation,
  computed,
  effect,
  inject,
  signal,
  viewChild,
} from '@angular/core';
import { toSignal } from '@angular/core/rxjs-interop';
import { ActivatedRoute } from '@angular/router';
import { distinctUntilChanged, map } from 'rxjs';
import { APP_REGISTRY } from './desktop/apps/app-view';
import { APPS } from './desktop/desktop-catalogue.data';
import { DesktopAppWindowBridgeService } from './desktop/desktop-app-window-bridge.service';
import { panesFor } from './desktop/desktop-panes.data';
import { DesktopWindowBridgeService } from './desktop/desktop-window-bridge.service';
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
 * dock, no menu bar, no start menu, no tray.
 *
 * What it does paint is a title bar, because the native window is frameless:
 * the operating system draws nothing at all, so without one there is no way to
 * move the window, close it, or read which application it holds. It is the
 * shell's own window chrome — the traffic lights and the centred title — wired
 * to the real window rather than to the desktop's window manager, plus the one
 * control the shell's titlebar does not need: the way back in.
 *
 * It is an ordinary hash route, so /#/w/<app> and /#/w/<app>/<pane> are
 * deep-linkable in the browser demo too.
 */
@Component({
  selector: 'app-standalone-app-host',
  imports: [NgTemplateOutlet, WindowRouteContent],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
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
          <!-- Native window chrome. The window is frameless, so this strip is
               the only thing to grab it by and the only way out of it. -->
          <ng-template #lights>
            <span
              class="lights"
              [class.unavailable]="!controlsAvailable()"
              [attr.data-wc-error]="windowError() || null"
            >
              <button
                class="c"
                type="button"
                [disabled]="!controlsAvailable()"
                (click)="closeWindow()"
                title="Close"
                i18n-title="Close window@@solo.window.close"
                aria-label="Close window"
                i18n-aria-label="Close window@@solo.window.closeLabel"
              ></button
              ><button
                class="m"
                type="button"
                [disabled]="!controlsAvailable()"
                (click)="minimiseWindow()"
                title="Minimise"
                i18n-title="Minimise window@@solo.window.minimise"
                aria-label="Minimise window"
                i18n-aria-label="Minimise window@@solo.window.minimiseLabel"
              ></button
              ><button
                class="x"
                type="button"
                [disabled]="!controlsAvailable()"
                (click)="toggleMaximiseWindow()"
                [title]="windowMaximised() ? 'Restore' : 'Zoom'"
                aria-label="Zoom or restore window"
                i18n-aria-label="Zoom window@@solo.window.zoomLabel"
              ></button>
            </span>
          </ng-template>
          <ng-template #dock>
            <button
              class="dockback"
              type="button"
              [disabled]="!dockAvailable() || docking()"
              (click)="dockBack()"
              title="Return to the desktop"
              i18n-title="Dock back action@@solo.window.dockBack"
              aria-label="Return to the desktop"
              i18n-aria-label="Dock back action@@solo.window.dockBack"
            >
              <lthn-icon name="down-left-and-up-right-to-center" size="11"></lthn-icon>
            </button>
          </ng-template>

          <div class="titlebar solotitle" [attr.data-controls]="controlSide()">
            <ng-container
              [ngTemplateOutlet]="controlSide() === 'right' ? dock : lights"
            ></ng-container>
            <span class="tb-title">{{ title() }}</span>
            <ng-container
              [ngTemplateOutlet]="controlSide() === 'right' ? lights : dock"
            ></ng-container>
          </div>

          @if (win(); as window) {
            <div class="appwrap">
              <lthn-window-route-content [win]="window" />
            </div>
          } @else {
            <p class="solo-empty" i18n="Solo window empty state@@solo.unknownApp">
              That application is not installed on this desktop.
            </p>
          }
          @if (lastError(); as failure) {
            <p class="solo-error" role="alert">{{ failure }}</p>
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
  private readonly windowBridge = inject(DesktopWindowBridgeService);
  private readonly appWindows = inject(DesktopAppWindowBridgeService);
  readonly prefs = inject(PreferencesService);

  /**
   * The real close, minimise and zoom. These drive the native window this
   * document is in — the bridge addresses it by name — not the desktop's
   * in-canvas window manager, which is not even running here.
   */
  readonly controlSide = this.windowBridge.side;
  readonly controlsAvailable = this.windowBridge.available;
  readonly windowMaximised = this.windowBridge.maximised;
  readonly dockAvailable = this.appWindows.available;

  private readonly _docking = signal(false);

  /** A dock-back in flight. The control stays put until the shell has it. */
  readonly docking = this._docking.asReadonly();

  /**
   * Why a window control did nothing. Published as an attribute the way the
   * shell's menu bar publishes it — visible to anyone inspecting the window,
   * not shouted at someone whose window is behaving.
   */
  readonly windowError = this.windowBridge.lastError;

  /**
   * Why the application is still here.
   *
   * Only ever set by a dock-back the person asked for, so it is a report on
   * their action rather than ambient noise: a control that refuses silently
   * leaves an application stranded in a window with no explanation.
   */
  readonly lastError = this.appWindows.lastError;

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

  /**
   * What the title bar reads. The application, and the pane it is showing when
   * the window carries one — a solo "Control" window is ambiguous in a way
   * "Control · Models" is not.
   */
  readonly title = computed(() => {
    const appId = this.appId();
    const app = appId ? APPS[appId] : undefined;
    if (!app) {
      return $localize`:Solo window title, unknown app@@solo.window.unknownTitle:Lethean Desktop`;
    }
    const sub = this.win()?.sub || this.pane() || '';
    const pane = sub ? panesFor(app.id).find(({ path }) => path === sub) : undefined;
    return pane ? `${app.title} · ${pane.title}` : app.title;
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

  minimiseWindow(): void {
    void this.windowBridge.minimise();
  }

  toggleMaximiseWindow(): void {
    void this.windowBridge.toggleMaximise();
  }

  closeWindow(): void {
    void this.windowBridge.close();
  }

  /**
   * Put the application back on the desktop.
   *
   * The mirror of tear-off, and it carries tear-off's rule: no half-move. The
   * request goes to Go, which validates it and broadcasts the adoption to the
   * shell; only once that has been accepted does this window close. A refused
   * request leaves the window exactly where it is, with the reason on screen.
   */
  async dockBack(): Promise<void> {
    const appId = this.appId();
    if (!appId || this._docking()) return;
    this._docking.set(true);
    try {
      const adopted = await this.appWindows.dockApp({
        app: appId,
        pane: this.win()?.sub || this.pane() || '',
      });
      if (adopted) await this.windowBridge.close();
    } finally {
      this._docking.set(false);
    }
  }
}

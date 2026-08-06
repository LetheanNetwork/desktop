import { ChangeDetectionStrategy, Component, computed, effect, inject } from '@angular/core';
import { toSignal } from '@angular/core/rxjs-interop';
import { ActivatedRoute } from '@angular/router';
import { distinctUntilChanged, map } from 'rxjs';
import { APP_REGISTRY } from './desktop/apps/app-view';
import { APPS } from './desktop/desktop-catalogue.data';
import { Win } from './desktop/desktop.data';
import { WindowManagerService } from './desktop/window-manager.service';
import { WindowRouteContent } from './desktop/window-route-content';

/**
 * The solo shell — one application and nothing else.
 *
 * A torn-off application lives in its own native window, where the operating
 * system paints the frame and the desktop's taskbar, start menu, and window
 * chrome would all be a second copy of things the window already has. So this
 * host renders the application's own content only, pane rail included, at
 * /#/w/<app> and /#/w/<app>/<pane>. It is an ordinary hash route, so the same
 * URL is deep-linkable in the browser demo.
 */
@Component({
  selector: 'app-standalone-app-host',
  imports: [WindowRouteContent],
  template: `
    @if (win(); as window) {
      <lthn-window-route-content [win]="window" />
    } @else {
      <p class="solo-empty" i18n="Solo window empty state@@solo.unknownApp">
        That application is not installed on this desktop.
      </p>
    }
  `,
  styles: `
    :host {
      display: flex;
      flex-direction: column;
      width: 100%;
      height: 100%;
      overflow: hidden;
      background: var(--ink-1);
    }
    .solo-empty {
      margin: auto;
      padding: 24px;
      text-align: center;
      font-size: 13px;
      color: var(--fg-3);
    }
  `,
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class StandaloneAppHost {
  private readonly route = inject(ActivatedRoute);
  private readonly windowManager = inject(WindowManagerService);

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

  constructor() {
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

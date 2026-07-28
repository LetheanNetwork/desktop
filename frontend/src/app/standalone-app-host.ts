import { ChangeDetectionStrategy, Component, computed, effect, inject } from '@angular/core';
import { toSignal } from '@angular/core/rxjs-interop';
import { ActivatedRoute } from '@angular/router';
import { distinctUntilChanged, map } from 'rxjs';
import { APP_REGISTRY } from './desktop/apps/app-view';
import { Win } from './desktop/desktop.data';
import { WindowManagerService } from './desktop/window-manager.service';
import { WindowRouteContent } from './desktop/window-route-content';

@Component({
  selector: 'app-standalone-app-host',
  imports: [WindowRouteContent],
  template: `
    @if (win()) {
      <lthn-window-route-content [win]="win()!" />
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
  readonly win = computed<Win | null>(() => {
    const appId = this.appId();
    return appId ? (this.windowManager.wins().find((win) => win.app === appId) ?? null) : null;
  });
  constructor() {
    effect(() => {
      const appId = this.appId();
      if (
        appId &&
        APP_REGISTRY[appId] &&
        !this.windowManager.wins().some((win) => win.app === appId)
      ) {
        this.windowManager.launch(appId);
      }
    });
  }
}

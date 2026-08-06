// ─────────────────────────────────────────────────────────────────────────
// apps/composite.app.ts — one window, one rail, many panes.
//
// The panelled applications (IDE, Source Control, Project Manager, Agents, …)
// share this shell. It owns nothing but the rail and the pane it is showing:
// `win.sub` names the pane, desktop-panes.data.ts says what that pane renders,
// and the pane component is loaded lazily exactly as a routed application is.
// Panes are real child routes, so /<category>/<app>/<pane> reaches one.
// ─────────────────────────────────────────────────────────────────────────
import {
  ChangeDetectionStrategy,
  ChangeDetectorRef,
  Component,
  CUSTOM_ELEMENTS_SCHEMA,
  Input,
  OnChanges,
  PendingTasks,
  SimpleChanges,
  Type,
  inject,
} from '@angular/core';
import { NgComponentOutlet } from '@angular/common';
import { APPS, type AppDef, type AppNavItem } from '../desktop-catalogue.data';
import { paneFor, type AppPane } from '../desktop-panes.data';
import {
  devPanelEmptyFor,
  devPanelFor,
  EMPTY_DEV_PANEL,
  type DevPanelView,
} from '../dev-panel.data';
import { Win } from '../desktop.data';
import { WindowManagerService } from '../window-manager.service';
import { AppView, PANE_REGISTRY } from './app-view';
import { DevPanelApp } from './dev-panel.app';

interface DevPaneView {
  readonly path: string;
  readonly app: AppDef;
  readonly panel: DevPanelView;
  readonly empty: [string, string, string] | null;
}

@Component({
  selector: 'lthn-composite-app',
  standalone: true,
  imports: [NgComponentOutlet, DevPanelApp],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  host: { style: 'display: contents' },
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <nav class="rail">
      @for (item of nav; track item[0]; let last = $last) {
        <a [class.on]="path === item[0]" [class.last]="last" (click)="wm.setSub(win.id, item[0])">
          <lthn-icon [attr.name]="item[1]" [attr.aria-label]="item[2]" size="15"></lthn-icon>
        </a>
      }
    </nav>
    @if (devPane; as dev) {
      <lthn-dev-panel-app [win]="win" [app]="dev.app" [panel]="dev.panel" [empty]="dev.empty" />
    } @else if (component; as pane) {
      <div class="paneshell">
        <ng-container *ngComponentOutlet="pane; inputs: paneInputs" />
      </div>
    }
  `,
  styles: `
    /* Same box the window gives a surface today, now beside the rail. */
    .paneshell {
      display: flex;
      flex: 1;
      min-width: 0;
      min-height: 0;
      overflow: hidden;
    }
  `,
})
export class CompositeApp implements AppView, OnChanges {
  readonly wm = inject(WindowManagerService);
  private readonly changeDetector = inject(ChangeDetectorRef);
  private readonly pendingTasks = inject(PendingTasks);
  private request = 0;

  @Input({ required: true }) win!: Win;
  @Input() nav: AppNavItem[] = [];

  /** The pane path currently shown; '' before the first resolution. */
  path = '';
  component: Type<AppView> | null = null;
  devPane: DevPaneView | null = null;

  get paneInputs(): Record<string, unknown> {
    return { win: this.win };
  }

  ngOnChanges(changes: SimpleChanges): void {
    if (!changes['win'] || !this.win) return;
    const pane = paneFor(this.win.app, this.win.sub);
    if (!pane) {
      this.path = '';
      this.component = null;
      this.devPane = null;
      return;
    }
    if (pane.path === this.path) return;

    this.path = pane.path;
    const request = ++this.request;
    this.devPane = pane.dev ? this.devPaneView(pane, pane.dev) : null;
    this.component = null;
    if (this.devPane) return;

    const loader = PANE_REGISTRY[pane.surface ?? pane.view ?? ''];
    if (!loader) return;
    // A pane load is a real pending task: the window is not settled until the
    // pane it says it is showing has arrived.
    void this.pendingTasks.run(async () => {
      const component = await loader();
      if (request !== this.request) return;
      this.component = component;
      this.changeDetector.markForCheck();
    });
  }

  private devPaneView(pane: AppPane, route: string): DevPaneView {
    const owner = APPS[this.win.app];
    return {
      path: pane.path,
      app: {
        id: `${pane.app}.${pane.path}`,
        title: pane.title,
        icon: pane.icon,
        category: owner?.category ?? '',
        w: owner?.w ?? 920,
        h: owner?.h ?? 640,
        hint: owner?.hint ?? pane.title,
        dev: true,
        route: pane.dev,
      },
      panel: devPanelFor(route) ?? EMPTY_DEV_PANEL,
      empty: devPanelEmptyFor(route),
    };
  }
}

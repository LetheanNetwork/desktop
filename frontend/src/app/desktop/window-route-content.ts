import {
  ChangeDetectionStrategy,
  ChangeDetectorRef,
  Component,
  Input,
  OnChanges,
  SimpleChanges,
  Type,
  inject,
} from '@angular/core';
import { NgComponentOutlet } from '@angular/common';
import { Router } from '@angular/router';
import { APPS, type AppDef } from './desktop-catalogue.data';
import { EMPTY_DEV_PANEL, type DevPanelView } from './dev-panel.data';
import { Win } from './desktop.data';
import { AppView } from './apps/app-view';
import {
  AppNavItem,
  asAppViewType,
  readDesktopRouteCatalog,
} from './desktop-route-tree';

@Component({
  selector: 'lthn-window-route-content',
  standalone: true,
  imports: [NgComponentOutlet],
  host: { style: 'display: contents' },
  template: `
    @if (component) {
      <ng-container
        *ngComponentOutlet="component; inputs: componentInputs"
      />
    }
  `,
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class WindowRouteContent implements OnChanges {
  private readonly router = inject(Router);
  private readonly changeDetector = inject(ChangeDetectorRef);
  private request = 0;
  private loadedApp = '';

  @Input({ required: true }) win!: Win;
  @Input() panel: DevPanelView = EMPTY_DEV_PANEL;
  @Input() empty: [string, string, string] | null = null;

  component: Type<AppView> | null = null;
  nav: AppNavItem[] = [];

  get app(): AppDef | undefined {
    return APPS[this.win.app];
  }

  get componentInputs(): Record<string, unknown> {
    const inputs: Record<string, unknown> = { win: this.win };
    if (this.nav.length) inputs['nav'] = this.nav;
    // Only an application that IS one developer panel takes the panel inputs.
    // A panelled application has panes, and each pane resolves its own panel.
    if (this.app?.dev && this.app.route && !this.nav.length) {
      inputs['app'] = this.app;
      inputs['panel'] = this.panel;
      inputs['empty'] = this.empty;
    }
    return inputs;
  }

  ngOnChanges(changes: SimpleChanges): void {
    if (!changes['win'] || !this.win) return;
    const route = readDesktopRouteCatalog(this.router.config).apps[this.win.app];
    this.nav = route?.children ?? [];
    if (this.loadedApp === this.win.app && this.component) return;

    const request = ++this.request;
    this.loadedApp = this.win.app;
    this.component = null;
    if (!route) return;

    route.loadComponent().then((component) => {
      if (request !== this.request) return;
      this.component = asAppViewType(component);
      this.changeDetector.markForCheck();
    });
  }
}

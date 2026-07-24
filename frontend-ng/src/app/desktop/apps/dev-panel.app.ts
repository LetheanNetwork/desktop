// apps/dev-panel.app.ts — ONE dumb component for all 14 core/ide developer
// panels. The shell passes the panel spec (`panel`) + app meta; this renders the
// right shape (table/console/term/cards/tree/kanban) with a load-once skeleton
// and an honest empty state. Panel specs live in the shell/data today; in prod
// they arrive per-route from the CoreGo IDE API.
import { Component, CUSTOM_ELEMENTS_SCHEMA, Input, ChangeDetectionStrategy } from '@angular/core';
import { CommonModule } from '@angular/common';
import { AppView } from './app-view';
import { Win, AppDef } from '../desktop.data';

@Component({
  selector: 'lthn-dev-panel-app',
  standalone: true,
  imports: [CommonModule],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  host: { style: 'display: contents' },
  changeDetection: ChangeDetectionStrategy.Eager,
  template: `
    <div class="appbody">
      <div class="ctoolbar">
        <h1>{{ app.title }}</h1>
        <span class="cfgsrc" i18n="Developer panel route breadcrumb@@devPanel.routeBreadcrumb"
          >core/ide · pages/dev/{{ app.route }}</span
        >
      </div>
      <div class="skelwrap" *ngIf="!ready">
        <div class="skel" style="width:34%;height:16px"></div>
        <div class="tiles">
          <div class="skel" style="height:52px"></div>
          <div class="skel" style="height:52px"></div>
          <div class="skel" style="height:52px"></div>
          <div class="skel" style="height:52px"></div>
        </div>
        <div class="skel" style="height:190px"></div>
      </div>
      <div class="devph" *ngIf="ready && empty">
        <span class="devic"><lthn-icon [attr.name]="empty[1]" size="24"></lthn-icon></span
        ><b>{{ empty[0] }}</b>
        <p>{{ empty[2] }}</p>
      </div>
      <ng-container *ngIf="ready && !empty" [ngSwitch]="panel.kind">
        <ng-container *ngSwitchCase="'table'">
          <div class="tiles" *ngIf="panel.tiles">
            <lthn-card pad="11" *ngFor="let t of panel.tiles"
              ><lthn-stat [attr.value]="t[0]" [attr.label]="t[1]" mono></lthn-stat
            ></lthn-card>
          </div>
          <lthn-datatable [attr.columns]="panel.cols" [attr.rows]="panel.rows"></lthn-datatable>
        </ng-container>
        <ng-container *ngSwitchCase="'console'"
          ><div class="logs">
            <div class="ln" *ngFor="let l of panel.lines">
              <span class="t">{{ l[0] }}</span
              ><span class="c">{{ l[1] }}</span
              ><span class="m">{{ l[2] }}</span>
            </div>
          </div></ng-container
        >
        <ng-container *ngSwitchCase="'term'"
          ><div class="logs">
            <div class="ln" *ngFor="let l of panel.lines">
              <span class="m">{{ l }}</span>
            </div>
          </div></ng-container
        >
        <ng-container *ngSwitchCase="'cards'"
          ><div class="shelf">
            <div class="gcard" *ngFor="let c of panel.cards">
              <div class="gcover">
                <lthn-icon [attr.name]="c.icon || app.icon" size="16"></lthn-icon>
              </div>
              <div class="gmeta">
                <b>{{ c.title }}</b
                ><span>{{ c.sub }}</span>
              </div>
            </div>
          </div></ng-container
        >
        <ng-container *ngSwitchCase="'tree'"
          ><div class="tree">
            <div class="trow" *ngFor="let n of panel.nodes" [style.padding-left.px]="n[0] * 16 + 4">
              <lthn-icon [attr.name]="n[2] ? 'folder' : 'file'" size="12"></lthn-icon> {{ n[1] }}
            </div>
          </div></ng-container
        >
        <ng-container *ngSwitchCase="'kanban'"
          ><div class="kanban">
            <div class="kcol" *ngFor="let col of panel.columns">
              <div class="kh">
                {{ col.name }}<span>{{ col.cards.length }}</span>
              </div>
              <div class="kcard" *ngFor="let t of col.cards">{{ t }}</div>
            </div>
          </div></ng-container
        >
        <ng-container *ngSwitchDefault
          ><div class="devph">
            <span class="devic"><lthn-icon [attr.name]="app.icon" size="24"></lthn-icon></span
            ><b>{{ app.title }}</b>
            <p>{{ app.hint }}</p>
            <lthn-badge variant="ok" i18n="Developer panel mock badge@@devPanel.mockBadge"
              >mocked · core/ide</lthn-badge
            >
          </div></ng-container
        >
      </ng-container>
    </div>
  `,
})
export class DevPanelApp implements AppView {
  @Input() win!: Win;
  @Input() app!: AppDef;
  @Input() panel: any = {};
  /** [title, icon, message] when the panel should show an empty state (e.g. clean git tree). */
  @Input() empty: [string, string, string] | null = null;
  ready = false;
  ngOnInit() {
    setTimeout(() => (this.ready = true), 650);
  } // load-once skeleton → content
}

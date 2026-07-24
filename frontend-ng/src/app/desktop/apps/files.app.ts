// apps/files.app.ts — dumb file browser. Reads the FS tree from desktop.data;
// folder nav = win.sub, grid/list = win.systab, driven through WindowManagerService.
import {
  Component,
  CUSTOM_ELEMENTS_SCHEMA,
  Input,
  inject,
  ChangeDetectionStrategy,
  declareExperimentalWebMcpTool,
} from '@angular/core';
import { CommonModule } from '@angular/common';
import { AppView } from './app-view';
import { Win, FS } from '../desktop.data';
import { WindowManagerService } from '../window-manager.service';

@Component({
  selector: 'lthn-files-app',
  standalone: true,
  imports: [CommonModule],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  host: { style: 'display: contents' },
  changeDetection: ChangeDetectionStrategy.Eager,
  template: `
    <div class="fb">
      <div class="fbside">
        <ng-container *ngFor="let grp of places">
          <div class="slab">{{ grp[0] }}</div>
          <div
            class="fbplace"
            *ngFor="let p of grp[1]"
            [class.on]="on(p[0])"
            (click)="wm.setSub(win.id, p[0])"
          >
            <lthn-icon [attr.name]="p[2]" size="15"></lthn-icon> {{ p[1] }}
          </div>
        </ng-container>
      </div>
      <div class="fbmain">
        <div class="fbtop">
          <div class="fbnav">
            <button
              [attr.disabled]="up() ? null : ''"
              (click)="wm.setSub(win.id, up())"
              aria-label="Up"
              i18n-aria-label="Navigate to parent folder@@files.action.up"
            >
              <lthn-icon name="arrow-up" size="13"></lthn-icon>
            </button>
            <button
              [attr.disabled]="id() === 'home' ? '' : null"
              (click)="wm.setSub(win.id, 'home')"
              aria-label="Home"
              i18n-aria-label="Navigate to home folder@@files.action.home"
            >
              <lthn-icon name="house" size="13"></lthn-icon>
            </button>
          </div>
          <div class="fbcrumb">
            <ng-container *ngFor="let c of crumb(); let i = index; let last = last">
              <span class="sep" *ngIf="i">/</span>
              <span class="cr" [class.here]="last" (click)="wm.setSub(win.id, c[0])">{{
                c[1]
              }}</span>
            </ng-container>
          </div>
          <div class="fbvtog">
            <button
              [class.on]="!list()"
              (click)="wm.setSysTab(win.id, 'grid')"
              aria-label="Grid view"
              i18n-aria-label="File grid view action@@files.action.gridView"
            >
              <lthn-icon name="table-cells-large" size="12"></lthn-icon>
            </button>
            <button
              [class.on]="list()"
              (click)="wm.setSysTab(win.id, 'list')"
              aria-label="List view"
              i18n-aria-label="File list view action@@files.action.listView"
            >
              <lthn-icon name="list" size="12"></lthn-icon>
            </button>
          </div>
        </div>
        <div class="fbbody">
          <div class="fbempty" *ngIf="!items().length">
            <lthn-icon
              [attr.name]="id() === 'trash' ? 'trash-can' : 'folder-open'"
              size="36"
            ></lthn-icon>
            <div>{{ emptyLabel() }}</div>
          </div>
          <div class="fblist" *ngIf="items().length && list()">
            <div class="fbrow head">
              <span i18n="File list column@@files.column.name">Name</span
              ><span i18n="File list column@@files.column.size">Size</span
              ><span i18n="File list column@@files.column.modified">Modified</span>
            </div>
            <div
              class="fbrow"
              *ngFor="let it of items()"
              [attr.data-sub]="it.k === 'folder' ? it.to : null"
              (click)="it.k === 'folder' && wm.setSub(win.id, it.to ?? '')"
            >
              <span class="nm"
                ><lthn-icon [attr.name]="icon(it.k)" size="15"></lthn-icon
                ><span>{{ it.n }}</span></span
              ><span class="mut">{{ it.c || '—' }}</span
              ><span class="mut">{{ it.m || '—' }}</span>
            </div>
          </div>
          <div class="fbgrid" *ngIf="items().length && !list()">
            <div
              class="fbcell"
              *ngFor="let it of items()"
              (click)="it.k === 'folder' && wm.setSub(win.id, it.to ?? '')"
            >
              <lthn-icon
                class="fi"
                [class.folder]="it.k === 'folder'"
                [attr.name]="icon(it.k)"
                size="30"
              ></lthn-icon
              ><span class="fn">{{ it.n }}</span
              ><span class="fc">{{ it.c }}</span>
            </div>
          </div>
        </div>
        <div class="fbstatus">
          <span>{{ status() }}</span
          ><span class="v" i18n="Disk free-space status@@files.freeSpace"
            >218 GB free of 512 GB</span
          >
        </div>
      </div>
    </div>
  `,
})
export class FilesApp implements AppView {
  @Input() win!: Win;
  wm = inject(WindowManagerService);
  private readonly mcpTools = this.registerMcpTools();
  places: [string, [string, string, string][]][] = [
    [
      $localize`:File browser sidebar group@@files.sidebar.favourites:Favourites`,
      [
        ['home', $localize`:File browser place@@files.place.home:Home`, 'house'],
        ['documents', $localize`:File browser place@@files.place.documents:Documents`, 'folder'],
        ['downloads', $localize`:File browser place@@files.place.downloads:Downloads`, 'download'],
        ['models', $localize`:File browser place@@files.place.models:Models`, 'cube'],
        ['projects', $localize`:File browser place@@files.place.projects:Projects`, 'folder-tree'],
      ],
    ],
    [
      $localize`:File browser sidebar group@@files.sidebar.locations:Locations`,
      [
        [
          'lethernet',
          $localize`:Application title@@app.lethernet.title:LetherNet`,
          'network-wired',
        ],
        ['trash', $localize`:File browser place@@files.place.trash:Trash`, 'trash-can'],
      ],
    ],
  ];
  id() {
    return FS[this.win.sub] ? this.win.sub : 'home';
  }
  folder() {
    return FS[this.id()];
  }
  items() {
    return this.folder().items;
  }
  up() {
    return this.folder().up || '';
  }
  list() {
    return this.win.systab === 'list';
  }
  emptyLabel() {
    return this.id() === 'trash'
      ? $localize`:Empty trash message@@files.empty.trash:Trash is empty`
      : $localize`:Empty folder message@@files.empty.folder:This folder is empty`;
  }
  crumb(): [string, string][] {
    const out: [string, string][] = [];
    let c: string | null = this.id();
    while (c && FS[c]) {
      out.unshift([c, FS[c].name]);
      c = FS[c].up;
    }
    return out;
  }
  on(id: string) {
    return this.id() === id || this.crumb().some((c) => c[0] === id);
  }
  icon(k: string) {
    return (
      (
        {
          folder: 'folder',
          doc: 'file-lines',
          pdf: 'file-pdf',
          img: 'file-image',
          code: 'file-code',
          zip: 'file-zipper',
          model: 'cube',
          audio: 'file-audio',
        } as any
      )[k] || 'file'
    );
  }
  status() {
    const items = this.items();
    if (!items.length) return $localize`:File browser item count@@files.status.noItems:0 items`;
    const nf = items.filter((i) => i.k === 'folder').length,
      nfile = items.length - nf;
    const itemLabel =
      items.length === 1
        ? $localize`:Singular item label@@files.count.item:item`
        : $localize`:Plural item label@@files.count.items:items`;
    const folderLabel =
      nf === 1
        ? $localize`:Singular folder label@@files.count.folder:folder`
        : $localize`:Plural folder label@@files.count.folders:folders`;
    const fileLabel =
      nfile === 1
        ? $localize`:Singular file label@@files.count.file:file`
        : $localize`:Plural file label@@files.count.files:files`;
    return $localize`:File browser item summary@@files.status.summary:${items.length}:itemCount: ${itemLabel}:itemLabel: · ${nf}:folderCount: ${folderLabel}:folderLabel:, ${nfile}:fileCount: ${fileLabel}:fileLabel:`;
  }

  private async registerMcpTools(): Promise<void> {
    const locations = Object.keys(FS);

    await Promise.all([
      declareExperimentalWebMcpTool({
        name: 'files_read_location',
        description: 'Reads the Files app location, breadcrumbs, view mode, and visible items.',
        inputSchema: {
          type: 'object',
          properties: {},
          additionalProperties: false,
        },
        execute: () => ({
          content: [
            {
              type: 'text',
              text: JSON.stringify({
                location_id: this.id(),
                location_name: this.folder().name,
                breadcrumbs: this.crumb().map(([id, name]) => ({ id, name })),
                view: this.list() ? 'list' : 'grid',
                items: this.items(),
              }),
            },
          ],
        }),
      }),
      declareExperimentalWebMcpTool({
        name: 'files_navigate',
        description: 'Navigates the Files app to a known location.',
        inputSchema: {
          type: 'object',
          properties: {
            location_id: {
              type: 'string',
              enum: locations,
              description: 'Location id from files_read_location.',
            },
          },
          required: ['location_id'],
          additionalProperties: false,
        },
        execute: ({ location_id }) => {
          if (!Object.hasOwn(FS, location_id)) {
            throw new Error(
              `Unknown Files location "${location_id}". Expected one of: ${locations.join(', ')}.`,
            );
          }
          this.wm.setSub(this.win.id, location_id);
          return {
            content: [
              {
                type: 'text',
                text: JSON.stringify({ ok: true, location_id }),
              },
            ],
          };
        },
      }),
      declareExperimentalWebMcpTool({
        name: 'files_set_view',
        description: 'Switches the Files app between grid and list views.',
        inputSchema: {
          type: 'object',
          properties: {
            view: {
              type: 'string',
              enum: ['grid', 'list'],
              description: 'Files item presentation.',
            },
          },
          required: ['view'],
          additionalProperties: false,
        },
        execute: ({ view }) => {
          if (view !== 'grid' && view !== 'list') {
            throw new Error(`Unknown Files view "${view}". Expected grid or list.`);
          }
          this.wm.setSysTab(this.win.id, view);
          return {
            content: [
              {
                type: 'text',
                text: JSON.stringify({ ok: true, view }),
              },
            ],
          };
        },
      }),
    ]);
  }
}

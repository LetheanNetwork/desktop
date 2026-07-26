// apps/files.app.ts — dumb file browser. Reads the FS tree from desktop.data;
// folder nav = win.sub, grid/list = win.systab, driven through WindowManagerService.
import {
  Component,
  CUSTOM_ELEMENTS_SCHEMA,
  Input,
  OnInit,
  computed,
  inject,
  ChangeDetectionStrategy,
  declareExperimentalWebMcpTool,
  signal,
  ViewEncapsulation,
} from '@angular/core';
import { CommonModule } from '@angular/common';
import { AppView } from './app-view';
import { Win, FS, type FsNode } from '../desktop.data';
import { WindowManagerService } from '../window-manager.service';
import { DesktopLiveDataService, FilesSnapshot } from '../desktop-live-data.service';
import { DesktopDataStateBadge } from '../desktop-data-state-badge';
import { DesktopDataState } from '../desktop-data-state';

type FilePlace = [string, string, string];
type FilePlaceGroup = [string, FilePlace[]];

@Component({
  selector: 'lthn-files-app',
  standalone: true,
  imports: [CommonModule, DesktopDataStateBadge],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  host: { style: 'display: contents' },
  styleUrl: './files/files.app.scss',
  encapsulation: ViewEncapsulation.None,
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <div class="fb">
      <div class="fbside">
        <ng-container *ngFor="let grp of places()">
          <div class="slab">{{ grp[0] }}</div>
          <div
            class="fbplace"
            *ngFor="let p of grp[1]"
            [class.on]="on(p[0])"
            (click)="navigate(p[0])"
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
              (click)="navigate(up())"
              aria-label="Up"
              i18n-aria-label="Navigate to parent folder@@files.action.up"
            >
              <lthn-icon name="arrow-up" size="13"></lthn-icon>
            </button>
            <button
              [attr.disabled]="id() === 'home' ? '' : null"
              (click)="navigate('home')"
              aria-label="Home"
              i18n-aria-label="Navigate to home folder@@files.action.home"
            >
              <lthn-icon name="house" size="13"></lthn-icon>
            </button>
          </div>
          <div class="fbcrumb">
            <ng-container *ngFor="let c of crumb(); let i = index; let last = last">
              <span class="sep" *ngIf="i">/</span>
              <span class="cr" [class.here]="last" (click)="navigate(c[0])">{{ c[1] }}</span>
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
              (click)="it.k === 'folder' && navigate(it.to ?? '')"
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
              (click)="it.k === 'folder' && navigate(it.to ?? '')"
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
          <lthn-desktop-data-state [state]="dataState()" />
          <span>{{ status() }}</span
          ><span class="v">{{ diskLabel() }}</span>
        </div>
      </div>
    </div>
  `,
})
export class FilesApp implements AppView, OnInit {
  @Input() win!: Win;
  wm = inject(WindowManagerService);
  private readonly liveData = inject(DesktopLiveDataService);
  private readonly demoPlaces: FilePlaceGroup[] = [
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
  private readonly liveSnapshot = signal<FilesSnapshot | null>(null);
  private readonly liveFileSystem = signal<Record<string, FsNode>>({});
  readonly dataState = signal<DesktopDataState>(
    this.liveData.mode() === 'demo' ? 'demo' : 'loading',
  );
  readonly diskLabel = computed(() => {
    const disk = this.liveSnapshot()?.disk;
    return disk
      ? `${disk.free} free of ${disk.total}`
      : $localize`:Demo disk free-space status@@files.freeSpace:218 GB free of 512 GB`;
  });
  readonly places = computed<FilePlaceGroup[]>(() => {
    const snapshot = this.liveSnapshot();
    if (this.dataState() !== 'live' || !snapshot) return this.demoPlaces;
    return [
      [
        $localize`:File browser sidebar group@@files.sidebar.favourites:Favourites`,
        [['home', $localize`:File browser place@@files.place.home:Home`, 'house']],
      ],
      [
        $localize`:File browser sidebar group@@files.sidebar.locations:Locations`,
        snapshot.locations.map((location): FilePlace => [
          locationId(location.name),
          location.name,
          locationIcon(location.name),
        ]),
      ],
    ];
  });
  private readonly mcpTools = this.registerMcpTools();

  ngOnInit(): void {
    if (this.liveData.mode() === 'demo') {
      this.dataState.set('demo');
      return;
    }
    this.dataState.set('loading');
    void this.refresh(this.win.sub || 'home');
  }

  async refresh(requestedId = this.id()): Promise<void> {
    if (this.liveData.mode() === 'demo') return;
    try {
      const requestedLocation = locationNameForId(this.liveSnapshot(), requestedId);
      const snapshot = await this.liveData.files(requestedLocation);
      this.liveSnapshot.set(snapshot);
      this.liveFileSystem.set(buildLiveFileSystem(snapshot, requestedId));
      this.dataState.set('live');
    } catch {
      this.liveSnapshot.set(null);
      this.liveFileSystem.set({});
      this.dataState.set('unavailable');
    }
  }

  navigate(id: string): void {
    if (!id) return;
    this.wm.setSub(this.win.id, id);
    if (this.liveData.mode() === 'live') void this.refresh(id);
  }

  private fileSystem(): Record<string, FsNode> {
    return this.dataState() === 'live' ? this.liveFileSystem() : FS;
  }

  id() {
    const fileSystem = this.fileSystem();
    return fileSystem[this.win.sub] ? this.win.sub : 'home';
  }
  folder() {
    return this.fileSystem()[this.id()];
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
    const fileSystem = this.fileSystem();
    let c: string | null = this.id();
    while (c && fileSystem[c]) {
      out.unshift([c, fileSystem[c].name]);
      c = fileSystem[c].up;
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
    const locations = [
      ...Object.keys(FS),
      'code',
      'lthn-models',
      'recordings',
      'screenshots',
    ].filter((id, index, values) => values.indexOf(id) === index);

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
          const available = Object.keys(this.fileSystem());
          if (!available.includes(location_id)) {
            throw new Error(
              `Unknown Files location "${location_id}". Expected one of: ${available.join(', ')}.`,
            );
          }
          this.navigate(location_id);
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

function buildLiveFileSystem(snapshot: FilesSnapshot, requestedId: string): Record<string, FsNode> {
  const recentItems: FsNode['items'] = snapshot.recent.map((file) => ({
    n: file.name,
    k: fileKind(file.name),
    c: file.size,
    m: `${file.when} · ${file.path}`,
  }));
  const locationItems: FsNode['items'] = snapshot.locations.map((location) => ({
    n: location.name,
    k: 'folder',
    to: locationId(location.name),
    c: `${location.count} ${location.count === 1 ? 'item' : 'items'} · ${location.size}`,
  }));
  const fileSystem: Record<string, FsNode> = {
    home: {
      name: $localize`:File browser place@@files.place.home:Home`,
      up: null,
      items: [...locationItems, ...recentItems],
    },
  };
  for (const location of snapshot.locations) {
    const id = locationId(location.name);
    fileSystem[id] = {
      name: location.name,
      up: 'home',
      items: requestedId === id ? recentItems : [],
    };
  }
  return fileSystem;
}

function locationId(name: string): string {
  return (
    name
      .toLocaleLowerCase('en-GB')
      .replace(/[^a-z0-9]+/g, '-')
      .replace(/^-|-$/g, '') || 'location'
  );
}

function locationNameForId(snapshot: FilesSnapshot | null, id: string): string {
  if (!id || id === 'home') return '';
  const liveName = snapshot?.locations.find((location) => locationId(location.name) === id)?.name;
  if (liveName) return liveName;
  const known: Record<string, string> = {
    code: 'Code',
    documents: 'Documents',
    models: 'lthn / models',
    'lthn-models': 'lthn / models',
    recordings: 'Recordings',
    screenshots: 'Screenshots',
  };
  return known[id] ?? '';
}

function locationIcon(name: string): string {
  const id = locationId(name);
  if (id.includes('model')) return 'cube';
  if (id.includes('record')) return 'microphone';
  if (id.includes('screen')) return 'image';
  if (id === 'code') return 'code';
  return 'folder';
}

function fileKind(name: string): string {
  const extension = name.split('.').at(-1)?.toLocaleLowerCase('en-GB') ?? '';
  if (extension === 'pdf') return 'pdf';
  if (['png', 'jpg', 'jpeg', 'gif', 'webp', 'svg'].includes(extension)) return 'img';
  if (['zip', 'gz', 'tgz', 'tar', 'dmg'].includes(extension)) return 'zip';
  if (['gguf', 'safetensors'].includes(extension)) return 'model';
  if (['mp3', 'wav', 'm4a', 'flac'].includes(extension)) return 'audio';
  if (['ts', 'js', 'go', 'json', 'yaml', 'yml', 'md', 'html', 'css', 'scss'].includes(extension)) {
    return 'code';
  }
  return 'doc';
}

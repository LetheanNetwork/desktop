import {
  ChangeDetectionStrategy,
  Component,
  CUSTOM_ELEMENTS_SCHEMA,
  Injector,
  Input,
  OnInit,
  PendingTasks,
  ViewEncapsulation,
  computed,
  declareExperimentalWebMcpTool,
  inject,
  signal,
} from '@angular/core';
import { ConnectionManagerService } from '../../connection-manager.service';
import { DesktopFilesBridgeService } from '../desktop-files-bridge.service';
import type { Win } from '../desktop.data';
import { WindowManagerService } from '../window-manager.service';
import { AppView } from './app-view';
import { FilesBrowserView } from './files/files-browser.view';
import { FilesDemoStore } from './files/files-demo.store';
import { FilesOperationDialogViewComponent } from './files/files-operation-dialog.view';
import { FilesPreviewView } from './files/files-preview.view';
import { FilesSidebarView } from './files/files-sidebar.view';
import { FilesStatusView } from './files/files-status.view';
import { FilesToolbarView } from './files/files-toolbar.view';
import type {
  DirectorySnapshotView,
  FilePreviewView,
  FilesActionIntent,
  FilesBrowserEntryView,
  FilesCatalogueView,
  FilesDataSource,
  FilesDataState,
  FilesLocation,
  FilesOperationDialogView,
  FilesViewMode,
  TrashSnapshotView,
} from './files/files-view.models';
import {
  buildFilesViewState,
  filesToken,
  parseFilesToken,
  reconcileLocation,
} from './files/files-view-state';

const EMPTY_CATALOGUE: FilesCatalogueView = {
  mounts: [],
  favourites: [],
  recent: [],
};

@Component({
  selector: 'lthn-files-app',
  standalone: true,
  imports: [
    FilesSidebarView,
    FilesToolbarView,
    FilesBrowserView,
    FilesStatusView,
    FilesPreviewView,
    FilesOperationDialogViewComponent,
  ],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  host: { style: 'display: contents' },
  styleUrl: './files/files.app.scss',
  encapsulation: ViewEncapsulation.None,
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <div class="fb">
      <lthn-files-sidebar-view [state]="viewState()" (intent)="handleIntent($event)" />
      <main class="fbmain">
        <lthn-files-toolbar-view
          [state]="viewState()"
          [selection]="selection()"
          (intent)="handleIntent($event)"
        />
        <lthn-files-browser-view
          [state]="viewState()"
          [selectedKey]="selectedKey()"
          (intent)="handleIntent($event)"
        />
        <lthn-files-status-view [state]="viewState()" />
      </main>
      @if (preview(); as filePreview) {
        <lthn-files-preview-view [preview]="filePreview" (intent)="handleIntent($event)" />
      }
      @if (dialog(); as operationDialog) {
        <lthn-files-operation-dialog-view
          [dialog]="operationDialog"
          [catalogue]="catalogue()"
          (intent)="handleIntent($event)"
        />
      }
    </div>
  `,
})
export class FilesApp implements AppView, OnInit {
  @Input() win!: Win;

  private readonly connection = inject(ConnectionManagerService);
  private readonly bridge = inject(DesktopFilesBridgeService);
  private readonly wm = inject(WindowManagerService);
  private readonly pendingTasks = inject(PendingTasks);
  private readonly injector = inject(Injector);
  private readonly demoStore = new FilesDemoStore();

  readonly catalogue = signal<FilesCatalogueView>(EMPTY_CATALOGUE);
  readonly location = signal<FilesLocation>({ kind: 'home' });
  readonly directory = signal<DirectorySnapshotView | null>(null);
  readonly trashSnapshot = signal<TrashSnapshotView | null>(null);
  readonly preview = signal<FilePreviewView | null>(null);
  readonly dialog = signal<FilesOperationDialogView | null>(null);
  readonly dataState = signal<FilesDataState>(this.connection.offline() ? 'demo' : 'loading');
  readonly viewMode = signal<FilesViewMode>('grid');
  readonly selectedKey = signal('');
  readonly failure = signal('');

  readonly viewState = computed(() =>
    buildFilesViewState({
      catalogue: this.catalogue(),
      location: this.location(),
      directory: this.directory(),
      trash: this.trashSnapshot(),
      dataState: this.dataState(),
      viewMode: this.viewMode(),
    }),
  );
  readonly selection = computed<FilesBrowserEntryView | null>(
    () => this.viewState().entries.find((entry) => entryKey(entry) === this.selectedKey()) ?? null,
  );

  private loadVersion = 0;
  private previewVersion = 0;
  private hasSuccessfulView = false;
  private readonly observedMcpTokens = new Set<string>();

  ngOnInit(): void {
    this.viewMode.set(this.win.systab === 'grid' ? 'grid' : 'list');
    this.pendingTasks.run(async () => {
      await Promise.all([
        this.initialise(),
        this.win.app === 'files' ? this.registerMcpTools() : Promise.resolve(),
      ]);
    });
  }

  handleIntent(intent: FilesActionIntent): void {
    switch (intent.type) {
      case 'navigate':
        this.pendingTasks.run(() => this.navigateToken(intent.token));
        return;
      case 'home':
        this.pendingTasks.run(() => this.navigateToken('home'));
        return;
      case 'up':
        if (this.viewState().upToken) {
          this.pendingTasks.run(() => this.navigateToken(this.viewState().upToken));
        }
        return;
      case 'refresh':
        this.pendingTasks.run(() => this.refresh());
        return;
      case 'set-view':
        this.viewMode.set(intent.view);
        this.wm.setSysTab(this.win.id, intent.view);
        return;
      case 'select-entry':
        this.selectedKey.set(`${intent.mountId}::${intent.path}::${intent.receiptId}`);
        return;
      case 'open-directory':
        this.pendingTasks.run(() =>
          this.navigateLocation({
            kind: 'directory',
            mountId: intent.mountId,
            path: intent.path,
          }),
        );
        return;
      case 'preview':
        this.pendingTasks.run(() => this.loadPreview(intent.mountId, intent.path));
        return;
      case 'close-preview':
        this.preview.set(null);
        return;
      case 'dismiss-dialog':
        this.dialog.set(null);
        return;
      case 'open-operation':
      case 'submit-operation':
        // Task 14 owns mutation orchestration. The typed views are already wired.
        return;
    }
  }

  async navigateToken(token: string): Promise<void> {
    const location = reconcileLocation(parseFilesToken(token), this.catalogue());
    await this.navigateLocation(location);
  }

  async refresh(): Promise<void> {
    const version = ++this.loadVersion;
    const source = this.source();
    const previousState = this.dataState();
    if (!this.connection.offline()) this.dataState.set('loading');
    try {
      let catalogue = this.catalogue();
      if (this.location().kind === 'home') {
        catalogue = await source.listMounts();
        if (version !== this.loadVersion) return;
        this.catalogue.set(catalogue);
        this.location.set(reconcileLocation(this.location(), catalogue));
      }
      await this.loadLocation(source, this.location(), version);
      if (version !== this.loadVersion) return;
      this.markSuccessful();
    } catch (error) {
      if (version !== this.loadVersion) return;
      this.applyLoadFailure(error, previousState);
    }
  }

  private async initialise(): Promise<void> {
    const version = ++this.loadVersion;
    const source = this.source();
    this.dataState.set(this.connection.offline() ? 'demo' : 'loading');
    try {
      const catalogue = await source.listMounts();
      if (version !== this.loadVersion) return;
      this.catalogue.set(catalogue);
      const location = reconcileLocation(parseFilesToken(this.win.sub || 'home'), catalogue);
      this.location.set(location);
      await this.loadLocation(source, location, version);
      if (version !== this.loadVersion) return;
      this.markSuccessful();
    } catch (error) {
      if (version !== this.loadVersion) return;
      this.applyLoadFailure(error, this.dataState());
    }
  }

  private async navigateLocation(location: FilesLocation): Promise<void> {
    const reconciled = reconcileLocation(location, this.catalogue());
    const token = filesToken(reconciled);
    this.wm.setSub(this.win.id, token);
    this.location.set(reconciled);
    this.selectedKey.set('');
    this.preview.set(null);
    this.failure.set('');

    const version = ++this.loadVersion;
    const previousState = this.dataState();
    if (!this.connection.offline()) this.dataState.set('loading');
    try {
      await this.loadLocation(this.source(), reconciled, version);
      if (version !== this.loadVersion) return;
      this.markSuccessful();
    } catch (error) {
      if (version !== this.loadVersion) return;
      this.applyLoadFailure(error, previousState);
    }
  }

  private async loadLocation(
    source: FilesDataSource,
    location: FilesLocation,
    version: number,
  ): Promise<void> {
    if (location.kind === 'home') {
      if (version !== this.loadVersion) return;
      this.directory.set(null);
      this.trashSnapshot.set(null);
      return;
    }
    if (location.kind === 'trash') {
      const trash = await source.listTrash();
      if (version !== this.loadVersion) return;
      this.directory.set(null);
      this.trashSnapshot.set(trash);
      return;
    }
    const directory = await source.listDirectory({
      mountId: location.mountId,
      path: location.path,
      cursor: '',
      limit: 200,
    });
    if (version !== this.loadVersion) return;
    this.directory.set(directory);
    this.trashSnapshot.set(null);
  }

  private async loadPreview(mountId: string, path: string): Promise<void> {
    const version = ++this.previewVersion;
    try {
      const preview = await this.source().preview({ mountId, path });
      if (version !== this.previewVersion) return;
      this.preview.set(preview);
      this.failure.set('');
    } catch (error) {
      if (version !== this.previewVersion) return;
      this.failure.set(errorMessage(error));
    }
  }

  private markSuccessful(): void {
    this.hasSuccessfulView = true;
    this.failure.set('');
    this.dataState.set(this.connection.offline() ? 'demo' : 'live');
  }

  private applyLoadFailure(error: unknown, previousState: FilesDataState): void {
    this.failure.set(errorMessage(error));
    if (this.hasSuccessfulView) {
      this.dataState.set(this.connection.offline() ? previousState : 'stale');
      return;
    }
    this.catalogue.set(EMPTY_CATALOGUE);
    this.location.set({ kind: 'home' });
    this.directory.set(null);
    this.trashSnapshot.set(null);
    this.dataState.set('unavailable');
  }

  private source(): FilesDataSource {
    return this.connection.offline() ? this.demoStore : this.bridge;
  }

  private async registerMcpTools(): Promise<void> {
    await Promise.all([
      declareExperimentalWebMcpTool(
        {
          name: 'files_read_location',
          description:
            'Reads the Files location, breadcrumbs, view mode, data state, and visible provider-neutral entries.',
          inputSchema: {
            type: 'object',
            properties: {},
            additionalProperties: false,
          },
          execute: () => {
            const state = this.viewState();
            const observed = [state.token, ...state.breadcrumbs.map(({ token }) => token)];
            observed.forEach((token) => this.observedMcpTokens.add(token));
            const mountId = state.location.kind === 'directory' ? state.location.mountId : '';
            const path = state.location.kind === 'directory' ? state.location.path : '';
            return {
              content: [
                {
                  type: 'text',
                  text: JSON.stringify({
                    location_id: state.token,
                    location_name: state.providerLabel,
                    mount_id: mountId,
                    path,
                    breadcrumbs: state.breadcrumbs.map(({ token, label }) => ({
                      id: token,
                      name: label,
                    })),
                    view: state.viewMode,
                    data_state: state.dataState,
                    items: state.entries.map((entry) => ({
                      mountId: entry.mountId,
                      path: entry.relativePath,
                      name: entry.name,
                      kind: entry.kind,
                      sizeBytes: entry.sizeBytes,
                      modifiedAt: entry.modifiedAt,
                    })),
                  }),
                },
              ],
            };
          },
        },
        this.injector,
      ),
      declareExperimentalWebMcpTool(
        {
          name: 'files_navigate',
          description:
            'Navigates Files to a current root or a nested token previously returned by files_read_location.',
          inputSchema: {
            type: 'object',
            properties: {
              location_id: {
                type: 'string',
                description: 'Location token from files_read_location.',
              },
            },
            required: ['location_id'],
            additionalProperties: false,
          },
          execute: async ({ location_id }) => {
            const rootTokens = ['home', 'trash', ...this.catalogue().mounts.map(({ id }) => id)];
            if (!rootTokens.includes(location_id) && !this.observedMcpTokens.has(location_id)) {
              throw new Error(
                `Unknown Files location "${location_id}". Expected one of: ${rootTokens.join(', ')}.`,
              );
            }
            await this.navigateToken(location_id);
            return {
              content: [
                {
                  type: 'text',
                  text: JSON.stringify({ ok: true, location_id }),
                },
              ],
            };
          },
        },
        this.injector,
      ),
      declareExperimentalWebMcpTool(
        {
          name: 'files_set_view',
          description: 'Switches Files between grid and list views.',
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
            this.viewMode.set(view);
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
        },
        this.injector,
      ),
    ]);
  }
}

function entryKey(entry: FilesBrowserEntryView): string {
  return `${entry.mountId}::${entry.relativePath}::${entry.receiptId}`;
}

function errorMessage(error: unknown): string {
  return error instanceof Error
    ? error.message
    : $localize`:Unknown Files provider error@@files.error.unknown:The Files provider is unavailable.`;
}

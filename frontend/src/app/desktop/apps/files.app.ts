import {
  ChangeDetectionStrategy,
  Component,
  CUSTOM_ELEMENTS_SCHEMA,
  DestroyRef,
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
import { DesktopHostIntentService, type DesktopHostItem } from '../desktop-host-intent.service';
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
  FileAddressView,
  FilePreviewView,
  FilesActionIntent,
  FilesBrowserEntryView,
  FilesCatalogueView,
  FilesChangedEvent,
  FilesDataSource,
  FilesDataState,
  FilesLocation,
  FilesOperationKind,
  FilesOperationDialogView,
  FileOperationResultView,
  FilesViewState,
  FilesViewMode,
  TrashSnapshotView,
} from './files/files-view.models';
import {
  buildFilesViewState,
  eventAffectsLocation,
  filesToken,
  parseFilesToken,
  parentPath,
  reconcileLocation,
  validRelativePath,
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
        @if (failure()) {
          <div class="fberror" role="alert">{{ failure() }}</div>
        }
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
  private readonly hostIntents = inject(DesktopHostIntentService);
  private readonly wm = inject(WindowManagerService);
  private readonly pendingTasks = inject(PendingTasks);
  private readonly injector = inject(Injector);
  private readonly destroyRef = inject(DestroyRef);
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
  private refreshScheduled = false;
  private refreshTimer: ReturnType<typeof setTimeout> | null = null;
  private refreshTaskDone: (() => void) | null = null;
  private destroyed = false;
  private filesEventsOff: (() => void) | null = null;
  private hostItemsOff: (() => void) | null = null;
  private readonly observedMcpTokens = new Set<string>();

  constructor() {
    this.destroyRef.onDestroy(() => {
      this.destroyed = true;
      if (this.refreshTimer !== null) clearTimeout(this.refreshTimer);
      this.refreshTimer = null;
      this.refreshTaskDone?.();
      this.refreshTaskDone = null;
      this.filesEventsOff?.();
      this.filesEventsOff = null;
      this.hostItemsOff?.();
      this.hostItemsOff = null;
    });
  }

  ngOnInit(): void {
    this.viewMode.set(this.win.systab === 'grid' ? 'grid' : 'list');
    this.pendingTasks.run(async () => {
      await this.initialise();
      this.hostItemsOff = this.hostIntents.onItems('files', (items) => {
        this.pendingTasks.run(() => this.openHostItems(items));
      });
      if (this.win.app === 'files') await this.registerMcpTools();
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
      case 'open-host':
      case 'reveal-host':
        this.pendingTasks.run(() => this.performHostAction(intent));
        return;
      case 'close-preview':
        this.preview.set(null);
        return;
      case 'dismiss-dialog':
        this.dialog.set(null);
        return;
      case 'open-operation':
        this.openOperation(intent);
        return;
      case 'submit-operation':
        this.pendingTasks.run(() => this.performOperation(intent));
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

  private async openHostItems(items: readonly DesktopHostItem[]): Promise<void> {
    const item = items[0];
    if (!item) return;
    if (item.kind === 'directory') {
      await this.navigateLocation({
        kind: 'directory',
        mountId: item.mountId,
        path: item.path,
      });
      return;
    }

    await this.navigateLocation({
      kind: 'directory',
      mountId: item.mountId,
      path: parentPath(item.path),
    });
    this.selectedKey.set(`${item.mountId}::${item.path}::`);
    await this.loadPreview(item.mountId, item.path);
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

  private async performHostAction(
    intent: Extract<FilesActionIntent, { type: 'open-host' | 'reveal-host' }>,
  ): Promise<void> {
    if (this.connection.offline()) {
      this.failure.set(
        $localize`:Files native action offline@@files.error.nativeOffline:This action is available in the native desktop app.`,
      );
      return;
    }
    try {
      const address = { mountId: intent.mountId, path: intent.path };
      if (intent.type === 'open-host') {
        await this.bridge.open(address);
      } else {
        await this.bridge.reveal(address);
      }
      this.failure.set('');
    } catch (error) {
      this.failure.set(errorMessage(error));
    }
  }

  private openOperation(intent: Extract<FilesActionIntent, { type: 'open-operation' }>): void {
    const source = intent.source;
    switch (intent.operation) {
      case 'create-directory':
        if (!source) return;
        this.dialog.set({
          state: 'form',
          operation: intent.operation,
          title: $localize`:Create folder dialog title@@files.dialog.createFolder:Create folder`,
          message: $localize`:Create folder dialog message@@files.dialog.createFolderMessage:Choose a name for the new folder.`,
          source,
        });
        return;
      case 'rename':
        if (!source) return;
        this.dialog.set({
          state: 'form',
          operation: intent.operation,
          title: $localize`:Rename Files entry dialog title@@files.dialog.rename:Rename`,
          message: $localize`:Rename Files entry dialog message@@files.dialog.renameMessage:Enter one new name for this item.`,
          source,
          name: basename(source.path),
        });
        return;
      case 'copy':
      case 'move':
        if (!source) return;
        this.dialog.set({
          state: 'form',
          operation: intent.operation,
          title:
            intent.operation === 'copy'
              ? $localize`:Copy Files entry dialog title@@files.dialog.copy:Copy`
              : $localize`:Move Files entry dialog title@@files.dialog.move:Move`,
          message: $localize`:Transfer destination dialog message@@files.dialog.destinationMessage:Choose a registered destination and provider-relative path.`,
          source,
          destination: {
            mountId: source.mountId,
            path: source.path,
          },
        });
        return;
      case 'trash':
        if (!source) return;
        this.dialog.set({
          state: 'confirm',
          operation: intent.operation,
          title: $localize`:Move to Trash dialog title@@files.dialog.trash:Move to Trash`,
          message: $localize`:Move to Trash confirmation@@files.dialog.trashConfirm:Move this item to Trash?`,
          source,
          recursive: intent.recursive ?? false,
        });
        return;
      case 'restore':
        if (!source || !intent.receiptId) return;
        this.dialog.set({
          state: 'confirm',
          operation: intent.operation,
          title: $localize`:Restore Files entry dialog title@@files.dialog.restore:Restore`,
          message: $localize`:Restore Files entry confirmation@@files.dialog.restoreConfirm:Restore this item to its original location?`,
          source,
          receiptId: intent.receiptId,
        });
        return;
      case 'delete':
        if (!source) return;
        this.dialog.set({
          state: 'confirm',
          operation: intent.operation,
          title: $localize`:Permanent Files delete dialog title@@files.dialog.delete:Permanently delete`,
          message:
            intent.recursive === true
              ? $localize`:Recursive Files delete first confirmation@@files.dialog.deleteRecursiveFirst:This folder and its contents will be permanently deleted. Continue?`
              : $localize`:Permanent Files delete confirmation@@files.dialog.deleteConfirm:This item will be permanently deleted. Continue?`,
          source,
          ...(intent.receiptId ? { receiptId: intent.receiptId } : {}),
          recursive: intent.recursive ?? false,
          requiresRecursiveConfirmation: intent.recursive === true,
        });
        return;
    }
  }

  private async performOperation(
    intent: Extract<FilesActionIntent, { type: 'submit-operation' }>,
  ): Promise<void> {
    const currentDialog = this.dialog();
    if (
      intent.operation === 'delete' &&
      intent.recursive === true &&
      intent.confirmed === true &&
      currentDialog?.requiresRecursiveConfirmation
    ) {
      this.dialog.set({
        ...currentDialog,
        message: $localize`:Recursive Files delete second confirmation@@files.dialog.deleteRecursiveSecond:Confirm permanent recursive deletion. This cannot be undone.`,
        requiresRecursiveConfirmation: false,
      });
      return;
    }

    const title = operationTitle(intent.operation);
    this.dialog.set({
      state: 'busy',
      operation: intent.operation,
      title,
      message: $localize`:Files operation busy message@@files.dialog.working:Working…`,
      ...(intent.source ? { source: intent.source } : {}),
      ...(intent.destination ? { destination: intent.destination } : {}),
      ...(intent.receiptId ? { receiptId: intent.receiptId } : {}),
      recursive: intent.recursive ?? false,
    });

    try {
      const result = await this.callOperation(intent);
      this.showOperationResult(intent.operation, result);
      if (result.status === 'completed' || result.status === 'partial') {
        this.scheduleRefresh();
      }
    } catch (error) {
      this.dialog.set({
        state: 'error',
        operation: intent.operation,
        title,
        message: errorMessage(error),
        ...(intent.source ? { source: intent.source } : {}),
        ...(intent.destination ? { destination: intent.destination } : {}),
        ...(intent.receiptId ? { receiptId: intent.receiptId } : {}),
        recursive: intent.recursive ?? false,
      });
    }
  }

  private async callOperation(
    intent: Extract<FilesActionIntent, { type: 'submit-operation' }>,
  ): Promise<FileOperationResultView> {
    const source = this.source();
    switch (intent.operation) {
      case 'create-directory': {
        const address = requiredAddress(intent.source);
        return source.createDirectory({
          mountId: address.mountId,
          parentPath: address.path,
          name: requiredName(intent.name),
        });
      }
      case 'rename': {
        const address = requiredAddress(intent.source);
        return source.rename({
          mountId: address.mountId,
          path: address.path,
          name: requiredName(intent.name),
        });
      }
      case 'copy':
      case 'move': {
        const sourceAddress = requiredAddress(intent.source);
        const destination = requiredAddress(intent.destination);
        if (!destination.path) {
          throw new Error(
            $localize`:Files transfer destination required@@files.error.destinationRequired:A provider-relative destination path is required.`,
          );
        }
        return intent.operation === 'copy'
          ? source.copy({ source: sourceAddress, destination })
          : source.move({ source: sourceAddress, destination });
      }
      case 'trash': {
        if (intent.confirmed !== true) {
          throw new Error(
            $localize`:Files Trash confirmation required@@files.error.trashConfirmation:Moving an item to Trash requires confirmation.`,
          );
        }
        const address = requiredAddress(intent.source);
        return source.trash({
          mountId: address.mountId,
          path: address.path,
        });
      }
      case 'restore':
        if (intent.confirmed !== true || !intent.receiptId) {
          throw new Error(
            $localize`:Files restore confirmation required@@files.error.restoreConfirmation:Restoring an item requires its receipt and confirmation.`,
          );
        }
        return source.restore({ receiptId: intent.receiptId });
      case 'delete': {
        if (intent.confirmed !== true) {
          throw new Error(
            $localize`:Files delete confirmation required@@files.error.deleteConfirmation:Permanent deletion requires confirmation.`,
          );
        }
        const address = intent.source;
        return source.delete({
          mountId: intent.receiptId ? '' : requiredAddress(address).mountId,
          path: intent.receiptId ? '' : requiredAddress(address).path,
          receiptId: intent.receiptId ?? '',
          recursive: intent.recursive ?? false,
          confirmed: true,
        });
      }
    }
  }

  private showOperationResult(
    operation: FilesOperationKind,
    result: FileOperationResultView,
  ): void {
    const state =
      result.status === 'conflict'
        ? 'conflict'
        : result.status === 'partial'
          ? 'partial'
          : 'success';
    this.dialog.set({
      state,
      operation,
      title: operationTitle(operation),
      message: result.message,
      source: result.source,
      ...(result.destination ? { destination: result.destination } : {}),
      ...(result.receiptId ? { receiptId: result.receiptId } : {}),
      result,
    });
  }

  private scheduleRefresh(): void {
    if (this.refreshScheduled || this.destroyed) return;
    this.refreshScheduled = true;
    this.refreshTaskDone = this.pendingTasks.add();
    this.refreshTimer = setTimeout(() => {
      this.refreshTimer = null;
      this.refreshScheduled = false;
      if (this.destroyed) {
        this.finishScheduledRefresh();
        return;
      }
      void this.refresh().finally(() => this.finishScheduledRefresh());
    }, 0);
  }

  private finishScheduledRefresh(): void {
    this.refreshTaskDone?.();
    this.refreshTaskDone = null;
  }

  private subscribeToEvents(): void {
    if (this.connection.offline() || this.filesEventsOff) return;
    this.filesEventsOff = this.bridge.onChanged((event) => {
      if (!this.knownEvent(event)) return;
      if (eventAffectsLocation(event, this.location())) {
        this.scheduleRefresh();
      }
    });
  }

  private knownEvent(event: FilesChangedEvent): boolean {
    const knownMounts = new Set(this.catalogue().mounts.map(({ id }) => id));
    return event.mountIds.length > 0 && event.mountIds.every((mountId) => knownMounts.has(mountId));
  }

  private markSuccessful(): void {
    this.hasSuccessfulView = true;
    this.failure.set('');
    this.dataState.set(this.connection.offline() ? 'demo' : 'live');
    this.subscribeToEvents();
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
            const entryTokens = state.entries.flatMap((entry) => {
              const token = entryToken(state, entry);
              return token ? [token] : [];
            });
            const observed = [
              state.token,
              ...state.breadcrumbs.map(({ token }) => token),
              ...entryTokens,
            ];
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
                      token: entryToken(state, entry),
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

function entryToken(state: FilesViewState, entry: FilesBrowserEntryView): string {
  if (entry.kind !== 'directory' || state.location.kind === 'trash') return '';
  return filesToken({
    kind: 'directory',
    mountId: entry.mountId,
    path: entry.relativePath,
  });
}

function requiredAddress(address: FileAddressView | undefined): FileAddressView {
  if (!address || !address.mountId || !validRelativePath(address.path)) {
    throw new Error(
      $localize`:Invalid Files address@@files.error.invalidAddress:The Files address is invalid.`,
    );
  }
  return address;
}

function requiredName(name: string | undefined): string {
  if (
    !name ||
    name.trim() !== name ||
    name === '.' ||
    name === '..' ||
    name.includes('/') ||
    name.includes('\\') ||
    /[\u0000-\u001f\u007f]/.test(name)
  ) {
    throw new Error(
      $localize`:Invalid Files name@@files.error.invalidName:Enter one valid name without path separators.`,
    );
  }
  return name;
}

function basename(path: string): string {
  return path.slice(path.lastIndexOf('/') + 1);
}

function operationTitle(operation: FilesOperationKind): string {
  switch (operation) {
    case 'create-directory':
      return $localize`:Create folder operation title@@files.operation.createFolder:Create folder`;
    case 'rename':
      return $localize`:Rename operation title@@files.operation.rename:Rename`;
    case 'copy':
      return $localize`:Copy operation title@@files.operation.copy:Copy`;
    case 'move':
      return $localize`:Move operation title@@files.operation.move:Move`;
    case 'trash':
      return $localize`:Trash operation title@@files.operation.trash:Move to Trash`;
    case 'restore':
      return $localize`:Restore operation title@@files.operation.restore:Restore`;
    case 'delete':
      return $localize`:Delete operation title@@files.operation.delete:Permanently delete`;
  }
}

function errorMessage(error: unknown): string {
  return error instanceof Error
    ? error.message
    : $localize`:Unknown Files provider error@@files.error.unknown:The Files provider is unavailable.`;
}

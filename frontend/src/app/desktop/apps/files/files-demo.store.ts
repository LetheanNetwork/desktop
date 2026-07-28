import {
  DEMO_FILES_DIRECTORIES,
  DEMO_FILES_HOME_ENTRIES,
  DEMO_FILES_MOUNTS,
  DEMO_FILES_PREVIEWS,
  DEMO_FILES_RECENT,
} from './files-demo.data';
import type {
  CreateDirectoryInput,
  DeleteInput,
  DirectorySnapshotView,
  FileAddressView,
  FileEntryView,
  FileMountView,
  FileOperationResultView,
  FilePreviewView,
  FilesCatalogueView,
  FilesDataSource,
  FilesHomeSnapshotView,
  ListDirectoryInput,
  PreviewInput,
  RenameInput,
  RestoreInput,
  TrashEntryView,
  TrashInput,
  TrashSnapshotView,
  TransferInput,
} from './files-view.models';
import { joinRelativePath, parentPath, validMountId, validRelativePath } from './files-view-state';

interface DemoTrashRecord {
  readonly row: TrashEntryView;
  readonly entry: FileEntryView;
  readonly directories: ReadonlyMap<string, readonly FileEntryView[]>;
  readonly previews: ReadonlyMap<string, FilePreviewView>;
}

const MAX_LIST_ENTRIES = 2_000;
const BINARY_EXTENSIONS = new Set([
  'dmg',
  'gguf',
  'gif',
  'gz',
  'jpeg',
  'jpg',
  'pdf',
  'png',
  'safetensors',
  'tar',
  'tgz',
  'webp',
  'zip',
]);

export class FilesDemoStore implements FilesDataSource {
  private readonly mounts = structuredClone(DEMO_FILES_MOUNTS) as FileMountView[];
  private readonly homeEntries = structuredClone(DEMO_FILES_HOME_ENTRIES);
  private readonly recent = structuredClone(DEMO_FILES_RECENT);
  private readonly directories = new Map<string, FileEntryView[]>(
    DEMO_FILES_DIRECTORIES.map(({ mountId, path, entries }) => [
      directoryKey(mountId, path),
      structuredClone(entries) as FileEntryView[],
    ]),
  );
  private readonly previews = new Map<string, FilePreviewView>(
    DEMO_FILES_PREVIEWS.map((preview) => [
      addressKey(preview.mountId, preview.relativePath),
      structuredClone(preview),
    ]),
  );
  private readonly trashRecords = new Map<string, DemoTrashRecord>();
  private operationSequence = 0;
  private receiptSequence = 0;

  async listMounts(): Promise<FilesCatalogueView> {
    return {
      mounts: structuredClone(this.mounts),
      favourites: [],
      recent: structuredClone(this.recent),
    };
  }

  async listHome(): Promise<FilesHomeSnapshotView> {
    return {
      entries: this.homeEntries.map(({ entry }) => structuredClone(entry)),
    };
  }

  async listDirectory(input: ListDirectoryInput): Promise<DirectorySnapshotView> {
    const mount = this.requireMount(input.mountId, 'list');
    const path = requireRelativePath(input.path);
    if (!Number.isInteger(input.limit) || input.limit < 0) {
      throw new Error('The demo Files list limit is invalid.');
    }
    const entries = this.directories.get(directoryKey(mount.id, path));
    if (!entries) {
      throw new Error('The requested demo Files directory is unavailable.');
    }
    const limit = input.limit === 0 ? 200 : Math.min(input.limit, MAX_LIST_ENTRIES);
    const page = entries.slice(0, limit);
    return {
      mount: structuredClone(mount),
      path,
      breadcrumbs: breadcrumbRows(path),
      entries: structuredClone(page),
      nextCursor: page.length < entries.length ? String(page.length) : '',
      totalKnown: entries.length,
      refreshedAt: demoTimestamp(),
    };
  }

  async preview(input: PreviewInput): Promise<FilePreviewView> {
    const mount = this.requireMount(input.mountId, 'preview');
    const path = requireNonEmptyPath(input.path);
    const seeded = this.previews.get(addressKey(mount.id, path));
    if (seeded) return structuredClone(seeded);

    const homeEntry = this.homeEntries.find(
      (candidate) => candidate.mountId === mount.id && candidate.entry.relativePath === path,
    )?.entry;
    const entry = homeEntry ?? this.findEntry(mount.id, path);
    if (!entry || entry.kind !== 'file') {
      throw new Error('The requested demo Files preview is unavailable.');
    }
    const binary = BINARY_EXTENSIONS.has(extension(entry.name));
    return {
      mountId: mount.id,
      relativePath: path,
      name: entry.name,
      content: binary ? '' : `${entry.name}\n\nDeterministic demo preview.`,
      mime: binary ? 'application/octet-stream' : 'text/plain',
      bytesRead: binary ? 0 : Math.min(entry.sizeBytes, 64),
      sizeBytes: entry.sizeBytes,
      lines: binary ? 0 : 3,
      truncated: false,
      binary,
    };
  }

  async createDirectory(input: CreateDirectoryInput): Promise<FileOperationResultView> {
    const operationId = this.nextOperationId();
    const mount = this.requireMount(input.mountId, 'createDirectory');
    const parent = requireRelativePath(input.parentPath);
    const name = requireName(input.name);
    const entries = this.requireDirectory(mount.id, parent);
    const path = joinRelativePath(parent, name);
    const source = address(mount.id, parent);
    const destination = address(mount.id, path);
    if (this.pathExists(mount.id, path)) {
      return conflictResult(
        operationId,
        'create-directory',
        source,
        destination,
        this.findEntry(mount.id, path)?.kind ?? 'directory',
      );
    }

    entries.push(directoryEntry(name, path));
    this.directories.set(directoryKey(mount.id, path), []);
    return completedResult(
      operationId,
      'create-directory',
      source,
      destination,
      [destination],
      'Folder created.',
    );
  }

  async rename(input: RenameInput): Promise<FileOperationResultView> {
    const operationId = this.nextOperationId();
    const mount = this.requireMount(input.mountId, 'rename');
    const sourcePath = requireNonEmptyPath(input.path);
    const name = requireName(input.name);
    const destinationPath = joinRelativePath(parentPath(sourcePath), name);
    return this.transfer(
      operationId,
      'rename',
      { mountId: mount.id, path: sourcePath },
      { mountId: mount.id, path: destinationPath },
      true,
    );
  }

  async copy(input: TransferInput): Promise<FileOperationResultView> {
    const operationId = this.nextOperationId();
    this.requireMount(input.source.mountId, 'copyFrom');
    this.requireMount(input.destination.mountId, 'copyTo');
    return this.transfer(
      operationId,
      'copy',
      validateAddress(input.source),
      validateAddress(input.destination),
      false,
    );
  }

  async move(input: TransferInput): Promise<FileOperationResultView> {
    const operationId = this.nextOperationId();
    this.requireMount(input.source.mountId, 'move');
    this.requireMount(input.destination.mountId, 'copyTo');
    return this.transfer(
      operationId,
      'move',
      validateAddress(input.source),
      validateAddress(input.destination),
      true,
    );
  }

  async trash(input: TrashInput): Promise<FileOperationResultView> {
    const operationId = this.nextOperationId();
    const mount = this.requireMount(input.mountId, 'trash');
    const path = requireNonEmptyPath(input.path);
    const entry = this.findEntry(mount.id, path);
    if (!entry) throw new Error('The requested demo Files entry is unavailable.');

    const receiptId = `demo-trash-${++this.receiptSequence}`;
    const directories = this.captureDirectories(mount.id, path);
    const previews = this.capturePreviews(mount.id, path);
    this.removeEntry(mount.id, path);
    const row: TrashEntryView = {
      receiptId,
      mountId: mount.id,
      originalPath: path,
      name: entry.name,
      kind: entry.kind,
      sizeBytes: entry.sizeBytes,
      trashedAt: demoTimestamp(),
      available: true,
      errorCode: '',
    };
    this.trashRecords.set(receiptId, {
      row,
      entry: structuredClone(entry),
      directories,
      previews,
    });
    return {
      ...completedResult(
        operationId,
        'trash',
        address(mount.id, path),
        undefined,
        [address(mount.id, path)],
        'Item moved to Trash.',
      ),
      receiptId,
    };
  }

  async listTrash(): Promise<TrashSnapshotView> {
    return {
      entries: [...this.trashRecords.values()].map(({ row }) => structuredClone(row)),
      refreshedAt: demoTimestamp(),
    };
  }

  async restore(input: RestoreInput): Promise<FileOperationResultView> {
    const operationId = this.nextOperationId();
    const record = this.trashRecords.get(input.receiptId);
    if (!record) throw new Error('The demo Files trash receipt is unavailable.');
    const mount = this.requireMount(record.row.mountId, 'restore');
    const destination = address(mount.id, record.row.originalPath);
    const source = address(mount.id, record.row.originalPath);
    if (this.pathExists(mount.id, record.row.originalPath)) {
      return conflictResult(operationId, 'restore', source, destination, record.entry.kind);
    }
    const parent = this.requireDirectory(mount.id, parentPath(record.row.originalPath));
    parent.push(structuredClone(record.entry));
    for (const [path, entries] of record.directories) {
      this.directories.set(
        directoryKey(mount.id, path),
        structuredClone(entries) as FileEntryView[],
      );
    }
    for (const [path, preview] of record.previews) {
      this.previews.set(addressKey(mount.id, path), structuredClone(preview));
    }
    this.trashRecords.delete(input.receiptId);
    return {
      ...completedResult(
        operationId,
        'restore',
        source,
        destination,
        [destination],
        'Item restored.',
      ),
      receiptId: input.receiptId,
    };
  }

  async delete(input: DeleteInput): Promise<FileOperationResultView> {
    const operationId = this.nextOperationId();
    if (!input.confirmed) {
      throw new Error('Permanent deletion requires explicit confirmation.');
    }
    if (input.receiptId) {
      const record = this.trashRecords.get(input.receiptId);
      if (!record) throw new Error('The demo Files trash receipt is unavailable.');
      this.requireMount(record.row.mountId, 'delete');
      this.trashRecords.delete(input.receiptId);
      return {
        ...completedResult(
          operationId,
          'delete',
          address(record.row.mountId, record.row.originalPath),
          undefined,
          [address(record.row.mountId, record.row.originalPath)],
          'Item permanently deleted.',
        ),
        receiptId: input.receiptId,
      };
    }

    const mount = this.requireMount(input.mountId, 'delete');
    const path = requireNonEmptyPath(input.path);
    const entry = this.findEntry(mount.id, path);
    if (!entry) throw new Error('The requested demo Files entry is unavailable.');
    if (entry.kind === 'directory' && !input.recursive) {
      throw new Error('Deleting a folder requires recursive confirmation.');
    }
    this.removeEntry(mount.id, path);
    const source = address(mount.id, path);
    return completedResult(
      operationId,
      'delete',
      source,
      undefined,
      [source],
      'Item permanently deleted.',
    );
  }

  private transfer(
    operationId: string,
    operation: string,
    source: FileAddressView,
    destination: FileAddressView,
    removeSource: boolean,
  ): FileOperationResultView {
    const entry = this.findEntry(source.mountId, source.path);
    if (!entry) throw new Error('The requested demo Files entry is unavailable.');
    if (this.pathExists(destination.mountId, destination.path)) {
      return conflictResult(operationId, operation, source, destination, entry.kind);
    }
    const destinationParent = this.requireDirectory(
      destination.mountId,
      parentPath(destination.path),
    );
    const copiedEntry: FileEntryView = {
      ...structuredClone(entry),
      name: basename(destination.path),
      relativePath: destination.path,
    };
    destinationParent.push(copiedEntry);
    this.copyDescendants(source, destination);
    if (removeSource) this.removeEntry(source.mountId, source.path);
    return completedResult(
      operationId,
      operation,
      source,
      destination,
      [source, destination],
      operation === 'copy'
        ? 'Item copied.'
        : operation === 'rename'
          ? 'Item renamed.'
          : 'Item moved.',
    );
  }

  private copyDescendants(source: FileAddressView, destination: FileAddressView): void {
    const capturedDirectories = this.captureDirectories(source.mountId, source.path);
    for (const [sourcePath, entries] of capturedDirectories) {
      const destinationPath = replacePrefix(sourcePath, source.path, destination.path);
      this.directories.set(
        directoryKey(destination.mountId, destinationPath),
        entries.map((entry) => ({
          ...structuredClone(entry),
          relativePath: replacePrefix(entry.relativePath, source.path, destination.path),
        })),
      );
    }
    const capturedPreviews = this.capturePreviews(source.mountId, source.path);
    for (const [sourcePath, preview] of capturedPreviews) {
      const destinationPath = replacePrefix(sourcePath, source.path, destination.path);
      this.previews.set(addressKey(destination.mountId, destinationPath), {
        ...structuredClone(preview),
        mountId: destination.mountId,
        relativePath: destinationPath,
        name: basename(destinationPath),
      });
    }
  }

  private removeEntry(mountId: string, path: string): void {
    const parent = this.requireDirectory(mountId, parentPath(path));
    const index = parent.findIndex(({ relativePath }) => relativePath === path);
    if (index < 0) throw new Error('The requested demo Files entry is unavailable.');
    parent.splice(index, 1);
    for (const key of [...this.directories.keys()]) {
      const [keyMountId, keyPath] = splitKey(key);
      if (keyMountId === mountId && (keyPath === path || keyPath.startsWith(`${path}/`))) {
        this.directories.delete(key);
      }
    }
    for (const key of [...this.previews.keys()]) {
      const [keyMountId, keyPath] = splitKey(key);
      if (keyMountId === mountId && (keyPath === path || keyPath.startsWith(`${path}/`))) {
        this.previews.delete(key);
      }
    }
  }

  private captureDirectories(
    mountId: string,
    path: string,
  ): ReadonlyMap<string, readonly FileEntryView[]> {
    const captured = new Map<string, readonly FileEntryView[]>();
    for (const [key, entries] of this.directories) {
      const [keyMountId, keyPath] = splitKey(key);
      if (keyMountId === mountId && (keyPath === path || keyPath.startsWith(`${path}/`))) {
        captured.set(keyPath, structuredClone(entries));
      }
    }
    return captured;
  }

  private capturePreviews(mountId: string, path: string): ReadonlyMap<string, FilePreviewView> {
    const captured = new Map<string, FilePreviewView>();
    for (const [key, preview] of this.previews) {
      const [keyMountId, keyPath] = splitKey(key);
      if (keyMountId === mountId && (keyPath === path || keyPath.startsWith(`${path}/`))) {
        captured.set(keyPath, structuredClone(preview));
      }
    }
    return captured;
  }

  private findEntry(mountId: string, path: string): FileEntryView | undefined {
    return this.directories
      .get(directoryKey(mountId, parentPath(path)))
      ?.find(({ relativePath }) => relativePath === path);
  }

  private pathExists(mountId: string, path: string): boolean {
    return (
      this.findEntry(mountId, path) !== undefined ||
      this.directories.has(directoryKey(mountId, path))
    );
  }

  private requireDirectory(mountId: string, path: string): FileEntryView[] {
    const directory = this.directories.get(directoryKey(mountId, path));
    if (!directory) {
      throw new Error('The requested demo Files directory is unavailable.');
    }
    return directory;
  }

  private requireMount(
    mountId: string,
    capability: keyof FileMountView['capabilities'],
  ): FileMountView {
    if (!validMountId(mountId)) {
      throw new Error('The demo Files mount ID is invalid.');
    }
    const mount = this.mounts.find(({ id }) => id === mountId);
    if (!mount) throw new Error('The requested demo Files mount is unavailable.');
    if (!mount.capabilities[capability]) {
      throw new Error('The requested demo Files operation is not permitted.');
    }
    return mount;
  }

  private nextOperationId(): string {
    return `demo-files-${++this.operationSequence}`;
  }
}

function completedResult(
  operationId: string,
  operation: string,
  source: FileAddressView,
  destination: FileAddressView | undefined,
  affected: readonly FileAddressView[],
  message: string,
): FileOperationResultView {
  return {
    operationId,
    operation,
    status: 'completed',
    code: '',
    source,
    ...(destination ? { destination } : {}),
    affected,
    message,
    receiptId: '',
  };
}

function conflictResult(
  operationId: string,
  operation: string,
  source: FileAddressView,
  destination: FileAddressView,
  kind: FileEntryView['kind'],
): FileOperationResultView {
  return {
    operationId,
    operation,
    status: 'conflict',
    code: 'files.conflict',
    source,
    destination,
    affected: [],
    conflict: { source, destination, kind },
    message: 'The destination already exists.',
    receiptId: '',
  };
}

function validateAddress(value: FileAddressView): FileAddressView {
  if (!validMountId(value.mountId)) {
    throw new Error('The demo Files mount ID is invalid.');
  }
  return { mountId: value.mountId, path: requireNonEmptyPath(value.path) };
}

function requireRelativePath(value: string): string {
  if (!validRelativePath(value)) {
    throw new Error('Files paths must remain provider-relative.');
  }
  return value;
}

function requireNonEmptyPath(value: string): string {
  const path = requireRelativePath(value);
  if (!path) throw new Error('The demo Files path is required.');
  return path;
}

function requireName(value: string): string {
  const name = value.trim();
  if (
    !name ||
    name === '.' ||
    name === '..' ||
    name.includes('/') ||
    name.includes('\\') ||
    /[\u0000-\u001f\u007f]/.test(name)
  ) {
    throw new Error('The demo Files name is invalid.');
  }
  return name;
}

function address(mountId: string, path: string): FileAddressView {
  return { mountId, path };
}

function directoryEntry(name: string, relativePath: string): FileEntryView {
  return {
    name,
    relativePath,
    kind: 'directory',
    sizeBytes: 0,
    modifiedAt: demoTimestamp(),
    mode: 0o755,
    hidden: false,
  };
}

function breadcrumbRows(path: string) {
  if (!path) return [];
  const parts = path.split('/');
  return parts.map((name, index) => ({
    name,
    path: parts.slice(0, index + 1).join('/'),
  }));
}

function directoryKey(mountId: string, path: string): string {
  return addressKey(mountId, path);
}

function addressKey(mountId: string, path: string): string {
  return `${mountId}\u0000${path}`;
}

function splitKey(key: string): readonly [string, string] {
  const separator = key.indexOf('\u0000');
  return [key.slice(0, separator), key.slice(separator + 1)];
}

function basename(path: string): string {
  return path.slice(path.lastIndexOf('/') + 1);
}

function replacePrefix(path: string, source: string, destination: string): string {
  return `${destination}${path.slice(source.length)}`;
}

function extension(name: string): string {
  return name.split('.').at(-1)?.toLocaleLowerCase('en-GB') ?? '';
}

function demoTimestamp(): string {
  return '2026-07-26T12:00:00Z';
}

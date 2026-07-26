import { InjectionToken, Service, inject } from '@angular/core';
import { Events } from '@wailsio/runtime';
import { ConnectionManagerService } from '../connection-manager.service';
import type {
  CreateDirectoryInput,
  DeleteInput,
  DirectorySnapshotView,
  FileAddressView,
  FileEntryKind,
  FileEntryView,
  FileMountView,
  FileOperationResultView,
  FileOperationStatus,
  FilePreviewView,
  FilesCapabilities,
  FilesCatalogueView,
  FilesChangedEvent,
  FilesDataSource,
  FilesErrorCode,
  ListDirectoryInput,
  PreviewInput,
  RenameInput,
  RestoreInput,
  TrashInput,
  TrashSnapshotView,
  TransferInput,
} from './apps/files/files-view.models';
import { validMountId, validRelativePath } from './apps/files/files-view-state';
import { SurfaceBridgeService } from './surfaces/surface-bridge.service';

const FILES_SERVICE = 'dappco.re/lthn/desktop/pkg/office/files.Service';
const FILES_CHANGED_EVENT = 'lthn:files:changed';

export const FILES_METHODS = {
  listMounts: `${FILES_SERVICE}.ListMounts`,
  listDirectory: `${FILES_SERVICE}.ListDirectory`,
  preview: `${FILES_SERVICE}.Preview`,
  createDirectory: `${FILES_SERVICE}.CreateDirectory`,
  rename: `${FILES_SERVICE}.Rename`,
  copy: `${FILES_SERVICE}.Copy`,
  move: `${FILES_SERVICE}.Move`,
  trash: `${FILES_SERVICE}.Trash`,
  listTrash: `${FILES_SERVICE}.ListTrash`,
  restore: `${FILES_SERVICE}.Restore`,
  delete: `${FILES_SERVICE}.Delete`,
} as const;

export interface FilesEventSource {
  on(name: string, handler: (payload: unknown) => void): () => void;
}

export const FILES_EVENT_SOURCE = new InjectionToken<FilesEventSource>('FILES_EVENT_SOURCE', {
  providedIn: 'root',
  factory: () => ({
    on(name, handler): () => void {
      return Events.On(name, (event) => handler(event.data));
    },
  }),
});

@Service()
export class DesktopFilesBridgeService implements FilesDataSource {
  private readonly surface = inject(SurfaceBridgeService);
  private readonly connection = inject(ConnectionManagerService);
  private readonly events = inject(FILES_EVENT_SOURCE);

  async listMounts(): Promise<FilesCatalogueView> {
    return this.read(FILES_METHODS.listMounts, undefined, parseCatalogue);
  }

  async listDirectory(input: ListDirectoryInput): Promise<DirectorySnapshotView> {
    const request = listDirectoryRequest(input);
    return this.read(FILES_METHODS.listDirectory, [request], parseDirectorySnapshot);
  }

  async preview(input: PreviewInput): Promise<FilePreviewView> {
    const request = previewRequest(input);
    return this.read(FILES_METHODS.preview, [request], parsePreview);
  }

  async createDirectory(input: CreateDirectoryInput): Promise<FileOperationResultView> {
    const request = createDirectoryRequest(input);
    return this.read(FILES_METHODS.createDirectory, [request], parseOperationResult);
  }

  async rename(input: RenameInput): Promise<FileOperationResultView> {
    const request = renameRequest(input);
    return this.read(FILES_METHODS.rename, [request], parseOperationResult);
  }

  async copy(input: TransferInput): Promise<FileOperationResultView> {
    const request = transferRequest(input);
    return this.read(FILES_METHODS.copy, [request], parseOperationResult);
  }

  async move(input: TransferInput): Promise<FileOperationResultView> {
    const request = transferRequest(input);
    return this.read(FILES_METHODS.move, [request], parseOperationResult);
  }

  async trash(input: TrashInput): Promise<FileOperationResultView> {
    const request = trashRequest(input);
    return this.read(FILES_METHODS.trash, [request], parseOperationResult);
  }

  async listTrash(): Promise<TrashSnapshotView> {
    return this.read(FILES_METHODS.listTrash, undefined, parseTrashSnapshot);
  }

  async restore(input: RestoreInput): Promise<FileOperationResultView> {
    const request = restoreRequest(input);
    return this.read(FILES_METHODS.restore, [request], parseOperationResult);
  }

  async delete(input: DeleteInput): Promise<FileOperationResultView> {
    const request = deleteRequest(input);
    return this.read(FILES_METHODS.delete, [request], parseOperationResult);
  }

  onChanged(handler: (event: FilesChangedEvent) => void): () => void {
    if (this.connection.offline()) return () => undefined;
    return this.events.on(FILES_CHANGED_EVENT, (raw) => {
      try {
        rejectProviderFields(raw, 'Files event');
        handler(parseChangedEvent(raw));
      } catch {
        // Invalidations are advisory. A malformed event cannot enter view state.
      }
    });
  }

  private async read<T>(
    method: string,
    args: readonly unknown[] | undefined,
    parser: (raw: unknown) => T,
  ): Promise<T> {
    this.requireOnline();
    const raw = args ? await this.surface.call(method, args) : await this.surface.call(method);
    rejectProviderFields(raw, 'Files response');
    return parser(raw);
  }

  private requireOnline(): void {
    if (this.connection.offline()) {
      throw new Error('The Files live bridge is unavailable in offline demo mode.');
    }
  }
}

const ENTRY_KINDS: readonly FileEntryKind[] = ['file', 'directory', 'link', 'other'];
const OPERATION_STATUSES: readonly FileOperationStatus[] = ['completed', 'conflict', 'partial'];
const ERROR_CODES: readonly FilesErrorCode[] = [
  '',
  'files.invalid_input',
  'files.invalid_mount',
  'files.boundary_rejected',
  'files.capability_denied',
  'files.missing_entry',
  'files.conflict',
  'files.provider_unavailable',
  'files.limit_exceeded',
  'files.unsupported_entry',
  'files.partial_move',
];
const FORBIDDEN_RESPONSE_FIELDS = new Set([
  'root',
  'absolutepath',
  'endpoint',
  'credential',
  'secret',
  'key',
  'encryptionkey',
]);

function parseCatalogue(raw: unknown): FilesCatalogueView {
  const record = requiredRecord(raw, 'mount catalogue');
  return {
    mounts: requiredArray(record['mounts'], 'mount catalogue mounts').map(parseMount),
    favourites: requiredArray(record['favourites'], 'mount catalogue favourites').map((value) => {
      const favourite = requiredRecord(value, 'mount favourite');
      return {
        mountId: providerMountId(favourite['mountId'], 'mount favourite'),
        path: providerRelativePath(favourite['path'], 'mount favourite path'),
      };
    }),
    recent: requiredArray(record['recent'], 'mount catalogue recent').map((value) => {
      const recent = requiredRecord(value, 'recent file');
      return {
        mountId: providerMountId(recent['mountId'], 'recent file'),
        path: providerRelativePath(recent['path'], 'recent file path', false),
        name: fileName(recent['name'], 'recent file name'),
        kind: requiredEnum(recent['kind'], ENTRY_KINDS, 'recent file kind'),
        openedAt: requiredString(recent['openedAt'], 'recent file openedAt'),
      };
    }),
  };
}

function parseMount(raw: unknown): FileMountView {
  const record = requiredRecord(raw, 'file mount');
  const capacityRaw = record['capacity'];
  const capacity =
    capacityRaw === undefined || capacityRaw === null ? undefined : parseCapacity(capacityRaw);
  return {
    id: providerMountId(record['id'], 'file mount'),
    name: requiredString(record['name'], 'file mount name'),
    kind: requiredString(record['kind'], 'file mount kind'),
    icon: requiredStringAllowEmpty(record['icon'], 'file mount icon'),
    brand:
      record['brand'] === undefined ? false : requiredBoolean(record['brand'], 'file mount brand'),
    capabilities: parseCapabilities(record['capabilities']),
    ...(capacity ? { capacity } : {}),
  };
}

function parseCapabilities(raw: unknown): FilesCapabilities {
  const record = requiredRecord(raw, 'file mount capabilities');
  return {
    list: requiredBoolean(record['list'], 'list capability'),
    preview: requiredBoolean(record['preview'], 'preview capability'),
    createDirectory: requiredBoolean(record['createDirectory'], 'createDirectory capability'),
    write: requiredBoolean(record['write'], 'write capability'),
    rename: requiredBoolean(record['rename'], 'rename capability'),
    copyFrom: requiredBoolean(record['copyFrom'], 'copyFrom capability'),
    copyTo: requiredBoolean(record['copyTo'], 'copyTo capability'),
    move: requiredBoolean(record['move'], 'move capability'),
    trash: requiredBoolean(record['trash'], 'trash capability'),
    restore: requiredBoolean(record['restore'], 'restore capability'),
    delete: requiredBoolean(record['delete'], 'delete capability'),
  };
}

function parseCapacity(raw: unknown) {
  const record = requiredRecord(raw, 'file mount capacity');
  const freeBytes = requiredNonNegativeNumber(record['freeBytes'], 'file mount freeBytes');
  const totalBytes = requiredNonNegativeNumber(record['totalBytes'], 'file mount totalBytes');
  if (freeBytes > totalBytes) invalidResponse('file mount capacity');
  return { freeBytes, totalBytes };
}

function parseDirectorySnapshot(raw: unknown): DirectorySnapshotView {
  const record = requiredRecord(raw, 'directory snapshot');
  return {
    mount: parseMount(record['mount']),
    path: providerRelativePath(record['path'], 'directory path'),
    breadcrumbs: requiredArray(record['breadcrumbs'], 'directory breadcrumbs').map((value) => {
      const breadcrumb = requiredRecord(value, 'directory breadcrumb');
      return {
        name: fileName(breadcrumb['name'], 'directory breadcrumb name'),
        path: providerRelativePath(breadcrumb['path'], 'directory breadcrumb path', false),
      };
    }),
    entries: requiredArray(record['entries'], 'directory entries').map(parseEntry),
    nextCursor:
      record['nextCursor'] === undefined
        ? ''
        : requiredStringAllowEmpty(record['nextCursor'], 'directory nextCursor'),
    totalKnown: requiredNonNegativeInteger(record['totalKnown'], 'directory totalKnown'),
    refreshedAt: requiredString(record['refreshedAt'], 'directory refreshedAt'),
  };
}

function parseEntry(raw: unknown): FileEntryView {
  const record = requiredRecord(raw, 'directory entry');
  return {
    name: fileName(record['name'], 'directory entry name'),
    relativePath: providerRelativePath(record['relativePath'], 'directory entry path', false),
    kind: requiredEnum(record['kind'], ENTRY_KINDS, 'directory entry kind'),
    sizeBytes: requiredNonNegativeNumber(record['sizeBytes'], 'directory entry sizeBytes'),
    modifiedAt: requiredStringAllowEmpty(record['modifiedAt'], 'directory entry modifiedAt'),
    mode: requiredNonNegativeInteger(record['mode'], 'directory entry mode'),
    hidden: requiredBoolean(record['hidden'], 'directory entry hidden'),
  };
}

function parsePreview(raw: unknown): FilePreviewView {
  const record = requiredRecord(raw, 'file preview');
  return {
    mountId: providerMountId(record['mountId'], 'file preview'),
    relativePath: providerRelativePath(record['relativePath'], 'file preview path', false),
    name: fileName(record['name'], 'file preview name'),
    content:
      record['content'] === undefined
        ? ''
        : requiredStringAllowEmpty(record['content'], 'file preview content'),
    mime: requiredString(record['mime'], 'file preview MIME'),
    bytesRead: requiredNonNegativeNumber(record['bytesRead'], 'file preview bytesRead'),
    sizeBytes: requiredNonNegativeNumber(record['sizeBytes'], 'file preview sizeBytes'),
    lines: requiredNonNegativeInteger(record['lines'], 'file preview lines'),
    truncated: requiredBoolean(record['truncated'], 'file preview truncated'),
    binary: requiredBoolean(record['binary'], 'file preview binary'),
  };
}

function parseOperationResult(raw: unknown): FileOperationResultView {
  const record = requiredRecord(raw, 'file operation');
  const status = requiredEnum(record['status'], OPERATION_STATUSES, 'file operation status');
  const destination =
    record['destination'] === undefined || record['destination'] === null
      ? undefined
      : parseAddress(record['destination'], 'file operation destination');
  const conflict =
    record['conflict'] === undefined || record['conflict'] === null
      ? undefined
      : parseConflict(record['conflict']);
  if (status === 'conflict' && !conflict) invalidResponse('file operation conflict');
  return {
    operationId: requiredString(record['operationId'], 'file operation operationId'),
    operation: requiredString(record['operation'], 'file operation name'),
    status,
    code:
      record['code'] === undefined
        ? ''
        : requiredEnum(record['code'], ERROR_CODES, 'file operation code'),
    source: parseAddress(record['source'], 'file operation source'),
    ...(destination ? { destination } : {}),
    affected: requiredArray(record['affected'], 'file operation affected').map((value) =>
      parseAddress(value, 'file operation affected address'),
    ),
    ...(conflict ? { conflict } : {}),
    message: requiredStringAllowEmpty(record['message'], 'file operation message'),
    receiptId:
      record['receiptId'] === undefined
        ? ''
        : requiredStringAllowEmpty(record['receiptId'], 'file operation receiptId'),
  };
}

function parseAddress(raw: unknown, context: string): FileAddressView {
  const record = requiredRecord(raw, context);
  return {
    mountId: providerMountId(record['mountId'], context),
    path: providerRelativePath(record['path'], `${context} path`),
  };
}

function parseConflict(raw: unknown) {
  const record = requiredRecord(raw, 'file operation conflict');
  return {
    source: parseAddress(record['source'], 'file conflict source'),
    destination: parseAddress(record['destination'], 'file conflict destination'),
    kind: requiredEnum(record['kind'], ENTRY_KINDS, 'file conflict kind'),
  };
}

function parseTrashSnapshot(raw: unknown): TrashSnapshotView {
  const record = requiredRecord(raw, 'trash snapshot');
  return {
    entries: requiredArray(record['entries'], 'trash entries').map((value) => {
      const entry = requiredRecord(value, 'trash entry');
      return {
        receiptId: requiredString(entry['receiptId'], 'trash entry receiptId'),
        mountId: providerMountId(entry['mountId'], 'trash entry'),
        originalPath: providerRelativePath(
          entry['originalPath'],
          'trash entry originalPath',
          false,
        ),
        name: fileName(entry['name'], 'trash entry name'),
        kind: requiredEnum(entry['kind'], ENTRY_KINDS, 'trash entry kind'),
        sizeBytes: requiredNonNegativeNumber(entry['sizeBytes'], 'trash entry sizeBytes'),
        trashedAt: requiredString(entry['trashedAt'], 'trash entry trashedAt'),
        available: requiredBoolean(entry['available'], 'trash entry available'),
        errorCode:
          entry['errorCode'] === undefined
            ? ''
            : requiredEnum(entry['errorCode'], ERROR_CODES, 'trash entry errorCode'),
      };
    }),
    refreshedAt: requiredString(record['refreshedAt'], 'trash refreshedAt'),
  };
}

function parseChangedEvent(raw: unknown): FilesChangedEvent {
  const record = requiredRecord(raw, 'Files event');
  return {
    operation: requiredString(record['operation'], 'Files event operation'),
    operationId: requiredString(record['operationId'], 'Files event operationId'),
    mountIds: requiredArray(record['mountIds'], 'Files event mountIds').map((value) =>
      providerMountId(value, 'Files event mountId'),
    ),
    paths: requiredArray(record['paths'], 'Files event paths').map((value) =>
      providerRelativePath(value, 'Files event path'),
    ),
    at: requiredString(record['at'], 'Files event at'),
  };
}

function listDirectoryRequest(input: ListDirectoryInput): ListDirectoryInput {
  const limit = requiredNonNegativeInteger(input.limit, 'Files request limit');
  return {
    mountId: requestMountId(input.mountId),
    path: requestPath(input.path),
    cursor: requiredRequestString(input.cursor, 'cursor'),
    limit,
  };
}

function previewRequest(input: PreviewInput): PreviewInput {
  return {
    mountId: requestMountId(input.mountId),
    path: requestPath(input.path, false),
  };
}

function createDirectoryRequest(input: CreateDirectoryInput): CreateDirectoryInput {
  return {
    mountId: requestMountId(input.mountId),
    parentPath: requestPath(input.parentPath),
    name: requestName(input.name),
  };
}

function renameRequest(input: RenameInput): RenameInput {
  return {
    mountId: requestMountId(input.mountId),
    path: requestPath(input.path, false),
    name: requestName(input.name),
  };
}

function transferRequest(input: TransferInput): TransferInput {
  return {
    source: requestAddress(input.source),
    destination: requestAddress(input.destination),
  };
}

function trashRequest(input: TrashInput): TrashInput {
  return {
    mountId: requestMountId(input.mountId),
    path: requestPath(input.path, false),
  };
}

function restoreRequest(input: RestoreInput): RestoreInput {
  return {
    receiptId: requiredRequestString(input.receiptId, 'receipt ID', false),
  };
}

function deleteRequest(input: DeleteInput): DeleteInput {
  return {
    mountId: input.mountId ? requestMountId(input.mountId) : '',
    path: input.path ? requestPath(input.path, false) : '',
    receiptId: requiredRequestString(input.receiptId, 'receipt ID'),
    recursive: requiredRequestBoolean(input.recursive, 'recursive'),
    confirmed: requiredRequestBoolean(input.confirmed, 'confirmed'),
  };
}

function requestAddress(value: FileAddressView): FileAddressView {
  return {
    mountId: requestMountId(value.mountId),
    path: requestPath(value.path, false),
  };
}

function requestMountId(value: unknown): string {
  if (typeof value !== 'string' || !validMountId(value)) {
    throw new Error('The Files request contains an invalid mount ID.');
  }
  return value;
}

function requestPath(value: unknown, allowEmpty = true): string {
  if (typeof value !== 'string' || !validRelativePath(value) || (!allowEmpty && !value)) {
    throw new Error('The Files request contains an invalid provider-relative path.');
  }
  return value;
}

function requestName(value: unknown): string {
  if (
    typeof value !== 'string' ||
    !value ||
    value.trim() !== value ||
    value === '.' ||
    value === '..' ||
    value.includes('/') ||
    value.includes('\\') ||
    /[\u0000-\u001f\u007f]/.test(value)
  ) {
    throw new Error('The Files request contains an invalid name.');
  }
  return value;
}

function requiredRequestString(value: unknown, context: string, allowEmpty = true): string {
  if (typeof value !== 'string' || (!allowEmpty && !value)) {
    throw new Error(`The Files request contains an invalid ${context}.`);
  }
  return value;
}

function requiredRequestBoolean(value: unknown, context: string): boolean {
  if (typeof value !== 'boolean') {
    throw new Error(`The Files request contains an invalid ${context}.`);
  }
  return value;
}

function providerMountId(value: unknown, context: string): string {
  const mountId = requiredString(value, `${context} mountId`);
  if (!validMountId(mountId)) invalidResponse(`${context} mountId`);
  return mountId;
}

function providerRelativePath(value: unknown, context: string, allowEmpty = true): string {
  const path = requiredStringAllowEmpty(value, context);
  if (!validRelativePath(path) || (!allowEmpty && !path)) {
    invalidResponse(context);
  }
  return path;
}

function fileName(value: unknown, context: string): string {
  const name = requiredString(value, context);
  if (
    name === '.' ||
    name === '..' ||
    name.includes('/') ||
    name.includes('\\') ||
    /[\u0000-\u001f\u007f]/.test(name)
  ) {
    invalidResponse(context);
  }
  return name;
}

function requiredRecord(raw: unknown, context: string): Record<string, unknown> {
  if (raw === null || typeof raw !== 'object' || Array.isArray(raw)) {
    invalidResponse(context);
  }
  return raw as Record<string, unknown>;
}

function requiredArray(raw: unknown, context: string): readonly unknown[] {
  if (!Array.isArray(raw)) invalidResponse(context);
  return raw;
}

function requiredString(raw: unknown, context: string): string {
  if (typeof raw !== 'string' || !raw) invalidResponse(context);
  return raw;
}

function requiredStringAllowEmpty(raw: unknown, context: string): string {
  if (typeof raw !== 'string') invalidResponse(context);
  return raw;
}

function requiredBoolean(raw: unknown, context: string): boolean {
  if (typeof raw !== 'boolean') invalidResponse(context);
  return raw;
}

function requiredNonNegativeNumber(raw: unknown, context: string): number {
  if (typeof raw !== 'number' || !Number.isFinite(raw) || raw < 0) {
    invalidResponse(context);
  }
  return raw;
}

function requiredNonNegativeInteger(raw: unknown, context: string): number {
  const value = requiredNonNegativeNumber(raw, context);
  if (!Number.isInteger(value)) invalidResponse(context);
  return value;
}

function requiredEnum<const Value extends string>(
  raw: unknown,
  values: readonly Value[],
  context: string,
): Value {
  if (typeof raw !== 'string' || !values.includes(raw as Value)) {
    invalidResponse(context);
  }
  return raw as Value;
}

function rejectProviderFields(raw: unknown, context: string): void {
  const seen = new WeakSet<object>();
  const inspect = (value: unknown): void => {
    if (value === null || typeof value !== 'object') return;
    if (seen.has(value)) return;
    seen.add(value);
    if (Array.isArray(value)) {
      value.forEach(inspect);
      return;
    }
    for (const [key, nested] of Object.entries(value)) {
      if (FORBIDDEN_RESPONSE_FIELDS.has(key.toLocaleLowerCase('en-GB'))) {
        invalidResponse(context);
      }
      inspect(nested);
    }
  };
  inspect(raw);
}

function invalidResponse(context: string): never {
  throw new Error(`The ${context} contains an invalid Files response.`);
}

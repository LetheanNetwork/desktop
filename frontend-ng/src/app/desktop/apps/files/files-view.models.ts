export type FilesDataState = 'demo' | 'loading' | 'live' | 'stale' | 'unavailable';

export type FilesViewMode = 'grid' | 'list';
export type FileEntryKind = 'file' | 'directory' | 'link' | 'other';

export type FilesLocation =
  | { readonly kind: 'home' }
  | { readonly kind: 'trash' }
  | {
      readonly kind: 'directory';
      readonly mountId: string;
      readonly path: string;
    };

export interface FileAddressView {
  readonly mountId: string;
  readonly path: string;
}

export interface FilesCapabilities {
  readonly list: boolean;
  readonly preview: boolean;
  readonly createDirectory: boolean;
  readonly write: boolean;
  readonly rename: boolean;
  readonly copyFrom: boolean;
  readonly copyTo: boolean;
  readonly move: boolean;
  readonly trash: boolean;
  readonly restore: boolean;
  readonly delete: boolean;
}

export interface FileCapacityView {
  readonly freeBytes: number;
  readonly totalBytes: number;
}

export interface FileMountView {
  readonly id: string;
  readonly name: string;
  readonly kind: string;
  readonly icon: string;
  readonly brand: boolean;
  readonly capabilities: FilesCapabilities;
  readonly capacity?: FileCapacityView;
}

export interface FileEntryView {
  readonly name: string;
  readonly relativePath: string;
  readonly kind: FileEntryKind;
  readonly sizeBytes: number;
  readonly modifiedAt: string;
  readonly mode: number;
  readonly hidden: boolean;
}

export interface FileFavouriteView {
  readonly mountId: string;
  readonly path: string;
}

export interface FileRecentView {
  readonly mountId: string;
  readonly path: string;
  readonly name: string;
  readonly kind: FileEntryKind;
  readonly openedAt: string;
}

export interface FilesCatalogueView {
  readonly mounts: readonly FileMountView[];
  readonly favourites: readonly FileFavouriteView[];
  readonly recent: readonly FileRecentView[];
}

export interface FileBreadcrumbView {
  readonly name: string;
  readonly path: string;
}

export interface DirectorySnapshotView {
  readonly mount: FileMountView;
  readonly path: string;
  readonly breadcrumbs: readonly FileBreadcrumbView[];
  readonly entries: readonly FileEntryView[];
  readonly nextCursor: string;
  readonly totalKnown: number;
  readonly refreshedAt: string;
}

export interface FilePreviewView {
  readonly mountId: string;
  readonly relativePath: string;
  readonly name: string;
  readonly content: string;
  readonly mime: string;
  readonly bytesRead: number;
  readonly sizeBytes: number;
  readonly lines: number;
  readonly truncated: boolean;
  readonly binary: boolean;
}

export type FileOperationStatus = 'completed' | 'conflict' | 'partial';

export type FilesErrorCode =
  | ''
  | 'files.invalid_input'
  | 'files.invalid_mount'
  | 'files.boundary_rejected'
  | 'files.capability_denied'
  | 'files.missing_entry'
  | 'files.conflict'
  | 'files.provider_unavailable'
  | 'files.limit_exceeded'
  | 'files.unsupported_entry'
  | 'files.partial_move';

export interface FileConflictView {
  readonly source: FileAddressView;
  readonly destination: FileAddressView;
  readonly kind: FileEntryKind;
}

export interface FileOperationResultView {
  readonly operationId: string;
  readonly operation: string;
  readonly status: FileOperationStatus;
  readonly code: FilesErrorCode;
  readonly source: FileAddressView;
  readonly destination?: FileAddressView;
  readonly affected: readonly FileAddressView[];
  readonly conflict?: FileConflictView;
  readonly message: string;
  readonly receiptId: string;
}

export interface TrashEntryView {
  readonly receiptId: string;
  readonly mountId: string;
  readonly originalPath: string;
  readonly name: string;
  readonly kind: FileEntryKind;
  readonly sizeBytes: number;
  readonly trashedAt: string;
  readonly available: boolean;
  readonly errorCode: FilesErrorCode;
}

export interface TrashSnapshotView {
  readonly entries: readonly TrashEntryView[];
  readonly refreshedAt: string;
}

export interface FilesChangedEvent {
  readonly operation: string;
  readonly operationId: string;
  readonly mountIds: readonly string[];
  readonly paths: readonly string[];
  readonly at: string;
}

export interface ListDirectoryInput {
  readonly mountId: string;
  readonly path: string;
  readonly cursor: string;
  readonly limit: number;
}

export interface PreviewInput {
  readonly mountId: string;
  readonly path: string;
}

export interface CreateDirectoryInput {
  readonly mountId: string;
  readonly parentPath: string;
  readonly name: string;
}

export interface RenameInput {
  readonly mountId: string;
  readonly path: string;
  readonly name: string;
}

export interface TransferInput {
  readonly source: FileAddressView;
  readonly destination: FileAddressView;
}

export interface TrashInput {
  readonly mountId: string;
  readonly path: string;
}

export interface RestoreInput {
  readonly receiptId: string;
}

export interface DeleteInput {
  readonly mountId: string;
  readonly path: string;
  readonly receiptId: string;
  readonly recursive: boolean;
  readonly confirmed: boolean;
}

export interface FilesDataSource {
  listMounts(): Promise<FilesCatalogueView>;
  listDirectory(input: ListDirectoryInput): Promise<DirectorySnapshotView>;
  preview(input: PreviewInput): Promise<FilePreviewView>;
  createDirectory(input: CreateDirectoryInput): Promise<FileOperationResultView>;
  rename(input: RenameInput): Promise<FileOperationResultView>;
  copy(input: TransferInput): Promise<FileOperationResultView>;
  move(input: TransferInput): Promise<FileOperationResultView>;
  trash(input: TrashInput): Promise<FileOperationResultView>;
  listTrash(): Promise<TrashSnapshotView>;
  restore(input: RestoreInput): Promise<FileOperationResultView>;
  delete(input: DeleteInput): Promise<FileOperationResultView>;
}

export interface FilesHomeSnapshotView {
  readonly entries: readonly FileEntryView[];
}

export interface FilesBreadcrumbState {
  readonly label: string;
  readonly token: string;
}

export interface FilesBrowserEntryView extends FileEntryView {
  readonly mountId: string;
  readonly icon: string;
  readonly detail: string;
  readonly receiptId: string;
  readonly available: boolean;
}

export interface FilesViewState {
  readonly location: FilesLocation;
  readonly token: string;
  readonly dataState: FilesDataState;
  readonly viewMode: FilesViewMode;
  readonly catalogue: FilesCatalogueView;
  readonly activeMount?: FileMountView;
  readonly entries: readonly FilesBrowserEntryView[];
  readonly breadcrumbs: readonly FilesBreadcrumbState[];
  readonly upToken: string;
  readonly providerLabel: string;
  readonly capacityLabel: string;
  readonly itemCount: number;
  readonly folderCount: number;
  readonly fileCount: number;
  readonly emptyLabel: string;
  readonly capabilities: FilesCapabilities;
  readonly refreshedAt: string;
}

export type FilesOperationKind =
  'create-directory' | 'rename' | 'copy' | 'move' | 'trash' | 'restore' | 'delete';

export type FilesDialogState =
  'form' | 'confirm' | 'busy' | 'conflict' | 'partial' | 'success' | 'error';

export interface FilesOperationDialogView {
  readonly state: FilesDialogState;
  readonly operation: FilesOperationKind;
  readonly title: string;
  readonly message: string;
  readonly source?: FileAddressView;
  readonly destination?: FileAddressView;
  readonly receiptId?: string;
  readonly recursive?: boolean;
  readonly requiresRecursiveConfirmation?: boolean;
  readonly name?: string;
  readonly result?: FileOperationResultView;
}

export type FilesActionIntent =
  | { readonly type: 'navigate'; readonly token: string }
  | { readonly type: 'home' }
  | { readonly type: 'up' }
  | { readonly type: 'refresh' }
  | { readonly type: 'set-view'; readonly view: FilesViewMode }
  | {
      readonly type: 'select-entry';
      readonly mountId: string;
      readonly path: string;
      readonly receiptId: string;
    }
  | {
      readonly type: 'open-directory';
      readonly mountId: string;
      readonly path: string;
    }
  | {
      readonly type: 'preview';
      readonly mountId: string;
      readonly path: string;
    }
  | { readonly type: 'close-preview' }
  | {
      readonly type: 'open-operation';
      readonly operation: FilesOperationKind;
      readonly source?: FileAddressView;
      readonly receiptId?: string;
      readonly recursive?: boolean;
    }
  | {
      readonly type: 'submit-operation';
      readonly operation: FilesOperationKind;
      readonly source?: FileAddressView;
      readonly destination?: FileAddressView;
      readonly receiptId?: string;
      readonly name?: string;
      readonly recursive?: boolean;
      readonly confirmed?: boolean;
    }
  | { readonly type: 'dismiss-dialog' };

import type {
  DirectorySnapshotView,
  FileCapacityView,
  FileEntryKind,
  FileEntryView,
  FileMountView,
  FilesBrowserEntryView,
  FilesCapabilities,
  FilesCatalogueView,
  FilesChangedEvent,
  FilesDataState,
  FilesLocation,
  FilesViewMode,
  FilesViewState,
  TrashSnapshotView,
} from './files-view.models';

const HOME: FilesLocation = { kind: 'home' };
const MOUNT_ID = /^[a-z0-9](?:[a-z0-9-]{0,62}[a-z0-9])?$/;
const WINDOWS_ABSOLUTE = /^[A-Za-z]:/;
const CONTROL_CHARACTERS = /[\u0000-\u001f\u007f]/;

export interface BuildFilesViewStateInput {
  readonly catalogue: FilesCatalogueView;
  readonly location: FilesLocation;
  readonly dataState: FilesDataState;
  readonly viewMode: FilesViewMode;
  readonly directory?: DirectorySnapshotView | null;
  readonly trash?: TrashSnapshotView | null;
}

export function filesToken(location: FilesLocation): string {
  if (location.kind === 'home' || location.kind === 'trash') {
    return location.kind;
  }
  if (!validMountId(location.mountId) || !validRelativePath(location.path)) {
    return 'home';
  }
  return location.path
    ? `${location.mountId}::${encodeURIComponent(location.path)}`
    : location.mountId;
}

export function parseFilesToken(token: string | null | undefined): FilesLocation {
  if (!token || token === 'home') return HOME;
  if (token === 'trash') return { kind: 'trash' };
  if (CONTROL_CHARACTERS.test(token)) return HOME;

  const delimiter = token.indexOf('::');
  if (delimiter < 0) {
    return validMountId(token) ? { kind: 'directory', mountId: token, path: '' } : HOME;
  }
  if (delimiter === 0 || token.indexOf('::', delimiter + 2) >= 0) return HOME;

  const mountId = token.slice(0, delimiter);
  const encodedPath = token.slice(delimiter + 2);
  if (!validMountId(mountId) || !encodedPath) return HOME;

  try {
    const path = decodeURIComponent(encodedPath);
    return validRelativePath(path) ? { kind: 'directory', mountId, path } : HOME;
  } catch {
    return HOME;
  }
}

export function reconcileLocation(
  location: FilesLocation,
  catalogue: FilesCatalogueView,
): FilesLocation {
  if (location.kind !== 'directory') return location;
  return catalogue.mounts.some(({ id }) => id === location.mountId) ? location : HOME;
}

export function buildFilesViewState(input: BuildFilesViewStateInput): FilesViewState {
  const location = reconcileLocation(input.location, input.catalogue);
  const activeMount =
    location.kind === 'directory'
      ? input.catalogue.mounts.find(({ id }) => id === location.mountId)
      : undefined;
  const entries = viewEntries(
    input.catalogue,
    location,
    input.directory ?? null,
    input.trash ?? null,
  );
  const folderCount = entries.filter(({ kind }) => kind === 'directory').length;
  const fileCount = entries.length - folderCount;
  const capacity =
    activeMount?.capacity ??
    (location.kind === 'home' ? commonCapacity(input.catalogue.mounts) : undefined);

  return {
    location,
    token: filesToken(location),
    dataState: input.dataState,
    viewMode: input.viewMode,
    catalogue: input.catalogue,
    ...(activeMount ? { activeMount } : {}),
    entries,
    breadcrumbs: breadcrumbs(location, activeMount, input.directory ?? null),
    upToken: parentToken(location),
    providerLabel:
      location.kind === 'home'
        ? $localize`:File browser place@@files.place.home:Home`
        : location.kind === 'trash'
          ? $localize`:File browser place@@files.place.trash:Trash`
          : (activeMount?.name ?? ''),
    capacityLabel: capacity ? capacityLabel(capacity) : '',
    itemCount: entries.length,
    folderCount,
    fileCount,
    emptyLabel:
      location.kind === 'trash'
        ? $localize`:Empty trash message@@files.empty.trash:Trash is empty`
        : $localize`:Empty folder message@@files.empty.folder:This folder is empty`,
    capabilities: capabilitiesFor(location, input.catalogue, activeMount),
    refreshedAt:
      location.kind === 'directory'
        ? (input.directory?.refreshedAt ?? '')
        : location.kind === 'trash'
          ? (input.trash?.refreshedAt ?? '')
          : '',
  };
}

export function eventAffectsLocation(event: FilesChangedEvent, location: FilesLocation): boolean {
  if (location.kind === 'home') return event.mountIds.length > 0;
  if (location.kind === 'trash') {
    return ['trash', 'restore', 'delete'].includes(event.operation);
  }
  if (!event.mountIds.includes(location.mountId)) return false;
  if (!event.paths.length || location.path === '') return true;
  return event.paths.some(
    (path) =>
      path === location.path ||
      path.startsWith(`${location.path}/`) ||
      location.path.startsWith(`${parentPath(path)}/`) ||
      parentPath(path) === location.path,
  );
}

export function validMountId(value: string): boolean {
  return MOUNT_ID.test(value);
}

export function validRelativePath(value: string): boolean {
  if (
    value.startsWith('/') ||
    value.endsWith('/') ||
    value.includes('\\') ||
    value.includes('//') ||
    WINDOWS_ABSOLUTE.test(value) ||
    CONTROL_CHARACTERS.test(value)
  ) {
    return false;
  }
  return value === '' || value.split('/').every((part) => part !== '.' && part !== '..' && part);
}

export function joinRelativePath(parent: string, name: string): string {
  const path = parent ? `${parent}/${name}` : name;
  if (!validRelativePath(path)) {
    throw new Error('Files paths must remain provider-relative.');
  }
  return path;
}

export function parentPath(path: string): string {
  const separator = path.lastIndexOf('/');
  return separator < 0 ? '' : path.slice(0, separator);
}

function viewEntries(
  catalogue: FilesCatalogueView,
  location: FilesLocation,
  directory: DirectorySnapshotView | null,
  trash: TrashSnapshotView | null,
): readonly FilesBrowserEntryView[] {
  if (location.kind === 'home') {
    return [
      ...catalogue.mounts.map((mount): FilesBrowserEntryView => ({
        mountId: mount.id,
        name: mount.name,
        relativePath: '',
        kind: 'directory',
        sizeBytes: 0,
        modifiedAt: '',
        mode: 0,
        hidden: false,
        icon: mount.icon || 'folder',
        detail: '',
        receiptId: '',
        available: true,
      })),
      ...catalogue.recent.map((recent): FilesBrowserEntryView => ({
        mountId: recent.mountId,
        name: recent.name,
        relativePath: recent.path,
        kind: recent.kind,
        sizeBytes: 0,
        modifiedAt: recent.openedAt,
        mode: 0,
        hidden: false,
        icon: entryIcon(recent.kind, recent.name),
        detail: '',
        receiptId: '',
        available: true,
      })),
    ];
  }
  if (location.kind === 'trash') {
    return (trash?.entries ?? []).map((entry) => ({
      mountId: entry.mountId,
      name: entry.name,
      relativePath: entry.originalPath,
      kind: entry.kind,
      sizeBytes: entry.sizeBytes,
      modifiedAt: entry.trashedAt,
      mode: 0,
      hidden: false,
      icon: entryIcon(entry.kind, entry.name),
      detail: formatBytes(entry.sizeBytes),
      receiptId: entry.receiptId,
      available: entry.available,
    }));
  }
  return (directory?.entries ?? []).map((entry) => browserEntry(location.mountId, entry));
}

function browserEntry(mountId: string, entry: FileEntryView): FilesBrowserEntryView {
  return {
    ...entry,
    mountId,
    icon: entryIcon(entry.kind, entry.name),
    detail: entry.kind === 'directory' ? '' : formatBytes(entry.sizeBytes),
    receiptId: '',
    available: true,
  };
}

function breadcrumbs(
  location: FilesLocation,
  activeMount: FileMountView | undefined,
  directory: DirectorySnapshotView | null,
) {
  const home = {
    label: $localize`:File browser place@@files.place.home:Home`,
    token: 'home',
  };
  if (location.kind === 'home') return [home];
  if (location.kind === 'trash') {
    return [
      home,
      {
        label: $localize`:File browser place@@files.place.trash:Trash`,
        token: 'trash',
      },
    ];
  }

  return [
    home,
    {
      label: activeMount?.name ?? location.mountId,
      token: location.mountId,
    },
    ...(directory?.breadcrumbs ?? []).map(({ name, path }) => ({
      label: name,
      token: filesToken({
        kind: 'directory' as const,
        mountId: location.mountId,
        path,
      }),
    })),
  ];
}

function parentToken(location: FilesLocation): string {
  if (location.kind === 'home') return '';
  if (location.kind === 'trash') return 'home';
  if (!location.path) return 'home';
  return filesToken({
    kind: 'directory',
    mountId: location.mountId,
    path: parentPath(location.path),
  });
}

function capabilitiesFor(
  location: FilesLocation,
  catalogue: FilesCatalogueView,
  activeMount: FileMountView | undefined,
): FilesCapabilities {
  if (location.kind === 'directory' && activeMount) {
    return activeMount.capabilities;
  }
  if (location.kind === 'trash') {
    return mergeCapabilities(catalogue.mounts.map(({ capabilities }) => capabilities));
  }
  return noCapabilities();
}

function mergeCapabilities(values: readonly FilesCapabilities[]): FilesCapabilities {
  const fields: readonly (keyof FilesCapabilities)[] = [
    'list',
    'preview',
    'open',
    'reveal',
    'createDirectory',
    'write',
    'rename',
    'copyFrom',
    'copyTo',
    'move',
    'trash',
    'restore',
    'delete',
  ];
  return Object.fromEntries(
    fields.map((field) => [field, values.some((value) => value[field])]),
  ) as unknown as FilesCapabilities;
}

function noCapabilities(): FilesCapabilities {
  return {
    list: false,
    preview: false,
    open: false,
    reveal: false,
    createDirectory: false,
    write: false,
    rename: false,
    copyFrom: false,
    copyTo: false,
    move: false,
    trash: false,
    restore: false,
    delete: false,
  };
}

function commonCapacity(mounts: readonly FileMountView[]): FileCapacityView | undefined {
  const capacities = mounts.flatMap(({ capacity }) => (capacity ? [capacity] : []));
  return capacities[0];
}

function capacityLabel(capacity: FileCapacityView): string {
  return $localize`:File browser free-space status@@files.freeSpace:${formatBytes(capacity.freeBytes)}:free: free of ${formatBytes(capacity.totalBytes)}:total:`;
}

export function formatBytes(value: number): string {
  if (!Number.isFinite(value) || value <= 0) return value === 0 ? '0 B' : '—';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'] as const;
  const unitIndex = Math.min(Math.floor(Math.log(value) / Math.log(1024)), units.length - 1);
  const amount = value / 1024 ** unitIndex;
  const precision = amount >= 10 || Number.isInteger(amount) ? 0 : 1;
  return `${amount.toFixed(precision)} ${units[unitIndex]}`;
}

function entryIcon(kind: FileEntryKind, name: string): string {
  if (kind === 'directory') return 'folder';
  if (kind === 'link') return 'link';
  if (kind === 'other') return 'file-circle-question';
  const extension = name.split('.').at(-1)?.toLocaleLowerCase('en-GB') ?? '';
  if (extension === 'pdf') return 'file-pdf';
  if (['png', 'jpg', 'jpeg', 'gif', 'webp', 'svg'].includes(extension)) {
    return 'file-image';
  }
  if (['zip', 'gz', 'tgz', 'tar', 'dmg'].includes(extension)) {
    return 'file-zipper';
  }
  if (['gguf', 'safetensors'].includes(extension)) return 'cube';
  if (['mp3', 'wav', 'm4a', 'flac'].includes(extension)) return 'file-audio';
  if (['ts', 'js', 'go', 'json', 'yaml', 'yml', 'md', 'html', 'css', 'scss'].includes(extension)) {
    return 'file-code';
  }
  return 'file-lines';
}

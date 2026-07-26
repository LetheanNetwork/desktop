import type { DirectorySnapshotView, FilesCatalogueView } from './files-view.models';
import {
  buildFilesViewState,
  filesToken,
  parseFilesToken,
  reconcileLocation,
} from './files-view-state';

const capabilities = {
  list: true,
  preview: true,
  createDirectory: true,
  write: true,
  rename: true,
  copyFrom: true,
  copyTo: true,
  move: true,
  trash: true,
  restore: true,
  delete: true,
} as const;

const catalogue: FilesCatalogueView = {
  mounts: [
    {
      id: 'documents',
      name: 'Documents',
      kind: 'local',
      icon: 'folder',
      brand: false,
      capabilities,
      capacity: {
        freeBytes: 218 * 1024 ** 3,
        totalBytes: 512 * 1024 ** 3,
      },
    },
  ],
  favourites: [],
  recent: [
    {
      mountId: 'documents',
      path: 'welcome.txt',
      name: 'welcome.txt',
      kind: 'file',
      openedAt: '2026-07-26T09:14:00Z',
    },
  ],
};

describe('Files navigation tokens', () => {
  it('keeps existing root mount ids and reversibly encodes nested paths', () => {
    expect(filesToken({ kind: 'directory', mountId: 'documents', path: '' })).toBe('documents');
    const token = filesToken({
      kind: 'directory',
      mountId: 'documents',
      path: 'Invoices/2026 July',
    });
    expect(token).toBe('documents::Invoices%2F2026%20July');
    expect(parseFilesToken(token)).toEqual({
      kind: 'directory',
      mountId: 'documents',
      path: 'Invoices/2026 July',
    });
  });

  it.each([
    '/etc',
    '../secret',
    'documents::%ZZ',
    'documents::a%5Cb',
    'documents::a%2F..%2Fb',
    'documents::%00secret',
    'documents::',
    'unknown:shape:value',
  ])('fails closed to home for %s', (token) => {
    expect(parseFilesToken(token)).toEqual({ kind: 'home' });
  });

  it('reconciles an unknown or removed mount to Home', () => {
    expect(
      reconcileLocation({ kind: 'directory', mountId: 'missing', path: '' }, catalogue),
    ).toEqual({ kind: 'home' });
    expect(reconcileLocation({ kind: 'trash' }, catalogue)).toEqual({ kind: 'trash' });
  });
});

describe('Files view state', () => {
  it('builds provider-neutral Home rows and the demo capacity label', () => {
    const state = buildFilesViewState({
      catalogue,
      location: { kind: 'home' },
      dataState: 'demo',
      viewMode: 'grid',
    });

    expect(state.entries.map(({ name }) => name)).toEqual(['Documents', 'welcome.txt']);
    expect(state.providerLabel).toBe('Home');
    expect(state.capacityLabel).toBe('218 GB free of 512 GB');
    expect(state.itemCount).toBe(2);
    expect(state.folderCount).toBe(1);
    expect(state.fileCount).toBe(1);
  });

  it('derives breadcrumbs and Up from a directory snapshot', () => {
    const directory: DirectorySnapshotView = {
      mount: catalogue.mounts[0],
      path: 'Invoices/2026',
      breadcrumbs: [
        { name: 'Invoices', path: 'Invoices' },
        { name: '2026', path: 'Invoices/2026' },
      ],
      entries: [],
      nextCursor: '',
      totalKnown: 0,
      refreshedAt: '2026-07-26T12:00:00Z',
    };

    const state = buildFilesViewState({
      catalogue,
      location: {
        kind: 'directory',
        mountId: 'documents',
        path: 'Invoices/2026',
      },
      directory,
      dataState: 'live',
      viewMode: 'list',
    });

    expect(state.breadcrumbs.map(({ label, token }) => ({ label, token }))).toEqual([
      { label: 'Home', token: 'home' },
      { label: 'Documents', token: 'documents' },
      { label: 'Invoices', token: 'documents::Invoices' },
      { label: '2026', token: 'documents::Invoices%2F2026' },
    ]);
    expect(state.upToken).toBe('documents::Invoices');
    expect(state.providerLabel).toBe('Documents');
    expect(state.capacityLabel).toBe('218 GB free of 512 GB');
  });
});

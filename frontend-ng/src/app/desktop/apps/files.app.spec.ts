import { signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import '../../../kit/lthn-core';
import { ConnectionManagerService } from '../../connection-manager.service';
import { DesktopFilesBridgeService } from '../desktop-files-bridge.service';
import type { Win } from '../desktop.data';
import { WindowManagerService } from '../window-manager.service';
import type { DirectorySnapshotView, FilesCatalogueView } from './files/files-view.models';
import { FilesApp } from './files.app';

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

const documentsMount = {
  id: 'documents',
  name: 'Documents',
  kind: 'local',
  icon: 'folder',
  brand: false,
  capabilities,
  capacity: {
    freeBytes: 312 * 1024 ** 3,
    totalBytes: 1024 ** 4,
  },
} as const;

const liveCatalogue: FilesCatalogueView = {
  mounts: [documentsMount],
  favourites: [],
  recent: [
    {
      mountId: 'documents',
      path: 'desktop.data.ts',
      name: 'desktop.data.ts',
      kind: 'file',
      openedAt: '08:31',
    },
  ],
};

function operationResult(operation: string, overrides: Record<string, unknown> = {}) {
  return {
    operationId: `files-${operation}`,
    operation,
    status: 'completed',
    code: '',
    source: { mountId: 'documents', path: 'notes.md' },
    affected: [{ mountId: 'documents', path: 'notes.md' }],
    message: `${operation} completed`,
    receiptId: '',
    ...overrides,
  };
}

function directory(
  path = '',
  entries: DirectorySnapshotView['entries'] = [
    {
      name: 'Invoices',
      relativePath: path ? `${path}/Invoices` : 'Invoices',
      kind: 'directory',
      sizeBytes: 0,
      modifiedAt: '',
      mode: 493,
      hidden: false,
    },
    {
      name: 'notes.md',
      relativePath: path ? `${path}/notes.md` : 'notes.md',
      kind: 'file',
      sizeBytes: 2048,
      modifiedAt: 'Today',
      mode: 420,
      hidden: false,
    },
  ],
): DirectorySnapshotView {
  const parts = path ? path.split('/') : [];
  return {
    mount: documentsMount,
    path,
    breadcrumbs: parts.map((name, index) => ({
      name,
      path: parts.slice(0, index + 1).join('/'),
    })),
    entries,
    nextCursor: '',
    totalKnown: entries.length,
    refreshedAt: '2026-07-26T12:00:00Z',
  };
}

const filesWin: Win = {
  id: 'files-window',
  app: 'files',
  sub: 'home',
  systab: 'list',
  x: 0,
  y: 0,
  w: 760,
  h: 520,
  z: 1,
  min: false,
  max: false,
};

describe('FilesApp browsing', () => {
  const offline = signal(true);
  let changedHandler:
    | ((event: {
        operation: string;
        operationId: string;
        mountIds: readonly string[];
        paths: readonly string[];
        at: string;
      }) => void)
    | undefined;
  const offEvents = vi.fn();
  const bridge = {
    listMounts: vi.fn(),
    listDirectory: vi.fn(),
    preview: vi.fn(),
    listTrash: vi.fn(),
    onChanged: vi.fn((_handler: NonNullable<typeof changedHandler>) => vi.fn()),
    createDirectory: vi.fn(),
    rename: vi.fn(),
    copy: vi.fn(),
    move: vi.fn(),
    trash: vi.fn(),
    restore: vi.fn(),
    delete: vi.fn(),
  };
  const windowManager = {
    setSub: vi.fn(),
    setSysTab: vi.fn(),
  };

  beforeEach(() => {
    offline.set(true);
    changedHandler = undefined;
    vi.clearAllMocks();
    bridge.listMounts.mockResolvedValue(liveCatalogue);
    bridge.listDirectory.mockImplementation(async ({ path }: { path: string }) => directory(path));
    bridge.listTrash.mockResolvedValue({
      entries: [],
      refreshedAt: '2026-07-26T12:00:00Z',
    });
    bridge.preview.mockResolvedValue({
      mountId: 'documents',
      relativePath: 'notes.md',
      name: 'notes.md',
      content: '# Notes',
      mime: 'text/markdown',
      bytesRead: 7,
      sizeBytes: 7,
      lines: 1,
      truncated: false,
      binary: false,
    });
    bridge.createDirectory.mockResolvedValue(operationResult('create-directory'));
    bridge.rename.mockResolvedValue(operationResult('rename'));
    bridge.copy.mockResolvedValue(operationResult('copy'));
    bridge.move.mockResolvedValue(operationResult('move'));
    bridge.trash.mockResolvedValue(operationResult('trash', { receiptId: 'receipt-1' }));
    bridge.restore.mockResolvedValue(operationResult('restore'));
    bridge.delete.mockResolvedValue(operationResult('delete'));
    bridge.onChanged.mockImplementation((handler: NonNullable<typeof changedHandler>) => {
      changedHandler = handler;
      return offEvents;
    });
    TestBed.configureTestingModule({
      providers: [
        {
          provide: ConnectionManagerService,
          useValue: { offline: offline.asReadonly() },
        },
        { provide: DesktopFilesBridgeService, useValue: bridge },
        { provide: WindowManagerService, useValue: windowManager },
      ],
    });
  });

  afterEach(() => TestBed.resetTestingModule());

  async function create(win: Win = filesWin) {
    const fixture = TestBed.createComponent(FilesApp);
    fixture.componentRef.setInput('win', { ...win });
    await fixture.whenStable();
    return fixture;
  }

  it('shows the complete labelled demo without a bridge call or event listener', async () => {
    const fixture = await create();
    const text = (fixture.nativeElement as HTMLElement).textContent ?? '';

    expect(text).toContain('Demo data');
    expect(text).toContain('Documents');
    expect(text).toContain('welcome.txt');
    expect(text).toContain('218 GB free of 512 GB');
    expect(bridge.listMounts).not.toHaveBeenCalled();
    expect(bridge.listDirectory).not.toHaveBeenCalled();
    expect(bridge.onChanged).not.toHaveBeenCalled();
  });

  it('loads the live catalogue and current mount through the bridge', async () => {
    offline.set(false);
    const fixture = await create({ ...filesWin, sub: 'documents' });

    expect(bridge.listMounts).toHaveBeenCalledTimes(1);
    expect(bridge.listDirectory).toHaveBeenCalledWith({
      mountId: 'documents',
      path: '',
      cursor: '',
      limit: 200,
    });
    const text = (fixture.nativeElement as HTMLElement).textContent ?? '';
    expect(text).toContain('Live data');
    expect(text).toContain('notes.md');
    expect(text).not.toContain('welcome.txt');
  });

  it('keeps the legacy root token and opens a reversible nested token', async () => {
    offline.set(false);
    await create({ ...filesWin, sub: 'documents' });
    expect(bridge.listDirectory).toHaveBeenLastCalledWith(
      expect.objectContaining({ mountId: 'documents', path: '' }),
    );

    vi.clearAllMocks();
    await create({
      ...filesWin,
      id: 'nested-window',
      sub: 'documents::Invoices%2F2026%20July',
    });
    expect(bridge.listDirectory).toHaveBeenCalledWith({
      mountId: 'documents',
      path: 'Invoices/2026 July',
      cursor: '',
      limit: 200,
    });
  });

  it('opens directories, writes a reversible token, and preserves Up navigation', async () => {
    offline.set(false);
    const fixture = await create({ ...filesWin, sub: 'documents' });
    const element = fixture.nativeElement as HTMLElement;

    element
      .querySelector<HTMLButtonElement>('[data-path="Invoices"]')
      ?.dispatchEvent(new MouseEvent('dblclick', { bubbles: true }));
    await fixture.whenStable();

    expect(windowManager.setSub).toHaveBeenCalledWith('files-window', 'documents::Invoices');
    expect(bridge.listDirectory).toHaveBeenLastCalledWith(
      expect.objectContaining({ mountId: 'documents', path: 'Invoices' }),
    );

    element.querySelector<HTMLButtonElement>('[data-action="up"]')?.click();
    await fixture.whenStable();
    expect(windowManager.setSub).toHaveBeenLastCalledWith('files-window', 'documents');
  });

  it('loads bounded file previews and keeps grid/list in setSysTab', async () => {
    offline.set(false);
    const fixture = await create({ ...filesWin, sub: 'documents' });
    const element = fixture.nativeElement as HTMLElement;

    element
      .querySelector<HTMLButtonElement>('[data-path="notes.md"]')
      ?.dispatchEvent(new MouseEvent('dblclick', { bubbles: true }));
    await fixture.whenStable();
    expect(bridge.preview).toHaveBeenCalledWith({
      mountId: 'documents',
      path: 'notes.md',
    });
    expect(element.textContent).toContain('# Notes');

    element.querySelector<HTMLButtonElement>('[data-action="grid"]')?.click();
    expect(windowManager.setSysTab).toHaveBeenCalledWith('files-window', 'grid');
    expect(windowManager.setSub).not.toHaveBeenCalledWith('files-window', 'grid');
  });

  it('does not let an earlier response replace later navigation', async () => {
    offline.set(false);
    let resolveInvoices!: (value: DirectorySnapshotView) => void;
    bridge.listDirectory.mockImplementation(({ path }: { path: string }) =>
      path === 'Invoices'
        ? new Promise<DirectorySnapshotView>((resolve) => {
            resolveInvoices = resolve;
          })
        : Promise.resolve(
            directory(path, [
              {
                name: path ? 'later.txt' : 'Invoices',
                relativePath: path ? `${path}/later.txt` : 'Invoices',
                kind: path ? 'file' : 'directory',
                sizeBytes: 1,
                modifiedAt: 'Now',
                mode: 420,
                hidden: false,
              },
            ]),
          ),
    );
    const fixture = await create({ ...filesWin, sub: 'documents' });

    void fixture.componentInstance.navigateToken('documents::Invoices');
    await Promise.resolve();
    void fixture.componentInstance.navigateToken('documents::Archive');
    await fixture.whenStable();
    expect((fixture.nativeElement as HTMLElement).textContent).toContain('later.txt');

    resolveInvoices(
      directory('Invoices', [
        {
          name: 'stale.txt',
          relativePath: 'Invoices/stale.txt',
          kind: 'file',
          sizeBytes: 1,
          modifiedAt: 'Earlier',
          mode: 420,
          hidden: false,
        },
      ]),
    );
    await fixture.whenStable();
    expect((fixture.nativeElement as HTMLElement).textContent).not.toContain('stale.txt');
  });

  it('retains a successful live snapshot as stale and never inserts demo data', async () => {
    offline.set(false);
    const fixture = await create({ ...filesWin, sub: 'documents' });
    bridge.listDirectory.mockRejectedValue(new Error('provider unavailable'));

    await fixture.componentInstance.refresh();
    await fixture.whenStable();
    const text = (fixture.nativeElement as HTMLElement).textContent ?? '';
    expect(text).toContain('Live data stale');
    expect(text).toContain('notes.md');
    expect(text).not.toContain('welcome.txt');
  });

  it('shows unavailable live state without demo fallback when initial loading fails', async () => {
    offline.set(false);
    bridge.listMounts.mockRejectedValue(new Error('provider unavailable'));

    const fixture = await create();
    const text = (fixture.nativeElement as HTMLElement).textContent ?? '';
    expect(text).toContain('Live unavailable');
    expect(text).not.toContain('welcome.txt');
    expect(text).not.toContain('218 GB free of 512 GB');
  });
});

describe('FilesApp operations and events', () => {
  const offline = signal(false);
  let changedHandler:
    | ((event: {
        operation: string;
        operationId: string;
        mountIds: readonly string[];
        paths: readonly string[];
        at: string;
      }) => void)
    | undefined;
  const offEvents = vi.fn();
  const bridge = {
    listMounts: vi.fn(),
    listDirectory: vi.fn(),
    preview: vi.fn(),
    listTrash: vi.fn(),
    onChanged: vi.fn(),
    createDirectory: vi.fn(),
    rename: vi.fn(),
    copy: vi.fn(),
    move: vi.fn(),
    trash: vi.fn(),
    restore: vi.fn(),
    delete: vi.fn(),
  };
  const windowManager = {
    setSub: vi.fn(),
    setSysTab: vi.fn(),
  };

  beforeEach(() => {
    offline.set(false);
    changedHandler = undefined;
    vi.clearAllMocks();
    bridge.listMounts.mockResolvedValue(liveCatalogue);
    bridge.listDirectory.mockImplementation(async ({ path }: { path: string }) => directory(path));
    bridge.listTrash.mockResolvedValue({
      entries: [
        {
          receiptId: 'receipt-1',
          mountId: 'documents',
          originalPath: 'notes.md',
          name: 'notes.md',
          kind: 'file',
          sizeBytes: 2048,
          trashedAt: '2026-07-26T12:00:00Z',
          available: true,
          errorCode: '',
        },
      ],
      refreshedAt: '2026-07-26T12:00:00Z',
    });
    bridge.createDirectory.mockResolvedValue(operationResult('create-directory'));
    bridge.rename.mockResolvedValue(operationResult('rename'));
    bridge.copy.mockResolvedValue(operationResult('copy'));
    bridge.move.mockResolvedValue(operationResult('move'));
    bridge.trash.mockResolvedValue(operationResult('trash', { receiptId: 'receipt-1' }));
    bridge.restore.mockResolvedValue(operationResult('restore'));
    bridge.delete.mockResolvedValue(operationResult('delete'));
    bridge.onChanged.mockImplementation((handler: NonNullable<typeof changedHandler>) => {
      changedHandler = handler;
      return offEvents;
    });
    TestBed.configureTestingModule({
      providers: [
        {
          provide: ConnectionManagerService,
          useValue: { offline: offline.asReadonly() },
        },
        { provide: DesktopFilesBridgeService, useValue: bridge },
        { provide: WindowManagerService, useValue: windowManager },
      ],
    });
  });

  afterEach(() => TestBed.resetTestingModule());

  async function create(win: Win = { ...filesWin, sub: 'documents' }) {
    const fixture = TestBed.createComponent(FilesApp);
    fixture.componentRef.setInput('win', { ...win });
    await fixture.whenStable();
    return fixture;
  }

  function select(element: HTMLElement, path: string): void {
    element.querySelector<HTMLButtonElement>(`[data-path="${path}"]`)?.click();
  }

  it('shows actions only when the active mount capability permits them', async () => {
    bridge.listMounts.mockResolvedValue({
      ...liveCatalogue,
      mounts: [
        {
          ...documentsMount,
          capabilities: {
            ...capabilities,
            createDirectory: false,
            rename: false,
            copyFrom: false,
            move: false,
            trash: false,
          },
        },
      ],
    });
    const fixture = await create();
    const element = fixture.nativeElement as HTMLElement;
    select(element, 'notes.md');
    await fixture.whenStable();

    expect(element.querySelector('[data-action="create-directory"]')).toBeNull();
    expect(element.querySelector('[data-action="rename"]')).toBeNull();
    expect(element.querySelector('[data-action="copy"]')).toBeNull();
    expect(element.querySelector('[data-action="move"]')).toBeNull();
    expect(element.querySelector('[data-action="trash"]')).toBeNull();
  });

  it('creates and renames with one validated name and provider-relative addresses', async () => {
    const fixture = await create();
    const element = fixture.nativeElement as HTMLElement;

    element.querySelector<HTMLButtonElement>('[data-action="create-directory"]')?.click();
    await fixture.whenStable();
    const createName = element.querySelector<HTMLInputElement>('.fbdialog input');
    if (!createName) throw new Error('Missing create name input.');
    createName.value = 'Ideas';
    createName.dispatchEvent(new Event('input', { bubbles: true }));
    element.querySelector<HTMLButtonElement>('.fbdialog [data-action="submit"]')?.click();
    await fixture.whenStable();
    expect(bridge.createDirectory).toHaveBeenCalledWith({
      mountId: 'documents',
      parentPath: '',
      name: 'Ideas',
    });

    fixture.componentInstance.dialog.set(null);
    select(element, 'notes.md');
    await fixture.whenStable();
    element.querySelector<HTMLButtonElement>('[data-action="rename"]')?.click();
    await fixture.whenStable();
    const renameName = element.querySelector<HTMLInputElement>('.fbdialog input');
    if (!renameName) throw new Error('Missing rename name input.');
    renameName.value = 'ideas.md';
    renameName.dispatchEvent(new Event('input', { bubbles: true }));
    element.querySelector<HTMLButtonElement>('.fbdialog [data-action="submit"]')?.click();
    await fixture.whenStable();
    expect(bridge.rename).toHaveBeenCalledWith({
      mountId: 'documents',
      path: 'notes.md',
      name: 'ideas.md',
    });
  });

  it('copies and moves to explicit destination addresses without overwrite flags', async () => {
    const fixture = await create();

    fixture.componentInstance.handleIntent({
      type: 'submit-operation',
      operation: 'copy',
      source: { mountId: 'documents', path: 'notes.md' },
      destination: { mountId: 'documents', path: 'Archive/notes.md' },
      recursive: false,
      confirmed: false,
    });
    await fixture.whenStable();
    expect(bridge.copy).toHaveBeenCalledWith({
      source: { mountId: 'documents', path: 'notes.md' },
      destination: { mountId: 'documents', path: 'Archive/notes.md' },
    });
    expect(bridge.copy.mock.calls[0][0]).not.toHaveProperty('overwrite');

    fixture.componentInstance.handleIntent({
      type: 'submit-operation',
      operation: 'move',
      source: { mountId: 'documents', path: 'notes.md' },
      destination: { mountId: 'documents', path: 'Archive/notes.md' },
      recursive: false,
      confirmed: false,
    });
    await fixture.whenStable();
    expect(bridge.move).toHaveBeenCalledWith({
      source: { mountId: 'documents', path: 'notes.md' },
      destination: { mountId: 'documents', path: 'Archive/notes.md' },
    });
  });

  it('confirms Trash and supports Restore and permanent Delete from Trash', async () => {
    const fixture = await create();
    const element = fixture.nativeElement as HTMLElement;
    select(element, 'notes.md');
    await fixture.whenStable();
    element.querySelector<HTMLButtonElement>('[data-action="trash"]')?.click();
    await fixture.whenStable();
    expect(bridge.trash).not.toHaveBeenCalled();
    element.querySelector<HTMLButtonElement>('.fbdialog [data-action="confirm"]')?.click();
    await fixture.whenStable();
    expect(bridge.trash).toHaveBeenCalledWith({
      mountId: 'documents',
      path: 'notes.md',
    });

    await fixture.componentInstance.navigateToken('trash');
    await fixture.whenStable();
    select(element, 'notes.md');
    await fixture.whenStable();
    element.querySelector<HTMLButtonElement>('[data-action="restore"]')?.click();
    await fixture.whenStable();
    element.querySelector<HTMLButtonElement>('.fbdialog [data-action="confirm"]')?.click();
    await fixture.whenStable();
    expect(bridge.restore).toHaveBeenCalledWith({ receiptId: 'receipt-1' });

    fixture.componentInstance.dialog.set(null);
    element.querySelector<HTMLButtonElement>('[data-action="delete"]')?.click();
    await fixture.whenStable();
    element.querySelector<HTMLButtonElement>('.fbdialog [data-action="confirm"]')?.click();
    await fixture.whenStable();
    expect(bridge.delete).toHaveBeenCalledWith({
      mountId: '',
      path: '',
      receiptId: 'receipt-1',
      recursive: false,
      confirmed: true,
    });
  });

  it('requires a second explicit confirmation before recursive permanent deletion', async () => {
    bridge.listTrash.mockResolvedValue({
      entries: [
        {
          receiptId: 'receipt-folder',
          mountId: 'documents',
          originalPath: 'Archive',
          name: 'Archive',
          kind: 'directory',
          sizeBytes: 0,
          trashedAt: '2026-07-26T12:00:00Z',
          available: true,
          errorCode: '',
        },
      ],
      refreshedAt: '2026-07-26T12:00:00Z',
    });
    const fixture = await create({ ...filesWin, sub: 'trash' });
    const element = fixture.nativeElement as HTMLElement;
    select(element, 'Archive');
    await fixture.whenStable();
    element.querySelector<HTMLButtonElement>('[data-action="delete"]')?.click();
    await fixture.whenStable();

    element.querySelector<HTMLButtonElement>('.fbdialog [data-action="confirm"]')?.click();
    await fixture.whenStable();
    expect(bridge.delete).not.toHaveBeenCalled();
    expect(element.textContent).toContain('cannot be undone');

    element.querySelector<HTMLButtonElement>('.fbdialog [data-action="confirm"]')?.click();
    await fixture.whenStable();
    expect(bridge.delete).toHaveBeenCalledWith({
      mountId: '',
      path: '',
      receiptId: 'receipt-folder',
      recursive: true,
      confirmed: true,
    });
  });

  it('keeps conflict and partial feedback visible and refreshes affected data', async () => {
    const fixture = await create();
    bridge.copy.mockResolvedValue(
      operationResult('copy', {
        status: 'conflict',
        code: 'files.conflict',
        destination: { mountId: 'documents', path: 'notes-copy.md' },
        affected: [],
        conflict: {
          source: { mountId: 'documents', path: 'notes.md' },
          destination: { mountId: 'documents', path: 'notes-copy.md' },
          kind: 'file',
        },
        message: 'Destination already exists',
      }),
    );
    fixture.componentInstance.handleIntent({
      type: 'submit-operation',
      operation: 'copy',
      source: { mountId: 'documents', path: 'notes.md' },
      destination: { mountId: 'documents', path: 'notes-copy.md' },
      recursive: false,
      confirmed: false,
    });
    await fixture.whenStable();
    expect((fixture.nativeElement as HTMLElement).textContent).toContain(
      'Destination already exists',
    );

    bridge.move.mockResolvedValue(
      operationResult('move', {
        status: 'partial',
        code: 'files.partial_move',
        message: 'Copy completed but source removal failed',
        affected: [
          { mountId: 'documents', path: 'notes.md' },
          { mountId: 'documents', path: 'Archive/notes.md' },
        ],
      }),
    );
    fixture.componentInstance.handleIntent({
      type: 'submit-operation',
      operation: 'move',
      source: { mountId: 'documents', path: 'notes.md' },
      destination: { mountId: 'documents', path: 'Archive/notes.md' },
      recursive: false,
      confirmed: false,
    });
    await fixture.whenStable();
    expect((fixture.nativeElement as HTMLElement).textContent).toContain(
      'Copy completed but source removal failed',
    );
    expect(bridge.listDirectory).toHaveBeenCalledTimes(2);
  });

  it('shows provider errors without substituting demo content', async () => {
    const fixture = await create();
    bridge.rename.mockRejectedValue(new Error('provider unavailable'));

    fixture.componentInstance.handleIntent({
      type: 'submit-operation',
      operation: 'rename',
      source: { mountId: 'documents', path: 'notes.md' },
      name: 'ideas.md',
      recursive: false,
      confirmed: false,
    });
    await fixture.whenStable();
    const text = (fixture.nativeElement as HTMLElement).textContent ?? '';
    expect(text).toContain('provider unavailable');
    expect(text).not.toContain('welcome.txt');
  });

  it('subscribes once, coalesces event/local refresh, filters unknown mounts, and unsubscribes', async () => {
    const fixture = await create();
    expect(bridge.onChanged).toHaveBeenCalledTimes(1);
    const initialCalls = bridge.listDirectory.mock.calls.length;

    changedHandler?.({
      operation: 'rename',
      operationId: 'files-external',
      mountIds: ['missing'],
      paths: ['notes.md'],
      at: '2026-07-26T12:00:00Z',
    });
    await fixture.whenStable();
    expect(bridge.listDirectory).toHaveBeenCalledTimes(initialCalls);

    await fixture.componentInstance.navigateToken('documents::Invoices');
    await fixture.whenStable();
    const nestedCalls = bridge.listDirectory.mock.calls.length;
    changedHandler?.({
      operation: 'rename',
      operationId: 'files-unrelated',
      mountIds: ['documents'],
      paths: ['Archive/unrelated.txt'],
      at: '2026-07-26T12:00:00Z',
    });
    await fixture.whenStable();
    expect(bridge.listDirectory).toHaveBeenCalledTimes(nestedCalls);

    changedHandler?.({
      operation: 'rename',
      operationId: 'files-external',
      mountIds: ['documents'],
      paths: ['Invoices/notes.md'],
      at: '2026-07-26T12:00:00Z',
    });
    changedHandler?.({
      operation: 'rename',
      operationId: 'files-external-2',
      mountIds: ['documents'],
      paths: ['Invoices/notes.md'],
      at: '2026-07-26T12:00:00Z',
    });
    await fixture.whenStable();
    expect(bridge.listDirectory).toHaveBeenCalledTimes(nestedCalls + 1);

    fixture.destroy();
    expect(offEvents).toHaveBeenCalledTimes(1);
  });

  it('coalesces an event emitted during a successful local operation into one refresh', async () => {
    const fixture = await create();
    const initialCalls = bridge.listDirectory.mock.calls.length;
    bridge.rename.mockImplementation(async () => {
      changedHandler?.({
        operation: 'rename',
        operationId: 'files-rename',
        mountIds: ['documents'],
        paths: ['notes.md', 'ideas.md'],
        at: '2026-07-26T12:00:00Z',
      });
      return operationResult('rename');
    });

    fixture.componentInstance.handleIntent({
      type: 'submit-operation',
      operation: 'rename',
      source: { mountId: 'documents', path: 'notes.md' },
      name: 'ideas.md',
      recursive: false,
      confirmed: false,
    });
    await fixture.whenStable();

    expect(bridge.listDirectory).toHaveBeenCalledTimes(initialCalls + 1);
  });

  it('performs safe mutations only in its isolated demo store while offline', async () => {
    offline.set(true);
    const fixture = await create({ ...filesWin, sub: 'documents' });

    fixture.componentInstance.handleIntent({
      type: 'submit-operation',
      operation: 'create-directory',
      source: { mountId: 'documents', path: '' },
      name: 'Ideas',
      recursive: false,
      confirmed: false,
    });
    await fixture.whenStable();

    expect((fixture.nativeElement as HTMLElement).textContent).toContain('Ideas');
    expect(bridge.createDirectory).not.toHaveBeenCalled();
    expect(bridge.onChanged).not.toHaveBeenCalled();
  });
});

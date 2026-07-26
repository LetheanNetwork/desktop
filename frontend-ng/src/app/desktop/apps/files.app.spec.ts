import { signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
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
  const bridge = {
    listMounts: vi.fn(),
    listDirectory: vi.fn(),
    preview: vi.fn(),
    listTrash: vi.fn(),
    onChanged: vi.fn(() => vi.fn()),
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

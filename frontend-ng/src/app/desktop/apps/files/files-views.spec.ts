import type { Type } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import type {
  FilePreviewView,
  FilesActionIntent,
  FilesOperationDialogView,
  FilesViewState,
} from './files-view.models';
import { FilesBrowserView } from './files-browser.view';
import { FilesOperationDialogViewComponent } from './files-operation-dialog.view';
import { FilesPreviewView } from './files-preview.view';
import { FilesSidebarView } from './files-sidebar.view';
import { FilesStatusView } from './files-status.view';
import { FilesToolbarView } from './files-toolbar.view';

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

function viewState(overrides: Partial<FilesViewState> = {}): FilesViewState {
  return {
    location: { kind: 'directory', mountId: 'documents', path: '' },
    token: 'documents',
    dataState: 'demo',
    viewMode: 'grid',
    catalogue: {
      mounts: [
        {
          id: 'documents',
          name: 'Documents',
          kind: 'memory',
          icon: 'folder',
          brand: false,
          capabilities,
          capacity: {
            freeBytes: 218 * 1024 ** 3,
            totalBytes: 512 * 1024 ** 3,
          },
        },
        {
          id: 'lethernet',
          name: 'LetherNet',
          kind: 'memory',
          icon: 'network-wired',
          brand: true,
          capabilities: { ...capabilities, write: false },
        },
      ],
      favourites: [],
      recent: [],
    },
    activeMount: {
      id: 'documents',
      name: 'Documents',
      kind: 'memory',
      icon: 'folder',
      brand: false,
      capabilities,
      capacity: {
        freeBytes: 218 * 1024 ** 3,
        totalBytes: 512 * 1024 ** 3,
      },
    },
    entries: [
      {
        mountId: 'documents',
        name: 'Invoices',
        relativePath: 'Invoices',
        kind: 'directory',
        sizeBytes: 0,
        modifiedAt: '',
        mode: 493,
        hidden: false,
        icon: 'folder',
        detail: '',
        receiptId: '',
        available: true,
      },
      {
        mountId: 'documents',
        name: 'notes.md',
        relativePath: 'notes.md',
        kind: 'file',
        sizeBytes: 2048,
        modifiedAt: 'Today',
        mode: 420,
        hidden: false,
        icon: 'file-code',
        detail: '2 KB',
        receiptId: '',
        available: true,
      },
    ],
    breadcrumbs: [
      { label: 'Home', token: 'home' },
      { label: 'Documents', token: 'documents' },
    ],
    upToken: 'home',
    providerLabel: 'Documents',
    capacityLabel: '218 GB free of 512 GB',
    itemCount: 2,
    folderCount: 1,
    fileCount: 1,
    emptyLabel: 'This folder is empty',
    capabilities,
    refreshedAt: '2026-07-26T12:00:00Z',
    ...overrides,
  };
}

async function render<T>(component: Type<T>, inputs: Record<string, unknown>) {
  const fixture = TestBed.createComponent(component);
  for (const [name, value] of Object.entries(inputs)) {
    fixture.componentRef.setInput(name, value);
  }
  await fixture.whenStable();
  return fixture;
}

describe('Files presentation views', () => {
  afterEach(() => TestBed.resetTestingModule());

  it('renders sidebar groups and emits provider-neutral navigation tokens', async () => {
    const fixture = await render(FilesSidebarView, { state: viewState() });
    const intents: FilesActionIntent[] = [];
    fixture.componentInstance.intent.subscribe((intent) => intents.push(intent));
    const element = fixture.nativeElement as HTMLElement;

    expect(element.textContent).toContain('Favourites');
    expect(element.textContent).toContain('Locations');
    expect(element.textContent).toContain('Home');
    expect(element.textContent).toContain('LetherNet');
    expect(element.textContent).toContain('Trash');
    expect(element.querySelector('[data-token="documents"]')?.getAttribute('aria-current')).toBe(
      'page',
    );

    element.querySelector<HTMLButtonElement>('[data-token="lethernet"]')?.click();
    expect(intents).toEqual([{ type: 'navigate', token: 'lethernet' }]);
  });

  it('emits Up, Home, breadcrumb, Refresh, grid, and list toolbar intents', async () => {
    const fixture = await render(FilesToolbarView, { state: viewState() });
    const intents: FilesActionIntent[] = [];
    fixture.componentInstance.intent.subscribe((intent) => intents.push(intent));
    const element = fixture.nativeElement as HTMLElement;

    for (const action of ['up', 'home', 'refresh', 'grid', 'list']) {
      element.querySelector<HTMLButtonElement>(`[data-action="${action}"]`)?.click();
    }
    element.querySelector<HTMLButtonElement>('[data-token="documents"]')?.click();

    expect(intents).toEqual([
      { type: 'up' },
      { type: 'home' },
      { type: 'refresh' },
      { type: 'set-view', view: 'grid' },
      { type: 'set-view', view: 'list' },
      { type: 'navigate', token: 'documents' },
    ]);
  });

  it('renders empty, grid, and list states and emits selection/open intents', async () => {
    const empty = await render(FilesBrowserView, {
      state: viewState({ entries: [], itemCount: 0, folderCount: 0, fileCount: 0 }),
      selectedKey: '',
    });
    expect((empty.nativeElement as HTMLElement).textContent).toContain('This folder is empty');
    empty.destroy();

    const fixture = await render(FilesBrowserView, {
      state: viewState(),
      selectedKey: '',
    });
    const intents: FilesActionIntent[] = [];
    fixture.componentInstance.intent.subscribe((intent) => intents.push(intent));
    const directory = (fixture.nativeElement as HTMLElement).querySelector<HTMLButtonElement>(
      '[data-path="Invoices"]',
    );
    directory?.click();
    directory?.dispatchEvent(new MouseEvent('dblclick', { bubbles: true }));
    directory?.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true }));

    expect(intents).toContainEqual({
      type: 'select-entry',
      mountId: 'documents',
      path: 'Invoices',
      receiptId: '',
    });
    expect(intents.filter(({ type }) => type === 'open-directory')).toHaveLength(2);

    fixture.componentRef.setInput('state', viewState({ viewMode: 'list' }));
    await fixture.whenStable();
    expect((fixture.nativeElement as HTMLElement).querySelector('.fblist')).not.toBeNull();
  });

  it('emits file preview and never offers open for unsupported entries', async () => {
    const unsupported = {
      ...viewState().entries[1],
      name: 'socket',
      relativePath: 'socket',
      kind: 'other' as const,
    };
    const fixture = await render(FilesBrowserView, {
      state: viewState({ entries: [viewState().entries[1], unsupported] }),
      selectedKey: '',
    });
    const intents: FilesActionIntent[] = [];
    fixture.componentInstance.intent.subscribe((intent) => intents.push(intent));
    const element = fixture.nativeElement as HTMLElement;

    element
      .querySelector<HTMLButtonElement>('[data-path="notes.md"]')
      ?.dispatchEvent(new MouseEvent('dblclick', { bubbles: true }));
    element
      .querySelector<HTMLButtonElement>('[data-path="socket"]')
      ?.dispatchEvent(new MouseEvent('dblclick', { bubbles: true }));

    expect(intents).toContainEqual({
      type: 'preview',
      mountId: 'documents',
      path: 'notes.md',
    });
    expect(intents.some((intent) => 'path' in intent && intent.path === 'socket')).toBe(false);
  });

  it('shows data state, counts, provider, and optional capacity', async () => {
    const fixture = await render(FilesStatusView, { state: viewState() });
    const text = (fixture.nativeElement as HTMLElement).textContent ?? '';

    expect(text).toContain('Demo data');
    expect(text).toContain('2 items');
    expect(text).toContain('1 folder');
    expect(text).toContain('1 file');
    expect(text).toContain('Documents');
    expect(text).toContain('218 GB free of 512 GB');
  });

  it('renders escaped bounded text and binary preview metadata', async () => {
    const textPreview: FilePreviewView = {
      mountId: 'documents',
      relativePath: 'notes.md',
      name: 'notes.md',
      content: '<img src=x onerror=alert(1)>',
      mime: 'text/markdown',
      bytesRead: 32,
      sizeBytes: 32,
      lines: 1,
      truncated: false,
      binary: false,
    };
    const fixture = await render(FilesPreviewView, { preview: textPreview });
    const element = fixture.nativeElement as HTMLElement;

    expect(element.textContent).toContain('<img src=x onerror=alert(1)>');
    expect(element.querySelector('img')).toBeNull();

    fixture.componentRef.setInput('preview', {
      ...textPreview,
      content: '',
      mime: 'image/png',
      binary: true,
      lines: 0,
    });
    await fixture.whenStable();
    expect(element.textContent).toContain('Binary preview');
    expect(element.textContent).toContain('image/png');
  });

  it.each([
    ['form', 'Create folder'],
    ['confirm', 'Move this item to Trash?'],
    ['busy', 'Working…'],
    ['conflict', 'Destination already exists'],
    ['partial', 'Part of the operation completed'],
    ['success', 'Folder created'],
    ['error', 'The provider is unavailable'],
  ] as const)('renders the %s operation dialog state', async (state, message) => {
    const dialog: FilesOperationDialogView = {
      state,
      operation: 'create-directory',
      title: 'Create folder',
      message,
    };
    const fixture = await render(FilesOperationDialogViewComponent, { dialog });
    expect((fixture.nativeElement as HTMLElement).textContent).toContain(message);
  });

  it('emits a typed confirmation from an operation dialog', async () => {
    const dialog: FilesOperationDialogView = {
      state: 'confirm',
      operation: 'trash',
      title: 'Move to Trash',
      message: 'Move this item to Trash?',
      source: { mountId: 'documents', path: 'notes.md' },
    };
    const fixture = await render(FilesOperationDialogViewComponent, { dialog });
    const intents: FilesActionIntent[] = [];
    fixture.componentInstance.intent.subscribe((intent) => intents.push(intent));

    (fixture.nativeElement as HTMLElement)
      .querySelector<HTMLButtonElement>('[data-action="confirm"]')
      ?.click();

    expect(intents).toEqual([
      {
        type: 'submit-operation',
        operation: 'trash',
        source: { mountId: 'documents', path: 'notes.md' },
        recursive: false,
        confirmed: true,
      },
    ]);
  });
});

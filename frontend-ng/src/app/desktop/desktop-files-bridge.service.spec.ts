import { signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { ConnectionManagerService } from '../connection-manager.service';
import type {
  CreateDirectoryInput,
  DeleteInput,
  RenameInput,
  RestoreInput,
  TrashInput,
  TransferInput,
} from './apps/files/files-view.models';
import {
  DesktopFilesBridgeService,
  FILES_EVENT_SOURCE,
  type FilesEventSource,
} from './desktop-files-bridge.service';
import { SurfaceBridgeService } from './surfaces/surface-bridge.service';

const capabilities = {
  list: true,
  preview: true,
  open: true,
  reveal: true,
  createDirectory: true,
  write: true,
  rename: true,
  copyFrom: true,
  copyTo: true,
  move: true,
  trash: true,
  restore: true,
  delete: true,
};

function mountWireFixture() {
  return {
    id: 'documents',
    name: 'Documents',
    kind: 'local',
    icon: 'folder',
    brand: false,
    capabilities,
    capacity: { freeBytes: 218, totalBytes: 512 },
  };
}

function catalogueWireFixture() {
  return {
    mounts: [mountWireFixture()],
    favourites: [{ mountId: 'documents', path: 'Invoices' }],
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
}

function directoryWireFixture() {
  return {
    mount: mountWireFixture(),
    path: 'Invoices',
    breadcrumbs: [{ name: 'Invoices', path: 'Invoices' }],
    entries: [
      {
        name: 'receipt.txt',
        relativePath: 'Invoices/receipt.txt',
        kind: 'file',
        sizeBytes: 42,
        modifiedAt: '2026-07-26T09:14:00Z',
        mode: 420,
        hidden: false,
      },
    ],
    nextCursor: '',
    totalKnown: 1,
    refreshedAt: '2026-07-26T12:00:00Z',
  };
}

function operationWireFixture() {
  return {
    operationId: 'files-1',
    operation: 'rename',
    status: 'completed',
    code: '',
    source: { mountId: 'documents', path: 'draft.txt' },
    destination: { mountId: 'documents', path: 'final.txt' },
    affected: [
      { mountId: 'documents', path: 'draft.txt' },
      { mountId: 'documents', path: 'final.txt' },
    ],
    message: 'Item renamed.',
    receiptId: '',
  };
}

describe('DesktopFilesBridgeService', () => {
  const offline = signal(false);
  const surface = {
    call: vi.fn(),
  };
  const eventHandlers = new Map<string, (payload: unknown) => void>();
  const events: FilesEventSource = {
    on: vi.fn((name, handler) => {
      eventHandlers.set(name, handler);
      return vi.fn(() => eventHandlers.delete(name));
    }),
  };
  let service: DesktopFilesBridgeService;

  beforeEach(() => {
    offline.set(false);
    eventHandlers.clear();
    vi.clearAllMocks();
    TestBed.configureTestingModule({
      providers: [
        DesktopFilesBridgeService,
        {
          provide: ConnectionManagerService,
          useValue: { offline: offline.asReadonly() },
        },
        { provide: SurfaceBridgeService, useValue: surface },
        { provide: FILES_EVENT_SOURCE, useValue: events },
      ],
    });
    service = TestBed.inject(DesktopFilesBridgeService);
  });

  afterEach(() => TestBed.resetTestingModule());

  it('calls the provider-neutral list method with mount and relative path', async () => {
    surface.call.mockResolvedValue(directoryWireFixture());

    await expect(
      service.listDirectory({
        mountId: 'documents',
        path: 'Invoices',
        cursor: '',
        limit: 200,
      }),
    ).resolves.toMatchObject({ path: 'Invoices' });

    expect(surface.call).toHaveBeenCalledWith(
      'dappco.re/lthn/desktop/pkg/office/files.Service.ListDirectory',
      [{ mountId: 'documents', path: 'Invoices', cursor: '', limit: 200 }],
    );
  });

  it('parses the complete mount catalogue without defaulting capabilities', async () => {
    surface.call.mockResolvedValue(catalogueWireFixture());

    await expect(service.listMounts()).resolves.toEqual(catalogueWireFixture());
    expect(surface.call).toHaveBeenCalledWith(
      'dappco.re/lthn/desktop/pkg/office/files.Service.ListMounts',
    );

    surface.call.mockResolvedValue({
      ...catalogueWireFixture(),
      mounts: [
        {
          ...mountWireFixture(),
          capabilities: { ...capabilities, preview: undefined },
        },
      ],
    });
    await expect(service.listMounts()).rejects.toThrow('invalid Files response');
  });

  it('rejects absolute, traversal, and provider-leaking responses recursively', async () => {
    for (const payload of [
      { ...directoryWireFixture(), path: '/Users/sarah/Documents' },
      { ...directoryWireFixture(), path: '../Documents' },
      { ...directoryWireFixture(), root: '/Users/sarah' },
      { ...directoryWireFixture(), nested: { endpoint: 's3://private-bucket' } },
      {
        ...directoryWireFixture(),
        entries: [
          {
            ...directoryWireFixture().entries[0],
            absolutePath: '/Users/sarah/Documents/receipt.txt',
          },
        ],
      },
    ]) {
      surface.call.mockResolvedValueOnce(payload);
      await expect(
        service.listDirectory({
          mountId: 'documents',
          path: '',
          cursor: '',
          limit: 200,
        }),
      ).rejects.toThrow('invalid Files response');
    }
  });

  it('parses bounded preview and rejects invalid sizes and enums', async () => {
    surface.call.mockResolvedValue({
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
    await expect(
      service.preview({ mountId: 'documents', path: 'notes.md' }),
    ).resolves.toMatchObject({ content: '# Notes', binary: false });

    surface.call.mockResolvedValue({
      ...directoryWireFixture(),
      entries: [{ ...directoryWireFixture().entries[0], sizeBytes: -1 }],
    });
    await expect(
      service.listDirectory({
        mountId: 'documents',
        path: '',
        cursor: '',
        limit: 200,
      }),
    ).rejects.toThrow('invalid Files response');

    surface.call.mockResolvedValue({
      ...directoryWireFixture(),
      entries: [{ ...directoryWireFixture().entries[0], kind: 'socket' }],
    });
    await expect(
      service.listDirectory({
        mountId: 'documents',
        path: '',
        cursor: '',
        limit: 200,
      }),
    ).rejects.toThrow('invalid Files response');
  });

  it('opens and reveals only a validated opaque Files address', async () => {
    surface.call.mockResolvedValue(undefined);

    await expect(
      service.open({ mountId: 'documents', path: 'Invoices/receipt.txt' }),
    ).resolves.toBeUndefined();
    await expect(
      service.reveal({ mountId: 'documents', path: 'Invoices/receipt.txt' }),
    ).resolves.toBeUndefined();

    expect(surface.call).toHaveBeenNthCalledWith(
      1,
      'dappco.re/lthn/desktop/pkg/office/files.Service.Open',
      [{ mountId: 'documents', path: 'Invoices/receipt.txt' }],
    );
    expect(surface.call).toHaveBeenNthCalledWith(
      2,
      'dappco.re/lthn/desktop/pkg/office/files.Service.Reveal',
      [{ mountId: 'documents', path: 'Invoices/receipt.txt' }],
    );
  });

  it('rejects unsafe host-action addresses before making a Wails call', async () => {
    await expect(service.open({ mountId: 'documents', path: '../private.txt' })).rejects.toThrow();
    await expect(
      service.reveal({ mountId: '/Users/sarah', path: 'private.txt' }),
    ).rejects.toThrow();

    expect(surface.call).not.toHaveBeenCalled();
  });

  it('rejects unexpected host-action response data', async () => {
    surface.call.mockResolvedValue({ absolutePath: '/Users/sarah/Documents/receipt.txt' });

    await expect(
      service.open({ mountId: 'documents', path: 'Invoices/receipt.txt' }),
    ).rejects.toThrow('invalid Files response');
  });

  it.each([
    ['createDirectory', { mountId: 'documents', parentPath: '', name: 'Ideas' }, 'CreateDirectory'],
    ['rename', { mountId: 'documents', path: 'draft.txt', name: 'final.txt' }, 'Rename'],
    [
      'copy',
      {
        source: { mountId: 'documents', path: 'draft.txt' },
        destination: { mountId: 'documents', path: 'copy.txt' },
      },
      'Copy',
    ],
    [
      'move',
      {
        source: { mountId: 'documents', path: 'draft.txt' },
        destination: { mountId: 'documents', path: 'final.txt' },
      },
      'Move',
    ],
    ['trash', { mountId: 'documents', path: 'draft.txt' }, 'Trash'],
    ['restore', { receiptId: 'receipt-1' }, 'Restore'],
    [
      'delete',
      {
        mountId: 'documents',
        path: 'drafts',
        receiptId: '',
        recursive: true,
        confirmed: true,
      },
      'Delete',
    ],
  ] as const)('calls and parses %s', async (method, input, wireMethod) => {
    surface.call.mockResolvedValue(operationWireFixture());

    await expect(
      (
        service[method] as (
          value:
            | CreateDirectoryInput
            | RenameInput
            | TransferInput
            | TrashInput
            | RestoreInput
            | DeleteInput,
        ) => Promise<unknown>
      )(input),
    ).resolves.toMatchObject({
      operationId: 'files-1',
      status: 'completed',
    });
    expect(surface.call).toHaveBeenCalledWith(
      `dappco.re/lthn/desktop/pkg/office/files.Service.${wireMethod}`,
      [input],
    );
  });

  it('parses Trash and rejects malformed operation/result envelopes', async () => {
    surface.call.mockResolvedValue({
      entries: [
        {
          receiptId: 'receipt-1',
          mountId: 'documents',
          originalPath: 'draft.txt',
          name: 'draft.txt',
          kind: 'file',
          sizeBytes: 12,
          trashedAt: '2026-07-26T12:00:00Z',
          available: true,
          errorCode: '',
        },
      ],
      refreshedAt: '2026-07-26T12:00:00Z',
    });
    await expect(service.listTrash()).resolves.toMatchObject({
      entries: [{ receiptId: 'receipt-1' }],
    });

    for (const payload of [
      { OK: true, Value: operationWireFixture() },
      { ...operationWireFixture(), status: 'done' },
      { ...operationWireFixture(), affected: 'documents' },
      { ...operationWireFixture(), code: 'provider.secret' },
    ]) {
      surface.call.mockResolvedValueOnce(payload);
      await expect(
        service.rename({
          mountId: 'documents',
          path: 'draft.txt',
          name: 'final.txt',
        }),
      ).rejects.toThrow('invalid Files response');
    }
  });

  it('subscribes to exactly the typed Files event and unsubscribes', () => {
    const handler = vi.fn();
    const off = service.onChanged(handler);

    expect(events.on).toHaveBeenCalledWith('lthn:files:changed', expect.any(Function));
    eventHandlers.get('lthn:files:changed')?.({
      operation: 'rename',
      operationId: 'files-1',
      mountIds: ['documents'],
      paths: ['draft.txt', 'final.txt'],
      at: '2026-07-26T12:00:00Z',
    });
    expect(handler).toHaveBeenCalledWith({
      operation: 'rename',
      operationId: 'files-1',
      mountIds: ['documents'],
      paths: ['draft.txt', 'final.txt'],
      at: '2026-07-26T12:00:00Z',
    });

    eventHandlers.get('lthn:files:changed')?.({
      operation: 'rename',
      operationId: 'files-2',
      mountIds: ['/Users/sarah'],
      paths: ['draft.txt'],
      at: '2026-07-26T12:00:00Z',
    });
    expect(handler).toHaveBeenCalledTimes(1);

    off();
    expect(eventHandlers.has('lthn:files:changed')).toBe(false);
  });

  it('makes no Wails call or event registration while explicitly offline', async () => {
    offline.set(true);

    await expect(service.listMounts()).rejects.toThrow('offline demo mode');
    await expect(service.open({ mountId: 'documents', path: 'notes.md' })).rejects.toThrow(
      'offline demo mode',
    );
    await expect(service.reveal({ mountId: 'documents', path: 'notes.md' })).rejects.toThrow(
      'offline demo mode',
    );
    const off = service.onChanged(vi.fn());

    expect(surface.call).not.toHaveBeenCalled();
    expect(events.on).not.toHaveBeenCalled();
    expect(off).toBeTypeOf('function');
    expect(() => off()).not.toThrow();
  });
});

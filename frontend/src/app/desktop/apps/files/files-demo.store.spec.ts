import type { ListDirectoryInput } from './files-view.models';
import { FilesDemoStore } from './files-demo.store';

function directoryInput(mountId: string, path = ''): ListDirectoryInput {
  return { mountId, path, cursor: '', limit: 200 };
}

describe('FilesDemoStore', () => {
  it('retains the complete nested design fixture', async () => {
    const store = new FilesDemoStore();

    const home = await store.listHome();
    const documents = await store.listDirectory(directoryInput('documents'));
    const invoices = await store.listDirectory(directoryInput('documents', 'Invoices'));
    const projects = await store.listDirectory(directoryInput('projects'));
    const lethean = await store.listDirectory(directoryInput('projects', 'lethean'));

    expect(home.entries.map(({ name }) => name)).toContain('welcome.txt');
    expect(documents.entries.map(({ name }) => name)).toContain('whitepaper.pdf');
    expect(invoices.entries).toEqual([]);
    expect(projects.entries.map(({ name }) => name)).toEqual(['lethean', 'core-ide']);
    expect(lethean.entries.map(({ name }) => name)).toContain('desktop.component.ts');
  });

  it('exposes the complete mount catalogue without implying a network provider', async () => {
    const catalogue = await new FilesDemoStore().listMounts();

    expect(catalogue.mounts.map(({ id }) => id)).toEqual([
      'documents',
      'downloads',
      'models',
      'projects',
      'lethernet',
    ]);
    expect(catalogue.mounts.find(({ id }) => id === 'lethernet')).toMatchObject({
      kind: 'memory',
      icon: 'network-wired',
    });
    expect(JSON.stringify(catalogue)).not.toMatch(/credential|endpoint|absolutePath|secret/i);
  });

  it('mutates only its own in-memory catalogue', async () => {
    const first = new FilesDemoStore();
    const second = new FilesDemoStore();

    await expect(
      first.createDirectory({
        mountId: 'documents',
        parentPath: '',
        name: 'Ideas',
      }),
    ).resolves.toMatchObject({ status: 'completed' });

    expect(
      (await first.listDirectory(directoryInput('documents'))).entries.map(({ name }) => name),
    ).toContain('Ideas');
    expect(
      (await second.listDirectory(directoryInput('documents'))).entries.map(({ name }) => name),
    ).not.toContain('Ideas');
  });

  it('returns non-destructive conflicts and supports trash lifecycle', async () => {
    const store = new FilesDemoStore();

    await expect(
      store.rename({
        mountId: 'documents',
        path: 'roadmap.md',
        name: 'whitepaper.pdf',
      }),
    ).resolves.toMatchObject({
      status: 'conflict',
      conflict: {
        source: { mountId: 'documents', path: 'roadmap.md' },
        destination: { mountId: 'documents', path: 'whitepaper.pdf' },
      },
    });

    const trashed = await store.trash({
      mountId: 'documents',
      path: 'meeting.txt',
    });
    expect(trashed.status).toBe('completed');
    const trash = await store.listTrash();
    expect(trash.entries).toHaveLength(1);
    expect(trash.entries[0].name).toBe('meeting.txt');

    await expect(store.restore({ receiptId: trash.entries[0].receiptId })).resolves.toMatchObject({
      status: 'completed',
    });
    expect(
      (await store.listDirectory(directoryInput('documents'))).entries.map(({ name }) => name),
    ).toContain('meeting.txt');
  });

  it('uses monotonic operation ids and validates provider-relative paths', async () => {
    const store = new FilesDemoStore();
    const first = await store.createDirectory({
      mountId: 'documents',
      parentPath: '',
      name: 'Ideas',
    });
    const second = await store.rename({
      mountId: 'documents',
      path: 'Ideas',
      name: 'Notes',
    });

    expect(first.operationId).toBe('demo-files-1');
    expect(second.operationId).toBe('demo-files-2');
    await expect(store.listDirectory(directoryInput('documents', '../private'))).rejects.toThrow(
      'provider-relative',
    );
  });
});

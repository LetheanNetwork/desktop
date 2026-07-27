import { signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { ConnectionManagerService } from '../connection-manager.service';
import { SurfaceBridgeService } from './surfaces/surface-bridge.service';
import {
  TerminalWorkspace,
  terminalTabsFromWorkspace,
  terminalWorkspaceFromTabs,
} from './terminal-workspace.models';
import { TerminalWorkspaceService } from './terminal-workspace.service';

const workspace: TerminalWorkspace = {
  activeKey: 'terminal-one',
  tabs: [
    {
      key: 'terminal-one',
      title: 'desktop',
      kind: 'shell',
      workspace: {
        mountId: '',
        path: '',
        repository: 'desktop',
      },
      sharedAgentId: '',
    },
    {
      key: 'agent-one',
      title: 'OpenCode agent',
      kind: 'agent',
      workspace: {
        mountId: '',
        path: '',
        repository: '',
      },
      sharedAgentId: 'agent-session-one',
    },
  ],
};

const snapshot = {
  version: 1,
  revision: 4,
  updatedAt: '2026-07-27T14:00:00Z',
  workspace,
};

describe('TerminalWorkspaceService', () => {
  const offline = signal(false);
  const surface = {
    call: vi.fn(),
  };

  beforeEach(() => {
    vi.useFakeTimers();
    offline.set(false);
    vi.resetAllMocks();
    TestBed.configureTestingModule({
      providers: [
        TerminalWorkspaceService,
        {
          provide: ConnectionManagerService,
          useValue: { offline: offline.asReadonly() },
        },
        { provide: SurfaceBridgeService, useValue: surface },
      ],
    });
  });

  afterEach(() => {
    TestBed.resetTestingModule();
    vi.useRealTimers();
  });

  it('loads a validated connected workspace from the Medium-backed service', async () => {
    surface.call.mockResolvedValue(snapshot);
    const service = TestBed.inject(TerminalWorkspaceService);

    await expect(service.load()).resolves.toEqual(snapshot);
    expect(surface.call).toHaveBeenCalledWith(
      'dappco.re/lthn/desktop/pkg/desktopstate.WailsService.LoadTerminalWorkspace',
    );
  });

  it('debounces tab mutations into one bounded revision save', async () => {
    surface.call
      .mockResolvedValueOnce(snapshot)
      .mockResolvedValueOnce({ ...snapshot, revision: 5 });
    const service = TestBed.inject(TerminalWorkspaceService);
    await service.load();
    surface.call.mockClear();

    service.schedule({
      ...workspace,
      activeKey: 'agent-one',
    });
    service.schedule(workspace);
    await vi.advanceTimersByTimeAsync(249);
    expect(surface.call).not.toHaveBeenCalled();
    await vi.advanceTimersByTimeAsync(1);
    await service.flush();

    expect(surface.call).toHaveBeenCalledTimes(1);
    expect(surface.call).toHaveBeenCalledWith(
      'dappco.re/lthn/desktop/pkg/desktopstate.WailsService.SaveTerminalWorkspace',
      [{ expectedRevision: 4, workspace }],
    );
  });

  it('keeps an isolated in-memory document and makes no Wails call offline', async () => {
    offline.set(true);
    const service = TestBed.inject(TerminalWorkspaceService);

    const initial = await service.load();
    service.schedule(workspace);
    await vi.advanceTimersByTimeAsync(250);
    await service.flush();
    const reloaded = await service.load();

    expect(initial).toMatchObject({
      version: 1,
      revision: 0,
      workspace: { activeKey: '', tabs: [] },
    });
    expect(reloaded).toMatchObject({ revision: 1, workspace });
    expect(surface.call).not.toHaveBeenCalled();
  });

  it('rejects malformed and authority-bearing workspace documents', async () => {
    const service = TestBed.inject(TerminalWorkspaceService);
    for (const invalid of [
      { ...snapshot, revision: -1 },
      { ...snapshot, workspace: { ...workspace, command: ['/bin/sh'] } },
      {
        ...snapshot,
        workspace: {
          ...workspace,
          tabs: [{ ...workspace.tabs[0], cwd: '/Users/sarah' }],
        },
      },
      {
        ...snapshot,
        workspace: {
          ...workspace,
          tabs: [{ ...workspace.tabs[0], environment: ['TOKEN=secret'] }],
        },
      },
      {
        ...snapshot,
        workspace: {
          ...workspace,
          tabs: [
            {
              ...workspace.tabs[0],
              workspace: { mountId: 'documents', path: '../escape', repository: '' },
            },
          ],
        },
      },
    ]) {
      surface.call.mockResolvedValueOnce(invalid);
      await expect(service.load()).rejects.toThrow('invalid terminal workspace');
    }
  });
});

describe('Terminal workspace projections', () => {
  it('persists presentation intent without transient process or host authority', () => {
    const durable = terminalWorkspaceFromTabs(
      [
        {
          key: 'terminal-one',
          title: 'desktop',
          repo: 'desktop',
          cwd: '/Users/sarah/Code/lthn/desktop',
          attachId: 'transient-shell-id',
          command: ['/bin/sh', '-lc', 'secret'],
        },
        {
          key: 'agent-one',
          title: 'OpenCode agent',
          attachId: 'agent-session-one',
          shared: true,
        },
      ],
      'terminal-one',
    );

    expect(durable).toEqual(workspace);
    const encoded = JSON.stringify(durable);
    expect(encoded).not.toContain('/Users/');
    expect(encoded).not.toContain('transient-shell-id');
    expect(encoded).not.toContain('/bin/sh');
    expect(encoded).not.toContain('secret');
    expect(encoded).not.toContain('environment');
    expect(encoded).not.toContain('output');
  });

  it('restores shared tabs only when their trusted live session still exists', () => {
    expect(terminalTabsFromWorkspace(workspace, new Set(['agent-session-one']))).toEqual([
      expect.objectContaining({
        key: 'terminal-one',
        repo: 'desktop',
      }),
      expect.objectContaining({
        key: 'agent-one',
        attachId: 'agent-session-one',
        shared: true,
        exited: false,
      }),
    ]);

    expect(terminalTabsFromWorkspace(workspace, new Set())).toEqual([
      expect.objectContaining({
        key: 'terminal-one',
        repo: 'desktop',
      }),
      expect.objectContaining({
        key: 'agent-one',
        attachId: undefined,
        shared: true,
        exited: true,
      }),
    ]);
  });
});

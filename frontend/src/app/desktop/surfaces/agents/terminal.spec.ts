import { TestBed } from '@angular/core/testing';
import { Win } from '../../desktop.data';
import { TerminalWorkspaceSnapshot } from '../../terminal-workspace.models';
import { TerminalWorkspaceService } from '../../terminal-workspace.service';
import { SurfaceBridgeService } from '../surface-bridge.service';
import { AgentsTerminalSurface } from './terminal';

const terminalWin: Win = {
  id: 'w-terminal',
  app: 'surface-agents-terminal',
  sub: '',
  x: 0,
  y: 0,
  w: 920,
  h: 640,
  z: 1,
  min: false,
  max: false,
};

describe('AgentsTerminalSurface', () => {
  const bridge = {
    call: vi.fn(async () => ({
      sessions: [
        {
          id: 'agent-session-1',
          label: 'OpenCode agent',
          shell: 'opencode',
          kind: 'agent',
        },
      ],
    })),
  };
  const workspace = {
    error: () => '',
    isOffline: vi.fn(() => false),
    load: vi.fn<() => Promise<TerminalWorkspaceSnapshot>>(),
    schedule: vi.fn(),
    flush: vi.fn(async () => undefined),
  };

  beforeEach(() => {
    vi.resetAllMocks();
    workspace.isOffline.mockReturnValue(false);
    workspace.load.mockResolvedValue({
      version: 1,
      revision: 0,
      updatedAt: '',
      workspace: { activeKey: '', tabs: [] },
    });
    bridge.call.mockResolvedValue({
      sessions: [
        {
          id: 'agent-session-1',
          label: 'OpenCode agent',
          shell: 'opencode',
          kind: 'agent',
        },
      ],
    });
    TestBed.configureTestingModule({
      providers: [
        { provide: SurfaceBridgeService, useValue: bridge },
        { provide: TerminalWorkspaceService, useValue: workspace },
      ],
    });
    TestBed.overrideComponent(AgentsTerminalSurface, {
      set: { imports: [], template: '' },
    });
  });

  afterEach(() => TestBed.resetTestingModule());

  async function create() {
    const fixture = TestBed.createComponent(AgentsTerminalSurface);
    fixture.componentRef.setInput('win', terminalWin);
    fixture.detectChanges();
    await vi.waitFor(() => expect(workspace.load).toHaveBeenCalled());
    await vi.waitFor(() => expect(fixture.componentInstance.tabs().length).toBeGreaterThan(0));
    return fixture;
  }

  it('starts a local shell tab and discovers running agent sessions', async () => {
    const fixture = await create();
    const terminal = fixture.componentInstance;

    expect(terminal.tabs()).toEqual([
      expect.objectContaining({ title: 'shell 1' }),
      expect.objectContaining({
        attachId: 'agent-session-1',
        title: 'OpenCode agent',
        shared: true,
      }),
    ]);
    expect(terminal.activeKey()).toBe(terminal.tabs()[0].key);
    expect(bridge.call).toHaveBeenCalledWith('dappco.re/lthn/desktop/pkg/terminal.Service.List');
  });

  it('restores durable tab order, title, active key, and live shared agents', async () => {
    workspace.load.mockResolvedValue({
      version: 1,
      revision: 7,
      updatedAt: '2026-07-27T14:00:00Z',
      workspace: {
        activeKey: 'agent-one',
        tabs: [
          {
            key: 'terminal-one',
            title: 'Desktop shell',
            kind: 'shell',
            workspace: { mountId: '', path: '', repository: 'desktop' },
            sharedAgentId: '',
          },
          {
            key: 'agent-one',
            title: 'OpenCode agent',
            kind: 'agent',
            workspace: { mountId: '', path: '', repository: '' },
            sharedAgentId: 'agent-session-1',
          },
        ],
      },
    });

    const fixture = await create();
    const terminal = fixture.componentInstance;

    expect(terminal.tabs()).toEqual([
      expect.objectContaining({
        key: 'terminal-one',
        title: 'Desktop shell',
        repo: 'desktop',
      }),
      expect.objectContaining({
        key: 'agent-one',
        title: 'OpenCode agent',
        attachId: 'agent-session-1',
        shared: true,
        exited: false,
      }),
    ]);
    expect(terminal.activeKey()).toBe('agent-one');
  });

  it('restores unavailable shared agents as exited and offers a fresh shell', async () => {
    bridge.call.mockResolvedValue({ sessions: [] });
    workspace.load.mockResolvedValue({
      version: 1,
      revision: 2,
      updatedAt: '2026-07-27T14:00:00Z',
      workspace: {
        activeKey: 'agent-one',
        tabs: [
          {
            key: 'agent-one',
            title: 'Finished agent',
            kind: 'agent',
            workspace: { mountId: '', path: '', repository: '' },
            sharedAgentId: 'agent-session-1',
          },
        ],
      },
    });
    const fixture = await create();
    const terminal = fixture.componentInstance;

    expect(terminal.tabs()[0]).toMatchObject({
      key: 'agent-one',
      exited: true,
      attachId: undefined,
    });
    terminal.restartTab('agent-one');
    expect(terminal.tabs()[0]).toMatchObject({
      key: 'agent-one',
      exited: false,
      shared: false,
      attachId: undefined,
    });
    expect(workspace.schedule).toHaveBeenCalled();
  });

  it('accepts open-terminal hand-offs and never leaves the tab strip empty', async () => {
    const fixture = await create();
    const terminal = fixture.componentInstance;

    window.dispatchEvent(
      new CustomEvent('lthn:open-terminal', {
        detail: { repo: 'desktop', cwd: '/work/desktop', title: 'Desktop shell' },
      }),
    );
    expect(terminal.tabs().at(-1)).toMatchObject({
      repo: 'desktop',
      cwd: '/work/desktop',
      title: 'Desktop shell',
    });

    for (const tab of [...terminal.tabs()]) terminal.closeTab(tab.key);
    expect(terminal.tabs()).toHaveLength(1);
    expect(terminal.tabs()[0].title).toMatch(/^shell /);
    expect(workspace.schedule).toHaveBeenCalled();
  });

  it('uses only its in-memory workspace and skips terminal discovery offline', async () => {
    workspace.isOffline.mockReturnValue(true);
    const fixture = await create();
    const terminal = fixture.componentInstance;

    expect(terminal.offline()).toBe(true);
    expect(terminal.tabs()).toHaveLength(1);
    expect(bridge.call).not.toHaveBeenCalled();
  });
});

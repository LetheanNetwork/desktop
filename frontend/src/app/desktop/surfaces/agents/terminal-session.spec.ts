import { signal } from '@angular/core';
import { ComponentFixture, TestBed } from '@angular/core/testing';
import { ConnectionManagerService, ConnectionState } from '../../../connection-manager.service';
import { SurfaceBridgeService } from '../surface-bridge.service';
import { AgentTerminalSession, TerminalCursorTracker, TerminalTab } from './terminal-session';

const xterm = vi.hoisted(() => ({
  instances: [] as Array<{
    cols: number;
    rows: number;
    write: ReturnType<typeof vi.fn>;
    reset: ReturnType<typeof vi.fn>;
    focus: ReturnType<typeof vi.fn>;
    onDataHandler?: (data: string) => void;
  }>,
}));

vi.mock('@xterm/xterm', () => ({
  Terminal: class {
    cols = 80;
    rows = 24;
    unicode = { versions: [] as string[], activeVersion: '' };
    write = vi.fn();
    reset = vi.fn();
    focus = vi.fn();
    dispose = vi.fn();
    open = vi.fn();
    loadAddon = vi.fn();
    attachCustomKeyEventHandler = vi.fn();
    onDataHandler?: (data: string) => void;

    constructor() {
      xterm.instances.push(this);
    }

    onData(handler: (data: string) => void) {
      this.onDataHandler = handler;
      return { dispose: vi.fn() };
    }
  },
}));

vi.mock('@xterm/addon-fit', () => ({
  FitAddon: class {
    fit = vi.fn();
  },
}));
vi.mock('@xterm/addon-search', () => ({
  SearchAddon: class {
    clearDecorations = vi.fn();
    findNext = vi.fn();
    findPrevious = vi.fn();
    onDidChangeResults = vi.fn();
  },
}));
vi.mock('@xterm/addon-clipboard', () => ({ ClipboardAddon: class {} }));
vi.mock('@xterm/addon-unicode-graphemes', () => ({ UnicodeGraphemesAddon: class {} }));
vi.mock('@xterm/addon-web-links', () => ({ WebLinksAddon: class {} }));
vi.mock('@xterm/addon-webgl', () => ({
  WebglAddon: class {
    dispose = vi.fn();
    onContextLoss = vi.fn();
  },
}));

describe('TerminalCursorTracker', () => {
  it('writes contiguous data once, drops duplicates, and reports gaps', () => {
    const tracker = new TerminalCursorTracker();

    expect(tracker.accept({ start: 0, end: 3, data: base64('abc'), reset: false })).toMatchObject({
      kind: 'write',
      start: 0,
      end: 3,
      data: new Uint8Array([97, 98, 99]),
    });
    expect(tracker.cursor).toBe(3);
    expect(tracker.accept({ start: 0, end: 3, data: base64('abc'), reset: false })).toEqual({
      kind: 'duplicate',
    });
    expect(tracker.accept({ start: 5, end: 6, data: base64('x'), reset: false })).toEqual({
      kind: 'gap',
    });
    expect(tracker.cursor).toBe(3);
  });

  it('replaces stale output on reset and rejects malformed cursor payloads', () => {
    const tracker = new TerminalCursorTracker();
    tracker.accept({ start: 0, end: 3, data: base64('old'), reset: false });

    expect(tracker.accept({ start: 8, end: 11, data: base64('new'), reset: true })).toMatchObject({
      kind: 'reset',
      start: 8,
      end: 11,
      data: new Uint8Array([110, 101, 119]),
    });
    expect(tracker.cursor).toBe(11);

    for (const invalid of [
      'raw-base64',
      { start: -1, end: 0, data: '', reset: false },
      { start: 12, end: 11, data: '', reset: false },
      { start: 11, end: 13, data: base64('x'), reset: false },
      { start: 11, end: 12, data: '*not-base64*', reset: false },
      { start: 11, end: 12, data: base64('x'), reset: false, command: ['/bin/sh'] },
    ]) {
      expect(tracker.accept(invalid)).toEqual({ kind: 'invalid' });
    }
    expect(tracker.cursor).toBe(11);
  });
});

describe('AgentTerminalSession reconnect', () => {
  const connectionState = signal<ConnectionState>('connected');
  const offline = signal(false);
  const bridge = {
    call: vi.fn(),
  };
  const tab: TerminalTab = {
    key: 'terminal-one',
    title: 'shell',
  };

  beforeEach(() => {
    xterm.instances.length = 0;
    connectionState.set('connected');
    offline.set(false);
    vi.resetAllMocks();
    vi.stubGlobal(
      'ResizeObserver',
      class {
        observe() {}
        disconnect() {}
      },
    );
    vi.stubGlobal('requestAnimationFrame', (callback: FrameRequestCallback) => {
      return window.setTimeout(() => callback(0), 0);
    });
    bridge.call.mockImplementation(async (method: string) => {
      if (method.endsWith('.Open')) {
        return { id: 'session-one', cwd: '/workspace', shell: '/bin/zsh' };
      }
      return null;
    });
    TestBed.configureTestingModule({
      providers: [
        {
          provide: ConnectionManagerService,
          useValue: {
            state: connectionState.asReadonly(),
            offline: offline.asReadonly(),
          },
        },
        { provide: SurfaceBridgeService, useValue: bridge },
      ],
    });
  });

  afterEach(() => {
    TestBed.resetTestingModule();
    vi.unstubAllGlobals();
  });

  async function create(input: TerminalTab = tab): Promise<ComponentFixture<AgentTerminalSession>> {
    const fixture = TestBed.createComponent(AgentTerminalSession);
    fixture.componentRef.setInput('tab', input);
    fixture.componentRef.setInput('active', true);
    fixture.detectChanges();
    await vi.waitFor(() => {
      expect(bridge.call).toHaveBeenCalledWith(
        'dappco.re/lthn/desktop/pkg/terminal.Service.Attach',
        [{ id: 'session-one', after: 0 }],
      );
    });
    return fixture;
  }

  it('creates no PTY or Wails listener in explicit offline mode', async () => {
    offline.set(true);
    connectionState.set('offline');
    const fixture = TestBed.createComponent(AgentTerminalSession);
    fixture.componentRef.setInput('tab', tab);
    fixture.componentRef.setInput('active', true);
    fixture.detectChanges();
    await fixture.whenStable();

    expect(bridge.call).not.toHaveBeenCalled();
    expect(xterm.instances).toHaveLength(0);
    expect(fixture.componentInstance.transportStatus()).toContain('Demo terminal');
    fixture.destroy();
  });

  it('opens restored Files workspaces through opaque mount addresses', async () => {
    const fixture = await create({
      key: 'terminal-mounted',
      title: 'Documents',
      mountId: 'documents',
      workspacePath: 'projects/desktop',
    });

    expect(bridge.call).toHaveBeenCalledWith('dappco.re/lthn/desktop/pkg/terminal.Service.Open', [
      expect.objectContaining({
        mountId: 'documents',
        path: 'projects/desktop',
        cwd: '',
        repo: '',
      }),
    ]);
    const encoded = JSON.stringify(
      bridge.call.mock.calls.find(([method]) => String(method).endsWith('.Open')),
    );
    expect(encoded).not.toContain('/Users/');
    fixture.destroy();
  });

  it('resumes after the accepted cursor without adding another event listener', async () => {
    const fixture = await create();
    const component = fixture.componentInstance as unknown as {
      offHandlers: Array<() => void>;
    };
    const instance = xterm.instances[0];
    const listenerCount = component.offHandlers.length;

    dispatchTerminalOutput('session-one', {
      start: 0,
      end: 3,
      data: base64('abc'),
      reset: false,
    });
    expect(instance.write).toHaveBeenCalledTimes(1);

    connectionState.set('reconnecting');
    fixture.detectChanges();
    const callsBeforeInput = bridge.call.mock.calls.length;
    instance.onDataHandler?.('blocked');
    expect(bridge.call.mock.calls).toHaveLength(callsBeforeInput);

    connectionState.set('connected');
    fixture.detectChanges();
    await vi.waitFor(() => {
      expect(bridge.call).toHaveBeenCalledWith(
        'dappco.re/lthn/desktop/pkg/terminal.Service.Attach',
        [{ id: 'session-one', after: 3 }],
      );
    });
    expect(component.offHandlers).toHaveLength(listenerCount);

    dispatchTerminalOutput('session-one', {
      start: 3,
      end: 4,
      data: base64('d'),
      reset: false,
    });
    expect(instance.write).toHaveBeenCalledTimes(2);
    fixture.destroy();
  });

  it('reattaches on gaps, resets xterm snapshots, and marks missing sessions exited', async () => {
    const fixture = await create();
    const instance = xterm.instances[0];
    const exited = vi.fn();
    fixture.componentInstance.exited.subscribe(exited);

    dispatchTerminalOutput('session-one', {
      start: 0,
      end: 3,
      data: base64('abc'),
      reset: false,
    });
    dispatchTerminalOutput('session-one', {
      start: 5,
      end: 6,
      data: base64('x'),
      reset: false,
    });
    await vi.waitFor(() => {
      expect(bridge.call).toHaveBeenCalledWith(
        'dappco.re/lthn/desktop/pkg/terminal.Service.Attach',
        [{ id: 'session-one', after: 3 }],
      );
    });

    dispatchTerminalOutput('session-one', {
      start: 8,
      end: 11,
      data: base64('new'),
      reset: true,
    });
    expect(instance.reset).toHaveBeenCalledTimes(1);
    expect(instance.write).toHaveBeenLastCalledWith(new Uint8Array([110, 101, 119]));

    bridge.call.mockImplementation(async (method: string) => {
      if (method.endsWith('.Attach')) throw new Error('session not found: session-one');
      return null;
    });
    connectionState.set('reconnecting');
    fixture.detectChanges();
    connectionState.set('connected');
    fixture.detectChanges();

    await vi.waitFor(() => expect(exited).toHaveBeenCalledWith('terminal-one'));
    expect(fixture.componentInstance.error()).toContain('session not found');
    fixture.destroy();
  });
});

function base64(value: string): string {
  return btoa(value);
}

function dispatchTerminalOutput(sessionID: string, data: unknown): void {
  const runtimeWindow = window as Window & {
    _wails?: {
      dispatchWailsEvent?: (event: { name: string; data: unknown }) => void;
    };
  };
  runtimeWindow._wails?.dispatchWailsEvent?.({
    name: `lthn:term:out:${sessionID}`,
    data,
  });
}

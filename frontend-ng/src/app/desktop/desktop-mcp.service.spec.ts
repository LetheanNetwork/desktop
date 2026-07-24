import { signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { APPS, Win } from './desktop.data';
import { DesktopMcpService } from './desktop-mcp.service';
import { WindowManagerService } from './window-manager.service';

interface RegisteredTool {
  name: string;
  description: string;
  inputSchema: Record<string, unknown>;
  execute: (args: Record<string, string>, client: { signal: AbortSignal }) => unknown;
}

const openWin: Win = {
  id: 'w-control',
  app: 'control',
  sub: 'models',
  systab: '',
  x: 70,
  y: 24,
  w: 780,
  h: 560,
  z: 11,
  min: false,
  max: false,
};

describe('DesktopMcpService', () => {
  const registered = new Map<string, RegisteredTool>();
  const view = signal<'desktop' | 'shell' | 'device'>('shell');
  const device = signal<'small' | 'large' | 'full'>('small');
  const wins = signal<Win[]>([openWin]);
  const focusId = signal<string | null>('w-control');
  const windowed = signal(false);
  const windows = {
    view,
    device,
    focusId,
    windowed,
    openWins: wins,
    app: (id: string) => APPS[id],
    launch: vi.fn(),
    focus: vi.fn(),
    close: vi.fn(),
    minimise: vi.fn(),
    maximise: vi.fn(),
    setView: vi.fn(),
  };

  beforeEach(async () => {
    registered.clear();
    vi.clearAllMocks();
    view.set('shell');
    device.set('small');
    wins.set([openWin]);
    focusId.set('w-control');
    windowed.set(false);

    Object.defineProperty(document, 'modelContext', {
      configurable: true,
      value: {
        registerTool: vi.fn(async (tool: RegisteredTool, options?: { signal?: AbortSignal }) => {
          registered.set(tool.name, tool);
          options?.signal?.addEventListener('abort', () => registered.delete(tool.name), {
            once: true,
          });
        }),
      },
    });

    TestBed.configureTestingModule({
      providers: [
        {
          provide: WindowManagerService,
          useValue: windows,
        },
      ],
    });
    await TestBed.inject(DesktopMcpService).ready;
  });

  afterEach(() => TestBed.resetTestingModule());

  const call = async (name: string, args: Record<string, string> = {}): Promise<unknown> => {
    const tool = registered.get(name);
    if (!tool) throw new Error(`Missing fixture tool: ${name}`);
    return await tool.execute(args, {
      signal: new AbortController().signal,
    });
  };

  it('declares the complete OS tool set with strict schemas', () => {
    expect([...registered.keys()].sort()).toEqual([
      'desktop_close_window',
      'desktop_focus_window',
      'desktop_launch_app',
      'desktop_list_windows',
      'desktop_maximise_window',
      'desktop_minimise_window',
      'desktop_read_state',
      'desktop_switch_view',
    ]);
    expect(registered.get('desktop_launch_app')?.inputSchema).toMatchObject({
      required: ['app_id'],
      additionalProperties: false,
      properties: {
        app_id: { enum: Object.keys(APPS) },
      },
    });
  });

  it('reads live desktop and focused-window signals', async () => {
    await expect(call('desktop_list_windows')).resolves.toEqual({
      content: [
        {
          type: 'text',
          text: JSON.stringify({
            view: 'shell',
            focused_window_id: 'w-control',
            windows: [
              {
                id: 'w-control',
                app_id: 'control',
                title: 'Control',
                focused: true,
                minimised: false,
                maximised: false,
                subview: 'models',
                system_tab: '',
              },
            ],
          }),
        },
      ],
    });

    device.set('full');
    windowed.set(true);
    const result = await call('desktop_read_state');
    expect(JSON.parse((result as any).content[0].text)).toMatchObject({
      view: 'shell',
      device: 'full',
      windowed: true,
      focused: {
        id: 'w-control',
        app_id: 'control',
      },
    });
  });

  it('dispatches validated desktop actions through the facade', async () => {
    await call('desktop_launch_app', { app_id: 'chat' });
    expect(windows.launch).toHaveBeenCalledWith('chat');

    await call('desktop_focus_window', { window_id: 'w-control' });
    expect(windows.focus).toHaveBeenCalledWith('w-control');
    await call('desktop_close_window', { window_id: 'w-control' });
    expect(windows.close).toHaveBeenCalledWith('w-control');
    await call('desktop_minimise_window', { window_id: 'w-control' });
    expect(windows.minimise).toHaveBeenCalledWith('w-control');
    await call('desktop_maximise_window', { window_id: 'w-control' });
    expect(windows.maximise).toHaveBeenCalledWith('w-control');
    await call('desktop_switch_view', { view: 'device' });
    expect(windows.setView).toHaveBeenCalledWith('device');
  });

  it('does not toggle terminal states and rejects invalid ids', async () => {
    wins.set([{ ...openWin, min: true, max: true }]);
    await call('desktop_minimise_window', { window_id: 'w-control' });
    await call('desktop_maximise_window', { window_id: 'w-control' });
    expect(windows.minimise).not.toHaveBeenCalled();
    expect(windows.maximise).not.toHaveBeenCalled();

    await expect(call('desktop_launch_app', { app_id: 'unknown' })).rejects.toThrow(
      'Unknown app id',
    );
    await expect(call('desktop_focus_window', { window_id: 'missing' })).rejects.toThrow(
      'Unknown open window id',
    );
    await expect(call('desktop_switch_view', { view: 'invalid' })).rejects.toThrow('Unknown view');
  });
});

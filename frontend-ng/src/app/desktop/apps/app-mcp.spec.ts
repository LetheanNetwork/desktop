import { signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { ConnectionManagerService } from '../../connection-manager.service';
import { DesktopFilesBridgeService } from '../desktop-files-bridge.service';
import { Win } from '../desktop.data';
import { WindowManagerService } from '../window-manager.service';
import { ControlApp } from './control.app';
import { FilesApp } from './files.app';

interface RegisteredTool {
  name: string;
  execute: (args: Record<string, string>, client: { signal: AbortSignal }) => unknown;
}

const windows = {
  setSub: vi.fn(),
  setSysTab: vi.fn(),
};

const controlWin: Win = {
  id: 'w-control',
  app: 'control',
  sub: 'models',
  systab: '',
  x: 0,
  y: 0,
  w: 780,
  h: 560,
  z: 1,
  min: false,
  max: false,
};

const filesWin: Win = {
  ...controlWin,
  id: 'w-files',
  app: 'files',
  sub: 'documents',
  systab: 'list',
};

describe('app-view WebMCP tools', () => {
  const registered = new Map<string, RegisteredTool>();
  const offline = signal(true);
  const filesBridge = {
    listMounts: vi.fn(),
    listDirectory: vi.fn(),
    preview: vi.fn(),
    listTrash: vi.fn(),
    onChanged: vi.fn(),
  };

  beforeEach(() => {
    offline.set(true);
    registered.clear();
    vi.clearAllMocks();
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
        {
          provide: ConnectionManagerService,
          useValue: { offline: offline.asReadonly() },
        },
        {
          provide: DesktopFilesBridgeService,
          useValue: filesBridge,
        },
      ],
    });
  });

  afterEach(() => TestBed.resetTestingModule());

  const call = async (name: string, args: Record<string, string> = {}): Promise<unknown> => {
    const tool = registered.get(name);
    if (!tool) throw new Error(`Missing fixture tool: ${name}`);
    return await tool.execute(args, {
      signal: new AbortController().signal,
    });
  };

  it('reads Control state and navigates through its facade', async () => {
    const fixture = TestBed.createComponent(ControlApp);
    fixture.componentRef.setInput('win', controlWin);
    fixture.componentRef.setInput('nav', []);
    fixture.detectChanges();

    await vi.waitFor(() => {
      expect([...registered.keys()].sort()).toEqual(['control_read_state', 'control_show_section']);
    });
    const result = await call('control_read_state');
    const state = JSON.parse((result as any).content[0].text);
    expect(state).toMatchObject({
      section: 'models',
      system_tab: 'overview',
    });
    expect(state.models).toEqual(
      expect.arrayContaining([expect.objectContaining({ name: 'llama-3.1-70b' })]),
    );

    await call('control_show_section', { section: 'power' });
    expect(windows.setSub).toHaveBeenCalledWith('w-control', 'power');
    await expect(call('control_show_section', { section: 'invalid' })).rejects.toThrow(
      'Unknown Control section',
    );

    fixture.destroy();
    expect(registered.has('control_read_state')).toBe(false);
  });

  it('reads and navigates the Files app through its facade', async () => {
    const fixture = TestBed.createComponent(FilesApp);
    fixture.componentRef.setInput('win', filesWin);
    fixture.detectChanges();

    await vi.waitFor(() => {
      expect([...registered.keys()].sort()).toEqual([
        'files_navigate',
        'files_read_location',
        'files_set_view',
      ]);
    });
    const result = await call('files_read_location');
    expect(JSON.parse((result as any).content[0].text)).toMatchObject({
      location_id: 'documents',
      location_name: 'Documents',
      mount_id: 'documents',
      path: '',
      view: 'list',
      data_state: 'demo',
      breadcrumbs: [
        { id: 'home', name: 'Home' },
        { id: 'documents', name: 'Documents' },
      ],
    });

    await call('files_navigate', { location_id: 'models' });
    expect(windows.setSub).toHaveBeenCalledWith('w-files', 'models');
    await call('files_set_view', { view: 'grid' });
    expect(windows.setSysTab).toHaveBeenCalledWith('w-files', 'grid');
    await expect(call('files_navigate', { location_id: 'missing' })).rejects.toThrow(
      'Unknown Files location',
    );
    await expect(call('files_set_view', { view: 'columns' })).rejects.toThrow('Unknown Files view');
    expect(
      [...registered.keys()].some((name) =>
        /create|rename|copy|move|trash|restore|delete/i.test(name),
      ),
    ).toBe(false);
  });
});

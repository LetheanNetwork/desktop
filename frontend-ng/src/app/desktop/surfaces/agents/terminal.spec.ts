import { TestBed } from '@angular/core/testing';
import { Win } from '../../desktop.data';
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

  beforeEach(() => {
    vi.clearAllMocks();
    TestBed.configureTestingModule({
      providers: [{ provide: SurfaceBridgeService, useValue: bridge }],
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
    await fixture.whenStable();
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
  });
});

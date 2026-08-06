import { TestBed } from '@angular/core/testing';
import { Win } from '../../desktop.data';
import { SurfaceBridgeService } from '../surface-bridge.service';
import { ExtensionsPluginViewSurface } from './plugin-view';
import { PluginViewDescriptor } from './plugin-view-runtime';

const opencode: PluginViewDescriptor = {
  id: 'opencode',
  label: 'OpenCode',
  icon: 'terminal',
  group: 'plugin',
  kind: 'iframe',
  source: '/v1/api/plugin/opencode/',
  pluginCode: 'opencode',
  capabilities: ['session-token'],
  proxied: true,
};

const win: Win = {
  id: 'plugin-view-window',
  app: 'extensions',
  sub: 'plugin-view',
  x: 0,
  y: 0,
  w: 640,
  h: 480,
  z: 1,
  min: false,
  max: false,
};

describe('ExtensionsPluginViewSurface', () => {
  const bridge = { call: vi.fn(), request: vi.fn() };

  beforeEach(() => {
    bridge.call.mockReset();
    bridge.request.mockReset();
    TestBed.configureTestingModule({
      providers: [{ provide: SurfaceBridgeService, useValue: bridge }],
    });
  });

  afterEach(() => TestBed.resetTestingModule());

  function create() {
    const fixture = TestBed.createComponent(ExtensionsPluginViewSurface);
    fixture.componentRef.setInput('win', win);
    fixture.detectChanges();
    return fixture;
  }

  it('defaults to the opencode view id and resolves its descriptor badges', async () => {
    bridge.call.mockResolvedValueOnce(opencode);
    const fixture = create();
    const page = fixture.componentInstance;

    expect(page.viewId()).toBe('opencode');
    const input = fixture.nativeElement.querySelector('input') as HTMLInputElement;
    expect(input.value).toBe('opencode');

    await vi.waitFor(() => expect(page.descriptor()).toEqual(opencode));
    fixture.detectChanges();

    expect(bridge.call).toHaveBeenCalledWith(
      'dappco.re/lthn/desktop/pkg/marketplace.Service.GetViewDescriptor',
      ['opencode'],
    );
    const badges = Array.from(
      fixture.nativeElement.querySelectorAll('.descriptor span'),
    ) as HTMLElement[];
    expect(badges.map((badge) => badge.textContent)).toEqual([
      'opencode',
      'iframe',
      'session-token',
    ]);
    fixture.destroy();
  });

  it('switches to a new view id and clears the resolved descriptor', async () => {
    bridge.call.mockResolvedValueOnce(opencode);
    const fixture = create();
    const page = fixture.componentInstance;
    await vi.waitFor(() => expect(page.descriptor()).toEqual(opencode));

    bridge.call.mockResolvedValueOnce({ ...opencode, id: 'notes', pluginCode: 'notes' });
    const input = fixture.nativeElement.querySelector('input') as HTMLInputElement;
    input.value = 'notes';
    input.dispatchEvent(new Event('change'));
    fixture.detectChanges();

    expect(page.descriptor()).toBeNull();
    expect(page.viewId()).toBe('notes');

    await vi.waitFor(() => expect(page.descriptor()?.id).toBe('notes'));
    fixture.destroy();
  });

  it('ignores a change back to the same id and a blank value', () => {
    bridge.call.mockResolvedValue(opencode);
    const fixture = create();
    const page = fixture.componentInstance;

    page.changeView({ target: { value: 'opencode' } } as unknown as Event);
    expect(page.viewId()).toBe('opencode');

    page.changeView({ target: { value: '   ' } } as unknown as Event);
    expect(page.viewId()).toBe('opencode');
    fixture.destroy();
  });
});

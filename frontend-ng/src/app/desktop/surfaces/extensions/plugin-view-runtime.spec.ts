import { TestBed } from '@angular/core/testing';
import { SurfaceBridgeService } from '../surface-bridge.service';
import {
  DEFAULT_PLUGIN_IFRAME_SANDBOX,
  PLUGIN_VIEW_MOUNT_TIMEOUT_MS,
  PluginViewDescriptor,
  PluginViewFrame,
  normaliseDescriptor,
  validatedPluginSource,
} from './plugin-view-runtime';

const descriptor: PluginViewDescriptor = {
  id: 'opencode',
  label: 'OpenCode',
  icon: 'terminal',
  group: 'plugin',
  kind: 'iframe',
  source: 'http://127.0.0.1:4096/workspace',
  pluginCode: 'opencode',
  capabilities: ['session-token'],
  loopbackOrigin: 'http://127.0.0.1:4096',
};

describe('plugin view runtime', () => {
  const bridge = { call: vi.fn() };

  beforeEach(() => {
    vi.clearAllMocks();
    TestBed.configureTestingModule({
      providers: [{ provide: SurfaceBridgeService, useValue: bridge }],
    });
  });

  afterEach(() => {
    vi.useRealTimers();
    TestBed.resetTestingModule();
  });

  it('keeps the exact iframe sandbox floor', () => {
    expect(DEFAULT_PLUGIN_IFRAME_SANDBOX).toBe('allow-scripts allow-forms allow-same-origin');
    expect(DEFAULT_PLUGIN_IFRAME_SANDBOX).not.toContain('allow-popups');
    expect(DEFAULT_PLUGIN_IFRAME_SANDBOX).not.toContain('allow-top-navigation');
  });

  it('accepts exact loopback origins and registered proxy routes only', () => {
    expect(validatedPluginSource(descriptor)).toBe('http://127.0.0.1:4096/workspace');
    expect(validatedPluginSource({ ...descriptor, source: 'http://127.0.0.1:9999/' })).toBe('');
    expect(
      validatedPluginSource({
        ...descriptor,
        source: '/v1/api/plugin/opencode/',
        proxied: true,
      }),
    ).toBe('/v1/api/plugin/opencode/');
    expect(
      validatedPluginSource({
        ...descriptor,
        source: 'https://example.com/plugin',
        loopbackOrigin: 'https://example.com',
      }),
    ).toBe('');
  });

  it('normalises valid backend descriptors and rejects malformed values', () => {
    expect(
      normaliseDescriptor({
        id: 'opencode',
        label: 'OpenCode',
        source: '/v1/api/plugin/opencode/',
        pluginCode: 'opencode',
        kind: 'iframe',
        capabilities: ['session-token', 1],
        proxied: true,
      }),
    ).toMatchObject({
      id: 'opencode',
      group: 'plugin',
      capabilities: ['session-token'],
      proxied: true,
    });
    expect(normaliseDescriptor({ id: 'missing-fields' })).toBeNull();
    expect(normaliseDescriptor(null)).toBeNull();
  });

  it('hydrates through the Wails bridge and renders the exact sandbox', async () => {
    const proxied = {
      ...descriptor,
      source: '/v1/api/plugin/opencode/',
      proxied: true,
    };
    bridge.call.mockResolvedValueOnce(proxied);
    const fixture = TestBed.createComponent(PluginViewFrame);
    fixture.componentRef.setInput('viewId', 'opencode');
    fixture.detectChanges();

    await vi.waitFor(() => expect(fixture.componentInstance.current()).toEqual(proxied));
    fixture.detectChanges();

    expect(bridge.call).toHaveBeenCalledWith(
      'dappco.re/lthn/desktop/pkg/marketplace.Service.GetViewDescriptor',
      ['opencode'],
    );
    const iframe = fixture.nativeElement.querySelector('iframe') as HTMLIFrameElement;
    expect(iframe).not.toBeNull();
    expect(iframe.getAttribute('sandbox')).toBe(DEFAULT_PLUGIN_IFRAME_SANDBOX);
    fixture.destroy();
  });

  it('emits the mount-timeout fallback for an unregistered integrated view', () => {
    vi.useFakeTimers();
    const fixture = TestBed.createComponent(PluginViewFrame);
    const timedOut = vi.fn();
    fixture.componentInstance.mountTimeout.subscribe(timedOut);
    fixture.componentRef.setInput('descriptor', {
      ...descriptor,
      kind: 'lit',
      source: 'lthn-not-registered',
    });
    fixture.detectChanges();

    vi.advanceTimersByTime(PLUGIN_VIEW_MOUNT_TIMEOUT_MS);

    expect(fixture.componentInstance.state()).toBe('timed-out');
    expect(timedOut).toHaveBeenCalledWith({
      viewId: 'opencode',
      pluginCode: 'opencode',
    });
    fixture.destroy();
  });
});

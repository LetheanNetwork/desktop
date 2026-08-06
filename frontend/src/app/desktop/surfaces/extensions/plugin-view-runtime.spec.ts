import { TestBed } from '@angular/core/testing';
import { SurfaceBridgeService } from '../surface-bridge.service';
import {
  DEFAULT_PLUGIN_IFRAME_SANDBOX,
  PLUGIN_VIEW_MOUNT_TIMEOUT_MS,
  PluginLitOutlet,
  PluginViewDescriptor,
  PluginViewFrame,
  normaliseDescriptor,
  validatedPluginSource,
} from './plugin-view-runtime';

if (!customElements.get('lthn-runtime-test-outlet')) {
  customElements.define(
    'lthn-runtime-test-outlet',
    class extends HTMLElement {},
  );
}

const litDescriptor: PluginViewDescriptor = {
  id: 'notepad',
  label: 'Notepad',
  icon: 'note',
  group: 'plugin',
  kind: 'lit',
  source: 'lthn-runtime-test-outlet',
  pluginCode: 'notepad-plugin',
};

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

  it('renders the timed-out fallback message once the mount deadline passes', () => {
    vi.useFakeTimers();
    const fixture = TestBed.createComponent(PluginViewFrame);
    fixture.componentRef.setInput('descriptor', {
      ...descriptor,
      kind: 'lit',
      source: 'lthn-not-registered',
    });
    fixture.detectChanges();

    vi.advanceTimersByTime(PLUGIN_VIEW_MOUNT_TIMEOUT_MS);
    fixture.detectChanges();

    const alert = fixture.nativeElement.querySelector('[role="alert"]') as HTMLElement;
    expect(alert.textContent).toContain('Plugin view failed to mount within 1,500 ms.');
    fixture.destroy();
  });

  it('exposes the live iframe window once the frame has mounted', async () => {
    bridge.call.mockResolvedValueOnce(descriptor);
    const fixture = TestBed.createComponent(PluginViewFrame);
    fixture.componentRef.setInput('viewId', 'opencode');
    fixture.detectChanges();
    await vi.waitFor(() => expect(fixture.componentInstance.current()).toMatchObject({ id: descriptor.id }));
    fixture.detectChanges();

    expect(fixture.componentInstance.iframeWindow).not.toBeNull();
    fixture.destroy();
  });

  it('reports the missing state for a blank view id with no descriptor supplied', () => {
    const fixture = TestBed.createComponent(PluginViewFrame);
    fixture.detectChanges();

    expect(fixture.componentInstance.state()).toBe('missing');
    expect(fixture.componentInstance.loading()).toBe(false);
    const message = fixture.nativeElement.querySelector('.plugin-state span') as HTMLElement;
    expect(message.textContent).toContain('No installed plugin view matches this id.');
    fixture.destroy();
  });

  it('falls back to the missing state when the bridge cannot resolve a descriptor', async () => {
    bridge.call.mockRejectedValueOnce(new Error('not found'));
    const fixture = TestBed.createComponent(PluginViewFrame);
    fixture.componentRef.setInput('viewId', 'unknown-view');
    fixture.detectChanges();

    await vi.waitFor(() => expect(fixture.componentInstance.state()).toBe('missing'));

    expect(fixture.componentInstance.current()).toBeNull();
    fixture.destroy();
  });

  it('flags an iframe descriptor whose source fails the loopback-origin policy', () => {
    const fixture = TestBed.createComponent(PluginViewFrame);
    fixture.componentRef.setInput('descriptor', {
      ...descriptor,
      source: 'https://example.com/plugin',
      loopbackOrigin: undefined,
    });
    fixture.detectChanges();

    expect(fixture.componentInstance.state()).toBe('invalid');
    const alert = fixture.nativeElement.querySelector('[role="alert"]') as HTMLElement;
    expect(alert.textContent).toContain('Plugin source failed the loopback-origin policy.');
    fixture.destroy();
  });

  it('treats an unparsable iframe source as invalid rather than throwing', () => {
    expect(validatedPluginSource({ ...descriptor, source: 'not a url::' })).toBe('');
  });

  it('marks the frame ready once the sandboxed iframe reports load', async () => {
    bridge.call.mockResolvedValueOnce(descriptor);
    const fixture = TestBed.createComponent(PluginViewFrame);
    fixture.componentRef.setInput('viewId', 'opencode');
    fixture.detectChanges();
    await vi.waitFor(() => expect(fixture.componentInstance.current()).toMatchObject({ id: descriptor.id }));
    fixture.detectChanges();

    const iframe = fixture.nativeElement.querySelector('iframe') as HTMLIFrameElement;
    iframe.dispatchEvent(new Event('load'));

    expect(fixture.componentInstance.state()).toBe('ready');
    expect(fixture.componentInstance.timedOut()).toBe(false);
    fixture.destroy();
  });

  it('mounts a registered lit-integrated plugin element into the outlet', () => {
    const fixture = TestBed.createComponent(PluginViewFrame);
    fixture.componentRef.setInput('descriptor', litDescriptor);
    fixture.detectChanges();

    expect(fixture.componentInstance.state()).toBe('ready');
    const outlet = fixture.nativeElement.querySelector('lthn-plugin-lit-outlet') as HTMLElement;
    expect(outlet).not.toBeNull();
    expect(outlet.querySelector('lthn-runtime-test-outlet')).not.toBeNull();
    expect(outlet.querySelector('lthn-runtime-test-outlet')?.getAttribute('plugin-code')).toBe(
      'notepad-plugin',
    );
    fixture.destroy();
  });

  it('re-renders the lit outlet element when its descriptor changes after mount', () => {
    const fixture = TestBed.createComponent(PluginLitOutlet);
    fixture.componentRef.setInput('descriptor', litDescriptor);
    fixture.detectChanges();
    expect(
      (fixture.nativeElement as HTMLElement).querySelector('lthn-runtime-test-outlet'),
    ).not.toBeNull();

    fixture.componentRef.setInput('descriptor', { ...litDescriptor, pluginCode: 'renamed' });
    fixture.detectChanges();

    expect(
      (fixture.nativeElement as HTMLElement)
        .querySelector('lthn-runtime-test-outlet')
        ?.getAttribute('plugin-code'),
    ).toBe('renamed');
    fixture.destroy();
  });

  it('clears the outlet host without mounting anything for an unresolved custom element', () => {
    const fixture = TestBed.createComponent(PluginLitOutlet);
    fixture.componentRef.setInput('descriptor', { ...litDescriptor, source: 'lthn-not-registered' });
    fixture.detectChanges();

    expect(
      (fixture.nativeElement as HTMLElement).querySelector('lthn-not-registered'),
    ).toBeNull();
    fixture.destroy();
  });
});

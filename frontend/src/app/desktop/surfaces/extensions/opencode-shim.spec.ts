import { TestBed } from '@angular/core/testing';
import { Win } from '../../desktop.data';
import { SurfaceBridgeService } from '../surface-bridge.service';
import {
  clearPluginSessionToken,
  setPluginSessionToken,
} from './plugin-auth-broker';
import { PluginViewDescriptor } from './plugin-view-runtime';
import {
  AUTH_DENIED_EVENT,
  AUTH_GRANTED_EVENT,
  AUTH_REQUEST_TYPE,
  DENY_REASON_AUDIT_FAILED,
  DENY_REASON_CAPABILITY_NOT_DECLARED,
  DENY_REASON_NO_ORIGIN,
  DENY_REASON_ORIGIN_MISMATCH,
  DENY_REASON_SOURCE_MISMATCH,
  DENY_REASON_UNKNOWN_SCOPE,
  ExtensionsOpencodeShimSurface,
  decideAuthRequest,
  expectedPluginOrigin,
} from './opencode-shim';

const frame = {} as Window;
const descriptor: PluginViewDescriptor = {
  id: 'opencode',
  label: 'OpenCode',
  icon: 'terminal',
  group: 'plugin',
  kind: 'iframe',
  source: 'http://127.0.0.1:4096',
  pluginCode: 'opencode',
  capabilities: ['session-token'],
  loopbackOrigin: 'http://127.0.0.1:4096',
};

describe('OpenCode auth request decision', () => {
  it('grants only a declared session-token request from the exact frame and origin', () => {
    expect(
      decideAuthRequest(descriptor, frame, frame, 'http://127.0.0.1:4096', {
        type: AUTH_REQUEST_TYPE,
        scopes: ['session-token'],
      }),
    ).toEqual({
      kind: 'grant',
      scopes: ['session-token'],
      origin: 'http://127.0.0.1:4096',
    });
  });

  it.each([
    [
      'wrong source',
      descriptor,
      {} as Window,
      'http://127.0.0.1:4096',
      ['session-token'],
      DENY_REASON_SOURCE_MISMATCH,
    ],
    [
      'missing origin',
      { ...descriptor, loopbackOrigin: '' },
      frame,
      'http://127.0.0.1:4096',
      ['session-token'],
      DENY_REASON_NO_ORIGIN,
    ],
    [
      'wrong origin',
      descriptor,
      frame,
      'http://127.0.0.1:9999',
      ['session-token'],
      DENY_REASON_ORIGIN_MISMATCH,
    ],
    [
      'no scopes requested',
      descriptor,
      frame,
      'http://127.0.0.1:4096',
      [],
      DENY_REASON_CAPABILITY_NOT_DECLARED,
    ],
    [
      'undeclared scope',
      { ...descriptor, capabilities: [] },
      frame,
      'http://127.0.0.1:4096',
      ['session-token'],
      DENY_REASON_CAPABILITY_NOT_DECLARED,
    ],
    [
      'unknown forward scope',
      { ...descriptor, capabilities: ['vi-events'] },
      frame,
      'http://127.0.0.1:4096',
      ['vi-events'],
      DENY_REASON_UNKNOWN_SCOPE,
    ],
  ])('denies %s', (_label, desc, source, origin, scopes, reason) => {
    expect(
      decideAuthRequest(desc, frame, source, origin, {
        type: AUTH_REQUEST_TYPE,
        scopes,
      }),
    ).toEqual({ kind: 'deny', reason });
  });

  it('ignores unrelated messages and requests without a descriptor', () => {
    expect(decideAuthRequest(descriptor, frame, frame, '', { type: 'other' })).toEqual({
      kind: 'ignore',
    });
    expect(
      decideAuthRequest(null, frame, frame, '', {
        type: AUTH_REQUEST_TYPE,
        scopes: ['session-token'],
      }),
    ).toEqual({ kind: 'ignore' });
  });

  it('uses the desktop origin for same-origin proxied plugin views', () => {
    expect(
      expectedPluginOrigin(
        { ...descriptor, loopbackOrigin: undefined, proxied: true },
        'http://127.0.0.1:9245',
      ),
    ).toBe('http://127.0.0.1:9245');
  });
});

describe('ExtensionsOpencodeShimSurface', () => {
  const bridge = { call: vi.fn(), request: vi.fn() };
  const win: Win = {
    id: 'opencode-window',
    app: 'extensions',
    sub: 'opencode',
    x: 0,
    y: 0,
    w: 640,
    h: 480,
    z: 1,
    min: false,
    max: false,
  };

  beforeEach(() => {
    bridge.call.mockReset();
    bridge.request.mockReset();
    TestBed.configureTestingModule({
      providers: [{ provide: SurfaceBridgeService, useValue: bridge }],
    });
  });

  afterEach(() => {
    clearPluginSessionToken();
    vi.unstubAllGlobals();
    TestBed.resetTestingModule();
  });

  function create() {
    const fixture = TestBed.createComponent(ExtensionsOpencodeShimSurface);
    fixture.componentRef.setInput('win', win);
    fixture.detectChanges();
    return fixture;
  }

  it('ignores a message received before the frame descriptor has resolved', () => {
    const fixture = create();

    window.dispatchEvent(
      new MessageEvent('message', {
        origin: 'http://127.0.0.1:4096',
        data: { type: AUTH_REQUEST_TYPE, scopes: ['session-token'] },
      }),
    );

    expect(fixture.componentInstance.status()).toBe('');
    fixture.destroy();
  });

  it('denies and answers a mismatched-origin request once the descriptor has resolved', async () => {
    bridge.call.mockResolvedValueOnce(descriptor);
    const fixture = create();
    await vi.waitFor(() =>
      expect(fixture.nativeElement.querySelector('iframe')).not.toBeNull(),
    );
    fixture.detectChanges();
    const iframe = fixture.nativeElement.querySelector('iframe') as HTMLIFrameElement;
    const denied = vi.fn();
    window.addEventListener(AUTH_DENIED_EVENT, denied);

    window.dispatchEvent(
      new MessageEvent('message', {
        source: iframe.contentWindow,
        origin: 'http://wrong-origin.example',
        data: { type: AUTH_REQUEST_TYPE, scopes: ['session-token'] },
      }),
    );

    window.removeEventListener(AUTH_DENIED_EVENT, denied);
    expect(denied).toHaveBeenCalledOnce();
    expect((denied.mock.calls[0][0] as CustomEvent).detail).toMatchObject({
      reason: DENY_REASON_ORIGIN_MISMATCH,
    });
    fixture.detectChanges();
    expect(fixture.nativeElement.querySelector('.status')?.textContent).toContain(
      DENY_REASON_ORIGIN_MISMATCH,
    );
    fixture.destroy();
  });

  it('denies a granted-shaped request the broker cannot audit', async () => {
    setPluginSessionToken('LTHN-SESS-1.secret');
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => ({ ok: false }) as Response),
    );
    bridge.call.mockResolvedValueOnce(descriptor);
    const fixture = create();
    await vi.waitFor(() =>
      expect(fixture.nativeElement.querySelector('iframe')).not.toBeNull(),
    );
    const iframe = fixture.nativeElement.querySelector('iframe') as HTMLIFrameElement;
    const denied = vi.fn();
    window.addEventListener(AUTH_DENIED_EVENT, denied);

    window.dispatchEvent(
      new MessageEvent('message', {
        source: iframe.contentWindow,
        origin: 'http://127.0.0.1:4096',
        data: { type: AUTH_REQUEST_TYPE, scopes: ['session-token'] },
      }),
    );

    await vi.waitFor(() => expect(denied).toHaveBeenCalledOnce());
    window.removeEventListener(AUTH_DENIED_EVENT, denied);
    expect((denied.mock.calls[0][0] as CustomEvent).detail).toMatchObject({
      reason: DENY_REASON_AUDIT_FAILED,
    });
    fixture.destroy();
  });

  it('brokers a verified session-token grant through to the plugin frame', async () => {
    setPluginSessionToken('LTHN-SESS-1.secret');
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => ({ ok: true }) as Response),
    );
    bridge.call.mockResolvedValueOnce(descriptor);
    const fixture = create();
    await vi.waitFor(() =>
      expect(fixture.nativeElement.querySelector('iframe')).not.toBeNull(),
    );
    const iframe = fixture.nativeElement.querySelector('iframe') as HTMLIFrameElement;
    const granted = vi.fn();
    window.addEventListener(AUTH_GRANTED_EVENT, granted);

    window.dispatchEvent(
      new MessageEvent('message', {
        source: iframe.contentWindow,
        origin: 'http://127.0.0.1:4096',
        data: { type: AUTH_REQUEST_TYPE, scopes: ['session-token'] },
      }),
    );

    await vi.waitFor(() => expect(granted).toHaveBeenCalledOnce());
    window.removeEventListener(AUTH_GRANTED_EVENT, granted);
    expect((granted.mock.calls[0][0] as CustomEvent).detail).toMatchObject({
      scopes: ['session-token'],
    });
    fixture.detectChanges();
    expect(fixture.nativeElement.querySelector('.status')?.textContent).toContain(
      'Account access granted',
    );
    fixture.destroy();
  });

  it('stops listening for postMessage auth requests once destroyed', async () => {
    bridge.call.mockResolvedValueOnce(descriptor);
    const fixture = create();
    await vi.waitFor(() =>
      expect(fixture.nativeElement.querySelector('iframe')).not.toBeNull(),
    );
    fixture.destroy();
    const denied = vi.fn();
    window.addEventListener(AUTH_DENIED_EVENT, denied);

    window.dispatchEvent(
      new MessageEvent('message', {
        origin: 'http://wrong-origin.example',
        data: { type: AUTH_REQUEST_TYPE, scopes: ['session-token'] },
      }),
    );

    window.removeEventListener(AUTH_DENIED_EVENT, denied);
    expect(denied).not.toHaveBeenCalled();
  });
});

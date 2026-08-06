import {
  AUTH_GRANT_TYPE,
  CAPABILITY_GRANT_AUDIT_PATH,
  clearPluginSessionToken,
  grantTokenToFrame,
  setPluginSessionToken,
} from './plugin-auth-broker';

describe('plugin auth broker', () => {
  afterEach(() => {
    clearPluginSessionToken();
    vi.unstubAllGlobals();
  });

  it('audits once before posting the closure-scoped token', async () => {
    const order: string[] = [];
    const fetchMock = vi.fn(async (_input: RequestInfo | URL, _init?: RequestInit) => {
      order.push('audit');
      return { ok: true } as Response;
    });
    vi.stubGlobal('fetch', fetchMock);
    const postMessage = vi.fn(() => order.push('post'));
    setPluginSessionToken('LTHN-SESS-1.secret');

    const outcome = await grantTokenToFrame(
      {
        source: { postMessage } as unknown as Window,
        targetOrigin: 'http://127.0.0.1:4096',
        pluginCode: 'opencode',
      },
      ['session-token'],
    );

    expect(order).toEqual(['audit', 'post']);
    expect(outcome).toEqual({ ok: true, scopes: ['session-token'] });
    expect(fetchMock).toHaveBeenCalledOnce();
    expect(fetchMock.mock.calls[0][0]).toBe(CAPABILITY_GRANT_AUDIT_PATH);
    const init = fetchMock.mock.calls[0][1] as RequestInit;
    expect(JSON.parse(String(init.body))).toEqual({
      plugin_id: 'opencode',
      capabilities: ['session-token'],
      origin: 'http://127.0.0.1:4096',
      outcome: 'granted',
    });
    expect(postMessage).toHaveBeenCalledWith(
      {
        type: AUTH_GRANT_TYPE,
        token: 'LTHN-SESS-1.secret',
        scopes: ['session-token'],
      },
      'http://127.0.0.1:4096',
    );
    expect(JSON.stringify(outcome)).not.toContain('LTHN-SESS-1');
  });

  it('fails closed when the session or audit is unavailable', async () => {
    const postMessage = vi.fn();
    const target = {
      source: { postMessage } as unknown as Window,
      targetOrigin: 'http://127.0.0.1:4096',
      pluginCode: 'opencode',
    };

    await expect(grantTokenToFrame(target, ['session-token'])).resolves.toEqual({
      ok: false,
      reason: 'no-session',
    });
    expect(postMessage).not.toHaveBeenCalled();

    setPluginSessionToken('LTHN-SESS-1.secret');
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => ({ ok: false }) as Response),
    );
    await expect(grantTokenToFrame(target, ['session-token'])).resolves.toEqual({
      ok: false,
      reason: 'audit-failed',
    });
    expect(postMessage).not.toHaveBeenCalled();
  });

  it('treats a network failure during the audit call as an audit failure', async () => {
    setPluginSessionToken('LTHN-SESS-1.secret');
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => {
        throw new Error('network unreachable');
      }),
    );
    const postMessage = vi.fn();

    await expect(
      grantTokenToFrame(
        {
          source: { postMessage } as unknown as Window,
          targetOrigin: 'http://127.0.0.1:4096',
          pluginCode: 'opencode',
        },
        ['session-token'],
      ),
    ).resolves.toEqual({ ok: false, reason: 'audit-failed' });
    expect(postMessage).not.toHaveBeenCalled();
  });

  it('reports postmessage-failed when the verified frame rejects the delivery', async () => {
    setPluginSessionToken('LTHN-SESS-1.secret');
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => ({ ok: true }) as Response),
    );
    const postMessage = vi.fn(() => {
      throw new DOMException('target origin mismatch');
    });

    await expect(
      grantTokenToFrame(
        {
          source: { postMessage } as unknown as Window,
          targetOrigin: 'http://127.0.0.1:4096',
          pluginCode: 'opencode',
        },
        ['session-token'],
      ),
    ).resolves.toEqual({ ok: false, reason: 'postmessage-failed' });
  });
});

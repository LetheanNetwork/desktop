const runtimeHarness = vi.hoisted(() => {
  let resolveRuntime!: () => void;
  const runtimeReady = new Promise<void>((resolve) => {
    resolveRuntime = resolve;
  });

  return {
    emit: vi.fn(() => Promise.resolve(true)),
    listeners: new Map<string, (event: { data: unknown }) => void>(),
    on: vi.fn((name: string, callback: (event: { data: unknown }) => void) => {
      runtimeHarness.listeners.set(name, callback);
      return () => runtimeHarness.listeners.delete(name);
    }),
    dispatch: (name: string, data: unknown) => {
      runtimeHarness.listeners.get(name)?.({ data });
    },
    release: () => resolveRuntime(),
    runtimeReady,
  };
});

vi.mock('@wailsio/runtime', async () => {
  await runtimeHarness.runtimeReady;
  return {
    Events: {
      Emit: runtimeHarness.emit,
      On: runtimeHarness.on,
    },
  };
});

const originalConsole = {
  log: console.log,
  info: console.info,
  warn: console.warn,
  error: console.error,
  debug: console.debug,
};

await import('./wails-bridge');

export {};

describe('Wails bridge bootstrap', () => {
  afterAll(() => {
    Object.assign(console, originalConsole);
  });

  it('publishes immediately, caps its queue, and flushes through Events.Emit', async () => {
    expect(window.__lthnEmit).toEqual(expect.any(Function));

    for (let index = 0; index < 300; index++) {
      window.__lthnEmit('lthn:eval-reply', { index });
    }
    expect(runtimeHarness.emit).not.toHaveBeenCalled();

    runtimeHarness.release();

    await vi.waitFor(() => {
      expect(runtimeHarness.emit).toHaveBeenCalledTimes(256);
    });
    expect(runtimeHarness.emit.mock.calls[0]).toMatchObject([
      'lthn:console',
      {
        level: 'info',
        message: 'lthn shim ready (module emit path)',
        source: 'shim',
      },
    ]);
    expect(runtimeHarness.emit).toHaveBeenLastCalledWith('lthn:eval-reply', {
      index: 254,
    });
  });

  it('forwards formatted console arguments after calling the original', async () => {
    console.debug(new TypeError('bridge exploded'), { nested: { ok: true } });

    await vi.waitFor(() => {
      expect(runtimeHarness.emit).toHaveBeenLastCalledWith(
        'lthn:console',
        expect.objectContaining({
          level: 'debug',
          message: expect.stringMatching(
            /TypeError: bridge exploded[\s\S]*\{"nested":\{"ok":true\}\}/,
          ),
          source: 'console',
          at: expect.any(String),
        }),
      );
    });
  });

  it('forwards window errors with source coordinates', () => {
    window.dispatchEvent(
      new ErrorEvent('error', {
        message: 'paint failed',
        filename: 'main.js',
        lineno: 12,
        colno: 34,
        error: new Error('paint failed'),
      }),
    );

    expect(runtimeHarness.emit).toHaveBeenLastCalledWith(
      'lthn:error',
      expect.objectContaining({
        message: 'paint failed',
        source: 'main.js',
        line: 12,
        col: 34,
      }),
    );
  });

  it('registers Angular WebMCP tools on a window-callable bridge', async () => {
    const registration = new AbortController();

    await document.modelContext?.registerTool(
      {
        name: 'test_read_signal',
        description: 'Reads test state.',
        inputSchema: {
          type: 'object',
          properties: {},
          additionalProperties: false,
        },
        execute: (args, client) => ({
          content: [
            {
              type: 'text',
              text: JSON.stringify({
                args,
                aborted: client.signal.aborted,
              }),
            },
          ],
        }),
      },
      { signal: registration.signal },
    );

    await expect(window.__lthnWebMcp.request({ method: 'tools/list' })).resolves.toEqual({
      tools: [
        expect.objectContaining({
          name: 'test_read_signal',
          description: 'Reads test state.',
        }),
      ],
    });
    await expect(
      window.__lthnWebMcp.request({
        method: 'tools/call',
        params: {
          name: 'test_read_signal',
          arguments: { value: 7 },
        },
      }),
    ).resolves.toEqual({
      content: [
        {
          type: 'text',
          text: JSON.stringify({
            args: { value: 7 },
            aborted: false,
          }),
        },
      ],
    });

    runtimeHarness.dispatch('lthn:webmcp:call', {
      requestId: 'request-7',
      request: {
        method: 'tools/call',
        params: {
          name: 'test_read_signal',
          arguments: { via: 'event' },
        },
      },
    });
    await vi.waitFor(() => {
      expect(runtimeHarness.emit).toHaveBeenLastCalledWith('lthn:webmcp:result', {
        requestId: 'request-7',
        ok: true,
        result: {
          content: [
            {
              type: 'text',
              text: JSON.stringify({
                args: { via: 'event' },
                aborted: false,
              }),
            },
          ],
        },
      });
    });

    registration.abort();
    expect(window.__lthnWebMcp.listTools()).toEqual([]);
  });

  it('rejects duplicate and unknown tool names', async () => {
    const registration = new AbortController();
    const tool = {
      name: 'test_duplicate',
      description: 'Duplicate registration fixture.',
      inputSchema: { type: 'object', properties: {} },
      execute: () => 'ok',
    };

    await document.modelContext?.registerTool(tool, {
      signal: registration.signal,
    });
    await expect(document.modelContext?.registerTool(tool)).rejects.toThrow(
      'WebMCP tool already registered: test_duplicate',
    );
    await expect(window.__lthnWebMcp.callTool('missing_tool')).rejects.toThrow(
      'Unknown WebMCP tool: missing_tool',
    );

    registration.abort();
  });
});

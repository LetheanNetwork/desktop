type BridgeEmit = (name: string, payload: unknown) => void;
type ConsoleLevel = 'log' | 'info' | 'warn' | 'error' | 'debug';

interface WebMcpTool {
  readonly name: string;
  readonly description: string;
  readonly inputSchema: Record<string, unknown>;
  readonly execute: (args: Record<string, unknown>, client: { signal: AbortSignal }) => unknown;
}

interface WebMcpRegistrationOptions {
  readonly signal?: AbortSignal;
}

interface WebMcpModelContext {
  registerTool(tool: WebMcpTool, options?: WebMcpRegistrationOptions): Promise<void>;
}

interface WebMcpToolSummary {
  readonly name: string;
  readonly description: string;
  readonly inputSchema: Record<string, unknown>;
}

type WebMcpRequest =
  | { readonly method: 'tools/list' }
  | {
      readonly method: 'tools/call';
      readonly params: {
        readonly name: string;
        readonly arguments?: Record<string, unknown>;
      };
    };

interface LetheanWebMcpBridge {
  listTools(): WebMcpToolSummary[];
  callTool(name: string, args?: Record<string, unknown>): Promise<unknown>;
  request(request: WebMcpRequest): Promise<unknown>;
}

interface WebMcpEventRequest {
  readonly requestId: string;
  readonly request: WebMcpRequest;
}

declare global {
  interface Document {
    modelContext?: WebMcpModelContext;
  }

  interface Navigator {
    modelContext?: WebMcpModelContext;
  }

  interface Window {
    __lthnEmit: BridgeEmit;
    __lthnWebMcp: LetheanWebMcpBridge;
  }
}

const MAX_QUEUED_EVENTS = 256;

function installWebMcpBridge(): void {
  const tools = new Map<string, WebMcpTool>();
  const nativeContext = document.modelContext ?? navigator.modelContext;

  const summaries = (): WebMcpToolSummary[] =>
    [...tools.values()]
      .map(({ name, description, inputSchema }) => ({
        name,
        description,
        inputSchema,
      }))
      .sort((left, right) => left.name.localeCompare(right.name));

  const notifyToolsChanged = (): void => {
    window.__lthnEmit?.('lthn:webmcp:tools-changed', {
      tools: summaries(),
    });
  };

  const modelContext: WebMcpModelContext = {
    async registerTool(tool, options = {}): Promise<void> {
      if (!tool.name.trim()) throw new Error('WebMCP tool name is required.');
      if (tools.has(tool.name)) {
        throw new Error(`WebMCP tool already registered: ${tool.name}`);
      }

      tools.set(tool.name, tool);
      const unregister = (): void => {
        if (tools.get(tool.name) !== tool) return;
        tools.delete(tool.name);
        notifyToolsChanged();
      };
      options.signal?.addEventListener('abort', unregister, { once: true });

      try {
        if (
          nativeContext &&
          nativeContext !== modelContext &&
          typeof nativeContext.registerTool === 'function'
        ) {
          await nativeContext.registerTool(tool, options);
        }
      } catch (error) {
        unregister();
        throw error;
      }

      notifyToolsChanged();
    },
  };

  const callTool = async (name: string, args: Record<string, unknown> = {}): Promise<unknown> => {
    const tool = tools.get(name);
    if (!tool) throw new Error(`Unknown WebMCP tool: ${name}`);

    return await tool.execute(args, {
      signal: new AbortController().signal,
    });
  };

  window.__lthnWebMcp = Object.freeze({
    listTools: summaries,
    callTool,
    async request(request: WebMcpRequest): Promise<unknown> {
      if (request.method === 'tools/list') {
        return { tools: summaries() };
      }
      if (request.method === 'tools/call') {
        return await callTool(request.params.name, request.params.arguments ?? {});
      }
      throw new Error('Unsupported WebMCP request.');
    },
  });

  // Angular checks document first and navigator second. This is installed
  // synchronously, before bootstrapApplication imports app services.
  Object.defineProperty(document, 'modelContext', {
    configurable: true,
    value: modelContext,
  });
}

function formatConsoleArgs(args: unknown[]): string {
  return args
    .map((arg) => {
      if (arg === null || arg === undefined) return String(arg);
      if (typeof arg === 'string') return arg;
      if (arg instanceof Error) {
        return (
          `${arg.name || 'Error'}: ${arg.message || '(no message)'}` +
          (arg.stack ? `\n${arg.stack}` : '')
        );
      }
      if (typeof arg === 'object') {
        try {
          return JSON.stringify(arg);
        } catch {
          return String(arg);
        }
      }
      return String(arg);
    })
    .join(' ');
}

function installBridge(): void {
  const queue: Array<[name: string, payload: unknown]> = [];
  let emitter: BridgeEmit | null = null;

  const emit: BridgeEmit = (name, payload) => {
    if (emitter) {
      try {
        emitter(name, payload);
      } catch {
        // A missing/closing Wails runtime must not break the web app.
      }
      return;
    }
    if (queue.length < MAX_QUEUED_EVENTS) queue.push([name, payload]);
  };

  // ExecJS evaluates the Go bridge's wrapper as a classic script, so it cannot
  // import the bare Wails specifier itself. Publish this synchronously for the
  // wrapper's lthn:eval-reply event before Angular starts bootstrapping.
  window.__lthnEmit = emit;

  void import('@wailsio/runtime')
    .then((runtime) => {
      if (typeof runtime.Events?.Emit !== 'function') return;

      emitter = (name, payload) => {
        try {
          void runtime.Events.Emit(name, payload).catch(() => undefined);
        } catch {
          // Browser previews and a closing runtime stay silent.
        }
      };

      const pending = queue.splice(0);
      for (const [name, payload] of pending) emitter(name, payload);

      // The Wails MCP server can already emit and wait for window events.
      // Keep this request/reply path alongside direct js_eval so callers can
      // use whichever transport primitive their MCP client supports.
      if (typeof runtime.Events.On === 'function') {
        runtime.Events.On('lthn:webmcp:call', (event) => {
          const payload = event.data as Partial<WebMcpEventRequest> | null;
          if (!payload || typeof payload.requestId !== 'string' || !payload.request) {
            emit('lthn:webmcp:result', {
              requestId: payload?.requestId ?? '',
              ok: false,
              error: 'Invalid WebMCP event request.',
            });
            return;
          }

          void window.__lthnWebMcp
            .request(payload.request)
            .then((result) => {
              emit('lthn:webmcp:result', {
                requestId: payload.requestId,
                ok: true,
                result,
              });
            })
            .catch((error: unknown) => {
              emit('lthn:webmcp:result', {
                requestId: payload.requestId,
                ok: false,
                error: error instanceof Error ? error.message : String(error),
              });
            });
        });
      }
    })
    .catch(() => {
      // No Wails runtime in tests/browser previews: retain the bounded queue.
    });

  const bridgeConsole = console as unknown as Record<ConsoleLevel, (...args: unknown[]) => void>;
  const levels: readonly ConsoleLevel[] = ['log', 'info', 'warn', 'error', 'debug'];

  for (const level of levels) {
    const original = bridgeConsole[level].bind(console);
    bridgeConsole[level] = (...args: unknown[]) => {
      original(...args);
      emit('lthn:console', {
        level,
        message: formatConsoleArgs(args),
        source: 'console',
        at: new Date().toISOString(),
      });
    };
  }

  window.addEventListener('error', (event) => {
    emit('lthn:error', {
      message: event.message || 'unknown error',
      source: event.filename || '',
      line: event.lineno || 0,
      col: event.colno || 0,
      stack: event.error instanceof Error ? event.error.stack || '' : '',
      at: new Date().toISOString(),
    });
  });

  window.addEventListener('unhandledrejection', (event) => {
    const reason = event.reason;
    emit('lthn:error', {
      message:
        'unhandled promise rejection: ' +
        (reason instanceof Error ? reason.message : String(reason)),
      source: 'unhandledrejection',
      line: 0,
      col: 0,
      stack: reason instanceof Error ? reason.stack || '' : '',
      at: new Date().toISOString(),
    });
  });

  emit('lthn:console', {
    level: 'info',
    message: 'lthn shim ready (module emit path)',
    source: 'shim',
    at: new Date().toISOString(),
  });
}

if (typeof window !== 'undefined') {
  installBridge();
  installWebMcpBridge();
}

export {};

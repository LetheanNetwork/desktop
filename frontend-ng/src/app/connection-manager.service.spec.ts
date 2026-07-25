import { TestBed } from '@angular/core/testing';
import { Events, getTransport, setTransport } from '@wailsio/runtime';
import {
  CONNECTION_LOCATION,
  CONNECTION_MANAGER_OPTIONS,
  CONNECTION_SOCKET_FACTORY,
  ConnectionManagerService,
  type ConnectionLocation,
  type ConnectionSocket,
  type ConnectionSocketFactory,
} from './connection-manager.service';

class FakeSocket implements ConnectionSocket {
  readonly sent: string[] = [];
  readyState: number = WebSocket.CONNECTING;
  onopen: ((event: Event) => void) | null = null;
  onmessage: ((event: MessageEvent<string | Blob | ArrayBuffer>) => void) | null = null;
  onerror: ((event: Event) => void) | null = null;
  onclose: ((event: CloseEvent) => void) | null = null;

  send(data: string): void {
    this.sent.push(data);
  }

  close(): void {
    if (this.readyState === WebSocket.CLOSED) return;
    this.readyState = WebSocket.CLOSED;
    this.onclose?.(new CloseEvent('close', { code: 1000, reason: 'closed' }));
  }

  open(): void {
    this.readyState = WebSocket.OPEN;
    this.onopen?.(new Event('open'));
  }

  message(data: unknown): void {
    this.onmessage?.(new MessageEvent('message', { data: JSON.stringify(data) }));
  }

  drop(): void {
    this.readyState = WebSocket.CLOSED;
    this.onclose?.(new CloseEvent('close', { code: 1006, reason: 'network lost' }));
  }
}

describe('ConnectionManagerService', () => {
  let sockets: FakeSocket[];
  let socketURLs: string[];
  let service: ConnectionManagerService;

  const configure = (
    location: ConnectionLocation = {
      protocol: 'wails:',
      host: 'localhost',
      search: '',
    },
    options: Record<string, unknown> = {
      url: 'ws://localhost:9099/wails/ws',
      reconnectDelayMs: 25,
      requestTimeoutMs: 1_000,
    },
  ): void => {
    sockets = [];
    socketURLs = [];
    const factory: ConnectionSocketFactory = (url) => {
      socketURLs.push(url);
      const socket = new FakeSocket();
      sockets.push(socket);
      return socket;
    };
    TestBed.configureTestingModule({
      providers: [
        { provide: CONNECTION_SOCKET_FACTORY, useValue: factory },
        { provide: CONNECTION_LOCATION, useValue: location },
        { provide: CONNECTION_MANAGER_OPTIONS, useValue: options },
      ],
    });
    service = TestBed.inject(ConnectionManagerService);
  };

  afterEach(() => {
    service?.destroy();
    setTransport(null);
    window.localStorage.removeItem('lthn.websocket.url');
    vi.useRealTimers();
    TestBed.resetTestingModule();
  });

  it('installs itself as the Wails transport and connects to the desktop default', async () => {
    configure();
    await service.ready;

    expect(getTransport()).toBe(service);
    expect(service.url()).toBe('ws://localhost:9099/wails/ws');
    expect(socketURLs).toEqual(['ws://localhost:9099/wails/ws']);
    expect(service.state()).toBe('connecting');

    const connected = service.connect();
    sockets[0].open();
    await connected;

    expect(service.connected()).toBe(true);
    expect(service.state()).toBe('connected');
    expect(service.lastError()).toBe('');
  });

  it('resolves a secure same-origin proxy and appends the configured token', async () => {
    configure(
      {
        protocol: 'https:',
        host: 'desktop.lethean.example',
        search: '?lthn-ws=%2Fwails%2Fws&lthn-token=mobile-secret',
      },
      { reconnectDelayMs: 25, requestTimeoutMs: 1_000 },
    );
    await service.ready;

    expect(service.url()).toBe(
      'wss://desktop.lethean.example/wails/ws?access_token=mobile-secret',
    );
  });

  it('prefers a secure remote query endpoint over the browser origin', async () => {
    configure(
      {
        protocol: 'https:',
        host: 'ui.lethean.example',
        search: '?lthn-ws=wss%3A%2F%2Fapi.lethean.example%2Fwails%2Fws',
      },
      {},
    );
    await service.ready;

    expect(service.url()).toBe('wss://api.lethean.example/wails/ws');
    expect(socketURLs).toEqual(['wss://api.lethean.example/wails/ws']);
  });

  it('correlates successful and failed Wails runtime calls', async () => {
    configure();
    const connected = service.connect();
    sockets[0].open();
    await connected;

    const success = service.call(0, 12, 'app', { name: 'Ada' });
    await Promise.resolve();
    const request = JSON.parse(sockets[0].sent[0]) as {
      id: string;
      type: string;
      request: Record<string, unknown>;
    };
    expect(request.type).toBe('request');
    expect(request.request).toMatchObject({
      object: 0,
      method: 12,
      args: { name: 'Ada' },
      webviewWindowName: 'app',
    });
    expect(request.request).not.toHaveProperty('windowName');
    expect(typeof request.request['clientId']).toBe('string');

    sockets[0].message({
      id: request.id,
      type: 'response',
      response: { statusCode: 200, data: { greeting: 'Hello Ada' } },
    });
    await expect(success).resolves.toEqual({ greeting: 'Hello Ada' });

    const failed = service.call(0, 13, '', {});
    await Promise.resolve();
    const failedRequest = JSON.parse(sockets[0].sent[1]) as { id: string };
    sockets[0].message({
      id: failedRequest.id,
      type: 'response',
      response: { statusCode: 422, data: 'binding rejected the request' },
    });
    await expect(failed).rejects.toThrow('binding rejected the request');
  });

  it('dispatches Wails events and reconnects after an unexpected close', async () => {
    vi.useFakeTimers();
    configure();
    const connected = service.connect();
    sockets[0].open();
    await connected;

    const eventData: unknown[] = [];
    const off = Events.On('lthn:test', (event) => eventData.push(event.data));
    sockets[0].message({
      type: 'event',
      event: { name: 'lthn:test', data: { ready: true } },
    });
    expect(eventData).toEqual([{ ready: true }]);

    const pending = service.call(2, 1, '', {});
    await Promise.resolve();
    sockets[0].drop();
    await expect(pending).rejects.toThrow('connection closed');
    expect(service.state()).toBe('reconnecting');
    expect(service.reconnectAttempt()).toBe(1);

    await vi.advanceTimersByTimeAsync(25);
    expect(sockets).toHaveLength(2);
    sockets[1].open();
    await vi.runAllTimersAsync();
    expect(service.connected()).toBe(true);
    expect(service.reconnectAttempt()).toBe(0);
    off();
  });

  it('persists a configured URL without persisting its token', async () => {
    configure();
    const initial = service.connect();
    sockets[0].open();
    await initial;

    const configured = service.configure({
      url: 'wss://desktop.lethean.example/proxy/wails/ws?tenant=mobile',
      token: 'short-lived-secret',
    });
    expect(sockets).toHaveLength(2);
    sockets[1].open();
    await configured;

    expect(service.url()).toBe(
      'wss://desktop.lethean.example/proxy/wails/ws?tenant=mobile&access_token=short-lived-secret',
    );
    expect(window.localStorage.getItem('lthn.websocket.url')).toBe(
      'wss://desktop.lethean.example/proxy/wails/ws?tenant=mobile',
    );
    expect(window.localStorage.getItem('lthn.websocket.url')).not.toContain(
      'short-lived-secret',
    );
  });

  it('rejects credential-bearing and insecure remote URLs before opening a socket', async () => {
    configure(undefined, {
      url: 'wss://user:password@desktop.lethean.example/wails/ws',
    });
    await service.ready;

    expect(sockets).toHaveLength(0);
    expect(service.state()).toBe('disconnected');
    expect(service.lastError()).toContain('credentials');
    await expect(service.connect()).rejects.toThrow('credentials');

    service.destroy();
    TestBed.resetTestingModule();
    setTransport(null);

    configure(undefined, {
      url: 'ws://desktop.lethean.example/wails/ws',
    });
    await service.ready;
    expect(sockets).toHaveLength(0);
    expect(service.lastError()).toContain('wss');
  });

  it('bounds pending binding calls and times out unanswered requests', async () => {
    configure(undefined, {
      url: 'ws://localhost:9099/wails/ws',
      reconnectDelayMs: 25,
      requestTimeoutMs: 10,
      maxPendingRequests: 1,
    });
    const connected = service.connect();
    sockets[0].open();
    await connected;

    const first = service.call(0, 1, '', {});
    await Promise.resolve();
    await expect(service.call(0, 2, '', {})).rejects.toThrow('pending request limit');

    await expect(first).rejects.toThrow('timed out after 10ms');
    expect(sockets[0].sent).toHaveLength(1);
  });

  it('records malformed messages and suppresses reconnect after an explicit disconnect', async () => {
    configure();
    const connected = service.connect();
    sockets[0].open();
    await connected;

    sockets[0].onmessage?.(new MessageEvent('message', { data: '{not-json' }));
    await Promise.resolve();
    expect(service.lastError()).toContain('JSON');

    service.disconnect();
    await new Promise((resolve) => setTimeout(resolve, 35));
    expect(service.state()).toBe('disconnected');
    expect(sockets).toHaveLength(1);
  });
});

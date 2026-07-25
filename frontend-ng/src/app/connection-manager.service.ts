import { DOCUMENT } from '@angular/common';
import {
  InjectionToken,
  OnDestroy,
  Service,
  computed,
  inject,
  signal,
} from '@angular/core';
import {
  clientId,
  getTransport,
  setTransport,
  type RuntimeTransport,
} from '@wailsio/runtime';

const DEFAULT_WEBSOCKET_URL = 'ws://localhost:9099/wails/ws';
const DEFAULT_RECONNECT_DELAY_MS = 1_000;
const DEFAULT_MAX_RECONNECT_DELAY_MS = 30_000;
const DEFAULT_REQUEST_TIMEOUT_MS = 30_000;
const DEFAULT_CONNECTION_TIMEOUT_MS = 10_000;
const DEFAULT_MAX_PENDING_REQUESTS = 256;
const URL_STORAGE_KEY = 'lthn.websocket.url';

export type ConnectionState =
  | 'connecting'
  | 'connected'
  | 'reconnecting'
  | 'disconnected'
  | 'offline';

export interface ConnectionManagerOptions {
  readonly url?: string;
  readonly token?: string;
  readonly offline?: boolean;
  readonly reconnectDelayMs?: number;
  readonly maxReconnectDelayMs?: number;
  readonly requestTimeoutMs?: number;
  readonly connectionTimeoutMs?: number;
  readonly maxPendingRequests?: number;
}

export interface ConnectionLocation {
  readonly protocol: string;
  readonly host: string;
  readonly search: string;
}

export interface ConnectionSocket {
  readonly readyState: number;
  onopen: ((event: Event) => void) | null;
  onmessage: ((event: MessageEvent<string | Blob | ArrayBuffer>) => void) | null;
  onerror: ((event: Event) => void) | null;
  onclose: ((event: CloseEvent) => void) | null;
  send(data: string): void;
  close(code?: number, reason?: string): void;
}

export type ConnectionSocketFactory = (url: string) => ConnectionSocket;

export const CONNECTION_MANAGER_OPTIONS = new InjectionToken<ConnectionManagerOptions>(
  'CONNECTION_MANAGER_OPTIONS',
  {
    providedIn: 'root',
    factory: () => ({}),
  },
);

export const CONNECTION_LOCATION = new InjectionToken<ConnectionLocation>(
  'CONNECTION_LOCATION',
  {
    providedIn: 'root',
    factory: () => {
      const location = inject(DOCUMENT).defaultView?.location;
      return {
        protocol: location?.protocol ?? '',
        host: location?.host ?? '',
        search: location?.search ?? '',
      };
    },
  },
);

export const CONNECTION_SOCKET_FACTORY = new InjectionToken<ConnectionSocketFactory>(
  'CONNECTION_SOCKET_FACTORY',
  {
    providedIn: 'root',
    factory: () => (url) => new WebSocket(url),
  },
);

interface RuntimeConnectionConfiguration {
  readonly webSocketUrl?: string;
  readonly token?: string;
}

interface PendingRequest {
  readonly resolve: (value: unknown) => void;
  readonly reject: (reason: Error) => void;
  readonly timeout: ReturnType<typeof setTimeout>;
}

interface SocketEnvelope {
  readonly id?: string;
  readonly type?: string;
  readonly response?: {
    readonly statusCode?: number;
    readonly data?: unknown;
  };
  readonly event?: {
    readonly name: string;
    readonly data?: unknown;
    readonly sender?: string;
  };
}

type WailsWindow = Window & {
  readonly __LTHN_CONNECTION__?: RuntimeConnectionConfiguration;
  readonly _wails?: {
    readonly dispatchWailsEvent?: (event: SocketEnvelope['event']) => void;
  };
};

/**
 * Owns the Wails runtime connection independently of the native WebView.
 *
 * Generated Wails bindings continue to call RuntimeTransport.call(); this
 * service correlates those calls over one WebSocket and forwards server-push
 * events into the normal @wailsio/runtime Events dispatcher.
 */
@Service()
export class ConnectionManagerService implements RuntimeTransport, OnDestroy {
  private readonly document = inject(DOCUMENT);
  private readonly location = inject(CONNECTION_LOCATION);
  private readonly socketFactory = inject(CONNECTION_SOCKET_FACTORY);
  private readonly initialOptions = inject(CONNECTION_MANAGER_OPTIONS);

  private socket: ConnectionSocket | null = null;
  private connectPromise: Promise<void> | null = null;
  private reconnectTimer: ReturnType<typeof setTimeout> | null = null;
  private pending = new Map<string, PendingRequest>();
  private suppressReconnect = false;
  private destroyed = false;
  private reconnectDelayMs = DEFAULT_RECONNECT_DELAY_MS;
  private maxReconnectDelayMs = DEFAULT_MAX_RECONNECT_DELAY_MS;
  private requestTimeoutMs = DEFAULT_REQUEST_TIMEOUT_MS;
  private connectionTimeoutMs = DEFAULT_CONNECTION_TIMEOUT_MS;
  private maxPendingRequests = DEFAULT_MAX_PENDING_REQUESTS;
  private requestSequence = 0;
  private configurationError: Error | null = null;
  private previewOffline = false;

  private readonly _state = signal<ConnectionState>('disconnected');
  readonly state = this._state.asReadonly();
  readonly connected = computed(() => this._state() === 'connected');
  readonly offline = computed(() => this._state() === 'offline');

  private readonly _url = signal(DEFAULT_WEBSOCKET_URL);
  readonly url = this._url.asReadonly();

  private readonly _lastError = signal('');
  readonly lastError = this._lastError.asReadonly();

  private readonly _reconnectAttempt = signal(0);
  readonly reconnectAttempt = this._reconnectAttempt.asReadonly();

  readonly ready = this.initialise();

  connect(): Promise<void> {
    if (this.previewOffline) {
      this._state.set('offline');
      return Promise.reject(new Error('Offline preview mode is enabled.'));
    }
    if (this.destroyed) {
      return Promise.reject(new Error('The Wails connection manager has been destroyed.'));
    }
    if (this.configurationError) {
      return Promise.reject(this.configurationError);
    }
    if (this.socket?.readyState === WebSocket.OPEN) {
      return Promise.resolve();
    }
    if (this.connectPromise) {
      return this.connectPromise;
    }

    this.suppressReconnect = false;
    this.clearReconnectTimer();
    this._state.set(this._reconnectAttempt() > 0 ? 'reconnecting' : 'connecting');

    let socket: ConnectionSocket;
    try {
      socket = this.socketFactory(this._url());
    } catch (error) {
      const reason = asError(error, 'Unable to create the Wails WebSocket.');
      this.recordConnectionFailure(reason);
      this.scheduleReconnect();
      return Promise.reject(reason);
    }
    this.socket = socket;

    const attempt = new Promise<void>((resolve, reject) => {
      let settled = false;
      const settle = (callback: () => void): void => {
        if (settled) return;
        settled = true;
        clearTimeout(connectionTimeout);
        callback();
      };
      const connectionTimeout = setTimeout(() => {
        const reason = new Error(`Wails WebSocket connection timed out after ${this.connectionTimeoutMs}ms.`);
        settle(() => reject(reason));
        this.recordConnectionFailure(reason);
        if (this.socket === socket) {
          this.socket = null;
        }
        socket.close(1000, 'connection timeout');
        this.scheduleReconnect();
      }, this.connectionTimeoutMs);

      socket.onopen = () => {
        if (this.socket !== socket || this.destroyed) {
          socket.close(1000, 'stale connection');
          return;
        }
        settle(resolve);
        this._state.set('connected');
        this._lastError.set('');
        this._reconnectAttempt.set(0);
      };
      socket.onmessage = (event) => {
        void this.handleMessage(event.data);
      };
      socket.onerror = () => {
        const reason = new Error('The Wails WebSocket reported a connection error.');
        settle(() => reject(reason));
        this.recordConnectionFailure(reason);
        if (this.socket === socket) {
          this.socket = null;
        }
        if (socket.readyState !== WebSocket.CLOSED) {
          socket.close(1000, 'connection error');
        }
        this.scheduleReconnect();
      };
      socket.onclose = (event) => {
        if (this.socket === socket) {
          this.socket = null;
        }
        const detail = event.reason ? `: ${event.reason}` : '';
        const reason = new Error(`Wails WebSocket connection closed (${event.code}${detail}).`);
        settle(() => reject(reason));
        this.rejectPending(reason);
        if (!this.destroyed && !this.suppressReconnect) {
          this.recordConnectionFailure(reason);
          this.scheduleReconnect();
        } else if (!this.destroyed) {
          this._state.set('disconnected');
        }
      };
    });

    this.connectPromise = attempt;
    void attempt.finally(() => {
      if (this.connectPromise === attempt) {
        this.connectPromise = null;
      }
    }).catch(() => undefined);
    return attempt;
  }

  async call(
    objectID: number,
    method: number,
    windowName: string,
    args: unknown,
  ): Promise<unknown> {
    await this.connect();
    const socket = this.socket;
    if (!socket || socket.readyState !== WebSocket.OPEN) {
      throw new Error('The Wails WebSocket is not connected.');
    }
    if (this.pending.size >= this.maxPendingRequests) {
      throw new Error(
        `The Wails pending request limit (${this.maxPendingRequests}) has been reached.`,
      );
    }

    const id = `${clientId}-${++this.requestSequence}`;
    const message = {
      id,
      type: 'request',
      request: {
        object: objectID,
        method,
        args,
        webviewWindowName: windowName || undefined,
        clientId,
      },
    };

    return await new Promise<unknown>((resolve, reject) => {
      const timeout = setTimeout(() => {
        if (!this.pending.delete(id)) return;
        reject(new Error(`Wails binding request timed out after ${this.requestTimeoutMs}ms.`));
      }, this.requestTimeoutMs);
      this.pending.set(id, { resolve, reject, timeout });
      try {
        socket.send(JSON.stringify(message));
      } catch (error) {
        clearTimeout(timeout);
        this.pending.delete(id);
        reject(asError(error, 'Unable to send the Wails binding request.'));
      }
    });
  }

  async configure(options: ConnectionManagerOptions): Promise<void> {
    const current = this.connectionConfiguration(options);
    this.configurationError = null;
    this.applyConfiguration(current);
    this.persistURL(current.url);
    this.disconnect();
    if (this.previewOffline) return;
    await this.connect();
  }

  async reconnect(): Promise<void> {
    this.disconnect();
    await this.connect();
  }

  disconnect(): void {
    this.suppressReconnect = true;
    this.clearReconnectTimer();
    const socket = this.socket;
    this.socket = null;
    this.connectPromise = null;
    this.rejectPending(new Error('Wails WebSocket connection closed by the client.'));
    if (socket && socket.readyState !== WebSocket.CLOSED) {
      socket.close(1000, 'client disconnect');
    }
    if (!this.destroyed) {
      this._state.set(this.previewOffline ? 'offline' : 'disconnected');
    }
  }

  destroy(): void {
    if (this.destroyed) return;
    this.destroyed = true;
    this.disconnect();
    this._state.set('disconnected');
    if (getTransport() === this) {
      setTransport(null);
    }
  }

  ngOnDestroy(): void {
    this.destroy();
  }

  private initialise(): Promise<void> {
    // Install synchronously so every generated binding created by a later
    // application initializer sees the socket transport.
    setTransport(this);
    try {
      this.applyConfiguration(this.connectionConfiguration(this.initialOptions));
      this.configurationError = null;
      if (this.previewOffline) {
        this._state.set('offline');
      } else {
        void this.connect().catch(() => undefined);
      }
    } catch (error) {
      const reason = asError(error, 'The Wails WebSocket configuration is invalid.');
      this.configurationError = reason;
      this.recordConnectionFailure(reason);
    }
    // Offline startup is valid: binding calls await connect and the manager
    // continues reconnecting without holding Angular's bootstrap barrier.
    return Promise.resolve();
  }

  private connectionConfiguration(options: ConnectionManagerOptions): {
    readonly url: string;
    readonly offline: boolean;
    readonly reconnectDelayMs: number;
    readonly maxReconnectDelayMs: number;
    readonly requestTimeoutMs: number;
    readonly connectionTimeoutMs: number;
    readonly maxPendingRequests: number;
  } {
    const search = new URLSearchParams(this.location.search);
    const runtime = (this.document.defaultView as WailsWindow | null)
      ?.__LTHN_CONNECTION__;
    const storedURL = this.document.defaultView?.localStorage?.getItem(URL_STORAGE_KEY) ?? '';
    const selectedURL =
      clean(options.url) ||
      clean(search.get('lthn-ws')) ||
      clean(runtime?.webSocketUrl) ||
      clean(storedURL) ||
      this.sameOriginURL();
    const token =
      clean(options.token) ||
      clean(search.get('lthn-token')) ||
      clean(runtime?.token);

    return {
      url: authenticatedURL(withoutAccessToken(this.normaliseURL(selectedURL)), token),
      offline: options.offline ?? queryFlag(search.get('lthn-offline')),
      reconnectDelayMs: positiveInteger(options.reconnectDelayMs, DEFAULT_RECONNECT_DELAY_MS),
      maxReconnectDelayMs: positiveInteger(
        options.maxReconnectDelayMs,
        DEFAULT_MAX_RECONNECT_DELAY_MS,
      ),
      requestTimeoutMs: positiveInteger(
        options.requestTimeoutMs,
        DEFAULT_REQUEST_TIMEOUT_MS,
      ),
      connectionTimeoutMs: positiveInteger(
        options.connectionTimeoutMs,
        DEFAULT_CONNECTION_TIMEOUT_MS,
      ),
      maxPendingRequests: positiveInteger(
        options.maxPendingRequests,
        DEFAULT_MAX_PENDING_REQUESTS,
      ),
    };
  }

  private applyConfiguration(configuration: {
    readonly url: string;
    readonly offline: boolean;
    readonly reconnectDelayMs: number;
    readonly maxReconnectDelayMs: number;
    readonly requestTimeoutMs: number;
    readonly connectionTimeoutMs: number;
    readonly maxPendingRequests: number;
  }): void {
    this._url.set(configuration.url);
    this.previewOffline = configuration.offline;
    this.reconnectDelayMs = configuration.reconnectDelayMs;
    this.maxReconnectDelayMs = Math.max(
      configuration.reconnectDelayMs,
      configuration.maxReconnectDelayMs,
    );
    this.requestTimeoutMs = configuration.requestTimeoutMs;
    this.connectionTimeoutMs = configuration.connectionTimeoutMs;
    this.maxPendingRequests = configuration.maxPendingRequests;
  }

  private sameOriginURL(): string {
    if (this.location.protocol === 'https:') {
      return `wss://${this.location.host}/wails/ws`;
    }
    if (this.location.protocol === 'http:') {
      return `ws://${this.location.host}/wails/ws`;
    }
    return DEFAULT_WEBSOCKET_URL;
  }

  private normaliseURL(value: string): string {
    let parsed: URL;
    if (value.startsWith('/')) {
      if (this.location.protocol === 'https:' || this.location.protocol === 'http:') {
        const protocol = this.location.protocol === 'https:' ? 'wss:' : 'ws:';
        parsed = new URL(`${protocol}//${this.location.host}${value}`);
      } else {
        parsed = new URL(value, DEFAULT_WEBSOCKET_URL);
      }
    } else {
      parsed = new URL(value);
    }
    if (parsed.protocol === 'http:') parsed.protocol = 'ws:';
    if (parsed.protocol === 'https:') parsed.protocol = 'wss:';
    if (parsed.protocol !== 'ws:' && parsed.protocol !== 'wss:') {
      throw new Error('The Wails connection URL must use ws or wss.');
    }
    if (parsed.username || parsed.password) {
      throw new Error('The Wails connection URL must not contain credentials.');
    }
    if (parsed.hash) {
      throw new Error('The Wails connection URL must not contain a fragment.');
    }
    if (parsed.protocol === 'ws:' && !isLoopbackHost(parsed.hostname)) {
      throw new Error('Remote Wails connections must use wss.');
    }
    return parsed.toString();
  }

  private persistURL(value: string): void {
    try {
      this.document.defaultView?.localStorage?.setItem(
        URL_STORAGE_KEY,
        withoutAccessToken(value),
      );
    } catch {
      // Storage may be unavailable in hardened WebViews or private browsing.
    }
  }

  private async handleMessage(data: string | Blob | ArrayBuffer): Promise<void> {
    try {
      const text =
        typeof data === 'string'
          ? data
          : data instanceof Blob
            ? await data.text()
            : new TextDecoder().decode(data);
      const message = JSON.parse(text) as SocketEnvelope;
      if (message.type === 'response' && message.id) {
        this.handleResponse(message.id, message.response);
        return;
      }
      if (message.type === 'event' && message.event) {
        const target = this.document.defaultView as WailsWindow | null;
        target?._wails?.dispatchWailsEvent?.(message.event);
      }
    } catch (error) {
      this._lastError.set(asError(error, 'Invalid Wails WebSocket message.').message);
    }
  }

  private handleResponse(id: string, response: SocketEnvelope['response']): void {
    const pending = this.pending.get(id);
    if (!pending) return;
    this.pending.delete(id);
    clearTimeout(pending.timeout);
    if (!response) {
      pending.reject(new Error('Invalid Wails response: response envelope is missing.'));
      return;
    }
    if (response.statusCode === 200) {
      pending.resolve(response.data ?? undefined);
      return;
    }
    pending.reject(remoteError(response.data));
  }

  private recordConnectionFailure(error: Error): void {
    this._lastError.set(error.message);
    this._state.set('disconnected');
  }

  private scheduleReconnect(): void {
    if (
      this.destroyed ||
      this.previewOffline ||
      this.suppressReconnect ||
      this.reconnectTimer
    ) {
      return;
    }
    const attempt = this._reconnectAttempt() + 1;
    this._reconnectAttempt.set(attempt);
    this._state.set('reconnecting');
    const delay = Math.min(
      this.maxReconnectDelayMs,
      this.reconnectDelayMs * 2 ** Math.max(0, attempt - 1),
    );
    this.reconnectTimer = setTimeout(() => {
      this.reconnectTimer = null;
      void this.connect().catch(() => undefined);
    }, delay);
  }

  private clearReconnectTimer(): void {
    if (!this.reconnectTimer) return;
    clearTimeout(this.reconnectTimer);
    this.reconnectTimer = null;
  }

  private rejectPending(reason: Error): void {
    for (const request of this.pending.values()) {
      clearTimeout(request.timeout);
      request.reject(reason);
    }
    this.pending.clear();
  }
}

function clean(value: string | null | undefined): string {
  return value?.trim() ?? '';
}

function positiveInteger(value: number | undefined, fallback: number): number {
  return typeof value === 'number' && Number.isFinite(value) && value > 0
    ? Math.floor(value)
    : fallback;
}

function queryFlag(value: string | null): boolean {
  return ['1', 'true', 'yes'].includes(clean(value).toLocaleLowerCase('en-GB'));
}

function authenticatedURL(value: string, token: string): string {
  if (!token) return value;
  const parsed = new URL(value);
  parsed.searchParams.set('access_token', token);
  return parsed.toString();
}

function withoutAccessToken(value: string): string {
  const parsed = new URL(value);
  parsed.searchParams.delete('access_token');
  return parsed.toString();
}

function isLoopbackHost(hostname: string): boolean {
  const host = hostname.toLowerCase();
  return host === 'localhost' || host === '127.0.0.1' || host === '::1';
}

function remoteError(value: unknown): Error {
  if (typeof value === 'string') return new Error(value);
  if (value && typeof value === 'object') {
    const message = Reflect.get(value, 'message');
    if (typeof message === 'string') {
      return new Error(message, { cause: Reflect.get(value, 'cause') });
    }
  }
  return new Error('The Wails binding request failed.');
}

function asError(value: unknown, fallback: string): Error {
  return value instanceof Error ? value : new Error(fallback, { cause: value });
}

import { InjectionToken, PLATFORM_ID, Service, inject } from '@angular/core';
import { isPlatformBrowser } from '@angular/common';

const RUNNER_SERVICE = 'dappco.re/lthn/desktop/pkg/runner.Service';
const WAILS_CHAT_STREAM_METHOD = `${RUNNER_SERVICE}.WChatStream`;
const WAILS_ROUTES_METHOD = `${RUNNER_SERVICE}.WRoutes`;
const RUNNER_CHAT_DELTA = 'runner:chat:delta';
const RUNNER_CHAT_DONE = 'runner:chat:done';
const RUNNER_CHAT_FAILED = 'runner:chat:failed';

export type ChatRole = 'system' | 'user' | 'assistant';

export interface AiChatMessage {
  readonly role: ChatRole;
  readonly content: string;
}

export interface AiRoute {
  readonly name: string;
  readonly kind: string;
  readonly baseUrl: string;
  readonly model: string;
}

export interface AiToolCall {
  readonly id: string;
  readonly name: string;
  readonly status: 'pending' | 'running' | 'completed' | 'failed';
  readonly input?: unknown;
  readonly output?: unknown;
}

export type AiStreamEvent =
  | { readonly type: 'text'; readonly text: string }
  | { readonly type: 'tool-calls'; readonly toolCalls: AiToolCall[] }
  | { readonly type: 'welfare'; readonly reworded: boolean };

export interface DesktopAiTransport {
  call(method: string, args: readonly unknown[], signal: AbortSignal): Promise<unknown>;
  on(name: string, handler: (payload: unknown) => void): Promise<() => void>;
}

export const DESKTOP_AI_TRANSPORT = new InjectionToken<DesktopAiTransport>('DESKTOP_AI_TRANSPORT', {
  providedIn: 'root',
  factory: () => ({
    async call(method, args, signal): Promise<unknown> {
      const { Call } = await import('@wailsio/runtime');
      return await Call.ByName(method, ...args).cancelOn(signal);
    },
    async on(name, handler): Promise<() => void> {
      const { Events } = await import('@wailsio/runtime');
      return Events.On(name, (event) => handler(event.data));
    },
  }),
});

interface ResultEnvelope {
  readonly OK?: boolean;
  readonly ok?: boolean;
  readonly Value?: unknown;
  readonly value?: unknown;
}

interface ChatReply {
  readonly text: string;
  readonly warnUser: boolean;
  readonly toolCalls: AiToolCall[];
}

interface QueuedRunnerEvent {
  readonly name: string;
  readonly payload: unknown;
}

/**
 * Provider-neutral access to the desktop runner.
 *
 * WChat already routes through the configured Go provider router, which may
 * resolve to local lem, an OpenCode-provided route, or an external provider.
 * This service deliberately has no provider-specific branching.
 */
@Service()
export class DesktopAiService {
  private readonly platformId = inject(PLATFORM_ID);
  private readonly transport = inject(DESKTOP_AI_TRANSPORT);

  async listRoutes(signal: AbortSignal): Promise<AiRoute[]> {
    if (!isPlatformBrowser(this.platformId)) return [];

    const raw = await this.transport.call(WAILS_ROUTES_METHOD, [], signal);
    const value = unwrapResult(raw);
    if (!Array.isArray(value)) return [];

    return value.flatMap((candidate): AiRoute[] => {
      if (!isRecord(candidate) || typeof candidate['name'] !== 'string') {
        return [];
      }
      return [
        {
          name: candidate['name'],
          kind: typeof candidate['kind'] === 'string' ? candidate['kind'] : 'unknown',
          baseUrl: typeof candidate['base_url'] === 'string' ? candidate['base_url'] : '',
          model: typeof candidate['model'] === 'string' ? candidate['model'] : '',
        },
      ];
    });
  }

  /**
   * Relays the Go runner's correlated token events into Angular's streaming
   * resource API. WChatStream still returns its final ChatReply so a missing
   * terminal event can be reconciled without inventing synthetic chunks.
   */
  async *streamChat(
    messages: readonly AiChatMessage[],
    route: string,
    signal: AbortSignal,
  ): AsyncGenerator<AiStreamEvent> {
    if (!isPlatformBrowser(this.platformId)) {
      throw new Error('The desktop AI bridge is unavailable during rendering.');
    }

    const callId = nextStreamCallId();
    const queued: QueuedRunnerEvent[] = [];
    let resume: (() => void) | undefined;
    const enqueue = (name: string, payload: unknown): void => {
      queued.push({ name, payload });
      const wake = resume;
      resume = undefined;
      wake?.();
    };

    const off = await Promise.all([
      this.transport.on(RUNNER_CHAT_DELTA, (payload) => enqueue(RUNNER_CHAT_DELTA, payload)),
      this.transport.on(RUNNER_CHAT_DONE, (payload) => enqueue(RUNNER_CHAT_DONE, payload)),
      this.transport.on(RUNNER_CHAT_FAILED, (payload) => enqueue(RUNNER_CHAT_FAILED, payload)),
    ]);

    let rawReply: unknown;
    let callFailure: unknown;
    let callSettled = false;
    let terminalEvent = false;
    let terminalFailure: Error | undefined;
    let streamedText = '';
    let welfareEmitted = false;
    const wakeOnAbort = (): void => {
      const wake = resume;
      resume = undefined;
      wake?.();
    };
    signal.addEventListener('abort', wakeOnAbort, { once: true });

    const call = this.transport
      .call(WAILS_CHAT_STREAM_METHOD, [callId, messages, route], signal)
      .then(
        (raw) => {
          rawReply = raw;
        },
        (error: unknown) => {
          callFailure = error;
        },
      )
      .finally(() => {
        callSettled = true;
        const wake = resume;
        resume = undefined;
        wake?.();
      });

    try {
      while (!terminalEvent && !terminalFailure) {
        if (signal.aborted) throw abortError(signal);

        const event = queued.shift();
        if (!event) {
          if (callSettled) break;
          await new Promise<void>((resolve) => {
            resume = resolve;
          });
          continue;
        }

        const payload = runnerEventPayload(event.payload, callId);
        if (!payload) continue;
        if (event.name === RUNNER_CHAT_DELTA) {
          const delta = typeof payload['delta'] === 'string' ? payload['delta'] : '';
          if (!delta) continue;
          streamedText += delta;
          yield { type: 'text', text: delta };
          continue;
        }
        if (event.name === RUNNER_CHAT_FAILED) {
          terminalFailure = new Error(
            typeof payload['error'] === 'string' && payload['error'].trim()
              ? payload['error']
              : 'Desktop AI provider stream failed.',
          );
          continue;
        }

        const doneText = typeof payload['text'] === 'string' ? payload['text'] : '';
        const suffix = streamSuffix(streamedText, doneText);
        if (suffix) {
          streamedText += suffix;
          yield { type: 'text', text: suffix };
        }
        if (payload['warn_user'] === true) {
          welfareEmitted = true;
          yield { type: 'welfare', reworded: true };
        }
        terminalEvent = true;
      }

      await call;
      if (signal.aborted) throw abortError(signal);
      if (callFailure) throw toError(callFailure);
      if (terminalFailure) throw terminalFailure;

      const reply = normaliseReply(unwrapResult(rawReply));
      const suffix = streamSuffix(streamedText, reply.text);
      if (suffix) {
        streamedText += suffix;
        yield { type: 'text', text: suffix };
      }
      if (reply.warnUser && !welfareEmitted) {
        yield { type: 'welfare', reworded: true };
      }
      if (reply.toolCalls.length) {
        yield { type: 'tool-calls', toolCalls: reply.toolCalls };
      }
    } finally {
      signal.removeEventListener('abort', wakeOnAbort);
      for (const unsubscribe of off) unsubscribe();
    }
  }
}

function unwrapResult(raw: unknown): unknown {
  if (!isRecord(raw)) return raw;
  const envelope = raw as ResultEnvelope;
  if (typeof envelope.OK !== 'boolean' && typeof envelope.ok !== 'boolean') {
    return raw;
  }

  const ok = envelope.OK ?? envelope.ok ?? false;
  const value = Object.hasOwn(envelope, 'Value') ? envelope.Value : envelope.value;
  if (ok) return value;
  throw new Error(resultErrorMessage(value));
}

function resultErrorMessage(value: unknown): string {
  if (typeof value === 'string' && value.trim()) return value;
  if (isRecord(value)) {
    for (const key of ['error', 'message', 'Message']) {
      const candidate = value[key];
      if (typeof candidate === 'string' && candidate.trim()) return candidate;
    }
  }
  return 'Desktop AI provider call failed.';
}

function normaliseReply(value: unknown): ChatReply {
  if (typeof value === 'string') {
    if (!value.trim()) throw new Error('Empty reply from desktop AI provider.');
    return { text: value, warnUser: false, toolCalls: [] };
  }
  if (!isRecord(value)) {
    throw new Error('Invalid reply from desktop AI provider.');
  }

  const text = typeof value['text'] === 'string' ? value['text'] : '';
  if (!text.trim() && !Array.isArray(value['tool_calls'])) {
    throw new Error('Empty reply from desktop AI provider.');
  }

  return {
    text,
    warnUser: value['warn_user'] === true || value['warnUser'] === true,
    toolCalls: normaliseToolCalls(value['tool_calls'] ?? value['toolCalls']),
  };
}

function normaliseToolCalls(value: unknown): AiToolCall[] {
  if (!Array.isArray(value)) return [];

  return value.flatMap((candidate, index): AiToolCall[] => {
    if (!isRecord(candidate)) return [];
    const fn = isRecord(candidate['function']) ? candidate['function'] : {};
    const name =
      typeof candidate['name'] === 'string'
        ? candidate['name']
        : typeof fn['name'] === 'string'
          ? fn['name']
          : '';
    if (!name) return [];

    const rawStatus = candidate['status'];
    const status = isToolStatus(rawStatus) ? rawStatus : 'completed';
    return [
      {
        id: typeof candidate['id'] === 'string' ? candidate['id'] : `tool-${index + 1}`,
        name,
        status,
        input: candidate['input'] ?? candidate['arguments'] ?? fn['arguments'],
        output: candidate['output'] ?? candidate['result'],
      },
    ];
  });
}

function runnerEventPayload(value: unknown, callId: string): Record<string, unknown> | undefined {
  if (!isRecord(value) || value['call_id'] !== callId) return undefined;
  return value;
}

function streamSuffix(current: string, complete: string): string {
  if (!complete || complete === current) return '';
  if (!complete.startsWith(current)) {
    throw new Error('Desktop AI stream did not match its final reply.');
  }
  return complete.slice(current.length);
}

function abortError(signal: AbortSignal): Error {
  return signal.reason instanceof Error
    ? signal.reason
    : new DOMException('The AI request was cancelled.', 'AbortError');
}

let streamSequence = 0;

function nextStreamCallId(): string {
  streamSequence += 1;
  return `chat-${Date.now()}-${streamSequence}`;
}

function toError(value: unknown): Error {
  return value instanceof Error ? value : new Error(String(value));
}

function isToolStatus(value: unknown): value is AiToolCall['status'] {
  return value === 'pending' || value === 'running' || value === 'completed' || value === 'failed';
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null;
}

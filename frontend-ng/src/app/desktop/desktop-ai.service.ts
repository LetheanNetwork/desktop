import { InjectionToken, PLATFORM_ID, Service, inject } from '@angular/core';
import { isPlatformBrowser } from '@angular/common';

const RUNNER_SERVICE = 'dappco.re/lthn/desktop/pkg/runner.Service';
const WAILS_CHAT_METHOD = `${RUNNER_SERVICE}.WChat`;
const WAILS_ROUTES_METHOD = `${RUNNER_SERVICE}.WRoutes`;

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
}

export const DESKTOP_AI_TRANSPORT = new InjectionToken<DesktopAiTransport>('DESKTOP_AI_TRANSPORT', {
  providedIn: 'root',
  factory: () => ({
    async call(method, args, signal): Promise<unknown> {
      const { Call } = await import('@wailsio/runtime');
      return await Call.ByName(method, ...args).cancelOn(signal);
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
   * Emits a stream-shaped sequence for Angular's streaming resource API.
   *
   * The current Go WChat binding is a one-shot RPC. Once it resolves, this
   * adapter progressively presents bounded text chunks. A future streaming
   * Wails binding can replace this method without changing the chat surface.
   */
  async *streamChat(
    messages: readonly AiChatMessage[],
    signal: AbortSignal,
  ): AsyncGenerator<AiStreamEvent> {
    if (!isPlatformBrowser(this.platformId)) {
      throw new Error('The desktop AI bridge is unavailable during rendering.');
    }

    const raw = await this.transport.call(WAILS_CHAT_METHOD, [messages], signal);
    const reply = normaliseReply(unwrapResult(raw));

    if (reply.warnUser) {
      yield { type: 'welfare', reworded: true };
    }
    if (reply.toolCalls.length) {
      yield { type: 'tool-calls', toolCalls: reply.toolCalls };
    }

    const chunks = textChunks(reply.text);
    for (let index = 0; index < chunks.length; index++) {
      if (signal.aborted) throw abortError(signal);
      if (index > 0) await nextPaint(signal);
      yield { type: 'text', text: chunks[index] };
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

function textChunks(text: string): string[] {
  if (!text) return [];
  const words = text.match(/\S+\s*/g) ?? [text];
  const wordsPerChunk = Math.max(1, Math.ceil(words.length / 32));
  const chunks: string[] = [];
  for (let index = 0; index < words.length; index += wordsPerChunk) {
    chunks.push(words.slice(index, index + wordsPerChunk).join(''));
  }
  return chunks;
}

function nextPaint(signal: AbortSignal): Promise<void> {
  return new Promise((resolve, reject) => {
    const onAbort = () => {
      clearTimeout(timer);
      reject(abortError(signal));
    };
    const timer = setTimeout(() => {
      signal.removeEventListener('abort', onAbort);
      resolve();
    }, 16);
    signal.addEventListener('abort', onAbort, { once: true });
  });
}

function abortError(signal: AbortSignal): Error {
  return signal.reason instanceof Error
    ? signal.reason
    : new DOMException('The AI request was cancelled.', 'AbortError');
}

function isToolStatus(value: unknown): value is AiToolCall['status'] {
  return value === 'pending' || value === 'running' || value === 'completed' || value === 'failed';
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null;
}

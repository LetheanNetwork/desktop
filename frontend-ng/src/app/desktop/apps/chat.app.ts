import { CommonModule } from '@angular/common';
import {
  ChangeDetectionStrategy,
  Component,
  CUSTOM_ELEMENTS_SCHEMA,
  Input,
  ResourceStreamItem,
  Signal,
  computed,
  declareExperimentalWebMcpTool,
  inject,
  resource,
  signal,
} from '@angular/core';
import { AiChatMessage, AiRoute, AiToolCall, DesktopAiService } from '../desktop-ai.service';
import { Win } from '../desktop.data';
import { AppView } from './app-view';

type ThreadRole = 'user' | 'assistant';

interface ThreadMessage {
  readonly id: string;
  readonly role: ThreadRole;
  readonly content: string;
  readonly toolCalls: readonly AiToolCall[];
  readonly reworded?: boolean;
  readonly pending?: boolean;
}

interface ChatStreamValue {
  readonly requestId: string | null;
  readonly text: string;
  readonly toolCalls: readonly AiToolCall[];
  readonly reworded: boolean;
}

interface PendingCompletion {
  settled: boolean;
  resolve(message: ThreadMessage): void;
  reject(error: Error): void;
}

interface ChatRequest {
  readonly id: string;
  readonly history: readonly AiChatMessage[];
  readonly route: string;
  readonly completion: PendingCompletion;
}

const EMPTY_STREAM: ChatStreamValue = {
  requestId: null,
  text: '',
  toolCalls: [],
  reworded: false,
};

let messageSequence = 0;

@Component({
  selector: 'lthn-chat-app',
  standalone: true,
  imports: [CommonModule],
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  host: { style: 'display: contents' },
  changeDetection: ChangeDetectionStrategy.Eager,
  styles: `
    .chat {
      grid-template-rows: minmax(0, 1fr) auto;
    }
    .thread {
      min-height: 0;
    }
    .thread-empty {
      margin: auto;
      max-width: 340px;
      text-align: center;
      color: var(--fg-3);
      font-size: 13px;
      line-height: 1.55;
    }
    .thread-empty lthn-icon {
      display: block;
      margin-bottom: 10px;
      color: var(--brand-300);
    }
    .msg .bd {
      white-space: pre-wrap;
      overflow-wrap: anywhere;
    }
    .pending-copy {
      color: var(--fg-3);
      font-style: italic;
    }
    .welfare-note,
    .thread-error,
    .thread-note {
      margin-top: 7px;
      font-size: 11px;
      color: var(--fg-3);
    }
    .thread-error {
      align-self: center;
      color: var(--error-300);
      background: color-mix(in oklch, var(--error-500) 10%, var(--ink-2));
      border: 1px solid color-mix(in oklch, var(--error-500) 28%, transparent);
      border-radius: 8px;
      padding: 8px 11px;
    }
    .thread-note {
      align-self: center;
    }
    .tool-calls {
      display: grid;
      gap: 7px;
      margin-top: 8px;
    }
    .tool-call {
      min-width: 220px;
      padding: 8px 9px;
      border: 1px solid var(--line-2);
      border-radius: 7px;
      background: var(--ink-1);
      text-align: left;
    }
    .tool-head {
      display: flex;
      align-items: center;
      gap: 8px;
      font-family: var(--font-mono);
      font-size: 10.5px;
      color: var(--fg-1);
    }
    .tool-status {
      margin-left: auto;
      color: var(--fg-3);
      text-transform: uppercase;
      letter-spacing: 0.08em;
      font-size: 9px;
    }
    .tool-call pre {
      margin: 7px 0 0;
      max-height: 120px;
      overflow: auto;
      white-space: pre-wrap;
      font: 10.5px/1.45 var(--font-mono);
      color: var(--fg-2);
    }
    .provider-summary {
      font-size: 12px;
      color: var(--fg-1);
      line-height: 1.4;
    }
    .provider-route {
      display: grid;
      gap: 2px;
      padding: 7px 8px;
      border: 1px solid var(--line-1);
      border-radius: 7px;
      background: var(--ink-1);
    }
    .provider-route b {
      font-size: 11.5px;
      color: var(--fg-1);
      font-weight: 550;
    }
    .provider-route span,
    .rail-copy {
      font: 10.5px/1.4 var(--font-mono);
      color: var(--fg-3);
      overflow-wrap: anywhere;
    }
    .provider-select {
      width: 100%;
      min-width: 0;
      padding: 7px 26px 7px 8px;
      border: 1px solid var(--line-2);
      border-radius: 7px;
      background: var(--ink-1);
      color: var(--fg-1);
      font: 11px var(--font-mono);
    }
    .provider-select:focus {
      outline: 1px solid var(--brand-400);
      border-color: var(--brand-400);
    }
    .rail-action {
      padding: 6px 8px;
      border: 1px solid var(--line-2);
      border-radius: 6px;
      background: transparent;
      color: var(--fg-2);
      font: 11px var(--font);
      cursor: pointer;
    }
    .composer textarea {
      flex: 1;
      min-height: 38px;
      max-height: 120px;
      resize: vertical;
      background: var(--ink-0);
      border: 1px solid var(--line-2);
      border-radius: 8px;
      color: var(--fg-1);
      font: 13px/1.45 var(--font);
      padding: 9px 12px;
    }
    .composer textarea:focus {
      outline: 1px solid var(--brand-400);
      border-color: var(--brand-400);
    }
    .composer-actions {
      display: flex;
      align-items: flex-end;
      gap: 7px;
    }
  `,
  template: `
    <div class="chat">
      <section
        class="thread"
        aria-live="polite"
        [attr.aria-busy]="sending()"
        aria-label="Chat message thread"
      >
        <div class="thread-empty" *ngIf="!renderedMessages().length">
          <lthn-icon name="comments" size="28"></lthn-icon>
          <div i18n="Empty chat heading@@chat.empty.heading">
            Start a conversation with the configured desktop AI provider.
          </div>
        </div>

        <article
          class="msg"
          *ngFor="let message of renderedMessages(); trackBy: trackMessage"
          [class.me]="message.role === 'user'"
          [attr.data-message-id]="message.id"
        >
          <div class="who">
            {{ message.role === 'user' ? userLabel : assistantLabel() }}
          </div>
          <div class="bd">
            <span *ngIf="message.content">{{ message.content }}</span>
            <span
              class="pending-copy"
              *ngIf="message.pending && !message.content"
              i18n="Chat response pending@@chat.response.pending"
              >Thinking…</span
            >
            <div class="tool-calls" *ngIf="message.toolCalls.length">
              <div
                class="tool-call"
                *ngFor="let tool of message.toolCalls; trackBy: trackTool"
                [attr.data-status]="tool.status"
              >
                <div class="tool-head">
                  <lthn-icon name="wrench" size="11"></lthn-icon>
                  <span>{{ tool.name }}</span>
                  <span class="tool-status">{{ tool.status }}</span>
                </div>
                <pre *ngIf="tool.input !== undefined">{{ formatValue(tool.input) }}</pre>
                <pre *ngIf="tool.output !== undefined">{{ formatValue(tool.output) }}</pre>
              </div>
            </div>
          </div>
          <div
            class="welfare-note"
            *ngIf="message.reworded"
            i18n="Welfare rewording notice@@chat.welfare.reworded"
          >
            The desktop welfare layer reworded this request before routing it.
          </div>
        </article>

        <div class="thread-error" role="alert" *ngIf="lastError()">
          {{ lastError() }}
        </div>
        <div class="thread-note" role="status" *ngIf="notice()">
          {{ notice() }}
        </div>
      </section>

      <aside class="crail" aria-label="AI provider status">
        <span class="lab" i18n="Provider routing label@@chat.provider.routing"
          >Provider routing</span
        >
        <div class="provider-summary">{{ providerSummary() }}</div>
        <select
          class="provider-select"
          [value]="selectedRoute()"
          (change)="selectProvider($event)"
          aria-label="Provider route"
          i18n-aria-label="Provider route selector@@chat.provider.selector"
          [disabled]="providerRoutes.isLoading()"
        >
          <option value="" i18n="Automatic provider route option@@chat.provider.autoOption">
            Automatic
          </option>
          <option
            *ngFor="let route of providerRoutes.value(); trackBy: trackRoute"
            [value]="route.name"
          >
            {{ route.name }} · {{ route.model || 'default model' }}
          </option>
        </select>
        <lthn-badge [attr.variant]="providerRoutes.error() ? 'warn' : 'ok'">
          {{ providerRoutes.error() ? 'bridge unavailable' : selectedRoute() || 'automatic' }}
        </lthn-badge>

        <ng-container *ngFor="let route of providerRoutes.value(); trackBy: trackRoute">
          <div class="provider-route">
            <b>{{ route.name }}</b>
            <span>{{ route.kind }} · {{ route.model || 'default model' }}</span>
          </div>
        </ng-container>
        <button
          class="rail-action"
          type="button"
          *ngIf="providerRoutes.error()"
          (click)="providerRoutes.reload()"
          i18n="Retry provider bridge action@@chat.provider.retry"
        >
          Retry bridge
        </button>

        <lthn-divider></lthn-divider>
        <span class="lab" i18n="Chat thread label@@chat.thread.label">Thread</span>
        <div class="rail-copy">
          {{ messages().length }}
          {{ messages().length === 1 ? 'message' : 'messages' }}
        </div>
        <button
          class="rail-action"
          type="button"
          (click)="clearThread()"
          [disabled]="!messages().length && !sending()"
          i18n="Clear chat thread action@@chat.thread.clear"
        >
          Clear thread
        </button>
      </aside>

      <div class="composer">
        <textarea
          [value]="draft()"
          (input)="updateDraft($event)"
          (keydown)="composerKeydown($event)"
          placeholder="Message the configured provider…"
          i18n-placeholder="Chat composer placeholder@@chat.messagePlaceholder"
          aria-label="Message"
          i18n-aria-label="Chat message input label@@chat.messageInputLabel"
          [disabled]="sending()"
          rows="1"
        ></textarea>
        <div class="composer-actions">
          <lthn-button
            *ngIf="sending()"
            variant="secondary"
            (click)="stopResponse()"
            i18n="Stop chat response action@@chat.stop"
            >Stop</lthn-button
          >
          <lthn-button
            variant="primary"
            icon-trailing="arrow-up"
            (click)="sendComposer()"
            [attr.disabled]="sending() || !draft().trim() ? '' : null"
            i18n="Send chat message action@@chat.send"
            >Send</lthn-button
          >
        </div>
      </div>
    </div>
  `,
})
export class ChatApp implements AppView {
  @Input() win!: Win;

  private readonly ai = inject(DesktopAiService);
  private readonly activeRequest = signal<ChatRequest | undefined>(undefined);
  private readonly mcpTools = this.registerMcpTools();

  readonly draft = signal('');
  readonly messages = signal<ThreadMessage[]>([]);
  readonly selectedRoute = signal('');
  readonly lastError = signal<string | null>(null);
  readonly notice = signal<string | null>(null);
  readonly sending = computed(() => this.activeRequest() !== undefined);

  readonly providerRoutes = resource<AiRoute[], undefined>({
    defaultValue: [],
    loader: ({ abortSignal }) => this.ai.listRoutes(abortSignal),
  });

  readonly response = resource<ChatStreamValue, ChatRequest | undefined>({
    params: () => this.activeRequest(),
    defaultValue: EMPTY_STREAM,
    stream: ({ params, abortSignal }) => {
      const streamItem = signal<ResourceStreamItem<ChatStreamValue>>({
        value: {
          requestId: params.id,
          text: '',
          toolCalls: [],
          reworded: false,
        },
      });
      void this.consumeResponse(params, abortSignal, streamItem);
      return streamItem;
    },
  });

  readonly renderedMessages = computed<ThreadMessage[]>(() => {
    const active = this.activeRequest();
    if (!active) return this.messages();
    const current = this.response.value();
    const stream =
      current.requestId === active.id ? current : { ...EMPTY_STREAM, requestId: active.id };
    return [
      ...this.messages(),
      {
        id: `stream-${active.id}`,
        role: 'assistant',
        content: stream.text,
        toolCalls: stream.toolCalls,
        reworded: stream.reworded,
        pending: true,
      },
    ];
  });

  readonly providerSummary = computed(() => {
    if (this.providerRoutes.isLoading()) return 'Reading configured routes…';
    if (this.providerRoutes.error()) return 'Desktop runner is not reachable.';
    const routes = this.providerRoutes.value();
    if (!routes.length) return 'No provider routes are configured.';
    const selected = this.selectedRoute();
    if (selected) {
      const route = routes.find((candidate) => candidate.name === selected);
      return route
        ? `Routing this request through ${route.name} (${route.model || 'default model'}).`
        : `Routing this request through ${selected}.`;
    }
    return `${routes.length} configured route${routes.length === 1 ? '' : 's'}; Go selects the route.`;
  });

  readonly userLabel = $localize`:Current user label@@chat.participant.you:You`;
  readonly assistantLabel = computed(() => {
    const routes = this.providerRoutes.value();
    const selected = this.selectedRoute();
    if (selected) {
      const route = routes.find((candidate) => candidate.name === selected);
      return route?.model || route?.name || selected;
    }
    return routes.length === 1
      ? routes[0].model || routes[0].name
      : $localize`:Automatic provider label@@chat.provider.automatic:Assistant`;
  });

  updateDraft(event: Event): void {
    this.draft.set((event.target as HTMLTextAreaElement).value);
  }

  selectProvider(event: Event): void {
    this.selectedRoute.set((event.target as HTMLSelectElement).value);
  }

  composerKeydown(event: KeyboardEvent): void {
    if (event.key !== 'Enter' || (!event.metaKey && !event.ctrlKey)) return;
    event.preventDefault();
    this.sendComposer();
  }

  sendComposer(): void {
    void this.sendPrompt(this.draft()).catch(() => undefined);
  }

  sendPrompt(prompt: string, routeOverride = this.selectedRoute()): Promise<ThreadMessage> {
    const content = prompt.trim();
    if (!content) return Promise.reject(new Error('Prompt is required.'));
    if (this.activeRequest()) {
      return Promise.reject(new Error('Wait for the active response or stop it before sending.'));
    }

    this.lastError.set(null);
    this.notice.set(null);
    this.draft.set('');
    const userMessage: ThreadMessage = {
      id: nextMessageId('user'),
      role: 'user',
      content,
      toolCalls: [],
    };
    const nextMessages = [...this.messages(), userMessage];
    this.messages.set(nextMessages);

    return new Promise<ThreadMessage>((resolve, reject) => {
      this.activeRequest.set({
        id: nextMessageId('request'),
        route: routeOverride.trim(),
        history: nextMessages.map(({ role, content: text }) => ({
          role,
          content: text,
        })),
        completion: { settled: false, resolve, reject },
      });
    });
  }

  stopResponse(): void {
    const active = this.activeRequest();
    if (!active) return;
    const error = new DOMException('Response stopped.', 'AbortError');
    this.activeRequest.set(undefined);
    this.reject(active.completion, error);
    this.notice.set('Response stopped.');
  }

  clearThread(): void {
    this.stopResponse();
    this.messages.set([]);
    this.lastError.set(null);
    this.notice.set(null);
  }

  trackMessage(_index: number, message: ThreadMessage): string {
    return message.id;
  }

  trackTool(_index: number, tool: AiToolCall): string {
    return tool.id;
  }

  trackRoute(_index: number, route: AiRoute): string {
    return route.name;
  }

  formatValue(value: unknown): string {
    if (typeof value === 'string') return value;
    try {
      return JSON.stringify(value, null, 2);
    } catch {
      return String(value);
    }
  }

  private async consumeResponse(
    request: ChatRequest,
    abortSignal: AbortSignal,
    streamItem: Signal<ResourceStreamItem<ChatStreamValue>> & {
      set(value: ResourceStreamItem<ChatStreamValue>): void;
    },
  ): Promise<void> {
    let current: ChatStreamValue = {
      requestId: request.id,
      text: '',
      toolCalls: [],
      reworded: false,
    };

    try {
      for await (const event of this.ai.streamChat(request.history, request.route, abortSignal)) {
        if (event.type === 'text') {
          current = { ...current, text: current.text + event.text };
        } else if (event.type === 'tool-calls') {
          current = { ...current, toolCalls: event.toolCalls };
        } else {
          current = { ...current, reworded: event.reworded };
        }
        streamItem.set({ value: current });
      }

      if (!current.text.trim() && !current.toolCalls.length) {
        throw new Error('Empty reply from desktop AI provider.');
      }
      if (abortSignal.aborted || this.activeRequest()?.id !== request.id) {
        throw abortFailure(abortSignal);
      }

      const assistant: ThreadMessage = {
        id: nextMessageId('assistant'),
        role: 'assistant',
        content: current.text,
        toolCalls: current.toolCalls,
        reworded: current.reworded,
      };
      streamItem.set({ value: current });
      this.messages.update((messages) => [...messages, assistant]);
      this.resolve(request.completion, assistant);
      this.activeRequest.set(undefined);
    } catch (caught) {
      const error = toError(caught);
      streamItem.set({ error });
      if (this.activeRequest()?.id === request.id) {
        this.lastError.set(error.message);
        this.activeRequest.set(undefined);
      }
      this.reject(request.completion, error);
    }
  }

  private async registerMcpTools(): Promise<void> {
    await Promise.all([
      declareExperimentalWebMcpTool({
        name: 'chat_read_thread',
        description:
          'Reads the Chat app thread, provider-routing mode, and configured redacted routes.',
        inputSchema: {
          type: 'object',
          properties: {},
          additionalProperties: false,
        },
        execute: () => ({
          content: [
            {
              type: 'text',
              text: JSON.stringify({
                provider_mode: this.selectedRoute() || 'automatic',
                routes: this.providerRoutes.value(),
                sending: this.sending(),
                messages: this.messages(),
              }),
            },
          ],
        }),
      }),
      declareExperimentalWebMcpTool({
        name: 'chat_send_prompt',
        description:
          'Sends a prompt through the desktop configured AI provider and returns the assistant response.',
        inputSchema: {
          type: 'object',
          properties: {
            prompt: {
              type: 'string',
              minLength: 1,
              maxLength: 32768,
              description: 'Prompt to append to the current Chat thread.',
            },
            route: {
              type: 'string',
              maxLength: 128,
              description:
                'Optional live provider route name. Empty keeps automatic Go fallback routing.',
            },
          },
          required: ['prompt'],
          additionalProperties: false,
        },
        execute: async ({ prompt, route = '' }) => {
          if (!prompt.trim() || prompt.length > 32768) {
            throw new Error('Prompt must contain 1 to 32768 characters.');
          }
          if (route.length > 128) {
            throw new Error('Provider route must not exceed 128 characters.');
          }
          const assistant = await this.sendPrompt(prompt, route || this.selectedRoute());
          return {
            content: [
              {
                type: 'text',
                text: JSON.stringify({
                  response: assistant.content,
                  tool_calls: assistant.toolCalls,
                  welfare_reworded: assistant.reworded ?? false,
                }),
              },
            ],
          };
        },
      }),
      declareExperimentalWebMcpTool({
        name: 'chat_clear_thread',
        description: 'Stops any active Chat response and clears the in-memory message thread.',
        inputSchema: {
          type: 'object',
          properties: {},
          additionalProperties: false,
        },
        execute: () => {
          this.clearThread();
          return {
            content: [
              {
                type: 'text',
                text: JSON.stringify({ ok: true }),
              },
            ],
          };
        },
      }),
    ]);
  }

  private resolve(completion: PendingCompletion, message: ThreadMessage): void {
    if (completion.settled) return;
    completion.settled = true;
    completion.resolve(message);
  }

  private reject(completion: PendingCompletion, error: Error): void {
    if (completion.settled) return;
    completion.settled = true;
    completion.reject(error);
  }
}

function nextMessageId(prefix: string): string {
  messageSequence += 1;
  return `${prefix}-${Date.now()}-${messageSequence}`;
}

function abortFailure(signal: AbortSignal): Error {
  return signal.reason instanceof Error
    ? signal.reason
    : new DOMException('Response stopped.', 'AbortError');
}

function toError(value: unknown): Error {
  return value instanceof Error ? value : new Error(String(value));
}

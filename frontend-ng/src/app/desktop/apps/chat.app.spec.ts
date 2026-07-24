import { TestBed } from '@angular/core/testing';
import { AiChatMessage, AiStreamEvent, DesktopAiService } from '../desktop-ai.service';
import { Win } from '../desktop.data';
import { ChatApp } from './chat.app';

interface RegisteredTool {
  name: string;
  execute: (args: Record<string, string>, client: { signal: AbortSignal }) => unknown;
}

const chatWin: Win = {
  id: 'w-chat',
  app: 'chat',
  sub: '',
  systab: '',
  x: 0,
  y: 0,
  w: 720,
  h: 520,
  z: 1,
  min: false,
  max: false,
};

describe('ChatApp', () => {
  const registered = new Map<string, RegisteredTool>();
  const ai = {
    listRoutes: vi.fn(async () => [
      {
        name: 'automatic-local',
        kind: 'openai',
        baseUrl: 'http://127.0.0.1:1988/v1',
        model: 'lem',
      },
    ]),
    streamChat: vi.fn(async function* (
      messages: readonly AiChatMessage[],
      _route: string,
    ): AsyncGenerator<AiStreamEvent> {
      const prompt = messages.at(-1)?.content ?? '';
      if (prompt === 'fail') throw new Error('provider unavailable');
      yield {
        type: 'tool-calls',
        toolCalls: [
          {
            id: 'tool-1',
            name: 'desktop_list_windows',
            status: 'completed',
            input: {},
            output: { windows: 1 },
          },
        ],
      };
      yield { type: 'text', text: 'Done ' };
      yield { type: 'text', text: 'through the configured route.' };
    }),
  };

  beforeEach(() => {
    registered.clear();
    vi.clearAllMocks();
    Object.defineProperty(document, 'modelContext', {
      configurable: true,
      value: {
        registerTool: vi.fn(async (tool: RegisteredTool, options?: { signal?: AbortSignal }) => {
          registered.set(tool.name, tool);
          options?.signal?.addEventListener('abort', () => registered.delete(tool.name), {
            once: true,
          });
        }),
      },
    });
    TestBed.configureTestingModule({
      providers: [{ provide: DesktopAiService, useValue: ai }],
    });
  });

  afterEach(() => TestBed.resetTestingModule());

  async function createChat() {
    const fixture = TestBed.createComponent(ChatApp);
    fixture.componentRef.setInput('win', chatWin);
    fixture.detectChanges();
    await fixture.whenStable();
    fixture.detectChanges();
    return fixture;
  }

  it('streams a provider response into history and renders tool calls', async () => {
    const fixture = await createChat();
    const chat = fixture.componentInstance;
    chat.selectedRoute.set('automatic-local');

    const assistant = await chat.sendPrompt('List the windows');
    await fixture.whenStable();
    fixture.detectChanges();

    expect(assistant.content).toBe('Done through the configured route.');
    expect(chat.messages().map(({ role }) => role)).toEqual(['user', 'assistant']);
    expect(ai.streamChat).toHaveBeenCalledWith(
      [
        {
          role: 'user',
          content: 'List the windows',
        },
      ],
      'automatic-local',
      expect.any(AbortSignal),
    );
    expect(
      fixture.nativeElement.querySelector('[data-message-id] .tool-call')?.textContent,
    ).toContain('desktop_list_windows');
    expect(fixture.nativeElement.textContent).toContain('Done through the configured route.');
    expect(chat.providerSummary()).toContain('automatic-local');
    expect(
      (fixture.nativeElement.querySelector('.provider-select') as HTMLSelectElement).value,
    ).toBe('automatic-local');
  });

  it('exposes read, send, and clear actions as component-scoped tools', async () => {
    const fixture = await createChat();
    await vi.waitFor(() => {
      expect([...registered.keys()].sort()).toEqual([
        'chat_clear_thread',
        'chat_read_thread',
        'chat_send_prompt',
      ]);
    });

    const send = registered.get('chat_send_prompt');
    const result = await send?.execute(
      { prompt: 'Operate the desktop' },
      { signal: new AbortController().signal },
    );
    expect(JSON.parse((result as any).content[0].text)).toMatchObject({
      response: 'Done through the configured route.',
      tool_calls: [
        {
          name: 'desktop_list_windows',
          status: 'completed',
        },
      ],
    });

    const read = await registered
      .get('chat_read_thread')
      ?.execute({}, { signal: new AbortController().signal });
    expect(JSON.parse((read as any).content[0].text)).toMatchObject({
      provider_mode: 'automatic',
      sending: false,
      routes: [{ name: 'automatic-local', model: 'lem' }],
    });

    await registered
      .get('chat_clear_thread')
      ?.execute({}, { signal: new AbortController().signal });
    expect(fixture.componentInstance.messages()).toEqual([]);
  });

  it('surfaces provider errors and keeps the failed user turn', async () => {
    const fixture = await createChat();
    const chat = fixture.componentInstance;

    await expect(chat.sendPrompt('fail')).rejects.toThrow('provider unavailable');
    await fixture.whenStable();
    fixture.detectChanges();

    expect(chat.lastError()).toBe('provider unavailable');
    expect(chat.messages()).toEqual([
      expect.objectContaining({
        role: 'user',
        content: 'fail',
      }),
    ]);
    expect(fixture.nativeElement.querySelector('[role="alert"]')).not.toBeNull();
  });
});

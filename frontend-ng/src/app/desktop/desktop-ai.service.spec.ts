import { PLATFORM_ID } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { DESKTOP_AI_TRANSPORT, DesktopAiService, DesktopAiTransport } from './desktop-ai.service';

describe('DesktopAiService', () => {
  const transport: DesktopAiTransport = {
    call: vi.fn(),
  };

  beforeEach(() => {
    vi.clearAllMocks();
    TestBed.configureTestingModule({
      providers: [
        { provide: PLATFORM_ID, useValue: 'browser' },
        { provide: DESKTOP_AI_TRANSPORT, useValue: transport },
      ],
    });
  });

  afterEach(() => TestBed.resetTestingModule());

  it('reads redacted provider routes through runner.WRoutes', async () => {
    vi.mocked(transport.call).mockResolvedValue({
      OK: true,
      Value: [
        {
          name: 'local',
          kind: 'openai',
          base_url: 'http://127.0.0.1:1988/v1',
          model: 'lem',
        },
        {
          name: 'claude',
          kind: 'anthropic',
          base_url: 'https://provider.example/v1',
          model: 'claude',
        },
      ],
    });
    const abort = new AbortController();

    await expect(TestBed.inject(DesktopAiService).listRoutes(abort.signal)).resolves.toEqual([
      {
        name: 'local',
        kind: 'openai',
        baseUrl: 'http://127.0.0.1:1988/v1',
        model: 'lem',
      },
      {
        name: 'claude',
        kind: 'anthropic',
        baseUrl: 'https://provider.example/v1',
        model: 'claude',
      },
    ]);
    expect(transport.call).toHaveBeenCalledWith(
      'dappco.re/lthn/desktop/pkg/runner.Service.WRoutes',
      [],
      abort.signal,
    );
  });

  it('normalises welfare, tool calls, and progressive reply text', async () => {
    vi.mocked(transport.call).mockResolvedValue({
      OK: true,
      Value: {
        text: 'The desktop action completed.',
        warn_user: true,
        tool_calls: [
          {
            id: 'call-1',
            name: 'desktop_list_windows',
            status: 'completed',
            arguments: {},
            result: { windows: 2 },
          },
        ],
      },
    });
    const service = TestBed.inject(DesktopAiService);
    const events = [];
    const messages = [{ role: 'user' as const, content: 'List windows' }];

    for await (const event of service.streamChat(messages, new AbortController().signal)) {
      events.push(event);
    }

    expect(events[0]).toEqual({ type: 'welfare', reworded: true });
    expect(events[1]).toEqual({
      type: 'tool-calls',
      toolCalls: [
        {
          id: 'call-1',
          name: 'desktop_list_windows',
          status: 'completed',
          input: {},
          output: { windows: 2 },
        },
      ],
    });
    expect(
      events
        .filter((event) => event.type === 'text')
        .map((event) => event.text)
        .join(''),
    ).toBe('The desktop action completed.');
    expect(transport.call).toHaveBeenCalledWith(
      'dappco.re/lthn/desktop/pkg/runner.Service.WChat',
      [messages],
      expect.any(AbortSignal),
    );
  });

  it('surfaces failed Result envelopes and empty replies', async () => {
    vi.mocked(transport.call).mockResolvedValueOnce({
      OK: false,
      Value: { error: 'no provider configured' },
    });
    const service = TestBed.inject(DesktopAiService);

    await expect(async () => {
      for await (const _event of service.streamChat(
        [{ role: 'user', content: 'Hello' }],
        new AbortController().signal,
      )) {
        // Consume the generator so transport errors are observed.
      }
    }).rejects.toThrow('no provider configured');

    vi.mocked(transport.call).mockResolvedValueOnce({
      OK: true,
      Value: { text: '', warn_user: false },
    });
    await expect(async () => {
      for await (const _event of service.streamChat(
        [{ role: 'user', content: 'Hello' }],
        new AbortController().signal,
      )) {
        // Consume the generator so reply validation runs.
      }
    }).rejects.toThrow('Empty reply');
  });
});

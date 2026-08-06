import { TestBed } from '@angular/core/testing';
import { SurfaceBridgeService } from './surface-bridge.service';
import { SurfaceConfig, SurfacePage } from './surface-page';

const listConfig: SurfaceConfig = {
  id: 'test-list',
  title: 'Issues',
  subtitle: 'Ported interaction contract',
  icon: 'list',
  searchPlaceholder: 'Filter issues',
  filters: [
    { id: 'open', label: 'Open' },
    { id: 'closed', label: 'Closed' },
  ],
  actions: [{ id: 'refresh', label: 'Refresh', icon: 'rotate', kind: 'refresh' }],
  rows: [
    { id: 'M-1', title: 'Port route tree', status: 'open', tags: ['routing'] },
    { id: 'M-2', title: 'Verify lazy chunks', status: 'closed', tags: ['build'] },
  ],
  bridgeMethod: 'surface.List',
  footer: 'local fixture',
};

describe('SurfacePage', () => {
  const bridge = {
    call: vi.fn(),
    request: vi.fn(),
  };

  beforeEach(() => {
    bridge.call.mockReset();
    bridge.request.mockReset();
    TestBed.configureTestingModule({
      providers: [{ provide: SurfaceBridgeService, useValue: bridge }],
    });
  });

  afterEach(() => TestBed.resetTestingModule());

  function create(config: SurfaceConfig = listConfig) {
    const fixture = TestBed.createComponent(SurfacePage);
    fixture.componentRef.setInput('config', config);
    fixture.detectChanges();
    return fixture;
  }

  it('filters by text and status, and toggles selected rows', () => {
    const fixture = create();
    const page = fixture.componentInstance;

    page.setQuery({ target: { value: 'lazy' } } as unknown as Event);
    expect(page.visibleRows().map(({ id }) => id)).toEqual(['M-2']);

    page.setQuery({ target: { value: '' } } as unknown as Event);
    page.selectFilter('open');
    expect(page.visibleRows().map(({ id }) => id)).toEqual(['M-1']);
    page.selectFilter('open');
    expect(page.visibleRows()).toHaveLength(2);

    page.selectRow('M-1');
    expect(page.selectedId()).toBe('M-1');
    page.selectRow('M-1');
    expect(page.selectedId()).toBe('');
  });

  it('labels fixture-backed data and changes the label after a live refresh', async () => {
    bridge.call.mockResolvedValueOnce({
      items: [{ id: 'live-1', title: 'Live issue', status: 'open' }],
    });
    const fixture = create({ ...listConfig, bridgeInput: 'text' });
    await fixture.whenStable();

    const status = (): HTMLElement | null =>
      fixture.nativeElement.querySelector('[data-data-state]');
    expect(status()?.textContent).toContain('Fixture data');
    expect(status()?.dataset['dataState']).toBe('fixture');

    await fixture.componentInstance.runAction(listConfig.actions![0]);
    await fixture.whenStable();

    expect(status()?.textContent).toContain('Live data');
    expect(status()?.dataset['dataState']).toBe('live');
    expect(fixture.componentInstance.rows()[0].title).toBe('Live issue');
  });

  it('labels an initial backend failure while retaining the fixture rows', async () => {
    bridge.call.mockRejectedValueOnce(new Error('connection refused'));
    const fixture = create();

    await vi.waitFor(() => expect(bridge.call).toHaveBeenCalledOnce());
    await fixture.whenStable();

    const status: HTMLElement | null =
      fixture.nativeElement.querySelector('[data-data-state]');
    expect(status?.textContent).toContain('Offline · fixture kept');
    expect(status?.dataset['dataState']).toBe('offline');
    expect(fixture.componentInstance.rows().map(({ id }) => id)).toEqual([
      'M-1',
      'M-2',
    ]);
  });

  it('replaces fixtures with bounded live rows and keeps them on backend failure', async () => {
    const fixture = create();
    const page = fixture.componentInstance;
    bridge.call.mockResolvedValueOnce({
      items: Array.from({ length: 2_100 }, (_, index) => ({
        id: `live-${index}`,
        title: `Live item ${index}`,
        status: 'open',
      })),
    });

    await page.runAction(listConfig.actions![0]);
    expect(page.rows()).toHaveLength(2_000);
    expect(page.rows()[0].title).toBe('Live item 0');

    bridge.call.mockRejectedValueOnce(new Error('offline'));
    await page.runAction(listConfig.actions![0]);
    expect(page.rows()).toHaveLength(2_000);
    expect(page.notice()).toContain('existing local data has been kept');
  });

  it('reconciles on its poll cadence and stops polling when destroyed', async () => {
    vi.useFakeTimers();
    bridge.call.mockResolvedValue({ items: [] });
    const fixture = create({
      ...listConfig,
      pollMs: 60_000,
    });

    await vi.advanceTimersByTimeAsync(0);
    bridge.call.mockClear();
    await vi.advanceTimersByTimeAsync(60_000);
    expect(bridge.call).toHaveBeenCalledOnce();

    fixture.destroy();
    bridge.call.mockClear();
    await vi.advanceTimersByTimeAsync(60_000);
    expect(bridge.call).not.toHaveBeenCalled();
    vi.useRealTimers();
  });

  it('reloads only for conflict events addressed to its service', async () => {
    bridge.call.mockResolvedValue({ items: [] });
    const fixture = create({
      ...listConfig,
      conflictReloadService: 'campaigns.update',
    });
    await vi.waitFor(() => expect(bridge.call).toHaveBeenCalledOnce());
    bridge.call.mockClear();

    window.dispatchEvent(
      new CustomEvent('lthn:conflict:reload-requested', {
        detail: { service: 'content.update' },
      }),
    );
    await Promise.resolve();
    expect(bridge.call).not.toHaveBeenCalled();

    window.dispatchEvent(
      new CustomEvent('lthn:conflict:reload-requested', {
        detail: { service: 'campaigns.update' },
      }),
    );
    await vi.waitFor(() => expect(bridge.call).toHaveBeenCalledOnce());
    fixture.destroy();
  });

  it('moves board cards optimistically and emits the cross-view sales event', () => {
    const fixture = create({
      id: 'sales-pipeline',
      title: 'Pipeline',
      subtitle: 'Drag contract',
      icon: 'filter',
      kind: 'board',
      columns: [
        { id: 'qual', title: 'Qualifying', cards: [{ id: 'D-1', title: 'Crown Estates' }] },
        { id: 'engage', title: 'Engaging', cards: [] },
      ],
      footer: 'drag cards',
    });
    const page = fixture.componentInstance;
    const moved = vi.fn();
    window.addEventListener('lthn:sales:moved', moved);

    const transfer = { setData: vi.fn(), effectAllowed: '' };
    page.dragStart(
      { dataTransfer: transfer } as unknown as DragEvent,
      page.columns()[0].cards[0],
      'qual',
    );
    page.drop({ preventDefault: vi.fn() } as unknown as DragEvent, 'engage');

    window.removeEventListener('lthn:sales:moved', moved);
    expect(transfer.setData).toHaveBeenCalledWith(
      'application/x-lthn-surface-card',
      JSON.stringify({ id: 'D-1', source: 'qual' }),
    );
    expect(page.columns()[0].cards).toEqual([]);
    expect(page.columns()[1].cards.map(({ id }) => id)).toEqual(['D-1']);
    expect(moved).toHaveBeenCalledOnce();
    expect((moved.mock.calls[0][0] as CustomEvent).detail).toMatchObject({
      deal_id: 'D-1',
      from_stage: 'qual',
      to_stage: 'engage',
    });
  });

  it('reverts an optimistic pipeline move when the backend rejects it', async () => {
    bridge.call.mockRejectedValueOnce(new Error('transition_illegal'));
    const fixture = create({
      id: 'sales-pipeline',
      title: 'Pipeline',
      subtitle: 'Persisted drag contract',
      icon: 'filter',
      kind: 'board',
      moveBridgeMethod: 'pipeline.MoveDeal',
      columns: [
        { id: 'qual', title: 'Qualifying', cards: [{ id: 'D-1', title: 'Crown Estates' }] },
        { id: 'engage', title: 'Engaging', cards: [] },
      ],
      footer: 'drag cards',
    });
    const page = fixture.componentInstance;
    const moved = vi.fn();
    window.addEventListener('lthn:sales:moved', moved);

    page.dragStart(
      { dataTransfer: null } as unknown as DragEvent,
      page.columns()[0].cards[0],
      'qual',
    );
    page.drop({ preventDefault: vi.fn() } as unknown as DragEvent, 'engage');
    expect(page.columns()[1].cards.map(({ id }) => id)).toEqual(['D-1']);

    await vi.waitFor(() => expect(page.notice()).toContain('has been reverted'));
    window.removeEventListener('lthn:sales:moved', moved);
    expect(page.columns()[0].cards.map(({ id }) => id)).toEqual(['D-1']);
    expect(page.columns()[1].cards).toEqual([]);
    expect(bridge.call).toHaveBeenCalledWith('pipeline.MoveDeal', [
      { dealId: 'D-1', toStage: 'engage' },
    ]);
    expect(moved).not.toHaveBeenCalled();
  });

  it('persists a board move through the bridge, applies the live columns and leaves untouched columns be', async () => {
    bridge.call.mockResolvedValueOnce({
      columns: [
        { id: 'qual', cards: [] },
        { id: 'engage', cards: [{ id: 'D-1', title: 'Crown Estates (confirmed)' }] },
        { id: 'won', cards: [] },
      ],
    });
    const fixture = create({
      id: 'sales-pipeline',
      title: 'Pipeline',
      subtitle: 'Persisted drag contract',
      icon: 'filter',
      kind: 'board',
      moveBridgeMethod: 'pipeline.MoveDeal',
      columns: [
        { id: 'qual', title: 'Qualifying', cards: [{ id: 'D-1', title: 'Crown Estates' }] },
        { id: 'engage', title: 'Engaging', cards: [] },
        { id: 'won', title: 'Won', cards: [] },
      ],
      footer: 'drag contract',
    });
    const page = fixture.componentInstance;
    const moved = vi.fn();
    window.addEventListener('lthn:sales:moved', moved);

    page.dragStart(
      { dataTransfer: null } as unknown as DragEvent,
      page.columns()[0].cards[0],
      'qual',
    );
    page.drop({ preventDefault: vi.fn() } as unknown as DragEvent, 'engage');

    await vi.waitFor(() =>
      expect(page.columns().find((column) => column.id === 'engage')?.cards[0]?.title).toBe(
        'Crown Estates (confirmed)',
      ),
    );
    window.removeEventListener('lthn:sales:moved', moved);
    expect(moved).toHaveBeenCalledOnce();
    // The 'won' column was neither the drag source nor the target, so the
    // column map's fallthrough branch must return it untouched.
    expect(page.columns().map((column) => column.id)).toEqual(['qual', 'engage', 'won']);
  });

  it('flags a terminal pipeline move for confirmation', () => {
    const fixture = create({
      id: 'sales-pipeline',
      title: 'Pipeline',
      subtitle: 'Drag contract',
      icon: 'filter',
      kind: 'board',
      columns: [
        { id: 'qual', title: 'Qualifying', cards: [{ id: 'D-1', title: 'Crown Estates' }] },
        { id: 'won', title: 'Won', cards: [] },
      ],
      footer: 'drag cards',
    });
    const page = fixture.componentInstance;

    page.dragStart(
      { dataTransfer: null } as unknown as DragEvent,
      page.columns()[0].cards[0],
      'qual',
    );
    page.drop({ preventDefault: vi.fn() } as unknown as DragEvent, 'won');

    expect(page.notice()).toContain('Crown Estates moved to won');
    expect(page.notice()).toContain('Confirm the terminal transition');
  });

  it('allows drag-over on board columns', () => {
    const fixture = create({
      id: 'test-allow-drop',
      title: 'Board',
      subtitle: 'Kanban',
      icon: 'columns',
      kind: 'board',
      columns: [{ id: 'todo', title: 'To do', cards: [] }],
      footer: 'board fixture',
    });
    const page = fixture.componentInstance;
    const dataTransfer = { dropEffect: '' };
    const event = { preventDefault: vi.fn(), dataTransfer } as unknown as DragEvent;

    page.allowDrop(event);

    expect(event.preventDefault).toHaveBeenCalledOnce();
    expect(dataTransfer.dropEffect).toBe('move');
  });

  it('bounds progress values into a CSS width percentage', () => {
    const fixture = create();
    const page = fixture.componentInstance;

    expect(page.progress(150)).toBe('100%');
    expect(page.progress(-20)).toBe('0%');
    expect(page.progress(42)).toBe('42%');
    expect(page.progress(undefined)).toBe('0%');
  });

  it('filters board columns by search across ids, titles and card fields', () => {
    const fixture = create({
      id: 'test-board-search',
      title: 'Board',
      subtitle: 'Kanban search',
      icon: 'columns',
      kind: 'board',
      searchPlaceholder: 'Filter cards',
      columns: [
        {
          id: 'todo',
          title: 'To do',
          cards: [
            { id: 'C-1', title: 'Fix router', detail: 'lazy chunk regression' },
            { id: 'C-2', title: 'Write docs' },
          ],
        },
        { id: 'done', title: 'Done', cards: [{ id: 'C-3', title: 'Ship release' }] },
      ],
      footer: 'board fixture',
    });
    const page = fixture.componentInstance;

    page.setQuery({ target: { value: 'router' } } as unknown as Event);
    expect(page.visibleColumns().map((column) => column.cards.map(({ id }) => id))).toEqual([
      ['C-1'],
      [],
    ]);

    page.setQuery({ target: { value: '' } } as unknown as Event);
    expect(page.visibleColumns()[0].cards).toHaveLength(2);
  });

  it('updates the editor text as it is typed', () => {
    const fixture = create({
      id: 'test-editor',
      title: 'Editor',
      subtitle: 'Query surface',
      icon: 'terminal',
      kind: 'editor',
      editorLabel: 'SQL',
      footer: 'editor fixture',
    });
    const page = fixture.componentInstance;

    page.setEditor({ target: { value: 'select 1' } } as unknown as Event);

    expect(page.editor()).toBe('select 1');
  });

  it('clears the editor and result via the clear action', async () => {
    bridge.request.mockResolvedValueOnce({ rows: 1 });
    const fixture = create({
      id: 'test-clear',
      title: 'Editor',
      subtitle: 'Query surface',
      icon: 'terminal',
      kind: 'editor',
      editorLabel: 'SQL',
      endpoint: '/api/query',
      actions: [
        { id: 'run', label: 'Run', icon: 'play', kind: 'run' },
        { id: 'clear', label: 'Clear', icon: 'eraser', kind: 'clear' },
      ],
      footer: 'editor fixture',
    });
    const page = fixture.componentInstance;
    page.setEditor({ target: { value: 'select 1' } } as unknown as Event);
    await page.runAction(page.config.actions![0]);
    expect(page.result()).not.toBe('');

    await page.runAction(page.config.actions![1]);

    expect(page.editor()).toBe('');
    expect(page.result()).toBe('');
  });

  it('copies the editor text to the clipboard, and reports when the clipboard rejects', async () => {
    const writeText = vi
      .fn()
      .mockResolvedValueOnce(undefined)
      .mockRejectedValueOnce(new Error('denied'));
    Object.defineProperty(navigator, 'clipboard', {
      configurable: true,
      value: { writeText },
    });
    const fixture = create({
      id: 'test-copy',
      title: 'Editor',
      subtitle: 'Query surface',
      icon: 'terminal',
      kind: 'editor',
      editorLabel: 'SQL',
      actions: [{ id: 'copy', label: 'Copy', icon: 'copy', kind: 'copy' }],
      footer: 'editor fixture',
    });
    const page = fixture.componentInstance;
    page.setEditor({ target: { value: 'select 1' } } as unknown as Event);

    await page.runAction(page.config.actions![0]);
    expect(writeText).toHaveBeenCalledWith('select 1');
    expect(page.notice()).toContain('Copied to clipboard.');

    await page.runAction(page.config.actions![0]);
    expect(page.notice()).toContain('Clipboard access is unavailable.');
  });

  it('adds a local draft row via the add action for a list surface', async () => {
    const fixture = create({
      ...listConfig,
      actions: [{ id: 'add', label: 'Add', icon: 'plus', kind: 'add' }],
    });
    const page = fixture.componentInstance;
    const before = page.rows().length;

    await page.runAction(page.config.actions![0]);

    expect(page.rows()).toHaveLength(before + 1);
    expect(page.rows()[0].title).toBe('New item');
    expect(page.selectedId()).toBe(page.rows()[0].id);
  });

  it('adds a local draft card to the first column via the add action for a board surface', async () => {
    const fixture = create({
      id: 'test-board-add',
      title: 'Board',
      subtitle: 'Kanban',
      icon: 'columns',
      kind: 'board',
      actions: [{ id: 'add', label: 'Add', icon: 'plus', kind: 'add' }],
      columns: [
        { id: 'todo', title: 'To do', cards: [] },
        { id: 'done', title: 'Done', cards: [] },
      ],
      footer: 'board fixture',
    });
    const page = fixture.componentInstance;

    await page.runAction(page.config.actions![0]);

    expect(page.columns()[0].cards).toHaveLength(1);
    expect(page.columns()[0].cards[0].title).toBe('New item');
    expect(page.columns()[1].cards).toHaveLength(0);
    expect(page.selectedId()).toBe(page.columns()[0].cards[0].id);
  });

  it('runs an endpoint action with the query/body/sql payload and shows the JSON result', async () => {
    bridge.request.mockResolvedValueOnce({ rows: 2 });
    const fixture = create({
      id: 'test-run',
      title: 'Run',
      subtitle: 'Query surface',
      icon: 'terminal',
      kind: 'editor',
      editorLabel: 'SQL',
      endpoint: '/api/query',
      actions: [{ id: 'run', label: 'Run', icon: 'play', kind: 'run' }],
      footer: 'run fixture',
    });
    const page = fixture.componentInstance;
    page.setEditor({ target: { value: 'select 1' } } as unknown as Event);

    await page.runAction(page.config.actions![0]);

    expect(bridge.request).toHaveBeenCalledWith('/api/query', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ query: 'select 1', body: 'select 1', sql: 'select 1' }),
    });
    expect(page.result()).toBe(JSON.stringify({ rows: 2 }, null, 2));
    expect(page.notice()).toContain('Query complete.');
  });

  it('runs an ask-shaped endpoint action with the prompt payload', async () => {
    bridge.request.mockResolvedValueOnce({ answer: 'ok' });
    const fixture = create({
      id: 'test-run-ask',
      title: 'Run',
      subtitle: 'Ask surface',
      icon: 'terminal',
      kind: 'editor',
      editorLabel: 'Prompt',
      endpoint: '/api/ask',
      endpointPayload: 'ask',
      actions: [{ id: 'run', label: 'Ask', icon: 'play', kind: 'run' }],
      footer: 'ask fixture',
    });
    const page = fixture.componentInstance;
    page.setEditor({ target: { value: 'hello' } } as unknown as Event);

    await page.runAction(page.config.actions![0]);

    expect(bridge.request).toHaveBeenCalledWith('/api/ask', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ prompt: 'hello', active_tab: 'models' }),
    });
  });

  it('reports a run-action failure and keeps the surface usable', async () => {
    bridge.request.mockRejectedValueOnce(new Error('bad request'));
    const fixture = create({
      id: 'test-run-fail',
      title: 'Run',
      subtitle: 'Query surface',
      icon: 'terminal',
      kind: 'editor',
      editorLabel: 'SQL',
      endpoint: '/api/query',
      actions: [{ id: 'run', label: 'Run', icon: 'play', kind: 'run' }],
      footer: 'run fixture',
    });
    const page = fixture.componentInstance;

    await page.runAction(page.config.actions![0]);

    expect(page.notice()).toContain('bad request');
    expect(page.busy()).toBe(false);
  });

  it('shows the local, offline-safe notice for an action with no backend wiring', async () => {
    const fixture = create({
      id: 'test-local-only',
      title: 'List',
      subtitle: 'Local only',
      icon: 'list',
      actions: [{ id: 'refresh', label: 'Refresh', icon: 'rotate', kind: 'refresh' }],
      rows: [{ id: 'R-1', title: 'Row' }],
      footer: 'local fixture',
    });
    const page = fixture.componentInstance;

    await page.runAction(page.config.actions![0]);

    expect(page.notice()).toContain('Showing the local, offline-safe data set.');
  });

  it('loads live data from a GET loadEndpoint and reflects the live state', async () => {
    const fixture = create({
      id: 'test-load-endpoint',
      title: 'Endpoint list',
      subtitle: 'GET-backed',
      icon: 'list',
      loadEndpoint: '/api/rows',
      actions: [{ id: 'refresh', label: 'Refresh', icon: 'rotate', kind: 'refresh' }],
      footer: 'endpoint fixture',
    });
    const page = fixture.componentInstance;
    await vi.waitFor(() => expect(bridge.request).toHaveBeenCalledOnce());
    await fixture.whenStable();
    bridge.request.mockClear();

    bridge.request.mockResolvedValueOnce({
      items: [{ id: 'L-1', title: 'Loaded row', detail: 'from the endpoint' }],
    });
    await page.runAction(page.config.actions![0]);
    fixture.detectChanges();

    expect(bridge.request).toHaveBeenCalledWith('/api/rows', { method: 'GET' });
    expect(page.dataState()).toBe('live');
    expect(page.rows()[0].title).toBe('Loaded row');
    const row = fixture.nativeElement.querySelector('.surface__row') as HTMLElement;
    expect(row.textContent).toContain('from the endpoint');
  });

  it('renders a progress bar for a fixture row carrying a progress value', () => {
    const fixture = create({
      ...listConfig,
      rows: [{ id: 'P-1', title: 'In progress', progress: 55 }],
    });

    const bar = fixture.nativeElement.querySelector(
      '.surface__progress span',
    ) as HTMLElement | null;

    expect(bar).not.toBeNull();
    expect(bar?.style.width).toBe('55%');
  });

  it('sends a parsed JSON object as the bridge input and rejects a non-object body', async () => {
    bridge.call.mockResolvedValueOnce({ items: [] });
    const fixture = create({ ...listConfig, bridgeInput: 'json' });
    const page = fixture.componentInstance;

    page.setEditor({ target: { value: '{"active_tab":"models"}' } } as unknown as Event);
    await page.runAction(listConfig.actions![0]);
    expect(bridge.call).toHaveBeenCalledWith('surface.List', [{ active_tab: 'models' }]);

    page.setEditor({ target: { value: '[1,2,3]' } } as unknown as Event);
    await page.runAction(listConfig.actions![0]);
    expect(page.notice()).toContain('must be a JSON object');
  });

  it('honours a custom liveKeys list when scanning the bridge payload', async () => {
    const fixture = create({ ...listConfig, liveKeys: ['tickets'] });
    const page = fixture.componentInstance;
    await vi.waitFor(() => expect(bridge.call).toHaveBeenCalledOnce());
    await fixture.whenStable();
    bridge.call.mockClear();

    bridge.call.mockResolvedValueOnce({ tickets: [{ id: 'T-1', title: 'Custom key row' }] });
    await page.runAction(listConfig.actions![0]);

    expect(page.rows()[0].title).toBe('Custom key row');
  });

  it('distributes flat live rows into existing board columns using the state aliases', async () => {
    const fixture = create({
      id: 'coding-issues',
      title: 'Issues',
      subtitle: 'Board',
      icon: 'bug',
      kind: 'board',
      bridgeMethod: 'issues.List',
      actions: [{ id: 'refresh', label: 'Refresh', icon: 'rotate', kind: 'refresh' }],
      columns: [
        { id: 'todo', title: 'To do', cards: [] },
        { id: 'doing', title: 'Doing', cards: [] },
        { id: 'done', title: 'Done', cards: [] },
      ],
      footer: 'board fixture',
    });
    const page = fixture.componentInstance;
    await vi.waitFor(() => expect(bridge.call).toHaveBeenCalledOnce());
    await fixture.whenStable();
    bridge.call.mockClear();

    bridge.call.mockResolvedValueOnce({
      issues: [
        { id: 'I-1', title: 'Fix router', state: 'open' },
        { id: 'I-2', name: 'Write docs', status: 'in_progress' },
        { id: 'I-3', summary: 'Ship release', state: 'done' },
        { id: 'I-4', state: 'unrecognised' },
      ],
    });
    await page.runAction(page.config.actions![0]);

    expect(page.columns().find((column) => column.id === 'todo')?.cards.map(({ title }) => title)).toEqual(
      ['Fix router'],
    );
    expect(
      page.columns().find((column) => column.id === 'doing')?.cards.map(({ title }) => title),
    ).toEqual(['Write docs']);
    expect(page.columns().find((column) => column.id === 'done')?.cards.map(({ title }) => title)).toEqual(
      ['Ship release'],
    );
  });

  it('drives the header chrome through real DOM events: eyebrow, run action, search and filters', async () => {
    bridge.call.mockResolvedValue({ items: [] });
    const fixture = create({ ...listConfig, eyebrow: 'Ported' });
    const page = fixture.componentInstance;
    const element = fixture.nativeElement as HTMLElement;

    expect(element.querySelector('.surface__eyebrow')?.textContent).toBe('Ported');

    const runButton = element.querySelector('.surface__button') as HTMLButtonElement;
    runButton.click();
    await vi.waitFor(() => expect(bridge.call).toHaveBeenCalled());

    const search = element.querySelector('.surface__search input') as HTMLInputElement;
    search.value = 'lazy';
    search.dispatchEvent(new Event('input'));
    expect(page.query()).toBe('lazy');

    const filterButton = element.querySelector('.surface__filters button') as HTMLButtonElement;
    filterButton.click();
    expect(page.activeFilter()).toBe('open');
  });

  it('drives the editor textarea and renders the result block through real DOM events', async () => {
    bridge.request.mockResolvedValueOnce({ rows: 1 });
    const fixture = create({
      id: 'test-editor-dom',
      title: 'Editor',
      subtitle: 'Query surface',
      icon: 'terminal',
      kind: 'editor',
      editorLabel: 'SQL',
      endpoint: '/api/query',
      actions: [{ id: 'run', label: 'Run', icon: 'play', kind: 'run' }],
      footer: 'editor fixture',
    });
    const page = fixture.componentInstance;
    const element = fixture.nativeElement as HTMLElement;

    const textarea = element.querySelector('textarea') as HTMLTextAreaElement;
    textarea.value = 'select 1';
    textarea.dispatchEvent(new Event('input'));
    expect(page.editor()).toBe('select 1');

    await page.runAction(page.config.actions![0]);
    fixture.detectChanges();

    const pre = element.querySelector('.surface__result') as HTMLElement;
    expect(pre.textContent).toContain('"rows": 1');
  });

  it('returns no bridge input value once the config sets neither a text nor a json shape', () => {
    const fixture = create();
    const page = fixture.componentInstance as unknown as { bridgeInputValue(): unknown };

    expect(page.bridgeInputValue()).toBeUndefined();
  });
});

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
});

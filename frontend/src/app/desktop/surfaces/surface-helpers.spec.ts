import {
  boundedRows,
  clampFields,
  clampRows,
  clampString,
  countFocusBlocks,
  eventColours,
  filterCards,
  isRecord,
  projectBacklog,
  toneColours,
  totalItems,
  visibleLanes,
} from './surface-helpers';

describe('ported surface helpers', () => {
  it('bounds untrusted backend rows, fields, and UTF-8 text', () => {
    expect(clampRows([1, 2, 3], 2)).toEqual([1, 2]);
    expect(clampRows('not rows')).toEqual([]);
    expect(clampFields({ a: 1, b: 2, c: 3 }, 2)).toEqual({ a: 1, b: 2 });
    expect(clampString('£££', 4)).toBe('££');
    expect(isRecord({ id: 1 })).toBe(true);
    expect(isRecord([])).toBe(false);
    expect(
      boundedRows(
        [{ id: 'one' }, null, { id: 'two' }],
        (row) => (isRecord(row) && typeof row['id'] === 'string' ? row['id'] : null),
        3,
      ),
    ).toEqual(['one', 'two']);
  });

  it('projects backlog rows without mutating the input', () => {
    const items = [
      { id: 'M-1', title: 'Smaller task', score: 4, impact: 2, effort: 1, conf: 3 },
      { id: 'M-2', title: 'Important route', score: 9, impact: 8, effort: 5, conf: 4 },
    ] as const;
    const snapshot = structuredClone(items);

    expect(projectBacklog(items, '', 'score', 'desc').map(({ id }) => id)).toEqual(['M-2', 'M-1']);
    expect(projectBacklog(items, 'route', 'impact', 'asc').map(({ id }) => id)).toEqual(['M-2']);
    expect(items).toEqual(snapshot);
  });

  it('preserves calendar and retrospective colour contracts', () => {
    expect(eventColours('brand')).toEqual([
      'rgba(64,193,197,0.18)',
      'rgba(64,193,197,0.40)',
      'var(--brand-300)',
    ]);
    expect(
      ['focus', 'brand', 'ship', 'mute'].map((tone) => eventColours(tone as any)),
    ).toHaveLength(4);
    expect(countFocusBlocks([{ tone: 'focus' }, { tone: 'ship' }, { tone: 'focus' }])).toBe(2);
    expect(toneColours('warn')[2]).toBe('var(--warning-400)');
    expect(toneColours('ok')[2]).toBe('var(--success-400)');
    expect(toneColours('brand')[2]).toBe('var(--brand-300)');
    expect(totalItems([{ items: [1, 2] }, { items: [3] }])).toBe(3);
  });

  it('filters roadmap lanes and sprint cards case-insensitively', () => {
    const lanes = [{ id: 'now' }, { id: 'next' }, { id: 'later' }];
    expect(visibleLanes(lanes, new Set(['next']))).toEqual([{ id: 'now' }, { id: 'later' }]);

    const columns = [
      {
        id: 'todo',
        count: 2,
        cards: [
          { id: 'M-101', title: 'Hash routes' },
          { id: 'M-102', title: 'Menu catalogue' },
        ],
      },
      { id: 'done', count: 1, cards: [{ id: 'M-99', title: 'Angular shell' }] },
    ];
    const filtered = filterCards(columns, 'HASH');
    expect(filtered[0].cards.map(({ id }) => id)).toEqual(['M-101']);
    expect(filtered.map(({ count }) => count)).toEqual([1, 0]);
    expect(columns[0].count).toBe(2);
  });
});

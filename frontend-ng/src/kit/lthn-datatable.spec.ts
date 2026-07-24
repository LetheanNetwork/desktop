import type { Column, Row, SortEventDetail } from './lthn-datatable';
import './lthn-datatable';

type DatatableElement = HTMLElement & {
  columns: readonly Column[];
  rows: readonly Row[];
  selectable: boolean;
  updateComplete: Promise<unknown>;
};

type MalformedDatatableElement = HTMLElement & {
  columns: readonly Column[];
  emptyLabel: string;
  rows: string;
  updateComplete: Promise<unknown>;
};

afterEach(() => {
  document.body.replaceChildren();
});

describe('Lethean datatable kit', () => {
  it('sorts numeric rows and emits the selected direction', async () => {
    const element = document.createElement('lthn-datatable') as DatatableElement;
    element.columns = [
      { key: 'name', label: 'Name' },
      { key: 'rate', label: 'Rate', type: 'num' },
    ];
    element.rows = [
      { name: 'Zulu', rate: 12 },
      { name: 'Alpha', rate: 2 },
    ];
    const sort = vi.fn();
    element.addEventListener('sort', sort);
    document.body.appendChild(element);
    await element.updateComplete;

    element.querySelector<HTMLButtonElement>('button[aria-label="Sort by Rate"]')?.click();
    await element.updateComplete;

    const renderedRows = [...element.querySelectorAll('tbody tr')];
    expect(renderedRows.map((row) => row.textContent)).toEqual([
      expect.stringContaining('Alpha'),
      expect.stringContaining('Zulu'),
    ]);
    expect((sort.mock.calls[0]?.[0] as CustomEvent<SortEventDetail>).detail).toEqual({
      key: 'rate',
      dir: 'asc',
    });
  });

  it('renders malformed input as the configured empty state', async () => {
    const element = document.createElement('lthn-datatable') as MalformedDatatableElement;
    element.columns = [{ key: 'name', label: 'Name' }];
    element.rows = '{not-json';
    element.emptyLabel = 'Nothing queued';
    document.body.appendChild(element);
    await element.updateComplete;

    expect(element.querySelector('tbody')?.textContent).toContain('Nothing queued');
    expect(element.querySelector('[aria-busy]')?.getAttribute('aria-busy')).toBe('false');
  });
});

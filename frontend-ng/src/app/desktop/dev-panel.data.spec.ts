import {
  DEV_PANEL_CATALOGUE,
  DevPanelColumn,
  DevPanelTableRow,
  devPanelFor,
} from './dev-panel.data';

describe('developer panel fixture catalogue', () => {
  it('returns the typed empty view for an unknown route', () => {
    expect(devPanelFor('not-a-developer-route')).toEqual({ kind: 'empty' });
  });

  it('serialises valid table columns and rows for every table fixture', () => {
    const invalidTables = Object.entries(DEV_PANEL_CATALOGUE).flatMap(([route, panel]) => {
      if (panel.kind !== 'table') return [];

      const columns = JSON.parse(panel.cols) as DevPanelColumn[];
      const rows = JSON.parse(panel.rows) as DevPanelTableRow[];
      const keys = columns.map((column) => column.key);
      const hasDuplicateColumns = new Set(keys).size !== keys.length;
      const missingRowColumns = rows.flatMap((row) =>
        Object.keys(row).filter((key) => !keys.includes(key)),
      );

      return hasDuplicateColumns || missingRowColumns.length ? [route] : [];
    });

    expect(invalidTables).toEqual([]);
  });
});

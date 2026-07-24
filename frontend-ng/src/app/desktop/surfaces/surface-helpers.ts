export const DEFAULT_MAX_ROWS = 2_000;
export const DEFAULT_MAX_FIELD_BYTES = 64_000;
export const DEFAULT_MAX_FIELDS_PER_ROW = 50;

export function isRecord(value: unknown): value is Record<string, unknown> {
  return (
    typeof value === 'object' &&
    value !== null &&
    !Array.isArray(value) &&
    Object.getPrototypeOf(value) === Object.prototype
  );
}

export function clampRows<T>(rows: unknown, maxRows = DEFAULT_MAX_ROWS): T[] {
  if (!Array.isArray(rows)) return [];
  return (rows.length <= maxRows ? rows : rows.slice(0, maxRows)) as T[];
}

export function boundedRows<T>(
  rows: unknown,
  validate: (row: unknown) => T | null,
  maxRows = DEFAULT_MAX_ROWS,
): T[] {
  return clampRows<unknown>(rows, maxRows).flatMap((row): T[] => {
    const value = validate(row);
    return value === null ? [] : [value];
  });
}

export function clampString(value: unknown, maxBytes = DEFAULT_MAX_FIELD_BYTES): string {
  if (typeof value !== 'string') return '';
  const encoder = new TextEncoder();
  const bytes = encoder.encode(value);
  if (bytes.length <= maxBytes) return value;
  return new TextDecoder('utf-8').decode(bytes.slice(0, maxBytes)).replace(/�+$/, '');
}

export function clampFields(
  value: unknown,
  maxFields = DEFAULT_MAX_FIELDS_PER_ROW,
): Record<string, unknown> {
  if (!isRecord(value)) return {};
  const keys = Object.keys(value);
  if (keys.length <= maxFields) return value;
  return Object.fromEntries(keys.slice(0, maxFields).map((key) => [key, value[key]]));
}

export interface BacklogItem {
  readonly id: string;
  readonly title: string;
  readonly score: number;
  readonly impact: number;
  readonly effort: number;
  readonly conf: number;
}

export function projectBacklog(
  items: readonly BacklogItem[],
  query: string,
  sortBy: 'score' | 'impact' | 'effort' | 'conf',
  sortDir: 'asc' | 'desc',
): BacklogItem[] {
  const normalised = query.trim().toLocaleLowerCase('en-GB');
  return items
    .filter(
      ({ id, title }) =>
        !normalised ||
        id.toLocaleLowerCase('en-GB').includes(normalised) ||
        title.toLocaleLowerCase('en-GB').includes(normalised),
    )
    .slice()
    .sort((left, right) =>
      sortDir === 'asc' ? left[sortBy] - right[sortBy] : right[sortBy] - left[sortBy],
    );
}

export type CalendarTone = 'focus' | 'brand' | 'ship' | 'mute';

export function eventColours(tone: CalendarTone): [string, string, string] {
  switch (tone) {
    case 'focus':
      return ['rgba(64,193,197,0.08)', 'rgba(64,193,197,0.22)', 'var(--brand-300)'];
    case 'brand':
      return ['rgba(64,193,197,0.18)', 'rgba(64,193,197,0.40)', 'var(--brand-300)'];
    case 'ship':
      return ['rgba(245,158,11,0.10)', 'rgba(245,158,11,0.30)', 'var(--warning-400)'];
    case 'mute':
      return ['rgba(255,255,255,0.03)', 'rgba(255,255,255,0.07)', 'var(--fg-2)'];
  }
}

export function countFocusBlocks(events: readonly { tone: CalendarTone }[]): number {
  return events.filter(({ tone }) => tone === 'focus').length;
}

export type RetroTone = 'ok' | 'warn' | 'brand';

export function toneColours(tone: RetroTone): [string, string, string] {
  switch (tone) {
    case 'ok':
      return ['rgba(34,197,94,0.06)', 'rgba(34,197,94,0.22)', 'var(--success-400)'];
    case 'warn':
      return ['rgba(245,158,11,0.06)', 'rgba(245,158,11,0.22)', 'var(--warning-400)'];
    case 'brand':
      return ['rgba(64,193,197,0.06)', 'rgba(64,193,197,0.22)', 'var(--brand-300)'];
  }
}

export function totalItems(columns: readonly { items: readonly unknown[] }[]): number {
  return columns.reduce((total, column) => total + column.items.length, 0);
}

export function visibleLanes<T extends { id: string }>(
  lanes: readonly T[],
  hidden: ReadonlySet<string>,
): T[] {
  return lanes.filter(({ id }) => !hidden.has(id));
}

export interface SprintCard {
  readonly id: string;
  readonly title: string;
}

export interface SprintColumn<T extends SprintCard = SprintCard> {
  readonly id: string;
  readonly count: number;
  readonly cards: readonly T[];
}

export function filterCards<T extends SprintCard, C extends SprintColumn<T>>(
  columns: readonly C[],
  query: string,
): C[] {
  const normalised = query.trim().toLocaleLowerCase('en-GB');
  return columns.map((column) => {
    const cards = normalised
      ? column.cards.filter(
          ({ id, title }) =>
            id.toLocaleLowerCase('en-GB').includes(normalised) ||
            title.toLocaleLowerCase('en-GB').includes(normalised),
        )
      : column.cards.slice();
    return { ...column, count: cards.length, cards } as C;
  });
}

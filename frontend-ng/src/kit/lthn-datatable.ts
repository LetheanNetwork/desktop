/* lthn-datatable.ts — <lthn-datatable>, the design system's data-grid default.
 *
 * Dense, sortable, optionally selectable table on the tokens. columns + rows are
 * JSON attributes; cell rendering is by column type (text · num · mono · status ·
 * badge). Sorting + selection are handled internally and emit events.
 *
 *   <lthn-datatable selectable page-size="8" loading  (or)  empty-label="No models yet"
 *     columns='[{"key":"name","label":"Model"},{"key":"rate","label":"tok/s","type":"num"},
 *               {"key":"status","label":"State","type":"status"}]'
 *     rows='[{"name":"llama-3.1-70b","rate":34.2,"status":"running"}, …]'></lthn-datatable>
 *
 * States: `loading` → skeleton rows · no rows → the `empty-label` message.
 * A11y: <th scope=col> with aria-sort; sort headers are real <button>s (keyboard +
 *   focus-visible ring); checkboxes are labelled; the grid sets aria-busy when loading.
 * Events: `sort` {key,dir} · `selection` {rows} · `rowclick` {row}.
 */
import { LitElement, html, nothing, type PropertyDeclarations, type TemplateResult } from 'lit';

export type CellValue = string | number | boolean | null | undefined;
export interface Row {
  [key: string]: CellValue;
}

export type ColumnType = 'text' | 'num' | 'mono' | 'status' | 'badge';
export type ColumnAlignment = 'left' | 'center' | 'right';
export interface Column {
  key: string;
  label: string;
  type?: ColumnType;
  align?: ColumnAlignment;
  sortable?: boolean;
}

export type SortDirection = 'asc' | 'desc';
export interface SortEventDetail {
  key: string;
  dir: SortDirection;
}
export interface SelectionEventDetail {
  rows: Row[];
}
export interface RowClickEventDetail {
  row: Row;
}
export interface DatatableProperties {
  columns?: readonly Column[] | string;
  rows?: readonly Row[] | string;
  selectable?: boolean;
  loading?: boolean;
  emptyLabel?: string;
  pageSize?: number;
}

type StatusTone = 'success' | 'warn' | 'danger' | 'neutral';
interface IndexedRow {
  r: Row;
  i: number;
}

const define = (name: string, cls: CustomElementConstructor): void => {
  if (!customElements.get(name)) customElements.define(name, cls);
};
const STATUS: Record<string, readonly [StatusTone, string]> = {
  running: ['success', $localize`:Data status@@datatable.status.running:Running`],
  connected: ['success', $localize`:Data status@@datatable.status.connected:Connected`],
  active: ['success', $localize`:Data status@@datatable.status.active:Active`],
  queued: ['neutral', $localize`:Data status@@datatable.status.queued:Queued`],
  idle: ['neutral', $localize`:Data status@@datatable.status.idle:Idle`],
  disconnected: ['neutral', $localize`:Data status@@datatable.status.off:Off`],
  warning: ['warn', $localize`:Data status@@datatable.status.warning:Warning`],
  due: ['warn', $localize`:Data status@@datatable.status.paymentDue:Payment due`],
  preview: ['warn', $localize`:Data status@@datatable.status.preview:Preview`],
  error: ['danger', $localize`:Data status@@datatable.status.error:Error`],
  stalled: ['danger', $localize`:Data status@@datatable.status.stalled:Stalled`],
  failed: ['danger', $localize`:Data status@@datatable.status.failed:Failed`],
};
const PILL: Record<StatusTone, readonly [string | null, string]> = {
  success: ['var(--success-500)', 'var(--success-400)'],
  warn: ['var(--warning-500)', 'var(--warning-400)'],
  danger: ['var(--danger-500)', 'var(--danger-400)'],
  neutral: [null, 'var(--fg-3)'],
};

class LthnDatatable extends LitElement implements DatatableProperties {
  static override properties: PropertyDeclarations = {
    columns: {},
    rows: {},
    selectable: { type: Boolean },
    loading: { type: Boolean },
    emptyLabel: { attribute: 'empty-label' },
    sortKey: { state: true },
    sortDir: { state: true },
    page: { state: true },
    pageSize: { type: Number, attribute: 'page-size' },
    _sel: { state: true },
  };

  declare columns: readonly Column[] | string;
  declare rows: readonly Row[] | string;
  declare selectable: boolean;
  declare loading: boolean;
  declare emptyLabel: string;
  declare sortKey: string;
  declare sortDir: SortDirection;
  declare page: number;
  declare pageSize: number;
  declare _sel: Set<number>;

  constructor() {
    super();
    this.selectable = false;
    this.loading = false;
    this.emptyLabel = $localize`:Data table empty state@@datatable.emptyLabel:No rows to show.`;
    this.sortDir = 'asc';
    this.sortKey = '';
    this.page = 0;
    this.pageSize = 0;
    this._sel = new Set<number>();
  }
  protected override createRenderRoot(): HTMLElement | DocumentFragment {
    return this;
  }

  get cols(): readonly Column[] {
    try {
      return (
        typeof this.columns === 'string' ? JSON.parse(this.columns) : this.columns || []
      ) as readonly Column[];
    } catch {
      return [];
    }
  }

  get data(): readonly Row[] {
    try {
      return (
        typeof this.rows === 'string' ? JSON.parse(this.rows) : this.rows || []
      ) as readonly Row[];
    } catch {
      return [];
    }
  }

  get sorted(): IndexedRow[] {
    const d = this.data.map((r: Row, i: number): IndexedRow => ({ r, i }));
    if (this.sortKey) {
      const k = this.sortKey,
        dir = this.sortDir === 'asc' ? 1 : -1;
      d.sort((a: IndexedRow, b: IndexedRow): number => {
        const x = a.r[k],
          y = b.r[k];
        return typeof x === 'number' && typeof y === 'number'
          ? (x - y) * dir
          : String(x ?? '').localeCompare(String(y ?? '')) * dir;
      });
    }
    return d;
  }

  get paged(): IndexedRow[] {
    const s = this.sorted;
    if (!this.pageSize) return s;
    const start = this.page * this.pageSize;
    return s.slice(start, start + this.pageSize);
  }

  private _sort(c: Column): void {
    if (c.sortable === false) return;
    if (this.sortKey === c.key) this.sortDir = this.sortDir === 'asc' ? 'desc' : 'asc';
    else {
      this.sortKey = c.key;
      this.sortDir = 'asc';
    }
    this.dispatchEvent(
      new CustomEvent<SortEventDetail>('sort', {
        detail: { key: this.sortKey, dir: this.sortDir },
      }),
    );
  }

  private _toggle(i: number): void {
    const s = new Set<number>(this._sel);
    s.has(i) ? s.delete(i) : s.add(i);
    this._sel = s;
    this._emitSel();
  }

  private _toggleAll(): void {
    const all = this.data.map((_row: Row, i: number): number => i);
    this._sel = this._sel.size === all.length ? new Set<number>() : new Set<number>(all);
    this._emitSel();
  }

  private _emitSel(): void {
    const rows = [...this._sel].map((i: number): Row => this.data[i]!);
    this.dispatchEvent(new CustomEvent<SelectionEventDetail>('selection', { detail: { rows } }));
  }

  private _sortLabel(column: string): string {
    return $localize`:Data table sort action@@datatable.sortBy:Sort by ${column}:column:`;
  }

  private _cell(c: Column, row: Row): CellValue | TemplateResult {
    const v = row[c.key];
    if (c.type === 'status') {
      const status: readonly [StatusTone, CellValue] = STATUS[v as string] || ['neutral', v];
      const [tone, label] = status;
      const [bg, fg] = PILL[tone] || PILL.neutral;
      const bgc = bg ? `color-mix(in oklch, ${bg} 16%, transparent)` : 'var(--ink-3)';
      const bdc = bg ? `color-mix(in oklch, ${bg} 30%, transparent)` : 'var(--line-1)';
      return html`<span
        style="font-family:var(--font-mono);font-size:9.5px;padding:2px 8px;border-radius:999px;background:${bgc};border:1px solid ${bdc};color:${fg};letter-spacing:.06em;text-transform:uppercase"
        >${label}</span
      >`;
    }
    if (c.type === 'badge')
      return html`<span
        style="display:inline-flex;height:20px;align-items:center;padding:0 8px;border-radius:999px;background:color-mix(in oklch,var(--brand-500) 20%,var(--ink-2));color:var(--brand-200);font-size:11px"
        >${v}</span
      >`;
    return v;
  }

  protected override render(): TemplateResult {
    const cols = this.cols,
      total = this.data.length;
    const span = cols.length + (this.selectable ? 1 : 0);
    const numAlign = (c: Column): ColumnAlignment =>
      c.align || (c.type === 'num' || c.type === 'mono' ? 'right' : 'left');
    const mono = (c: Column): boolean => c.type === 'num' || c.type === 'mono';
    const th =
      'padding:9px 14px;font-family:var(--font-mono);font-size:10px;letter-spacing:.06em;text-transform:uppercase;color:var(--fg-3);border-bottom:1px solid var(--line-1);white-space:nowrap;user-select:none';
    const td =
      'padding:9px 14px;border-bottom:1px solid var(--line-1);color:var(--fg-1);font-size:13px';
    const pages = this.pageSize ? Math.ceil(total / this.pageSize) : 1;
    const skelRows = this.pageSize || 5;
    const tableLabel = $localize`:Data table region label@@datatable.regionLabel:Data table`;
    const selectAllLabel = $localize`:Select every data table row@@datatable.selectAll:Select all rows`;
    const selectRowLabel = $localize`:Select one data table row@@datatable.selectRow:Select row`;
    const previousPageLabel = $localize`:Data table pagination@@datatable.previousPage:Previous page`;
    const nextPageLabel = $localize`:Data table pagination@@datatable.nextPage:Next page`;
    const rowSummary = this.loading
      ? $localize`:Data table loading status@@datatable.loading:Loading…`
      : this._sel.size
        ? $localize`:Data table row summary with selection@@datatable.rowSummarySelected:${this._sel.size}:selectedCount: selected · ${total}:rowCount: rows`
        : $localize`:Data table row summary@@datatable.rowSummary:${total}:rowCount: rows`;

    return html`<div
      role="region"
      aria-label=${tableLabel}
      aria-busy=${this.loading ? 'true' : 'false'}
      style="border:1px solid var(--line-1);border-radius:var(--r-lg,12px);overflow:hidden;background:var(--ink-2)"
    >
      <table style="width:100%;border-collapse:collapse">
        <thead>
          <tr>
            ${this.selectable ? html`<th scope="col" style="${th};text-align:left;width:36px"><input type="checkbox" aria-label=${selectAllLabel} ?disabled=${this.loading} .checked=${this._sel.size === total && total > 0} @change=${(): void => this._toggleAll()} style="accent-color:var(--brand-500)" /></th>` : nothing}
            ${cols.map((c: Column): TemplateResult => {
            const sortable = c.sortable !== false;
            const active = this.sortKey === c.key;
            const ariaSort = !sortable
              ? undefined
              : active
                ? this.sortDir === 'asc'
                  ? 'ascending'
                  : 'descending'
                : 'none';
            const caret = active
              ? html`<span aria-hidden="true" style="color:var(--brand-300);margin-left:5px"
                  >${this.sortDir === 'asc' ? '\u2191' : '\u2193'}</span
                >`
              : nothing;
            return html`<th
              scope="col"
              aria-sort=${ariaSort ?? nothing}
              style="${th};text-align:${numAlign(c)}"
            >
              ${
                sortable
                  ? html`<button
                      @click=${(): void => this._sort(c)}
                      aria-label=${this._sortLabel(c.label)}
                      style="all:unset;cursor:pointer;display:inline-flex;align-items:center;gap:2px;font:inherit;letter-spacing:inherit;text-transform:inherit;color:inherit"
                    >
                      ${c.label}${caret}
                    </button>`
                  : html`${c.label}`
              }
            </th>`;
          })}
          </tr>
        </thead>
        <tbody>
          ${
            this.loading
              ? Array.from({ length: skelRows }).map(
                  (_value: unknown): TemplateResult =>
                    html`<tr>
                      ${this.selectable ? html`<td style="${td}"><span class="lthn-skeleton" style="display:block;width:16px;height:16px"></span></td>` : nothing}
                      ${cols.map((c: Column, ci: number): TemplateResult => html`<td style="${td};text-align:${numAlign(c)}"><span class="lthn-skeleton" style="display:inline-block;height:11px;width:${ci === 0 ? 60 : 34}%"></span></td>`)}
                    </tr>`,
                )
              : total === 0
                ? html`<tr>
                    <td
                      colspan=${span}
                      style="padding:34px 14px;text-align:center;color:var(--fg-3);font-size:13px"
                    >
                      ${this.emptyLabel}
                    </td>
                  </tr>`
                : this.paged.map(
                    ({ r, i }: IndexedRow): TemplateResult =>
                      html`<tr
                        @click=${(): boolean => this.dispatchEvent(new CustomEvent<RowClickEventDetail>('rowclick', { detail: { row: r } }))}
                        style="background:${this._sel.has(i) ? 'color-mix(in oklch,var(--brand-500) 10%,transparent)' : 'transparent'}"
                        @mouseenter=${(event: MouseEvent): void => {
                    if (!this._sel.has(i))
                      (event.currentTarget as HTMLTableRowElement).style.background =
                        'var(--ink-3)';
                  }}
                        @mouseleave=${(event: MouseEvent): void => {
                    if (!this._sel.has(i))
                      (event.currentTarget as HTMLTableRowElement).style.background = 'transparent';
                  }}
                      >
                        ${this.selectable ? html`<td style="${td};width:36px" @click=${(event: MouseEvent): void => event.stopPropagation()}><input type="checkbox" aria-label=${selectRowLabel} .checked=${this._sel.has(i)} @change=${(): void => this._toggle(i)} style="accent-color:var(--brand-500)" /></td>` : nothing}
                        ${cols.map((c: Column): TemplateResult => html`<td style="${td};text-align:${numAlign(c)};${mono(c) ? 'font-family:var(--font-mono);font-variant-numeric:tabular-nums;color:var(--fg-2)' : ''}">${this._cell(c, r)}</td>`)}
                      </tr>`,
                  )
          }
        </tbody>
      </table>
      <div
        style="display:flex;align-items:center;justify-content:space-between;padding:9px 14px;font-size:12px;color:var(--fg-3)"
      >
        <span>${rowSummary}</span>
        ${
          !this.loading && pages > 1
            ? html`<span
                style="display:flex;align-items:center;gap:10px;font-family:var(--font-mono)"
              >
                <button
                  ?disabled=${this.page === 0}
                  @click=${(): void => {
            this.page = Math.max(0, this.page - 1);
          }}
                  aria-label=${previousPageLabel}
                  style="background:none;border:0;color:${this.page === 0 ? 'var(--fg-4)' : 'var(--fg-1)'};cursor:pointer"
                >
                  ‹
                </button>
                ${this.page + 1} / ${pages}
                <button
                  ?disabled=${this.page >= pages - 1}
                  @click=${(): void => {
            this.page = Math.min(pages - 1, this.page + 1);
          }}
                  aria-label=${nextPageLabel}
                  style="background:none;border:0;color:${this.page >= pages - 1 ? 'var(--fg-4)' : 'var(--fg-1)'};cursor:pointer"
                >
                  ›
                </button>
              </span>`
            : nothing
        }
      </div>
    </div>`;
  }
}
define('lthn-datatable', LthnDatatable);

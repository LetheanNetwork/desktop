// SPDX-Licence-Identifier: EUPL-1.2

// <lthn-view-ml-lab-duckdb> — DuckDB tab of the ML Lab workbench,
// per plans/project/lthn/desktop/RFC.ml-lab.md §5. Editor on top
// (SQL — SELECT / WITH / EXPLAIN only, backend enforces),
// paginated result table below.
//
// Backend: POST /v1/ml-lab/duckdb with DuckDBQuery body. Mock
// provider returns shape-tagged stubs for lora_runs +
// lab_saved_queries; everything else returns a stub note row so
// the UX is shaped even before a real DuckDB handle backs the
// substrate (lands when pkg/ml schema migrations promote).
//
// Read-only enforcement: backend handler rejects any SQL whose
// upper-cased trimmed body doesn't start with SELECT/WITH/
// EXPLAIN/SHOW/DESCRIBE — INSERT/UPDATE/DELETE/CREATE/DROP/ALTER
// never reach the underlying DuckDB.

import { LitElement, html, type TemplateResult } from "lit";
import { renderChrome } from "../../chrome";
import { apiFetch } from "../../api-fetch";

interface ColumnSpec {
  name: string;
  kind: string;
}
interface DuckDBResult {
  columns:           ColumnSpec[];
  rows:              Array<Array<string | number | boolean | null>>;
  row_count_total:   number;
  pagination_cursor: string;
}

const DEFAULT_SQL = `SELECT id, base_model, mode, status, last_loss, started_at
FROM lora_runs
ORDER BY started_at DESC
LIMIT 50;`;

const FIXTURE_RESULT: DuckDBResult = {
  columns: [
    { name: "id",         kind: "string" },
    { name: "base_model", kind: "string" },
    { name: "mode",       kind: "string" },
    { name: "status",     kind: "string" },
    { name: "last_loss",  kind: "float" },
    { name: "started_at", kind: "time" },
  ],
  rows: [
    ["run-2026-05-22-12b-v4-p6-rsft",  "lemma-12b-v4-p6",   "sft",     "active",    0.318, "2026-05-22T08:14:02Z"],
    ["run-2026-05-21-3-1b-cl-bpl",     "lemma-3-1b-cl-bpl", "distill", "completed", 0.142, "2026-05-21T19:48:51Z"],
    ["run-2026-05-20-qwen-grpo",       "qwen-3-32b-think",  "grpo",    "paused",    0.587, "2026-05-20T13:02:31Z"],
  ],
  row_count_total: 3,
  pagination_cursor: "",
};

// SCHEMA_BROWSER lists the tables a fresh user sees on the left so
// they can copy-paste a starter query. Source of truth lives in
// pkg/ml schema migrations; this static list mirrors the
// minimum-viable shape until the backend exposes a schema-discovery
// endpoint (TODO follow-up). Each entry is purely cosmetic — the
// editor still accepts any SELECT against any table.
const SCHEMA_BROWSER: Array<{ table: string; columns: string[] }> = [
  { table: "lora_runs",         columns: ["id", "base_model", "mode", "status", "iteration", "total_iters", "last_loss", "started_at", "completed_at", "train_path"] },
  { table: "lab_saved_queries", columns: ["id", "tab", "name", "body", "created_at", "last_used_at", "use_count"] },
  { table: "r1_records",        columns: ["id", "model", "prompt_hash", "score", "tier", "captured_at"] },
];

class LthnViewMlLabDuckDB extends LitElement {
  static readonly properties = {
    embedded:      { type: Boolean, reflect: true },
    selectedModel: { type: String, attribute: "selected-model", reflect: true },
    sql:           { state: true },
    result:        { state: true },
    loading:       { state: true },
    error:         { state: true },
  };
  declare embedded: boolean;
  declare selectedModel: string;
  declare sql: string;
  declare result: DuckDBResult;
  declare loading: boolean;
  declare error: string;

  constructor() {
    super();
    this.embedded = false;
    this.selectedModel = "";
    this.sql = DEFAULT_SQL;
    this.result = FIXTURE_RESULT;
    this.loading = false;
    this.error = "";
  }
  createRenderRoot() { return this; }

  async connectedCallback(): Promise<void> {
    super.connectedCallback();
    await Promise.resolve();
    await this._run();
  }

  private async _run(): Promise<void> {
    this.loading = true;
    this.error = "";
    try {
      const res = await apiFetch("/v1/ml-lab/duckdb", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          sql: this.sql,
          bind_vars: this.selectedModel ? { model: this.selectedModel } : undefined,
        }),
      });
      if (!res.ok) {
        const txt = await res.text();
        this.error = `HTTP ${res.status}: ${txt.slice(0, 200)}`;
        return;
      }
      const result = await res.json() as DuckDBResult;
      if (result.rows && result.rows.length > 0) {
        this.result = result;
      }
    } catch (e) {
      this.error = (e as Error).message ?? "offline";
    } finally {
      this.loading = false;
    }
  }

  private _onSqlInput(e: Event): void {
    this.sql = (e.target as HTMLTextAreaElement).value;
  }
  private _onRun(): void {
    void this._run();
  }
  private _insertTable(table: string): void {
    this.sql = `SELECT * FROM ${table} LIMIT 50;`;
  }

  private _renderSchemaBrowser(): TemplateResult {
    return html`
      <aside style="width:200px;border-right:1px solid var(--colour-border-subtle);overflow:auto;padding:12px 14px;background:var(--colour-surface-1);">
        <div style="font-size:11px;text-transform:uppercase;letter-spacing:0.05em;color:var(--colour-text-tertiary);margin-bottom:8px;">Tables</div>
        ${SCHEMA_BROWSER.map(t => html`
          <details style="margin-bottom:6px;">
            <summary style="cursor:pointer;font-family:var(--font-mono);font-size:12px;color:var(--colour-text-primary);padding:3px 0;">
              <span @click=${(e: Event) => { e.preventDefault(); this._insertTable(t.table); }}>${t.table}</span>
            </summary>
            <div style="padding-left:14px;">
              ${t.columns.map(c => html`
                <div style="font-family:var(--font-mono);font-size:11px;color:var(--colour-text-secondary);padding:2px 0;">${c}</div>
              `)}
            </div>
          </details>
        `)}
      </aside>
    `;
  }

  private _renderTable(): TemplateResult {
    return html`
      <div style="overflow:auto;flex:1;font-family:var(--font-mono);font-size:11px;">
        <table style="width:100%;border-collapse:collapse;">
          <thead style="background:var(--colour-surface-2);position:sticky;top:0;">
            <tr>
              ${this.result.columns.map(c => html`
                <th style="text-align:left;padding:6px 10px;color:var(--colour-text-secondary);font-weight:500;border-bottom:1px solid var(--colour-border);">
                  ${c.name}
                  <span style="color:var(--colour-text-tertiary);font-weight:400;margin-left:6px;">${c.kind}</span>
                </th>
              `)}
            </tr>
          </thead>
          <tbody>
            ${this.result.rows.slice(0, 500).map(r => html`
              <tr>
                ${r.map(v => html`
                  <td style="padding:4px 10px;color:var(--colour-text-primary);border-bottom:1px solid var(--colour-border-subtle);">
                    ${v === null ? "·" : String(v)}
                  </td>
                `)}
              </tr>
            `)}
          </tbody>
        </table>
      </div>
    `;
  }

  render() {
    return renderChrome({
      title: "DuckDB",
      embedded: this.embedded,
      body: html`
        <div style="display:flex;height:100%;">
          ${this._renderSchemaBrowser()}
          <div style="display:flex;flex-direction:column;flex:1;min-width:0;">
            <div style="border-bottom:1px solid var(--colour-border);padding:10px 14px;">
              <textarea
                .value=${this.sql}
                @input=${this._onSqlInput}
                spellcheck="false"
                style="width:100%;height:90px;background:var(--colour-surface-1);border:1px solid var(--colour-border);color:var(--colour-text-primary);padding:8px 12px;border-radius:6px;font-family:var(--font-mono);font-size:12px;resize:vertical;outline:none;"
              ></textarea>
              <div style="display:flex;gap:8px;margin-top:8px;align-items:center;">
                <button type="button" @click=${this._onRun} ?disabled=${this.loading}
                  style="background:var(--colour-accent);color:var(--colour-text-inverse);border:none;padding:6px 14px;border-radius:6px;font-size:12px;cursor:pointer;font-weight:500;">
                  ${this.loading ? "Running…" : "Run"}
                </button>
                ${this.error
                  ? html`<span style="color:var(--colour-danger);font-size:11px;">${this.error}</span>`
                  : html`<span style="color:var(--colour-text-tertiary);font-size:11px;">${this.result.row_count_total} rows</span>`}
              </div>
            </div>
            ${this._renderTable()}
          </div>
        </div>
      `,
    });
  }
}

if (!customElements.get("lthn-view-ml-lab-duckdb")) {
  customElements.define("lthn-view-ml-lab-duckdb", LthnViewMlLabDuckDB);
}

export { LthnViewMlLabDuckDB };

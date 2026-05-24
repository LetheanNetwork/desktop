// SPDX-Licence-Identifier: EUPL-1.2

// <lthn-view-ml-lab-models> — Models tab of the ML Lab workbench,
// per plans/project/lthn/desktop/RFC.ml-lab.md §6. Renders the local
// .model library: file name, model architecture, quant, size, identity
// fingerprint (first 12 chars).
//
// Same canonical wire pattern as the other lit views: lazy backend
// fetch with fixture fallback, 60s refresh poller, vi.mock at test
// module scope. Backend endpoint TODO(snider) — wire to whatever the
// go-ml/go/lab service settles on; current implementation surfaces
// fixture rows so the panel can be styled + reviewed without backend.
//
// The identity fingerprint shown here matches the SHA-256 returned by
// `pack.Fingerprint(manifest)` in go-inference/go/model/pack — same
// logical model => same fingerprint, across machines, irrespective of
// when it was packed.

import { LitElement, html } from "lit";
import { renderChrome } from "../../chrome";
import { apiFetch } from "../../api-fetch";
import { clampRows } from "../_bounds";

interface ModelRow {
  path:        string;   // local filesystem path
  filename:    string;   // basename
  arch:        string;
  quant_bits:  number;
  ctx:         number;
  size_bytes:  number;
  fingerprint: string;   // sha256 hex of the identity projection
  created:     string;   // RFC3339; provenance, not identity
  producer:    string;
}

interface ModelsQueryResponse {
  models?: ModelRow[];
}

// FIXTURE_MODELS shows a hand-curated library a fresh install sees
// before any real .model files are packed. The fingerprints below are
// not real — they are well-shaped synthetic placeholders that round-
// trip cleanly through the column formatter. Real fingerprints take
// over the moment the backend returns anything.
const FIXTURE_MODELS: ModelRow[] = [
  {
    path:        "/Users/agent/Lethean/data/models/lemma-12b-v4-p6.model",
    filename:    "lemma-12b-v4-p6.model",
    arch:        "gemma-4",
    quant_bits:  4,
    ctx:         262144,
    size_bytes:  6_812_345_678,
    fingerprint: "a40925748cf8d8078d5d65e0fd8168828444b9679b2c3ece18724a95d812ac2c",
    created:     "2026-05-12T08:14:22Z",
    producer:    "go-mlx",
  },
  {
    path:        "/Users/agent/Lethean/data/models/lemma-12b-v4-p6.q8.model",
    filename:    "lemma-12b-v4-p6.q8.model",
    arch:        "gemma-4",
    quant_bits:  8,
    ctx:         262144,
    size_bytes:  12_443_998_212,
    fingerprint: "c19fe18b5a3b71d22ae08c7a6c11ff45c0d0e9e6a4172a3d1b6c4f028d0b4eb1",
    created:     "2026-05-13T11:02:45Z",
    producer:    "go-mlx",
  },
  {
    path:        "/Users/agent/Lethean/data/models/lemer-2b-e2b.q4.model",
    filename:    "lemer-2b-e2b.q4.model",
    arch:        "gemma-4",
    quant_bits:  4,
    ctx:         131072,
    size_bytes:  1_543_221_098,
    fingerprint: "2f49ab10c5be6e7e6cd8aa4e4d77f3bb91ce67fe9a13d50281e5b2a4dbf6c7d3",
    created:     "2026-05-08T22:11:09Z",
    producer:    "go-mlx",
  },
  {
    path:        "/Users/agent/Lethean/data/models/lemmy-26b-a4b-moe.q4.model",
    filename:    "lemmy-26b-a4b-moe.q4.model",
    arch:        "gemma-4-moe",
    quant_bits:  4,
    ctx:         262144,
    size_bytes:  13_876_543_210,
    fingerprint: "8e7d6c5b4a392817160514131211100f0e0d0c0b0a09080706050403020100ff",
    created:     "2026-05-14T03:55:13Z",
    producer:    "go-mlx",
  },
];

const REFRESH_MS = 60_000;

class LthnViewMlLabModels extends LitElement {
  static readonly properties = {
    w:        { type: Number },
    h:        { type: Number },
    embedded: { type: Boolean, reflect: true },
    models:   { state: true },
    loading:  { state: true },
    error:    { state: true },
    filter:   { state: true },
  };
  declare w: number;
  declare h: number;
  declare embedded: boolean;
  declare models: ModelRow[];
  declare loading: boolean;
  declare error: string;
  declare filter: string;
  private _timer: ReturnType<typeof setInterval> | null = null;

  constructor() {
    super();
    this.w = 1180;
    this.h = 720;
    this.embedded = false;
    this.models = FIXTURE_MODELS;
    this.loading = false;
    this.error = "";
    this.filter = "";
  }

  createRenderRoot() { return this; }

  async connectedCallback() {
    super.connectedCallback();
    await this._loadFromBackend();
    this._timer = setInterval(() => { void this._loadFromBackend(); }, REFRESH_MS);
  }

  disconnectedCallback() {
    if (this._timer) { clearInterval(this._timer); this._timer = null; }
    super.disconnectedCallback();
  }

  // _loadFromBackend hits GET /v1/ml-lab/models (planned). On any
  // failure / unexpected shape / empty response, keeps the current
  // row list (fixture or previously-loaded models) per the canonical
  // wire pattern's reject-keeps / empty-keeps invariants.
  async _loadFromBackend() {
    if (this.loading) return;
    this.loading = true;
    this.error = "";
    try {
      const res = await apiFetch("/v1/ml-lab/models", {
        method: "GET",
        headers: { "Accept": "application/json" },
      });
      if (!res.ok) {
        if (res.status !== 401) {
          this.error = `models query returned ${res.status}`;
        }
        return;
      }
      const wrapper = await res.json();
      const data = (wrapper as { data?: ModelsQueryResponse })?.data;
      const models = clampRows<ModelRow>(data?.models);
      if (models.length === 0) {
        return; // keep current state per design contract
      }
      this.models = models;
    } catch (e) {
      this.error = (e instanceof Error) ? e.message : "models fetch failed";
    } finally {
      this.loading = false;
    }
  }

  _filtered(): ModelRow[] {
    const q = (this.filter || "").trim().toLowerCase();
    if (!q) return this.models;
    return this.models.filter(m =>
      (m.filename    || "").toLowerCase().includes(q) ||
      (m.arch        || "").toLowerCase().includes(q) ||
      (m.fingerprint || "").toLowerCase().includes(q) ||
      (m.producer    || "").toLowerCase().includes(q)
    );
  }

  _onFilterInput(e: Event) {
    this.filter = (e.target as HTMLInputElement)?.value ?? "";
  }

  _fmtSize(n: number): string {
    if (!Number.isFinite(n) || n <= 0) return "—";
    const units = ["B", "KB", "MB", "GB", "TB"];
    let i = 0;
    let v = n;
    while (v >= 1024 && i < units.length - 1) { v /= 1024; i++; }
    return `${v.toFixed(v < 10 ? 2 : v < 100 ? 1 : 0)} ${units[i]}`;
  }

  _fmtQuant(b: number): string {
    if (!b) return "fp16";
    return `q${b}`;
  }

  _fmtCtx(n: number): string {
    if (!n) return "—";
    if (n >= 1024) return `${Math.round(n / 1024)}k`;
    return String(n);
  }

  _fmtFingerprint(fp: string): string {
    if (!fp || fp.length < 12) return fp || "—";
    return fp.slice(0, 12);
  }

  _fmtTimestamp(iso: string): string {
    if (!iso) return "";
    const d = new Date(iso);
    if (Number.isNaN(d.getTime())) return iso;
    return d.toLocaleString("en-GB", { hour12: false });
  }

  _renderRow(m: ModelRow) {
    return html`
      <tr class="model-row">
        <td class="filename" title=${m.path}>${m.filename}</td>
        <td class="arch">${m.arch}</td>
        <td class="quant">${this._fmtQuant(m.quant_bits)}</td>
        <td class="ctx">${this._fmtCtx(m.ctx)}</td>
        <td class="size">${this._fmtSize(m.size_bytes)}</td>
        <td class="fingerprint" title=${m.fingerprint}>${this._fmtFingerprint(m.fingerprint)}</td>
        <td class="producer">${m.producer || "—"}</td>
        <td class="created">${this._fmtTimestamp(m.created)}</td>
      </tr>
    `;
  }

  render() {
    const rows = this._filtered();
    const body = html`
      <div class="models-toolbar">
        <input
          type="text"
          placeholder="filter by name / arch / fingerprint / producer"
          .value=${this.filter}
          @input=${this._onFilterInput}
          class="filter-input"
        />
        <span class="status">
          ${this.error ? html`<span class="error">${this.error}</span>` : null}
          ${this.loading ? html`<span class="loading">refreshing…</span>` : null}
          <span class="count">${rows.length} of ${this.models.length}</span>
        </span>
      </div>
      <table class="models-table">
        <thead>
          <tr>
            <th>filename</th>
            <th>arch</th>
            <th>quant</th>
            <th>ctx</th>
            <th>size</th>
            <th>fingerprint</th>
            <th>producer</th>
            <th>created</th>
          </tr>
        </thead>
        <tbody>
          ${rows.length === 0
            ? html`<tr><td colspan="8" class="empty-state">No .model files match these filters. Adjust the filter or pack a model with <code>lthn-model-pack pack</code> to populate the library.</td></tr>`
            : rows.map(r => this._renderRow(r))}
        </tbody>
      </table>
    `;
    if (this.embedded) return body;
    return renderChrome({
      title:    "Models",
      subtitle: "Local .model library — Trix containers, content-addressable",
      w:        this.w,
      h:        this.h,
      embedded: false,
      body,
    });
  }
}

if (!customElements.get("lthn-view-ml-lab-models")) {
  customElements.define("lthn-view-ml-lab-models", LthnViewMlLabModels);
}

export { LthnViewMlLabModels };
export type { ModelRow, ModelsQueryResponse };

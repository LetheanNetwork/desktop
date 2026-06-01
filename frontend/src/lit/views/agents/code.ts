// SPDX-Licence-Identifier: EUPL-1.2
// Agents view · Code — <lthn-view-code>
//
// Read-only code viewer for navigating to a finding's source. Seeded by the
// shell (the lthn:open-code event → _instantiate sets repo/file/line/path) and
// reads via Files.Read; renders the file with line numbers, highlighting and
// scrolling to the target line. Closes the loop: a Backlog task → see the
// offending code.
//
// Supports the `embedded` attribute — no chrome when set.

import { LitElement, html, nothing } from "lit";
import { renderChrome } from "../../chrome";

class LthnViewCode extends LitElement {
  static readonly properties = {
    w:        { type: Number },
    h:        { type: Number },
    embedded: { type: Boolean, reflect: true },
    repo:     { type: String }, // set by the shell on open
    file:     { type: String },
    line:     { type: Number },
    path:     { type: String },
    lines:      { state: true },
    loadedPath: { state: true },
    truncated:  { state: true },
    loading:    { state: true },
    err:        { state: true },
  };

  declare w: number;
  declare h: number;
  declare embedded: boolean;
  declare repo: string;
  declare file: string;
  declare line: number;
  declare path: string;
  declare lines: string[];
  declare loadedPath: string;
  declare truncated: boolean;
  declare loading: boolean;
  declare err: string;

  constructor() {
    super();
    this.w = 1180; this.h = 720; this.embedded = false;
    this.repo = ""; this.file = ""; this.line = 0; this.path = "";
    this.lines = []; this.loadedPath = ""; this.truncated = false;
    this.loading = false; this.err = "";
  }

  createRenderRoot() { return this; }

  async connectedCallback() {
    super.connectedCallback();
    await this._load();
  }

  /** Read the seeded file (Files.Read) and split it into lines. */
  async _load() {
    if (!this.repo && !this.file && !this.path) return; // nothing seeded yet
    this.loading = true;
    this.err = "";
    try {
      const svc = await import("@desktop/files/service").catch(() => null);
      const read = svc && (svc as {
        Read?: (i: unknown) => Promise<{ Value?: { content?: string; path?: string; truncated?: boolean } }>;
      }).Read;
      if (!read) { this.err = "file reader unavailable"; this.lines = []; return; }
      const v = (await read({ repo: this.repo, file: this.file, path: this.path }))?.Value ?? {};
      if (typeof v.content !== "string") { this.err = "could not read the file"; this.lines = []; return; }
      this.lines = v.content.split("\n");
      this.loadedPath = v.path ?? "";
      this.truncated = !!v.truncated;
    } catch (e: unknown) {
      this.err = e instanceof Error ? e.message : String(e);
      this.lines = [];
    } finally {
      this.loading = false;
    }
    await this.updateComplete;
    this._scrollToLine();
  }

  /** Centre the highlighted line in the viewport once rendered. */
  _scrollToLine() {
    if (this.line <= 0) return;
    const row = this.querySelector(`[data-ln="${this.line}"]`) as HTMLElement | null;
    if (row && typeof row.scrollIntoView === "function") row.scrollIntoView({ block: "center" });
  }

  render() {
    const label = this.loadedPath || this.path || (this.repo ? `${this.repo}/${this.file}` : this.file) || "—";
    const body = html`
      <div style="flex:1; display:flex; flex-direction:column; min-height:0;">
        <div style="padding:14px 22px 10px; display:flex; align-items:center; gap:10px;">
          <span style="font-family:var(--font-mono); font-size:12px; color:var(--fg-1);
                       overflow:hidden; text-overflow:ellipsis; white-space:nowrap;">${label}</span>
          ${this.line > 0 ? html`<span style="font-family:var(--font-mono); font-size:10.5px; color:var(--brand-300);">:${this.line}</span>` : nothing}
          ${this.truncated ? html`<span style="font-size:10px; color:var(--warning-400);">truncated</span>` : nothing}
        </div>
        ${this.err ? html`<div style="padding:8px 22px; color:var(--err-400); font-size:12px;">${this.err}</div>` : nothing}
        <div style="flex:1; overflow:auto; padding:0 0 18px;">
          ${this.lines.length === 0 ? html`
            <div style="padding:40px; text-align:center; color:var(--fg-3); font-size:12px;">
              ${this.loading ? "Loading…" : "Nothing to show — open a finding from the Backlog."}
            </div>
          ` : html`
            <div style="font-family:var(--font-mono); font-size:12px; line-height:1.5;">
              ${this.lines.map((src, i) => {
                const n = i + 1;
                const hl = n === this.line;
                return html`<div data-ln=${n} style="display:flex; ${hl ? "background:rgba(64,193,197,0.12);" : ""}">
                  <span style="width:52px; flex:0 0 auto; text-align:right; padding:0 12px; color:var(--fg-3); user-select:none;">${n}</span>
                  <span style="white-space:pre; color:var(--fg-0);">${src || " "}</span>
                </div>`;
              })}
            </div>
          `}
        </div>
      </div>
    `;
    return renderChrome({
      title: "Code",
      subtitle: label,
      w: this.w, h: this.h,
      toolbar: nothing,
      body,
      footer: html`read-only source view · Files.Read · open a finding from the Backlog`,
      embedded: this.embedded,
    });
  }
}

customElements.define("lthn-view-code", LthnViewCode);

// SPDX-Licence-Identifier: EUPL-1.2
// Marketing · Content — <lthn-view-content>
//
// Content calendar — kanban-style pipeline across {idea, draft, review,
// ready, live}. Cards carry an author or a publish timestamp. Calm +
// observational; no celebration, no streak counters.
//
// Two-shell: embedded attribute collapses chrome for app-shell mount.
//
// Data: fixture only for v1. Real source is a future
// `pkg/marketing/content` Go service backed by the editorial CMS — the
// column / item / author / due / when shape is the contract.

import { LitElement, html } from "lit";
import { renderChrome } from "../../chrome";

interface ContentItem {
  t:    string;
  who?: string;
  when?: string;
  due?: string;
}

interface ContentColumn {
  id:    "idea" | "draft" | "review" | "ready" | "live";
  label: string;
  items: ContentItem[];
}

const FIXTURE_COLUMNS: ContentColumn[] = [
  { id: "idea", label: "Ideas", items: [
    { t: "Watt-per-token comparison post", who: "you" },
    { t: "Tutorial: connect lthn to Claude Code", who: "Mei" },
  ]},
  { id: "draft", label: "Drafting", items: [
    { t: "v0.2 release notes", who: "you", due: "today" },
    { t: "State of local AI · long-read", who: "Tobi" },
  ]},
  { id: "review", label: "Review", items: [
    { t: "Why we chose EUPL-1.2", who: "you → Mei" },
  ]},
  { id: "ready", label: "Ready", items: [
    { t: "Sovereign-compute manifesto", who: "you" },
  ]},
  { id: "live", label: "Live", items: [
    { t: "Tray-app architecture · v0.1 retro", when: "3 d ago" },
    { t: "Hiring · senior Go engineer", when: "1 w ago" },
  ]},
];

class LthnViewContent extends LitElement {
  static readonly properties = {
    w:        { type: Number },
    h:        { type: Number },
    embedded: { type: Boolean, reflect: true },
    filter:   { state: true },
    columns:  { state: true },
    loading:  { state: true },
  };
  declare w: number;
  declare h: number;
  declare embedded: boolean;
  declare filter: ContentColumn["id"] | "";
  declare columns: ContentColumn[];
  declare loading: boolean;
  private _timer: ReturnType<typeof setInterval> | null = null;

  constructor() {
    super();
    this.w = 1180;
    this.h = 720;
    this.embedded = false;
    this.filter = "";
    this.columns = FIXTURE_COLUMNS;
    this.loading = false;
  }

  createRenderRoot() { return this; }

  async connectedCallback() {
    super.connectedCallback();
    await this._loadFromBackend();
    this._timer = setInterval(() => { void this._loadFromBackend(); }, 60_000);
  }

  disconnectedCallback() {
    if (this._timer) { clearInterval(this._timer); this._timer = null; }
    super.disconnectedCallback();
  }

  async _loadFromBackend(): Promise<void> {
    if (this.loading) return;
    this.loading = true;
    try {
      const svc = await import("@desktop/marketing/content/service").catch(() => null);
      if (!svc || typeof (svc as { List?: unknown }).List !== "function") return;
      const r = await (svc as {
        List: (input: { col?: string }) => Promise<{ Value?: { columns?: ContentColumn[] } }>
      }).List({});
      const columns = r?.Value?.columns;
      if (columns && columns.length > 0) this.columns = columns;
    } catch {
      // Binding unavailable — keep fixture data.
    } finally {
      this.loading = false;
    }
  }

  _setFilter(id: ContentColumn["id"] | "") {
    this.filter = this.filter === id ? "" : id;
  }

  render() {
    const cols = this.filter
      ? this.columns.filter(c => c.id === this.filter)
      : this.columns;
    const totalInFlight = this.columns
      .filter(c => c.id !== "live")
      .reduce((s, c) => s + c.items.length, 0);
    const dueToday = this.columns
      .flatMap(c => c.items)
      .filter(it => it.due === "today").length;

    const gridCols = cols.length > 0 ? `repeat(${cols.length}, 1fr)` : "1fr";

    const body = html`
      <div class="lthn-view-content" style="flex:1; padding:18px 22px; overflow:auto;">
        <div style="display:grid; grid-template-columns: ${gridCols}; gap:12px; min-width:0;">
          ${cols.map(c => html`
            <div style="display:flex; flex-direction:column; gap:8px;">
              <div class="lthn-view-content-col-header" data-col=${c.id} @click=${() => this._setFilter(c.id)} style="display:flex; align-items:center; gap:8px; padding:0 4px 4px; cursor:pointer;">
                <span style="font-family:var(--font-mono); font-size:10px; color:var(--fg-3); letter-spacing:0.10em; text-transform:uppercase;">${c.label}</span>
                <span style="font-family:var(--font-mono); font-size:10px; color:var(--fg-2); background:rgba(255,255,255,0.06); padding:1px 7px; border-radius:999px;">${c.items.length}</span>
              </div>
              ${c.items.map(it => html`
                <div class="lthn-view-content-card" style="padding:12px 14px; border-radius:8px; background:rgba(255,255,255,0.025); border:1px solid rgba(255,255,255,0.06);">
                  <div style="font-size:12.5px; color:var(--fg-0); line-height:1.4;">${it.t}</div>
                  <div style="font-family:var(--font-mono); font-size:10px; color:var(--fg-3); margin-top:8px;">${it.who ?? it.when ?? ""}${it.due ? ` · due ${it.due}` : ""}</div>
                </div>
              `)}
            </div>
          `)}
        </div>
      </div>
    `;

    return renderChrome({
      title:    "Content calendar",
      subtitle: `${totalInFlight} posts in flight${dueToday ? ` · ${dueToday} due today` : ""}`,
      w: this.w, h: this.h,
      embedded: this.embedded,
      body,
      footer: html`drag cards to advance · ⌘N to write · publishes to lthn.ai/blog`,
    });
  }
}

customElements.define("lthn-view-content", LthnViewContent);

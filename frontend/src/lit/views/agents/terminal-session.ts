// SPDX-Licence-Identifier: EUPL-1.2
// Agents view · Terminal session — <lthn-terminal-session>
//
// One PTY-backed xterm session. The tab manager (<lthn-view-terminal>) hosts N
// of these — one per tab — and shows the active one. Chrome lives in the
// manager; this element is just the terminal body + ⌘F search overlay.
//
// Two ways a session starts (see pkg/terminal — Open spawns, Attach streams):
//   - repo/cwd  → Open a fresh shell there (the normal case)
//   - attachId  → Attach to an EXISTING pool session by ID (no spawn). This is
//     the "watch a running agent's terminal" hook: any session already in the
//     pool can be surfaced as a tab — one prop, no new transport.
//
// Reports up to the manager via bubbling events:
//   lthn-term-ready {key, title, cwd, shell}   — session opened, label it
//   lthn-term-exit  {key}                       — shell exited

import { LitElement, html, nothing } from "lit";
import { Terminal } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import { WebLinksAddon } from "@xterm/addon-web-links";
import { SearchAddon } from "@xterm/addon-search";
import { ClipboardAddon } from "@xterm/addon-clipboard";
import { UnicodeGraphemesAddon } from "@xterm/addon-unicode-graphemes";
import "@xterm/xterm/css/xterm.css";

/** The Terminal Wails service surface (plain bindings → @desktop/terminal/service). */
interface TermSvc {
  Open?:   (i: unknown) => Promise<{ Value?: { id?: string; shell?: string; cwd?: string } }>;
  Attach?: (i: unknown) => Promise<unknown>;
  Write?:  (i: unknown) => Promise<unknown>;
  Resize?: (i: unknown) => Promise<unknown>;
  Close?:  (i: unknown) => Promise<unknown>;
}

/** Search highlight colours — neutral amber so matches read against the dark bg
 *  without colliding with the brand cursor/selection. */
const SEARCH_DECORATIONS = {
  matchBackground: "#4d3b00",
  matchBorder: "#7a5e00",
  matchOverviewRuler: "#7a5e00",
  activeMatchBackground: "#b3860b",
  activeMatchBorder: "#ffd966",
  activeMatchColorOverviewRuler: "#ffd966",
};

/** Decode a base64 output chunk from the event bus into raw bytes for xterm. */
function b64ToBytes(b64: string): Uint8Array {
  const bin = atob(b64);
  const bytes = new Uint8Array(bin.length);
  for (let i = 0; i < bin.length; i++) bytes[i] = bin.charCodeAt(i);
  return bytes;
}

/** Last path segment of a dir, for a tab label. */
function basename(p: string): string {
  const parts = (p || "").split("/").filter(Boolean);
  return parts.length ? parts[parts.length - 1] : "";
}

class LthnTerminalSession extends LitElement {
  static readonly properties = {
    tabKey:   { type: String },
    repo:     { type: String },
    cwd:      { type: String },
    attachId: { type: String },
    active:   { type: Boolean },
    err:        { state: true },
    searchOpen: { state: true },
    searchTerm: { state: true },
    searchCount: { state: true },
  };

  declare tabKey: string;
  declare repo: string;
  declare cwd: string;
  declare attachId: string;
  declare active: boolean;
  declare err: string;
  declare searchOpen: boolean;
  declare searchTerm: string;
  declare searchCount: string;

  private term?: Terminal;
  private fitAddon?: FitAddon;
  private searchAddon?: SearchAddon;
  private resizeObserver?: ResizeObserver;
  private offHandlers: Array<() => void> = [];
  private svc?: TermSvc;
  private sessionId = "";
  private booted = false;
  private disposed = false;
  private refitPending = false;

  constructor() {
    super();
    this.tabKey = ""; this.repo = ""; this.cwd = ""; this.attachId = ""; this.active = false;
    this.err = ""; this.searchOpen = false; this.searchTerm = ""; this.searchCount = "";
  }

  createRenderRoot() { return this; }

  connectedCallback() {
    super.connectedCallback();
    this.style.flex = "1";
    this.style.minWidth = "0";
    this.style.minHeight = "0";
    this.style.display = "flex";
  }

  firstUpdated() { void this._boot(); }

  updated(changed: Map<string, unknown>) {
    // When this tab becomes the active one, its host went display:none →
    // visible, so xterm needs a refit to the now-real size + focus.
    if (changed.has("active") && this.active) {
      this._refit();
      requestAnimationFrame(() => { try { this.term?.focus(); } catch { /* not ready */ } });
    }
  }

  disconnectedCallback() {
    super.disconnectedCallback();
    this.disposed = true;
    this.resizeObserver?.disconnect();
    for (const off of this.offHandlers) { try { off(); } catch { /* already gone */ } }
    this.offHandlers = [];
    // Close only sessions we spawned; an attached (agent) session outlives the tab.
    if (this.sessionId && !this.attachId && this.svc?.Close) {
      void this.svc.Close({ id: this.sessionId }).catch(() => { /* tearing down */ });
    }
    if (this.term) { try { this.term.dispose(); } catch { /* already torn down */ } }
  }

  /** Resolve a Lethean CSS token to an rgb() string xterm can parse (tokens are
   *  oklch(), which xterm's colour parser can't read — a hidden probe lets the
   *  browser resolve var() → rgb()). */
  private rgbVar(name: string, fallback: string): string {
    try {
      const probe = document.createElement("span");
      probe.style.color = `var(${name})`;
      probe.style.position = "absolute";
      probe.style.visibility = "hidden";
      document.body.appendChild(probe);
      const c = getComputedStyle(probe).color;
      probe.remove();
      return c || fallback;
    } catch { return fallback; }
  }

  private theme() {
    return {
      background: this.rgbVar("--ink-1", "#16151c"),
      foreground: this.rgbVar("--fg-1", "#e5e7eb"),
      cursor: this.rgbVar("--brand-400", "#40c1c5"),
      cursorAccent: this.rgbVar("--ink-1", "#16151c"),
      selectionBackground: "rgba(64,193,197,0.30)",
      black: "#1f2937", red: "#f87171", green: "#34d399", yellow: "#fbbf24",
      blue: "#60a5fa", magenta: "#c084fc", cyan: "#22d3ee", white: "#e5e7eb",
      brightBlack: "#475569", brightRed: "#fca5a5", brightGreen: "#6ee7b7",
      brightYellow: "#fcd34d", brightBlue: "#93c5fd", brightMagenta: "#d8b4fe",
      brightCyan: "#67e8f9", brightWhite: "#f8fafc",
    };
  }

  /** rAF-debounced fit + PTY resize (see the long-form note in the manager). */
  private _refit() {
    if (this.refitPending) return;
    this.refitPending = true;
    requestAnimationFrame(() => {
      this.refitPending = false;
      if (this.disposed || !this.term || !this.fitAddon || !this.sessionId) return;
      try { this.fitAddon.fit(); } catch { return; }
      void this.svc?.Resize?.({ id: this.sessionId, cols: this.term.cols, rows: this.term.rows }).catch(() => {});
    });
  }

  private async _boot() {
    if (this.booted) return;
    this.booted = true;

    const host = this.querySelector("#term-host") as HTMLDivElement | null;
    if (!host) { this.err = "terminal host not found"; return; }

    const fontMono = getComputedStyle(document.documentElement)
      .getPropertyValue("--font-mono").trim() || "Menlo, Monaco, monospace";

    const term = new Terminal({
      cursorBlink: true, fontFamily: fontMono, fontSize: 13, lineHeight: 1.2,
      scrollback: 5000, allowProposedApi: true, theme: this.theme(),
    });
    this.term = term;

    const fit = new FitAddon();
    this.fitAddon = fit;
    const search = new SearchAddon();
    this.searchAddon = search;
    term.loadAddon(fit);
    term.loadAddon(new WebLinksAddon());
    term.loadAddon(search);
    term.loadAddon(new ClipboardAddon());
    term.loadAddon(new UnicodeGraphemesAddon());
    const gv = term.unicode.versions.find((v) => v.includes("graphemes"));
    if (gv) term.unicode.activeVersion = gv;

    search.onDidChangeResults(({ resultIndex, resultCount }) => {
      this.searchCount = resultCount > 0 ? `${resultIndex + 1}/${resultCount}` : (this.searchTerm ? "0/0" : "");
    });
    term.attachCustomKeyEventHandler((e) => {
      if (e.type === "keydown" && (e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "f") {
        this._openSearch();
        return false;
      }
      return true;
    });

    try { await (document as Document & { fonts?: FontFaceSet }).fonts?.ready; } catch { /* no FontFaceSet */ }

    term.open(host);
    try {
      const { WebglAddon } = await import("@xterm/addon-webgl");
      const webgl = new WebglAddon();
      webgl.onContextLoss(() => { try { webgl.dispose(); } catch { /* gone */ } });
      term.loadAddon(webgl);
    } catch { /* no WebGL2 — canvas/DOM renderer */ }
    try { fit.fit(); } catch { /* corrected by _refit */ }

    const svc = await import("@desktop/terminal/service").catch(() => null) as TermSvc | null;
    if (!svc) { this.err = "terminal service unavailable"; return; }
    this.svc = svc;

    let id = "";
    let title = this.repo;
    if (this.attachId) {
      // Attach to an existing pool session (e.g. a running agent's PTY).
      id = this.attachId;
      title = title || "↪ " + id.slice(0, 8);
    } else {
      if (!svc.Open) { this.err = "terminal service unavailable"; return; }
      let opened;
      try {
        opened = (await svc.Open({ repo: this.repo, cwd: this.cwd, term: "xterm-256color", cols: term.cols, rows: term.rows }))?.Value;
      } catch (e: unknown) {
        this.err = "open: " + (e instanceof Error ? e.message : String(e));
        return;
      }
      if (this.disposed) return;
      if (!opened?.id) { this.err = "failed to open a shell session"; return; }
      id = opened.id;
      title = title || basename(opened.cwd || "") || "shell";
      this._emit("lthn-term-ready", { key: this.tabKey, title, cwd: opened.cwd || "", shell: opened.shell || "" });
    }
    this.sessionId = id;

    const { Events } = await import("@wailsio/runtime");
    this.offHandlers.push(Events.On("lthn:term:out:" + id, (e: { data?: string }) => {
      if (e?.data) term.write(b64ToBytes(e.data));
    }));
    this.offHandlers.push(Events.On("lthn:term:exit:" + id, () => {
      term.write("\r\n\x1b[2;33m[process exited]\x1b[0m\r\n");
      this._emit("lthn-term-exit", { key: this.tabKey });
    }));

    term.onData((data) => { void svc.Write?.({ id, data }).catch(() => {}); });
    await svc.Attach?.({ id }).catch(() => {});
    if (this.attachId) this._emit("lthn-term-ready", { key: this.tabKey, title, cwd: "", shell: "" });

    this._refit();
    this.resizeObserver = new ResizeObserver(() => this._refit());
    this.resizeObserver.observe(host);
    if (this.active) term.focus();
  }

  private _emit(name: string, detail: Record<string, unknown>) {
    this.dispatchEvent(new CustomEvent(name, { detail, bubbles: true, composed: true }));
  }

  // --- search overlay ---

  private async _openSearch() {
    this.searchOpen = true;
    await this.updateComplete;
    const input = this.querySelector("#term-search-input") as HTMLInputElement | null;
    if (input) { input.focus(); input.select(); }
    if (this.searchTerm) this._find(true);
  }
  private _closeSearch() {
    this.searchOpen = false;
    this.searchCount = "";
    try { this.searchAddon?.clearDecorations(); } catch { /* no active search */ }
    this.term?.focus();
  }
  private _find(forward: boolean) {
    const t = this.searchTerm;
    if (!t || !this.searchAddon) return;
    const opts = { decorations: SEARCH_DECORATIONS };
    if (forward) this.searchAddon.findNext(t, opts);
    else this.searchAddon.findPrevious(t, opts);
  }
  private _onSearchInput(e: Event) { this.searchTerm = (e.target as HTMLInputElement).value; this._find(true); }
  private _onSearchKey(e: KeyboardEvent) {
    if (e.key === "Enter") { e.preventDefault(); this._find(!e.shiftKey); }
    else if (e.key === "Escape") { e.preventDefault(); this._closeSearch(); }
  }
  private _searchBar() {
    return html`
      <div style="position:absolute; top:8px; right:14px; z-index:5; display:flex; align-items:center; gap:6px;
                  padding:5px 8px; border-radius:8px; background:var(--ink-3); border:1px solid rgba(255,255,255,0.10);
                  box-shadow:0 6px 18px rgba(0,0,0,0.45);">
        <i class="fa-solid fa-magnifying-glass" style="font-size:10px; color:var(--fg-3);"></i>
        <input id="term-search-input" .value=${this.searchTerm} @input=${(e: Event) => this._onSearchInput(e)}
               @keydown=${(e: KeyboardEvent) => this._onSearchKey(e)} placeholder="find in terminal"
               style="background:transparent; border:none; outline:none; color:var(--fg-0); font-size:12px;
                      font-family:var(--font-mono); width:180px;" />
        <span style="font-family:var(--font-mono); font-size:10.5px; color:var(--fg-3); min-width:34px; text-align:right;">${this.searchCount}</span>
        <i class="fa-solid fa-chevron-up" title="Previous (⇧⏎)" @click=${() => this._find(false)}
           style="font-size:10px; color:var(--fg-2); cursor:pointer; padding:2px;"></i>
        <i class="fa-solid fa-chevron-down" title="Next (⏎)" @click=${() => this._find(true)}
           style="font-size:10px; color:var(--fg-2); cursor:pointer; padding:2px;"></i>
        <i class="fa-solid fa-xmark" title="Close (Esc)" @click=${() => this._closeSearch()}
           style="font-size:11px; color:var(--fg-3); cursor:pointer; padding:2px;"></i>
      </div>
    `;
  }

  render() {
    return html`
      <div style="flex:1; width:100%; min-width:0; min-height:0; display:flex; flex-direction:column; overflow:hidden;">
        ${this.err ? html`<div style="padding:6px 18px; color:var(--err-400); font-size:11.5px; flex-shrink:0;">${this.err}</div>` : nothing}
        <div style="flex:1; min-height:0; min-width:0; position:relative; overflow:hidden;">
          ${this.searchOpen ? this._searchBar() : nothing}
          <div id="term-host" style="position:absolute; inset:0; padding:8px 12px 4px;
               background:var(--ink-1); overflow:hidden;"></div>
        </div>
      </div>
    `;
  }
}

customElements.define("lthn-terminal-session", LthnTerminalSession);

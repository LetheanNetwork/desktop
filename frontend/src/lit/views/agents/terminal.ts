// SPDX-Licence-Identifier: EUPL-1.2
// Agents view · Terminal — <lthn-view-terminal>
//
// A real PTY-backed shell in the app — the surface that lets the desktop stand
// in for an external editor's console. xterm.js renders; the bytes ride the
// Wails event bus (the desktop has no always-on localhost listener in GUI mode,
// and retired its /internal HTTP surface for events). The Go side is pkg/terminal:
//
//   Terminal.Open({repo|cwd, cols, rows}) → { id, shell, cwd }
//     → register Events.On("lthn:term:out:<id>")   ← output (base64) + scrollback
//       register Events.On("lthn:term:exit:<id>")  ← shell exited
//     → Terminal.Attach({id})                       starts the stream (race-free:
//                                                    listeners exist before bytes)
//   term.onData → Terminal.Write({id, data})        keystrokes down
//   ResizeObserver → Terminal.Resize({id, cols, rows})
//
// Addons (the GoLand-parity set): webgl (GPU renderer, falls back to canvas on
// context loss), search (⌘F overlay), clipboard (OSC 52), unicode-graphemes
// (correct emoji/CJK width), web-links. (addon-web-fonts needs the xterm 6.1
// beta core, so font-readiness is handled with document.fonts.ready instead.)
//
// Seeded by the shell with an optional repo/cwd (open here); defaults to $HOME.
// Supports the `embedded` attribute — no chrome when set.

import { LitElement, html, nothing } from "lit";
import { Terminal } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import { WebLinksAddon } from "@xterm/addon-web-links";
import { SearchAddon } from "@xterm/addon-search";
import { ClipboardAddon } from "@xterm/addon-clipboard";
import { UnicodeGraphemesAddon } from "@xterm/addon-unicode-graphemes";
import "@xterm/xterm/css/xterm.css";
import { renderChrome } from "../../chrome";

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

class LthnViewTerminal extends LitElement {
  static readonly properties = {
    w:        { type: Number },
    h:        { type: Number },
    embedded: { type: Boolean, reflect: true },
    repo:     { type: String }, // optional, seeded by the shell — open here
    cwd:      { type: String },
    sessionId:  { state: true },
    shell:      { state: true },
    cwdLabel:   { state: true },
    err:        { state: true },
    searchOpen: { state: true },
    searchTerm: { state: true },
    searchCount: { state: true },
  };

  declare w: number;
  declare h: number;
  declare embedded: boolean;
  declare repo: string;
  declare cwd: string;
  declare sessionId: string;
  declare shell: string;
  declare cwdLabel: string;
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
  private booted = false;
  private disposed = false;
  private refitPending = false;

  constructor() {
    super();
    this.w = 1180; this.h = 720; this.embedded = false;
    this.repo = ""; this.cwd = "";
    this.sessionId = ""; this.shell = ""; this.cwdLabel = ""; this.err = "";
    this.searchOpen = false; this.searchTerm = ""; this.searchCount = "";
  }

  createRenderRoot() { return this; }

  connectedCallback() {
    super.connectedCallback();
    // The shell mounts panes as items in a flex row. Content-heavy panes
    // (code, backlog) fill it via their intrinsic width, but the terminal's
    // xterm host is out of flow (see render()), so this element has no content
    // width and would shrink-wrap to the header (~257px). Tell it to grow.
    this.style.flex = "1";
    this.style.minWidth = "0";
  }

  firstUpdated() { void this._boot(); }

  disconnectedCallback() {
    super.disconnectedCallback();
    this.disposed = true;
    this.resizeObserver?.disconnect();
    for (const off of this.offHandlers) { try { off(); } catch { /* already gone */ } }
    this.offHandlers = [];
    const id = this.sessionId;
    if (id && this.svc?.Close) { void this.svc.Close({ id }).catch(() => { /* tearing down */ }); }
    if (this.term) { try { this.term.dispose(); } catch { /* xterm already torn down */ } }
  }

  /** Resolve a Lethean CSS token to an rgb() string xterm can parse (the token
   *  values are oklch(), which xterm's colour parser doesn't understand — a
   *  hidden probe lets the browser resolve var() → rgb()). */
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
      // Neutral ANSI-16 (Tomorrow Night lineage) so tool output stays legible;
      // brand colour only bleeds into cursor + selection above.
      black: "#1f2937", red: "#f87171", green: "#34d399", yellow: "#fbbf24",
      blue: "#60a5fa", magenta: "#c084fc", cyan: "#22d3ee", white: "#e5e7eb",
      brightBlack: "#475569", brightRed: "#fca5a5", brightGreen: "#6ee7b7",
      brightYellow: "#fcd34d", brightBlue: "#93c5fd", brightMagenta: "#d8b4fe",
      brightCyan: "#67e8f9", brightWhite: "#f8fafc",
    };
  }

  /** Refit xterm to its container and push the new size to the PTY. Debounced
   *  to one fit per animation frame: a synchronous fit inside a ResizeObserver
   *  callback re-triggers the observer ("ResizeObserver loop" warning) and the
   *  corrective resize gets throttled away — the rAF breaks that cycle, and the
   *  deferral also lets xterm finish measuring its font cell before the first
   *  fit (an immediate post-open fit mis-sizes to a tiny column). */
  private _refit(id: string) {
    if (this.refitPending) return;
    this.refitPending = true;
    requestAnimationFrame(() => {
      this.refitPending = false;
      if (this.disposed || !this.term || !this.fitAddon) return;
      try { this.fitAddon.fit(); } catch { return; }
      void this.svc?.Resize?.({ id, cols: this.term.cols, rows: this.term.rows }).catch(() => {});
    });
  }

  /** Stand up xterm, open a PTY session, and wire the event stream. */
  private async _boot() {
    if (this.booted) return;
    this.booted = true;

    const host = this.querySelector("#term-host") as HTMLDivElement | null;
    if (!host) { this.err = "terminal host not found"; return; }

    const fontMono = getComputedStyle(document.documentElement)
      .getPropertyValue("--font-mono").trim() || "Menlo, Monaco, monospace";

    const term = new Terminal({
      cursorBlink: true,
      fontFamily: fontMono,
      fontSize: 13,
      lineHeight: 1.2,
      scrollback: 5000,
      allowProposedApi: true,
      theme: this.theme(),
    });
    this.term = term;

    const fit = new FitAddon();
    this.fitAddon = fit;
    const search = new SearchAddon();
    this.searchAddon = search;
    term.loadAddon(fit);
    term.loadAddon(new WebLinksAddon());
    term.loadAddon(search);
    term.loadAddon(new ClipboardAddon());            // OSC 52 system clipboard
    term.loadAddon(new UnicodeGraphemesAddon());     // correct emoji / CJK width
    const gv = term.unicode.versions.find((v) => v.includes("graphemes"));
    if (gv) term.unicode.activeVersion = gv;

    // Match count for the search bar.
    search.onDidChangeResults(({ resultIndex, resultCount }) => {
      this.searchCount = resultCount > 0 ? `${resultIndex + 1}/${resultCount}` : (this.searchTerm ? "0/0" : "");
    });

    // ⌘F / Ctrl+F opens the search overlay instead of going to the shell.
    term.attachCustomKeyEventHandler((e) => {
      if (e.type === "keydown" && (e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "f") {
        this._openSearch();
        return false;
      }
      return true;
    });

    // Wait for the mono webfont before xterm measures its cell (a mis-measure
    // here is what makes the first fit tiny). Best-effort — never block boot.
    try { await (document as Document & { fonts?: FontFaceSet }).fonts?.ready; } catch { /* no FontFaceSet */ }

    term.open(host);
    // GPU renderer — big win under heavy output; gracefully drop to the canvas
    // renderer if the WebGL context is lost or can't be created.
    try {
      const { WebglAddon } = await import("@xterm/addon-webgl");
      const webgl = new WebglAddon();
      webgl.onContextLoss(() => { try { webgl.dispose(); } catch { /* already gone */ } });
      term.loadAddon(webgl);
    } catch { /* no WebGL2 — canvas/DOM renderer stays */ }
    try { fit.fit(); } catch { /* font metrics not ready — _refit corrects it */ }

    const svc = await import("@desktop/terminal/service").catch(() => null) as TermSvc | null;
    if (!svc?.Open) { this.err = "terminal service unavailable"; return; }
    this.svc = svc;

    let opened;
    try {
      opened = (await svc.Open({ repo: this.repo, cwd: this.cwd, term: "xterm-256color", cols: term.cols, rows: term.rows }))?.Value;
    } catch (e: unknown) {
      this.err = "open: " + (e instanceof Error ? e.message : String(e));
      return;
    }
    if (this.disposed) return;
    if (!opened?.id) { this.err = "failed to open a shell session"; return; }

    const id = opened.id;
    this.sessionId = id;
    this.shell = opened.shell || "";
    this.cwdLabel = opened.cwd || "";

    // Register listeners BEFORE Attach so no output (or scrollback replay) is
    // produced before we're listening.
    const { Events } = await import("@wailsio/runtime");
    this.offHandlers.push(Events.On("lthn:term:out:" + id, (e: { data?: string }) => {
      if (e?.data) term.write(b64ToBytes(e.data));
    }));
    this.offHandlers.push(Events.On("lthn:term:exit:" + id, () => {
      term.write("\r\n\x1b[2;33m[process exited]\x1b[0m\r\n");
    }));

    // Keystrokes down. Fire-and-forget: a paste arrives as a single onData call,
    // and human keystrokes are spaced far beyond IPC latency, so ordering holds.
    term.onData((data) => { void svc.Write?.({ id, data }).catch(() => {}); });

    await svc.Attach?.({ id }).catch(() => {});

    // Correct the initial fit now that the pane is laid out + xterm measured,
    // then keep refitting whenever the container changes (sidebar, window).
    this._refit(id);
    this.resizeObserver = new ResizeObserver(() => this._refit(id));
    this.resizeObserver.observe(host);
    term.focus();
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

  private _onSearchInput(e: Event) {
    this.searchTerm = (e.target as HTMLInputElement).value;
    this._find(true);
  }

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
    const status = this.err
      ? html`<span style="color:var(--err-400);">${this.err}</span>`
      : this.sessionId
        ? html`<code style="font-family:var(--font-mono); color:var(--brand-200);">${this.sessionId.slice(0, 12)}</code> · ${this.shell} · ${this.cwdLabel}`
        : html`connecting…`;

    // The xterm host is positioned absolute:inset-0 inside a relative,
    // flex-bounded wrapper. This is deliberate, not cosmetic: a plain flex
    // child lets xterm's wide content dictate the host's width, which feeds
    // back into FitAddon (more cols → wider canvas → wider host → …) and runs
    // the width away to tens of thousands of px. Taking the host out of flow
    // pins its size to the wrapper, so fit always measures a bounded box. The
    // search overlay also anchors to this relative wrapper.
    const body = html`
      <div style="flex:1; width:100%; min-width:0; display:flex; flex-direction:column; min-height:0; overflow:hidden;">
        <div style="padding:10px 18px; font-size:11.5px; color:var(--fg-3);
                    border-bottom:1px solid rgba(255,255,255,0.05); flex-shrink:0;">
          ${status}
        </div>
        <div style="flex:1; min-height:0; min-width:0; position:relative; overflow:hidden;">
          ${this.searchOpen ? this._searchBar() : nothing}
          <div id="term-host" style="position:absolute; inset:0; padding:8px 12px 4px;
               background:var(--ink-1); overflow:hidden;"></div>
        </div>
      </div>
    `;

    return renderChrome({
      title: "Terminal",
      subtitle: this.cwdLabel || "PTY shell — your machine, in the app",
      w: this.w, h: this.h,
      toolbar: nothing,
      body,
      footer: html`pkg/terminal · ${this.shell || "shell"} · ⌘F search · bytes over the event bus`,
      embedded: this.embedded,
    });
  }
}

customElements.define("lthn-view-terminal", LthnViewTerminal);

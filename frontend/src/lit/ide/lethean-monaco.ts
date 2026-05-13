// SPDX-Licence-Identifier: EUPL-1.2
// <lethean-monaco> — Monaco editor mounted inside the lthn shell.
//
// Ported from core/ide (frontend/src/elements/editor/lethean-monaco.ts)
// to lthn/desktop's Lit convention (static properties + declare, no
// decorators) so it slots into the existing vite + tsc pipeline
// without a build-config change.

import { LitElement, html, type PropertyValues } from "lit";
import type * as monacoTypes from "monaco-editor";

// Monaco's pre-built distribution lives at /monaco/vs/ (synced from
// node_modules/monaco-editor/min/vs into frontend/public/monaco/vs
// by package.json's `monaco:sync` script). The AMD loader
// (vs/loader.js) bootstraps everything — fonts, languages, workers
// — without going through Vite's esbuild bundler. That avoids the
// "no loader for .ttf" problem the bundled-import path hits.
//
// One-time global load; subsequent <lethean-monaco> instances reuse
// the same monaco namespace.
declare global {
  interface Window {
    monaco?: typeof monacoTypes;
    require?: {
      (deps: string[], cb: () => void): void;
      config: (opts: { paths: Record<string, string> }) => void;
    };
    MonacoEnvironment?: monacoTypes.Environment;
  }
}

let monacoLoadingPromise: Promise<typeof monacoTypes> | null = null;

const editorOptionKeys = [
  "fontSize", "tabSize", "wordWrap", "lineNumbers", "minimap",
  "renderWhitespace", "readonly", "theme",
] as const;

function ensureMonacoLoaded(): Promise<typeof monacoTypes> {
  if (window.monaco) return Promise.resolve(window.monaco);
  if (monacoLoadingPromise) return monacoLoadingPromise;

  // Skip real workers — degrades TS IntelliSense but keeps
  // highlighting / find / multi-cursor / indentation. Wire workers
  // later if we ship a build with language services as a hard
  // requirement (needs cross-origin-isolated headers which Wails'
  // AssetServer doesn't currently set).
  window.MonacoEnvironment = {
    getWorker: () => {
      const blob = new Blob(["self.onmessage=function(){};"], {
        type: "application/javascript",
      });
      return new Worker(URL.createObjectURL(blob));
    },
  };

  monacoLoadingPromise = new Promise((resolve, reject) => {
    const script = document.createElement("script");
    script.src = "/monaco/vs/loader.js";
    script.onload = () => {
      const req = window.require;
      if (!req) {
        reject(new Error("monaco loader.js did not provide global require"));
        return;
      }
      req.config({ paths: { vs: "/monaco/vs" } });
      req(["vs/editor/editor.main"], () => {
        if (window.monaco) {
          resolve(window.monaco);
        } else {
          reject(new Error("monaco loader completed without exporting monaco"));
        }
      });
    };
    script.onerror = () => reject(new Error("failed to fetch /monaco/vs/loader.js"));
    document.head.appendChild(script);
  });
  return monacoLoadingPromise;
}

/**
 * <lethean-monaco>
 *
 * Monaco editor mounted inside an lthn surface. Light DOM so the host's
 * flex layout cascades into the editor's host div without shadow
 * boundary fights.
 *
 * Properties:
 *   value      — current editor content
 *   language   — Monaco language id (typescript, go, markdown, …)
 *   path       — display only; carried through change / save events
 *   readonly   — when true, edits are rejected
 *   theme      — vs / vs-dark / hc-black (Lethean canon is vs-dark)
 *   font-size  — px font size (default 12.5)
 *   tab-size   — tab width (default 2)
 *   word-wrap  — on / off / wordWrapColumn / bounded
 *   line-numbers — on / off / relative / interval
 *   minimap    — show minimap rail (default false)
 *   render-whitespace — none / boundary / selection / trailing / all
 *
 * Events:
 *   lethean-editor-save   — Ctrl/Cmd+S, detail: { path, value }
 *   lethean-editor-change — every edit, detail: { path, value }
 */
class LetheanMonaco extends LitElement {
  static properties = {
    value:            { type: String },
    language:         { type: String },
    path:             { type: String },
    readonly:         { type: Boolean },
    theme:            { type: String },
    fontSize:         { type: Number, attribute: "font-size" },
    tabSize:          { type: Number, attribute: "tab-size" },
    wordWrap:         { type: String, attribute: "word-wrap" },
    lineNumbers:      { type: String, attribute: "line-numbers" },
    minimap:          { type: Boolean },
    renderWhitespace: { type: String, attribute: "render-whitespace" },
    mounted:          { state: true },
  };
  declare value: string;
  declare language: string;
  declare path: string;
  declare readonly: boolean;
  declare theme: "vs" | "vs-dark" | "hc-black";
  declare fontSize: number;
  declare tabSize: number;
  declare wordWrap: "on" | "off" | "wordWrapColumn" | "bounded";
  declare lineNumbers: "on" | "off" | "relative" | "interval";
  declare minimap: boolean;
  declare renderWhitespace: "none" | "boundary" | "selection" | "trailing" | "all";
  declare mounted: boolean;

  private editor?: monacoTypes.editor.IStandaloneCodeEditor;
  private resizeObs?: ResizeObserver;
  // One model per path → each tab keeps its own undo stack, cursor,
  // scroll position. Models live until closeModel() is called or
  // the element disconnects.
  private models = new Map<string, monacoTypes.editor.ITextModel>();
  private activePath?: string;
  private suppressNextChangeEvent = false;

  constructor() {
    super();
    this.value = "";
    this.language = "plaintext";
    this.path = "";
    this.readonly = false;
    this.theme = "vs-dark";
    this.fontSize = 12.5;
    this.tabSize = 2;
    this.wordWrap = "off";
    this.lineNumbers = "on";
    this.minimap = false;
    this.renderWhitespace = "selection";
    this.mounted = false;
  }

  protected createRenderRoot(): HTMLElement | DocumentFragment {
    return this;
  }

  async connectedCallback() {
    super.connectedCallback();
    try {
      await ensureMonacoLoaded();
      // Wait one frame so the host has sized the .editor-mount div.
      requestAnimationFrame(() => this.mount());
    } catch (e) {
      console.error("lethean-monaco: failed to load monaco", e);
    }
  }

  disconnectedCallback() {
    super.disconnectedCallback();
    this.resizeObs?.disconnect();
    this.editor?.dispose();
    this.editor = undefined;
    for (const m of this.models.values()) m.dispose();
    this.models.clear();
  }

  /** Reveal a specific line (used by search → open-at-line).
   *  Centres the line, places the cursor at column 1, focuses the
   *  editor. No-op when the editor isn't mounted. */
  revealLine(line: number, column = 1) {
    if (!this.editor || line < 1) return;
    this.editor.revealLineInCenter(line);
    this.editor.setPosition({ lineNumber: line, column });
    this.editor.focus();
  }

  /** Dispose the model for a given path. Called by the host when a
   *  tab is closed so the model's memory is reclaimed. */
  closeModel(path: string) {
    const m = this.models.get(path);
    if (!m) return;
    if (this.editor?.getModel() === m) {
      this.editor.setModel(null);
    }
    m.dispose();
    this.models.delete(path);
    if (this.activePath === path) this.activePath = undefined;
  }

  private mount() {
    if (this.editor || !this.isConnected || !window.monaco) return;
    const host = this.querySelector(".editor-mount") as HTMLElement | null;
    if (!host) return;

    const m = window.monaco;
    this.editor = m.editor.create(host, {
      // value / language are managed via models below — leave the
      // editor model-less here, then call setModel() in
      // switchToPath().
      theme: this.theme,
      readOnly: this.readonly,
      automaticLayout: false,
      minimap: { enabled: this.minimap },
      fontFamily:
        'ui-monospace, SFMono-Regular, Menlo, Consolas, "Liberation Mono", monospace',
      fontSize: this.fontSize,
      lineNumbers: this.lineNumbers,
      renderWhitespace: this.renderWhitespace,
      scrollBeyondLastLine: false,
      smoothScrolling: true,
      tabSize: this.tabSize,
      wordWrap: this.wordWrap,
    });

    this.editor.onDidChangeModelContent(() => {
      if (this.suppressNextChangeEvent) {
        this.suppressNextChangeEvent = false;
        return;
      }
      const v = this.editor?.getValue() ?? "";
      this.value = v;
      this.dispatchEvent(new CustomEvent("lethean-editor-change", {
        detail: { path: this.activePath ?? this.path, value: v },
        bubbles: true, composed: true,
      }));
    });

    this.editor.addCommand(m.KeyMod.CtrlCmd | m.KeyCode.KeyS, () => {
      this.dispatchEvent(new CustomEvent("lethean-editor-save", {
        detail: {
          path: this.activePath ?? this.path,
          value: this.editor?.getValue() ?? "",
        },
        bubbles: true, composed: true,
      }));
    });

    this.resizeObs = new ResizeObserver(() => this.editor?.layout());
    this.resizeObs.observe(host);

    this.mounted = true;
    if (this.path) this.switchToPath(this.path);
  }

  private switchToPath(path: string) {
    if (!this.editor || !window.monaco) return;
    let model = this.models.get(path);
    if (!model) {
      // URI must have an authority component (RFC 3986). "model"
      // works — avoids monaco's UriError on triple-slashes.
      model = window.monaco.editor.createModel(
        this.value,
        this.language,
        window.monaco.Uri.parse("inmemory://model/" + encodeURIComponent(path)),
      );
      this.models.set(path, model);
    }
    this.editor.setModel(model);
    this.activePath = path;
  }

  private editorOptions() {
    return {
      fontSize: this.fontSize,
      tabSize: this.tabSize,
      wordWrap: this.wordWrap,
      lineNumbers: this.lineNumbers,
      minimap: { enabled: this.minimap },
      renderWhitespace: this.renderWhitespace,
      readOnly: this.readonly,
      theme: this.theme,
    };
  }

  private applyEditorOptionChanges(changed: PropertyValues) {
    if (!editorOptionKeys.some(k => changed.has(k))) return;
    this.editor?.updateOptions(this.editorOptions());
    this.applyModelTabSize(changed);
  }

  private applyModelTabSize(changed: PropertyValues) {
    if (!changed.has("tabSize")) return;
    this.editor?.getModel()?.updateOptions({ tabSize: this.tabSize });
  }

  private applyPathChange(changed: PropertyValues) {
    if (changed.has("path") && this.path) {
      this.switchToPath(this.path);
    }
  }

  private syncValueChange(changed: PropertyValues) {
    if (!changed.has("value") || changed.has("path")) return;
    const model = this.editor?.getModel();
    if (!model || model.getValue() === this.value) return;
    this.suppressNextChangeEvent = true;
    model.setValue(this.value);
  }

  private applyLanguageChange(changed: PropertyValues) {
    if (!changed.has("language")) return;
    const model = this.editor?.getModel();
    if (model) window.monaco?.editor.setModelLanguage(model, this.language);
  }

  private applyThemeChange(changed: PropertyValues) {
    if (changed.has("theme")) {
      window.monaco?.editor.setTheme(this.theme);
    }
  }

  updated(changed: PropertyValues) {
    if (!this.editor || !window.monaco) return;
    this.applyEditorOptionChanges(changed);
    this.applyPathChange(changed);
    this.syncValueChange(changed);
    this.applyLanguageChange(changed);
    this.applyThemeChange(changed);
  }

  render() {
    return html`
      <div class="editor-mount" style="width:100%; height:100%; min-height:0;"></div>
      ${this.mounted ? "" : html`
        <div style="padding:14px; font-size:12px; color:var(--fg-3);
                    font-family:var(--font-mono);">
          loading editor…
        </div>
      `}
    `;
  }
}
customElements.define("lethean-monaco", LetheanMonaco);

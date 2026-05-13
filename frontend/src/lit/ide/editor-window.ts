// SPDX-Licence-Identifier: EUPL-1.2
// IDE · editor — <lthn-editor-window>
//
// Wraps <lethean-monaco> in the canonical lthn chrome. Spawned via
// windowservice.Open("editor") — used to view config files, log
// captures, generated SDK output, anything that benefits from
// syntax highlighting + find / multi-cursor / line numbers
// without cramming Monaco into a corner of the main app shell.
//
// Properties bound through to lethean-monaco:
//   value · language · path · readonly
//
// Defaults render a TypeScript scratch buffer so opening the
// surface raw shows something useful instead of an empty viewport.

import { LitElement, html } from "lit";
import { renderChrome } from "../chrome";
import { T } from "@lthn/i18n/coreservice";
import "./lethean-monaco";

class LthnEditorWindow extends LitElement {
  static properties = {
    w:        { type: Number },
    h:        { type: Number },
    embedded: { type: Boolean, reflect: true },
    value:    { type: String },
    language: { type: String },
    path:     { type: String },
    readonly: { type: Boolean },
    chrome:   { state: true },
    dirty:    { state: true },
  };
  declare w: number;
  declare h: number;
  declare embedded: boolean;
  declare value: string;
  declare language: string;
  declare path: string;
  declare readonly: boolean;
  declare chrome: { title: string; subtitle: string };
  declare dirty: boolean;

  constructor() {
    super();
    this.w = 1040; this.h = 700; this.embedded = false;
    this.value = "// scratch.ts — saved nowhere, lives in memory.\n// ⌘S emits lethean-editor-save; subscribe via the editor element.\n\n";
    this.language = "typescript";
    this.path = "scratch.ts";
    this.readonly = false;
    this.chrome = { title: "Editor", subtitle: "scratch · typescript" };
    this.dirty = false;
  }
  createRenderRoot() { return this; }
  async connectedCallback() {
    super.connectedCallback();
    const [title, subtitle] = await Promise.all([
      T("window.editor.title"),
      T("window.editor.subtitle"),
    ]);
    this.chrome = { title, subtitle };
  }

  private _onChange(e: Event) {
    const detail = (e as CustomEvent<{ path: string; value: string }>).detail;
    this.value = detail.value;
    this.dirty = true;
  }

  private _onSave(e: Event) {
    const detail = (e as CustomEvent<{ path: string; value: string }>).detail;
    // No persistence target today — the editor window is a viewer +
    // scratch surface. ⌘S clears the dirty flag so the user sees
    // immediate feedback; future versions wire to fs / git / etc.
    this.value = detail.value;
    this.dirty = false;
  }

  render() {
    const toolbar = html`
      <span style="font-family:var(--font-mono); font-size:10.5px; color:var(--fg-3);
                   padding:0 6px; letter-spacing:0.02em;">
        ${this.path}${this.dirty ? " ·" : ""}
      </span>
      <div style="flex:1"></div>
      <span style="font-family:var(--font-mono); font-size:10.5px; color:var(--fg-3);
                   padding:0 6px;">${this.language}</span>
    `;

    const body = html`
      <lethean-monaco
        style="flex:1; min-height:0; min-width:0;"
        .value=${this.value}
        .language=${this.language}
        .path=${this.path}
        ?readonly=${this.readonly}
        @lethean-editor-change=${(e: Event) => this._onChange(e)}
        @lethean-editor-save=${(e: Event) => this._onSave(e)}
      ></lethean-monaco>
    `;

    return renderChrome({
      title: this.chrome.title, subtitle: this.chrome.subtitle,
      w: this.w, h: this.h, toolbar, body,
      footer: html`monaco · ${this.language} · ⌘S to save`,
      embedded: this.embedded,
    });
  }
}
customElements.define("lthn-editor-window", LthnEditorWindow);

// SPDX-Licence-Identifier: EUPL-1.2
// IDE · build — <lthn-build-window>
//
// Project-type detection + build runner + live output. Open via
// windowservice.Open("build") with ?path=… for the workspace root;
// the panel auto-detects what kind of project it is (Go, Wails,
// Docker, npm, …) and surfaces the canonical build command.
//
// Live output polls ProcessOutput every 800 ms while the build is
// running; the runner.Status field flips to Stopped/Failed when
// the underlying process exits and we drop the poll.

import { LitElement, html, nothing } from "lit";
import { renderChrome } from "../chrome";
import { T } from "@lthn/i18n/coreservice";

interface Detection {
  path: string;
  project_type: string;
  command: string;
  args: string[];
  has_core_bin: boolean;
}

interface RunResult {
  process_id: string;
  build_command: string;
  build_args: string[];
  project_type: string;
}

interface ProcessEntry {
  id: string;
  command: string;
  status: string;
  exit_code: number;
}

class LthnBuildWindow extends LitElement {
  static properties = {
    w:        { type: Number },
    h:        { type: Number },
    embedded: { type: Boolean, reflect: true },
    path:     { type: String },
    chrome:   { state: true },
    detected: { state: true },
    procID:   { state: true },
    output:   { state: true },
    running:  { state: true },
    err:      { state: true },
    procs:    { state: true },
  };
  declare w: number;
  declare h: number;
  declare embedded: boolean;
  declare path: string;
  declare chrome: { title: string; subtitle: string };
  declare detected: Detection | null;
  declare procID: string;
  declare output: string;
  declare running: boolean;
  declare err: string;
  declare procs: ProcessEntry[];

  private _pollTimer: number | null = null;

  constructor() {
    super();
    this.w = 1080; this.h = 700; this.embedded = false;
    this.path = "";
    this.chrome = { title: "Build", subtitle: "detect · run · stream" };
    this.detected = null;
    this.procID = "";
    this.output = "";
    this.running = false;
    this.err = "";
    this.procs = [];
  }
  createRenderRoot() { return this; }

  async connectedCallback() {
    super.connectedCallback();
    const [title, subtitle] = await Promise.all([
      T("window.build.title"),
      T("window.build.subtitle"),
    ]);
    this.chrome = { title, subtitle };
    await this._detect();
    await this._reloadProcs();
  }

  disconnectedCallback() {
    super.disconnectedCallback();
    if (this._pollTimer !== null) {
      clearInterval(this._pollTimer);
      this._pollTimer = null;
    }
  }

  async _detect() {
    if (!this.path) return;
    this.err = "";
    try {
      const svc = await import("@desktop/build/service");
      this.detected = await svc.Detect(this.path) as Detection;
    } catch (e: unknown) {
      this.err = e instanceof Error ? e.message : String(e);
      this.detected = null;
    }
  }

  async _reloadProcs() {
    try {
      const svc = await import("@desktop/build/service");
      const list = await svc.ProcessList();
      this.procs = (list || []) as ProcessEntry[];
    } catch { /* non-fatal */ }
  }

  async _run() {
    if (!this.path || this.running) return;
    this.err = "";
    this.output = "";
    try {
      const svc = await import("@desktop/build/service");
      const r = await svc.Run(this.path, "", []) as RunResult;
      this.procID = r.process_id;
      this.running = true;
      this.output = `$ ${r.build_command} ${r.build_args.join(" ")}\n\n`;
      // Poll output until the process exits; ProcessList tells us
      // the run completed when the matching entry's status flips
      // away from "running".
      this._pollTimer = window.setInterval(() => void this._pollOutput(), 800);
    } catch (e: unknown) {
      this.err = e instanceof Error ? e.message : String(e);
      this.running = false;
    }
  }

  async _pollOutput() {
    if (!this.procID) return;
    try {
      const svc = await import("@desktop/build/service");
      this.output = await svc.ProcessOutput(this.procID);
      const list = await svc.ProcessList() as ProcessEntry[];
      this.procs = list || [];
      const me = this.procs.find(p => p.id === this.procID);
      if (me && me.status !== "running" && me.status !== "starting") {
        this.running = false;
        if (this._pollTimer !== null) {
          clearInterval(this._pollTimer);
          this._pollTimer = null;
        }
      }
    } catch (e: unknown) {
      this.err = e instanceof Error ? e.message : String(e);
    }
  }

  async _stop() {
    if (!this.procID) return;
    try {
      const svc = await import("@desktop/build/service");
      await svc.ProcessKill(this.procID);
      this.running = false;
      if (this._pollTimer !== null) {
        clearInterval(this._pollTimer);
        this._pollTimer = null;
      }
    } catch (e: unknown) {
      this.err = e instanceof Error ? e.message : String(e);
    }
  }

  render() {
    const d = this.detected;

    const toolbar = html`
      <span style="font-family:var(--font-mono); font-size:11px; color:var(--fg-1);
                   padding:4px 9px; background:rgba(255,255,255,0.04);
                   border:1px solid rgba(255,255,255,0.07); border-radius:6px;">
        ${d ? d.project_type : "—"}
      </span>
      <div style="flex:1"></div>
      ${this.running ? html`
        <lthn-btn tone="danger" size="sm" @click=${() => void this._stop()}>
          <i class="fa-solid fa-stop" style="font-size:10px;"></i> Stop
        </lthn-btn>
      ` : html`
        <lthn-btn tone="primary" size="sm" ?dim=${!d?.command}
          @click=${() => void this._run()}>
          <i class="fa-solid fa-play" style="font-size:10px;"></i> Run build
        </lthn-btn>
      `}
    `;

    const body = html`
      <div style="flex:1; display:grid; grid-template-columns:280px 1fr; min-height:0;">
        <aside style="background:rgba(0,0,0,0.18); border-right:1px solid rgba(255,255,255,0.05);
                      padding:14px; overflow:auto; display:flex; flex-direction:column; gap:12px;">
          <div>
            <lthn-label>Detected</lthn-label>
            ${d ? html`
              <div style="display:flex; flex-direction:column; gap:6px; margin-top:6px;
                          font-size:11.5px; color:var(--fg-1);">
                <lthn-rail-row k="Type"      v=${d.project_type}></lthn-rail-row>
                <lthn-rail-row k="Command"   v=${d.command || "—"}></lthn-rail-row>
                <lthn-rail-row k="Args"      v=${d.args.join(" ") || "—"}></lthn-rail-row>
                <lthn-rail-row k="core bin"  v=${d.has_core_bin ? "yes" : "no"}></lthn-rail-row>
              </div>
            ` : html`
              <div style="font-size:11px; color:var(--fg-3); margin-top:6px; line-height:1.55;">
                ${this.err || "pass ?path=/abs/path to detect"}
              </div>
            `}
          </div>

          <div>
            <lthn-label>Processes</lthn-label>
            <div style="display:flex; flex-direction:column; gap:2px; margin-top:6px;">
              ${this.procs.length === 0 ? html`
                <div style="font-size:11px; color:var(--fg-3); font-style:italic;">none</div>
              ` : this.procs.map(p => html`
                <div style="display:flex; align-items:center; gap:6px;
                            font-family:var(--font-mono); font-size:10.5px; padding:3px 0;">
                  <lthn-status-dot variant=${p.status === "running" ? "ok" : p.exit_code === 0 ? "idle" : "err"}></lthn-status-dot>
                  <span style="color:var(--fg-1); flex:1; white-space:nowrap; overflow:hidden; text-overflow:ellipsis;">${p.command}</span>
                  <span style="color:var(--fg-3);">${p.status}</span>
                </div>
              `)}
            </div>
          </div>
        </aside>

        <main style="display:flex; flex-direction:column; min-height:0;">
          <div style="padding:10px 14px; border-bottom:1px solid rgba(255,255,255,0.05);
                      font-family:var(--font-mono); font-size:11px; color:var(--fg-3);">
            ${this.procID ? `process ${this.procID}` : "output"}
            ${this.running ? html`<span style="color:var(--brand-300); margin-left:6px;">· running</span>` : nothing}
          </div>
          <pre style="flex:1; margin:0; padding:12px 14px; overflow:auto;
                      font-family:var(--font-mono); font-size:11px; line-height:1.55;
                      color:var(--fg-1); white-space:pre-wrap;
                      background:rgba(0,0,0,0.20);">${this.output || (this.running ? "starting…" : "no run yet")}</pre>
        </main>
      </div>
    `;

    return renderChrome({
      title: this.chrome.title, subtitle: this.chrome.subtitle,
      w: this.w, h: this.h, toolbar, body,
      footer: html`${this.path || "no workspace"} · ${d ? d.project_type : "—"}${this.running ? " · running" : ""}`,
      embedded: this.embedded,
    });
  }
}
customElements.define("lthn-build-window", LthnBuildWindow);

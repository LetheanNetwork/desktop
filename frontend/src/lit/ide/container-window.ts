// SPDX-Licence-Identifier: EUPL-1.2
// IDE · containers — <lthn-container-window>
//
// Three sections inside the chrome body:
//   1. Runtime cards — Docker/Podman/LinuxKit/QEMU/Apple Container,
//      greyed when unavailable, with capability pills.
//   2. Running list — flat table across all queried runtimes.
//   3. Logs pane — populated when a row is clicked; tails the last
//      200 lines via runtime-native `logs`.
//
// Open via windowservice.Open("containers"). No ?path= needed —
// container probes are host-global.

import { LitElement, html, nothing } from "lit";
import { renderChrome } from "../chrome";
import { T } from "@ui/i18n";

interface Runtime {
  name: string;
  available: boolean;
  version?: string;
  path?: string;
  description: string;
  has_gpu: boolean;
  has_network_isolation: boolean;
  has_volume_mounts: boolean;
  has_encryption: boolean;
  hardware_isolated: boolean;
  sub_second_start: boolean;
}

interface Container {
  id: string;
  name?: string;
  image: string;
  status: string;
  runtime: string;
  created?: string;
}

class LthnContainerWindow extends LitElement {
  static readonly properties = {
    w:         { type: Number },
    h:         { type: Number },
    embedded:  { type: Boolean, reflect: true },
    chrome:    { state: true },
    runtimes:  { state: true },
    list:      { state: true },
    selected:  { state: true },
    logs:      { state: true },
    loading:   { state: true },
    err:       { state: true },
  };
  declare w: number;
  declare h: number;
  declare embedded: boolean;
  declare chrome: { title: string; subtitle: string };
  declare runtimes: Runtime[];
  declare list: Container[];
  declare selected: string;
  declare logs: string;
  declare loading: boolean;
  declare err: string;

  constructor() {
    super();
    this.w = 1180; this.h = 760; this.embedded = false;
    this.chrome = { title: "Containers", subtitle: "runtime · image · status · logs" };
    this.runtimes = [];
    this.list = [];
    this.selected = "";
    this.logs = "";
    this.loading = false;
    this.err = "";
  }
  createRenderRoot() { return this; }

  async connectedCallback() {
    super.connectedCallback();
    const [title, subtitle] = await Promise.all([
      T("window.containers.title"),
      T("window.containers.subtitle"),
    ]);
    this.chrome = { title, subtitle };
    await this._refresh();
  }

  async _refresh() {
    if (this.loading) return;
    this.loading = true;
    this.err = "";
    try {
      const svc = await import("@desktop/container/service");
      const { unwrap } = await import("../result");
      const [det, lst] = await Promise.all([
        unwrap<{ runtimes: Runtime[]; available: Runtime[] }>(
          svc.Detect(false),
          { runtimes: [], available: [] },
        ),
        unwrap<{ containers: Container[]; count: number }>(
          svc.List(""),
          { containers: [], count: 0 },
        ),
      ]);
      this.runtimes = det.runtimes || [];
      this.list = lst.containers || [];
    } catch (e: unknown) {
      this.err = e instanceof Error ? e.message : String(e);
    } finally {
      this.loading = false;
    }
  }

  async _loadLogs(id: string, runtime: string) {
    this.selected = id;
    this.logs = "Loading…";
    try {
      const svc = await import("@desktop/container/service");
      const { unwrap } = await import("../result");
      const r = await unwrap<{ logs: string }>(
        svc.Logs(id, runtime, 200),
        { logs: "" },
      );
      this.logs = r.logs || "(no output)";
    } catch (e: unknown) {
      this.logs = "Error: " + (e instanceof Error ? e.message : String(e));
    }
  }

  _capPills(r: Runtime) {
    const pills: { icon: string; label: string }[] = [];
    if (r.hardware_isolated)    pills.push({ icon: "⚙", label: "hardware" });
    if (r.has_network_isolation) pills.push({ icon: "⌗", label: "network" });
    if (r.has_volume_mounts)    pills.push({ icon: "▢", label: "mounts" });
    if (r.has_gpu)              pills.push({ icon: "⚡", label: "gpu" });
    if (r.has_encryption)       pills.push({ icon: "🔒", label: "encrypted" });
    if (r.sub_second_start)     pills.push({ icon: "⚡", label: "sub-second" });
    return pills;
  }

  render() {
    const runningCount = this.list.length;
    const availableCount = this.runtimes.filter(r => r.available).length;

    const toolbar = html`
      <span style="font-family:var(--font-mono); font-size:11px; color:var(--fg-3);
                   padding:0 6px;">
        ${runningCount} running · ${availableCount}/${this.runtimes.length} runtimes
      </span>
      <div style="flex:1"></div>
      <lthn-btn tone="primary" size="sm" ?dim=${this.loading}
        @click=${() => void this._refresh()}>
        <i class="fa-solid ${this.loading ? "fa-spinner" : "fa-rotate"}"
           style="font-size:10px;"></i>
        ${this.loading ? "Scanning…" : "Re-scan"}
      </lthn-btn>
    `;

    const body = html`
      <div style="flex:1; min-height:0; overflow:auto; display:flex;
                  flex-direction:column; gap:20px; padding:16px 18px;">
        ${this.err ? html`
          <div style="padding:10px 14px; color:var(--err-400);
                      background:rgba(255,76,76,0.06);
                      border:1px solid rgba(255,76,76,0.18);
                      border-radius:6px; font-size:12px; line-height:1.55;">
            ${this.err}
          </div>
        ` : nothing}

        <section>
          <h3 style="font-size:11px; text-transform:uppercase;
                     letter-spacing:0.06em; color:var(--fg-2);
                     margin:0 0 10px;">Runtimes</h3>
          <div style="display:grid;
                      grid-template-columns:repeat(auto-fill, minmax(260px, 1fr));
                      gap:10px;">
            ${this.runtimes.map(r => html`
              <div style="background:rgba(0,0,0,0.18); border-radius:8px;
                          padding:12px; display:flex; flex-direction:column;
                          gap:6px; border:1px solid rgba(255,255,255,0.05);
                          opacity:${r.available ? 1 : 0.45};">
                <div style="display:flex; justify-content:space-between;
                            align-items:center;">
                  <span style="font-weight:600; font-size:13px;
                               color:var(--fg-0); text-transform:capitalize;">
                    ${r.name}</span>
                  <span style="font-size:10px; padding:2px 8px;
                               border-radius:999px; text-transform:uppercase;
                               letter-spacing:0.06em;
                               background:${r.available
                                 ? "rgba(52, 211, 153, 0.14)"
                                 : "rgba(255,255,255,0.05)"};
                               color:${r.available ? "#34d399" : "var(--fg-3)"};">
                    ${r.available ? "available" : "missing"}
                  </span>
                </div>
                <p style="font-size:11px; color:var(--fg-3); margin:0;
                          line-height:1.4;">${r.description}</p>
                ${r.version ? html`
                  <code style="font-family:var(--font-mono); font-size:10px;
                               color:var(--fg-3); background:rgba(0,0,0,0.30);
                               padding:2px 6px; border-radius:3px;
                               align-self:flex-start;">${r.version}</code>
                ` : nothing}
                <div style="display:flex; flex-wrap:wrap; gap:4px;
                            margin-top:6px;">
                  ${this._capPills(r).map(p => html`
                    <span style="font-size:10px; color:var(--fg-2);
                                 background:rgba(0,0,0,0.30);
                                 padding:1px 7px; border-radius:4px;">
                      ${p.icon} ${p.label}
                    </span>
                  `)}
                </div>
              </div>
            `)}
          </div>
        </section>

        <section>
          <h3 style="font-size:11px; text-transform:uppercase;
                     letter-spacing:0.06em; color:var(--fg-2);
                     margin:0 0 10px;">Running</h3>
          ${this.list.length === 0 ? html`
            <div style="padding:18px; text-align:center; color:var(--fg-3);
                        font-style:italic; font-size:12px;
                        background:rgba(0,0,0,0.18); border-radius:6px;
                        border:1px solid rgba(255,255,255,0.05);">
              no running containers
            </div>
          ` : html`
            <div style="display:flex; flex-direction:column; gap:4px;
                        background:rgba(0,0,0,0.18); border-radius:6px;
                        padding:4px; border:1px solid rgba(255,255,255,0.05);
                        font-family:var(--font-mono); font-size:11px;">
              ${this.list.map(c => html`
                <div style="display:grid;
                            grid-template-columns:100px 200px 1fr 140px 70px;
                            gap:10px; padding:8px 12px; align-items:center;
                            cursor:pointer; border-radius:4px;
                            background:${this.selected === c.id
                              ? "rgba(122, 95, 196, 0.18)"
                              : "transparent"};"
                     @click=${() => void this._loadLogs(c.id, c.runtime)}>
                  <span style="color:var(--fg-3);">${c.id}</span>
                  <span style="color:var(--fg-0);">${c.name || "—"}</span>
                  <span style="color:var(--fg-2); overflow:hidden;
                               text-overflow:ellipsis; white-space:nowrap;">
                    ${c.image}</span>
                  <span style="color:var(--fg-2);">${c.status}</span>
                  <span style="font-size:10px; text-transform:uppercase;
                               color:var(--brand-300);">${c.runtime}</span>
                </div>
              `)}
            </div>
          `}
        </section>

        ${this.selected && this.logs ? html`
          <section>
            <h3 style="font-size:11px; text-transform:uppercase;
                       letter-spacing:0.06em; color:var(--fg-2);
                       margin:0 0 10px;">Logs · ${this.selected}</h3>
            <pre style="background:rgba(0,0,0,0.30);
                        border:1px solid rgba(255,255,255,0.05);
                        border-radius:6px; padding:12px 14px;
                        font-family:var(--font-mono); font-size:11px;
                        line-height:1.5; color:var(--fg-2);
                        white-space:pre-wrap; max-height:300px;
                        overflow-y:auto; margin:0;">${this.logs}</pre>
          </section>
        ` : nothing}
      </div>
    `;

    return renderChrome({
      title: this.chrome.title, subtitle: this.chrome.subtitle,
      w: this.w, h: this.h, toolbar, body,
      footer: html`${availableCount} runtime${availableCount === 1 ? "" : "s"} ·
        ${runningCount} container${runningCount === 1 ? "" : "s"}`,
      embedded: this.embedded,
    });
  }
}
customElements.define("lthn-container-window", LthnContainerWindow);

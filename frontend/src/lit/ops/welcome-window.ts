// SPDX-Licence-Identifier: EUPL-1.2
// E1.1 · welcome / first-run — <lthn-welcome-window>
// Light-DOM Lit element. Composes renderChrome() from ../chrome.js.

import { LitElement, html, nothing } from "lit";
import { renderChrome } from "../chrome";
import { T } from "@lthn/i18n/coreservice";

/** Shape returned by integrations.List() — mirrored here so the
 *  wizard doesn't pull a hard dependency on the bindings module
 *  graph at type-check time. Kept aligned with go/pkg/integrations. */
interface ClientStatus {
  id:          string;
  name:        string;
  description: string;
  config_path: string;
  exists:      boolean;
  state:       string;
}

/* Step 3 "Finish" handler — marks onboarding complete, opens the
 * settings window so the user can change their mind, and closes
 * the wizard. Dynamic import so a Lit unit test or canvas preview
 * doesn't pull the Wails runtime at module load. */
async function completeOnboarding(): Promise<void> {
  try {
    const [config, windowSvc] = await Promise.all([
      import("@lthn/config/service"),
      import("@desktop/desktop/windowservice"),
    ]);
    await config.Set("welcome.completed", "true");
    await windowSvc.Open("settings");
    await windowSvc.Hide("welcome");
  } catch (err) {
    console.error("welcome: completeOnboarding failed", err);
  }
}

/* Advance helper — used by Back / Skip / forward buttons. Mutates the
 * step property on the host element via a property setter. */
function advance(host: LthnWelcomeWindow, delta: number): void {
  const next = host.step + delta;
  if (next < 1) return;
  if (next > 3) {
    void completeOnboarding();
    return;
  }
  host.step = next;
}

class LthnWelcomeWindow extends LitElement {
  static properties = {
    step: { type: Number, reflect: true },
    w:    { type: Number },
    h:    { type: Number },
    embedded: { type: Boolean, reflect: true },
    chrome: { state: true },
    modelsDir: { state: true },
    fresh:     { state: true },
    clients:   { state: true },
    endpoint:  { state: true },
    version:   { state: true },
    stepLabels:{ state: true },
  };
  declare step: number;
  declare w: number;
  declare h: number;
  declare embedded: boolean;
  declare chrome: { title: string; subtitleFmt: string };
  declare modelsDir: string;
  declare fresh: boolean;
  declare clients: ClientStatus[];
  declare endpoint: string;
  declare version: string;
  declare stepLabels: { l: string; h: string }[];
  constructor() {
    super();
    this.step = 1; this.w = 760; this.h = 580; this.embedded = false;
    this.chrome = { title: "Welcome to lthn", subtitleFmt: "step %s of 3" };
    this.modelsDir = "~/Lethean/conf/models/";
    this.fresh = true;
    this.clients = [];
    this.endpoint = "http://localhost:8000/v1";
    this.version = "0.2.0-rc1";
    this.stepLabels = [
      { l: "Model directory", h: "Where models live" },
      { l: "First model",     h: "Pick a starter" },
      { l: "Connect",         h: "Wire up clients" },
    ];
  }
  createRenderRoot() { return this; }
  async connectedCallback() {
    super.connectedCallback();
    const [title, subtitleFmt, s1l, s1h, s2l, s2h, s3l, s3h] = await Promise.all([
      T("window.welcome.title"),
      T("window.welcome.subtitle"),
      T("window.welcome.step1_label"), T("window.welcome.step1_hint"),
      T("window.welcome.step2_label"), T("window.welcome.step2_hint"),
      T("window.welcome.step3_label"), T("window.welcome.step3_hint"),
    ]);
    this.chrome = { title, subtitleFmt };
    this.stepLabels = [
      { l: s1l, h: s1h },
      { l: s2l, h: s2h },
      { l: s3l, h: s3h },
    ];
    // Pull the canonical Lethean paths + the fresh-install flag
    // from the firstlaunch service. The wizard's "Where shall we
    // keep your models?" step shows the real directory the runner
    // will scan; the fresh flag lets a future cmd/lthn handler skip
    // welcome on subsequent launches.
    try {
      const fl = await import("@desktop/firstlaunch/wailsservice");
      const [paths, state, build] = await Promise.all([
        fl.Paths(),
        fl.Detect(),
        fl.Build().catch(() => null),
      ]);
      if (paths?.models_dir) {
        this.modelsDir = displayHome(paths.models_dir);
      }
      this.fresh = !!state?.fresh;
      if (build?.version) this.version = build.version;
    } catch (err) {
      // Non-fatal — keep the default ~/Lethean placeholder.
      console.error("welcome: firstlaunch lookup failed", err);
    }
    // Step 3 "Connect" — the integrations service already inspects
    // each client's config file on disk; the wizard reuses the same
    // surface so the catalogue stays single-sourced. The endpoint
    // string in the same step reads from server.WAddr() so the
    // wizard, Settings → API and Integrations all agree.
    try {
      const [int, server] = await Promise.all([
        import("@desktop/integrations/wailsservice"),
        import("@desktop/server/service"),
      ]);
      const [list, addr] = await Promise.all([
        int.List(),
        server.WAddr().catch((): string => ""),
      ]);
      this.clients = (list || []) as ClientStatus[];
      if (addr) {
        const a = addr.startsWith(":") ? `localhost${addr}` : addr;
        this.endpoint = `http://${a}/v1`;
      }
    } catch (err) {
      console.error("welcome: integrations lookup failed", err);
      this.clients = [];
    }
  }

  render() {
    const steps = [
      { n: 1, label: this.stepLabels[0].l, hint: this.stepLabels[0].h },
      { n: 2, label: this.stepLabels[1].l, hint: this.stepLabels[1].h },
      { n: 3, label: this.stepLabels[2].l, hint: this.stepLabels[2].h },
    ];

    const body = html`
      <div style="flex:1; display:grid; grid-template-columns: 240px 1fr; min-height:0;">
        <!-- steps rail -->
        <aside style="background:rgba(0,0,0,0.18); border-right:1px solid rgba(255,255,255,0.05);
                      padding:26px 22px; display:flex; flex-direction:column; gap:18px;">
          <div style="display:flex; align-items:center; gap:10px;">
            <lthn-glyph size="24" color="var(--fg-0)" active></lthn-glyph>
            <div>
              <div style="font-size:13px; font-weight:600; color:var(--fg-0);">lthn</div>
              <div style="font-family:var(--font-mono); font-size:10px; color:var(--fg-3); letter-spacing:0.04em;">
                sovereign · single-watt
              </div>
            </div>
          </div>
          <div style="height:1px; background:rgba(255,255,255,0.06); margin:4px 0;"></div>
          ${steps.map(s => {
            const done = s.n < this.step;
            const here = s.n === this.step;
            return html`
              <div style="display:flex; gap:12px; align-items:flex-start;">
                <div style="width:22px; height:22px; border-radius:50%;
                            background:${done ? "var(--brand-500)" : "transparent"};
                            border:${here ? "1.5px solid var(--brand-400)" :
                                    done ? "1.5px solid var(--brand-500)" :
                                    "1.5px solid rgba(255,255,255,0.12)"};
                            display:flex; align-items:center; justify-content:center;
                            font-size:11px; font-weight:600;
                            color:${done ? "#fff" : here ? "var(--brand-300)" : "var(--fg-3)"};
                            flex-shrink:0;">
                  ${done
                    ? html`<i class="fa-solid fa-check" style="font-size:9px;"></i>`
                    : s.n}
                </div>
                <div>
                  <div style="font-size:12px; font-weight:500;
                              color:${here ? "var(--fg-0)" : "var(--fg-2)"};">${s.label}</div>
                  <div style="font-size:10.5px; color:var(--fg-3); margin-top:2px;">${s.hint}</div>
                </div>
              </div>
            `;
          })}
          <div style="flex:1"></div>
          <div style="font-size:10.5px; color:var(--fg-3); line-height:1.5;">
            You can change all of this later in Settings. Nothing leaves this Mac.
          </div>
        </aside>

        <!-- step body -->
        <main style="padding:32px 40px; display:flex; flex-direction:column; min-height:0;">
          ${this.step === 1 ? this._step1() : this.step === 2 ? this._step2() : this._step3()}
          <div style="flex:1"></div>
          <div style="display:flex; align-items:center; gap:10px; padding-top:18px;">
            ${this.step > 1
              ? html`<lthn-btn tone="ghost" size="lg" @click=${() => advance(this, -1)}>Back</lthn-btn>`
              : nothing}
            <lthn-btn tone="quiet" size="lg" @click=${completeOnboarding}>Skip for now</lthn-btn>
            <div style="flex:1"></div>
            <lthn-btn tone="primary" size="lg" @click=${() => advance(this, 1)}>
              ${this.step === 3
                ? html`<i class="fa-solid fa-check"></i> Finish`
                : this.step === 1
                ? html`<i class="fa-solid fa-arrow-right"></i> Use this folder`
                : html`<i class="fa-solid fa-arrow-right"></i> Download & continue`}
            </lthn-btn>
          </div>
        </main>
      </div>
    `;

    return renderChrome({
      title: this.chrome.title,
      subtitle: this.chrome.subtitleFmt.replace("%s", String(this.step)),
      w: this.w, h: this.h,
      body,
      footer: html`British English · dark default · accessibility light in Settings · v${this.version}`,
      embedded: this.embedded,
    });
  }

  _step1() {
    return html`
      <div style="display:flex; flex-direction:column; gap:18px;">
        <div>
          <div style="font-size:24px; font-weight:600; color:var(--fg-0); letter-spacing:-0.018em;">
            Where shall we keep your models?
          </div>
          <div style="font-size:13px; color:var(--fg-2); margin-top:8px; line-height:1.55; max-width:440px;">
            A folder on this Mac. Models can be big — pick somewhere with room.
            We default to your home directory; change it if you have a faster volume.
          </div>
        </div>
        <div style="margin-top:4px; padding:20px 22px; border:1.5px dashed rgba(64,193,197,0.30);
                    border-radius:10px; background:rgba(64,193,197,0.04);
                    display:flex; align-items:center; gap:18px;">
          <div style="width:44px; height:44px; border-radius:10px;
                      background:rgba(64,193,197,0.10); border:1px solid rgba(64,193,197,0.20);
                      display:flex; align-items:center; justify-content:center;">
            <i class="fa-solid fa-folder-open" style="font-size:18px; color:var(--brand-300);"></i>
          </div>
          <div style="flex:1;">
            <div style="font-family:var(--font-mono); font-size:13px; color:var(--fg-0); letter-spacing:-0.005em;">
              ${this.modelsDir}
            </div>
            <div style="font-size:11px; color:var(--fg-3); margin-top:2px;">
              Canonical Lethean layout · visible in Finder · safe to inspect
            </div>
          </div>
          <lthn-btn tone="ghost" size="md">Choose folder…</lthn-btn>
        </div>
      </div>
    `;
  }

  _step2() {
    const models = [
      { name:"Gemma 4 E2B (-assistant)", author:"Google",    size:"2.1 GB", ram:"4 GB", desc:"Best balance for first run · Lethean-recommended", rec:true },
      { name:"Llama 3.2 3B Instruct",    author:"Meta",      size:"3.4 GB", ram:"6 GB", desc:"Solid general-purpose · longer context window" },
      { name:"Phi 3.5 Mini Instruct",    author:"Microsoft", size:"2.6 GB", ram:"5 GB", desc:"Punches above its weight on reasoning" },
    ];
    return html`
      <div style="display:flex; flex-direction:column; gap:16px; min-height:0;">
        <div>
          <div style="font-size:24px; font-weight:600; color:var(--fg-0); letter-spacing:-0.018em;">
            Pick a model to start with.
          </div>
          <div style="font-size:13px; color:var(--fg-2); margin-top:8px; line-height:1.55; max-width:460px;">
            Three small models that run comfortably on Apple Silicon.
            You can add more from the model browser anytime.
          </div>
        </div>
        <div style="display:flex; flex-direction:column; gap:8px;">
          ${models.map(m => html`
            <div style="display:flex; align-items:center; gap:14px; padding:14px 16px; border-radius:10px;
                        background:${m.rec ? "rgba(64,193,197,0.06)" : "rgba(255,255,255,0.03)"};
                        border:1px solid ${m.rec ? "rgba(64,193,197,0.22)" : "rgba(255,255,255,0.06)"};">
              <div style="width:18px; height:18px; border-radius:50%;
                          border:1.5px solid ${m.rec ? "var(--brand-400)" : "rgba(255,255,255,0.18)"};
                          display:flex; align-items:center; justify-content:center; flex-shrink:0;">
                ${m.rec ? html`<div style="width:8px; height:8px; border-radius:50%; background:var(--brand-400);"></div>` : nothing}
              </div>
              <div style="flex:1;">
                <div style="display:flex; align-items:baseline; gap:8px;">
                  <span style="font-size:13.5px; font-weight:500; color:var(--fg-0); letter-spacing:-0.005em;">${m.name}</span>
                  <span style="font-size:11px; color:var(--fg-3);">· ${m.author}</span>
                  ${m.rec ? html`<lthn-state-pill variant="latest">Recommended</lthn-state-pill>` : nothing}
                </div>
                <div style="font-size:11.5px; color:var(--fg-2); margin-top:3px;">${m.desc}</div>
              </div>
              <div style="text-align:right; font-family:var(--font-mono); font-size:11px; color:var(--fg-3); letter-spacing:0.02em;">
                <div>${m.size}</div>
                <div style="margin-top:2px;">${m.ram} RAM</div>
              </div>
            </div>
          `)}
        </div>
      </div>
    `;
  }

  _step3() {
    // Map live ClientStatus → step 3's compact row shape. Default-check
    // clients that have a config file we could write to (state ===
    // "available"); already-wired clients ("configured") show wired but
    // unchecked since they don't need re-wiring; "n/a" hides.
    const clients = (this.clients || [])
      .filter(c => c.state !== "n/a")
      .map(c => ({
        name:    c.name,
        desc:    c.description,
        path:    displayHome(c.config_path),
        checked: c.state === "available",
        wired:   c.state === "configured",
      }));
    return html`
      <div style="display:flex; flex-direction:column; gap:16px;">
        <div>
          <div style="font-size:24px; font-weight:600; color:var(--fg-0); letter-spacing:-0.018em;">
            Want to wire it into your tools?
          </div>
          <div style="font-size:13px; color:var(--fg-2); margin-top:8px; line-height:1.55; max-width:460px;">
            lthn speaks the OpenAI-compatible API on
            <span style="font-family:var(--font-mono); color:var(--fg-1);">${this.endpoint}</span>.
            We can drop the endpoint into these configs for you. The only outbound action lthn ever takes without you asking.
          </div>
        </div>
        <div style="display:flex; flex-direction:column; gap:6px;">
          ${clients.length === 0 ? html`
            <div style="padding:14px 16px; border-radius:8px; background:rgba(255,255,255,0.025);
                        border:1px solid rgba(255,255,255,0.05); font-size:12px; color:var(--fg-3); line-height:1.55;">
              No supported clients detected on this Mac. You can always wire one up later from Settings → Integrations.
            </div>
          ` : clients.map(c => html`
            <div style="display:flex; align-items:center; gap:14px; padding:12px 14px; border-radius:8px;
                        background:rgba(255,255,255,0.03); border:1px solid rgba(255,255,255,0.06);">
              <input type="checkbox" ?checked=${c.checked} ?disabled=${c.wired} style="accent-color:var(--brand-400);" />
              <div style="flex:1;">
                <div style="display:flex; align-items:baseline; gap:8px;">
                  <span style="font-size:12.5px; font-weight:500; color:var(--fg-0);">${c.name}</span>
                  ${c.wired ? html`<lthn-state-pill variant="latest">Already wired</lthn-state-pill>` : nothing}
                </div>
                <div style="font-size:11px; color:var(--fg-3); margin-top:1px;
                            font-family:var(--font-mono); letter-spacing:0.01em;">${c.path}</div>
              </div>
              <div style="font-size:11px; color:var(--fg-3);">${c.desc}</div>
            </div>
          `)}
        </div>
      </div>
    `;
  }
}
customElements.define("lthn-welcome-window", LthnWelcomeWindow);

/** Collapse the leading $HOME into "~/" so the wizard shows the
 *  same short form a terminal user is used to. Falls back to the
 *  raw absolute path when $HOME isn't known (web preview, etc.). */
function displayHome(absPath: string): string {
  if (!absPath) return absPath;
  // No browser API for $HOME — match common macOS / Linux user dirs.
  const m = absPath.match(/^\/(Users|home)\/[^/]+\//);
  if (m) {
    return "~/" + absPath.slice(m[0].length);
  }
  return absPath;
}

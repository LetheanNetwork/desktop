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

/** Total number of steps in the welcome wizard. Step 4 is the Menu
 *  Behaviours tour — see plans/project/lthn/desktop/RFC.menu-behaviours.md.
 *  Replay-from-Settings opens the welcome window with start-step=4 via
 *  localStorage["lthn.welcome.start-step"]. */
const MAX_STEP = 4;

/* Final-step handler — marks onboarding complete, marks the menu
 * tour seen, opens the settings window so the user can change their
 * mind, and closes the wizard. Dynamic import so a Lit unit test or
 * canvas preview doesn't pull the Wails runtime at module load. */
async function completeOnboarding(): Promise<void> {
  try {
    const [config, windowSvc] = await Promise.all([
      import("@lthn/config/service"),
      import("@lthn/gui/windowbindingservice"),
    ]);
    await config.Set("welcome.completed", "true");
    // Mark the menu tour as seen — the Settings → Menu Behaviours
    // "Replay menu tour" button clears this flag and re-opens the
    // wizard at step 4 to surface it again.
    try { globalThis.localStorage?.setItem("lthn.menu.tour-completed", "true"); }
    catch { /* localStorage unavailable — replay still works via Open */ }
    // Land in the unified app shell — the "settings" cute popover
    // is the wrong destination for a first-run user who just finished
    // onboarding. They should drop into the desktop they've just
    // configured. Settings remains reachable from the app shell's
    // own navigation.
    await windowSvc.Open("app");
    await windowSvc.Hide("welcome");
  } catch (err) {
    console.error("welcome: completeOnboarding failed", err);
  }
}

/* Advance helper — used by Back / Skip / forward buttons. Mutates the
 * step property on the host element via a property setter. Also
 * persists per-step side effects when advancing forward:
 *   - leaving step 2 forward → write welcome.preferred_model (slug)
 *     so the chat surface can offer the user the download after
 *     onboarding completes.
 *   - leaving step 3 forward → no-op today (client wiring lives in
 *     Integrations; the wizard's checkboxes are preview-only). */
function advance(host: LthnWelcomeWindow, delta: number): void {
  const from = host.step;
  const next = from + delta;
  if (next < 1) return;
  if (delta > 0 && from === 2) {
    void persistStep2Choice(host);
  }
  if (next > MAX_STEP) {
    void completeOnboarding();
    return;
  }
  host.step = next;
}

/* Persist the step-2 model recommendation. Best-effort — a config
 * write failure logs but doesn't block the wizard advance. The chat
 * surface reads welcome.preferred_model on first launch to surface
 * the "want to download $name?" banner. */
async function persistStep2Choice(host: LthnWelcomeWindow): Promise<void> {
  try {
    const slug = host.preferredModelSlug();
    if (!slug) return;
    const config = await import("@lthn/config/service");
    await config.Set("welcome.preferred_model", slug);
  } catch (err) {
    console.warn("welcome: persist preferred model failed", err);
  }
}

class LthnWelcomeWindow extends LitElement {
  static readonly properties = {
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
    selectedModelIdx: { state: true },
    t:         { state: true },
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
  /** Index into the step 2 model list of the currently-selected row.
   *  Default 0 = the recommended model. Reset to 0 when the wizard
   *  resets (replay tour, etc.). Click on any row sets this; the
   *  forward button reads it when the download wire lands. */
  declare selectedModelIdx: number;
  declare t: {
    s1Title: string; s1Body: string; s1LayoutHint: string; s1Choose: string;
    s2Title: string; s2Body: string; s2Recommended: string;
    s3Title: string; s3Body: string; s3Empty: string; s3Wired: string;
    s4Title: string; s4Body: string;
    s4WordLabel: string; s4IconLabel: string;
    s4Modifier: string; s4SettingsHint: string;
    btnBack: string; btnSkip: string; btnUse: string; btnDownload: string;
    btnContinue: string; btnFinish: string;
    footer: string;
  };
  constructor() {
    super();
    // Honour the replay-from-Settings hand-off: if the Settings
    // panel wrote a start-step before calling WindowService.Open,
    // land on that step instead of 1 and clear the key so a later
    // first-launch path doesn't inherit it.
    let initialStep = 1;
    try {
      const saved = globalThis.localStorage?.getItem("lthn.welcome.start-step");
      if (saved) {
        const parsed = Number(saved);
        if (Number.isInteger(parsed) && parsed >= 1 && parsed <= MAX_STEP) initialStep = parsed;
        globalThis.localStorage?.removeItem("lthn.welcome.start-step");
      }
    } catch { /* localStorage unavailable — fall through to step 1 */ }
    this.step = initialStep; this.w = 1200; this.h = 800; this.embedded = false;
    this.chrome = { title: "Welcome to lthn", subtitleFmt: "step %s of 4" };
    this.modelsDir = "~/Lethean/conf/models/";
    this.fresh = true;
    this.selectedModelIdx = 0;
    this.clients = [];
    this.endpoint = "http://localhost:8000/v1";
    this.version = "0.2.0-rc1";
    this.stepLabels = [
      { l: "Model directory", h: "Where models live" },
      { l: "First model",     h: "Pick a starter" },
      { l: "Connect",         h: "Wire up clients" },
      { l: "Menu tour",       h: "How clicks behave" },
    ];
    this.t = {
      s1Title: "Where shall we keep your models?",
      s1Body:  "A folder on this Mac. Models can be big — pick somewhere with room. We default to your home directory; change it if you have a faster volume.",
      s1LayoutHint: "Canonical Lethean layout · visible in Finder · safe to inspect",
      s1Choose: "Choose folder…",
      s2Title: "Pick a model to start with.",
      s2Body:  "Three small models that run comfortably on Apple Silicon. You can add more from the model browser anytime.",
      s2Recommended: "Recommended",
      s3Title: "Want to wire it into your tools?",
      s3Body:  "lthn speaks the OpenAI-compatible API on %s. We can drop the endpoint into these configs for you. The only outbound action lthn ever takes without you asking.",
      s3Empty: "No supported clients detected on this Mac. You can always wire one up later from Settings → Integrations.",
      s3Wired: "Already wired",
      s4Title: "Two clicks, two outcomes.",
      s4Body:  "Click the word to switch view here. Click the icon to pop the surface out into a new window. Or hold ⌘ (Ctrl on PC) and click anywhere on the row — that always pops out.",
      s4WordLabel: "click → switch here",
      s4IconLabel: "click → new window",
      s4Modifier:  "⌘ + click works anywhere on the row",
      s4SettingsHint: "Change defaults later in Settings → General → Menu Behaviours.",
      btnBack:    "Back",
      btnSkip:    "Skip for now",
      btnUse:     "Use this folder",
      btnDownload:"Download & continue",
      btnContinue:"Continue",
      btnFinish:  "Finish",
      footer:     "British English · dark default · accessibility light in Settings · v%s",
    };
  }
  createRenderRoot() { return this; }

  /** firstUpdated runs once after the initial render lands. We use it
   *  to drive the Wails window size from the element's declared w/h —
   *  the Go boot-time spec sets the initial dimensions, this callback
   *  lets the rendered content own the final size. Dynamic import so
   *  vitest / design-preview environments without the Wails runtime
   *  silently fall through. */
  async firstUpdated(): Promise<void> {
    try {
      const ws = await import("@lthn/gui/windowbindingservice");
      await ws.SetSize("welcome", this.w, this.h);
    } catch { /* binding unavailable — design preview / unit test path */ }
  }

  async connectedCallback() {
    super.connectedCallback();
    const [
      title, subtitleFmt, s1l, s1h, s2l, s2h, s3l, s3h, s4l, s4h,
      s1Title, s1Body, s1Lh, s1Choose,
      s2Title, s2Body, s2Rec,
      s3Title, s3Body, s3Empty, s3Wired,
      s4Title, s4Body, s4Word, s4Icon, s4Mod, s4Set,
      bBack, bSkip, bUse, bDownload, bCont, bFinish, foot,
    ] = await Promise.all([
      T("window.welcome.title"),
      T("window.welcome.subtitle"),
      T("window.welcome.step1_label"), T("window.welcome.step1_hint"),
      T("window.welcome.step2_label"), T("window.welcome.step2_hint"),
      T("window.welcome.step3_label"), T("window.welcome.step3_hint"),
      T("window.welcome.step4_label"), T("window.welcome.step4_hint"),
      T("window.welcome.step1_title"), T("window.welcome.step1_body"),
      T("window.welcome.step1_layout_hint"), T("window.welcome.step1_choose"),
      T("window.welcome.step2_title"), T("window.welcome.step2_body"),
      T("window.welcome.step2_recommended"),
      T("window.welcome.step3_title"), T("window.welcome.step3_body"),
      T("window.welcome.step3_empty"), T("window.welcome.step3_wired"),
      T("window.welcome.step4_title"), T("window.welcome.step4_body"),
      T("window.welcome.step4_word_label"), T("window.welcome.step4_icon_label"),
      T("window.welcome.step4_modifier"), T("window.welcome.step4_settings_hint"),
      T("window.welcome.btn_back"), T("window.welcome.btn_skip"),
      T("window.welcome.btn_use"), T("window.welcome.btn_download"),
      T("window.welcome.btn_continue"), T("window.welcome.btn_finish"),
      T("window.welcome.footer"),
    ]);
    this.chrome = { title, subtitleFmt };
    this.stepLabels = [
      { l: s1l, h: s1h },
      { l: s2l, h: s2h },
      { l: s3l, h: s3h },
      { l: s4l, h: s4h },
    ];
    this.t = {
      s1Title, s1Body, s1LayoutHint: s1Lh, s1Choose,
      s2Title, s2Body, s2Recommended: s2Rec,
      s3Title, s3Body, s3Empty, s3Wired,
      s4Title, s4Body, s4WordLabel: s4Word, s4IconLabel: s4Icon,
      s4Modifier: s4Mod, s4SettingsHint: s4Set,
      btnBack: bBack, btnSkip: bSkip, btnUse: bUse,
      btnDownload: bDownload, btnContinue: bCont, btnFinish: bFinish,
      footer: foot,
    };
    // Pull the canonical Lethean paths + the fresh-install flag
    // from the firstlaunch service. The wizard's "Where shall we
    // keep your models?" step shows the real directory the runner
    // will scan; the fresh flag lets a future cmd/lthn handler skip
    // welcome on subsequent launches.
    try {
      const fl = await import("@desktop/firstlaunch/wailsservice");
      const { unwrap } = await import("../result");
      const [paths, state, build] = await Promise.all([
        unwrap<{ models_dir?: string }>(fl.Paths(), {}),
        unwrap<{ fresh?: boolean }>(fl.Detect(), {}),
        unwrap<{ version?: string }>(fl.Build(), {}),
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
      const { unwrap } = await import("../result");
      const [list, addr] = await Promise.all([
        int.List(),
        unwrap<string>(server.WAddr(), ""),
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
      { n: 4, label: this.stepLabels[3].l, hint: this.stepLabels[3].h },
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
          ${this.step === 1 ? this._step1()
            : this.step === 2 ? this._step2()
            : this.step === 3 ? this._step3()
            : this._step4()}
          <div style="flex:1"></div>
          <div style="display:flex; align-items:center; gap:10px; padding-top:18px;">
            ${this.step > 1
              ? html`<lthn-btn tone="ghost" size="lg" @click=${() => advance(this, -1)}>${this.t.btnBack}</lthn-btn>`
              : nothing}
            <lthn-btn tone="quiet" size="lg" @click=${completeOnboarding}>${this.t.btnSkip}</lthn-btn>
            <div style="flex:1"></div>
            <lthn-btn tone="primary" size="lg" @click=${() => advance(this, 1)}>
              ${this.step === MAX_STEP
                ? html`<i class="fa-solid fa-check"></i> ${this.t.btnFinish}`
                : this.step === 1
                ? html`<i class="fa-solid fa-arrow-right"></i> ${this.t.btnUse}`
                : this.step === 2
                ? html`<i class="fa-solid fa-arrow-right"></i> ${this.t.btnDownload}`
                : html`<i class="fa-solid fa-arrow-right"></i> ${this.t.btnContinue}`}
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
      footer: html`${this.t.footer.replace("%s", this.version)}`,
      embedded: this.embedded,
    });
  }

  _step1() {
    return html`
      <div style="display:flex; flex-direction:column; gap:18px;">
        <div>
          <div style="font-size:24px; font-weight:600; color:var(--fg-0); letter-spacing:-0.018em;">
            ${this.t.s1Title}
          </div>
          <div style="font-size:13px; color:var(--fg-2); margin-top:8px; line-height:1.55; max-width:440px;">
            ${this.t.s1Body}
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
              ${this.t.s1LayoutHint}
            </div>
          </div>
          <div style="display:flex; flex-direction:column; gap:6px; align-items:flex-end;">
            <lthn-btn class="lthn-welcome-choose-folder"
                      tone="ghost" size="md"
                      @click=${() => void this._chooseModelsFolder()}>
              ${this.t.s1Choose}
            </lthn-btn>
            <button class="lthn-welcome-reset-folder"
                    @click=${() => void this._resetModelsFolder()}
                    title="Restore the default ~/Lethean/conf/models/ path"
                    style="
                      background:transparent; border:none; cursor:pointer;
                      font:inherit; font-size:10.5px;
                      color:var(--fg-3);
                      padding:0; letter-spacing:0.01em;
                      --wails-draggable: no-drag;
                    "
                    onmouseover="this.style.color='var(--fg-1)';"
                    onmouseout="this.style.color='var(--fg-3)';">
              Reset to default
            </button>
          </div>
        </div>
      </div>
    `;
  }

  /** Reset the models-dir override so paths.ModelsDir falls back to
   *  ~/Lethean/conf/models/. Refreshes the visible path on success.
   *  Public so the test harness can drive without simulating a click. */
  async _resetModelsFolder() {
    try {
      const fl = await import("@desktop/firstlaunch/wailsservice");
      const { demand } = await import("../result");
      await demand<unknown>(fl.ClearModelsDir());
      // Re-fetch the default path so the rendered modelsDir updates.
      const { unwrap } = await import("../result");
      const paths = await unwrap<{ models_dir?: string }>(fl.Paths(), {});
      if (paths?.models_dir) {
        this.modelsDir = displayHome(paths.models_dir);
      }
    } catch (err: unknown) {
      console.warn("welcome: ClearModelsDir failed:", err);
    }
  }

  /** Fire the native folder picker, then persist the result via
   *  FirstLaunch.SetModelsDir. Refreshes the visible modelsDir on
   *  success. Split from the @click so tests can drive the wire
   *  without the native dialog. */
  async _chooseModelsFolder() {
    try {
      const dlg = await import("@wailsio/runtime").then(m => m.Dialogs);
      const picked = await dlg.OpenFile({
        Title: "Choose models folder",
        ButtonText: "Use folder",
        CanChooseFiles: false,
        CanChooseDirectories: true,
      });
      if (!picked || typeof picked !== "string") return;
      await this._adoptModelsFolder(picked);
    } catch (err: unknown) {
      console.warn("welcome: choose folder dialog failed:", err);
    }
  }

  /** Persist the chosen folder + update the visible modelsDir.
   *  Public so tests can drive without firing the native dialog. */
  async _adoptModelsFolder(path: string) {
    if (!path) return;
    try {
      const fl = await import("@desktop/firstlaunch/wailsservice");
      const { demand } = await import("../result");
      const resolved = await demand<string>(fl.SetModelsDir(path));
      this.modelsDir = displayHome(resolved || path);
    } catch (err: unknown) {
      console.warn("welcome: SetModelsDir failed:", err);
      throw err;
    }
  }

  /** Return the HF repo slug for the model currently highlighted in
   *  step 2, or "" when the index is out of bounds. Public so the
   *  module-level advance helper can persist the choice on forward
   *  without reaching into private state. */
  preferredModelSlug(): string {
    // Mirrors the step-2 catalogue order — kept inline rather than
    // hoisted so the names + repos stay in one place.
    const slugs = [
      "google/gemma-4-e2b",
      "meta-llama/Llama-3.2-3B-Instruct",
      "microsoft/Phi-3.5-mini-instruct",
    ];
    const idx = this.selectedModelIdx;
    if (!Number.isInteger(idx) || idx < 0 || idx >= slugs.length) return "";
    return slugs[idx];
  }

  _step2() {
    // Catalogue is hand-curated. Selection persists to
    // welcome.preferred_model on advance — the actual download
    // happens via Lemma admin's Download form post-onboarding
    // (the chat surface surfaces the choice once the model lands).
    // hfRepo lets the post-welcome download path resolve cleanly
    // to the Lemma admin form the same way model-browser does.
    const models = [
      { name:"Gemma 4 E2B (-assistant)", author:"Google",    size:"2.1 GB", ram:"4 GB", desc:"Best balance for first run · Lethean-recommended", rec:true,  hfRepo:"google/gemma-4-e2b" },
      { name:"Llama 3.2 3B Instruct",    author:"Meta",      size:"3.4 GB", ram:"6 GB", desc:"Solid general-purpose · longer context window",        rec:false, hfRepo:"meta-llama/Llama-3.2-3B-Instruct" },
      { name:"Phi 3.5 Mini Instruct",    author:"Microsoft", size:"2.6 GB", ram:"5 GB", desc:"Punches above its weight on reasoning",                rec:false, hfRepo:"microsoft/Phi-3.5-mini-instruct" },
    ];
    return html`
      <div style="display:flex; flex-direction:column; gap:16px; min-height:0;">
        <div>
          <div style="font-size:24px; font-weight:600; color:var(--fg-0); letter-spacing:-0.018em;">
            ${this.t.s2Title}
          </div>
          <div style="font-size:13px; color:var(--fg-2); margin-top:8px; line-height:1.55; max-width:460px;">
            ${this.t.s2Body}
          </div>
        </div>
        <div style="padding:10px 12px; border-radius:6px;
                    background:rgba(64,193,197,0.06);
                    border:1px solid rgba(64,193,197,0.18);
                    font-size:11.5px; color:var(--fg-2); line-height:1.55;">
          <i class="fa-solid fa-circle-info" style="color:var(--brand-300); font-size:11px; margin-right:6px;"></i>
          We save your pick for later — the download happens from the
          Lemma window once you finish onboarding. Nothing pulls in the
          background here.
        </div>
        <div style="display:flex; flex-direction:column; gap:8px;">
          ${models.map((m, idx) => {
            const sel = idx === this.selectedModelIdx;
            return html`
            <div class="lthn-welcome-model-row"
                 data-idx=${idx}
                 ?data-selected=${sel}
                 @click=${() => { this.selectedModelIdx = idx; }}
                 style="display:flex; align-items:center; gap:14px; padding:14px 16px; border-radius:10px;
                        background:${sel ? "rgba(64,193,197,0.06)" : "rgba(255,255,255,0.03)"};
                        border:1px solid ${sel ? "rgba(64,193,197,0.22)" : "rgba(255,255,255,0.06)"};
                        cursor:pointer;
                        --wails-draggable: no-drag;">
              <div style="width:18px; height:18px; border-radius:50%;
                          border:1.5px solid ${sel ? "var(--brand-400)" : "rgba(255,255,255,0.18)"};
                          display:flex; align-items:center; justify-content:center; flex-shrink:0;">
                ${sel ? html`<div style="width:8px; height:8px; border-radius:50%; background:var(--brand-400);"></div>` : nothing}
              </div>
              <div style="flex:1;">
                <div style="display:flex; align-items:baseline; gap:8px;">
                  <span style="font-size:13.5px; font-weight:500; color:var(--fg-0); letter-spacing:-0.005em;">${m.name}</span>
                  <span style="font-size:11px; color:var(--fg-3);">· ${m.author}</span>
                  ${m.rec ? html`<lthn-state-pill variant="latest">${this.t.s2Recommended}</lthn-state-pill>` : nothing}
                </div>
                <div style="font-size:11.5px; color:var(--fg-2); margin-top:3px;">${m.desc}</div>
              </div>
              <div style="text-align:right; font-family:var(--font-mono); font-size:11px; color:var(--fg-3); letter-spacing:0.02em;">
                <div>${m.size}</div>
                <div style="margin-top:2px;">${m.ram} RAM</div>
              </div>
            </div>
          `;
          })}
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
            ${this.t.s3Title}
          </div>
          <div style="font-size:13px; color:var(--fg-2); margin-top:8px; line-height:1.55; max-width:460px;">
            ${(() => {
              const parts = this.t.s3Body.split("%s");
              return html`${parts[0]}<span style="font-family:var(--font-mono); color:var(--fg-1);">${this.endpoint}</span>${parts[1] || ""}`;
            })()}
          </div>
        </div>
        <div style="padding:10px 12px; border-radius:6px;
                    background:rgba(64,193,197,0.06);
                    border:1px solid rgba(64,193,197,0.18);
                    font-size:11.5px; color:var(--fg-2); line-height:1.55;">
          <i class="fa-solid fa-circle-info" style="color:var(--brand-300); font-size:11px; margin-right:6px;"></i>
          Showing what we detected — toggles are preview here. Wiring
          a client (writing to its config file) happens from the
          Integrations window after onboarding.
        </div>
        <div style="display:flex; flex-direction:column; gap:6px;">
          ${clients.length === 0 ? html`
            <div style="padding:14px 16px; border-radius:8px; background:rgba(255,255,255,0.025);
                        border:1px solid rgba(255,255,255,0.05); font-size:12px; color:var(--fg-3); line-height:1.55;">
              ${this.t.s3Empty}
            </div>
          ` : clients.map(c => html`
            <div style="display:flex; align-items:center; gap:14px; padding:12px 14px; border-radius:8px;
                        background:rgba(255,255,255,0.03); border:1px solid rgba(255,255,255,0.06);">
              <input type="checkbox" ?checked=${c.checked} ?disabled=${c.wired} style="accent-color:var(--brand-400);" />
              <div style="flex:1;">
                <div style="display:flex; align-items:baseline; gap:8px;">
                  <span style="font-size:12.5px; font-weight:500; color:var(--fg-0);">${c.name}</span>
                  ${c.wired ? html`<lthn-state-pill variant="latest">${this.t.s3Wired}</lthn-state-pill>` : nothing}
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

  /** Step 4 — Menu Behaviours tour. Single step, single illustration:
   *  a mock rail row with two callouts (word vs icon) and a footer
   *  reminding users that ⌘-click works anywhere on the row. See
   *  plans/project/lthn/desktop/RFC.menu-behaviours.md § 6. */
  _step4() {
    return html`
      <div style="display:flex; flex-direction:column; gap:18px;">
        <div>
          <div style="font-size:24px; font-weight:600; color:var(--fg-0); letter-spacing:-0.018em;">
            ${this.t.s4Title}
          </div>
          <div style="font-size:13px; color:var(--fg-2); margin-top:8px; line-height:1.55; max-width:480px;">
            ${this.t.s4Body}
          </div>
        </div>
        <!-- Mock rail row — a single illustrative <button> identical
             in shape to the live <lthn-app-shell> rail, with callout
             arrows pointing at icon vs word. Non-interactive. -->
        <div style="display:flex; align-items:center; gap:24px; padding:24px 22px;
                    background:rgba(64,193,197,0.04);
                    border:1.5px dashed rgba(64,193,197,0.30);
                    border-radius:10px;">
          <div style="position:relative;">
            <div style="display:flex; align-items:center; gap:12px;
                        padding:8px 12px; border-radius:6px;
                        background:rgba(64,193,197,0.10);
                        border:1px solid rgba(64,193,197,0.22);
                        font-family:var(--font-sans); font-size:12.5px; font-weight:500;
                        color:var(--brand-300); min-width:180px;">
              <i class="fa-solid fa-comments" style="font-size:13px; width:14px; text-align:center; color:var(--brand-300);"></i>
              <span style="flex:1;">Chat</span>
            </div>
            <!-- Word callout (right side) -->
            <div style="position:absolute; left:108%; top:50%; transform:translateY(-50%);
                        display:flex; align-items:center; gap:6px; white-space:nowrap;">
              <div style="width:18px; height:1px; background:var(--brand-300);"></div>
              <div style="font-family:var(--font-mono); font-size:10.5px; color:var(--brand-300); letter-spacing:0.02em;">
                ${this.t.s4WordLabel}
              </div>
            </div>
            <!-- Icon callout (left side) -->
            <div style="position:absolute; right:108%; top:50%; transform:translateY(-50%);
                        display:flex; align-items:center; gap:6px; white-space:nowrap;">
              <div style="font-family:var(--font-mono); font-size:10.5px; color:var(--fg-2); letter-spacing:0.02em;">
                ${this.t.s4IconLabel}
              </div>
              <div style="width:18px; height:1px; background:var(--fg-2);"></div>
            </div>
          </div>
        </div>
        <div style="display:flex; align-items:center; gap:8px; font-size:11.5px; color:var(--fg-3);">
          <i class="fa-solid fa-keyboard" style="font-size:12px; color:var(--fg-3);"></i>
          <span>${this.t.s4Modifier}</span>
        </div>
        <div style="font-size:11px; color:var(--fg-3); line-height:1.5; margin-top:-4px;">
          ${this.t.s4SettingsHint}
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

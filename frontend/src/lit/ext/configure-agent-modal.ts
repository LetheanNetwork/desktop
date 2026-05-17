// SPDX-Licence-Identifier: EUPL-1.2
// E4.3.a · configure-agent modal — <lthn-configure-agent-modal>
//
// Two-step wizard fired from the fleet panel's "Configure an agent"
// CTA. Step 1 picks a provider (catalogue baked in below); step 2
// renders provider-aware fields (remote → endpoint + api key; local
// → context length / temperature / sampling params). Save calls:
//
//   1. keys.WPutTier1(ref, plaintext) — for remote providers only
//      (tier-1: post-unlock provider creds per RFC.stage-e-keys-
//      partition v3; the untiered WPut shim was removed in E.K.C)
//   2. fleet.UpsertAgent(agent)       — always
//
// Then dispatches "lthn:fleet:agent-saved" so the host panel can
// refresh. ESC dismisses; backdrop click dismisses; explicit Cancel
// dismisses. No partial saves on cancel.
//
// Open by toggling the `open` property (or setting attribute open):
//
//   <lthn-configure-agent-modal .open=${state.openConfig}
//      @lthn:fleet:agent-saved=${this.handleSaved}
//      @lthn:fleet:configure-cancel=${this.handleCancel}>
//   </lthn-configure-agent-modal>

import { LitElement, html, nothing } from "lit";
import { UpsertAgent } from "@desktop/fleet/service";
import { WPutTier1 } from "@desktop/keys/service";
import { Agent as AgentModel } from "@desktop/fleet/models";
import { unwrap } from "../result";

type Step = "provider" | "configure";
type Kind = "remote" | "local";

interface ProviderSpec {
  id:           string;   // matches agents.provider column
  label:        string;
  kind:         Kind;
  icon:         string;
  blurb:        string;
  defaultModel: string;
  defaultBase:  string;
  needsKey:     boolean;
  needsBaseUrl: boolean;  // hide Base URL field entirely when false
  modelHint:    string;
}

/**
 * Provider catalogue — single source of truth. Adding a provider is
 * a one-spec append; the wizard, picker grid, and field shape all
 * derive from this list.
 *
 * To add a provider in future: prepend `{ id, label, kind, icon,
 * blurb, defaultModel, defaultBase, needsKey, modelHint }` and the
 * grid + step-2 form pick it up automatically.
 */
const PROVIDERS: ProviderSpec[] = [
  {
    id: "openai", label: "OpenAI", kind: "remote", icon: "fa-circle",
    blurb: "Official OpenAI API — GPT-5 / GPT-5-pro / o-series.",
    defaultModel: "gpt-5", defaultBase: "https://api.openai.com/v1",
    needsKey: true, needsBaseUrl: false,
    modelHint: "gpt-5 / gpt-5-pro / o3",
  },
  {
    id: "anthropic", label: "Anthropic", kind: "remote", icon: "fa-mountain",
    blurb: "Claude — Opus 4.7, Sonnet 4.6, Haiku 4.5.",
    defaultModel: "claude-opus-4-7", defaultBase: "https://api.anthropic.com/v1",
    needsKey: true, needsBaseUrl: false,
    modelHint: "claude-opus-4-7 / sonnet-4-6",
  },
  {
    id: "opencode-go", label: "OpenCode Go", kind: "remote", icon: "fa-code-branch",
    blurb: "OpenCode subscription — DeepSeek v4 Pro, GLM, Qwen-Coder.",
    defaultModel: "deepseek-v4-pro", defaultBase: "https://opencode.com/api/v1",
    needsKey: true, needsBaseUrl: true,
    modelHint: "deepseek-v4-pro / glm-4.6",
  },
  {
    id: "openai-compat", label: "OpenAI-compatible", kind: "remote", icon: "fa-plug",
    blurb: "Any OpenAI-API-compatible endpoint — Groq, Cerebras, Together, custom.",
    defaultModel: "", defaultBase: "",
    needsKey: true, needsBaseUrl: true,
    modelHint: "model name as listed by the provider",
  },
  {
    id: "ollama", label: "Ollama (local)", kind: "local", icon: "fa-laptop-code",
    blurb: "Local Ollama daemon on this machine or the fleet.",
    defaultModel: "gemma-4-e2b", defaultBase: "http://localhost:11434",
    needsKey: false, needsBaseUrl: true,
    modelHint: "ollama list",
  },
  {
    id: "lemma-local", label: "Lemma (local)", kind: "local", icon: "fa-cube",
    blurb: "Native Lethean inference via lthn-mlx / go-mlx on the fleet.",
    defaultModel: "lemma-q8", defaultBase: "",
    needsKey: false, needsBaseUrl: false,
    modelHint: "lemma-q8 / gemma-3-27b-q4",
  },
];

const DEFAULT_CONTEXT = 8192;
const DEFAULT_TEMP    = 0.7;

class LthnConfigureAgentModal extends LitElement {
  static readonly properties = {
    open:         { type: Boolean, reflect: true },
    editing:      { attribute: false },
    step:         { state: true },
    provider:     { state: true },
    name:         { state: true },
    model:        { state: true },
    baseUrl:      { state: true },
    apiKey:       { state: true },
    persona:      { state: true },
    contextLen:   { state: true },
    temperature:  { state: true },
    saving:       { state: true },
    saveErr:      { state: true },
  };
  declare open:        boolean;
  /** When set, the modal opens in edit mode pre-filled from this row.
   *  Save path is the same UpsertAgent call (id collision = update). */
  declare editing:     AgentModel | null;
  declare step:        Step;
  declare provider:    ProviderSpec | null;
  declare name:        string;
  declare model:       string;
  declare baseUrl:     string;
  declare apiKey:      string;
  declare persona:     string;
  declare contextLen:  number;
  declare temperature: number;
  declare saving:      boolean;
  declare saveErr:     string;

  constructor() {
    super();
    this.open = false;
    this.editing = null;
    this.step = "provider";
    this.provider = null;
    this.name = "";
    this.model = "";
    this.baseUrl = "";
    this.apiKey = "";
    this.persona = "";
    this.contextLen = DEFAULT_CONTEXT;
    this.temperature = DEFAULT_TEMP;
    this.saving = false;
    this.saveErr = "";
  }
  createRenderRoot() { return this; }

  /** When `editing` is set (or `open` toggles to true with editing
   *  present), pre-fill all fields and skip straight to the
   *  configure step. Provider is resolved from the catalogue by id;
   *  unknown providers fall back to openai-compat. API key field
   *  stays blank in edit mode — we can't show plaintext (Get is
   *  Go-side only) and the user only types it when rotating. */
  updated(changed: Map<string, unknown>) {
    if ((changed.has("open") || changed.has("editing")) && this.open && this.editing) {
      const e = this.editing;
      const provider = PROVIDERS.find(p => p.id === e.provider)
        || PROVIDERS.find(p => p.id === "openai-compat")!;
      this.provider = provider;
      this.name = e.name || "";
      this.model = e.model || "";
      this.baseUrl = e.base_url || "";
      this.apiKey = "";
      this.persona = e.persona || "";
      let ctx = DEFAULT_CONTEXT;
      let temp = DEFAULT_TEMP;
      if (e.model_settings) {
        try {
          const ms = JSON.parse(e.model_settings) as { context_length?: number; temperature?: number };
          if (typeof ms.context_length === "number") ctx = ms.context_length;
          if (typeof ms.temperature === "number") temp = ms.temperature;
        } catch { /* ignore malformed JSON, defaults apply */ }
      }
      this.contextLen = ctx;
      this.temperature = temp;
      this.step = "configure";
      this.saveErr = "";
    }
    if (changed.has("open") && !this.open) {
      // Closed (externally or via cancel/save) — clear editing so
      // the next add-flow opens at the provider picker.
      this.editing = null;
    }
  }

  /** Reset wizard state to step 1, no fields. Called on cancel,
   *  save-success, and external re-open. */
  private reset() {
    this.step = "provider";
    this.provider = null;
    this.name = "";
    this.model = "";
    this.baseUrl = "";
    this.apiKey = "";
    this.persona = "";
    this.contextLen = DEFAULT_CONTEXT;
    this.temperature = DEFAULT_TEMP;
    this.saving = false;
    this.saveErr = "";
  }

  /** Set the chosen provider + advance to step 2, prefilling
   *  defaults so the user only edits what's specific to them. */
  private pickProvider(p: ProviderSpec) {
    this.provider = p;
    if (!this.name) this.name = `${p.label}${p.defaultModel ? " · " + p.defaultModel : ""}`;
    if (!this.model) this.model = p.defaultModel;
    if (!this.baseUrl) this.baseUrl = p.defaultBase;
    this.step = "configure";
  }

  private back() {
    this.step = "provider";
  }

  private cancel() {
    this.open = false;
    this.reset();
    this.dispatchEvent(new CustomEvent("lthn:fleet:configure-cancel", {
      bubbles: true, composed: true,
    }));
  }

  /** Build the agent row from current state + save. Remote
   *  providers also persist their api key to the encrypted store
   *  first, with the agent row's api_key_ref pointing at it. */
  private async save() {
    if (!this.provider) return;
    if (!this.name.trim()) {
      this.saveErr = "Name is required";
      return;
    }
    const isEdit = !!this.editing;
    // New rows MUST supply an API key for remote providers; edits
    // leave the existing sealed blob alone unless the user types
    // a new value (rotation).
    if (this.provider.needsKey && !isEdit && !this.apiKey.trim()) {
      this.saveErr = "API key is required for this provider";
      return;
    }
    this.saving = true;
    this.saveErr = "";
    // Preserve the id when editing so a rename updates the existing
    // row rather than spawning a duplicate. New-flow derives id
    // from name as before.
    const id = this.editing?.id || this.idFromName(this.name);
    const ref = this.provider.needsKey ? `agent:${id}` : "";
    try {
      if (this.provider.needsKey && this.apiKey.trim()) {
        const keyR = await WPutTier1(ref, this.apiKey.trim());
        if (!keyR.OK) {
          this.saveErr = "Failed to seal API key";
          this.saving = false;
          return;
        }
      }
      const modelSettings = this.provider.kind === "local"
        ? JSON.stringify({ context_length: this.contextLen, temperature: this.temperature })
        : "";
      const agent: AgentModel = new AgentModel({
        id,
        name: this.name.trim(),
        provider: this.provider.id,
        kind: this.provider.kind,
        base_url: this.baseUrl.trim(),
        api_key_ref: ref,
        model: this.model.trim(),
        persona: this.persona.trim(),
        features: "",
        model_settings: modelSettings,
        status: "idle",
        tags: "",
        created_at: 0,
        updated_at: 0,
      });
      const agentR = await unwrap<unknown>(UpsertAgent(agent), null);
      if (agentR === null && !this.saveErr) {
        this.saveErr = "Failed to save agent";
        this.saving = false;
        return;
      }
      // Success — notify host, close, reset.
      this.dispatchEvent(new CustomEvent("lthn:fleet:agent-saved", {
        detail: { id, provider: this.provider.id },
        bubbles: true, composed: true,
      }));
      this.open = false;
      this.reset();
    } catch (e: unknown) {
      this.saveErr = e instanceof Error ? e.message : String(e);
    } finally {
      this.saving = false;
    }
  }

  /** Stable id from human name — lowercase, alphanumeric + hyphens,
   *  truncated. Collisions handled by upsert behaviour on the Go
   *  side (later same-id config replaces). */
  private idFromName(name: string): string {
    const slug = name.toLowerCase()
      .replace(/[^a-z0-9]+/g, "-")
      .replace(/^-+|-+$/g, "")
      .slice(0, 48);
    return slug || `agent-${Date.now().toString(36)}`;
  }

  private onBackdropClick(e: MouseEvent) {
    if (e.target === e.currentTarget) this.cancel();
  }

  private onKeydown(e: KeyboardEvent) {
    if (e.key === "Escape") {
      e.stopPropagation();
      this.cancel();
    }
  }

  private renderProviderCard(p: ProviderSpec) {
    return html`
      <button
        @click=${() => this.pickProvider(p)}
        style="
          display:flex; flex-direction:column; gap:8px; align-items:flex-start;
          padding:14px 16px; text-align:left;
          background:rgba(255,255,255,0.03);
          border:1px solid rgba(255,255,255,0.06);
          border-radius:10px;
          cursor:pointer;
          --wails-draggable: no-drag;
        "
        onmouseover="this.style.background='rgba(64,193,197,0.05)'; this.style.borderColor='rgba(64,193,197,0.25)';"
        onmouseout="this.style.background='rgba(255,255,255,0.03)'; this.style.borderColor='rgba(255,255,255,0.06)';">
        <div style="display:flex; align-items:center; gap:10px; width:100%;">
          <div style="width:32px; height:32px;
                      display:flex; align-items:center; justify-content:center;
                      background:rgba(64,193,197,0.08);
                      border-radius:8px;">
            <i class="fa-solid ${p.icon}" style="font-size:14px; color:var(--brand-300);"></i>
          </div>
          <div style="font-size:13px; font-weight:600; color:var(--fg-0); flex:1;">${p.label}</div>
          <lthn-state-pill variant=${p.kind === "local" ? "latest" : "preview"}>${p.kind}</lthn-state-pill>
        </div>
        <div style="font-size:11.5px; color:var(--fg-2); line-height:1.5;">${p.blurb}</div>
      </button>
    `;
  }

  private renderProviderPicker() {
    return html`
      <div style="display:flex; flex-direction:column; gap:6px; margin-bottom:14px;">
        <div style="font-size:16px; font-weight:600; color:var(--fg-0);
                    letter-spacing:-0.005em;">
          Configure an agent
        </div>
        <div style="font-size:12px; color:var(--fg-2); line-height:1.5;">
          Pick a provider. Remote providers need an API key (encrypted at rest in
          ~/Lethean/data/keys/); local providers run against the machine fleet.
        </div>
      </div>
      <div style="display:grid; grid-template-columns:1fr 1fr; gap:10px;">
        ${PROVIDERS.map(p => this.renderProviderCard(p))}
      </div>
    `;
  }

  /** Heading + back-button row for the configure step. Edit mode
   *  hides the back button (no meaningful "back" — there's no
   *  step-1 picker for an existing row). */
  private renderConfigureHeader(p: ProviderSpec, isEdit: boolean) {
    return html`
      <div style="display:flex; align-items:center; gap:10px;">
        ${isEdit
          ? nothing
          : html`
            <lthn-btn tone="quiet" size="sm" @click=${() => this.back()}>
              <i class="fa-solid fa-chevron-left" style="font-size:10px;"></i>
              Back
            </lthn-btn>
          `}
        <div style="flex:1; display:flex; align-items:center; gap:8px;">
          <i class="fa-solid ${p.icon}" style="color:var(--brand-300);"></i>
          <div style="font-size:14px; font-weight:600; color:var(--fg-0);">
            ${isEdit ? "Edit " : ""}${p.label}
          </div>
          <lthn-state-pill variant=${p.kind === "local" ? "latest" : "preview"}>${p.kind}</lthn-state-pill>
        </div>
      </div>
    `;
  }

  private renderField(label: string, child: unknown, hint?: string) {
    return html`
      <label style="display:flex; flex-direction:column; gap:4px;">
        <span style="font-size:11px; color:var(--fg-3);
                     text-transform:uppercase; letter-spacing:0.06em;">
          ${label}
        </span>
        ${child}
        ${hint ? html`<span style="font-size:10.5px; color:var(--fg-3); margin-top:1px;">${hint}</span>` : nothing}
      </label>
    `;
  }

  private inputStyle() {
    return `
      width:100%; padding:8px 10px;
      background:rgba(0,0,0,0.2);
      border:1px solid rgba(255,255,255,0.08);
      border-radius:6px;
      color:var(--fg-0);
      font-family:var(--font-mono); font-size:12px;
      outline:none;
      --wails-draggable: no-drag;
    `;
  }

  private renderConfigureStep() {
    const p = this.provider;
    if (!p) return nothing;
    const isEdit = !!this.editing;
    return html`
      <div style="display:flex; flex-direction:column; gap:14px;">
        ${this.renderConfigureHeader(p, isEdit)}

        ${this.renderField("Name", html`
          <input type="text" .value=${this.name}
            @input=${(e: Event) => { this.name = (e.target as HTMLInputElement).value; }}
            placeholder="e.g. ${p.label} · production"
            style=${this.inputStyle()}>
        `, "Shown in the fleet panel and chat headers.")}

        ${this.renderField("Model", html`
          <input type="text" .value=${this.model}
            @input=${(e: Event) => { this.model = (e.target as HTMLInputElement).value; }}
            placeholder=${p.modelHint}
            style=${this.inputStyle()}>
        `, p.modelHint ? `Examples: ${p.modelHint}` : undefined)}

        ${p.needsBaseUrl
          ? this.renderField("Base URL", html`
              <input type="text" .value=${this.baseUrl}
                @input=${(e: Event) => { this.baseUrl = (e.target as HTMLInputElement).value; }}
                placeholder=${p.defaultBase || "https://your-endpoint/v1"}
                style=${this.inputStyle()}>
            `, p.kind === "local"
                 ? "Where the local runtime is listening. Default usually works."
                 : "OpenAI-compatible base URL ending in /v1.")
          : nothing}

        ${p.needsKey
          ? this.renderField(isEdit ? "API key (leave blank to keep current)" : "API key", html`
              <input type="password" .value=${this.apiKey}
                @input=${(e: Event) => { this.apiKey = (e.target as HTMLInputElement).value; }}
                placeholder=${isEdit ? "•••••••••• (stored — type to rotate)" : "sk-…"}
                autocomplete="off"
                style=${this.inputStyle()}>
            `, isEdit
                 ? "Existing key stays sealed. Type a new one to rotate it."
                 : "Sealed with ChaCha20-Poly1305 at rest. Never crosses the WebView again.")
          : nothing}

        ${this.renderField("Persona", html`
          <textarea .value=${this.persona}
            @input=${(e: Event) => { this.persona = (e.target as HTMLInputElement).value; }}
            placeholder="One paragraph: voice, constraints, what this agent is for."
            rows="3"
            style="${this.inputStyle()}; font-family:var(--font-sans); resize:vertical; min-height:60px;"></textarea>
        `, "Becomes the system prompt the runtime prepends to every turn.")}

        ${p.kind === "local"
          ? html`
            <div style="display:grid; grid-template-columns:1fr 1fr; gap:12px;">
              ${this.renderField("Context length", html`
                <input type="number" .value=${String(this.contextLen)} min="512" step="512"
                  @input=${(e: Event) => { this.contextLen = parseInt((e.target as HTMLInputElement).value, 10) || DEFAULT_CONTEXT; }}
                  style=${this.inputStyle()}>
              `, "Tokens the runtime keeps in KV cache.")}
              ${this.renderField("Temperature", html`
                <input type="number" .value=${String(this.temperature)} min="0" max="2" step="0.05"
                  @input=${(e: Event) => { this.temperature = parseFloat((e.target as HTMLInputElement).value) || DEFAULT_TEMP; }}
                  style=${this.inputStyle()}>
              `, "0 = deterministic, 1 = creative.")}
            </div>
          `
          : nothing}

        ${this.saveErr ? html`
          <div style="padding:9px 12px;
                      background:rgba(244,67,54,0.06);
                      border:1px solid rgba(244,67,54,0.22);
                      border-radius:6px;
                      font-size:11.5px; color:var(--warning-400);">
            ${this.saveErr}
          </div>
        ` : nothing}

        <div style="display:flex; justify-content:flex-end; gap:8px; padding-top:6px;">
          <lthn-btn tone="ghost" size="md" @click=${() => this.cancel()}>Cancel</lthn-btn>
          <lthn-btn tone="primary" size="md" ?disabled=${this.saving}
            @click=${() => { void this.save(); }}>
            ${this.saving
              ? html`<i class="fa-solid fa-spinner fa-spin" style="font-size:11px;"></i> Saving…`
              : isEdit ? "Update agent" : "Save agent"}
          </lthn-btn>
        </div>
      </div>
    `;
  }

  render() {
    if (!this.open) return nothing;
    return html`
      <div
        @click=${(e: MouseEvent) => this.onBackdropClick(e)}
        @keydown=${(e: KeyboardEvent) => this.onKeydown(e)}
        tabindex="-1"
        style="
          position:fixed; inset:0;
          background:rgba(0,0,0,0.55);
          backdrop-filter: blur(4px);
          -webkit-backdrop-filter: blur(4px);
          display:flex; align-items:center; justify-content:center;
          padding:28px;
          z-index:9999;
          --wails-draggable: no-drag;
        ">
        <div style="
          width:100%; max-width:520px;
          max-height:calc(100vh - 56px);
          overflow:auto;
          background:#1a1a1c;
          border:1px solid rgba(255,255,255,0.08);
          border-radius:14px;
          padding:22px 24px;
          box-shadow: 0 24px 60px rgba(0,0,0,0.4);
        ">
          ${this.step === "provider" ? this.renderProviderPicker() : this.renderConfigureStep()}
        </div>
      </div>
    `;
  }
}
customElements.define("lthn-configure-agent-modal", LthnConfigureAgentModal);

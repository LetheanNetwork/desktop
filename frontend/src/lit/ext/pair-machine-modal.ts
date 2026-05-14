// SPDX-Licence-Identifier: EUPL-1.2
// E4.3.b · pair-machine modal — <lthn-pair-machine-modal>
//
// Single-page wizard fired from the fleet panel's "Pair a machine"
// CTA. Adds a row to fleet_machines with the operator-picked
// capability set. Discovery (probing the host for what's installed)
// is reserved for the first-run wizard; this modal is the manual
// path for "I know what's on this box, let me say so".
//
// Save calls fleet.UpsertMachine then dispatches
// "lthn:fleet:machine-saved" so the host panel can refresh.
// ESC / backdrop / Cancel all dismiss with no partial saves.

import { LitElement, html, nothing } from "lit";
import { UpsertMachine } from "@desktop/fleet/service";
import { Machine as MachineModel } from "@desktop/fleet/models";
import { unwrap } from "../result";

/**
 * Capability spec — id matches fleet.Capability* constants on the Go
 * side. Adding a capability later is a one-spec append; the picker
 * grid + save flow pick it up automatically.
 */
interface CapabilitySpec {
  id:    string;
  label: string;
  icon:  string;
  blurb: string;
}

const CAPABILITIES: CapabilitySpec[] = [
  {
    id: "inference", label: "Inference", icon: "fa-microchip",
    blurb: "Runs models — Ollama, Lemma, lthn-mlx, or any HTTP-compatible inference endpoint.",
  },
  {
    id: "sandbox", label: "Sandbox Runner", icon: "fa-cubes",
    blurb: "Executes containers and sandboxed code via Docker, Podman, or Apple containers.",
  },
  {
    id: "bastion", label: "Bastion Machine", icon: "fa-shield-halved",
    blurb: "Always-on host for homelab services + the manager agent you talk to daily. Network reachable.",
  },
];

class LthnPairMachineModal extends LitElement {
  static readonly properties = {
    open:        { type: Boolean, reflect: true },
    editing:     { attribute: false },
    name:        { state: true },
    host:        { state: true },
    port:        { state: true },
    capabilities: { state: true },
    tags:        { state: true },
    isSelf:      { state: true },
    saving:      { state: true },
    saveErr:     { state: true },
  };
  declare open:        boolean;
  /** When set, the modal opens pre-filled from this row. Save calls
   *  the same UpsertMachine path; the existing id is preserved
   *  regardless of name edits. */
  declare editing:     MachineModel | null;
  declare name:        string;
  declare host:        string;
  declare port:        number;
  declare capabilities: string[];
  declare tags:        string;
  declare isSelf:      boolean;
  declare saving:      boolean;
  declare saveErr:     string;

  constructor() {
    super();
    this.open = false;
    this.editing = null;
    this.name = "";
    this.host = "";
    this.port = 0;
    this.capabilities = [];
    this.tags = "";
    this.isSelf = false;
    this.saving = false;
    this.saveErr = "";
  }
  createRenderRoot() { return this; }

  updated(changed: Map<string, unknown>) {
    if ((changed.has("open") || changed.has("editing")) && this.open && this.editing) {
      const e = this.editing;
      this.name = e.name || "";
      this.host = e.host || "";
      this.port = e.port || 0;
      this.capabilities = [...(e.capabilities || [])];
      this.tags = e.tags || "";
      this.isSelf = !!e.is_self;
      this.saveErr = "";
    }
    if (changed.has("open") && !this.open) {
      this.editing = null;
    }
  }

  private reset() {
    this.name = "";
    this.host = "";
    this.port = 0;
    this.capabilities = [];
    this.tags = "";
    this.isSelf = false;
    this.saving = false;
    this.saveErr = "";
  }

  private cancel() {
    this.open = false;
    this.reset();
    this.dispatchEvent(new CustomEvent("lthn:fleet:pair-cancel", {
      bubbles: true, composed: true,
    }));
  }

  private toggleCapability(id: string) {
    if (this.capabilities.includes(id)) {
      this.capabilities = this.capabilities.filter(c => c !== id);
    } else {
      this.capabilities = [...this.capabilities, id];
    }
  }

  private async save() {
    if (!this.name.trim()) {
      this.saveErr = "Name is required";
      return;
    }
    if (this.capabilities.length === 0) {
      this.saveErr = "Pick at least one capability — a machine with no roles is just a name";
      return;
    }
    this.saving = true;
    this.saveErr = "";
    // Preserve the id when editing so a rename updates the existing
    // row rather than spawning a duplicate.
    const id = this.editing?.id || this.idFromName(this.name);
    try {
      const machine = new MachineModel({
        id,
        name: this.name.trim(),
        arch: "",
        host: this.host.trim(),
        port: this.port || 0,
        status: this.isSelf ? "online" : "paired",
        model: "",
        load_pct: 0,
        tps: 0,
        is_self: this.isSelf,
        tags: this.tags.trim(),
        capabilities: [...this.capabilities],
        created_at: 0,
        updated_at: 0,
      });
      const r = await unwrap<unknown>(UpsertMachine(machine), null);
      if (r === null && !this.saveErr) {
        this.saveErr = "Failed to save machine";
        this.saving = false;
        return;
      }
      this.dispatchEvent(new CustomEvent("lthn:fleet:machine-saved", {
        detail: { id, capabilities: [...this.capabilities] },
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

  private idFromName(name: string): string {
    const slug = name.toLowerCase()
      .replace(/[^a-z0-9]+/g, "-")
      .replace(/^-+|-+$/g, "")
      .slice(0, 48);
    return slug || `machine-${Date.now().toString(36)}`;
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

  private renderCapabilityCard(c: CapabilitySpec) {
    const picked = this.capabilities.includes(c.id);
    const bg = picked ? "rgba(64,193,197,0.08)" : "rgba(255,255,255,0.03)";
    const border = picked ? "rgba(64,193,197,0.35)" : "rgba(255,255,255,0.06)";
    return html`
      <button
        @click=${() => this.toggleCapability(c.id)}
        type="button"
        style="
          display:flex; flex-direction:column; gap:6px; align-items:flex-start;
          padding:12px 14px; text-align:left; width:100%;
          background:${bg};
          border:1px solid ${border};
          border-radius:10px;
          cursor:pointer;
          --wails-draggable: no-drag;
        ">
        <div style="display:flex; align-items:center; gap:10px; width:100%;">
          <div style="width:30px; height:30px;
                      display:flex; align-items:center; justify-content:center;
                      background:rgba(64,193,197,0.08);
                      border-radius:8px;">
            <i class="fa-solid ${c.icon}" style="font-size:13px; color:var(--brand-300);"></i>
          </div>
          <div style="font-size:13px; font-weight:600; color:var(--fg-0); flex:1;">${c.label}</div>
          ${picked
            ? html`<i class="fa-solid fa-circle-check" style="font-size:14px; color:var(--brand-300);"></i>`
            : html`<i class="fa-regular fa-circle" style="font-size:14px; color:var(--fg-3);"></i>`}
        </div>
        <div style="font-size:11px; color:var(--fg-2); line-height:1.5; padding-left:40px;">
          ${c.blurb}
        </div>
      </button>
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

  render() {
    if (!this.open) return nothing;
    const isEdit = !!this.editing;
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
          width:100%; max-width:560px;
          max-height:calc(100vh - 56px);
          overflow:auto;
          background:#1a1a1c;
          border:1px solid rgba(255,255,255,0.08);
          border-radius:14px;
          padding:22px 24px;
          box-shadow: 0 24px 60px rgba(0,0,0,0.4);
        ">
          <div style="display:flex; flex-direction:column; gap:14px;">

            <div style="display:flex; flex-direction:column; gap:6px;">
              <div style="font-size:16px; font-weight:600; color:var(--fg-0);
                          letter-spacing:-0.005em;">
                ${isEdit ? `Edit ${this.editing?.name || "machine"}` : "Pair a machine"}
              </div>
              <div style="font-size:12px; color:var(--fg-2); line-height:1.5;">
                ${isEdit
                  ? "Tweak any field. Save updates the row in-place — same id, same routing rules attached."
                  : "Add a node to the compute fleet. Auto-discovery lands in the first-run wizard; for now, name the box and pick the roles it fulfils."}
              </div>
            </div>

            <div style="display:grid; grid-template-columns:2fr 1fr; gap:12px;">
              ${this.renderField("Name", html`
                <input type="text" .value=${this.name}
                  @input=${(e: Event) => { this.name = (e.target as HTMLInputElement).value; }}
                  placeholder="e.g. shop · 7950X"
                  style=${this.inputStyle()}>
              `, "Shown in the fleet panel and routing rules.")}
              ${this.renderField("Tags", html`
                <input type="text" .value=${this.tags}
                  @input=${(e: Event) => { this.tags = (e.target as HTMLInputElement).value; }}
                  placeholder="gpu,prod,home"
                  style=${this.inputStyle()}>
              `, "Comma-separated.")}
            </div>

            <div style="display:grid; grid-template-columns:2fr 1fr; gap:12px;">
              ${this.renderField("Host", html`
                <input type="text" .value=${this.host}
                  @input=${(e: Event) => { this.host = (e.target as HTMLInputElement).value; }}
                  placeholder="studio.local / 192.168.1.42 / leave empty for self"
                  style=${this.inputStyle()}>
              `, "Hostname or IP. Empty if this entry represents this Mac.")}
              ${this.renderField("Port", html`
                <input type="number" .value=${String(this.port || "")}
                  @input=${(e: Event) => { this.port = parseInt((e.target as HTMLInputElement).value, 10) || 0; }}
                  placeholder="0"
                  style=${this.inputStyle()}>
              `, "Optional.")}
            </div>

            <label style="display:flex; align-items:center; gap:8px;
                          font-size:12px; color:var(--fg-1); cursor:pointer;
                          --wails-draggable: no-drag;">
              <input type="checkbox" .checked=${this.isSelf}
                @change=${(e: Event) => { this.isSelf = (e.target as HTMLInputElement).checked; }}>
              This entry represents the local machine (sorts first in the fleet list).
            </label>

            <div style="display:flex; flex-direction:column; gap:4px;">
              <span style="font-size:11px; color:var(--fg-3);
                           text-transform:uppercase; letter-spacing:0.06em;">
                Capabilities
              </span>
              <div style="font-size:10.5px; color:var(--fg-3); margin-bottom:6px;">
                Pick everything this machine can do. Routing rules use these
                to match work to the right node.
              </div>
              <div style="display:flex; flex-direction:column; gap:8px;">
                ${CAPABILITIES.map(c => this.renderCapabilityCard(c))}
              </div>
            </div>

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
                  : isEdit ? "Update machine" : "Pair machine"}
              </lthn-btn>
            </div>

          </div>
        </div>
      </div>
    `;
  }
}
customElements.define("lthn-pair-machine-modal", LthnPairMachineModal);

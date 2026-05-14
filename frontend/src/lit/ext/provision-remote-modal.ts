// SPDX-Licence-Identifier: EUPL-1.2
// E4.3.c · provision-remote modal — <lthn-provision-remote-modal>
//
// Operator gives us SSH details for a fresh remote (Linux box,
// VPS, homelab node); we ssh in, create the `lthn` user, install +
// start lthn-agent under it, and pair it with the asking bastion.
//
// V1 (this file): UI surface only. The "Provision" button is
// disabled with "(coming soon)" — the SSH/install path lands in a
// follow-up commit with the lthn-agent binary it'll need. Form
// fields are collected so the eventual implementation has a real
// shape to fill.
//
// The "don't have a server yet?" footer surfaces affiliate-tracked
// VPS recommendations from _providers-catalogue.ts. Useful because
// the user is literally in the "I need a server" flow; not spammy.

import { LitElement, html, nothing } from "lit";
import { vpsProviders, refURL } from "./_providers-catalogue";

class LthnProvisionRemoteModal extends LitElement {
  static readonly properties = {
    open:     { type: Boolean, reflect: true },
    host:     { state: true },
    port:     { state: true },
    user:     { state: true },
    authMode: { state: true },
    keyPath:  { state: true },
    password: { state: true },
    label:    { state: true },
    saveErr:  { state: true },
  };
  declare open:     boolean;
  declare host:     string;
  declare port:     number;
  declare user:     string;
  declare authMode: "key" | "password";
  declare keyPath:  string;
  declare password: string;
  declare label:    string;
  declare saveErr:  string;

  constructor() {
    super();
    this.open = false;
    this.host = "";
    this.port = 22;
    this.user = "root";
    this.authMode = "key";
    this.keyPath = "~/.ssh/id_ed25519";
    this.password = "";
    this.label = "";
    this.saveErr = "";
  }
  createRenderRoot() { return this; }

  private reset() {
    this.host = "";
    this.port = 22;
    this.user = "root";
    this.authMode = "key";
    this.keyPath = "~/.ssh/id_ed25519";
    this.password = "";
    this.label = "";
    this.saveErr = "";
  }

  private cancel() {
    this.open = false;
    this.reset();
    this.dispatchEvent(new CustomEvent("lthn:fleet:provision-cancel", {
      bubbles: true, composed: true,
    }));
  }

  private onBackdropClick(e: MouseEvent) {
    if (e.target === e.currentTarget) this.cancel();
  }

  private onKeydown(e: KeyboardEvent) {
    if (e.key === "Escape") { e.stopPropagation(); this.cancel(); }
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

  private renderVpsCard(p: ReturnType<typeof vpsProviders>[number]) {
    return html`
      <a href=${refURL(p.id)} target="_blank" rel="noopener"
         style="display:flex; flex-direction:column; gap:6px;
                padding:10px 12px;
                background:rgba(255,255,255,0.025);
                border:1px solid rgba(255,255,255,0.06);
                border-radius:8px;
                text-decoration:none;
                --wails-draggable: no-drag;"
         onmouseover="this.style.borderColor='rgba(64,193,197,0.25)';"
         onmouseout="this.style.borderColor='rgba(255,255,255,0.06)';">
        <div style="display:flex; align-items:center; gap:8px;">
          <i class="fa-solid ${p.icon}" style="font-size:12px; color:var(--brand-300);"></i>
          <span style="font-size:12px; font-weight:600; color:var(--fg-0); flex:1;">${p.name}</span>
          <i class="fa-solid fa-arrow-up-right-from-square" style="font-size:9px; color:var(--fg-3);"></i>
        </div>
        <div style="font-size:10.5px; color:var(--fg-3); line-height:1.5;">
          ${p.blurb}
        </div>
      </a>
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
          width:100%; max-width:600px;
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
                Provision remote
              </div>
              <div style="font-size:12px; color:var(--fg-2); line-height:1.55;">
                SSH in, create the
                <span style="font-family:var(--font-mono); color:var(--brand-300);">lthn</span>
                user, install + start lthn-agent under it, pair it back
                to this bastion. One bash session, fully reversible —
                <span style="font-family:var(--font-mono); color:var(--brand-300);">userdel -r lthn</span>
                unwinds everything.
              </div>
            </div>

            ${this.renderField("Label", html`
              <input type="text" .value=${this.label}
                @input=${(e: Event) => { this.label = (e.target as HTMLInputElement).value; }}
                placeholder="e.g. de1 · bastion"
                style=${this.inputStyle()}>
            `, "How this machine appears in the fleet list. Optional — defaults to host.")}

            <div style="display:grid; grid-template-columns:2fr 1fr 1fr; gap:12px;">
              ${this.renderField("Host", html`
                <input type="text" .value=${this.host}
                  @input=${(e: Event) => { this.host = (e.target as HTMLInputElement).value; }}
                  placeholder="hostname or IP"
                  style=${this.inputStyle()}>
              `)}
              ${this.renderField("Port", html`
                <input type="number" .value=${String(this.port)}
                  @input=${(e: Event) => { this.port = parseInt((e.target as HTMLInputElement).value, 10) || 22; }}
                  style=${this.inputStyle()}>
              `)}
              ${this.renderField("SSH user", html`
                <input type="text" .value=${this.user}
                  @input=${(e: Event) => { this.user = (e.target as HTMLInputElement).value; }}
                  placeholder="root"
                  style=${this.inputStyle()}>
              `, "User we SSH as to do setup (NOT the lthn user we'll create).")}
            </div>

            <div style="display:flex; flex-direction:column; gap:4px;">
              <span style="font-size:11px; color:var(--fg-3);
                           text-transform:uppercase; letter-spacing:0.06em;">
                Authentication
              </span>
              <div style="display:flex; gap:8px;">
                <label style="display:inline-flex; align-items:center; gap:6px;
                              font-size:12px; color:var(--fg-1); cursor:pointer;
                              --wails-draggable: no-drag;">
                  <input type="radio" name="auth"
                    .checked=${this.authMode === "key"}
                    @change=${() => { this.authMode = "key"; }}>
                  SSH key
                </label>
                <label style="display:inline-flex; align-items:center; gap:6px;
                              font-size:12px; color:var(--fg-1); cursor:pointer;
                              --wails-draggable: no-drag;">
                  <input type="radio" name="auth"
                    .checked=${this.authMode === "password"}
                    @change=${() => { this.authMode = "password"; }}>
                  Password
                </label>
              </div>
            </div>

            ${this.authMode === "key"
              ? this.renderField("Private-key path", html`
                  <input type="text" .value=${this.keyPath}
                    @input=${(e: Event) => { this.keyPath = (e.target as HTMLInputElement).value; }}
                    placeholder="~/.ssh/id_ed25519"
                    style=${this.inputStyle()}>
                `, "Local path to the private key. Read once, never stored.")
              : this.renderField("Password", html`
                  <input type="password" .value=${this.password}
                    @input=${(e: Event) => { this.password = (e.target as HTMLInputElement).value; }}
                    placeholder="••••••••"
                    autocomplete="off"
                    style=${this.inputStyle()}>
                `, "Used for this single SSH session. Never persisted.")}

            ${this.saveErr ? html`
              <div style="padding:9px 12px;
                          background:rgba(244,67,54,0.06);
                          border:1px solid rgba(244,67,54,0.22);
                          border-radius:6px;
                          font-size:11.5px; color:var(--warning-400);">
                ${this.saveErr}
              </div>
            ` : nothing}

            <div style="display:flex; justify-content:flex-end; gap:8px; padding-top:6px;
                        padding-bottom:14px;
                        border-bottom:1px solid rgba(255,255,255,0.05);">
              <lthn-btn tone="ghost" size="md" @click=${() => this.cancel()}>Cancel</lthn-btn>
              <lthn-btn tone="primary" size="md" disabled
                title="SSH provisioning lands in a follow-up alongside the lthn-agent binary.">
                <i class="fa-solid fa-bolt" style="font-size:11px;"></i>
                Provision
                <span style="font-size:9.5px; opacity:0.7; margin-left:4px;">(coming soon)</span>
              </lthn-btn>
            </div>

            <!-- VPS recommendations — affiliate-tracked. Genuinely
                 useful here because the user is in the "I need a
                 server" flow. -->
            <div style="display:flex; flex-direction:column; gap:8px;">
              <div style="display:flex; align-items:baseline; gap:8px;">
                <span style="font-size:11px; color:var(--fg-3);
                             text-transform:uppercase; letter-spacing:0.06em;">
                  Don't have a server yet?
                </span>
                <span style="font-size:10.5px; color:var(--fg-3);">
                  these links support lthn
                </span>
              </div>
              <div style="display:grid; grid-template-columns:1fr 1fr 1fr; gap:8px;">
                ${vpsProviders().map(p => this.renderVpsCard(p))}
              </div>
            </div>

          </div>
        </div>
      </div>
    `;
  }
}
customElements.define("lthn-provision-remote-modal", LthnProvisionRemoteModal);

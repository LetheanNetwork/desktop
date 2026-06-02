// SPDX-Licence-Identifier: EUPL-1.2
// Agents view · Connect — <lthn-view-agent-connect>
//
// "Attach your Claude Code to this machine's CoreAgent hub." The desktop
// runs lthn-agent hub (crew member, MCP plane on :9202, fail-closed bearer).
// This pane surfaces the bearer + the copy-paste steps so the operator can
// wire their own Claude Code — Lethean NEVER edits ~/.claude (not ours).
//
// Backend: Agents.ClaudeConnectRecipe() from @desktop/agents/service, which
// reads MCP_AUTH_TOKEN from pkg/keys tier-0 (the same bearer the crew handed
// the hub). The settings.json snippet + export line are composed here from
// the token; the plugin install commands come straight from the recipe.

import { LitElement, html, nothing } from "lit";
import { renderChrome } from "../../chrome";

/** Mirrors agents.ConnectRecipe (connect.go). */
interface ConnectRecipe {
  token:            string;
  mcp_url:          string;
  marketplace_url:  string;
  install_commands: string[];
}

class LthnViewAgentConnect extends LitElement {
  static readonly properties = {
    w:        { type: Number },
    h:        { type: Number },
    embedded: { type: Boolean, reflect: true },
    recipe:   { state: true },
    err:      { state: true },
    copied:   { state: true },
  };

  declare w: number;
  declare h: number;
  declare embedded: boolean;
  declare recipe: ConnectRecipe | null;
  declare err: string;
  declare copied: string; // key of the block last copied (for the ✓ flash)

  constructor() {
    super();
    this.w = 1180; this.h = 720; this.embedded = false;
    this.recipe = null;
    this.err = "";
    this.copied = "";
  }

  createRenderRoot() { return this; }

  async connectedCallback() {
    super.connectedCallback();
    await this._load();
  }

  /** Pull the connect recipe. On the Go side a failure returns a Result with
   *  no .token in Value — surface a "still starting" hint rather than the raw
   *  error, since the usual cause is opening this before the crew's hub is up. */
  async _load() {
    this.err = "";
    try {
      const svc = await import("@desktop/agents/service");
      const r = await (svc as { ClaudeConnectRecipe: () => Promise<{ Value: unknown }> }).ClaudeConnectRecipe();
      const v = r?.Value as ConnectRecipe | undefined;
      if (!v || !v.token) {
        this.err = "The hub bearer isn't available yet — give the desktop a moment to finish starting (the hub comes up just after launch), then retry.";
        return;
      }
      this.recipe = v;
    } catch (e: unknown) {
      this.err = e instanceof Error ? e.message : String(e);
    }
  }

  async _copy(text: string, key: string) {
    try {
      await navigator.clipboard.writeText(text);
      this.copied = key;
      setTimeout(() => { if (this.copied === key) this.copied = ""; }, 1600);
    } catch { /* clipboard blocked — the text is selectable manually */ }
  }

  _exportLine(): string {
    return `export MCP_AUTH_TOKEN=${this.recipe?.token ?? ""}`;
  }

  /** A copyable code block with a corner copy button. */
  _block(text: string, key: string) {
    return html`
      <div style="position:relative; margin-top:6px;">
        <pre style="margin:0; padding:11px 13px; padding-right:80px; background:rgba(0,0,0,0.28);
                    border:1px solid rgba(255,255,255,0.08); border-radius:6px; overflow-x:auto;
                    font-family:var(--font-mono); font-size:11.5px; line-height:1.55; color:var(--fg-1);
                    white-space:pre-wrap; word-break:break-all;">${text}</pre>
        <span @click=${() => void this._copy(text, key)} title="Copy"
              style="position:absolute; top:8px; right:8px; cursor:pointer; --wails-draggable:no-drag;
                     font-size:10.5px; padding:3px 9px; border-radius:5px;
                     background:rgba(64,193,197,0.12); border:1px solid rgba(64,193,197,0.28);
                     color:var(--brand-300);">
          <i class="fa-solid ${this.copied === key ? "fa-check" : "fa-copy"}" style="font-size:10px;"></i>
          ${this.copied === key ? "copied" : "copy"}
        </span>
      </div>
    `;
  }

  render() {
    const intro = html`
      <div style="font-size:12px; color:var(--fg-2); line-height:1.65;">
        This machine runs your CoreAgent hub. Attach Claude Code so Cladius can drive the fleet from
        the editor — dispatch, review, memory, fleet control — over the loopback MCP plane.
        <span style="color:var(--fg-1);">Lethean never edits your Claude Code config</span> — copy these in yourself.
      </div>
    `;

    let inner;
    if (this.err) {
      inner = html`
        <div style="padding:12px 14px; color:var(--err-400);
                    background:rgba(255,76,76,0.06); border:1px solid rgba(255,76,76,0.18);
                    border-radius:6px; font-size:12px; line-height:1.55;">
          ${this.err}
          <div style="margin-top:10px;">
            <lthn-btn @click=${() => void this._load()}>
              <i class="fa-solid fa-rotate-right" style="font-size:10px;"></i> Retry
            </lthn-btn>
          </div>
        </div>
      `;
    } else if (!this.recipe) {
      inner = html`<div style="font-size:12px; color:var(--fg-3);">Resolving hub connection…</div>`;
    } else {
      inner = html`
        <div>
          <lthn-label>1 · Install the plugin</lthn-label>
          <div style="font-size:10.5px; color:var(--fg-3); margin-top:5px;">
            Run these in Claude Code — the plugin ships the <code style="font-family:var(--font-mono);">core</code> MCP server + the agent commands:
          </div>
          ${this._block((this.recipe.install_commands ?? []).join("\n"), "install")}
          <div style="font-size:10.5px; color:var(--fg-3); margin-top:7px;">
            <code style="font-family:var(--font-mono);">/reload-plugins</code> applies it — no restart.
          </div>
        </div>

        <div>
          <lthn-label>2 · Put the bearer in your launch env</lthn-label>
          <div style="font-size:10.5px; color:var(--fg-3); margin-top:5px;">
            The plugin's <code style="font-family:var(--font-mono);">core</code> reads <code style="font-family:var(--font-mono);">\${MCP_AUTH_TOKEN}</code>. Lethean Desktop sets it when it launches Claude Code; for a manual launch, export it first:
          </div>
          ${this._block(this._exportLine(), "export")}
          <div style="font-size:10px; color:var(--fg-3); margin-top:7px;">
            <i class="fa-solid fa-shield-halved" style="font-size:9px;"></i>
            Treat this token like a password — it authenticates into your hub.
          </div>
        </div>

        <div style="font-size:10.5px; color:var(--fg-3); border-top:1px solid rgba(255,255,255,0.06); padding-top:12px;">
          MCP endpoint <code style="font-family:var(--font-mono); color:var(--fg-2);">${this.recipe.mcp_url}</code>
          · marketplace <code style="font-family:var(--font-mono); color:var(--fg-2);">${this.recipe.marketplace_url}</code>
        </div>
      `;
    }

    const body = html`
      <div style="flex:1; padding:18px 22px 22px; overflow:auto; max-width:680px;">
        <div style="display:flex; flex-direction:column; gap:18px;">
          ${intro}
          ${inner}
        </div>
      </div>
    `;

    return renderChrome({
      title: "Connect Claude Code",
      subtitle: "attach your editor to this machine's CoreAgent hub",
      w: this.w, h: this.h,
      toolbar: nothing,
      body,
      footer: html`bearer from pkg/keys tier-0 · read-only · Lethean never writes your ~/.claude config`,
      embedded: this.embedded,
    });
  }
}

customElements.define("lthn-view-agent-connect", LthnViewAgentConnect);

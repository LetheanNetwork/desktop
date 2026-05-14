// SPDX-Licence-Identifier: EUPL-1.2
// E4.3 · fleet — <lthn-fleet-window>
// Light-DOM Lit element. Composes renderChrome() from ../chrome.js.
//
// Two contexts at the top level:
//   - Machines : compute fleet (hardware nodes) — pair, route, monitor
//   - Agents   : configured Team Members (provider + persona + features)
//                that run against the machine fleet
//
// Each tab has its own empty state with a centred CTA so a fresh user
// is never staring at an empty grid wondering what to do. The CTAs
// dispatch a window-level event ("lthn:fleet:configure-agent" /
// "lthn:fleet:pair-machine") that the future modal/wizard wiring
// listens for; until that lands the buttons are visible but no-op.

import { LitElement, html, nothing } from "lit";
import { renderChrome } from "../chrome";
import { T } from "@lthn/i18n/coreservice";
import { Machines, Queue, RoutingRules, Agents, DeleteAgent, DeleteMachine } from "@desktop/fleet/service";
import type * as fleetModels from "@desktop/fleet/models";
import { unwrap } from "../result";
import "./configure-agent-modal";
import "./pair-machine-modal";

type MachineRow = fleetModels.Machine;
type QueueRow = fleetModels.QueueRow;
type RuleRow = fleetModels.RoutingRule;
type AgentRow = fleetModels.Agent;

type Tab = "machines" | "agents";

class LthnFleetWindow extends LitElement {
  static readonly properties = {
    w: { type: Number },
    h: { type: Number },
    embedded: { type: Boolean, reflect: true },
    chrome: { state: true },
    tab: { state: true },
    machines: { state: true },
    agents: { state: true },
    queue: { state: true },
    rules: { state: true },
    err: { state: true },
    configureAgentOpen: { state: true },
    pairMachineOpen: { state: true },
    editingAgent: { state: true },
    editingMachine: { state: true },
  };
  declare w: number;
  declare h: number;
  declare embedded: boolean;
  declare chrome: { title: string; subtitle: string };
  declare tab: Tab;
  declare machines: MachineRow[];
  declare agents: AgentRow[];
  declare queue: QueueRow[];
  declare rules: RuleRow[];
  declare err: string | null;
  declare configureAgentOpen: boolean;
  declare pairMachineOpen: boolean;
  declare editingAgent: AgentRow | null;
  declare editingMachine: MachineRow | null;

  /* Poll interval id — cleared on disconnect. */
  private pollId: number | null = null;
  private static readonly POLL_MS = 2000;

  constructor() {
    super();
    this.w = 1080; this.h = 700; this.embedded = false;
    this.chrome = { title: "Fleet", subtitle: "machines + agents" };
    this.tab = "agents";
    this.machines = [];
    this.agents = [];
    this.queue = [];
    this.rules = [];
    this.err = null;
    this.configureAgentOpen = false;
    this.pairMachineOpen = false;
    this.editingAgent = null;
    this.editingMachine = null;
  }
  createRenderRoot() { return this; }
  async connectedCallback() {
    super.connectedCallback();
    const [title, subtitle] = await Promise.all([
      T("window.fleet.title"),
      T("window.fleet.subtitle"),
    ]);
    this.chrome = { title, subtitle };
    await this.poll();
    this.pollId = window.setInterval(() => { void this.poll(); }, LthnFleetWindow.POLL_MS);
  }

  disconnectedCallback() {
    if (this.pollId !== null) {
      window.clearInterval(this.pollId);
      this.pollId = null;
    }
    super.disconnectedCallback();
  }

  private async poll() {
    try {
      const [mR, agR, qR, rR] = await Promise.all([
        Machines(),
        Agents(),
        Queue(20),
        RoutingRules(),
      ]);
      this.machines = (mR.OK ? (mR.Value as MachineRow[]) : []) ?? [];
      this.agents   = (agR.OK ? (agR.Value as AgentRow[]) : []) ?? [];
      this.queue    = (qR.OK ? (qR.Value as QueueRow[]) : []) ?? [];
      this.rules    = (rR.OK ? (rR.Value as RuleRow[]) : []) ?? [];
      this.err = mR.OK ? null : "fleet: machines query failed";
    } catch (e: unknown) {
      this.err = e instanceof Error ? e.message : String(e);
    }
  }

  private machineNameOf(id: string): string {
    if (!id) return "—";
    const found = this.machines.find(m => m.id === id);
    return found?.name || id;
  }

  private static fmtStart(unixSec: number): string {
    if (!unixSec) return "—";
    const d = new Date(unixSec * 1000);
    const pad = (n: number) => String(n).padStart(2, "0");
    return `${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`;
  }

  private static fmtElapsed(unixSec: number): string {
    if (!unixSec) return "—";
    const ageMs = Date.now() - (unixSec * 1000);
    if (ageMs < 0) return "—";
    if (ageMs < 1000) return "<1 s";
    if (ageMs < 60000) return `${(ageMs / 1000).toFixed(1)} s`;
    return `${Math.trunc(ageMs / 60000)}m ${Math.trunc((ageMs % 60000) / 1000)}s`;
  }

  private selectTab(t: Tab) { this.tab = t; }

  private emitConfigureAgent() {
    this.editingAgent = null;
    this.configureAgentOpen = true;
    this.dispatchEvent(new CustomEvent("lthn:fleet:configure-agent", {
      bubbles: true, composed: true,
    }));
  }

  private editAgent(a: AgentRow) {
    this.editingAgent = a;
    this.configureAgentOpen = true;
  }

  /** Confirm + delete an agent. Also tries to drop the encrypted
   *  key blob, but failure there is non-fatal — the agents row
   *  going away is the load-bearing step. */
  private async deleteAgentRow(a: AgentRow) {
    const ok = window.confirm(`Delete agent "${a.name}"? This cannot be undone.`);
    if (!ok) return;
    const r = await DeleteAgent(a.id);
    if (!r.OK) {
      this.err = `Failed to delete agent: ${a.name}`;
      return;
    }
    await this.poll();
  }

  /** Modal callback — agent landed in the store. Force a fresh
   *  poll so the new row appears in the list without waiting for
   *  the 2s interval. */
  private async onAgentSaved() {
    this.configureAgentOpen = false;
    this.editingAgent = null;
    await this.poll();
  }

  private onConfigureCancel() {
    this.configureAgentOpen = false;
    this.editingAgent = null;
  }

  private emitPairMachine() {
    this.editingMachine = null;
    this.pairMachineOpen = true;
    this.dispatchEvent(new CustomEvent("lthn:fleet:pair-machine", {
      bubbles: true, composed: true,
    }));
  }

  private editMachine(m: MachineRow) {
    this.editingMachine = m;
    this.pairMachineOpen = true;
  }

  private async deleteMachineRow(m: MachineRow) {
    const ok = window.confirm(`Delete machine "${m.name}"? This cannot be undone.`);
    if (!ok) return;
    const r = await DeleteMachine(m.id);
    if (!r.OK) {
      this.err = `Failed to delete machine: ${m.name}`;
      return;
    }
    await this.poll();
  }

  private async onMachineSaved() {
    this.pairMachineOpen = false;
    this.editingMachine = null;
    await this.poll();
  }

  private onPairCancel() {
    this.pairMachineOpen = false;
    this.editingMachine = null;
  }

  /* Centred empty-state card. icon is a fontawesome glyph name
   * (without the "fa-" prefix). The card is the on-ramp — never
   * just a placeholder; always has a primary action. */
  private renderEmptyState(icon: string, headline: string, body: string, ctaLabel: string, onClick: () => void) {
    return html`
      <div style="flex:1; display:flex; align-items:center; justify-content:center; padding:32px;">
        <div style="max-width:380px; width:100%;
                    display:flex; flex-direction:column; align-items:center; gap:14px;
                    padding:32px 28px;
                    background:rgba(255,255,255,0.025);
                    border:1px solid rgba(255,255,255,0.06);
                    border-radius:12px;
                    text-align:center;">
          <div style="width:56px; height:56px;
                      display:flex; align-items:center; justify-content:center;
                      background:rgba(64,193,197,0.08);
                      border:1px solid rgba(64,193,197,0.18);
                      border-radius:14px;">
            <i class="fa-solid ${icon}" style="font-size:22px; color:var(--brand-300);"></i>
          </div>
          <div style="font-size:15px; font-weight:600; color:var(--fg-0);
                      letter-spacing:-0.005em;">
            ${headline}
          </div>
          <div style="font-size:12px; color:var(--fg-2); line-height:1.5;">
            ${body}
          </div>
          <lthn-btn tone="primary" size="md" @click=${onClick}>
            ${ctaLabel}
          </lthn-btn>
        </div>
      </div>
    `;
  }

  private renderMachineList() {
    return html`
      <div style="padding:0 22px; display:flex; flex-direction:column; gap:8px;">
        ${this.machines.map(m => this.renderMachineRow(m))}
      </div>
    `;
  }

  private renderMachineRow(m: MachineRow) {
    const offline = !m.status || m.status === "offline" || m.status.startsWith("offline");
    const bg = offline ? "rgba(255,255,255,0.015)"
             : m.is_self ? "rgba(64,193,197,0.06)"
             : "rgba(255,255,255,0.03)";
    const border = m.is_self ? "1px solid rgba(64,193,197,0.22)" : "1px solid rgba(255,255,255,0.05)";
    const opacity = offline ? 0.55 : 1;
    const loadBar = m.load_pct > 70 ? "var(--warning-400)" : "var(--brand-400)";
    return html`
      <div style="display:grid; grid-template-columns:16px 1.3fr 1.2fr 1.2fr 1fr 0.8fr 80px; gap:14px; padding:12px 16px; border-radius:8px; background:${bg}; border:${border}; opacity:${opacity}; align-items:center;">
        <i class="fa-solid fa-grip-vertical" style="font-size:11px; color:var(--fg-3);"></i>
        <div>
          <div style="display:flex; align-items:center; gap:8px; flex-wrap:wrap;">
            <lthn-status-dot variant=${offline ? "idle" : "ok"}></lthn-status-dot>
            <span style="font-size:13px; color:var(--fg-0); font-weight:500;">${m.name}</span>
            ${m.is_self ? html`<lthn-state-pill variant="latest">You</lthn-state-pill>` : nothing}
            ${(m.capabilities || []).map(cap => html`
              <span style="display:inline-flex; align-items:center; gap:4px;
                           padding:1px 6px; font-size:9.5px;
                           color:var(--brand-300);
                           background:rgba(64,193,197,0.06);
                           border:1px solid rgba(64,193,197,0.18);
                           border-radius:4px;
                           letter-spacing:0.02em;
                           text-transform:lowercase;">
                <i class="fa-solid ${cap === "inference" ? "fa-microchip" : cap === "sandbox" ? "fa-cubes" : cap === "bastion" ? "fa-shield-halved" : "fa-tag"}" style="font-size:8px;"></i>
                ${cap}
              </span>
            `)}
          </div>
          <div style="font-family:var(--font-mono); font-size:10.5px; color:var(--fg-3); margin-top:3px;">${m.arch || "—"}</div>
        </div>
        <div style="font-size:11.5px; color:${offline ? "var(--fg-3)" : "var(--fg-2)"};">${m.status || "—"}</div>
        <div style="font-family:var(--font-mono); font-size:11px; color:var(--fg-1);">${m.model || "—"}</div>
        <div>
          <div style="display:flex; justify-content:space-between; font-size:10px; color:var(--fg-3); margin-bottom:3px;">
            <span>load</span>
            <span style="font-family:var(--font-mono); color:var(--fg-1);">${m.load_pct}%</span>
          </div>
          <div style="height:4px; background:rgba(255,255,255,0.06); border-radius:2px; overflow:hidden;">
            <div style="width:${m.load_pct}%; height:100%; background:${loadBar};"></div>
          </div>
        </div>
        <div style="font-family:var(--font-mono); font-size:11px; color:var(--fg-1); text-align:right;">${(m.tps || 0).toFixed(1)} tok/s</div>
        <div style="display:flex; gap:4px; justify-content:flex-end;">
          <lthn-btn tone="quiet" size="sm" @click=${() => this.editMachine(m)} title="Edit machine">
            <i class="fa-solid fa-pen" style="font-size:10px;"></i>
          </lthn-btn>
          <lthn-btn tone="quiet" size="sm" @click=${() => { void this.deleteMachineRow(m); }} title="Delete machine">
            <i class="fa-solid fa-trash" style="font-size:10px;"></i>
          </lthn-btn>
        </div>
      </div>
    `;
  }

  private renderAgentList() {
    return html`
      <div style="padding:0 22px; display:flex; flex-direction:column; gap:8px;">
        ${this.agents.map(a => this.renderAgentRow(a))}
      </div>
    `;
  }

  private renderAgentRow(a: AgentRow) {
    const kindBadge = a.kind === "local" ? "local · fleet-routed" : "remote · provider-routed";
    return html`
      <div style="display:grid; grid-template-columns:36px 1.4fr 1.1fr 1.1fr 0.8fr 80px; gap:14px; padding:12px 16px; border-radius:8px;
                  background:rgba(255,255,255,0.03);
                  border:1px solid rgba(255,255,255,0.05);
                  align-items:center;">
        <div style="width:32px; height:32px;
                    display:flex; align-items:center; justify-content:center;
                    background:rgba(64,193,197,0.08);
                    border-radius:8px;">
          <i class="fa-solid fa-user-astronaut" style="font-size:13px; color:var(--brand-300);"></i>
        </div>
        <div>
          <div style="font-size:13px; color:var(--fg-0); font-weight:500;">${a.name}</div>
          <div style="font-family:var(--font-mono); font-size:10.5px; color:var(--fg-3); margin-top:3px;">
            ${a.provider} · ${a.model || "no model"}
          </div>
        </div>
        <div style="font-size:11.5px; color:var(--fg-2);">${kindBadge}</div>
        <div style="font-size:11.5px; color:var(--fg-2); overflow:hidden; text-overflow:ellipsis; white-space:nowrap;">
          ${a.persona || "—"}
        </div>
        <div>
          <lthn-state-pill variant=${a.status === "ready" ? "ok" : "idle"}>${a.status}</lthn-state-pill>
        </div>
        <div style="display:flex; gap:4px; justify-content:flex-end;">
          <lthn-btn tone="quiet" size="sm" @click=${() => this.editAgent(a)} title="Edit agent">
            <i class="fa-solid fa-pen" style="font-size:10px;"></i>
          </lthn-btn>
          <lthn-btn tone="quiet" size="sm" @click=${() => { void this.deleteAgentRow(a); }} title="Delete agent">
            <i class="fa-solid fa-trash" style="font-size:10px;"></i>
          </lthn-btn>
        </div>
      </div>
    `;
  }

  private renderQueueRail() {
    return html`
      <div style="padding:20px 22px 8px;">
        <lthn-label>Live queue</lthn-label>
      </div>
      <div style="margin:0 22px 22px; background:rgba(255,255,255,0.025); border:1px solid rgba(255,255,255,0.06); border-radius:8px; font-family:var(--font-mono); font-size:11.5px;">
        <div style="display:grid; grid-template-columns:100px 1fr 1fr 1fr 0.8fr 0.8fr; padding:10px 14px; border-bottom:1px solid rgba(255,255,255,0.05); color:var(--fg-3); font-size:10px; letter-spacing:0.04em; text-transform:uppercase;">
          <span>State</span><span>Caller</span><span>Model</span><span>Routed to</span><span>Started</span><span>Elapsed</span>
        </div>
        ${this.queue.length === 0
          ? html`<div style="padding:14px; color:var(--fg-3); font-size:11px; text-align:center;">queue is idle — no running agent activity</div>`
          : this.queue.map(q => html`
            <div style="display:grid; grid-template-columns:100px 1fr 1fr 1fr 0.8fr 0.8fr; padding:10px 14px; border-bottom:1px solid rgba(255,255,255,0.04); align-items:center;">
              <lthn-state-pill variant=${q.status || "idle"}>${q.status || "—"}</lthn-state-pill>
              <span style="color:var(--fg-1);">${q.caller || q.agent || "—"}</span>
              <span style="color:var(--fg-0);">${q.model || "—"}</span>
              <span style="color:var(--brand-300);">${this.machineNameOf(q.machine_id || "")}</span>
              <span style="color:var(--fg-2);">${LthnFleetWindow.fmtStart(q.started_at || 0)}</span>
              <span style="color:var(--fg-1);">${LthnFleetWindow.fmtElapsed(q.started_at || 0)}</span>
            </div>
          `)}
      </div>
    `;
  }

  private renderTabButton(id: Tab, label: string) {
    const on = this.tab === id;
    return html`
      <lthn-btn tone=${on ? "primary" : "ghost"} size="sm" ?active=${on}
        @click=${() => this.selectTab(id)}>
        ${label}
      </lthn-btn>
    `;
  }

  render() {
    const toolbar = html`
      ${this.renderTabButton("machines", `Machines${this.machines.length ? ` · ${this.machines.length}` : ""}`)}
      ${this.renderTabButton("agents", `Agents${this.agents.length ? ` · ${this.agents.length}` : ""}`)}
      <div style="flex:1"></div>
      <lthn-state-pill variant="preview">Preview · v1.0</lthn-state-pill>
    `;

    const sectionLabel = this.tab === "machines"
      ? html`<lthn-label>Machines · drag-reorder to set route priority</lthn-label>`
      : html`<lthn-label>Agents · configured Team Members</lthn-label>`;

    const showEmptyMachines = this.tab === "machines" && this.machines.length === 0;
    const showEmptyAgents   = this.tab === "agents"   && this.agents.length === 0;

    const tabBody = this.tab === "machines"
      ? (showEmptyMachines
          ? this.renderEmptyState(
              "fa-server",
              "No machines paired",
              "Add this Mac to the fleet, or pair another machine over the network. Once paired, agents can route inference to whichever node is best-suited for the model.",
              "Pair a machine",
              () => this.emitPairMachine(),
            )
          : this.renderMachineList())
      : (showEmptyAgents
          ? this.renderEmptyState(
              "fa-user-astronaut",
              "No agents configured",
              "An agent is a Team Member — endpoint + persona + features. Add a provider (OpenCode, Anthropic, Ollama, local Lemma) and the agent runs against your fleet.",
              "Configure an agent",
              () => this.emitConfigureAgent(),
            )
          : this.renderAgentList());

    const showFooterAction = (this.tab === "machines" && this.machines.length > 0)
                           || (this.tab === "agents"   && this.agents.length > 0);

    const body = html`
      <div style="flex:1; display:flex; flex-direction:column; min-height:0; overflow:auto;">
        ${(showEmptyMachines || showEmptyAgents)
          ? tabBody
          : html`
            <div style="padding:16px 22px 8px; display:flex; align-items:center; justify-content:space-between;">
              ${sectionLabel}
              ${showFooterAction
                ? html`<lthn-btn tone="ghost" size="sm"
                          @click=${this.tab === "agents" ? () => this.emitConfigureAgent() : () => this.emitPairMachine()}>
                          <i class="fa-solid fa-plus" style="font-size:10px;"></i>
                          ${this.tab === "agents" ? "Add agent" : "Pair machine"}
                        </lthn-btn>`
                : nothing}
            </div>
            ${tabBody}
            ${this.renderQueueRail()}
          `}
        ${this.err ? html`
          <div style="margin:0 22px 14px; padding:10px 14px;
                      background:rgba(244,67,54,0.06);
                      border:1px solid rgba(244,67,54,0.2);
                      border-radius:6px;
                      font-size:11px; color:var(--warning-400);">
            ${this.err}
          </div>
        ` : nothing}
      </div>
    `;

    const footer = this.tab === "machines"
      ? html`${this.machines.filter(m => m.status && m.status !== "offline").length} of ${this.machines.length} online`
      : html`${this.agents.length} agent${this.agents.length === 1 ? "" : "s"} configured`;

    const chrome = renderChrome({
      title: this.chrome.title, subtitle: this.chrome.subtitle,
      w: this.w, h: this.h, toolbar, body,
      footer,
      embedded: this.embedded,
    });
    return html`
      ${chrome}
      <lthn-configure-agent-modal
        .open=${this.configureAgentOpen}
        .editing=${this.editingAgent}
        @lthn:fleet:agent-saved=${() => { void this.onAgentSaved(); }}
        @lthn:fleet:configure-cancel=${() => this.onConfigureCancel()}>
      </lthn-configure-agent-modal>
      <lthn-pair-machine-modal
        .open=${this.pairMachineOpen}
        .editing=${this.editingMachine}
        @lthn:fleet:machine-saved=${() => { void this.onMachineSaved(); }}
        @lthn:fleet:pair-cancel=${() => this.onPairCancel()}>
      </lthn-pair-machine-modal>
    `;
  }
}
customElements.define("lthn-fleet-window", LthnFleetWindow);

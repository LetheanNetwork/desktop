// SPDX-Licence-Identifier: EUPL-1.2
// Coding view · Deploys — <lthn-view-deploys>
//
// Deploy history + live environment state. Green-field for v1 — fixtures
// only. Future: pkg/process + lthn-vm bundle-install path feed deploy
// activity. Tracked as a Mantis follow-up.
//
// Layout: top section = live environments (card grid), bottom section =
// recent deploy history (table). Mirrors the Claude Design reference.
//
// Supports the `embedded` attribute — no chrome when set.

import { LitElement, html } from "lit";
import { renderChrome } from "../../chrome";

/** One live deployment target. */
interface EnvRow {
  name:    string;
  url:     string;
  version: string;
  commit:  string;
  age:     string;
  health:  "ok" | "degraded" | "down";
}

/** One entry in the deploy history table. */
interface DeployRow {
  ts:      string;
  env:     string;
  by:      string;
  commit:  string;
  outcome: "success" | "rolled-back" | "failed";
  dur:     string;
}

/** Colour for the outcome label. */
function outcomeColour(outcome: DeployRow["outcome"]): string {
  switch (outcome) {
    case "success":     return "var(--success-400)";
    case "rolled-back": return "var(--warning-400)";
    case "failed":      return "var(--err-400)";
  }
}

class LthnViewDeploys extends LitElement {
  static readonly properties = {
    w:        { type: Number },
    h:        { type: Number },
    embedded: { type: Boolean, reflect: true },
    /** Live environments — fixture data until pkg/process bindings land. */
    envs:     { state: true },
    /** Recent deploy history — fixture data. */
    history:  { state: true },
  };

  declare w: number;
  declare h: number;
  declare embedded: boolean;
  declare envs: EnvRow[];
  declare history: DeployRow[];

  constructor() {
    super();
    this.w = 1180; this.h = 720; this.embedded = false;
    // Design-reference fixtures — identical to Claude Design mockup.
    this.envs = [
      { name: "production", url: "lthn.ai",         version: "v0.1.8",       commit: "4a82c1", age: "4d",  health: "ok" },
      { name: "staging",    url: "staging.lthn.ai",  version: "v0.2.0-rc3",   commit: "a3f12c", age: "2h",  health: "ok" },
      { name: "preview",    url: "preview.lthn.ai",  version: "v0.2.0-pr482", commit: "b8e034", age: "22m", health: "ok" },
    ];
    this.history = [
      { ts: "14:32", env: "preview",    by: "Tobi", commit: "b8e034", outcome: "success",     dur: "58s"     },
      { ts: "13:18", env: "preview",    by: "you",  commit: "7a221d", outcome: "success",     dur: "1m 04s"  },
      { ts: "10:42", env: "staging",    by: "you",  commit: "a3f12c", outcome: "success",     dur: "2m 18s"  },
      { ts: "yest",  env: "staging",    by: "Mei",  commit: "e1d99c", outcome: "rolled-back", dur: "1m 50s"  },
      { ts: "yest",  env: "production", by: "Mei",  commit: "4a82c1", outcome: "success",     dur: "4m 12s"  },
    ];
  }

  createRenderRoot() { return this; }

  /** Counts how many envs are healthy. */
  _healthSummary(): { ok: number; degraded: number } {
    return this.envs.reduce(
      (acc, e) => {
        if (e.health === "ok") acc.ok++;
        else acc.degraded++;
        return acc;
      },
      { ok: 0, degraded: 0 },
    );
  }

  render() {
    const health = this._healthSummary();

    const body = html`
      <div style="flex:1; display:flex; flex-direction:column; min-height:0; padding:20px 22px; gap:22px; overflow:auto;">
        <!-- live environments -->
        <div>
          <h3 style="margin:0 0 12px; font-family:var(--font-mono); font-size:11px; color:var(--fg-3);
                     letter-spacing:0.10em; text-transform:uppercase;">Live environments</h3>
          <div style="display:grid; grid-template-columns: repeat(3, 1fr); gap:12px;">
            ${this.envs.map(e => html`
              <div class="lthn-view-deploys-env"
                   data-env=${e.name}
                   style="padding:16px 18px; border-radius:10px;
                          background:rgba(255,255,255,0.025);
                          border:1px solid rgba(255,255,255,0.06);">
                <div style="display:flex; align-items:center; gap:8px;">
                  <lthn-status-dot variant=${e.health === "ok" ? "ok" : "error"}></lthn-status-dot>
                  <span style="font-size:14px; color:var(--fg-0); font-weight:500;">${e.name}</span>
                </div>
                <div style="font-family:var(--font-mono); font-size:11px; color:var(--brand-300); margin-top:10px;">${e.url}</div>
                <div style="display:flex; justify-content:space-between; margin-top:14px; padding-top:12px;
                            border-top:1px solid rgba(255,255,255,0.05);
                            font-family:var(--font-mono); font-size:10.5px; color:var(--fg-3);">
                  <span>${e.version}</span>
                  <span>${e.commit}</span>
                </div>
                <div style="font-family:var(--font-mono); font-size:10.5px; color:var(--fg-3); margin-top:4px;">
                  ${e.age} ago
                </div>
              </div>
            `)}
          </div>
        </div>

        <!-- recent deploy history -->
        <div>
          <h3 style="margin:0 0 12px; font-family:var(--font-mono); font-size:11px; color:var(--fg-3);
                     letter-spacing:0.10em; text-transform:uppercase;">Recent deploys</h3>
          <div style="background:rgba(255,255,255,0.025); border:1px solid rgba(255,255,255,0.06); border-radius:10px; overflow:hidden;">
            ${this.history.map((d, i) => html`
              <div class="lthn-view-deploys-row"
                   data-deploy=${i}
                   style="display:grid; grid-template-columns: 80px 120px 100px 110px 90px 80px; gap:14px;
                          padding:11px 16px;
                          border-bottom:${i < this.history.length - 1 ? "1px solid rgba(255,255,255,0.04)" : "none"};
                          font-family:var(--font-mono); font-size:11px; align-items:center;">
                <span style="color:var(--fg-3);">${d.ts}</span>
                <span style="color:var(--fg-0);">${d.env}</span>
                <span style="color:var(--fg-2);">${d.by}</span>
                <span style="color:var(--fg-3);">${d.commit}</span>
                <span style="color:${outcomeColour(d.outcome)};">${d.outcome}</span>
                <span style="color:var(--fg-3); text-align:right;">${d.dur}</span>
              </div>
            `)}
          </div>
        </div>
      </div>
    `;

    return renderChrome({
      title: "Deploys",
      subtitle: `${this.envs.length} envs · ${health.ok === this.envs.length ? "all green" : `${health.degraded} degraded`}`,
      w: this.w, h: this.h,
      body,
      footer: html`auto-deploy preview on PR open · staging on main · production on tag · fixtures (pkg/process bindings pending)`,
      embedded: this.embedded,
    });
  }
}

customElements.define("lthn-view-deploys", LthnViewDeploys);

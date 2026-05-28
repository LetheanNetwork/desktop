// SPDX-Licence-Identifier: EUPL-1.2
//
// <lthn-cred-migration-banner> — persistent banner that surfaces the
// "provider credential needs migration — unlock account to complete"
// state when runner's boot-scan detects plaintext provider keys still
// on disk after the at-rest substrate landed (Cerberus #71 ADD-4 /
// Mantis #1745).
//
// Wire shape mirrors <lthn-conflict-toast>:
//   - mounted ONCE in <lthn-app-shell> as a top-level overlay
//   - polls WCredentialMigrationStatus on mount + on a slow interval
//   - hides when PendingMigrationCount == 0
//
// Backend contract (pkg/runner/migration.go ScanMigrationStatus /
// WCredentialMigrationStatus):
//
//   type MigrationStatus = {
//     pending_migration_count: number;
//     plaintext_on_disk: boolean;
//     locked_routes: string[];
//   };
//
// Field names are snake_case — the Go struct (pkg/runner/types.go)
// carries explicit json tags, so that IS the wire shape. Reading
// PascalCase here yields undefined, which slips the render guard
// (`undefined <= 0` is false) and crashes on `.join`.
//
// Calm-voice copy per HANDOVER-VIEWS.md — observational, not alarming.
// Dark-only treatment per feedback_dark_only_design_system. Banner is
// non-dismissable (the underlying state persists until the user unlocks
// + migration completes; dismissing would hide an actionable security
// gap until next boot).
//
// Usage example (mount in app-shell once):
//
//   import "./lthn-cred-migration-banner";
//   // in render(): html`<lthn-cred-migration-banner></lthn-cred-migration-banner>`

import { LitElement, html, nothing } from "lit";

interface MigrationStatus {
  pending_migration_count: number;
  plaintext_on_disk: boolean;
  locked_routes: string[];
}

/** Refresh cadence — slow because the underlying signal only changes
 *  on unlock or next boot. 60s is the same interval the rest of the
 *  user-driven backend-wired views use per
 *  [[design_lit_view_backend_wire_pattern]]. */
const REFRESH_INTERVAL_MS = 60_000;

class LthnCredMigrationBanner extends LitElement {
  static readonly properties = {
    _status: { state: true },
  };

  declare _status: MigrationStatus | null;

  private _pollTimer: ReturnType<typeof setInterval> | null = null;

  /** createRenderRoot returns this so the banner inherits app-shell
   *  styles (matches the rest of the lthn-* component family — no
   *  shadow DOM, parent's tokens.css applies). */
  createRenderRoot() { return this; }

  connectedCallback() {
    super.connectedCallback();
    this._status = null;
    void this._refresh();
    this._pollTimer = setInterval(() => { void this._refresh(); }, REFRESH_INTERVAL_MS);
  }

  disconnectedCallback() {
    super.disconnectedCallback();
    if (this._pollTimer) {
      clearInterval(this._pollTimer);
      this._pollTimer = null;
    }
  }

  /** Pull the latest MigrationStatus from the backend. Backend not
   *  available (canvas / test environment) → leave _status null so the
   *  banner stays hidden. */
  private async _refresh(): Promise<void> {
    try {
      const mod = await import("@desktop/runner/service");
      const r = await mod.WCredentialMigrationStatus();
      if (r?.OK && r.Value) {
        this._status = r.Value as MigrationStatus;
      } else {
        this._status = null;
      }
    } catch {
      this._status = null;
    }
  }

  render() {
    if (!this._status || this._status.pending_migration_count <= 0) {
      return nothing;
    }
    const count = this._status.pending_migration_count;
    const providers = (this._status.locked_routes ?? []).join(", ") || "(unnamed)";
    return html`
      <div role="alert"
           aria-label="Provider credential migration pending"
           style="position:fixed; top:0; left:0; right:0;
                  z-index:90;
                  display:flex; gap:10px; align-items:flex-start;
                  padding:8px 14px;
                  background:rgba(212,140,40,0.18);
                  border-bottom:1px solid rgba(212,140,40,0.45);
                  font-size:11.5px; color:var(--fg-1);
                  --wails-draggable:no-drag;">
        <i class="fa-solid fa-key"
           style="font-size:13px; color:#e8a647; margin-top:1px;"></i>
        <div style="flex:1; line-height:1.5;">
          <span style="color:#e8a647; font-weight:600;">
            Provider credential needs migration
          </span>
          —
          ${count === 1
            ? html`<span>${providers} still holds its API key in plaintext config.</span>`
            : html`<span>${count} providers (${providers}) still hold their API keys in plaintext config.</span>`}
          <span style="color:var(--fg-3);">
            Unlock your account to complete the migration into encrypted storage.
          </span>
        </div>
      </div>
    `;
  }
}

if (!customElements.get("lthn-cred-migration-banner")) {
  customElements.define("lthn-cred-migration-banner", LthnCredMigrationBanner);
}

export { LthnCredMigrationBanner };

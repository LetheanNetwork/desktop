// SPDX-Licence-Identifier: EUPL-1.2
// E3.1 · integrations — <lthn-integrations-window>
// Light-DOM Lit element. Composes renderChrome() from ../chrome.js.

import { LitElement, html, nothing } from "lit";
import { renderChrome } from "../chrome";
import { T } from "@lthn/i18n/coreservice";

interface ClientView {
  id: string;
  name: string;
  description: string;
  config_path: string;
  config_path_raw: string;
  exists: boolean;
  state: string;
}

type SandboxState = "stopped" | "starting" | "ready" | "stopping" | "error";

/** Mantis #1630 — known sandbox image digests the user can pick from
 *  in the upgrade dialog. Backend (Cerberus #22 MED-2 / Mantis #1621)
 *  refuses pulls without an `image_digest` matching `sha256:<64 hex>`,
 *  so the frontend ships a hardcoded list rather than free-text entry.
 *
 *  TODO(Snider): replace the placeholder digest below with the real
 *  sha256 manifest digest of the current lthn/dev:latest image
 *  (`docker manifest inspect lthn/dev:latest | jq -r '.manifests[0].digest'`
 *  or equivalent) before ship. The all-zero digest is a deliberately
 *  obvious placeholder — it passes the sha256 regex but cannot match
 *  any real registry manifest, so the backend's post-pull divergence
 *  check at upgrade.go:214 will surface the mismatch loudly. */
const KNOWN_DIGESTS: ReadonlyArray<{ readonly label: string; readonly digest: string }> = [
  {
    // Label intentionally version-less so a stale "v2026.05.17" string
    // doesn't drift past the actual ship date. When the placeholder
    // digest gets replaced, swap this label for the matching
    // `vYYYY.MM.DD (current)` form alongside the real sha256.
    label:  "lthn/dev:latest (placeholder)",
    digest: "sha256:0000000000000000000000000000000000000000000000000000000000000000",
  },
];

class LthnIntegrationsWindow extends LitElement {
  static readonly properties = {
    w: { type: Number },
    h: { type: Number },
    embedded: { type: Boolean, reflect: true },
    chrome: { state: true },
    clients: { state: true },
    selectedId: { state: true },
    endpoint: { state: true },
    defaultModel: { state: true },
    apiKey: { state: true },
    sandboxState: { state: true },
    sandboxId: { state: true },
    sandboxCreatedAt: { state: true },
    mergeBusy: { state: true },
    mergeStatus: { state: true },
    busyStart: { state: true },
    busyImport: { state: true },
    importStatus: { state: true },
    studioInstalled: { state: true },
    upgradeBusy: { state: true },
    upgradeStatus: { state: true },
    showUpgradeDialog: { state: true },
    selectedDigest: { state: true },
    t: { state: true },
  };
  declare w: number;
  declare h: number;
  declare embedded: boolean;
  declare chrome: { title: string; subtitle: string };
  declare clients: ClientView[];
  declare selectedId: string;
  declare endpoint: string;
  declare defaultModel: string;
  declare apiKey: string;
  declare sandboxState: SandboxState;
  declare sandboxId: string;
  declare sandboxCreatedAt: string;
  declare mergeBusy: boolean;
  declare mergeStatus: { kind: "ok" | "err"; text: string } | null;
  declare busyStart: boolean;
  declare busyImport: boolean;
  declare importStatus: { kind: "ok" | "err"; text: string } | null;
  declare studioInstalled: boolean;
  declare upgradeBusy: boolean;
  declare upgradeStatus: { kind: "ok" | "err"; text: string } | null;
  /** Mantis #1623 — gate WUpgradeWithConsent behind a supply-chain
   *  warning dialog. False on construction; flips true on
   *  "Check for updates" click; flips back false on confirm/cancel. */
  declare showUpgradeDialog: boolean;
  /** Mantis #1630 — sha256 digest currently selected in the upgrade
   *  dialog. Initialised to KNOWN_DIGESTS[0].digest in the constructor
   *  so confirm has a usable value even if the user never touches the
   *  selector. */
  declare selectedDigest: string;
  declare t: {
    railLabel: string; railEmpty: string;
    rowConfigPath: string; rowOnDisk: string; rowEndpoint: string; rowDefaultModel: string;
    snippetLabel: string; snippetHelp: string;
    yes: string; no: string;
    noClient: string;
    ocSandboxLabel: string;
    ocStateStopped: string; ocStateStarting: string; ocStateReady: string; ocStateError: string;
    ocStart: string; ocStop: string;
    ocMergeLabel: string; ocCopy: string; ocMerge: string;
    ocMergedOk: string; ocMergedConflict: string; ocMergeForce: string;
    ocSnippetHelp: string;
    ocOpenTUI: string; ocOpenTUIFailed: string;
    ocOpenStudio: string;
    ocCheckUpdates: string; ocUpgrading: string;
    ocUpgradeNoChange: string; ocUpgradeUpdated: string;
    ocUpgradeFailed: string;
    ocUpgradeDialogTitle: string; ocUpgradeDialogBody: string;
    ocUpgradeDialogConfirm: string; ocUpgradeDialogCancel: string;
    ocUpgradeDialogDigestLabel: string; ocUpgradeDialogDigestSelected: string;
    ocOpenWeb: string; ocOpenWebFailed: string;
    ocImport: string; ocImporting: string;
    ocImportSummary: string;
    ocImportFailed: string;
  };
  /* Poll handle for sandbox state — cleared on disconnect. */
  private pollId: number | null = null;
  private static readonly POLL_MS = 4000;

  constructor() {
    super();
    this.w = 880; this.h = 660; this.embedded = false;
    this.chrome = { title: "Integrations", subtitle: "clients · MCP · webhooks" };
    this.clients = [];
    this.selectedId = "";
    this.endpoint = "http://localhost:8000/v1";
    this.defaultModel = "—";
    this.apiKey = "sk-lthn-•••• (managed by lthn)";
    this.sandboxState = "stopped";
    this.sandboxId = "";
    this.sandboxCreatedAt = "";
    this.mergeBusy = false;
    this.mergeStatus = null;
    this.busyStart = false;
    this.busyImport = false;
    this.importStatus = null;
    this.studioInstalled = false;
    this.upgradeBusy = false;
    this.upgradeStatus = null;
    this.showUpgradeDialog = false;
    this.selectedDigest = KNOWN_DIGESTS[0].digest;
    this.t = {
      railLabel: "Clients",
      railEmpty: "No clients enumerated yet. The integrations service is the source of truth.",
      rowConfigPath: "Config path", rowOnDisk: "On disk",
      rowEndpoint: "Endpoint", rowDefaultModel: "Default model",
      snippetLabel: "Config snippet · drop this into %s",
      snippetHelp:  "Only the apiBase, apiKey and model keys are lthn-managed. Anything else you set in this file is left alone.",
      yes: "yes", no: "no",
      noClient: "No client selected.",
      ocSandboxLabel: "Sandbox",
      ocStateStopped: "not running", ocStateStarting: "starting",
      ocStateReady: "running", ocStateError: "error",
      ocStart: "Start", ocStop: "Stop",
      ocMergeLabel: "Host integration · merges into %s",
      ocCopy: "Copy snippet", ocMerge: "Merge into config",
      ocMergedOk: "Merged. Restart opencode to pick up the lthn provider.",
      ocMergedConflict: "provider.lthn already exists with a different baseURL.",
      ocMergeForce: "Overwrite",
      ocSnippetHelp:
        "Adds the lthn provider alongside any others you have configured. Non-destructive — your existing providers are untouched.",
      ocOpenTUI: "Open in terminal",
      ocOpenTUIFailed: "Couldn't open the terminal — see logs for details.",
      ocOpenStudio: "Open desktop app",
      ocCheckUpdates: "Check for updates",
      ocUpgrading: "Checking…",
      ocUpgradeNoChange: "Already up to date.",
      ocUpgradeUpdated: "Updated. Sandbox restarted on the new image.",
      ocUpgradeFailed: "Update check failed — see logs for details.",
      ocUpgradeDialogTitle: "Pull a new sandbox image?",
      ocUpgradeDialogBody:
        "This pulls lthn/dev:latest from the registry. The image is NOT signature-verified yet — pulls trust whatever the registry currently serves. Any running sandboxes will keep using their current image until you stop and restart them.",
      ocUpgradeDialogConfirm: "Pull update",
      ocUpgradeDialogCancel: "Cancel",
      ocUpgradeDialogDigestLabel: "Image digest",
      ocUpgradeDialogDigestSelected: "Pinned: {digest}",
      ocOpenWeb: "Open in window",
      ocOpenWebFailed: "Couldn't open the web window — see logs for details.",
      ocImport: "Import from host opencode",
      ocImporting: "Importing…",
      ocImportSummary: "Imported %p providers · %j projects from host opencode.",
      ocImportFailed: "Import failed — is the opencode CLI installed + logged in on the host?",
    };
  }
  createRenderRoot() { return this; }

  disconnectedCallback() {
    if (this.pollId !== null) {
      window.clearInterval(this.pollId);
      this.pollId = null;
    }
    super.disconnectedCallback();
  }

  /** Refresh OpenCode sandbox state. Cheap, polls every POLL_MS while
   *  the OpenCode card is selected. */
  private async pollOpencode() {
    if (this.selectedId !== "opencode") return;
    try {
      const oc = await import("@desktop/opencode/wailsservice");
      const r = await oc.WStatus();
      // Result.OK + Result.Value: []Sandbox
      const ok = (r as any)?.OK === true;
      const list = ((r as any)?.Value || []) as Array<{ id: string; status: string; created_at?: string }>;
      if (!ok || list.length === 0) {
        // No running sandbox — only flip back to "stopped" when we're
        // not in a transient state we triggered (avoid clobbering
        // "starting" before the spawn record materialises).
        if (this.sandboxState !== "starting" && this.sandboxState !== "stopping") {
          this.sandboxState = "stopped";
          this.sandboxId = "";
          this.sandboxCreatedAt = "";
        }
        return;
      }
      const running = list[0];
      this.sandboxId = running.id;
      this.sandboxCreatedAt = running.created_at || "";
      this.sandboxState = running.status === "running" ? "ready" : "starting";
    } catch (err) {
      console.warn("integrations: opencode WStatus failed", err);
    }
  }

  private async startSandbox() {
    if (this.busyStart) return;
    this.busyStart = true;
    this.sandboxState = "starting";
    try {
      const oc = await import("@desktop/opencode/wailsservice");
      const r = await oc.WStart("");
      const ok = (r as any)?.OK === true;
      if (!ok) {
        this.sandboxState = "error";
        console.error("opencode WStart failed", r);
      } else {
        const id = (r as any)?.Value as string;
        this.sandboxId = id || "";
        this.sandboxState = "ready";
      }
    } catch (err) {
      this.sandboxState = "error";
      console.error("opencode WStart threw", err);
    } finally {
      this.busyStart = false;
      void this.pollOpencode();
    }
  }

  private async stopSandbox() {
    if (!this.sandboxId) return;
    const id = this.sandboxId;
    this.sandboxState = "stopping";
    try {
      const oc = await import("@desktop/opencode/wailsservice");
      const r = await oc.WStop(id);
      const ok = (r as any)?.OK === true;
      if (!ok) {
        console.error("opencode WStop failed", r);
      }
    } catch (err) {
      console.error("opencode WStop threw", err);
    } finally {
      this.sandboxId = "";
      this.sandboxCreatedAt = "";
      this.sandboxState = "stopped";
    }
  }

  /** Import the operator's host-side opencode credentials (~/.local/
   *  share/opencode/auth.json) + project list into the lthn orm, so a
   *  spawned sandbox opencode-serve can talk to the same providers
   *  the host CLI already authenticated against (your opencode
   *  subscription's free models + DeepSeek + paid tiers).
   *
   *  ImportFromHost is idempotent — re-running is safe + re-syncs
   *  any credentials that changed on the host side since the last
   *  import (e.g. you ran `opencode auth login` against a new
   *  provider). The running sandbox picks the new providers up on
   *  its next /provider refresh, which the runner already polls
   *  for the dynamic-route set. */
  private async importHost() {
    if (this.busyImport) return;
    this.busyImport = true;
    this.importStatus = null;
    try {
      const oc = await import("@desktop/opencode/wailsservice");
      const r = await oc.WImportFromHost();
      const ok = (r as { OK?: boolean })?.OK === true;
      if (!ok) {
        this.importStatus = { kind: "err", text: this.t.ocImportFailed };
        return;
      }
      const summary = (r as { Value?: { providers?: number; projects?: number } })?.Value;
      const providers = summary?.providers ?? 0;
      const projects = summary?.projects ?? 0;
      this.importStatus = {
        kind: "ok",
        text: this.t.ocImportSummary
          .replace("%p", String(providers))
          .replace("%j", String(projects)),
      };
    } catch (err) {
      console.error("opencode WImportFromHost threw", err);
      this.importStatus = { kind: "err", text: this.t.ocImportFailed };
    } finally {
      this.busyImport = false;
    }
  }

  private async mergeHostConfig(force: boolean) {
    if (this.mergeBusy) return;
    this.mergeBusy = true;
    this.mergeStatus = null;
    try {
      const oc = await import("@desktop/opencode/wailsservice");
      const r = await oc.WMergeHostConfig({ profile: "", force });
      const ok = (r as any)?.OK === true;
      if (ok) {
        this.mergeStatus = { kind: "ok", text: this.t.ocMergedOk };
      } else {
        const code = (r as any)?.Value?.Code as string | undefined;
        if (code === "opencode.host-config.conflict") {
          this.mergeStatus = { kind: "err", text: this.t.ocMergedConflict };
        } else {
          const msg = (r as any)?.Value?.Message || (r as any)?.Value?.message || "merge failed";
          this.mergeStatus = { kind: "err", text: String(msg) };
        }
      }
    } catch (err) {
      this.mergeStatus = { kind: "err", text: String(err) };
    } finally {
      this.mergeBusy = false;
    }
  }

  private async openWebWindow() {
    if (!this.sandboxId) return;
    try {
      const oc = await import("@desktop/opencode/wailsservice");
      const r = await oc.WOpenWebWindow(this.sandboxId);
      if ((r as any)?.OK !== true) {
        console.error("opencode WOpenWebWindow failed", r);
        this.mergeStatus = { kind: "err", text: this.t.ocOpenWebFailed };
      }
    } catch (err) {
      console.error("opencode WOpenWebWindow threw", err);
      this.mergeStatus = { kind: "err", text: this.t.ocOpenWebFailed };
    }
  }

  private async openTUI() {
    if (!this.sandboxId) return;
    try {
      const oc = await import("@desktop/opencode/wailsservice");
      const r = await oc.WOpenTUI(this.sandboxId);
      const ok = (r as any)?.OK === true;
      if (!ok) {
        console.error("opencode WOpenTUI failed", r);
        // Reuse the merge-status surface for transient TUI feedback —
        // single-error-line slot keeps the card density consistent.
        this.mergeStatus = { kind: "err", text: this.t.ocOpenTUIFailed };
      }
    } catch (err) {
      console.error("opencode WOpenTUI threw", err);
      this.mergeStatus = { kind: "err", text: this.t.ocOpenTUIFailed };
    }
  }

  /** JSONC snippet matching DefaultLthnProfile.Provider["lthn"]. Users
   *  see this in the card AND it's what Merge writes. Static for now;
   *  if the default profile gains baseURL knobs the snippet should
   *  derive from WGetProfile("default") instead. */
  private opencodeSnippet(): string {
    return `{
  "provider": {
    "lthn": {
      "npm":     "@ai-sdk/openai-compatible",
      "name":    "Lethean Local",
      "options": { "baseURL": "${this.endpoint}" },
      "models":  { "lthn-local": { "name": "Lethean Local" } }
    }
  }
}`;
  }

  private async copySnippet() {
    try { await navigator.clipboard?.writeText(this.opencodeSnippet()); }
    catch { /* clipboard unavailable — silent */ }
  }

  async connectedCallback() {
    super.connectedCallback();
    const [
      title, subtitle, rl, re, rcp, rod, rep, rdm, sl, sh, yes, no, nc,
      ocSL, ocSStop, ocSStart, ocSReady, ocSErr, ocStartL, ocStopL,
      ocML, ocCopy, ocMerge, ocMOk, ocMConflict, ocMForce, ocSHelp,
      ocTUI, ocTUIFail, ocStudio,
      ocCheck, ocUpgring, ocUpNoCh, ocUpUpd, ocUpFail,
      ocUpDlgT, ocUpDlgB, ocUpDlgC, ocUpDlgX,
      ocUpDlgDL, ocUpDlgDS,
      ocWeb, ocWebFail,
    ] = await Promise.all([
      T("window.integrations.title"),
      T("window.integrations.subtitle"),
      T("window.integrations.rail_label"),
      T("window.integrations.rail_empty"),
      T("window.integrations.row_config_path"),
      T("window.integrations.row_on_disk"),
      T("window.integrations.row_endpoint"),
      T("window.integrations.row_default_model"),
      T("window.integrations.snippet_label"),
      T("window.integrations.snippet_help"),
      T("window.integrations.yes"),
      T("window.integrations.no"),
      T("window.integrations.no_client"),
      T("window.integrations.oc_sandbox_label"),
      T("window.integrations.oc_state_stopped"),
      T("window.integrations.oc_state_starting"),
      T("window.integrations.oc_state_ready"),
      T("window.integrations.oc_state_error"),
      T("window.integrations.oc_start"),
      T("window.integrations.oc_stop"),
      T("window.integrations.oc_merge_label"),
      T("window.integrations.oc_copy"),
      T("window.integrations.oc_merge"),
      T("window.integrations.oc_merged_ok"),
      T("window.integrations.oc_merged_conflict"),
      T("window.integrations.oc_merge_force"),
      T("window.integrations.oc_snippet_help"),
      T("window.integrations.oc_open_tui"),
      T("window.integrations.oc_open_tui_failed"),
      T("window.integrations.oc_open_studio"),
      T("window.integrations.oc_check_updates"),
      T("window.integrations.oc_upgrading"),
      T("window.integrations.oc_upgrade_no_change"),
      T("window.integrations.oc_upgrade_updated"),
      T("window.integrations.oc_upgrade_failed"),
      T("window.integrations.oc_upgrade_dialog_title"),
      T("window.integrations.oc_upgrade_dialog_body"),
      T("window.integrations.oc_upgrade_dialog_confirm"),
      T("window.integrations.oc_upgrade_dialog_cancel"),
      T("window.integrations.oc_upgrade_dialog_digest_label"),
      T("window.integrations.oc_upgrade_dialog_digest_selected"),
      T("window.integrations.oc_open_web"),
      T("window.integrations.oc_open_web_failed"),
    ]);
    this.chrome = { title, subtitle };
    this.t = {
      railLabel: rl, railEmpty: re,
      rowConfigPath: rcp, rowOnDisk: rod, rowEndpoint: rep, rowDefaultModel: rdm,
      snippetLabel: sl, snippetHelp: sh,
      yes, no, noClient: nc,
      ocSandboxLabel: ocSL,
      ocStateStopped: ocSStop, ocStateStarting: ocSStart,
      ocStateReady: ocSReady, ocStateError: ocSErr,
      ocStart: ocStartL, ocStop: ocStopL,
      ocMergeLabel: ocML, ocCopy: ocCopy, ocMerge: ocMerge,
      ocMergedOk: ocMOk, ocMergedConflict: ocMConflict,
      ocMergeForce: ocMForce, ocSnippetHelp: ocSHelp,
      ocOpenTUI: ocTUI, ocOpenTUIFailed: ocTUIFail,
      ocOpenStudio: ocStudio,
      ocCheckUpdates: ocCheck, ocUpgrading: ocUpgring,
      ocUpgradeNoChange: ocUpNoCh, ocUpgradeUpdated: ocUpUpd,
      ocUpgradeFailed: ocUpFail,
      ocUpgradeDialogTitle: ocUpDlgT, ocUpgradeDialogBody: ocUpDlgB,
      ocUpgradeDialogConfirm: ocUpDlgC, ocUpgradeDialogCancel: ocUpDlgX,
      ocUpgradeDialogDigestLabel: ocUpDlgDL, ocUpgradeDialogDigestSelected: ocUpDlgDS,
      ocOpenWeb: ocWeb, ocOpenWebFailed: ocWebFail,
      // Import-from-host (Mantis #1775 follow-on): default copy
      // shipped inline. i18n keys can land later if these prove
      // load-bearing for the FR/ZH locales.
      ocImport: "Import from host opencode",
      ocImporting: "Importing…",
      ocImportSummary: "Imported %p providers · %j projects from host opencode.",
      ocImportFailed: "Import failed — is the opencode CLI installed + logged in on the host?",
    };
    try {
      const [integrations, runner, server, ak, resultMod] = await Promise.all([
        import("@desktop/integrations/wailsservice"),
        import("@desktop/runner/service"),
        import("@desktop/server/service"),
        import("@desktop/apikey/wailsservice"),
        import("../result"),
      ]);
      const { unwrap } = resultMod;
      const [list, models, addr, masked] = await Promise.all([
        integrations.List(),
        unwrap<string[]>(runner.WModels(), []),
        unwrap<string>(server.WAddr(), ""),
        unwrap<string>(ak.Masked(), ""),
      ]);
      this.clients = (list || []) as ClientView[];
      if (this.clients.length > 0) this.selectedId = this.clients[0].id;
      if (models && models.length > 0) this.defaultModel = models[0];
      if (addr) this.endpoint = endpointFromAddr(addr);
      if (masked) this.apiKey = masked;
    } catch (err) {
      console.error("integrations: lookup failed", err);
    }
    // Kick the opencode poll loop; updates() will gate per-tick on
    // whether opencode is currently selected.
    await this.pollOpencode();
    this.pollId = window.setInterval(() => { void this.pollOpencode(); }, LthnIntegrationsWindow.POLL_MS);
    // One-shot host-app detection — the install state doesn't
    // change mid-session frequently enough to justify polling.
    try {
      const oc = await import("@desktop/opencode/wailsservice");
      const r = await oc.WIsStudioInstalled();
      this.studioInstalled = (r as any)?.OK === true && (r as any)?.Value === true;
    } catch {
      this.studioInstalled = false;
    }
  }

  /** Mantis #1623 — "Check for updates" no longer fires the pull
   *  directly. It opens a supply-chain warning dialog. Confirm
   *  inside the dialog is what calls WUpgradeWithConsent; Cancel is
   *  a no-op that closes the dialog. */
  private openUpgradeDialog() {
    if (this.upgradeBusy) return;
    this.upgradeStatus = null;
    this.showUpgradeDialog = true;
  }

  /** User dismissed the supply-chain dialog. No backend call. */
  private cancelUpgradeDialog() {
    this.showUpgradeDialog = false;
  }

  /** User accepted the supply-chain warning. Fires the pull through
   *  the consent-required wrapper with ConfirmedByUser=true. We don't
   *  auto-restart running sandboxes — the user knows their sandbox
   *  state better than we do; the dialog body explains the new image
   *  takes effect on next sandbox start. */
  private async confirmUpgrade() {
    if (this.upgradeBusy) return;
    this.showUpgradeDialog = false;
    this.upgradeBusy = true;
    this.upgradeStatus = null;
    try {
      const oc = await import("@desktop/opencode/wailsservice");
      const r = await oc.WUpgradeWithConsent({
        confirmed_by_user: true,
        image_digest: this.selectedDigest,
        restart_sandboxes: false,
      });
      const ok = (r as any)?.OK === true;
      if (!ok) {
        this.upgradeStatus = { kind: "err", text: this.t.ocUpgradeFailed };
        console.error("opencode WUpgradeWithConsent failed", r);
      } else {
        const updated = !!(r as any)?.Value?.updated;
        this.upgradeStatus = {
          kind: "ok",
          text: updated ? this.t.ocUpgradeUpdated : this.t.ocUpgradeNoChange,
        };
      }
    } catch (err) {
      this.upgradeStatus = { kind: "err", text: this.t.ocUpgradeFailed };
      console.error("opencode WUpgradeWithConsent threw", err);
    } finally {
      this.upgradeBusy = false;
    }
  }

  private async openStudio() {
    try {
      const oc = await import("@desktop/opencode/wailsservice");
      const r = await oc.WOpenStudio();
      if ((r as any)?.OK !== true) {
        console.error("opencode WOpenStudio failed", r);
      }
    } catch (err) {
      console.error("opencode WOpenStudio threw", err);
    }
  }

  /** Reset transient merge feedback when the user navigates between
   *  clients so a previous attempt's "merged ok" doesn't bleed into
   *  the next client's view. */
  updated(changed: Map<string, unknown>) {
    if (changed.has("selectedId")) {
      this.mergeStatus = null;
      // Refresh sandbox state when (re-)entering the opencode card.
      if (this.selectedId === "opencode") {
        void this.pollOpencode();
      }
    }
  }

  _statusVariant(state: string) {
    if (state === "configured") return "ok";
    if (state === "available") return "warn";
    return "idle";
  }

  /** Stable pill variant for sandbox states — mirrors the existing
   *  state-pill design palette. */
  private _sandboxPillVariant(): string {
    switch (this.sandboxState) {
      case "ready":    return "ok";
      case "starting":
      case "stopping": return "preview";
      case "error":    return "warn";
      default:         return "idle";
    }
  }

  private _sandboxStateLabel(): string {
    switch (this.sandboxState) {
      case "ready":    return this.t.ocStateReady;
      case "starting": return this.t.ocStateStarting;
      case "stopping": return this.t.ocStateStarting;
      case "error":    return this.t.ocStateError;
      default:         return this.t.ocStateStopped;
    }
  }

  /** Bespoke main panel for the OpenCode client. T1 (host integration —
   *  snippet + Merge) + T2 (sandbox lifecycle — Start/Stop). T3 (Fleet
   *  provider list) lives in <lthn-fleet-window>. */
  private renderOpencodePanel(selected: ClientView) {
    const snippet = this.opencodeSnippet();
    const sandboxRunning = this.sandboxState === "ready";
    const sandboxBusy = this.sandboxState === "starting" || this.sandboxState === "stopping";
    return html`
      <div>
        <div style="display:flex; align-items:center; gap:12px;">
          <div style="font-size:20px; font-weight:600; color:var(--fg-0); letter-spacing:-0.015em;">${selected.name}</div>
          <lthn-state-pill variant=${selected.state === "configured" ? "connected" : "disconnected"}>${selected.state}</lthn-state-pill>
        </div>
        <div style="font-size:12.5px; color:var(--fg-2); margin-top:6px; max-width:540px; line-height:1.55;">${selected.description}</div>
      </div>

      <div>
        <lthn-label>${this.t.ocMergeLabel.replace("%s", selected.config_path_raw)}</lthn-label>
        <div style="margin-top:8px; background:rgba(0,0,0,0.30); border:1px solid rgba(255,255,255,0.06); border-radius:8px; padding:12px 14px; font-family:var(--font-mono); font-size:11.5px; line-height:1.6; color:var(--fg-1); white-space:pre;">${snippet}</div>
        <div style="display:flex; gap:8px; margin-top:10px; align-items:center;">
          <button
            @click=${() => void this.copySnippet()}
            style="padding:6px 12px; font-size:11.5px; background:rgba(255,255,255,0.04); border:1px solid rgba(255,255,255,0.10); border-radius:6px; color:var(--fg-0); cursor:pointer; --wails-draggable: no-drag;">
            ${this.t.ocCopy}
          </button>
          <button
            ?disabled=${this.mergeBusy}
            @click=${() => void this.mergeHostConfig(false)}
            style="padding:6px 12px; font-size:11.5px; background:rgba(64,193,197,0.12); border:1px solid rgba(64,193,197,0.40); border-radius:6px; color:var(--brand-100); cursor:${this.mergeBusy ? "default" : "pointer"}; opacity:${this.mergeBusy ? 0.6 : 1}; --wails-draggable: no-drag;">
            ${this.t.ocMerge}
          </button>
          ${this.mergeStatus ? html`
            <span style="font-size:11px; color:${this.mergeStatus.kind === "ok" ? "var(--ok-300, #6cb)" : "var(--warn-300, #d99)"}; line-height:1.4;">
              ${this.mergeStatus.text}
              ${this.mergeStatus.kind === "err" && this.mergeStatus.text === this.t.ocMergedConflict ? html`
                <button
                  ?disabled=${this.mergeBusy}
                  @click=${() => void this.mergeHostConfig(true)}
                  style="margin-left:6px; padding:4px 10px; font-size:11px; background:rgba(217,153,153,0.12); border:1px solid rgba(217,153,153,0.40); border-radius:5px; color:var(--warn-300, #d99); cursor:pointer; --wails-draggable: no-drag;">
                  ${this.t.ocMergeForce}
                </button>
              ` : nothing}
            </span>
          ` : nothing}
        </div>
        <div style="margin-top:10px; font-size:11px; color:var(--fg-3); line-height:1.55; max-width:540px;">
          ${this.t.ocSnippetHelp}
        </div>
      </div>

      <div>
        <div style="display:flex; align-items:center; justify-content:space-between;">
          <lthn-label>${this.t.ocSandboxLabel}</lthn-label>
          <div style="display:flex; align-items:center; gap:8px;">
            ${this.upgradeStatus ? html`
              <span style="font-size:10.5px; color:${this.upgradeStatus.kind === "ok" ? "var(--ok-300, #6cb)" : "var(--warn-300, #d99)"};">
                ${this.upgradeStatus.text}
              </span>
            ` : nothing}
            <button
              data-testid="oc-check-updates"
              ?disabled=${this.upgradeBusy}
              @click=${() => this.openUpgradeDialog()}
              style="padding:4px 10px; font-size:10.5px; background:transparent; border:1px solid rgba(255,255,255,0.10); border-radius:5px; color:var(--fg-2); cursor:${this.upgradeBusy ? "default" : "pointer"}; opacity:${this.upgradeBusy ? 0.6 : 1}; --wails-draggable: no-drag;">
              ${this.upgradeBusy ? this.t.ocUpgrading : this.t.ocCheckUpdates}
            </button>
          </div>
        </div>
        <div style="margin-top:8px; padding:14px 16px; border-radius:8px; background:rgba(255,255,255,0.025); border:1px solid rgba(255,255,255,0.05); display:flex; flex-direction:column; gap:10px;">
          <div style="display:flex; align-items:center; gap:10px;">
            <lthn-status-dot variant=${this._sandboxPillVariant()}></lthn-status-dot>
            <span style="font-size:12.5px; color:var(--fg-0);">${this._sandboxStateLabel()}</span>
            ${this.sandboxId ? html`
              <span style="font-family:var(--font-mono); font-size:10.5px; color:var(--fg-3);">${this.sandboxId}</span>
            ` : nothing}
          </div>
          <div style="display:flex; gap:8px; flex-wrap:wrap;">
            ${sandboxRunning ? html`
              <button
                ?disabled=${sandboxBusy}
                @click=${() => void this.stopSandbox()}
                style="padding:6px 12px; font-size:11.5px; background:rgba(217,153,153,0.10); border:1px solid rgba(217,153,153,0.35); border-radius:6px; color:var(--warn-300, #d99); cursor:${sandboxBusy ? "default" : "pointer"}; opacity:${sandboxBusy ? 0.6 : 1}; --wails-draggable: no-drag;">
                ${this.t.ocStop}
              </button>
              <button
                ?disabled=${sandboxBusy}
                @click=${() => void this.openWebWindow()}
                style="padding:6px 12px; font-size:11.5px; background:rgba(255,255,255,0.04); border:1px solid rgba(255,255,255,0.10); border-radius:6px; color:var(--fg-0); cursor:${sandboxBusy ? "default" : "pointer"}; opacity:${sandboxBusy ? 0.6 : 1}; --wails-draggable: no-drag;">
                <i class="fa-solid fa-window-maximize" style="font-size:10px; margin-right:6px;"></i>${this.t.ocOpenWeb}
              </button>
              <button
                ?disabled=${sandboxBusy}
                @click=${() => void this.openTUI()}
                style="padding:6px 12px; font-size:11.5px; background:rgba(255,255,255,0.04); border:1px solid rgba(255,255,255,0.10); border-radius:6px; color:var(--fg-0); cursor:${sandboxBusy ? "default" : "pointer"}; opacity:${sandboxBusy ? 0.6 : 1}; --wails-draggable: no-drag;">
                <i class="fa-solid fa-terminal" style="font-size:10px; margin-right:6px;"></i>${this.t.ocOpenTUI}
              </button>
            ` : html`
              <button
                ?disabled=${sandboxBusy || this.busyStart}
                @click=${() => void this.startSandbox()}
                style="padding:6px 12px; font-size:11.5px; background:rgba(64,193,197,0.12); border:1px solid rgba(64,193,197,0.40); border-radius:6px; color:var(--brand-100); cursor:${sandboxBusy ? "default" : "pointer"}; opacity:${sandboxBusy ? 0.6 : 1}; --wails-draggable: no-drag;">
                ${this.t.ocStart}
              </button>
            `}
            ${this.studioInstalled ? html`
              <button
                @click=${() => void this.openStudio()}
                style="padding:6px 12px; font-size:11.5px; background:rgba(255,255,255,0.04); border:1px solid rgba(255,255,255,0.10); border-radius:6px; color:var(--fg-0); cursor:pointer; --wails-draggable: no-drag;">
                <i class="fa-solid fa-arrow-up-right-from-square" style="font-size:10px; margin-right:6px;"></i>${this.t.ocOpenStudio}
              </button>
            ` : nothing}
            <button
              data-testid="oc-import-host"
              ?disabled=${this.busyImport}
              @click=${() => void this.importHost()}
              style="padding:6px 12px; font-size:11.5px; background:rgba(255,255,255,0.04); border:1px solid rgba(255,255,255,0.10); border-radius:6px; color:var(--fg-0); cursor:${this.busyImport ? "default" : "pointer"}; opacity:${this.busyImport ? 0.6 : 1}; --wails-draggable: no-drag;">
              <i class="fa-solid ${this.busyImport ? "fa-spinner fa-spin" : "fa-file-import"}" style="font-size:10px; margin-right:6px;"></i>
              ${this.busyImport ? this.t.ocImporting : this.t.ocImport}
            </button>
          </div>
          ${this.importStatus ? html`
            <div
              data-testid="oc-import-status"
              style="padding:6px 8px; border-radius:5px; font-size:11px;
                     background:${this.importStatus.kind === "ok" ? "rgba(64,193,197,0.08)" : "rgba(248,113,113,0.06)"};
                     border:1px solid ${this.importStatus.kind === "ok" ? "rgba(64,193,197,0.20)" : "rgba(248,113,113,0.18)"};
                     color:${this.importStatus.kind === "ok" ? "var(--brand-300)" : "var(--error-400)"};">
              ${this.importStatus.text}
            </div>
          ` : nothing}
        </div>
      </div>
      ${this.showUpgradeDialog ? this.renderUpgradeDialog() : nothing}
    `;
  }

  /** Mantis #1623 — supply-chain warning dialog. Inline scrim+card
   *  (no shared lthn-confirm primitive yet — see brief escape valve).
   *  Dark-only palette mirrors the rest of the surface. Confirm fires
   *  WUpgradeWithConsent({confirmed_by_user: true}); Cancel closes the
   *  dialog with no side effects. */
  private renderUpgradeDialog() {
    return html`
      <div
        data-testid="oc-upgrade-dialog"
        role="dialog"
        aria-modal="true"
        aria-label=${this.t.ocUpgradeDialogTitle}
        style="position:fixed; inset:0; z-index:50; display:flex; align-items:center; justify-content:center; background:rgba(0,0,0,0.55); --wails-draggable: no-drag;">
        <div
          style="max-width:480px; width:calc(100% - 64px); background:var(--bg-1, #16181c); border:1px solid rgba(255,255,255,0.10); border-radius:10px; padding:22px 24px; box-shadow:0 12px 48px rgba(0,0,0,0.55); display:flex; flex-direction:column; gap:14px;">
          <div style="display:flex; align-items:center; gap:10px;">
            <i class="fa-solid fa-triangle-exclamation" style="font-size:14px; color:var(--warn-300, #d99);"></i>
            <div style="font-size:14.5px; font-weight:600; color:var(--fg-0); letter-spacing:-0.01em;">
              ${this.t.ocUpgradeDialogTitle}
            </div>
          </div>
          <div style="font-size:12.5px; color:var(--fg-2); line-height:1.6;">
            ${this.t.ocUpgradeDialogBody}
          </div>
          <div style="display:flex; flex-direction:column; gap:6px;">
            <label
              for="oc-upgrade-digest"
              style="font-size:10.5px; color:var(--fg-3); letter-spacing:0.04em; text-transform:uppercase;">
              ${this.t.ocUpgradeDialogDigestLabel}
            </label>
            <select
              id="oc-upgrade-digest"
              data-testid="oc-upgrade-digest-select"
              .value=${this.selectedDigest}
              @change=${(ev: Event) => { this.selectedDigest = (ev.target as HTMLSelectElement).value; }}
              style="padding:6px 10px; font-size:12px; background:rgba(0,0,0,0.30); border:1px solid rgba(255,255,255,0.10); border-radius:6px; color:var(--fg-0); --wails-draggable: no-drag;">
              ${KNOWN_DIGESTS.map(d => html`
                <option value=${d.digest} ?selected=${d.digest === this.selectedDigest}>${d.label}</option>
              `)}
            </select>
            <div
              data-testid="oc-upgrade-digest-preview"
              style="font-family:var(--font-mono); font-size:10.5px; color:var(--fg-3); line-height:1.5;">
              ${this.t.ocUpgradeDialogDigestSelected.replace("{digest}", truncateDigest(this.selectedDigest))}
            </div>
          </div>
          <div style="display:flex; justify-content:flex-end; gap:8px; margin-top:4px;">
            <button
              data-testid="oc-upgrade-cancel"
              @click=${() => this.cancelUpgradeDialog()}
              style="padding:6px 14px; font-size:11.5px; background:transparent; border:1px solid rgba(255,255,255,0.10); border-radius:6px; color:var(--fg-1); cursor:pointer; --wails-draggable: no-drag;">
              ${this.t.ocUpgradeDialogCancel}
            </button>
            <button
              data-testid="oc-upgrade-confirm"
              @click=${() => void this.confirmUpgrade()}
              style="padding:6px 14px; font-size:11.5px; background:rgba(217,153,153,0.14); border:1px solid rgba(217,153,153,0.45); border-radius:6px; color:var(--warn-300, #d99); cursor:pointer; --wails-draggable: no-drag;">
              ${this.t.ocUpgradeDialogConfirm}
            </button>
          </div>
        </div>
      </div>
    `;
  }

  /** Generic snippet panel for non-OpenCode clients — preserves the
   *  original OpenAI-compat snippet UX. */
  private renderGenericPanel(selected: ClientView) {
    return html`
      <div>
        <div style="display:flex; align-items:center; gap:12px;">
          <div style="font-size:20px; font-weight:600; color:var(--fg-0); letter-spacing:-0.015em;">${selected.name}</div>
          <lthn-state-pill variant=${selected.state === "configured" ? "connected" : "disconnected"}>${selected.state}</lthn-state-pill>
        </div>
        <div style="font-size:12.5px; color:var(--fg-2); margin-top:6px; max-width:460px; line-height:1.55;">${selected.description}</div>
      </div>
      <div style="display:grid; grid-template-columns:1fr 1fr; gap:10px;">
        ${[
          { k: this.t.rowConfigPath,   v: selected.config_path_raw },
          { k: this.t.rowOnDisk,       v: selected.exists ? this.t.yes : this.t.no },
          { k: this.t.rowEndpoint,     v: this.endpoint },
          { k: this.t.rowDefaultModel, v: this.defaultModel },
        ].map(row => html`
          <div style="padding:10px 14px; border-radius:6px; background:rgba(255,255,255,0.025); border:1px solid rgba(255,255,255,0.05);">
            <div style="font-size:10.5px; color:var(--fg-3); letter-spacing:0.04em; text-transform:uppercase;">${row.k}</div>
            <div style="font-family:var(--font-mono); font-size:12.5px; color:var(--fg-0); margin-top:4px;">${row.v}</div>
          </div>
        `)}
      </div>
      <div>
        <lthn-label>${this.t.snippetLabel.replace("%s", selected.config_path_raw)}</lthn-label>
        <div style="margin-top:8px; background:rgba(0,0,0,0.30); border:1px solid rgba(255,255,255,0.06); border-radius:8px; padding:12px 14px; font-family:var(--font-mono); font-size:11.5px; line-height:1.6; color:var(--fg-1); white-space:pre;">${`{
  "apiBase":  "${this.endpoint}",
  "apiKey":   "${this.apiKey}",
  "model":    "${this.defaultModel}",
  "stream":   true
}`}</div>
        <div style="margin-top:10px; font-size:11px; color:var(--fg-3); line-height:1.55;">
          ${this.t.snippetHelp}
        </div>
      </div>
    `;
  }

  render() {
    const clients = this.clients;
    const selected = clients.find(c => c.id === this.selectedId) || clients[0] || null;

    const body = html`
      <div style="flex:1; display:grid; grid-template-columns:260px 1fr; min-height:0;">
        <aside style="background:rgba(0,0,0,0.18); border-right:1px solid rgba(255,255,255,0.05); padding:12px 8px; display:flex; flex-direction:column; gap:1px;">
          <lthn-label style="padding:4px 10px 8px;">${this.t.railLabel}</lthn-label>
          ${clients.length === 0 ? html`
            <div style="padding:14px 12px; font-size:11.5px; color:var(--fg-3); line-height:1.55;">
              ${this.t.railEmpty}
            </div>
          ` : clients.map((c) => {
            const active = c.id === this.selectedId;
            const variant = this._statusVariant(c.state);
            return html`
              <div
                @click=${() => { this.selectedId = c.id; }}
                style="padding:10px 12px; border-radius:6px; background:${active ? "rgba(255,255,255,0.06)" : "transparent"}; border-left:${active ? "2px solid var(--brand-400)" : "2px solid transparent"}; display:flex; flex-direction:column; gap:4px; cursor:pointer; --wails-draggable: no-drag;">
                <div style="display:flex; align-items:center; gap:8px;">
                  <lthn-status-dot variant=${variant}></lthn-status-dot>
                  <span style="font-size:12.5px; color:var(--fg-0); font-weight:500;">${c.name}</span>
                </div>
                <div style="font-size:10.5px; color:var(--fg-3); margin-left:15px;">${c.state}</div>
              </div>
            `;
          })}
        </aside>
        <main style="padding:24px 32px; overflow:auto; display:flex; flex-direction:column; gap:22px;">
          ${selected ? (selected.id === "opencode"
            ? this.renderOpencodePanel(selected)
            : this.renderGenericPanel(selected)
          ) : html`
            <div style="font-size:13px; color:var(--fg-3); padding:24px 0;">${this.t.noClient}</div>
          `}
        </main>
      </div>
    `;
    const configured = clients.filter(c => c.state === "configured").length;
    return renderChrome({
      title: this.chrome.title, subtitle: this.chrome.subtitle,
      w: this.w, h: this.h, body,
      footer: html`${configured} configured · ${clients.length} known · endpoint ${this.endpoint} · only outbound action lthn ever takes`,
      embedded: this.embedded,
    });
  }
}
customElements.define("lthn-integrations-window", LthnIntegrationsWindow);

/** Compose the OpenAI-compatible endpoint URL from the server's raw
 *  bind address. ":8000" → "http://localhost:8000/v1"; an explicit
 *  host already in the addr ("127.0.0.1:8000") passes through. */
function endpointFromAddr(addr: string): string {
  if (!addr) return "http://localhost:8000/v1";
  const a = addr.startsWith(":") ? `localhost${addr}` : addr;
  return `http://${a}/v1`;
}

/** Shorten a `sha256:<64 hex>` digest to `sha256:abc…def` for preview
 *  text in the upgrade dialog. Anything not matching the expected
 *  shape is returned verbatim so a malformed value stays visible
 *  rather than silently collapsing to an ellipsis. */
function truncateDigest(digest: string): string {
  const m = /^sha256:([0-9a-f]{64})$/i.exec(digest);
  if (!m) return digest;
  return `sha256:${m[1].slice(0, 6)}…${m[1].slice(-6)}`;
}

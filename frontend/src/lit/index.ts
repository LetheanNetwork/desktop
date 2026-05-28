// SPDX-Licence-Identifier: EUPL-1.2
// Lethean Desktop — Lit element registry
//
// Single import for the surface router (frontend/src/main.js):
//   import './lit/index'
// Side-effect imports register every custom element. The named
// renderChrome export is for callers that compose a window inline
// (e.g. the tray popover route).

export { renderChrome } from "./chrome";
import "./chrome";

import "./chat/chat-window";
import "./ops/welcome-window";
import "./ops/settings-window";
import "./ops/model-browser-window";
import "./ops/process-window";
import "./obs/benchmark-window";
import "./obs/logs-window";
import "./obs/telemetry-window";
// Audit-events viewer per plans/code/lthn/desktop/auth-gate/
// RFC.stage-e-audit-viewer.md v2 (Mantis #1612 E.C.C). Registers
// <lthn-audit-viewer>. Sibling of logs/telemetry; renders the
// forensic readout the operator reaches for via Activity → Audit.
import "./obs/audit-window";
import "./ext/integrations-window";
import "./ext/tools-window";
import "./ext/network-window";
import "./ext/distillation-window";
import "./ext/training-window";
import "./ext/fleet-window";
import "./ext/providers-window";

// Marketing surface views (Phase 2 wave 1) — registers
// <lthn-view-campaigns|content|social|audience|analytics>.
import "./views/marketing";

// Office surface views (Phase 2 wave 1) — registers
// <lthn-view-documents|mail|files>. Calendar is intentionally not
// re-registered here; it's owned by the Planning view and shared.
import "./views/office";

// Agents role view (CoreAgent surface) — registers
// <lthn-view-agent-activity>. Spec: code/lthn/desktop/views/RFC.agents-view.md
import "./views/agents";

// Sales role views (Phase 2 wave 1) — registers
// <lthn-view-pipeline|contacts|deals|forecast>. Fixtures-only in v1;
// future pkg/sales/* Go bindings replace the per-element fixture arrays.
import "./views/sales";

// Operations role views (Phase 2 wave 1) — registers
// <lthn-view-status|incidents|runbooks>. Status reads live data
// from vi.Sites (#363); incidents + runbooks fixture-only until
// pkg/incidents + pkg/runbooks land.
import "./views/operations";

// Planning role views (Phase 2 wave 1) — registers
// <lthn-view-today|sprints|backlog|roadmap|retros|calendar>.
// Backed by core/ide pkg/tasks (already adopted in lthn via #359);
// fixtures-only today, _loadFromBackend() seam awaits a Wails
// surface for go/pkg/tasks (no binding yet at module-write time).
// Calendar is shared with the Office view per HANDOVER-VIEWS.md.
import "./views/planning";

// First-run + 401 auth-gate — registers <lthn-auth-gate>. Mounted by
// <lthn-app-shell> ahead of the normal body slot whenever authState
// is anything other than "ok" (Stage C of plans/code/lthn/desktop/
// auth-gate/RFC.md).
import "./auth-gate";

// Lethean-6 application shell — frameless single-window chrome that
// auto-mounts the matching <lthn-*-window> by `active` attribute.
import "./app-shell";

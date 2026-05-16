// SPDX-Licence-Identifier: EUPL-1.2

// Side-effect import — registers the three Operations view elements
// with customElements. Import from `frontend/src/lit/index.ts` to
// mount automatically on app shell load.
//
// Operations · ops surface from HANDOVER-VIEWS.md:
//   <lthn-view-status>     — live uptime via vi.Sites (#363)
//   <lthn-view-incidents>  — incident log (v1 fixture; pkg/incidents planned)
//   <lthn-view-runbooks>   — runbook library (v1 fixture; pkg/runbooks planned)
//
// Logs + Telemetry are reused from the Admin view's existing
// <lthn-logs-window> + <lthn-telemetry-window> per HANDOVER-VIEWS.md.

import "./status";
import "./incidents";
import "./runbooks";

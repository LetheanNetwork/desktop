// SPDX-Licence-Identifier: EUPL-1.2
// Coding view — element registry
//
// Side-effect import: registers all 4 custom elements for the Coding role.
// Import via:  import "@/lit/views/coding"
//
// Elements registered:
//   <lthn-view-repos>   — repo list, build state, PR count (fixtures v1)
//   <lthn-view-issues>  — filterable issue tracker (fixtures v1)
//   <lthn-view-prs>     — PR review queue, wired to Vi.Activity()
//   <lthn-view-deploys> — deploy history + env state (fixtures v1)

import "./repos";
import "./issues";
import "./prs";
import "./deploys";

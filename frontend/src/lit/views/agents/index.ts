// SPDX-Licence-Identifier: EUPL-1.2
// Agents view — element registry
//
// The Agents view is CoreAgent's surface — the harness-agnostic
// orchestration layer above the fleet: dispatch work across whatever the
// fleet offers and watch it run. Spec: code/lthn/desktop/views/RFC.agents-view.md
//
// Side-effect import: registers the Agents-role custom elements.
// Import via:  import "@/lit/views/agents"
//
// Elements registered:
//   <lthn-view-agent-activity>           — live dispatch feed, wired to Fleet.Activity()
//   <lthn-view-repos|issues|prs|deploys> — repo context, reused from the former Coding view

import "./activity";

// Pulled-in Coding panels — repos/issues/prs/deploys are repo context
// (what the fleet works on), reused here rather than discarded.
import "../coding";

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
//   <lthn-view-agent-activity>           — live fleet pulse, wired to Agents.Status()
//   <lthn-view-agent-dispatch>           — launch a run, wired to Agents.Dispatch()
//   <lthn-view-agent-scan>               — find dispatch candidates, wired to Agents.Scan()
//   <lthn-view-agent-flows>              — the Do-Engine ability registry, wired to Tools.List()
//   <lthn-view-repos|issues|prs|deploys> — repo context, reused from the former Coding view

import "./activity";
import "./dispatch";
import "./workspaces";
import "./scan";
import "./flows";

// Pulled-in Coding panels — repos/issues/prs/deploys are repo context
// (what the fleet works on), reused here rather than discarded.
import "../coding";

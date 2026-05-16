// SPDX-Licence-Identifier: EUPL-1.2
//
// Planning view registry — single side-effect entry point.
//
// Imported by frontend/src/lit/index.ts so the six <lthn-view-*>
// custom elements register before the app shell mounts. Once
// registered they take over from the placeholder fallback in
// app-shell._instantiate(), so the Planning view's nav items
// (Today / Sprints / Backlog / Roadmap / Retros / Calendar) mount
// real content instead of the "coming soon" card.

import "./today";
import "./sprints";
import "./backlog";
import "./roadmap";
import "./retros";
import "./calendar";

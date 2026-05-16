// SPDX-Licence-Identifier: EUPL-1.2
// Office view — element registry
//
// Single import for the parent ../index.ts:
//   import "./views/office";
// Side-effect imports register every <lthn-view-…> custom element the
// Office view's nav references in app-shell.ts.
//
// NOTE: `<lthn-view-calendar>` is intentionally NOT registered here.
// Calendar is owned by the Planning view (see ../planning/calendar.ts)
// and shared into Office's nav via the same tag — cross-view reuse is
// by design (HANDOVER-VIEWS.md § Office). One element, two view
// memberships, zero duplication.

import "./documents";
import "./mail";
import "./files";

// SPDX-Licence-Identifier: EUPL-1.2
// Marketing view registry — single side-effect import that registers
// every <lthn-view-*> custom element in the Marketing surface.
//
// Tag → file map:
//   <lthn-view-campaigns>  → ./campaigns
//   <lthn-view-content>    → ./content
//   <lthn-view-social>     → ./social
//   <lthn-view-audience>   → ./audience
//   <lthn-view-analytics>  → ./analytics
//
// Mirrors plans/ops/lthn/desktop/design-package-2026-05-16/
//   lit-views-marketing.js. Five views, monochrome treatment, no
//   growth-hack dashboard ornament.

import "./campaigns";
import "./content";
import "./social";
import "./audience";
import "./analytics";

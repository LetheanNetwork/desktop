// SPDX-Licence-Identifier: EUPL-1.2
// Sales view · element registry
//
// Single import for the Lit registry (`frontend/src/lit/index.ts`):
//   import "./views/sales";
// Side-effect imports register every <lthn-view-*> Sales custom element
// so the <lthn-app-shell> view-switcher can mount them by `tag`.

import "./pipeline";
import "./contacts";
import "./deals";
import "./forecast";

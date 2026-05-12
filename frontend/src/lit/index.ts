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
import "./obs/benchmark-window";
import "./obs/logs-window";
import "./obs/telemetry-window";
import "./ext/integrations-window";
import "./ext/tools-window";
import "./ext/network-window";
import "./ext/distillation-window";
import "./ext/fleet-window";

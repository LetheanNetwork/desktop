// SPDX-Licence-Identifier: EUPL-1.2
// Lethean Desktop — Lit element registry
//
// Single import for the surface router (frontend/src/main.js):
//   import './lit/index.js'
// Side-effect imports register every custom element. The named
// renderChrome export is for callers that compose a window inline
// (e.g. the tray popover route).

export { renderChrome } from "./chrome.js";
import "./chrome.js";

import "./chat/chat-window.js";
import "./ops/welcome-window.js";
import "./ops/settings-window.js";
import "./ops/model-browser-window.js";
import "./obs/benchmark-window.js";
import "./obs/logs-window.js";
import "./obs/telemetry-window.js";
import "./ext/integrations-window.js";
import "./ext/tools-window.js";
import "./ext/network-window.js";
import "./ext/distillation-window.js";
import "./ext/fleet-window.js";

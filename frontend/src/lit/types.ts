// SPDX-Licence-Identifier: EUPL-1.2
// Shared types for the Lethean Desktop Lit element catalogue.
//
// These describe the fixture data shapes used by the design-canon
// windows. As real Go bindings replace fixture data, components
// migrate to the canonical service-binding types from
// frontend/bindings/dappco.re/lthn/desktop/pkg/desktop/.

import type { TemplateResult } from "lit";

/* ── chat (E0) ─────────────────────────────────────────────────── */

export type ChatState = "empty" | "generating" | "multi-turn" | "switched-model" | "no-model";
export type RailMode = "empty" | "filled";
export type RightRailMode = "expanded" | "collapsed" | "hidden";

export interface RailData {
  toksLive: string;
  watts:    string;
  kvHit:    string;
  tokens:   string;
  ctx:      string;
  sparkline?: boolean;
  sources?: { title: string; kind: string }[];
}

export interface ChatTurn {
  role: "you" | "model";
  text: string;
  code?: { lang: string; text: string };
  citations?: string[];
}

export interface ChatBanner {
  tone: "ok" | "warn" | "err";
  text: string;
  action: string;
}

export interface ChatComposer {
  value:    string;
  hint?:    string;
  sending?: boolean;
  disabled?:boolean;
}

export interface ChatStateData {
  railData:     RailData;
  turns:        ChatTurn[] | null;
  banner:       ChatBanner | null;
  composer:     ChatComposer;
  toolbarModel: string;
}

export interface Conversation {
  id:      string;
  bucket:  "today" | "yesterday" | "week";
  title:   string;
  snippet: string;
  model:   string;
}

/* ── shared template helpers ───────────────────────────────────── */

export type LitContent = TemplateResult | string | typeof import("lit").nothing | unknown;

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

/* ── model browser (display, derived from ModelsService.Entry + runner) ── */

export type LocalModelStatus = "loaded" | "available" | "downloading" | "error";

export interface LocalModel {
  id:     string;            // slug (kebab-case)
  name:   string;            // friendly label
  family: string;            // "Gemma" / "Llama" / "Phi" / "Qwen"
  size:   string;            // human-readable "2.1 GB"
  status: LocalModelStatus;
}

/* ── chrome primitives (lit-chrome) ────────────────────────────── */

export type BtnTone   = "primary" | "ghost" | "quiet" | "danger";
export type BtnSize   = "sm" | "md" | "lg";
export type StatusDotVariant = "ok" | "warn" | "err" | "idle" | "active";
export type StatePillVariant = "connected" | "running" | "queued" | "disconnected" | "preview" | "latest";

/* ── shared template helpers ───────────────────────────────────── */

import type { nothing } from "lit";
export type LitContent = TemplateResult | string | typeof nothing | null;

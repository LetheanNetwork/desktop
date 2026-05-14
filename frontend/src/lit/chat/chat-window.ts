// SPDX-Licence-Identifier: EUPL-1.2
// E0 · chat — <lthn-chat-window>
// Light-DOM Lit element. Composes renderChrome() from ../chrome.js.

import { LitElement, html, nothing } from "lit";
import { renderChrome } from "../chrome";
import { T } from "@lthn/i18n/coreservice";
import type {
  ChatState, ChatStateData, ChatTurn, ChatBanner, ChatComposer,
  Conversation, RailData, RailMode, RightRailMode,
} from "../types";

/* ── data fixtures (shared with the React version) ────────────────── */
const CONVERSATIONS: Conversation[] = [
  { id:"c1", bucket:"today",     title:"Refactor the embed loop",  snippet:"Looks like the issue is the closure capturing…", model:"Gemma 4 E2B" },
  { id:"c2", bucket:"today",     title:"Brief read · drone bill",  snippet:"Two paragraphs summarising the key clauses.",   model:"Llama 3.2 3B" },
  { id:"c3", bucket:"yesterday", title:"JSON to TOML",             snippet:"Here's the converted config in TOML…",         model:"Gemma 4 E2B" },
  { id:"c4", bucket:"yesterday", title:"Vi voice samples",         snippet:"Three drafts in plain-spoken register.",       model:"Gemma 4 E2B" },
  { id:"c5", bucket:"week",      title:"Tokeniser benchmarks",     snippet:"PP throughput on M3 Pro vs M4 Air…",           model:"Gemma 4 E2B" },
  { id:"c6", bucket:"week",      title:"Onboarding microcopy",     snippet:"Calm-presence voice across the welcome flow.", model:"Llama 3.2 3B" },
];

const TURNS_MULTI: ChatTurn[] = [
  { role:"you",   text:"Walk me through how this Go embed loop closes over its loop variable. The captured value is wrong on every iteration." },
  { role:"model", text:"The loop variable is shared across iterations — every closure captures the same address, so by the time the goroutines fire, they all see the final value.\n\nGo 1.22 changed this so each iteration gets its own copy. If you're on 1.21 or earlier, you need to shadow the variable explicitly.",
    code:{ lang:"go", text:"for _, item := range items {\n    item := item // shadow for the closure\n    go func() {\n        process(item)\n    }()\n}" } },
  { role:"you",   text:"Right, so just `item := item` before the goroutine. Why does the runtime not see this as a redundant assignment?" },
  { role:"model", text:"Because it isn't redundant — it creates a new variable in the inner scope. The compiler treats the inner `item` as a distinct binding; the closure captures THAT one. The optimiser can't elide it because the goroutine outlives the iteration.",
    citations:["go.dev/ref/spec#Variable_scope", "go.dev/blog/loopvar-preview"] },
];

const TURNS_GEN: ChatTurn[] = [
  ...TURNS_MULTI.slice(0, 2),
  { role:"you",   text:"Now write the test that would have caught this." },
  { role:"model", text:"A table-driven test that fires each goroutine and asserts the captured value matches the iteration — running it under `-race` proves the closure capture rather than just timing luck.\n\n" },
];

/* ── per-state derived props ──────────────────────────────────────── */
function chatStateData(state: ChatState): ChatStateData {
  const railData: Record<ChatState, RailData> = {
    empty:            { toksLive:"—",    watts:"—",      kvHit:"—",   tokens:"—",              ctx:"—" },
    generating:       { toksLive:"47.2", watts:"12.4 W", kvHit:"94%", tokens:"1,284 / 4,096",  ctx:"1,284 / 4,096", sparkline:true,
                        sources:[{title:"Go specification · variable scope", kind:"Reference · loaded from cache"}] },
    "multi-turn":     { toksLive:"44.6", watts:"11.8 W", kvHit:"96%", tokens:"2,041 / 4,096",  ctx:"2,041 / 4,096" },
    "switched-model": { toksLive:"—",    watts:"—",      kvHit:"0%",  tokens:"0 / 8,192",      ctx:"0 / 8,192" },
    "no-model":       { toksLive:"—",    watts:"—",      kvHit:"—",   tokens:"—",              ctx:"—" },
  };

  const turns: Record<ChatState, ChatTurn[] | null> = {
    empty: null,
    generating: TURNS_GEN,
    "multi-turn": TURNS_MULTI,
    "switched-model": TURNS_MULTI,
    "no-model": null,
  };

  const banner: ChatBanner | null =
    state === "switched-model" ? { tone:"warn", text:"Switched to Llama 3.2 3B mid-conversation — KV cache cleared. The next turn will replay context.", action:"Restore Gemma" } :
    state === "no-model"       ? { tone:"warn", text:"No model loaded. Pick one from the tray to start composing.", action:"Open tray" } :
    null;

  const composer: Record<ChatState, ChatComposer> = {
    empty:            { value:"" },
    generating:       { value:"Now write the test that would have caught this.", sending:true, hint:"Esc · stop" },
    "multi-turn":     { value:"", hint:"⌘↵ · send" },
    "switched-model": { value:"", hint:"⌘↵ · send" },
    "no-model":       { value:"", disabled:true },
  };

  const toolbarModel: Record<ChatState, string> = {
    empty:"Gemma 4 E2B", generating:"Gemma 4 E2B",
    "multi-turn":"Gemma 4 E2B", "switched-model":"Llama 3.2 3B",
    "no-model":"No model",
  };

  return {
    railData: railData[state],
    turns: turns[state],
    banner,
    composer: composer[state],
    toolbarModel: toolbarModel[state],
  };
}

class LthnChatWindow extends LitElement {
  static readonly properties = {
    state:     { type: String, reflect: true },
    rail:      { type: String, reflect: true },
    rightRail: { type: String, attribute: "right-rail", reflect: true },
    w:         { type: Number },
    h:         { type: Number },
    // Without this static-properties entry Lit won't observe the
    // `embedded=""` attribute that <lthn-app-shell>._instantiate sets
    // on the child element, so renderChrome's embedded branch never
    // fires and the window double-renders inside the shell.
    embedded:  { type: Boolean, reflect: true },
    chrome:    { state: true },
    conversations: { state: true },
    activeConversationId: { state: true },
    railErr: { state: true },
    liveTurns: { state: true },
    composerValue: { state: true },
    sending: { state: true },
    sendErr: { state: true },
    activeModel: { state: true },
    version: { state: true },
    runnerCount: { state: true },
    t: { state: true },
  };
  declare state:     ChatState;
  declare rail:      RailMode;
  declare rightRail: RightRailMode;
  declare w:         number;
  declare h:         number;
  declare embedded: boolean;
  declare chrome:    { title: string; subtitle: string };
  declare conversations: Conversation[];
  declare activeConversationId: string | null;
  declare railErr: string;
  declare liveTurns: ChatTurn[] | null;
  declare composerValue: string;
  declare sending: boolean;
  declare sendErr: string;
  declare activeModel: string;
  declare version: string;
  declare runnerCount: number;
  declare t: {
    railSearch: string;
    bToday: string; bYesterday: string; bWeek: string;
    railEmpty: string; railNew: string;
    emptyTitle: string; emptyBody: string;
    composerDisabled: string; composerReady: string;
    composerAttach: string; composerSlash: string;
    btnSend: string; btnStop: string;
    railMeta: string;
    statTps: string; statWatts: string; statKv: string; statTokens: string;
    labelSampling: string;
    sampTemp: string; sampTopP: string; sampMaxTok: string; sampContext: string;
    labelSources: string; sourcesEmpty: string;
  };
  constructor() {
    super();
    this.state = "multi-turn";
    this.rail = "filled";
    this.rightRail = "expanded";
    this.w = 1100;
    this.h = 740; this.embedded = false;
    this.chrome = { title: "lthn · chat", subtitle: "conversation · local" };
    this.conversations = [];
    this.activeConversationId = null;
    this.railErr = "";
    this.liveTurns = null;
    this.composerValue = "";
    this.sending = false;
    this.sendErr = "";
    this.activeModel = "";
    this.version = "0.2.0-rc1";
    this.runnerCount = 1;
    this.t = {
      railSearch: "Search conversations",
      bToday: "Today", bYesterday: "Yesterday", bWeek: "This week",
      railEmpty: "No conversations yet. Start one from the composer.",
      railNew: "New conversation",
      emptyTitle: "What shall we look at?",
      emptyBody:  "Conversations stay on this Mac. Nothing leaves unless you flip on the API server in Settings and a client connects.",
      composerDisabled: "Load a model from the tray to start composing.",
      composerReady:    "Ask anything — runs locally on this Mac. ⌘↵ to send.",
      composerAttach:   "Attach",
      composerSlash:    "Slash commands",
      btnSend:          "Send",
      btnStop:          "Stop",
      railMeta:         "Turn metadata",
      statTps:          "Tok/s · live",
      statWatts:        "Watts · this turn",
      statKv:           "KV cache hit",
      statTokens:       "Tokens used",
      labelSampling:    "Sampling",
      sampTemp:         "Temperature",
      sampTopP:         "Top-p",
      sampMaxTok:       "Max tokens",
      sampContext:      "Context",
      labelSources:     "Sources",
      sourcesEmpty:     "None this turn. Citations appear here when the model grounds an answer.",
    };
  }
  createRenderRoot() { return this; }
  async connectedCallback() {
    super.connectedCallback();
    const [
      title, subtitle, rs, bt, by, bw, re, rn,
      et, eb, cd, cr, ca, cs, bSend, bStop,
      rMeta, sTps, sWatts, sKv, sTokens,
      lSamp, sT, sP, sMax, sCtx,
      lSrc, sEmpty,
    ] = await Promise.all([
      T("window.chat.title"),
      T("window.chat.subtitle"),
      T("window.chat.rail_search"),
      T("window.chat.rail_bucket_today"),
      T("window.chat.rail_bucket_yesterday"),
      T("window.chat.rail_bucket_week"),
      T("window.chat.rail_empty"),
      T("window.chat.rail_new"),
      T("window.chat.empty_title"),
      T("window.chat.empty_body"),
      T("window.chat.composer_disabled"),
      T("window.chat.composer_ready"),
      T("window.chat.composer_attach"),
      T("window.chat.composer_slash"),
      T("window.chat.btn_send"),
      T("window.chat.btn_stop"),
      T("window.chat.rail_meta"),
      T("window.chat.stat_tps_live"),
      T("window.chat.stat_watts"),
      T("window.chat.stat_kv_hit"),
      T("window.chat.stat_tokens"),
      T("window.chat.label_sampling"),
      T("window.chat.samp_temperature"),
      T("window.chat.samp_top_p"),
      T("window.chat.samp_max_tokens"),
      T("window.chat.samp_context"),
      T("window.chat.label_sources"),
      T("window.chat.sources_empty"),
    ]);
    this.chrome = { title, subtitle };
    this.t = {
      railSearch: rs, bToday: bt, bYesterday: by, bWeek: bw, railEmpty: re, railNew: rn,
      emptyTitle: et, emptyBody: eb,
      composerDisabled: cd, composerReady: cr,
      composerAttach: ca, composerSlash: cs,
      btnSend: bSend, btnStop: bStop,
      railMeta: rMeta,
      statTps: sTps, statWatts: sWatts, statKv: sKv, statTokens: sTokens,
      labelSampling: lSamp,
      sampTemp: sT, sampTopP: sP, sampMaxTok: sMax, sampContext: sCtx,
      labelSources: lSrc, sourcesEmpty: sEmpty,
    };
    await Promise.all([this._reloadRail(), this._reloadModel(), this._reloadBuild()]);
  }

  /** Pull the binary version from firstlaunch.Build() so the footer
   *  status line reflects the running binary instead of the design's
   *  v0.2.0-rc1 literal. Errors stay silent — the fallback string
   *  already covers the canvas-preview case. */
  async _reloadBuild() {
    try {
      const svc = await import("@desktop/firstlaunch/wailsservice");
      const b = await svc.Build();
      if (b?.version) this.version = b.version;
    } catch { /* keep fallback */ }
  }

  /** Pull the runner's loaded model list and pick the first one for
   *  the toolbar + footer. Empty list → blank string so the render
   *  falls back to the fixture label ("No model" / etc.). Also reads
   *  the configured route count for the toolbar's "N runner" slot. */
  async _reloadModel() {
    try {
      const svc = await import("@desktop/runner/service");
      const { unwrap } = await import("../result");
      const [models, routes] = await Promise.all([
        unwrap<string[]>(svc.WModels(), []),
        unwrap<unknown[]>(svc.WRoutes(), []),
      ]);
      this.activeModel = models?.[0] || "";
      if ((routes?.length ?? 0) > 0) this.runnerCount = routes.length;
    } catch {
      this.activeModel = "";
    }
  }

  /** Lit lifecycle — when activeConversationId changes, reload
   *  the live turns from sessions.Read so the transcript matches
   *  the selected conversation. */
  updated(changed: Map<string, unknown>) {
    if (changed.has("activeConversationId")) {
      void this._loadTurns();
    }
  }

  /** Load messages for the active session and map inference.Message
   *  → ChatTurn for rendering. Empty array when no active session
   *  or the session has no messages yet. */
  async _loadTurns() {
    if (!this.activeConversationId) {
      this.liveTurns = null;
      return;
    }
    try {
      const svc = await import("@desktop/sessions/wailsservice");
      const { unwrap } = await import("../result");
      type Msg = Parameters<typeof messageToTurn>[0];
      const msgs = await unwrap<Msg[]>(svc.Read(this.activeConversationId), []);
      this.liveTurns = (msgs || []).map(messageToTurn);
    } catch (err: unknown) {
      this.sendErr = err instanceof Error ? err.message : String(err);
      this.liveTurns = [];
    }
  }

  /** Send the composer's current value to the active session.
   *  Round-trip: append user → runner.WChat(history) → append
   *  assistant → reload turns. Errors surface inline via sendErr. */
  async _send() {
    const text = this.composerValue.trim();
    if (!text || this.sending) return;
    if (!this.activeConversationId) {
      // Auto-create on first send if no session is selected.
      await this._newConversation();
      if (!this.activeConversationId) return;
    }
    const id = this.activeConversationId;
    this.sending = true;
    this.sendErr = "";
    this.composerValue = "";
    try {
      const [sessions, runner] = await Promise.all([
        import("@desktop/sessions/wailsservice"),
        import("@desktop/runner/service"),
      ]);
      const { demand, unwrap } = await import("../result");
      type Msg = Parameters<typeof messageToTurn>[0];
      await demand<unknown>(sessions.Append(id, "user", text));
      const history = await unwrap<Msg[]>(sessions.Read(id), []);
      const reply = await unwrap<string>(runner.WChat(history || []), "");
      await demand<unknown>(sessions.Append(id, "assistant", reply || ""));
      await this._loadTurns();
      await this._reloadRail();
    } catch (err: unknown) {
      this.sendErr = err instanceof Error ? err.message : String(err);
    } finally {
      this.sending = false;
    }
  }

  /** Composer keydown — ⌘↵ / Ctrl+↵ sends; plain Enter inserts a
   *  newline (textarea default). */
  _composerKeydown(e: KeyboardEvent) {
    if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) {
      e.preventDefault();
      void this._send();
    }
  }

  /** Create a fresh session via sessions.Create + select it.
   *  Title is left as "New conversation" until the user names it
   *  or the first message lands (future polish — rename based on
   *  first user turn). */
  async _newConversation() {
    try {
      const svc = await import("@desktop/sessions/wailsservice");
      const { demand } = await import("../result");
      const id = await demand<string>(svc.Create(this.t.railNew));
      await this._reloadRail();
      this.activeConversationId = id;
    } catch (err: unknown) {
      this.railErr = err instanceof Error ? err.message : String(err);
    }
  }

  /** Pull the session catalogue from sessions.WailsService.List()
   *  and map each SessionInfo → Conversation. Bucket falls out of
   *  the updated_at timestamp; snippet is intentionally blank for
   *  now (would require sessions.Read per id — N+1, defer until
   *  the service surfaces a last-message preview natively). */
  async _reloadRail() {
    try {
      const svc = await import("@desktop/sessions/wailsservice");
      const { unwrap } = await import("../result");
      type SessionInfo = Parameters<typeof deriveConversation>[0];
      const list = await unwrap<SessionInfo[]>(svc.List(), []);
      this.conversations = (list || []).map(deriveConversation).filter(Boolean) as Conversation[];
      if (this.conversations.length > 0 && !this.activeConversationId) {
        this.activeConversationId = this.conversations[0].id;
      }
      this.railErr = "";
    } catch (err: unknown) {
      this.railErr = err instanceof Error ? err.message : String(err);
      this.conversations = [];
    }
  }

  render() {
    const fixture = chatStateData(this.state);
    const { railData, banner } = fixture;
    // Toolbar + footer prefer the live model name when the runner has
    // one loaded; fall back to the per-state fixture label so canvas
    // previews + the "no-model" state still read coherently.
    const toolbarModel = this.activeModel || fixture.toolbarModel;
    // When a real session is selected, use its live turns instead of
    // the demo fixtures. Live empty (session with no messages yet)
    // also wins so we don't fall back to demo turns mid-session.
    const turns = this.activeConversationId !== null ? this.liveTurns : fixture.turns;
    const composer = {
      ...fixture.composer,
      value: this.composerValue,
      sending: this.sending,
    };

    /* — footer per state — */
    const footer =
      this.state === "no-model" ? html`
        <lthn-status-dot variant="idle"></lthn-status-dot>Idle · no model loaded`
      : this.state === "generating" ? html`
        <lthn-status-dot variant="ok"></lthn-status-dot>Generating · 47.2 t/s · 12.4 W · Airplane-mode OK`
      : html`
        <lthn-status-dot variant="ok"></lthn-status-dot>Model ready · Airplane-mode OK · ${this.runnerCount} runner · v${this.version}`;

    /* — toolbar — */
    const toolbar = html`
      <div style="display:flex; align-items:center; gap:6px; padding:4px 9px; border-radius:7px;
                  background:rgba(255,255,255,0.04); border:1px solid rgba(255,255,255,0.07);
                  font-size:11.5px; color:var(--fg-1);">
        <lthn-status-dot variant=${this.state === "no-model" ? "idle" : "ok"}></lthn-status-dot>
        ${toolbarModel}
        <i class="fa-solid fa-angle-down" style="font-size:9px; color:var(--fg-3); margin-left:2px;"></i>
      </div>
      <div style="font-family:var(--font-mono); font-size:10.5px; color:var(--fg-3); letter-spacing:0.02em; padding:0 4px;">
        ${railData.ctx} · ${this.runnerCount} runner
      </div>
      <lthn-btn tone="ghost" size="sm"><i class="fa-solid fa-sliders" style="font-size:10px;"></i></lthn-btn>
      <lthn-btn tone="ghost" size="sm" ?active=${this.rightRail === "expanded"}>
        <i class="fa-solid fa-chart-line" style="font-size:10px;"></i>
      </lthn-btn>
    `;

    /* — body — */
    const body = html`
      <div style="flex:1; min-height:0; display:flex;">
        ${this._renderRail()}
        <div style="flex:1; min-width:0; display:flex; flex-direction:column;">
          ${this._renderSurface(turns, banner)}
          ${this._renderComposer(composer)}
        </div>
        ${this.rightRail !== "hidden" ? this._renderRightRail(railData) : nothing}
      </div>
    `;

    return renderChrome({
      title: this.chrome.title,
      subtitle: this.chrome.subtitle,
      w: this.w, h: this.h,
      toolbar, body, footer,
      embedded: this.embedded,
    });
  }

  /* — left rail (conversation list) — */
  _renderRail() {
    const empty = this.conversations.length === 0;
    const buckets = [
      { label: this.t.bToday,     key: "today" },
      { label: this.t.bYesterday, key: "yesterday" },
      { label: this.t.bWeek,      key: "week" },
    ];
    const activeId = this.activeConversationId;
    return html`
      <aside style="width:240px; flex-shrink:0; border-right:1px solid rgba(255,255,255,0.05);
                    background:rgba(0,0,0,0.18); display:flex; flex-direction:column; min-height:0;">
        <div style="padding:12px 12px 8px;">
          <div style="display:flex; align-items:center; gap:7px; height:28px; padding:0 10px;
                      background:rgba(255,255,255,0.04); border:1px solid rgba(255,255,255,0.06);
                      border-radius:6px;">
            <i class="fa-solid fa-magnifying-glass" style="font-size:10px; color:var(--fg-3);"></i>
            <span style="font-size:11.5px; color:var(--fg-3);">${this.t.railSearch}</span>
            <div style="flex:1"></div>
            <span style="font-family:var(--font-mono); font-size:9.5px; color:var(--fg-3);
                         padding:1px 4px; border:1px solid rgba(255,255,255,0.08); border-radius:3px;">⌘K</span>
          </div>
        </div>
        <div style="flex:1; min-height:0; overflow:hidden; padding:0 6px 8px;">
          ${empty ? html`
            <div style="padding:60px 16px 0; text-align:center; font-size:11.5px;
                        color:var(--fg-3); line-height:1.55;">
              <div style="width:36px; height:36px; margin:0 auto 12px; border-radius:8px;
                          background:rgba(255,255,255,0.04); display:flex; align-items:center; justify-content:center;">
                <i class="fa-regular fa-comment" style="font-size:14px; color:var(--fg-3);"></i>
              </div>
              ${this.t.railEmpty}
            </div>
          ` : html`
            <div style="display:flex; flex-direction:column; gap:1px;">
              ${buckets.map(b => html`
                <lthn-label style="display:block; padding:8px 8px 4px;">${b.label}</lthn-label>
                ${this.conversations.filter(c => c.bucket === b.key).map(c =>
                  this._renderRailItem(c, c.id === activeId))}
              `)}
            </div>
          `}
        </div>
        <div style="padding:8px 10px; border-top:1px solid rgba(255,255,255,0.05); display:flex; gap:6px;">
          <lthn-btn tone="ghost" size="md" style="flex:1; justify-content:center;"
            @click=${() => this._newConversation()}>
            <i class="fa-solid fa-plus" style="font-size:10px;"></i> ${this.t.railNew}
          </lthn-btn>
          <lthn-btn tone="quiet" size="md"><i class="fa-solid fa-ellipsis" style="font-size:11px;"></i></lthn-btn>
        </div>
      </aside>
    `;
  }

  _renderRailItem(c: Conversation, active: boolean) {
    return html`
      <div
        @click=${() => { this.activeConversationId = c.id; }}
        style="padding:8px 10px; border-radius:6px;
               background:${active ? "rgba(255,255,255,0.07)" : "transparent"};
               border-left:${active ? "2px solid var(--brand-400)" : "2px solid transparent"};
               display:flex; flex-direction:column; gap:2px; cursor:pointer;
               --wails-draggable: no-drag;">
        <div style="font-size:12px; font-weight:500; color:var(--fg-0); white-space:nowrap;
                    overflow:hidden; text-overflow:ellipsis; letter-spacing:-0.005em;">${c.title}</div>
        ${c.snippet ? html`<div style="font-size:10.5px; color:var(--fg-3); white-space:nowrap;
                    overflow:hidden; text-overflow:ellipsis;">${c.snippet}</div>` : nothing}
        ${c.model ? html`<div style="font-family:var(--font-mono); font-size:9px; color:var(--fg-3);
                    letter-spacing:0.02em; margin-top:1px;">${c.model}</div>` : nothing}
      </div>
    `;
  }

  /* — surface (conversation transcript or empty hero) — */
  _renderSurface(turns: ChatTurn[] | null, banner: ChatBanner | null) {
    const empty = this.state === "empty";
    const streamingIdx = this.state === "generating" ? (turns?.length || 0) - 1 : -1;
    return html`
      <main style="flex:1; min-height:0; display:flex; flex-direction:column; position:relative;">
        ${banner ? html`
          <div style="flex-shrink:0; margin:12px 22px 0; padding:10px 12px; border-radius:8px;
                      background:${banner.tone === "warn" ? "rgba(217,154,72,0.08)" : "rgba(64,193,197,0.06)"};
                      border:1px solid ${banner.tone === "warn" ? "rgba(217,154,72,0.20)" : "rgba(64,193,197,0.18)"};
                      display:flex; align-items:center; gap:10px;
                      font-size:11.5px; color:${banner.tone === "warn" ? "var(--warning-300)" : "var(--brand-200)"};">
            <i class="fa-solid ${banner.tone === "warn" ? "fa-circle-exclamation" : "fa-circle-info"}" style="font-size:11px;"></i>
            <div style="flex:1; line-height:1.5;">${banner.text}</div>
            ${banner.action ? html`<lthn-btn tone="ghost" size="sm">${banner.action}</lthn-btn>` : nothing}
          </div>
        ` : nothing}
        <div style="flex:1; min-height:0; overflow:hidden; padding:20px 22px;
                    display:flex; flex-direction:column; gap:22px;">
          ${empty ? this._renderEmpty() :
            (turns || []).map((t, i) => this._renderTurn(t, i === streamingIdx))}
        </div>
      </main>
    `;
  }

  _renderEmpty() {
    const starters = [
      { icon:"fa-code",    text:"Explain this regex" },
      { icon:"fa-pen-nib", text:"Rewrite a paragraph" },
      { icon:"fa-table",   text:"Reshape this JSON" },
      { icon:"fa-question",text:"What can this model do?" },
    ];
    return html`
      <div style="flex:1; display:flex; flex-direction:column; align-items:center;
                  justify-content:center; gap:22px; padding:40px;">
        <div style="width:56px; height:56px; border-radius:14px;
                    background:linear-gradient(155deg, rgba(64,193,197,0.18), rgba(64,193,197,0.02));
                    border:1px solid rgba(64,193,197,0.18);
                    display:flex; align-items:center; justify-content:center;">
          <lthn-glyph size="28" color="var(--brand-200)" active></lthn-glyph>
        </div>
        <div style="text-align:center; display:flex; flex-direction:column; gap:8px;">
          <div style="font-size:22px; font-weight:600; color:var(--fg-0); letter-spacing:-0.015em;">
            ${this.t.emptyTitle}
          </div>
          <div style="font-size:13px; color:var(--fg-2); max-width:420px; line-height:1.55;">
            ${this.t.emptyBody}
          </div>
        </div>
        <div style="display:grid; grid-template-columns:repeat(2, 220px); gap:8px; margin-top:6px;">
          ${starters.map(s => html`
            <div style="padding:10px 12px; border-radius:8px;
                        background:rgba(255,255,255,0.03);
                        border:1px solid rgba(255,255,255,0.06);
                        display:flex; align-items:center; gap:10px;
                        font-size:12px; color:var(--fg-1); cursor:pointer;">
              <i class="fa-solid ${s.icon}" style="font-size:11px; color:var(--fg-3);"></i>${s.text}
            </div>
          `)}
        </div>
      </div>
    `;
  }

  _renderTurn(t: ChatTurn, streaming: boolean) {
    const isYou = t.role === "you";
    return html`
      <div style="display:flex; flex-direction:column; gap:8px; padding-top:4px;">
        <div style="display:flex; align-items:center; gap:8px;">
          <div style="width:22px; height:22px; border-radius:6px;
                      background:${isYou
                        ? "linear-gradient(145deg, rgba(255,255,255,0.10), rgba(255,255,255,0.04))"
                        : "linear-gradient(145deg, rgba(64,193,197,0.30), rgba(64,193,197,0.08))"};
                      border:1px solid rgba(255,255,255,0.06);
                      display:flex; align-items:center; justify-content:center;">
            ${isYou
              ? html`<i class="fa-solid fa-user" style="font-size:10px; color:var(--fg-1);"></i>`
              : html`<lthn-glyph size="12" color="var(--brand-200)"></lthn-glyph>`}
          </div>
          <div style="font-size:12px; font-weight:600; color:var(--fg-0); letter-spacing:-0.005em;">
            ${isYou ? "You" : (this.activeModel || "Gemma 4 E2B")}
          </div>
          <div style="flex:1"></div>
          ${!streaming ? html`
            <div style="display:flex; gap:4px; opacity:0.5;">
              <lthn-btn tone="quiet" size="sm"><i class="fa-regular fa-copy" style="font-size:10px;"></i></lthn-btn>
              <lthn-btn tone="quiet" size="sm"><i class="fa-solid fa-rotate-right" style="font-size:10px;"></i></lthn-btn>
            </div>
          ` : nothing}
        </div>
        <div style="margin-left:30px; font-size:13.5px; line-height:1.6;
                    color:var(--fg-1); white-space:pre-wrap; letter-spacing:-0.003em;">
          ${t.text}${streaming ? html`<span style="display:inline-block; width:7px; height:14px;
            background:var(--brand-300); vertical-align:-3px; margin-left:2px;
            animation:lthn-cursor 1s steps(2) infinite;"></span>` : nothing}
        </div>
        ${t.code ? html`
          <div style="margin-left:30px; border-radius:8px;
                      background:rgba(0,0,0,0.32);
                      border:1px solid rgba(255,255,255,0.05); overflow:hidden;">
            <div style="display:flex; align-items:center; gap:8px; padding:6px 12px;
                        border-bottom:1px solid rgba(255,255,255,0.04);
                        background:rgba(0,0,0,0.18);">
              <span style="font-family:var(--font-mono); font-size:10px; color:var(--fg-3);
                           letter-spacing:0.04em; text-transform:uppercase;">${t.code.lang}</span>
              <div style="flex:1"></div>
              <lthn-btn tone="quiet" size="sm">
                <i class="fa-regular fa-copy" style="font-size:10px;"></i> Copy
              </lthn-btn>
            </div>
            <pre style="margin:0; padding:12px 14px; font-family:var(--font-mono);
                        font-size:11.5px; line-height:1.6; color:var(--fg-1);
                        white-space:pre; overflow:auto;">${t.code.text}</pre>
          </div>
        ` : nothing}
        ${t.citations ? html`
          <div style="margin-left:30px; display:flex; flex-wrap:wrap; gap:6px; margin-top:2px;">
            ${t.citations.map(c => html`
              <span style="font-family:var(--font-mono); font-size:10px; color:var(--brand-300);
                           padding:2px 7px; border-radius:999px;
                           background:rgba(64,193,197,0.08);
                           border:1px solid rgba(64,193,197,0.18);">
                <i class="fa-solid fa-link" style="font-size:8px; margin-right:4px;"></i>${c}
              </span>
            `)}
          </div>
        ` : nothing}
      </div>
    `;
  }

  /* — composer — */
  _renderComposer({ value, disabled, sending, error, hint }: ChatComposer & { error?: string }) {
    const liveErr = error || this.sendErr;
    return html`
      <div style="padding:14px 22px 16px; border-top:1px solid rgba(255,255,255,0.05);
                  background:rgba(0,0,0,0.12); display:flex; flex-direction:column; gap:8px;">
        ${liveErr ? html`
          <div style="display:flex; align-items:center; gap:8px; padding:7px 11px;
                      background:rgba(255,76,76,0.08); border:1px solid rgba(255,76,76,0.18);
                      border-radius:6px; font-size:11.5px; color:var(--err-300, #ffb4b4);">
            <i class="fa-solid fa-triangle-exclamation" style="font-size:11px;"></i>${liveErr}
          </div>
        ` : nothing}
        <div style="position:relative;
                    background:${disabled ? "rgba(255,255,255,0.02)" : "rgba(255,255,255,0.04)"};
                    border:1px solid rgba(255,255,255,0.07); border-radius:10px;
                    min-height:78px; padding:12px 14px 38px;
                    opacity:${disabled ? 0.55 : 1};">
          <textarea
            .value=${value}
            ?disabled=${disabled || sending}
            @input=${(e: Event) => { this.composerValue = (e.target as HTMLTextAreaElement).value; }}
            @keydown=${(e: KeyboardEvent) => this._composerKeydown(e)}
            placeholder=${disabled ? this.t.composerDisabled : this.t.composerReady}
            style="width:100%; min-height:52px; resize:vertical;
                   background:transparent; border:none; outline:none;
                   font-family:var(--font-sans); font-size:13px;
                   line-height:1.5; color:var(--fg-0);
                   --wails-draggable: no-drag;
                   ${value ? "color:var(--fg-0);" : ""}"
          ></textarea>
          <div style="position:absolute; left:12px; right:12px; bottom:8px;
                      display:flex; align-items:center; gap:8px;">
            <lthn-btn tone="quiet" size="sm" ?dim=${disabled}>
              <i class="fa-regular fa-paperclip" style="font-size:11px;"></i> ${this.t.composerAttach}
            </lthn-btn>
            <lthn-btn tone="quiet" size="sm" ?dim=${disabled}>
              <i class="fa-solid fa-slash-forward" style="font-size:10px;"></i> ${this.t.composerSlash}
            </lthn-btn>
            <div style="flex:1"></div>
            ${hint ? html`<span style="font-family:var(--font-mono); font-size:10px;
                          color:var(--fg-3); letter-spacing:0.02em;">${hint}</span>` : nothing}
            ${sending ? html`
              <lthn-btn tone="danger" size="sm">
                <i class="fa-solid fa-stop" style="font-size:10px;"></i> ${this.t.btnStop}
              </lthn-btn>
            ` : html`
              <lthn-btn tone="primary" size="sm" ?dim=${disabled}
                @click=${() => void this._send()}>
                <i class="fa-solid fa-arrow-up" style="font-size:11px;"></i> ${this.t.btnSend}
              </lthn-btn>
            `}
          </div>
        </div>
      </div>
    `;
  }

  /* — right rail (turn metadata) — */
  _renderRightRail(data: RailData) {
    if (this.rightRail === "collapsed") {
      return html`
        <aside style="width:36px; flex-shrink:0; border-left:1px solid rgba(255,255,255,0.05);
                      background:rgba(0,0,0,0.18); display:flex; flex-direction:column;
                      align-items:center; padding:10px 0; gap:10px;">
          <lthn-btn tone="quiet" size="sm"><i class="fa-solid fa-chart-line" style="font-size:11px;"></i></lthn-btn>
          <lthn-btn tone="quiet" size="sm"><i class="fa-solid fa-bolt" style="font-size:11px;"></i></lthn-btn>
          <lthn-btn tone="quiet" size="sm"><i class="fa-solid fa-database" style="font-size:11px;"></i></lthn-btn>
        </aside>
      `;
    }
    return html`
      <aside style="width:280px; flex-shrink:0; border-left:1px solid rgba(255,255,255,0.05);
                    background:rgba(0,0,0,0.18); display:flex; flex-direction:column;
                    min-height:0; overflow:hidden;">
        <div style="padding:14px 18px 8px; display:flex; align-items:center; gap:8px;">
          <lthn-label>${this.t.railMeta}</lthn-label>
          <div style="flex:1"></div>
          <lthn-btn tone="quiet" size="sm"><i class="fa-solid fa-angle-right" style="font-size:10px;"></i></lthn-btn>
        </div>
        <div style="padding:0 18px 16px; display:flex; flex-direction:column; gap:14px; overflow:auto;">
          ${this._renderRailStat(this.t.statTps, data.toksLive, data.sparkline)}
          ${this._renderRailStat(this.t.statWatts, data.watts)}
          ${this._renderRailStat(this.t.statKv, data.kvHit)}
          ${this._renderRailStat(this.t.statTokens, data.tokens)}

          <lthn-label style="margin-top:4px;">${this.t.labelSampling}</lthn-label>
          <div style="display:flex; flex-direction:column; gap:6px; font-size:11.5px;">
            <lthn-rail-row k=${this.t.sampTemp}    v="0.7"></lthn-rail-row>
            <lthn-rail-row k=${this.t.sampTopP}    v="0.95"></lthn-rail-row>
            <lthn-rail-row k=${this.t.sampMaxTok}  v="1024"></lthn-rail-row>
            <lthn-rail-row k=${this.t.sampContext} v=${data.ctx || "—"}></lthn-rail-row>
          </div>

          <lthn-label style="margin-top:4px;">${this.t.labelSources}</lthn-label>
          ${data.sources ? data.sources.map(s => html`
            <div style="padding:8px 10px; border-radius:6px;
                        background:rgba(255,255,255,0.03);
                        border:1px solid rgba(255,255,255,0.06);
                        font-size:11px; color:var(--fg-2);
                        display:flex; flex-direction:column; gap:3px;">
              <div style="color:var(--fg-1); font-weight:500;">${s.title}</div>
              <div style="color:var(--fg-3); font-size:10px;">${s.kind}</div>
            </div>
          `) : html`
            <div style="font-size:11px; color:var(--fg-3); font-style:italic;">
              ${this.t.sourcesEmpty}
            </div>
          `}
        </div>
      </aside>
    `;
  }

  _renderRailStat(label: string, value: string, sparkline = false) {
    return html`
      <div style="padding:10px 12px; border-radius:8px;
                  background:rgba(255,255,255,0.03);
                  border:1px solid rgba(255,255,255,0.06);
                  display:flex; flex-direction:column; gap:4px;">
        <div style="font-family:var(--font-mono); font-size:9.5px; color:var(--fg-3);
                    letter-spacing:0.06em; text-transform:uppercase;">${label}</div>
        <div style="display:flex; align-items:baseline; justify-content:space-between; gap:6px;">
          <div style="font-family:var(--font-mono); font-size:19px; color:var(--fg-0);
                      letter-spacing:-0.005em; font-weight:500;">${value}</div>
          ${sparkline ? html`<lthn-sparkline></lthn-sparkline>` : nothing}
        </div>
      </div>
    `;
  }
}
customElements.define("lthn-chat-window", LthnChatWindow);

// ─── helpers ────────────────────────────────────────────────────────

/** inference.Message → ChatTurn shape the existing transcript
 *  template expects. role: "user" → "you"; everything else maps
 *  to "model" so the assistant + future system / tool messages
 *  render with the assistant pill until we surface richer roles. */
function messageToTurn(msg: { role: string; content: string }): ChatTurn {
  return {
    role: msg.role === "user" ? "you" : "model",
    text: msg.content || "",
  };
}

interface SessionInfoShape {
  id: string;
  title: string;
  created_at: number;
  updated_at: number;
  messages: number;
}

/** SessionInfo → Conversation. Returns null when the timestamp is
 *  outside the rail's "today / yesterday / week" buckets so older
 *  sessions stay out of the immediate view (a "view all" surface
 *  reveals them later). */
function deriveConversation(info: SessionInfoShape): Conversation | null {
  const updatedSec = info.updated_at || info.created_at || 0;
  const ageSec = Math.max(0, Math.floor(Date.now() / 1000) - updatedSec);
  let bucket: Conversation["bucket"];
  if (ageSec < 86400)        bucket = "today";       // < 24 h
  else if (ageSec < 172800)  bucket = "yesterday";   // 24–48 h
  else if (ageSec < 604800)  bucket = "week";        // < 7 d
  else                       return null;            // older — out of rail
  return {
    id: info.id,
    bucket,
    title: info.title || "(untitled)",
    snippet: info.messages > 0 ? `${info.messages} messages` : "",
    model: "",
  };
}

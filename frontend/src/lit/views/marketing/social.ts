// SPDX-Licence-Identifier: EUPL-1.2
// Marketing · Social — <lthn-view-social>
//
// Social posts queue across channels (Mastodon, X, LinkedIn, Bluesky).
// Each card: channels · when · state · text · optional attachment.
// Calm + observational — no engagement counts, no virality scoring.
//
// Two-shell: embedded attribute collapses chrome for app-shell mount.
//
// Data: fixture only for v1. Real source is a future
// `pkg/marketing/social` Go service wrapping core-social (the MixPost
// wrapper tracked in plans/CLAUDE.md). The { ch[], when, state, text,
// attach } shape mirrors what that service will yield.

import { LitElement, html, nothing } from "lit";
import { renderChrome } from "../../chrome";

type Channel = "mastodon" | "x" | "linkedin" | "bluesky";
type PostState = "scheduled" | "sent" | "draft";

interface SocialPost {
  ch:     Channel[];
  when:   string;
  state:  PostState;
  text:   string;
  attach?: string;
}

const FIXTURE_POSTS: SocialPost[] = [
  { ch: ["mastodon", "x", "linkedin"], when: "today · 16:00",   state: "scheduled", text: "Lethean v0.2 is out. Tray-first local AI on Apple Silicon at single-watt power draw. EUPL-1.2. Try it — link in reply.", attach: "image" },
  { ch: ["mastodon", "x"],             when: "today · 09:00",   state: "sent",      text: "Reminder: every model in lthn ships with a reproducible benchmark, run on YOUR hardware. Honest tok/s, honest watts." },
  { ch: ["linkedin"],                  when: "tomorrow · 10:00",state: "scheduled", text: "We're hiring a senior Go engineer to work on the lthn runtime. UK-based, EUPL-1.2 codebase, you'll own a layer. Link in profile." },
  { ch: ["mastodon"],                  when: "yest · 11:14",    state: "sent",      text: "@grimer A fair point — we're tracking that one in the backlog as L-203. Filed your username so we can ping when it ships." },
];

const CHANNEL_COLOUR: Record<Channel, string> = {
  mastodon: "#6364ff",
  x:        "#fff",
  linkedin: "#0a66c2",
  bluesky:  "#0285ff",
};

const STATE_COLOUR: Record<PostState, string> = {
  scheduled: "var(--brand-300)",
  sent:      "var(--success-400)",
  draft:     "var(--fg-3)",
};

class LthnViewSocial extends LitElement {
  static readonly properties = {
    w:        { type: Number },
    h:        { type: Number },
    embedded: { type: Boolean, reflect: true },
    channelFilter: { state: true },
  };
  declare w: number;
  declare h: number;
  declare embedded: boolean;
  /** Optional channel filter — clicking the channel chip in the toolbar
   *  narrows the queue. Empty = all channels. */
  declare channelFilter: Channel | "";

  constructor() {
    super();
    this.w = 1180;
    this.h = 720;
    this.embedded = false;
    this.channelFilter = "";
  }

  createRenderRoot() { return this; }

  _setChannel(c: Channel | "") {
    this.channelFilter = this.channelFilter === c ? "" : c;
  }

  render() {
    const posts = this.channelFilter
      ? FIXTURE_POSTS.filter(p => p.ch.includes(this.channelFilter as Channel))
      : FIXTURE_POSTS;
    const scheduledCount = FIXTURE_POSTS.filter(p => p.state === "scheduled").length;
    const channels: Channel[] = ["mastodon", "x", "linkedin", "bluesky"];

    const toolbar = html`
      <div class="lthn-view-social-channels" style="display:flex; gap:8px; align-items:center;">
        <span style="font-family:var(--font-mono); font-size:10px; color:var(--fg-3); letter-spacing:0.10em; text-transform:uppercase;">channels</span>
        ${channels.map(c => html`
          <button
            class="lthn-view-social-channel-chip"
            data-channel=${c}
            data-active=${this.channelFilter === c}
            @click=${() => this._setChannel(c)}
            style="--wails-draggable: no-drag; cursor:pointer; padding:4px 10px; border-radius:999px; border:1px solid ${this.channelFilter === c ? CHANNEL_COLOUR[c] : "rgba(255,255,255,0.08)"}; background:${this.channelFilter === c ? CHANNEL_COLOUR[c] + "22" : "transparent"}; color:var(--fg-1); font-family:var(--font-mono); font-size:10.5px; letter-spacing:0.04em;">
            ${c}
          </button>
        `)}
      </div>
    `;

    const body = html`
      <div class="lthn-view-social" style="flex:1; padding:18px 22px; display:flex; flex-direction:column; gap:14px; overflow:auto;">
        ${posts.map(p => html`
          <div class="lthn-view-social-post" data-state=${p.state} style="padding:16px 18px; border-radius:10px; background:rgba(255,255,255,0.025); border:1px solid rgba(255,255,255,0.06);">
            <div style="display:flex; align-items:center; gap:10px; margin-bottom:10px;">
              <div style="display:flex; gap:6px;">
                ${p.ch.map(c => html`<span class="lthn-view-social-chan" data-channel=${c} style="width:22px; height:22px; border-radius:6px; background:${CHANNEL_COLOUR[c]}; opacity:0.85; display:flex; align-items:center; justify-content:center; font-family:var(--font-mono); font-size:9.5px; color:#000; font-weight:600; letter-spacing:-0.02em;">${c[0].toUpperCase()}</span>`)}
              </div>
              <span style="font-family:var(--font-mono); font-size:10.5px; color:var(--fg-3);">${p.when}</span>
              <div style="flex:1"></div>
              <span style="font-family:var(--font-mono); font-size:10px; padding:3px 9px; border-radius:999px; background:${STATE_COLOUR[p.state]}1a; color:${STATE_COLOUR[p.state]}; letter-spacing:0.06em; text-transform:uppercase;">${p.state}</span>
            </div>
            <div style="font-size:13px; color:var(--fg-0); line-height:1.5;">${p.text}</div>
            ${p.attach ? html`<div class="lthn-view-social-attach" style="margin-top:10px; padding:8px 12px; border-radius:6px; background:rgba(64,193,197,0.06); border:1px dashed rgba(64,193,197,0.22); font-family:var(--font-mono); font-size:10.5px; color:var(--brand-300); display:inline-flex; align-items:center; gap:8px;"><i class="fa-regular fa-image" style="font-size:11px;"></i>${p.attach} attached · 1280×720</div>` : nothing}
          </div>
        `)}
      </div>
    `;

    return renderChrome({
      title:    "Social queue",
      subtitle: `${posts.length} posts · ${scheduledCount} scheduled`,
      w: this.w, h: this.h,
      embedded: this.embedded,
      toolbar,
      body,
      footer: html`channels: Mastodon, X, LinkedIn, Bluesky · ⌘N to compose · drafts saved locally only`,
    });
  }
}

customElements.define("lthn-view-social", LthnViewSocial);

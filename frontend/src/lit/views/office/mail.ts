// SPDX-Licence-Identifier: EUPL-1.2
// Office · Mail — <lthn-view-mail>
//
// IMAP inbox triaged by Vi. Three-pane layout: folders (left) · threads
// (centre) · selected message + Vi callout (right). Calm, observational
// tone; the Vi callout is a soft suggestion, never a celebration.
//
// Two-shell: standalone (with chrome) when spawned directly, embedded (no
// chrome) when mounted inside <lthn-app-shell>. Mirrors the welcome /
// settings window pattern in ../../ops/welcome-window.ts.
//
// Data: fixture arrays only for v1. Real source is a future Go service
// at `pkg/mail` — an IMAP client that talks to host.uk.com mailboxes
// and runs Vi triage on-device (no remote LLM call). The Vi suggestion
// is generated locally by the Lemma/Violet inference sidecar, never
// leaves the machine. When bindings land, swap FIXTURE_THREADS for a
// fetch (mirror chat-window's pattern).

import { LitElement, html, nothing } from "lit";
import { renderChrome } from "../../chrome";

/** Shape of one inbox folder. Mirrors the future pkg/mail folder API. */
interface MailFolder {
  /** Folder label as shown to the user (UK English). */
  label:  string;
  /** Unread count — 0 means no badge. */
  unread: number;
  /** Is this the currently-selected folder? */
  active: boolean;
}

/** Shape of one inbox thread. Mirrors the future pkg/mail thread API.
 *  Strings stay opaque ("14:32", "2 d") because the source-of-truth is
 *  the mail service's locale-aware formatter, not browser arithmetic. */
interface MailThread {
  /** Sender display name — "Ada Penley", "GitHub", "Mei (team)" etc. */
  from:   string;
  /** Subject line — RFC 5322 Subject header, decoded. */
  subj:   string;
  /** Human-readable received time ("now", "14:32", "yest", "2 d"). */
  when:   string;
  /** Has the thread been read yet? */
  unread: boolean;
  /** Snippet preview — first ~120 chars of the body for the list row. */
  body:   string;
}

const FIXTURE_FOLDERS: MailFolder[] = [
  { label: "Inbox",   unread: 24, active: true  },
  { label: "Sent",    unread: 0,  active: false },
  { label: "Drafts",  unread: 3,  active: false },
  { label: "Archive", unread: 0,  active: false },
  { label: "Trash",   unread: 0,  active: false },
];

const FIXTURE_THREADS: MailThread[] = [
  { from: "Ada Penley",   subj: "Re: SOW v2 — DPO feedback",         when: "now",   unread: true,  body: "Looking forward to seeing the proposal. We'll need to loop in our DPO for the privacy posture review..." },
  { from: "GitHub",       subj: "[lethean/desktop] #482 reviewed",   when: "14:32", unread: true,  body: "Tobi requested changes on your pull request. 3 comments · 2 files." },
  { from: "Calliope VC",  subj: "Pitch follow-up",                   when: "yest",  unread: false, body: "Thanks for the call. Sending the diligence questionnaire your way later this week..." },
  { from: "Mei (team)",   subj: "Linux packaging — blocker",         when: "yest",  unread: false, body: "Hit a snag with the AppImage signing flow. Have a workaround but flagging early..." },
  { from: "r/LocalLLaMA", subj: "Your post got 412 upvotes",         when: "2 d",   unread: false, body: "You reached r/LocalLLaMA's front page. Top comment thread now has 84 replies..." },
];

class LthnViewMail extends LitElement {
  static readonly properties = {
    w:        { type: Number },
    h:        { type: Number },
    embedded: { type: Boolean, reflect: true },
    selectedIdx: { state: true },
    folders:  { state: true },
    threads:  { state: true },
    loading:  { state: true },
  };
  declare w: number;
  declare h: number;
  declare embedded: boolean;
  declare selectedIdx: number;
  declare folders: MailFolder[];
  declare threads: MailThread[];
  declare loading: boolean;
  private _timer: ReturnType<typeof setInterval> | null = null;

  constructor() {
    super();
    this.w = 1280;
    this.h = 720;
    this.embedded = false;
    this.selectedIdx = 0;
    this.folders = FIXTURE_FOLDERS;
    this.threads = FIXTURE_THREADS;
    this.loading = false;
  }

  createRenderRoot() { return this; }

  async connectedCallback() {
    super.connectedCallback();
    await this._loadFromBackend();
    this._timer = setInterval(() => { void this._loadFromBackend(); }, 60_000);
  }

  disconnectedCallback() {
    if (this._timer) { clearInterval(this._timer); this._timer = null; }
    super.disconnectedCallback();
  }

  async _loadFromBackend(): Promise<void> {
    if (this.loading) return;
    this.loading = true;
    try {
      const svc = await import("@desktop/office/mail/service").catch(() => null);
      if (!svc || typeof (svc as { ListFolders?: unknown }).ListFolders !== "function") {
        return;
      }
      const mailSvc = svc as {
        ListFolders: () => Promise<{ Value?: { folders?: MailFolder[] } }>;
        ListThreads: (input: { folderSlug: string; limit?: number }) => Promise<{ Value?: { threads?: MailThread[] } }>;
      };

      const [fr, tr] = await Promise.all([
        mailSvc.ListFolders(),
        mailSvc.ListThreads({ folderSlug: "inbox", limit: 50 }),
      ]);

      const folders = fr?.Value?.folders;
      if (folders && folders.length > 0) {
        this.folders = folders;
      }

      const threads = tr?.Value?.threads;
      if (threads && threads.length > 0) {
        this.threads = threads;
      }
    } catch {
      // Binding unavailable — keep fixture data.
    } finally {
      this.loading = false;
    }
  }

  /** The thread currently shown in the reading-pane. Defensive index
   *  clamp so a bad selectedIdx (e.g. set to -1 by a test) falls back
   *  to the top thread instead of throwing. */
  _selected(): MailThread {
    const idx = Math.max(0, Math.min(this.selectedIdx, this.threads.length - 1));
    return this.threads[idx];
  }

  /** Total unread count across all threads. */
  _unreadCount(): number {
    return this.threads.filter(t => t.unread).length;
  }

  render() {
    const unread = this._unreadCount();
    const selected = this._selected();

    const body = html`
      <div class="lthn-view-mail" style="flex:1; display:grid; grid-template-columns: 160px 360px 1fr; min-height:0;">
        <!-- folders rail -->
        <aside class="lthn-view-mail-folders" style="background:rgba(0,0,0,0.20); border-right:1px solid rgba(255,255,255,0.05); padding:14px 8px; display:flex; flex-direction:column; gap:2px;">
          ${this.folders.map(f => html`
            <div class="lthn-view-mail-folder"
                 data-label=${f.label}
                 ?data-active=${f.active}
                 style="display:flex; align-items:center; gap:10px; padding:8px 12px; border-radius:6px;
                        background:${f.active ? "rgba(64,193,197,0.10)" : "transparent"};
                        border:1px solid ${f.active ? "rgba(64,193,197,0.22)" : "transparent"};
                        color:${f.active ? "var(--brand-300)" : "var(--fg-2)"};
                        font-size:12.5px; cursor:pointer;">
              <span style="flex:1;">${f.label}</span>
              ${f.unread > 0 ? html`<span style="font-family:var(--font-mono); font-size:10px; color:var(--fg-3);">${f.unread}</span>` : nothing}
            </div>
          `)}
        </aside>

        <!-- threads list -->
        <aside class="lthn-view-mail-threads" style="background:rgba(0,0,0,0.12); border-right:1px solid rgba(255,255,255,0.05); overflow:auto;">
          ${this.threads.map((t, i) => {
            const sel = i === this.selectedIdx;
            return html`
              <div class="lthn-view-mail-thread"
                   data-idx=${i}
                   ?data-selected=${sel}
                   ?data-unread=${t.unread}
                   @click=${() => { this.selectedIdx = i; }}
                   style="padding:14px 16px;
                          border-bottom:1px solid rgba(255,255,255,0.04);
                          border-left:${sel ? "3px solid var(--brand-400)" : "3px solid transparent"};
                          background:${sel ? "rgba(64,193,197,0.04)" : "transparent"};
                          cursor:pointer;
                          --wails-draggable: no-drag;">
                <div style="display:flex; justify-content:space-between; align-items:center; gap:8px;">
                  <span style="font-size:12.5px; color:var(--fg-0); font-weight:${t.unread ? 600 : 400};">${t.from}</span>
                  <span style="font-family:var(--font-mono); font-size:10px; color:var(--fg-3);">${t.when}</span>
                </div>
                <div style="font-size:12px; color:${t.unread ? "var(--fg-1)" : "var(--fg-2)"}; font-weight:${t.unread ? 500 : 400}; margin-top:4px;">${t.subj}</div>
                <div style="font-size:11px; color:var(--fg-3); margin-top:4px; overflow:hidden; text-overflow:ellipsis; white-space:nowrap;">${t.body}</div>
              </div>
            `;
          })}
        </aside>

        <!-- reading pane -->
        <main class="lthn-view-mail-reading" style="padding:24px 30px; overflow:auto;">
          <div style="display:flex; align-items:center; gap:10px; margin-bottom:6px;">
            <h2 style="margin:0; font-size:22px; color:var(--fg-0); letter-spacing:-0.02em; font-weight:600;">${selected.subj}</h2>
          </div>
          <div style="font-family:var(--font-mono); font-size:11px; color:var(--fg-3); margin-bottom:18px;">
            From ${selected.from} · ${selected.when}
          </div>
          <div style="font-size:13.5px; color:var(--fg-1); line-height:1.7;">
            <p>${selected.body}</p>
          </div>

          <!-- Vi triage callout — soft, observational, never theatrical.
               The suggestion is generated locally by Lemma/Violet (the
               on-device inference sidecar) so the mail body never leaves
               the machine. See HANDOVER-VIEWS.md: "Vi cross-cuts views". -->
          <div class="lthn-view-mail-vi" style="margin-top:24px; padding:14px 16px; border-radius:10px; background:rgba(64,193,197,0.06); border:1px solid rgba(64,193,197,0.22); display:flex; align-items:center; gap:16px;">
            <i class="fa-solid fa-feather" style="color:var(--brand-300); font-size:13px;"></i>
            <span style="flex:1; font-size:12.5px; color:var(--fg-1); line-height:1.55;">
              <span style="font-family:var(--font-mono); color:var(--brand-300); font-size:10px; letter-spacing:0.10em; text-transform:uppercase;">Vi</span> · Suggested reply ready. Acknowledges, confirms timing, attaches updated context. Want me to send?
            </span>
            <lthn-btn tone="ghost" size="sm">View draft</lthn-btn>
          </div>
        </main>
      </div>
    `;

    return renderChrome({
      title: "Mail",
      subtitle: `${unread} unread`,
      w: this.w, h: this.h,
      body,
      footer: html`${this.threads.length} threads · IMAP via host.uk.com · Vi triage on-device only`,
      embedded: this.embedded,
    });
  }
}

customElements.define("lthn-view-mail", LthnViewMail);

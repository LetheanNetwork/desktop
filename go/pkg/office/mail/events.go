// SPDX-Licence-Identifier: EUPL-1.2

// Event name constants and typed-event infrastructure for the mail surface.
// v1 reserved EventThreadReceived + EventThreadRead.
// v2 promotes those and adds EventSent, EventResyncStarted/Completed,
// EventAttachmentDownloaded, EventConnectionFailed, EventSessionLocked.
//
// Audit recorder (RFC.stage-f §3.2) subscribes via c.RegisterAction —
// passwords NEVER traverse this bus (only structural metadata).

package mail

import core "dappco.re/go"

const (
	// EventThreadReceived fires when a new thread arrives via IMAP fetch.
	EventThreadReceived = "mail.thread.received"

	// EventThreadRead fires when a thread's read state changes.
	EventThreadRead = "mail.thread.read"

	// EventSent fires on successful SMTP send.
	EventSent = "mail.sent"

	// EventResyncStarted fires when UIDVALIDITY rotation triggers a full resync (§2.2).
	EventResyncStarted = "mail.resync.started"

	// EventResyncCompleted fires when the full resync fetch completes (§2.2).
	EventResyncCompleted = "mail.resync.completed"

	// EventAttachmentDownloaded fires when DownloadAttachment completes (§4.3).
	EventAttachmentDownloaded = "mail.attachment.downloaded"

	// EventConnectionFailed fires when an IMAP/SMTP connection attempt fails (§2.3 backoff).
	EventConnectionFailed = "mail.connection.failed"

	// EventSessionLocked fires once per lock transition (§6 banner contract).
	EventSessionLocked = "mail.session.locked"

	// EventPathsAppendFellBackToRewrite fires when appendThreadRecord
	// trips the Darwin PIPE_BUF size limit and lands the record via
	// the AtomicWriteWithVersion rewrite-merge fallback instead of
	// AtomicAppendLine. Cerberus #9 Concern 1.A — on Darwin
	// (PIPE_BUF=512) this is the NORMAL case for thread records with
	// long subjects + "Re: Re:" chains, NOT an edge. Distinguishable
	// from primitive-rejection so dashboards can show "fallback
	// engaged" vs "primitive rejected, data lost". Promoted from the
	// in-package fellBackToRewriteCode literal in imap.go (Mantis
	// #1560) so future cross-package consumers get a typed handle.
	EventPathsAppendFellBackToRewrite = "paths.append.fell_back_to_rewrite"
)

// MailEvent is the typed payload for all mail events.
// Passwords NEVER appear in any field — structural metadata only.
//
// Meta carries free-form per-event fields (path_hash, record-size
// counters, etc.) introduced for Cerberus #9 Concern 1.A's fallback
// observability per Mantis #1559. Keys are stable strings the audit
// substrate's redactor sees verbatim — passphrases / token bytes
// MUST NEVER be placed here.
//
// Usage example:
//
//	mail.Subscribe(c, func(_ *core.Core, ev mail.MailEvent) {
//	    _ = ev.Kind
//	    _ = ev.AccountName
//	    _ = ev.Meta["path_hash"]
//	})
type MailEvent struct {
	Kind        string         `json:"kind"`
	AccountName string         `json:"account_name"`
	FolderSlug  string         `json:"folder_slug,omitempty"`
	ThreadID    string         `json:"thread_id,omitempty"`
	Error       string         `json:"error,omitempty"`
	At          core.Time      `json:"at"`
	Meta        map[string]any `json:"meta,omitempty"`
}

// Subscribe registers fn to receive all MailEvent payloads emitted by the
// mail service. Uses c.RegisterAction internally — the subscription lives
// for the Core container's lifetime.
//
// Usage example:
//
//	mail.Subscribe(c, func(c *core.Core, ev mail.MailEvent) {
//	    if ev.Kind == mail.EventThreadReceived { … }
//	})
func Subscribe(c *core.Core, fn func(*core.Core, MailEvent)) {
	c.RegisterAction(func(c *core.Core, msg core.Message) core.Result {
		ev, ok := msg.(MailEvent)
		if !ok {
			return core.Ok(nil)
		}
		fn(c, ev)
		return core.Ok(nil)
	})
}

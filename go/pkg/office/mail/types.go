// SPDX-Licence-Identifier: EUPL-1.2

// Package mail is the lthn-side v1 mailbox catalogue service.
// Reads folder and thread metadata from
// ~/Lethean/office/mail/{folder-slug}/threads.md — Trix-style YAML
// frontmatter per thread; no message body stored.
//
// v1 scope: catalogue only — folder list + thread headers.
// No IMAP fetch, SMTP send, MIME parsing, or attachment download.
// v2 (separate Mantis ticket) adds live IMAP integration.
//
// Wire shape matches MailFolder + MailThread consumed by
// <lthn-view-mail> in the Office role view.
//
// Usage example (Wails):
//
//	r := mailSvc.ListFolders()
//	if r.OK { out := r.Value.(mail.ListFoldersOutput) }
package mail

import core "dappco.re/go"

// MailFolder is the JSON wire type for a mailbox folder.
// Matches the MailFolder interface in
// frontend/src/lit/views/office/mail.ts exactly.
//
// Usage example:
//
//	folder := mail.MailFolder{Label: "Inbox", Slug: "inbox", Unread: 24, Active: true}
type MailFolder struct {
	// Label is the display name shown to the user (e.g. "Inbox").
	Label string `json:"label"`

	// Slug is the directory name under ~/Lethean/office/mail/.
	Slug string `json:"slug"`

	// Unread is the count of unread threads in this folder.
	Unread int `json:"unread"`

	// Active is true when this is the currently-selected folder.
	// Set by the Wails method based on the caller's FolderSlug input.
	Active bool `json:"active"`
}

// MailThread is the JSON wire type for a single email thread.
// Matches the MailThread interface in
// frontend/src/lit/views/office/mail.ts exactly.
//
// Usage example:
//
//	thread := mail.MailThread{
//	    ID: "abc123", From: "Ada Penley",
//	    Subj: "Re: SOW v2", When: "now", Unread: true,
//	    Body: "Looking forward to seeing the proposal...",
//	}
type MailThread struct {
	// ID is the opaque thread identifier from the frontmatter.
	ID string `json:"id"`

	// From is the sender display name.
	From string `json:"from"`

	// Subj is the decoded RFC 5322 Subject header.
	Subj string `json:"subj"`

	// When is the human-readable received time:
	// "now" (< 5 min), "HH:MM" (today), "yest" (yesterday), "X d" (older).
	When string `json:"when"`

	// Unread is true when the thread has not been read yet.
	Unread bool `json:"unread"`

	// Body is the snippet preview (first ~120 chars of the message body).
	Body string `json:"body"`
}

// MailThreadRecord is the internal persistence type decoded from one
// YAML document block inside a folder's threads.md file.
//
// Usage example:
//
//	rec := mail.MailThreadRecord{
//	    ID: "abc123", From: "Ada Penley",
//	    Subj: "Re: SOW v2", LastTouched: core.Now(),
//	    Unread: true, Snippet: "Looking forward...",
//	}
type MailThreadRecord struct {
	// ID is the opaque thread identifier.
	ID string `yaml:"id"`

	// From is the sender display name.
	From string `yaml:"from"`

	// Subj is the subject line.
	Subj string `yaml:"subject"`

	// LastTouched is when the thread was last received / updated.
	LastTouched core.Time `yaml:"last_touched"`

	// Unread is the read state.
	Unread bool `yaml:"unread"`

	// Snippet is the first ~120 chars of the body, pre-truncated by
	// whoever wrote the threads.md file.
	Snippet string `yaml:"snippet"`
}

// ListFoldersOutput is the ListFolders response envelope.
//
// Usage example:
//
//	out := r.Value.(mail.ListFoldersOutput)
//	for _, f := range out.Folders { _ = f.Label }
type ListFoldersOutput struct {
	// Folders is the list of mail folders found in the mail directory.
	Folders []MailFolder `json:"folders"`
}

// ListThreadsInput drives the ListThreads method.
//
// Usage example:
//
//	r := svc.ListThreads(mail.ListThreadsInput{FolderSlug: "inbox", Limit: 20})
type ListThreadsInput struct {
	// FolderSlug is the directory name of the folder to read.
	FolderSlug string `json:"folderSlug"`

	// Limit caps the result count. Zero defaults to 50.
	Limit int `json:"limit,omitempty"`
}

// ListThreadsOutput is the ListThreads response envelope.
//
// Usage example:
//
//	out := r.Value.(mail.ListThreadsOutput)
//	for _, t := range out.Threads { _ = t.From }
type ListThreadsOutput struct {
	// Threads is the sorted (newest-first) thread list for the folder.
	Threads []MailThread `json:"threads"`

	// Total is the unfiltered thread count in the folder.
	Total int `json:"total"`

	// Unread is the count of unread threads in the folder.
	Unread int `json:"unread"`
}

// folderLabels maps slug → display label for the five canonical folders.
// Unknown slugs use a title-cased fallback generated at runtime.
var folderLabels = map[string]string{
	"inbox":   "Inbox",
	"sent":    "Sent",
	"drafts":  "Drafts",
	"archive": "Archive",
	"trash":   "Trash",
}

// folderLabel returns the display label for a slug.
// Falls back to slug with first letter upper-cased when not in the map.
func folderLabel(slug string) string {
	if label, ok := folderLabels[slug]; ok {
		return label
	}
	if len(slug) == 0 {
		return slug
	}
	first := slug[0]
	if first >= 'a' && first <= 'z' {
		return string(first-32) + slug[1:]
	}
	return slug
}

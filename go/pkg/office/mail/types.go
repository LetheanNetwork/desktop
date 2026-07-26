// SPDX-Licence-Identifier: EUPL-1.2

// Package mail is the lthn-side v2 mailbox service.
// v1 scope (catalogue only: folder list + thread headers) is unchanged.
// v2 layers live IMAP fetch, SMTP send, MIME parsing, and asymmetrically-
// encrypted credential storage ON TOP of the v1 threads.md files.
//
// Credential storage (§1): _accounts.enc is PGP-asymmetrically encrypted
// to the user's public key (Enchantrix pgp.Encrypt). Decryption requires
// the unlocked private key from pkg/account. Writes work without unlock;
// reads gate on unlock.
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
// frontend-ng/src/app/desktop/surfaces/office/mail.ts exactly.
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
// frontend-ng/src/app/desktop/surfaces/office/mail.ts exactly.
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

	// IMAPUID is the IMAP UID of the message (v2 only). Zero for v1 records.
	IMAPUID uint32 `yaml:"imap_uid,omitempty"`

	// AllowHTML is the per-thread HTML-rendering opt-in (§4.2 — stored
	// so the choice persists across FetchBody calls).
	AllowHTML bool `yaml:"allow_html,omitempty"`
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

// --- v2 credential types (§1.4) ---

// IMAPConfig holds IMAP server connection details for one account.
//
// Usage example:
//
//	cfg := mail.IMAPConfig{Host: "imap.fastmail.com", Port: 993, User: "me@fastmail.com", TLS: true}
type IMAPConfig struct {
	Host string `json:"host"`
	Port int    `json:"port"`
	User string `json:"user"`
	TLS  bool   `json:"tls"`
}

// SMTPConfig holds SMTP server connection details for one account.
// Either TLS (implicit) or TLSStarttls (STARTTLS) must be true per §3.
//
// Usage example:
//
//	cfg := mail.SMTPConfig{Host: "smtp.fastmail.com", Port: 587, User: "me@fastmail.com", TLSStarttls: true}
type SMTPConfig struct {
	Host        string `json:"host"`
	Port        int    `json:"port"`
	User        string `json:"user"`
	TLSStarttls bool   `json:"tls_starttls"`
}

// AuthSpec describes authentication credentials for an account.
// Secret is INPUT ONLY — never returned by ListAccounts (§1.4).
//
// Usage example:
//
//	auth := mail.AuthSpec{Kind: "appPassword", Secret: "app-password-here"}
type AuthSpec struct {
	Kind   string `json:"kind"`
	Secret string `json:"secret"`
}

// AccountInput is the SaveAccount request body.
//
// Usage example:
//
//	r := svc.SaveAccount(mail.AccountInput{Name: "personal", IMAP: imapCfg, SMTP: smtpCfg, Auth: auth})
type AccountInput struct {
	Name string     `json:"name"`
	IMAP IMAPConfig `json:"imap"`
	SMTP SMTPConfig `json:"smtp"`
	Auth AuthSpec   `json:"auth"`
}

// MailAccount is the on-disk JSON record (per entry in _accounts.enc).
// Secret is zeroed before any return to callers (§1.4).
//
// Usage example:
//
//	var acct mail.MailAccount
//	_ = core.JSONUnmarshal(plaintext, &acct)
type MailAccount struct {
	Name string     `json:"name"`
	IMAP IMAPConfig `json:"imap"`
	SMTP SMTPConfig `json:"smtp"`
	Auth AuthSpec   `json:"auth"`
}

// --- v2 IMAP fetch types (§2) ---

// FetchOnceInput drives the manual-refresh endpoint.
//
// Usage example:
//
//	r := svc.FetchOnce(mail.FetchOnceInput{AccountName: "personal", FolderSlug: "inbox"})
type FetchOnceInput struct {
	AccountName string `json:"account_name"`
	FolderSlug  string `json:"folder_slug"`
}

// FolderState records the IMAP state for one (account, folder) pair.
// Written to ~/Lethean/office/mail/_state/{account}/{folder}.json.
//
// Usage example:
//
//	state := mail.FolderState{UIDValidity: 1234567, LastUIDSeen: 42, LastFetchTS: core.Now()}
type FolderState struct {
	UIDValidity uint32    `json:"uid_validity"`
	LastUIDSeen uint32    `json:"last_uid_seen"`
	LastFetchTS core.Time `json:"last_fetch_ts"`
}

// --- v2 SMTP send types (§3) ---

// SendAttachment is one file to attach to an outgoing message.
// Path must be absolute and under ~/Lethean/ per §3.1.
//
// Usage example:
//
//	att := mail.SendAttachment{Filename: "invoice.pdf", Path: "/Users/x/Lethean/docs/invoice.pdf"}
type SendAttachment struct {
	Filename string `json:"filename"`
	Path     string `json:"path"`
	MIME     string `json:"mime,omitempty"`
}

// SendInput drives the SMTP send endpoint (§3).
//
// Usage example:
//
//	r := svc.Send(mail.SendInput{AccountName: "personal", To: []string{"ada@lthn.io"}, Subject: "Hello", BodyText: "Hi!"})
type SendInput struct {
	AccountName string           `json:"account_name"`
	To          []string         `json:"to"`
	Cc          []string         `json:"cc,omitempty"`
	Bcc         []string         `json:"bcc,omitempty"`
	Subject     string           `json:"subject"`
	BodyText    string           `json:"body_text"`
	BodyHTML    string           `json:"body_html,omitempty"`
	Attachments []SendAttachment `json:"attachments,omitempty"`
	InReplyTo   string           `json:"in_reply_to,omitempty"`
}

// --- v2 MIME / body fetch types (§4) ---

// FetchBodyInput drives the on-demand body fetch (§4.1).
//
// Usage example:
//
//	r := svc.FetchBody(mail.FetchBodyInput{AccountName: "personal", FolderSlug: "inbox", ThreadID: "abc123"})
type FetchBodyInput struct {
	AccountName string `json:"account_name"`
	FolderSlug  string `json:"folder_slug"`
	ThreadID    string `json:"thread_id"`
	AllowHTML   bool   `json:"allow_html,omitempty"`
}

// AttachmentMeta is the metadata for one MIME part (§4.1).
// Bytes are fetched separately via DownloadAttachment.
//
// Usage example:
//
//	for _, a := range out.Attachments { _ = a.Filename }
type AttachmentMeta struct {
	Filename string `json:"filename"`
	MIME     string `json:"mime"`
	SizeB    int64  `json:"size_b"`
	PartID   string `json:"part_id"`
}

// FetchBodyOutput is the FetchBody response (§4.1).
// HTML is empty by default (AllowHTML=false per §4.2 Q3 ruling).
//
// Usage example:
//
//	out := r.Value.(mail.FetchBodyOutput)
//	_ = out.Plain
type FetchBodyOutput struct {
	Plain       string           `json:"plain"`
	HTML        string           `json:"html"`
	Attachments []AttachmentMeta `json:"attachments"`
}

// DownloadAttachmentInput drives the attachment-download endpoint (§4.3).
// DestFilename is validated via paths.IsValidID (slug-style, no traversal).
//
// Usage example:
//
//	r := svc.DownloadAttachment(mail.DownloadAttachmentInput{
//	    AccountName: "personal", FolderSlug: "inbox", ThreadID: "abc123",
//	    PartID: "2.1", DestFilename: "invoice-jan.pdf",
//	})
type DownloadAttachmentInput struct {
	AccountName  string `json:"account_name"`
	FolderSlug   string `json:"folder_slug"`
	ThreadID     string `json:"thread_id"`
	PartID       string `json:"part_id"`
	DestFilename string `json:"dest_filename"`
}

// --- folder-label helpers (v1, unchanged) ---

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

// deprecatedProviders is the set of mail providers for which
// Auth.Kind=="password" is rejected per §1.5 Q2 ruling.
var deprecatedProviders = map[string]bool{
	"gmail.com":   true,
	"outlook.com": true,
	"live.com":    true,
	"hotmail.com": true,
	"yahoo.com":   true,
	"aol.com":     true,
}

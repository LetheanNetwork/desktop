// SPDX-Licence-Identifier: EUPL-1.2

// Service — Office mail service.
// v1: catalogue (read ~/Lethean/office/mail/{folder-slug}/threads.md).
// v2: live IMAP fetch + SMTP send + MIME parsing + asymmetric cred storage.
//
// Lifecycle:
//   - Register(c)   wires into the Core container
//   - ServiceName() returns "Mail" for the Wails namespace
//
// Credential encryption (§1.2):
//   - SaveAccount writes _accounts.enc via Enchantrix pgp.Encrypt(pubKey, …) —
//     requires the user's public key (always readable from disk).
//   - ListAccounts / polling reads via pgp.Decrypt(privKey, …) —
//     requires the unlocked private key from pkg/account.
//   - SetAccountService wires the dependency post-construction.
//
// Locked-state discipline (§6):
//   - PausePolling / ResumePolling hook into Stage E lock/unlock events.
//   - Every read-gated method returns mail.session.locked when paused.
//   - NO silent fallthrough (§6 HIGH-mail-2 Cerberus ruling).
//
// All I/O via CoreGO wrappers: core.ReadFile / core.MkdirAll /
// core.ReadDir / core.DirFS / core.WriteFile / core.Rename / core.Stat.
// Banned: os, path/filepath, strings, encoding/json, fmt, log, errors.

package mail

import (
	"sync"
	"sync/atomic"

	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/paths"
	"forge.lthn.ai/Snider/Enchantrix/pkg/crypt/std/pgp"
	"gopkg.in/yaml.v3"
)

// AccountProvider abstracts pkg/account.Service for private-key access.
// The mail service depends on it for decrypt; the interface avoids an
// import cycle (lib must not import consumer — AX principle 8).
//
// Usage example:
//
//	svc.SetAccountService(accountSvc)
type AccountProvider interface {
	PrivateKeyFor(accountID string) ([]byte, bool)
	PublicKeyFor(accountID string) ([]byte, bool)
	DefaultAccountID() string
}

// Service owns the Office mail catalogue surface.
//
// Usage example:
//
//	svc := mail.NewService(c)
type Service struct {
	core    *core.Core
	pgp     *pgp.Service
	account AccountProvider

	// paused is true while Stage E is locked (§6).
	paused atomic.Bool

	// mu guards pooled IMAP connections (§2.3).
	mu sync.Mutex

	// pool holds open IMAP connections keyed by "account/folder".
	// Pool cap = poolMax per account (default 5 per §2.3).
	pool map[string]*imapConn

	// backoff tracks per-account retry delay index (§2.3 exponential backoff).
	backoff map[string]int

	// deferredPolls counts folders that would have polled while locked (§6 banner).
	deferredPolls int
}

// imapConn is one pooled IMAP connection with its LRU timestamp.
type imapConn struct {
	conn    interface{ Close() error }
	lastUse core.Time
}

const (
	poolMax         = 5
	accountsEncFile = "_accounts.enc"
	stateDir        = "_state"
)

// backoffTable is the exponential backoff schedule for IMAP failures (§2.3).
// Values are in seconds; the last entry repeats.
var backoffTable = []core.Duration{
	30 * core.Second,
	core.Minute,
	2 * core.Minute,
	5 * core.Minute,
	10 * core.Minute,
}

// NewService constructs the mail service against a Core container.
// Wired via core.WithName("office-mail", mail.Register) in app.go.
//
// Usage example:
//
//	svc := mail.NewService(c)
func NewService(c *core.Core) *Service {
	return &Service{
		core:    c,
		pgp:     pgp.NewService(),
		pool:    map[string]*imapConn{},
		backoff: map[string]int{},
	}
}

// Register constructs the mail service for Core registration.
//
// Usage example:
//
//	core.New(core.WithName("office-mail", mail.Register))
func Register(c *core.Core) core.Result {
	return core.Ok(NewService(c))
}

// ServiceName labels the binding namespace exposed to JS.
// Wails binds methods as "Mail.ListFolders()" etc.
func (s *Service) ServiceName() string { return "Mail" }

// SetAccountService wires the AccountProvider dependency.
// Must be called after NewService when both services are live.
// app.go calls this post-construction (same pattern as account.SetServerKey).
//
// Usage example:
//
//	mailSvc.SetAccountService(accountSvc)
func (s *Service) SetAccountService(a AccountProvider) {
	s.account = a
}

// errLocked returns the standard locked-session result (§6).
// Every read-gated method returns this — no silent fallthrough.
func (s *Service) errLocked() core.Result {
	return core.Fail(core.NewCode("mail.session.locked",
		"sign in to continue receiving mail"))
}

// assertUnlocked returns an error result when the session is paused.
func (s *Service) assertUnlocked() core.Result {
	if s.paused.Load() {
		return s.errLocked()
	}
	return core.Ok(nil)
}

// PausePolling pauses all read-gated operations. Called by Stage E lock event.
// Fires EventSessionLocked with the deferred-poll count (§6 banner contract).
//
// Usage example:
//
//	mail.Subscribe(c, func(_ *core.Core, ev mail.MailEvent) {})
//	svc.PausePolling(c)
func (s *Service) PausePolling(c *core.Core) core.Result {
	s.mu.Lock()
	s.paused.Store(true)
	deferred := s.deferredPolls
	s.mu.Unlock()

	c.ACTION(MailEvent{
		Kind:  EventSessionLocked,
		Error: core.Sprintf("%d folders queued for sync", deferred),
		At:    core.Now(),
	})
	return core.Ok(nil)
}

// ResumePolling resumes operations after Stage E unlock.
// Resets deferred-poll counter; callers must trigger immediate fetch.
//
// Usage example:
//
//	svc.ResumePolling(c)
func (s *Service) ResumePolling(c *core.Core) core.Result {
	s.mu.Lock()
	s.paused.Store(false)
	s.deferredPolls = 0
	s.mu.Unlock()
	return core.Ok(nil)
}

// StartPolling wires Stage E unlock/lock event subscriptions.
// Called at boot to hook into the event bus.
//
// Usage example:
//
//	svc.StartPolling(c)
func (s *Service) StartPolling(c *core.Core) core.Result {
	return core.Ok(nil)
}

// mailDir resolves ~/Lethean/office/mail/ and creates it if missing.
// Mode 0o700 — mail metadata is PII (Cerberus #1487 mandate).
func mailDir() core.Result {
	root := paths.Root()
	if !root.OK {
		return root
	}
	dir := core.PathJoin(root.Value.(string), "office", "mail")
	if r := core.MkdirAll(dir, 0o700); !r.OK {
		return r
	}
	return core.Ok(dir)
}

// folderDir resolves ~/Lethean/office/mail/{slug}/ and creates it.
// Validates slug via paths.IsValidID before path join.
func folderDir(slug string) core.Result {
	if err := paths.IsValidID(slug); err != nil {
		return core.Fail(core.E("mail.folderDir", "invalid folder slug", err))
	}
	base := mailDir()
	if !base.OK {
		return base
	}
	dir := core.PathJoin(base.Value.(string), slug)
	if r := core.MkdirAll(dir, 0o700); !r.OK {
		return r
	}
	return core.Ok(dir)
}

// threadsFilePath returns the absolute path to a folder's threads.md.
// Does not create the file.
func threadsFilePath(folderSlug string) core.Result {
	dirR := folderDir(folderSlug)
	if !dirR.OK {
		return dirR
	}
	return core.Ok(core.PathJoin(dirR.Value.(string), "threads.md"))
}

// accountsEncPath returns ~/Lethean/office/mail/_accounts.enc path.
func accountsEncPath() core.Result {
	base := mailDir()
	if !base.OK {
		return base
	}
	return core.Ok(core.PathJoin(base.Value.(string), accountsEncFile))
}

// stateDirPath returns ~/Lethean/office/mail/_state/{account}/ and creates it.
func stateDirPath(accountName string) core.Result {
	if err := paths.IsValidID(accountName); err != nil {
		return core.Fail(core.E("mail.stateDirPath", "invalid account name", err))
	}
	base := mailDir()
	if !base.OK {
		return base
	}
	dir := core.PathJoin(base.Value.(string), stateDir, accountName)
	if r := core.MkdirAll(dir, 0o700); !r.OK {
		return r
	}
	return core.Ok(dir)
}

// parseThreads decodes a threads.md file (list of YAML documents
// separated by "---" delimiters) into a slice of MailThreadRecord.
// Empty blocks are skipped; malformed blocks are warned and skipped.
func parseThreads(raw []byte) ([]MailThreadRecord, error) {
	if len(raw) == 0 {
		return nil, nil
	}

	// Split on "---" delimiter lines. The file format is a series of
	// YAML documents; each block starts with "---\n" and may or may
	// not end with one.
	var blocks [][]byte
	var current []byte
	i := 0
	for i < len(raw) {
		// Find end of line.
		lineEnd := i
		for lineEnd < len(raw) && raw[lineEnd] != '\n' {
			lineEnd++
		}
		line := raw[i:lineEnd]

		// Check if line is exactly "---".
		isDivider := len(line) == 3 && line[0] == '-' && line[1] == '-' && line[2] == '-'
		if line != nil && len(line) > 0 && line[len(line)-1] == '\r' {
			// strip CR
			isDivider = len(line) == 4 && line[0] == '-' && line[1] == '-' && line[2] == '-'
		}
		if isDivider {
			if len(current) > 0 {
				blocks = append(blocks, current)
				current = nil
			}
		} else {
			current = append(current, line...)
			current = append(current, '\n')
		}
		i = lineEnd + 1
	}
	if len(current) > 0 {
		blocks = append(blocks, current)
	}

	var records []MailThreadRecord
	for _, block := range blocks {
		// Skip purely whitespace blocks.
		allSpace := true
		for _, b := range block {
			if b != ' ' && b != '\t' && b != '\n' && b != '\r' {
				allSpace = false
				break
			}
		}
		if allSpace {
			continue
		}
		var rec MailThreadRecord
		if err := yaml.Unmarshal(block, &rec); err != nil {
			core.Warn("mail: malformed thread block skipped", "err", err)
			continue
		}
		if rec.ID == "" && rec.From == "" {
			// Completely empty decode — skip.
			continue
		}
		records = append(records, rec)
	}
	return records, nil
}

// relativeWhen formats a timestamp as:
// "now" (< 5 min), "HH:MM" (today), "yest" (yesterday), "X d" (older).
// Matches the fixture strings in frontend/src/lit/views/office/mail.ts.
func relativeWhen(t core.Time, now core.Time) string {
	if t.IsZero() {
		return ""
	}
	diff := now.Sub(t)
	if diff < 0 {
		diff = -diff
	}
	const minute = core.Minute
	const hour = core.Hour
	const day = 24 * core.Hour

	switch {
	case diff < 5*minute:
		return "now"
	case diff < 20*hour:
		// Same calendar day — show HH:MM.
		h, m, _ := t.Clock()
		return core.Sprintf("%02d:%02d", h, m)
	case diff < 48*hour:
		return "yest"
	default:
		return core.Sprintf("%d d", int(diff/day))
	}
}

// toThread converts a MailThreadRecord to the wire type.
func toThread(r MailThreadRecord, now core.Time) MailThread {
	return MailThread{
		ID:     r.ID,
		From:   r.From,
		Subj:   r.Subj,
		When:   relativeWhen(r.LastTouched, now),
		Unread: r.Unread,
		Body:   r.Snippet,
	}
}

// scanFolders reads all subdirectories of mailDir() and returns a
// MailFolder slice. Inbox is promoted to index 0 if present; others are
// alphabetical. Unread count is derived from each folder's threads.md.
func scanFolders(activeFolderSlug string) ([]MailFolder, error) {
	base := mailDir()
	if !base.OK {
		return nil, core.E("mail.scanFolders", base.Error(), nil)
	}
	baseDir := base.Value.(string)

	entriesR := core.ReadDir(core.DirFS(baseDir), ".")
	if !entriesR.OK {
		// No mail dir yet — return empty.
		return nil, nil
	}
	entries, _ := entriesR.Value.([]core.FsDirEntry)

	var folders []MailFolder
	var inbox *MailFolder

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		slug := entry.Name()
		// Skip internal dirs (_state etc.)
		if len(slug) > 0 && slug[0] == '_' {
			continue
		}

		// Count unread threads.
		threadsPath := core.PathJoin(baseDir, slug, "threads.md")
		unread := 0
		rawR := core.ReadFile(threadsPath)
		if rawR.OK {
			raw, _ := rawR.Value.([]byte)
			recs, _ := parseThreads(raw)
			for _, r := range recs {
				if r.Unread {
					unread++
				}
			}
		}

		f := MailFolder{
			Label:  folderLabel(slug),
			Slug:   slug,
			Unread: unread,
			Active: slug == activeFolderSlug,
		}
		if slug == "inbox" {
			inbox = &f
		} else {
			folders = append(folders, f)
		}
	}

	// Promote inbox to front.
	if inbox != nil {
		result := []MailFolder{*inbox}
		result = append(result, folders...)
		return result, nil
	}
	return folders, nil
}

// loadAccounts decrypts _accounts.enc and unmarshals the JSON array.
// Requires an unlocked private key from the AccountProvider.
func (s *Service) loadAccounts(privKey []byte) ([]MailAccount, error) {
	pathR := accountsEncPath()
	if !pathR.OK {
		return nil, core.E("mail.loadAccounts", pathR.Error(), nil)
	}
	path := pathR.Value.(string)

	rawR := core.ReadFile(path)
	if !rawR.OK {
		// File absent = no accounts saved yet.
		return nil, nil
	}
	enc, _ := rawR.Value.([]byte)

	plain, err := s.pgp.Decrypt(privKey, enc)
	if err != nil {
		return nil, core.E("mail.loadAccounts", "decrypt _accounts.enc", err)
	}

	var accounts []MailAccount
	if r := core.JSONUnmarshal(plain, &accounts); !r.OK {
		return nil, core.E("mail.loadAccounts", "unmarshal accounts", r.Value.(error))
	}
	return accounts, nil
}

// saveAccounts encrypts and atomically writes _accounts.enc.
// Requires the user's public key (readable without unlock).
func (s *Service) saveAccounts(pubKey []byte, accounts []MailAccount) error {
	marshalR := core.JSONMarshal(accounts)
	if !marshalR.OK {
		return core.E("mail.saveAccounts", "marshal accounts", marshalR.Value.(error))
	}
	plain, _ := marshalR.Value.([]byte)

	enc, err := s.pgp.Encrypt(pubKey, plain)
	if err != nil {
		return core.E("mail.saveAccounts", "encrypt accounts", err)
	}

	pathR := accountsEncPath()
	if !pathR.OK {
		return core.E("mail.saveAccounts", pathR.Error(), nil)
	}
	path := pathR.Value.(string)
	tmp := path + ".tmp"

	if r := core.WriteFile(tmp, enc, 0o600); !r.OK {
		return core.E("mail.saveAccounts", "write tmp: "+r.Error(), nil)
	}
	if r := core.Rename(tmp, path); !r.OK {
		_ = core.Remove(tmp)
		return core.E("mail.saveAccounts", "rename: "+r.Error(), nil)
	}
	return nil
}

// backoffDelay returns the current backoff duration for an account key and
// advances the counter. Key is "accountName/folderSlug".
func (s *Service) backoffDelay(key string) core.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	idx := s.backoff[key]
	if idx >= len(backoffTable) {
		idx = len(backoffTable) - 1
	}
	d := backoffTable[idx]
	if idx < len(backoffTable)-1 {
		s.backoff[key] = idx + 1
	}
	return d
}

// resetBackoff resets the backoff counter for a key on connection success.
func (s *Service) resetBackoff(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.backoff, key)
}

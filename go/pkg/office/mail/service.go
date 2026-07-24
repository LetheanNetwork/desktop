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
	"dappco.re/lthn/desktop/pkg/account"
	"dappco.re/lthn/desktop/pkg/office/internal/safedir"
	"dappco.re/lthn/desktop/pkg/paths"
	"github.com/Snider/Enchantrix/pkg/crypt/std/pgp"
	"gopkg.in/yaml.v3"
)

// SECURITY-NOTE — TIER-1 TRUSTED SURFACE (Mantis #1502, Cerberus pass-10):
//
// This Wails surface returns PII (mail threads / file paths / document bodies)
// to any in-process WebView caller. Today's caller is the trusted host SPA only.
// Post-#1421 (plugin enablement) MUST NOT inherit these methods — plugin tier
// requires explicit user consent + a stronger boundary than the in-process Wails
// IPC.
//
// Bridge bearer auth (#1423) gates only /mcp/* + /internal/* — it does NOT gate
// this Wails surface. Per design_sandbox_is_the_safety_floor + Snider's A1 floor:
// adding new endpoints here MUST be deliberate; new fields on existing endpoints
// MUST be reviewed for PII propagation.
//
// When v2 features (#1495/#1496/#1497) populate threads / docs / file index,
// re-audit this surface against the plugin-capability split.

// AccountProvider abstracts pkg/account.Service for private-key access.
// The mail service depends on it for decrypt; the interface avoids an
// import cycle (lib must not import consumer — AX principle 8).
//
// UnlockedAccountIDs replaces the previous DefaultAccountID() lottery
// (Mantis #1588 — Go map iteration randomised per-iter): every entry
// point now ASSERTS exactly one unlocked account via
// singleUnlockedAccount() and surfaces a loud error on zero / >1.
// The whole-file _accounts.enc PGP envelope (Mantis #1591) is bound to
// ONE pubkey at first-write time; reads must use THAT key. Encoding
// the beta single-account invariant in code makes the multi-account
// surface trip the assertion explicitly instead of mis-binding silently.
//
// PrivateKeyFor returns a single-use handle (Mantis #1589 / Cerberus
// #18) — the bytes are zeroised inside account.PrivateKeyHandle.Use's
// defer so per-poll key copies do not accumulate on the heap.
//
// Usage example:
//
//	svc.SetAccountService(accountSvc)
type AccountProvider interface {
	PrivateKeyFor(accountID string) (*account.PrivateKeyHandle, bool)
	PublicKeyFor(accountID string) ([]byte, bool)
	UnlockedAccountIDs() []string
}

// singleUnlockedAccount returns the unlocked account_id when exactly
// one account is unlocked. Zero unlocked → mail.no_unlocked_account;
// multiple unlocked → mail.multi_account_not_supported (Mantis #1591
// Option D — beta invariant explicit; multi-account UX needs RFC).
//
// Usage example:
//
//	id, err := s.singleUnlockedAccount()
//	if err != nil { return core.Fail(err) }
func (s *Service) singleUnlockedAccount() (string, error) {
	ids := s.account.UnlockedAccountIDs()
	if len(ids) == 0 {
		return "", core.NewCode("mail.no_unlocked_account",
			"no Lethean account is unlocked")
	}
	if len(ids) > 1 {
		return "", core.NewCode("mail.multi_account_not_supported",
			"mail does not yet support multiple unlocked accounts (Mantis #1591)")
	}
	return ids[0], nil
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

	// mu guards pooled IMAP connections (§2.3) and the
	// folderMu / accountMu maps below.
	mu sync.Mutex

	// pool holds open IMAP connections keyed by "account/folder".
	// Pool cap = poolMax per account (default 5 per §2.3).
	pool map[string]*imapConn

	// backoff tracks per-account retry delay index (§2.3 exponential backoff).
	backoff map[string]int

	// deferredPolls counts folders that would have polled while locked (§6 banner).
	deferredPolls int

	// folderMu holds per-folder-path Mutexes (Cerberus #9 Concern
	// 1.B/1.C, cascade W4 Part 2) for defence-in-depth above
	// paths.WithFileLock. Two concurrent appendThreadRecord calls
	// on the same folder serialise here so the rotation race window
	// at paths.maybeRotate's archived-suffix stamp (1-second
	// resolution, Mantis #1554) never opens from this surface.
	folderMu map[string]*sync.Mutex

	// accountMu makes FetchOnce single-flight per account (Mantis
	// #1555) — second concurrent FetchOnce for the same account
	// BLOCKS (rather than returning ErrPollInFlight) so callers do
	// not need to wire a retry loop. Lock is acquired at FetchOnce
	// entry, released on exit.
	accountMu map[string]*sync.Mutex

	// bodyFetchOverride lets tests inject a synthetic IMAP body
	// fetcher so FetchBody's at-rest write contract (Cerberus #9
	// Concern 3.A / Mantis #1556) can be exercised without a real
	// IMAP server. nil in production — the real IMAP wire-up will
	// live in fetchBodyFromIMAP when §4.2 lazy-fetch lands. Tests
	// set this to a closure returning canned RFC822 bytes.
	bodyFetchOverride func(FetchBodyInput) ([]byte, error)
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
		core:      c,
		pgp:       pgp.NewService(),
		pool:      map[string]*imapConn{},
		backoff:   map[string]int{},
		folderMu:  map[string]*sync.Mutex{},
		accountMu: map[string]*sync.Mutex{},
	}
}

// folderMutex returns the per-folder Mutex for path, creating it on
// first call. Cerberus #9 Concern 1.B/1.C — serialise all
// appendThreadRecord calls for the same folder so the rotation race
// window at paths.maybeRotate (archive-suffix 1-second resolution,
// Mantis #1554) never opens from this surface.
//
// Usage example:
//
//	m := s.folderMutex(threadsPath)
//	m.Lock(); defer m.Unlock()
func (s *Service) folderMutex(path string) *sync.Mutex {
	s.mu.Lock()
	defer s.mu.Unlock()
	if m, ok := s.folderMu[path]; ok {
		return m
	}
	m := &sync.Mutex{}
	s.folderMu[path] = m
	return m
}

// accountMutex returns the per-account Mutex for FetchOnce single-
// flight (Mantis #1555). Second concurrent FetchOnce for the same
// account name blocks here.
//
// Usage example:
//
//	m := s.accountMutex(acct.Name)
//	m.Lock(); defer m.Unlock()
func (s *Service) accountMutex(accountName string) *sync.Mutex {
	s.mu.Lock()
	defer s.mu.Unlock()
	if m, ok := s.accountMu[accountName]; ok {
		return m
	}
	m := &sync.Mutex{}
	s.accountMu[accountName] = m
	return m
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

// Stop drains the per-folder and per-account Mutex maps and closes
// any pooled IMAP connections. Mantis #1557 (Cerberus #11 LOW) —
// folderMu / accountMu are keyed by folder path + account name and
// accumulate one entry per distinct key seen for the lifetime of the
// Service. A long-running daemon polling many folders or rotating
// many account configurations could grow these maps without bound.
// Stop() called at process shutdown clears them in one pass.
//
// The pool map is also drained best-effort: any imapConn entries
// have their Close() called and the map is re-initialised. fetchFolder
// today opens-and-closes per call so pool is typically empty; the
// defensive close handles any future pool-reuse path.
//
// Backoff counters are reset for the same hygiene reason.
//
// Usage example:
//
//	_ = svc.Stop(core.Background())
func (s *Service) Stop(_ core.Context) core.Result {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, conn := range s.pool {
		if conn != nil && conn.conn != nil {
			_ = conn.conn.Close()
		}
	}
	s.pool = map[string]*imapConn{}
	s.folderMu = map[string]*sync.Mutex{}
	s.accountMu = map[string]*sync.Mutex{}
	s.backoff = map[string]int{}
	return core.Ok(nil)
}

// mailDir resolves ~/Lethean/office/mail/ and creates it if missing.
// Mode 0o700 — mail metadata is PII (Cerberus #1487 mandate).
//
// Symlink-pivot defence (Mantis #1499 MED, Cerberus wave-2/pass-10):
// goes through safedir.MkdirAll so an attacker who pre-creates
// ~/Lethean/office/mail as a symlink to /tmp/evil cannot redirect
// mail metadata writes — safedir refuses with
// safedir.UnsafeSymlinkParentCode.
func mailDir() core.Result {
	root := paths.Root()
	if !root.OK {
		return root
	}
	dir := core.PathJoin(root.Value.(string), "office", "mail")
	if r := safedir.MkdirAll(dir, 0o700); !r.OK {
		return r
	}
	return core.Ok(dir)
}

// folderDir resolves ~/Lethean/office/mail/{slug}/ and creates it.
// Validates slug via paths.IsValidID before path join. Goes through
// safedir.MkdirAll — per-folder symlink-pivot defence (Mantis #1499).
func folderDir(slug string) core.Result {
	if err := paths.IsValidID(slug); err != nil {
		return core.Fail(core.E("mail.folderDir", "invalid folder slug", err))
	}
	base := mailDir()
	if !base.OK {
		return base
	}
	dir := core.PathJoin(base.Value.(string), slug)
	if r := safedir.MkdirAll(dir, 0o700); !r.OK {
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

// stateDirPath returns ~/Lethean/office/mail/_state/{account}/ and
// creates it. Goes through safedir.MkdirAll — per-account symlink-pivot
// defence (Mantis #1499 MED).
func stateDirPath(accountName string) core.Result {
	if err := paths.IsValidID(accountName); err != nil {
		return core.Fail(core.E("mail.stateDirPath", "invalid account name", err))
	}
	base := mailDir()
	if !base.OK {
		return base
	}
	dir := core.PathJoin(base.Value.(string), stateDir, accountName)
	if r := safedir.MkdirAll(dir, 0o700); !r.OK {
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
//
// Usage example:
//
//	accounts, err := s.loadAccounts(priv)
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

// readAccountsCiphertextHash returns the SHA-256 hex of the current
// _accounts.enc bytes on disk WITHOUT decrypting them, plus an
// "exists" flag. The hash is the IfMatchHash anchor passed to
// AtomicWriteWithVersion — Cerberus #9 Concern 2.C: the optimistic-
// lock check operates on CIPHERTEXT, never plaintext, so the
// unlock-or-locked session state of the caller is irrelevant.
//
// PGP encryption is non-deterministic (fresh session key per
// message), so even an unchanged plaintext re-encrypted by a
// concurrent writer produces a fresh ciphertext + fresh hash → the
// optimistic-lock check trips correctly. The hash NEVER leaks
// plaintext semantics.
//
// Returns ("", false, nil) when the file does not exist (first-
// write path — IfMatchHash left empty, primitive treats as
// unconditional first-write per RFC §3.2 MED-2).
//
// Usage example:
//
//	priorHash, _, err := s.readAccountsCiphertextHash()
func (s *Service) readAccountsCiphertextHash() (string, bool, error) {
	pathR := accountsEncPath()
	if !pathR.OK {
		return "", false, core.E("mail.readAccountsCiphertextHash", pathR.Error(), nil)
	}
	path := pathR.Value.(string)
	rawR := core.ReadFile(path)
	if !rawR.OK {
		// Missing file → unconditional first-write path.
		return "", false, nil
	}
	enc, _ := rawR.Value.([]byte)
	return core.SHA256Hex(enc), true, nil
}

// saveAccounts encrypts the accounts list under pubKey and writes
// _accounts.enc via paths.AtomicWriteWithVersion (cascade W4 Part 1,
// RFC.atomic-write-cascade-adoption.md §2 row 10; Cerberus #9 pre-
// fire DREAD Concerns 2.A/2.B/2.C).
//
// priorHash is the IfMatchHash anchor — pass the SHA-256 hex of the
// ciphertext bytes observed on disk before the load-modify cycle
// (or empty for first-write to a missing file). The composite-or-
// pick-one check at the primitive treats empty as "skip this check".
//
// IncludeBody is passed EXPLICITLY as false (Cerberus #9 Concern
// 2.A — never rely on struct-default). The path
// "office/mail/_accounts.enc" is also in paths.AtRestEncryptedPrefixes,
// so the primitive enforces body-omission as a second layer even if
// a caller mistakenly flipped IncludeBody — defence-in-depth.
//
// Stale-write path returns core.Fail(paths.ConflictEnvelope{Code:
// "mail.accounts.update.conflict"}) so the Wails-marshalled
// Result.Value carries the lowercase-json wire shape conflict-
// dispatch.ts extractEnvelope pattern-matches on (inherits Mantis
// #1544 gating from W1+W2+W3).
//
// Audit emission is automatic via paths.AuditModeForPath — the
// _accounts.enc path routes through AuditModeSync per RFC §6.1
// (auth-substrate; the IMAP credential blob is a RecordSync surface).
//
// Usage example:
//
//	wr := s.saveAccounts(pub, existing, priorHash)
//	if !wr.OK { return wr }
func (s *Service) saveAccounts(pubKey []byte, accounts []MailAccount, priorHash string) core.Result {
	marshalR := core.JSONMarshal(accounts)
	if !marshalR.OK {
		return core.Fail(core.E("mail.saveAccounts", "marshal accounts", marshalR.Value.(error)))
	}
	plain, _ := marshalR.Value.([]byte)

	enc, err := s.pgp.Encrypt(pubKey, plain)
	if err != nil {
		return core.Fail(core.E("mail.saveAccounts", "encrypt accounts", err))
	}

	pathR := accountsEncPath()
	if !pathR.OK {
		return core.Fail(core.E("mail.saveAccounts", pathR.Error(), nil))
	}
	path := pathR.Value.(string)

	// Cerberus #9 pre-fire DREAD Concern 2.B: path MUST remain the
	// literal single-file blob "~/Lethean/office/mail/_accounts.enc"
	// — splitting per-account would change the AtRestEncryptedPrefixes
	// routing + the RecordSync audit contract. accountsEncFile is the
	// only string referenced by accountsEncPath() above.
	res := paths.AtomicWriteWithVersion(path, paths.WriteInput{
		Body:        enc,
		IfMatchHash: priorHash,
		// Cerberus #9 Concern 2.A — EXPLICIT false. Never rely on
		// struct-default. Mantis #1553 tracks the paths-tier reject
		// of IncludeBody for at-rest paths; until that lands, every
		// _accounts.enc call site MUST pass explicit false so a
		// future refactor cannot accidentally surface ciphertext via
		// the VersionStale envelope.
		IncludeBody: false,
	})
	if res.OK {
		return res
	}
	if stale, ok := paths.VersionStaleFromError(res.Value); ok {
		return core.Fail(paths.NewConflictEnvelope(
			"mail.accounts.update.conflict", stale))
	}
	return core.Fail(core.E("mail.saveAccounts",
		"write failed: "+res.Error(), nil))
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

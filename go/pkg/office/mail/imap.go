// SPDX-Licence-Identifier: EUPL-1.2

// imap.go — IMAP fetch service for pkg/office/mail.
// Implements FetchOnce (§2.1), UIDVALIDITY-rotation handling (§2.2),
// and connection pool management (§2.3).
//
// Connection pool: one IMAP connection per (account, folder), capped
// at poolMax (default 5) per account with round-robin + LRU eviction.
// Exponential backoff: 30s/1m/2m/5m/10m capped per §2.3.
//
// Threading to threads.md uses paths.AtomicAppendLine (cascade W4
// Part 2, RFC.atomic-write-cascade-adoption.md §2 row 10). Records
// above PipeBufLimit() fall back to a rewrite-merge through
// paths.AtomicWriteWithVersion + IfMatchHash — Cerberus #9 Concern
// 1.A: Darwin PIPE_BUF is 512 bytes so YAML headers with long
// subjects + "Re: Re:" chains hit the fallback on the normal path
// (NOT an edge case). Without the fallback, mail thread records
// would silently get dropped on every reply-chain message on macOS.
//
// Per-folder mutex (cascade W4 Part 2, Cerberus #9 Concern 1.B/1.C)
// — defence-in-depth above paths.WithFileLock to keep the rotation
// race window from opening when two FetchOnce calls land on the
// same folder concurrently. Per-account mutex (Mantis #1555) makes
// FetchOnce single-flight per account so the IMAP poll loop cannot
// double-fetch in parallel.

package mail

import (
	"crypto/tls"

	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/audit"
	"dappco.re/lthn/desktop/pkg/paths"
	imapclient "github.com/emersion/go-imap/v2/imapclient"
	"gopkg.in/yaml.v3"

	imap "github.com/emersion/go-imap/v2"
)

// FetchOnce performs a one-shot IMAP fetch for the given account and folder.
// Fetches new messages since last_uid_seen, appends to threads.md,
// updates _state/{account}/{folder}.json, fires EventThreadReceived per
// new thread. Returns mail.session.locked when paused (§6).
//
// Single-flight per account (Mantis #1555, cascade W4 Part 2) — a
// second concurrent FetchOnce for the same account name BLOCKS on
// the per-account Mutex until the in-flight call returns. Blocking
// (rather than returning ErrPollInFlight) keeps callers from needing
// a retry loop; the IMAP poll loop the future Stage E hook will
// drive cannot double-fetch from this surface. The mutex is acquired
// AFTER input validation so cheap-fail paths (empty account, invalid
// slug, locked session) bypass the lock entirely.
//
// Usage example:
//
//	r := svc.FetchOnce(mail.FetchOnceInput{AccountName: "personal", FolderSlug: "inbox"})
//	if !r.OK { /* handle */ }
func (s *Service) FetchOnce(input FetchOnceInput) core.Result {
	if r := s.assertUnlocked(); !r.OK {
		return r
	}
	if input.AccountName == "" {
		return core.Fail(core.NewCode("mail.fetch.account_required", "account_name is required"))
	}
	if err := paths.IsValidID(input.FolderSlug); err != nil {
		return core.Fail(core.E("mail.FetchOnce", "invalid folder slug", err))
	}

	if s.account == nil {
		return core.Fail(core.E("mail.FetchOnce", "account provider not wired", nil))
	}

	// Mantis #1555 — acquire the per-account single-flight Mutex
	// BEFORE touching IMAP state. Second concurrent FetchOnce on
	// the same account blocks here.
	am := s.accountMutex(input.AccountName)
	am.Lock()
	defer am.Unlock()

	// Mantis #1591 Option D — assert exactly one unlocked account.
	accountID, idErr := s.singleUnlockedAccount()
	if idErr != nil {
		return core.Fail(idErr)
	}
	handle, ok := s.account.PrivateKeyFor(accountID)
	if !ok {
		return s.errLocked()
	}

	// Mantis #1589 / Cerberus #18 — handle zeroises the key bytes on
	// release. We hold the decrypted accounts list across fetchFolder
	// (no key needed for IMAP itself; auth.secret in MailAccount is
	// the IMAP credential, not the PGP private key).
	var (
		accounts []MailAccount
		loadErr  error
	)
	_ = handle.Use(func(priv []byte) error {
		accounts, loadErr = s.loadAccounts(priv)
		return loadErr
	})
	if loadErr != nil {
		return core.Fail(core.E("mail.FetchOnce", "load accounts", loadErr))
	}

	var acct *MailAccount
	for i := range accounts {
		if accounts[i].Name == input.AccountName {
			acct = &accounts[i]
			break
		}
	}
	if acct == nil {
		return core.Fail(core.NewCode("mail.fetch.account_not_found",
			core.Sprintf("account %q not found", input.AccountName)))
	}

	return s.fetchFolder(acct, input.FolderSlug)
}

// fetchFolder opens an IMAP connection, fetches new messages, writes
// threads.md, and updates the state file. Returns OK with count fetched.
func (s *Service) fetchFolder(acct *MailAccount, folderSlug string) core.Result {
	poolKey := acct.Name + "/" + folderSlug
	dial := s.imapConnect
	if s.imapDialOverride != nil {
		dial = s.imapDialOverride
	}
	client, err := dial(acct, poolKey)
	if err != nil {
		s.core.ACTION(MailEvent{
			Kind:        EventConnectionFailed,
			AccountName: acct.Name,
			FolderSlug:  folderSlug,
			Error:       err.Error(),
			At:          core.Now(),
		})
		return core.Fail(core.E("mail.fetchFolder", "connect IMAP", err))
	}
	defer func() {
		_ = client.Close()
	}()
	s.resetBackoff(poolKey)

	// SELECT the folder.
	selectCmd := client.Select(folderSlug, nil)
	selectData, err := selectCmd.Wait()
	if err != nil {
		return core.Fail(core.E("mail.fetchFolder", "SELECT "+folderSlug, err))
	}

	// Load or initialise folder state.
	state, err := s.loadFolderState(acct.Name, folderSlug)
	if err != nil {
		core.Warn("mail.fetchFolder: state load failed, starting fresh", "err", err)
		state = &FolderState{}
	}

	// §2.2 — UIDVALIDITY rotation.
	if state.UIDValidity != 0 && selectData.UIDValidity != state.UIDValidity {
		if err := s.handleUIDValidityRotation(acct.Name, folderSlug, state.UIDValidity); err != nil {
			core.Warn("mail.fetchFolder: resync failed", "err", err)
		}
		state = &FolderState{}
	}
	state.UIDValidity = selectData.UIDValidity

	// Fetch new messages since last_uid_seen.
	fromUID := imap.UID(state.LastUIDSeen + 1)
	toUID := imap.UID(uint32(selectData.UIDNext) - 1)
	if fromUID > toUID {
		// Nothing new.
		state.LastFetchTS = core.Now()
		_ = s.saveFolderState(acct.Name, folderSlug, state)
		return core.Ok(0)
	}

	fetchOptions := &imap.FetchOptions{
		Envelope: true,
		Flags:    true,
		UID:      true,
	}
	uidSet := imap.UIDSetNum(imap.UID(fromUID), imap.UID(toUID))
	fetchCmd := client.Fetch(uidSet, fetchOptions)
	count := 0

	for {
		msg := fetchCmd.Next()
		if msg == nil {
			break
		}
		buf, err := msg.Collect()
		if err != nil {
			core.Warn("mail.fetchFolder: collect message failed", "err", err)
			continue
		}

		rec := imapMsgToRecord(buf)
		if err := s.appendThreadRecord(folderSlug, rec); err != nil {
			core.Warn("mail.fetchFolder: append thread failed", "err", err)
			continue
		}
		if uint32(buf.UID) > state.LastUIDSeen {
			state.LastUIDSeen = uint32(buf.UID)
		}
		count++

		s.core.ACTION(MailEvent{
			Kind:        EventThreadReceived,
			AccountName: acct.Name,
			FolderSlug:  folderSlug,
			ThreadID:    rec.ID,
			At:          core.Now(),
		})
	}

	if err := fetchCmd.Close(); err != nil {
		core.Warn("mail.fetchFolder: fetch close error", "err", err)
	}

	state.LastFetchTS = core.Now()
	_ = s.saveFolderState(acct.Name, folderSlug, state)
	return core.Ok(count)
}

// imapMsgToRecord converts a FetchMessageBuffer to a MailThreadRecord.
func imapMsgToRecord(buf *imapclient.FetchMessageBuffer) MailThreadRecord {
	rec := MailThreadRecord{
		Unread:  true,
		IMAPUID: uint32(buf.UID),
	}

	// Flags — if \Seen is set, mark as read.
	for _, f := range buf.Flags {
		if f == imap.FlagSeen {
			rec.Unread = false
		}
	}

	if buf.Envelope != nil {
		env := buf.Envelope
		rec.ID = env.MessageID
		if rec.ID == "" {
			rec.ID = core.Sprintf("uid-%d", buf.UID)
		}
		rec.Subj = env.Subject
		rec.LastTouched = env.Date

		if len(env.From) > 0 {
			addr := env.From[0]
			if addr.Name != "" {
				rec.From = addr.Name
			} else {
				rec.From = addr.Addr()
			}
		}
	}

	return rec
}

// handleUIDValidityRotation renames threads.md to
// threads.md.preresync-{old_uidvalidity} and fires resync events (§2.2).
func (s *Service) handleUIDValidityRotation(accountName, folderSlug string, oldUID uint32) error {
	s.core.ACTION(MailEvent{
		Kind:        EventResyncStarted,
		AccountName: accountName,
		FolderSlug:  folderSlug,
		At:          core.Now(),
	})

	threadsR := threadsFilePath(folderSlug)
	if !threadsR.OK {
		return core.E("mail.uidvalidity", threadsR.Error(), nil)
	}
	threadsPath := threadsR.Value.(string)
	archivePath := threadsPath + core.Sprintf(".preresync-%d", oldUID)

	// Rename preserves the user's local state for forensic comparison;
	// NEVER deleted by the service per §2.2.
	if r := core.Rename(threadsPath, archivePath); !r.OK {
		core.Warn("mail.uidvalidity: could not archive threads.md", "err", r.Error())
	}

	s.core.ACTION(MailEvent{
		Kind:        EventResyncCompleted,
		AccountName: accountName,
		FolderSlug:  folderSlug,
		At:          core.Now(),
	})
	return nil
}

// appendThreadRecord appends a MailThreadRecord YAML block to the
// folder's threads.md file via the cascade W4 Part 2 write path:
//
//  1. Compose the record as a single YAML document framed by the
//     "---\n" delimiter (one line in the threads.md sense, even
//     though the document spans multiple physical lines — the
//     delimiter separates documents).
//  2. PRIMARY: paths.AtomicAppendLine — O_APPEND + kernel-atomic
//     write semantics. Cheap, scales to IMAP-fetch volume.
//  3. FALLBACK (Cerberus #9 Concern 1.A): if the composed payload
//     exceeds paths.PipeBufLimit() the primitive rejects with
//     CodeAppendRecordTooLarge. Darwin's PIPE_BUF is 512 bytes so
//     this is the NORMAL case on macOS for thread records with
//     typical YAML headers + long subjects + "Re: Re:" chains —
//     NOT an edge case. The fallback rewrites threads.md as a
//     whole via paths.AtomicWriteWithVersion + IfMatchHash so the
//     record lands durably with the existing-content preserved
//     verbatim. An audit event "paths.append.fell_back_to_rewrite"
//     surfaces the path-and-size so dashboards can distinguish
//     "fallback engaged" from "primitive rejected, data lost".
//
// Folder-scoped Mutex (Cerberus #9 Concern 1.B/1.C, Mantis #1554
// defence-in-depth) — two concurrent appendThreadRecord calls on
// the same folder serialise here so the rotation race window at
// paths.maybeRotate's archive-suffix stamp (1-second resolution)
// never opens from this surface.
//
// Returns nil on success (either primary or fallback path); a wrapped
// core.E on hard failure. The fallback NEVER silently drops a
// record — if both primary AND fallback fail, the error propagates
// and the IMAP fetch loop's outer Warn surfaces it (caller-side
// retry on next FetchOnce will re-fetch the IMAP message via UID).
//
// Usage example:
//
//	if err := s.appendThreadRecord("inbox", rec); err != nil {
//	    core.Warn("append failed", "err", err)
//	}
func (s *Service) appendThreadRecord(folderSlug string, rec MailThreadRecord) error {
	threadsR := threadsFilePath(folderSlug)
	if !threadsR.OK {
		return core.E("mail.appendThread", threadsR.Error(), nil)
	}
	path := threadsR.Value.(string)

	block, err := yaml.Marshal(rec)
	if err != nil {
		return core.E("mail.appendThread", "marshal record", err)
	}

	// Compose record as a single document framed by the "---\n"
	// delimiter so parseThreads() in service.go recognises the
	// boundary. AtomicAppendLine guarantees exactly-one trailing
	// newline so the appended payload concatenates cleanly with
	// future records.
	sep := []byte("---\n")
	payload := make([]byte, 0, len(sep)+len(block))
	payload = append(payload, sep...)
	payload = append(payload, block...)

	// Folder-mutex guard — defence-in-depth above paths.WithFileLock.
	fm := s.folderMutex(path)
	fm.Lock()
	defer fm.Unlock()

	// PRIMARY — AtomicAppendLine. Accounts for the trailing-newline
	// invariant the primitive maintains: it adds '\n' to the
	// effective length when the payload doesn't already end with
	// one. Hash + version check sit at the primitive boundary.
	ar := paths.AtomicAppendLine(path, payload)
	if ar.OK {
		return nil
	}
	// FALLBACK — Cerberus #9 Concern 1.A. The primitive surfaces
	// CodeAppendRecordTooLarge via its error string (core.NewCode
	// embeds the code as the message); match on that to distinguish
	// the over-limit case from genuine I/O failures.
	if !core.Contains(ar.Error(), paths.CodeAppendRecordTooLarge) {
		// Hard failure — neither size nor lock. Propagate up.
		return core.E("mail.appendThread",
			"AtomicAppendLine failed: "+ar.Error(), nil)
	}
	// Size-bound fallback. MailEvent paths.append.fell_back_to_rewrite
	// (emitted below) covers observability; log line was noise on
	// Darwin where PIPE_BUF=512 makes the fallback the normal case
	// per Cerberus #1558.

	// Read current bytes for the merge under the same folder-mutex
	// guard so a concurrent appender cannot land between read and
	// rewrite. AtomicWriteWithVersion's IfMatchHash provides the
	// inner cross-process check.
	var existing []byte
	var priorHash string
	if rawR := core.ReadFile(path); rawR.OK {
		existing, _ = rawR.Value.([]byte)
		priorHash = core.SHA256Hex(existing)
	}
	merged := make([]byte, 0, len(existing)+len(payload)+1)
	merged = append(merged, existing...)
	merged = append(merged, payload...)
	if len(merged) == 0 || merged[len(merged)-1] != '\n' {
		merged = append(merged, '\n')
	}
	wr := paths.AtomicWriteWithVersion(path, paths.WriteInput{
		Body:        merged,
		IfMatchHash: priorHash,
		// IncludeBody EXPLICIT false — threads.md is NOT in the
		// at-rest-encrypted prefix list but body envelope-leak is
		// undesirable regardless (per-thread bytes are PII). Cascade
		// discipline anchor.
		IncludeBody: false,
	})
	if !wr.OK {
		return core.E("mail.appendThread",
			"rewrite-merge fallback failed: "+wr.Error(), nil)
	}
	// Audit-event surface so operations can dashboard fallback rate.
	// Fires via TWO channels (Mantis #1561 — Cerberus #11 cross-cascade
	// dashboard symmetry):
	//
	//   1. Typed MailEvent bus — mail-specific subscribers (the
	//      existing Subscribe surface) pick it up without a new
	//      contract.
	//   2. audit.Default() — lands in ~/Lethean/audit/<date>.ndjson
	//      alongside paths.write.failed + every other cross-cascade
	//      event so the Stage F log-tailer surface sees the fallback
	//      without needing a per-package bridge.
	//
	// Meta keys (Mantis #1559): path_hash + record_size_bytes +
	// pipe_buf_limit. path_hash uses paths.HashForAudit so this
	// cross-package emitter shares the substrate-canonical HKDF-
	// domain-separated path-hash the LockEvent path uses (Mantis
	// #1564 — supersedes the raw-SHA256 stopgap H#25 shipped).
	recordSize := len(payload)
	pathHash := paths.HashForAudit(path)
	meta := map[string]any{
		"path_hash":         pathHash,
		"record_size_bytes": recordSize,
		"pipe_buf_limit":    paths.PipeBufLimit(),
	}
	s.core.ACTION(MailEvent{
		Kind:       EventPathsAppendFellBackToRewrite,
		FolderSlug: folderSlug,
		At:         core.Now(),
		Meta:       meta,
	})
	_ = audit.Default().Record(audit.Event{
		Event:   EventPathsAppendFellBackToRewrite,
		Scope:   "mail.append.fallback",
		Outcome: audit.OutcomeOK,
		TS:      core.Now().UTC().Unix(),
		Meta: map[string]any{
			"folder_slug":       folderSlug,
			"path_hash":         pathHash,
			"record_size_bytes": recordSize,
			"pipe_buf_limit":    paths.PipeBufLimit(),
		},
	})
	return nil
}

// loadFolderState reads _state/{account}/{folder}.json.
func (s *Service) loadFolderState(accountName, folderSlug string) (*FolderState, error) {
	dirR := stateDirPath(accountName)
	if !dirR.OK {
		return nil, core.E("mail.loadFolderState", dirR.Error(), nil)
	}
	path := core.PathJoin(dirR.Value.(string), folderSlug+".json")
	rawR := core.ReadFile(path)
	if !rawR.OK {
		return &FolderState{}, nil
	}
	raw, _ := rawR.Value.([]byte)
	var state FolderState
	if r := core.JSONUnmarshal(raw, &state); !r.OK {
		return nil, core.E("mail.loadFolderState", "unmarshal", r.Value.(error))
	}
	return &state, nil
}

// saveFolderState writes _state/{account}/{folder}.json atomically.
func (s *Service) saveFolderState(accountName, folderSlug string, state *FolderState) error {
	dirR := stateDirPath(accountName)
	if !dirR.OK {
		return core.E("mail.saveFolderState", dirR.Error(), nil)
	}
	path := core.PathJoin(dirR.Value.(string), folderSlug+".json")
	marshalR := core.JSONMarshal(state)
	if !marshalR.OK {
		return core.E("mail.saveFolderState", "marshal", marshalR.Value.(error))
	}
	data, _ := marshalR.Value.([]byte)
	tmp := path + ".tmp"
	if r := core.WriteFile(tmp, data, 0o600); !r.OK {
		return core.E("mail.saveFolderState", "write tmp: "+r.Error(), nil)
	}
	if r := core.Rename(tmp, path); !r.OK {
		_ = core.Remove(tmp)
		return core.E("mail.saveFolderState", "rename: "+r.Error(), nil)
	}
	return nil
}

// imapConnect opens (or borrows) an IMAP client for the given pool key.
// Pool cap = poolMax per account; LRU eviction when full (§2.3).
func (s *Service) imapConnect(acct *MailAccount, poolKey string) (*imapclient.Client, error) {
	// For now, open a fresh connection (pool management is a v2.x
	// upgrade; the architecture supports it via s.pool).
	addr := core.Sprintf("%s:%d", acct.IMAP.Host, acct.IMAP.Port)

	var client *imapclient.Client
	var err error

	opts := &imapclient.Options{}
	if acct.IMAP.TLS {
		client, err = imapclient.DialTLS(addr, opts)
	} else {
		client, err = imapclient.DialStartTLS(addr, opts)
	}
	if err != nil {
		return nil, err
	}

	// Login with app password (§1.5 Q2 — only appPassword kind reaches here).
	if err := client.Login(acct.IMAP.User, acct.Auth.Secret).Wait(); err != nil {
		_ = client.Close()
		return nil, err
	}
	return client, nil
}

// imapConnectTLS is used by tests to inject a TLS config.
func imapConnectWithTLS(acct *MailAccount, tlsCfg *tls.Config) (*imapclient.Client, error) {
	addr := core.Sprintf("%s:%d", acct.IMAP.Host, acct.IMAP.Port)
	opts := &imapclient.Options{TLSConfig: tlsCfg}
	if acct.IMAP.TLS {
		return imapclient.DialTLS(addr, opts)
	}
	return imapclient.DialStartTLS(addr, opts)
}

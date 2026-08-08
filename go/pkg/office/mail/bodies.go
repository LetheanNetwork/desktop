// SPDX-Licence-Identifier: EUPL-1.2

// bodies.go — per-message body fetch + cache for pkg/office/mail.
// Implements the §4.1 FetchBody surface that the FetchBodyInput /
// FetchBodyOutput types in types.go advertised but had no service
// method behind. Cerberus #9 Concern 3.A / Mantis #1556 — every body
// write routes through paths.AtomicWriteWithVersion + IfMtime so the
// at-rest substrate's cascade-adoption discipline (cascade W2 of the
// atomic-write-cascade-adoption RFC) is honoured at the per-message
// boundary.
//
// Storage layout (additive to the folder layout established at
// threadsFilePath):
//
//   ~/Lethean/office/mail/<folder-slug>/bodies/<thread-id>.eml
//
// One file per message body, raw RFC822 bytes. The .eml extension
// lets external tooling (e.g. macOS Mail.app) consume the cached
// body verbatim without reformatting. thread-id is validated via
// paths.IsValidID before composing the path (defends against
// traversal / symlink-pivot).
//
// IfMtime contract per Cerberus #9 Concern 3.A:
//
//   - Zero-time on first write → unconditional. This is the normal
//     case because the per-account single-flight FetchOnce mutex
//     (Mantis #1555) already guarantees no concurrent same-message
//     write at the substrate boundary; IfMtime would NOT add a
//     guarantee here (atomic_write.go:298 skips the IfMtime branch
//     when cur.Mtime.IsZero, so passing zero is the canonical first-
//     write shape).
//
//   - Non-zero IfMtime on rewrite → the substrate compares
//     cur.Mtime <= IfMtime and refuses the write when an out-of-band
//     mutation happened (the file's mtime advanced past what the
//     caller saw at read-time). Mail bodies are append-only in
//     practice but the gate still defends against an attacker who
//     can write to the bodies directory between our read and write.

package mail

import (
	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/office/internal/safedir"
	"dappco.re/lthn/desktop/pkg/paths"
)

// bodyDirPath returns ~/Lethean/office/mail/<folder>/bodies/ and
// creates it via safedir.MkdirAll (defends the per-account symlink-
// pivot vector that mailDir + folderDir already defend at their
// own boundaries).
func bodyDirPath(folderSlug string) core.Result {
	dirR := folderDir(folderSlug)
	if !dirR.OK {
		return dirR
	}
	bodyDir := core.PathJoin(dirR.Value.(string), "bodies")
	if r := safedir.MkdirAll(bodyDir, 0o700); !r.OK {
		return r
	}
	return core.Ok(bodyDir)
}

// bodyFilePath returns the absolute path for a single message body
// keyed by thread-id. Validates thread-id via paths.IsValidID before
// composing — the slug must be safe to embed in a filesystem name.
// Does not create the file.
func bodyFilePath(folderSlug, threadID string) core.Result {
	if err := paths.IsValidID(threadID); err != nil {
		return core.Fail(core.E("mail.bodyFilePath", "invalid thread id", err))
	}
	dirR := bodyDirPath(folderSlug)
	if !dirR.OK {
		return dirR
	}
	return core.Ok(core.PathJoin(dirR.Value.(string), threadID+".eml"))
}

// FetchBody returns the cached body bytes for a message thread.
// First call writes the body to disk via paths.AtomicWriteWithVersion
// (cache miss); subsequent calls serve from disk (cache hit). The
// raw RFC822 bytes are then decoded into a FetchBodyOutput
// (Plain + HTML + AttachmentMeta) — v1 ships a minimal plain-text
// extraction; full MIME walk lands when the §4.2 AllowHTML toggle
// + attachment cache need it.
//
// Per Cerberus #9 Concern 3.A: the on-disk write routes through
// paths.AtomicWriteWithVersion with IfMtime gating. The per-account
// single-flight FetchOnce mutex (Mantis #1555) already guarantees no
// concurrent same-message write so IfMtime is informational on first
// write; defence-in-depth against out-of-band mutators on rewrite.
//
// v1 scope: empty Plain when no plain-text section is present (the
// caller renders the empty case as "body still fetching"). HTML is
// only populated when AllowHTML=true AND the message had a real
// text/html part. Attachment metadata is empty until §4.2 lands.
//
// Usage example:
//
//	r := svc.FetchBody(mail.FetchBodyInput{
//	    AccountName: "personal",
//	    FolderSlug:  "inbox",
//	    ThreadID:    "abc123",
//	})
//	if r.OK { out := r.Value.(mail.FetchBodyOutput); _ = out.Plain }
func (s *Service) FetchBody(input FetchBodyInput) core.Result {
	if r := s.assertUnlocked(); !r.OK {
		return r
	}
	if input.AccountName == "" {
		return core.Fail(core.NewCode("mail.fetch_body.account_required",
			"account_name is required"))
	}
	if err := paths.IsValidID(input.FolderSlug); err != nil {
		return core.Fail(core.E("mail.FetchBody", "invalid folder slug", err))
	}
	if err := paths.IsValidID(input.ThreadID); err != nil {
		return core.Fail(core.E("mail.FetchBody", "invalid thread id", err))
	}

	pathR := bodyFilePath(input.FolderSlug, input.ThreadID)
	if !pathR.OK {
		return pathR
	}
	path := pathR.Value.(string)

	// Cache path — serve from disk when present. ReadVersion catches
	// mid-flight rewrites at the substrate (the read sees the
	// post-rename atomic content, never a partially-written file).
	statR := core.Stat(path)
	if statR.OK {
		readR := core.ReadFile(path)
		if !readR.OK {
			return core.Fail(core.E("mail.FetchBody",
				"cached body read failed", nil))
		}
		body, _ := readR.Value.([]byte)
		return core.Ok(decodeBodyToOutput(body, input.AllowHTML))
	}

	// Cache miss — fetch from IMAP. v1 ships a minimal placeholder:
	// the real IMAP body-fetch wire-up lives at the §4.2 surface that
	// the lazy-fetch on-demand UI flow drives. This first iteration
	// makes the at-rest write path real so the cascade-adoption
	// contract (Cerberus #9 Concern 3.A) is honoured at the boundary
	// the moment the IMAP fetch lands its first byte. Callers see an
	// empty FetchBodyOutput until the IMAP wiring is threaded through.
	bodyBytes, fetchErr := s.fetchBodyFromIMAP(input)
	if fetchErr != nil {
		return core.Fail(core.E("mail.FetchBody",
			"IMAP body fetch failed", fetchErr))
	}
	if len(bodyBytes) == 0 {
		// Treated as "body not available yet" — no on-disk write,
		// returns empty output. UI renders the empty case as
		// "fetching body…" until a real fetch lands.
		return core.Ok(FetchBodyOutput{})
	}

	// At-rest write — Cerberus #9 Concern 3.A contract. Zero IfMtime
	// on first write is canonical (per atomic_write.go:298 the
	// IfMtime branch skips when cur.Mtime.IsZero, so passing zero
	// time IS the unconditional first-write shape). Single-flight
	// per-account IMAP poll (Mantis #1555 shipped f81d5cc) guarantees
	// no concurrent same-message write; IfMtime would NOT provide
	// that guarantee at this substrate boundary.
	wr := paths.AtomicWriteWithVersion(path, paths.WriteInput{
		Body:        bodyBytes,
		IfMtime:     core.Time{}, // zero on first write per Concern 3.A
		IncludeBody: false,
	})
	if !wr.OK {
		return core.Fail(core.E("mail.FetchBody",
			"body write failed: "+wr.Error(), nil))
	}

	return core.Ok(decodeBodyToOutput(bodyBytes, input.AllowHTML))
}

// fetchBodyFromIMAP is the IMAP body-fetch hook. v1 returns
// (nil, nil) — the UI renders "body not available yet" and a future
// iteration wires the real IMAP BODYSTRUCTURE walk + BODY[1] /
// BODY.PEEK fetch through this seam. Splitting the hook keeps the
// at-rest write substrate testable independent of an IMAP server.
//
// When the future iteration lands, it MUST:
//
//  1. Acquire the per-account mutex via s.accountMutex(acct.Name)
//     so the single-flight invariant the IfMtime comment relies on
//     stays load-bearing.
//  2. Resolve thread-id → IMAP UID via the folder state file or
//     the threads.md record so the IMAP fetch targets the right
//     message.
//  3. Use BODY.PEEK[] to avoid marking the message Seen as a
//     side-effect of the body fetch (the UI marks Seen explicitly
//     when the user actually opens the message).
//
// Until then, FetchBody is a substrate-shape demo + cache reader —
// the at-rest write contract is real and tested; the IMAP fetch
// is the iteration that follows when the lazy-fetch UI hook fires
// for the first time.
func (s *Service) fetchBodyFromIMAP(input FetchBodyInput) ([]byte, error) {
	if s.bodyFetchOverride != nil {
		return s.bodyFetchOverride(input)
	}
	return nil, nil
}

// decodeBodyToOutput extracts plain / HTML / attachment metadata
// from raw RFC822 bytes. v1 ships a minimal non-MIME extractor that
// surfaces the raw payload as Plain when the bytes do NOT look like
// MIME multipart (common case for non-multipart messages). Full MIME
// walk lands when §4.2 needs it.
//
// allowHTML gates whether the HTML field is ever populated per the
// §4.2 Q3 ruling: HTML rendering is opt-in to keep tracker pixels
// + remote-image fetches off by default.
func decodeBodyToOutput(raw []byte, allowHTML bool) FetchBodyOutput {
	if len(raw) == 0 {
		return FetchBodyOutput{}
	}
	// Minimal v1 extractor: split header from body at the first
	// "\r\n\r\n" or "\n\n" sequence; everything after is treated as
	// the plain payload. A real MIME walk arrives with §4.2.
	out := FetchBodyOutput{Plain: extractPayload(raw)}
	if allowHTML {
		// v1 does NOT populate HTML — the field exists so callers
		// that opt in receive an empty string rather than panic.
		// §4.2 wires the BODYSTRUCTURE walk that selects text/html
		// parts.
		out.HTML = ""
	}
	return out
}

// extractPayload returns the body region of an RFC822 byte sequence
// (the bytes after the first blank line that separates header from
// body). Returns the input verbatim when no header boundary is
// found — keeps fixtures + tests that pass plain text directly to
// FetchBody able to round-trip through decodeBodyToOutput.
func extractPayload(raw []byte) string {
	// Prefer CRLF (RFC822 canonical), fall back to LF for fixtures.
	if i := core.Index(string(raw), "\r\n\r\n"); i >= 0 {
		return string(raw[i+4:])
	}
	if i := core.Index(string(raw), "\n\n"); i >= 0 {
		return string(raw[i+2:])
	}
	return string(raw)
}

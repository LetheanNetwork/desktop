// SPDX-Licence-Identifier: EUPL-1.2

// Service — Core integration for the runbooks surface. Reads and
// writes Trix-style markdown files from ~/Lethean/runbooks/*.md.
// No queue, no orm — filesystem is the persistence layer.
//
// On first use (empty directory), seed runbooks are written matching
// the frontend fixture data. Seeding is idempotent — skipped when any
// *.md file already exists.
//
// Lifecycle:
//   - Register(c)        wires the service into the Core container
//   - ServiceName()      returns "Runbooks" for the Wails namespace
//   - SetSessionGate(g)  wired post-construction in cmd/lthn/app.go
//     against *account.Service (live-read pattern — mirrors
//     office/mail.AccountProvider; no cached bool, no event bus).
//   - Stop(ctx)          nils the SessionGate reference so a draining
//     Service fails-closed on any late-arriving write.
//
// All I/O uses CoreGO wrappers. Banned stdlib imports: os,
// path/filepath, strings, encoding/json, fmt, log, errors.

package runbooks

import (
	"sync"
	"sync/atomic"

	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/account"
	"dappco.re/lthn/desktop/pkg/paths"
	"dappco.re/lthn/desktop/pkg/recordfile"
	"github.com/Snider/Enchantrix/pkg/crypt/std/pgp"
	"gopkg.in/yaml.v3"
)

// SessionGate is the minimal consumer-defined interface satisfied by
// *account.Service. Live-read at every gate check — no cached bool, no
// subscribe/event bus (RFC.stage-e-unlockgate v2 §1.1 — Pushback 2
// CONFIRMED by Cerberus #27/#28). When the returned slice is empty the
// session is locked; when non-empty at least one Lethean account is
// unlocked and writes may proceed.
//
// Wired in cmd/lthn/app.go (Mantis #1613 B.3, deferred to that lane):
//
//	runbooksSvc.SetSessionGate(accountSvc)
//
// AX-8 compliance: this interface is defined in the consumer
// (runbooks) and satisfied by the producer (*account.Service). No
// pkg/account import lands in the SessionGate interface itself; the
// account import below is consumed only by the wider runtime-asserted
// accountKeyProvider used by the at-rest writer path (mirrors deals).
//
// Stage E.D.B.2 (Mantis #1487 wave 2) adds a SEPARATE at-rest-keys
// lookup via a runtime type-assertion against the accountKeyProvider
// shape below. The narrow SessionGate stays minimal so the existing
// app.go wire + sibling-consumer stubs keep working; the wider keys
// surface is opt-in by the implementer.
type SessionGate interface {
	UnlockedAccountIDs() []string
}

// accountKeyProvider aliases account.AccountKeyProvider so the
// runtime-assertion call-site stays grep-stable (`gate.
// (accountKeyProvider)`) while the contract definition lives once in
// pkg/account (Cerberus #44 PRBW F-3 substrate extract).
//
//	if kp, ok := s.gate.(accountKeyProvider); ok { ... }
type accountKeyProvider = account.AccountKeyProvider

// Freshness thresholds. Runbooks rehearsed within 30 days are "fresh";
// between 30 and 90 days are "aging"; beyond 90 days (or never) are
// "stale".
const (
	FreshThreshold = 30 * 24 * core.Hour
	AgingThreshold = 90 * 24 * core.Hour
)

// Service owns the runbooks surface.
//
// Usage example:
//
//	svc := runbooks.NewService(c)
//	svc.SetSessionGate(accountSvc)
type Service struct {
	core *core.Core

	// gateMu guards reads/writes of the session gate reference. A
	// sync.RWMutex protects against the wire/Stop race where app.go
	// SetSessionGate runs concurrent with a late-arriving Wails call
	// reading the reference. Read-heavy access (every write gates
	// once) — RWMutex.RLock is microseconds.
	gateMu sync.RWMutex
	// gate is the live-read session source (RFC §1.1). nil before
	// SetSessionGate runs in app.go and after Stop nils it; the
	// nilWarned one-shot warning fires on the first nil-hit to
	// surface wire-ordering bugs without log spam (§2.2 ADD-1.5).
	gate SessionGate
	// nilWarned is the one-shot guard for the nil-gate fail-safe
	// (§2.2 / Cerberus #28 Q2). CompareAndSwap-on-first-hit emits
	// core.Warn exactly once per Service instance.
	nilWarned atomic.Bool

	// atrestMu serialises lazy construction of atrestWriter so the
	// first concurrent write doesn't double-build the substrate.
	atrestMu sync.Mutex
	// atrestWriter is built on first writer-access against the live
	// SessionGate (RFC.stage-e-encrypt-at-rest v2 §5, Wave 2 — Mantis
	// #1487). nil before the first use; replaced on every
	// SetSessionGate so adapter→gate remains coherent.
	atrestWriter *recordfile.AtRestWriter[RunbookRecord]
}

// NewService constructs the runbooks service against a Core container.
// Wired via core.WithName("runbooks", runbooks.Register) in app.go.
//
// Usage example:
//
//	svc := runbooks.NewService(c)
func NewService(c *core.Core) *Service {
	return &Service{core: c}
}

// Register constructs the runbooks service for Core registration.
//
// Usage example:
//
//	core.New(core.WithName("runbooks", runbooks.Register))
func Register(c *core.Core) core.Result {
	return core.Ok(NewService(c))
}

// ServiceName labels the binding namespace exposed to JS.
// Wails binds methods as "Runbooks.List()" etc.
func (s *Service) ServiceName() string { return "Runbooks" }

// OnStart seeds the runbook library when the directory is empty.
// Safe to call when core is nil (unit test paths).
func (s *Service) OnStart() core.Result {
	dir := runbooksDir()
	if !dir.OK {
		return core.Ok(nil) // best-effort — non-fatal on startup
	}
	dirPath := dir.Value.(string)
	if countMdFiles(dirPath) > 0 {
		return core.Ok(nil) // already seeded
	}
	seedAll(dirPath)
	return core.Ok(nil)
}

// runbooksDir resolves ~/Lethean/runbooks/ and creates it if missing.
// Mode 0o700 (Cerberus #1487 PR-1): runbooks contain operational
// secrets (recovery procedures, credential-rotation steps) — owner-only.
func runbooksDir() core.Result {
	root := paths.Root()
	if !root.OK {
		return root
	}
	dir := core.PathJoin(root.Value.(string), "runbooks")
	if r := core.MkdirAll(dir, 0o700); !r.OK {
		return r
	}
	return core.Ok(dir)
}

// countMdFiles returns the number of runbook record files in dir.
// Counts BOTH legacy ".md" plaintext AND ".lthn" encrypted records so
// the OnStart seed short-circuit advances correctly across the
// .md → .lthn cutover (Mantis #1487 Wave 2). Without the .lthn count
// a post-migration directory would look empty to seedAll and trigger
// re-seed of seven default runbooks on top of the operator's data.
func countMdFiles(dir string) int {
	r := core.ReadDir(core.DirFS(dir), ".")
	if !r.OK {
		return 0
	}
	entries, _ := r.Value.([]core.FsDirEntry)
	n := 0
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		nm := e.Name()
		if hasRecordSuffix(nm) {
			n++
		}
	}
	return n
}

// hasRecordSuffix reports whether nm ends in ".md" (legacy plaintext)
// or ".lthn" (encrypted at-rest). Mirrors sales/deals' hasSuffix
// helper but keeps the predicate self-describing inside this package.
func hasRecordSuffix(nm string) bool {
	if len(nm) >= 3 && nm[len(nm)-3:] == ".md" {
		return true
	}
	if len(nm) >= 5 && nm[len(nm)-5:] == ".lthn" {
		return true
	}
	return false
}

// --- At-rest substrate plumbing (RFC.stage-e-encrypt-at-rest v2 Wave 2)

// runbooksHeaderSchema declares the RFC §2.4 per-field header whitelist
// for the runbooks surface. Per the per-field MUST table:
//
//   - `title`   → BODY only (never in header)
//   - `tags`    → HEADER as SHA-256 hash-tag list ([]string of
//     16-hex-char SHA-256 truncations); full plaintext tag list stays
//     in the encrypted body. List operations compare hashes without
//     learning the operator's tag vocabulary.
//   - everything else (area, last_rehearsed, body) → BODY only.
//
// While locked the operator sees the 16-hex-char hashes rather than
// the plaintext tag names — that's the intent of the §2.4 ruling
// (Cerberus C#7 Q8: tags MUST hash, free-form rejected).
//
//	w := recordfile.NewAtRestWriter(recordfile.AtRestDeps[RunbookRecord]{
//	    ..., Schema: runbooksHeaderSchema,
//	})
var runbooksHeaderSchema = recordfile.HeaderSchema[RunbookRecord]{
	Surface: recordfile.SurfaceRunbooks,
	AllowedKeys: map[string]recordfile.FieldValidator{
		"tags": recordfile.ValidateHashTagList,
	},
	HeaderFor: func(r RunbookRecord) map[string]any {
		out := map[string]any{}
		// Project plaintext tags through HashTag — the substrate's
		// ValidateHashTagList requires []any so we widen each string
		// individually rather than passing []string directly.
		if len(r.Tags) > 0 {
			hashes := make([]any, 0, len(r.Tags))
			for _, tag := range r.Tags {
				if tag == "" {
					continue
				}
				hashes = append(hashes, recordfile.HashTag(tag))
			}
			if len(hashes) > 0 {
				out["tags"] = hashes
			}
		}
		return out
	},
	IDFor: func(r RunbookRecord) string {
		// Runbooks identify by ID in-memory (e.g. "R-01"), but the
		// filesystem basename is the slug (e.g. "rotate-runtime-api-
		// keys"). The header `id` field MUST round-trip the in-memory
		// ID so List/Get can resolve back via header peek without
		// reading the body. Slug stays in the filename only.
		return r.ID
	},
	VersionFor: func(r RunbookRecord) int64 { return int64(r.Version) },
}

// atrestWriterFor returns the lazy-constructed AtRestWriter wired
// against the live SessionGate. Returns (nil, false) when the gate is
// not yet wired OR when the gate doesn't satisfy the wider keys
// surface — caller falls back to legacy plaintext write.
//
// Construction happens once per Service post-SetSessionGate (atrestMu
// serialises the racy first-call path); SetSessionGate invalidates the
// cache so re-wiring against a fresh accountSvc never reuses stale
// adapter pointers.
//
//	w, ok := s.atrestWriterFor()
//	if !ok { return writeLegacy(rec, dir, ifVersion) }
func (s *Service) atrestWriterFor() (*recordfile.AtRestWriter[RunbookRecord], bool) {
	if s == nil {
		return nil, false
	}
	s.gateMu.RLock()
	gate := s.gate
	s.gateMu.RUnlock()
	if gate == nil {
		return nil, false
	}
	// Runtime type-assert the wider keys surface. *account.Service
	// satisfies it today (unlock.go:870 / :903); a minimal-gate stub
	// (UnlockedAccountIDs-only) skips the at-rest path entirely and
	// the consumer falls back to the legacy plaintext writer.
	keys, ok := gate.(accountKeyProvider)
	if !ok {
		return nil, false
	}
	s.atrestMu.Lock()
	defer s.atrestMu.Unlock()
	if s.atrestWriter != nil {
		return s.atrestWriter, true
	}
	w := recordfile.NewAtRestWriter(recordfile.AtRestDeps[RunbookRecord]{
		Surface: recordfile.SurfaceRunbooks,
		Keys: account.NewAtRestKeys("runbooks", account.AtRestKeysDeps{
			Gate: gate,
			Keys: keys,
		}),
		PGP:    pgp.NewService(),
		Schema: runbooksHeaderSchema,
		Atomic: paths.AtRestAdapter("runbooks"),
	})
	s.atrestWriter = w
	return w, true
}

// parseAtRestRecord turns a substrate ReadResult into a RunbookRecord
// by running the frontmatter through yaml.Unmarshal (same path
// parseRecord uses for plaintext) and copying the body text into
// Body. Slug is stamped by the caller because the substrate doesn't
// carry the basename.
//
//	rec, err := parseAtRestRecord(slug, rr.Value.(recordfile.ReadResult))
func parseAtRestRecord(slug string, r recordfile.ReadResult) (RunbookRecord, error) {
	var rec RunbookRecord
	if err := yaml.Unmarshal(r.BodyYAML, &rec); err != nil {
		return RunbookRecord{}, core.E("runbooks.parseAtRest", "yaml unmarshal", err)
	}
	rec.Slug = slug
	rec.Body = string(r.BodyText)
	// Header version takes precedence on at-rest reads (substrate
	// stamps it via VersionFor); ensure Version reflects the persisted
	// header value rather than any stale frontmatter copy.
	if hdrV := int(r.Header.Version); hdrV > 0 {
		rec.Version = hdrV
	}
	return rec, nil
}

// headerPubKey resolves the public key bytes for the account.id named
// in a raw at-rest blob's header. Peeks the JSON header directly so
// the lookup is independent of whether any account is unlocked —
// matches RFC §4.1 "List stays open while LOCKED".
func (s *Service) headerPubKey(raw []byte) ([]byte, error) {
	s.gateMu.RLock()
	gate := s.gate
	s.gateMu.RUnlock()
	if gate == nil {
		return nil, core.E("runbooks.headerPubKey", "session gate not wired", nil)
	}
	keys, ok := gate.(accountKeyProvider)
	if !ok {
		return nil, core.E("runbooks.headerPubKey",
			"session gate does not provide account keys", nil)
	}
	accountID, err := recordfile.PeekAccountID(raw)
	if err != nil {
		return nil, err
	}
	pub, ok := keys.PublicKeyFor(accountID)
	if !ok || len(pub) == 0 {
		return nil, core.E("runbooks.headerPubKey",
			"PublicKeyFor("+accountID+") returned not-ok", nil)
	}
	return pub, nil
}

// loadHeaderOnly decodes a .lthn file via the substrate's DecodeHeader
// path (no decrypt). Returns a RunbookRecord populated only with
// header-visible fields (ID, Version, hash-projected Tags). Body
// fields stay zero-valued (Title="", Area="", Body="") — toEntry sees
// an empty Title which the frontend renders as "(encrypted)" or
// similar.
//
// PubKey lookup walks the SessionGate via PublicKeyFor which does NOT
// require unlock so List stays open while LOCKED per RFC §4.1.
//
// Returns an error when the gate is unwired (cannot resolve any
// public key) OR when DecodeHeader rejects.
func (s *Service) loadHeaderOnly(path, slug string) (RunbookRecord, error) {
	w, ok := s.atrestWriterFor()
	if !ok {
		return RunbookRecord{}, core.E("runbooks.loadHeaderOnly",
			"session gate not wired", nil)
	}
	raw := core.ReadFile(path)
	if !raw.OK {
		return RunbookRecord{}, core.E("runbooks.loadHeaderOnly", raw.Error(), nil)
	}
	rawBytes, _ := raw.Value.([]byte)
	pub, perr := s.headerPubKey(rawBytes)
	if perr != nil {
		return RunbookRecord{}, perr
	}
	rr := w.DecodeHeader(rawBytes, pub)
	if !rr.OK {
		return RunbookRecord{}, core.E("runbooks.loadHeaderOnly", rr.Error(), nil)
	}
	hdr, _ := rr.Value.(recordfile.Header)
	rec := RunbookRecord{
		ID:      hdr.ID,
		Slug:    slug,
		Version: int(hdr.Version),
	}
	// Surface the header's hash-tag list onto rec.Tags so the
	// header-only projection still carries SOMETHING for the frontend
	// to render tag-shaped UI (16-hex-char hashes per RFC §2.4 — NOT
	// plaintext tag names). The operator sees hashes while locked.
	if rawTags, ok := hdr.Raw["tags"].([]any); ok {
		for _, t := range rawTags {
			if s, ok := t.(string); ok {
				rec.Tags = append(rec.Tags, s)
			}
		}
	}
	return rec, nil
}

// parseRecord splits a Trix file (slug + raw bytes) into frontmatter
// + body via the shared recordfile.Split helper and decodes the YAML
// header into a RunbookRecord. The slug is stamped onto the record
// because runbooks identify themselves by filename, not by a YAML
// field.
func parseRecord(slug string, raw []byte) (RunbookRecord, error) {
	fm, body := recordfile.Split(raw)
	var rec RunbookRecord
	if err := yaml.Unmarshal(fm, &rec); err != nil {
		return RunbookRecord{}, core.E("runbooks.parse", "yaml unmarshal", err)
	}
	rec.Slug = slug
	rec.Body = string(body)
	return rec, nil
}

// marshalRecord serialises a RunbookRecord to the Trix file format
// via the shared recordfile.Stitch helper.
func marshalRecord(r RunbookRecord) ([]byte, error) {
	fm, err := yaml.Marshal(r)
	if err != nil {
		return nil, core.E("runbooks.marshal", "yaml marshal", err)
	}
	return recordfile.Stitch(fm, []byte(r.Body)), nil
}

// relativeTime returns a human-readable age string ("2 d", "3 w",
// "4 mo", "never") — without "ago" suffix, matching the runbooks
// fixture's "last" field shape.
func relativeTime(t core.Time, now core.Time) string {
	if t.IsZero() {
		return "never"
	}
	diff := now.Sub(t)
	if diff < 0 {
		diff = -diff
	}
	const hour = core.Hour
	const day = 24 * hour
	const week = 7 * day
	const month = 30 * day

	switch {
	case diff < day:
		return "< 1 d"
	case diff < 2*week:
		return core.Sprintf("%d d", int(diff/day))
	case diff < month:
		return core.Sprintf("%d w", int(diff/week))
	default:
		return core.Sprintf("%d mo", int(diff/month))
	}
}

// computeHealth returns "fresh", "aging", or "stale" based on the
// last-rehearsed timestamp.
func computeHealth(r RunbookRecord, now core.Time) string {
	if r.LastRehearsed.IsZero() {
		return "stale"
	}
	diff := now.Sub(r.LastRehearsed)
	switch {
	case diff < FreshThreshold:
		return "fresh"
	case diff < AgingThreshold:
		return "aging"
	default:
		return "stale"
	}
}

// toEntry converts a RunbookRecord to the wire type.
func toEntry(r RunbookRecord, now core.Time) RunbookEntry {
	return RunbookEntry{
		ID:     r.ID,
		Title:  r.Title,
		Area:   r.Area,
		Last:   relativeTime(r.LastRehearsed, now),
		Health: computeHealth(r, now),
	}
}

// containsCI returns true when needle appears (case-insensitive) in
// the haystack byte slice. Avoids the banned `strings` package by
// lowercasing both sides via toLower.
func containsCI(haystack, needle string) bool {
	if needle == "" {
		return true
	}
	h := toLower(haystack)
	n := toLower(needle)
	if len(n) > len(h) {
		return false
	}
	for i := 0; i <= len(h)-len(n); i++ {
		match := true
		for j := 0; j < len(n); j++ {
			if h[i+j] != n[j] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

// toLower lowercases ASCII only — sufficient for runbook search terms
// (titles, area slugs, IDs, tags). Non-ASCII passes through unchanged.
func toLower(s string) string {
	b := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 32
		} else {
			b[i] = c
		}
	}
	return string(b)
}

// matchSearch returns true when the runbook matches the search query
// (case-insensitive substring across title, area, id, and tags).
func matchSearch(r RunbookRecord, q string) bool {
	if q == "" {
		return true
	}
	if containsCI(r.Title, q) || containsCI(r.Area, q) || containsCI(r.ID, q) {
		return true
	}
	for _, tag := range r.Tags {
		if containsCI(tag, q) {
			return true
		}
	}
	return false
}

// scanAll reads runbook files in ~/Lethean/runbooks/ and returns
// RunbookRecord values. Returns empty slice (not error) when the
// directory is missing or empty.
//
// Dual-format read (RFC.stage-e-encrypt-at-rest v2 §4.1, Wave 2):
//   - .lthn files are decoded HEADER-ONLY via the substrate's
//     DecodeHeader path (no decrypt, MAC-verified). Title / Area /
//     Body / LastRehearsed / CreatedAt stay sealed. The returned
//     RunbookRecord has header-visible fields (ID, Version, hash-
//     projected Tags) populated.
//   - .md files parse as legacy plaintext for backward-compat.
//   - Prefer .lthn over .md when both exist for the same slug
//     (cutover invariant — once the encrypted record lands the
//     plaintext gets removed, but a crash between AtomicWrite and
//     Remove could leave both on disk).
//
// MAC-failure entries are SKIPPED (RFC §4.1: "do NOT abort whole List
// on one bad file"). When the session is locked the .lthn branch
// still emits a degraded record (ID + Version + hash-tags from
// header) because header MAC verification only needs the PUBLIC key —
// no unlock required.
//
// The s receiver may be nil for tests of the legacy plaintext-only
// path; the at-rest header-only branch is skipped when s is nil so
// existing no-Service helpers (package-level scans in tests) keep
// working.
func (s *Service) scanAll() ([]RunbookRecord, error) {
	dir := runbooksDir()
	if !dir.OK {
		return nil, core.E("runbooks.scanAll", dir.Error(), nil)
	}
	dirPath := dir.Value.(string)

	entriesR := core.ReadDir(core.DirFS(dirPath), ".")
	if !entriesR.OK {
		return nil, nil
	}
	entries, _ := entriesR.Value.([]core.FsDirEntry)

	// Two-pass pick: prefer .lthn over .md when both exist for the
	// same slug. Stable name basis: filename without extension.
	seen := map[string]bool{}
	var records []RunbookRecord

	// First pass: .lthn (encrypted, header-only). Only engaged when
	// the at-rest writer can be built — without a wired SessionGate +
	// keys provider, the consumer falls back to legacy .md scan only
	// (legacy services that haven't been retrofitted yet still work).
	_, hasAtRest := s.atrestWriterFor()
	if hasAtRest {
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			name := entry.Name()
			if len(name) < 5 || name[len(name)-5:] != ".lthn" {
				continue
			}
			slug := name[:len(name)-5]
			fpath := core.PathJoin(dirPath, name)
			rec, err := s.loadHeaderOnly(fpath, slug)
			if err != nil {
				// MAC-failure / corrupt envelope / decode error —
				// SKIP per RFC §4.1.
				continue
			}
			seen[slug] = true
			records = append(records, rec)
		}
	}

	// Second pass: .md (legacy plaintext) for slugs not already
	// covered.
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if len(name) < 4 || name[len(name)-3:] != ".md" {
			continue
		}
		slug := name[:len(name)-3]
		if seen[slug] {
			continue
		}
		fpath := core.PathJoin(dirPath, name)
		raw := core.ReadFile(fpath)
		if !raw.OK {
			continue
		}
		rec, err := parseRecord(slug, raw.Value.([]byte))
		if err != nil {
			continue
		}
		records = append(records, rec)
	}
	return records, nil
}

// loadOne finds a runbook by ID or slug and returns the full record
// with Body populated. Dual-format (RFC §3.1):
//   - .lthn is preferred. Decrypt requires an unlocked session; on a
//     locked-session read this returns the typed runbooks.session.
//     locked error so callers (Get) surface the lock state instead of
//     leaking "file not found" confusion.
//   - .md is the legacy plaintext fallthrough.
//
// The s receiver may be nil for tests of the legacy plaintext-only
// path; the at-rest branch is skipped when s is nil.
func (s *Service) loadOne(id, slug string) (RunbookRecord, string, error) {
	dir := runbooksDir()
	if !dir.OK {
		return RunbookRecord{}, "", core.E("runbooks.loadOne", dir.Error(), nil)
	}
	dirPath := dir.Value.(string)

	// Cheap-listing: scan once to translate id↔slug. Header-only
	// projection is enough to resolve the basename, then we full-load
	// the chosen record below.
	records, err := s.scanAll()
	if err != nil {
		return RunbookRecord{}, "", err
	}
	target := ""
	for _, r := range records {
		if (id != "" && r.ID == id) || (slug != "" && r.Slug == slug) {
			target = r.Slug
			break
		}
	}
	if target == "" {
		label := id
		if label == "" {
			label = slug
		}
		return RunbookRecord{}, "", core.E("runbooks.loadOne", "not found: "+label, nil)
	}

	// .lthn first — encrypted records take precedence when present.
	lthnPath, jerr := paths.JoinAndCheck(dirPath, target+".lthn")
	if jerr != nil {
		return RunbookRecord{}, "", jerr
	}
	if exists := core.Stat(lthnPath); exists.OK {
		w, ok := s.atrestWriterFor()
		if !ok {
			return RunbookRecord{}, "",
				core.E("runbooks.loadOne", "runbooks.session.locked", nil)
		}
		rr := w.Read(lthnPath)
		if !rr.OK {
			return RunbookRecord{}, "", core.E("runbooks.loadOne", rr.Error(), nil)
		}
		res, _ := rr.Value.(recordfile.ReadResult)
		rec, perr := parseAtRestRecord(target, res)
		if perr != nil {
			return RunbookRecord{}, "", perr
		}
		return rec, dirPath, nil
	}

	// Legacy plaintext .md fallthrough.
	mdPath, jerr := paths.JoinAndCheck(dirPath, target+".md")
	if jerr != nil {
		return RunbookRecord{}, "", jerr
	}
	raw := core.ReadFile(mdPath)
	if !raw.OK {
		return RunbookRecord{}, "",
			core.E("runbooks.loadOne", "not found: "+target, nil)
	}
	rec, err := parseRecord(target, raw.Value.([]byte))
	if err != nil {
		return RunbookRecord{}, "", err
	}
	return rec, dirPath, nil
}

// writeRecord serialises a RunbookRecord back to disk via the legacy
// plaintext path. Service-tier callers should use
// `(*Service).writeRecord` (below) which dual-paths into the at-rest
// substrate when the SessionGate is wired. This free-function shape
// stays available for tests via WriteRecordExported and as the
// fallback path inside the method.
//
// ifVersion is the optimistic-lock anchor — pass the Version the
// caller observed on disk (via parseRecord through loadOne), or 0 for
// first-writes / legacy-file upgrades. writeRecord stamps the next
// monotonic version (ifVersion+1) into the marshalled frontmatter so
// subsequent reads see version=1,2,3... monotonically.
//
// Cerberus #1486: slug lands directly in the filename; validate before
// the join even though loadOne paths route through it.
// Cerberus #1487 PR-1: 0o600 — owner-only at rest (applied by the
// primitive's atomic-rename path).
//
// Return shape (Mantis #1544 gating, inherited from W1+W2): on the
// stale-write path the function returns
// core.Fail(paths.ConflictEnvelope{...}) so the Wails-marshalled
// Result.Value carries the lowercase-json shape
// (`{code, current_version, current_hash}`) that
// conflict-dispatch.ts extractEnvelope pattern-matches on. The
// per-service Code is "runbooks.update.conflict" — the frontend
// scopes its toast + reload-listener on this exact string.
//
// Audit emission is automatic via paths.AuditModeForPath —
// runbooks/* paths route through AuditModeBatch per RFC §6.1.
//
// Usage example:
//
//	if wr := writeRecord(rec, dirPath, prior.Version); !wr.OK {
//	    return wr
//	}
func writeRecord(r RunbookRecord, dirPath string, ifVersion int) core.Result {
	if err := paths.IsValidID(r.Slug); err != nil {
		return core.Fail(err)
	}
	// Stamp the next monotonic version. ifVersion=0 (Create / legacy
	// upgrade) yields version=1; subsequent writes increment.
	nextVersion := ifVersion + 1
	if nextVersion < 1 {
		nextVersion = 1
	}
	r.Version = nextVersion
	raw, err := marshalRecord(r)
	if err != nil {
		return core.Fail(core.E("runbooks.writeRecord", "marshal", err))
	}
	// Cerberus #1486 belt-and-braces (Mantis #1607, forward-arc from
	// H#85): WithinDir check after the join. IsValidID above is the
	// cheap shape gate; JoinAndCheck refuses any path that resolves
	// outside dirPath even if cousin-validator drift weakens IsValidID.
	target, jerr := paths.JoinAndCheck(dirPath, r.Slug+".md")
	if jerr != nil {
		return core.Fail(jerr)
	}
	res := paths.AtomicWriteWithVersion(target, paths.WriteInput{
		Body:      raw,
		IfVersion: ifVersion,
	})
	if res.OK {
		return res
	}
	if stale, ok := paths.VersionStaleFromError(res.Value); ok {
		return core.Fail(paths.NewConflictEnvelope(
			"runbooks.update.conflict", stale))
	}
	return core.Fail(core.E("runbooks.writeRecord",
		"write failed: "+res.Error(), nil))
}

// writeRecord is the Service-tier write entry point used by MarkRehearsed.
//
// Stage E.D.B.2 dual-path (RFC.stage-e-encrypt-at-rest v2 §3, Wave 2):
//   - When the SessionGate is wired AND reports exactly one unlocked
//     account AND satisfies accountKeyProvider, writes the encrypted
//     .lthn envelope via the recordfile.AtRestWriter substrate. After
//     successful encrypted write, removes the legacy <slug>.md when
//     present (lazy migration completion per §3.1).
//   - When the gate is unwired OR the gate doesn't satisfy the wider
//     keys surface (test fixtures pre-#1613 retrofit), falls through
//     to the legacy plaintext writer above. This preserves existing
//     Cascade W3 behaviour for any caller not yet wired to a session
//     source. Once #1613 + #1487 ship across all consumers this branch
//     goes away.
//
// ifVersion is the optimistic-lock anchor — pass the Version the
// caller observed on disk before mutating the record, or 0 for first-
// writes / legacy-file upgrades. writeRecord stamps r.Version=
// ifVersion+1 into the marshalled body.
//
// Cerberus #1486: slug lands directly in the filename; IsValidID gates
// it before JoinAndCheck (belt-and-braces).
//
// Return shape (Mantis #1544): conflict path returns
// core.Fail(paths.ConflictEnvelope{...}) with code
// "runbooks.update.conflict". Non-conflict failures propagate via
// core.Fail(core.E(...)).
//
// Usage example:
//
//	if wr := s.writeRecord(rec, dirPath, prior.Version); !wr.OK {
//	    return wr
//	}
func (s *Service) writeRecord(r RunbookRecord, dirPath string, ifVersion int) core.Result {
	if err := paths.IsValidID(r.Slug); err != nil {
		return core.Fail(err)
	}
	// Stamp the next monotonic version. ifVersion=0 (Create / legacy
	// upgrade) yields version=1; subsequent updates increment.
	r.Version = ifVersion + 1
	if r.Version < 1 {
		r.Version = 1
	}

	// At-rest path — preferred when the gate is wired.
	if w, ok := s.atrestWriterFor(); ok {
		return s.writeRecordAtRest(w, r, dirPath)
	}

	// Legacy plaintext fallback — gate unwired (test fixtures only).
	// Re-uses the free-function path so the call site stays a single
	// place to audit for the .md write semantics.
	return writeRecord(r, dirPath, ifVersion)
}

// writeRecordAtRest encrypts + writes via the substrate. Body is the
// YAML frontmatter (RunbookRecord without Body) + the Body string;
// header carries the hash-projected tags per §2.4. After success,
// removes the legacy <slug>.md if present (lazy migration per §3.1).
//
// The substrate's IfMatch optimistic-lock uses hex(sha256(prior
// ciphertext)). For first-writes (no prior file) the empty PriorHash
// is the correct IfMatch input. For updates we read the prior
// ciphertext bytes here so the IfMatch hash is well-formed.
func (s *Service) writeRecordAtRest(w *recordfile.AtRestWriter[RunbookRecord], r RunbookRecord, dirPath string) core.Result {
	// Marshal a copy without Body — substrate carries Body via the
	// BodyText field which gets concatenated after the YAML frontmatter
	// in the encrypted plaintext. Don't double-emit.
	yamlSrc := r
	yamlSrc.Body = ""
	yamlBody, err := yaml.Marshal(yamlSrc)
	if err != nil {
		return core.Fail(core.E("runbooks.writeRecord", "yaml marshal", err))
	}
	target, jerr := paths.JoinAndCheck(dirPath, r.Slug+".lthn")
	if jerr != nil {
		return core.Fail(jerr)
	}
	priorHash := ""
	if existing := core.ReadFile(target); existing.OK {
		priorHash = core.SHA256Hex(existing.Value.([]byte))
	}

	wr := w.Write(recordfile.WriteRequest[RunbookRecord]{
		Record:    r,
		BodyYAML:  yamlBody,
		BodyText:  []byte(r.Body),
		DestPath:  target,
		PriorHash: priorHash,
		// AccountID: deliberately empty — substrate uses
		// SingleUnlockedAccount() as the canonical id.
	})
	if !wr.OK {
		// Forward the typed substrate error verbatim — its Code names
		// (recordfile.atrest.*) are the contract Stage F audit binds.
		return wr
	}

	// Lazy-migration completion: remove the legacy plaintext file when
	// the encrypted write landed cleanly. Failure is tolerated (RFC
	// §3.1: warn-on-failure, do NOT abort the encrypted write) — the
	// next read will see both files, prefer .lthn, and the cleanup
	// can be re-attempted by the bulk-migrate CLI (§3.3).
	mdPath, jerr := paths.JoinAndCheck(dirPath, r.Slug+".md")
	if jerr == nil {
		if existsMd := core.Stat(mdPath); existsMd.OK {
			if rm := core.Remove(mdPath); !rm.OK {
				core.Warn("runbooks: failed to remove legacy plaintext after encrypted write",
					"path", mdPath, "err", rm.Error())
			}
		}
	}

	// Synthesise a paths.WriteOutput-shaped success Value so existing
	// callers (MarkRehearsed) keep their typed expectations. The
	// Version field is surfaced from the substrate receipt.
	receipt, _ := wr.Value.(recordfile.WriteReceipt)
	return core.Ok(paths.WriteOutput{
		Version: int(receipt.Version),
		Hash:    "", // future: feed back from substrate when needed
	})
}

// fireEvent publishes a runbook event on the Core ACTION bus.
func (s *Service) fireEvent(name string, entry RunbookEntry) {
	if s == nil || s.core == nil {
		return
	}
	s.core.ACTION(RunbookEvent{
		EventName: name,
		Entry:     entry,
		At:        core.Now().UTC(),
	})
}

// SetSessionGate wires the live-read session source. Called by
// cmd/lthn/app.go post-construction (Mantis #1613 B.3) once
// *account.Service exists.
//
// Mirrors the office/mail.AccountProvider setter pattern. Live-read on
// every gate check — no event-bus reliability concerns, no cache
// coherence concerns (RFC.stage-e-unlockgate v2 §1.1).
//
// Usage example:
//
//	runbooksSvc.SetSessionGate(accountSvc)
func (s *Service) SetSessionGate(g SessionGate) {
	s.gateMu.Lock()
	s.gate = g
	s.gateMu.Unlock()
	// Invalidate any cached AtRestWriter so a re-wire (e.g. account
	// service rotation) rebuilds adapters against the fresh gate.
	s.atrestMu.Lock()
	s.atrestWriter = nil
	s.atrestMu.Unlock()
}

// Stop nils the SessionGate reference so a draining Service
// fails-closed on any late-arriving write (§B.2 mirror mail's drain
// hygiene / Cerberus #28 ADD-5). Read-only methods (List, Get, Search)
// continue to function — Stop only severs the write gate.
//
// Usage example:
//
//	_ = svc.Stop(core.Background())
func (s *Service) Stop(_ core.Context) core.Result {
	s.gateMu.Lock()
	s.gate = nil
	s.gateMu.Unlock()
	s.atrestMu.Lock()
	s.atrestWriter = nil
	s.atrestMu.Unlock()
	return core.Ok(nil)
}

// assertUnlocked returns a Fail result when the session is locked or
// the session gate is not wired. Called at the top of every write
// method before any FS touch.
//
// Live-read semantics (RFC §1.1): consults s.gate.UnlockedAccountIDs()
// at every call — no cached bool — so a lock transition is observable
// on the very next write attempt.
//
// Fail-safe on nil gate (§2.2 / Cerberus #28 Q2): when SetSessionGate
// has not yet wired the gate (or Stop has nilled it), the gate fails
// LOCKED rather than panicking. The first nil-hit per Service
// instance emits a one-shot core.Warn via CompareAndSwap so
// wire-ordering bugs surface in dev without log spam in production.
//
// Usage example:
//
//	if fail, ok := s.assertUnlocked("runbooks.MarkRehearsed"); !ok {
//	    return fail
//	}
func (s *Service) assertUnlocked(scope string) (core.Result, bool) {
	s.gateMu.RLock()
	g := s.gate
	s.gateMu.RUnlock()
	if g == nil {
		if s.nilWarned.CompareAndSwap(false, true) {
			core.Warn("runbooks: session gate not wired; failing locked", "scope", scope)
		}
		return core.Fail(core.E(scope, "runbooks.session.locked", nil)), false
	}
	if len(g.UnlockedAccountIDs()) == 0 {
		return core.Fail(core.E(scope, "runbooks.session.locked", nil)), false
	}
	return core.Result{}, true
}

// buildCounts tallies health across all records.
func buildCounts(records []RunbookRecord, now core.Time) (fresh, aging, stale int) {
	for _, r := range records {
		switch computeHealth(r, now) {
		case "fresh":
			fresh++
		case "aging":
			aging++
		default:
			stale++
		}
	}
	return
}

// seedAll writes the seven default runbooks derived from the frontend
// fixture data. Called once on startup when ~/Lethean/runbooks/ is empty.
func seedAll(dirPath string) {
	now := core.Now().UTC()
	// Build seed records matching the fixture data in runbooks.ts.
	seeds := []struct {
		rec          RunbookRecord
		rehearsedAgo core.Duration // negative = in past
	}{
		{RunbookRecord{ID: "R-01", Title: "Rotate runtime API keys", Area: "auth", Tags: []string{"auth", "api-keys", "security"}, Slug: "rotate-runtime-api-keys", CreatedAt: now}, 2 * 24 * core.Hour},
		{RunbookRecord{ID: "R-02", Title: "Recover from corrupt model directory", Area: "runtime", Tags: []string{"runtime", "model", "recovery"}, Slug: "recover-from-corrupt-model-directory", CreatedAt: now}, 3 * 7 * 24 * core.Hour},
		{RunbookRecord{ID: "R-03", Title: "Rollback bad deploy · production", Area: "deploy", Tags: []string{"deploy", "rollback", "production"}, Slug: "rollback-bad-deploy-production", CreatedAt: now}, 5 * 24 * core.Hour},
		{RunbookRecord{ID: "R-04", Title: "Reset DNS · cache poisoning incident", Area: "network", Tags: []string{"network", "dns", "security"}, Slug: "reset-dns-cache-poisoning-incident", CreatedAt: now}, 2 * 7 * 24 * core.Hour},
		{RunbookRecord{ID: "R-05", Title: "Drain a stuck Postfix queue", Area: "mail", Tags: []string{"mail", "postfix", "queue"}, Slug: "drain-a-stuck-postfix-queue", CreatedAt: now}, 4 * 30 * 24 * core.Hour},
		{RunbookRecord{ID: "R-06", Title: "Trigger emergency model unload", Area: "runtime", Tags: []string{"runtime", "model", "emergency"}, Slug: "trigger-emergency-model-unload", CreatedAt: now}, 30 * 24 * core.Hour},
		{RunbookRecord{ID: "R-07", Title: "Restore tray-app from corrupt config", Area: "client", Tags: []string{"client", "config", "recovery"}, Slug: "restore-tray-app-from-corrupt-config", CreatedAt: now}, 6 * 30 * 24 * core.Hour},
	}
	for _, s := range seeds {
		rec := s.rec
		rec.LastRehearsed = now.Add(-s.rehearsedAgo)
		rec.Body = "# " + rec.Title + "\n\nThis is a seed runbook. " +
			"Replace this with your organisation's actual procedure.\n"
		// Stamp the seed at version 1 so subsequent edits see the
		// monotonic anchor immediately (Cascade W3, RFC §B.3 row 9).
		rec.Version = 1
		raw, err := marshalRecord(rec)
		if err != nil {
			continue
		}
		// Cerberus #1486 belt-and-braces (Mantis #1607, forward-arc from
		// H#85): WithinDir check after the join. Seed slugs are
		// developer-controlled constants today, but JoinAndCheck closes
		// the door on future churn where the seed list might pull from
		// fixtures or user-editable config without re-auditing the path
		// surface. On escape, log + skip (consistent with the seedAll
		// best-effort discipline established in #1572).
		target, jerr := paths.JoinAndCheck(dirPath, rec.Slug+".md")
		if jerr != nil {
			core.Print(core.Stderr(),
				"runbooks: seedAll path escape for %s: %s\n",
				rec.Slug, jerr.Error())
			continue
		}
		// Cascade W3 (RFC §B.3 row 9) — seed-path replaces the prior
		// bare `core.WriteFile` with an unconditional first-write
		// through paths.AtomicWriteWithVersion. Per RFC §3.2 lazy-
		// migration semantics, an empty WriteInput (no IfVersion /
		// IfMtime / IfMatchHash) treats this as a legitimate
		// unconditional first-write. seedAll() is invoked once on
		// first start when the runbooks directory is empty (see
		// OnStart + List entry-points) so there's no concurrent
		// writer to race against.
		// 0o600 (Cerberus #1487 PR-1): runbooks carry operational
		// secrets; even seed content uses the production mode,
		// applied inside the primitive's atomic-rename path.
		// Mantis #1572 (Cerberus #12 F5): log on !OK rather than
		// silently swallow — seedAll is best-effort (called from
		// OnStart, single caller) so an individual seed failure
		// shouldn't abort the rest, but it MUST surface.
		if wr := paths.AtomicWriteWithVersion(target, paths.WriteInput{
			Body: raw,
		}); !wr.OK {
			core.Print(core.Stderr(),
				"runbooks: seedAll write failed for %s: %s\n",
				rec.Slug, wr.Error())
		}
	}
}

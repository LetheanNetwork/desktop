// SPDX-Licence-Identifier: EUPL-1.2

// Service — Core integration for the deals surface. Reads and
// writes Trix-style markdown files from ~/Lethean/sales/deals/.
// Deals are also the source-of-truth for pipeline stage — pkg/sales/pipeline
// derives its Kanban view by scanning this directory.
//
// Lifecycle:
//   - Register(c)        wires the service into the Core container
//   - ServiceName()      returns "Deals" for the Wails namespace
//   - SetSessionGate(g)  wired post-construction in cmd/lthn/app.go
//     against *account.Service (live-read pattern — mirrors
//     sales/contacts + office/mail.AccountProvider; no cached bool,
//     no event bus).
//   - Stop(ctx)          nils the SessionGate reference so a draining
//     Service fails-closed on any late-arriving write.
//
// All I/O uses CoreGO wrappers (core.ReadFile / core.WriteFile /
// core.ReadDir / core.DirFS / core.MkdirAll / core.PathJoin).
// Banned stdlib imports: os, path/filepath, strings, encoding/json,
// fmt, log, errors.

package deals

import (
	"sync"
	"sync/atomic"

	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/account"
	"dappco.re/lthn/desktop/pkg/paths"
	"dappco.re/lthn/desktop/pkg/recordfile"
	"forge.lthn.ai/Snider/Enchantrix/pkg/crypt/std/pgp"
	"gopkg.in/yaml.v3"
)

// SessionGate is the minimal consumer-defined interface satisfied by
// *account.Service. Live-read at every gate check — no cached bool, no
// subscribe/event bus (RFC.stage-e-unlockgate v2 §1.1 — Pushback 2
// CONFIRMED by Cerberus #27). When the returned slice is empty the
// session is locked; when non-empty at least one Lethean account is
// unlocked and writes may proceed.
//
// Wired in cmd/lthn/app.go (Mantis #1613 B.3):
//
//	dealsSvc.SetSessionGate(accountSvc)
//
// AX-8 compliance: this interface is defined in the consumer (deals)
// and satisfied by the producer (*account.Service). The duplication
// across sales/contacts, incidents, runbooks, deals etc. IS the AX-8
// boundary — each consumer owns its own contract.
//
// Stage E.D.B.1 RFC v2 §5.1 (Mantis #1487) adds a SEPARATE
// at-rest-keys lookup via a runtime type-assertion against the
// AccountKeyProvider shape below. The narrow SessionGate stays
// minimal so the existing app.go wire + sibling-consumer stubs keep
// working; the wider keys surface is opt-in by the implementer.
type SessionGate interface {
	UnlockedAccountIDs() []string
}

// AccountKeyProvider is the wider runtime-assertion surface the
// at-rest path expects on a wired SessionGate. *account.Service
// satisfies it today (PublicKeyFor at unlock.go:903, PrivateKeyFor
// at unlock.go:870). Test fixtures stub it independently — see the
// stubSessionGate in deals_test.go.
//
// Kept package-private — consumers wire the producer-side directly
// via SetSessionGate; this is the substrate's view of what the gate
// must additionally satisfy to engage the encrypted write path.
//
//	if kp, ok := s.gate.(accountKeyProvider); ok { ... }
type accountKeyProvider interface {
	PublicKeyFor(accountID string) ([]byte, bool)
	PrivateKeyFor(accountID string) (*account.PrivateKeyHandle, bool)
}

// Service owns the deals surface.
//
// Usage example:
//
//	svc := deals.NewService(c)
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
	// (§2.2 / Cerberus #27 Q2). CompareAndSwap-on-first-hit emits
	// core.Warn exactly once per Service instance.
	nilWarned atomic.Bool

	// atrestMu serialises lazy construction of atrestWriter so the
	// first concurrent write doesn't double-build the substrate.
	atrestMu sync.Mutex
	// atrestWriter is built on first writer-access against the live
	// SessionGate (RFC.stage-e-encrypt-at-rest v2 §5). nil before the
	// first use; replaced on every SetSessionGate so adapter→gate
	// remains coherent.
	atrestWriter *recordfile.AtRestWriter[DealRecord]
}

// NewService constructs the deals service against a Core container.
// Wired via core.WithName("sales-deals", deals.Register) in app.go.
//
// Usage example:
//
//	svc := deals.NewService(c)
func NewService(c *core.Core) *Service {
	return &Service{core: c}
}

// Register constructs the deals service for Core registration.
//
// Usage example:
//
//	core.New(core.WithName("sales-deals", deals.Register))
func Register(c *core.Core) core.Result {
	return core.Ok(NewService(c))
}

// ServiceName labels the binding namespace exposed to JS.
// Wails binds methods as "Deals.List()" etc.
func (s *Service) ServiceName() string { return "Deals" }

// dealsDir resolves ~/Lethean/sales/deals/ and creates it if missing.
// Mode 0o700 (Cerberus #1487 PR-1): deal records carry sensitive
// commercial data (customer names, amounts in pence, stakeholders,
// activity log) — owner-only at rest.
func dealsDir() core.Result {
	root := paths.Root()
	if !root.OK {
		return root
	}
	dir := core.PathJoin(root.Value.(string), "sales", "deals")
	if r := core.MkdirAll(dir, 0o700); !r.OK {
		return r
	}
	return core.Ok(dir)
}

// knownStages is the canonical ordered stage set.
// Matches the pipeline.ts fixture ids.
var knownStages = map[string]string{
	"qual":    "Qualifying",
	"engage":  "Engaging",
	"propose": "Proposal",
	"close":   "Closing",
	"won":     "Won",
	"lost":    "Lost",
}

// stageLabel maps a stage id to its human label.
func stageLabel(id string) string {
	if label, ok := knownStages[id]; ok {
		return label
	}
	return id
}

// isValidStage reports whether the given stage id is in the known set.
func isValidStage(id string) bool {
	_, ok := knownStages[id]
	return ok
}

// formatGBPK formats integer pence as "£N K" (threshold 1000p = £10).
// Values below 1000p are formatted as "£N".
func formatGBPK(pence int) string {
	if pence >= 1000 {
		k := (pence + 500) / 1000 // round to nearest £K
		return core.Sprintf("£%d K", k)
	}
	pounds := pence / 100
	return core.Sprintf("£%d", pounds)
}

// activityTS formats a timestamp relative to now for activity log display.
// Returns "today · HH:MM" for same calendar day, "yest · HH:MM" for
// yesterday, or "N d ago" otherwise.
func activityTS(t core.Time, now core.Time) string {
	if t.IsZero() {
		return ""
	}
	diff := now.Sub(t)
	if diff < 0 {
		diff = -diff
	}
	const day = 24 * core.Hour

	// Same calendar day check by comparing date parts.
	tLocal := t.UTC()
	nLocal := now.UTC()
	sameDay := core.TimeFormat(tLocal, "2006-01-02") == core.TimeFormat(nLocal, "2006-01-02")
	if sameDay {
		return core.Sprintf("today · %s", core.TimeFormat(tLocal, "15:04"))
	}
	yesterday := now.Add(-day)
	yLocal := yesterday.UTC()
	prevDay := core.TimeFormat(tLocal, "2006-01-02") == core.TimeFormat(yLocal, "2006-01-02")
	if prevDay {
		return core.Sprintf("yest · %s", core.TimeFormat(tLocal, "15:04"))
	}
	days := int(diff / day)
	if days < 7 {
		return core.Sprintf("%d d ago", days)
	}
	weeks := days / 7
	return core.Sprintf("%d w ago", weeks)
}

// slugify converts a customer name to a filesystem-safe slug.
func slugify(name string) string {
	b := []byte(name)
	out := make([]byte, 0, len(b))
	prevHyphen := false
	for _, ch := range b {
		var c byte
		switch {
		case ch >= 'A' && ch <= 'Z':
			c = ch + 32
		case ch >= 'a' && ch <= 'z':
			c = ch
		case ch >= '0' && ch <= '9':
			c = ch
		default:
			c = '-'
		}
		if c == '-' {
			if prevHyphen {
				continue
			}
			prevHyphen = true
		} else {
			prevHyphen = false
		}
		out = append(out, c)
	}
	for len(out) > 0 && out[len(out)-1] == '-' {
		out = out[:len(out)-1]
	}
	return string(out)
}

// countDeals returns the number of deal record files in the dir.
// Counts both legacy ".md" plaintext and ".lthn" encrypted records so
// the {YYYYMM}-DEAL-NNN suffix counter advances across the cutover.
// Used to generate NNN suffixes for new deal IDs.
func countDeals(dir string) int {
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
		if hasSuffix(nm, ".md") || hasSuffix(nm, ".lthn") {
			n++
		}
	}
	return n
}

// hasSuffix mirrors core.HasSuffix without dragging the import in here
// where the AX-6 wrapper isn't already needed for any other call.
func hasSuffix(s, suffix string) bool {
	return len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix
}

// --- At-rest substrate plumbing (RFC.stage-e-encrypt-at-rest v2) -----

// dealStageEnumOut maps the deals-internal stage vocabulary (qual /
// engage / propose / close / won / lost — see knownStages) onto the
// canonical RFC §2.4 SalesDealsStages enum that the at-rest header
// MUST emit. Body retains the original internal value via the YAML
// frontmatter — the header is the LIST/search projection only.
//
//	hdrStage := dealStageEnumOut("close") // → "negotiate"
func dealStageEnumOut(internal string) string {
	switch internal {
	case "qual", "engage", "propose":
		return internal
	case "close":
		return "negotiate"
	case "won":
		return "closed_won"
	case "lost":
		return "closed_lost"
	}
	// Unknown / future stages → "on_hold" (still a valid enum member;
	// keeps the encode-side schema validator happy and pushes the
	// curation problem back to the canonical knownStages table rather
	// than into the at-rest substrate).
	return "on_hold"
}

// dealsHeaderSchema declares the RFC §2.4 per-field header whitelist
// for the sales/deals surface. Stage is bucketed to the canonical
// enum; CloseTarget is bucketed to YYYY-MM (when parseable). All
// customer / amount / activity / notes data stays in the encrypted
// body — never in the header.
//
//	w := recordfile.NewAtRestWriter(recordfile.AtRestDeps[DealRecord]{
//	    ..., Schema: dealsHeaderSchema,
//	})
var dealsHeaderSchema = recordfile.HeaderSchema[DealRecord]{
	Surface: recordfile.SurfaceSalesDeals,
	AllowedKeys: map[string]recordfile.FieldValidator{
		"stage":        recordfile.ValidateEnum(recordfile.SalesDealsStages),
		"close.target": recordfile.ValidateMonthBucket,
	},
	HeaderFor: func(r DealRecord) map[string]any {
		out := map[string]any{
			"stage": dealStageEnumOut(r.Stage),
		}
		// close.target is human-shaped ("14 Jun") today; only emit the
		// header field when the consumer-supplied string parses as a
		// strict YYYY-MM (Q5-friendly — incidental clean-data paths
		// pass through; legacy free-form values stay BODY-only by
		// being absent from the header). The validator would reject
		// any free-form string at encode anyway.
		if isStrictYYYYMM(r.CloseTarget) {
			out["close.target"] = r.CloseTarget
		}
		return out
	},
	IDFor:      func(r DealRecord) string { return r.ID },
	VersionFor: func(r DealRecord) int64 { return int64(r.Version) },
}

// isStrictYYYYMM is the internal predicate that mirrors the substrate's
// ValidateMonthBucket parser. Keeps deals self-contained without
// re-exporting the substrate's private predicate.
func isStrictYYYYMM(s string) bool {
	if len(s) != 7 || s[4] != '-' {
		return false
	}
	for i := 0; i < 4; i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	if s[5] < '0' || s[5] > '1' {
		return false
	}
	if s[6] < '0' || s[6] > '9' {
		return false
	}
	month := int(s[5]-'0')*10 + int(s[6]-'0')
	return month >= 1 && month <= 12
}

// gateAccountKeys is the consumer-side adapter that satisfies
// recordfile.AccountKeys against the deals SessionGate's wider
// accountKeyProvider runtime assertion. The substrate never imports
// pkg/account directly — this thin wrapper bridges the producer's
// concrete PrivateKeyHandle type onto the substrate's interface AND
// computes SingleUnlockedAccount from UnlockedAccountIDs (the same
// shape pkg/office/mail's singleUnlockedAccount uses — Mantis #1591).
//
//	keys := gateAccountKeys{gate: dealsSvc.gate, keys: kpProvider}
//	w := recordfile.NewAtRestWriter(recordfile.AtRestDeps[DealRecord]{Keys: keys, ...})
type gateAccountKeys struct {
	gate SessionGate
	keys accountKeyProvider
}

// PublicKeyFor wraps the gate's PublicKeyFor; the (bytes, bool) shape
// is rewritten as (bytes, error) per the substrate's contract. Missing
// keys produce a typed error rather than a silent empty slice so the
// substrate's len(pub)==0 check has structured context to report.
func (g gateAccountKeys) PublicKeyFor(accountID string) ([]byte, error) {
	pub, ok := g.keys.PublicKeyFor(accountID)
	if !ok {
		return nil, core.NewCode("deals.atrest.public_key_unavailable",
			core.Sprintf("PublicKeyFor(%q) returned not-ok", accountID))
	}
	return pub, nil
}

// PrivateKeyFor wraps the producer's PrivateKeyFor and bridges the
// concrete *account.PrivateKeyHandle return onto the substrate's
// recordfile.PrivateKeyHandle interface. The concrete handle already
// satisfies the interface shape (Use(func([]byte) error) error) — the
// wrapper is just the type-conversion seam.
func (g gateAccountKeys) PrivateKeyFor(accountID string) (recordfile.PrivateKeyHandle, bool) {
	h, ok := g.keys.PrivateKeyFor(accountID)
	if !ok || h == nil {
		return nil, false
	}
	return h, true
}

// SingleUnlockedAccount mirrors pkg/office/mail's singleUnlockedAccount
// (Mantis #1591) against the deals gate. Multi-unlock returns the same
// typed code the substrate surfaces back to the consumer.
func (g gateAccountKeys) SingleUnlockedAccount() (string, error) {
	ids := g.gate.UnlockedAccountIDs()
	if len(ids) == 0 {
		return "", core.NewCode("deals.no_unlocked_account",
			"no Lethean account is unlocked")
	}
	if len(ids) > 1 {
		return "", core.NewCode("deals.multi_account_not_supported",
			"deals does not yet support multiple unlocked accounts (Mantis #1591)")
	}
	return ids[0], nil
}

// pathsAtomicAdapter is the consumer-side adapter that satisfies
// recordfile.AtomicWriter against the existing paths.AtomicWriteWith
// Version + paths.ReadVersion + core.Remove primitives. The IfMatch
// gate is wired through paths.WriteInput.IfMatchHash so the optimistic-
// lock semantics already proven in W1+W2 inherit unchanged.
//
//	atomic := pathsAtomicAdapter{}
//	w := recordfile.NewAtRestWriter(recordfile.AtRestDeps[DealRecord]{Atomic: atomic, ...})
type pathsAtomicAdapter struct{}

// Write routes through paths.AtomicWriteWithVersion. The substrate's
// IfMatch == hex(sha256(prior ciphertext)) matches the primitive's
// IfMatchHash semantics exactly. Empty IfMatch (first-write) means
// the primitive skips the hash check — same as the existing legacy-
// upgrade path. Mode is honoured verbatim (substrate defaults to 0o600
// which matches the surface's existing #1487 PR-1 ruling).
func (pathsAtomicAdapter) Write(req recordfile.AtomicWriteRequest) error {
	r := paths.AtomicWriteWithVersion(req.Path, paths.WriteInput{
		Body:        req.Payload,
		IfMatchHash: req.IfMatch,
		IfNotExist:  req.IfNotExist,
	})
	if r.OK {
		return nil
	}
	return core.NewCode("deals.atrest.atomic_write_failed",
		core.Sprintf("AtomicWriteWithVersion(%q): %s", req.Path, r.Error()))
}

// ReadFile is a thin wrapper around core.ReadFile that adapts the
// (Result) → (bytes, error) shape the substrate expects. Missing
// files surface as core-coded errors so the substrate's read-failed
// path stays grep-stable.
func (pathsAtomicAdapter) ReadFile(path string) ([]byte, error) {
	r := core.ReadFile(path)
	if !r.OK {
		return nil, core.NewCode("deals.atrest.read_failed",
			core.Sprintf("ReadFile(%q): %s", path, r.Error()))
	}
	b, _ := r.Value.([]byte)
	return b, nil
}

// Remove is a thin wrapper around core.Remove. Failures are surfaced
// but tolerated by the substrate's lazy-migration legacy-remove path
// (RFC §3.1: warn-on-failure, do NOT abort the encrypted write).
func (pathsAtomicAdapter) Remove(path string) error {
	r := core.Remove(path)
	if !r.OK {
		return core.NewCode("deals.atrest.remove_failed",
			core.Sprintf("Remove(%q): %s", path, r.Error()))
	}
	return nil
}

// atrestWriterFor returns the lazy-constructed AtRestWriter wired
// against the live SessionGate. Returns (nil, false) when the gate is
// not yet wired — caller falls back to legacy plaintext write.
//
// Construction happens once per Service post-SetSessionGate (atrestMu
// serialises the racy first-call path); SetSessionGate invalidates the
// cache so re-wiring against a fresh accountSvc never reuses stale
// adapter pointers.
//
//	w, ok := s.atrestWriterFor()
//	if !ok { return s.writeLegacy(rec, dir, ifVersion) }
func (s *Service) atrestWriterFor() (*recordfile.AtRestWriter[DealRecord], bool) {
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
	w := recordfile.NewAtRestWriter(recordfile.AtRestDeps[DealRecord]{
		Surface: recordfile.SurfaceSalesDeals,
		Keys:    gateAccountKeys{gate: gate, keys: keys},
		PGP:     pgp.NewService(),
		Schema:  dealsHeaderSchema,
		Atomic:  pathsAtomicAdapter{},
	})
	s.atrestWriter = w
	return w, true
}

// parseRecord splits a Trix file into frontmatter + body via the
// shared recordfile.Split helper and decodes the YAML header into a
// DealRecord. The body is stored in the Notes field.
func parseRecord(raw []byte) (DealRecord, error) {
	fm, body := recordfile.Split(raw)
	var rec DealRecord
	if err := yaml.Unmarshal(fm, &rec); err != nil {
		return DealRecord{}, core.E("deals.parse", "yaml unmarshal", err)
	}
	rec.Notes = string(body)
	return rec, nil
}

// marshalRecord serialises a DealRecord to the Trix file format via
// the shared recordfile.Stitch helper.
func marshalRecord(r DealRecord) ([]byte, error) {
	fm, err := yaml.Marshal(r)
	if err != nil {
		return nil, core.E("deals.marshal", "yaml marshal", err)
	}
	return recordfile.Stitch(fm, []byte(r.Notes)), nil
}

// toDeal converts a DealRecord to the wire type.
// now is passed so callers control the clock reference (test-friendly).
func toDeal(r DealRecord, now core.Time, includeLog bool) Deal {
	var log []Activity
	if includeLog {
		log = make([]Activity, 0, len(r.Log))
		for _, a := range r.Log {
			log = append(log, Activity{
				TS:  activityTS(a.At, now),
				K:   a.K,
				Who: a.Who,
				T:   a.T,
			})
		}
	}

	docs := make([]DocLink, 0, len(r.Docs))
	for _, d := range r.Docs {
		docs = append(docs, DocLink{T: d.Title, S: d.State})
	}

	prob := core.Sprintf("%d%%", r.ProbabilityPct)

	// Build headline: "£24 K · 12-month hosted" — for v1, headline is the
	// formatted amount. A custom headline suffix is a follow-up.
	headline := formatGBPK(r.AmountPence)

	return Deal{
		ID:           r.ID,
		Customer:     r.Customer,
		Headline:     headline,
		Stage:        stageLabel(r.Stage),
		Probability:  prob,
		CloseTarget:  r.CloseTarget,
		Log:          log,
		Stakeholders: r.Stakeholders,
		Docs:         docs,
	}
}

// loadAll scans ~/Lethean/sales/deals/ and returns all parseable
// DealRecord values without loading the notes body.
//
// Dual-format read (RFC.stage-e-encrypt-at-rest v2 §4.1):
//   - .md files parse as legacy plaintext.
//   - .lthn files are decoded HEADER-ONLY via the substrate's
//     DecodeHeader path (no decrypt, MAC-verified). Customer / amount
//     / notes stay sealed. The returned DealRecord has the header-
//     visible fields populated (ID, Version, Stage) and the body
//     fields empty — frontend renders "encrypted" placeholders.
//
// MAC-failure entries are SKIPPED (RFC §4.1: "do NOT abort whole List
// on one bad file"). When the session is locked the .lthn branch
// still emits a degraded record (ID + Stage from header) because
// header MAC verification only needs the PUBLIC key — no unlock
// required.
func (s *Service) loadAll() ([]DealRecord, error) {
	dirR := dealsDir()
	if !dirR.OK {
		return nil, core.E("deals.loadAll", dirR.Error(), nil)
	}
	dir := dirR.Value.(string)

	entriesR := core.ReadDir(core.DirFS(dir), ".")
	if !entriesR.OK {
		return nil, nil
	}
	entries, _ := entriesR.Value.([]core.FsDirEntry)

	// Two-pass pick: prefer .lthn over .md when both exist for the same
	// id (cutover invariant — once the encrypted record lands the
	// plaintext gets removed, but a crash between AtomicWrite and
	// Remove could leave both on disk). Stable name basis: filename
	// without extension.
	seen := map[string]bool{}
	var records []DealRecord
	// First pass: .lthn (encrypted, header-only).
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !hasSuffix(name, ".lthn") {
			continue
		}
		id := name[:len(name)-len(".lthn")]
		fpath := core.PathJoin(dir, name)
		rec, err := s.loadHeaderOnly(fpath)
		if err != nil {
			// MAC-failure / corrupt envelope / decode error — SKIP per
			// RFC §4.1. Future audit-emission lane (#1487 PR-4) hooks
			// in here.
			continue
		}
		seen[id] = true
		records = append(records, rec)
	}
	// Second pass: .md (legacy plaintext) for ids not already covered.
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !hasSuffix(name, ".md") {
			continue
		}
		id := name[:len(name)-len(".md")]
		if seen[id] {
			continue
		}
		fpath := core.PathJoin(dir, name)
		raw := core.ReadFile(fpath)
		if !raw.OK {
			continue
		}
		rec, err := parseRecord(raw.Value.([]byte))
		if err != nil {
			continue
		}
		rec.Notes = ""
		records = append(records, rec)
	}
	return records, nil
}

// loadHeaderOnly decodes a .lthn file via the substrate's
// DecodeHeader path (no decrypt). Returns a DealRecord populated only
// with header-visible fields (ID, Version, Stage). Body fields stay
// zero-valued — toDeal sees an empty Customer ("(encrypted)" rendering
// is the frontend's call).
//
// PubKey lookup walks the SessionGate's currently-unlocked accounts;
// when none are unlocked the header.account.id field from the raw
// trix is used to query PublicKeyFor directly — PublicKeyFor does NOT
// require unlock (account/unlock.go:903) so List stays open while
// LOCKED per RFC §4.1.
//
// Returns an error when the gate is unwired (cannot resolve any
// public key) OR when DecodeHeader rejects.
func (s *Service) loadHeaderOnly(path string) (DealRecord, error) {
	w, ok := s.atrestWriterFor()
	if !ok {
		return DealRecord{}, core.E("deals.loadHeaderOnly", "session gate not wired", nil)
	}
	raw := core.ReadFile(path)
	if !raw.OK {
		return DealRecord{}, core.E("deals.loadHeaderOnly", raw.Error(), nil)
	}
	rawBytes, _ := raw.Value.([]byte)
	pub, perr := s.headerPubKey(rawBytes)
	if perr != nil {
		return DealRecord{}, perr
	}
	rr := w.DecodeHeader(rawBytes, pub)
	if !rr.OK {
		return DealRecord{}, core.E("deals.loadHeaderOnly", rr.Error(), nil)
	}
	hdr, _ := rr.Value.(recordfile.Header)
	rec := DealRecord{
		ID:      hdr.ID,
		Version: int(hdr.Version),
		Stage:   dealStageEnumIn(stringFromRaw(hdr.Raw, "stage")),
	}
	return rec, nil
}

// headerPubKey resolves the public key bytes for the account.id named
// in a raw at-rest blob's header. Peeks the JSON header directly so
// the lookup is independent of whether any account is unlocked —
// matches RFC §4.1 "List stays open while LOCKED".
//
// The substrate's parseHeader handles the JSON shape; we re-decode
// just enough to extract account.id before asking the gate for the
// public key.
func (s *Service) headerPubKey(raw []byte) ([]byte, error) {
	s.gateMu.RLock()
	gate := s.gate
	s.gateMu.RUnlock()
	if gate == nil {
		return nil, core.E("deals.headerPubKey", "session gate not wired", nil)
	}
	keys, ok := gate.(accountKeyProvider)
	if !ok {
		return nil, core.E("deals.headerPubKey",
			"session gate does not provide account keys", nil)
	}
	accountID, err := peekTrixAccountID(raw)
	if err != nil {
		return nil, err
	}
	pub, ok := keys.PublicKeyFor(accountID)
	if !ok || len(pub) == 0 {
		return nil, core.E("deals.headerPubKey",
			"PublicKeyFor("+accountID+") returned not-ok", nil)
	}
	return pub, nil
}

// peekTrixAccountID extracts the account.id from a raw at-rest blob
// without going through PGP decrypt. The on-disk shape is
// `[Magic(4)][Version(1)][HeaderLen(4)][HeaderJSON][Payload]` per RFC
// §2.2; we read the JSON header bytes and unmarshal just the account
// substructure.
//
// Returns an error when the blob is too short, the magic/length is
// nonsense, or account.id is absent. The substrate's full DecodeHeader
// path runs after this — so structural errors here are not
// authoritative, they're just enough to pick the correct pub key.
func peekTrixAccountID(raw []byte) (string, error) {
	const headerStart = 4 + 1 + 4 // Magic + Version + HeaderLen
	if len(raw) < headerStart {
		return "", core.E("deals.peekTrix", "blob too short", nil)
	}
	hdrLen := int(uint32(raw[5])<<24 | uint32(raw[6])<<16 | uint32(raw[7])<<8 | uint32(raw[8]))
	if hdrLen <= 0 || headerStart+hdrLen > len(raw) {
		return "", core.E("deals.peekTrix", "header length out of bounds", nil)
	}
	hdrJSON := raw[headerStart : headerStart+hdrLen]
	var probe struct {
		Account struct {
			ID string `json:"id"`
		} `json:"account"`
	}
	if r := core.JSONUnmarshalString(string(hdrJSON), &probe); !r.OK {
		return "", core.E("deals.peekTrix", "json unmarshal: "+r.Error(), nil)
	}
	if probe.Account.ID == "" {
		return "", core.E("deals.peekTrix", "account.id missing from header", nil)
	}
	return probe.Account.ID, nil
}

// stringFromRaw extracts a string field from a substrate Header.Raw
// map. Missing or non-string values return "" so toDeal renders the
// fallback stage label without panicking.
func stringFromRaw(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

// dealStageEnumIn maps the canonical RFC §2.4 SalesDealsStages enum
// back to the deals-internal stage vocabulary used by knownStages /
// stageLabel. Symmetric inverse of dealStageEnumOut.
//
//	internalStage := dealStageEnumIn("negotiate") // → "close"
func dealStageEnumIn(headerStage string) string {
	switch headerStage {
	case "qual", "engage", "propose":
		return headerStage
	case "negotiate":
		return "close"
	case "closed_won":
		return "won"
	case "closed_lost":
		return "lost"
	case "on_hold":
		return ""
	}
	return ""
}

// loadOne reads and fully parses the deal file for the given ID.
// Cerberus #1486: id is rejected unless IsValidID — defence in depth
// even though every wails entry point already gates on input.ID.
//
// Dual-format read (RFC.stage-e-encrypt-at-rest v2 §3.1):
//   - .lthn is preferred. Decrypt requires an unlocked session; on a
//     locked-session read this returns the typed deals.session.locked
//     error so callers (Get) surface the lock state instead of leaking
//     "file not found" confusion.
//   - .md is the legacy plaintext fallthrough. Continues to parse via
//     recordfile.Split + YAML unmarshal so pre-cutover records remain
//     readable across the lazy migration.
//
// The s receiver may be nil for tests of the legacy plaintext-only
// path; the at-rest branch is skipped when s is nil so existing
// no-Service helpers (loadAll without retrofit) keep working.
func (s *Service) loadOne(id string) (DealRecord, error) {
	if err := paths.IsValidID(id); err != nil {
		return DealRecord{}, err
	}
	dirR := dealsDir()
	if !dirR.OK {
		return DealRecord{}, core.E("deals.loadOne", dirR.Error(), nil)
	}
	dir := dirR.Value.(string)

	// .lthn first — encrypted records take precedence when present.
	lthnPath, jerr := paths.JoinAndCheck(dir, id+".lthn")
	if jerr != nil {
		return DealRecord{}, jerr
	}
	if exists := core.Stat(lthnPath); exists.OK {
		w, ok := s.atrestWriterFor()
		if !ok {
			return DealRecord{}, core.E("deals.loadOne", "deals.session.locked", nil)
		}
		rr := w.Read(lthnPath)
		if !rr.OK {
			return DealRecord{}, core.E("deals.loadOne", rr.Error(), nil)
		}
		res, _ := rr.Value.(recordfile.ReadResult)
		rec, perr := parseAtRestRecord(res)
		if perr != nil {
			return DealRecord{}, perr
		}
		return rec, nil
	}

	// Legacy plaintext .md fallthrough.
	mdPath, jerr := paths.JoinAndCheck(dir, id+".md")
	if jerr != nil {
		return DealRecord{}, jerr
	}
	raw := core.ReadFile(mdPath)
	if !raw.OK {
		return DealRecord{}, core.E("deals.loadOne", "not found: "+id, nil)
	}
	return parseRecord(raw.Value.([]byte))
}

// parseAtRestRecord turns a substrate ReadResult into a DealRecord by
// running the frontmatter through yaml.Unmarshal (same path
// parseRecord uses for plaintext) and copying the body text into
// Notes.
//
//	rec, err := parseAtRestRecord(rr.Value.(recordfile.ReadResult))
func parseAtRestRecord(r recordfile.ReadResult) (DealRecord, error) {
	var rec DealRecord
	if err := yaml.Unmarshal(r.BodyYAML, &rec); err != nil {
		return DealRecord{}, core.E("deals.parseAtRest", "yaml unmarshal", err)
	}
	rec.Notes = string(r.BodyText)
	return rec, nil
}

// writeRecord serialises and writes a DealRecord to disk.
//
// Stage E.D.B.1 dual-path (RFC.stage-e-encrypt-at-rest v2 §3):
//   - When the SessionGate is wired AND reports exactly one unlocked
//     account, writes the encrypted .lthn envelope via the
//     recordfile.AtRestWriter substrate. After successful encrypted
//     write, removes the legacy <id>.md when present (lazy migration
//     completion per §3.1).
//   - When the gate is unwired (test fixtures pre-#1613 retrofit) the
//     legacy paths.AtomicWriteWithVersion plaintext write executes
//     verbatim. This preserves existing Cascade W1 / W2 behaviour for
//     any caller not yet wired to a session source. Once #1613 ships
//     across all consumers this branch goes away (#1487 PR-5).
//
// ifVersion is the optimistic-lock anchor — pass the Version value the
// caller observed on disk before mutating the record, or 0 for first-
// writes / legacy-file upgrades. writeRecord stamps r.Version=ifVersion+1
// into the marshalled body so subsequent reads see a monotonic version.
//
// Cerberus #1486: the record ID lands directly in the filename so
// IsValidID guards it even though Create generates the ID via Sprintf.
//
// Return shape (Mantis #1544, gating W2): on the stale-write path
// the function returns core.Fail(paths.ConflictEnvelope{...}) so the
// Wails-marshalled Result.Value carries the lowercase-json shape
// (`{code, current_version, current_hash}`) that conflict-dispatch.ts
// extractEnvelope already pattern-matches on. Non-conflict failures
// propagate via core.Fail(core.E(...)) so audit + diagnostic detail
// reach the caller unchanged.
//
// Audit emission is automatic via paths.AuditModeForPath — sales/deals/*
// routes through AuditModeBatch per RFC §6.1.
//
// Usage example:
//
//	if wr := s.writeRecord(rec, dir, prior.Version); !wr.OK {
//	    return wr
//	}
func (s *Service) writeRecord(r DealRecord, dir string, ifVersion int) core.Result {
	if err := paths.IsValidID(r.ID); err != nil {
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
		return s.writeRecordAtRest(w, r, dir)
	}

	// Legacy plaintext fallback (gate not wired — test fixtures only).
	raw, err := marshalRecord(r)
	if err != nil {
		return core.Fail(err)
	}
	target, jerr := paths.JoinAndCheck(dir, r.ID+".md")
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
			"deals.update.conflict", stale))
	}
	return core.Fail(core.E("deals.writeRecord", res.Error(), nil))
}

// writeRecordAtRest encrypts + writes via the substrate. Body is the
// YAML frontmatter (DealRecord without Notes) + the Notes string;
// header carries the bucketed enums per §2.4. After success, removes
// the legacy <id>.md if present (lazy migration per §3.1).
//
// The substrate's IfMatch optimistic-lock uses hex(sha256(prior
// ciphertext)). For first-writes (no prior file) the empty PriorHash
// is the correct IfMatch input. For updates we read the prior
// ciphertext bytes here so the IfMatch hash is well-formed; absence
// means a true first-write and the substrate's first-write path
// applies. AtRestWriter does NOT carry the version-frontmatter
// IfVersion gate (the substrate's optimistic-lock is ciphertext-hash
// based; per-record monotonic Version stays embedded in the body
// frontmatter for forward-compat with future Stage F audit needs).
func (s *Service) writeRecordAtRest(w *recordfile.AtRestWriter[DealRecord], r DealRecord, dir string) core.Result {
	yamlBody, err := yaml.Marshal(r)
	if err != nil {
		return core.Fail(core.E("deals.writeRecord", "yaml marshal", err))
	}
	target, jerr := paths.JoinAndCheck(dir, r.ID+".lthn")
	if jerr != nil {
		return core.Fail(jerr)
	}
	priorHash := ""
	if existing := core.ReadFile(target); existing.OK {
		priorHash = core.SHA256Hex(existing.Value.([]byte))
	}

	wr := w.Write(recordfile.WriteRequest[DealRecord]{
		Record:    r,
		BodyYAML:  yamlBody,
		BodyText:  []byte(r.Notes),
		DestPath:  target,
		PriorHash: priorHash,
		// AccountID: deliberately empty — the substrate uses the
		// SingleUnlockedAccount() result as the canonical id; passing
		// an explicit id would require deals to track the unlocked id
		// in a second place and add a drift surface.
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
	// can be re-attempted by the bulk-migrate CLI (§3.3, separate
	// Hephaestus lane E.D.B.1.5).
	mdPath, jerr := paths.JoinAndCheck(dir, r.ID+".md")
	if jerr == nil {
		if existsMd := core.Stat(mdPath); existsMd.OK {
			if rm := core.Remove(mdPath); !rm.OK {
				core.Warn("deals: failed to remove legacy plaintext after encrypted write",
					"path", mdPath, "err", rm.Error())
			}
		}
	}

	// Synthesise a paths.WriteOutput-shaped success Value so existing
	// callers (writeRecord's success path returned the primitive's
	// Result verbatim) keep their typed expectations. The Version /
	// Mtime fields are surfaced from the substrate receipt where
	// available; Hash is the encrypted-payload sha256 the IfMatch
	// gate will need on the NEXT write.
	receipt, _ := wr.Value.(recordfile.WriteReceipt)
	return core.Ok(paths.WriteOutput{
		Version: int(receipt.Version),
		// Mtime stays zero — caller (Wails wrapper) does not consume it
		// today; Stage F audit will fold in via the substrate's
		// auth.account.atrest.written event hook.
		Hash: "", // future: feed back from substrate when needed
	})
}

// fireEvent publishes a deal event on the Core ACTION bus.
func (s *Service) fireEvent(name string, rec DealRecord) {
	if s == nil || s.core == nil {
		return
	}
	s.core.ACTION(DealEvent{
		EventName: name,
		DealID:    rec.ID,
		Stage:     rec.Stage,
		Customer:  rec.Customer,
		At:        core.Now().UTC(),
	})
}

// SetSessionGate wires the live-read session source. Called by
// cmd/lthn/app.go post-construction (Mantis #1613 B.3) once
// *account.Service exists.
//
// Mirrors the sales/contacts + office/mail.AccountProvider setter
// pattern. Live-read on every gate check — no event-bus reliability
// concerns, no cache coherence concerns (RFC.stage-e-unlockgate v2
// §1.1).
//
// Usage example:
//
//	dealsSvc.SetSessionGate(accountSvc)
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
// hygiene / Cerberus #27 ADD-5). Read-only methods (List, Get)
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
// Fail-safe on nil gate (§2.2 / Cerberus #27 Q2): when SetSessionGate
// has not yet wired the gate (or Stop has nilled it), the gate fails
// LOCKED rather than panicking. The first nil-hit per Service
// instance emits a one-shot core.Warn via CompareAndSwap so
// wire-ordering bugs surface in dev without log spam in production.
//
// Usage example:
//
//	if fail, ok := s.assertUnlocked("deals.Create"); !ok {
//	    return fail
//	}
func (s *Service) assertUnlocked(scope string) (core.Result, bool) {
	s.gateMu.RLock()
	g := s.gate
	s.gateMu.RUnlock()
	if g == nil {
		if s.nilWarned.CompareAndSwap(false, true) {
			core.Warn("deals: session gate not wired; failing locked", "scope", scope)
		}
		return core.Fail(core.E(scope, "deals.session.locked", nil)), false
	}
	if len(g.UnlockedAccountIDs()) == 0 {
		return core.Fail(core.E(scope, "deals.session.locked", nil)), false
	}
	return core.Result{}, true
}

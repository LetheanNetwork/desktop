// SPDX-Licence-Identifier: EUPL-1.2

// Service — Core integration for the incidents surface. Reads and
// writes Trix-style markdown files from ~/Lethean/incidents/{YYYY}/{MM}/.
// No queue, no orm — filesystem is the persistence layer.
//
// Lifecycle:
//   - Register(c)        wires the service into the Core container
//   - ServiceName()      returns "Incidents" for the Wails namespace
//   - SetSessionGate(g)  wired post-construction in cmd/lthn/app.go
//     against *account.Service (live-read pattern — mirrors
//     sales/contacts + sales/deals; no cached bool, no event bus).
//   - Stop(ctx)          nils the SessionGate reference so a draining
//     Service fails-closed on any late-arriving write.
//
// All I/O uses CoreGO wrappers (core.ReadFile / core.WriteFile /
// core.ReadDir / core.DirFS / core.MkdirAll / core.PathJoin).
// Banned stdlib imports: os, path/filepath, strings, encoding/json,
// fmt, log, errors.

package incidents

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
// CONFIRMED by Cerberus #27, B.2 inherits the same shape per Cerberus
// #28 confirm). When the returned slice is empty the session is locked;
// when non-empty at least one Lethean account is unlocked and writes
// may proceed.
//
// Wired in cmd/lthn/app.go (Mantis #1613 B.3):
//
//	incidentsSvc.SetSessionGate(accountSvc)
//
// AX-8 compliance: this interface is defined in the consumer
// (incidents) and satisfied by the producer (*account.Service). No
// pkg/account import lands in pkg/incidents purely for the gate; the
// at-rest path below adds the import as part of the wider
// accountKeyProvider runtime assertion (Stage E.D.B.2 RFC v2 §5.1).
// Each writer pkg defines its OWN gate interface per Pushback 1.
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

// Service owns the incidents surface.
//
// Usage example:
//
//	svc := incidents.NewService(c)
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
	// (§2.2 / Cerberus #27 Q2, B.2 mirror confirmed by Cerberus #28).
	// CompareAndSwap-on-first-hit emits core.Warn exactly once per
	// Service instance. Uses stdlib sync/atomic.Bool to mirror H#147
	// documents pattern (codebase convention — not core.AtomicBool).
	nilWarned atomic.Bool

	// atrestMu serialises lazy construction of atrestWriter so the
	// first concurrent write doesn't double-build the substrate.
	atrestMu sync.Mutex
	// atrestWriter is built on first writer-access against the live
	// SessionGate (RFC.stage-e-encrypt-at-rest v2 §5). nil before the
	// first use; replaced on every SetSessionGate so adapter→gate
	// remains coherent.
	atrestWriter *recordfile.AtRestWriter[IncidentRecord]
}

// NewService constructs the incidents service against a Core container.
// Wired via core.WithName("incidents", incidents.Register) in app.go.
//
// Usage example:
//
//	svc := incidents.NewService(c)
func NewService(c *core.Core) *Service {
	return &Service{core: c}
}

// Register constructs the incidents service for Core registration.
//
// Usage example:
//
//	core.New(core.WithName("incidents", incidents.Register))
func Register(c *core.Core) core.Result {
	return core.Ok(NewService(c))
}

// ServiceName labels the binding namespace exposed to JS.
// Wails binds methods as "Incidents.List()" etc.
func (s *Service) ServiceName() string { return "Incidents" }

// incidentsDir resolves ~/Lethean/incidents/ and creates it if missing.
// Mode 0o700 (Cerberus #1487 PR-1): post-mortem bodies + service names
// + people-attribution are sensitive — owner-only at rest.
func incidentsDir() core.Result {
	root := paths.Root()
	if !root.OK {
		return root
	}
	dir := core.PathJoin(root.Value.(string), "incidents")
	if r := core.MkdirAll(dir, 0o700); !r.OK {
		return r
	}
	return core.Ok(dir)
}

// yearMonthDir resolves ~/Lethean/incidents/{YYYY}/{MM}/ and creates it.
// Mode 0o700 (Cerberus #1487 PR-1): mirrors the parent.
func yearMonthDir(t core.Time) core.Result {
	base := incidentsDir()
	if !base.OK {
		return base
	}
	yr := core.TimeFormat(t, "2006")
	mo := core.TimeFormat(t, "01")
	dir := core.PathJoin(base.Value.(string), yr, mo)
	if r := core.MkdirAll(dir, 0o700); !r.OK {
		return r
	}
	return core.Ok(dir)
}

// --- At-rest substrate plumbing (RFC.stage-e-encrypt-at-rest v2) -----

// incidentStateEnumOut maps the incidents-internal state vocabulary
// (investigating / post-mortem / resolved) onto the canonical RFC §2.4
// IncidentsStatuses enum that the at-rest header MUST emit (open or
// closed). Body retains the original internal state via the YAML
// frontmatter — the header is the LIST/search projection only.
//
//	hdrStatus := incidentStateEnumOut("investigating") // → "open"
//	hdrStatus := incidentStateEnumOut("resolved")      // → "closed"
func incidentStateEnumOut(internal string) string {
	switch internal {
	case "resolved":
		return "closed"
	case "investigating", "post-mortem":
		return "open"
	}
	// Unknown / legacy states project as "open" (still a valid enum
	// member; keeps the encode-side schema validator happy and pushes
	// the curation problem back to the wails-tier UpdateState
	// validStates table rather than into the at-rest substrate).
	return "open"
}

// incidentsHeaderSchema declares the RFC §2.4 per-field header
// whitelist for the incidents surface. Status is the only consumer-
// owned header key; it is bucketed to {open, closed} via the
// IncidentsStatuses enum. All title / post-mortem / svc / who /
// comments / dur / created_at / resolved_at data stays in the
// encrypted body — never in the header.
//
//	w := recordfile.NewAtRestWriter(recordfile.AtRestDeps[IncidentRecord]{
//	    ..., Schema: incidentsHeaderSchema,
//	})
var incidentsHeaderSchema = recordfile.HeaderSchema[IncidentRecord]{
	Surface: recordfile.SurfaceIncidents,
	AllowedKeys: map[string]recordfile.FieldValidator{
		"status": recordfile.ValidateEnum(recordfile.IncidentsStatuses),
	},
	HeaderFor: func(r IncidentRecord) map[string]any {
		return map[string]any{
			"status": incidentStateEnumOut(r.State),
		}
	},
	IDFor:      func(r IncidentRecord) string { return r.ID },
	VersionFor: func(r IncidentRecord) int64 { return int64(r.Version) },
}

// atrestWriterFor returns the lazy-constructed AtRestWriter wired
// against the live SessionGate. Returns (nil, false) when the gate is
// not yet wired OR does not satisfy accountKeyProvider — caller falls
// back to legacy plaintext write.
//
// Construction happens once per Service post-SetSessionGate (atrestMu
// serialises the racy first-call path); SetSessionGate invalidates the
// cache so re-wiring against a fresh accountSvc never reuses stale
// adapter pointers.
//
//	w, ok := s.atrestWriterFor()
//	if !ok { return s.writeRecordLegacy(rec, dir, ifVersion) }
func (s *Service) atrestWriterFor() (*recordfile.AtRestWriter[IncidentRecord], bool) {
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
	w := recordfile.NewAtRestWriter(recordfile.AtRestDeps[IncidentRecord]{
		Surface: recordfile.SurfaceIncidents,
		Keys: account.NewAtRestKeys("incidents", account.AtRestKeysDeps{
			Gate: gate,
			Keys: keys,
		}),
		PGP:    pgp.NewService(),
		Schema: incidentsHeaderSchema,
		Atomic: paths.AtRestAdapter("incidents"),
	})
	s.atrestWriter = w
	return w, true
}

// parseRecord splits a Trix file into frontmatter + body via the
// shared recordfile.Split helper and decodes the YAML header into an
// IncidentRecord. The body is stored in the PostMortem field.
func parseRecord(raw []byte) (IncidentRecord, error) {
	fm, body := recordfile.Split(raw)
	var rec IncidentRecord
	if err := yaml.Unmarshal(fm, &rec); err != nil {
		return IncidentRecord{}, core.E("incidents.parse", "yaml unmarshal", err)
	}
	rec.PostMortem = string(body)
	return rec, nil
}

// parseAtRestRecord turns a substrate ReadResult into an
// IncidentRecord by running the frontmatter through yaml.Unmarshal
// (same path parseRecord uses for plaintext) and copying the body
// text into PostMortem.
//
//	rec, err := parseAtRestRecord(rr.Value.(recordfile.ReadResult))
func parseAtRestRecord(r recordfile.ReadResult) (IncidentRecord, error) {
	var rec IncidentRecord
	if err := yaml.Unmarshal(r.BodyYAML, &rec); err != nil {
		return IncidentRecord{}, core.E("incidents.parseAtRest", "yaml unmarshal", err)
	}
	rec.PostMortem = string(r.BodyText)
	return rec, nil
}

// marshalRecord serialises an IncidentRecord to the Trix file format
// via the shared recordfile.Stitch helper: YAML frontmatter block
// followed by the post-mortem markdown body. Legacy plaintext path —
// at-rest writes stitch via the substrate directly.
func marshalRecord(r IncidentRecord) ([]byte, error) {
	fm, err := yaml.Marshal(r)
	if err != nil {
		return nil, core.E("incidents.marshal", "yaml marshal", err)
	}
	return recordfile.Stitch(fm, []byte(r.PostMortem)), nil
}

// relativeTime returns a human-readable relative duration string
// matching the fixture data shape: "now", "X min ago", "X h ago",
// "X d ago", "X w ago", "X mo ago".
func relativeTime(t core.Time, now core.Time) string {
	if t.IsZero() {
		return ""
	}
	diff := core.Since(t)
	if diff < 0 {
		diff = -diff
	}
	const minute = core.Minute
	const hour = core.Hour
	const day = 24 * core.Hour
	const week = 7 * day
	const month = 30 * day
	// Switch to "X w ago" after 2 weeks (14 days); below that use "X d ago".
	// This matches the fixture data: "11 d ago" (< 2 weeks), "2 w ago" (≥ 2 weeks).
	const weekThreshold = 2 * week

	switch {
	case diff < minute:
		return "now"
	case diff < hour:
		return core.Sprintf("%d min ago", int(diff/minute))
	case diff < day:
		return core.Sprintf("%d h ago", int(diff/hour))
	case diff < weekThreshold:
		return core.Sprintf("%d d ago", int(diff/day))
	case diff < month:
		return core.Sprintf("%d w ago", int(diff/week))
	default:
		return core.Sprintf("%d mo ago", int(diff/month))
	}
}

// durString converts a resolved incident's DurMinutes to a display
// string ("42 min", "1 h 02", "2 h 14").
func durString(minutes int) string {
	if minutes <= 0 {
		return ""
	}
	if minutes < 60 {
		return core.Sprintf("%d min", minutes)
	}
	h := minutes / 60
	m := minutes % 60
	return core.Sprintf("%d h %02d", h, m)
}

// toEntry converts an IncidentRecord to the wire type. now is passed
// in so callers control the clock reference (test-friendly).
func toEntry(r IncidentRecord, now core.Time) IncidentEntry {
	return IncidentEntry{
		ID:       r.ID,
		TS:       relativeTime(r.CreatedAt, now),
		Sev:      r.Sev,
		State:    r.State,
		Title:    r.Title,
		Svc:      r.Svc,
		Who:      r.Who,
		Comments: r.Comments,
		Dur:      durString(r.DurMinutes),
	}
}

// hasSuffix mirrors core.HasSuffix without dragging the import in here
// where the AX-6 wrapper isn't already needed for any other call.
func hasSuffix(s, suffix string) bool {
	return len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix
}

// list90 scans the last two YYYY/MM directories in ~/Lethean/incidents/
// and returns all parseable IncidentRecord values. Malformed files are
// skipped silently. The body (PostMortem) is NOT loaded — use loadOne
// for full records.
//
// Dual-format read (RFC.stage-e-encrypt-at-rest v2 §4.1):
//   - .md files parse as legacy plaintext (full Sev / Title / Svc /
//     Who present from yaml).
//   - .lthn files are decoded HEADER-ONLY via the substrate's
//     DecodeHeader path (no decrypt, MAC-verified). Title / Svc / Who
//     stay sealed. The returned IncidentRecord has the header-visible
//     fields populated (ID, Version, State from status enum) and the
//     body fields empty — frontend renders "encrypted" placeholders.
//
// MAC-failure entries are SKIPPED (RFC §4.1: "do NOT abort whole List
// on one bad file"). When the session is locked the .lthn branch
// still emits a degraded record (ID + State from header) because
// header MAC verification only needs the PUBLIC key — no unlock
// required.
//
// Per-id de-dup: when both <id>.lthn and <id>.md exist in the same
// month (mid-migration crash window), .lthn wins.
func (s *Service) list90() ([]IncidentRecord, error) {
	base := incidentsDir()
	if !base.OK {
		return nil, core.E("incidents.list90", base.Error(), nil)
	}
	baseDir := base.Value.(string)

	now := core.Now()
	yrNow := core.TimeFormat(now, "2006")
	moNow := core.TimeFormat(now, "01")

	// Build the month-directory candidates: current + previous month.
	prev := now.Add(-32 * 24 * core.Hour)
	yrPrev := core.TimeFormat(prev, "2006")
	moPrev := core.TimeFormat(prev, "01")

	candidates := []string{
		core.PathJoin(baseDir, yrNow, moNow),
		core.PathJoin(baseDir, yrPrev, moPrev),
	}

	var records []IncidentRecord
	for _, dir := range candidates {
		entriesR := core.ReadDir(core.DirFS(dir), ".")
		if !entriesR.OK {
			// Directory may not exist yet — normal for a new install.
			continue
		}
		entries, _ := entriesR.Value.([]core.FsDirEntry)

		// Two-pass pick: prefer .lthn over .md when both exist for the
		// same id. Stable name basis: filename without extension.
		seen := map[string]bool{}

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
				// MAC-failure / corrupt envelope / decode error — SKIP
				// per RFC §4.1. Future audit-emission lane (#1487 PR-4)
				// hooks in here.
				continue
			}
			seen[id] = true
			records = append(records, rec)
		}

		// Second pass: .md (legacy plaintext) for ids not already
		// covered by the encrypted entry.
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
			// Strip the body for list performance — Get() loads it.
			rec.PostMortem = ""
			records = append(records, rec)
		}
	}
	return records, nil
}

// loadHeaderOnly decodes a .lthn file via the substrate's
// DecodeHeader path (no decrypt). Returns an IncidentRecord populated
// only with header-visible fields (ID, Version, State from status
// enum). Body fields stay zero-valued — toEntry sees empty Title /
// Svc / Who ("encrypted" rendering is the frontend's call).
//
// PubKey lookup walks the SessionGate's currently-unlocked accounts;
// when none are unlocked the header.account.id field from the raw
// trix is used to query PublicKeyFor directly — PublicKeyFor does NOT
// require unlock (account/unlock.go:903) so List stays open while
// LOCKED per RFC §4.1.
//
// Returns an error when the gate is unwired (cannot resolve any
// public key) OR when DecodeHeader rejects.
func (s *Service) loadHeaderOnly(path string) (IncidentRecord, error) {
	w, ok := s.atrestWriterFor()
	if !ok {
		return IncidentRecord{}, core.E("incidents.loadHeaderOnly", "session gate not wired", nil)
	}
	raw := core.ReadFile(path)
	if !raw.OK {
		return IncidentRecord{}, core.E("incidents.loadHeaderOnly", raw.Error(), nil)
	}
	rawBytes, _ := raw.Value.([]byte)
	pub, perr := s.headerPubKey(rawBytes)
	if perr != nil {
		return IncidentRecord{}, perr
	}
	rr := w.DecodeHeader(rawBytes, pub)
	if !rr.OK {
		return IncidentRecord{}, core.E("incidents.loadHeaderOnly", rr.Error(), nil)
	}
	hdr, _ := rr.Value.(recordfile.Header)
	rec := IncidentRecord{
		ID:      hdr.ID,
		Version: int(hdr.Version),
		State:   incidentStateEnumIn(stringFromRaw(hdr.Raw, "status")),
	}
	return rec, nil
}

// headerPubKey resolves the public key bytes for the account.id
// named in a raw at-rest blob's header. Peeks the JSON header
// directly so the lookup is independent of whether any account is
// unlocked — matches RFC §4.1 "List stays open while LOCKED".
//
// The substrate's parseHeader handles the JSON shape; we re-decode
// just enough to extract account.id before asking the gate for the
// public key.
func (s *Service) headerPubKey(raw []byte) ([]byte, error) {
	s.gateMu.RLock()
	gate := s.gate
	s.gateMu.RUnlock()
	if gate == nil {
		return nil, core.E("incidents.headerPubKey", "session gate not wired", nil)
	}
	keys, ok := gate.(accountKeyProvider)
	if !ok {
		return nil, core.E("incidents.headerPubKey",
			"session gate does not provide account keys", nil)
	}
	accountID, err := recordfile.PeekAccountID(raw)
	if err != nil {
		return nil, err
	}
	pub, ok := keys.PublicKeyFor(accountID)
	if !ok || len(pub) == 0 {
		return nil, core.E("incidents.headerPubKey",
			"PublicKeyFor("+accountID+") returned not-ok", nil)
	}
	return pub, nil
}

// stringFromRaw extracts a string field from a substrate Header.Raw
// map. Missing or non-string values return "" so toEntry renders the
// fallback empty state without panicking.
func stringFromRaw(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

// incidentStateEnumIn maps the canonical RFC §2.4 IncidentsStatuses
// enum back to the incidents-internal state vocabulary. Symmetric
// inverse of incidentStateEnumOut. The "open" header bucket loses
// the investigating-vs-post-mortem distinction; the legacy / body
// path (full record decrypt) restores it. Header-only list entries
// therefore project as "investigating" by default — frontend can
// distinguish via Get() once unlocked.
//
//	internalState := incidentStateEnumIn("open")   // → "investigating"
//	internalState := incidentStateEnumIn("closed") // → "resolved"
func incidentStateEnumIn(headerStatus string) string {
	switch headerStatus {
	case "closed":
		return "resolved"
	case "open":
		return "investigating"
	}
	return ""
}

// loadOne finds and fully parses the incident file for the given ID,
// returning the record with the PostMortem body populated.
//
// Cerberus #1486: id is validated before any directory walk so a
// traversal payload cannot inflate ReadDir attempts across the tree.
//
// Dual-format read (RFC.stage-e-encrypt-at-rest v2 §3.1):
//   - .lthn is preferred. Decrypt requires an unlocked session; on a
//     locked-session read this returns the typed incidents.session.locked
//     error so callers (Get) surface the lock state instead of leaking
//     "incident not found" confusion.
//   - .md is the legacy plaintext fallthrough. Continues to parse via
//     recordfile.Split + YAML unmarshal so pre-cutover records remain
//     readable across the lazy migration.
//
// The s receiver may be nil for callers that only exercise the
// legacy plaintext path (export-test shim before SessionGate exists);
// the at-rest branch is skipped when s is nil and loadOne falls
// through to .md.
func (s *Service) loadOne(id string) (IncidentRecord, string, error) {
	if err := paths.IsValidID(id); err != nil {
		return IncidentRecord{}, "", err
	}
	base := incidentsDir()
	if !base.OK {
		return IncidentRecord{}, "", core.E("incidents.loadOne", base.Error(), nil)
	}
	baseDir := base.Value.(string)

	// Walk all YYYY/MM directories under the base.
	yearsR := core.ReadDir(core.DirFS(baseDir), ".")
	if !yearsR.OK {
		return IncidentRecord{}, "", core.E("incidents.loadOne", yearsR.Error(), nil)
	}
	years, _ := yearsR.Value.([]core.FsDirEntry)
	for _, yEntry := range years {
		if !yEntry.IsDir() {
			continue
		}
		yrPath := core.PathJoin(baseDir, yEntry.Name())
		monthsR := core.ReadDir(core.DirFS(yrPath), ".")
		if !monthsR.OK {
			continue
		}
		months, _ := monthsR.Value.([]core.FsDirEntry)
		for _, mEntry := range months {
			if !mEntry.IsDir() {
				continue
			}
			dirPath := core.PathJoin(yrPath, mEntry.Name())

			// .lthn first — encrypted records take precedence when
			// present. Cerberus #1486 belt-and-braces (Mantis #1607,
			// forward-arc from H#85): WithinDir check after the join.
			lthnPath, jerr := paths.JoinAndCheck(dirPath, id+".lthn")
			if jerr != nil {
				return IncidentRecord{}, "", jerr
			}
			if exists := core.Stat(lthnPath); exists.OK {
				w, ok := s.atrestWriterFor()
				if !ok {
					return IncidentRecord{}, dirPath,
						core.E("incidents.loadOne", "incidents.session.locked", nil)
				}
				rr := w.Read(lthnPath)
				if !rr.OK {
					return IncidentRecord{}, dirPath,
						core.E("incidents.loadOne", rr.Error(), nil)
				}
				res, _ := rr.Value.(recordfile.ReadResult)
				rec, perr := parseAtRestRecord(res)
				if perr != nil {
					return IncidentRecord{}, dirPath, perr
				}
				return rec, dirPath, nil
			}

			// Legacy plaintext .md fallthrough.
			mdPath, jerr := paths.JoinAndCheck(dirPath, id+".md")
			if jerr != nil {
				return IncidentRecord{}, "", jerr
			}
			raw := core.ReadFile(mdPath)
			if !raw.OK {
				continue
			}
			rec, err := parseRecord(raw.Value.([]byte))
			if err != nil {
				return IncidentRecord{}, "", err
			}
			return rec, dirPath, nil
		}
	}
	return IncidentRecord{}, "", core.E("incidents.loadOne", "incident not found: "+id, nil)
}

// writeRecord serialises a record back to disk.
//
// Stage E.D.B.2 dual-path (RFC.stage-e-encrypt-at-rest v2 §3):
//   - When the SessionGate is wired AND reports exactly one unlocked
//     account, writes the encrypted .lthn envelope via the
//     recordfile.AtRestWriter substrate. After successful encrypted
//     write, removes the legacy <id>.md when present (lazy migration
//     completion per §3.1).
//   - When the gate is unwired (test fixtures pre-#1613 retrofit) the
//     legacy paths.AtomicWriteWithVersion plaintext write executes
//     verbatim. This preserves existing Cascade W3 behaviour for any
//     caller not yet wired to a session source. Once #1613 ships
//     across all consumers this branch goes away (#1487 PR-5).
//
// ifVersion is the optimistic-lock anchor — pass the Version the
// caller observed on disk (via parseRecord through loadOne), or 0 for
// first-writes / legacy-file upgrades. writeRecord stamps the next
// monotonic version (ifVersion+1) into the marshalled frontmatter so
// subsequent reads see version=1,2,3... monotonically.
//
// Cerberus #1486: rec.ID lands in the filename — validate before join.
// Cerberus #1487 PR-1: 0o600 — owner-only at rest (applied by the
// primitive's atomic-rename path).
//
// Return shape (Mantis #1544 gating, inherited from W1+W2): on the
// stale-write path the function returns
// core.Fail(paths.ConflictEnvelope{...}) so the Wails-marshalled
// Result.Value carries the lowercase-json shape
// (`{code, current_version, current_hash}`) that
// conflict-dispatch.ts extractEnvelope pattern-matches on. The
// per-service Code is "incidents.update.conflict" — the frontend
// scopes its toast + reload-listener on this exact string.
//
// Audit emission is automatic via paths.AuditModeForPath —
// incidents/* paths route through AuditModeBatch per RFC §6.1.
//
// Usage example:
//
//	if wr := s.writeRecord(rec, dirPath, prior.Version); !wr.OK {
//	    return wr
//	}
func (s *Service) writeRecord(r IncidentRecord, dirPath string, ifVersion int) core.Result {
	if err := paths.IsValidID(r.ID); err != nil {
		return core.Fail(err)
	}
	// Stamp the next monotonic version. ifVersion=0 (Create / legacy
	// upgrade) yields version=1; subsequent writes increment.
	nextVersion := ifVersion + 1
	if nextVersion < 1 {
		nextVersion = 1
	}
	r.Version = nextVersion

	// At-rest path — preferred when the gate is wired.
	if w, ok := s.atrestWriterFor(); ok {
		return s.writeRecordAtRest(w, r, dirPath)
	}

	// Legacy plaintext fallback (gate not wired — test fixtures only).
	raw, err := marshalRecord(r)
	if err != nil {
		return core.Fail(core.E("incidents.writeRecord", "marshal", err))
	}
	// Cerberus #1486 belt-and-braces (Mantis #1607, forward-arc from
	// H#85): WithinDir check after the join. IsValidID above is the
	// cheap shape gate; JoinAndCheck refuses any path that resolves
	// outside dirPath even if cousin-validator drift weakens IsValidID.
	target, jerr := paths.JoinAndCheck(dirPath, r.ID+".md")
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
			"incidents.update.conflict", stale))
	}
	return core.Fail(core.E("incidents.writeRecord",
		"write failed: "+res.Error(), nil))
}

// writeRecordAtRest encrypts + writes via the substrate. Body is the
// YAML frontmatter (IncidentRecord without PostMortem) + the
// PostMortem string; header carries the bucketed status enum per
// §2.4. After success, removes the legacy <id>.md if present (lazy
// migration per §3.1).
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
func (s *Service) writeRecordAtRest(w *recordfile.AtRestWriter[IncidentRecord], r IncidentRecord, dirPath string) core.Result {
	yamlBody, err := yaml.Marshal(r)
	if err != nil {
		return core.Fail(core.E("incidents.writeRecord", "yaml marshal", err))
	}
	target, jerr := paths.JoinAndCheck(dirPath, r.ID+".lthn")
	if jerr != nil {
		return core.Fail(jerr)
	}
	priorHash := ""
	if existing := core.ReadFile(target); existing.OK {
		priorHash = core.SHA256Hex(existing.Value.([]byte))
	}

	wr := w.Write(recordfile.WriteRequest[IncidentRecord]{
		Record:    r,
		BodyYAML:  yamlBody,
		BodyText:  []byte(r.PostMortem),
		DestPath:  target,
		PriorHash: priorHash,
		// AccountID: deliberately empty — the substrate uses the
		// SingleUnlockedAccount() result as the canonical id; passing
		// an explicit id would require incidents to track the unlocked
		// id in a second place and add a drift surface.
	})
	if !wr.OK {
		// Forward the typed substrate error verbatim — its Code names
		// (recordfile.atrest.*) are the contract Stage F audit binds.
		return wr
	}

	// Lazy-migration completion: remove the legacy plaintext file when
	// the encrypted write landed cleanly. Failure is tolerated (RFC
	// §3.1: warn-on-failure, do NOT abort the encrypted write) — the
	// next read will see both files, prefer .lthn, and the cleanup can
	// be re-attempted by the bulk-migrate CLI (§3.3).
	mdPath, jerr := paths.JoinAndCheck(dirPath, r.ID+".md")
	if jerr == nil {
		if existsMd := core.Stat(mdPath); existsMd.OK {
			if rm := core.Remove(mdPath); !rm.OK {
				core.Warn("incidents: failed to remove legacy plaintext after encrypted write",
					"path", mdPath, "err", rm.Error())
			}
		}
	}

	// Synthesise a paths.WriteOutput-shaped success Value so existing
	// callers (writeRecord's success path returned the primitive's
	// Result verbatim) keep their typed expectations.
	receipt, _ := wr.Value.(recordfile.WriteReceipt)
	return core.Ok(paths.WriteOutput{
		Version: int(receipt.Version),
		Hash:    "", // future: feed back from substrate when needed
	})
}

// countMonthFiles returns the number of .md AND .lthn files in a
// month directory. Counts BOTH formats so the {YYYY-MM}-INC-NNN
// suffix counter advances across the cutover.
// Used to derive the NNN suffix for a new incident ID.
func countMonthFiles(dirPath string) int {
	r := core.ReadDir(core.DirFS(dirPath), ".")
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

// IncidentEvent is the Core ACTION bus payload for incident lifecycle
// transitions.
//
// Usage example:
//
//	c.RegisterAction(func(c *core.Core, msg core.Message) core.Result {
//	    if ev, ok := msg.(incidents.IncidentEvent); ok {
//	        _ = ev.Entry.ID
//	    }
//	    return core.Result{OK: true}
//	})
type IncidentEvent struct {
	// EventName is the event constant that triggered this message
	// (EventOpened or EventTransitioned).
	EventName string `json:"event"`

	// Entry is the incident state after the change.
	Entry IncidentEntry `json:"entry"`

	// At is the event timestamp.
	At core.Time `json:"at"`
}

// fireEvent publishes an incident event on the Core ACTION bus.
func (s *Service) fireEvent(name string, entry IncidentEntry) {
	if s == nil || s.core == nil {
		return
	}
	s.core.ACTION(IncidentEvent{
		EventName: name,
		Entry:     entry,
		At:        core.Now().UTC(),
	})
}

// SetSessionGate wires the live-read session source. Called by
// cmd/lthn/app.go post-construction (Mantis #1613 B.3) once
// *account.Service exists.
//
// Mirrors the H#147 documents.SetSessionGate + H#164 deals
// SetSessionGate setter pattern. Live-read on every gate check — no
// event-bus reliability concerns, no cache coherence concerns
// (RFC.stage-e-unlockgate v2 §1.1).
//
// Usage example:
//
//	incidentsSvc.SetSessionGate(accountSvc)
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
// fails-closed on any late-arriving write (Cerberus #28 ADD-5 / H#147
// documents.Stop mirror). Read-only methods (List, Get) continue to
// function — Stop only severs the write gate.
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
//	if fail, ok := s.assertUnlocked("incidents.Create"); !ok {
//	    return fail
//	}
func (s *Service) assertUnlocked(scope string) (core.Result, bool) {
	s.gateMu.RLock()
	g := s.gate
	s.gateMu.RUnlock()
	if g == nil {
		if s.nilWarned.CompareAndSwap(false, true) {
			core.Warn("incidents: session gate not wired; failing locked", "scope", scope)
		}
		return core.Fail(core.E(scope, "incidents.session.locked", nil)), false
	}
	if len(g.UnlockedAccountIDs()) == 0 {
		return core.Fail(core.E(scope, "incidents.session.locked", nil)), false
	}
	return core.Result{}, true
}

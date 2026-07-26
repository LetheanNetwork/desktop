// SPDX-Licence-Identifier: EUPL-1.2

// Service — Core integration for the marketing audience segment surface.
// Manages subscriber segments at ~/Lethean/marketing/audience/{slug}.lthn.
// Each file is the encrypted Trix envelope at-rest (Stage E.D.B.3 /
// Mantis #1487 wave 3) wrapping a YAML frontmatter persistence record.
//
// Lifecycle:
//   - Register(c)        wires the service into the Core container
//   - ServiceName()      returns "Audience" for the Wails namespace
//   - SetSessionGate(g)  wired post-construction in cmd/lthn/app.go
//     against *account.Service (live-read pattern — mirrors
//     incidents/runbooks/sales/deals/marketing-campaigns; no cached
//     bool, no event bus).
//   - Stop(ctx)          nils the SessionGate reference so a draining
//     Service fails-closed on any late-arriving write.
//
// All I/O uses CoreGO wrappers. Banned stdlib imports: os, path/filepath,
// strings, encoding/json, fmt, log, errors.

package audience

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
//	audienceSvc.SetSessionGate(accountSvc)
//
// AX-8 compliance: this interface is defined in the consumer
// (marketing/audience) and satisfied by the producer (*account.Service).
// No pkg/account import lands purely for the gate; the account import
// is consumed by the runtime-asserted accountKeyProvider used by the
// at-rest writer path (mirrors campaigns wave 3 retrofit).
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

// Service owns the marketing audience surface.
//
// Usage example:
//
//	svc := audience.NewService(c)
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
	// SessionGate (RFC.stage-e-encrypt-at-rest v2 §5, Wave 3 — Mantis
	// #1487 E.D.B.3). nil before the first use; replaced on every
	// SetSessionGate so adapter→gate remains coherent.
	atrestWriter *recordfile.AtRestWriter[SegmentRecord]
}

// NewService constructs the audience service against a Core container.
//
// Usage example:
//
//	svc := audience.NewService(c)
func NewService(c *core.Core) *Service {
	return &Service{core: c}
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
//	audienceSvc.SetSessionGate(accountSvc)
//
//wails:ignore
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
// hygiene / Cerberus #28 ADD-5). Read-only methods (List, Get) continue
// to function — Stop only severs the write gate.
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
//	if fail, ok := s.assertUnlocked("audience.Create"); !ok {
//	    return fail
//	}
func (s *Service) assertUnlocked(scope string) (core.Result, bool) {
	s.gateMu.RLock()
	g := s.gate
	s.gateMu.RUnlock()
	if g == nil {
		if s.nilWarned.CompareAndSwap(false, true) {
			core.Warn("audience: session gate not wired; failing locked", "scope", scope)
		}
		return core.Fail(core.E(scope, "audience.session.locked", nil)), false
	}
	if len(g.UnlockedAccountIDs()) == 0 {
		return core.Fail(core.E(scope, "audience.session.locked", nil)), false
	}
	return core.Result{}, true
}

// Register constructs the audience service for Core registration.
//
// Usage example:
//
//	core.New(core.WithName("marketing-audience", audience.Register))
func Register(c *core.Core) core.Result {
	return core.Ok(NewService(c))
}

// ServiceName labels the binding namespace exposed to JS.
// Wails binds methods as "Audience.List()" etc.
func (s *Service) ServiceName() string { return "Audience" }

// segmentFrontmatter is the YAML shape stored in each segment file.
//
// Cascade W2 (RFC §B.3 row 6) — Version carries the monotonic
// optimistic-lock anchor. omitempty so legacy files predating the
// cutover (no version: line) round-trip cleanly as Version=0; the
// first write through writeSegment stamps version=1.
type segmentFrontmatter struct {
	Version int    `yaml:"version,omitempty"`
	ID      string `yaml:"id"`
	Name    string `yaml:"name"`
	N       int    `yaml:"n"`
	Growth  string `yaml:"growth"`
	Src     string `yaml:"src"`
	Spark   string `yaml:"spark"`
}

// audienceDir resolves ~/Lethean/marketing/audience/ and creates it if missing.
// Mode 0o700 (Cerberus #1487 PR-1): segment definitions can carry
// subscriber-list shape data; owner-only at rest.
func audienceDir() core.Result {
	root := paths.Root()
	if !root.OK {
		return root
	}
	dir := core.PathJoin(root.Value.(string), "marketing", "audience")
	if r := core.MkdirAll(dir, 0o700); !r.OK {
		return r
	}
	return core.Ok(dir)
}

// slugifyAudience converts a segment name to a filesystem-safe slug.
func slugifyAudience(name string) string {
	out := make([]byte, 0, len(name))
	for i := 0; i < len(name); i++ {
		b := name[i]
		if b >= 'A' && b <= 'Z' {
			out = append(out, b+32)
		} else if b >= 'a' && b <= 'z' || b >= '0' && b <= '9' || b == '-' {
			out = append(out, b)
		} else if b == ' ' || b == '_' {
			if len(out) > 0 && out[len(out)-1] != '-' {
				out = append(out, '-')
			}
		}
	}
	for len(out) > 0 && out[len(out)-1] == '-' {
		out = out[:len(out)-1]
	}
	return string(out)
}

// --- At-rest substrate plumbing (RFC.stage-e-encrypt-at-rest v2 wave 3)

// audienceHeaderSchema declares the RFC §2.4 per-field header
// whitelist for the marketing/audience surface. Per the per-field MUST
// table (RFC.stage-e-encrypt-at-rest v2 §2.4) + Cerberus C#7 Q2
// ruling:
//
//   - `name` → BODY only (REJECT in header). Segment branding is
//     PII-adjacent — visible-while-locked would expose targeting.
//   - `size` → HEADER as LOG-BUCKET enum (`<1k`|`1-10k`|`10-100k`|
//     `100k+`). Raw integer N MUST NEVER appear in the plaintext
//     header; operators see only the bucketed magnitude while LOCKED.
//
// All other fields (growth / src / spark / criteria / member-list) are
// BODY-only — either reveal-rate-of-change (growth) or carry targeting
// vocabulary (src) or duplicate the BODY-only magnitude (spark, member
// list). The Segment model has no `criteria` or explicit `member list`
// field at present; both stay encrypted in the body if added later.
//
//	w := recordfile.NewAtRestWriter(recordfile.AtRestDeps[SegmentRecord]{
//	    ..., Schema: audienceHeaderSchema,
//	})
var audienceHeaderSchema = recordfile.HeaderSchema[SegmentRecord]{
	Surface: recordfile.SurfaceMarketingAudience,
	AllowedKeys: map[string]recordfile.FieldValidator{
		"size": recordfile.ValidateEnum(recordfile.MarketingAudienceSizeBuckets),
	},
	HeaderFor: func(r SegmentRecord) map[string]any {
		return map[string]any{
			"size": recordfile.LogSizeBucket(int64(r.N)),
		}
	},
	IDFor:      func(r SegmentRecord) string { return r.ID },
	VersionFor: func(r SegmentRecord) int64 { return int64(r.Version) },
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
//	if !ok { return writeSegmentLegacy(dir, seg, ifVersion) }
func (s *Service) atrestWriterFor() (*recordfile.AtRestWriter[SegmentRecord], bool) {
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
	w := recordfile.NewAtRestWriter(recordfile.AtRestDeps[SegmentRecord]{
		Surface: recordfile.SurfaceMarketingAudience,
		Keys: account.NewAtRestKeys("marketing-audience", account.AtRestKeysDeps{
			Gate: gate,
			Keys: keys,
		}),
		PGP:    pgp.NewService(),
		Schema: audienceHeaderSchema,
		Atomic: paths.AtRestAdapter("marketing-audience"),
	})
	s.atrestWriter = w
	return w, true
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
		return nil, core.E("audience.headerPubKey", "session gate not wired", nil)
	}
	keys, ok := gate.(accountKeyProvider)
	if !ok {
		return nil, core.E("audience.headerPubKey",
			"session gate does not provide account keys", nil)
	}
	accountID, err := recordfile.PeekAccountID(raw)
	if err != nil {
		return nil, err
	}
	pub, ok := keys.PublicKeyFor(accountID)
	if !ok || len(pub) == 0 {
		return nil, core.E("audience.headerPubKey",
			"PublicKeyFor("+accountID+") returned not-ok", nil)
	}
	return pub, nil
}

// loadHeaderOnly decodes a .lthn file via the substrate's DecodeHeader
// path (no decrypt). Returns a Segment populated only with header-
// visible fields (ID, Version). Body fields stay zero-valued (Name="",
// N=0, Growth="", Src="", Spark="") — the frontend renders an
// "encrypted" placeholder. N is deliberately 0 (not the bucketed
// value) because the wire type carries raw N for unlocked List rows;
// the header `size` bucket is operator-only context, not subscriber
// magnitude.
//
// PubKey lookup walks the SessionGate via PublicKeyFor which does NOT
// require unlock so List stays open while LOCKED per RFC §4.1.
//
// Returns an error when the gate is unwired (cannot resolve any
// public key) OR when DecodeHeader rejects.
func (s *Service) loadHeaderOnly(path string) (Segment, error) {
	w, ok := s.atrestWriterFor()
	if !ok {
		return Segment{}, core.E("audience.loadHeaderOnly",
			"session gate not wired", nil)
	}
	raw := core.ReadFile(path)
	if !raw.OK {
		return Segment{}, core.E("audience.loadHeaderOnly", raw.Error(), nil)
	}
	rawBytes, _ := raw.Value.([]byte)
	pub, perr := s.headerPubKey(rawBytes)
	if perr != nil {
		return Segment{}, perr
	}
	rr := w.DecodeHeader(rawBytes, pub)
	if !rr.OK {
		return Segment{}, core.E("audience.loadHeaderOnly", rr.Error(), nil)
	}
	hdr, _ := rr.Value.(recordfile.Header)
	return Segment{
		ID:      hdr.ID,
		Version: int(hdr.Version),
	}, nil
}

// parseAtRestRecord turns a substrate ReadResult into a Segment by
// running the frontmatter through yaml.Unmarshal. Mirrors
// campaigns.parseAtRestRecord (audience has no markdown body so
// BodyText is ignored).
//
//	seg, err := parseAtRestRecord(rr.Value.(recordfile.ReadResult))
func parseAtRestRecord(r recordfile.ReadResult) (Segment, error) {
	var rec SegmentRecord
	if err := yaml.Unmarshal(r.BodyYAML, &rec); err != nil {
		return Segment{}, core.E("audience.parseAtRest", "yaml unmarshal", err)
	}
	seg := Segment{
		ID:      rec.ID,
		Name:    rec.Name,
		N:       rec.N,
		Growth:  rec.Growth,
		Src:     rec.Src,
		Spark:   rec.Spark,
		Version: rec.Version,
	}
	// Header version takes precedence on at-rest reads (substrate
	// stamps it via VersionFor); ensure Version reflects the persisted
	// header value rather than any stale frontmatter copy.
	if hdrV := int(r.Header.Version); hdrV > 0 {
		seg.Version = hdrV
	}
	return seg, nil
}

// hasSuffix mirrors core.HasSuffix without dragging the import in here
// where the AX-6 wrapper isn't already needed for any other call.
func hasSuffix(s, suffix string) bool {
	return len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix
}

// parseSegment extracts frontmatter from a Trix-formatted file.
func parseSegment(raw []byte) (Segment, error) {
	content := raw

	open := []byte("---\n")
	if len(content) >= len(open) {
		match := true
		for i, b := range open {
			if content[i] != b {
				match = false
				break
			}
		}
		if match {
			content = content[len(open):]
		}
	}

	closeIdx := -1
	for i := 0; i < len(content)-2; i++ {
		if content[i] == '-' && content[i+1] == '-' && content[i+2] == '-' {
			if i == 0 || content[i-1] == '\n' {
				closeIdx = i
				break
			}
		}
	}

	var fm segmentFrontmatter
	fmBytes := content
	if closeIdx >= 0 {
		fmBytes = content[:closeIdx]
	}
	if err := yaml.Unmarshal(fmBytes, &fm); err != nil {
		return Segment{}, core.E("audience.parseSegment", "yaml unmarshal", err)
	}
	return Segment{
		ID:      fm.ID,
		Name:    fm.Name,
		N:       fm.N,
		Growth:  fm.Growth,
		Src:     fm.Src,
		Spark:   fm.Spark,
		Version: fm.Version,
	}, nil
}

// writeSegment serialises a Segment and persists it via the at-rest
// substrate (Stage E.D.B.3 / Mantis #1487 wave 3) when the SessionGate
// is wired. Falls back to the legacy paths.AtomicWriteWithVersion
// plaintext path when the gate is unwired (test fixtures pre-#1613
// retrofit).
//
// ifVersion is the optimistic-lock anchor — pass the Version the
// caller observed on disk (via parseSegment / parseAtRestRecord), or 0
// for first-writes / legacy-file upgrades. writeSegment stamps the
// next monotonic version (ifVersion+1) into the persisted record so
// subsequent reads see version=1,2,3... monotonically.
//
// Cerberus #1486: seg.ID lands directly in the filename — validate.
// Cerberus #1487 PR-1: 0o600 — owner-only at rest (applied by the
// primitive / substrate's atomic-rename path).
//
// Return shape (Mantis #1544 gating, inherited from W1+W2): on the
// stale-write path the function returns
// core.Fail(paths.ConflictEnvelope{...}) so the Wails-marshalled
// Result.Value carries the lowercase-json shape
// (`{code, current_version, current_hash}`) that
// conflict-dispatch.ts extractEnvelope pattern-matches on. The
// per-service Code is "audience.update.conflict" — the frontend
// scopes its toast + reload-listener on this exact string.
//
// Audit emission is automatic via paths.AuditModeForPath —
// marketing/* paths route through AuditModeBatch per RFC §6.1.
//
// Usage example:
//
//	if wr := s.writeSegment(dir, seg, prior.Version); !wr.OK {
//	    return wr
//	}
func (s *Service) writeSegment(dir string, seg Segment, ifVersion int) core.Result {
	if err := paths.IsValidID(seg.ID); err != nil {
		return core.Fail(err)
	}
	nextVersion := ifVersion + 1
	if nextVersion < 1 {
		nextVersion = 1
	}
	seg.Version = nextVersion

	// At-rest path — preferred when the gate is wired.
	if w, ok := s.atrestWriterFor(); ok {
		return s.writeSegmentAtRest(w, seg, dir)
	}

	// Legacy plaintext fallback (gate unwired — test fixtures only).
	return writeSegmentLegacy(dir, seg, ifVersion)
}

// writeSegmentLegacy is the pre-cutover plaintext write path. Kept
// available as the gate-unwired fallback and exercised directly by
// tests that pin the pre-Mantis-#1487 behaviour. Stamps seg.Version
// into the marshalled frontmatter.
func writeSegmentLegacy(dir string, seg Segment, ifVersion int) core.Result {
	if err := paths.IsValidID(seg.ID); err != nil {
		return core.Fail(err)
	}
	nextVersion := ifVersion + 1
	if nextVersion < 1 {
		nextVersion = 1
	}
	fm := segmentFrontmatter{
		Version: nextVersion,
		ID:      seg.ID,
		Name:    seg.Name,
		N:       seg.N,
		Growth:  seg.Growth,
		Src:     seg.Src,
		Spark:   seg.Spark,
	}
	fmBytes, err := yaml.Marshal(fm)
	if err != nil {
		return core.Fail(core.E("audience.writeSegment", "yaml marshal", err))
	}
	data := append([]byte("---\n"), fmBytes...)
	data = append(data, []byte("---\n")...)
	// Cerberus #1486 belt-and-braces (Mantis #1607, forward-arc from
	// H#85): WithinDir check after the join. IsValidID above is the
	// cheap shape gate; JoinAndCheck refuses any path that resolves
	// outside dir even if cousin-validator drift (slugifyAudience edge
	// cases) weakens IsValidID.
	fpath, jerr := paths.JoinAndCheck(dir, seg.ID+".md")
	if jerr != nil {
		return core.Fail(jerr)
	}
	res := paths.AtomicWriteWithVersion(fpath, paths.WriteInput{
		Body:      data,
		IfVersion: ifVersion,
	})
	if res.OK {
		return res
	}
	if stale, ok := paths.VersionStaleFromError(res.Value); ok {
		return core.Fail(paths.NewConflictEnvelope(
			"audience.update.conflict", stale))
	}
	return core.Fail(core.E("audience.writeSegment",
		"write failed: "+res.Error(), nil))
}

// writeSegmentAtRest encrypts + writes via the substrate. Body is the
// YAML frontmatter (SegmentRecord); the audience surface has no
// markdown body so BodyText stays empty. Header carries the `size`
// LogSizeBucket projection per §2.4 + Cerberus C#7 Q2. After success,
// removes the legacy <id>.md if present (lazy migration per §3.1).
//
// The substrate's IfMatch optimistic-lock uses hex(sha256(prior
// ciphertext)). For first-writes (no prior file) the empty PriorHash
// is the correct IfMatch input. For updates we read the prior
// ciphertext bytes here so the IfMatch hash is well-formed; absence
// means a true first-write and the substrate's first-write path
// applies.
func (s *Service) writeSegmentAtRest(w *recordfile.AtRestWriter[SegmentRecord], seg Segment, dir string) core.Result {
	rec := SegmentRecord{
		ID:      seg.ID,
		Name:    seg.Name,
		N:       seg.N,
		Growth:  seg.Growth,
		Src:     seg.Src,
		Spark:   seg.Spark,
		Version: seg.Version,
	}
	yamlBody, err := yaml.Marshal(rec)
	if err != nil {
		return core.Fail(core.E("audience.writeSegment", "yaml marshal", err))
	}
	target, jerr := paths.JoinAndCheck(dir, seg.ID+".lthn")
	if jerr != nil {
		return core.Fail(jerr)
	}
	priorHash := ""
	if existing := core.ReadFile(target); existing.OK {
		priorHash = core.SHA256Hex(existing.Value.([]byte))
	}

	wr := w.Write(recordfile.WriteRequest[SegmentRecord]{
		Record:    rec,
		BodyYAML:  yamlBody,
		BodyText:  nil,
		DestPath:  target,
		PriorHash: priorHash,
		// AccountID: deliberately empty — the substrate uses
		// SingleUnlockedAccount() as the canonical id, so the consumer
		// doesn't track the unlocked id in a second place.
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
	mdPath, jerr := paths.JoinAndCheck(dir, seg.ID+".md")
	if jerr == nil {
		if existsMd := core.Stat(mdPath); existsMd.OK {
			if rm := core.Remove(mdPath); !rm.OK {
				core.Warn("audience: failed to remove legacy plaintext after encrypted write",
					"path", mdPath, "err", rm.Error())
			}
		}
	}

	// Synthesise a paths.WriteOutput-shaped success Value so existing
	// callers keep their typed expectations. The Version field is
	// surfaced from the substrate receipt.
	receipt, _ := wr.Value.(recordfile.WriteReceipt)
	return core.Ok(paths.WriteOutput{
		Version: int(receipt.Version),
		Hash:    "", // future: feed back from substrate when needed
	})
}

// loadSegments scans ~/Lethean/marketing/audience/ and returns all parseable
// segment records with the "all" segment sorted first.
//
// Dual-format read (RFC.stage-e-encrypt-at-rest v2 §4.1, Wave 3):
//   - .lthn files are decoded HEADER-ONLY via the substrate's
//     DecodeHeader path (no decrypt, MAC-verified). Name / N / Growth /
//     Src / Spark stay sealed. The returned Segment carries header-
//     visible fields only (ID, Version); the operator-visible-while-
//     locked `size` bucket lives in the header but is intentionally
//     NOT projected back into the Segment wire shape (the wire `n` is
//     raw subscriber count for unlocked rows — surfacing the bucket as
//     a stand-in would conflate categories).
//   - .md files parse as legacy plaintext for backward-compat.
//   - Prefer .lthn over .md when both exist for the same id (cutover
//     invariant — once the encrypted record lands the plaintext gets
//     removed, but a crash between AtomicWrite and Remove could leave
//     both on disk).
//
// MAC-failure entries are SKIPPED (RFC §4.1: "do NOT abort whole List
// on one bad file"). When the session is locked the .lthn branch
// still emits a degraded record (ID + Version from header) because
// header MAC verification only needs the PUBLIC key — no unlock
// required.
//
// The s receiver may be nil for tests of the legacy plaintext-only
// path; the at-rest header-only branch is skipped when s is nil so
// existing helpers stay usable.
func (s *Service) loadSegments() ([]Segment, error) {
	dirR := audienceDir()
	if !dirR.OK {
		return nil, core.E("audience.loadSegments", dirR.Error(), nil)
	}
	dir := dirR.Value.(string)

	entriesR := core.ReadDir(core.DirFS(dir), ".")
	if !entriesR.OK {
		return nil, nil
	}
	entries, _ := entriesR.Value.([]core.FsDirEntry)

	// Two-pass pick: prefer .lthn over .md when both exist for the
	// same id. Stable name basis: filename without extension.
	seen := map[string]bool{}
	var allSeg *Segment
	var rest []Segment

	// First pass: .lthn (encrypted, header-only). Only engaged when
	// the at-rest writer can be built — without a wired SessionGate +
	// keys provider, the consumer falls back to legacy .md scan only.
	_, hasAtRest := s.atrestWriterFor()
	if hasAtRest {
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			name := entry.Name()
			if !hasSuffix(name, ".lthn") {
				continue
			}
			id := name[:len(name)-len(".lthn")]
			// Cerberus #1486 belt-and-braces: WithinDir check after
			// the join even for filesystem-trusted leafs.
			fpath, jerr := paths.JoinAndCheck(dir, name)
			if jerr != nil {
				continue
			}
			seg, err := s.loadHeaderOnly(fpath)
			if err != nil {
				// MAC-failure / corrupt envelope / decode error —
				// SKIP per RFC §4.1.
				continue
			}
			seen[id] = true
			// Header-only entries carry no Src; they cannot be the
			// "all" aggregate (the aggregate's Src="all" classification
			// requires body decrypt). Treat them as rest.
			rest = append(rest, seg)
		}
	}

	// Second pass: .md (legacy plaintext) for ids not already covered
	// by the encrypted entry.
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if len(name) < 4 || !hasSuffix(name, ".md") {
			continue
		}
		id := name[:len(name)-len(".md")]
		if seen[id] {
			continue
		}
		// Cerberus #1486 belt-and-braces (Mantis #1607, forward-arc from
		// H#85): WithinDir check after the join. The leaf comes from
		// ReadDir (filesystem-trusted), but JoinAndCheck costs nothing
		// and closes the door on future code paths that might splice
		// user-controlled names into this loop.
		fpath, jerr := paths.JoinAndCheck(dir, name)
		if jerr != nil {
			continue
		}
		raw := core.ReadFile(fpath)
		if !raw.OK {
			continue
		}
		seg, err := parseSegment(raw.Value.([]byte))
		if err != nil {
			continue
		}
		if seg.Src == "all" || seg.ID == "all-subscribers" || seg.Name == "All subscribers" {
			cp := seg
			allSeg = &cp
		} else {
			rest = append(rest, seg)
		}
	}

	var out []Segment
	if allSeg != nil {
		out = append(out, *allSeg)
	}
	out = append(out, rest...)
	return out, nil
}

// loadOne resolves a single segment by ID, returning the full record
// with body fields populated. Dual-format (RFC §3.1):
//   - .lthn is preferred. Decrypt requires an unlocked session; on a
//     locked-session read this returns the typed audience.session.
//     locked error so callers (Get) surface the lock state instead of
//     leaking "not found" confusion.
//   - .md is the legacy plaintext fallthrough.
//
// Returns (Segment, dir, error). The dir return mirrors the
// incidents/runbooks/campaigns loadOne shape so writers can re-use
// the discovered directory without re-resolving audienceDir().
func (s *Service) loadOne(id string) (Segment, string, error) {
	if err := paths.IsValidID(id); err != nil {
		return Segment{}, "", err
	}
	dirR := audienceDir()
	if !dirR.OK {
		return Segment{}, "", core.E("audience.loadOne", dirR.Error(), nil)
	}
	dir := dirR.Value.(string)

	// .lthn first — encrypted records take precedence when present.
	// Cerberus #1486 belt-and-braces: WithinDir check after the join.
	lthnPath, jerr := paths.JoinAndCheck(dir, id+".lthn")
	if jerr != nil {
		return Segment{}, "", jerr
	}
	if exists := core.Stat(lthnPath); exists.OK {
		w, ok := s.atrestWriterFor()
		if !ok {
			return Segment{}, dir,
				core.E("audience.loadOne", "audience.session.locked", nil)
		}
		rr := w.Read(lthnPath)
		if !rr.OK {
			return Segment{}, dir,
				core.E("audience.loadOne", rr.Error(), nil)
		}
		res, _ := rr.Value.(recordfile.ReadResult)
		seg, perr := parseAtRestRecord(res)
		if perr != nil {
			return Segment{}, dir, perr
		}
		return seg, dir, nil
	}

	// Legacy plaintext .md fallthrough.
	mdPath, jerr := paths.JoinAndCheck(dir, id+".md")
	if jerr != nil {
		return Segment{}, "", jerr
	}
	raw := core.ReadFile(mdPath)
	if !raw.OK {
		return Segment{}, "",
			core.E("audience.loadOne", "not found: "+id, nil)
	}
	seg, err := parseSegment(raw.Value.([]byte))
	if err != nil {
		return Segment{}, "", err
	}
	return seg, dir, nil
}

// fireAudienceEvent publishes an audience event on the Core ACTION bus.
func (s *Service) fireAudienceEvent(eventName, segmentID string, n int) {
	if s == nil || s.core == nil {
		return
	}
	s.core.ACTION(AudienceEvent{
		EventName: eventName,
		SegmentID: segmentID,
		N:         n,
		At:        core.Now().UTC(),
	})
}

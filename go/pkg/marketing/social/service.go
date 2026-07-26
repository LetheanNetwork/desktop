// SPDX-Licence-Identifier: EUPL-1.2

// Service — Core integration for the marketing social post queue.
// Manages posts at ~/Lethean/marketing/social/{id}.lthn.
// Each file is the encrypted Trix envelope at-rest (Stage E.D.B.3 /
// Mantis #1487 wave 3) wrapping a YAML frontmatter + markdown body.
//
// Channels are stored as a comma-separated string in frontmatter and split
// back to []string on parse — avoids YAML array quoting complexity while
// keeping the file human-readable.
//
// Lifecycle:
//   - Register(c)        wires the service into the Core container
//   - ServiceName()      returns "Social" for the Wails namespace
//   - SetSessionGate(g)  wired post-construction in cmd/lthn/app.go
//     against *account.Service (live-read pattern — mirrors
//     incidents/runbooks/sales/deals/campaigns; no cached bool, no
//     event bus).
//   - Stop(ctx)          nils the SessionGate reference so a draining
//     Service fails-closed on any late-arriving write.
//
// All I/O uses CoreGO wrappers. Banned stdlib imports: os, path/filepath,
// strings, encoding/json, fmt, log, errors.

package social

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
//	socialSvc.SetSessionGate(accountSvc)
//
// AX-8 compliance: this interface is defined in the consumer
// (marketing/social) and satisfied by the producer (*account.Service).
// No pkg/account import lands purely for the gate; the account import
// below is consumed only by the wider runtime-asserted
// accountKeyProvider used by the at-rest writer path (mirrors
// incidents/runbooks/campaigns wave retrofit).
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

// Service owns the marketing social surface.
//
// Usage example:
//
//	svc := social.NewService(c)
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
	atrestWriter *recordfile.AtRestWriter[SocialPostRecord]
}

// NewService constructs the social service against a Core container.
//
// Usage example:
//
//	svc := social.NewService(c)
func NewService(c *core.Core) *Service {
	return &Service{core: c}
}

// SetSessionGate wires the live-read session source. Called by
// cmd/lthn/app.go post-construction (Mantis #1613 B.3) once
// *account.Service exists.
//
// Mirrors the H#180 campaigns.SetSessionGate + H#147 documents.
// SetSessionGate setter pattern. Live-read on every gate check — no
// event-bus reliability concerns, no cache coherence concerns
// (RFC.stage-e-unlockgate v2 §1.1).
//
// Usage example:
//
//	socialSvc.SetSessionGate(accountSvc)
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
//	if fail, ok := s.assertUnlocked("social.Create"); !ok {
//	    return fail
//	}
func (s *Service) assertUnlocked(scope string) (core.Result, bool) {
	s.gateMu.RLock()
	g := s.gate
	s.gateMu.RUnlock()
	if g == nil {
		if s.nilWarned.CompareAndSwap(false, true) {
			core.Warn("social: session gate not wired; failing locked", "scope", scope)
		}
		return core.Fail(core.E(scope, "social.session.locked", nil)), false
	}
	if len(g.UnlockedAccountIDs()) == 0 {
		return core.Fail(core.E(scope, "social.session.locked", nil)), false
	}
	return core.Result{}, true
}

// Register constructs the social service for Core registration.
//
// Usage example:
//
//	core.New(core.WithName("marketing-social", social.Register))
func Register(c *core.Core) core.Result {
	return core.Ok(NewService(c))
}

// ServiceName labels the binding namespace exposed to JS.
// Wails binds methods as "Social.List()" etc.
func (s *Service) ServiceName() string { return "Social" }

// postFrontmatter is the minimal shape parsed from each legacy
// .md social file (pre-cutover). The at-rest read path uses
// SocialPostRecord directly via yaml.Unmarshal; this struct stays around
// to keep the legacy plaintext parser self-contained.
//
// Cascade W2 (RFC §B.3 row 5) — Version carries the monotonic
// optimistic-lock anchor. omitempty so legacy files predating the
// cutover (no version: line) round-trip cleanly as Version=0; the
// first write stamps version=1.
type postFrontmatter struct {
	Version int    `yaml:"version,omitempty"`
	ID      string `yaml:"id"`
	Ch      string `yaml:"ch"` // comma-separated channels
	When    string `yaml:"when"`
	State   string `yaml:"state"`
	Attach  string `yaml:"attach"`
}

// socialDir resolves ~/Lethean/marketing/social/ and creates it if missing.
// Mode 0o700 (Cerberus #1487 PR-1): unsent post drafts + scheduled
// content shape — owner-only at rest.
func socialDir() core.Result {
	root := paths.Root()
	if !root.OK {
		return root
	}
	dir := core.PathJoin(root.Value.(string), "marketing", "social")
	if r := core.MkdirAll(dir, 0o700); !r.OK {
		return r
	}
	return core.Ok(dir)
}

// splitChannels splits a comma-separated channel string into a slice.
// Returns nil (not empty slice) when the input is empty.
func splitChannels(raw string) []string {
	if raw == "" {
		return nil
	}
	var out []string
	start := 0
	for i := 0; i <= len(raw); i++ {
		if i == len(raw) || raw[i] == ',' {
			part := trimSpace(raw[start:i])
			if part != "" {
				out = append(out, part)
			}
			start = i + 1
		}
	}
	return out
}

// trimSpace trims leading and trailing ASCII spaces from s.
func trimSpace(s string) string {
	start := 0
	for start < len(s) && s[start] == ' ' {
		start++
	}
	end := len(s)
	for end > start && s[end-1] == ' ' {
		end--
	}
	return s[start:end]
}

// joinChannels joins a channel slice into a comma-separated string.
func joinChannels(ch []string) string {
	if len(ch) == 0 {
		return ""
	}
	n := len(ch) - 1
	for _, c := range ch {
		n += len(c)
	}
	b := make([]byte, 0, n)
	for i, c := range ch {
		if i > 0 {
			b = append(b, ',')
		}
		b = append(b, c...)
	}
	return string(b)
}

// hasSuffix mirrors core.HasSuffix without dragging the import in here
// where the AX-6 wrapper isn't already needed for any other call.
func hasSuffix(s, suffix string) bool {
	return len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix
}

// --- At-rest substrate plumbing (RFC.stage-e-encrypt-at-rest v2 wave 3)

// socialHeaderSchema declares the RFC §2.4 per-field header
// whitelist for the marketing/social surface. Per the per-field
// MUST table (RFC.stage-e-encrypt-at-rest v2 §2.4):
//
//   - `text`         → BODY only (REJECT in header). Post content is
//     PII-adjacent + brand-voice sensitive — visible-while-locked
//     would defeat the entire encryption purpose.
//   - `platform`     → HEADER (CONFIRM). The current SocialPost model
//     carries a list of channels (Ch); we project the comma-joined
//     list as `platform` to match the RFC's vocabulary (channel ≅
//     platform for the marketing/social surface).
//   - `scheduled.at` → HEADER (MONTH-only, YYYY-MM). The current
//     SocialPost model carries When as free-form text ("today · 16:00",
//     "yest · 11:14") rather than a Unix timestamp — the header schema
//     deliberately omits this key. SECURITY-NOTE: promoting When to a
//     structured timestamp later requires adding a Unix-seconds field
//     to SocialPostRecord AND wiring a `scheduled.at` HeaderFor entry
//     through MonthBucket. Absence is the safe default (no-leak);
//     presence with the free-form text would either leak the wrong
//     month or break the MonthBucket validator.
//
// All other fields (state / attach) are BODY-only — default-body rule
// where no §2.4 ruling exists.
//
//	w := recordfile.NewAtRestWriter(recordfile.AtRestDeps[SocialPostRecord]{
//	    ..., Schema: socialHeaderSchema,
//	})
var socialHeaderSchema = recordfile.HeaderSchema[SocialPostRecord]{
	Surface: recordfile.SurfaceMarketingSocial,
	AllowedKeys: map[string]recordfile.FieldValidator{
		"platform": recordfile.ValidateString,
	},
	HeaderFor: func(r SocialPostRecord) map[string]any {
		out := map[string]any{}
		if r.Ch != "" {
			out["platform"] = r.Ch
		}
		return out
	},
	IDFor:      func(r SocialPostRecord) string { return r.ID },
	VersionFor: func(r SocialPostRecord) int64 { return int64(r.Version) },
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
//	if !ok { return writePostLegacy(dir, p, ifVersion) }
func (s *Service) atrestWriterFor() (*recordfile.AtRestWriter[SocialPostRecord], bool) {
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
	w := recordfile.NewAtRestWriter(recordfile.AtRestDeps[SocialPostRecord]{
		Surface: recordfile.SurfaceMarketingSocial,
		Keys: account.NewAtRestKeys("marketing-social", account.AtRestKeysDeps{
			Gate: gate,
			Keys: keys,
		}),
		PGP:    pgp.NewService(),
		Schema: socialHeaderSchema,
		Atomic: paths.AtRestAdapter("marketing-social"),
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
		return nil, core.E("social.headerPubKey", "session gate not wired", nil)
	}
	keys, ok := gate.(accountKeyProvider)
	if !ok {
		return nil, core.E("social.headerPubKey",
			"session gate does not provide account keys", nil)
	}
	accountID, err := recordfile.PeekAccountID(raw)
	if err != nil {
		return nil, err
	}
	pub, ok := keys.PublicKeyFor(accountID)
	if !ok || len(pub) == 0 {
		return nil, core.E("social.headerPubKey",
			"PublicKeyFor("+accountID+") returned not-ok", nil)
	}
	return pub, nil
}

// loadHeaderOnly decodes a .lthn file via the substrate's DecodeHeader
// path (no decrypt). Returns a SocialPost populated only with header-
// visible fields (ID, Version, Ch from header `platform`). Body fields
// stay zero-valued (When="", State="", Text="", Attach="") — frontend
// renders an "encrypted" placeholder.
//
// PubKey lookup walks the SessionGate via PublicKeyFor which does NOT
// require unlock so List stays open while LOCKED per RFC §4.1.
//
// Returns an error when the gate is unwired (cannot resolve any
// public key) OR when DecodeHeader rejects.
func (s *Service) loadHeaderOnly(path string) (SocialPost, error) {
	w, ok := s.atrestWriterFor()
	if !ok {
		return SocialPost{}, core.E("social.loadHeaderOnly",
			"session gate not wired", nil)
	}
	raw := core.ReadFile(path)
	if !raw.OK {
		return SocialPost{}, core.E("social.loadHeaderOnly", raw.Error(), nil)
	}
	rawBytes, _ := raw.Value.([]byte)
	pub, perr := s.headerPubKey(rawBytes)
	if perr != nil {
		return SocialPost{}, perr
	}
	rr := w.DecodeHeader(rawBytes, pub)
	if !rr.OK {
		return SocialPost{}, core.E("social.loadHeaderOnly", rr.Error(), nil)
	}
	hdr, _ := rr.Value.(recordfile.Header)
	p := SocialPost{
		ID:      hdr.ID,
		Version: int(hdr.Version),
	}
	if pf, ok := hdr.Raw["platform"].(string); ok {
		p.Ch = splitChannels(pf)
	}
	return p, nil
}

// parseAtRestRecord turns a substrate ReadResult into a SocialPost by
// running the frontmatter through yaml.Unmarshal and copying the body
// text into Text. Mirrors campaigns.parseAtRestRecord.
//
//	p, err := parseAtRestRecord(rr.Value.(recordfile.ReadResult))
func parseAtRestRecord(r recordfile.ReadResult) (SocialPost, error) {
	var rec SocialPostRecord
	if err := yaml.Unmarshal(r.BodyYAML, &rec); err != nil {
		return SocialPost{}, core.E("social.parseAtRest", "yaml unmarshal", err)
	}
	p := SocialPost{
		ID:      rec.ID,
		Ch:      splitChannels(rec.Ch),
		When:    rec.When,
		State:   rec.State,
		Text:    string(r.BodyText),
		Attach:  rec.Attach,
		Version: rec.Version,
	}
	// Header version takes precedence on at-rest reads (substrate
	// stamps it via VersionFor); ensure Version reflects the persisted
	// header value rather than any stale frontmatter copy.
	if hdrV := int(r.Header.Version); hdrV > 0 {
		p.Version = hdrV
	}
	return p, nil
}

// parsePost extracts frontmatter + text body from a legacy plaintext
// Trix-formatted .md file (pre-cutover format).
func parsePost(raw []byte) (SocialPost, error) {
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

	var fm postFrontmatter
	fmBytes := content
	text := ""
	if closeIdx >= 0 {
		fmBytes = content[:closeIdx]
		rest := content[closeIdx+3:]
		// Strip the newline written immediately after "---" by writePost.
		for len(rest) > 0 && rest[0] == '\n' {
			rest = rest[1:]
		}
		// Strip trailing newline.
		for len(rest) > 0 && rest[len(rest)-1] == '\n' {
			rest = rest[:len(rest)-1]
		}
		text = string(rest)
	}
	if err := yaml.Unmarshal(fmBytes, &fm); err != nil {
		return SocialPost{}, core.E("social.parsePost", "yaml unmarshal", err)
	}
	return SocialPost{
		ID:      fm.ID,
		Ch:      splitChannels(fm.Ch),
		When:    fm.When,
		State:   fm.State,
		Text:    text,
		Attach:  fm.Attach,
		Version: fm.Version,
	}, nil
}

// writePost serialises a SocialPost and persists it via the at-rest
// substrate (Stage E.D.B.3 / Mantis #1487 wave 3) when the SessionGate
// is wired. Falls back to the legacy paths.AtomicWriteWithVersion
// plaintext path when the gate is unwired (test fixtures pre-#1613
// retrofit).
//
// ifVersion is the optimistic-lock anchor — pass the Version the
// caller observed on disk (via parsePost / parseAtRestRecord), or 0
// for first-writes / legacy-file upgrades. writePost stamps the next
// monotonic version (ifVersion+1) into the persisted record so
// subsequent reads see version=1,2,3... monotonically.
//
// Cerberus #1486: p.ID lands directly in the filename — validate.
// Cerberus #1487 PR-1: 0o600 — owner-only at rest (applied by the
// primitive / substrate's atomic-rename path).
//
// Return shape (Mantis #1544 gating, inherited from W1+W2): on the
// stale-write path the function returns
// core.Fail(paths.ConflictEnvelope{...}) so the Wails-marshalled
// Result.Value carries the lowercase-json shape
// (`{code, current_version, current_hash}`) that
// conflict-dispatch.ts extractEnvelope pattern-matches on. The
// per-service Code is "social.update.conflict" — the frontend
// scopes its toast + reload-listener on this exact string.
//
// Audit emission is automatic via paths.AuditModeForPath —
// marketing/* paths route through AuditModeBatch per RFC §6.1.
//
// Usage example:
//
//	if wr := s.writePost(dir, p, prior.Version); !wr.OK {
//	    return wr
//	}
func (s *Service) writePost(dir string, p SocialPost, ifVersion int) core.Result {
	if err := paths.IsValidID(p.ID); err != nil {
		return core.Fail(err)
	}
	nextVersion := ifVersion + 1
	if nextVersion < 1 {
		nextVersion = 1
	}
	p.Version = nextVersion

	// At-rest path — preferred when the gate is wired.
	if w, ok := s.atrestWriterFor(); ok {
		return s.writePostAtRest(w, p, dir)
	}

	// Legacy plaintext fallback (gate unwired — test fixtures only).
	return writePostLegacy(dir, p, ifVersion)
}

// writePostLegacy is the pre-cutover plaintext write path. Kept
// available as the gate-unwired fallback and exercised directly by
// tests that pin the pre-Mantis-#1487 behaviour. Stamps p.Version into
// the marshalled frontmatter.
func writePostLegacy(dir string, p SocialPost, ifVersion int) core.Result {
	if err := paths.IsValidID(p.ID); err != nil {
		return core.Fail(err)
	}
	nextVersion := ifVersion + 1
	if nextVersion < 1 {
		nextVersion = 1
	}
	fm := postFrontmatter{
		Version: nextVersion,
		ID:      p.ID,
		Ch:      joinChannels(p.Ch),
		When:    p.When,
		State:   p.State,
		Attach:  p.Attach,
	}
	fmBytes, err := yaml.Marshal(fm)
	if err != nil {
		return core.Fail(core.E("social.writePost", "yaml marshal", err))
	}
	data := append([]byte("---\n"), fmBytes...)
	data = append(data, []byte("---\n")...)
	if p.Text != "" {
		data = append(data, '\n')
		data = append(data, []byte(p.Text)...)
	}
	fpath, jerr := paths.JoinAndCheck(dir, p.ID+".md")
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
			"social.update.conflict", stale))
	}
	return core.Fail(core.E("social.writePost",
		"write failed: "+res.Error(), nil))
}

// writePostAtRest encrypts + writes via the substrate. Body is the
// YAML frontmatter (SocialPostRecord without Text) + the Text string;
// header carries the `platform` enum-shaped string per §2.4. After
// success, removes the legacy <id>.md if present (lazy migration per
// §3.1).
//
// The substrate's IfMatch optimistic-lock uses hex(sha256(prior
// ciphertext)). For first-writes (no prior file) the empty PriorHash
// is the correct IfMatch input. For updates we read the prior
// ciphertext bytes here so the IfMatch hash is well-formed; absence
// means a true first-write and the substrate's first-write path
// applies.
func (s *Service) writePostAtRest(w *recordfile.AtRestWriter[SocialPostRecord], p SocialPost, dir string) core.Result {
	rec := SocialPostRecord{
		ID:      p.ID,
		Ch:      joinChannels(p.Ch),
		When:    p.When,
		State:   p.State,
		Attach:  p.Attach,
		Version: p.Version,
	}
	yamlBody, err := yaml.Marshal(rec)
	if err != nil {
		return core.Fail(core.E("social.writePost", "yaml marshal", err))
	}
	target, jerr := paths.JoinAndCheck(dir, p.ID+".lthn")
	if jerr != nil {
		return core.Fail(jerr)
	}
	priorHash := ""
	if existing := core.ReadFile(target); existing.OK {
		priorHash = core.SHA256Hex(existing.Value.([]byte))
	}

	wr := w.Write(recordfile.WriteRequest[SocialPostRecord]{
		Record:    rec,
		BodyYAML:  yamlBody,
		BodyText:  []byte(p.Text),
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
	mdPath, jerr := paths.JoinAndCheck(dir, p.ID+".md")
	if jerr == nil {
		if existsMd := core.Stat(mdPath); existsMd.OK {
			if rm := core.Remove(mdPath); !rm.OK {
				core.Warn("social: failed to remove legacy plaintext after encrypted write",
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

// loadPosts scans ~/Lethean/marketing/social/ and returns all
// parseable SocialPost records.
//
// Dual-format read (RFC.stage-e-encrypt-at-rest v2 §4.1, Wave 3):
//   - .lthn files are decoded HEADER-ONLY via the substrate's
//     DecodeHeader path (no decrypt, MAC-verified). When / State /
//     Text / Attach stay sealed. The returned SocialPost has header-
//     visible fields (ID, Version, Ch from header `platform`)
//     populated.
//   - .md files parse as legacy plaintext for backward-compat.
//   - Prefer .lthn over .md when both exist for the same id (cutover
//     invariant — once the encrypted record lands the plaintext gets
//     removed, but a crash between AtomicWrite and Remove could leave
//     both on disk).
//
// MAC-failure entries are SKIPPED (RFC §4.1: "do NOT abort whole List
// on one bad file"). When the session is locked the .lthn branch
// still emits a degraded record (ID + Version + Ch from header)
// because header MAC verification only needs the PUBLIC key — no
// unlock required.
//
// The s receiver may be nil for tests of the legacy plaintext-only
// path; the at-rest header-only branch is skipped when s is nil so
// existing helpers stay usable.
func (s *Service) loadPosts() ([]SocialPost, error) {
	dirR := socialDir()
	if !dirR.OK {
		return nil, core.E("social.loadPosts", dirR.Error(), nil)
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
	var posts []SocialPost

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
			fpath := core.PathJoin(dir, name)
			p, err := s.loadHeaderOnly(fpath)
			if err != nil {
				// MAC-failure / corrupt envelope / decode error —
				// SKIP per RFC §4.1.
				continue
			}
			seen[id] = true
			posts = append(posts, p)
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
		fpath := core.PathJoin(dir, name)
		raw := core.ReadFile(fpath)
		if !raw.OK {
			continue
		}
		p, err := parsePost(raw.Value.([]byte))
		if err != nil {
			continue
		}
		posts = append(posts, p)
	}
	return posts, nil
}

// loadOne resolves a single post by ID, returning the full record
// with Text populated. Dual-format (RFC §3.1):
//   - .lthn is preferred. Decrypt requires an unlocked session; on a
//     locked-session read this returns the typed social.session.
//     locked error so callers (Get / MarkSent) surface the lock state
//     instead of leaking "not found" confusion.
//   - .md is the legacy plaintext fallthrough.
//
// Returns (SocialPost, dir, error). The dir return mirrors the
// campaigns loadOne shape so writers can re-use the discovered
// directory without re-resolving socialDir().
func (s *Service) loadOne(id string) (SocialPost, string, error) {
	if err := paths.IsValidID(id); err != nil {
		return SocialPost{}, "", err
	}
	dirR := socialDir()
	if !dirR.OK {
		return SocialPost{}, "", core.E("social.loadOne", dirR.Error(), nil)
	}
	dir := dirR.Value.(string)

	// .lthn first — encrypted records take precedence when present.
	// Cerberus #1486 belt-and-braces: WithinDir check after the join.
	lthnPath, jerr := paths.JoinAndCheck(dir, id+".lthn")
	if jerr != nil {
		return SocialPost{}, "", jerr
	}
	if exists := core.Stat(lthnPath); exists.OK {
		w, ok := s.atrestWriterFor()
		if !ok {
			return SocialPost{}, dir,
				core.E("social.loadOne", "social.session.locked", nil)
		}
		rr := w.Read(lthnPath)
		if !rr.OK {
			return SocialPost{}, dir,
				core.E("social.loadOne", rr.Error(), nil)
		}
		res, _ := rr.Value.(recordfile.ReadResult)
		p, perr := parseAtRestRecord(res)
		if perr != nil {
			return SocialPost{}, dir, perr
		}
		return p, dir, nil
	}

	// Legacy plaintext .md fallthrough.
	mdPath, jerr := paths.JoinAndCheck(dir, id+".md")
	if jerr != nil {
		return SocialPost{}, "", jerr
	}
	raw := core.ReadFile(mdPath)
	if !raw.OK {
		return SocialPost{}, "",
			core.E("social.loadOne", "not found: "+id, nil)
	}
	p, err := parsePost(raw.Value.([]byte))
	if err != nil {
		return SocialPost{}, "", err
	}
	return p, dir, nil
}

// containsChannel returns true if ch slice contains the given channel.
func containsChannel(ch []string, target string) bool {
	for _, c := range ch {
		if c == target {
			return true
		}
	}
	return false
}

// fireSocialEvent publishes a social event on the Core ACTION bus.
func (s *Service) fireSocialEvent(eventName, postID, state string) {
	if s == nil || s.core == nil {
		return
	}
	s.core.ACTION(SocialEvent{
		EventName: eventName,
		PostID:    postID,
		State:     state,
		At:        core.Now().UTC(),
	})
}

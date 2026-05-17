// SPDX-Licence-Identifier: EUPL-1.2

// Service — Core integration for the marketing content calendar surface.
// Manages editorial items at ~/Lethean/marketing/content/{id}.lthn.
// Each file is the encrypted Trix envelope at-rest (Stage E.D.B.3 /
// Mantis #1487 wave 3 LAST consumer) wrapping a YAML frontmatter +
// markdown body. Legacy .md plaintext files remain readable via the
// lazy-migration fallthrough until promoted to .lthn per RFC §3.1.
//
// Lifecycle:
//   - Register(c)        wires the service into the Core container
//   - ServiceName()      returns "Content" for the Wails namespace
//   - SetSessionGate(g)  wired post-construction in cmd/lthn/app.go
//     against *account.Service (live-read pattern — mirrors
//     marketing/campaigns + incidents/runbooks/sales/deals; no cached
//     bool, no event bus).
//   - Stop(ctx)          nils the SessionGate reference so a draining
//     Service fails-closed on any late-arriving write.
//
// All I/O uses CoreGO wrappers. Banned stdlib imports: os, path/filepath,
// strings, encoding/json, fmt, log, errors.

package content

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
// Wired in cmd/lthn/app.go (Mantis #1613 B.3, deferred to that lane):
//
//	contentSvc.SetSessionGate(accountSvc)
//
// AX-8 compliance: this interface is defined in the consumer (content)
// and satisfied by the producer (*account.Service). The duplication
// across sales/contacts, sales/deals, sales/pipeline, incidents,
// runbooks, marketing/* etc. IS the AX-8 boundary — each consumer owns
// its own contract, no shared types package importing producer.
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

// Service owns the marketing content surface.
//
// Usage example:
//
//	svc := content.NewService(c)
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
	// SessionGate (RFC.stage-e-encrypt-at-rest v2 §5, Wave 3 LAST —
	// Mantis #1487 E.D.B.3). nil before the first use; replaced on
	// every SetSessionGate so adapter→gate remains coherent.
	atrestWriter *recordfile.AtRestWriter[ContentRecord]
}

// NewService constructs the content service against a Core container.
//
// Usage example:
//
//	svc := content.NewService(c)
func NewService(c *core.Core) *Service {
	return &Service{core: c}
}

// Register constructs the content service for Core registration.
//
// Usage example:
//
//	core.New(core.WithName("marketing-content", content.Register))
func Register(c *core.Core) core.Result {
	return core.Ok(NewService(c))
}

// ServiceName labels the binding namespace exposed to JS.
// Wails binds methods as "Content.List()" etc.
func (s *Service) ServiceName() string { return "Content" }

// colSpec is ordered column metadata.
type colSpec struct {
	ID    string
	Label string
}

// columnOrder returns the canonical ordered column slice.
// The order matches the fixture column order in content.ts.
func columnOrder() []colSpec {
	return []colSpec{
		{ID: "idea", Label: "Ideas"},
		{ID: "draft", Label: "Drafting"},
		{ID: "review", Label: "Review"},
		{ID: "ready", Label: "Ready"},
		{ID: "live", Label: "Live"},
	}
}

// itemFrontmatter is the minimal shape parsed from each legacy .md
// content file (pre-cutover). The at-rest read path uses ContentRecord
// directly via yaml.Unmarshal; this struct stays around to keep the
// legacy plaintext parser self-contained.
//
// Cascade W2 (RFC §B.3 row 4) — Version carries the monotonic
// optimistic-lock anchor. omitempty so legacy files predating the
// cutover (no version: line) round-trip cleanly as Version=0; the
// first write through writeItem stamps version=1.
type itemFrontmatter struct {
	Version int    `yaml:"version,omitempty"`
	ID      string `yaml:"id"`
	T       string `yaml:"t"`
	Who     string `yaml:"who"`
	When    string `yaml:"when"`
	Due     string `yaml:"due"`
	Col     string `yaml:"col"`
}

// contentDir resolves ~/Lethean/marketing/content/ and creates it if missing.
// Mode 0o700 (Cerberus #1487 PR-1): content drafts + Due dates carry
// pre-publication strategy — owner-only at rest.
func contentDir() core.Result {
	root := paths.Root()
	if !root.OK {
		return root
	}
	dir := core.PathJoin(root.Value.(string), "marketing", "content")
	if r := core.MkdirAll(dir, 0o700); !r.OK {
		return r
	}
	return core.Ok(dir)
}

// slugifyContent converts a title to a filesystem-safe slug.
func slugifyContent(title string) string {
	out := make([]byte, 0, len(title))
	for i := 0; i < len(title); i++ {
		b := title[i]
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

// SetSessionGate wires the live-read session source. Called by
// cmd/lthn/app.go post-construction (Mantis #1613 B.3) once
// *account.Service exists.
//
// Mirrors the sales/contacts + sales/deals + sales/pipeline +
// marketing/campaigns + office/mail.AccountProvider setter pattern.
// Live-read on every gate check — no event-bus reliability concerns,
// no cache coherence concerns (RFC.stage-e-unlockgate v2 §1.1).
//
// Usage example:
//
//	contentSvc.SetSessionGate(accountSvc)
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
//	if fail, ok := s.assertUnlocked("content.Create"); !ok {
//	    return fail
//	}
func (s *Service) assertUnlocked(scope string) (core.Result, bool) {
	s.gateMu.RLock()
	g := s.gate
	s.gateMu.RUnlock()
	if g == nil {
		if s.nilWarned.CompareAndSwap(false, true) {
			core.Warn("content: session gate not wired; failing locked", "scope", scope)
		}
		return core.Fail(core.E(scope, "content.session.locked", nil)), false
	}
	if len(g.UnlockedAccountIDs()) == 0 {
		return core.Fail(core.E(scope, "content.session.locked", nil)), false
	}
	return core.Result{}, true
}

// --- At-rest substrate plumbing (RFC.stage-e-encrypt-at-rest v2 wave 3 LAST)

// contentHeaderSchema declares the RFC §2.4 per-field header whitelist
// for the marketing/content surface. Per the per-field MUST table
// (RFC.stage-e-encrypt-at-rest v2 §2.4):
//
//   - `title`  → BODY only (REJECT in header). Editorial intent is
//     PII-adjacent — visible-while-locked would expose go-to-market
//     content roadmap.
//   - `due.at` → HEADER (MONTH-only, YYYY-MM). The wire/persisted Due
//     field is human-shaped today ("today", "next Friday"); HeaderFor
//     emits `due.at` only when Due parses as strict YYYY-MM
//     (Q5-friendly per sales/deals close.target pattern). Legacy
//     free-form values stay BODY-only by being absent from the header.
//     ValidateMonthBucket would reject any free-form string at encode
//     anyway.
//
// All other fields (who / when / col / body) are BODY-only — either
// PII-adjacent (who) or have no RFC ruling (col / when) and the
// substrate's default-body discipline is the safe choice. SECURITY-
// NOTE: brief offered `status (enum if any)` escape valve; RFC §2.4
// does NOT name a status header key for marketing/content, so the
// schema omits this header key deliberately (per [[feedback_brief_vs_
// rfc_deferral_check]] + [[feedback_beta_ticket_dont_gate]]).
//
//	w := recordfile.NewAtRestWriter(recordfile.AtRestDeps[ContentRecord]{
//	    ..., Schema: contentHeaderSchema,
//	})
var contentHeaderSchema = recordfile.HeaderSchema[ContentRecord]{
	Surface: recordfile.SurfaceMarketingContent,
	AllowedKeys: map[string]recordfile.FieldValidator{
		"due.at": recordfile.ValidateMonthBucket,
	},
	HeaderFor: func(r ContentRecord) map[string]any {
		out := map[string]any{}
		// due.at is human-shaped ("today") today; only emit the header
		// field when the consumer-supplied string parses as a strict
		// YYYY-MM (Q5-friendly — incidental clean-data paths pass
		// through; legacy free-form values stay BODY-only by being
		// absent from the header). The validator would reject any
		// free-form string at encode anyway.
		if isStrictYYYYMM(r.Due) {
			out["due.at"] = r.Due
		}
		return out
	},
	IDFor:      func(r ContentRecord) string { return r.ID },
	VersionFor: func(r ContentRecord) int64 { return int64(r.Version) },
}

// isStrictYYYYMM is the internal predicate that mirrors the substrate's
// ValidateMonthBucket parser. Keeps content self-contained without
// re-exporting the substrate's private predicate (matches sales/deals
// service.go pattern).
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
//	if !ok { return writeItemLegacy(dir, item, ifVersion) }
func (s *Service) atrestWriterFor() (*recordfile.AtRestWriter[ContentRecord], bool) {
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
	w := recordfile.NewAtRestWriter(recordfile.AtRestDeps[ContentRecord]{
		Surface: recordfile.SurfaceMarketingContent,
		Keys: account.NewAtRestKeys("marketing-content", account.AtRestKeysDeps{
			Gate: gate,
			Keys: keys,
		}),
		PGP:    pgp.NewService(),
		Schema: contentHeaderSchema,
		Atomic: paths.AtRestAdapter("marketing-content"),
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
		return nil, core.E("content.headerPubKey", "session gate not wired", nil)
	}
	keys, ok := gate.(accountKeyProvider)
	if !ok {
		return nil, core.E("content.headerPubKey",
			"session gate does not provide account keys", nil)
	}
	accountID, err := recordfile.PeekAccountID(raw)
	if err != nil {
		return nil, err
	}
	pub, ok := keys.PublicKeyFor(accountID)
	if !ok || len(pub) == 0 {
		return nil, core.E("content.headerPubKey",
			"PublicKeyFor("+accountID+") returned not-ok", nil)
	}
	return pub, nil
}

// loadHeaderOnly decodes a .lthn file via the substrate's DecodeHeader
// path (no decrypt). Returns a ContentItem populated only with header-
// visible fields (ID, Version, Due iff present as `due.at` MONTH-
// bucket). Body fields stay zero-valued (T="", Who="", When="", Col="",
// Body="") — frontend renders an "encrypted" placeholder.
//
// PubKey lookup walks the SessionGate via PublicKeyFor which does NOT
// require unlock so List stays open while LOCKED per RFC §4.1.
//
// Returns an error when the gate is unwired (cannot resolve any
// public key) OR when DecodeHeader rejects.
func (s *Service) loadHeaderOnly(path string) (ContentItem, error) {
	w, ok := s.atrestWriterFor()
	if !ok {
		return ContentItem{}, core.E("content.loadHeaderOnly",
			"session gate not wired", nil)
	}
	raw := core.ReadFile(path)
	if !raw.OK {
		return ContentItem{}, core.E("content.loadHeaderOnly", raw.Error(), nil)
	}
	rawBytes, _ := raw.Value.([]byte)
	pub, perr := s.headerPubKey(rawBytes)
	if perr != nil {
		return ContentItem{}, perr
	}
	rr := w.DecodeHeader(rawBytes, pub)
	if !rr.OK {
		return ContentItem{}, core.E("content.loadHeaderOnly", rr.Error(), nil)
	}
	hdr, _ := rr.Value.(recordfile.Header)
	item := ContentItem{
		ID:      hdr.ID,
		Version: int(hdr.Version),
		// Col is BODY-only per RFC §2.4 default-rule; header-only
		// reads (reads-while-locked, §4.1) project the encrypted
		// entry into the first pipeline column ("idea") so the
		// operator can SEE it exists. The frontend renders an
		// "encrypted" placeholder on the entry. Without this fallback
		// the entry would be filtered out by columnOrder() in List —
		// invisible-while-locked is worse than empty-while-locked.
		Col: columnOrder()[0].ID,
	}
	if due, ok := hdr.Raw["due.at"].(string); ok {
		item.Due = due
	}
	return item, nil
}

// parseAtRestRecord turns a substrate ReadResult into a ContentItem by
// running the frontmatter through yaml.Unmarshal and copying the body
// text into Body. Mirrors campaigns.parseAtRestRecord.
//
//	item, err := parseAtRestRecord(rr.Value.(recordfile.ReadResult))
func parseAtRestRecord(r recordfile.ReadResult) (ContentItem, error) {
	var rec ContentRecord
	if err := yaml.Unmarshal(r.BodyYAML, &rec); err != nil {
		return ContentItem{}, core.E("content.parseAtRest", "yaml unmarshal", err)
	}
	item := ContentItem{
		ID:      rec.ID,
		T:       rec.Title,
		Who:     rec.Who,
		When:    rec.When,
		Due:     rec.Due,
		Col:     rec.Col,
		Body:    string(r.BodyText),
		Version: rec.Version,
	}
	// Header version takes precedence on at-rest reads (substrate
	// stamps it via VersionFor); ensure Version reflects the persisted
	// header value rather than any stale frontmatter copy.
	if hdrV := int(r.Header.Version); hdrV > 0 {
		item.Version = hdrV
	}
	return item, nil
}

// parseItem extracts frontmatter + body from a legacy plaintext Trix-
// formatted .md file (pre-cutover format).
func parseItem(raw []byte) (ContentItem, error) {
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

	var fm itemFrontmatter
	fmBytes := content
	body := ""
	if closeIdx >= 0 {
		fmBytes = content[:closeIdx]
		rest := content[closeIdx+3:]
		if len(rest) > 0 && rest[0] == '\n' {
			rest = rest[1:]
		}
		body = string(rest)
	}
	if err := yaml.Unmarshal(fmBytes, &fm); err != nil {
		return ContentItem{}, core.E("content.parseItem", "yaml unmarshal", err)
	}
	return ContentItem{
		ID:      fm.ID,
		T:       fm.T,
		Who:     fm.Who,
		When:    fm.When,
		Due:     fm.Due,
		Col:     fm.Col,
		Body:    body,
		Version: fm.Version,
	}, nil
}

// hasSuffix mirrors core.HasSuffix without dragging the import in here
// where the AX-6 wrapper isn't already needed for any other call.
func hasSuffix(s, suffix string) bool {
	return len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix
}

// writeItem serialises a ContentItem and persists it via the at-rest
// substrate (Stage E.D.B.3 / Mantis #1487 wave 3 LAST consumer) when
// the SessionGate is wired. Falls back to the legacy
// paths.AtomicWriteWithVersion plaintext path when the gate is unwired
// (test fixtures pre-#1613 retrofit).
//
// ifVersion is the optimistic-lock anchor — pass the Version the
// caller observed on disk (via parseItem / parseAtRestRecord), or 0
// for first-writes / legacy-file upgrades. writeItem stamps the next
// monotonic version (ifVersion+1) into the persisted record so
// subsequent reads see version=1,2,3... monotonically.
//
// Cerberus #1486: item.ID lands directly in the filename — validate.
// Cerberus #1487 PR-1: 0o600 — owner-only at rest (applied by the
// primitive / substrate's atomic-rename path).
//
// Return shape (Mantis #1544 gating, inherited from W1+W2): on the
// stale-write path the function returns
// core.Fail(paths.ConflictEnvelope{...}) so the Wails-marshalled
// Result.Value carries the lowercase-json shape
// (`{code, current_version, current_hash}`) that
// conflict-dispatch.ts extractEnvelope pattern-matches on. The
// per-service Code is "content.update.conflict" — the frontend
// scopes its toast + reload-listener on this exact string.
//
// Audit emission is automatic via paths.AuditModeForPath —
// marketing/* paths route through AuditModeBatch per RFC §6.1.
//
// Usage example:
//
//	if wr := s.writeItem(dir, item, prior.Version); !wr.OK {
//	    return wr
//	}
func (s *Service) writeItem(dir string, item ContentItem, ifVersion int) core.Result {
	if err := paths.IsValidID(item.ID); err != nil {
		return core.Fail(err)
	}
	nextVersion := ifVersion + 1
	if nextVersion < 1 {
		nextVersion = 1
	}
	item.Version = nextVersion

	// At-rest path — preferred when the gate is wired.
	if w, ok := s.atrestWriterFor(); ok {
		return s.writeItemAtRest(w, item, dir)
	}

	// Legacy plaintext fallback (gate unwired — test fixtures only).
	return writeItemLegacy(dir, item, ifVersion)
}

// writeItemLegacy is the pre-cutover plaintext write path. Kept
// available as the gate-unwired fallback and exercised directly by
// tests that pin the pre-Mantis-#1487 behaviour. Stamps item.Version
// into the marshalled frontmatter.
func writeItemLegacy(dir string, item ContentItem, ifVersion int) core.Result {
	if err := paths.IsValidID(item.ID); err != nil {
		return core.Fail(err)
	}
	nextVersion := ifVersion + 1
	if nextVersion < 1 {
		nextVersion = 1
	}
	fm := itemFrontmatter{
		Version: nextVersion,
		ID:      item.ID,
		T:       item.T,
		Who:     item.Who,
		When:    item.When,
		Due:     item.Due,
		Col:     item.Col,
	}
	fmBytes, err := yaml.Marshal(fm)
	if err != nil {
		return core.Fail(core.E("content.writeItem", "yaml marshal", err))
	}
	data := append([]byte("---\n"), fmBytes...)
	data = append(data, []byte("---\n")...)
	if item.Body != "" {
		data = append(data, '\n')
		data = append(data, []byte(item.Body)...)
	}
	fpath, jerr := paths.JoinAndCheck(dir, item.ID+".md")
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
			"content.update.conflict", stale))
	}
	return core.Fail(core.E("content.writeItem",
		"write failed: "+res.Error(), nil))
}

// writeItemAtRest encrypts + writes via the substrate. Body is the
// YAML frontmatter (ContentRecord without Body) + the Body string;
// header carries the `due.at` MONTH-only string per §2.4 (only when
// Due parses strict YYYY-MM). After success, removes the legacy
// <id>.md if present (lazy migration per §3.1).
//
// The substrate's IfMatch optimistic-lock uses hex(sha256(prior
// ciphertext)). For first-writes (no prior file) the empty PriorHash
// is the correct IfMatch input. For updates we read the prior
// ciphertext bytes here so the IfMatch hash is well-formed; absence
// means a true first-write and the substrate's first-write path
// applies.
func (s *Service) writeItemAtRest(w *recordfile.AtRestWriter[ContentRecord], item ContentItem, dir string) core.Result {
	rec := ContentRecord{
		ID:      item.ID,
		Title:   item.T,
		Who:     item.Who,
		When:    item.When,
		Due:     item.Due,
		Col:     item.Col,
		Version: item.Version,
	}
	yamlBody, err := yaml.Marshal(rec)
	if err != nil {
		return core.Fail(core.E("content.writeItem", "yaml marshal", err))
	}
	target, jerr := paths.JoinAndCheck(dir, item.ID+".lthn")
	if jerr != nil {
		return core.Fail(jerr)
	}
	priorHash := ""
	if existing := core.ReadFile(target); existing.OK {
		priorHash = core.SHA256Hex(existing.Value.([]byte))
	}

	wr := w.Write(recordfile.WriteRequest[ContentRecord]{
		Record:    rec,
		BodyYAML:  yamlBody,
		BodyText:  []byte(item.Body),
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
	mdPath, jerr := paths.JoinAndCheck(dir, item.ID+".md")
	if jerr == nil {
		if existsMd := core.Stat(mdPath); existsMd.OK {
			if rm := core.Remove(mdPath); !rm.OK {
				core.Warn("content: failed to remove legacy plaintext after encrypted write",
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

// loadItems scans ~/Lethean/marketing/content/ and returns all
// parseable item records.
//
// Dual-format read (RFC.stage-e-encrypt-at-rest v2 §4.1, Wave 3 LAST):
//   - .lthn files are decoded HEADER-ONLY via the substrate's
//     DecodeHeader path (no decrypt, MAC-verified). T / Who / When /
//     Col / Body stay sealed. The returned ContentItem has header-
//     visible fields (ID, Version, Due iff MONTH-bucket) populated.
//   - .md files parse as legacy plaintext for backward-compat.
//   - Prefer .lthn over .md when both exist for the same id (cutover
//     invariant — once the encrypted record lands the plaintext gets
//     removed, but a crash between AtomicWrite and Remove could leave
//     both on disk).
//
// MAC-failure entries are SKIPPED (RFC §4.1: "do NOT abort whole List
// on one bad file"). When the session is locked the .lthn branch
// still emits a degraded record (ID + Version + Due) because header
// MAC verification only needs the PUBLIC key — no unlock required.
//
// The s receiver may be nil for tests of the legacy plaintext-only
// path; the at-rest header-only branch is skipped when s is nil so
// existing helpers stay usable.
func (s *Service) loadItems() ([]ContentItem, error) {
	dirR := contentDir()
	if !dirR.OK {
		return nil, core.E("content.loadItems", dirR.Error(), nil)
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
	var items []ContentItem

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
			item, err := s.loadHeaderOnly(fpath)
			if err != nil {
				// MAC-failure / corrupt envelope / decode error —
				// SKIP per RFC §4.1.
				continue
			}
			seen[id] = true
			items = append(items, item)
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
		item, err := parseItem(raw.Value.([]byte))
		if err != nil {
			continue
		}
		items = append(items, item)
	}
	return items, nil
}

// loadOne resolves a single item by ID, returning the full record
// with Body populated. Dual-format (RFC §3.1):
//   - .lthn is preferred. Decrypt requires an unlocked session; on a
//     locked-session read this returns the typed content.session.
//     locked error so callers (Get) surface the lock state instead of
//     leaking "not found" confusion.
//   - .md is the legacy plaintext fallthrough.
//
// Returns (ContentItem, dir, error). The dir return mirrors the
// campaigns/incidents/runbooks loadOne shape so writers can re-use the
// discovered directory without re-resolving contentDir().
func (s *Service) loadOne(id string) (ContentItem, string, error) {
	if err := paths.IsValidID(id); err != nil {
		return ContentItem{}, "", err
	}
	dirR := contentDir()
	if !dirR.OK {
		return ContentItem{}, "", core.E("content.loadOne", dirR.Error(), nil)
	}
	dir := dirR.Value.(string)

	// .lthn first — encrypted records take precedence when present.
	// Cerberus #1486 belt-and-braces: WithinDir check after the join.
	lthnPath, jerr := paths.JoinAndCheck(dir, id+".lthn")
	if jerr != nil {
		return ContentItem{}, "", jerr
	}
	if exists := core.Stat(lthnPath); exists.OK {
		w, ok := s.atrestWriterFor()
		if !ok {
			return ContentItem{}, dir,
				core.E("content.loadOne", "content.session.locked", nil)
		}
		rr := w.Read(lthnPath)
		if !rr.OK {
			return ContentItem{}, dir,
				core.E("content.loadOne", rr.Error(), nil)
		}
		res, _ := rr.Value.(recordfile.ReadResult)
		item, perr := parseAtRestRecord(res)
		if perr != nil {
			return ContentItem{}, dir, perr
		}
		return item, dir, nil
	}

	// Legacy plaintext .md fallthrough.
	mdPath, jerr := paths.JoinAndCheck(dir, id+".md")
	if jerr != nil {
		return ContentItem{}, "", jerr
	}
	raw := core.ReadFile(mdPath)
	if !raw.OK {
		return ContentItem{}, "",
			core.E("content.loadOne", "not found: "+id, nil)
	}
	item, err := parseItem(raw.Value.([]byte))
	if err != nil {
		return ContentItem{}, "", err
	}
	return item, dir, nil
}

// nextCol returns the column ID following the given one in the canonical order.
// Returns empty string if already at "live".
func nextCol(current string) string {
	order := columnOrder()
	for i, spec := range order {
		if spec.ID == current && i+1 < len(order) {
			return order[i+1].ID
		}
	}
	return ""
}

// fireContentEvent publishes a content event on the Core ACTION bus.
func (s *Service) fireContentEvent(eventName, itemID, col string) {
	if s == nil || s.core == nil {
		return
	}
	s.core.ACTION(ContentEvent{
		EventName: eventName,
		ItemID:    itemID,
		Col:       col,
		At:        core.Now().UTC(),
	})
}

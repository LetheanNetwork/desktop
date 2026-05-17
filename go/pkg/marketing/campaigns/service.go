// SPDX-Licence-Identifier: EUPL-1.2

// Service — Core integration for the marketing campaigns surface.
// Manages campaign threads at ~/Lethean/marketing/campaigns/{slug}.lthn.
// Each file is the encrypted Trix envelope at-rest (Stage E.D.B.3 /
// Mantis #1487 wave 3) wrapping a YAML frontmatter + markdown body.
//
// Lifecycle:
//   - Register(c)        wires the service into the Core container
//   - ServiceName()      returns "Campaigns" for the Wails namespace
//   - SetSessionGate(g)  wired post-construction in cmd/lthn/app.go
//     against *account.Service (live-read pattern — mirrors
//     incidents/runbooks/sales/deals; no cached bool, no event bus).
//   - Stop(ctx)          nils the SessionGate reference so a draining
//     Service fails-closed on any late-arriving write.
//
// All I/O uses CoreGO wrappers. Banned stdlib imports: os, path/filepath,
// strings, encoding/json, fmt, log, errors.

package campaigns

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
// CONFIRMED by Cerberus #27/#28). When the returned slice is empty the
// session is locked; when non-empty at least one Lethean account is
// unlocked and writes may proceed.
//
// Wired in cmd/lthn/app.go (Mantis #1613 B.3, deferred to that lane):
//
//	campaignsSvc.SetSessionGate(accountSvc)
//
// AX-8 compliance: this interface is defined in the consumer
// (marketing/campaigns) and satisfied by the producer (*account.Service).
// No pkg/account import lands purely for the gate; the account import
// below is consumed only by the wider runtime-asserted
// accountKeyProvider used by the at-rest writer path (mirrors
// incidents/runbooks wave 2 retrofit).
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

// Service owns the marketing campaigns surface.
//
// Usage example:
//
//	svc := campaigns.NewService(c)
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
	atrestWriter *recordfile.AtRestWriter[CampaignRecord]
}

// NewService constructs the campaigns service against a Core container.
//
// Usage example:
//
//	svc := campaigns.NewService(c)
func NewService(c *core.Core) *Service {
	return &Service{core: c}
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
//	campaignsSvc.SetSessionGate(accountSvc)
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
//	if fail, ok := s.assertUnlocked("campaigns.Create"); !ok {
//	    return fail
//	}
func (s *Service) assertUnlocked(scope string) (core.Result, bool) {
	s.gateMu.RLock()
	g := s.gate
	s.gateMu.RUnlock()
	if g == nil {
		if s.nilWarned.CompareAndSwap(false, true) {
			core.Warn("campaigns: session gate not wired; failing locked", "scope", scope)
		}
		return core.Fail(core.E(scope, "campaigns.session.locked", nil)), false
	}
	if len(g.UnlockedAccountIDs()) == 0 {
		return core.Fail(core.E(scope, "campaigns.session.locked", nil)), false
	}
	return core.Result{}, true
}

// Register constructs the campaigns service for Core registration.
//
// Usage example:
//
//	core.New(core.WithName("marketing-campaigns", campaigns.Register))
func Register(c *core.Core) core.Result {
	return core.Ok(NewService(c))
}

// ServiceName labels the binding namespace exposed to JS.
// Wails binds methods as "Campaigns.List()" etc.
func (s *Service) ServiceName() string { return "Campaigns" }

// campaignFrontmatter is the minimal shape parsed from each legacy
// .md campaign file (pre-cutover). The at-rest read path uses
// CampaignRecord directly via yaml.Unmarshal; this struct stays around
// to keep the legacy plaintext parser self-contained.
//
// Cascade W2 (RFC §B.3 row 7) — Version carries the monotonic
// optimistic-lock anchor. omitempty so legacy files predating the
// cutover (no version: line) round-trip cleanly as Version=0; the
// first write stamps version=1.
type campaignFrontmatter struct {
	Version int    `yaml:"version,omitempty"`
	ID      string `yaml:"id"`
	Name    string `yaml:"name"`
	State   string `yaml:"state"`
	Reach   string `yaml:"reach"`
	Convert string `yaml:"convert"`
	Spend   string `yaml:"spend"`
	Channel string `yaml:"channel"`
}

// campaignsDir resolves ~/Lethean/marketing/campaigns/ and creates it if missing.
// Mode 0o700 (Cerberus #1487 PR-1): campaign bodies + spend + reach
// metrics — owner-only at rest.
func campaignsDir() core.Result {
	root := paths.Root()
	if !root.OK {
		return root
	}
	dir := core.PathJoin(root.Value.(string), "marketing", "campaigns")
	if r := core.MkdirAll(dir, 0o700); !r.OK {
		return r
	}
	return core.Ok(dir)
}

// slugify converts a name to a filesystem-safe slug: lowercase, spaces to
// hyphens, strips non-alphanumeric except hyphens.
func slugify(name string) string {
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
		// all other chars dropped
	}
	// trim trailing hyphen
	for len(out) > 0 && out[len(out)-1] == '-' {
		out = out[:len(out)-1]
	}
	return string(out)
}

// --- At-rest substrate plumbing (RFC.stage-e-encrypt-at-rest v2 wave 3)

// campaignsHeaderSchema declares the RFC §2.4 per-field header
// whitelist for the marketing/campaigns surface. Per the per-field
// MUST table (RFC.stage-e-encrypt-at-rest v2 §2.4):
//
//   - `name`         → BODY only (REJECT in header). Campaign branding
//     is PII-adjacent — visible-while-locked would expose go-to-market
//     intent.
//   - `platform`     → HEADER (CONFIRM). The current Campaign model
//     names the platform field "Channel" (earned|direct|email|paid);
//     we project it as `platform` to match the RFC's vocabulary.
//   - `scheduled.at` → HEADER (MONTH-only, YYYY-MM). The current
//     Campaign model has NO scheduled-time field — the header schema
//     deliberately omits this key. SECURITY-NOTE: adding scheduled-time
//     to the Campaign model later requires extending CampaignRecord
//     AND adding a `scheduled.at` HeaderFor entry. Absence is the
//     safe default (no-leak); presence with a fake value would either
//     leak the wrong month or break the MonthBucket validator.
//
// All other fields (state / reach / convert / spend / body) are BODY-
// only — either sensitive metrics or no-RFC-ruling fields where the
// default-body discipline applies.
//
//	w := recordfile.NewAtRestWriter(recordfile.AtRestDeps[CampaignRecord]{
//	    ..., Schema: campaignsHeaderSchema,
//	})
var campaignsHeaderSchema = recordfile.HeaderSchema[CampaignRecord]{
	Surface: recordfile.SurfaceMarketingCampaigns,
	AllowedKeys: map[string]recordfile.FieldValidator{
		"platform": recordfile.ValidateString,
	},
	HeaderFor: func(r CampaignRecord) map[string]any {
		out := map[string]any{}
		if r.Channel != "" {
			out["platform"] = r.Channel
		}
		return out
	},
	IDFor:      func(r CampaignRecord) string { return r.ID },
	VersionFor: func(r CampaignRecord) int64 { return int64(r.Version) },
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
func (s *Service) atrestWriterFor() (*recordfile.AtRestWriter[CampaignRecord], bool) {
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
	w := recordfile.NewAtRestWriter(recordfile.AtRestDeps[CampaignRecord]{
		Surface: recordfile.SurfaceMarketingCampaigns,
		Keys: account.NewAtRestKeys("marketing-campaigns", account.AtRestKeysDeps{
			Gate: gate,
			Keys: keys,
		}),
		PGP:    pgp.NewService(),
		Schema: campaignsHeaderSchema,
		Atomic: paths.AtRestAdapter("marketing-campaigns"),
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
		return nil, core.E("campaigns.headerPubKey", "session gate not wired", nil)
	}
	keys, ok := gate.(accountKeyProvider)
	if !ok {
		return nil, core.E("campaigns.headerPubKey",
			"session gate does not provide account keys", nil)
	}
	accountID, err := recordfile.PeekAccountID(raw)
	if err != nil {
		return nil, err
	}
	pub, ok := keys.PublicKeyFor(accountID)
	if !ok || len(pub) == 0 {
		return nil, core.E("campaigns.headerPubKey",
			"PublicKeyFor("+accountID+") returned not-ok", nil)
	}
	return pub, nil
}

// loadHeaderOnly decodes a .lthn file via the substrate's DecodeHeader
// path (no decrypt). Returns a Campaign populated only with header-
// visible fields (ID, Version, Channel from header `platform`). Body
// fields stay zero-valued (Name="", State="", Reach="", Convert="",
// Spend="", Body="") — frontend renders an "encrypted" placeholder.
//
// PubKey lookup walks the SessionGate via PublicKeyFor which does NOT
// require unlock so List stays open while LOCKED per RFC §4.1.
//
// Returns an error when the gate is unwired (cannot resolve any
// public key) OR when DecodeHeader rejects.
func (s *Service) loadHeaderOnly(path string) (Campaign, error) {
	w, ok := s.atrestWriterFor()
	if !ok {
		return Campaign{}, core.E("campaigns.loadHeaderOnly",
			"session gate not wired", nil)
	}
	raw := core.ReadFile(path)
	if !raw.OK {
		return Campaign{}, core.E("campaigns.loadHeaderOnly", raw.Error(), nil)
	}
	rawBytes, _ := raw.Value.([]byte)
	pub, perr := s.headerPubKey(rawBytes)
	if perr != nil {
		return Campaign{}, perr
	}
	rr := w.DecodeHeader(rawBytes, pub)
	if !rr.OK {
		return Campaign{}, core.E("campaigns.loadHeaderOnly", rr.Error(), nil)
	}
	hdr, _ := rr.Value.(recordfile.Header)
	c := Campaign{
		ID:      hdr.ID,
		Version: int(hdr.Version),
	}
	if pf, ok := hdr.Raw["platform"].(string); ok {
		c.Channel = pf
	}
	return c, nil
}

// parseAtRestRecord turns a substrate ReadResult into a Campaign by
// running the frontmatter through yaml.Unmarshal and copying the body
// text into Body. Mirrors runbooks.parseAtRestRecord.
//
//	c, err := parseAtRestRecord(rr.Value.(recordfile.ReadResult))
func parseAtRestRecord(r recordfile.ReadResult) (Campaign, error) {
	var rec CampaignRecord
	if err := yaml.Unmarshal(r.BodyYAML, &rec); err != nil {
		return Campaign{}, core.E("campaigns.parseAtRest", "yaml unmarshal", err)
	}
	c := Campaign{
		ID:      rec.ID,
		Name:    rec.Name,
		State:   rec.State,
		Reach:   rec.Reach,
		Convert: rec.Convert,
		Spend:   rec.Spend,
		Channel: rec.Channel,
		Body:    string(r.BodyText),
		Version: rec.Version,
	}
	// Header version takes precedence on at-rest reads (substrate
	// stamps it via VersionFor); ensure Version reflects the persisted
	// header value rather than any stale frontmatter copy.
	if hdrV := int(r.Header.Version); hdrV > 0 {
		c.Version = hdrV
	}
	return c, nil
}

// parseCampaign extracts frontmatter + body from a legacy plaintext
// Trix-formatted .md file (pre-cutover format).
func parseCampaign(raw []byte) (Campaign, error) {
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

	var fm campaignFrontmatter
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
		return Campaign{}, core.E("campaigns.parseCampaign", "yaml unmarshal", err)
	}
	return Campaign{
		ID:      fm.ID,
		Name:    fm.Name,
		State:   fm.State,
		Reach:   fm.Reach,
		Convert: fm.Convert,
		Spend:   fm.Spend,
		Channel: fm.Channel,
		Body:    body,
		Version: fm.Version,
	}, nil
}

// hasSuffix mirrors core.HasSuffix without dragging the import in here
// where the AX-6 wrapper isn't already needed for any other call.
func hasSuffix(s, suffix string) bool {
	return len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix
}

// writeCampaign serialises a Campaign and persists it via the at-rest
// substrate (Stage E.D.B.3 / Mantis #1487 wave 3) when the SessionGate
// is wired. Falls back to the legacy paths.AtomicWriteWithVersion
// plaintext path when the gate is unwired (test fixtures pre-#1613
// retrofit).
//
// ifVersion is the optimistic-lock anchor — pass the Version the
// caller observed on disk (via parseCampaign / parseAtRestRecord), or
// 0 for first-writes / legacy-file upgrades. writeCampaign stamps the
// next monotonic version (ifVersion+1) into the persisted record so
// subsequent reads see version=1,2,3... monotonically.
//
// Cerberus #1486: c.ID lands directly in the filename — validate.
// Cerberus #1487 PR-1: 0o600 — owner-only at rest (applied by the
// primitive / substrate's atomic-rename path).
//
// Return shape (Mantis #1544 gating, inherited from W1+W2): on the
// stale-write path the function returns
// core.Fail(paths.ConflictEnvelope{...}) so the Wails-marshalled
// Result.Value carries the lowercase-json shape
// (`{code, current_version, current_hash}`) that
// conflict-dispatch.ts extractEnvelope pattern-matches on. The
// per-service Code is "campaigns.update.conflict" — the frontend
// scopes its toast + reload-listener on this exact string.
//
// Audit emission is automatic via paths.AuditModeForPath —
// marketing/* paths route through AuditModeBatch per RFC §6.1.
//
// Usage example:
//
//	if wr := s.writeCampaign(dir, c, prior.Version); !wr.OK {
//	    return wr
//	}
func (s *Service) writeCampaign(dir string, c Campaign, ifVersion int) core.Result {
	if err := paths.IsValidID(c.ID); err != nil {
		return core.Fail(err)
	}
	nextVersion := ifVersion + 1
	if nextVersion < 1 {
		nextVersion = 1
	}
	c.Version = nextVersion

	// At-rest path — preferred when the gate is wired.
	if w, ok := s.atrestWriterFor(); ok {
		return s.writeCampaignAtRest(w, c, dir)
	}

	// Legacy plaintext fallback (gate unwired — test fixtures only).
	return writeCampaignLegacy(dir, c, ifVersion)
}

// writeCampaignLegacy is the pre-cutover plaintext write path. Kept
// available as the gate-unwired fallback and exercised directly by
// tests that pin the pre-Mantis-#1487 behaviour. Stamps c.Version into
// the marshalled frontmatter.
func writeCampaignLegacy(dir string, c Campaign, ifVersion int) core.Result {
	if err := paths.IsValidID(c.ID); err != nil {
		return core.Fail(err)
	}
	nextVersion := ifVersion + 1
	if nextVersion < 1 {
		nextVersion = 1
	}
	fm := campaignFrontmatter{
		Version: nextVersion,
		ID:      c.ID,
		Name:    c.Name,
		State:   c.State,
		Reach:   c.Reach,
		Convert: c.Convert,
		Spend:   c.Spend,
		Channel: c.Channel,
	}
	fmBytes, err := yaml.Marshal(fm)
	if err != nil {
		return core.Fail(core.E("campaigns.writeCampaign", "yaml marshal", err))
	}
	content := append([]byte("---\n"), fmBytes...)
	content = append(content, []byte("---\n")...)
	if c.Body != "" {
		content = append(content, '\n')
		content = append(content, []byte(c.Body)...)
	}
	fpath, jerr := paths.JoinAndCheck(dir, c.ID+".md")
	if jerr != nil {
		return core.Fail(jerr)
	}
	res := paths.AtomicWriteWithVersion(fpath, paths.WriteInput{
		Body:      content,
		IfVersion: ifVersion,
	})
	if res.OK {
		return res
	}
	if stale, ok := paths.VersionStaleFromError(res.Value); ok {
		return core.Fail(paths.NewConflictEnvelope(
			"campaigns.update.conflict", stale))
	}
	return core.Fail(core.E("campaigns.writeCampaign",
		"write failed: "+res.Error(), nil))
}

// writeCampaignAtRest encrypts + writes via the substrate. Body is the
// YAML frontmatter (CampaignRecord without Body) + the Body string;
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
func (s *Service) writeCampaignAtRest(w *recordfile.AtRestWriter[CampaignRecord], c Campaign, dir string) core.Result {
	rec := CampaignRecord{
		ID:      c.ID,
		Name:    c.Name,
		State:   c.State,
		Reach:   c.Reach,
		Convert: c.Convert,
		Spend:   c.Spend,
		Channel: c.Channel,
		Version: c.Version,
	}
	yamlBody, err := yaml.Marshal(rec)
	if err != nil {
		return core.Fail(core.E("campaigns.writeCampaign", "yaml marshal", err))
	}
	target, jerr := paths.JoinAndCheck(dir, c.ID+".lthn")
	if jerr != nil {
		return core.Fail(jerr)
	}
	priorHash := ""
	if existing := core.ReadFile(target); existing.OK {
		priorHash = core.SHA256Hex(existing.Value.([]byte))
	}

	wr := w.Write(recordfile.WriteRequest[CampaignRecord]{
		Record:    rec,
		BodyYAML:  yamlBody,
		BodyText:  []byte(c.Body),
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
	mdPath, jerr := paths.JoinAndCheck(dir, c.ID+".md")
	if jerr == nil {
		if existsMd := core.Stat(mdPath); existsMd.OK {
			if rm := core.Remove(mdPath); !rm.OK {
				core.Warn("campaigns: failed to remove legacy plaintext after encrypted write",
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

// loadCampaigns scans ~/Lethean/marketing/campaigns/ and returns all
// parseable Campaign records.
//
// Dual-format read (RFC.stage-e-encrypt-at-rest v2 §4.1, Wave 3):
//   - .lthn files are decoded HEADER-ONLY via the substrate's
//     DecodeHeader path (no decrypt, MAC-verified). Name / State /
//     Reach / Convert / Spend / Body stay sealed. The returned
//     Campaign has header-visible fields (ID, Version, Channel from
//     header `platform`) populated.
//   - .md files parse as legacy plaintext for backward-compat.
//   - Prefer .lthn over .md when both exist for the same id (cutover
//     invariant — once the encrypted record lands the plaintext gets
//     removed, but a crash between AtomicWrite and Remove could leave
//     both on disk).
//
// MAC-failure entries are SKIPPED (RFC §4.1: "do NOT abort whole List
// on one bad file"). When the session is locked the .lthn branch
// still emits a degraded record (ID + Version + Channel from header)
// because header MAC verification only needs the PUBLIC key — no
// unlock required.
//
// The s receiver may be nil for tests of the legacy plaintext-only
// path; the at-rest header-only branch is skipped when s is nil so
// existing helpers stay usable.
func (s *Service) loadCampaigns() ([]Campaign, error) {
	dirR := campaignsDir()
	if !dirR.OK {
		return nil, core.E("campaigns.loadCampaigns", dirR.Error(), nil)
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
	var cs []Campaign

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
			c, err := s.loadHeaderOnly(fpath)
			if err != nil {
				// MAC-failure / corrupt envelope / decode error —
				// SKIP per RFC §4.1.
				continue
			}
			seen[id] = true
			cs = append(cs, c)
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
		c, err := parseCampaign(raw.Value.([]byte))
		if err != nil {
			continue
		}
		cs = append(cs, c)
	}
	return cs, nil
}

// loadOne resolves a single campaign by ID, returning the full record
// with Body populated. Dual-format (RFC §3.1):
//   - .lthn is preferred. Decrypt requires an unlocked session; on a
//     locked-session read this returns the typed campaigns.session.
//     locked error so callers (Get) surface the lock state instead of
//     leaking "not found" confusion.
//   - .md is the legacy plaintext fallthrough.
//
// Returns (Campaign, dir, error). The dir return mirrors the
// incidents/runbooks loadOne shape so writers can re-use the discovered
// directory without re-resolving campaignsDir().
func (s *Service) loadOne(id string) (Campaign, string, error) {
	if err := paths.IsValidID(id); err != nil {
		return Campaign{}, "", err
	}
	dirR := campaignsDir()
	if !dirR.OK {
		return Campaign{}, "", core.E("campaigns.loadOne", dirR.Error(), nil)
	}
	dir := dirR.Value.(string)

	// .lthn first — encrypted records take precedence when present.
	// Cerberus #1486 belt-and-braces: WithinDir check after the join.
	lthnPath, jerr := paths.JoinAndCheck(dir, id+".lthn")
	if jerr != nil {
		return Campaign{}, "", jerr
	}
	if exists := core.Stat(lthnPath); exists.OK {
		w, ok := s.atrestWriterFor()
		if !ok {
			return Campaign{}, dir,
				core.E("campaigns.loadOne", "campaigns.session.locked", nil)
		}
		rr := w.Read(lthnPath)
		if !rr.OK {
			return Campaign{}, dir,
				core.E("campaigns.loadOne", rr.Error(), nil)
		}
		res, _ := rr.Value.(recordfile.ReadResult)
		c, perr := parseAtRestRecord(res)
		if perr != nil {
			return Campaign{}, dir, perr
		}
		return c, dir, nil
	}

	// Legacy plaintext .md fallthrough.
	mdPath, jerr := paths.JoinAndCheck(dir, id+".md")
	if jerr != nil {
		return Campaign{}, "", jerr
	}
	raw := core.ReadFile(mdPath)
	if !raw.OK {
		return Campaign{}, "",
			core.E("campaigns.loadOne", "not found: "+id, nil)
	}
	c, err := parseCampaign(raw.Value.([]byte))
	if err != nil {
		return Campaign{}, "", err
	}
	return c, dir, nil
}

// fireCampaignEvent publishes a campaign event on the Core ACTION bus.
func (s *Service) fireCampaignEvent(eventName, campaignID string) {
	if s == nil || s.core == nil {
		return
	}
	s.core.ACTION(CampaignEvent{
		EventName:  eventName,
		CampaignID: campaignID,
		At:         core.Now().UTC(),
	})
}

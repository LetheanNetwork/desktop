// SPDX-Licence-Identifier: EUPL-1.2

// atrest_test.go — Stage E.D.B.3 marketing/audience at-rest cover
// (Mantis #1487 PR-3 wave 3 consumer #2). Anchors:
//
//   - RFC.stage-e-encrypt-at-rest v2 §2.4 per-field MUST table for
//     marketing/audience + Cerberus C#7 Q2 ruling:
//       * name → BODY (REJECT in header)
//       * size → HEADER as LOG-BUCKET enum (`<1k`|`1-10k`|`10-100k`|
//         `100k+`) — raw integer N MUST NEVER appear in the header
//       * criteria + member list → BODY (PII / targeting strategy)
//     All other fields (growth / src / spark) are BODY-only by
//     default-rule.
//   - §4.1 reads-while-locked: header-only List works; full-body Get
//     refused.
//   - §3.1 lazy migration: writes always emit .lthn; .md removed on
//     success; reads accept BOTH formats.
//
// Substrate-level invariants (Trix shape, header MAC, single-unlock,
// schema enforcement) are exercised by pkg/recordfile/atrest_test.go
// — this file pins consumer-side compositions only.

package audience_test

import (
	"sync"
	"testing"

	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/account"
	subject "dappco.re/lthn/desktop/pkg/marketing/audience"
	"forge.lthn.ai/Snider/Enchantrix/pkg/crypt/std/pgp"
)

// stubKeySessionGate is the at-rest-capable test double — satisfies
// both the narrow SessionGate (UnlockedAccountIDs) AND the wider
// accountKeyProvider runtime-assertion (PublicKeyFor + PrivateKeyFor)
// the at-rest writer engages. Distinct from stubSessionGate in
// audience_test.go so the existing minimal-gate fail-safe + legacy
// .md tests stay untouched.
type stubKeySessionGate struct {
	ids  []string
	pub  []byte
	priv []byte
}

func (s *stubKeySessionGate) UnlockedAccountIDs() []string { return s.ids }

func (s *stubKeySessionGate) PublicKeyFor(_ string) ([]byte, bool) {
	if len(s.pub) == 0 {
		return nil, false
	}
	cp := make([]byte, len(s.pub))
	copy(cp, s.pub)
	return cp, true
}

// PrivateKeyFor returns a real *account.PrivateKeyHandle so the
// substrate's read path engages the canonical zeroise-on-return
// semantics (Mantis #1589 / Cerberus #18). account.NewPrivateKey
// HandleForTest is the test-only factory exported by pkg/account
// expressly for sibling-package consumers like marketing/audience.
func (s *stubKeySessionGate) PrivateKeyFor(_ string) (*account.PrivateKeyHandle, bool) {
	if len(s.priv) == 0 {
		return nil, false
	}
	cp := make([]byte, len(s.priv))
	copy(cp, s.priv)
	return account.NewPrivateKeyHandleForTest(cp), true
}

// atrestKey* are the package-shared keypair generated once per process
// so multiple at-rest tests reuse the same costly setup.
var (
	atrestKeyOnce sync.Once
	atrestKeyPub  []byte
	atrestKeyPriv []byte
)

// genAtRestKeyPair lazily generates a real PGP keypair for the at-
// rest tests. Idempotent — subsequent calls reuse the cached keys.
//
//	pub, priv := genAtRestKeyPair(t)
func genAtRestKeyPair(t *testing.T) (pub, priv []byte) {
	t.Helper()
	atrestKeyOnce.Do(func() {
		svc := pgp.NewService()
		p, k, err := svc.GenerateKeyPair("Test", "test@lthn.local", "test")
		if err != nil {
			t.Fatalf("generate test key pair: %v", err)
		}
		atrestKeyPub = p
		atrestKeyPriv = k
	})
	return atrestKeyPub, atrestKeyPriv
}

// newAtRestTestSvc constructs an audience.Service pre-wired with the
// WIDE stubKeySessionGate (ids + pub + priv). The at-rest writer
// engages by default — Create produces `<id>.lthn` envelopes, Get
// decrypts via PrivateKeyFor.
//
// Tests that need the locked-session path call SetSessionGate
// explicitly with ids=[]. Tests that need the legacy plaintext .md
// fallback use the NARROW stubSessionGate from audience_test.go.
//
// Usage example:
//
//	svc := newAtRestTestSvc(t)
//	r := svc.Create(subject.CreateInput{Name: "..."})
func newAtRestTestSvc(t *testing.T) *subject.Service {
	t.Helper()
	pub, priv := genAtRestKeyPair(t)
	svc := subject.NewService(nil)
	svc.SetSessionGate(&stubKeySessionGate{
		ids:  []string{"acct-test"},
		pub:  pub,
		priv: priv,
	})
	return svc
}

// trixHeaderEnd returns the byte offset at which the Trix header ends
// (i.e. where the encrypted payload begins) per the on-disk shape
// `[Magic(4)][Version(1)][HeaderLen(4)][HeaderJSON][Payload]`. Used to
// slice header-vs-payload regions for plaintext-leak assertions.
//
//	end := trixHeaderEnd(raw)
//	header := string(raw[:end])
func trixHeaderEnd(raw []byte) int {
	if len(raw) < 9 {
		return 0
	}
	hdrLen := int(uint32(raw[5])<<24 | uint32(raw[6])<<16 | uint32(raw[7])<<8 | uint32(raw[8]))
	end := 9 + hdrLen
	if end > len(raw) {
		return len(raw)
	}
	return end
}

// escapeAngle returns s with `<` replaced by the JSON unicode-escape
// `<`. The substrate's canonical-json header serialisation uses
// the escaped form for the `<1k` log bucket value; tests must accept
// either surface form when asserting on raw header bytes.
//
//	escapeAngle("<1k") // → "\\u003c1k"
func escapeAngle(s string) string {
	out := make([]byte, 0, len(s)+5)
	for i := 0; i < len(s); i++ {
		if s[i] == '<' {
			// Six literal bytes: backslash u 0 0 3 c.
			out = append(out, '\\', 'u', '0', '0', '3', 'c')
			continue
		}
		out = append(out, s[i])
	}
	return string(out)
}

// audienceLthnPath returns the absolute path to the encrypted segment
// file for the named id under the test HOME.
//
//	p := audienceLthnPath(t, home, "local-ai-developers")
func audienceLthnPath(t *testing.T, home, id string) string {
	t.Helper()
	return core.PathJoin(home, "Lethean", "marketing", "audience", id+".lthn")
}

// --- Per-field MUST rules (RFC §2.4 + Cerberus C#7 Q2) --------------

// TestAudience_AtRestSchema_NameInBodyNotHeader_Bad — Name (segment
// branding, PII-adjacent) MUST NEVER appear in the plaintext header.
// Body decrypt round-trips the value back to the in-memory Segment.
func TestAudience_AtRestSchema_NameInBodyNotHeader_Bad(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	svc := newAtRestTestSvc(t)

	r := svc.Create(subject.CreateInput{
		Name: "Sentinel Bank · Q3 prospect list",
		Src:  "manual",
		N:    50,
	})
	if !r.OK {
		t.Fatalf("Create: %s", r.Error())
	}
	seg := r.Value.(subject.Segment)
	lthnPath := audienceLthnPath(t, home, seg.ID)
	raw := core.ReadFile(lthnPath)
	if !raw.OK {
		t.Fatalf("read encrypted file: %s", raw.Error())
	}
	body, _ := raw.Value.([]byte)
	header := string(body[:trixHeaderEnd(body)])

	if core.Contains(header, "Sentinel Bank") {
		t.Fatal("Name leaked into encrypted-file header — must be body-only (RFC §2.4)")
	}
	if core.Contains(string(body[trixHeaderEnd(body):]), "Sentinel Bank") {
		t.Fatal("Name appeared in ciphertext payload — encryption skipped?")
	}

	// Round-trip via Get to prove the body retains the value.
	g := svc.Get(seg.ID)
	if !g.OK {
		t.Fatalf("Get after Create: %s", g.Error())
	}
	got := g.Value.(subject.Segment)
	if got.Name != "Sentinel Bank · Q3 prospect list" {
		t.Fatalf("Name round-trip lost: got %q", got.Name)
	}
}

// TestAudience_AtRestSchema_SizeBucketInHeader_Good — size projects
// into the header as a LogSizeBucket enum value per RFC §2.4 +
// Cerberus C#7 Q2. The chosen N=4892 maps to "1-10k".
func TestAudience_AtRestSchema_SizeBucketInHeader_Good(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	svc := newAtRestTestSvc(t)

	r := svc.Create(subject.CreateInput{
		Name: "header-size-probe",
		Src:  "signup-tagged",
		N:    4892,
	})
	if !r.OK {
		t.Fatalf("Create: %s", r.Error())
	}
	seg := r.Value.(subject.Segment)
	lthnPath := audienceLthnPath(t, home, seg.ID)
	raw := core.ReadFile(lthnPath)
	if !raw.OK {
		t.Fatalf("read encrypted file: %s", raw.Error())
	}
	body, _ := raw.Value.([]byte)
	header := string(body[:trixHeaderEnd(body)])

	if !core.Contains(header, `"size":"1-10k"`) {
		t.Fatalf("expected header size=\"1-10k\" for N=4892, got header: %s", header)
	}
}

// TestAudience_AtRestSchema_RawNNeverInHeader_Bad — the integer N
// MUST NEVER appear in the plaintext header. Only the bucket enum is
// header-visible (Cerberus C#7 Q2: operators see magnitude, not exact
// count). Probe with values across every bucket boundary.
func TestAudience_AtRestSchema_RawNNeverInHeader_Bad(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	svc := newAtRestTestSvc(t)

	type probe struct {
		n      int
		bucket string
	}
	// N values chosen with two constraints: (1) cover every bucket
	// boundary, (2) the decimal representation MUST NOT be a substring
	// of any reasonable Unix timestamp the substrate writes into
	// created_at/updated_at (10-digit values starting "17" or "18").
	// Values >=999_999_999 or composed so a 4-5 digit run never lands
	// inside the timestamp digit run.
	cases := []probe{
		{n: 873, bucket: "<1k"},        // 3-digit, won't collide
		{n: 4892, bucket: "1-10k"},     // pre-existing fixture value
		{n: 42813, bucket: "10-100k"},  // 5-digit, won't substring 1779…
		{n: 250000, bucket: "100k+"},   // 6-digit, won't substring 1779…
	}
	for _, c := range cases {
		r := svc.Create(subject.CreateInput{
			Name: "raw-n-probe-" + c.bucket,
			Src:  "manual",
			N:    c.n,
		})
		if !r.OK {
			t.Fatalf("Create(N=%d): %s", c.n, r.Error())
		}
		seg := r.Value.(subject.Segment)
		lthnPath := audienceLthnPath(t, home, seg.ID)
		raw := core.ReadFile(lthnPath)
		if !raw.OK {
			t.Fatalf("read encrypted file: %s", raw.Error())
		}
		body, _ := raw.Value.([]byte)
		header := string(body[:trixHeaderEnd(body)])

		// The plain-text `"n":` key MUST NOT appear in the header (no
		// raw-count field projection — the substrate's writer ONLY
		// projects what HeaderFor returns, and HeaderFor returns only
		// `size`).
		if core.Contains(header, `"n":`) {
			t.Fatalf("raw `n` key leaked into encrypted-file header: %s", header)
		}
		// The raw integer MUST NOT appear as a free-standing substring
		// in the header. Choice of probe N values above guarantees no
		// timestamp collision.
		needle := core.Sprintf("%d", c.n)
		if core.Contains(header, needle) {
			t.Fatalf("raw N=%d leaked into encrypted-file header (RFC §2.4 + Cerberus C#7 Q2): %s",
				c.n, header)
		}
		// The bucket label MUST appear in the header (positive
		// assertion that the bucket-projection ran).
		// canonical-json escapes `<` as < — accept either literal
		// or unicode-escaped form. The bucket projection is the truth;
		// the surface form is the encoder's choice.
		wantLiteral := `"size":"` + c.bucket + `"`
		wantEscaped := `"size":"` + escapeAngle(c.bucket) + `"`
		if !core.Contains(header, wantLiteral) && !core.Contains(header, wantEscaped) {
			t.Fatalf("expected header size=%q for N=%d, got header: %s", c.bucket, c.n, header)
		}
	}
}

// TestAudience_AtRestSchema_MetricsInBodyNotHeader_Bad — Growth / Src
// / Spark are sensitive (rate-of-change reveals momentum, src reveals
// targeting strategy, spark reveals the magnitudes already covered by
// the BODY-only N rule). RFC §2.4 names no header key for any of
// them; default-body rule keeps them encrypted. Header MUST NOT
// contain the values.
func TestAudience_AtRestSchema_MetricsInBodyNotHeader_Bad(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	svc := newAtRestTestSvc(t)

	r := svc.Create(subject.CreateInput{
		Name:   "metric-leak-probe",
		Src:    "telemetry · opt-in",
		N:      4892,
		Growth: "+184 / w",
		Spark:  "284,302,318,341,362,401,438",
	})
	if !r.OK {
		t.Fatalf("Create: %s", r.Error())
	}
	seg := r.Value.(subject.Segment)
	lthnPath := audienceLthnPath(t, home, seg.ID)
	raw := core.ReadFile(lthnPath)
	if !raw.OK {
		t.Fatalf("read encrypted file: %s", raw.Error())
	}
	body, _ := raw.Value.([]byte)
	header := string(body[:trixHeaderEnd(body)])

	for _, banned := range []string{
		"+184 / w",
		"telemetry",
		"284,302,318",
		`"growth":`,
		`"src":`,
		`"spark":`,
	} {
		if core.Contains(header, banned) {
			t.Fatalf("metric %q leaked into encrypted-file header (RFC §2.4 default-body): %s",
				banned, header)
		}
	}

	// Round-trip body confirms persistence.
	g := svc.Get(seg.ID)
	if !g.OK {
		t.Fatalf("Get: %s", g.Error())
	}
	got := g.Value.(subject.Segment)
	if got.Growth != "+184 / w" || got.Src != "telemetry · opt-in" ||
		got.Spark != "284,302,318,341,362,401,438" {
		t.Fatalf("metrics round-trip lost: growth=%q src=%q spark=%q",
			got.Growth, got.Src, got.Spark)
	}
}

// --- Reads-while-locked (RFC §4.1 + §4.2) ---------------------------

// TestAudience_LockedSession_ListWorks_Good — when the SessionGate
// reports zero unlocked accounts AFTER a record was written while
// unlocked, List MUST still return the encrypted record (header-only
// projection). The PublicKeyFor call does NOT require unlock so MAC
// verification stays open.
//
// Operator sees:
//   - ID from the header
//   - Name / N / Growth / Src / Spark EMPTY (BODY-only)
//   - the header `size` bucket is present in the file (Cerberus C#7
//     Q2 operator context) but NOT projected onto the wire `n` (raw
//     count vs bucket conflation guard).
func TestAudience_LockedSession_ListWorks_Good(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	svc := newAtRestTestSvc(t)
	pub, priv := genAtRestKeyPair(t)

	// Seed: write while unlocked.
	cr := svc.Create(subject.CreateInput{
		Name: "header-only-survives-lock",
		Src:  "signup-tagged",
		N:    4892,
	})
	if !cr.OK {
		t.Fatalf("seed Create: %s", cr.Error())
	}

	// Flip to locked while keeping PublicKeyFor wired (so header MAC
	// verify can still resolve a pub key without unlock — mirrors
	// account.Service.PublicKeyFor's no-unlock contract).
	svc.SetSessionGate(&stubKeySessionGate{ids: []string{}, pub: pub, priv: priv})

	r := svc.List(subject.ListInput{})
	if !r.OK {
		t.Fatalf("List while locked MUST succeed (header-only path), got: %s", r.Error())
	}
	out := r.Value.(subject.ListOutput)
	if len(out.Segments) != 1 {
		t.Fatalf("expected 1 list entry while locked, got %d", len(out.Segments))
	}
	got := out.Segments[0]
	// Header-only projection: every BODY-only field empty, ID present.
	if got.Name != "" {
		t.Fatalf("header-only list entry MUST NOT carry Name, got %q", got.Name)
	}
	if got.N != 0 {
		t.Fatalf("header-only list entry MUST NOT carry raw N (wire `n` shape is integer; bucket lives in header only), got %d", got.N)
	}
	if got.Growth != "" {
		t.Fatalf("header-only list entry MUST NOT carry Growth, got %q", got.Growth)
	}
	if got.Src != "" {
		t.Fatalf("header-only list entry MUST NOT carry Src, got %q", got.Src)
	}
	if got.Spark != "" {
		t.Fatalf("header-only list entry MUST NOT carry Spark, got %q", got.Spark)
	}
	if got.ID == "" {
		t.Fatal("header-only list entry MUST carry ID from header")
	}
}

// TestAudience_LockedSession_GetRefused_Bad — Get on an encrypted
// record while the session is locked MUST refuse. The substrate's Read
// calls SingleUnlockedAccount() and the adapter returns
// marketing-audience.no_unlocked_account; the wails layer surfaces
// session.locked (or atrest.* codes) verbatim for the frontend.
func TestAudience_LockedSession_GetRefused_Bad(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	svc := newAtRestTestSvc(t)
	pub, priv := genAtRestKeyPair(t)

	cr := svc.Create(subject.CreateInput{
		Name: "locked-get-target",
		Src:  "manual",
		N:    100,
	})
	if !cr.OK {
		t.Fatalf("seed Create: %s", cr.Error())
	}
	id := cr.Value.(subject.Segment).ID

	// Lock the session.
	svc.SetSessionGate(&stubKeySessionGate{ids: []string{}, pub: pub, priv: priv})

	g := svc.Get(id)
	if g.OK {
		t.Fatal("Get on encrypted record while locked MUST be refused")
	}
	errStr := g.Error()
	if !core.Contains(errStr, "session.locked") &&
		!core.Contains(errStr, "multi_account_ambiguous") &&
		!core.Contains(errStr, "no_unlocked_account") {
		t.Fatalf("expected session.locked / multi_account_ambiguous / no_unlocked_account on locked Get, got %q", errStr)
	}
}

// TestAudience_UnlockedSession_GetReturnsBody_Good — Get round-trips
// the full body (Name, N, Growth, Src, Spark) when the session is
// unlocked.
func TestAudience_UnlockedSession_GetReturnsBody_Good(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	svc := newAtRestTestSvc(t)

	cr := svc.Create(subject.CreateInput{
		Name:   "Sentinel Bank · Q3",
		Src:    "manual",
		N:      4892,
		Growth: "+184 / w",
		Spark:  "284,302,318,341",
	})
	if !cr.OK {
		t.Fatalf("seed Create: %s", cr.Error())
	}
	id := cr.Value.(subject.Segment).ID

	g := svc.Get(id)
	if !g.OK {
		t.Fatalf("Get on unlocked record MUST succeed: %s", g.Error())
	}
	d := g.Value.(subject.Segment)
	if d.Name != "Sentinel Bank · Q3" {
		t.Fatalf("Name round-trip lost: got %q", d.Name)
	}
	if d.N != 4892 {
		t.Fatalf("N round-trip lost: got %d", d.N)
	}
	if d.Growth != "+184 / w" {
		t.Fatalf("Growth round-trip lost: got %q", d.Growth)
	}
	if d.Src != "manual" {
		t.Fatalf("Src round-trip lost: got %q", d.Src)
	}
	if d.Spark != "284,302,318,341" {
		t.Fatalf("Spark round-trip lost: got %q", d.Spark)
	}
}

// --- Lazy migration round-trips (RFC §3.1) --------------------------

// TestAudience_AtRest_ReadAcceptsLegacyMd_Good — a pre-existing .md
// plaintext stays readable via loadOne fallthrough until the next
// write triggers re-encrypt-as-.lthn.
func TestAudience_AtRest_ReadAcceptsLegacyMd_Good(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	svc := newAtRestTestSvc(t)

	dir := core.PathJoin(home, "Lethean", "marketing", "audience")
	if mk := core.MkdirAll(dir, 0o700); !mk.OK {
		t.Fatalf("MkdirAll: %s", mk.Error())
	}
	legacyID := "legacy-segment-2026"
	mdPath := core.PathJoin(dir, legacyID+".md")
	legacy := []byte("---\nid: " + legacyID + "\nname: LegacySeg\nn: 4321\ngrowth: \"+10 / w\"\nsrc: manual\nspark: \"\"\n---\n")
	if w := core.WriteFile(mdPath, legacy, 0o600); !w.OK {
		t.Fatalf("WriteFile legacy: %s", w.Error())
	}

	g := svc.Get(legacyID)
	if !g.OK {
		t.Fatalf("Get on legacy .md MUST succeed via fallthrough: %s", g.Error())
	}
	got := g.Value.(subject.Segment)
	if got.Name != "LegacySeg" {
		t.Fatalf("legacy Name lost: got %q", got.Name)
	}
	if got.N != 4321 {
		t.Fatalf("legacy N lost: got %d", got.N)
	}
}

// TestAudience_AtRest_WriteRemovesLegacyMd_Good — Update against a
// legacy .md record writes the .lthn envelope AND removes the legacy
// plaintext file (RFC §3.1 lazy-migration completion).
func TestAudience_AtRest_WriteRemovesLegacyMd_Good(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	svc := newAtRestTestSvc(t)

	dir := core.PathJoin(home, "Lethean", "marketing", "audience")
	if mk := core.MkdirAll(dir, 0o700); !mk.OK {
		t.Fatalf("MkdirAll: %s", mk.Error())
	}
	id := "legacy-cutover"
	mdPath := core.PathJoin(dir, id+".md")
	legacy := []byte("---\nid: " + id + "\nname: Cutover\nn: 200\ngrowth: \"+5 / w\"\nsrc: manual\nspark: \"\"\n---\n")
	if w := core.WriteFile(mdPath, legacy, 0o600); !w.OK {
		t.Fatalf("seed legacy .md: %s", w.Error())
	}

	ur := svc.Update(subject.UpdateInput{ID: id, N: 250})
	if !ur.OK {
		t.Fatalf("Update: %s", ur.Error())
	}

	// .lthn MUST exist post-write.
	lthnPath := core.PathJoin(dir, id+".lthn")
	if st := core.Stat(lthnPath); !st.OK {
		t.Fatalf("encrypted .lthn MUST be written, missing at %s", lthnPath)
	}
	// .md MUST be removed (lazy-migration completion).
	if st := core.Stat(mdPath); st.OK {
		t.Fatal("legacy .md MUST be removed after successful encrypted write (RFC §3.1)")
	}
}

// TestAudience_AtRest_ReadPrefersLthnOverMd_Good — when both
// extensions exist (mid-migration crash window between AtomicWrite and
// Remove) loadSegments prefers the .lthn entry and skips the .md
// duplicate.
func TestAudience_AtRest_ReadPrefersLthnOverMd_Good(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	svc := newAtRestTestSvc(t)

	cr := svc.Create(subject.CreateInput{
		Name: "encrypted-original",
		Src:  "manual",
		N:    100,
	})
	if !cr.OK {
		t.Fatalf("seed Create: %s", cr.Error())
	}
	id := cr.Value.(subject.Segment).ID

	// Drop a shadow .md alongside the encrypted record.
	dir := core.PathJoin(home, "Lethean", "marketing", "audience")
	mdShadow := core.PathJoin(dir, id+".md")
	shadow := []byte("---\nid: " + id + "\nname: PLAINTEXT-SHADOW\nn: 999\ngrowth: \"+0 / w\"\nsrc: manual\nspark: \"\"\n---\n")
	if w := core.WriteFile(mdShadow, shadow, 0o600); !w.OK {
		t.Fatalf("seed shadow .md: %s", w.Error())
	}

	r := svc.List(subject.ListInput{})
	if !r.OK {
		t.Fatalf("List: %s", r.Error())
	}
	out := r.Value.(subject.ListOutput)
	if len(out.Segments) != 1 {
		t.Fatalf("expected exactly one entry (.lthn preferred), got %d", len(out.Segments))
	}
	// The entry is the encrypted one — header-only projection means
	// Name is empty.
	if out.Segments[0].Name == "PLAINTEXT-SHADOW" {
		t.Fatal("List returned the shadow .md entry instead of preferring .lthn")
	}
}

// --- File-mode + dir-mode cover for the .lthn write path ------------

// TestAudience_AtRest_FileMode0600_Cerberus1487 — the substrate's
// atomic-rename path applies mode 0o600 to the encrypted file. Pin it
// alongside the legacy .md cover in security_test.go.
func TestAudience_AtRest_FileMode0600_Cerberus1487(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	svc := newAtRestTestSvc(t)

	r := svc.Create(subject.CreateInput{
		Name: "mode-probe",
		Src:  "manual",
		N:    50,
	})
	if !r.OK {
		t.Fatalf("Create: %s", r.Error())
	}
	seg := r.Value.(subject.Segment)
	lthnPath := audienceLthnPath(t, home, seg.ID)
	stat := core.Stat(lthnPath)
	if !stat.OK {
		t.Fatalf("stat(%s): %s", lthnPath, stat.Error())
	}
	info := stat.Value.(core.FsFileInfo)
	mode := info.Mode().Perm()
	if int(mode) != 0o600 {
		t.Fatalf(".lthn file mode = %o, want 0o600", mode)
	}
}

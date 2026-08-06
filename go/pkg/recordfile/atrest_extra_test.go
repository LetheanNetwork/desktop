// SPDX-Licence-Identifier: EUPL-1.2

// atrest_extra_test.go — the remaining NewAtRestWriter dep-panic
// branches, Write/Read fault-injection paths that the happy-path and
// tamper suites in atrest_test.go don't reach, and PeekAccountID
// (F-4 / Cerberus #44 PRBW extract) unit coverage.

package recordfile_test

import (
	"errors"
	"testing"

	"dappco.re/lthn/desktop/pkg/recordfile"
	"github.com/Snider/Enchantrix/pkg/crypt/std/pgp"
)

// --- NewAtRestWriter dep panics --------------------------------------

// TestAtRest_NewAtRestWriter_AllDepPanics_Ugly — every required dep
// panics with a distinct typed code when missing.
// TestAtRest_NewAtRestWriter_DepPanics_Ugly (atrest_test.go) already
// pins the PGP-nil branch; this covers the remaining five.
func TestAtRest_NewAtRestWriter_AllDepPanics_Ugly(t *testing.T) {
	fullDeps := func() recordfile.AtRestDeps[dealRecord] {
		return recordfile.AtRestDeps[dealRecord]{
			Surface: recordfile.SurfaceSalesDeals,
			Keys:    &testKeys{},
			PGP:     pgp.NewService(),
			Schema:  dealsSchema,
			Atomic:  newFakeAtomic(),
		}
	}

	cases := []struct {
		name   string
		mutate func(d *recordfile.AtRestDeps[dealRecord])
	}{
		{"surface missing", func(d *recordfile.AtRestDeps[dealRecord]) { d.Surface = "" }},
		{"keys missing", func(d *recordfile.AtRestDeps[dealRecord]) { d.Keys = nil }},
		{"atomic missing", func(d *recordfile.AtRestDeps[dealRecord]) { d.Atomic = nil }},
		{"schema.HeaderFor missing", func(d *recordfile.AtRestDeps[dealRecord]) { d.Schema.HeaderFor = nil }},
		{"schema.IDFor missing", func(d *recordfile.AtRestDeps[dealRecord]) { d.Schema.IDFor = nil }},
		{"schema.VersionFor missing", func(d *recordfile.AtRestDeps[dealRecord]) { d.Schema.VersionFor = nil }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			deps := fullDeps()
			tc.mutate(&deps)
			defer func() {
				if r := recover(); r == nil {
					t.Fatalf("expected panic for %s, got none", tc.name)
				}
			}()
			_ = recordfile.NewAtRestWriter(deps)
		})
	}
}

// --- fakePGP: a PGPService double with independently controllable
// Encrypt/Decrypt failure + oversize-output injection ----------------

type fakePGP struct {
	inner       *pgp.Service
	encryptErr  error
	encryptSize int // when > 0, Encrypt returns this many zero bytes instead of real ciphertext
}

func (f *fakePGP) Encrypt(publicKey, data []byte) ([]byte, error) {
	if f.encryptErr != nil {
		return nil, f.encryptErr
	}
	if f.encryptSize > 0 {
		return make([]byte, f.encryptSize), nil
	}
	return f.inner.Encrypt(publicKey, data)
}

func (f *fakePGP) Decrypt(privateKey, ciphertext []byte) ([]byte, error) {
	return f.inner.Decrypt(privateKey, ciphertext)
}

// --- Write() fault injection -----------------------------------------

// TestAtRest_Write_Bad_PublicKeyLookupFails — AccountKeys.PublicKeyFor
// returning an error must surface as public_key_unavailable, not
// propagate the raw error or panic.
func TestAtRest_Write_Bad_PublicKeyLookupFails(t *testing.T) {
	keys := &testKeys{unlocked: []string{"abc123def4567890"}} // pub deliberately unset
	atomic := newFakeAtomic()
	w := recordfile.NewAtRestWriter(recordfile.AtRestDeps[dealRecord]{
		Surface: recordfile.SurfaceSalesDeals,
		Keys:    keys,
		PGP:     pgp.NewService(),
		Schema:  dealsSchema,
		Atomic:  atomic,
		Now:     func() int64 { return 1779494400 },
	})
	wr := w.Write(newWriteReq(newRecord(), "/test/deal-001.lthn"))
	if wr.OK {
		t.Fatalf("expected public_key_unavailable reject, got OK")
	}
	if wr.Code() != "recordfile.atrest.public_key_unavailable" {
		t.Fatalf("expected public_key_unavailable, got %q", wr.Code())
	}
}

// emptyPubKeys is a minimal AccountKeys double that returns a
// non-nil-error but EMPTY public key — distinct from testKeys, which
// only models the error-returning case. Exercises Write/Read's
// separate "len(pub) == 0" guard.
type emptyPubKeys struct{ unlocked string }

func (k *emptyPubKeys) PublicKeyFor(_ string) ([]byte, error) { return []byte{}, nil }
func (k *emptyPubKeys) PrivateKeyFor(_ string) (recordfile.PrivateKeyHandle, bool) {
	return nil, false
}
func (k *emptyPubKeys) SingleUnlockedAccount() (string, error) { return k.unlocked, nil }

// TestAtRest_Write_Bad_PublicKeyEmpty — PublicKeyFor succeeding with
// zero bytes must be rejected the same as an outright error.
func TestAtRest_Write_Bad_PublicKeyEmpty(t *testing.T) {
	keys := &emptyPubKeys{unlocked: "abc123def4567890"}
	w := recordfile.NewAtRestWriter(recordfile.AtRestDeps[dealRecord]{
		Surface: recordfile.SurfaceSalesDeals,
		Keys:    keys,
		PGP:     pgp.NewService(),
		Schema:  dealsSchema,
		Atomic:  newFakeAtomic(),
		Now:     func() int64 { return 1779494400 },
	})
	wr := w.Write(newWriteReq(newRecord(), "/test/deal-001.lthn"))
	if wr.OK {
		t.Fatalf("expected public_key_unavailable reject, got OK")
	}
	if wr.Code() != "recordfile.atrest.public_key_unavailable" {
		t.Fatalf("expected public_key_unavailable, got %q", wr.Code())
	}
}

// TestAtRest_Write_Bad_PGPEncryptFails — a PGP encrypt failure must
// surface as pgp_encrypt_failed.
func TestAtRest_Write_Bad_PGPEncryptFails(t *testing.T) {
	pgpSvc := pgp.NewService()
	pub, priv, err := pgpSvc.GenerateKeyPair("Test", "test@lthn.local", "test")
	if err != nil {
		t.Fatalf("generate key pair: %v", err)
	}
	keys := &testKeys{pub: pub, priv: priv, unlocked: []string{"abc123def4567890"}}
	fp := &fakePGP{inner: pgpSvc, encryptErr: errors.New("simulated encrypt failure")}
	w := recordfile.NewAtRestWriter(recordfile.AtRestDeps[dealRecord]{
		Surface: recordfile.SurfaceSalesDeals,
		Keys:    keys,
		PGP:     fp,
		Schema:  dealsSchema,
		Atomic:  newFakeAtomic(),
		Now:     func() int64 { return 1779494400 },
	})
	wr := w.Write(newWriteReq(newRecord(), "/test/deal-001.lthn"))
	if wr.OK {
		t.Fatalf("expected pgp_encrypt_failed reject, got OK")
	}
	if wr.Code() != "recordfile.atrest.pgp_encrypt_failed" {
		t.Fatalf("expected pgp_encrypt_failed, got %q", wr.Code())
	}
}

// TestAtRest_Write_Bad_PayloadCapExceeded — a ciphertext larger than
// MaxPayloadBytes must be rejected before ever reaching Atomic.Write.
func TestAtRest_Write_Bad_PayloadCapExceeded(t *testing.T) {
	pgpSvc := pgp.NewService()
	pub, priv, err := pgpSvc.GenerateKeyPair("Test", "test@lthn.local", "test")
	if err != nil {
		t.Fatalf("generate key pair: %v", err)
	}
	keys := &testKeys{pub: pub, priv: priv, unlocked: []string{"abc123def4567890"}}
	fp := &fakePGP{inner: pgpSvc, encryptSize: recordfile.MaxPayloadBytes + 1}
	w := recordfile.NewAtRestWriter(recordfile.AtRestDeps[dealRecord]{
		Surface: recordfile.SurfaceSalesDeals,
		Keys:    keys,
		PGP:     fp,
		Schema:  dealsSchema,
		Atomic:  newFakeAtomic(),
		Now:     func() int64 { return 1779494400 },
	})
	wr := w.Write(newWriteReq(newRecord(), "/test/deal-001.lthn"))
	if wr.OK {
		t.Fatalf("expected payload_cap_exceeded reject, got OK")
	}
	if wr.Code() != "recordfile.atrest.payload_cap_exceeded" {
		t.Fatalf("expected payload_cap_exceeded, got %q", wr.Code())
	}
}

// TestAtRest_Write_Bad_IDForEmpty — a schema whose IDFor returns an
// empty string must reject with id_required before any disk write.
func TestAtRest_Write_Bad_IDForEmpty(t *testing.T) {
	_, keys, atomic := newWriter(t)
	pgpSvc := pgp.NewService()
	rogueSchema := recordfile.HeaderSchema[dealRecord]{
		Surface: recordfile.SurfaceSalesDeals,
		AllowedKeys: map[string]recordfile.FieldValidator{
			"stage": recordfile.ValidateEnum(recordfile.SalesDealsStages),
		},
		HeaderFor:  func(r dealRecord) map[string]any { return map[string]any{"stage": r.Stage} },
		IDFor:      func(r dealRecord) string { return "" },
		VersionFor: func(r dealRecord) int64 { return r.Version },
	}
	w := recordfile.NewAtRestWriter(recordfile.AtRestDeps[dealRecord]{
		Surface: recordfile.SurfaceSalesDeals,
		Keys:    keys,
		PGP:     pgpSvc,
		Schema:  rogueSchema,
		Atomic:  atomic,
		Now:     func() int64 { return 1779494400 },
	})
	wr := w.Write(newWriteReq(newRecord(), "/test/deal-001.lthn"))
	if wr.OK {
		t.Fatalf("expected id_required reject, got OK")
	}
	if wr.Code() != "recordfile.atrest.id_required" {
		t.Fatalf("expected id_required, got %q", wr.Code())
	}
}

// TestAtRest_Write_Bad_HeaderMACComputeFails — a schema key with a
// nil validator (permitted — see validateConsumerHeader) lets an
// unmarshalable value straight through schema validation; the header
// MAC compute step (which canonical-JSON-encodes the whole header)
// must then surface the canonjson failure as-is rather than panicking
// or silently omitting the MAC.
func TestAtRest_Write_Bad_HeaderMACComputeFails(t *testing.T) {
	_, keys, atomic := newWriter(t)
	pgpSvc := pgp.NewService()
	rogueSchema := recordfile.HeaderSchema[dealRecord]{
		Surface: recordfile.SurfaceSalesDeals,
		AllowedKeys: map[string]recordfile.FieldValidator{
			"weird": nil, // no validator — value passes through unchecked
		},
		HeaderFor: func(r dealRecord) map[string]any {
			return map[string]any{"weird": make(chan int)}
		},
		IDFor:      func(r dealRecord) string { return r.ID },
		VersionFor: func(r dealRecord) int64 { return r.Version },
	}
	w := recordfile.NewAtRestWriter(recordfile.AtRestDeps[dealRecord]{
		Surface: recordfile.SurfaceSalesDeals,
		Keys:    keys,
		PGP:     pgpSvc,
		Schema:  rogueSchema,
		Atomic:  atomic,
		Now:     func() int64 { return 1779494400 },
	})
	wr := w.Write(newWriteReq(newRecord(), "/test/deal-001.lthn"))
	if wr.OK {
		t.Fatalf("expected canonjson-driven MAC-compute reject, got OK")
	}
	if wr.Code() != "recordfile.atrest.canonjson.unsupported_type" {
		t.Fatalf("expected canonjson.unsupported_type, got %q (err: %s)", wr.Code(), wr.Error())
	}
}

// --- Read() fault injection -------------------------------------------

// TestAtRest_Read_Bad_FileNotFound — reading a path the AtomicWriter
// has never seen must surface as read_failed.
func TestAtRest_Read_Bad_FileNotFound(t *testing.T) {
	w, _, _ := newWriter(t)
	rr := w.Read("/test/never-written.lthn")
	if rr.OK {
		t.Fatalf("expected read_failed reject, got OK")
	}
	if rr.Code() != "recordfile.atrest.read_failed" {
		t.Fatalf("expected read_failed, got %q", rr.Code())
	}
}

// TestAtRest_Read_Bad_PublicKeyLookupFails — PublicKeyFor erroring
// AFTER a successful decode must surface as public_key_unavailable.
func TestAtRest_Read_Bad_PublicKeyLookupFails(t *testing.T) {
	w, keys, _ := newWriter(t)
	wr := w.Write(newWriteReq(newRecord(), "/test/deal-001.lthn"))
	if !wr.OK {
		t.Fatalf("Write failed: %s", wr.Error())
	}
	// Key becomes unavailable between write and read (e.g. account
	// removed / keystore corrupted) — same account stays "unlocked"
	// so we reach the PublicKeyFor call inside Read.
	keys.pub = nil

	rr := w.Read("/test/deal-001.lthn")
	if rr.OK {
		t.Fatalf("expected public_key_unavailable reject, got OK")
	}
	if rr.Code() != "recordfile.atrest.public_key_unavailable" {
		t.Fatalf("expected public_key_unavailable, got %q", rr.Code())
	}
}

// TestAtRest_Read_Bad_AccountMisdelivered_DifferentUnlockedAccount —
// a file correctly written+MAC'd for one account, read back while a
// DIFFERENT account_id is the "single unlocked" account (but the
// SAME underlying key material — e.g. a multi-alias keystore), must
// reject with account_id_misdelivered. This is the one scenario that
// reaches the check without also invalidating the header MAC (the
// MAC's key input is the resolved pubkey bytes, not the account_id
// string, so an id-only mismatch survives MAC verification).
func TestAtRest_Read_Bad_AccountMisdelivered_DifferentUnlockedAccount(t *testing.T) {
	w, keys, _ := newWriter(t)
	wr := w.Write(newWriteReq(newRecord(), "/test/deal-001.lthn"))
	if !wr.OK {
		t.Fatalf("Write failed: %s", wr.Error())
	}
	// testKeys.PublicKeyFor ignores its argument, so swapping the
	// unlocked id alone (same pub/priv bytes) keeps the MAC valid
	// while flipping hdr.AccountID != unlockedID.
	keys.unlocked = []string{"different-account-0000"}

	rr := w.Read("/test/deal-001.lthn")
	if rr.OK {
		t.Fatalf("expected account_id_misdelivered reject, got OK")
	}
	if rr.Code() != "recordfile.atrest.account_id_misdelivered" {
		t.Fatalf("expected account_id_misdelivered, got %q (err: %s)", rr.Code(), rr.Error())
	}
}

// --- PeekAccountID (F-4 / Cerberus #44 PRBW extract) ------------------

// buildPeekBlob assembles a raw at-rest blob shell (Magic + Version +
// HeaderLen + HeaderJSON, no payload) matching PeekAccountID's
// expected wire prefix, so header-parsing edge cases can be pinned
// without a full Write() round trip.
func buildPeekBlob(headerJSON string) []byte {
	hdr := []byte(headerJSON)
	buf := make([]byte, 9+len(hdr))
	copy(buf[0:4], []byte("LTHN"))
	buf[4] = 2 // version byte — PeekAccountID does not interpret it
	n := uint32(len(hdr))
	buf[5] = byte(n >> 24)
	buf[6] = byte(n >> 16)
	buf[7] = byte(n >> 8)
	buf[8] = byte(n)
	copy(buf[9:], hdr)
	return buf
}

func TestAtRest_PeekAccountID_Good_RoundTrip(t *testing.T) {
	w, _, atomic := newWriter(t)
	wr := w.Write(newWriteReq(newRecord(), "/test/deal-001.lthn"))
	if !wr.OK {
		t.Fatalf("Write failed: %s", wr.Error())
	}
	raw, _ := atomic.ReadFile("/test/deal-001.lthn")
	id, err := recordfile.PeekAccountID(raw)
	if err != nil {
		t.Fatalf("PeekAccountID failed on a real written blob: %v", err)
	}
	if id != "abc123def4567890" {
		t.Fatalf("PeekAccountID id mismatch: got %q", id)
	}
}

func TestAtRest_PeekAccountID_Bad_BlobTooShort(t *testing.T) {
	_, err := recordfile.PeekAccountID(make([]byte, 5))
	if err == nil {
		t.Fatalf("expected blob_too_short reject, got nil")
	}
	if !errCodeIs(err, "recordfile.atrest.peek.blob_too_short") {
		t.Fatalf("expected blob_too_short, got %v", err)
	}
}

func TestAtRest_PeekAccountID_Bad_HeaderLenZero(t *testing.T) {
	blob := make([]byte, 9)
	copy(blob[0:4], []byte("LTHN"))
	// bytes 5-8 left zero → hdrLen == 0
	_, err := recordfile.PeekAccountID(blob)
	if err == nil {
		t.Fatalf("expected header_len_oob reject, got nil")
	}
	if !errCodeIs(err, "recordfile.atrest.peek.header_len_oob") {
		t.Fatalf("expected header_len_oob, got %v", err)
	}
}

func TestAtRest_PeekAccountID_Bad_HeaderLenOverflows(t *testing.T) {
	blob := make([]byte, 9)
	copy(blob[0:4], []byte("LTHN"))
	blob[5], blob[6], blob[7], blob[8] = 0, 0, 0x10, 0x00 // hdrLen = 4096, far beyond len(blob)
	_, err := recordfile.PeekAccountID(blob)
	if err == nil {
		t.Fatalf("expected header_len_oob reject, got nil")
	}
	if !errCodeIs(err, "recordfile.atrest.peek.header_len_oob") {
		t.Fatalf("expected header_len_oob, got %v", err)
	}
}

func TestAtRest_PeekAccountID_Bad_HeaderNotJSON(t *testing.T) {
	blob := buildPeekBlob("not valid json at all")
	_, err := recordfile.PeekAccountID(blob)
	if err == nil {
		t.Fatalf("expected json_unmarshal reject, got nil")
	}
	if !errCodeIs(err, "recordfile.atrest.peek.json_unmarshal") {
		t.Fatalf("expected json_unmarshal, got %v", err)
	}
}

func TestAtRest_PeekAccountID_Bad_AccountIDMissing(t *testing.T) {
	blob := buildPeekBlob(`{"schema":"lthn.atrest.v1"}`)
	_, err := recordfile.PeekAccountID(blob)
	if err == nil {
		t.Fatalf("expected account_id_missing reject, got nil")
	}
	if !errCodeIs(err, "recordfile.atrest.peek.account_id_missing") {
		t.Fatalf("expected account_id_missing, got %v", err)
	}
}

func TestAtRest_PeekAccountID_Good_ExplicitAccountBlob(t *testing.T) {
	blob := buildPeekBlob(`{"account":{"id":"acct-42"}}`)
	id, err := recordfile.PeekAccountID(blob)
	if err != nil {
		t.Fatalf("PeekAccountID failed: %v", err)
	}
	if id != "acct-42" {
		t.Fatalf("id mismatch: got %q want %q", id, "acct-42")
	}
}

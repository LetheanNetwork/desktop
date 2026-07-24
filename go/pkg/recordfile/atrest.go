// SPDX-Licence-Identifier: EUPL-1.2

// Package recordfile — at-rest encryption substrate for user-data
// writers (sales/deals, incidents, runbooks, marketing/*) per
// RFC.stage-e-encrypt-at-rest v2 (Mantis #1487 E.D.B.0).
//
// AtRestWriter[T] is the generic substrate that compile-time
// enforces every Cerberus #34 "consumer cannot bypass" invariant:
//
//   - Account.id binding + key-fingerprint precheck (T3 defence)
//   - Header MAC computed inside Write, verified inside Read (T1+T2)
//   - PrivateKeyHandle.Use closure (no raw []byte access to consumers)
//   - SingleUnlockedAccount() reject inside Write AND Read (#1636)
//   - Schema version mandatory on decode (Cerberus lean-7 amendment)
//   - Body checksum computed + verified by substrate (no per-writer roll)
//   - Trix HEADER + NEWLINE + PAYLOAD shape via canonical encoder
//
// Wire shape (per RFC §2.2 + §2.4):
//
//	[Magic="LTHN"(4)] [Version=2(1)] [HeaderLen(4)]
//	[HeaderJSON {schema, id, version, account, surface, body.checksum,
//	             header.mac, created_at, updated_at, ...allowed-keys}]
//	[Payload = pgp.Encrypt(account_pub, recordfile.Stitch(fm, body))]
//
// Usage example (sales/deals consumer):
//
//	writer := recordfile.NewAtRestWriter(recordfile.AtRestDeps[DealRecord]{
//	    Surface: recordfile.SurfaceSalesDeals,
//	    Keys:    accountKeysAdapter{accountSvc},  // wraps account.Service
//	    PGP:     pgp.NewService(),
//	    Schema:  dealsHeaderSchema,
//	    Now:     core.UnixNow,
//	})
//	r := writer.Write(WriteRequest[DealRecord]{
//	    AccountID: "abc123def4567890",
//	    Record:    rec,
//	    BodyYAML:  yamlBytes,
//	    BodyText:  []byte(rec.Notes),
//	    PriorHash: "",  // first-write
//	    DestPath:  "/Users/me/Lethean/sales/deals/<id>.lthn",
//	})

package recordfile

import (
	"bytes"
	"sync"

	core "dappco.re/go"
	"github.com/Snider/Enchantrix/pkg/trix"
)

// --- Lib-defined interfaces (RFC §5.1) -------------------------------

// AccountKeys is the lib-defined account-key surface AtRestWriter
// depends on. Method names mirror account.Service exactly so the
// consumer wires a thin pass-through adapter.
//
// Pure read-side — substrate never asks AccountKeys to MUTATE state.
//
//	type adapter struct{ s *account.Service }
//	func (a adapter) PublicKeyFor(id string) ([]byte, error) { ... }
type AccountKeys interface {
	// PublicKeyFor returns the raw armoured public-key bytes for
	// accountID, or a non-nil error when unknown / unreadable.
	// The substrate uses this for header MAC + Trix encrypt — does
	// NOT require unlock.
	PublicKeyFor(accountID string) ([]byte, error)

	// PrivateKeyFor returns a single-use handle wrapping the
	// in-memory unlocked private-key bytes. (nil, false) when no
	// unlocked key is available. The substrate ALWAYS uses
	// handle.Use(...) — raw []byte access is impossible by contract.
	PrivateKeyFor(accountID string) (PrivateKeyHandle, bool)

	// SingleUnlockedAccount returns the single unlocked account_id
	// or a typed error when zero / multiple accounts are unlocked.
	// Mirrors mail/service.go:singleUnlockedAccount semantics per
	// Mantis #1591. Substrate calls this BEFORE encrypt + BEFORE
	// decode work to fold ADD-HIGH-2.
	SingleUnlockedAccount() (string, error)
}

// PrivateKeyHandle is the substrate-side view of
// account.PrivateKeyHandle. Single-use closure that zeroises on
// return so per-decrypt key copies do not accumulate on the heap.
//
//	err := handle.Use(func(priv []byte) error {
//	    plaintext, err := pgpSvc.Decrypt(priv, ciphertext)
//	    return err
//	})
type PrivateKeyHandle interface {
	Use(fn func(priv []byte) error) error
}

// PGPService is the lib-defined PGP surface (matches
// external/enchantrix/pkg/crypt/std/pgp.Service:94/:118).
//
//	svc := pgp.NewService()  // satisfies PGPService
type PGPService interface {
	Encrypt(publicKey, data []byte) ([]byte, error)
	Decrypt(privateKey, ciphertext []byte) ([]byte, error)
}

// AtomicWriter writes plaintext at-rest envelopes to disk under the
// existing paths.AtomicWriteWithVersion + IfMatchHash gates. Lib-
// defined so the substrate stays independent of the paths package
// (consumer wires the adapter — see RFC §3.2 anchors).
//
//	type pathAdapter struct{}
//	func (pathAdapter) Write(req recordfile.AtomicWriteRequest) error { ... }
type AtomicWriter interface {
	Write(req AtomicWriteRequest) error
	ReadFile(path string) ([]byte, error)
	Remove(path string) error
}

// AtomicWriteRequest is the substrate-side write payload. Mirrors
// paths.WriteInput's IfMatchHash + IfNotExist gates without dragging
// the paths package into this lib.
type AtomicWriteRequest struct {
	Path       string
	Payload    []byte
	Mode       uint32
	IfMatch    string // hex sha256 of prior ciphertext, or "" for first-write
	IfNotExist bool   // create-only when true
}

// --- Substrate dependency wiring -------------------------------------

// AtRestDeps bundles the lib-defined collaborators an AtRestWriter
// needs. Consumer constructs once at Service.New time.
type AtRestDeps[T any] struct {
	Surface Surface             // canonical surface label
	Keys    AccountKeys         // wraps account.Service
	PGP     PGPService          // wraps pgp.NewService()
	Schema  HeaderSchema[T]     // per-surface field whitelist
	Atomic  AtomicWriter        // wraps paths.AtomicWriteWithVersion
	Now     func() int64        // unix-seconds source; defaults to core.UnixNow
}

// AtRestWriter is the generic at-rest substrate. T is the per-writer
// record type — the substrate does not introspect it; the schema's
// HeaderFor / IDFor / VersionFor projections do the work.
type AtRestWriter[T any] struct {
	deps AtRestDeps[T]
	mu   sync.Mutex // serialises encode/decode within a single writer
}

// NewAtRestWriter constructs an AtRestWriter against the supplied
// deps. Panics with a typed error when a required dep is missing —
// failing loud at Service.New beats a nil-deref deep inside a Write
// call.
//
//	w := recordfile.NewAtRestWriter(deps)
func NewAtRestWriter[T any](deps AtRestDeps[T]) *AtRestWriter[T] {
	if deps.Surface == "" {
		panic(core.NewCode("recordfile.atrest.deps.surface_missing",
			"AtRestDeps.Surface is required"))
	}
	if deps.Keys == nil {
		panic(core.NewCode("recordfile.atrest.deps.keys_missing",
			"AtRestDeps.Keys is required"))
	}
	if deps.PGP == nil {
		panic(core.NewCode("recordfile.atrest.deps.pgp_missing",
			"AtRestDeps.PGP is required"))
	}
	if deps.Atomic == nil {
		panic(core.NewCode("recordfile.atrest.deps.atomic_missing",
			"AtRestDeps.Atomic is required"))
	}
	if deps.Schema.HeaderFor == nil {
		panic(core.NewCode("recordfile.atrest.deps.schema_headerfor_missing",
			"HeaderSchema.HeaderFor is required"))
	}
	if deps.Schema.IDFor == nil {
		panic(core.NewCode("recordfile.atrest.deps.schema_idfor_missing",
			"HeaderSchema.IDFor is required"))
	}
	if deps.Schema.VersionFor == nil {
		panic(core.NewCode("recordfile.atrest.deps.schema_versionfor_missing",
			"HeaderSchema.VersionFor is required"))
	}
	if deps.Now == nil {
		deps.Now = core.UnixNow
	}
	return &AtRestWriter[T]{deps: deps}
}

// --- Write path ------------------------------------------------------

// WriteRequest is the substrate-side write payload. Caller supplies
// the record, the YAML frontmatter bytes, the body bytes, and the
// destination path; the substrate handles single-unlock assertion,
// schema validation, body stitch + encrypt, header MAC, and atomic
// write under the IfMatch optimistic-lock gate.
type WriteRequest[T any] struct {
	AccountID string // expected unlocked account_id
	Record    T      // typed record passed through Schema.HeaderFor + IDFor + VersionFor
	BodyYAML  []byte // YAML frontmatter for in-body stitching
	BodyText  []byte // body bytes (notes / post-mortem / runbook body)
	DestPath  string // absolute path to write (typically <root>/<surface>/<id>.lthn)
	PriorHash string // hex sha256 of prior ciphertext, or "" for first-write (IfMatch gate)
}

// Write encrypts + writes a record. The substrate:
//
//  1. Asserts SingleUnlockedAccount() — multi-unlock returns
//     typed "recordfile.atrest.multi_account_ambiguous".
//  2. Asserts req.AccountID matches the unlocked id — mismatch
//     returns "recordfile.atrest.account_id_mismatch".
//  3. Resolves the public key + computes its sha256 fingerprint.
//  4. Validates the schema's HeaderFor output (reserved-key check +
//     per-field validator).
//  5. Stitches BodyYAML + BodyText via Stitch.
//  6. PGP-encrypts the stitched payload under the public key.
//  7. Enforces MaxPayloadBytes on the ciphertext.
//  8. Computes body.checksum = sha256(stitched plaintext).
//  9. Composes the header map (substrate-owned keys + schema keys).
// 10. Computes header.mac = HMAC-SHA256(pubkey, canon-json(header
//     minus header.mac)).
// 11. Encodes Trix with Magic="LTHN".
// 12. Calls Atomic.Write with the IfMatch gate.
//
//	r := w.Write(req)
//	if !r.OK { return r }
func (w *AtRestWriter[T]) Write(req WriteRequest[T]) core.Result {
	w.mu.Lock()
	defer w.mu.Unlock()

	// 1. Single-unlock assertion (#1636 fold).
	unlockedID, err := w.deps.Keys.SingleUnlockedAccount()
	if err != nil {
		return core.Fail(core.NewCode("recordfile.atrest.multi_account_ambiguous",
			core.Sprintf("at-rest write requires exactly one unlocked account: %s", err.Error())))
	}
	// 2. Account.id binding (C6 — pre-encrypt check).
	if req.AccountID != "" && req.AccountID != unlockedID {
		return core.Fail(core.NewCode("recordfile.atrest.account_id_mismatch",
			core.Sprintf("write requested for account_id %q but unlocked account is %q", req.AccountID, unlockedID)))
	}
	accountID := unlockedID

	// 3. Public key + fingerprint.
	pub, keyErr := w.deps.Keys.PublicKeyFor(accountID)
	if keyErr != nil {
		return core.Fail(core.NewCode("recordfile.atrest.public_key_unavailable",
			core.Sprintf("PublicKeyFor(%q) failed: %s", accountID, keyErr.Error())))
	}
	if len(pub) == 0 {
		return core.Fail(core.NewCode("recordfile.atrest.public_key_unavailable",
			core.Sprintf("PublicKeyFor(%q) returned empty bytes", accountID)))
	}
	fingerprint := core.SHA256Hex(pub)

	// 4. Schema validation.
	consumerHeader := w.deps.Schema.HeaderFor(req.Record)
	if consumerHeader == nil {
		consumerHeader = map[string]any{}
	}
	if vErr := validateConsumerHeader(w.deps.Schema.AllowedKeys, consumerHeader); vErr != nil {
		return core.Fail(vErr)
	}

	// 5. Stitch plaintext payload.
	plaintext := Stitch(req.BodyYAML, req.BodyText)

	// 8. Body checksum BEFORE encrypt (so checksum binds plaintext, not ciphertext).
	bodyChecksum := core.SHA256Hex(plaintext)

	// 6. PGP-encrypt.
	ciphertext, encErr := w.deps.PGP.Encrypt(pub, plaintext)
	if encErr != nil {
		return core.Fail(core.NewCode("recordfile.atrest.pgp_encrypt_failed",
			core.Sprintf("pgp encrypt failed: %s", encErr.Error())))
	}

	// 7. Payload cap (defence-in-depth at write — read also enforces).
	if len(ciphertext) > MaxPayloadBytes {
		return core.Fail(core.NewCode("recordfile.atrest.payload_cap_exceeded",
			core.Sprintf("ciphertext %d bytes exceeds MaxPayloadBytes %d", len(ciphertext), MaxPayloadBytes)))
	}

	// 9. Compose header (substrate-owned keys + schema keys).
	now := w.deps.Now()
	id := w.deps.Schema.IDFor(req.Record)
	if id == "" {
		return core.Fail(core.NewCode("recordfile.atrest.id_required",
			"HeaderSchema.IDFor returned empty id"))
	}
	version := w.deps.Schema.VersionFor(req.Record)

	hdr := map[string]any{
		"schema":  SchemaVersion,
		"id":      id,
		"version": version,
		"account": map[string]any{
			"id":              accountID,
			"key.fingerprint": fingerprint,
		},
		"surface":       string(w.deps.Schema.Surface),
		"body.checksum": bodyChecksum,
		"created_at":    now,
		"updated_at":    now,
	}
	for k, v := range consumerHeader {
		hdr[k] = v
	}

	// 10. Header MAC over canonical-json of header-without-mac.
	mac, macErr := computeHeaderMAC(hdr, pub)
	if macErr != nil {
		return core.Fail(macErr)
	}
	hdr["header.mac"] = mac

	// 11. Trix encode.
	container := &trix.Trix{
		Header:  hdr,
		Payload: ciphertext,
	}
	encoded, encErr := trix.Encode(container, MagicLTHN, nil)
	if encErr != nil {
		return core.Fail(core.NewCode("recordfile.atrest.trix_encode_failed",
			core.Sprintf("trix encode failed: %s", encErr.Error())))
	}

	// 12. Atomic write.
	writeErr := w.deps.Atomic.Write(AtomicWriteRequest{
		Path:    req.DestPath,
		Payload: encoded,
		Mode:    0o600,
		IfMatch: req.PriorHash,
	})
	if writeErr != nil {
		return core.Fail(core.NewCode("recordfile.atrest.atomic_write_failed",
			core.Sprintf("atomic write to %q failed: %s", req.DestPath, writeErr.Error())))
	}

	return core.Ok(WriteReceipt{
		ID:          id,
		Version:     version,
		AccountID:   accountID,
		Fingerprint: fingerprint,
		Surface:     w.deps.Schema.Surface,
		EncodedSize: int64(len(encoded)),
	})
}

// WriteReceipt is returned (wrapped in core.Ok) from a successful
// Write. Consumers use it to feed audit events (`auth.account.atrest.
// written`) — Body sizes are intentionally absent per ADD-LOW-1.
type WriteReceipt struct {
	ID          string
	Version     int64
	AccountID   string
	Fingerprint string
	Surface     Surface
	EncodedSize int64 // total on-disk Trix size; consumer audit choice (not sent to AuditEvent.meta)
}

// --- Read path -------------------------------------------------------

// ReadResult is returned (wrapped in core.Ok) from a successful Read.
// Body* fields hold the post-decrypt, post-Split plaintext for the
// consumer to yaml.Unmarshal as needed.
type ReadResult struct {
	Header   Header
	BodyYAML []byte
	BodyText []byte
}

// Header is the decoded at-rest header view. Consumer-visible fields
// mirror the §2.4 schema; the underlying map is preserved for
// per-surface field lookup.
type Header struct {
	Schema       string
	ID           string
	Version      int64
	AccountID    string
	Fingerprint  string
	Surface      Surface
	BodyChecksum string
	HeaderMAC    string
	CreatedAt    int64
	UpdatedAt    int64
	Raw          map[string]any // every header field including allowlisted keys
}

// Read decodes + decrypts a record file. The substrate:
//
//  1. Asserts SingleUnlockedAccount() — multi-unlock returns
//     typed reject BEFORE any disk read (#1636).
//  2. Reads + Trix-decodes the file (Magic="LTHN" check, header
//     allocation cap inherited from trix.MaxHeaderSize).
//  3. Parses + validates the header structural keys (schema present
//     + matches SchemaVersion, surface matches deps.Surface).
//  4. Resolves public key for the unlocked account and verifies
//     header.mac BEFORE any decrypt work (header MAC defence; T1).
//  5. Pre-checks fingerprint header vs computed fingerprint (T3 —
//     cheap reject before PGP decrypt).
//  6. Enforces MaxPayloadBytes on the ciphertext (T5 cap).
//  7. Acquires PrivateKeyHandle + decrypts inside handle.Use(...).
//  8. Verifies body.checksum vs sha256(plaintext) (defence-in-depth).
//  9. Splits the plaintext back into frontmatter + body.
//
//	r := w.Read(path)
//	if r.OK { res := r.Value.(recordfile.ReadResult) }
func (w *AtRestWriter[T]) Read(path string) core.Result {
	w.mu.Lock()
	defer w.mu.Unlock()

	// 1. Single-unlock assertion BEFORE any disk read.
	unlockedID, err := w.deps.Keys.SingleUnlockedAccount()
	if err != nil {
		return core.Fail(core.NewCode("recordfile.atrest.multi_account_ambiguous",
			core.Sprintf("at-rest read requires exactly one unlocked account: %s", err.Error())))
	}

	// 2. Read + Trix decode.
	raw, readErr := w.deps.Atomic.ReadFile(path)
	if readErr != nil {
		return core.Fail(core.NewCode("recordfile.atrest.read_failed",
			core.Sprintf("read %q: %s", path, readErr.Error())))
	}
	container, hdr, decErr := decodeTrix(raw)
	if decErr != nil {
		return core.Fail(decErr)
	}

	// 3. Header structural validation (schema + surface).
	if err := validateHeaderStructure(hdr, w.deps.Schema.Surface); err != nil {
		return core.Fail(err)
	}

	// 4. Public key + header MAC verify (no unlock required for MAC).
	pub, keyErr := w.deps.Keys.PublicKeyFor(unlockedID)
	if keyErr != nil {
		return core.Fail(core.NewCode("recordfile.atrest.public_key_unavailable",
			core.Sprintf("PublicKeyFor(%q) failed: %s", unlockedID, keyErr.Error())))
	}
	if len(pub) == 0 {
		return core.Fail(core.NewCode("recordfile.atrest.public_key_unavailable",
			core.Sprintf("PublicKeyFor(%q) returned empty bytes", unlockedID)))
	}
	if vErr := verifyHeaderMAC(hdr, pub); vErr != nil {
		return core.Fail(vErr)
	}

	// 5. Fingerprint pre-check (T3 — cheap reject before decrypt).
	wantFingerprint, _ := hdr.Raw["account"].(map[string]any)["key.fingerprint"].(string)
	gotFingerprint := core.SHA256Hex(pub)
	if wantFingerprint != gotFingerprint {
		return core.Fail(core.NewCode("recordfile.atrest.fingerprint_mismatch",
			core.Sprintf("header fingerprint %q does not match unlocked account fingerprint %q", wantFingerprint, gotFingerprint)))
	}

	// Account.id misdelivery check (header → unlocked).
	if hdr.AccountID != unlockedID {
		return core.Fail(core.NewCode("recordfile.atrest.account_id_misdelivered",
			core.Sprintf("header account.id %q does not match unlocked account %q", hdr.AccountID, unlockedID)))
	}

	// 6. Payload cap (defence-in-depth at read — write also enforces).
	if len(container.Payload) > MaxPayloadBytes {
		return core.Fail(core.NewCode("recordfile.atrest.payload_cap_exceeded",
			core.Sprintf("ciphertext %d bytes exceeds MaxPayloadBytes %d", len(container.Payload), MaxPayloadBytes)))
	}

	// 7. Decrypt inside handle.Use(...).
	handle, ok := w.deps.Keys.PrivateKeyFor(unlockedID)
	if !ok {
		return core.Fail(core.NewCode("recordfile.atrest.session_locked",
			core.Sprintf("no unlocked private key for account %q", unlockedID)))
	}
	var plaintext []byte
	useErr := handle.Use(func(priv []byte) error {
		decrypted, dErr := w.deps.PGP.Decrypt(priv, container.Payload)
		if dErr != nil {
			return core.NewCode("recordfile.atrest.pgp_decrypt_failed",
				core.Sprintf("pgp decrypt failed: %s", dErr.Error()))
		}
		// Copy so the slice survives handle zeroisation. The decrypted
		// slice is owned by pgp; copy defends against any future
		// impl that aliases priv-derived buffers.
		plaintext = make([]byte, len(decrypted))
		copy(plaintext, decrypted)
		return nil
	})
	if useErr != nil {
		return core.Fail(useErr)
	}

	// 8. Body checksum verify.
	gotChecksum := core.SHA256Hex(plaintext)
	if gotChecksum != hdr.BodyChecksum {
		return core.Fail(core.NewCode("recordfile.atrest.body_checksum_invalid",
			core.Sprintf("decrypted body checksum %q does not match header %q", gotChecksum, hdr.BodyChecksum)))
	}

	// 9. Split into frontmatter + body for the consumer.
	fm, body := Split(plaintext)
	// Defensive copies — Split aliases into plaintext; callers may
	// hold these slices across goroutine boundaries.
	fmCopy := make([]byte, len(fm))
	copy(fmCopy, fm)
	bodyCopy := make([]byte, len(body))
	copy(bodyCopy, body)

	return core.Ok(ReadResult{
		Header:   hdr,
		BodyYAML: fmCopy,
		BodyText: bodyCopy,
	})
}

// --- List path -------------------------------------------------------

// ListEntry is one element of List's result: the parsed header view
// for a single file. Body fields are absent — List is the
// no-unlock-required list-view path per RFC §4.1.
type ListEntry struct {
	Path   string
	Header Header
}

// ListSkip is returned (via core.Result with OK=false + Code) inside
// the per-file iteration when a single entry fails Trix decode or
// MAC verify — caller iterates and decides per RFC §4.1 "MAC-failure
// entries SKIPPED + audit-emitted (do NOT abort whole List on one
// bad file)".
//
// Codes a caller maps onto SKIP behaviour:
//
//   - "recordfile.atrest.magic_mismatch"
//   - "recordfile.atrest.version_mismatch"
//   - "recordfile.atrest.schema_missing"
//   - "recordfile.atrest.schema_unsupported"
//   - "recordfile.atrest.surface_mismatch"
//   - "recordfile.atrest.header_mac_invalid"
//   - "recordfile.atrest.fingerprint_mismatch"

// DecodeHeader parses + MAC-verifies a single at-rest file's HEADER
// only — no decrypt. The intended call-shape for Service.List
// iteration. Public key is supplied directly so the caller controls
// the unlock-state semantics (List MUST work while LOCKED — RFC
// §4.1; the caller can read PublicKeyFor at iteration time without
// requiring SingleUnlockedAccount, useful for read-only header
// extraction during a multi-account migration sweep).
//
// Returns a core.Result wrapping Header on success; on failure the
// Result carries a typed error whose Code names the failure reason
// the caller should fold into auth.account.atrest.read_failed.
//
//	r := w.DecodeHeader(raw, pubKey)
//	if !r.OK { audit.Emit("auth.account.atrest.read_failed", "reason", r.Code()); continue }
func (w *AtRestWriter[T]) DecodeHeader(raw []byte, pubKey []byte) core.Result {
	_, hdr, decErr := decodeTrix(raw)
	if decErr != nil {
		return core.Fail(decErr)
	}
	if err := validateHeaderStructure(hdr, w.deps.Schema.Surface); err != nil {
		return core.Fail(err)
	}
	if len(pubKey) == 0 {
		return core.Fail(core.NewCode("recordfile.atrest.public_key_unavailable",
			"DecodeHeader requires non-empty pubKey"))
	}
	if vErr := verifyHeaderMAC(hdr, pubKey); vErr != nil {
		return core.Fail(vErr)
	}
	wantFingerprint, _ := hdr.Raw["account"].(map[string]any)["key.fingerprint"].(string)
	gotFingerprint := core.SHA256Hex(pubKey)
	if wantFingerprint != gotFingerprint {
		return core.Fail(core.NewCode("recordfile.atrest.fingerprint_mismatch",
			core.Sprintf("header fingerprint %q does not match supplied pub key fingerprint %q", wantFingerprint, gotFingerprint)))
	}
	return core.Ok(hdr)
}

// --- Internal helpers ------------------------------------------------

// decodeTrix wraps trix.Decode and surfaces every failure as a typed
// core error with a stable Code. The Code values are the contract the
// audit-event spec at §8 binds against.
func decodeTrix(raw []byte) (*trix.Trix, Header, error) {
	container, err := trix.Decode(raw, MagicLTHN, nil)
	if err != nil {
		switch {
		case err == trix.ErrInvalidMagicNumber || bytes.Contains([]byte(err.Error()), []byte("invalid magic")):
			return nil, Header{}, core.NewCode("recordfile.atrest.magic_mismatch",
				core.Sprintf("trix magic mismatch: %s", err.Error()))
		case err == trix.ErrInvalidVersion:
			return nil, Header{}, core.NewCode("recordfile.atrest.version_mismatch",
				core.Sprintf("trix version mismatch: %s", err.Error()))
		case err == trix.ErrHeaderTooLarge:
			return nil, Header{}, core.NewCode("recordfile.atrest.header_oversize",
				core.Sprintf("trix header exceeds %d bytes", trix.MaxHeaderSize))
		default:
			return nil, Header{}, core.NewCode("recordfile.atrest.trix_decode_failed",
				core.Sprintf("trix decode failed: %s", err.Error()))
		}
	}
	hdr, parseErr := parseHeader(container.Header)
	if parseErr != nil {
		return nil, Header{}, parseErr
	}
	return container, hdr, nil
}

// parseHeader extracts the structural keys per §2.4. Missing fields
// return typed errors whose Codes the audit-event spec binds to the
// `reason` meta field.
func parseHeader(raw map[string]any) (Header, error) {
	hdr := Header{Raw: raw}
	schemaV, ok := raw["schema"].(string)
	if !ok || schemaV == "" {
		return Header{}, core.NewCode("recordfile.atrest.schema_missing",
			"header missing required `schema` field")
	}
	hdr.Schema = schemaV
	if id, ok := raw["id"].(string); ok {
		hdr.ID = id
	}
	if v, ok := raw["version"].(float64); ok {
		// JSON unmarshal lands integers as float64; convert.
		hdr.Version = int64(v)
	} else if v, ok := raw["version"].(int64); ok {
		hdr.Version = v
	}
	if acct, ok := raw["account"].(map[string]any); ok {
		if aid, ok := acct["id"].(string); ok {
			hdr.AccountID = aid
		}
		if fp, ok := acct["key.fingerprint"].(string); ok {
			hdr.Fingerprint = fp
		}
	}
	if s, ok := raw["surface"].(string); ok {
		hdr.Surface = Surface(s)
	}
	if c, ok := raw["body.checksum"].(string); ok {
		hdr.BodyChecksum = c
	}
	if m, ok := raw["header.mac"].(string); ok {
		hdr.HeaderMAC = m
	}
	if v, ok := raw["created_at"].(float64); ok {
		hdr.CreatedAt = int64(v)
	} else if v, ok := raw["created_at"].(int64); ok {
		hdr.CreatedAt = v
	}
	if v, ok := raw["updated_at"].(float64); ok {
		hdr.UpdatedAt = int64(v)
	} else if v, ok := raw["updated_at"].(int64); ok {
		hdr.UpdatedAt = v
	}
	return hdr, nil
}

// validateHeaderStructure enforces the schema-version + surface
// invariants on a parsed Header. Called by both Read and DecodeHeader.
func validateHeaderStructure(hdr Header, want Surface) error {
	if hdr.Schema == "" {
		return core.NewCode("recordfile.atrest.schema_missing",
			"header `schema` field is empty")
	}
	if hdr.Schema != SchemaVersion {
		return core.NewCode("recordfile.atrest.schema_unsupported",
			core.Sprintf("header schema %q does not match supported %q", hdr.Schema, SchemaVersion))
	}
	if hdr.Surface == "" {
		return core.NewCode("recordfile.atrest.surface_missing",
			"header `surface` field is empty")
	}
	if want != "" && hdr.Surface != want {
		return core.NewCode("recordfile.atrest.surface_mismatch",
			core.Sprintf("header surface %q does not match writer surface %q", hdr.Surface, want))
	}
	if hdr.BodyChecksum == "" {
		return core.NewCode("recordfile.atrest.body_checksum_missing",
			"header `body.checksum` field is empty")
	}
	if hdr.HeaderMAC == "" {
		return core.NewCode("recordfile.atrest.header_mac_missing",
			"header `header.mac` field is empty")
	}
	if hdr.AccountID == "" {
		return core.NewCode("recordfile.atrest.account_id_missing",
			"header `account.id` field is empty")
	}
	if hdr.Fingerprint == "" {
		return core.NewCode("recordfile.atrest.fingerprint_missing",
			"header `account.key.fingerprint` field is empty")
	}
	return nil
}

// computeHeaderMAC strips the header.mac field, canonicalises the
// remainder per RFC 8785 JCS, and returns hex(HMAC-SHA256(pubKey,
// canonical-bytes)).
//
// Safe to call before the MAC has been assigned (the function works
// off a copy of hdr that excludes "header.mac" regardless of whether
// the source map already has it).
func computeHeaderMAC(hdr map[string]any, pubKey []byte) (string, error) {
	stripped := make(map[string]any, len(hdr))
	for k, v := range hdr {
		if k == "header.mac" {
			continue
		}
		stripped[k] = v
	}
	canon := CanonicalJSON(stripped)
	if !canon.OK {
		if e, ok := canon.Value.(error); ok {
			return "", e
		}
		return "", core.NewCode("recordfile.atrest.canonjson_failed",
			"canonical-json encode returned non-error failure")
	}
	macR := core.HMAC("sha256", pubKey, canon.Value.([]byte))
	if !macR.OK {
		return "", core.NewCode("recordfile.atrest.hmac_failed",
			core.Sprintf("HMAC compute failed: %s", macR.Error()))
	}
	return core.HexEncode(macR.Value.([]byte)), nil
}

// verifyHeaderMAC recomputes the header MAC over the parsed Raw map
// (sans header.mac) and constant-time compares against hdr.HeaderMAC.
func verifyHeaderMAC(hdr Header, pubKey []byte) error {
	got, err := computeHeaderMAC(hdr.Raw, pubKey)
	if err != nil {
		return err
	}
	if !constantTimeEqualString(got, hdr.HeaderMAC) {
		return core.NewCode("recordfile.atrest.header_mac_invalid",
			"header MAC verification failed")
	}
	return nil
}

// constantTimeEqualString avoids leaking MAC-length-prefix timing
// against a partial attacker. Both args are hex strings so the
// underlying byte compare is a fine substitute for crypto/subtle.
func constantTimeEqualString(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	var diff byte
	for i := 0; i < len(a); i++ {
		diff |= a[i] ^ b[i]
	}
	return diff == 0
}

// --- F-4 PeekAccountID (Cerberus #44 PRBW extract) -------------------

// PeekAccountID extracts the account.id from a raw at-rest blob
// without going through PGP decrypt. The on-disk shape is
// `[Magic(4)][Version(1)][HeaderLen(4)][HeaderJSON][Payload]` per RFC
// §2.2; we read the JSON header bytes and unmarshal just the account
// substructure.
//
// Returns an error when the blob is too short, the magic/length is
// nonsense, or account.id is absent. The substrate's full DecodeHeader
// path runs after this — so structural errors here are not
// authoritative, they're just enough to pick the correct pub key for
// the header MAC verification.
//
// Extracted from the wave 1/wave 2 per-consumer copies (sales/deals,
// incidents, runbooks) per Cerberus #44 PRBW F-3 to eliminate 3x
// duplication before wave 3 (marketing 4 sub-pkgs) lands. Error codes
// use the `recordfile.atrest.peek.*` scope so consumer-side typed-
// error matching surfaces the substrate boundary cleanly.
//
// Usage example:
//
//	accountID, err := recordfile.PeekAccountID(rawBytes)
//	if err != nil { return ... }
//	pub, ok := keys.PublicKeyFor(accountID)
func PeekAccountID(raw []byte) (string, error) {
	const headerStart = 4 + 1 + 4 // Magic + Version + HeaderLen
	if len(raw) < headerStart {
		return "", core.NewCode("recordfile.atrest.peek.blob_too_short",
			"blob too short")
	}
	hdrLen := int(uint32(raw[5])<<24 | uint32(raw[6])<<16 | uint32(raw[7])<<8 | uint32(raw[8]))
	if hdrLen <= 0 || headerStart+hdrLen > len(raw) {
		return "", core.NewCode("recordfile.atrest.peek.header_len_oob",
			"header length out of bounds")
	}
	hdrJSON := raw[headerStart : headerStart+hdrLen]
	var probe struct {
		Account struct {
			ID string `json:"id"`
		} `json:"account"`
	}
	if r := core.JSONUnmarshalString(string(hdrJSON), &probe); !r.OK {
		return "", core.NewCode("recordfile.atrest.peek.json_unmarshal",
			"json unmarshal: "+r.Error())
	}
	if probe.Account.ID == "" {
		return "", core.NewCode("recordfile.atrest.peek.account_id_missing",
			"account.id missing from header")
	}
	return probe.Account.ID, nil
}

// SPDX-Licence-Identifier: EUPL-1.2

// Package keys is the encrypted-at-rest store for the desktop's
// secrets — partitioned into two cryptographic tiers per
// RFC.stage-e-keys-partition v3 (Mantis #1625):
//
//   - Tier-0 (pre-unlock): refs that MUST be readable at process
//     boot, BEFORE any human input. Today that's the
//     single-instance Wails IPC authenticator. Sealed under the
//     tier-0 master at ~/Lethean/data/keys/.master-tier0; the
//     tier-0 master is KEK-wrapped under an HKDF derivation over
//     ~/Lethean/wallets/.seed ("lthn-keys-kek" / "tier0-v1") that
//     is available pre-unlock per the auth-gate v1 bootstrap.
//   - Tier-1 (post-unlock): long-lived bearer credentials (OpenAI
//     keys, Anthropic keys, future provider creds). Sealed under
//     the tier-1 master at ~/Lethean/data/keys/.master-tier1; the
//     tier-1 master is KEK-wrapped under HKDF over the unlocked
//     LetheanAccount PGP private key. Reads/writes require the
//     tier-1 KEK provider to be live (account unlocked).
//
// Tier confusion is impossible at the filesystem layer — blobs
// carry a tier suffix (".t0.aead" / ".t1.aead"), so a tier-0 file
// is a different file from its tier-1 namesake and never read by
// the wrong-tier method. The keyPath validator rejects user refs
// matching `\.t[0-9]\.aead$` to prevent callers from constructing
// refs that would collide with the on-disk suffix scheme.
//
// Per design_no_hidden_user_bloat — keys/ sits visible under the
// user's $HOME/Lethean/ tree, NOT in ~/.config or ~/Library. The
// master files are dot-prefixed so they're hidden from default
// Finder views but still discoverable via ls -A.
//
// Usage from another package — tier-1 (post-unlock provider creds):
//
//	svc, r := keys.New()
//	if !r.OK { return r }
//	if r := svc.PutTier1("openai-default", []byte("sk-...")); !r.OK { return r }
//	r := svc.GetTier1("openai-default")
//	if r.OK { plaintext := r.Value.([]byte); _ = plaintext }
//
// Usage from another package — tier-0 (pre-unlock single-instance key):
//
//	r := svc.SingleInstanceKey()
//	if r.OK { key := r.Value.([32]byte); _ = key }
//
// Wails binding registers as the "Keys" service so the TS side can
// reach it via @desktop/keys/service — only tier-1 W-methods are
// exposed (frontend has no legitimate tier-0 surface):
//
//	import { WPutTier1, WListTier1, WDeleteTier1 } from "@desktop/keys/service";
//	await WPutTier1("openai-default", "sk-...");
//
// IMPORTANT: GetTier1 is NOT exposed to the Wails surface —
// plaintext secrets must not cross the WebView boundary. Reading
// the key is a Go-side action only (e.g. when the runner spawns
// an agent). Frontend code only writes and lists.

package keys

import (
	core "dappco.re/go"
	"dappco.re/go/io/sigil"
	"dappco.re/lthn/desktop/pkg/paths"
)

const (
	// legacyMasterFileName is the pre-partition master file. Read on
	// boot for legacy-install detection (RFC §4.4 Step 0b); deleted
	// only after the tier-1 migration completes successfully.
	legacyMasterFileName = ".master"

	// masterTier0FileName / masterTier1FileName are the per-tier
	// KEK-wrapped masters. Tier-0 is wrapped under the .seed-derived
	// KEK (available pre-unlock); tier-1 is wrapped under the
	// unlocked-account KEK per Mantis #1624.
	masterTier0FileName = ".master-tier0"
	masterTier1FileName = ".master-tier1"

	// legacyBlobSuffix is the pre-partition blob suffix. Read on
	// boot for legacy-install detection; never written by
	// post-migration code.
	legacyBlobSuffix = ".aead"

	// tier0BlobSuffix / tier1BlobSuffix encode the tier of a blob in
	// its filename (RFC §4.1, Q1-B). Tier confusion is impossible at
	// the filesystem layer.
	tier0BlobSuffix = ".t0.aead"
	tier1BlobSuffix = ".t1.aead"

	// masterKeySize is the ChaCha20-Poly1305 key length.
	masterKeySize = 32

	// dirMode + fileMode — 0700 for the directory, 0600 for files.
	// Owner-only. The master files' modes are load-bearing for the
	// at-rest protection story.
	dirMode  = 0o700
	fileMode = 0o600
)

// Tier identifies which cryptographic partition a method targets.
// Internal — callers reach a tier via the explicit per-tier
// methods (PutTier0/PutTier1/...) rather than this enum.
type tier int

const (
	tier0 tier = 0
	tier1 tier = 1
)

// KEKProvider returns the 32-byte key-encryption key (KEK) that
// wraps the on-disk master for a tier, plus a boolean reporting
// whether the KEK is currently available. The same shape services
// both tiers (RFC §4.2):
//
//   - Tier-0 provider derives via HKDF(.seed, "lthn-keys-kek",
//     "tier0-v1", 32) — available pre-unlock; .seed is bootstrap
//     substrate per the auth-gate v1.
//   - Tier-1 provider derives via HKDF over the unlocked
//     LetheanAccount PGP private key — returns (_, false)
//     pre-unlock so tier-1 reads/writes fail cleanly until the
//     account is unlocked.
//
// The keys Service treats the returned bytes as opaque and does
// not retain a reference beyond the call; providers are expected
// to return a fresh slice every invocation.
//
// Mantis #1624 (KEK gate) — original definition.
// Mantis #1625 (tier partition) — extended to two tiers via
// SetKEKProviderTier0 / SetKEKProvider.
type KEKProvider func() ([]byte, bool)

// Service owns the encrypted keys directory. Stateless beyond the
// per-tier master-key caches; the disk is the source of truth.
type Service struct {
	// mu guards the per-tier master caches AND the migration
	// sequence. Held by ensureMaster and the migration steps so
	// concurrent readers/writers cannot observe a partial state.
	mu core.RWMutex
	// getOrCreateMu serialises the miss→generate→put window in
	// GetOrCreateTier0/Tier1 so two concurrent callers don't both
	// generate distinct keys for the same ref. Separate from `mu`
	// because PutTierN internally takes `mu` via ensureMaster;
	// recursive Lock would deadlock. Cerberus pass-6 MEDIUM.
	getOrCreateMu core.Mutex

	// tier0Master / tier1Master are the 32-byte unwrapped masters
	// for each tier. Loaded lazily on first per-tier operation.
	tier0Master []byte
	tier1Master []byte

	// kekProviderMu guards the per-tier provider slots so
	// SetKEKProvider / SetKEKProviderTier0 can race safely against
	// ensureMaster on a concurrent boot path. Held briefly under
	// the slow-path of ensureMaster to snapshot the provider
	// pointer.
	kekProviderMu core.RWMutex
	// tier0KEKProvider derives the tier-0 KEK from .seed (wired in
	// cmd/lthn/app.go before desktop.Run). Nil before
	// SetKEKProviderTier0 fires; ensureMaster(tier0) Fails with a
	// clear error rather than falling back to a raw-on-disk path
	// (RFC §4.2 — tier-0 has no raw mode).
	tier0KEKProvider KEKProvider
	// tier1KEKProvider derives the tier-1 KEK from the unlocked
	// account PGP key (wired in cmd/lthn/app.go after the account
	// service exists). Nil pre-unlock — ensureMaster(tier1) Fails
	// with keys.no_tier1_provider so callers cannot accidentally
	// read provider secrets while the account is locked.
	tier1KEKProvider KEKProvider

	// migrationComplete latches once the tier-1 migration has run
	// (or has been determined unnecessary). Subsequent
	// SetKEKProvider invocations skip the migration sub-sequence.
	// Guarded by mu.
	migrationComplete bool
}

const (
	// KEKHKDFSalt + KEKHKDFInfo are the canonical HKDF parameters
	// the wiring closure in cmd/lthn/app.go passes to core.HKDF
	// when deriving the TIER-1 KEK from the unlocked PGP private
	// key. Exported as constants so the call-site reads as one
	// declaration and the salt/info pair stays grep-able from a
	// single source. (Tier-0 uses the same salt with a different
	// info string "tier0-v1" — RFC §4.2 — to keep the derivation
	// namespaces distinct.)
	//
	// Usage example (in cmd/lthn/app.go's tier-1 provider closure):
	//
	//	kek := core.HKDF("sha256", priv,
	//	    []byte(keys.KEKHKDFSalt), []byte(keys.KEKHKDFInfo), 32).Value.([]byte)
	KEKHKDFSalt = "lthn-keys-kek"
	KEKHKDFInfo = "v1"

	// KEKHKDFInfoTier0 is the HKDF info-string the tier-0 KEK
	// provider closure in cmd/lthn/app.go (E.K.C lane) uses when
	// deriving the tier-0 KEK from ~/Lethean/wallets/.seed. Distinct
	// from KEKHKDFInfo so the same .seed bytes can produce a tier-0
	// KEK alongside the server-key passphrase (which uses its own
	// info string) without collision.
	//
	// Usage example (in cmd/lthn/app.go's tier-0 provider closure):
	//
	//	seedR := core.ReadFile(core.PathJoin(walletsDir, ".seed"))
	//	kek := core.HKDF("sha256", seedR.Value.([]byte),
	//	    []byte(keys.KEKHKDFSalt), []byte(keys.KEKHKDFInfoTier0), 32).Value.([]byte)
	KEKHKDFInfoTier0 = "tier0-v1"

	// kekWrappedMasterLen is the on-disk length of a KEK-wrapped
	// master: 24-byte XChaCha20-Poly1305 nonce + 32-byte ciphertext
	// + 16-byte Poly1305 tag = 72 bytes. (sigil.NewChaChaPolySigil
	// defaults to the XChaCha20 variant.) Used as the length
	// sentinel that distinguishes a legacy raw 32-byte master from
	// a KEK-wrapped envelope on read — no version field needed
	// because the two lengths can never collide.
	// ADD-K1 — single declaration reused for both tiers.
	kekWrappedMasterLen = 24 + masterKeySize + 16
)

// New constructs a Service and ensures $HOME/Lethean/data/keys/
// exists. Per-tier masters are loaded lazily on the first
// per-tier op.
//
// Cerberus Mantis #1441 — asserts dir mode is 0o700 + any
// existing master files are 0o600 at construction time.
//
// Usage example:
//
//	r := keys.New()
//	if r.OK { svc := r.Value.(*keys.Service); _ = svc }
func New() core.Result {
	dirR := paths.KeysDir()
	if !dirR.OK {
		return dirR
	}
	dir := dirR.Value.(string)
	assertKeysDirMode(dir)
	return core.Ok(&Service{})
}

// assertKeysDirMode is the Mantis #1441 startup check. Stats the
// keys dir + every per-tier master file; logs a core.Warn for any
// mode that's wider than the expected 0o700 / 0o600. Best-effort —
// Stat failure is silent (KeysDir would have caught a real I/O
// error).
func assertKeysDirMode(dir string) {
	statR := core.Stat(dir)
	if !statR.OK {
		return
	}
	info, ok := statR.Value.(core.FsFileInfo)
	if !ok {
		return
	}
	gotPerm := info.Mode().Perm()
	if gotPerm != dirMode {
		core.Warn("keys: directory mode wider than 0o700 — "+
			"someone (or full-disk tooling) loosened it; "+
			"`chmod 700 ~/Lethean/data/keys` to restore (Mantis #1441)",
			"got", core.Sprintf("%o", gotPerm),
			"want", core.Sprintf("%o", dirMode))
	}
	for _, name := range []string{legacyMasterFileName, masterTier0FileName, masterTier1FileName} {
		masterPath := core.PathJoin(dir, name)
		mStatR := core.Stat(masterPath)
		if !mStatR.OK {
			continue
		}
		mInfo, ok := mStatR.Value.(core.FsFileInfo)
		if !ok {
			continue
		}
		if mPerm := mInfo.Mode().Perm(); mPerm != fileMode {
			core.Warn("keys: master file mode wider than 0o600 — "+
				"`chmod 600 ~/Lethean/data/keys/"+name+"` to restore (Mantis #1441)",
				"file", name,
				"got", core.Sprintf("%o", mPerm),
				"want", core.Sprintf("%o", fileMode))
		}
	}
}

// Register adopts the canonical Service shape (Mantis #1336).
// Constructs a Service and registers it on Core under "keys".
//
// Usage example:
//
//	if r := keys.Register(c); !r.OK { return r }
func Register(c *core.Core) core.Result {
	core.Warn("keys: tier-partition substrate active (Mantis #1625) — " +
		"tier-0 from .seed; tier-1 from unlocked account PGP key")
	return New()
}

// SetKEKProviderTier0 wires the tier-0 KEK source (RFC §4.2). The
// provider is expected to derive a 32-byte KEK via HKDF over
// ~/Lethean/wallets/.seed using KEKHKDFSalt + KEKHKDFInfoTier0;
// because .seed is auto-generated pre-unlock by the auth-gate
// bootstrap, this provider is normally live from process start
// (wired in cmd/lthn/app.go before desktop.Run).
//
// May be called more than once; the most recent non-nil provider
// wins. Passing nil disables tier-0 access (test fixtures only —
// production wiring is always non-nil after E.K.C lands).
//
// Mantis #1625 — additive over SetKEKProvider (tier-1 wire) so
// the two providers can be wired independently at boot.
//
// Usage example (in cmd/lthn/app.go before desktop.Run):
//
//	walletsR := paths.WalletsDir()
//	if !walletsR.OK { return walletsR }
//	seedPath := core.PathJoin(walletsR.Value.(string), ".seed")
//	keysSvc.SetKEKProviderTier0(func() ([]byte, bool) {
//	    seedR := core.ReadFile(seedPath)
//	    if !seedR.OK { return nil, false }
//	    kekR := core.HKDF("sha256", seedR.Value.([]byte),
//	        []byte(keys.KEKHKDFSalt), []byte(keys.KEKHKDFInfoTier0), 32)
//	    if !kekR.OK { return nil, false }
//	    return kekR.Value.([]byte), true
//	})
func (s *Service) SetKEKProviderTier0(provider KEKProvider) {
	s.kekProviderMu.Lock()
	s.tier0KEKProvider = provider
	// Invalidate the cached tier-0 master so the next operation
	// re-reads from disk under the new provider.
	s.mu.Lock()
	s.tier0Master = nil
	s.mu.Unlock()
	s.kekProviderMu.Unlock()
}

// SetKEKProvider wires the tier-1 KEK source — the post-unlock
// KEK derived from the active account's PGP private key. Wired in
// cmd/lthn/app.go after both the keys + account services have
// been constructed.
//
// Mantis #1624 — original definition.
// Mantis #1625 — clarified as the TIER-1 wire (tier-0 has its own
// SetKEKProviderTier0). Behaviour unchanged for callers.
//
// When the provider returns (kek, true) the tier-1 master is
// treated as a KEK-wrapped envelope. When the provider returns
// (_, false) tier-1 reads/writes fail cleanly with
// keys.no_tier1_provider — the legacy raw-fallback path is gone
// with the A2 partition cutover (RFC §4.3).
//
// The first non-nil wire ALSO triggers the lazy tier-1 migration
// from a pre-partition install (RFC §4.4 Step 1 onwards) — under
// s.mu, atomic with the first read/write that follows.
//
// Usage example (in cmd/lthn/app.go after services are wired):
//
//	keysSvc.SetKEKProvider(func() ([]byte, bool) {
//	    if !accountSvc.HasUnlocked(activeID) { return nil, false }
//	    handle, ok := accountSvc.PrivateKeyFor(activeID)
//	    if !ok { return nil, false }
//	    var kek []byte
//	    _ = handle.Use(func(priv []byte) error {
//	        kek = core.HKDF("sha256", priv,
//	            []byte(keys.KEKHKDFSalt), []byte(keys.KEKHKDFInfo), 32).Value.([]byte)
//	        return nil
//	    })
//	    if len(kek) != 32 { return nil, false }
//	    return kek, true
//	})
func (s *Service) SetKEKProvider(provider KEKProvider) {
	s.kekProviderMu.Lock()
	s.tier1KEKProvider = provider
	// Invalidate the cached tier-1 master so the next operation
	// re-reads from disk under the new provider — keeps the wire
	// observably atomic from the consumer POV (matches the #1624
	// invalidate-on-wire shape).
	s.mu.Lock()
	s.tier1Master = nil
	s.kekProviderMu.Unlock()
	// Tier-1 migration from legacy install — fires once per
	// Service instance after the first non-nil wire. Held under
	// s.mu so concurrent ensureMaster(tier1) callers see the
	// migrated state.
	if provider != nil && !s.migrationComplete {
		s.migrateTier1Locked()
	}
	s.mu.Unlock()
}

// currentKEKFor snapshots the wired provider for tier t and
// invokes it once. Returns (nil, false) when no provider is
// wired, the provider returns false, or the returned KEK is not
// 32 bytes (defensive — a misbehaving provider must not crash
// the keys path).
//
// Called under s.mu held by ensureMaster's slow path; the
// provider itself MUST NOT call back into keys.Service to avoid
// deadlock. Production wiring in app.go satisfies this — the
// providers read from pkg/serverkey (.seed for tier-0) or
// pkg/account (unlocked private key for tier-1) only.
func (s *Service) currentKEKFor(t tier) ([]byte, bool) {
	s.kekProviderMu.RLock()
	var provider KEKProvider
	switch t {
	case tier0:
		provider = s.tier0KEKProvider
	case tier1:
		provider = s.tier1KEKProvider
	}
	s.kekProviderMu.RUnlock()
	if provider == nil {
		return nil, false
	}
	kek, ok := provider()
	if !ok {
		return nil, false
	}
	if len(kek) != masterKeySize {
		core.Warn("keys: KEK provider returned wrong-length key",
			"tier", core.Sprintf("%d", int(t)),
			"got", core.Sprintf("%d", len(kek)),
			"want", core.Sprintf("%d", masterKeySize))
		return nil, false
	}
	return kek, true
}

// masterFileNameFor returns the per-tier master filename.
func masterFileNameFor(t tier) string {
	if t == tier0 {
		return masterTier0FileName
	}
	return masterTier1FileName
}

// blobSuffixFor returns the per-tier blob suffix.
func blobSuffixFor(t tier) string {
	if t == tier0 {
		return tier0BlobSuffix
	}
	return tier1BlobSuffix
}

// ensureMaster loads or generates the master for tier t. Idempotent;
// repeated calls return the cached master. Tier-0 may also fire
// the Step 0b boot migration sub-sequence (RFC §4.4 Step 0b) on
// legacy installs where a 32B raw .master file is present.
//
// Read/write paths (RFC §4.2 — no raw-on-disk for either tier):
//
//   - Tier-0: requires tier-0 KEK provider live. On-disk format is
//     always 72-byte KEK-wrapped (.master-tier0). Step 0b legacy
//     branch handles the one-shot migration from pre-partition
//     installs.
//   - Tier-1: requires tier-1 KEK provider live. On-disk format is
//     72-byte KEK-wrapped (.master-tier1). Pre-partition installs
//     migrate via SetKEKProvider → migrateTier1Locked (RFC §4.4
//     Steps 3a-3h).
func (s *Service) ensureMaster(t tier) core.Result {
	s.mu.RLock()
	if t == tier0 && len(s.tier0Master) == masterKeySize {
		master := s.tier0Master
		s.mu.RUnlock()
		return core.Ok(master)
	}
	if t == tier1 && len(s.tier1Master) == masterKeySize {
		master := s.tier1Master
		s.mu.RUnlock()
		return core.Ok(master)
	}
	s.mu.RUnlock()

	s.mu.Lock()
	defer s.mu.Unlock()

	if t == tier0 && len(s.tier0Master) == masterKeySize {
		return core.Ok(s.tier0Master)
	}
	if t == tier1 && len(s.tier1Master) == masterKeySize {
		return core.Ok(s.tier1Master)
	}

	dirR := paths.KeysDir()
	if !dirR.OK {
		return dirR
	}
	dir := dirR.Value.(string)

	if t == tier0 {
		return s.ensureTier0Locked(dir)
	}
	return s.ensureTier1Locked(dir)
}

// ensureTier0Locked is the tier-0 master load/generate/migrate
// path. Caller MUST hold s.mu. Implements RFC §4.4 Step 0b: if
// .master-tier0 is present, KEK-unwrap and cache. Otherwise check
// for a legacy .master file — if present + raw 32B, run the
// tier-0-only migration sub-sequence (Steps 0b.i–0b.vi) so the
// single-instance Wails IPC continues uninterrupted across the
// pre→post-#1625 restart. Otherwise (fresh install) generate a
// new tier-0 master and KEK-wrap.
func (s *Service) ensureTier0Locked(dir string) core.Result {
	tier0Path := core.PathJoin(dir, masterTier0FileName)

	if statR := core.Stat(tier0Path); statR.OK {
		readR := core.ReadFile(tier0Path)
		if !readR.OK {
			return core.Fail(core.E("keys.ensureMaster", "read tier-0 master", readR.Value.(error)))
		}
		blob := readR.Value.([]byte)
		if len(blob) != kekWrappedMasterLen {
			return core.Fail(core.NewError("keys.ensureMaster: tier-0 master has wrong length; refusing to use"))
		}
		kek, ok := s.currentKEKFor(tier0)
		if !ok {
			return core.Fail(core.NewError("keys.no_tier0_provider: tier-0 KEK provider unavailable; .seed substrate must be wired before tier-0 ops"))
		}
		kekSigil, err := sigil.NewChaChaPolySigil(kek, nil)
		if err != nil {
			return core.Fail(core.E("keys.ensureMaster", "init tier-0 KEK sigil", err))
		}
		unwrapped, err := kekSigil.Out(blob)
		if err != nil {
			return core.Fail(core.E("keys.ensureMaster", "unwrap tier-0 KEK envelope", err))
		}
		if len(unwrapped) != masterKeySize {
			return core.Fail(core.NewError("keys.ensureMaster: tier-0 KEK-unwrapped master has wrong length"))
		}
		s.tier0Master = unwrapped
		return core.Ok(s.tier0Master)
	}

	// Tier-0 master absent — check for legacy install (RFC §4.4
	// Step 0b legacy branch) before fresh-install path.
	legacyPath := core.PathJoin(dir, legacyMasterFileName)
	if statR := core.Stat(legacyPath); statR.OK {
		readR := core.ReadFile(legacyPath)
		if !readR.OK {
			return core.Fail(core.E("keys.ensureMaster", "read legacy master for tier-0 migration", readR.Value.(error)))
		}
		legacy := readR.Value.([]byte)
		switch len(legacy) {
		case masterKeySize:
			// 32B raw legacy — runnable now (no tier-1 KEK
			// needed). Migrate single-instance.aead to
			// single-instance.t0.aead atomically.
			return s.migrateTier0FromRawLegacyLocked(dir, legacy)
		case kekWrappedMasterLen:
			// Post-#1624 wrapped legacy — cannot decrypt
			// single-instance.aead without the tier-1 KEK live.
			// Defer per RFC §4.4 Step 0b.ii — return a
			// dedicated error so callers know the legacy unlock
			// is required.
			return core.Fail(core.NewError("keys.no_legacy_unlock: tier-0 sub-migration cannot run pre-unlock for post-#1624 legacy install; first interactive unlock completes the migration"))
		default:
			return core.Fail(core.NewError("keys.ensureMaster: legacy master file has wrong length; refusing to use"))
		}
	}

	// Fresh install — no tier-0 master, no legacy master. Generate
	// a new tier-0 master and KEK-wrap.
	return s.generateTier0Locked(dir)
}

// generateTier0Locked generates a fresh tier-0 master, KEK-wraps
// it under the live tier-0 KEK provider, and persists to
// .master-tier0. Caller MUST hold s.mu.
func (s *Service) generateTier0Locked(dir string) core.Result {
	kek, ok := s.currentKEKFor(tier0)
	if !ok {
		return core.Fail(core.NewError("keys.no_tier0_provider: cannot generate tier-0 master without KEK provider (wire .seed substrate first)"))
	}
	randR := core.RandomBytes(masterKeySize)
	if !randR.OK {
		return core.Fail(core.E("keys.ensureMaster", "generate tier-0 master", randR.Value.(error)))
	}
	master := randR.Value.([]byte)
	if w := s.writeMasterLocked(core.PathJoin(dir, masterTier0FileName), master, kek); !w.OK {
		return w
	}
	s.tier0Master = master
	return core.Ok(s.tier0Master)
}

// migrateTier0FromRawLegacyLocked runs the RFC §4.4 Step 0b
// tier-0-only sub-sequence for a 32B-raw-legacy install: generate
// a fresh tier-0 master, KEK-wrap it, then if a legacy
// single-instance.aead exists, decrypt under the raw legacy
// master and re-encrypt under the fresh tier-0 master to
// single-instance.t0.aead (atomic-write). The legacy .master file
// is NOT deleted — Step 3 owns that, post-unlock, after the
// tier-1 migration completes. Caller MUST hold s.mu.
func (s *Service) migrateTier0FromRawLegacyLocked(dir string, legacy []byte) core.Result {
	if len(legacy) != masterKeySize {
		return core.Fail(core.NewError("keys.migrate: legacy raw master has wrong length"))
	}
	kek, ok := s.currentKEKFor(tier0)
	if !ok {
		return core.Fail(core.NewError("keys.no_tier0_provider: cannot complete tier-0 sub-migration without KEK"))
	}
	// 0b.iii — generate fresh tier-0 master + KEK-wrap + persist.
	randR := core.RandomBytes(masterKeySize)
	if !randR.OK {
		return core.Fail(core.E("keys.migrate", "generate tier-0 master", randR.Value.(error)))
	}
	tier0Master := randR.Value.([]byte)
	if w := s.writeMasterLocked(core.PathJoin(dir, masterTier0FileName), tier0Master, kek); !w.OK {
		return w
	}
	// 0b.iv — re-seal single-instance.aead if present.
	legacySIPath := core.PathJoin(dir, singleInstanceRef+legacyBlobSuffix)
	if statR := core.Stat(legacySIPath); statR.OK {
		blobR := core.ReadFile(legacySIPath)
		if !blobR.OK {
			return core.Fail(core.E("keys.migrate", "read legacy single-instance", blobR.Value.(error)))
		}
		oldSigil, err := sigil.NewChaChaPolySigil(legacy, nil)
		if err != nil {
			return core.Fail(core.E("keys.migrate", "init legacy sigil", err))
		}
		plaintext, err := oldSigil.Out(blobR.Value.([]byte))
		if err != nil {
			return core.Fail(core.E("keys.migrate", "decrypt legacy single-instance", err))
		}
		newSigil, err := sigil.NewChaChaPolySigil(tier0Master, nil)
		if err != nil {
			return core.Fail(core.E("keys.migrate", "init tier-0 sigil", err))
		}
		newBlob, err := newSigil.In(plaintext)
		if err != nil {
			return core.Fail(core.E("keys.migrate", "re-seal single-instance under tier-0", err))
		}
		newSIPath := core.PathJoin(dir, singleInstanceRef+tier0BlobSuffix)
		if w := core.WriteFile(newSIPath, newBlob, fileMode); !w.OK {
			return core.Fail(core.E("keys.migrate", "write tier-0 single-instance", w.Value.(error)))
		}
		if r := core.Remove(legacySIPath); !r.OK {
			core.Warn("keys: failed to remove legacy single-instance.aead after re-seal",
				"err", r.Error())
		}
	}
	// 0b.vi — log completion.
	core.Info("keys: tier-0 sub-migration complete at boot (legacy install)")
	s.tier0Master = tier0Master
	return core.Ok(s.tier0Master)
}

// ensureTier1Locked is the tier-1 master load path. Caller MUST
// hold s.mu. Requires the tier-1 KEK provider to be live;
// pre-unlock callers see keys.no_tier1_provider.
func (s *Service) ensureTier1Locked(dir string) core.Result {
	tier1Path := core.PathJoin(dir, masterTier1FileName)

	if statR := core.Stat(tier1Path); statR.OK {
		readR := core.ReadFile(tier1Path)
		if !readR.OK {
			return core.Fail(core.E("keys.ensureMaster", "read tier-1 master", readR.Value.(error)))
		}
		blob := readR.Value.([]byte)
		if len(blob) != kekWrappedMasterLen {
			return core.Fail(core.NewError("keys.ensureMaster: tier-1 master has wrong length; refusing to use"))
		}
		kek, ok := s.currentKEKFor(tier1)
		if !ok {
			return core.Fail(core.NewError("keys.no_tier1_provider: tier-1 KEK provider unavailable; unlock the account first"))
		}
		kekSigil, err := sigil.NewChaChaPolySigil(kek, nil)
		if err != nil {
			return core.Fail(core.E("keys.ensureMaster", "init tier-1 KEK sigil", err))
		}
		unwrapped, err := kekSigil.Out(blob)
		if err != nil {
			return core.Fail(core.E("keys.ensureMaster", "unwrap tier-1 KEK envelope", err))
		}
		if len(unwrapped) != masterKeySize {
			return core.Fail(core.NewError("keys.ensureMaster: tier-1 KEK-unwrapped master has wrong length"))
		}
		s.tier1Master = unwrapped
		return core.Ok(s.tier1Master)
	}

	// Tier-1 master absent — fresh install path. Requires
	// tier-1 KEK provider live so we can wrap the new master.
	kek, ok := s.currentKEKFor(tier1)
	if !ok {
		return core.Fail(core.NewError("keys.no_tier1_provider: cannot generate tier-1 master without KEK provider (unlock the account first)"))
	}
	randR := core.RandomBytes(masterKeySize)
	if !randR.OK {
		return core.Fail(core.E("keys.ensureMaster", "generate tier-1 master", randR.Value.(error)))
	}
	master := randR.Value.([]byte)
	if w := s.writeMasterLocked(tier1Path, master, kek); !w.OK {
		return w
	}
	s.tier1Master = master
	return core.Ok(s.tier1Master)
}

// migrateTier1Locked runs the RFC §4.4 Step 1-3 tier-1 migration
// from a pre-partition install. Caller MUST hold s.mu. Idempotent
// via s.migrationComplete latch; subsequent SetKEKProvider calls
// skip the sub-sequence.
func (s *Service) migrateTier1Locked() {
	// Defensive: any error logs a core.Warn and leaves
	// migrationComplete=false so the next SetKEKProvider retries.
	// Tier-1 reads will fail-closed via ensureTier1Locked until
	// migration succeeds.
	dirR := paths.KeysDir()
	if !dirR.OK {
		core.Warn("keys: tier-1 migration skipped — keys dir resolve failed", "err", dirR.Error())
		return
	}
	dir := dirR.Value.(string)

	legacyPath := core.PathJoin(dir, legacyMasterFileName)
	tier1Path := core.PathJoin(dir, masterTier1FileName)

	legacyStat := core.Stat(legacyPath)
	tier1Stat := core.Stat(tier1Path)

	// Step 2 — branch.
	if !legacyStat.OK && !tier1Stat.OK {
		// Fresh tier-1 install — no migration needed.
		s.migrationComplete = true
		return
	}
	if legacyStat.OK && tier1Stat.OK {
		// RFC §4.4 Step 2 corruption signal — both masters
		// present means a prior migration failed mid-way.
		// Refuse to proceed; operator must resolve manually.
		core.Warn("keys: BOTH .master and .master-tier1 present — refusing to migrate; resolve manually before unlocking",
			"dir", dir)
		return
	}
	if tier1Stat.OK && !legacyStat.OK {
		// Already-migrated tier-1 substrate — nothing to do.
		s.migrationComplete = true
		return
	}

	// Step 3 — legacy .master present, tier-1 absent. Migrate.
	kek, ok := s.currentKEKFor(tier1)
	if !ok {
		// Defensive — caller guard already checks but the
		// provider could race-flip in between. Leave latch
		// unset so the next SetKEKProvider retries.
		return
	}

	// 3a. Read legacy .master.
	legacyR := core.ReadFile(legacyPath)
	if !legacyR.OK {
		core.Warn("keys: tier-1 migration read legacy master failed", "err", legacyR.Error())
		return
	}
	legacy := legacyR.Value.([]byte)

	// 3b. Unwrap if needed (post-#1624 wrapped form may have been
	// produced by an earlier session under a different tier-1 KEK
	// — we just need the underlying 32 bytes to re-wrap under the
	// current KEK).
	var legacyMaster []byte
	switch len(legacy) {
	case masterKeySize:
		legacyMaster = legacy
	case kekWrappedMasterLen:
		kekSigil, err := sigil.NewChaChaPolySigil(kek, nil)
		if err != nil {
			core.Warn("keys: tier-1 migration init legacy KEK sigil failed", "err", err.Error())
			return
		}
		unwrapped, err := kekSigil.Out(legacy)
		if err != nil {
			core.Warn("keys: tier-1 migration KEK-unwrap legacy master failed (provider key mismatch?)",
				"err", err.Error(),
				"len", core.Sprintf("%d", len(legacy)))
			return
		}
		if len(unwrapped) != masterKeySize {
			core.Warn("keys: tier-1 migration unwrapped legacy master wrong length",
				"got", core.Sprintf("%d", len(unwrapped)))
			return
		}
		legacyMaster = unwrapped
	default:
		core.Warn("keys: tier-1 migration legacy master wrong length",
			"got", core.Sprintf("%d", len(legacy)))
		return
	}

	// 3b.5. Mantis #1625 S2 (Cerberus #39, tightened by #40) —
	// pre-write atomicity guard. If legacy single-instance.aead
	// is present, Step 3e will need a live tier-0 master to
	// re-seal it. The ONLY thing that guarantees we can produce
	// a tier-0 master at re-seal time is a live tier-0 KEK
	// provider: file-presence (.master-tier0 on disk) is NOT
	// sufficient because without a provider's KEK we cannot
	// unwrap it, and Step 3c only writes-on-absence (it does
	// NOT load-on-presence), so s.tier0Master would stay empty
	// while Step 3d still wrote .master-tier1 and Step 3e's
	// defensive defer fired AFTER the persistent write. Result:
	// next boot sees legacyStat.OK && tier1Stat.OK and
	// corruption-refuses (operator-only recovery). Provider-
	// liveness is the gate, full stop. Leave latch unset so the
	// next SetKEKProvider call (after tier-0 wires) can complete
	// Steps 3c-3g atomically.
	legacySIPath := core.PathJoin(dir, singleInstanceRef+legacyBlobSuffix)
	siLegacyPresent := core.Stat(legacySIPath).OK
	if siLegacyPresent {
		_, tier0ProviderLive := s.currentKEKFor(tier0)
		if !tier0ProviderLive {
			core.Warn("keys: tier-1 migration deferred — single-instance.aead present but tier-0 KEK provider not wired; retaining legacy .master + single-instance.aead, latch unset (will retry on next SetKEKProvider with tier-0 wired)")
			return
		}
	}

	// 3c. If tier-0 master is absent at this point AND we have a
	// tier-0 KEK provider, generate + persist. (Tier-0 may already
	// be present from Step 0b.) Skip silently if no tier-0 provider
	// — tier-0 is independent and can be wired later, UNLESS legacy
	// SI is present (guarded by Step 3b.5 above).
	//
	// Mantis #1653 (Cerberus C#19 deferred from #1625): the original
	// 3c was write-on-absence only. On the retry path (first call
	// deferred at 3b.5 because tier-0 KEK provider not live, then
	// tier-0 wires + SetKEKProvider re-fires) .master-tier0 is
	// already on disk from a prior boot — the write-on-absence
	// branch skips, s.tier0Master stays empty, and Step 3e's
	// defensive defer catches but only AFTER Step 3d has already
	// written .master-tier1, bricking the next boot
	// (legacyStat.OK && tier1Stat.OK → corruption-refuse). Add a
	// load-on-presence branch: when the file IS present and
	// s.tier0Master is empty, unwrap it under the live tier-0 KEK so
	// Step 3e has the master it needs.
	tier0Path := core.PathJoin(dir, masterTier0FileName)
	tier0Stat := core.Stat(tier0Path)
	if !tier0Stat.OK {
		if t0kek, t0ok := s.currentKEKFor(tier0); t0ok {
			randR := core.RandomBytes(masterKeySize)
			if !randR.OK {
				core.Warn("keys: tier-1 migration Step 3c gen tier-0 master failed", "err", randR.Error())
				return
			}
			t0master := randR.Value.([]byte)
			if w := s.writeMasterLocked(tier0Path, t0master, t0kek); !w.OK {
				core.Warn("keys: tier-1 migration Step 3c write tier-0 master failed", "err", w.Error())
				return
			}
			s.tier0Master = t0master
		}
	} else if len(s.tier0Master) != masterKeySize {
		// Load-on-presence path (Mantis #1653 / C#19). File present
		// but in-memory cache empty — the retry-half-state shape.
		// 3b.5 already verified tier-0 KEK provider is live for the
		// SI-present case, but we re-check here defensively because
		// the no-SI branch reaches this code too (file may be from
		// a prior fresh-install Step 0b under a provider that has
		// since been rewired). If the KEK provider isn't live AND
		// no legacy SI is present, leave tier0Master empty —
		// downstream callers will fail-closed cleanly via
		// ensureTier0Locked when they actually need tier-0 ops.
		if t0kek, t0ok := s.currentKEKFor(tier0); t0ok {
			blobR := core.ReadFile(tier0Path)
			if !blobR.OK {
				core.Warn("keys: tier-1 migration Step 3c load tier-0 master read failed", "err", blobR.Error())
				return
			}
			blob := blobR.Value.([]byte)
			if len(blob) != kekWrappedMasterLen {
				core.Warn("keys: tier-1 migration Step 3c on-disk tier-0 master has wrong length; refusing to use",
					"got", core.Sprintf("%d", len(blob)))
				return
			}
			kekSigil, err := sigil.NewChaChaPolySigil(t0kek, nil)
			if err != nil {
				core.Warn("keys: tier-1 migration Step 3c init tier-0 KEK sigil failed", "err", err.Error())
				return
			}
			unwrapped, err := kekSigil.Out(blob)
			if err != nil {
				core.Warn("keys: tier-1 migration Step 3c unwrap tier-0 KEK envelope failed (provider key mismatch?)",
					"err", err.Error())
				return
			}
			if len(unwrapped) != masterKeySize {
				core.Warn("keys: tier-1 migration Step 3c KEK-unwrapped tier-0 master has wrong length",
					"got", core.Sprintf("%d", len(unwrapped)))
				return
			}
			s.tier0Master = unwrapped
		}
	}

	// 3d. Re-wrap legacy master bytes under tier-1 KEK + write to
	// .master-tier1.
	if w := s.writeMasterLocked(tier1Path, legacyMaster, kek); !w.OK {
		core.Warn("keys: tier-1 migration Step 3d write tier-1 master failed", "err", w.Error())
		return
	}
	s.tier1Master = legacyMaster

	// 3e. Re-encrypt single-instance.aead under tier-0 master,
	// atomic-rename to single-instance.t0.aead — only if it's
	// still present (Step 0b 32B-raw-legacy path may have already
	// migrated it). The pre-write guard at Step 3b.5 (Mantis
	// #1625 S2 / Cerberus #39) ensures that if we reach this
	// point with legacy SI still present, tier-0 master is
	// loaded — so the len() check below is defence-in-depth, not
	// the primary deferral path.
	if siLegacyPresent && core.Stat(legacySIPath).OK {
		// Defence-in-depth: 3b.5 should have caught this, but
		// if a race or unexpected state path leaves us without
		// a tier-0 master, fail loud rather than orphan SI.
		if len(s.tier0Master) != masterKeySize {
			core.Warn("keys: tier-1 migration Step 3e defensive — single-instance.aead present but tier-0 master not loaded at re-seal point (Step 3b.5 guard should have caught this); deferring")
			return
		}
		blobR := core.ReadFile(legacySIPath)
		if !blobR.OK {
			core.Warn("keys: tier-1 migration Step 3e read legacy single-instance failed", "err", blobR.Error())
			return
		}
		oldSigil, err := sigil.NewChaChaPolySigil(legacyMaster, nil)
		if err != nil {
			core.Warn("keys: tier-1 migration Step 3e init legacy sigil failed", "err", err.Error())
			return
		}
		plaintext, err := oldSigil.Out(blobR.Value.([]byte))
		if err != nil {
			core.Warn("keys: tier-1 migration Step 3e decrypt single-instance failed", "err", err.Error())
			return
		}
		newSigil, err := sigil.NewChaChaPolySigil(s.tier0Master, nil)
		if err != nil {
			core.Warn("keys: tier-1 migration Step 3e init tier-0 sigil failed", "err", err.Error())
			return
		}
		newBlob, err := newSigil.In(plaintext)
		if err != nil {
			core.Warn("keys: tier-1 migration Step 3e re-seal single-instance failed", "err", err.Error())
			return
		}
		newSIPath := core.PathJoin(dir, singleInstanceRef+tier0BlobSuffix)
		if w := core.WriteFile(newSIPath, newBlob, fileMode); !w.OK {
			core.Warn("keys: tier-1 migration Step 3e write tier-0 single-instance failed", "err", w.Error())
			return
		}
		if r := core.Remove(legacySIPath); !r.OK {
			core.Warn("keys: tier-1 migration Step 3e remove legacy single-instance failed", "err", r.Error())
		}
	}

	// 3f. Rename every remaining .aead (non-single-instance,
	// non-tier-suffixed) to .t1.aead — the same key bytes are now
	// the tier-1 master, so the crypto is unchanged; this step is
	// a pure rename per RFC §4.4 Step 3f optimisation note.
	entriesR := core.ReadDir(core.DirFS(dir), ".")
	if !entriesR.OK {
		core.Warn("keys: tier-1 migration Step 3f readdir failed", "err", entriesR.Error())
		return
	}
	entries := entriesR.Value.([]core.FsDirEntry)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		// Only legacy .aead files; skip already-suffixed
		// .t0.aead / .t1.aead, dot-files, and the
		// single-instance.aead which Step 3e (or Step 0b) owns.
		if !core.HasSuffix(name, legacyBlobSuffix) {
			continue
		}
		if core.HasSuffix(name, tier0BlobSuffix) || core.HasSuffix(name, tier1BlobSuffix) {
			continue
		}
		if name == singleInstanceRef+legacyBlobSuffix {
			continue
		}
		ref := name[:len(name)-len(legacyBlobSuffix)]
		oldPath := core.PathJoin(dir, name)
		newPath := core.PathJoin(dir, ref+tier1BlobSuffix)
		// Read + write + remove. core/go has no Rename primitive
		// in the imported surface; copy-then-remove is equivalent
		// for the migration window (file mode preserved).
		blobR := core.ReadFile(oldPath)
		if !blobR.OK {
			core.Warn("keys: tier-1 migration Step 3f read blob failed",
				"file", name, "err", blobR.Error())
			return
		}
		if w := core.WriteFile(newPath, blobR.Value.([]byte), fileMode); !w.OK {
			core.Warn("keys: tier-1 migration Step 3f write blob failed",
				"file", ref+tier1BlobSuffix, "err", w.Error())
			return
		}
		if r := core.Remove(oldPath); !r.OK {
			core.Warn("keys: tier-1 migration Step 3f remove legacy blob failed",
				"file", name, "err", r.Error())
		}
	}

	// 3g. Delete legacy .master only after 3a-3f succeeded.
	if r := core.Remove(legacyPath); !r.OK {
		core.Warn("keys: tier-1 migration Step 3g remove legacy .master failed", "err", r.Error())
		// Don't unlatch — operator can `rm .master` manually;
		// the migration substrate is otherwise complete.
	}

	// 3h.
	core.Info("keys: migrated to tier-partitioned scheme")
	s.migrationComplete = true
}

// writeMasterLocked persists master to disk in KEK-wrapped form.
// Caller MUST hold s.mu and supply a 32-byte KEK from the live
// provider for the target tier (caller controls which tier).
//
// Mantis #1624 — single write surface so the format decision
// lives in one place. Mantis #1625 — used for both tiers; the
// caller supplies the per-tier KEK.
func (s *Service) writeMasterLocked(masterPath string, master, kek []byte) core.Result {
	kekSigil, err := sigil.NewChaChaPolySigil(kek, nil)
	if err != nil {
		return core.Fail(core.E("keys.writeMasterLocked", "init KEK sigil", err))
	}
	wrapped, err := kekSigil.In(master)
	if err != nil {
		return core.Fail(core.E("keys.writeMasterLocked", "wrap master under KEK", err))
	}
	if w := core.WriteFile(masterPath, wrapped, fileMode); !w.OK {
		return core.Fail(core.E("keys.writeMasterLocked", "write wrapped master", w.Value.(error)))
	}
	return core.Ok(nil)
}

// keyPath returns the absolute path for a key's ciphertext file
// in Result.Value (string), per tier. Ref is the caller's stable
// identifier (e.g. "openai-default"); we never trust it to
// contain slashes — basename only. Returns Fail for empty /
// traversal / dot-prefixed refs, AND for refs matching
// `\.t[0-9]\.aead$` (ADD-K3) so a caller can't sneak a tier
// suffix into the ref and shadow on-disk tier ownership.
func keyPath(ref string, t tier) core.Result {
	if ref == "" {
		return core.Fail(core.NewError("keys: ref must not be empty"))
	}
	if core.Contains(ref, "/") || core.Contains(ref, "\\") || core.HasPrefix(ref, ".") {
		return core.Fail(core.NewError("keys: ref must not contain path separators or start with '.'"))
	}
	if hasTierSuffix(ref) {
		return core.Fail(core.NewError("keys.invalid_ref_suffix: ref must not end in .tN.aead (reserved for on-disk tier scheme)"))
	}
	dirR := paths.KeysDir()
	if !dirR.OK {
		return dirR
	}
	return core.Ok(core.PathJoin(dirR.Value.(string), ref+blobSuffixFor(t)))
}

// hasTierSuffix reports whether ref matches `\.t[0-9]\.aead$` —
// the on-disk tier-suffix scheme. ADD-K3 defence against a user
// ref shadowing on-disk tier ownership.
func hasTierSuffix(ref string) bool {
	const suffix = ".aead"
	if !core.HasSuffix(ref, suffix) {
		return false
	}
	// Strip ".aead" → look for ".tN" tail where N is one digit.
	rem := ref[:len(ref)-len(suffix)]
	if len(rem) < 3 {
		return false
	}
	if rem[len(rem)-3] != '.' || rem[len(rem)-2] != 't' {
		return false
	}
	d := rem[len(rem)-1]
	return d >= '0' && d <= '9'
}

// putLocked seals plaintext under the tier master and writes the
// per-tier blob. Caller provides the resolved master from
// ensureMaster.
func (s *Service) putLocked(t tier, ref string, plaintext, master []byte) core.Result {
	pR := keyPath(ref, t)
	if !pR.OK {
		return pR
	}
	path := pR.Value.(string)
	cipherSigil, err := sigil.NewChaChaPolySigil(master, nil)
	if err != nil {
		return core.Fail(core.E("keys.put", "init sigil", err))
	}
	blob, err := cipherSigil.In(plaintext)
	if err != nil {
		return core.Fail(core.E("keys.put", "seal", err))
	}
	if w := core.WriteFile(path, blob, fileMode); !w.OK {
		return core.Fail(core.E("keys.put", "write ciphertext", w.Value.(error)))
	}
	return core.Ok(nil)
}

// getLocked reads + decrypts the per-tier blob using master.
func (s *Service) getLocked(t tier, ref string, master []byte) core.Result {
	pR := keyPath(ref, t)
	if !pR.OK {
		return pR
	}
	path := pR.Value.(string)
	readR := core.ReadFile(path)
	if !readR.OK {
		return core.Fail(core.E("keys.get", "read ciphertext", readR.Value.(error)))
	}
	cipherSigil, err := sigil.NewChaChaPolySigil(master, nil)
	if err != nil {
		return core.Fail(core.E("keys.get", "init sigil", err))
	}
	plaintext, err := cipherSigil.Out(readR.Value.([]byte))
	if err != nil {
		return core.Fail(core.E("keys.get", "open ciphertext", err))
	}
	return core.Ok(plaintext)
}

// --- Tier-0 API (pre-unlock substrate) ---

// PutTier0 encrypts and stores plaintext under ref in the tier-0
// partition. Tier-0 is the pre-unlock substrate — only refs that
// MUST be readable at process boot (e.g. single-instance Wails
// IPC key) belong here.
//
// Usage example:
//
//	r := svc.PutTier0("single-instance", key32)
func (s *Service) PutTier0(ref string, plaintext []byte) core.Result {
	masterR := s.ensureMaster(tier0)
	if !masterR.OK {
		return masterR
	}
	return s.putLocked(tier0, ref, plaintext, masterR.Value.([]byte))
}

// GetTier0 reads and decrypts plaintext stored under ref in the
// tier-0 partition. NOT exposed via the Wails binding surface —
// frontend has no legitimate tier-0 surface (RFC §4.3 Q4).
//
// Usage example:
//
//	r := svc.GetTier0("single-instance")
//	if r.OK { key := r.Value.([]byte); _ = key }
func (s *Service) GetTier0(ref string) core.Result {
	masterR := s.ensureMaster(tier0)
	if !masterR.OK {
		return masterR
	}
	return s.getLocked(tier0, ref, masterR.Value.([]byte))
}

// HasTier0 reports whether a tier-0 blob exists for ref. Cheap;
// doesn't touch the master key.
//
// Usage example:
//
//	r := svc.HasTier0("single-instance")
//	if r.OK { exists := r.Value.(bool); _ = exists }
func (s *Service) HasTier0(ref string) core.Result {
	pR := keyPath(ref, tier0)
	if !pR.OK {
		return pR
	}
	statR := core.Stat(pR.Value.(string))
	return core.Ok(statR.OK)
}

// DeleteTier0 removes the tier-0 encrypted file. Idempotent.
//
// Usage example:
//
//	r := svc.DeleteTier0("single-instance")
func (s *Service) DeleteTier0(ref string) core.Result {
	pR := keyPath(ref, tier0)
	if !pR.OK {
		return pR
	}
	path := pR.Value.(string)
	if statR := core.Stat(path); !statR.OK {
		return core.Ok(nil)
	}
	if r := core.Remove(path); !r.OK {
		return core.Fail(core.E("keys.DeleteTier0", "remove ciphertext", r.Value.(error)))
	}
	return core.Ok(nil)
}

// ListTier0 returns every tier-0 ref (filenames stripped of the
// .t0.aead suffix). Master files are excluded.
//
// Usage example:
//
//	r := svc.ListTier0()
//	if r.OK { refs := r.Value.([]string); _ = refs }
func (s *Service) ListTier0() core.Result {
	return s.listTier(tier0)
}

// GetOrCreateTier0 returns the plaintext stored under ref in
// tier-0; if no blob exists yet it calls generate(), stores the
// result, then returns it. Concurrent callers serialise via
// getOrCreateMu so generate fires at most once per ref.
//
// Usage example:
//
//	r := svc.GetOrCreateTier0("single-instance", func() ([]byte, error) {
//	    return core.RandomBytes(32).Value.([]byte), nil
//	})
//	if r.OK { key := r.Value.([]byte); _ = key }
func (s *Service) GetOrCreateTier0(ref string, generate func() ([]byte, error)) core.Result {
	return s.getOrCreate(tier0, ref, generate)
}

// --- Tier-1 API (post-unlock provider creds) ---

// PutTier1 encrypts and stores plaintext under ref in the tier-1
// partition. Tier-1 holds long-lived bearer credentials (OpenAI,
// Anthropic, ...) — requires the account to be unlocked.
//
// Usage example:
//
//	r := svc.PutTier1("openai-default", []byte("sk-abc123"))
func (s *Service) PutTier1(ref string, plaintext []byte) core.Result {
	masterR := s.ensureMaster(tier1)
	if !masterR.OK {
		return masterR
	}
	return s.putLocked(tier1, ref, plaintext, masterR.Value.([]byte))
}

// GetTier1 reads and decrypts plaintext stored under ref in the
// tier-1 partition. NOT exposed via the Wails binding surface —
// plaintext secrets must not cross the WebView. Internal Go
// callers only.
//
// Usage example:
//
//	r := svc.GetTier1("openai-default")
//	if r.OK { plaintext := r.Value.([]byte); _ = plaintext }
func (s *Service) GetTier1(ref string) core.Result {
	masterR := s.ensureMaster(tier1)
	if !masterR.OK {
		return masterR
	}
	return s.getLocked(tier1, ref, masterR.Value.([]byte))
}

// HasTier1 reports whether a tier-1 blob exists for ref. Cheap;
// doesn't touch the master key.
//
// Usage example:
//
//	r := svc.HasTier1("openai-default")
//	if r.OK { exists := r.Value.(bool); _ = exists }
func (s *Service) HasTier1(ref string) core.Result {
	pR := keyPath(ref, tier1)
	if !pR.OK {
		return pR
	}
	statR := core.Stat(pR.Value.(string))
	return core.Ok(statR.OK)
}

// DeleteTier1 removes the tier-1 encrypted file. Idempotent.
//
// Usage example:
//
//	r := svc.DeleteTier1("openai-default")
func (s *Service) DeleteTier1(ref string) core.Result {
	pR := keyPath(ref, tier1)
	if !pR.OK {
		return pR
	}
	path := pR.Value.(string)
	if statR := core.Stat(path); !statR.OK {
		return core.Ok(nil)
	}
	if r := core.Remove(path); !r.OK {
		return core.Fail(core.E("keys.DeleteTier1", "remove ciphertext", r.Value.(error)))
	}
	return core.Ok(nil)
}

// ListTier1 returns every tier-1 ref (filenames stripped of the
// .t1.aead suffix). Master files are excluded. Refs only;
// plaintext is never read.
//
// Usage example:
//
//	r := svc.ListTier1()
//	if r.OK { refs := r.Value.([]string); _ = refs }
func (s *Service) ListTier1() core.Result {
	return s.listTier(tier1)
}

// GetOrCreateTier1 returns the plaintext stored under ref in
// tier-1; if no blob exists yet it calls generate(), stores the
// result, then returns it. Concurrent callers serialise via
// getOrCreateMu so generate fires at most once per ref.
//
// Usage example:
//
//	r := svc.GetOrCreateTier1("provider-x", func() ([]byte, error) {
//	    return core.RandomBytes(32).Value.([]byte), nil
//	})
//	if r.OK { key := r.Value.([]byte); _ = key }
func (s *Service) GetOrCreateTier1(ref string, generate func() ([]byte, error)) core.Result {
	return s.getOrCreate(tier1, ref, generate)
}

// listTier is the shared body for ListTier0 / ListTier1.
func (s *Service) listTier(t tier) core.Result {
	dirR := paths.KeysDir()
	if !dirR.OK {
		return dirR
	}
	entriesR := core.ReadDir(core.DirFS(dirR.Value.(string)), ".")
	if !entriesR.OK {
		return core.Fail(core.E("keys.list", "read keys dir", entriesR.Value.(error)))
	}
	suffix := blobSuffixFor(t)
	entries := entriesR.Value.([]core.FsDirEntry)
	out := []string{}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !core.HasSuffix(name, suffix) {
			continue
		}
		out = append(out, name[:len(name)-len(suffix)])
	}
	return core.Ok(out)
}

// getOrCreate is the shared body for GetOrCreateTier0 /
// GetOrCreateTier1. Mirrors the Cerberus pass-6 MEDIUM lock
// discipline: fast-path Get under no extra lock; slow-path
// re-check under getOrCreateMu so concurrent racers don't both
// fire generate.
func (s *Service) getOrCreate(t tier, ref string, generate func() ([]byte, error)) core.Result {
	get := s.getByTier(t, ref)
	if get.OK {
		return get
	}
	s.getOrCreateMu.Lock()
	defer s.getOrCreateMu.Unlock()
	get = s.getByTier(t, ref)
	if get.OK {
		return get
	}
	raw, err := generate()
	if err != nil {
		return core.Fail(core.E("keys.GetOrCreate", "generate", err))
	}
	if r := s.putByTier(t, ref, raw); !r.OK {
		return r
	}
	return core.Ok(raw)
}

// getByTier dispatches Get to the per-tier method without
// re-entering the public surface (avoids double validator + clear
// lock semantics for the GetOrCreate slow-path).
func (s *Service) getByTier(t tier, ref string) core.Result {
	if t == tier0 {
		return s.GetTier0(ref)
	}
	return s.GetTier1(ref)
}

// putByTier dispatches Put to the per-tier method.
func (s *Service) putByTier(t tier, ref string, plaintext []byte) core.Result {
	if t == tier0 {
		return s.PutTier0(ref, plaintext)
	}
	return s.PutTier1(ref, plaintext)
}

// --- Wails-binding lifecycle ---

// ServiceName / ServiceStartup / ServiceShutdown register the
// service for TS bindings at @desktop/keys/service.
func (s *Service) ServiceName() string { return "Keys" }

// ServiceStartup is the Wails3 lifecycle hook; no-op (lazy init).
func (s *Service) ServiceStartup(_ core.Context, _ any) core.Result {
	return core.Ok(nil)
}

// ServiceShutdown clears the cached per-tier masters from memory.
func (s *Service) ServiceShutdown() core.Result {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tier0Master = nil
	s.tier1Master = nil
	return core.Ok(nil)
}

// WPutTier1 is the Wails-binding-friendly PutTier1 (string
// plaintext for JS-ergonomic call sites — TS doesn't enjoy raw
// []byte). Tier-0 W-methods are NOT generated per RFC §4.3 Q4 —
// frontend has no legitimate tier-0 surface.
//
//	import { WPutTier1 } from "@desktop/keys/service";
//	await WPutTier1("openai-default", "sk-abc123");
func (s *Service) WPutTier1(ref, plaintext string) core.Result {
	return s.PutTier1(ref, []byte(plaintext))
}

// WListTier1 is the Wails-binding-friendly ListTier1.
func (s *Service) WListTier1() core.Result { return s.ListTier1() }

// WHasTier1 reports tier-1 presence without decrypting.
func (s *Service) WHasTier1(ref string) core.Result { return s.HasTier1(ref) }

// WDeleteTier1 removes a tier-1 ref. Idempotent.
func (s *Service) WDeleteTier1(ref string) core.Result { return s.DeleteTier1(ref) }

// singleInstanceRef is the stable key name for the per-install
// SingleInstance encryption key — tier-0 (pre-unlock substrate).
const singleInstanceRef = "single-instance"

// SingleInstanceKey returns the 32-byte per-install key used to
// authenticate the Wails single-instance IPC channel. On first
// call it generates a cryptographically random key, persists it
// under ~/Lethean/data/keys/single-instance.t0.aead (tier-0), and
// returns it. Every subsequent call reloads the same persisted
// bytes — the key is stable across restarts.
//
// Cerberus #1442 — replaces the build-time constant in
// pkg/desktop that shared the same EncryptionKey across every
// installed binary on every machine.
//
// Mantis #1625 — wired to tier-0 (pre-unlock substrate) so the
// Wails IPC continues to work before the user has unlocked an
// account.
//
// Usage example:
//
//	r := svc.SingleInstanceKey()
//	if r.OK { key := r.Value.([32]byte); _ = key }
func (s *Service) SingleInstanceKey() core.Result {
	r := s.GetOrCreateTier0(singleInstanceRef, func() ([]byte, error) {
		rr := core.RandomBytes(masterKeySize)
		if !rr.OK {
			return nil, rr.Value.(error)
		}
		return rr.Value.([]byte), nil
	})
	if !r.OK {
		return r
	}
	raw := r.Value.([]byte)
	if len(raw) != masterKeySize {
		return core.Fail(core.NewError("keys.SingleInstanceKey: stored key has wrong length"))
	}
	var key [32]byte
	copy(key[:], raw)
	return core.Ok(key)
}

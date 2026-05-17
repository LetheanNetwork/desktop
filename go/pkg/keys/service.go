// SPDX-Licence-Identifier: EUPL-1.2

// Package keys is the encrypted-at-rest store for the desktop's
// secrets (provider API keys, wallet seeds, signing material). Every
// secret is sealed with ChaCha20-Poly1305 under a 32-byte master
// stored at $HOME/Lethean/data/keys/.master (mode 0600). The master
// is generated on first use; rotating it requires re-encrypting
// every stored blob (out of scope for v1).
//
// Per design_no_hidden_user_bloat — keys/ sits visible under the
// user's $HOME/Lethean/ tree, NOT in ~/.config or ~/Library. The
// master file is a dotfile so it's hidden from default Finder views
// but still discoverable via ls -A.
//
// Usage from another package:
//
//	svc, r := keys.New()
//	if !r.OK { return r }
//	if r := svc.Put("openai-default", []byte("sk-...")); !r.OK { return r }
//	r := svc.Get("openai-default")
//	if r.OK { plaintext := r.Value.([]byte); _ = plaintext }
//
// Wails binding registers as the "Keys" service so the TS side can
// reach it via @desktop/keys/service:
//
//	import { Put, List, Delete } from "@desktop/keys/service";
//	await Put("openai-default", "sk-...");
//
// IMPORTANT: Get is NOT exposed to the Wails surface — plaintext
// secrets must not cross the WebView boundary. Reading the key is
// a Go-side action only (e.g. when the runner spawns an agent).
// Frontend code only writes and lists.

package keys

import (

	core "dappco.re/go"
	"dappco.re/go/io/sigil"
	"dappco.re/lthn/desktop/pkg/paths"
)

const (
	// masterFileName is the dot-prefixed master-key file under
	// $HOME/Lethean/data/keys/. Dot-prefix hides from default Finder
	// without buying us actual security — the master is bound to
	// $HOME, not the file's visibility.
	masterFileName = ".master"

	// keyFileSuffix marks encrypted blobs. ".aead" telegraphs the
	// envelope shape: nonce-prepended ChaCha20-Poly1305 ciphertext.
	keyFileSuffix = ".aead"

	// masterKeySize is the ChaCha20-Poly1305 key length.
	masterKeySize = 32

	// dirMode + fileMode — 0700 for the directory, 0600 for files.
	// Owner-only. The master file's mode is load-bearing for the
	// at-rest protection story.
	dirMode  = 0o700
	fileMode = 0o600
)

// KEKProvider returns the 32-byte key-encryption key (KEK) that
// wraps the on-disk master, plus a boolean reporting whether the KEK
// is currently available. Returns (_, false) pre-unlock (headless
// `lthn serve` boot, account locked, KEK material not yet derivable)
// so the keys Service falls back to the legacy raw-master scheme.
// Returns (kek, true) post-unlock — the provider is expected to
// derive the KEK from post-unlock secret material (e.g. HKDF over
// the account's PGP private key) and return a fresh slice every
// call. The keys Service treats the returned bytes as opaque and
// does not retain a reference beyond the call.
//
// Mantis #1624 — gates pkg/keys's master decrypt on post-unlock
// PGP-derived KEK without breaking pre-unlock boot paths.
type KEKProvider func() ([]byte, bool)

// Service owns the encrypted keys directory. Stateless beyond the
// master-key cache; the disk is the source of truth.
type Service struct {
	// mu guards the master-key cache. Held by ensureMaster.
	mu core.RWMutex
	// getOrCreateMu serialises the miss→generate→put window in
	// GetOrCreate so two concurrent callers don't both generate
	// distinct keys for the same ref. Separate from `mu` because
	// Put internally takes `mu` via ensureMaster; recursive Lock
	// would deadlock. Cerberus pass-6 MEDIUM.
	getOrCreateMu core.Mutex
	master        []byte // 32-byte cached master; loaded on first use

	// kekProviderMu guards kekProvider so SetKEKProvider can race
	// safely against ensureMaster on a concurrent boot path. Held
	// briefly under the slow-path of ensureMaster to snapshot the
	// provider pointer.
	kekProviderMu core.RWMutex
	// kekProvider is the post-unlock KEK source wired by app.go
	// after the account service is constructed. Nil pre-unlock
	// (headless `lthn serve`, account locked) — ensureMaster falls
	// back to the legacy raw-master scheme so single-instance key
	// generation pre-unlock keeps working. Mantis #1624.
	kekProvider KEKProvider
}

const (
	// kekHKDFInfo is the canonical info-string the wiring closure
	// in cmd/lthn/app.go passes to core.HKDF when deriving a KEK
	// from the unlocked PGP private key. Exported as a constant so
	// the call-site reads as one declaration and the salt/info
	// pair stays grep-able from a single source.
	//
	// Usage example (in cmd/lthn/app.go's provider closure):
	//
	//	kek := core.HKDF("sha256", priv, []byte(keys.KEKHKDFSalt), []byte(keys.KEKHKDFInfo), 32).Value.([]byte)
	KEKHKDFSalt = "lthn-keys-kek"
	KEKHKDFInfo = "v1"

	// kekWrappedMasterLen is the on-disk length of a KEK-wrapped
	// master: 24-byte XChaCha20-Poly1305 nonce + 32-byte ciphertext
	// + 16-byte Poly1305 tag = 72 bytes. (sigil.NewChaChaPolySigil
	// defaults to the XChaCha20 variant — see crypto_sigil.go's
	// activeNonceSize() falling through to NonceSizeX.) Used as the
	// length sentinel that distinguishes a legacy raw 32-byte master
	// from a KEK-wrapped envelope on read — no version field needed
	// because the two lengths can never collide (sigil's minLen
	// guarantees > 32 even for the smaller ChaCha20 nonce).
	kekWrappedMasterLen = 24 + masterKeySize + 16
)

// New constructs a Service and ensures $HOME/Lethean/data/keys/
// exists. The master key is loaded (or generated) lazily on the
// first Put/Get. Returns Result with Value=*Service.
//
// Cerberus Mantis #1441 — asserts dir mode is 0o700 + master file
// (if present) is 0o600 at construction time. paths.KeysDir creates
// the dir with 0o700 on first use, but MkdirAll silently no-ops on
// an existing dir regardless of its mode. If the operator or a
// rogue tool widened the perms, we surface that at startup as a
// core.Warn rather than silently shipping under reduced protection.
// CoreGO has no Chmod primitive today (tracked as a separate export
// gap), so we detect-and-warn rather than detect-and-fix. The
// operator gets a clear actionable message.
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
// keys dir + master file; logs a core.Warn for any mode that's
// wider than the expected 0o700 / 0o600. Best-effort — Stat
// failure is silent (KeysDir would have caught a real I/O error).
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
	masterPath := core.PathJoin(dir, masterFileName)
	mStatR := core.Stat(masterPath)
	if !mStatR.OK {
		return // not generated yet — first-write will use fileMode
	}
	mInfo, ok := mStatR.Value.(core.FsFileInfo)
	if !ok {
		return
	}
	if mPerm := mInfo.Mode().Perm(); mPerm != fileMode {
		core.Warn("keys: master file mode wider than 0o600 — "+
			"`chmod 600 ~/Lethean/data/keys/.master` to restore (Mantis #1441)",
			"got", core.Sprintf("%o", mPerm),
			"want", core.Sprintf("%o", fileMode))
	}
}

// Register adopts the canonical Service shape (Mantis #1336).
// Constructs a Service and registers it on Core under "keys".
//
// Cerberus Mantis #1440 (Stage A — Athena's doc-and-warning shim) —
// the user-facing LetheanAccount canon (memory
// `project_lethean_account_pgp_armoured.md`) says identity is an
// armoured PGP key gated by a passphrase. This package's CURRENT
// implementation is a local ChaCha20-Poly1305 master key with NO
// passphrase + NO PGP-armoured account + NO unlock gate. The master
// key file at `~/Lethean/data/keys/.master` is recoverable by
// anyone with the file (mode 0o600 helps; full-disk encryption from
// the host is the only at-rest protection today). Lose the master
// → lose every encrypted secret. The startup Warn surfaces this in
// operator logs while Stage B implementation is queued.
//
// Stage B wires firstlaunch.Phase1 → Enchantrix `pgp.GenerateKeyPair`
// + passphrase prompt + SymmetricallyEncrypt-wrap + unlock-at-start
// gate. Tracking ticket: Mantis #1440.
//
// Usage example:
//
//	if r := keys.Register(c); !r.OK { return r }
func Register(c *core.Core) core.Result {
	core.Warn("keys: backup ~/Lethean/data/keys/.master — " +
		"no passphrase gate yet (Mantis #1440 Stage B)")
	return New()
}

// SetKEKProvider wires the post-unlock KEK source the keys Service
// uses to wrap / unwrap the on-disk master. Wired at boot in
// cmd/lthn/app.go after both the keys + account services have been
// constructed; the provider closure asks pkg/account whether the
// user account is unlocked and, if so, derives a 32-byte KEK via
// core.HKDF over the unlocked PGP private key bytes.
//
// Mantis #1624 — additive over the legacy raw-master scheme:
//
//   - When the provider returns (_, false) the Service falls back to
//     the legacy code path — read the master verbatim, write it
//     verbatim. Preserves headless `lthn serve` + single-instance key
//     generation pre-unlock.
//   - When the provider returns (kek, true) the Service treats the
//     master as a KEK-wrapped envelope: WRITE seals master under KEK
//     before persisting; READ unwraps via KEK before returning.
//   - Existing legacy-format master files (32 bytes raw) continue to
//     decrypt — read-path detects format by length (32 = legacy,
//     60 = KEK-wrapped) and routes accordingly. NEXT WRITE post-
//     unlock re-persists in the wrapped format, completing the
//     migration in-place without a separate ticket.
//
// SetKEKProvider may be called more than once during a process
// lifetime (e.g. service reconfiguration); the most recent
// non-nil provider wins. Passing nil disables KEK wrapping
// entirely (test fixtures + intentional rollback).
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
	s.kekProvider = provider
	// Invalidate the cached master so the next operation re-reads
	// from disk under the new provider. Without this, a kek-provider
	// wire that happens AFTER ensureMaster's first load would leave
	// the in-memory master inconsistent with the on-disk format
	// (caller reads cached legacy bytes; next write would re-seal
	// the same plaintext under KEK, but interim reads would see the
	// stale cache). Clearing here makes the wire observably atomic
	// from the consumer POV.
	s.mu.Lock()
	s.master = nil
	s.mu.Unlock()
	s.kekProviderMu.Unlock()
}

// currentKEK snapshots the wired provider and invokes it once.
// Returns (nil, false) when no provider is wired, the provider
// returns false, or the returned KEK is not 32 bytes (defensive —
// a misbehaving provider must not crash the keys path).
//
// Called under s.mu held by ensureMaster's slow path; the provider
// itself MUST NOT call back into keys.Service to avoid deadlock.
// The wiring in app.go satisfies this — the provider reads from
// pkg/account only.
func (s *Service) currentKEK() ([]byte, bool) {
	s.kekProviderMu.RLock()
	provider := s.kekProvider
	s.kekProviderMu.RUnlock()
	if provider == nil {
		return nil, false
	}
	kek, ok := provider()
	if !ok {
		return nil, false
	}
	if len(kek) != masterKeySize {
		core.Warn("keys: KEK provider returned wrong-length key — falling back to legacy path",
			"got", core.Sprintf("%d", len(kek)),
			"want", core.Sprintf("%d", masterKeySize))
		return nil, false
	}
	return kek, true
}

// ensureMaster loads or generates the master key. Idempotent;
// repeated calls return the cached key.
//
// Mantis #1624 — read-path is format-aware:
//
//   - 32 bytes on disk → legacy raw master. Returned as-is (works
//     pre-KEK and post-KEK so existing installed users don't break).
//   - 60 bytes on disk → KEK-wrapped envelope. Requires the KEK
//     provider to return (kek, true) — Fail otherwise so a locked
//     state can't accidentally hand back the wrong bytes.
//   - any other length → corrupted/tampered, Fail.
//
// Write-path is provider-aware:
//
//   - provider available → seal new master under KEK before persist,
//     so the on-disk format is the 60-byte envelope.
//   - provider unavailable → persist 32 bytes verbatim (legacy).
func (s *Service) ensureMaster() core.Result {
	s.mu.RLock()
	if len(s.master) == masterKeySize {
		s.mu.RUnlock()
		return core.Ok(s.master)
	}
	s.mu.RUnlock()

	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.master) == masterKeySize {
		return core.Ok(s.master)
	}

	dirR := paths.KeysDir()
	if !dirR.OK {
		return dirR
	}
	masterPath := core.PathJoin(dirR.Value.(string), masterFileName)

	statR := core.Stat(masterPath)
	if statR.OK {
		readR := core.ReadFile(masterPath)
		if !readR.OK {
			return core.Fail(core.E("keys.ensureMaster", "read master", readR.Value.(error)))
		}
		blob := readR.Value.([]byte)
		switch len(blob) {
		case masterKeySize:
			// Legacy format — raw 32-byte master. Accepted whether
			// or not the KEK provider is available; the NEXT write
			// (Put / GetOrCreate / SingleInstanceKey) re-persists
			// in the wrapped format if the provider is live, which
			// is the in-place migration shape.
			s.master = blob
			return core.Ok(s.master)
		case kekWrappedMasterLen:
			// KEK-wrapped envelope — needs the provider live to
			// unwrap. If unwrap fails (provider returns false, KEK
			// mismatch, tamper) the master is unreachable and we
			// Fail so a caller doesn't operate on stale legacy bytes.
			kek, ok := s.currentKEK()
			if !ok {
				return core.Fail(core.NewError("keys.ensureMaster: master is KEK-wrapped but KEK provider unavailable; unlock the account first"))
			}
			kekSigil, err := sigil.NewChaChaPolySigil(kek, nil)
			if err != nil {
				return core.Fail(core.E("keys.ensureMaster", "init KEK sigil", err))
			}
			unwrapped, err := kekSigil.Out(blob)
			if err != nil {
				return core.Fail(core.E("keys.ensureMaster", "unwrap KEK envelope", err))
			}
			if len(unwrapped) != masterKeySize {
				return core.Fail(core.NewError("keys.ensureMaster: KEK-unwrapped master has wrong length"))
			}
			s.master = unwrapped
			return core.Ok(s.master)
		default:
			return core.Fail(core.NewError("keys.ensureMaster: master file has wrong length; refusing to use"))
		}
	}

	// Generate a fresh master.
	randR := core.RandomBytes(masterKeySize)
	if !randR.OK {
		return core.Fail(core.E("keys.ensureMaster", "generate master", randR.Value.(error)))
	}
	master := randR.Value.([]byte)
	if writeR := s.writeMasterLocked(masterPath, master); !writeR.OK {
		return writeR
	}
	s.master = master
	return core.Ok(s.master)
}

// writeMasterLocked persists master to disk in the format dictated
// by the current KEK provider state — KEK-wrapped when the provider
// is live, raw 32-byte legacy when not. Caller MUST hold s.mu;
// queries the provider via currentKEK which takes kekProviderMu
// independently of s.mu.
//
// Mantis #1624 — single write surface so the format decision lives
// in one place. Called from ensureMaster on first-generate AND from
// rotateMasterToKEK on lazy-migration (the next-write-post-unlock
// path).
func (s *Service) writeMasterLocked(masterPath string, master []byte) core.Result {
	kek, ok := s.currentKEK()
	if !ok {
		if w := core.WriteFile(masterPath, master, fileMode); !w.OK {
			return core.Fail(core.E("keys.ensureMaster", "write master", w.Value.(error)))
		}
		return core.Ok(nil)
	}
	kekSigil, err := sigil.NewChaChaPolySigil(kek, nil)
	if err != nil {
		return core.Fail(core.E("keys.ensureMaster", "init KEK sigil for write", err))
	}
	wrapped, err := kekSigil.In(master)
	if err != nil {
		return core.Fail(core.E("keys.ensureMaster", "wrap master under KEK", err))
	}
	if w := core.WriteFile(masterPath, wrapped, fileMode); !w.OK {
		return core.Fail(core.E("keys.ensureMaster", "write wrapped master", w.Value.(error)))
	}
	return core.Ok(nil)
}

// rotateMasterToKEKIfNeeded re-persists the on-disk master under the
// current KEK iff (a) the cached master is loaded AND (b) the disk
// representation is still the legacy 32-byte form AND (c) the KEK
// provider is now live. Idempotent — re-running on an already-wrapped
// master is a no-op. Caller MUST hold s.mu.
//
// Mantis #1624 lazy-migration shape — invoked from Put after the
// ciphertext is sealed so any successful write post-unlock graduates
// the master format without changing user-observable behaviour. Read
// always handled both formats from Mantis #1624 onwards, so the
// migration window is closed silently on the next Put.
func (s *Service) rotateMasterToKEKIfNeeded() {
	if len(s.master) != masterKeySize {
		return
	}
	if _, ok := s.currentKEK(); !ok {
		return
	}
	dirR := paths.KeysDir()
	if !dirR.OK {
		return
	}
	masterPath := core.PathJoin(dirR.Value.(string), masterFileName)
	statR := core.Stat(masterPath)
	if !statR.OK {
		return
	}
	readR := core.ReadFile(masterPath)
	if !readR.OK {
		return
	}
	if len(readR.Value.([]byte)) != masterKeySize {
		return // already wrapped or unrecognised; leave alone
	}
	// Re-persist under KEK. Failure here is logged-and-tolerated:
	// the cached master is still authoritative for the running
	// process, and the next boot will retry the migration on first
	// Put. Surfacing as core.Warn matches the sibling tamper warnings.
	if w := s.writeMasterLocked(masterPath, s.master); !w.OK {
		core.Warn("keys: lazy KEK-wrap of legacy master failed; will retry on next write",
			"err", w.Error())
	}
}

// keyPath returns the absolute path for a key's ciphertext file in
// Result.Value (string). Ref is the caller's stable identifier
// (e.g. "openai-default"); we never trust it to contain slashes —
// basename only. Returns Fail for empty / traversal / dot-prefixed
// refs.
func keyPath(ref string) core.Result {
	if ref == "" {
		return core.Fail(core.NewError("keys: ref must not be empty"))
	}
	if core.Contains(ref, "/") || core.Contains(ref, "\\") || core.HasPrefix(ref, ".") {
		return core.Fail(core.NewError("keys: ref must not contain path separators or start with '.'"))
	}
	dirR := paths.KeysDir()
	if !dirR.OK {
		return dirR
	}
	return core.Ok(core.PathJoin(dirR.Value.(string), ref+keyFileSuffix))
}

// Put encrypts and stores plaintext under ref. Overwrites silently.
//
// Usage example:
//
//	r := svc.Put("openai-default", []byte("sk-abc123"))
func (s *Service) Put(ref string, plaintext []byte) core.Result {
	masterR := s.ensureMaster()
	if !masterR.OK {
		return masterR
	}
	pR := keyPath(ref)
	if !pR.OK {
		return pR
	}
	path := pR.Value.(string)
	cipherSigil, err := sigil.NewChaChaPolySigil(masterR.Value.([]byte), nil)
	if err != nil {
		return core.Fail(core.E("keys.Put", "init sigil", err))
	}
	blob, err := cipherSigil.In(plaintext)
	if err != nil {
		return core.Fail(core.E("keys.Put", "seal", err))
	}
	if w := core.WriteFile(path, blob, fileMode); !w.OK {
		return core.Fail(core.E("keys.Put", "write ciphertext", w.Value.(error)))
	}
	// Mantis #1624 lazy KEK-wrap of legacy master — if the master
	// on disk is still the raw 32-byte form AND a KEK provider is
	// now live (account unlocked), re-persist the master under KEK
	// so the install converges on the wrapped format without an
	// explicit migration ticket. Held under s.mu to keep the
	// read-after-rotate window coherent.
	s.mu.Lock()
	s.rotateMasterToKEKIfNeeded()
	s.mu.Unlock()
	return core.Ok(nil)
}

// Get reads and decrypts the plaintext stored under ref. The
// returned Result.Value is []byte; an empty ref or missing file
// fails. NOT exposed via the Wails binding surface — plaintext
// secrets must not cross the WebView. Internal Go callers only.
//
// Usage example:
//
//	r := svc.Get("openai-default")
//	if r.OK { plaintext := r.Value.([]byte); _ = plaintext }
func (s *Service) Get(ref string) core.Result {
	masterR := s.ensureMaster()
	if !masterR.OK {
		return masterR
	}
	pR := keyPath(ref)
	if !pR.OK {
		return pR
	}
	path := pR.Value.(string)
	readR := core.ReadFile(path)
	if !readR.OK {
		return core.Fail(core.E("keys.Get", "read ciphertext", readR.Value.(error)))
	}
	cipherSigil, err := sigil.NewChaChaPolySigil(masterR.Value.([]byte), nil)
	if err != nil {
		return core.Fail(core.E("keys.Get", "init sigil", err))
	}
	plaintext, err := cipherSigil.Out(readR.Value.([]byte))
	if err != nil {
		return core.Fail(core.E("keys.Get", "open ciphertext", err))
	}
	return core.Ok(plaintext)
}

// Delete removes the encrypted file. No-op (OK) when the ref
// isn't registered.
//
// Usage example:
//
//	r := svc.Delete("openai-default")
func (s *Service) Delete(ref string) core.Result {
	pR := keyPath(ref)
	if !pR.OK {
		return pR
	}
	path := pR.Value.(string)
	statR := core.Stat(path)
	if !statR.OK {
		// Missing → success (idempotent).
		return core.Ok(nil)
	}
	if r := core.Remove(path); !r.OK {
		return core.Fail(core.E("keys.Delete", "remove ciphertext", r.Value.(error)))
	}
	return core.Ok(nil)
}

// List returns every registered key ref (filenames stripped of
// the .aead suffix). The .master file is excluded. Refs only;
// plaintext is never read.
//
// Usage example:
//
//	r := svc.List()
//	if r.OK { refs := r.Value.([]string); _ = refs }
func (s *Service) List() core.Result {
	dirR := paths.KeysDir()
	if !dirR.OK {
		return dirR
	}
	entriesR := core.ReadDir(core.DirFS(dirR.Value.(string)), ".")
	if !entriesR.OK {
		return core.Fail(core.E("keys.List", "read keys dir", entriesR.Value.(error)))
	}
	entries := entriesR.Value.([]core.FsDirEntry)
	out := []string{}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !core.HasSuffix(name, keyFileSuffix) {
			continue
		}
		out = append(out, name[:len(name)-len(keyFileSuffix)])
	}
	return core.Ok(out)
}

// Has reports whether a ciphertext file exists for ref. Cheap;
// doesn't touch the master key.
//
// Usage example:
//
//	r := svc.Has("openai-default")
//	if r.OK { exists := r.Value.(bool); _ = exists }
func (s *Service) Has(ref string) core.Result {
	pR := keyPath(ref)
	if !pR.OK {
		return pR
	}
	path := pR.Value.(string)
	statR := core.Stat(path)
	return core.Ok(statR.OK)
}

// --- Wails-binding lifecycle ---

// ServiceName / ServiceStartup / ServiceShutdown register the
// service for TS bindings at @desktop/keys/service.
func (s *Service) ServiceName() string { return "Keys" }

// ServiceStartup is the Wails3 lifecycle hook; no-op (lazy init).
func (s *Service) ServiceStartup(_ core.Context, _ any) core.Result {
	return core.Ok(nil)
}

// ServiceShutdown clears the cached master from memory.
func (s *Service) ServiceShutdown() core.Result {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.master = nil
	return core.Ok(nil)
}

// WPut is the Wails-binding-friendly Put (string plaintext for
// JS-ergonomic call sites — TS doesn't enjoy raw []byte). Frontend:
//
//	import { WPut } from "@desktop/keys/service";
//	await WPut("openai-default", "sk-abc123");
func (s *Service) WPut(ref, plaintext string) core.Result {
	return s.Put(ref, []byte(plaintext))
}

// WList is the Wails-binding-friendly List. Same shape; named with
// the W- prefix for binding-clarity matching the runner / server
// W-prefixed lifecycle.
func (s *Service) WList() core.Result { return s.List() }

// WHas reports presence without decrypting. Plaintext never crosses
// the binding.
func (s *Service) WHas(ref string) core.Result { return s.Has(ref) }

// WDelete removes a ref. Idempotent.
func (s *Service) WDelete(ref string) core.Result { return s.Delete(ref) }

// GetOrCreate returns the plaintext stored under ref; if no blob exists
// yet it calls generate(), stores the result under ref, then returns it.
// generate must return exactly masterKeySize bytes for keys used as
// symmetric keys (the caller is responsible for the length contract).
//
// Cerberus pass-6 MEDIUM — the previous implementation did Get →
// generate() → Put without holding a lock across the sequence. Two
// concurrent callers could both miss the Get, both generate distinct
// random keys, both Put — the second Put silently overwrites the
// first, but each caller returns ITS OWN generated bytes. The
// persisted file then disagrees with one of the in-memory copies,
// breaking any channel auth that relies on "the key on disk is the
// one I'm using". The fix wraps the full miss→generate→put window
// under s.mu — at most one generate() call fires per ref across
// concurrent callers.
//
// In actual lthn boot today only one caller (desktop.Run's
// SingleInstanceKey) reaches this — single goroutine, no live race.
// But the contract is now correct for future callers (rotation,
// multi-key schemas) without surprising them.
//
// Usage example:
//
//	r := svc.GetOrCreate("single-instance", func() ([]byte, error) {
//	    return core.RandomBytes(32).Value.([]byte), nil
//	})
//	if r.OK { key := r.Value.([]byte); _ = key }
func (s *Service) GetOrCreate(ref string, generate func() ([]byte, error)) core.Result {
	// Fast path — unlocked Get for the common case (key already
	// persisted from a prior boot).
	if r := s.Get(ref); r.OK {
		return r
	}
	// Slow path — take getOrCreateMu (distinct from s.mu which Put
	// → ensureMaster already takes — recursive lock would deadlock).
	// Re-check inside the lock so a concurrent winner's Put becomes
	// the canonical value (both racers won't run generate twice).
	s.getOrCreateMu.Lock()
	defer s.getOrCreateMu.Unlock()
	if r := s.Get(ref); r.OK {
		return r
	}
	raw, err := generate()
	if err != nil {
		return core.Fail(core.E("keys.GetOrCreate", "generate", err))
	}
	if r := s.Put(ref, raw); !r.OK {
		return r
	}
	return core.Ok(raw)
}

// singleInstanceRef is the stable key name for the per-install
// SingleInstance encryption key.
const singleInstanceRef = "single-instance"

// SingleInstanceKey returns the 32-byte per-install key used to
// authenticate the Wails single-instance IPC channel. On first call
// it generates a cryptographically random key, persists it under
// ~/Lethean/data/keys/single-instance.aead, and returns it. Every
// subsequent call reloads the same persisted bytes — the key is stable
// across restarts.
//
// Cerberus #1442: replaces the build-time constant in pkg/desktop that
// shared the same EncryptionKey across every installed binary on every
// machine, defeating the authenticated-channel guarantee.
//
// Usage example:
//
//	r := svc.SingleInstanceKey()
//	if r.OK { key := r.Value.([32]byte); _ = key }
func (s *Service) SingleInstanceKey() core.Result {
	r := s.GetOrCreate(singleInstanceRef, func() ([]byte, error) {
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

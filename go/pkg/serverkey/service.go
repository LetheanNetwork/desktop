// SPDX-Licence-Identifier: EUPL-1.2

package serverkey

import (
	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/paths"
	"forge.lthn.ai/Snider/Enchantrix/pkg/crypt/std/pgp"
)

// NewService constructs the serverkey Service. Generates the per-
// process random salt that keys the consumed-nonce set (Cerberus
// #1463) — same salt for every call across the service's lifetime,
// distinct per process restart so a stolen consumed-nonce set from
// one run can't be replayed against another.
//
// The core handle is stored for future event-bus emission (e.g.
// "serverkey.rotated" once rotation lands in Stage D) but is not
// required for Bootstrap / IssueBootstrapToken / VerifyBootstrapToken
// today. Pass nil from tests that don't care.
//
// Usage example:
//
//	svc := serverkey.NewService(c)
//	if r := svc.Bootstrap(); !r.OK { return r }
func NewService(c *core.Core) *Service {
	saltR := core.RandomBytes(processSaltSize)
	salt := []byte{}
	if saltR.OK {
		salt, _ = saltR.Value.([]byte)
	}
	// Fallback — RandomBytes only fails on crypto/rand exhaustion;
	// fall back to a deterministic but per-launch salt rather than
	// crash the boot path. The nonce-set keying still gives the
	// "memory-read attacker can't read raw nonces" property because
	// HMAC with any salt obscures the input.
	if len(salt) == 0 {
		stamp := core.Sprintf("salt-%d", core.Now().UnixNano())
		salt = []byte(stamp)
	}
	return &Service{
		core:                  c,
		processSalt:           salt,
		consumedNonces:        map[string]core.Time{},
		consumedSessionNonces: map[string]core.Time{},
	}
}

// Register satisfies the canonical core.WithName signature so app.go
// can wire serverkey alongside the other Core services. The returned
// *Service is what subsequent ServiceFor[*serverkey.Service] resolves.
// Bootstrap() must still be invoked explicitly — Register only
// constructs; it does not generate keys (so unit tests can pin paths
// before Bootstrap runs).
//
// Usage example:
//
//	core.WithName("serverkey", serverkey.Register)
func Register(c *core.Core) core.Result {
	return core.Ok(NewService(c))
}

// ServiceName labels the binding namespace exposed to JS.
// Wails binds methods as "ServerKey.AccountStatus()" etc.
func (s *Service) ServiceName() string { return "ServerKey" }

// ServiceStartup is the Wails3 lifecycle hook; no-op (Bootstrap runs
// from cmd/lthn/app.go before the API server starts so the verifier
// is live before the WebView can issue a request).
func (s *Service) ServiceStartup(_ core.Context, _ any) core.Result {
	return core.Ok(nil)
}

// ServiceShutdown clears the in-memory private key from memory.
// Public key + nonce-set are left alone (no secrets in them; clearing
// them costs nothing but adds no protection).
func (s *Service) ServiceShutdown() core.Result {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.privateKey = nil
	return core.Ok(nil)
}

// Bootstrap idempotently ensures the server key is live in memory.
// On first call: generates ~/Lethean/wallets/.seed (if missing),
// derives the symmetric passphrase via HKDF-SHA256, then either
// loads + decrypts ~/Lethean/wallets/server.key OR generates +
// encrypts + persists a fresh PGP key pair. On subsequent calls in
// the same process: returns OK immediately (in-memory keys already
// populated).
//
// Cerberus #1466 — the call acquires a first-launch lock first.
// CoreGUI's SingleInstanceOptions only enforces at application.New()
// / .Run() time, which happens AFTER newAppCore returns. Two CLI
// invocations of `lthn serve` (or one of each verb) can race against
// the seed / key files. We acquire an exclusive-create sentinel at
// ~/Lethean/wallets/.bootstrap.lock for the critical section; concurrent
// callers either inherit the freshly-created keys from the winner OR
// retry once the lock releases.
//
// Cerberus #1464 — every load path re-Stats both seed + server.key
// after open and verifies mode is exactly 0o600. A widened mode is
// treated as tamper: fail-closed with "serverkey.mode.tamper" so the
// operator notices instead of silently shipping under reduced
// protection.
//
// Cerberus #1470 — passphrase derivation NEVER uses CWD. The .seed
// file is the unique-per-machine secret (no surface on ps / lsof /
// /proc); HKDF-SHA256 domain-separates by salt + info constants.
//
// Usage example:
//
//	if r := svc.Bootstrap(); !r.OK { return r }
func (s *Service) Bootstrap() core.Result {
	// Fast path — keys already live; skip the lock + IO entirely.
	s.mu.RLock()
	if len(s.publicKey) > 0 && len(s.privateKey) > 0 {
		s.mu.RUnlock()
		return core.Ok(nil)
	}
	s.mu.RUnlock()

	walletsR := paths.WalletsDir()
	if !walletsR.OK {
		return walletsR
	}
	walletsDir, _ := walletsR.Value.(string)

	// Acquire the bootstrap lock. CoreGUI's single-instance gate runs
	// later (after application.New().Run()) — newAppCore can race
	// against parallel CLI invocations of other verbs, so we need our
	// own gate here.
	release, lockR := s.acquireBootstrapLock(walletsDir)
	if !lockR.OK {
		return lockR
	}
	defer release()

	s.mu.Lock()
	defer s.mu.Unlock()

	// Re-check under the write lock — a parallel goroutine in the
	// same process could have populated the cache between RUnlock and
	// Lock.
	if len(s.publicKey) > 0 && len(s.privateKey) > 0 {
		return core.Ok(nil)
	}

	seedPath := core.PathJoin(walletsDir, seedFile)
	keyPath := core.PathJoin(walletsDir, serverKeyFile)

	// Ensure wallets/ exists with strict mode so a freshly-installed
	// box doesn't ship the seed under group/other-readable perms.
	if r := core.MkdirAll(walletsDir, dirMode); !r.OK {
		return core.Fail(core.E("serverkey.Bootstrap", "mkdir wallets", r.Value.(error)))
	}

	seedR := s.loadOrCreateSeed(seedPath)
	if !seedR.OK {
		return seedR
	}
	seed, _ := seedR.Value.([]byte)

	// Derive the symmetric passphrase via HKDF-SHA256. The .seed file
	// is the secret; the salt + info constants give us domain-
	// separated derivation for future contexts (rotation, multi-purpose
	// keys) without re-using the same bytes.
	passR := core.HKDF("sha256", seed, hkdfSalt, hkdfInfo, passphraseSize)
	if !passR.OK {
		// Wrap the HKDF error so the boot logger sees an actionable
		// scope. Cause already carries the crypto error code.
		if err, ok := passR.Value.(error); ok {
			return core.Fail(core.E("serverkey.Bootstrap", "derive passphrase", err))
		}
		return core.Fail(core.NewError("serverkey.Bootstrap: derive passphrase failed"))
	}
	passphrase, _ := passR.Value.([]byte)

	if keyStat := core.Stat(keyPath); keyStat.OK {
		// Load + decrypt path.
		if modeR := s.verifyMode(keyStat, fileMode, keyPath); !modeR.OK {
			return modeR
		}
		pub, priv, loadR := s.loadServerKey(keyPath, passphrase)
		if !loadR.OK {
			return loadR
		}
		s.publicKey = pub
		s.privateKey = priv
		return core.Ok(nil)
	}

	// Generate + persist path.
	svc := pgp.NewService()
	pub, privEnc, err := svc.GenerateKeyPair("lthn-server", "server@lthn.local", "first-run")
	if err != nil {
		return core.Fail(core.E("serverkey.Bootstrap", "generate keypair", err))
	}

	// Symmetric-wrap the private key — we never persist cleartext.
	priv, _, err := encryptPrivate(svc, passphrase, privEnc)
	if err != nil {
		return core.Fail(core.E("serverkey.Bootstrap", "encrypt private", err))
	}

	blob := marshalServerKey(pub, priv)
	if r := atomicWrite(keyPath, blob, fileMode); !r.OK {
		return r
	}
	// Re-Stat + verify mode is exactly 0o600 — umask / mount-option
	// quirks can drop bits on WriteFile (Cerberus #1464).
	if statR := core.Stat(keyPath); statR.OK {
		if modeR := s.verifyMode(statR, fileMode, keyPath); !modeR.OK {
			return modeR
		}
	}

	s.publicKey = pub
	s.privateKey = privEnc
	return core.Ok(nil)
}

// loadOrCreateSeed returns the 32-byte .seed bytes, generating them
// on first call. Verifies mode 0o600 at open time per Cerberus #1464.
func (s *Service) loadOrCreateSeed(seedPath string) core.Result {
	statR := core.Stat(seedPath)
	if statR.OK {
		if modeR := s.verifyMode(statR, fileMode, seedPath); !modeR.OK {
			return modeR
		}
		readR := core.ReadFile(seedPath)
		if !readR.OK {
			return core.Fail(core.E("serverkey.loadSeed", "read seed", readR.Value.(error)))
		}
		seed, _ := readR.Value.([]byte)
		if len(seed) != seedSize {
			return core.Fail(core.E("serverkey.loadSeed", "seed has wrong length — refusing to use", nil))
		}
		return core.Ok(seed)
	}

	randR := core.RandomBytes(seedSize)
	if !randR.OK {
		return core.Fail(core.E("serverkey.loadSeed", "generate seed", randR.Value.(error)))
	}
	seed, _ := randR.Value.([]byte)
	if r := atomicWrite(seedPath, seed, fileMode); !r.OK {
		return r
	}
	// Re-Stat + verify mode (Cerberus #1464 — open-time verification,
	// not just write-time).
	if statR := core.Stat(seedPath); statR.OK {
		if modeR := s.verifyMode(statR, fileMode, seedPath); !modeR.OK {
			return modeR
		}
	}
	return core.Ok(seed)
}

// verifyMode fails closed when the stat'd file's mode differs from
// the expected perm bits. Cerberus #1464 — a widened mode is treated
// as tamper: caller refuses to use the file rather than silently
// shipping under reduced protection.
func (s *Service) verifyMode(statR core.Result, want core.FileMode, path string) core.Result {
	info, ok := statR.Value.(core.FsFileInfo)
	if !ok {
		return core.Fail(core.E("serverkey.verifyMode", "stat returned non-FileInfo", nil))
	}
	got := info.Mode().Perm()
	if got != want {
		core.Warn("serverkey: file mode wider than expected (Cerberus #1464) — refusing to use",
			"path", path,
			"got", core.Sprintf("%o", got),
			"want", core.Sprintf("%o", want))
		return core.Fail(core.E("serverkey.verifyMode",
			core.Sprintf("file mode tamper: got %o want %o (path %s)", got, want, path),
			nil))
	}
	return core.Ok(nil)
}

// AccountStatus reports whether a Lethean Account exists on this
// machine. Per Cerberus #1471 the signal is the leaf file
// ~/Lethean/account/<id>/private.key for at least one <id> — NOT
// the presence of the parent directory alone (a partial account
// directory created-then-deleted strands the gate in `auth` with
// nothing to unlock).
//
// Usage example:
//
//	r := svc.AccountStatus()
//	if r.OK { s := r.Value.(AccountStatusOutput); _ = s.HasUserAccount }
func (s *Service) AccountStatus() core.Result {
	rootR := paths.Root()
	if !rootR.OK {
		return rootR
	}
	root, _ := rootR.Value.(string)
	accountRoot := core.PathJoin(root, accountDir)

	// Container dir may not exist yet — that's a fully-fresh install,
	// has_user_account=false is correct.
	if !core.Stat(accountRoot).OK {
		return core.Ok(AccountStatusOutput{HasUserAccount: false})
	}

	entriesR := core.ReadDir(core.DirFS(accountRoot), ".")
	if !entriesR.OK {
		// Dir exists but unreadable — surface as has_user_account=false
		// so the gate falls back to `setup`. Logging at warn so an
		// operator can see what happened.
		core.Warn("serverkey.AccountStatus: read account dir failed",
			"path", accountRoot,
			"err", entriesR.Error())
		return core.Ok(AccountStatusOutput{HasUserAccount: false})
	}
	entries, _ := entriesR.Value.([]core.FsDirEntry)
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		leaf := core.PathJoin(accountRoot, e.Name(), accountKeyFile)
		if core.Stat(leaf).OK {
			return core.Ok(AccountStatusOutput{HasUserAccount: true, AccountID: e.Name()})
		}
	}
	return core.Ok(AccountStatusOutput{HasUserAccount: false})
}

// acquireBootstrapLock returns a release closure + the acquisition
// Result. Uses OpenFile(O_CREATE|O_EXCL) as an atomic sentinel; if
// the lock file exists, retries with backoff up to ~5s, then fails.
// On success the lock file lives at ~/Lethean/wallets/.bootstrap.lock
// and is removed by release() (deferred from Bootstrap).
//
// Cerberus #1466 — CoreGUI's SingleInstance enforcement only kicks
// in after application.New().Run(). newAppCore can race against
// parallel CLI invocations, so we need our own gate at the bootstrap
// critical section. OpenFile with O_EXCL is the cgo-free, banned-
// imports-friendly mechanism available within CoreGO's primitive set
// (no syscall.Flock wrapper exposed today).
//
// If singleInstanceCheck is wired (tests use this for DI), it's
// consulted FIRST — when it returns true the CoreGUI gate is assumed
// to have already serialised us and the file lock is skipped.
func (s *Service) acquireBootstrapLock(walletsDir string) (func(), core.Result) {
	noop := func() {}
	if s.singleInstanceCheck != nil && s.singleInstanceCheck() {
		// CoreGUI says it's already serialised us — no file lock
		// needed. Document in trace so operators reading boot logs
		// understand which branch ran.
		return noop, core.Ok(nil)
	}
	lockPath := core.PathJoin(walletsDir, bootstrapLock)
	// Ensure wallets/ exists before attempting to create the lock
	// file — Bootstrap's MkdirAll runs LATER under the lock, so we
	// can't rely on it.
	if r := core.MkdirAll(walletsDir, dirMode); !r.OK {
		return noop, core.Fail(core.E("serverkey.acquireBootstrapLock", "mkdir wallets", r.Value.(error)))
	}
	// Backoff: 100ms, 200ms, 400ms, 800ms, 1600ms, 2000ms = ~5s total.
	delays := []core.Duration{
		100 * core.Millisecond,
		200 * core.Millisecond,
		400 * core.Millisecond,
		800 * core.Millisecond,
		1600 * core.Millisecond,
		2000 * core.Millisecond,
	}
	for attempt := 0; attempt <= len(delays); attempt++ {
		openR := core.OpenFile(lockPath, core.O_CREATE|core.O_EXCL|core.O_WRONLY, fileMode)
		if openR.OK {
			f, _ := openR.Value.(*core.OSFile)
			if f != nil {
				_ = f.Close()
			}
			release := func() {
				_ = core.Remove(lockPath)
			}
			return release, core.Ok(nil)
		}
		if attempt == len(delays) {
			break
		}
		core.Sleep(delays[attempt])
	}
	return noop, core.Fail(core.E("serverkey.acquireBootstrapLock",
		"could not acquire ~/Lethean/wallets/.bootstrap.lock within 5s — "+
			"another lthn process may be holding it; remove the file manually "+
			"if you're sure no other instance is running", nil))
}

// SPDX-Licence-Identifier: EUPL-1.2

package account

import (
	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/audit"
	"dappco.re/lthn/desktop/pkg/paths"
)

// Service owns the /v1/account/create endpoint contract. Construct
// via NewService; Register so cmd/lthn can resolve it via the Core
// container. The endpoint handler (routes.go) gates on the bootstrap-
// auth middleware in pkg/server — by the time Create is called the
// bootstrap-token's nonce has already been consumed by
// serverkey.VerifyBootstrapToken (see Cerberus #1460 (d) note on
// Create below).
//
// Usage example:
//
//	svc := account.NewService(c)
//	if r := svc.Create(account.CreateInput{...}); !r.OK {
//	    return r
//	}
type Service struct {
	core *core.Core

	// mu guards unlocked + lockouts. RW because HasUnlocked reads;
	// Unlock + Lock + recordFailedAttempt write.
	mu core.RWMutex

	// unlocked holds the in-memory decrypted PGP private key per
	// account_id for the session lifetime. Cleared by Lock or by
	// ServiceShutdown. NEVER persisted to disk in cleartext —
	// re-decrypt on next Bootstrap is the policy.
	unlocked map[string][]byte

	// lockouts tracks per-account_id failed-unlock counters per RFC
	// §5 M1. In-memory only — restart resets the counter. RFC §5
	// notes the rationale: disk-persistence inverts the threat model
	// (attacker with disk access can wipe the counter).
	lockouts map[string]*lockoutEntry

	// serverkey is the session-token issuer Unlock invokes after a
	// successful decrypt. Wired via SetServerKey at construction
	// time (cmd/lthn/app.go) so the same serverkey instance the
	// bearer middleware verifies tokens against is the one that
	// minted them.
	serverkey SessionTokenIssuer
}

// NewService constructs the account service. The core handle is
// stored for future event-bus emission (e.g. "account.created" once
// the cooperative task queue plugs in) but is not required for
// Create today — pass nil from tests that don't care.
//
// Maps are pre-allocated so the first Unlock / Lock call doesn't
// race against a nil map; SetServerKey wires the session-token
// issuer separately (cmd/lthn/app.go calls it after Bootstrap).
//
// Usage example:
//
//	svc := account.NewService(c)
//	svc.SetServerKey(serverkeySvc)
func NewService(c *core.Core) *Service {
	return &Service{
		core:     c,
		unlocked: map[string][]byte{},
		lockouts: map[string]*lockoutEntry{},
	}
}

// Register is the canonical core.WithName-compatible factory. Wired
// in cmd/lthn/app.go so the service is reachable via
// core.ServiceFor[*Service](c, "account") — and the server picks
// up its API routes via pkg/server's RoutesProvider auto-discovery.
//
// Usage example:
//
//	core.WithName("account", account.Register)
func Register(c *core.Core) core.Result {
	return core.Ok(NewService(c))
}

// ServiceName labels the binding namespace exposed to JS. The Wails
// surface is currently empty (the only consumer is the REST endpoint),
// but registering the service via application.NewService lets future
// Wails bindings land at frontend-ng/bindings/dappco.re/lthn/desktop/pkg/account/.
func (s *Service) ServiceName() string { return "Account" }

// ServiceStartup is the Wails3 lifecycle hook; no-op (account
// creation is request-scoped — there is no boot-time work).
func (s *Service) ServiceStartup(_ core.Context, _ any) core.Result {
	return core.Ok(nil)
}

// ServiceShutdown clears the in-memory unlocked private keys so a
// shutdown leaves no decrypted material in process memory.
// Lockouts cleared too — the next process incarnation starts fresh
// (RFC §5 — disk-persisted counters would invert the threat model).
func (s *Service) ServiceShutdown() core.Result {
	s.mu.Lock()
	s.unlocked = nil
	s.lockouts = nil
	s.mu.Unlock()
	return core.Ok(nil)
}

// Create executes the four-MUST-NOT contract per RFC §2.5 (Cerberus
// #1460) and writes the on-disk account atomically. Returns
// core.Ok(CreateOutput) on success; core.Fail with a code from the
// "account.*" namespace on any failure so routes.go can shape the
// HTTP response without leaking internal state.
//
// Contract — every one of these MUST hold (tests in service_test.go
// pin them as Triadic Good / Bad / Ugly):
//
//  1. Cerberus #1460 (a) — refuse to overwrite. If
//     ~/Lethean/account/<id>/private.key exists at call time, return
//     code "account.exists" with no disk write.
//  2. Cerberus #1460 (b) — recompute the canonical account ID from
//     PublicKey via deriveAccountID and reject if input.AccountID
//     differs. Returns "account.id_mismatch" with no disk write.
//  3. Cerberus #1460 (c) — atomic write. public.key + meta.json +
//     private.key all write as <file>.tmp, fsync, rename. private.key
//     renames LAST so a crash between renames leaves AccountStatus
//     reporting has_user_account=false (#1471 leaf-file rule).
//  4. Cerberus #1460 (d) — nonce-consumption-before-write. NOT
//     enforced inside Create because pkg/server.BootstrapAuthMiddleware
//     calls serverkey.VerifyBootstrapToken BEFORE dispatching to the
//     gin handler that invokes Create. VerifyBootstrapToken adds the
//     nonce to the consumed-set before returning OK (see
//     pkg/serverkey/token.go around line 195). Net: by the time Create
//     runs, the nonce is burned, even if Create later returns an
//     error from a failed disk write — replay defence holds. The
//     code comment in routes.go reiterates this so a future reader
//     doesn't add a duplicate burn here.
//
// Usage example:
//
//	r := svc.Create(account.CreateInput{
//	    PublicKey: armouredPub,
//	    AccountID: "abc123def4567890",
//	})
//	if r.OK { out := r.Value.(account.CreateOutput); _ = out.Path }
func (s *Service) Create(input CreateInput) core.Result {
	if len(input.PublicKey) == 0 {
		return core.Fail(core.NewCode(codePublicKeyRequired, "public_key is required"))
	}
	if input.AccountID == "" {
		return core.Fail(core.NewCode(codeAccountIDRequired, "account_id is required"))
	}

	// Cerberus #1460 (b) — recompute canonical ID from the supplied
	// public key. Mismatch with the supplied AccountID is rejected
	// BEFORE any filesystem call so a malicious client can't steer
	// the install into an attacker-controlled directory.
	canonical := deriveAccountID(input.PublicKey)
	if canonical != input.AccountID {
		return core.Fail(core.NewCode(codeAccountIDMismatch,
			"supplied account_id does not match SHA-256 fingerprint of public_key"))
	}

	rootR := paths.Root()
	if !rootR.OK {
		return rootR
	}
	root, _ := rootR.Value.(string)

	accountRoot := core.PathJoin(root, accountDir)
	if r := core.MkdirAll(accountRoot, dirMode); !r.OK {
		return core.Fail(core.E("account.Create", "mkdir account root", r.Value.(error)))
	}

	dir := core.PathJoin(accountRoot, canonical)
	privatePath := core.PathJoin(dir, privateKeyFile)

	// Cerberus #1460 (a) — refuse to overwrite. The leaf signal is
	// private.key (#1471) — checking it (rather than the directory)
	// matches the AccountStatus rule, so a partial directory from a
	// prior aborted Create is correctly seen as "no account here yet"
	// and the retry can proceed.
	if core.Stat(privatePath).OK {
		return core.Fail(core.NewCode(codeAccountExists,
			"an account already exists at this id; refusing to overwrite"))
	}

	if r := core.MkdirAll(dir, dirMode); !r.OK {
		return core.Fail(core.E("account.Create", "mkdir account dir", r.Value.(error)))
	}

	// Build the metadata blob FIRST so a marshal failure aborts
	// before any rename happens.
	meta := map[string]any{
		"account_id": canonical,
		"created_at": core.Now().UTC().Unix(),
		"version":    1,
	}
	metaR := core.JSONMarshal(meta)
	if !metaR.OK {
		return core.Fail(core.E("account.Create", "marshal meta", metaR.Value.(error)))
	}
	metaBytes, _ := metaR.Value.([]byte)

	// Cerberus #1460 (c) — atomic three-file write with private.key
	// LAST. Each file writes as <file>.tmp + fsync + rename in
	// declaration order. If the process crashes between rename #2
	// (meta.json) and rename #3 (private.key), AccountStatus reads
	// the directory but does not find private.key → returns
	// has_user_account=false. The next Create call then sees no
	// private.key, refuses by the existing-account rule only if it's
	// present, and re-runs the write. No half-account state escapes.
	publicPath := core.PathJoin(dir, publicKeyFile)
	metaPath := core.PathJoin(dir, metaFile)

	// Mantis #1578 — cutover from local atomicWrite to the canonical
	// paths.AtomicWriteWithVersion substrate (RFC.atomicwrite-migration
	// §1 + §3). Per-call WriteInput is the minimal first-write shape
	// (no IfVersion / IfMtime / IfMatchHash — every account write is
	// first-write only; refuse-to-overwrite gates above on the leaf).
	// The primitive's at-rest mode-verify gate (Cerberus #19 §5.1
	// Option C, Mantis #1592) automatically fires for the "account/"
	// prefix and replaces the local atomicWrite's post-rename mode
	// check verbatim — same defence, owned in one place.
	if r := paths.AtomicWriteWithVersion(publicPath, paths.WriteInput{Body: input.PublicKey}); !r.OK {
		return core.Fail(core.NewCode(codeAccountWriteFailed,
			"writing public.key failed: "+r.Error()))
	}
	if r := paths.AtomicWriteWithVersion(metaPath, paths.WriteInput{Body: metaBytes}); !r.OK {
		// public.key has already landed but meta.json failed — the
		// directory is still half-built (no private.key), so the
		// next Create call sees has_user_account=false and can
		// re-attempt cleanly.
		return core.Fail(core.NewCode(codeAccountWriteFailed,
			"writing meta.json failed: "+r.Error()))
	}
	// private.key is the LEAF signal — must rename LAST so the
	// directory only flips to has_user_account=true atomically.
	// Note: the input doesn't carry the private key (the client
	// holds it; the server stores only the encrypted blob the user
	// later submits). For Stage B' we persist a marker shape that
	// future passphrase-unlock flows can rewrite. The marker is the
	// hex SHA-256 of the public key — a deterministic, recoverable
	// placeholder that is NOT a real PGP private block. The frontend
	// is expected to follow up with a PUT /v1/account/<id>/seal call
	// (out of scope for Stage B', tracked in RFC §7) that replaces
	// this marker with the encrypted private blob.
	//
	// SECURITY-NOTE (Mantis #1578 §5.3): the refuse-to-overwrite stat
	// at line 183 is NOT inside the primitive's per-path WithFileLock.
	// Moving it inside would require either an IfNotExist condition
	// on WriteInput (primitive surface change) or wrapping
	// AtomicWriteWithVersion in an outer WithFileLock on the same
	// path — but WithFileLock is documented non-reentrant (lock.go:93)
	// so that deadlocks. Shipping per RFC §7 escape valve: cutover
	// proceeds, residual TOCTOU window matches pre-cutover behaviour
	// (primitive's lock strictly narrows it, never widens). Follow-up
	// ticket filed to add IfNotExist / IfCreateOnly to WriteInput.
	marker := []byte(core.SHA256HexString(string(input.PublicKey)))
	if r := paths.AtomicWriteWithVersion(privatePath, paths.WriteInput{Body: marker}); !r.OK {
		return core.Fail(core.NewCode(codeAccountWriteFailed,
			"writing private.key failed: "+r.Error()))
	}

	// Mantis #1574 (MED) / Cerberus #13 — emit auth.account.created
	// post-success. Sibling of Provision's auth.account.provisioned
	// emission (provision.go:225) — Create owns the legacy two-step
	// onboarding flow; Provision the consolidated one-shot. Both land
	// the same audit row shape so the Operations panel's pivot counts
	// stay coherent across either flow.
	//
	// Stage F.B Phase 2.4 parity-grep contract (RFC §3.3.1) — the
	// core.Print debug-echo MUST appear alongside the typed Record
	// emission so an operator tailing stderr sees the same event name
	// the audit log carries.
	//
	// Cerberus #1465 closure-only-scope discipline — the canonical
	// public-key bytes are NEVER in Meta; only the SHA-256 hex of the
	// account directory path so an audit reader can correlate across
	// rows without exfiltrating filesystem layout (username / install
	// location) verbatim. RequestID is already plumbed by routes.go:148
	// (server-generated, caller input.RequestID is dropped per Cerberus
	// #1524).
	//
	// Recorder write failure ignored — audit failures MUST NEVER block
	// the auth path, especially not the just-succeeded create.
	now := core.Now().UTC().Unix()
	pathHash := core.SHA256HexString(dir)
	core.Print(core.Stderr(),
		"event=auth.account.created account_id=%s ts=%d\n",
		canonical, now)
	_ = audit.Default().Record(audit.Event{
		Event:     audit.EventAuthAccountCreated,
		AccountID: canonical,
		TS:        now,
		Scope:     "account.create",
		Outcome:   audit.OutcomeOK,
		RequestID: input.RequestID,
		Meta: map[string]any{
			"path_hash": pathHash,
		},
	})

	return core.Ok(CreateOutput{
		AccountID: canonical,
		Path:      dir,
	})
}

// AccountStatus is a convenience wrapper that defers to
// serverkey.Service.AccountStatus via the file-system check. Today's
// REST surface doesn't expose it (the Wails binding on
// pkg/serverkey is the canonical reader per RFC §1); the method is
// here so future REST consumers can resolve it without adding a
// circular dependency on pkg/serverkey from a handler-only package.
//
// Usage example:
//
//	r := svc.AccountStatus()
//	if r.OK { st := r.Value.(AccountStatus); _ = st.HasAccount }
func (s *Service) AccountStatus() core.Result {
	rootR := paths.Root()
	if !rootR.OK {
		return rootR
	}
	root, _ := rootR.Value.(string)
	accountRoot := core.PathJoin(root, accountDir)
	if !core.Stat(accountRoot).OK {
		return core.Ok(AccountStatus{HasAccount: false})
	}
	entriesR := core.ReadDir(core.DirFS(accountRoot), ".")
	if !entriesR.OK {
		return core.Ok(AccountStatus{HasAccount: false})
	}
	entries, _ := entriesR.Value.([]core.FsDirEntry)
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		leaf := core.PathJoin(accountRoot, e.Name(), privateKeyFile)
		if core.Stat(leaf).OK {
			return core.Ok(AccountStatus{HasAccount: true, AccountID: e.Name()})
		}
	}
	return core.Ok(AccountStatus{HasAccount: false})
}

// deriveAccountID computes the canonical account ID from a public
// key: hex(SHA-256(public_key))[:accountIDLength]. Used by Create
// (canonical recomputation for the #1460 (b) mismatch check) AND
// exported via DeriveAccountID for clients (frontend tests, future
// CLI tools) that need to mirror the derivation. 16 hex chars = 64
// bits of keyspace — enough collision margin for the one-Mac-many-
// years-handful-of-accounts use-case.
//
// Documented inline (not in a separate /docs file) per Snider's
// "comments as usage examples" convention.
//
// Usage example:
//
//	id := account.DeriveAccountID(pubBytes)
//	_ = id  // "abc123def4567890" (16 hex chars)
func deriveAccountID(publicKey []byte) string {
	full := core.SHA256HexString(string(publicKey))
	if len(full) < accountIDLength {
		return full
	}
	return full[:accountIDLength]
}

// DeriveAccountID is the exported wrapper around deriveAccountID for
// callers outside this package (e.g. test fixtures, future client
// utilities). The unexported helper stays internal so the package
// can change the derivation without breaking external callers (the
// derivation is part of the public contract per RFC §2.5; bumping
// it is a deliberate spec change, not an implementation tweak).
//
// Usage example:
//
//	id := account.DeriveAccountID(pubBytes)
//	in := account.CreateInput{PublicKey: pubBytes, AccountID: id}
func DeriveAccountID(publicKey []byte) string {
	return deriveAccountID(publicKey)
}

// Mantis #1578 — the local atomicWrite helper has been removed in
// favour of paths.AtomicWriteWithVersion (Cerberus #1464 mode-verify
// regression closed by primitive's at-rest mode-verify gate per
// Mantis #1592 / Cerberus #19 §5.1 Option C). All three Create writes
// + all three Provision writes route through the primitive in
// declaration order; the leaf-last invariant (Cerberus #1471) lives
// in the call-site sequence, not the primitive.

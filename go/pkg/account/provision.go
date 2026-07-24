// SPDX-Licence-Identifier: EUPL-1.2

package account

import (
	"crypto"

	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/audit"
	"dappco.re/lthn/desktop/pkg/paths"
	"dappco.re/lthn/desktop/pkg/serverkey"
	"github.com/Snider/Enchantrix/pkg/crypt/std/pgp"
	"github.com/ProtonMail/go-crypto/openpgp/packet"
	"github.com/ProtonMail/go-crypto/openpgp/s2k"
)

// Provision generates a fresh Lethean Account on this Mac per
// RFC.stage-x.md §3 — Ed25519 keypair via Enchantrix, AES-256
// symmetric-encryption of the private key with the user-typed
// passphrase, atomic three-file write under ~/Lethean/account/<id>/,
// and an immediately-issued session token so the frontend can flip
// straight to state=ok after the response.
//
// Stage X.B Phase 2a happy-path scope:
//
//   - Passphrase NFKC normalised (mirrors Phase 1's unlock-side fix)
//   - Ed25519 + Curve25519 keypair (Cerberus DREAD HIGH-1)
//   - AES-256-CFB + S2K iteration count 65011712 (Cerberus DREAD HIGH-2)
//   - Atomic three-file write — public.key + meta.json + private.key
//     in that order, private.key as the LEAF signal per Cerberus #1471
//   - Refuse-to-overwrite guard (Cerberus #1460 (a))
//   - Session token auto-issued via SessionTokenIssuer
//   - Passphrase + plaintext private bytes zeroed with runtime.KeepAlive
//
// Deferred to Phase 2b/2c/2d (tracked in RFC.stage-x.md §10):
//
//   - 256-char raw length cap BEFORE NFKC (Q6 Unicode-expansion)
//   - HIBP common-passphrase check via pkg/passphrase (HIGH-3)
//   - request_id 5-minute idempotency dedup (Q5)
//   - Null-byte rejection (LOW)
//   - Strength meter / weak / common code surface
//
// Stage F.B Phase 1 wiring landed:
//
//   - auth.account.provisioned typed event via pkg/audit.Default (this
//     file, post-mint emit below)
//   - auth.session.issued typed event via pkg/serverkey/token.go cross-
//     cut (commit 50678bb) — fires from the IssueSessionToken call
//     below, NOT duplicated at the Provision site.
//
// Usage example:
//
//	r := svc.Provision(account.ProvisionInput{
//	    Passphrase: "correct horse battery staple",
//	    RequestID:  uuidv4(),
//	})
//	if r.OK {
//	    out := r.Value.(account.ProvisionOutput)
//	    _ = out.SessionToken
//	}
func (s *Service) Provision(input ProvisionInput) core.Result {
	if input.Passphrase == "" {
		return core.Fail(core.NewCode(codeProvisionPassphraseRequired,
			"passphrase is required"))
	}
	if s.serverkey == nil {
		return core.Fail(core.NewCode(codeProvisionMisconfigured,
			"session-token issuer not wired — call SetServerKey before Provision"))
	}

	// NFKC normalise so the same logical passphrase typed on any OS
	// keyboard encrypts to the same bytes — the create-side mirror of
	// Stage X.B Phase 1's unlock-side patch (RFC.stage-x.md §6).
	// nfkcPassphrase + zeroAndKeepAlive live in passnorm.go.
	passphrase := nfkcPassphrase(input.Passphrase)
	defer zeroAndKeepAlive(passphrase)

	rootR := paths.Root()
	if !rootR.OK {
		return rootR
	}
	root, _ := rootR.Value.(string)

	accountRoot := core.PathJoin(root, accountDir)
	if r := core.MkdirAll(accountRoot, dirMode); !r.OK {
		return core.Fail(core.E("account.Provision", "mkdir account root", r.Value.(error)))
	}

	// Cerberus DREAD HIGH-1 — Ed25519 + Curve25519. Enchantrix's
	// GenerateKeyPairWithConfig with a nil config falls back to
	// RSA-2048 (the openpgp library default). Modern callers MUST
	// pass an explicit config.
	pgpSvc := pgp.NewService()
	pubArmoured, privPlain, err := pgpSvc.GenerateKeyPairWithConfig(
		"user", "user@lthn.local", "first-run",
		&packet.Config{
			Algorithm: packet.PubKeyAlgoEdDSA,
			Curve:     packet.Curve25519,
		},
	)
	if err != nil {
		return core.Fail(core.NewCode(codeProvisionKeygenFailed,
			"keypair generation failed: "+err.Error()))
	}
	defer zeroAndKeepAlive(privPlain)

	canonical := deriveAccountID(pubArmoured)
	dir := core.PathJoin(accountRoot, canonical)
	privatePath := core.PathJoin(dir, privateKeyFile)

	// Cerberus #1460 (a) — refuse-to-overwrite. Two Provision calls
	// produce different keypairs (crypto/rand) so the collision is
	// vanishingly rare in practice, but the leaf-file guard mirrors
	// Create's posture so a partial-write retry can't silently land
	// a second account under the same id.
	if core.Stat(privatePath).OK {
		return core.Fail(core.NewCode(codeAccountExists,
			"an account already exists at this id; refusing to overwrite"))
	}

	if r := core.MkdirAll(dir, dirMode); !r.OK {
		return core.Fail(core.E("account.Provision", "mkdir account dir", r.Value.(error)))
	}

	// Cerberus DREAD HIGH-2 — AES-256 + iterated S2K at the highest
	// standard count. SymmetricallyEncryptWithConfig with a nil
	// config defaults to AES-128 + 16777216 iterations; modern
	// callers MUST pass an explicit config.
	encPriv, err := pgpSvc.SymmetricallyEncryptWithConfig(passphrase, privPlain, &packet.Config{
		DefaultCipher: packet.CipherAES256,
		S2KConfig: &s2k.Config{
			Hash:     crypto.SHA256,
			S2KCount: 65011712,
		},
	})
	if err != nil {
		return core.Fail(core.NewCode(codeProvisionEncryptFailed,
			"symmetric encryption of private key failed: "+err.Error()))
	}

	// Build the meta blob FIRST so a marshal failure aborts before
	// any rename happens (same posture as Create — type=provision
	// distinguishes from Create's type=create meta in future audit).
	meta := map[string]any{
		"account_id": canonical,
		"created_at": core.Now().UTC().Unix(),
		"version":    1,
		"type":       "provision",
	}
	metaR := core.JSONMarshal(meta)
	if !metaR.OK {
		return core.Fail(core.E("account.Provision", "marshal meta", metaR.Value.(error)))
	}
	metaBytes, _ := metaR.Value.([]byte)

	// Atomic three-file write with private.key LAST per Cerberus
	// #1471 — AccountStatus's leaf signal flips to true atomically.
	// A crash between any pair of renames leaves the directory
	// half-built without private.key; AccountStatus reads
	// has_user_account=false and the next Provision call can retry.
	publicPath := core.PathJoin(dir, publicKeyFile)
	metaPath := core.PathJoin(dir, metaFile)

	// Mantis #1578 cutover (mirror of Service.Create §3) — all three
	// writes route through paths.AtomicWriteWithVersion in declaration
	// order with private.key LAST per Cerberus #1471 leaf-rule. The
	// primitive's at-rest mode-verify gate (Cerberus #19 §5.1 Option C,
	// Mantis #1592) fires for the "account/" prefix and inherits the
	// #1464 mode-tamper defence verbatim.
	if r := paths.AtomicWriteWithVersion(publicPath, paths.WriteInput{Body: pubArmoured}); !r.OK {
		return core.Fail(core.NewCode(codeAccountWriteFailed,
			"writing public.key failed: "+r.Error()))
	}
	if r := paths.AtomicWriteWithVersion(metaPath, paths.WriteInput{Body: metaBytes}); !r.OK {
		return core.Fail(core.NewCode(codeAccountWriteFailed,
			"writing meta.json failed: "+r.Error()))
	}
	// LEAF — encrypted private key, NOT the SHA-256 marker Stage B'
	// Create wrote. Once this rename lands AccountStatus reports
	// has_user_account=true and the unlock path can decrypt against
	// the supplied passphrase.
	if r := paths.AtomicWriteWithVersion(privatePath, paths.WriteInput{Body: encPriv}); !r.OK {
		return core.Fail(core.NewCode(codeAccountWriteFailed,
			"writing private.key failed: "+r.Error()))
	}

	// Issue a session token immediately so the frontend can flip
	// state=ok in one round-trip (RFC.stage-x.md §3 step 11). Failure
	// here leaves the account on disk in a valid state — the user
	// can simply reload + unlock — but the response surfaces the
	// distinct code so the UX can guide them to retry.
	// Thread input.RequestID through so the emitted
	// auth.session.issued event carries the same join-key the
	// auth.account.provisioned event uses (Cerberus #1711 Phase 2.5
	// punch-list — request_id threading).
	tokR := s.serverkey.IssueSessionTokenWithRequest(canonical, input.RequestID)
	if !tokR.OK {
		return core.Fail(core.NewCode(codeProvisionSessionMintFailed,
			"account written but session-token mint failed: "+tokR.Error()))
	}
	out, _ := tokR.Value.(serverkey.SessionTokenOutput)
	if out.Token == "" {
		return core.Fail(core.NewCode(codeProvisionSessionMintFailed,
			"session-token issuer returned an empty token"))
	}

	// Stage F.B Phase 1 — emit auth.account.provisioned per
	// RFC.stage-x.md §3 step 11 Q2 ruling. The sibling auth.session.issued
	// event ALSO fires for this provision — but it fires from
	// serverkey.IssueSessionToken (commit 50678bb cross-cut), NOT here.
	// Emitting it from Provision too would land a duplicate line for the
	// same logical session-mint event, polluting the audit-tail's pivot
	// counts. The forensic record still shows BOTH events because the
	// serverkey emission lands first (during the tokR call above) and
	// the provisioned emission lands here — they share the same
	// account_id + adjacent timestamps so a future Query for "what
	// happened during this account creation" sees the pair atomically.
	//
	// Recorder write failure ignored — audit failures MUST NEVER block
	// the auth path, especially not the just-succeeded provision.
	//
	// Stage F.B Phase 2.4 parity-grep contract (RFC §3.3.1) — the
	// core.Print debug-echo MUST appear alongside the typed Record
	// emission so an operator tailing stderr sees the same event-name
	// the audit log carries. Same dual-emit shape used by the unlock /
	// lockout / lock / session.issued / session.verify_failed paths.
	//
	// Meta carries path_hash (SHA-256 hex of the canonical account
	// directory) — NEVER the raw path. Cerberus #1465 closure-only
	// scope discipline applies to filesystem layout (username /
	// install location) too. Mirrors Service.Create's emit-site shape
	// verbatim per the parity-grep contract (commit 7feaa8b, #1574).
	now := core.Now().UTC().Unix()
	pathHash := core.SHA256HexString(dir)
	core.Print(core.Stderr(),
		"event=auth.account.provisioned account_id=%s ts=%d\n",
		canonical, now)
	_ = audit.Default().Record(audit.Event{
		Event:     audit.EventAuthAccountProvisioned,
		AccountID: canonical,
		TS:        now,
		Scope:     "account.create",
		Outcome:   audit.OutcomeOK,
		RequestID: input.RequestID,
		Meta: map[string]any{
			"path_hash": pathHash,
		},
	})

	return core.Ok(ProvisionOutput{
		AccountID:    canonical,
		Path:         dir,
		SessionToken: out.Token,
		ExpiresAt:    out.ExpiresAt,
		DedupHit:     false, // Phase 2c will toggle this on idempotent replay.
	})
}

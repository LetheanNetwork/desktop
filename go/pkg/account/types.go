// SPDX-Licence-Identifier: EUPL-1.2

// Package account owns the /v1/account/create endpoint handler that
// closes the seam left open by Stage B (Mantis #1474). The endpoint is
// the only consumer of bootstrap-tokens minted by pkg/serverkey, and
// its contract is load-bearing per Cerberus #1460:
//
//   - MUST NOT overwrite an existing account (returns
//     core.NewCode("account.exists", ...) → HTTP 409).
//   - MUST recompute the account ID from the supplied public key and
//     reject if the supplied AccountID mismatches
//     (core.NewCode("account.id_mismatch", ...) → HTTP 400).
//   - MUST consume the bootstrap-token nonce BEFORE any disk write so
//     a failed write still burns the nonce (replay defence holds even
//     on partial failure).
//   - MUST write atomically. public.key + meta.json + private.key all
//     write as `<file>.tmp`, fsync, then rename. private.key renames
//     LAST so a crash mid-write leaves AccountStatus() reporting
//     has_user_account=false (per Cerberus #1471 the leaf signal is
//     private.key, not the directory).
//
// On-disk layout (Snider canon — visible, never hidden):
//
//	~/Lethean/account/<id>/public.key      armoured PGP public block. Mode 0600.
//	~/Lethean/account/<id>/private.key     armoured PGP private block. Mode 0600.
//	                                       Written LAST — leaf signal for AccountStatus.
//	~/Lethean/account/<id>/meta.json       creation metadata. Mode 0600.
//
// Account ID derivation (documented inline so future readers don't
// need to git-blame to discover the algorithm): the account ID is
// hex(SHA-256(public_key))[0:16] — first 16 hex characters of the
// SHA-256 fingerprint of the supplied public-key bytes. 64 bits of
// keyspace gives near-zero collision risk for the desktop use-case
// (one Mac = a handful of accounts, ever) while keeping the on-disk
// directory name short enough to type. The deterministic derivation
// lets the endpoint reject mismatched supplied IDs without server-
// side ID storage (Cerberus #1460 (b)).
//
// Usage example:
//
//	svc := account.NewService(c)
//	r := svc.Create(account.CreateInput{
//	    PublicKey: pubBytes,
//	    AccountID: "abc123def4567890",
//	})
//	if r.OK {
//	    out := r.Value.(account.CreateOutput)
//	    _ = out.AccountID  // canonical (recomputed) id
//	    _ = out.Path       // ~/Lethean/account/<id>/
//	}
//
// Coding constraints (same as Stage B):
//   - All errors via core.E() / core.NewCode().
//   - All I/O through core.* wrappers (no os / path/filepath / strings).
//   - Atomic writes mirror pkg/serverkey's atomicWrite helper pattern.
//   - File mode 0o600 verified at OPEN-time (Cerberus #1464 carried
//     forward from pkg/serverkey).

package account

import (
	core "dappco.re/go"
)

// CreateInput is the request body the frontend setup flow POSTs to
// /v1/account/create. PublicKey is the armoured PGP public block the
// client generated. AccountID is the client-computed ID for the same
// bytes — the server recomputes it and rejects a mismatch (Cerberus
// #1460 (b)) so a malicious client can't steer the install into a
// directory of its choosing.
//
// Usage example:
//
//	in := account.CreateInput{
//	    PublicKey: armouredPub,
//	    AccountID: "abc123def4567890",
//	}
type CreateInput struct {
	// PublicKey is the armoured PGP public-key block. Required; an
	// empty value is rejected with code "account.public_key.required".
	PublicKey []byte `json:"public_key"`

	// AccountID is the client-computed ID. Required; an empty value is
	// rejected with code "account.id.required". The server recomputes
	// the canonical ID from PublicKey and rejects mismatches with
	// "account.id_mismatch" (Cerberus #1460 (b)).
	AccountID string `json:"account_id"`
}

// CreateOutput is the success-shape returned on a 200 OK. AccountID
// is the canonical (server-recomputed) ID — always equal to the
// requested AccountID because we reject mismatches before reaching
// the persist path. Path is the absolute on-disk directory so the
// frontend can surface "your account lives at …" copy if it wants.
//
// Usage example:
//
//	out := r.Value.(account.CreateOutput)
//	core.Print(core.Stdout(), "wrote account %s at %s\n", out.AccountID, out.Path)
type CreateOutput struct {
	AccountID string `json:"account_id"`
	Path      string `json:"path"`
}

// AccountStatus is a lightweight, JSON-shaped projection of the
// underlying serverkey.AccountStatusOutput for callers that want to
// consume it as a package-local type. Today's Stage B' shipping
// surface delegates AccountStatus() rendering to pkg/serverkey via
// the existing Wails binding, so this type exists only as a
// canonical shape for future REST exposure (the spec's §1 trigger
// flow today reads AccountStatus over the Wails binding, not REST).
//
// Usage example:
//
//	st := account.AccountStatus{HasAccount: true, AccountID: "abc123"}
//	_ = st
type AccountStatus struct {
	HasAccount bool   `json:"has_account"`
	AccountID  string `json:"account_id,omitempty"`
}

// File modes — declared at package scope so service.go / routes.go
// all reference the same value. 0o600 is load-bearing for Cerberus
// #1464 (open-time mode verification carried forward).
const (
	dirMode  core.FileMode = 0o700
	fileMode core.FileMode = 0o600
)

// File names — declared at package scope so service.go and any
// future writers stay aligned. private.key MUST be written last per
// Cerberus #1471 (it is the leaf signal AccountStatus reads).
const (
	publicKeyFile  = "public.key"
	privateKeyFile = "private.key"
	metaFile       = "meta.json"
)

// Container directory under ~/Lethean/. Mirrors pkg/serverkey's
// accountDir constant so callers writing through both packages stay
// in lockstep.
const accountDir = "account"

// accountIDLength is the byte-length (hex chars) of the canonical
// account ID — first 16 hex chars of SHA-256(public_key). 64 bits
// of keyspace for the desktop use-case (one Mac, handful of accounts
// ever). Trimming early keeps the directory name short enough to
// type while leaving no collision risk in practice.
const accountIDLength = 16

// Error codes — declared at package scope so tests, handlers, and
// future loaders consume the same literal. The "account.*" namespace
// is the public contract the frontend keys error UI off (e.g. the
// auth-gate's "account already exists" branch reads error.code ==
// "account.exists"). Adding a new code requires updating the
// frontend's known-codes set in the same commit.
const (
	codeAccountExists       = "account.exists"
	codeAccountIDMismatch   = "account.id_mismatch"
	codeAccountIDRequired   = "account.id.required"
	codePublicKeyRequired   = "account.public_key.required"
	codeAccountWriteFailed  = "account.write_failed"
	codeAccountInvalidBody  = "account.invalid_body"
)

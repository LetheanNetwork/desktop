// SPDX-Licence-Identifier: EUPL-1.2

// atrest_keys.go — shared substrate adapter that satisfies
// recordfile.AccountKeys against a consumer's SessionGate +
// AccountKeyProvider runtime-assertion pair. Extracted from the
// wave 1/wave 2 consumer-side copies (sales/deals, incidents,
// runbooks) per Cerberus #44 PRBW F-3 to eliminate 3x duplication
// before wave 3 (marketing 4 sub-pkgs) lands.
//
// Why this lives in pkg/account: each consumer's per-package
// `accountKeyProvider` interface returns the CONCRETE
// `*account.PrivateKeyHandle` type, which makes any shared adapter
// that ABSTRACTS the key-lookup contract obliged to import this
// package. pkg/account already owns PrivateKeyHandle and is the
// natural home for the at-rest substrate seam.
//
// The adapter is `scope`-parameterised so each consumer's typed
// error codes stay grep-stable (`<scope>.atrest.public_key_unavailable`,
// `<scope>.no_unlocked_account`, `<scope>.multi_account_not_supported`).
//
// Usage example (sales/deals consumer):
//
//	keys := account.NewAtRestKeys("deals", account.AtRestKeysDeps{
//	    Gate: dealsSvc.gate,           // satisfies SessionIDs
//	    Keys: kpProvider,              // satisfies AccountKeyProvider
//	})
//	w := recordfile.NewAtRestWriter(recordfile.AtRestDeps[DealRecord]{
//	    Keys: keys,
//	    ...
//	})

package account

import (
	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/recordfile"
)

// SessionIDs is the narrow live-read interface every writer-package
// SessionGate already satisfies (RFC.stage-e-unlockgate v2 §1.1).
// Defined here as a SHARED contract so the substrate-side adapter can
// consume any consumer's gate uniformly.
//
// Mirrors the per-consumer SessionGate exactly — duplication ELIMINATED
// at the substrate boundary; per-consumer SessionGate interfaces stay
// as the AX-8 producer-contract surface.
type SessionIDs interface {
	// UnlockedAccountIDs returns the set of currently-unlocked account
	// IDs. Empty slice means the session is locked.
	UnlockedAccountIDs() []string
}

// AccountKeyProvider is the wider runtime-assertion surface the
// at-rest path expects on a wired SessionGate. *account.Service
// satisfies it today (PublicKeyFor at unlock.go:903, PrivateKeyFor at
// unlock.go:870). Test fixtures stub it independently — see each
// consumer's incidents_test.go / atrest_test.go.
//
// The shape is the consumer-side type that each writer pkg already
// defines per-package; centralising here removes the 3x duplication
// (Cerberus #44 F-3) while preserving the runtime-assertion contract.
type AccountKeyProvider interface {
	PublicKeyFor(accountID string) ([]byte, bool)
	PrivateKeyFor(accountID string) (*PrivateKeyHandle, bool)
}

// AtRestKeysDeps bundles the wired collaborators NewAtRestKeys
// composes into a recordfile.AccountKeys implementation.
type AtRestKeysDeps struct {
	// Gate is the live-read session source — typically the consumer's
	// SessionGate which holds a reference to *account.Service.
	Gate SessionIDs

	// Keys is the wider runtime-assertion surface that exposes
	// PublicKeyFor / PrivateKeyFor. Typically *account.Service via the
	// consumer-side type-assertion miss-it-skip-it pattern.
	Keys AccountKeyProvider
}

// NewAtRestKeys returns a recordfile.AccountKeys that bridges the
// consumer's SessionGate + AccountKeyProvider pair onto the substrate
// contract. The (bytes, bool) → (bytes, error) shape rewrite is
// applied for both PublicKeyFor (substrate's len(pub)==0 check has
// structured context) and PrivateKeyFor (bridges concrete
// *PrivateKeyHandle onto the substrate's interface).
//
// SingleUnlockedAccount mirrors office/mail.singleUnlockedAccount
// semantics (Mantis #1591) — zero unlocked → typed
// "<scope>.no_unlocked_account"; multiple unlocked → typed
// "<scope>.multi_account_not_supported".
//
// `scope` becomes the prefix on every typed error code the adapter
// emits. Empty `scope` falls back to "atrest".
//
// Usage example:
//
//	keys := account.NewAtRestKeys("incidents", account.AtRestKeysDeps{
//	    Gate: incSvc.gate,
//	    Keys: kpProvider,
//	})
func NewAtRestKeys(scope string, deps AtRestKeysDeps) recordfile.AccountKeys {
	if scope == "" {
		scope = "atrest"
	}
	return atrestKeys{scope: scope, deps: deps}
}

// atrestKeys is the concrete implementation behind NewAtRestKeys.
// Kept package-private so callers compose via the constructor and
// the scope-prefix invariant is preserved.
type atrestKeys struct {
	scope string
	deps  AtRestKeysDeps
}

// PublicKeyFor wraps the gate's PublicKeyFor; the (bytes, bool) shape
// is rewritten as (bytes, error) per the substrate's contract. Missing
// keys produce a typed error rather than a silent empty slice so the
// substrate's len(pub)==0 check has structured context to report.
func (a atrestKeys) PublicKeyFor(accountID string) ([]byte, error) {
	pub, ok := a.deps.Keys.PublicKeyFor(accountID)
	if !ok {
		return nil, core.NewCode(a.scope+".atrest.public_key_unavailable",
			core.Sprintf("PublicKeyFor(%q) returned not-ok", accountID))
	}
	return pub, nil
}

// PrivateKeyFor wraps the producer's PrivateKeyFor and bridges the
// concrete *account.PrivateKeyHandle return onto the substrate's
// recordfile.PrivateKeyHandle interface. The concrete handle already
// satisfies the interface shape (Use(func([]byte) error) error) — the
// wrapper is just the type-conversion seam.
func (a atrestKeys) PrivateKeyFor(accountID string) (recordfile.PrivateKeyHandle, bool) {
	h, ok := a.deps.Keys.PrivateKeyFor(accountID)
	if !ok || h == nil {
		return nil, false
	}
	return h, true
}

// SingleUnlockedAccount mirrors office/mail.singleUnlockedAccount
// (Mantis #1591) against the consumer's gate. Zero unlocked returns
// "<scope>.no_unlocked_account"; multi-unlock returns
// "<scope>.multi_account_not_supported" — both typed so the substrate
// surfaces the consumer's scope back to the caller.
func (a atrestKeys) SingleUnlockedAccount() (string, error) {
	ids := a.deps.Gate.UnlockedAccountIDs()
	if len(ids) == 0 {
		return "", core.NewCode(a.scope+".no_unlocked_account",
			"no Lethean account is unlocked")
	}
	if len(ids) > 1 {
		return "", core.NewCode(a.scope+".multi_account_not_supported",
			core.Sprintf("%s does not yet support multiple unlocked accounts (Mantis #1591)", a.scope))
	}
	return ids[0], nil
}

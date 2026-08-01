// SPDX-Licence-Identifier: EUPL-1.2

// Package keysvc wires dappco.re/go/crypt/keys into this application.
//
// The sealed-key service itself no longer lives here — it was promoted
// into go-crypt so the next package that needs credentials inherits a
// mature store instead of growing a second one. What stays behind is
// the pair of answers only an application can give:
//
//   - where its files live — pkg/paths.KeysDir, still
//     ~/Lethean/data/keys, still created 0700 on demand;
//   - where its audit rows go — pkg/audit's process recorder, reached
//     lazily at emit time so a test swapping audit.Default is still
//     observed.
//
// Every construction site goes through Register (or New) so those two
// answers are made once, here, rather than restated at each call site
// where one of them could quietly drift.
package keysvc

import (
	core "dappco.re/go"
	"dappco.re/go/crypt/keys"

	"dappco.re/lthn/desktop/pkg/audit"
	"dappco.re/lthn/desktop/pkg/paths"
)

// AuditAdapter satisfies keys.AuditRecorder over this application's
// audit substrate. Pure translation — it adds no policy of its own, so
// a credential-mutation row lands in ~/Lethean/audit/ exactly as it did
// when the keys service imported pkg/audit directly.
//
// Usage example:
//
//	svc := keys.New(keys.WithAuditRecorder(keysvc.AuditAdapter{}))
type AuditAdapter struct{}

// Record forwards a credential-mutation row to the process recorder.
// audit.Default() is resolved per call, not cached, so a test that
// installs a fixture via audit.SetDefault sees the rows the keys
// service emits afterwards.
func (AuditAdapter) Record(ev keys.AuditEvent) core.Result {
	return audit.Default().Record(audit.Event{
		Event:   ev.Event,
		TS:      ev.TS,
		Scope:   ev.Scope,
		Outcome: ev.Outcome,
		Meta:    ev.Meta,
	})
}

// ErrorCode delegates to the audit package's codespace resolution.
// Delegating rather than reimplementing is the point of the seam: the
// codespace is what this application's forensic tooling and its
// frontend decoder join against, so it must have exactly one
// definition.
func (AuditAdapter) ErrorCode(r core.Result) string {
	return audit.ErrorCode(r)
}

// Options returns this application's answers to the keys service's two
// construction seams.
//
// Usage example:
//
//	svc := keys.New(keysvc.Options()...)
func Options() []keys.Option {
	return []keys.Option{
		keys.WithDirResolver(paths.KeysDir),
		keys.WithAuditRecorder(AuditAdapter{}),
	}
}

// New constructs the application's sealed-key service.
//
// Usage example:
//
//	r := keysvc.New()
//	if r.OK { svc := r.Value.(*keys.Service); _ = svc }
func New() core.Result {
	return keys.New(Options()...)
}

// Register is the core.WithName-compatible registrar for the
// application's sealed-key service.
//
// Usage example:
//
//	c := core.New(core.WithName("keys", keysvc.Register))
func Register(c *core.Core) core.Result {
	return keys.Register(c, Options()...)
}

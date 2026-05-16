// SPDX-Licence-Identifier: EUPL-1.2

// options.go — Phase 2 boot-time configuration for pkg/audit.Service.
// The Phase 1 lazy Default() singleton stays callable as a back-compat
// alias so existing emit-sites (pkg/account, pkg/serverkey) keep
// working; new boot wiring in cmd/lthn/main.go constructs a Service
// explicitly via audit.New(c, Options{...}) and registers it as the
// default sink via audit.SetDefault(svc).
//
// Options follow RFC.stage-f.md §3.1 verbatim. Defaults are calibrated
// for the single-user-desktop boot path — operators wanting a different
// audit root / retention window / HMAC secret override field-by-field.

package audit

import (
	core "dappco.re/go"
)

// Options is the boot-time configuration shape Service consumes. Every
// field is optional — zero-values map onto the documented default
// listed in the field comment. Pass an Options literal to audit.New;
// pass Options{} for "do the right thing on this machine."
//
// Usage example:
//
//	svc := audit.New(c, audit.Options{
//	    AuditSecret: serverkeySvc.AuditHMACSecret(),
//	    ProcessLabel: "lthn",
//	})
//	audit.SetDefault(svc)
type Options struct {
	// Root is the audit-log root directory. Default: "" → resolves to
	// ~/Lethean/audit/ via the paths package at New-time. Operators
	// running lthn-mlx as a sibling binary set ProcessLabel="lthn-mlx"
	// + Root="~/Lethean/audit/mlx" so the two processes' streams stay
	// separate per RFC §6.5.
	Root string

	// AuditSecret is the HMAC-SHA256 key used to hash account_id values
	// at-rest (RFC §6.4). Phase 2 sources this from
	// serverkey.AuditHMACSecret() which HKDFs over ~/Lethean/wallets/.seed.
	// Default: nil → falls back to a process-local random secret cached
	// for the lifetime of this Service instance. The random-fallback
	// path is intentionally noisy: a debug-echo via core.Print warns
	// the operator that account_id-keyed Query results will not
	// survive process restarts.
	AuditSecret []byte

	// ProcessLabel is the per-process discriminator stamped into
	// every Meta map under the reserved key "__process". Default:
	// "lthn". The lthn-mlx subsystem (RFC §6.5) sets this to
	// "lthn-mlx" so a unified Query across both stream roots can
	// distinguish which binary emitted each event.
	ProcessLabel string

	// KeepDays is the retention window before the rotation sweep
	// renames a `.log.gz` to `.log.gz.candidate`. Default: 90.
	// Setting to 0 retains forever (operator manually trims).
	KeepDays int

	// CompressOlderThan rotates uncompressed `.log` files older than
	// this many days into `.log.gz`. Default: 7. Setting to 0 skips
	// compression entirely (operator-friendly debug mode).
	CompressOlderThan int

	// RotationCheckInterval is how often the rotation goroutine wakes
	// to look for day-boundary crossings + retention candidates.
	// Default: 60s (RFC §4.2). Tests inject smaller intervals to
	// exercise rotation without waiting.
	RotationCheckInterval core.Duration

	// BatchFlushInterval is the cascade-mode periodic flush cadence.
	// Default: 200ms. RecordBatch writes land on disk immediately
	// (POSIX append is atomic per-write); this controls fsync()
	// cadence so a power loss strands at most this much cascade data.
	BatchFlushInterval core.Duration

	// BatchFlushEventCount is the cascade-mode flush threshold by
	// event count. Default: 100 events. Whichever of BatchFlushInterval
	// or BatchFlushEventCount trips first triggers the fsync.
	BatchFlushEventCount int

	// NoopDropWarningEvery controls how often the noop fallback emits
	// a "events still dropping" warning via core.Print (Cerberus
	// #1711 MED-2 — counter-based re-emit). Default: 100 dropped
	// events between warnings. Setting to 0 disables warnings entirely
	// (used in tests that intentionally exercise the noop sink).
	NoopDropWarningEvery int
}

// resolved applies defaults to a caller-supplied Options literal. Pure
// function — does NOT mutate the input. Returned struct is what the
// Service holds for its lifetime.
//
// Usage example (internal):
//
//	opts := resolved(in)
//	// opts.ProcessLabel is "lthn" if in.ProcessLabel was ""
func resolved(in Options) Options {
	out := in
	if out.ProcessLabel == "" {
		out.ProcessLabel = defaultProcessLabel
	}
	if out.KeepDays == 0 {
		out.KeepDays = defaultKeepDays
	}
	if out.CompressOlderThan == 0 {
		out.CompressOlderThan = defaultCompressOlderThan
	}
	if out.RotationCheckInterval == 0 {
		out.RotationCheckInterval = defaultRotationCheckInterval
	}
	if out.BatchFlushInterval == 0 {
		out.BatchFlushInterval = defaultBatchFlushInterval
	}
	if out.BatchFlushEventCount == 0 {
		out.BatchFlushEventCount = defaultBatchFlushEventCount
	}
	if out.NoopDropWarningEvery == 0 {
		out.NoopDropWarningEvery = defaultNoopDropWarningEvery
	}
	return out
}

// Default values applied by resolved() when the caller passes the
// zero-value for the corresponding Options field. Declared at package
// scope so the Phase 2 tests can pin against the literal values
// without re-deriving them.
const (
	defaultProcessLabel          = "lthn"
	defaultKeepDays              = 90
	defaultCompressOlderThan     = 7
	defaultRotationCheckInterval = 60 * core.Second
	defaultBatchFlushInterval    = 200 * core.Millisecond
	defaultBatchFlushEventCount  = 100
	defaultNoopDropWarningEvery  = 100
)

// metaKeyProcess is the reserved Meta key carrying the ProcessLabel
// (RFC §6.5 multi-instance plumbing). Double-underscore prefix marks
// it as recorder-managed, mirroring the __emit_ts convention from
// RFC §6.2.
const metaKeyProcess = "__process"

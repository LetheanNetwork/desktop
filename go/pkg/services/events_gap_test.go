// SPDX-Licence-Identifier: EUPL-1.2

// events_gap_test.go closes the nil-guard and fallback-branch gaps
// left by events_test.go, which only ever drives the audit helpers
// through a fully-wired newServiceFixture (Audit always set, Now
// always set). Every audit* helper on *Service starts with
// `if service == nil || service.options.Audit == nil { return }` —
// none of those guard branches were ever taken.

package services

import (
	core "dappco.re/go"

	"dappco.re/lthn/desktop/pkg/audit"
)

// TestServiceAudit_NilGuards_Bad proves every audit* helper is safe to
// call on both a nil *Service and a *Service with no Audit recorder
// wired (the zero-value Options case bypassed by NewService's
// defaulting) — no panic, and the guarded early-return is exercised.
func TestServiceAudit_NilGuards_Bad(t *core.T) {
	var nilService *Service
	nilService.auditRequested("evt", "svc-id", RestartAlways)
	nilService.auditSucceeded("evt", "svc-id", "proc-1")
	nilService.auditFailed("evt", "svc-id", core.Fail(core.E("test", "boom", nil)))
	nilService.auditDefinitionChanged("svc-id")
	nilService.auditSignalRequested("svc-id", SignalTerminate)

	noAudit := &Service{}
	noAudit.auditRequested("evt", "svc-id", RestartAlways)
	noAudit.auditSucceeded("evt", "svc-id", "proc-1")
	noAudit.auditFailed("evt", "svc-id", core.Fail(core.E("test", "boom", nil)))
	noAudit.auditDefinitionChanged("svc-id")
	noAudit.auditSignalRequested("svc-id", SignalTerminate)
}

// TestServiceAuditFailed_Ugly_NonFailureErrorFallsBackToUnavailable
// drives auditFailed with a Result whose error is NOT a *Failure (so
// ErrorCodeOf returns "") — the fallback assigns ErrorServicesUnavailable
// rather than leaving the audit trail's error_code blank.
func TestServiceAuditFailed_Ugly_NonFailureErrorFallsBackToUnavailable(t *core.T) {
	recorder := &managedServiceAuditRecorder{}
	svc := &Service{options: Options{
		Audit: recorder,
		Now:   core.Now,
	}}

	svc.auditFailed(audit.EventServiceStartFailed, "svc-id",
		core.Fail(core.E("test.plain", "not a *Failure", nil)))

	events := recorder.snapshot()
	core.RequireTrue(t, len(events) == 1)
	core.AssertEqual(t, string(ErrorServicesUnavailable), events[0].Meta[audit.MetaKeyErrorCode])
}

// ---- serviceErrorScope ----------------------------------------------------

func TestServiceErrorScope_Bad_OKResultFallsBackToServices(t *core.T) {
	scope := serviceErrorScope(core.Ok(nil))
	core.AssertEqual(t, "services", scope)
}

func TestServiceErrorScope_Bad_NonFailureErrorFallsBackToServices(t *core.T) {
	scope := serviceErrorScope(core.Fail(core.E("test.plain", "not a *Failure", nil)))
	core.AssertEqual(t, "services", scope)
}

func TestServiceErrorScope_Good_FailureOperationSurfaces(t *core.T) {
	scope := serviceErrorScope(core.Fail(&Failure{
		Code:      ErrorProcessStartFailed,
		Operation: "services.Service.Start",
		Message:   "boom",
	}))
	core.AssertEqual(t, "services.Service.Start", scope)
}

// ---- Subscribe nil-guards --------------------------------------------------

func TestSubscribe_Bad_NilCoreIsANoOp(t *core.T) {
	Subscribe(nil, func(*core.Core, Event) {
		t.Fatal("handler must never be reached with a nil Core")
	})
}

func TestSubscribe_Bad_NilHandlerIsANoOp(t *core.T) {
	c := core.New()
	Subscribe(c, nil)
	// Must not panic when the (never-reached) handler is nil and no
	// Event is ever broadcast.
	c.ACTION(Event{ID: "x"})
}

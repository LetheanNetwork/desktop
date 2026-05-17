// SPDX-Licence-Identifier: EUPL-1.2

// Wails surface for the vi subsystem. Two types live here:
//
//   - *Service (in service.go) — the SUBSTRATE-tier service. Carries the
//     per-method auth.Require gate on the four read methods Catalogue /
//     Sites / Repos / Activity. OnStart / OnStop stay on *Service so the
//     Core composition root can drive them (lifecycle hooks); they are
//     INTENTIONALLY NOT mirrored onto *WailsService so the wails3 binding
//     generator cannot expose them to the renderer.
//
//   - *WailsService — the Shape (a.i) IPC-entry WRAPPER. Wraps the
//     *Service, and at each method entry stamps TierRenderer via
//     auth.SetCaller (with breadcrumb Subject="wails", Source="wails")
//     BEFORE delegating to the inner *Service read method, then defers
//     auth.ClearCaller so the stamp clears at return. Mirrors the
//     pkg/tasks shape landed by Mantis #1755.
//
// Registration shape: pkg/desktop/desktop.go binds *WailsService into
// wails3's service array (replacing the bare *Service binding the
// pre-stamp shape used). The inner *Service stays accessible via
// WailsService.Substrate() for tests + the Core composition root that
// legitimately drives OnStart / OnStop.
//
// Cerberus #72 F-3 (Mantis #1750) closure:
//   - OnStart / OnStop were renderer-reachable via the wails3 binding
//     generator because *Service was registered directly. A renderer JS
//     loop calling Vi.OnStart() in a tight loop could re-enqueue probe
//     Jobs faster than the queue worker could drain them. The fix is the
//     wrapper-shape split: OnStart / OnStop live on *Service only; the
//     wrapper (which is what's now bound to wails3) does not have those
//     methods, so the binding generator cannot emit them.
//   - Read methods (Catalogue, Sites, Repos, Activity) gain an
//     auth.Require gate permitting TierOperator + TierRenderer +
//     TierCascade so the Phase 1 ALLOW table is uniform with the tasks
//     adopter.
//
// SECURITY-NOTE (frontend binding shift — surfaced for Cladius
// adjudication, mirroring tasks adopter): wails3 generates the TS
// binding filename from the Go type name lowercased — *WailsService →
// wailsservice.ts under
// frontend/bindings/dappco.re/lthn/desktop/pkg/vi/. The pre-stamp shape
// lived at service.ts; the frontend imports "@desktop/vi/service" today.
// Method IDs (ByID hashes) are also keyed off the FQ type name and will
// shift. A follow-on Mantis ticket covers re-running `wails3 generate` +
// updating frontend imports. Per
// [[feedback_wails_table_shim_preserves_bindings]] precedent the
// alternative (in-place stamp on the existing *Service) was prototyped
// for the tasks adopter and rejected because in-place stamping breaks
// substrate-tier tests; the same logic applies here.

package vi

import (
	core "dappco.re/go"

	"dappco.re/lthn/desktop/pkg/auth"
)

// WailsService is the IPC-entry wrapper around *Service. Methods
// mirror the four read methods on *Service exactly (same return shape)
// so the wrapper-drift reflection guard (wrapper_drift_test.go) catches
// any new read method on *Service that is added without a matching
// wrapper entry here. OnStart / OnStop are DELIBERATELY ABSENT — they
// are container-lifecycle hooks driven by the Core composition root,
// not renderer-callable surface (Cerberus #72 F-3 / Mantis #1750).
//
// Construct via NewWailsService(svc); register via
// application.NewService(vi.NewWailsService(viSvc)) in
// pkg/desktop/desktop.go.
type WailsService struct {
	svc *Service
}

// NewWailsService wraps an existing *Service for Wails IPC binding.
// The wrapper holds a pointer to the substrate, NOT a fresh copy —
// callers can keep their own *Service reference for in-Go calls and
// the wrapper continues to delegate to the same instance. This is
// load-bearing: pkg/desktop/desktop.go fetches the Core-registered
// *vi.Service (so OnStart's queue-handler registration takes effect)
// then wraps it for Wails registration — the wrapper sits beside the
// composition-root reference, not in front of it.
//
// Usage example:
//
//	viSvc, _ := core.ServiceFor[*vi.Service](c, "vi")
//	wailsSvc := vi.NewWailsService(viSvc)
//	application.NewService(wailsSvc)
func NewWailsService(svc *Service) *WailsService {
	return &WailsService{svc: svc}
}

// Substrate returns the wrapped *Service so tests + the Core
// composition root can reach the substrate-tier surface (including
// OnStart / OnStop). Wails3's binding generator only walks exported
// methods with primitive / struct arg shapes; the *Service return type
// means Substrate() does NOT show up as a TS binding — verify via
// wrapper-drift test which exempts it.
//
// Usage example:
//
//	wailsSvc := vi.NewWailsService(vi.NewService(c))
//	// Composition root reaches lifecycle hooks directly:
//	_ = wailsSvc.Substrate().OnStart()
func (w *WailsService) Substrate() *Service { return w.svc }

// stampRenderer is the Shape (a.i) IPC-entry stamp helper. Every
// Wails-bound method on *WailsService calls stampRenderer at its top
// before delegating, and defers the returned clear-fn so the sidecar
// is cleared on return. Two-step (stamp + defer clear) keeps the stamp
// scoped to the single IPC call — the next concurrent Wails call on
// the same *core.Core gets its own stamp at entry.
//
// nil-safe on c: if the wrapped service holds a nil *core.Core, the
// no-op stamp+clear keeps the caller path consistent (the downstream
// Require will still deny because Caller(nil) returns TierInternal —
// visible deny, not panic).
//
// Mirrors pkg/tasks.stampRenderer; kept package-local rather than
// exported from pkg/auth so each adopter package owns its own
// breadcrumb constants (the audit trail distinguishes vi-IPC from
// tasks-IPC by the Source value).
func stampRenderer(c *core.Core) func() {
	if c == nil {
		return func() {}
	}
	auth.SetCaller(c, auth.CallerIdentity{
		Tier:    auth.TierRenderer,
		Subject: wailsIPCSubject,
		Source:  wailsIPCSource,
	})
	return func() { auth.ClearCaller(c) }
}

// wailsIPCSubject and wailsIPCSource are the Subject/Source breadcrumb
// pair stamped onto the sidecar at every Wails IPC entry. "wails" is
// the minimum-information identifier that lets the audit trail
// distinguish renderer-via-IPC calls from renderer-via-HTTP. The
// Source is kept generic "wails" so audit greps can collect every
// Wails-IPC entry across adopters with a single filter; per-package
// disambiguation happens at the audit row's op field.
const (
	wailsIPCSubject = "wails"
	wailsIPCSource  = "wails"
)

// Catalogue — Wails IPC entry → stamp TierRenderer → delegate to substrate.
// See *Service.Catalogue for the substrate-side documentation (allow-list
// permits TierOperator + TierRenderer + TierCascade).
//
// Usage example (TS binding):
//
//	const r = await Catalogue();
//	const catalogue = r.Value as SiteCatalogue[];
func (w *WailsService) Catalogue() core.Result {
	defer stampRenderer(w.svc.core)()
	return w.svc.Catalogue()
}

// Sites — Wails IPC entry → stamp TierRenderer → delegate to substrate.
// See *Service.Sites for the substrate-side documentation.
//
// Usage example (TS binding):
//
//	const r = await Sites();
//	const out = r.Value as SitesOutput;
func (w *WailsService) Sites() core.Result {
	defer stampRenderer(w.svc.core)()
	return w.svc.Sites()
}

// Repos — Wails IPC entry → stamp TierRenderer → delegate to substrate.
// See *Service.Repos for the substrate-side documentation.
//
// Usage example (TS binding):
//
//	const r = await Repos();
//	const repos = r.Value as PRRepo[];
func (w *WailsService) Repos() core.Result {
	defer stampRenderer(w.svc.core)()
	return w.svc.Repos()
}

// Activity — Wails IPC entry → stamp TierRenderer → delegate to substrate.
// See *Service.Activity for the substrate-side documentation.
//
// Usage example (TS binding):
//
//	const r = await Activity();
//	const out = r.Value as ActivityOutput;
func (w *WailsService) Activity() core.Result {
	defer stampRenderer(w.svc.core)()
	return w.svc.Activity()
}

// ServiceName labels the binding namespace exposed to JS. Mirrors
// *Service.ServiceName so the wails3 binding emits the same namespace
// — "Vi" — that the frontend already imports. Without this the
// wrapper would surface under "WailsService" (the type name lowercased
// is the wails3 default), breaking every existing Vi.* frontend
// import.
func (w *WailsService) ServiceName() string { return "Vi" }

// SPDX-Licence-Identifier: EUPL-1.2

// Wails3 Service shape for the r1 package — wraps the package-level
// Write / Read / ListModels / ListSubjects / ListProbes functions so
// Wails generates a TS binding at
// frontend/bindings/dappco.re/lthn/desktop/pkg/r1/.
// Bound by application.NewService(r1.NewWailsService()) in
// pkg/desktop/desktop.go; the package-level functions stay for
// non-WebView callers (training-loop pipelines, cascade orchestrator,
// CLI verbs, batch jobs).

package r1

import core "dappco.re/go"

// WailsService is the WebView-facing handle on the R₁ corpus surface.
// Stateless — every method delegates to the package-level entry points
// in store.go. The corpus store itself relies on paths.AtomicAppendLine
// for concurrent-writer safety, so no per-Service mutex is required.
//
// Tier-auth: deliberately ungated at this layer. R₁ records are
// non-secret training artefacts — the persisted output of running
// LEK-sandwiched probes against a Lemma model. There is no user-secret
// content on the read side, and writes are append-only into a
// per-(model, subject) JSONL file the store itself controls. Mirrors
// the ungated posture of pkg/contentshield/wails.go.
//
// Usage example (boot wire):
//
//	application.NewService(r1.NewWailsService())
type WailsService struct{}

// NewWailsService constructs the WailsService.
//
// Usage example:
//
//	application.NewService(r1.NewWailsService())
func NewWailsService() *WailsService { return &WailsService{} }

// ServiceName labels the binding namespace exposed to JS as
// "R1Corpus" — Wails generates the TS binding under
// frontend/bindings/dappco.re/lthn/desktop/pkg/r1/.
func (s *WailsService) ServiceName() string { return "R1Corpus" }

// ServiceStartup is the Wails3 lifecycle hook called once after the
// WebView attaches. The R₁ corpus has no warmup — paths are resolved
// lazily on first Write/Read — so this is a no-op.
func (s *WailsService) ServiceStartup(_ core.Context, _ any) core.Result {
	return core.Ok(nil)
}

// ServiceShutdown is the Wails3 lifecycle hook called once at app
// shutdown. The R₁ corpus holds no resources to release.
func (s *WailsService) ServiceShutdown() core.Result { return core.Ok(nil) }

// Write appends rec to the canonical R₁ corpus location. Delegates to
// the package-level r1.Write — see store.go for the validation rules
// (Model / Subject / ProbeID required) and defensive enrichment
// (Timestamp + Hash auto-fill).
//
// Usage example (TS):
//
//	import { Write } from "@desktop/r1/r1corpus";
//	const r = await Write({
//	  model: "gemma4-e2b-it-q4", subject: "english",
//	  probe_id: "P01_TEST", substrate: "CONT", tier: 0,
//	  prompt: sandwich, response: r1Text,
//	});
//	if (!r.ok) { /* surface error */ }
func (s *WailsService) Write(rec R1) core.Result { return Write(rec) }

// Read returns every R₁ record matching the filter. Empty filter
// returns every record across every model and subject. Delegates to
// the package-level r1.Read — see store.go for filter semantics
// (zero-valued fields wildcard, Limit caps result count).
//
// Usage example (TS):
//
//	import { Read } from "@desktop/r1/r1corpus";
//	const r = await Read({ model: "gemma4-e2b-it-q4", subject: "english" });
//	if (r.ok) { for (const rec of r.value) { /* ... */ } }
func (s *WailsService) Read(f Filter) core.Result { return Read(f) }

// ListModels returns the set of model IDs that have at least one R₁
// record in the corpus. Sorted alphabetically.
//
// Usage example (TS):
//
//	import { ListModels } from "@desktop/r1/r1corpus";
//	const r = await ListModels();
//	if (r.ok) { for (const m of r.value) { /* populate dropdown */ } }
func (s *WailsService) ListModels() core.Result { return ListModels() }

// ListSubjects returns the set of subjects this model has been run
// against. Sorted alphabetically.
//
// Usage example (TS):
//
//	import { ListSubjects } from "@desktop/r1/r1corpus";
//	const r = await ListSubjects("gemma4-e2b-it-q4");
//	if (r.ok) { /* render subject chips */ }
func (s *WailsService) ListSubjects(model string) core.Result { return ListSubjects(model) }

// ListProbes returns every distinct ProbeID present in the (model,
// subject) file, in first-seen order.
//
// Usage example (TS):
//
//	import { ListProbes } from "@desktop/r1/r1corpus";
//	const r = await ListProbes("gemma4-e2b-it-q4", "english");
//	if (r.ok) { /* list probe IDs */ }
func (s *WailsService) ListProbes(model, subject string) core.Result {
	return ListProbes(model, subject)
}

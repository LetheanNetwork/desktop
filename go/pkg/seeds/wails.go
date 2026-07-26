// SPDX-Licence-Identifier: EUPL-1.2

// Wails3 Service shape for the seeds package — wraps the
// package-level ListSubjects / ListProbes / Read functions so Wails
// generates a TS binding at
// frontend-ng/bindings/dappco.re/lthn/desktop/pkg/seeds/.
// Bound by application.NewService(seeds.NewWailsService()) in
// pkg/desktop/desktop.go; the package-level functions stay for
// non-WebView callers (training-loop pipelines, cascade orchestrator,
// CLI verbs, batch jobs).

package seeds

import core "dappco.re/go"

// WailsService is the WebView-facing handle on the Hypnos seed
// corpus. Stateless apart from an immutable Root configuration that
// production leaves empty (resolving to /Volumes/Data/lem/training/
// seeds/ via rootOrDefault) and tests set to a t.TempDir fixture.
// Every method delegates straight through to the package-level entry
// points in store.go — no caches, no per-Service mutable state, no
// length caps, no redaction, no filtering. The training-window
// decides what to surface; this layer stays a faithful pass-through.
//
// Tier-auth: deliberately ungated at this layer. The seed corpus
// itself is held context per the Hypnos protect-discipline
// (project_lek_probes_gemini_3_5_preview_authorship), but the Wails
// surface is strictly read-only and the data already lives at the
// canonical on-disk root on the operator's own machine. Same posture
// as pkg/r1/wails.go and pkg/contentshield/wails.go — no secret to
// protect at this boundary, no write surface to gate.
//
// Usage example (boot wire):
//
//	application.NewService(seeds.NewWailsService())
type WailsService struct {
	// Root is the seed corpus root. Empty in production so the
	// package default (/Volumes/Data/lem/training/seeds/) wins.
	// Tests construct &WailsService{Root: t.TempDir()} to point at a
	// fixture tree built via the buildFixture helper.
	Root string
}

// NewWailsService constructs the WailsService with an empty Root, so
// production resolves to the canonical on-disk corpus location.
//
// Usage example:
//
//	application.NewService(seeds.NewWailsService())
func NewWailsService() *WailsService { return &WailsService{} }

// ServiceName labels the binding namespace exposed to JS as
// "Seeds" — Wails generates the TS binding under
// frontend-ng/bindings/dappco.re/lthn/desktop/pkg/seeds/.
func (s *WailsService) ServiceName() string { return "Seeds" }

// ServiceStartup is the Wails3 lifecycle hook called once after the
// WebView attaches. The seeds surface walks the corpus lazily on
// each call — no warmup needed — so this is a no-op.
func (s *WailsService) ServiceStartup(_ core.Context, _ any) core.Result {
	return core.Ok(nil)
}

// ServiceShutdown is the Wails3 lifecycle hook called once at app
// shutdown. The seeds surface holds no resources to release.
func (s *WailsService) ServiceShutdown() core.Result { return core.Ok(nil) }

// ListSubjects returns the canonical subject IDs that have at least
// one populated subject directory under the corpus root, in rotation
// order. Delegates to the package-level seeds.ListSubjects — see
// store.go for the canonical subject set and the russian-bridge
// alias resolution.
//
// Usage example (TS):
//
//	import { ListSubjects } from "@desktop/seeds/seeds";
//	const r = await ListSubjects();
//	if (r.ok) { for (const s of r.value) { /* render chip */ } }
func (s *WailsService) ListSubjects() core.Result {
	return ListSubjects(Options{Root: s.Root})
}

// ListProbes returns the probe IDs available under subject, in the
// order they appear on disk. A subject that is canonical but absent
// from the corpus surfaces as an Ok Result with an empty slice —
// fresh / partial corpora are legitimate per the seeds contract.
// Delegates to the package-level seeds.ListProbes.
//
// Usage example (TS):
//
//	import { ListProbes } from "@desktop/seeds/seeds";
//	const r = await ListProbes("english");
//	if (r.ok) { for (const id of r.value) { /* preview row */ } }
func (s *WailsService) ListProbes(subject string) core.Result {
	return ListProbes(subject, Options{Root: s.Root})
}

// Read returns the raw prompt text for probeID under subject. A
// missing subject or probeID surfaces as a Fail Result so callers
// can distinguish "no such probe" from "probe exists with empty
// body". Delegates straight through to the package-level seeds.Read
// — no length cap, no redaction. The training-window decides what
// to surface.
//
// Usage example (TS):
//
//	import { Read } from "@desktop/seeds/seeds";
//	const r = await Read("english", "TW01_REGRET");
//	if (r.ok) { previewPane.textContent = r.value; }
func (s *WailsService) Read(subject, probeID string) core.Result {
	return Read(subject, probeID, Options{Root: s.Root})
}

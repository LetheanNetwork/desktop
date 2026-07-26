// SPDX-Licence-Identifier: EUPL-1.2

// Wails surface for the repos service. Methods are bound on the
// *Service receiver and exposed to the frontend via wails3 generate
// bindings — the TS shape lands at
// frontend-ng/bindings/dappco.re/lthn/desktop/pkg/repos/service.

package repos

import (
	core "dappco.re/go"
)

// StatusInput drives the Status method. Paths overrides the default
// canonical-root scan; Roots overrides the canonical roots (still
// scanned for git children). Force is accepted for API parity but
// currently a no-op (lthn doesn't cache repo status today).
type StatusInput struct {
	Paths []string `json:"paths,omitempty"`
	Roots []string `json:"roots,omitempty"`
	Force bool     `json:"force,omitempty"`
}

// StatusOutput is the shape returned to the Angular window — flat
// array of repo states plus the count of roots scanned for
// debugging "why is my repo missing".
type StatusOutput struct {
	Repos   []Status `json:"repos"`
	Scanned int      `json:"scanned"`
}

// Status enumerates the user's repos and returns each one's git
// state. When Paths is empty the service scans Roots (or the
// canonical default roots when those are empty too) AND merges
// paths contributed by registered SourceProviders (opencode imports
// etc.) so externally-tracked repos surface alongside scan results.
func (s *Service) Status(input StatusInput) core.Result {
	// 30s ceiling — git status across the canonical roots takes
	// well under a second on a healthy SSD, but pathological cases
	// (slow network FS mounts, hung git auth helpers) need a cap.
	ctx, cancel := core.WithTimeout(core.Background(), 30*core.Second)
	defer cancel()

	paths := input.Paths
	if len(paths) == 0 {
		if len(input.Roots) > 0 {
			paths = s.scanRoots(input.Roots)
		} else {
			paths = s.scanDefaultRoots()
		}
	}

	// Merge external sources (deduped against each other; deduped
	// against scanned paths below). Imports surface even when
	// `paths` is empty so the user always sees them.
	seen := map[string]bool{}
	for _, p := range paths {
		seen[p] = true
	}
	for _, p := range s.collectSourcePaths(ctx) {
		if seen[p] {
			continue
		}
		seen[p] = true
		paths = append(paths, p)
	}

	if len(paths) == 0 {
		return core.Ok(StatusOutput{Repos: []Status{}, Scanned: 0})
	}
	return core.Ok(StatusOutput{
		Repos:   s.statuses(ctx, paths),
		Scanned: len(paths),
	})
}

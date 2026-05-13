// SPDX-Licence-Identifier: EUPL-1.2

// Package repos is the lthn-side multi-repo dashboard surface.
// Aggregates git status across the user's canonical Code/* trees so
// the UI can answer "which repos need attention?" at a glance.
//
// Ported from core/ide's repos_bridge.go + reposbridge.go. Wraps
// dappco.re/go/scm/git which is already in the workspace via the
// external/api submodule. The MCP-tool indirection there existed
// for AI-tool dual-mount; lthn goes direct because the Wails GUI
// is the only consumer today.
//
// Caching from core/ide is intentionally dropped — each Re-scan
// click probes fresh. If the dashboard becomes a hot path, the
// cache shape can land later.

package repos

import (
	"context"

	core "dappco.re/go"
	scmgit "dappco.re/go/scm/git"
)

// Service owns the repos surface. Holds *core.Core for late
// resolution of any future dependencies (currently scmgit is a
// package-level static, no injection needed).
type Service struct {
	core *core.Core
}

// NewService constructs the repos surface against a Core container.
// Wired via application.NewService(repos.NewService(c)) in
// pkg/desktop/desktop.go.
//
// Usage example:
//
//	svc := repos.NewService(c)
func NewService(c *core.Core) *Service { return &Service{core: c} }

// Register constructs the repos service for Core registration.
//
// Usage example:
//
//	core.New(core.WithService(repos.Register))
func Register(c *core.Core) core.Result {
	return core.Ok(NewService(c))
}

// Status describes one repo's git state — branch, ahead/behind
// counts, dirty file counts. JSON-shaped for the Lit window.
type Status struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	Branch    string `json:"branch"`
	Modified  int    `json:"modified"`
	Untracked int    `json:"untracked"`
	Staged    int    `json:"staged"`
	Ahead     int    `json:"ahead"`
	Behind    int    `json:"behind"`
	Dirty     bool   `json:"dirty"`
	Err       string `json:"error,omitempty"`
}

// scanDefaultRoots walks the user's canonical workspace roots and
// returns the absolute path of every directory that contains a
// .git entry. Mirrors core/ide's defaults — Code/{core,lthn,
// host-uk,lab,snider} under $HOME.
func (s *Service) scanDefaultRoots() []string {
	home := core.UserHomeDir()
	if !home.OK {
		return nil
	}
	homeDir, _ := home.Value.(string)
	if homeDir == "" {
		return nil
	}
	return s.scanRoots([]string{
		core.PathJoin(homeDir, "Code", "core"),
		core.PathJoin(homeDir, "Code", "lthn"),
		core.PathJoin(homeDir, "Code", "host-uk"),
		core.PathJoin(homeDir, "Code", "lab"),
		core.PathJoin(homeDir, "Code", "snider"),
	})
}

// scanRoots expands a list of parent directories into git-repo
// paths. Any immediate subdirectory that contains a .git entry is
// included. Order is stable — root order then alphabetical within
// root (ReadDir's default).
func (s *Service) scanRoots(roots []string) []string {
	var out []string
	for _, root := range roots {
		res := core.ReadDir(core.DirFS(root), ".")
		if !res.OK {
			continue
		}
		entries, _ := res.Value.([]core.FsDirEntry)
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			full := core.PathJoin(root, entry.Name())
			info := core.Stat(core.PathJoin(full, ".git"))
			if !info.OK {
				continue
			}
			out = append(out, full)
		}
	}
	return out
}

// statuses runs the scm/git Status probe against the given paths
// and shapes the result for the Lit window. Calls fan out to N
// `git` invocations concurrently inside scmgit — bounded by its
// internal worker pool, not by us.
func (s *Service) statuses(ctx context.Context, paths []string) []Status {
	raw := scmgit.Status(ctx, scmgit.StatusOptions{Paths: paths})
	out := make([]Status, 0, len(raw))
	for _, st := range raw {
		entry := Status{
			Name:      st.Name,
			Path:      st.Path,
			Branch:    st.Branch,
			Modified:  st.Modified,
			Untracked: st.Untracked,
			Staged:    st.Staged,
			Ahead:     st.Ahead,
			Behind:    st.Behind,
			Dirty:     st.IsDirty(),
		}
		if st.Error != nil {
			entry.Err = st.Error.Error()
		}
		out = append(out, entry)
	}
	return out
}

// SPDX-Licence-Identifier: EUPL-1.2

// Package git is the lthn-side source-control surface. Branch /
// status / diff / add / unstage / commit / log against any local
// repo, exposed as Wails methods the GUI can bind directly. Direct
// shell-out via dappco.re/go/process — no MCPBridge layering, just
// `git -C <path> <args>` per call.
//
// Ported from core/ide's gitbridge.go + mcp_bridge.go toolGit* funcs.
// The two-layer indirection there (typed bridge → MCP tool → shell)
// existed to dual-mount the same surface as AI tools; lthn/desktop
// goes direct because the Wails GUI is the only consumer today
// (plugins can later re-export via the namespace-prefixed reverse-
// proxy pattern Snider outlined).

package git

import (

	core "dappco.re/go"
	"dappco.re/go/process"
)

const runGitOp = "git.runGit"

// Service owns the git surface. Holds a *core.Core ref so it can
// reach the process service at call time (the process service may
// not have been resolved yet at construction time, depending on
// boot order).
type Service struct {
	core *core.Core
}

// NewService constructs the git surface against a Core container.
// Wired via application.NewService(git.NewService(c)) in
// pkg/desktop/desktop.go.
//
// Usage example:
//
//	svc := git.NewService(c)
func NewService(c *core.Core) *Service { return &Service{core: c} }

// Register constructs the git service for Core registration.
//
// Usage example:
//
//	core.New(core.WithService(git.Register))
func Register(c *core.Core) core.Result {
	return core.Ok(NewService(c))
}

// runGit spawns `git -C <repo> <args...>`, returns the captured
// stdout on success, the captured stderr+stdout combined inside a
// failure value otherwise. Mirrors core/ide's runGit / gitRunOutput
// pair so the porcelain parsers below can stay byte-identical.
func (s *Service) runGit(repo string, args ...string) core.Result {
	if s == nil || s.core == nil {
		return core.Fail(core.E(runGitOp, "core not bound", nil))
	}
	proc, ok := core.ServiceFor[*process.Service](s.core, "process")
	if !ok || proc == nil {
		return core.Fail(core.E(runGitOp, "process service unavailable", nil))
	}
	full := append([]string{"-C", repo}, args...)
	r := proc.Run(core.Background(), "git", full...)
	out, _ := r.Value.(string)
	if !r.OK {
		return core.Fail(core.E(runGitOp, r.Error(), nil))
	}
	return core.Ok(out)
}

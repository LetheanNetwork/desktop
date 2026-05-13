// SPDX-Licence-Identifier: EUPL-1.2

// Package lint is the lthn-side surface over the core-lint binary.
// Runs `core-lint lint check <root> --format json`, parses the
// output into typed issues, surfaces severity counts and timing.
// The Wails-bindable shape lives in wails.go.
//
// Ported from core/ide's lint_bridge.go + lintbridge.go. Same
// direct-shell-out approach the git + build ports use — no MCP
// layering.

package lint

import (
	"context"

	core "dappco.re/go"
	"dappco.re/go/process"
)

// Service owns the lint surface. Holds *core.Core for late
// resolution of the process service.
type Service struct {
	core *core.Core
}

// NewService constructs the lint surface against a Core container.
// Wired via application.NewService(lint.NewService(c)) in
// pkg/desktop/desktop.go.
//
// Usage example:
//
//	svc := lint.NewService(c)
func NewService(c *core.Core) *Service { return &Service{core: c} }

// Register constructs the lint service for Core registration.
//
// Usage example:
//
//	core.New(core.WithService(lint.Register))
func Register(c *core.Core) core.Result {
	return core.Ok(NewService(c))
}

// proc resolves the process service at call time.
func (s *Service) proc() *process.Service {
	if s == nil || s.core == nil {
		return nil
	}
	ps, _ := core.ServiceFor[*process.Service](s.core, "process")
	return ps
}

// findLintBinary searches PATH for core-lint, falling back to a
// short list of canonical workstation paths Snider uses. Returns
// "" when no candidate resolves.
func findLintBinary() string {
	if found := (core.App{}).Find("core-lint", "core-lint"); found.OK {
		if app, ok := found.Value.(*core.App); ok && app != nil {
			return app.Path
		}
	}
	candidates := []string{
		"/Users/snider/Code/core/lint/bin/core-lint",
	}
	if home := core.UserHomeDir(); home.OK {
		if h, ok := home.Value.(string); ok && h != "" {
			candidates = append(candidates, core.PathJoin(h, "Code", "core", "lint", "bin", "core-lint"))
			candidates = append(candidates, core.PathJoin(h, "bin", "core-lint"))
		}
	}
	for _, p := range candidates {
		stat := core.Stat(p)
		if !stat.OK {
			continue
		}
		info, ok := stat.Value.(interface{ IsDir() bool })
		if ok && !info.IsDir() {
			return p
		}
	}
	return ""
}

// runLint shells out to the lint binary, returns the captured
// stdout. Non-zero exit codes are NOT treated as error when the
// binary still emitted JSON — core-lint exits non-zero when issues
// exist, which is the normal happy-path.
func (s *Service) runLint(binary string, args ...string) core.Result {
	ps := s.proc()
	if ps == nil {
		return core.Fail(core.E("lint.runLint", "process service unavailable", nil))
	}
	r := ps.Run(context.Background(), binary, args...)
	out, _ := r.Value.(string)
	if !r.OK && out == "" {
		return core.Fail(core.E("lint.runLint", r.Error(), nil))
	}
	return core.Ok(out)
}

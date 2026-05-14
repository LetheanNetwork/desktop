// SPDX-Licence-Identifier: EUPL-1.2

// Wails3 Service shape for pkg/lint — typed methods the WebView
// binds directly. Returns parsed core-lint issues + severity
// counts for the panel's grouped table.

package lint

import (
	"context"
	"time"

	core "dappco.re/go"
)

// ServiceName / Startup / Shutdown — Wails3 lifecycle.
func (s *Service) ServiceName() string { return "Lint" }
func (s *Service) ServiceStartup(_ context.Context, _ any) core.Result {
	return core.Ok(nil)
}
func (s *Service) ServiceShutdown() core.Result { return core.Ok(nil) }

// Issue mirrors one row of `core-lint lint check --format json`.
type Issue struct {
	RuleID   string `json:"rule_id"`
	Title    string `json:"title"`
	Severity string `json:"severity"`
	File     string `json:"file"`
	Line     int    `json:"line"`
	Match    string `json:"match"`
	Fix      string `json:"fix"`
}

// RunOutput wraps the parsed Issues with severity counts + timing.
type RunOutput struct {
	Path       string         `json:"path"`
	Issues     []Issue        `json:"issues"`
	Counts     map[string]int `json:"counts"`
	Total      int            `json:"total"`
	BinaryPath string         `json:"binary_path"`
	DurationMs int64          `json:"duration_ms"`
}

// Run executes core-lint against `path`. `severity` and `lang`
// are optional filters passed through to the binary; empty
// strings mean "no filter".
//
// Usage example (TS):
//
//	import { Run } from "@desktop/lint/service";
//	const { issues, counts, total } = await Run("/path", "", "");
func (s *Service) Run(path, severity, lang string) core.Result {
	if core.Trim(path) == "" {
		return core.Fail(core.E(runOp, "path required", nil))
	}
	stat := core.Stat(path)
	if !stat.OK {
		return core.Fail(core.E(runOp, "path is not a directory: "+path, nil))
	}
	if info, ok := stat.Value.(interface{ IsDir() bool }); !ok || !info.IsDir() {
		return core.Fail(core.E(runOp, "path is not a directory: "+path, nil))
	}
	binary := findLintBinary()
	if binary == "" {
		return core.Fail(core.E(runOp,
			"core-lint binary not found on PATH or canonical locations", nil))
	}
	args := []string{"lint", "check", path, "--format", "json"}
	if severity != "" {
		args = append(args, "--severity", severity)
	}
	if lang != "" {
		args = append(args, "--lang", lang)
	}
	start := time.Now()
	outR := s.runLint(binary, args...)
	dur := time.Since(start).Milliseconds()
	if !outR.OK {
		return outR
	}
	out, _ := outR.Value.(string)
	var issues []Issue
	if r := core.JSONUnmarshal([]byte(out), &issues); !r.OK {
		return core.Fail(core.E(runOp, "parse json: "+r.Error(), nil))
	}
	counts := map[string]int{}
	for _, iss := range issues {
		counts[iss.Severity]++
	}
	if issues == nil {
		issues = []Issue{}
	}
	return core.Ok(RunOutput{
		Path:       path,
		Issues:     issues,
		Counts:     counts,
		Total:      len(issues),
		BinaryPath: binary,
		DurationMs: dur,
	})
}

// CatalogEntry mirrors one rule emitted by `core-lint lint catalog list`.
// Useful for tooltips + a rule-reference surface.
type CatalogEntry struct {
	RuleID      string `json:"rule_id"`
	Title       string `json:"title"`
	Severity    string `json:"severity"`
	Description string `json:"description"`
}

// Catalog returns every rule the linter knows about. Returns an
// empty slice when core-lint isn't installed (UI surfaces a hint).
//
// Usage example (TS):
//
//	const rules = await Catalog();
func (s *Service) Catalog() core.Result {
	binary := findLintBinary()
	if binary == "" {
		return core.Ok([]CatalogEntry{})
	}
	outR := s.runLint(binary, "lint", "catalog", "list", "--format", "json")
	if !outR.OK {
		return outR
	}
	out, _ := outR.Value.(string)
	var entries []CatalogEntry
	if r := core.JSONUnmarshal([]byte(out), &entries); !r.OK {
		return core.Fail(core.E("lint.Catalog", "parse json: "+r.Error(), nil))
	}
	if entries == nil {
		entries = []CatalogEntry{}
	}
	return core.Ok(entries)
}

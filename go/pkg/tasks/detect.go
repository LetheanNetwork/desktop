// SPDX-Licence-Identifier: EUPL-1.2

package tasks

import (
	core "dappco.re/go"
	"dappco.re/go/process"

	"dappco.re/lthn/desktop/pkg/auth"
)

// lint → tasks bridge. core-lint detects issues (go + php, auto-detected) and
// emits pure JSON; Detect files each new finding into this queue so it surfaces
// in the Coding/Issues + Planning views and can be dispatched to an agent.
// core-lint stays a pure linter — the desktop shells it (as it shells
// lthn-agent / lthn-mlx) and owns the sealed per-account store here.
// Spec: plans/code/lthn/desktop/RFC.lint-tasks.md

const (
	lintBinaryName = "core-lint"
	// lintReporter tags every task this bridge files, so dedup (and a future
	// "clear lint tasks") can find exactly the queue items lint owns.
	lintReporter = "core-lint"
	// envLintBin points the bridge at a specific core-lint binary.
	envLintBin = "CORE_LINT_BIN"
)

// DetectInput names a repo to lint and where to lint it.
type DetectInput struct {
	Repo string `json:"repo"`           // task Project, e.g. "core/go-io"
	Path string `json:"path"`           // working dir core-lint scans
	Lang string `json:"lang,omitempty"` // "go"/"php" to narrow; empty = all detected
}

// DetectOutput summarises a Detect run.
type DetectOutput struct {
	Repo     string `json:"repo"`
	Findings int    `json:"findings"` // findings core-lint reported
	Created  int    `json:"created"`  // new tasks filed
	Skipped  int    `json:"skipped"`  // findings already on the board (deduped)
}

// lintReport / lintFinding mirror the JSON `core-lint run --output=json` emits
// (dappco.re/go/lint Report + Finding) — only the fields the bridge consumes.
type lintReport struct {
	Findings []lintFinding `json:"findings"`
}

type lintFinding struct {
	Tool     string `json:"tool"`
	File     string `json:"file"`
	Line     int    `json:"line"`
	Severity string `json:"severity"`
	Code     string `json:"code"`
	Message  string `json:"message"`
	Fix      string `json:"fix"`
	RuleID   string `json:"rule_id"`
	Title    string `json:"title"`
}

// Detect runs core-lint over a repo and files each finding not already on the
// board as a task. System-level (no auth here — the Wails Service.Detect
// wrapper gates the GUI call). Best-effort per finding: one failed Create does
// not abort the batch.
//
//	r := tasks.Detect(c, tasks.DetectInput{Repo: "core/go-io", Path: "/Users/snider/Code/core/go-io"})
//	if r.OK { out := r.Value.(tasks.DetectOutput); _ = out }
func Detect(c *core.Core, input DetectInput) core.Result {
	if core.Trim(input.Repo) == "" || core.Trim(input.Path) == "" {
		return core.Fail(core.E("tasks.Detect", "repo and path are required", nil))
	}
	report, r := runCoreLint(input.Path, input.Lang)
	if !r.OK {
		return r
	}
	seen := existingLintFingerprints(c, input.Repo)
	out := DetectOutput{Repo: input.Repo, Findings: len(report.Findings)}
	for _, f := range report.Findings {
		fp := lintFingerprint(f)
		if seen[fp] {
			out.Skipped++
			continue
		}
		seen[fp] = true // also dedup repeats within a single report
		if Create(c, CreateInput{
			Project:     input.Repo,
			Summary:     lintSummary(f, fp),
			Description: lintDescription(f),
			Severity:    lintSeverity(f.Severity),
			Reporter:    lintReporter,
		}).OK {
			out.Created++
		}
	}
	return core.Ok(out)
}

// Detect (Wails) runs detection for the GUI / a scheduler. Gated like the other
// writes; the system-level tasks.Detect does the work.
func (s *Service) Detect(input DetectInput) core.Result {
	id, ok := auth.Require(s.core, "tasks.Service.Detect", auth.TierOperator, auth.TierRenderer)
	if !ok {
		return core.Fail(core.E("tasks.Service.Detect",
			"tasks.tier_not_permitted: "+id.Tier.String(), nil))
	}
	return Detect(s.core, input)
}

// Detect (Wails IPC) — stamp TierRenderer → delegate to the substrate.
// See *Service.Detect for the substrate-side documentation.
func (w *WailsService) Detect(input DetectInput) core.Result {
	defer stampRenderer(w.svc.core)()
	return w.svc.Detect(input)
}

// runCoreLint shells `core-lint run --output=json` and parses the report.
// core-lint exits non-zero when it finds errors (fail-on has no "never"), so
// the captured output is read regardless of exit code — the JSON report is on
// stdout either way.
func runCoreLint(path, lang string) (lintReport, core.Result) {
	args := []string{"run", "--output=json", "--file_path=" + path}
	if core.Trim(lang) != "" {
		args = append(args, "--lang="+lang)
	}
	startR := process.Start(core.Background(), resolveLintBinary(), args...)
	if !startR.OK {
		return lintReport{}, core.Fail(core.E("tasks.Detect", "start core-lint: "+startR.Error(), nil))
	}
	id := lintProcID(startR)
	process.Wait(id) // blocks; a non-zero exit (findings present) is expected
	outR := process.Output(id)
	if !outR.OK {
		return lintReport{}, core.Fail(core.E("tasks.Detect", "read core-lint output: "+outR.Error(), nil))
	}
	out, _ := outR.Value.(string)
	report, ok := parseLintReport(out)
	if !ok {
		return lintReport{}, core.Fail(core.E("tasks.Detect", "no JSON report in core-lint output", nil))
	}
	return report, core.Ok(nil)
}

// parseLintReport extracts the JSON report document from core-lint's output.
// core-lint pretty-prints the report across many lines (and the captured
// stream may carry a leading log line or two), so the document is sliced from
// the first line that opens with '{' to the last '}'. ok=false when none.
func parseLintReport(out string) (lintReport, bool) {
	doc := jsonObjectSlice(out)
	if doc == "" {
		return lintReport{}, false
	}
	var report lintReport
	if pr := core.JSONUnmarshalString(doc, &report); !pr.OK {
		return lintReport{}, false
	}
	return report, true
}

// jsonObjectSlice returns the JSON document — from the first line whose first
// non-space char is '{' (skipping leading log lines) through the last '}'
// (dropping any trailing log lines). "" when there is no object.
func jsonObjectSlice(out string) string {
	lines := core.Split(out, "\n")
	start := -1
	for i, ln := range lines {
		if t := core.Trim(ln); len(t) > 0 && t[0] == '{' {
			start = i
			break
		}
	}
	if start < 0 {
		return ""
	}
	doc := core.Join("\n", lines[start:]...)
	end := -1
	for i := len(doc) - 1; i >= 0; i-- {
		if doc[i] == '}' {
			end = i
			break
		}
	}
	if end < 0 {
		return ""
	}
	return doc[:end+1]
}

// lintFingerprint is the stable identity of a finding — tool, rule, file, line.
// Embedded in the (plain, unsealed) task summary so dedup never needs decrypt.
func lintFingerprint(f lintFinding) string {
	rule := f.RuleID
	if rule == "" {
		rule = f.Code
	}
	return core.Concat(f.Tool, ":", rule, "@", f.File, ":", core.Itoa(f.Line))
}

// lintSummary is the queue title: a bracketed fingerprint (the dedup key, and a
// file:line locator) then the finding message.
func lintSummary(f lintFinding, fp string) string {
	msg := f.Message
	if msg == "" {
		msg = f.Title
	}
	return core.Concat("[", fp, "] ", msg)
}

// lintDescription is the sealed task body — finding detail and any fix hint.
func lintDescription(f lintFinding) string {
	b := core.Concat(f.File, ":", core.Itoa(f.Line), " — ", f.Severity)
	if f.Message != "" {
		b = core.Concat(b, "\n\n", f.Message)
	}
	if f.Fix != "" {
		b = core.Concat(b, "\n\nFix: ", f.Fix)
	}
	return b
}

// lintSeverity maps a core-lint severity onto the queue's Mantis-style set.
func lintSeverity(s string) string {
	switch core.Lower(core.Trim(s)) {
	case "error":
		return SeverityMajor
	case "warning":
		return SeverityMinor
	default:
		return SeverityTweak
	}
}

// existingLintFingerprints returns the fingerprints already on the board for a
// project — read from the bracketed prefix of OPEN lint-filed task summaries,
// so re-running detection does not duplicate. Closed (done/cancelled) tasks are
// excluded, letting a recurrence re-file once the prior task is resolved.
func existingLintFingerprints(c *core.Core, repo string) map[string]bool {
	seen := map[string]bool{}
	r := List(c, ListFilter{Project: repo, Reporter: lintReporter})
	if !r.OK {
		return seen
	}
	issues, ok := r.Value.([]Issue)
	if !ok {
		return seen
	}
	for _, iss := range issues {
		if iss.State == StateDone || iss.State == StateCancelled {
			continue
		}
		if fp := summaryFingerprint(iss.Summary); fp != "" {
			seen[fp] = true
		}
	}
	return seen
}

// summaryFingerprint extracts the bracketed fingerprint from a lint task
// summary ("[fp] message" → "fp"). "" when the summary is not lint-shaped.
func summaryFingerprint(summary string) string {
	s := core.Trim(summary)
	if len(s) < 2 || s[0] != '[' {
		return ""
	}
	end := core.Index(s, "]")
	if end <= 1 {
		return ""
	}
	return s[1:end]
}

// resolveLintBinary finds core-lint: explicit override → installed → core/lint
// dev build → PATH. Mirrors the agents / calibrate resolvers.
func resolveLintBinary() string {
	if p := core.Trim(core.Env(envLintBin)); p != "" {
		return p
	}
	home := core.Env("HOME")
	for _, candidate := range []string{
		core.PathJoin(home, "Lethean", "bin", lintBinaryName),
		core.PathJoin(home, "Code", "core", "lint", "bin", lintBinaryName),
	} {
		if core.Stat(candidate).OK {
			return candidate
		}
	}
	return lintBinaryName
}

// lintProcID extracts the go-process id from a Start result.
func lintProcID(r core.Result) string {
	switch v := r.Value.(type) {
	case *process.Process:
		return v.ID
	case string:
		return v
	default:
		return ""
	}
}

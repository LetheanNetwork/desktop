// SPDX-Licence-Identifier: EUPL-1.2

package tasks

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	core "dappco.re/go"
	"dappco.re/go/orm"
	"dappco.re/go/process"

	"dappco.re/lthn/desktop/pkg/auth"
)

// newDetectCore is the internal-package twin of api_test.go's
// newTestCore (that one lives in `tasks_test` and is not visible from
// here) — a *core.Core with orm + the tasks schemas wired against a
// fresh in-memory Memium. Shared by detect_test.go / updates_test.go /
// audit_test.go (same package, no import needed).
func newDetectCore(t *testing.T) *core.Core {
	t.Helper()
	c := core.New()
	if r := orm.Register(c); !r.OK {
		t.Fatalf("orm.Register: %s", r.Error())
	}
	mem := orm.NewMemium()
	if r := orm.Mount(c, "default", mem); !r.OK {
		t.Fatalf("orm.Mount: %s", r.Error())
	}
	for _, schema := range Schemas() {
		if r := orm.RegisterSchema(c, schema); !r.OK {
			t.Fatalf("orm.RegisterSchema(%s): %s", schema.Name, r.Error())
		}
		mem.RegisterTable(schema.Name, schema)
	}
	return c
}

// ensureProcess wires the dappco.re/go/process global default service
// (mirrors cmd/lthn/app.go's boot-time process.Init call) so
// captureCommand's process.Start/Wait/Output calls have a live
// service to dispatch through. process.Init is sync.Once-guarded
// package-internally, so repeat calls across tests in this binary are
// cheap no-ops after the first.
func ensureProcess(t *testing.T) {
	t.Helper()
	if r := process.Init(core.New()); !r.OK {
		t.Fatalf("process.Init: %s", r.Error())
	}
}

// writeScript writes an executable POSIX shell script at dir/name
// (shebang added here — body is just the script content) and returns
// its absolute path. Used to fake external tools (core-lint, go,
// composer) at the package's real subprocess seam — no network, no
// real binary required.
func writeScript(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	full := "#!/bin/sh\n" + body
	if err := os.WriteFile(path, []byte(full), 0o755); err != nil {
		t.Fatalf("write script %s: %s", path, err)
	}
	return path
}

func TestDetect_Fingerprint_RoundTrips(t *testing.T) {
	f := lintFinding{Tool: "golangci-lint", RuleID: "errcheck", File: "main.go", Line: 42, Message: "error return value not checked"}
	fp := lintFingerprint(f)
	if want := "golangci-lint:errcheck@main.go:42"; fp != want {
		t.Errorf("lintFingerprint = %q, want %q", fp, want)
	}
	// The summary embeds the fingerprint; summaryFingerprint (the dedup read)
	// must recover it unchanged — that round-trip is what stops duplicates.
	if got := summaryFingerprint(lintSummary(f, fp)); got != fp {
		t.Errorf("summaryFingerprint(lintSummary(...)) = %q, want %q", got, fp)
	}
}

func TestDetect_Fingerprint_CodeFallback(t *testing.T) {
	// No RuleID → Code is used as the rule component.
	f := lintFinding{Tool: "phpstan", Code: "missingType.return", File: "src/X.php", Line: 7}
	if fp := lintFingerprint(f); fp != "phpstan:missingType.return@src/X.php:7" {
		t.Errorf("Code-fallback fingerprint = %q", fp)
	}
}

func TestDetect_Severity_Map(t *testing.T) {
	cases := map[string]string{
		"error":   SeverityMajor,
		"ERROR":   SeverityMajor,
		"warning": SeverityMinor,
		"info":    SeverityTweak,
		"":        SeverityTweak,
		"weird":   SeverityTweak,
	}
	for in, want := range cases {
		if got := lintSeverity(in); got != want {
			t.Errorf("lintSeverity(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDetect_ParseReport_Good(t *testing.T) {
	// core-lint may emit a log line above the JSON report; parsing must skip it.
	out := "06:00:00 [INF] core-lint: scanning\n" +
		`{"findings":[{"tool":"gosec","file":"a.go","line":3,"severity":"error","rule_id":"G104","message":"unhandled error"}]}`
	report, ok := parseLintReport(out)
	if !ok {
		t.Fatal("parseLintReport returned ok=false on valid output")
	}
	if len(report.Findings) != 1 || report.Findings[0].Tool != "gosec" {
		t.Errorf("parsed findings = %+v", report.Findings)
	}
}

func TestDetect_ParseReport_PrettyMultiline(t *testing.T) {
	// The real shape: core-lint pretty-prints, and the captured stream may
	// carry a leading + trailing log line. The document must still parse.
	out := "06:23:04 [INF] core-lint: scanning go\n" +
		"{\n" +
		"  \"project\": \"go-io\",\n" +
		"  \"findings\": [\n" +
		"    {\n" +
		"      \"tool\": \"gosec\",\n" +
		"      \"file\": \"a.go\",\n" +
		"      \"line\": 3,\n" +
		"      \"severity\": \"error\",\n" +
		"      \"rule_id\": \"G104\",\n" +
		"      \"message\": \"unhandled error\"\n" +
		"    }\n" +
		"  ]\n" +
		"}\n" +
		"06:23:05 [INF] core-lint: done\n"
	report, ok := parseLintReport(out)
	if !ok {
		t.Fatal("parseLintReport ok=false on pretty multi-line output")
	}
	if len(report.Findings) != 1 || report.Findings[0].RuleID != "G104" {
		t.Errorf("pretty parse findings = %+v", report.Findings)
	}
}

func TestDetect_ParseReport_Bad(t *testing.T) {
	if _, ok := parseLintReport("no json here\njust logs\n"); ok {
		t.Error("parseLintReport should return ok=false when no JSON object is present")
	}
}

func TestDetect_ParseReport_NoFindings(t *testing.T) {
	// The common clean-repo case: a report with an empty findings array,
	// wrapped in log noise, parses to zero findings (not an error).
	out := "[INF] scanning\n" + `{"project":"go-io","findings":[]}` + "\n[INF] done\n"
	report, ok := parseLintReport(out)
	if !ok {
		t.Fatal("parseLintReport ok=false on an empty-findings report")
	}
	if len(report.Findings) != 0 {
		t.Errorf("expected 0 findings, got %d", len(report.Findings))
	}
}

func TestDetect_SummaryFingerprint_NonLint(t *testing.T) {
	// A human-authored task (no bracket prefix) yields no fingerprint, so the
	// dedup pass never confuses it for a lint-filed one.
	if fp := summaryFingerprint("Fix the login bug"); fp != "" {
		t.Errorf("summaryFingerprint(non-lint) = %q, want empty", fp)
	}
}

// --- Detect() / ClearDetected() — hermetic core-lint subprocess fixtures ---

// TestDetect_Detect_Good runs Detect against a fake core-lint that reports
// two findings, then re-runs it — the second pass must dedup via the
// bracketed-fingerprint summary read (existingFingerprints).
func TestDetect_Detect_Good(t *testing.T) {
	ensureProcess(t)
	c := newDetectCore(t)
	dir := t.TempDir()
	script := writeScript(t, dir, "core-lint", `cat <<'JSON'
{"findings":[
  {"tool":"gosec","file":"a.go","line":3,"severity":"error","rule_id":"G104","message":"unhandled error"},
  {"tool":"golangci-lint","file":"b.go","line":9,"severity":"warning","rule_id":"errcheck","message":"missing check"}
]}
JSON
`)
	t.Setenv("CORE_LINT_BIN", script)

	r := Detect(c, DetectInput{Repo: "core/go-io", Path: dir})
	if !r.OK {
		t.Fatalf("Detect: %s", r.Error())
	}
	out, ok := r.Value.(DetectOutput)
	if !ok {
		t.Fatalf("Detect value type = %T", r.Value)
	}
	if out.Findings != 2 || out.Created != 2 || out.Skipped != 0 {
		t.Errorf("first run = %+v, want Findings=2 Created=2 Skipped=0", out)
	}

	r2 := Detect(c, DetectInput{Repo: "core/go-io", Path: dir})
	if !r2.OK {
		t.Fatalf("Detect (rerun): %s", r2.Error())
	}
	out2, _ := r2.Value.(DetectOutput)
	if out2.Created != 0 || out2.Skipped != 2 {
		t.Errorf("rerun = %+v, want Created=0 Skipped=2 (dedup)", out2)
	}
}

// TestDetect_Detect_Bad covers the two required-field failures plus the
// case where core-lint's output carries no parseable JSON report.
func TestDetect_Detect_Bad(t *testing.T) {
	c := newDetectCore(t)
	if r := Detect(c, DetectInput{}); r.OK {
		t.Error("Detect with empty repo/path should fail")
	}
	if r := Detect(c, DetectInput{Repo: "x"}); r.OK {
		t.Error("Detect with empty path should fail")
	}

	ensureProcess(t)
	dir := t.TempDir()
	script := writeScript(t, dir, "core-lint", `echo "not json output"`)
	t.Setenv("CORE_LINT_BIN", script)
	if r := Detect(c, DetectInput{Repo: "x", Path: dir}); r.OK {
		t.Error("Detect should fail when core-lint output has no JSON report")
	}
}

// TestDetect_Detect_NonZeroExit_Ugly pins the documented core-lint
// contract: findings are read from captured output regardless of the
// process's exit code (fail-on has no "never" — core-lint legitimately
// exits non-zero when it found something).
func TestDetect_Detect_NonZeroExit_Ugly(t *testing.T) {
	ensureProcess(t)
	c := newDetectCore(t)
	dir := t.TempDir()
	script := writeScript(t, dir, "core-lint", `cat <<'JSON'
{"findings":[{"tool":"gosec","file":"c.go","line":1,"severity":"error","rule_id":"G101","message":"boom"}]}
JSON
exit 1
`)
	t.Setenv("CORE_LINT_BIN", script)
	r := Detect(c, DetectInput{Repo: "x", Path: dir})
	if !r.OK {
		t.Fatalf("Detect should succeed reading output despite a non-zero exit: %s", r.Error())
	}
	out, _ := r.Value.(DetectOutput)
	if out.Created != 1 {
		t.Errorf("Created = %d, want 1", out.Created)
	}
}

// TestDetect_RunCoreLint_LangArg_Good asserts the optional Lang narrows
// the invocation to --lang=<lang> — captured by a fake core-lint that
// records its argv to a marker file.
func TestDetect_RunCoreLint_LangArg_Good(t *testing.T) {
	ensureProcess(t)
	dir := t.TempDir()
	marker := filepath.Join(dir, "args.txt")
	script := writeScript(t, dir, "core-lint", `echo "$@" > `+marker+`
echo '{"findings":[]}'
`)
	t.Setenv("CORE_LINT_BIN", script)
	_, r := runCoreLint("/some/path", "go")
	if !r.OK {
		t.Fatalf("runCoreLint: %s", r.Error())
	}
	got, err := os.ReadFile(marker)
	if err != nil {
		t.Fatalf("read marker: %s", err)
	}
	if !strings.Contains(string(got), "--lang=go") {
		t.Errorf("argv = %q, want --lang=go present", got)
	}
}

// TestDetect_RunCoreLint_Bad exercises the no-JSON-in-output failure path
// directly (Detect_Bad also hits it end-to-end; this pins runCoreLint's
// own contract in isolation).
func TestDetect_RunCoreLint_Bad(t *testing.T) {
	ensureProcess(t)
	dir := t.TempDir()
	script := writeScript(t, dir, "core-lint", `echo "no json here"`)
	t.Setenv("CORE_LINT_BIN", script)
	if _, r := runCoreLint(dir, ""); r.OK {
		t.Error("runCoreLint should fail when no JSON report is present")
	}
}

// --- captureCommand — the shared subprocess-capture primitive ---

// TestDetect_CaptureCommand_Good spawns a real (fake) local binary and
// asserts its stdout is captured.
func TestDetect_CaptureCommand_Good(t *testing.T) {
	ensureProcess(t)
	dir := t.TempDir()
	script := writeScript(t, dir, "echoer", `echo "hello capture"`)
	out := captureCommand(script)
	if !strings.Contains(out, "hello capture") {
		t.Errorf("captureCommand output = %q", out)
	}
}

// TestDetect_CaptureCommand_Bad is the spawn-failure fault injection —
// a binary that does not exist fails cmd.Start(), and captureCommand
// degrades to "" rather than propagating the error (callers treat empty
// as "nothing detected").
func TestDetect_CaptureCommand_Bad(t *testing.T) {
	ensureProcess(t)
	out := captureCommand("this-binary-does-not-exist-anywhere-xyz")
	if out != "" {
		t.Errorf("captureCommand(nonexistent) = %q, want empty", out)
	}
}

// TestDetect_CaptureCommand_NonZeroExit_Ugly mirrors Detect_NonZeroExit —
// captureCommand itself is where the "read output regardless of exit
// code" contract is implemented.
func TestDetect_CaptureCommand_NonZeroExit_Ugly(t *testing.T) {
	ensureProcess(t)
	dir := t.TempDir()
	script := writeScript(t, dir, "failer", `echo "output before failure"
exit 3
`)
	out := captureCommand(script)
	if !strings.Contains(out, "output before failure") {
		t.Errorf("captureCommand(nonzero exit) = %q, want output preserved", out)
	}
}

// --- resolveLintBinary — explicit override → installed → dev build → PATH ---

func TestDetect_ResolveLintBinary_EnvOverride_Good(t *testing.T) {
	t.Setenv("CORE_LINT_BIN", "/custom/core-lint")
	if got := resolveLintBinary(); got != "/custom/core-lint" {
		t.Errorf("resolveLintBinary = %q, want env override", got)
	}
}

func TestDetect_ResolveLintBinary_Candidates_Good(t *testing.T) {
	t.Setenv("CORE_LINT_BIN", "")

	home := t.TempDir()
	t.Setenv("HOME", home)
	letheanBin := filepath.Join(home, "Lethean", "bin")
	if err := os.MkdirAll(letheanBin, 0o755); err != nil {
		t.Fatal(err)
	}
	letheanPath := filepath.Join(letheanBin, "core-lint")
	if err := os.WriteFile(letheanPath, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := resolveLintBinary(); got != letheanPath {
		t.Errorf("resolveLintBinary = %q, want the ~/Lethean/bin candidate %q", got, letheanPath)
	}

	// Second candidate only: ~/Code/core/lint/bin, no ~/Lethean/bin entry.
	home2 := t.TempDir()
	t.Setenv("HOME", home2)
	codeBin := filepath.Join(home2, "Code", "core", "lint", "bin")
	if err := os.MkdirAll(codeBin, 0o755); err != nil {
		t.Fatal(err)
	}
	codePath := filepath.Join(codeBin, "core-lint")
	if err := os.WriteFile(codePath, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := resolveLintBinary(); got != codePath {
		t.Errorf("resolveLintBinary = %q, want the ~/Code/core/lint/bin candidate %q", got, codePath)
	}
}

func TestDetect_ResolveLintBinary_Fallback_Bad(t *testing.T) {
	t.Setenv("CORE_LINT_BIN", "")
	t.Setenv("HOME", t.TempDir()) // neither candidate exists
	if got := resolveLintBinary(); got != lintBinaryName {
		t.Errorf("resolveLintBinary fallback = %q, want bare %q", got, lintBinaryName)
	}
}

// --- lintProcID — pure translation of a process.Start Result ---

func TestDetect_LintProcID_Good(t *testing.T) {
	r := core.Ok(&process.Process{ID: "proc-123"})
	if got := lintProcID(r); got != "proc-123" {
		t.Errorf("lintProcID(*Process) = %q", got)
	}
	r2 := core.Ok("proc-456")
	if got := lintProcID(r2); got != "proc-456" {
		t.Errorf("lintProcID(string) = %q", got)
	}
}

func TestDetect_LintProcID_Bad(t *testing.T) {
	r := core.Ok(42)
	if got := lintProcID(r); got != "" {
		t.Errorf("lintProcID(int) = %q, want empty (unexpected type)", got)
	}
}

// --- lintDescription — sealed task body composition ---

func TestDetect_LintDescription_Good(t *testing.T) {
	f := lintFinding{File: "a.go", Line: 3, Severity: "error", Message: "bad thing", Fix: "do this instead"}
	got := lintDescription(f)
	for _, want := range []string{"a.go:3 — error", "bad thing", "Fix: do this instead"} {
		if !strings.Contains(got, want) {
			t.Errorf("lintDescription missing %q in %q", want, got)
		}
	}
}

func TestDetect_LintDescription_Bad(t *testing.T) {
	// No message, no fix — only the file:line — severity locator line.
	f := lintFinding{File: "b.go", Line: 7, Severity: "warning"}
	got := lintDescription(f)
	if strings.Contains(got, "Fix:") {
		t.Errorf("lintDescription should omit the Fix section: %q", got)
	}
	if !strings.Contains(got, "b.go:7 — warning") {
		t.Errorf("lintDescription = %q", got)
	}
}

func TestDetect_LintDescription_Ugly(t *testing.T) {
	// Message but no fix — the middle case between Good and Bad.
	f := lintFinding{File: "c.go", Line: 1, Severity: "info", Message: "just a note"}
	got := lintDescription(f)
	if strings.Contains(got, "Fix:") {
		t.Error("lintDescription should not include a Fix section")
	}
	if !strings.Contains(got, "just a note") {
		t.Error("lintDescription should include the message")
	}
}

// --- existingFingerprints — dedup read over OPEN detector-filed tasks ---

func TestDetect_ExistingFingerprints_Good(t *testing.T) {
	c := newDetectCore(t)
	f := lintFinding{Tool: "gosec", RuleID: "G104", File: "a.go", Line: 3}
	fp := lintFingerprint(f)
	r := Create(c, CreateInput{Project: "x", Summary: lintSummary(f, fp), Reporter: lintReporter})
	if !r.OK {
		t.Fatalf("Create: %s", r.Error())
	}
	seen := existingFingerprints(c, "x", lintReporter)
	if !seen[fp] {
		t.Errorf("existingFingerprints missing %q, got %v", fp, seen)
	}
}

func TestDetect_ExistingFingerprints_Bad(t *testing.T) {
	// A closed (done) issue's fingerprint is excluded — recurrence must
	// be able to re-file once the prior task is resolved.
	c := newDetectCore(t)
	f := lintFinding{Tool: "gosec", RuleID: "G104", File: "a.go", Line: 3}
	fp := lintFingerprint(f)
	r := Create(c, CreateInput{Project: "x", Summary: lintSummary(f, fp), Reporter: lintReporter})
	issue, _, ok := orm.Detail[Issue](r)
	if !ok {
		t.Fatal("cast issue")
	}
	if cr := Close(c, issue.ID, "fixed"); !cr.OK {
		t.Fatalf("Close: %s", cr.Error())
	}
	seen := existingFingerprints(c, "x", lintReporter)
	if seen[fp] {
		t.Errorf("existingFingerprints should exclude a closed issue, got %v", seen)
	}
}

func TestDetect_ExistingFingerprints_Ugly(t *testing.T) {
	// A Core whose orm has no mounted store — List fails — degrades to
	// an empty map rather than panicking (the detector must never crash
	// the boot path over a storage hiccup).
	c := core.New()
	orm.Register(c)
	seen := existingFingerprints(c, "x", lintReporter)
	if len(seen) != 0 {
		t.Errorf("existingFingerprints on a broken core = %v, want empty", seen)
	}
}

// --- ClearDetected — detector-owned hard delete ---

func TestDetect_ClearDetected_Good(t *testing.T) {
	c := newDetectCore(t)
	mustCreate(t, c, CreateInput{Project: "a", Summary: "[gosec:G1@x.go:1] lint 1", Reporter: lintReporter})
	mustCreate(t, c, CreateInput{Project: "a", Summary: "[package-update:go@a] updates", Reporter: updateReporter})
	mustCreate(t, c, CreateInput{Project: "a", Summary: "human task", Reporter: "alice"})

	r := ClearDetected(c, "")
	if !r.OK {
		t.Fatalf("ClearDetected: %s", r.Error())
	}
	out, ok := r.Value.(ClearOutput)
	if !ok {
		t.Fatalf("ClearDetected value type = %T", r.Value)
	}
	if out.Cleared != 2 {
		t.Errorf("Cleared = %d, want 2", out.Cleared)
	}
	remaining, ok := orm.Cast[[]Issue](List(c, ListFilter{Project: "a"}))
	if !ok {
		t.Fatal("cast remaining issues")
	}
	if len(remaining) != 1 || remaining[0].Reporter != "alice" {
		t.Errorf("remaining = %+v, want only the human task", remaining)
	}
}

func TestDetect_ClearDetected_DeleteFails_Bad(t *testing.T) {
	// orm registered but no medium mounted / no schema — DeleteAll fails
	// and ClearDetected surfaces it rather than reporting a false Cleared.
	c := core.New()
	if r := orm.Register(c); !r.OK {
		t.Fatalf("orm.Register: %s", r.Error())
	}
	r := ClearDetected(c, "")
	if r.OK {
		t.Error("ClearDetected on an unmounted core should fail")
	}
	if !strings.Contains(r.Error(), "clear "+lintReporter) && !strings.Contains(r.Error(), "clear "+updateReporter) {
		t.Errorf("error = %q, want it to name the failing reporter clear", r.Error())
	}
}

func TestDetect_ClearDetected_ScopedMultiProject_Ugly(t *testing.T) {
	c := newDetectCore(t)
	mustCreate(t, c, CreateInput{Project: "a", Summary: "[gosec:G1@x.go:1] lint a", Reporter: lintReporter})
	mustCreate(t, c, CreateInput{Project: "b", Summary: "[gosec:G2@y.go:2] lint b", Reporter: lintReporter})

	r := ClearDetected(c, "a")
	if !r.OK {
		t.Fatalf("ClearDetected: %s", r.Error())
	}
	out, _ := r.Value.(ClearOutput)
	if out.Cleared != 1 {
		t.Errorf("Cleared = %d, want 1 (scoped to project a)", out.Cleared)
	}
	bIssues, ok := orm.Cast[[]Issue](List(c, ListFilter{Project: "b"}))
	if !ok || len(bIssues) != 1 {
		t.Errorf("project b should be untouched, got %+v", bIssues)
	}

	// Nothing left to clear in an empty project — OK with Cleared=0.
	r2 := ClearDetected(c, "empty-project")
	if !r2.OK {
		t.Fatalf("ClearDetected(empty project): %s", r2.Error())
	}
	out2, _ := r2.Value.(ClearOutput)
	if out2.Cleared != 0 {
		t.Errorf("Cleared = %d, want 0", out2.Cleared)
	}
}

// mustCreate is a small local helper — Create + require OK — used by
// the ClearDetected fixtures above where the created record itself
// isn't otherwise inspected.
func mustCreate(t *testing.T, c *core.Core, input CreateInput) {
	t.Helper()
	if r := Create(c, input); !r.OK {
		t.Fatalf("Create: %s", r.Error())
	}
}

// --- Service.Detect / WailsService.Detect / Service.ClearDetected /
// WailsService.ClearDetected — tier gate + delegation ---

func TestDetect_ServiceDetect_Bad(t *testing.T) {
	c := newDetectCore(t)
	svc := NewService(c)
	// No caller stamped — resolves to the TierInternal floor, denied.
	if r := svc.Detect(DetectInput{Repo: "x", Path: t.TempDir()}); r.OK {
		t.Error("Service.Detect should reject an unstamped (TierInternal) caller")
	}
}

func TestDetect_ServiceDetect_Good(t *testing.T) {
	ensureProcess(t)
	c := newDetectCore(t)
	dir := t.TempDir()
	script := writeScript(t, dir, "core-lint", `echo '{"findings":[]}'`)
	t.Setenv("CORE_LINT_BIN", script)
	auth.SetCaller(c, auth.CallerIdentity{Tier: auth.TierOperator, Subject: "op", Source: "test"})
	svc := NewService(c)
	if r := svc.Detect(DetectInput{Repo: "x", Path: dir}); !r.OK {
		t.Fatalf("Service.Detect: %s", r.Error())
	}
}

func TestDetect_WailsServiceDetect_Ugly(t *testing.T) {
	ensureProcess(t)
	c := newDetectCore(t)
	dir := t.TempDir()
	script := writeScript(t, dir, "core-lint", `echo '{"findings":[]}'`)
	t.Setenv("CORE_LINT_BIN", script)
	// No pre-stamp — the WailsService wrapper stamps TierRenderer itself
	// before delegating, so this must succeed without an explicit
	// auth.SetCaller call (the whole point of the wrapper).
	w := NewWailsService(NewService(c))
	if r := w.Detect(DetectInput{Repo: "x", Path: dir}); !r.OK {
		t.Fatalf("WailsService.Detect: %s", r.Error())
	}
}

func TestDetect_ServiceClearDetected_Bad(t *testing.T) {
	c := newDetectCore(t)
	svc := NewService(c)
	if r := svc.ClearDetected(ClearInput{}); r.OK {
		t.Error("Service.ClearDetected should reject an unstamped caller")
	}
}

func TestDetect_ServiceClearDetected_Good(t *testing.T) {
	c := newDetectCore(t)
	mustCreate(t, c, CreateInput{Project: "x", Summary: "[t:1@a.go:1] lint", Reporter: lintReporter})
	auth.SetCaller(c, auth.CallerIdentity{Tier: auth.TierOperator, Subject: "op", Source: "test"})
	svc := NewService(c)
	r := svc.ClearDetected(ClearInput{Project: "x"})
	if !r.OK {
		t.Fatalf("Service.ClearDetected: %s", r.Error())
	}
}

func TestDetect_WailsServiceClearDetected_Ugly(t *testing.T) {
	c := newDetectCore(t)
	mustCreate(t, c, CreateInput{Project: "x", Summary: "[t:1@a.go:1] lint", Reporter: lintReporter})
	w := NewWailsService(NewService(c))
	r := w.ClearDetected(ClearInput{Project: "x"})
	if !r.OK {
		t.Fatalf("WailsService.ClearDetected: %s", r.Error())
	}
}

// TestDetect_ParseReport_UnterminatedJSON_Bad covers jsonObjectSlice's
// other failure branch (found an opening '{' but no closing '}' at
// all) — distinct from TestDetect_ParseReport_Bad, which never finds a
// '{' in the first place.
func TestDetect_ParseReport_UnterminatedJSON_Bad(t *testing.T) {
	if _, ok := parseLintReport("noise\n{ this never closes"); ok {
		t.Error("parseLintReport should return ok=false for an unterminated JSON object")
	}
}

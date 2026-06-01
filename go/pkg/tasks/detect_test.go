// SPDX-Licence-Identifier: EUPL-1.2

package tasks

import "testing"

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

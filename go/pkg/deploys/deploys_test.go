// SPDX-Licence-Identifier: EUPL-1.2

package deploys

import (
	"testing"

	core "dappco.re/go"
)

// mustParseTime parses RFC3339 or fatals.
func mustParseTime(t *testing.T, s string) core.Time {
	t.Helper()
	r := core.TimeParse(core.TimeRFC3339, s)
	if !r.OK {
		t.Fatalf("TimeParse(%q): %v", s, r.Error())
	}
	return r.Value.(core.Time)
}

// TestGenerateID_Good — valid Time produces expected format.
func TestGenerateID_Good(t *testing.T) {
	ts := mustParseTime(t, "2026-05-16T14:32:00Z")
	got := generateID(ts)
	want := "deploy-20260516-1432"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

// TestGenerateID_Bad — zero time → format still valid (zero values).
func TestGenerateID_Bad(t *testing.T) {
	got := generateID(core.Time{})
	if !core.HasPrefix(got, "deploy-") {
		t.Fatalf("expected 'deploy-' prefix, got %q", got)
	}
	if len(got) != len("deploy-20260516-1432") {
		t.Fatalf("unexpected length %d for %q", len(got), got)
	}
}

// TestGenerateID_Ugly — two deploys in same minute produce identical IDs.
func TestGenerateID_Ugly(t *testing.T) {
	ts1 := mustParseTime(t, "2026-05-16T14:32:05Z")
	ts2 := mustParseTime(t, "2026-05-16T14:32:59Z")
	if generateID(ts1) != generateID(ts2) {
		t.Fatal("same-minute deploys must produce identical IDs")
	}
}

// TestRelativeAge_Good — canonical durations produce expected strings.
func TestRelativeAge_Good(t *testing.T) {
	now := mustParseTime(t, "2026-05-16T14:00:00Z")

	cases := []struct {
		offset string
		want   string
	}{
		{"2026-05-16T13:38:00Z", "22m"},
		{"2026-05-16T12:00:00Z", "2h"},
		{"2026-05-12T14:00:00Z", "4d"},
	}
	for _, c := range cases {
		ts := mustParseTime(t, c.offset)
		got := relativeAge(ts, now)
		if got != c.want {
			t.Errorf("relativeAge(%q) = %q, want %q", c.offset, got, c.want)
		}
	}
}

// TestRelativeAge_Bad — zero time → empty string.
func TestRelativeAge_Bad(t *testing.T) {
	now := mustParseTime(t, "2026-05-16T14:00:00Z")
	got := relativeAge(core.Time{}, now)
	if got != "" {
		t.Fatalf("expected empty string for zero time, got %q", got)
	}
}

// TestRelativeAge_Ugly — future timestamp (negative diff) → "0m".
func TestRelativeAge_Ugly(t *testing.T) {
	now := mustParseTime(t, "2026-05-16T14:00:00Z")
	future := mustParseTime(t, "2026-05-16T15:00:00Z")
	got := relativeAge(future, now)
	if got != "0m" {
		t.Fatalf("expected '0m' for future ts, got %q", got)
	}
}

// TestDeriveHealth_Good — "failed" outcome → "failed" health.
func TestDeriveHealth_Good(t *testing.T) {
	if deriveHealth("failed") != "failed" {
		t.Fatal("expected 'failed' for outcome='failed'")
	}
}

// TestDeriveHealth_Bad — empty outcome → "ok".
func TestDeriveHealth_Bad(t *testing.T) {
	if deriveHealth("") != "ok" {
		t.Fatal("expected 'ok' for empty outcome")
	}
}

// TestDeriveHealth_Ugly — non-failure outcomes all map to "ok".
func TestDeriveHealth_Ugly(t *testing.T) {
	for _, outcome := range []string{"success", "rolled-back", "pending"} {
		if deriveHealth(outcome) != "ok" {
			t.Errorf("expected 'ok' for outcome=%q", outcome)
		}
	}
}

// TestParseFrontmatter_Good — valid trix file → record + notes decoded.
func TestParseFrontmatter_Good(t *testing.T) {
	raw := []byte("---\nid: deploy-20260516-1432\nenv: preview\nby: Tobi\ncommit: b8e034\noutcome: success\ndur: 58s\ntimestamp: 2026-05-16T14:32:00Z\n---\nDeploy notes here.\n")
	rec, notes, err := parseFrontmatter(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.ID != "deploy-20260516-1432" {
		t.Errorf("expected ID='deploy-20260516-1432', got %q", rec.ID)
	}
	if rec.Env != "preview" {
		t.Errorf("expected Env='preview', got %q", rec.Env)
	}
	if notes != "Deploy notes here.\n" {
		t.Errorf("unexpected notes: %q", notes)
	}
}

// TestParseFrontmatter_Bad — empty input → error.
func TestParseFrontmatter_Bad(t *testing.T) {
	_, _, err := parseFrontmatter(nil)
	if err == nil {
		t.Fatal("expected error on nil input")
	}
	_, _, err = parseFrontmatter([]byte{})
	if err == nil {
		t.Fatal("expected error on empty input")
	}
}

// TestParseFrontmatter_Ugly — no closing "---" → treated as frontmatter only, no error.
func TestParseFrontmatter_Ugly(t *testing.T) {
	raw := []byte("---\nid: deploy-20260516-1200\nenv: staging\nby: Ada\ncommit: aabbcc\noutcome: success\ndur: 1m\ntimestamp: 2026-05-16T12:00:00Z\n")
	rec, notes, err := parseFrontmatter(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Env != "staging" {
		t.Errorf("expected Env='staging', got %q", rec.Env)
	}
	if notes != "" {
		t.Errorf("expected empty notes, got %q", notes)
	}
}

// TestRenderTrix_Good — record + notes → valid trix bytes.
func TestRenderTrix_Good(t *testing.T) {
	ts := mustParseTime(t, "2026-05-16T14:32:00Z")
	rec := DeployRecord{
		ID:        "deploy-20260516-1432",
		Env:       "preview",
		By:        "Tobi",
		Commit:    "b8e034",
		Outcome:   "success",
		Dur:       "58s",
		Timestamp: ts,
	}
	out, err := renderTrix(rec, "Some notes.")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	s := string(out)
	if !core.HasPrefix(s, "---\n") {
		t.Fatal("expected trix output to start with '---\\n'")
	}
	if !core.Contains(s, "Some notes.") {
		t.Fatal("expected notes in trix output")
	}
}

// TestRenderTrix_Bad — zero record renders without error (yaml handles it).
func TestRenderTrix_Bad(t *testing.T) {
	_, err := renderTrix(DeployRecord{}, "")
	if err != nil {
		t.Fatalf("unexpected error rendering zero record: %v", err)
	}
}

// TestRenderTrix_Ugly — round-trip: render then parse back produces same values.
func TestRenderTrix_Ugly(t *testing.T) {
	ts := mustParseTime(t, "2026-05-16T14:32:00Z")
	rec := DeployRecord{
		ID:        "deploy-20260516-1432",
		Env:       "production",
		By:        "Ada",
		Commit:    "deadbeef",
		Outcome:   "success",
		Dur:       "2m 10s",
		Timestamp: ts,
	}
	raw, err := renderTrix(rec, "Round-trip test.")
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	got, notes, err := parseFrontmatter(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got.ID != rec.ID {
		t.Errorf("ID mismatch: got %q want %q", got.ID, rec.ID)
	}
	if got.Env != rec.Env {
		t.Errorf("Env mismatch: got %q want %q", got.Env, rec.Env)
	}
	if notes != "Round-trip test.\n" {
		t.Errorf("notes mismatch: got %q", notes)
	}
}

// TestBuildEnvRows_Good — newest record per env fills EnvRow correctly.
func TestBuildEnvRows_Good(t *testing.T) {
	now := mustParseTime(t, "2026-05-16T14:00:00Z")
	ts1 := mustParseTime(t, "2026-05-16T12:00:00Z")
	ts2 := mustParseTime(t, "2026-05-16T10:00:00Z")

	records := []DeployRecord{
		{Env: "production", Commit: "aaa", Outcome: "success", Timestamp: ts1},
		{Env: "production", Commit: "bbb", Outcome: "failed", Timestamp: ts2},
		{Env: "staging", Commit: "ccc", Outcome: "success", Timestamp: ts2},
	}
	rows := buildEnvRows(records, now)
	if len(rows) != 2 {
		t.Fatalf("expected 2 env rows, got %d", len(rows))
	}
	if rows[0].Name != "production" {
		t.Errorf("expected first row='production', got %q", rows[0].Name)
	}
	if rows[0].Health != "ok" {
		t.Errorf("expected health='ok', got %q", rows[0].Health)
	}
	if rows[0].Commit != "aaa" {
		t.Errorf("expected commit='aaa' (newest), got %q", rows[0].Commit)
	}
}

// TestBuildEnvRows_Bad — empty records → empty result (no panic).
func TestBuildEnvRows_Bad(t *testing.T) {
	now := mustParseTime(t, "2026-05-16T14:00:00Z")
	rows := buildEnvRows(nil, now)
	if rows == nil {
		t.Fatal("expected non-nil slice, got nil")
	}
	if len(rows) != 0 {
		t.Fatalf("expected empty rows, got %d", len(rows))
	}
}

// TestBuildEnvRows_Ugly — canonical order: production → staging → preview → others alpha.
func TestBuildEnvRows_Ugly(t *testing.T) {
	now := mustParseTime(t, "2026-05-16T14:00:00Z")
	ts := mustParseTime(t, "2026-05-16T13:00:00Z")
	records := []DeployRecord{
		{Env: "zeta", Outcome: "success", Timestamp: ts},
		{Env: "preview", Outcome: "success", Timestamp: ts},
		{Env: "alpha-custom", Outcome: "success", Timestamp: ts},
		{Env: "staging", Outcome: "success", Timestamp: ts},
		{Env: "production", Outcome: "success", Timestamp: ts},
	}
	rows := buildEnvRows(records, now)
	if len(rows) != 5 {
		t.Fatalf("expected 5 rows, got %d", len(rows))
	}
	order := []string{"production", "staging", "preview", "alpha-custom", "zeta"}
	for i, want := range order {
		if rows[i].Name != want {
			t.Errorf("row[%d]: expected %q, got %q", i, want, rows[i].Name)
		}
	}
}

// TestToDeployRow_Good — record converts to wire shape correctly.
func TestToDeployRow_Good(t *testing.T) {
	now := mustParseTime(t, "2026-05-16T14:00:00Z")
	ts := mustParseTime(t, "2026-05-16T13:38:00Z")
	rec := DeployRecord{
		Env: "staging", By: "Ada", Commit: "abc123",
		Outcome: "success", Dur: "1m", Timestamp: ts,
	}
	row := toDeployRow(rec, now)
	if row.Ts != "22m" {
		t.Errorf("expected Ts='22m', got %q", row.Ts)
	}
	if row.Env != "staging" {
		t.Errorf("expected Env='staging', got %q", row.Env)
	}
	if row.Commit != "abc123" {
		t.Errorf("expected Commit='abc123', got %q", row.Commit)
	}
}

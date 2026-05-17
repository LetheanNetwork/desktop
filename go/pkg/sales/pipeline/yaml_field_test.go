// SPDX-Licence-Identifier: EUPL-1.2

// yaml_field_test.go — white-box (`package pipeline`) coverage for the
// frontmatter-bounded updateYAMLField + setVersionField helpers.
// Cerberus #1545 (HIGH-but-down-graded): the prior full-file scan
// silently corrupted body lines beginning "version: " and skipped the
// frontmatter stamp entirely, leaving optimistic-lock permanently
// bypassed on the affected record.

package pipeline

import (
	"testing"
)

// TestPipeline_UpdateYAMLField_BodyVersionStringIgnored_Ugly — a Trix
// file whose BODY contains a line beginning "version: " must NOT have
// that body line touched by updateYAMLField stamping the frontmatter
// "version" field. Pins the frontmatter-boundary discipline (Mantis
// #1545).
func TestPipeline_UpdateYAMLField_BodyVersionStringIgnored_Ugly(t *testing.T) {
	raw := []byte("---\nid: legacy\nstage: engage\n---\n## notes\n\nversion: 1.2.3 of the contract\n")
	out := updateYAMLField(raw, "version", "1")
	// updateYAMLField returns the original bytes unchanged when no
	// match lands inside the bounded frontmatter — the body line
	// "version: 1.2.3 of the contract" MUST NOT match.
	if string(out) != string(raw) {
		t.Fatalf("body version-string was corrupted by updateYAMLField:\nwant: %q\ngot:  %q", raw, out)
	}
}

// TestPipeline_UpdateYAMLField_FrontmatterStampWorks_Good — when the
// frontmatter DOES contain the target key, updateYAMLField rewrites
// it in place and leaves both the body and the closing delimiter
// intact.
func TestPipeline_UpdateYAMLField_FrontmatterStampWorks_Good(t *testing.T) {
	raw := []byte("---\nid: deal-1\nversion: 5\nstage: engage\n---\nbody text\n")
	out := updateYAMLField(raw, "version", "6")
	want := "---\nid: deal-1\nversion: 6\nstage: engage\n---\nbody text\n"
	if string(out) != want {
		t.Fatalf("frontmatter stamp wrong:\nwant: %q\ngot:  %q", want, out)
	}
}

// TestPipeline_SetVersionField_BodyVersionStringPreserved_Ugly — the
// stamp-or-insert helper MUST insert a fresh "version: N" line into
// the frontmatter and MUST leave a body line literally starting
// "version: " untouched (Mantis #1545 — primary harm: optimistic-lock
// permanently bypassed when setVersionField returns without
// stamping).
func TestPipeline_SetVersionField_BodyVersionStringPreserved_Ugly(t *testing.T) {
	raw := []byte("---\nid: legacy\nstage: engage\n---\n## notes\n\nversion: 1.2.3 of the contract\n")
	out := setVersionField(raw, 7)
	want := "---\nversion: 7\nid: legacy\nstage: engage\n---\n## notes\n\nversion: 1.2.3 of the contract\n"
	if string(out) != want {
		t.Fatalf("setVersionField outcome wrong:\nwant: %q\ngot:  %q", want, out)
	}
}

// TestPipeline_SetVersionField_InPlaceUpdate_Good — when frontmatter
// already has a version line, setVersionField updates in place rather
// than inserting a duplicate.
func TestPipeline_SetVersionField_InPlaceUpdate_Good(t *testing.T) {
	raw := []byte("---\nversion: 3\nid: deal-1\nstage: engage\n---\nbody\n")
	out := setVersionField(raw, 4)
	want := "---\nversion: 4\nid: deal-1\nstage: engage\n---\nbody\n"
	if string(out) != want {
		t.Fatalf("in-place update wrong:\nwant: %q\ngot:  %q", want, out)
	}
}

// TestPipeline_UpdateYAMLField_NoFrontmatter_Ugly — a file without a
// "---\n" opener returns unchanged (matches paths.parseFrontmatterVersion
// "no opener → version 0" discipline).
func TestPipeline_UpdateYAMLField_NoFrontmatter_Ugly(t *testing.T) {
	raw := []byte("just a body\nversion: 1\nmore body\n")
	out := updateYAMLField(raw, "version", "9")
	if string(out) != string(raw) {
		t.Fatalf("non-frontmatter file modified:\nwant: %q\ngot:  %q", raw, out)
	}
}

// TestPipeline_UpdateYAMLField_NoCloser_Ugly — a file with an opener
// but no closing "---" line returns unchanged (refuses to guess the
// frontmatter boundary).
func TestPipeline_UpdateYAMLField_NoCloser_Ugly(t *testing.T) {
	raw := []byte("---\nid: x\nversion: 1\n")
	out := updateYAMLField(raw, "version", "9")
	if string(out) != string(raw) {
		t.Fatalf("unclosed frontmatter modified:\nwant: %q\ngot:  %q", raw, out)
	}
}

// TestPipeline_HasVersionInFrontmatter_HandlesIndented_Good — an
// indented frontmatter "  version: N" line must be visible to the
// writer (mirrors paths.matchVersionLine's leading-whitespace trim).
// Without this, setVersionField takes the insert path and produces a
// duplicate key on every legacy-file upgrade (Mantis #1549).
func TestPipeline_HasVersionInFrontmatter_HandlesIndented_Good(t *testing.T) {
	raw := []byte("---\nid: deal-1\n  version: 5\nstage: engage\n---\nbody\n")
	if !hasVersionInFrontmatter(raw) {
		t.Fatalf("indented version line invisible to hasVersionInFrontmatter")
	}
}

// TestPipeline_UpdateYAMLField_HandlesIndented_Good — updating an
// indented frontmatter version line preserves the original indentation
// instead of rewriting at column 0.
func TestPipeline_UpdateYAMLField_HandlesIndented_Good(t *testing.T) {
	raw := []byte("---\nid: deal-1\n  version: 5\nstage: engage\n---\nbody\n")
	out := updateYAMLField(raw, "version", "6")
	want := "---\nid: deal-1\n  version: 6\nstage: engage\n---\nbody\n"
	if string(out) != want {
		t.Fatalf("indented update wrong:\nwant: %q\ngot:  %q", want, out)
	}
}

// TestPipeline_SetVersionField_UpdatesIndentedInPlace_Good — combined
// proof of #1549: a legacy file with an indented version line gets
// updated in place (no duplicate key insertion).
func TestPipeline_SetVersionField_UpdatesIndentedInPlace_Good(t *testing.T) {
	raw := []byte("---\nid: deal-1\n  version: 5\nstage: engage\n---\nbody\n")
	out := setVersionField(raw, 6)
	want := "---\nid: deal-1\n  version: 6\nstage: engage\n---\nbody\n"
	if string(out) != want {
		t.Fatalf("indented in-place update wrong:\nwant: %q\ngot:  %q", want, out)
	}
}

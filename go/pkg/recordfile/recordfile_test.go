// SPDX-Licence-Identifier: EUPL-1.2

// recordfile_test.go — Good/Bad/Ugly per Split + Stitch (Mantis
// #1494). Each test asserts byte-shape parity with the legacy
// per-package parseRecord/marshalRecord behaviour so the migration
// from those local helpers is observable as a no-op at the byte
// layer.

package recordfile_test

import (
	"testing"

	"dappco.re/lthn/desktop/pkg/recordfile"
)

// --- Split ------------------------------------------------------------

func TestRecordfile_Split_Good(t *testing.T) {
	raw := []byte("---\nname: rec\nstage: qual\n---\nbody bytes\n")
	fm, body := recordfile.Split(raw)
	if string(fm) != "name: rec\nstage: qual\n" {
		t.Fatalf("unexpected frontmatter: %q", string(fm))
	}
	if string(body) != "body bytes\n" {
		t.Fatalf("unexpected body: %q", string(body))
	}
}

func TestRecordfile_Split_Good_EmptyBody(t *testing.T) {
	// Legacy marshalRecord emits "---\n<fm>---\n" with nothing after
	// the closing delimiter when the body is empty. Split MUST
	// produce an empty (non-nil) body slice on that shape so a
	// round-trip caller sees zero-length, not nil.
	raw := []byte("---\nname: rec\n---\n")
	fm, body := recordfile.Split(raw)
	if string(fm) != "name: rec\n" {
		t.Fatalf("unexpected frontmatter: %q", string(fm))
	}
	if len(body) != 0 {
		t.Fatalf("expected empty body, got %d bytes: %q", len(body), string(body))
	}
}

func TestRecordfile_Split_Bad_NoClosingDelimiter(t *testing.T) {
	// Per legacy parseRecord: no closing delimiter ⇒ the entire
	// post-opening content is frontmatter; body is nil. The caller
	// is then free to yaml.Unmarshal the frontmatter (which will
	// typically succeed if the YAML is otherwise valid).
	raw := []byte("---\nname: rec\nstage: qual\n")
	fm, body := recordfile.Split(raw)
	if string(fm) != "name: rec\nstage: qual\n" {
		t.Fatalf("unexpected frontmatter: %q", string(fm))
	}
	if body != nil {
		t.Fatalf("expected nil body, got %q", string(body))
	}
}

func TestRecordfile_Split_Bad_NoOpeningDelimiter(t *testing.T) {
	// Per legacy parseRecord: missing opening "---\n" ⇒ scan from
	// the start of input for a closing "---" line. Mirrors the
	// "files written before frontmatter was introduced" tolerance.
	raw := []byte("name: rec\n---\nbody\n")
	fm, body := recordfile.Split(raw)
	if string(fm) != "name: rec\n" {
		t.Fatalf("unexpected frontmatter: %q", string(fm))
	}
	if string(body) != "body\n" {
		t.Fatalf("unexpected body: %q", string(body))
	}
}

func TestRecordfile_Split_Ugly_Empty(t *testing.T) {
	fm, body := recordfile.Split(nil)
	if fm != nil {
		t.Fatalf("expected nil frontmatter, got %q", string(fm))
	}
	if body != nil {
		t.Fatalf("expected nil body, got %q", string(body))
	}
}

func TestRecordfile_Split_Ugly_OnlyOpeningDelim(t *testing.T) {
	// "---\n" with nothing after — Split must not panic on a
	// content slice whose len is < 4 after the open-delim skip.
	raw := []byte("---\n")
	fm, body := recordfile.Split(raw)
	if len(fm) != 0 {
		t.Fatalf("expected empty frontmatter, got %q", string(fm))
	}
	if body != nil {
		t.Fatalf("expected nil body, got %q", string(body))
	}
}

func TestRecordfile_Split_Ugly_TripleDashInBody(t *testing.T) {
	// A line starting with "---" inside the BODY is none of our
	// business — the body split picks the first closing delim,
	// not the last. Legacy parseRecord matches this.
	raw := []byte("---\nname: rec\n---\nbody one\n---\nbody two\n")
	fm, body := recordfile.Split(raw)
	if string(fm) != "name: rec\n" {
		t.Fatalf("unexpected frontmatter: %q", string(fm))
	}
	if string(body) != "body one\n---\nbody two\n" {
		t.Fatalf("unexpected body: %q", string(body))
	}
}

// --- Stitch -----------------------------------------------------------

func TestRecordfile_Stitch_Good(t *testing.T) {
	fm := []byte("name: rec\nstage: qual\n")
	body := []byte("body bytes\n")
	got := recordfile.Stitch(fm, body)
	want := "---\nname: rec\nstage: qual\n---\nbody bytes\n"
	if string(got) != want {
		t.Fatalf("Stitch mismatch:\n  got:  %q\n  want: %q", string(got), want)
	}
}

func TestRecordfile_Stitch_Good_EmptyBody(t *testing.T) {
	fm := []byte("name: rec\n")
	got := recordfile.Stitch(fm, nil)
	want := "---\nname: rec\n---\n"
	if string(got) != want {
		t.Fatalf("Stitch mismatch:\n  got:  %q\n  want: %q", string(got), want)
	}
}

func TestRecordfile_Stitch_Bad_EmptyFrontmatter(t *testing.T) {
	// Empty frontmatter shouldn't panic. Output is "---\n---\nbody"
	// — yaml.Unmarshal of "" decodes to a zero-value record, which
	// is the consuming package's call to accept or reject.
	got := recordfile.Stitch(nil, []byte("body\n"))
	want := "---\n---\nbody\n"
	if string(got) != want {
		t.Fatalf("Stitch mismatch:\n  got:  %q\n  want: %q", string(got), want)
	}
}

func TestRecordfile_Stitch_Ugly_BothEmpty(t *testing.T) {
	got := recordfile.Stitch(nil, nil)
	want := "---\n---\n"
	if string(got) != want {
		t.Fatalf("Stitch mismatch:\n  got:  %q\n  want: %q", string(got), want)
	}
}

// --- Round-trip -------------------------------------------------------

func TestRecordfile_RoundTrip_Good(t *testing.T) {
	fm := []byte("name: rec\nstage: qual\n")
	body := []byte("body bytes\n")
	raw := recordfile.Stitch(fm, body)
	gotFM, gotBody := recordfile.Split(raw)
	if string(gotFM) != string(fm) {
		t.Fatalf("FM not preserved: got %q want %q", string(gotFM), string(fm))
	}
	if string(gotBody) != string(body) {
		t.Fatalf("Body not preserved: got %q want %q", string(gotBody), string(body))
	}
}

func TestRecordfile_RoundTrip_Ugly_EmptyBody(t *testing.T) {
	fm := []byte("name: rec\n")
	raw := recordfile.Stitch(fm, nil)
	gotFM, gotBody := recordfile.Split(raw)
	if string(gotFM) != string(fm) {
		t.Fatalf("FM not preserved: got %q want %q", string(gotFM), string(fm))
	}
	if len(gotBody) != 0 {
		t.Fatalf("expected empty body, got %d bytes: %q", len(gotBody), string(gotBody))
	}
}

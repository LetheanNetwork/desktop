// SPDX-Licence-Identifier: EUPL-1.2

// bodies_extra_test.go — direct coverage of extractPayload's LF-only
// and no-boundary branches, decodeBodyToOutput's empty/allowHTML
// paths, and fetchBodyFromIMAP's no-override default — none of which
// bodies_test.go's FetchBody-level tests reach (that file only
// exercises the CRLF-boundary branch via a real FetchBody round-trip).

package mail

import (
	"testing"

	core "dappco.re/go"
)

// TestExtractPayload_CRLFBoundary_Good — "\r\n\r\n" boundary found.
func TestExtractPayload_CRLFBoundary_Good(t *testing.T) {
	got := extractPayload([]byte("Subject: hi\r\n\r\nbody text"))
	if got != "body text" {
		t.Errorf("got %q, want %q", got, "body text")
	}
}

// TestExtractPayload_LFOnlyBoundary_Good — falls back to "\n\n" when
// no CRLF boundary is present (fixture-style input).
func TestExtractPayload_LFOnlyBoundary_Good(t *testing.T) {
	got := extractPayload([]byte("Subject: hi\n\nbody text"))
	if got != "body text" {
		t.Errorf("got %q, want %q", got, "body text")
	}
}

// TestExtractPayload_NoBoundary_Ugly — no header/body boundary at
// all → the raw bytes are returned verbatim.
func TestExtractPayload_NoBoundary_Ugly(t *testing.T) {
	got := extractPayload([]byte("just a plain string, no headers"))
	if got != "just a plain string, no headers" {
		t.Errorf("got %q, want input verbatim", got)
	}
}

// TestDecodeBodyToOutput_Empty_Bad — zero-length input short-circuits
// to a zero-value FetchBodyOutput.
func TestDecodeBodyToOutput_Empty_Bad(t *testing.T) {
	got := decodeBodyToOutput(nil, false)
	if got.Plain != "" || got.HTML != "" {
		t.Errorf("expected zero-value output for empty input, got %+v", got)
	}
}

// TestDecodeBodyToOutput_AllowHTML_Ugly — v1 never populates HTML
// even when AllowHTML=true; the field stays "" rather than panicking
// or echoing the plain payload.
func TestDecodeBodyToOutput_AllowHTML_Ugly(t *testing.T) {
	got := decodeBodyToOutput([]byte("Subject: x\r\n\r\nplain body"), true)
	if got.Plain != "plain body" {
		t.Errorf("Plain = %q, want %q", got.Plain, "plain body")
	}
	if got.HTML != "" {
		t.Errorf("HTML must stay empty in v1 even when AllowHTML=true, got %q", got.HTML)
	}
}

// TestFetchBodyFromIMAP_NoOverride_Good — with no bodyFetchOverride
// set (production default shape), fetchBodyFromIMAP returns (nil,
// nil) and FetchBody surfaces the "fetching…" empty-output shape.
func TestFetchBodyFromIMAP_NoOverride_Good(t *testing.T) {
	svc, _ := newTestMailService(t)
	// bodyFetchOverride intentionally left nil.

	r := svc.FetchBody(FetchBodyInput{
		AccountName: "personal",
		FolderSlug:  "inbox",
		ThreadID:    "no-override-thread",
	})
	if !r.OK {
		t.Fatalf("FetchBody: %s", r.Error())
	}
	out, ok := r.Value.(FetchBodyOutput)
	if !ok {
		t.Fatalf("FetchBody returned non-FetchBodyOutput")
	}
	if out.Plain != "" || out.HTML != "" {
		t.Errorf("expected empty output when no fetch hook is wired, got %+v", out)
	}

	// No on-disk artefact should have been written for the empty-body
	// "not available yet" path.
	pathR := bodyFilePath("inbox", "no-override-thread")
	if !pathR.OK {
		t.Fatalf("bodyFilePath: %s", pathR.Error())
	}
	if statR := core.Stat(pathR.Value.(string)); statR.OK {
		t.Errorf("expected no body file on disk when fetchBodyFromIMAP returns empty bytes")
	}
}

// TestFetchBodyFromIMAP_OverrideError_Bad — an override that returns
// an error propagates through FetchBody as a Fail Result.
func TestFetchBodyFromIMAP_OverrideError_Bad(t *testing.T) {
	svc, _ := newTestMailService(t)
	svc.bodyFetchOverride = func(_ FetchBodyInput) ([]byte, error) {
		return nil, core.E("mail.test", "synthetic IMAP fetch failure", nil)
	}

	r := svc.FetchBody(FetchBodyInput{
		AccountName: "personal",
		FolderSlug:  "inbox",
		ThreadID:    "err-thread",
	})
	if r.OK {
		t.Fatal("expected FetchBody to fail when fetchBodyFromIMAP errors")
	}
	if !core.Contains(r.Error(), "IMAP body fetch failed") {
		t.Errorf("unexpected error: %s", r.Error())
	}
}

// TestBodyDirPath_InvalidFolderSlug_Bad — traversal-shaped folder
// slug is rejected before any directory creation.
func TestBodyDirPath_InvalidFolderSlug_Bad(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	r := bodyDirPath("../escape")
	if r.OK {
		t.Fatal("expected bodyDirPath to reject a traversal folder slug")
	}
}

// TestBodyFilePath_InvalidThreadID_Bad — traversal-shaped thread id
// is rejected before path composition.
func TestBodyFilePath_InvalidThreadID_Bad(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	r := bodyFilePath("inbox", "../escape")
	if r.OK {
		t.Fatal("expected bodyFilePath to reject a traversal thread id")
	}
}

// TestFetchBody_CachedReadFails_Bad — Stat sees the file (it exists)
// but ReadFile fails because the "file" is actually a directory —
// exercises FetchBody's cached-read-failure branch without needing a
// permissions trick that would be fragile across CI users/root.
func TestFetchBody_CachedReadFails_Bad(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc, _ := newTestMailService(t)

	pathR := bodyFilePath("inbox", "dir-thread")
	if !pathR.OK {
		t.Fatalf("bodyFilePath: %s", pathR.Error())
	}
	// Make the cache "file" a directory so Stat succeeds but ReadFile fails.
	if r := core.MkdirAll(pathR.Value.(string), 0o700); !r.OK {
		t.Fatalf("mkdir cache-path-as-dir: %s", r.Error())
	}

	r := svc.FetchBody(FetchBodyInput{
		AccountName: "personal",
		FolderSlug:  "inbox",
		ThreadID:    "dir-thread",
	})
	if r.OK {
		t.Fatal("expected FetchBody to fail when the cached path is a directory")
	}
	if !core.Contains(r.Error(), "cached body read failed") {
		t.Errorf("unexpected error: %s", r.Error())
	}
}

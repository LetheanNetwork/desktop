// SPDX-Licence-Identifier: EUPL-1.2

// wails_surface_test.go — behavioural coverage for the wails.go
// entrypoints (ListFolders / ListThreads). wails_test.go only checks
// struct field shapes; these tests drive the real functions against
// a temp ~/Lethean/office/mail/ tree.

package mail

import (
	"testing"

	core "dappco.re/go"
)

// TestListFolders_EmptyDir_Good — no mail dir yet → empty (not nil)
// folder list, no error.
func TestListFolders_EmptyDir_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := &Service{core: core.New()}
	r := svc.ListFolders()
	if !r.OK {
		t.Fatalf("ListFolders: %s", r.Error())
	}
	out, ok := r.Value.(ListFoldersOutput)
	if !ok {
		t.Fatalf("ListFolders returned non-ListFoldersOutput value")
	}
	if out.Folders == nil {
		t.Errorf("Folders must be an empty slice, not nil")
	}
	if len(out.Folders) != 0 {
		t.Errorf("expected 0 folders, got %d", len(out.Folders))
	}
}

// TestListFolders_InboxPromoted_Good — inbox is always index 0; other
// folders are alphabetical; internal "_state" dir is skipped; unread
// counts derive from each folder's threads.md.
func TestListFolders_InboxPromoted_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := &Service{core: core.New()}

	for _, slug := range []string{"sent", "inbox", "archive"} {
		dirR := folderDir(slug)
		if !dirR.OK {
			t.Fatalf("folderDir(%s): %s", slug, dirR.Error())
		}
	}
	// Also create an internal "_state" dir under mailDir to confirm
	// it's skipped by scanFolders.
	base := mailDir()
	if !base.OK {
		t.Fatalf("mailDir: %s", base.Error())
	}
	if r := core.MkdirAll(core.PathJoin(base.Value.(string), "_state"), 0o700); !r.OK {
		t.Fatalf("mkdir _state: %s", r.Error())
	}

	// inbox gets one unread thread.
	inboxThreads := core.PathJoin(base.Value.(string), "inbox", "threads.md")
	if r := core.WriteFile(inboxThreads, []byte("---\nid: \"1\"\nfrom: \"A\"\nsubject: \"hi\"\nunread: true\n---\n"), 0o600); !r.OK {
		t.Fatalf("write inbox threads.md: %s", r.Error())
	}

	r := svc.ListFolders()
	if !r.OK {
		t.Fatalf("ListFolders: %s", r.Error())
	}
	out := r.Value.(ListFoldersOutput)
	if len(out.Folders) != 3 {
		t.Fatalf("expected 3 folders (sent, inbox, archive; _state skipped), got %d: %+v", len(out.Folders), out.Folders)
	}
	if out.Folders[0].Slug != "inbox" {
		t.Errorf("expected inbox promoted to index 0, got %q", out.Folders[0].Slug)
	}
	if out.Folders[0].Unread != 1 {
		t.Errorf("expected inbox unread=1, got %d", out.Folders[0].Unread)
	}
	// Remaining two must be alphabetical: archive, sent.
	if out.Folders[1].Slug != "archive" || out.Folders[2].Slug != "sent" {
		t.Errorf("expected [archive, sent] after inbox, got [%q, %q]", out.Folders[1].Slug, out.Folders[2].Slug)
	}
}

// TestListThreads_InvalidFolderSlug_Bad — traversal-shaped slug
// rejected before any file I/O.
func TestListThreads_InvalidFolderSlug_Bad(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := &Service{core: core.New()}
	r := svc.ListThreads(ListThreadsInput{FolderSlug: "../etc"})
	if r.OK {
		t.Fatal("expected ListThreads to reject an invalid folder slug")
	}
	if !core.Contains(r.Error(), "invalid folderSlug") {
		t.Errorf("unexpected error: %s", r.Error())
	}
}

// TestListThreads_EmptyFolder_Good — folder dir exists but no
// threads.md yet → empty result, not an error.
func TestListThreads_EmptyFolder_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := &Service{core: core.New()}
	if r := folderDir("inbox"); !r.OK {
		t.Fatalf("folderDir: %s", r.Error())
	}
	r := svc.ListThreads(ListThreadsInput{FolderSlug: "inbox"})
	if !r.OK {
		t.Fatalf("ListThreads: %s", r.Error())
	}
	out := r.Value.(ListThreadsOutput)
	if out.Total != 0 || out.Unread != 0 || len(out.Threads) != 0 {
		t.Errorf("expected empty result, got %+v", out)
	}
}

// TestListThreads_SortedNewestFirstAndLimited_Good — three records,
// unsorted on disk, come back newest-first and capped at Limit.
func TestListThreads_SortedNewestFirstAndLimited_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := &Service{core: core.New()}

	threadsR := threadsFilePath("inbox")
	if !threadsR.OK {
		t.Fatalf("threadsFilePath: %s", threadsR.Error())
	}
	body := "" +
		"---\nid: \"old\"\nfrom: \"A\"\nsubject: \"old\"\nlast_touched: \"2024-01-01T00:00:00Z\"\nunread: false\n---\n" +
		"---\nid: \"newest\"\nfrom: \"B\"\nsubject: \"newest\"\nlast_touched: \"2024-03-01T00:00:00Z\"\nunread: true\n---\n" +
		"---\nid: \"mid\"\nfrom: \"C\"\nsubject: \"mid\"\nlast_touched: \"2024-02-01T00:00:00Z\"\nunread: true\n---\n"
	if r := core.WriteFile(threadsR.Value.(string), []byte(body), 0o600); !r.OK {
		t.Fatalf("write threads.md: %s", r.Error())
	}

	r := svc.ListThreads(ListThreadsInput{FolderSlug: "inbox", Limit: 2})
	if !r.OK {
		t.Fatalf("ListThreads: %s", r.Error())
	}
	out := r.Value.(ListThreadsOutput)
	if out.Total != 3 {
		t.Errorf("Total = %d, want 3 (limit only caps the returned slice)", out.Total)
	}
	if out.Unread != 2 {
		t.Errorf("Unread = %d, want 2", out.Unread)
	}
	if len(out.Threads) != 2 {
		t.Fatalf("expected 2 threads after limit, got %d", len(out.Threads))
	}
	if out.Threads[0].ID != "newest" || out.Threads[1].ID != "mid" {
		t.Errorf("expected [newest, mid] order, got [%q, %q]", out.Threads[0].ID, out.Threads[1].ID)
	}
}

// TestListThreads_DefaultLimit_Good — Limit<=0 defaults to 50.
func TestListThreads_DefaultLimit_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := &Service{core: core.New()}
	threadsR := threadsFilePath("inbox")
	if !threadsR.OK {
		t.Fatalf("threadsFilePath: %s", threadsR.Error())
	}
	if r := core.WriteFile(threadsR.Value.(string),
		[]byte("---\nid: \"1\"\nfrom: \"A\"\nsubject: \"hi\"\nunread: false\n---\n"), 0o600); !r.OK {
		t.Fatalf("write threads.md: %s", r.Error())
	}
	r := svc.ListThreads(ListThreadsInput{FolderSlug: "inbox"})
	if !r.OK {
		t.Fatalf("ListThreads: %s", r.Error())
	}
	out := r.Value.(ListThreadsOutput)
	if len(out.Threads) != 1 {
		t.Errorf("expected 1 thread with default limit, got %d", len(out.Threads))
	}
}

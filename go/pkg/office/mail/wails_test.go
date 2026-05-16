// SPDX-Licence-Identifier: EUPL-1.2

package mail

import (
	"testing"
)

// TestServiceName — ServiceName returns the correct Wails binding namespace.
func TestServiceName(t *testing.T) {
	svc := &Service{}
	if svc.ServiceName() != "Mail" {
		t.Fatalf("expected ServiceName='Mail', got %q", svc.ServiceName())
	}
}

// TestMailFolder_Fields — MailFolder carries all fields the frontend expects.
func TestMailFolder_Fields(t *testing.T) {
	f := MailFolder{Label: "Inbox", Slug: "inbox", Unread: 5, Active: true}
	if f.Label == "" || f.Slug == "" {
		t.Fatal("MailFolder missing required fields")
	}
}

// TestMailThread_Fields — MailThread carries all fields the frontend expects.
func TestMailThread_Fields(t *testing.T) {
	th := MailThread{
		ID: "abc", From: "Ada", Subj: "Hi", When: "now", Unread: true, Body: "snippet",
	}
	if th.ID == "" || th.From == "" || th.Subj == "" || th.When == "" || th.Body == "" {
		t.Fatal("MailThread missing required fields")
	}
}

// TestListFoldersOutput_Fields — ListFoldersOutput carries folders slice.
func TestListFoldersOutput_Fields(t *testing.T) {
	out := ListFoldersOutput{Folders: []MailFolder{{Label: "Inbox", Slug: "inbox"}}}
	if len(out.Folders) == 0 {
		t.Fatal("ListFoldersOutput.Folders must not be empty")
	}
}

// TestListThreadsInput_Fields — ListThreadsInput carries FolderSlug + Limit.
func TestListThreadsInput_Fields(t *testing.T) {
	in := ListThreadsInput{FolderSlug: "inbox", Limit: 20}
	if in.FolderSlug == "" {
		t.Fatal("ListThreadsInput.FolderSlug must not be empty")
	}
}

// TestListThreadsOutput_Fields — ListThreadsOutput carries threads + counts.
func TestListThreadsOutput_Fields(t *testing.T) {
	out := ListThreadsOutput{
		Threads: []MailThread{{ID: "1", From: "Alice", Subj: "Hi", When: "now", Body: "body"}},
		Total:   1,
		Unread:  1,
	}
	if len(out.Threads) == 0 {
		t.Fatal("ListThreadsOutput.Threads must not be empty")
	}
	if out.Total != 1 {
		t.Fatalf("ListThreadsOutput.Total expected 1, got %d", out.Total)
	}
}

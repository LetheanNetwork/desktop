// SPDX-Licence-Identifier: EUPL-1.2

package mail

import (
	"testing"

	core "dappco.re/go"
)

// TestSend_AttachmentPathTraversal_Bad — path outside ~/Lethean/ → rejected
// BEFORE SMTP connection (HIGH-mail-3, §3.1).
func TestSend_AttachmentPathTraversal_Bad(t *testing.T) {
	svc, _ := newTestMailService(t)

	r := svc.Send(SendInput{
		AccountName: "personal",
		To:          []string{"ada@lthn.io"},
		Subject:     "Test",
		BodyText:    "Hello.",
		Attachments: []SendAttachment{
			{Filename: "passwd", Path: "/etc/passwd"},
		},
	})
	if r.OK {
		t.Fatal("expected Send to reject attachment outside ~/Lethean/")
	}
	if !core.Contains(r.Error(), "mail.send.invalid_attachment_path") {
		t.Errorf("expected invalid_attachment_path error, got: %s", r.Error())
	}
}

// TestSend_AttachmentPathNotAbsolute_Bad — relative path → rejected.
func TestSend_AttachmentPathNotAbsolute_Bad(t *testing.T) {
	svc, _ := newTestMailService(t)

	r := svc.Send(SendInput{
		AccountName: "personal",
		To:          []string{"ada@lthn.io"},
		Subject:     "Test",
		BodyText:    "Hello.",
		Attachments: []SendAttachment{
			{Filename: "doc.pdf", Path: "relative/path/doc.pdf"},
		},
	})
	if r.OK {
		t.Fatal("expected Send to reject relative attachment path")
	}
	if !core.Contains(r.Error(), "mail.send.invalid_attachment_path") {
		t.Errorf("expected invalid_attachment_path error, got: %s", r.Error())
	}
}

// TestSend_LockedSession_Bad — Send returns mail.session.locked when paused (§6).
func TestSend_LockedSession_Bad(t *testing.T) {
	c := core.New()
	svc := NewService(c)
	ap := newTestAccountProvider(t)
	svc.SetAccountService(ap)
	svc.paused.Store(true)

	r := svc.Send(SendInput{
		AccountName: "personal",
		To:          []string{"ada@lthn.io"},
		Subject:     "Test",
		BodyText:    "Hello.",
	})
	if r.OK {
		t.Fatal("expected Send to fail when locked")
	}
	if !core.Contains(r.Error(), "mail.session.locked") && !core.Contains(r.Error(), "sign in") {
		t.Errorf("expected session.locked, got: %s", r.Error())
	}
}

// TestCollectRecipients_Ugly — To+Cc+Bcc all land in the flat slice.
func TestCollectRecipients_Ugly(t *testing.T) {
	input := SendInput{
		To:  []string{"alice@lthn.io", "bob@lthn.io"},
		Cc:  []string{"carol@lthn.io"},
		Bcc: []string{"dave@lthn.io"},
	}
	got := collectRecipients(input)
	if len(got) != 4 {
		t.Fatalf("expected 4 recipients, got %d: %v", len(got), got)
	}
	// Order: To, Cc, Bcc.
	if got[0] != "alice@lthn.io" || got[1] != "bob@lthn.io" || got[2] != "carol@lthn.io" || got[3] != "dave@lthn.io" {
		t.Errorf("unexpected order: %v", got)
	}
}

// TestExtensionOf_Good — extract file extension from filename.
func TestExtensionOf_Good(t *testing.T) {
	cases := map[string]string{
		"invoice.pdf":   ".pdf",
		"photo.jpg":     ".jpg",
		"archive.tar.gz": ".gz",
		"noextension":   "",
		"":              "",
	}
	for input, want := range cases {
		got := extensionOf(input)
		if got != want {
			t.Errorf("extensionOf(%q) = %q, want %q", input, got, want)
		}
	}
}

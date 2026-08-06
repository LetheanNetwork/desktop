// SPDX-Licence-Identifier: EUPL-1.2

// smtp_extra_test.go — direct coverage of buildMIMEMessage's branch
// matrix (plain-only / HTML+plain / attachments / Cc / In-Reply-To /
// unresolvable attachment / unknown-extension fallback), Send's
// early-validation branches not yet covered by smtp_test.go, and a
// real-listener SMTP connection-failure fault injection for both the
// TLSStarttls and implicit-TLS branches.

package mail

import (
	"net"
	"os"
	"path/filepath"
	"testing"

	core "dappco.re/go"
)

func testMailAccount() *MailAccount {
	return &MailAccount{
		Name: "personal",
		SMTP: SMTPConfig{Host: "smtp.example.test", Port: 587, User: "me@example.test"},
		Auth: AuthSpec{Kind: "appPassword", Secret: "s3cr3t"},
	}
}

// TestBuildMIMEMessage_PlainOnly_Good — no HTML, no attachments takes
// the CreateSingleInline branch.
func TestBuildMIMEMessage_PlainOnly_Good(t *testing.T) {
	acct := testMailAccount()
	msg, err := buildMIMEMessage(SendInput{
		AccountName: "personal",
		To:          []string{"ada@example.test"},
		Subject:     "Hello",
		BodyText:    "Plain body text.",
	}, acct)
	if err != nil {
		t.Fatalf("buildMIMEMessage: %v", err)
	}
	s := string(msg)
	if !core.Contains(s, "Plain body text.") {
		t.Errorf("expected body text in output, got:\n%s", s)
	}
	if !core.Contains(s, "Subject: Hello") {
		t.Errorf("expected Subject header, got:\n%s", s)
	}
	if core.Contains(s, "multipart/alternative") {
		t.Errorf("plain-only message should NOT be multipart/alternative:\n%s", s)
	}
}

// TestBuildMIMEMessage_HTMLAndCc_Good — HTML body + Cc header takes
// the inline-multipart branch with two parts.
func TestBuildMIMEMessage_HTMLAndCc_Good(t *testing.T) {
	acct := testMailAccount()
	msg, err := buildMIMEMessage(SendInput{
		AccountName: "personal",
		To:          []string{"ada@example.test"},
		Cc:          []string{"bob@example.test"},
		Subject:     "Hello HTML",
		BodyText:    "plain fallback",
		BodyHTML:    "<p>hello</p>",
		InReplyTo:   "prior-message-id@example.test",
	}, acct)
	if err != nil {
		t.Fatalf("buildMIMEMessage: %v", err)
	}
	s := string(msg)
	if !core.Contains(s, "plain fallback") {
		t.Errorf("expected plain part, got:\n%s", s)
	}
	if !core.Contains(s, "<p>hello</p>") {
		t.Errorf("expected html part, got:\n%s", s)
	}
	if !core.Contains(s, "Cc: <bob@example.test>") && !core.Contains(s, "Cc: bob@example.test") {
		t.Errorf("expected Cc header, got:\n%s", s)
	}
	if !core.Contains(s, "In-Reply-To:") {
		t.Errorf("expected In-Reply-To header, got:\n%s", s)
	}
}

// TestBuildMIMEMessage_Attachment_Good — a real temp file attaches
// with its MIME type resolved from the extension.
func TestBuildMIMEMessage_Attachment_Good(t *testing.T) {
	acct := testMailAccount()
	dir := t.TempDir()
	path := filepath.Join(dir, "note.txt")
	if err := os.WriteFile(path, []byte("attachment contents"), 0o600); err != nil {
		t.Fatalf("write temp attachment: %v", err)
	}

	msg, err := buildMIMEMessage(SendInput{
		AccountName: "personal",
		To:          []string{"ada@example.test"},
		Subject:     "With attachment",
		BodyText:    "see attached",
		Attachments: []SendAttachment{
			{Filename: "note.txt", Path: path},
		},
	}, acct)
	if err != nil {
		t.Fatalf("buildMIMEMessage: %v", err)
	}
	s := string(msg)
	// Attachment bytes ride base64-encoded (Content-Transfer-Encoding:
	// base64), so assert on the encoded form rather than the raw text.
	if !core.Contains(s, "YXR0YWNobWVudCBjb250ZW50cw==") {
		t.Errorf("expected base64-encoded attachment bytes in output, got:\n%s", s)
	}
	if !core.Contains(s, "filename=note.txt") {
		t.Errorf("expected attachment filename header, got:\n%s", s)
	}
}

// TestBuildMIMEMessage_AttachmentExplicitMIME_Good — an explicit
// att.MIME bypasses the extension-sniff branch entirely.
func TestBuildMIMEMessage_AttachmentExplicitMIME_Good(t *testing.T) {
	acct := testMailAccount()
	dir := t.TempDir()
	path := filepath.Join(dir, "blob")
	if err := os.WriteFile(path, []byte{0x01, 0x02, 0x03}, 0o600); err != nil {
		t.Fatalf("write temp attachment: %v", err)
	}

	msg, err := buildMIMEMessage(SendInput{
		AccountName: "personal",
		To:          []string{"ada@example.test"},
		Subject:     "explicit mime",
		BodyText:    "body",
		Attachments: []SendAttachment{
			{Filename: "blob", Path: path, MIME: "application/x-custom"},
		},
	}, acct)
	if err != nil {
		t.Fatalf("buildMIMEMessage: %v", err)
	}
	if !core.Contains(string(msg), "application/x-custom") {
		t.Errorf("expected explicit MIME type honoured, got:\n%s", string(msg))
	}
}

// TestBuildMIMEMessage_AttachmentUnknownExtension_Ugly — an
// extension mime.TypeByExtension doesn't know falls back to
// application/octet-stream.
func TestBuildMIMEMessage_AttachmentUnknownExtension_Ugly(t *testing.T) {
	acct := testMailAccount()
	dir := t.TempDir()
	path := filepath.Join(dir, "mystery.zzzzz")
	if err := os.WriteFile(path, []byte("bytes"), 0o600); err != nil {
		t.Fatalf("write temp attachment: %v", err)
	}

	msg, err := buildMIMEMessage(SendInput{
		AccountName: "personal",
		To:          []string{"ada@example.test"},
		Subject:     "unknown ext",
		BodyText:    "body",
		Attachments: []SendAttachment{
			{Filename: "mystery.zzzzz", Path: path},
		},
	}, acct)
	if err != nil {
		t.Fatalf("buildMIMEMessage: %v", err)
	}
	if !core.Contains(string(msg), "application/octet-stream") {
		t.Errorf("expected octet-stream fallback, got:\n%s", string(msg))
	}
}

// TestBuildMIMEMessage_AttachmentReadFails_Bad — a nonexistent
// attachment path surfaces a wrapped error instead of panicking.
func TestBuildMIMEMessage_AttachmentReadFails_Bad(t *testing.T) {
	acct := testMailAccount()
	_, err := buildMIMEMessage(SendInput{
		AccountName: "personal",
		To:          []string{"ada@example.test"},
		Subject:     "missing attachment",
		BodyText:    "body",
		Attachments: []SendAttachment{
			{Filename: "ghost.pdf", Path: "/nonexistent/path/ghost.pdf"},
		},
	}, acct)
	if err == nil {
		t.Fatal("expected an error when the attachment can't be read")
	}
}

// --- Send() early-validation branches ---

// TestSend_AccountNameRequired_Bad — empty AccountName rejected
// before any I/O.
func TestSend_AccountNameRequired_Bad(t *testing.T) {
	svc, _ := newTestMailService(t)
	r := svc.Send(SendInput{To: []string{"ada@example.test"}, Subject: "x", BodyText: "y"})
	if r.OK {
		t.Fatal("expected Send to reject an empty account name")
	}
	if !core.Contains(r.Error(), "mail.send.account_required") {
		t.Errorf("unexpected error: %s", r.Error())
	}
}

// TestSend_ToRequired_Bad — empty recipient list rejected before any I/O.
func TestSend_ToRequired_Bad(t *testing.T) {
	svc, _ := newTestMailService(t)
	r := svc.Send(SendInput{AccountName: "personal", Subject: "x", BodyText: "y"})
	if r.OK {
		t.Fatal("expected Send to reject an empty recipient list")
	}
	if !core.Contains(r.Error(), "mail.send.to_required") {
		t.Errorf("unexpected error: %s", r.Error())
	}
}

// TestSend_AccountProviderNotWired_Bad — s.account is nil (never
// called SetAccountService).
func TestSend_AccountProviderNotWired_Bad(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := NewService(core.New())
	r := svc.Send(SendInput{AccountName: "personal", To: []string{"ada@example.test"}, Subject: "x", BodyText: "y"})
	if r.OK {
		t.Fatal("expected Send to fail when the account provider isn't wired")
	}
	if !core.Contains(r.Error(), "account provider not wired") {
		t.Errorf("unexpected error: %s", r.Error())
	}
}

// TestSend_AccountNotFound_Bad — unlocked session, but no saved
// account under that name.
func TestSend_AccountNotFound_Bad(t *testing.T) {
	svc, _ := newTestMailService(t)
	r := svc.Send(SendInput{AccountName: "ghost", To: []string{"ada@example.test"}, Subject: "x", BodyText: "y"})
	if r.OK {
		t.Fatal("expected Send to fail for an unknown account name")
	}
	if !core.Contains(r.Error(), "mail.send.account_not_found") {
		t.Errorf("unexpected error: %s", r.Error())
	}
}

// dropListener accepts and immediately closes every connection — a
// real TCP endpoint (127.0.0.1:0, never a fixed port) that always
// fails the SMTP handshake, exercising Send's connection-failure
// branch for both the STARTTLS and implicit-TLS code paths without
// implementing a real SMTP or TLS server.
func dropListener(t *testing.T) (host string, port int) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			_ = conn.Close()
		}
	}()
	h, p := splitFakeAddr(t, ln.Addr().String())
	return h, p
}

// TestSend_SMTPConnectFailure_StartTLS_Bad — TLSStarttls=true takes
// the gosmtp.SendMail (plaintext-dial-then-STARTTLS) branch; the
// server dropping the connection before any greeting surfaces as a
// wrapped "SMTP send" failure.
func TestSend_SMTPConnectFailure_StartTLS_Bad(t *testing.T) {
	svc, _ := newTestMailService(t)
	host, port := dropListener(t)

	if r := svc.SaveAccount(AccountInput{
		Name: "personal",
		IMAP: IMAPConfig{Host: "imap.example.test", Port: 993, TLS: true},
		SMTP: SMTPConfig{Host: host, Port: port, User: "me@example.test", TLSStarttls: true},
		Auth: AuthSpec{Kind: "appPassword", Secret: "s3cr3t"},
	}); !r.OK {
		t.Fatalf("SaveAccount: %s", r.Error())
	}

	r := svc.Send(SendInput{
		AccountName: "personal",
		To:          []string{"ada@example.test"},
		Subject:     "x",
		BodyText:    "y",
	})
	if r.OK {
		t.Fatal("expected Send to fail against a listener that drops the connection")
	}
	if !core.Contains(r.Error(), "SMTP send") {
		t.Errorf("unexpected error: %s", r.Error())
	}
}

// TestSend_SMTPConnectFailure_ImplicitTLS_Bad — TLSStarttls=false
// takes the gosmtp.SendMailTLS (implicit-TLS) branch; dialing a
// plaintext listener with a TLS client fails the handshake.
func TestSend_SMTPConnectFailure_ImplicitTLS_Bad(t *testing.T) {
	svc, _ := newTestMailService(t)
	host, port := dropListener(t)

	if r := svc.SaveAccount(AccountInput{
		Name: "personal",
		IMAP: IMAPConfig{Host: "imap.example.test", Port: 993, TLS: true},
		SMTP: SMTPConfig{Host: host, Port: port, User: "me@example.test", TLSStarttls: false},
		Auth: AuthSpec{Kind: "appPassword", Secret: "s3cr3t"},
	}); !r.OK {
		t.Fatalf("SaveAccount: %s", r.Error())
	}

	r := svc.Send(SendInput{
		AccountName: "personal",
		To:          []string{"ada@example.test"},
		Subject:     "x",
		BodyText:    "y",
	})
	if r.OK {
		t.Fatal("expected Send to fail the TLS handshake against a plaintext listener")
	}
	if !core.Contains(r.Error(), "SMTP send") {
		t.Errorf("unexpected error: %s", r.Error())
	}
}

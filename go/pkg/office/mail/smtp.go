// SPDX-Licence-Identifier: EUPL-1.2

// smtp.go — SMTP send for pkg/office/mail.
// Implements Send (§3) with attachment path validation (§3.1 HIGH-mail-3),
// MIME message construction, and EventSent on success.
//
// Attachment path validation uses paths.IsValidAbsoluteUnderRoot restricted
// to ~/Lethean/ — no arbitrary filesystem access (§3.1 tradeoff named).
// Q5: NEVER upload attachment bytes to any third-party scanner (privacy invariant).

package mail

import (
	"bytes"
	"io"
	"mime"

	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/paths"
	gomail "github.com/emersion/go-message/mail"
	gosmtp "github.com/emersion/go-smtp"
	"github.com/emersion/go-sasl"
)

// Send builds a MIME message and sends it via SMTP.
// Validates attachment paths (§3.1) BEFORE any SMTP connection (HIGH-mail-3).
// Fires EventSent on success. Does NOT write to local Sent folder —
// the IMAP poll on the Sent folder provides single-source-of-truth (§3 tradeoff).
//
// Usage example:
//
//	r := svc.Send(mail.SendInput{AccountName: "personal", To: []string{"ada@lthn.io"}, Subject: "Hi", BodyText: "Hello!"})
//	if !r.OK { /* handle */ }
func (s *Service) Send(input SendInput) core.Result {
	if r := s.assertUnlocked(); !r.OK {
		return r
	}
	if input.AccountName == "" {
		return core.Fail(core.NewCode("mail.send.account_required", "account_name is required"))
	}
	if len(input.To) == 0 {
		return core.Fail(core.NewCode("mail.send.to_required", "at least one recipient is required"))
	}

	// §3.1 — validate attachment paths BEFORE opening SMTP connection (HIGH-mail-3).
	rootR := paths.Root()
	if !rootR.OK {
		return core.Fail(core.E("mail.Send", "resolve Lethean root", nil))
	}
	letheanRoot := rootR.Value.(string)
	for _, att := range input.Attachments {
		if err := paths.IsValidAbsoluteUnderRoot(letheanRoot, att.Path); err != nil {
			return core.Fail(core.NewCode("mail.send.invalid_attachment_path",
				core.Sprintf("attachment %q path rejected: %s", att.Filename, err.Error())))
		}
		// No NUL byte (belt-and-braces after IsValidAbsoluteUnderRoot).
		for i := 0; i < len(att.Path); i++ {
			if att.Path[i] == 0 {
				return core.Fail(core.NewCode("mail.send.invalid_attachment_path",
					core.Sprintf("attachment path contains NUL byte")))
			}
		}
	}

	if s.account == nil {
		return core.Fail(core.E("mail.Send", "account provider not wired", nil))
	}
	// Mantis #1591 Option D — assert exactly one unlocked account.
	accountID, idErr := s.singleUnlockedAccount()
	if idErr != nil {
		return core.Fail(idErr)
	}
	handle, ok := s.account.PrivateKeyFor(accountID)
	if !ok {
		return s.errLocked()
	}

	// Mantis #1589 / Cerberus #18 — handle zeroises bytes on release.
	var (
		accounts []MailAccount
		loadErr  error
	)
	_ = handle.Use(func(priv []byte) error {
		accounts, loadErr = s.loadAccounts(priv)
		return loadErr
	})
	if loadErr != nil {
		return core.Fail(core.E("mail.Send", "load accounts", loadErr))
	}
	var acct *MailAccount
	for i := range accounts {
		if accounts[i].Name == input.AccountName {
			acct = &accounts[i]
			break
		}
	}
	if acct == nil {
		return core.Fail(core.NewCode("mail.send.account_not_found",
			core.Sprintf("account %q not found", input.AccountName)))
	}

	// Build MIME message.
	msgBytes, err := buildMIMEMessage(input, acct)
	if err != nil {
		return core.Fail(core.E("mail.Send", "build MIME message", err))
	}

	// SMTP send.
	addr := core.Sprintf("%s:%d", acct.SMTP.Host, acct.SMTP.Port)
	auth := sasl.NewPlainClient("", acct.SMTP.User, acct.Auth.Secret)
	recipients := collectRecipients(input)
	from := acct.SMTP.User

	var sendErr error
	if acct.SMTP.TLSStarttls {
		sendErr = gosmtp.SendMail(addr, auth, from, recipients, bytes.NewReader(msgBytes))
	} else {
		sendErr = gosmtp.SendMailTLS(addr, auth, from, recipients, bytes.NewReader(msgBytes))
	}
	if sendErr != nil {
		return core.Fail(core.E("mail.Send", "SMTP send", sendErr))
	}

	s.core.ACTION(MailEvent{
		Kind:        EventSent,
		AccountName: input.AccountName,
		At:          core.Now(),
	})
	return core.Ok(nil)
}

// buildMIMEMessage composes a multipart MIME message from SendInput.
func buildMIMEMessage(input SendInput, acct *MailAccount) ([]byte, error) {
	var buf bytes.Buffer

	// Build mail.Header.
	var h gomail.Header
	h.SetDate(core.Now())
	h.SetSubject(input.Subject)
	h.SetAddressList("From", []*gomail.Address{{Address: acct.SMTP.User}})

	var to []*gomail.Address
	for _, addr := range input.To {
		to = append(to, &gomail.Address{Address: addr})
	}
	h.SetAddressList("To", to)

	if len(input.Cc) > 0 {
		var cc []*gomail.Address
		for _, addr := range input.Cc {
			cc = append(cc, &gomail.Address{Address: addr})
		}
		h.SetAddressList("Cc", cc)
	}

	if input.InReplyTo != "" {
		h.SetMsgIDList("In-Reply-To", []string{input.InReplyTo})
	}

	hasAttachments := len(input.Attachments) > 0
	hasHTML := input.BodyHTML != ""

	mw, err := gomail.CreateWriter(&buf, h)
	if err != nil {
		return nil, err
	}

	if hasHTML || hasAttachments {
		inline, err := mw.CreateInline()
		if err != nil {
			return nil, err
		}
		// text/plain part.
		var ph gomail.InlineHeader
		ph.Set("Content-Type", "text/plain; charset=utf-8")
		pw, err := inline.CreatePart(ph)
		if err != nil {
			return nil, err
		}
		if _, err := io.WriteString(pw, input.BodyText); err != nil {
			return nil, err
		}
		_ = pw.Close()

		if hasHTML {
			var hh gomail.InlineHeader
			hh.Set("Content-Type", "text/html; charset=utf-8")
			hw, err := inline.CreatePart(hh)
			if err != nil {
				return nil, err
			}
			if _, err := io.WriteString(hw, input.BodyHTML); err != nil {
				return nil, err
			}
			_ = hw.Close()
		}
		_ = inline.Close()
	} else {
		// Plain-text-only message (simple case).
		var ph gomail.InlineHeader
		ph.Set("Content-Type", "text/plain; charset=utf-8")
		pw, err := mw.CreateSingleInline(ph)
		if err != nil {
			return nil, err
		}
		if _, err := io.WriteString(pw, input.BodyText); err != nil {
			return nil, err
		}
		_ = pw.Close()
	}

	// Attachments.
	for _, att := range input.Attachments {
		rawR := core.ReadFile(att.Path)
		if !rawR.OK {
			return nil, core.E("mail.buildMIMEMessage", "read attachment "+att.Filename, nil)
		}
		attBytes, _ := rawR.Value.([]byte)

		mimeType := att.MIME
		if mimeType == "" {
			mimeType = mime.TypeByExtension(extensionOf(att.Filename))
		}
		if mimeType == "" {
			mimeType = "application/octet-stream"
		}

		var ah gomail.AttachmentHeader
		ah.Set("Content-Type", mimeType)
		ah.SetFilename(att.Filename)

		aw, err := mw.CreateAttachment(ah)
		if err != nil {
			return nil, err
		}
		if _, err := aw.Write(attBytes); err != nil {
			return nil, err
		}
		_ = aw.Close()
	}

	_ = mw.Close()
	return buf.Bytes(), nil
}

// collectRecipients aggregates To + Cc + Bcc into one flat slice.
func collectRecipients(input SendInput) []string {
	total := len(input.To) + len(input.Cc) + len(input.Bcc)
	out := make([]string, 0, total)
	out = append(out, input.To...)
	out = append(out, input.Cc...)
	out = append(out, input.Bcc...)
	return out
}

// extensionOf returns the file extension including the dot, e.g. ".pdf".
func extensionOf(filename string) string {
	for i := len(filename) - 1; i >= 0; i-- {
		if filename[i] == '.' {
			return filename[i:]
		}
	}
	return ""
}

// SPDX-Licence-Identifier: EUPL-1.2

package opencode

import (
	core "dappco.re/go"
)

// TestShellQuote_HappyPath_Good — alphanumeric input is wrapped in
// single quotes with no escape needed.
func TestShellQuote_HappyPath_Good(t *core.T) {
	got := shellQuote("abc123")
	want := "'abc123'"
	if got != want {
		t.Errorf("shellQuote(abc123) = %q, want %q", got, want)
	}
}

// TestShellQuote_SingleQuoteEscaped_Good — embedded single quotes
// switch to the '\'' close-escape-open pattern.
func TestShellQuote_SingleQuoteEscaped_Good(t *core.T) {
	got := shellQuote("a'b")
	want := `'a'\''b'`
	if got != want {
		t.Errorf("shellQuote(a'b) = %q, want %q", got, want)
	}
}

// TestShellQuote_MetaCharsLiteral_Good — shell metacharacters inside
// single quotes lose their meaning; they pass through unescaped.
func TestShellQuote_MetaCharsLiteral_Good(t *core.T) {
	in := `$(rm -rf /); echo ` + "`pwd`" + ` && true | grep .`
	got := shellQuote(in)
	want := "'" + in + "'"
	if got != want {
		t.Errorf("shellQuote meta-chars = %q, want %q", got, want)
	}
}

// TestAppleScriptQuote_HappyPath_Good — alphanumeric input is wrapped
// in double quotes with no escape needed.
func TestAppleScriptQuote_HappyPath_Good(t *core.T) {
	got, err := appleScriptQuote("abc123")
	if err != nil {
		t.Fatalf("appleScriptQuote(abc123) unexpected err: %v", err)
	}
	want := `"abc123"`
	if got != want {
		t.Errorf("appleScriptQuote(abc123) = %q, want %q", got, want)
	}
}

// TestAppleScriptQuote_QuoteCharEscaped_Good — `"` becomes `\"`.
func TestAppleScriptQuote_QuoteCharEscaped_Good(t *core.T) {
	got, err := appleScriptQuote(`a"b`)
	if err != nil {
		t.Fatalf("appleScriptQuote unexpected err: %v", err)
	}
	want := `"a\"b"`
	if got != want {
		t.Errorf("appleScriptQuote(a\"b) = %q, want %q", got, want)
	}
}

// TestAppleScriptQuote_BackslashEscaped_Good — `\` becomes `\\`. Order
// matters: backslash must be escaped before quote-escapes are emitted
// (otherwise `\\"` collapses incorrectly).
func TestAppleScriptQuote_BackslashEscaped_Good(t *core.T) {
	got, err := appleScriptQuote(`a\b`)
	if err != nil {
		t.Fatalf("appleScriptQuote unexpected err: %v", err)
	}
	want := `"a\\b"`
	if got != want {
		t.Errorf("appleScriptQuote(a\\b) = %q, want %q", got, want)
	}
}

// TestAppleScriptQuote_BackslashAndQuoteCombined_Good — exercise both
// escapes in one pass so they cannot interact (the backslash escape
// must not consume the following quote into a malformed `\\\"`).
func TestAppleScriptQuote_BackslashAndQuoteCombined_Good(t *core.T) {
	got, err := appleScriptQuote(`\"`)
	if err != nil {
		t.Fatalf("appleScriptQuote unexpected err: %v", err)
	}
	want := `"\\\""`
	if got != want {
		t.Errorf("appleScriptQuote(\\\") = %q, want %q", got, want)
	}
}

// TestAppleScriptQuote_NewlineRejected_Bad — embedded LF must error;
// a bare newline would terminate the osascript -e line.
func TestAppleScriptQuote_NewlineRejected_Bad(t *core.T) {
	_, err := appleScriptQuote("a\nb")
	if err == nil {
		t.Errorf("appleScriptQuote(a\\nb) expected err, got nil")
	}
}

// TestAppleScriptQuote_CarriageReturnRejected_Bad — embedded CR must
// error for the same reason as LF.
func TestAppleScriptQuote_CarriageReturnRejected_Bad(t *core.T) {
	_, err := appleScriptQuote("a\rb")
	if err == nil {
		t.Errorf("appleScriptQuote(a\\rb) expected err, got nil")
	}
}

// TestAppleScriptQuote_NullByteRejected_Ugly — NUL byte cannot be
// represented in an AppleScript string literal; must error.
func TestAppleScriptQuote_NullByteRejected_Ugly(t *core.T) {
	_, err := appleScriptQuote("a\x00b")
	if err == nil {
		t.Errorf("appleScriptQuote(a\\0b) expected err, got nil")
	}
}

// TestCmdArgvQuote_HappyPath_Good — alphanumeric input is wrapped in
// double quotes (defensive — even safe input is quoted so the helper
// is grep-able as the security boundary).
func TestCmdArgvQuote_HappyPath_Good(t *core.T) {
	got, err := cmdArgvQuote("abc123")
	if err != nil {
		t.Fatalf("cmdArgvQuote(abc123) unexpected err: %v", err)
	}
	want := `"abc123"`
	if got != want {
		t.Errorf("cmdArgvQuote(abc123) = %q, want %q", got, want)
	}
}

// TestCmdArgvQuote_SpecialCharsEscaped_Good — `& | < >` inside double
// quotes lose meta meaning; they pass through unescaped. The quoting
// IS the escape for these.
func TestCmdArgvQuote_SpecialCharsEscaped_Good(t *core.T) {
	got, err := cmdArgvQuote(`a&b|c<d>e`)
	if err != nil {
		t.Fatalf("cmdArgvQuote unexpected err: %v", err)
	}
	want := `"a&b|c<d>e"`
	if got != want {
		t.Errorf("cmdArgvQuote = %q, want %q", got, want)
	}
}

// TestCmdArgvQuote_CaretEscaped_Good — `^` is cmd.exe's escape char
// even inside double quotes for some parsing contexts; doubled to `^^`
// so it round-trips as a literal caret.
func TestCmdArgvQuote_CaretEscaped_Good(t *core.T) {
	got, err := cmdArgvQuote(`a^b`)
	if err != nil {
		t.Fatalf("cmdArgvQuote unexpected err: %v", err)
	}
	want := `"a^^b"`
	if got != want {
		t.Errorf("cmdArgvQuote(a^b) = %q, want %q", got, want)
	}
}

// TestCmdArgvQuote_EmbeddedQuoteDoubled_Good — `"` is escaped by
// doubling inside cmd.exe quoted strings.
func TestCmdArgvQuote_EmbeddedQuoteDoubled_Good(t *core.T) {
	got, err := cmdArgvQuote(`a"b`)
	if err != nil {
		t.Fatalf("cmdArgvQuote unexpected err: %v", err)
	}
	want := `"a""b"`
	if got != want {
		t.Errorf("cmdArgvQuote(a\"b) = %q, want %q", got, want)
	}
}

// TestCmdArgvQuote_SpaceQuoted_Good — embedded spaces produce a single
// quoted argv token (the quoting prevents cmd from splitting at the
// space).
func TestCmdArgvQuote_SpaceQuoted_Good(t *core.T) {
	got, err := cmdArgvQuote(`a b c`)
	if err != nil {
		t.Fatalf("cmdArgvQuote unexpected err: %v", err)
	}
	want := `"a b c"`
	if got != want {
		t.Errorf("cmdArgvQuote(a b c) = %q, want %q", got, want)
	}
}

// TestCmdArgvQuote_NewlineRejected_Bad — embedded LF cannot be
// expressed in a single cmd.exe argv; must error.
func TestCmdArgvQuote_NewlineRejected_Bad(t *core.T) {
	_, err := cmdArgvQuote("a\nb")
	if err == nil {
		t.Errorf("cmdArgvQuote(a\\nb) expected err, got nil")
	}
}

// TestCmdArgvQuote_NullByteRejected_Ugly — NUL byte rejected as for
// AppleScript.
func TestCmdArgvQuote_NullByteRejected_Ugly(t *core.T) {
	_, err := cmdArgvQuote("a\x00b")
	if err == nil {
		t.Errorf("cmdArgvQuote(a\\0b) expected err, got nil")
	}
}

// TestOpenTUI_Darwin_PasswordWithSpecialChars_Good — verifies the
// AppleScript layer + shell layer combine correctly when the password
// contains chars that would break either layer. The shellQuote wrap
// puts the password in single quotes (shell-safe); appleScriptQuote
// then wraps the whole thing in double quotes and escapes `\` + `"`
// so the AppleScript literal carries through to the shell verbatim.
//
// We don't drive ps.Run here (that would need a process service stub);
// we drive the two helpers in the same order the production code does
// and assert the final string the AppleScript interpreter would see is
// what we expect.
func TestOpenTUI_Darwin_PasswordWithSpecialChars_Good(t *core.T) {
	password := `a"b\c&d|e f`
	targetURL := "http://127.0.0.1:42424/"
	shellCmd := "OPENCODE_SERVER_PASSWORD=" + shellQuote(password) +
		" opencode attach " + targetURL
	quoted, err := appleScriptQuote(shellCmd)
	if err != nil {
		t.Fatalf("appleScriptQuote unexpected err: %v", err)
	}
	// Backslash must appear as \\, double quote as \" — the helper
	// emits the literal four-byte sequence `\\` for one input `\`.
	// Assert two invariants: (1) starts and ends with `"`, (2) raw
	// password is shell-quoted inside.
	if quoted[0] != '"' || quoted[len(quoted)-1] != '"' {
		t.Errorf("appleScriptQuote envelope wrong: %q", quoted)
	}
	// The shell-quoted password segment must appear verbatim except
	// for AppleScript-escaped chars. The opening `'` after the equals
	// is unchanged (no special meaning in AppleScript). The `\` and
	// `"` inside must be escaped.
	mustContain := `OPENCODE_SERVER_PASSWORD='a\"b\\c&d|e f' opencode attach http://127.0.0.1:42424/`
	if !contains(quoted, mustContain) {
		t.Errorf("appleScript output missing expected escaped form\ngot:  %s\nwant substring: %s", quoted, mustContain)
	}
}

// TestOpenTUI_Windows_PasswordWithSpecialChars_Good — verifies the
// cmd /k argv layer escapes cmd.exe metacharacters. Mirrors the
// production composition: `set OPENCODE_SERVER_PASSWORD=<quoted> && opencode attach <url>`.
func TestOpenTUI_Windows_PasswordWithSpecialChars_Good(t *core.T) {
	password := `a"b^c&d|e f`
	targetURL := "http://127.0.0.1:42424/"
	quotedPw, err := cmdArgvQuote(password)
	if err != nil {
		t.Fatalf("cmdArgvQuote unexpected err: %v", err)
	}
	cmdLine := "set OPENCODE_SERVER_PASSWORD=" + quotedPw +
		" && opencode attach " + targetURL
	// Inside cmd quotes: `"` → `""`, `^` → `^^`, others literal.
	want := `set OPENCODE_SERVER_PASSWORD="a""b^^c&d|e f" && opencode attach http://127.0.0.1:42424/`
	if cmdLine != want {
		t.Errorf("cmd /k argv composition wrong\ngot:  %s\nwant: %s", cmdLine, want)
	}
}

// (substring helper `contains` is shared from wails_provider_test.go)

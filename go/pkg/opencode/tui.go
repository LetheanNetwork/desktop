// SPDX-Licence-Identifier: EUPL-1.2

// OpenTUI — opens the user's host opencode TUI attached to the
// running sandbox via `opencode attach <url>`. Per RFC.opencode.md
// §6, this is the "Open TUI" button on the integrations card.
//
// Why `attach`, not `docker exec`: opencode 1.14+ ships an `attach`
// subcommand that connects a host-side TUI to any reachable backend
// (serve/web) over HTTP. The user's host opencode brings their own
// theme, keybinds, auth profile, and history — strictly better UX
// than shelling into the container. The container is the BACKEND
// only; the TUI runs on the host.
//
// The container's bound `127.0.0.1:<host-port>` is the target URL.
// Auth is the per-install OPENCODE_SERVER_PASSWORD, passed via env
// to the spawned shell so it never lands on the command line / in
// `ps` output / in shell history.
//
// Platform branching:
//
//   - darwin → AppleScript via `osascript -e 'tell app "Terminal"
//     to do script "<cmd>"'`. Opens Terminal.app, fronts it, runs.
//   - linux → $TERMINAL env or x-terminal-emulator (Debian-ish) or
//     a per-DE fallback (gnome-terminal / konsole / xterm).
//   - windows → `wt.exe new-tab cmd /k "<cmd>"` if Windows Terminal
//     is installed; otherwise `cmd /c start cmd /k "<cmd>"`.
//
// The spawn is fire-and-forget — the host terminal app keeps running
// independently of the lthn binary. Returns Ok as soon as the launch
// command exits (Terminal.app keeps running after osascript returns).

package opencode

import (
	goruntime "runtime"

	core "dappco.re/go"
)

// OpenTUI launches `<runtime> exec -it <container> opencode` inside
// the user's default terminal for the named sandbox. Returns Fail
// when the sandbox isn't running or the platform path isn't
// supported.
//
// Usage example:
//
//	r := svc.OpenTUI("oc-1735843891234")
//	if !r.OK { core.Println("open-tui failed:", r.Error()) }
func (s *Service) OpenTUI(id string) core.Result {
	if s == nil {
		return core.Fail(core.E("opencode.OpenTUI", "service is nil", nil))
	}
	if core.Trim(id) == "" {
		return core.Fail(core.E("opencode.OpenTUI", "id is required", nil))
	}
	// Confirm sandbox is running — attaching to a stopped backend
	// produces a confusing connection-refused error inside the
	// user's new terminal window.
	infoR := s.Inspect(id)
	if !infoR.OK {
		return infoR
	}
	sb, _ := infoR.Value.(Sandbox)
	if sb.Status != StatusRunning {
		return core.Fail(core.E("opencode.OpenTUI",
			"sandbox is not running (status="+sb.Status+")", nil))
	}
	pwR := s.ServerPassword()
	if !pwR.OK {
		return pwR
	}
	password, _ := pwR.Value.(string)

	ps := s.proc()
	if ps == nil {
		return core.Fail(core.E("opencode.OpenTUI", "process service unavailable", nil))
	}

	// `opencode attach <url>` connects a host-side TUI to the
	// container's backend. Password rides on env so it doesn't
	// land in ps output or shell history; the upstream's --password
	// flag defaults to $OPENCODE_SERVER_PASSWORD when set.
	targetURL := core.Sprintf("http://127.0.0.1:%d/", sb.HostPort)

	ctx, cancel := core.WithTimeout(core.Background(), 10*core.Second)
	defer cancel()

	switch goruntime.GOOS {
	case "darwin":
		// AppleScript `do script` runs the string in a fresh
		// Terminal shell, so POSIX env-prefix parses correctly:
		// `VAR=val cmd args...`. Password is currently hex-only
		// but defence-in-depth: shell-quote first (single-quote
		// the value so the shell parses it as one literal VAR=val
		// token), then AppleScript-quote the whole command so the
		// AppleScript string literal does not lose meta-chars to
		// its own escape grammar.
		// SECURITY: password passed through shellQuote + the whole
		// shellCmd passed through appleScriptQuote; do NOT add
		// raw % formatting or string concat for untrusted input
		// here (Mantis #1601).
		shellCmd := "OPENCODE_SERVER_PASSWORD=" + shellQuote(password) +
			" opencode attach " + targetURL
		quotedScript, qErr := appleScriptQuote(shellCmd)
		if qErr != nil {
			return core.Fail(core.E("opencode.OpenTUI",
				"shell command contains characters unsafe for AppleScript", qErr))
		}
		script := `tell application "Terminal" to do script ` + quotedScript
		runR := ps.Run(ctx, "osascript", "-e", script)
		if !runR.OK {
			return runR
		}
		// Bring Terminal to the foreground so the user sees the
		// new window — osascript above runs the command but doesn't
		// always raise the window when Terminal is already open.
		_ = ps.Run(ctx, "osascript", "-e", `tell application "Terminal" to activate`)
		return core.Ok(nil)

	case "linux":
		// Wrap in `sh -c` so env-prefix parses across emulators
		// (xterm -e exec's argv directly; gnome-terminal -e parses
		// shell). The `sh -c '...'` shape is the lowest common
		// denominator. $TERMINAL takes priority for users who've
		// configured a preferred emulator.
		shellCmd := "OPENCODE_SERVER_PASSWORD=" + password +
			" opencode attach " + targetURL
		wrapped := "sh -c " + shellQuote(shellCmd)
		candidates := []string{
			core.Getenv("TERMINAL"),
			"x-terminal-emulator",
			"gnome-terminal",
			"konsole",
			"xterm",
		}
		for _, term := range candidates {
			if core.Trim(term) == "" {
				continue
			}
			runR := ps.Run(ctx, term, "-e", wrapped)
			if runR.OK {
				return core.Ok(nil)
			}
		}
		return core.Fail(core.E("opencode.OpenTUI",
			"no terminal emulator found (set $TERMINAL)", nil))

	case "windows":
		// cmd.exe needs `set VAR=val && cmd` rather than the POSIX
		// `VAR=val cmd` env-prefix. Windows Terminal first; falls
		// back to plain cmd.exe.
		// SECURITY: password passed through cmdArgvQuote; do NOT
		// add raw % formatting or string concat for untrusted
		// input here (Mantis #1601). Without quoting, a password
		// containing & | < > ^ " %% would break out of the `set`
		// statement into a chained command.
		quotedPw, qErr := cmdArgvQuote(password)
		if qErr != nil {
			return core.Fail(core.E("opencode.OpenTUI",
				"password contains characters unsafe for cmd.exe", qErr))
		}
		cmdLine := "set OPENCODE_SERVER_PASSWORD=" + quotedPw +
			" && opencode attach " + targetURL
		runR := ps.Run(ctx, "wt.exe", "new-tab", "cmd", "/k", cmdLine)
		if runR.OK {
			return core.Ok(nil)
		}
		runR = ps.Run(ctx, "cmd", "/c", "start", "cmd", "/k", cmdLine)
		if runR.OK {
			return core.Ok(nil)
		}
		return runR

	default:
		return core.Fail(core.E("opencode.OpenTUI",
			"unsupported platform: "+goruntime.GOOS, nil))
	}
}

// shellQuote single-quotes a string for safe inclusion in `sh -c`.
// Hex passwords don't need it but the helper protects against
// future callers that build commands with metacharacters.
//
// Usage example:
//
//	wrapped := "sh -c " + shellQuote(`echo "hello world"`)
//	// → sh -c 'echo "hello world"'
func shellQuote(s string) string {
	// Single-quote everything, escape any embedded single quote as
	// '\''. Cheap; runs once per OpenTUI invocation.
	var b []byte
	b = append(b, '\'')
	for i := 0; i < len(s); i++ {
		if s[i] == '\'' {
			b = append(b, '\'', '\\', '\'', '\'')
			continue
		}
		b = append(b, s[i])
	}
	b = append(b, '\'')
	return string(b)
}

// appleScriptQuote wraps a string in a double-quoted AppleScript
// string literal, escaping the two characters AppleScript's
// double-quoted string grammar treats as meta: backslash (\\) and
// double-quote (\"). Per Apple's AppleScript Language Guide
// (Lexical Conventions §"String Literals"), `\n`, `\r`, `\t` are
// the only legal escape sequences for control characters; raw
// embedded control bytes are rejected by the parser. We treat
// embedded NUL, LF, and CR as unsafe (they would terminate the
// osascript line or be silently dropped) and return an error so
// the caller can fail the launch rather than ship a corrupted
// command to the user's terminal.
//
// Returns the quoted form ready for splicing into an osascript
// argument (the returned value INCLUDES the surrounding double
// quotes).
//
// Usage example:
//
//	q, err := appleScriptQuote(`hello "world"`)
//	// → "hello \"world\"", nil
//	q, err := appleScriptQuote("bad\nstring")
//	// → "", err (control character rejected)
func appleScriptQuote(s string) (string, error) {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == 0x00 || c == '\n' || c == '\r' {
			return "", core.E("opencode.appleScriptQuote",
				"embedded control character is not safe for AppleScript literal", nil)
		}
	}
	var b []byte
	b = append(b, '"')
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case '\\':
			b = append(b, '\\', '\\')
		case '"':
			b = append(b, '\\', '"')
		default:
			b = append(b, c)
		}
	}
	b = append(b, '"')
	return string(b), nil
}

// cmdArgvQuote wraps a string as a single quoted argv token for
// cmd.exe. The cmd.exe parser treats `&`, `|`, `<`, `>`, `^`, `"`,
// `%`, `!` as special: unquoted, any of these can break out of the
// current command into a chained one or trigger variable expansion.
// Inside double quotes `&|<>` lose their meta meaning, but `"` must
// still be doubled (`""`) and `%`/`!` can still trigger delayed
// expansion in some contexts.
//
// Strategy:
//
//  1. Reject embedded control characters (NUL, LF, CR) — they cannot
//     be expressed safely in a single cmd.exe argv token.
//  2. Always wrap in double quotes (cheap; defensive even for plain
//     alphanumerics).
//  3. Double any embedded `"` to `""` (cmd.exe's escape for quoted
//     strings).
//  4. Escape `^` to `^^` so it survives cmd's de-caret pass.
//
// Returns the quoted form (INCLUDES surrounding double quotes).
//
// Usage example:
//
//	q, err := cmdArgvQuote(`a&b`)
//	// → "\"a&b\"", nil
//	q, err := cmdArgvQuote("bad\nstring")
//	// → "", err
func cmdArgvQuote(s string) (string, error) {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == 0x00 || c == '\n' || c == '\r' {
			return "", core.E("opencode.cmdArgvQuote",
				"embedded control character is not safe for cmd.exe argv", nil)
		}
	}
	var b []byte
	b = append(b, '"')
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case '"':
			b = append(b, '"', '"')
		case '^':
			b = append(b, '^', '^')
		default:
			b = append(b, c)
		}
	}
	b = append(b, '"')
	return string(b), nil
}

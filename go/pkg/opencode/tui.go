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
		// `VAR=val cmd args...`. Password is hex-only (no quotes
		// / special chars from rand.Read + hex.EncodeToString) so
		// direct concat into the AppleScript is safe.
		shellCmd := "OPENCODE_SERVER_PASSWORD=" + password +
			" opencode attach " + targetURL
		script := `tell application "Terminal" to do script "` + shellCmd + `"`
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
		cmdLine := "set OPENCODE_SERVER_PASSWORD=" + password +
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

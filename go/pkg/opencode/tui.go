// SPDX-Licence-Identifier: EUPL-1.2

// OpenTUI — spawns `<runtime> exec -it <container> opencode` inside
// the user's default terminal. Per RFC.opencode.md §6, this is the
// "Open TUI" button on the integrations card.
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
	"context"
	goruntime "runtime"
	"time"

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
	// Confirm sandbox is running — opening a TUI on a stopped
	// container produces a confusing "container not running" error
	// inside the user's new terminal window.
	infoR := s.Inspect(id)
	if !infoR.OK {
		return infoR
	}
	sb, _ := infoR.Value.(Sandbox)
	if sb.Status != StatusRunning {
		return core.Fail(core.E("opencode.OpenTUI",
			"sandbox is not running (status="+sb.Status+")", nil))
	}

	ps := s.proc()
	if ps == nil {
		return core.Fail(core.E("opencode.OpenTUI", "process service unavailable", nil))
	}

	// The command typed into the user's terminal. `runtime exec -it`
	// is the docker / podman idiom; both accept the same flags.
	innerCmd := s.runtime() + " exec -it " + ContainerName(id) + " opencode"

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	switch goruntime.GOOS {
	case "darwin":
		// AppleScript escapes embedded quotes with backslash; our
		// innerCmd has no quotes so direct concat is safe.
		script := `tell application "Terminal" to do script "` + innerCmd + `"`
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
		// $TERMINAL takes priority — distros set it via system
		// preferences. Fall through to x-terminal-emulator (the
		// Debian / Ubuntu convention) then well-known emulators.
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
			runR := ps.Run(ctx, term, "-e", innerCmd)
			if runR.OK {
				return core.Ok(nil)
			}
		}
		return core.Fail(core.E("opencode.OpenTUI",
			"no terminal emulator found (set $TERMINAL)", nil))

	case "windows":
		// Windows Terminal first; falls back to cmd.exe.
		runR := ps.Run(ctx, "wt.exe", "new-tab", "cmd", "/k", innerCmd)
		if runR.OK {
			return core.Ok(nil)
		}
		runR = ps.Run(ctx, "cmd", "/c", "start", "cmd", "/k", innerCmd)
		if runR.OK {
			return core.Ok(nil)
		}
		return runR

	default:
		return core.Fail(core.E("opencode.OpenTUI",
			"unsupported platform: "+goruntime.GOOS, nil))
	}
}

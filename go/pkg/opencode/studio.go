// SPDX-Licence-Identifier: EUPL-1.2

// Studio — host-side OpenCode native app detection + launch.
//
// Per RFC.opencode.md §6 "Open Studio (host-app)". Optional path —
// users who already run OpenCode's native desktop app on the host
// and want T1 config only get a one-click launcher here. The
// button is hidden when the app isn't detected.
//
// Platform paths:
//
//   - darwin → /Applications/OpenCode.app present? `open -a OpenCode`
//   - linux  → `opencode-studio` or similar binary on PATH (TBD —
//     opencode doesn't ship a Linux desktop app today; placeholder).
//   - windows → %ProgramFiles%/OpenCode/opencode.exe (TBD).
//
// IsStudioInstalled is the gate the frontend uses to decide
// whether to render the button at all.

package opencode

import (
	"context"
	goruntime "runtime"
	"time"

	core "dappco.re/go"
)

// studioMacPath is the canonical install location for OpenCode's
// macOS desktop app. Other paths (Setapp / sideloaded) aren't
// detected today — users can still launch via Spotlight.
const studioMacPath = "/Applications/OpenCode.app"

// IsStudioInstalled reports whether OpenCode's native desktop app
// is installed on the host. Frontend uses this to decide whether
// to render the "Open Studio" button on the integrations card.
//
// Usage example:
//
//	if svc.IsStudioInstalled() { /* render the button */ }
func (s *Service) IsStudioInstalled() bool {
	switch goruntime.GOOS {
	case "darwin":
		return core.Stat(studioMacPath).OK
	case "linux":
		// opencode doesn't ship a Linux desktop app today — leaving
		// the hook in place for when they do.
		return false
	case "windows":
		// Same — Windows desktop app TBD upstream.
		return false
	default:
		return false
	}
}

// OpenStudio launches the host's OpenCode native app. Returns
// Fail when the app isn't installed or the launch command errors.
//
// Usage example:
//
//	r := svc.OpenStudio()
//	if !r.OK { core.Println("open studio failed:", r.Error()) }
func (s *Service) OpenStudio() core.Result {
	if s == nil {
		return core.Fail(core.E("opencode.OpenStudio", "service is nil", nil))
	}
	if !s.IsStudioInstalled() {
		return core.Fail(core.E("opencode.OpenStudio",
			"OpenCode native app is not installed on this host", nil))
	}
	ps := s.proc()
	if ps == nil {
		return core.Fail(core.E("opencode.OpenStudio", "process service unavailable", nil))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	switch goruntime.GOOS {
	case "darwin":
		return ps.Run(ctx, "open", "-a", "OpenCode")
	default:
		return core.Fail(core.E("opencode.OpenStudio",
			"unsupported platform: "+goruntime.GOOS, nil))
	}
}

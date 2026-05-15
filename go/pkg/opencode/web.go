// SPDX-Licence-Identifier: EUPL-1.2

// Web — surfaces opencode-serve's browser web UI in a Wails-managed
// lthn window. The container runs `opencode web` (see opencode.go
// spawn args), which serves the same API endpoints PLUS the SPA at
// root.
//
// Why direct container port instead of the lthn reverse-proxy:
// opencode-web's HTML uses absolute asset paths (`/favicon.png`,
// `/manifest.json`, etc.), so mounting the SPA under
// `/v1/api/sandbox/<id>/` would 404 every asset. Pointing the
// Wails window at the container's directly-bound port
// (`http://127.0.0.1:<host-port>/`) makes the absolute paths
// resolve correctly. Auth is folded into the URL as Basic
// credentials — `http://opencode:<pw>@host:port/`. WebKit (Wails on
// macOS) accepts URL-embedded credentials at top-level navigation.
//
// Per the §6 launcher UX in RFC.opencode.md — this is the "Open in
// window" sibling of "Open in terminal" / "Open desktop app".

package opencode

import (
	"net/url"

	core "dappco.re/go"
)

// WebURL returns the direct-bind URL for the named sandbox's web
// UI, with Basic-auth credentials embedded so a webview navigating
// to it doesn't trip the upstream's auth gate. Returns Fail when
// the sandbox isn't running.
//
// Usage example:
//
//	r := svc.WebURL("oc-1735843891234")
//	if r.OK { url := r.Value.(string); _ = url }
func (s *Service) WebURL(id string) core.Result {
	if s == nil {
		return core.Fail(core.E("opencode.WebURL", "service is nil", nil))
	}
	if core.Trim(id) == "" {
		return core.Fail(core.E("opencode.WebURL", "id is required", nil))
	}
	infoR := s.Inspect(id)
	if !infoR.OK {
		return infoR
	}
	sb, _ := infoR.Value.(Sandbox)
	if sb.Status != StatusRunning {
		return core.Fail(core.E("opencode.WebURL",
			"sandbox is not running (status="+sb.Status+")", nil))
	}
	pwR := s.ServerPassword()
	if !pwR.OK {
		return pwR
	}
	password, _ := pwR.Value.(string)

	// url.UserPassword handles percent-encoding of the password so
	// special chars in the random hex don't break the URL.
	auth := url.UserPassword(serverAuthUsername, password)
	u := url.URL{
		Scheme: "http",
		User:   auth,
		Host:   core.Sprintf("127.0.0.1:%d", sb.HostPort),
		Path:   "/",
	}
	return core.Ok(u.String())
}

// OpenWebWindow spawns an lthn-managed Wails window pointing at the
// named sandbox's web UI. The window name is `opencode-web-<id>` so
// multiple sandboxes can have separate windows simultaneously.
//
// Requires the gui window service to be registered on the Core
// (i.e. lthn was launched via `lthn gui`, not `lthn serve`). In
// serve mode the action lookup fails — callers can fall back to
// WebURL + opening in the user's default browser.
//
// Usage example:
//
//	r := svc.OpenWebWindow("oc-1735843891234")
//	if !r.OK { /* fall back to system browser */ }
func (s *Service) OpenWebWindow(id string) core.Result {
	if s == nil {
		return core.Fail(core.E("opencode.OpenWebWindow", "service is nil", nil))
	}
	urlR := s.WebURL(id)
	if !urlR.OK {
		return urlR
	}
	target, _ := urlR.Value.(string)

	c := s.Core()
	if c == nil {
		return core.Fail(core.E("opencode.OpenWebWindow", "core is nil", nil))
	}

	// The window.open action is registered by core/gui's window
	// service. In serve mode it isn't registered, and Action.Run
	// returns a Fail with "action not found" — surface as a clear
	// error so the caller knows to fall back to system browser.
	ctx, cancel := core.WithTimeout(core.Background(), 5*core.Second)
	defer cancel()

	// Build the TaskOpenWindow payload as a typed map so we don't
	// take a hard dep on the upstream guiwindow package's exported
	// Window struct (consumed via the action surface keeps this
	// file dep-light + survives upstream API tweaks).
	taskWindow := map[string]any{
		"Name":             "opencode-web-" + id,
		"Title":            "OpenCode · " + id,
		"Width":            1280,
		"Height":           840,
		"MinWidth":         800,
		"MinHeight":        600,
		"URL":              target,
		"Frameless":        false,
		"Hidden":           false,
		"EnableFileDrop":   false,
		"BackgroundColour": [4]uint8{0, 0, 0, 0},
	}
	r := c.Action("window.open").Run(ctx, core.NewOptions(
		core.Option{Key: "task", Value: map[string]any{
			"Window": taskWindow,
		}},
	))
	if !r.OK {
		return core.Fail(core.E("opencode.OpenWebWindow",
			"window.open failed (is lthn running in GUI mode?): "+r.Error(), nil))
	}
	return core.Ok(map[string]any{
		"name": "opencode-web-" + id,
		"url":  target,
	})
}

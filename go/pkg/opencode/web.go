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
// resolve correctly.
//
// Auth discipline (Mantis #1600 HIGH, Cerberus #22):
// the previous implementation folded OPENCODE_SERVER_PASSWORD into
// the URL as Basic userinfo (`http://opencode:<pw>@host:port/`).
// That leaks via Referer headers, document.title, the clipboard,
// DevTools network panel, and every subresource fetch. The HTTP
// control surface (GET /v1/api/opencode/sandbox/:id/web) now
// returns a CREDENTIAL-FREE envelope — the bare URL plus auth-
// scheme metadata. The in-process Wails GUI path
// (OpenWebWindow → webURLWithCreds) keeps URL-userinfo for
// top-level navigation only; that path NEVER crosses an HTTP
// wire response, so a local-attacker holding the bearer token
// cannot exfiltrate the password through the documented endpoint.
// Per-request WebView header injection (the upstream Wails fix
// that would close the Referer / title side-channels) is tracked
// as Mantis #1606 follow-up — substrate not present in core/gui
// today.
//
// Per the §6 launcher UX in RFC.opencode.md — this is the "Open in
// window" sibling of "Open in terminal" / "Open desktop app".

package opencode

import (
	"net/url"

	core "dappco.re/go"
)

// WebAuthScheme is the credential scheme an opencode-web caller must
// present when navigating to the URL returned by WebURL. Pinned to
// the HTTP Basic format opencode-serve negotiates (RFC 7617).
const WebAuthScheme = "basic"

// WebAuthVia signals where the credential MUST be injected when the
// caller drives navigation themselves. "header" = Authorization
// request-header; "url-userinfo" would re-enable the leak vector
// Mantis #1600 closed, so the constant exists only as the safe value.
const WebAuthVia = "header"

// WebInfo is the credential-free envelope returned by WebURL and
// surfaced over the HTTP control endpoint. Field-shape is the wire
// contract — opencode-web frontends parse this directly.
//
// SECURITY-NOTE (Mantis #1600 HIGH, Cerberus #22): NO password field.
// The struct is the type-system gate that keeps the password out of
// every HTTP response, audit log entry, and JS console dump. If a
// future contributor adds a Password / Userinfo / Token field here,
// the leak vector is reopened — the field has no callers because the
// caller injects the credential at navigation time via the auth
// metadata, not by reading it from the response.
//
// Usage example:
//
//	r := svc.WebURL("oc-1735843891234")
//	if r.OK {
//	    info := r.Value.(WebInfo)
//	    // info.URL is "http://127.0.0.1:51823/"
//	    // info.Auth.Scheme is "basic"; caller forms Authorization
//	    // header from its own credential-store-resolved password.
//	}
type WebInfo struct {
	URL  string      `json:"url"`
	Auth WebAuthInfo `json:"auth"`
}

// WebAuthInfo documents the auth scheme a navigator must apply to
// reach WebInfo.URL. Wire-contract for frontends + audit-log shape.
type WebAuthInfo struct {
	// Scheme is the auth scheme literal (WebAuthScheme).
	Scheme string `json:"scheme"`
	// Via is the injection point literal (WebAuthVia).
	Via string `json:"via"`
	// Username is the static user opencode-serve expects. Never the
	// password — the caller resolves the password from its own
	// credential store (the lthn process holds it, not the wire).
	Username string `json:"username"`
}

// buildWebInfo composes the credential-free WebInfo envelope from the
// resolved host + port. Pure function for unit-test surface; no
// password ever flows in or out.
//
// Usage example:
//
//	info := buildWebInfo(51823)
//	// info.URL == "http://127.0.0.1:51823/"
//	// info.Auth.Scheme == "basic"; info.Auth.Username == "opencode"
func buildWebInfo(hostPort int) WebInfo {
	return WebInfo{
		URL: (&url.URL{
			Scheme: "http",
			Host:   core.Sprintf("127.0.0.1:%d", hostPort),
			Path:   "/",
		}).String(),
		Auth: WebAuthInfo{
			Scheme:   WebAuthScheme,
			Via:      WebAuthVia,
			Username: serverAuthUsername,
		},
	}
}

// WebURL returns the credential-free WebInfo envelope for the named
// sandbox's web UI. The URL has NO embedded password — callers must
// inject the credential per WebInfo.Auth at navigation time. Returns
// Fail when the sandbox isn't running.
//
// Mantis #1600 HIGH / Cerberus #22 — the HTTP control surface that
// wraps this method MUST NOT reintroduce the password into the wire
// response. The in-process Wails GUI path uses webURLWithCreds
// instead and consumes the URL inside the same process boundary.
//
// Usage example:
//
//	r := svc.WebURL("oc-1735843891234")
//	if r.OK { info := r.Value.(WebInfo); _ = info.URL }
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
	return core.Ok(buildWebInfo(sb.HostPort))
}

// webURLWithCreds returns the legacy URL-userinfo form
// (`http://opencode:<pw>@host:port/`) for the in-process Wails GUI
// path ONLY. Unexported so the HTTP control surface cannot reach it
// and accidentally reintroduce the Mantis #1600 leak.
//
// SECURITY-NOTE (Mantis #1600 HIGH, Cerberus #22): the returned URL
// embeds OPENCODE_SERVER_PASSWORD. The caller MUST treat it as a
// process-local credential — never log it, never echo to the wire,
// never write to disk outside the Wails navigation handler. Today
// the only caller is OpenWebWindow's window.open dispatch; that
// invocation hands the URL to core/gui in-process, which feeds the
// host WebView's top-level navigation. WebView side-channels
// (Referer, document.title, devtools network panel) still see the
// userinfo — Mantis #1606 tracks the per-request header-injection
// substrate that would close those vectors.
func (s *Service) webURLWithCreds(id string) core.Result {
	if s == nil {
		return core.Fail(core.E("opencode.webURLWithCreds", "service is nil", nil))
	}
	if core.Trim(id) == "" {
		return core.Fail(core.E("opencode.webURLWithCreds", "id is required", nil))
	}
	infoR := s.Inspect(id)
	if !infoR.OK {
		return infoR
	}
	sb, _ := infoR.Value.(Sandbox)
	if sb.Status != StatusRunning {
		return core.Fail(core.E("opencode.webURLWithCreds",
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
	// Mantis #1600 — webURLWithCreds is the in-process path. The
	// returned URL contains URL-userinfo credentials and is fed
	// directly to the Wails window.open action below; it never
	// crosses an HTTP response wire. See webURLWithCreds doc for
	// the residual side-channel scope (Referer / title — Mantis
	// #1606 follow-up for per-request header injection).
	urlR := s.webURLWithCreds(id)
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

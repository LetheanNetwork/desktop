// SPDX-Licence-Identifier: EUPL-1.2

// Wails3 Service shape for the server package — extends the existing
// *Service with the Wails lifecycle hooks and read-only accessors
// the WebView's "endpoint" slots in Settings → API, Integrations,
// and the Welcome step-3 snippet bind against. The standalone
// Start / Stop / Engine / Handler methods stay on the type for the
// `lthn serve` CLI path; the W-prefixed methods below are what the
// frontend binds.

package server

import (
	"context"

	core "dappco.re/go"
)

// ServiceName is the Wails service name. Lower-case + plural to
// match the existing Wails surfaces (runner, sessions, models).
func (s *Service) ServiceName() string { return "Server" }

// ServiceStartup runs on Wails boot. Returns nil unconditionally —
// the gin engine is constructed in NewService, before the Wails
// application takes ownership.
func (s *Service) ServiceStartup(_ context.Context, _ any) core.Result {
	return core.Ok(nil)
}

// ServiceShutdown runs on Wails teardown. No-op — pkg/desktop's
// shutdown path calls Stop on the underlying *Service directly.
func (s *Service) ServiceShutdown() core.Result { return core.Ok(nil) }

// WAddr returns the configured bind address for the OpenAI-compat
// HTTP surface. Default is ":8000"; the WebView display slots add
// the "http://localhost" + "/v1" affixes themselves so the same
// raw value works in both `lthn serve` log lines and the UI.
//
// Usage example (TS):
//
//	import { WAddr } from "@desktop/server/service";
//	const addr = await WAddr();   // ":8000"
func (s *Service) WAddr() core.Result {
	if s == nil {
		return core.Ok(":8000")
	}
	return core.Ok(s.opts.Addr)
}

// WListening reports whether Start() is actively serving on Addr.
// False in desktop mode (the gin engine is mounted on Wails'
// AssetServer instead — the configured Addr is never bound), true
// inside `lthn serve`.
//
// Usage example (TS):
//
//	import { WListening } from "@desktop/server/service";
//	const on = await WListening();
func (s *Service) WListening() core.Result {
	if s == nil {
		return core.Ok(false)
	}
	return core.Ok(s.listening)
}

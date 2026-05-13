// SPDX-Licence-Identifier: EUPL-1.2

// Wails3 Service shape for pkg/apikey — exposes the local bearer
// token so Settings → API can render it (masked + Reveal toggle)
// and the Integrations snippet can show the value users paste into
// client configs.
//
// The key never leaves the in-process Wails channel — bindings run
// in the WebView's same-process context, no network round-trip, no
// renderer-vs-main-process boundary.

package apikey

import (
	"context"

	core "dappco.re/go"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// WailsService is the bindable surface. Bound by
// application.NewService(apikey.NewWailsService(coreInstance)) in
// pkg/desktop/desktop.go.
type WailsService struct {
	core *core.Core
}

// NewWailsService constructs the Wails-bindable shape. The *core.Core
// is captured so Reveal / Rotate / Masked can resolve config.Service
// at call time without holding a stale reference.
func NewWailsService(c *core.Core) *WailsService {
	return &WailsService{core: c}
}

// ServiceName / Startup / Shutdown — Wails3 lifecycle.
func (s *WailsService) ServiceName() string { return "ApiKey" }
func (s *WailsService) ServiceStartup(_ context.Context, _ application.ServiceOptions) error {
	return nil
}
func (s *WailsService) ServiceShutdown() error { return nil }

// Reveal returns the full local bearer token. The Settings → API
// "Show" toggle calls this when the user explicitly clicks to
// expose the value; the default Masked() form is what renders by
// default.
//
// Usage example (TS):
//
//	import { Reveal } from "@desktop/apikey/wailsservice";
//	const key = await Reveal();
func (s *WailsService) Reveal() (string, error) {
	if s == nil || s.core == nil {
		return "", nil
	}
	key, r := GenerateOrLoad(s.core)
	if !r.OK {
		return "", nil
	}
	return key, nil
}

// Masked returns the UI-safe form — prefix + first/last 4 chars +
// bullets between. Drives the default render in Settings → API and
// the Integrations config snippet.
//
// Usage example (TS):
//
//	import { Masked } from "@desktop/apikey/wailsservice";
//	const display = await Masked();
//	// → "sk-lthn-0011••••••••••••••••eeff"
func (s *WailsService) Masked() (string, error) {
	if s == nil || s.core == nil {
		return "", nil
	}
	key, r := GenerateOrLoad(s.core)
	if !r.OK {
		return "", nil
	}
	return Mask(key), nil
}

// WRotate generates a fresh key, persists it, and returns the new
// value. The Settings → API "Rotate" button calls this; the new
// key applies on next server restart.
//
// Usage example (TS):
//
//	import { WRotate } from "@desktop/apikey/wailsservice";
//	const newKey = await WRotate();
func (s *WailsService) WRotate() (string, error) {
	if s == nil || s.core == nil {
		return "", nil
	}
	key, r := Rotate(s.core)
	if !r.OK {
		return "", nil
	}
	return key, nil
}

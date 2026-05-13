// SPDX-Licence-Identifier: EUPL-1.2

// pkg/desktop/bindings.go now carries only WindowService — the
// desktop-side router for our window registry (see windows.go).
// Every other adapter that used to live here has moved into its
// owning package as a proper Wails3 Service:
//
//	runner       → pkg/runner/wails.go
//	sessions     → pkg/sessions/wails.go
//	models       → pkg/models/wails.go
//	firstlaunch  → pkg/firstlaunch/wails.go
//	validator    → pkg/validator/wails.go
//	telemetry    → pkg/telemetry/service.go (extended with Wails shape)
//	lthnservices → pkg/services/wails.go
//
// Upstream services come straight from Core:
//
//	i18n   → dappco.re/go/i18n.CoreService     (registered in cmd/lthn/app.go)
//	config → dappco.re/go/config.Service       (registered in cmd/lthn/app.go)
//
// Frontend env / dialog / browser / screen / clipboard come from
// @wailsio/runtime directly — Wails ships these natively, no Go
// adapter needed. Removed from the Services array in desktop.go.
//
// WindowService stays here because it's a thin router over the
// in-process windows.go registry; lifting it into its own package
// would require pulling that registry along with it. Once windows.go
// becomes its own package this file can be deleted entirely.

package desktop

import (
	"context"

	core "dappco.re/go"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// ---------------------------------------------------------------------
// WindowService — open / hide / focus named windows from the frontend.
// ---------------------------------------------------------------------

type WindowService struct {
	app *application.App
}

func NewWindowService() *WindowService { return &WindowService{} }

func (s *WindowService) ServiceName() string { return "Window" }
func (s *WindowService) ServiceStartup(_ context.Context, _ application.ServiceOptions) error {
	return nil
}
func (s *WindowService) ServiceShutdown() error { return nil }

// Open shows + focuses the named window (chat / models / settings /
// about). No-op if the name isn't in the windows.go registry.
func (s *WindowService) Open(name string) error {
	if s.app == nil {
		return core.NewError("window service not yet attached to wails app")
	}
	openWindow(s.app, name)
	return nil
}

// Hide hides the named window. Steady-state windows (chat / models
// / settings) hide-on-close anyway; this lets the frontend dismiss
// programmatically without waiting for a close click.
func (s *WindowService) Hide(name string) error {
	if s.app == nil {
		return core.NewError("window service not yet attached to wails app")
	}
	w, ok := s.app.Window.GetByName(name)
	if !ok {
		return core.NewError("no window named: " + name)
	}
	w.Hide()
	return nil
}

// List returns the names of every registered window. Frontend can
// render a "window switcher" or jump-list from this.
func (s *WindowService) List() ([]string, error) {
	registry := windowRegistry()
	names := make([]string, len(registry))
	for i, spec := range registry {
		names[i] = spec.Name
	}
	return names, nil
}

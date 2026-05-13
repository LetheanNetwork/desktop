// SPDX-Licence-Identifier: EUPL-1.2

// Tests for the pure parts of pkg/desktop — the WindowSpec registry
// and the WindowService surface that delegates to it. The Wails-tied
// surfaces (openWindow, preCreateWindows, Service.Run) need a live
// *application.App, so coverage there belongs to an integration suite
// that boots Wails in headless mode — not this file.

package desktop_test

import (
	"strings"

	core "dappco.re/go"

	"dappco.re/lthn/desktop/pkg/desktop"
)

func TestDesktop_NewService_Good_Defaults(t *core.T) {
	s := desktop.NewService(desktop.Options{})
	core.AssertNotNil(t, s, "NewService must always return a value")
}

func TestDesktop_NewService_Good_AllOptionsSet(t *core.T) {
	// Even the option-rich constructor shouldn't touch Wails until
	// Run() is called — pure assembly.
	s := desktop.NewService(desktop.Options{
		Name:         "lthn-test",
		Description:  "test suite",
		FrontendRoot: "dist",
		TrayIcon:     []byte{0x89, 0x50, 0x4e, 0x47}, // PNG magic stub
		AppIcon:      []byte{0x89, 0x50, 0x4e, 0x47},
	})
	core.AssertNotNil(t, s)
}

// Run without Server set must Fail cleanly rather than panic on the
// nil dereference. Verifies the first defensive check in Run().
func TestDesktop_Run_Bad_NoServer(t *core.T) {
	s := desktop.NewService(desktop.Options{})
	r := s.Run()
	core.AssertFalse(t, r.OK, "Run() must Fail when Options.Server is nil")
}

func TestDesktop_NewWindowService_Good(t *core.T) {
	w := desktop.NewWindowService()
	core.AssertNotNil(t, w)
	core.AssertEqual(t, "Window", w.ServiceName())
}

// List returns the names of every registered window. Pure — pulls
// straight from the static windowRegistry() table.
func TestDesktop_WindowService_List_Good(t *core.T) {
	w := desktop.NewWindowService()
	names, err := w.List()
	core.AssertNoError(t, err)
	core.AssertGreater(t, len(names), 0, "registry must not be empty")

	// Every window we shipped this session should be in the list.
	expected := []string{"welcome", "app", "chat", "models", "settings", "telemetry", "about"}
	joined := strings.Join(names, ",")
	for _, want := range expected {
		core.AssertContains(t, joined, want, "registry should include "+want)
	}
}

// Open / Hide are no-ops before Service.Run wires up the Wails app
// pointer. The defensive guard returns an error rather than panicking
// — exercised here so the error path is covered. AssertError variadic
// args are substring requirements; we only care that err is non-nil
// here, so no substring is given.
func TestDesktop_WindowService_Open_Bad_NotAttached(t *core.T) {
	w := desktop.NewWindowService()
	err := w.Open("chat")
	core.AssertError(t, err)
}

func TestDesktop_WindowService_Hide_Bad_NotAttached(t *core.T) {
	w := desktop.NewWindowService()
	err := w.Hide("chat")
	core.AssertError(t, err)
}

// The package-level Wails services that used to live in bindings.go
// have moved into their owning packages — exercising them lives next
// to each package's own test file now. The desktop layer only owns
// WindowService today (smoke-tested above).

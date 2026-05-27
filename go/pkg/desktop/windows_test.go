// SPDX-Licence-Identifier: EUPL-1.2

// Tests for the pure parts of pkg/desktop — the WindowSpec registry
// and the WindowService surface that delegates to it. The Wails-tied
// surfaces (openWindow, preCreateWindows, Service.Run) need a live
// *application.App, so coverage there belongs to an integration suite
// that boots Wails in headless mode — not this file.

package desktop_test

import (
	core "dappco.re/go"

	"dappco.re/lthn/desktop/pkg/desktop"
)

func TestDesktop_NewService_Good_Defaults(t *core.T) {
	s := desktop.NewService(desktop.Options{})
	core.AssertNotNil(t, s, "NewService must always return a value")
	r := s.Run()
	core.AssertFalse(t, r.OK)
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

// WindowService now lives in core/gui as gui.WindowBindingService;
// the smoke tests that used to live here moved into core/gui's test
// suite. The desktop layer no longer owns a Wails-bindable window
// service — every consumer pulls gui.NewWindowBindingService instead.

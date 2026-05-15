// SPDX-Licence-Identifier: EUPL-1.2

// Activation-policy switching, routed through CoreGUI's dock service.
// The framework's HideAppIcon / ShowAppIcon calls [NSApp setActivationPolicy:]
// on macOS via its private substrate and no-ops on Windows/Linux/iOS,
// which is exactly the cross-platform shape we want here.
//
// We keep a package-level reference to Core captured at
// construction in desktop.Run() because:
//   - There's exactly one application runtime per process, so exactly
//     one dock service per process.
//   - The closures inside preCreateWindows() / openWindow() need to
//     call into it, and threading *core.Core through every
//     function signature would noise up the lifecycle code.
//
// If sharedDockCore is nil (e.g. tests that don't call Run()), both
// helpers no-op silently — same as the non-darwin dock backends.

package desktop

import (

	core "dappco.re/go"
)

var sharedDockCore *core.Core

// attachDock records Core so setPolicy* can find the dock service.
// Called once from desktop.Run() before any window is created.
func attachDock(c *core.Core) { sharedDockCore = c }

// setPolicyRegular elevates the macOS activation policy to Regular —
// the app gets a Dock icon, a Cmd+Tab entry, and a menubar. No-op
// on Windows/Linux/iOS.
func setPolicyRegular() {
	if sharedDockCore != nil {
		sharedDockCore.Action("dock.show_icon").Run(core.Background(), core.NewOptions())
	}
}

// setPolicyAccessory demotes back to Accessory — the app disappears
// from the Dock and Cmd+Tab, leaving only the menubar tray. No-op
// on Windows/Linux/iOS.
func setPolicyAccessory() {
	if sharedDockCore != nil {
		sharedDockCore.Action("dock.hide_icon").Run(core.Background(), core.NewOptions())
	}
}

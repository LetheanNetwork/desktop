// SPDX-Licence-Identifier: EUPL-1.2

// Activation-policy switching, routed through Wails3's built-in
// dock service. The dock service's HideAppIcon / ShowAppIcon calls
// [NSApp setActivationPolicy:] on macOS via cgo and no-op on
// Windows/Linux/iOS, which is exactly the cross-platform shape we
// want here.
//
// We keep a package-level reference to the dock service captured at
// construction in desktop.Run() because:
//   - There's exactly one application.App per process by Wails'
//     design, so exactly one dock service per process.
//   - The closures inside preCreateWindows() / openWindow() need to
//     call into it, and threading *dock.DockService through every
//     function signature would noise up the lifecycle code.
//
// If sharedDock is nil (e.g. tests that don't call Run()), both
// helpers no-op silently — same as the non-darwin dock backends.

package desktop

import "github.com/wailsapp/wails/v3/pkg/services/dock"

var sharedDock *dock.DockService

// attachDock records the dock service so setPolicy* can find it.
// Called once from desktop.Run() before any window is created.
func attachDock(d *dock.DockService) { sharedDock = d }

// setPolicyRegular elevates the macOS activation policy to Regular —
// the app gets a Dock icon, a Cmd+Tab entry, and a menubar. No-op
// on Windows/Linux/iOS.
func setPolicyRegular() {
	if sharedDock != nil {
		sharedDock.ShowAppIcon()
	}
}

// setPolicyAccessory demotes back to Accessory — the app disappears
// from the Dock and Cmd+Tab, leaving only the menubar tray. No-op
// on Windows/Linux/iOS.
func setPolicyAccessory() {
	if sharedDock != nil {
		sharedDock.HideAppIcon()
	}
}

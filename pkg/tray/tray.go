// SPDX-Licence-Identifier: EUPL-1.2

// Package tray registers the native system-tray icon and anchors the
// popover panel + transient expansion windows to the tray-process.
//
// The tray IS the process — windows are transient surfaces; closing
// them does NOT quit the app. See RFC.first-release.md §1.3.
//
//	c := core.New()
//	tray.Register(c)
package tray

// Service registers the NSStatusItem (on macOS), anchors the popover
// panel, and exposes a `tray.spawn` action that opens a transient
// expansion window by name (chat / settings / welcome / etc.).
type Service struct {
	// Fields wired against core/gui's tray + window APIs.
}

// New constructs the tray service.
//
//	s := tray.New()
//	s.Register(c)
func New() *Service { return &Service{} }

// Register wires the tray service into the Core container.
// Pattern per Mantis #1336 canonical Service.go.
func (s *Service) Register() error {
	// Wires:
	//   - NSStatusItem registration (light + dark icon variants)
	//   - Popover panel anchor (400×560 — see RFC.first-release.md §4)
	//   - Right-click menu (per-window entries: Chat… / Settings… / Quit)
	//   - tray.spawn action that opens an expansion window
	//   - tray.dismiss action that closes the popover
	return nil
}

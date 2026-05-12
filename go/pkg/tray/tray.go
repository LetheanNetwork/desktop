// SPDX-Licence-Identifier: EUPL-1.2

// Package tray registers the native system-tray icon and anchors the
// popover panel + transient expansion windows to the tray-process.
//
// The tray IS the process — windows are transient surfaces; closing
// them does NOT quit the app. See RFC.first-release.md §1.3.
//
// Usage example:
//
//	c := core.New()
//	t := tray.NewService(tray.Options{})
//	if r := t.Register(c); !r.OK {
//		return r
//	}
package tray

import (
	core "dappco.re/go"
)

// Options configures the tray service.
type Options struct {
	// IconLightPath is the SVG/PNG file path for the light-mode tray
	// icon. Empty = use embedded default.
	IconLightPath string
	// IconDarkPath is the dark-mode tray icon. Empty = use embedded.
	IconDarkPath string
}

// Service registers the NSStatusItem (on macOS), anchors the popover
// panel, and exposes a `tray.spawn` action that opens a transient
// expansion window by name (chat / settings / welcome / etc.).
type Service struct {
	opts Options
}

// NewService constructs the tray service.
//
// Usage example:
//
//	t := tray.NewService(tray.Options{})
//	t.Register(c)
func NewService(opts Options) *Service {
	return &Service{opts: opts}
}

// Register wires the tray service into the Core container. Pattern
// per Mantis #1336 canonical Service.go.
//
// Usage example:
//
//	if r := svc.Register(c); !r.OK {
//		return r
//	}
func (s *Service) Register(c *core.Core) core.Result {
	// TODO when core/gui tray API is integrable: wire
	//   - NSStatusItem registration (light + dark icon variants)
	//   - Popover panel anchor (400×560 — see RFC.first-release.md §4)
	//   - Right-click menu (per-window entries: Chat… / Settings… / Quit)
	//   - tray.spawn action that opens an expansion window
	//   - tray.dismiss action that closes the popover
	return core.Ok(nil)
}

// Register constructs a default tray Service and wires it into the
// Core container. One-shot canonical entry per Mantis #1336.
//
// Usage example:
//
//	if r := tray.Register(c); !r.OK {
//		return r
//	}
func Register(c *core.Core) core.Result {
	return NewService(Options{}).Register(c)
}

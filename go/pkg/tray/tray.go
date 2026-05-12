// SPDX-Licence-Identifier: EUPL-1.2

// Package tray registers the native system-tray icon and anchors the
// popover panel + transient expansion windows to the tray-process.
//
// The tray IS the process — windows are transient surfaces; closing
// them does NOT quit the app. The NSStatusItem (macOS) is the lifetime
// anchor; ApplicationShouldTerminateAfterLastWindowClosed is false.
// See RFC.first-release.md §1.3.
//
// Usage example:
//
//	t := tray.NewService(tray.Options{Name: "lthn", Description: "Lethean Desktop"})
//	if r := t.Run(); !r.OK {
//		return r
//	}
package tray

import (
	"runtime"

	core "dappco.re/go"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/icons"
)

// Options configures the tray service at construction time.
type Options struct {
	// Name is shown in the macOS menu bar accessibility label.
	Name string
	// Description is the app description shown by the OS.
	Description string
	// IconLight is the light-mode tray icon (PNG/SVG bytes). Empty =
	// use Wails default template icon (a generic Wails glyph).
	IconLight []byte
	// IconDark is the dark-mode tray icon. Empty = same as IconLight.
	IconDark []byte
}

// Service holds the Wails application and the SystemTray anchor.
// Constructed via NewService, started via Run.
type Service struct {
	opts Options
	app  *application.App
}

// NewService constructs the tray service. Does NOT start Wails yet;
// call Run() to block on the Wails event loop.
//
// Usage example:
//
//	t := tray.NewService(tray.Options{Name: "lthn"})
//	if r := t.Run(); !r.OK { return r }
func NewService(opts Options) *Service {
	if opts.Name == "" {
		opts.Name = "lthn"
	}
	if opts.Description == "" {
		opts.Description = "Lethean Desktop"
	}
	return &Service{opts: opts}
}

// Run launches the Wails app loop. Blocks. The NSStatusItem is the
// lifetime anchor — the app does NOT quit when windows close. Quit
// is via the tray menu.
//
// Usage example:
//
//	if r := tray.NewService(tray.Options{}).Run(); !r.OK { return r }
func (s *Service) Run() core.Result {
	s.app = application.New(application.Options{
		Name:        s.opts.Name,
		Description: s.opts.Description,
		Assets:      application.AlphaAssets,
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: false,
			ActivationPolicy:                                application.ActivationPolicyAccessory,
		},
	})

	systray := s.app.SystemTray.New()
	if runtime.GOOS == "darwin" {
		if s.opts.IconLight != nil {
			systray.SetTemplateIcon(s.opts.IconLight)
		} else {
			systray.SetTemplateIcon(icons.SystrayMacTemplate)
		}
	} else if s.opts.IconLight != nil {
		systray.SetIcon(s.opts.IconLight)
	}

	menu := s.app.Menu.New()
	menu.Add("Lethean Desktop").SetEnabled(false)
	menu.AddSeparator()
	menu.Add("Quit lthn").OnClick(func(_ *application.Context) {
		s.app.Quit()
	})
	systray.SetMenu(menu)

	if err := s.app.Run(); err != nil {
		return core.Fail(err)
	}
	return core.Ok(nil)
}

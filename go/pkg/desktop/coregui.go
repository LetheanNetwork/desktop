// SPDX-Licence-Identifier: EUPL-1.2

package desktop

import (
	core "dappco.re/go"
	gui "dappco.re/go/gui"
	guibrowser "dappco.re/go/gui/pkg/browser"
	guiclipboard "dappco.re/go/gui/pkg/clipboard"
	guicontextmenu "dappco.re/go/gui/pkg/contextmenu"
	guidialog "dappco.re/go/gui/pkg/dialog"
	guidisplay "dappco.re/go/gui/pkg/display"
	guidock "dappco.re/go/gui/pkg/dock"
	guienvironment "dappco.re/go/gui/pkg/environment"
	guievents "dappco.re/go/gui/pkg/events"
	guikeybinding "dappco.re/go/gui/pkg/keybinding"
	guilifecycle "dappco.re/go/gui/pkg/lifecycle"
	guimenu "dappco.re/go/gui/pkg/menu"
	guinotification "dappco.re/go/gui/pkg/notification"
	guiscreen "dappco.re/go/gui/pkg/screen"
	guisystray "dappco.re/go/gui/pkg/systray"
	guiwebview "dappco.re/go/gui/pkg/webview"
	guiwindow "dappco.re/go/gui/pkg/window"
)

func (s *Service) registerCoreGUI() core.Result {
	if s == nil || s.opts.Core == nil || s.app == nil {
		return core.Ok(nil)
	}
	registrations := []struct {
		name    string
		factory func(*core.Core) core.Result
	}{
		{name: "gui", factory: gui.NewService(gui.GuiConfig{})},
		{name: "display", factory: guidisplay.Register(s.app)},
		{name: "window", factory: guiwindow.Register(guiwindow.NewWailsPlatform(s.app))},
		{name: "webview", factory: guiwebview.Register()},
		{name: "menu", factory: guimenu.Register(guimenu.NewWailsPlatform(s.app))},
		{name: "systray", factory: guisystray.Register(guisystray.NewWailsPlatform(s.app))},
		{name: "browser", factory: guibrowser.Register(guibrowser.NewWailsPlatform(s.app))},
		{name: "notification", factory: guinotification.Register(guinotification.NewWailsPlatform(s.app))},
		{name: "lifecycle", factory: guilifecycle.Register(guilifecycle.NewWailsPlatform(s.app))},
		{name: "dialog", factory: guidialog.Register(guidialog.NewWailsPlatform(s.app))},
		{name: "contextmenu", factory: guicontextmenu.Register(guicontextmenu.NewWailsPlatform(s.app))},
		{name: "keybinding", factory: guikeybinding.Register(guikeybinding.NewWailsPlatform(s.app))},
		{name: "dock", factory: guidock.Register(guidock.NewWailsPlatform(s.app))},
		{name: "environment", factory: guienvironment.Register(guienvironment.NewWailsPlatform(s.app))},
		{name: "screen", factory: guiscreen.Register(guiscreen.NewWailsPlatform(s.app))},
		{name: "clipboard", factory: guiclipboard.Register(guiclipboard.NewWailsPlatform(s.app))},
		{name: "events", factory: guievents.Register(guievents.NewWailsPlatform(s.app))},
	}
	for _, registration := range registrations {
		if r := registerStartedCoreGUIService(s.opts.Core, registration.name, registration.factory); !r.OK {
			return r
		}
	}
	return core.Ok(nil)
}

func registerStartedCoreGUIService(c *core.Core, name string, factory func(*core.Core) core.Result) core.Result {
	if c.Service(name).OK {
		return core.Ok(nil)
	}
	r := factory(c)
	if !r.OK {
		return core.Fail(core.E("desktop.registerCoreGUI", "register "+name+" failed", r.Value.(error)))
	}
	if r.Value != nil {
		if rr := c.RegisterService(name, r.Value); !rr.OK {
			return rr
		}
	}
	svc := c.Service(name)
	if !svc.OK {
		return core.Ok(nil)
	}
	startable, ok := svc.Value.(core.Startable)
	if !ok {
		return core.Ok(nil)
	}
	if sr := startable.OnStartup(core.Background()); !sr.OK {
		return core.Fail(core.E("desktop.registerCoreGUI", "startup "+name+" failed", sr.Value.(error)))
	}
	return core.Ok(nil)
}

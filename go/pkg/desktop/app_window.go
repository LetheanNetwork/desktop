// SPDX-Licence-Identifier: EUPL-1.2

// Tear-off — one in-shell application window becomes one native window.
//
// The Angular desktop is a windowing shell inside a single frameless native
// window. Tearing an application out means opening a second native window on
// the same single-page application, at the solo route that renders only that
// application: /#/w/<app> and /#/w/<app>/<pane>.
//
// The renderer names an application, never a URL. A native window opened at a
// URL the renderer chose would be a browser, so the application identifier is
// checked against the catalogue whitelist below and the route is assembled
// here from constants. An unknown identifier opens nothing and says so.

package desktop

import (
	core "dappco.re/go"
	"dappco.re/go/config"
	gui "dappco.re/go/render/display/webkit"
	guiwindow "dappco.re/go/render/display/webkit/pkg/window"
	"dappco.re/lthn/desktop/pkg/paths"
)

const (
	// minimumTearOffDimension keeps a carried size large enough to still be a
	// window rather than a sliver the operator cannot grab.
	minimumTearOffDimension = 320
	// maximumTearOffDimension rejects a size no display could hold, which is
	// what a corrupt or hostile renderer payload looks like.
	maximumTearOffDimension = 8192
)

// tearOffAppIDs is the whitelist of applications that may own a native
// window. It mirrors APPS in frontend/src/app/desktop/desktop-catalogue.data.ts
// — one entry per launchable application — and the two are pinned together by
// scripts/verify-frontend-convergence.test.mjs, so an application added to the
// catalogue without being added here fails the contract suite rather than
// silently refusing to tear off.
var tearOffAppIDs = []string{
	"activity",
	"agents",
	"chat",
	"control",
	"crm",
	"databases",
	"documents",
	"files",
	"games",
	"ide",
	"lethernet",
	"mail",
	"marketing",
	"marketplace",
	"ml-lab",
	"notepad",
	"operations",
	"project-manager",
	"scm",
	"settings",
	"telemetry",
	"tenant",
	"welcome",
}

// AppWindowRequest is the renderer's tear-off payload: which application, on
// which pane, at the size it had in the shell. Width and Height are optional —
// zero carries nothing and the tear-off window profile keeps its own default.
type AppWindowRequest struct {
	App    string `json:"app"`
	Pane   string `json:"pane,omitempty"`
	Width  int    `json:"width,omitempty"`
	Height int    `json:"height,omitempty"`
}

// tearOffAppAllowed reports whether the application may own a native window.
func tearOffAppAllowed(appID string) bool {
	for _, candidate := range tearOffAppIDs {
		if candidate == appID {
			return true
		}
	}
	return false
}

// tearOffRoute assembles the solo hash route for one application and pane.
// The application must be on the whitelist; the pane, when present, must be a
// single safe route segment. Everything else returns false and opens nothing.
func tearOffRoute(appID, pane string) (string, bool) {
	if !tearOffAppAllowed(appID) {
		return "", false
	}
	if pane == "" {
		return nativeAppRoutePrefix + appID, true
	}
	if !paths.IsValidPluginCode(pane) {
		return "", false
	}
	return nativeAppRoutePrefix + appID + "/" + pane, true
}

// tearOffDimensions validates a carried window size. Zero means "carry
// nothing"; anything outside the sane range is rejected rather than clamped,
// so a wrong number is visible instead of quietly becoming a different one.
func tearOffDimensions(width, height int) (int, int, bool) {
	if width == 0 && height == 0 {
		return 0, 0, true
	}
	if width < minimumTearOffDimension || width > maximumTearOffDimension {
		return 0, 0, false
	}
	if height < minimumTearOffDimension || height > maximumTearOffDimension {
		return 0, 0, false
	}
	return width, height, true
}

// dockAppEvent is broadcast when a solo window asks the shell to take its
// application back. Every window receives it; the shell adopts, the solo
// window ignores its own request. Frontend listener:
// frontend/src/app/store/desktop.effects.ts (DOCK_APP_EVENT).
const dockAppEvent = "lthn:app:dock"

// AppWindowService is the renderer-facing tear-off binding. It is the only
// route from the WebView to a second native window, and it accepts an
// application identifier rather than a URL.
type AppWindowService struct {
	core *core.Core
}

// NewAppWindowService builds the tear-off binding bound to the supplied Core.
// Pass the result through gui.Bind into GuiConfig.Bindings.
func NewAppWindowService(c *core.Core) *AppWindowService {
	return &AppWindowService{core: c}
}

// ServiceName / ServiceStartup / ServiceShutdown — Wails3 lifecycle. The Core
// is captured at construction, so there is nothing to wire at startup.
func (s *AppWindowService) ServiceName() string { return "AppWindow" }

// ServiceStartup is the Wails lifecycle no-op.
func (s *AppWindowService) ServiceStartup(_ core.Context, _ any) core.Result {
	return core.Ok(nil)
}

// ServiceShutdown is the Wails lifecycle no-op.
func (s *AppWindowService) ServiceShutdown() core.Result { return core.Ok(nil) }

// OpenApp moves one application into its own native window. A Fail Result
// carries the reason to the renderer the way every other binding failure
// does, so a tear-off that cannot happen leaves the in-shell window alone
// instead of half-moving it.
func (s *AppWindowService) OpenApp(request AppWindowRequest) core.Result {
	if s == nil || s.core == nil {
		return core.Fail(core.E(
			"desktop.AppWindowService.OpenApp",
			"tear-off binding is not bound to a Core",
			nil,
		))
	}
	return openNativeAppWindow(s.core, request)
}

// DockApp moves one application back out of its own native window and into
// the shell. It is OpenApp mirrored, and it exists as a binding rather than as
// a bare event from the solo window for three reasons: the application is
// checked against the same whitelist a tear-off is, so an unrecognised
// identifier is refused instead of broadcast; the shell window is revealed
// before the request is announced, so the desktop is on screen by the time it
// adopts; and the renderer gets a Result, so it knows the hand-over happened
// before it closes the window holding the application.
//
// The shell — never this — writes the session. All Go does is say so.
func (s *AppWindowService) DockApp(request AppWindowRequest) core.Result {
	if s == nil || s.core == nil {
		return core.Fail(core.E(
			"desktop.AppWindowService.DockApp",
			"tear-off binding is not bound to a Core",
			nil,
		))
	}
	return dockNativeAppWindow(s.core, request)
}

// dockNativeAppWindow validates a dock-back request, reveals the shell, and
// puts the adoption on the WebView event bus for the shell to pick up.
func dockNativeAppWindow(c *core.Core, request AppWindowRequest) core.Result {
	if c == nil {
		return core.Fail(core.E("desktop.dockNativeAppWindow", "core is nil", nil))
	}
	pane, ok := dockPane(request.App, request.Pane)
	if !ok {
		return core.Fail(core.E(
			"desktop.dockNativeAppWindow",
			"no shell window for application: "+request.App,
			nil,
		))
	}
	if !gui.OpenWindow(c, mainWindowName) {
		return core.Fail(core.E(
			"desktop.dockNativeAppWindow",
			"reveal "+mainWindowName+" failed",
			nil,
		))
	}
	emitted := gui.EmitEvent(c, dockAppEvent, map[string]any{
		"app":  request.App,
		"pane": pane,
	})
	if !emitted.OK {
		return core.Fail(core.E(
			"desktop.dockNativeAppWindow",
			"announce "+request.App+" failed",
			emitted.Err(),
		))
	}
	return core.Ok(request.App)
}

// dockPane validates the application and the pane it is carrying home, under
// exactly the rules a tear-off is validated by: an application must be on the
// whitelist and a pane must be a single safe route segment.
func dockPane(appID, pane string) (string, bool) {
	if !tearOffAppAllowed(appID) {
		return "", false
	}
	if pane == "" {
		return "", true
	}
	if !paths.IsValidPluginCode(pane) {
		return "", false
	}
	return pane, true
}

// openNativeAppWindow opens or reveals the native window that hosts one
// application's solo route. A window pre-created by an earlier tear-off is
// warm and stays warm: it is re-pointed at the requested route and resized
// before it is restored, shown, and focused.
func openNativeAppWindow(c *core.Core, request AppWindowRequest) core.Result {
	if c == nil {
		return core.Fail(core.E("desktop.openNativeAppWindow", "core is nil", nil))
	}
	configSvc, _ := core.ServiceFor[*config.Service](c, "config")
	spec := nativeAppWindowSpec(request, configSvc)
	if spec == nil {
		return core.Fail(core.E(
			"desktop.openNativeAppWindow",
			"no native window route for application: "+request.App,
			nil,
		))
	}
	if !windowExists(c, spec.Name) {
		if !gui.OpenAdhocWindow(c, spec) {
			return core.Fail(core.E(
				"desktop.openNativeAppWindow",
				"open "+spec.Name+" failed",
				nil,
			))
		}
		return core.Ok(spec.Name)
	}

	ctx := core.Background()
	if r := c.Action("window.set_url").Run(ctx, core.NewOptions(
		core.Option{Key: "task", Value: guiwindow.TaskSetURL{Name: spec.Name, URL: spec.URL}},
	)); !r.OK {
		return core.Fail(core.E(
			"desktop.openNativeAppWindow",
			"route "+spec.Name+" failed",
			r.Err(),
		))
	}
	if request.Width > 0 && request.Height > 0 {
		c.Action("window.set_size").Run(ctx, core.NewOptions(
			core.Option{Key: "task", Value: guiwindow.TaskSetSize{
				Name:   spec.Name,
				Width:  spec.Width,
				Height: spec.Height,
			}},
		))
	}
	if spec.ShowDockIcon {
		c.Action("dock.show_icon").Run(ctx, core.NewOptions())
	}
	restore := c.Action("window.restore").Run(ctx, core.NewOptions(
		core.Option{Key: "task", Value: guiwindow.TaskRestore{Name: spec.Name}},
	))
	visible := c.Action("window.set_visibility").Run(ctx, core.NewOptions(
		core.Option{Key: "task", Value: guiwindow.TaskSetVisibility{Name: spec.Name, Visible: true}},
	))
	focus := c.Action("window.focus").Run(ctx, core.NewOptions(
		core.Option{Key: "task", Value: guiwindow.TaskFocus{Name: spec.Name}},
	))
	if !restore.OK || !visible.OK || !focus.OK {
		return core.Fail(core.E(
			"desktop.openNativeAppWindow",
			"reveal "+spec.Name+" failed",
			nil,
		))
	}
	return core.Ok(spec.Name)
}

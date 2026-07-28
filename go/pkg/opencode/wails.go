// SPDX-Licence-Identifier: EUPL-1.2

// Wails-bindable surface — exposes the opencode subsystem to the Angular
// frontend. The TS binding generator emits a `wailsservice.ts` under
// frontend/bindings/dappco.re/lthn/desktop/pkg/opencode/ that the
// integrations-window + fleet-window consume.
//
// Methods are thin wrappers around the Service — they return the
// canonical core.Result shape so the existing `unwrap` helper on the
// TS side handles fail-cases uniformly with the rest of the lthn
// surface.

package opencode

import (
	core "dappco.re/go"
)

// WailsService is the binding namespace exposed to JS.
type WailsService struct {
	svc *Service
}

// NewWailsService binds the Wails surface to an opencode Service.
//
// Usage example:
//
//	core.WithName("opencode-wails", opencode.NewWailsService(opencodeSvc))
func NewWailsService(svc *Service) *WailsService {
	return &WailsService{svc: svc}
}

// ServiceName labels the binding namespace exposed to JS — the TS
// generated client lives under bindings/.../opencode/.
func (w *WailsService) ServiceName() string { return "OpenCodeWails" }

// ServiceStartup satisfies the Wails Service lifecycle hook.
func (w *WailsService) ServiceStartup(_ core.Context, _ any) core.Result {
	return core.Ok(nil)
}

// ServiceShutdown satisfies the Wails Service lifecycle hook.
func (w *WailsService) ServiceShutdown() core.Result { return core.Ok(nil) }

// Sandbox lifecycle — frontend's Start/Stop/Status buttons call
// these directly. They delegate to the embedded Service which owns
// the in-process state.

// WStart spawns a sandbox with the named profile. Empty string =
// DefaultProfile.
//
// Usage example (TS):
//
//	const r = await OpenCodeWails.WStart("code-review")
//	const id = unwrap<string>(r, "")
func (w *WailsService) WStart(profile string) core.Result {
	if w == nil || w.svc == nil {
		return core.Fail(core.E("opencode.WStart", "service not bound", nil))
	}
	return w.svc.Start(profile)
}

// WStop stops + removes a sandbox by id.
func (w *WailsService) WStop(id string) core.Result {
	if w == nil || w.svc == nil {
		return core.Fail(core.E("opencode.WStop", "service not bound", nil))
	}
	return w.svc.Stop(id)
}

// WStatus returns the list of running sandboxes.
func (w *WailsService) WStatus() core.Result {
	if w == nil || w.svc == nil {
		return core.Fail(core.E("opencode.WStatus", "service not bound", nil))
	}
	return w.svc.Status()
}

// WInspect returns one sandbox's record.
func (w *WailsService) WInspect(id string) core.Result {
	if w == nil || w.svc == nil {
		return core.Fail(core.E("opencode.WInspect", "service not bound", nil))
	}
	return w.svc.Inspect(id)
}

// Profile CRUD — frontend's profile picker calls these.

// WListProfiles returns all stored profiles.
func (w *WailsService) WListProfiles() core.Result {
	if w == nil || w.svc == nil {
		return core.Fail(core.E("opencode.WListProfiles", "service not bound", nil))
	}
	return w.svc.ListProfiles()
}

// WGetProfile fetches one profile by name.
func (w *WailsService) WGetProfile(name string) core.Result {
	if w == nil || w.svc == nil {
		return core.Fail(core.E("opencode.WGetProfile", "service not bound", nil))
	}
	return w.svc.GetProfile(name)
}

// WSaveProfile upserts a profile. Frontend authoring + edit flows
// call this with the full Profile JSON.
func (w *WailsService) WSaveProfile(p Profile) core.Result {
	if w == nil || w.svc == nil {
		return core.Fail(core.E("opencode.WSaveProfile", "service not bound", nil))
	}
	return w.svc.SaveProfile(p)
}

// WDeleteProfile drops one profile by name. The "default" profile
// is protected — server returns an error if attempted.
func (w *WailsService) WDeleteProfile(name string) core.Result {
	if w == nil || w.svc == nil {
		return core.Fail(core.E("opencode.WDeleteProfile", "service not bound", nil))
	}
	return w.svc.DeleteProfile(name)
}

// WWebURL returns the direct-bind URL (with Basic auth embedded)
// for the named sandbox's opencode web UI. Frontend uses this to
// build buttons that copy / share the URL.
func (w *WailsService) WWebURL(id string) core.Result {
	if w == nil || w.svc == nil {
		return core.Fail(core.E("opencode.WWebURL", "service not bound", nil))
	}
	return w.svc.WebURL(id)
}

// WOpenWebWindow spawns an lthn Wails window with opencode's web
// UI loaded. Only works in GUI mode (lthn gui / lthn tray) — the
// window.open action isn't registered when running `lthn serve`.
func (w *WailsService) WOpenWebWindow(id string) core.Result {
	if w == nil || w.svc == nil {
		return core.Fail(core.E("opencode.WOpenWebWindow", "service not bound", nil))
	}
	return w.svc.OpenWebWindow(id)
}

// WImportFromHost runs the host-opencode import cycle: spawns
// `opencode serve` on a free port, drains /project + /provider,
// reads auth.json for credentials, and persists rows. Returns
// ImportSummary on success.
func (w *WailsService) WImportFromHost() core.Result {
	if w == nil || w.svc == nil {
		return core.Fail(core.E("opencode.WImportFromHost", "service not bound", nil))
	}
	return w.svc.ImportFromHost()
}

// WListImports returns every imported project, newest first.
// Used by the inbox UI to render the imported-project list.
func (w *WailsService) WListImports() core.Result {
	if w == nil || w.svc == nil {
		return core.Fail(core.E("opencode.WListImports", "service not bound", nil))
	}
	return w.svc.ListImports()
}

// ProviderView is the redacted shape returned to the WebView by
// WListImportedProviders. AuthKey is intentionally absent — the raw
// credential never crosses the Wails bridge. The runner reads the
// raw key Go-side via ListImportedProviders.
//
// JSON field names are camelCase to match the existing lthn binding
// convention (see WailsService.ts).
type ProviderView struct {
	// ID is "<source>:<provider_id>".
	ID string `json:"id"`
	// Source is the upstream client (e.g. SourceOpenCodeHost).
	Source string `json:"source"`
	// ProviderID is the upstream's own provider identifier.
	ProviderID string `json:"providerId"`
	// Name is the human-facing label.
	Name string `json:"name"`
	// AuthType is the credential shape ("apikey", "oauth", …).
	AuthType string `json:"authType"`
	// Present reports whether an AuthKey was stored for this provider.
	// True = "configured ✓". The raw key is never included.
	Present bool `json:"present"`
	// Masked is a partially-obscured form of the key for display
	// only (e.g. "sk-ant-…4f2a"). Empty when Present is false.
	Masked string `json:"masked"`
}

// maskProviderKey returns a UI-safe rendering of an arbitrary
// provider API key. It keeps a 6-char prefix and a 4-char suffix,
// replacing the middle with bullets. Short or empty keys fall back
// to empty string so the caller can treat "" as "not configured".
//
// Usage example:
//
//	maskProviderKey("sk-ant-api03-REDACTED4f2a")
//	// → "sk-ant-••••••4f2a"
func maskProviderKey(key string) string {
	const head = 6
	const tail = 4
	if len(key) <= head+tail {
		return ""
	}
	mid := len(key) - head - tail
	bullets := ""
	for i := 0; i < mid && i < 12; i++ { // cap bullet run at 12 for readability
		bullets += "•"
	}
	return key[:head] + bullets + key[len(key)-tail:]
}

// WListImportedProviders returns a redacted view of every imported
// provider. AuthKey is stripped — the WebView receives only enough
// to render "OpenAI: configured ✓" / "Anthropic: configured ✓".
// The runner reads raw keys Go-side via ListImportedProviders.
//
// Usage example (TS):
//
//	const r = await OpenCodeWails.WListImportedProviders()
//	const rows = unwrap<ProviderView[]>(r, [])
//	// rows[0].present → true; rows[0].masked → "sk-ant-••••••4f2a"
func (w *WailsService) WListImportedProviders() core.Result {
	if w == nil || w.svc == nil {
		return core.Fail(core.E("opencode.WListImportedProviders", "service not bound", nil))
	}
	r := w.svc.ListImportedProviders()
	if !r.OK {
		return r
	}
	rows, ok := r.Value.([]ImportedProvider)
	if !ok {
		return core.Fail(core.E("opencode.WListImportedProviders", "unexpected value type from ListImportedProviders", nil))
	}
	views := make([]ProviderView, len(rows))
	for i, p := range rows {
		views[i] = ProviderView{
			ID:         p.ID,
			Source:     p.Source,
			ProviderID: p.ProviderID,
			Name:       p.Name,
			AuthType:   p.AuthType,
			Present:    p.AuthKey != "",
			Masked:     maskProviderKey(p.AuthKey),
		}
	}
	return core.Ok(views)
}

// WUpgradeWithConsent pulls the configured image (lthn/dev:latest)
// and — when in.RestartSandboxes is true — restarts any running
// sandbox if the digest changed. UI button "Check for updates /
// Upgrade" calls this with UpgradeInput{ConfirmedByUser: true}
// after the user accepts the supply-chain warning dialog. Returns
// UpgradeResult in Value when successful.
//
// Per Cerberus #22 MED-2 / Mantis #1619 + Mantis #1623 thread-through:
// UpgradeInput.ConfirmedByUser MUST be true or the underlying
// Service.UpgradeWithConsent refuses with
// "upgrade.requires_confirmation" — no network call, no side
// effects. A zero UpgradeInput{} therefore reaches the gate and
// fails closed (matching the legacy Upgrade() fail-closed contract).
//
// Usage example (TS):
//
//	const r = await OpenCodeWails.WUpgradeWithConsent({
//	  confirmed_by_user: true,
//	  restart_sandboxes: false,
//	})
//	if (!r.OK) { /* dialog: "Please confirm upgrade" or substrate error */ }
func (w *WailsService) WUpgradeWithConsent(in UpgradeInput) core.Result {
	if w == nil || w.svc == nil {
		return core.Fail(core.E("opencode.WUpgradeWithConsent", "service not bound", nil))
	}
	return w.svc.UpgradeWithConsent(in)
}

// WIsStudioInstalled reports whether OpenCode's native desktop
// app is installed on the host. Frontend uses this to decide
// whether to render the "Open Studio" button.
func (w *WailsService) WIsStudioInstalled() core.Result {
	if w == nil || w.svc == nil {
		return core.Ok(false)
	}
	return core.Ok(w.svc.IsStudioInstalled())
}

// WOpenStudio launches the host's OpenCode native app. Fails when
// the app isn't installed — frontend gates on WIsStudioInstalled.
func (w *WailsService) WOpenStudio() core.Result {
	if w == nil || w.svc == nil {
		return core.Fail(core.E("opencode.WOpenStudio", "service not bound", nil))
	}
	return w.svc.OpenStudio()
}

// WOpenTUI spawns `<runtime> exec -it <container> opencode` in
// the user's default terminal — macOS Terminal.app via osascript,
// Linux $TERMINAL / x-terminal-emulator / gnome-terminal etc.,
// Windows wt.exe or cmd.exe. Frontend's Integrations card "Open
// TUI" button calls this when sandbox state == ready.
func (w *WailsService) WOpenTUI(id string) core.Result {
	if w == nil || w.svc == nil {
		return core.Fail(core.E("opencode.WOpenTUI", "service not bound", nil))
	}
	return w.svc.OpenTUI(id)
}

// WOpenTUIInApp spawns the opencode TUI (`opencode attach <url>`) as a PTY in
// lthn's own terminal pool and returns the session ID, so the FE can attach an
// in-app terminal tab to the running agent instead of opening an external
// terminal window. Integrations card "Open in-app" button.
func (w *WailsService) WOpenTUIInApp(id string) core.Result {
	if w == nil || w.svc == nil {
		return core.Fail(core.E("opencode.WOpenTUIInApp", "service not bound", nil))
	}
	return w.svc.OpenTUIInApp(id)
}

// WEnable persists `opencode.serve.enabled = true` and spawns a
// sandbox if none is running. Idempotent. Empty profile = default.
// Frontend uses this on the integrations card as a "remember my
// preference" alternative to one-shot Start.
func (w *WailsService) WEnable(profile string) core.Result {
	if w == nil || w.svc == nil {
		return core.Fail(core.E("opencode.WEnable", "service not bound", nil))
	}
	return w.svc.Enable(profile)
}

// WDisable persists the disabled flag + stops any running sandboxes.
func (w *WailsService) WDisable() core.Result {
	if w == nil || w.svc == nil {
		return core.Fail(core.E("opencode.WDisable", "service not bound", nil))
	}
	return w.svc.Disable()
}

// WIsEnabled returns the persisted enabled flag. Useful for the
// frontend to render the toggle's initial state without waiting
// for WStatus to return.
func (w *WailsService) WIsEnabled() core.Result {
	if w == nil || w.svc == nil {
		return core.Ok(false)
	}
	return core.Ok(w.svc.IsEnabled())
}

// WProviderList returns opencode-serve's /provider response for a
// running sandbox. The Fleet → Agents window consumes this to
// render the "OpenCode-routed providers" cards. Returned as a raw
// JSON string — caller parses to the opencode shape.
func (w *WailsService) WProviderList(id string) core.Result {
	if w == nil || w.svc == nil {
		return core.Fail(core.E("opencode.WProviderList", "service not bound", nil))
	}
	return w.svc.ProviderList(id)
}

// WMergeHostConfig merges the named profile's provider block into
// the user's host-side ~/.config/opencode/opencode.json. Returns
// HostConfigConflict (in Result.Code()) when provider.lthn already
// exists with a different baseURL and force=false — the frontend
// prompts the user before retrying with force=true.
//
// Usage example (TS):
//
//	const r = await OpenCodeWails.WMergeHostConfig({ profile: "default" })
//	if (r.code === "opencode.host-config.conflict") { /* prompt user */ }
func (w *WailsService) WMergeHostConfig(opts MergeHostConfigOptions) core.Result {
	if w == nil || w.svc == nil {
		return core.Fail(core.E("opencode.WMergeHostConfig", "service not bound", nil))
	}
	return w.svc.MergeHostConfig(opts)
}

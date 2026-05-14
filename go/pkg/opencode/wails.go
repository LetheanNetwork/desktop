// SPDX-Licence-Identifier: EUPL-1.2

// Wails-bindable surface — exposes the opencode subsystem to the Lit
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
	"context"

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
func (w *WailsService) ServiceStartup(_ context.Context, _ any) core.Result {
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

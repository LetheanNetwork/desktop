// SPDX-Licence-Identifier: EUPL-1.2

package api

import (
	core "dappco.re/go"
	coreapi "dappco.re/go/api"

	"dappco.re/lthn/desktop/pkg/runner"
)

// Register wires all lthn-owned RouteGroups onto the api.Service's
// Engine. Idempotent — registering a group whose BasePath/handlers
// already exist on the gin engine causes that pattern to be shadowed
// by the second registration (gin allows route overrides).
//
// Returns Ok with nil value when:
//
//   - The api Service isn't registered on this Core (headless build,
//     no HTTP surface needed). Group registration is silently skipped.
//   - All groups were registered successfully.
//
// Returns Fail when r is nil — the RunnerGroup would otherwise panic on
// first request.
//
// Called from pkg/desktop.mountSubsystems and (eventually) cmdServe to
// keep group registration centralised. Do NOT call from individual
// handlers — registration is a boot-time concern.
//
// Usage example:
//
//	if rr := api.Register(c, runner); !rr.OK {
//	    return rr
//	}
func Register(c *core.Core, r *runner.Service) core.Result {
	if c == nil {
		return core.Fail(core.E("api.Register", "core is nil", nil))
	}
	if r == nil {
		return core.Fail(core.E("api.Register", "runner is nil", nil))
	}

	apiSvc, ok := core.ServiceFor[*coreapi.Service](c, "api")
	if !ok || apiSvc == nil || apiSvc.Engine == nil {
		// No api service on this Core — that's fine, the binary just
		// won't expose the HTTP gateway. The Wails WebView keeps
		// talking to Services via in-process bindings.
		return core.Ok(nil)
	}

	apiSvc.Engine.Register(NewRunnerGroup(r))
	return core.Ok(nil)
}

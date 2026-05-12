// SPDX-Licence-Identifier: EUPL-1.2

// Command lthn — Lethean Desktop's tray-rooted binary.
//
//	lthn                 # boot the tray + popover (default)
//	lthn --window chat   # open a specific window (future: spawn-from-CLI)
//
// The tray IS the process. Closing windows does NOT quit the app —
// the NSStatusItem (system tray entry) is the lifetime anchor.
// See plans/project/lthn/desktop/RFC.first-release.md §1.3.
package main

func main() {
	// Boot order (per the spec):
	//  1. core.New()         — create the Core container with services
	//  2. tray.Register(c)   — register NSStatusItem + popover
	//  3. runner.Register(c) — register go-mlx runner service
	//  4. telemetry.Register(c) — register powermetrics/IOReport service
	//  5. core/gui app boot  — wire the Wails app, mount frontend
	//  6. block on signal    — clean shutdown on SIGINT/SIGTERM
	//
	// Implementation pending wiring against:
	//   - dappco.re/go/core (primitives)
	//   - dappco.re/go/gui  (window/tray/app)
	//   - dappco.re/go/mlx  (inference)
	//   - dappco.re/go/store (KV)
	//   - dappco.re/go/inference/state (KV portable state primitive)
}

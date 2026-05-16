// SPDX-Licence-Identifier: EUPL-1.2

// Wails3 Service shape for the firstlaunch package — wraps Detect
// for the WebView. Bound by application.NewService(firstlaunch.
// NewWailsService()) in pkg/desktop/desktop.go.

package firstlaunch

import (
	"runtime"

	core "dappco.re/go"
	lthn "dappco.re/lthn/desktop"
	"dappco.re/lthn/desktop/pkg/paths"
)

// BuildInfo bundles the runtime + build identification the About
// panel renders. JSON tags use snake_case so the generated TS
// binding lands idiomatic.
type BuildInfo struct {
	Version   string `json:"version"`
	GoVersion string `json:"go_version"`
	GoOS      string `json:"goos"`
	GoArch    string `json:"goarch"`
	NumCPU    int    `json:"num_cpu"`
}

// LetheanPaths bundles the canonical ~/Lethean/ filesystem layout the
// welcome wizard surfaces during onboarding. Mirrors the no-hidden-
// user-bloat rule — visible directories, not dot-folders. See
// design_no_hidden_user_bloat memory.
type LetheanPaths struct {
	Root       string `json:"root"`
	ConfDir    string `json:"conf_dir"`
	DataDir    string `json:"data_dir"`
	WalletsDir string `json:"wallets_dir"`
	ModelsDir  string `json:"models_dir"`
	ConfigFile string `json:"config_file"`
}

type WailsService struct{}

func NewWailsService() *WailsService { return &WailsService{} }

func (s *WailsService) ServiceName() string { return "FirstLaunch" }
func (s *WailsService) ServiceStartup(_ core.Context, _ any) core.Result {
	return core.Ok(nil)
}
func (s *WailsService) ServiceShutdown() core.Result { return core.Ok(nil) }

// Detect inspects on-disk state and reports whether this is a
// fresh install.
func (s *WailsService) Detect() core.Result {
	r := Detect(nil)
	if !r.OK {
		return core.Fail(core.E("firstlaunch.WailsService.Detect", "detect first launch state failed", r.Value.(error)))
	}
	state, _ := r.Value.(State)
	return core.Ok(state)
}

// Build returns version + runtime metadata for the About panel.
// Pure-Go reflection of the running process; no IO, no allocations
// beyond the struct.
//
// Cerberus Mantis #1445 — the returned BuildInfo includes
// {Version, GoVersion, GoOS, GoArch, NumCPU} = a passive host
// fingerprint. Safe today because this method is Wails-only
// (in-process IPC; only the user's own WebView reaches it). DO NOT
// expose this via a gateway scope (post-#1421 plugin tier) without
// either (a) gating behind a `system.fingerprint:read` permission
// the user grants per-plugin, OR (b) returning a sanitized subset
// (e.g. just Version, no GoOS/GoArch/NumCPU). Plugins should not
// silently learn the user's runtime fingerprint.
//
// Usage example (TS):
//
//	import { Build } from "@desktop/firstlaunch/wailsservice";
//	const b = await Build();
//	console.log(b.version, b.go_version, b.goos, b.goarch);
func (s *WailsService) Build() core.Result {
	return core.Ok(BuildInfo{
		Version:   core.TrimPrefix(lthn.Version, "v"),
		GoVersion: runtime.Version(),
		GoOS:      runtime.GOOS,
		GoArch:    runtime.GOARCH,
		NumCPU:    runtime.NumCPU(),
	})
}

// Paths returns the canonical ~/Lethean/ layout. Driven by
// pkg/paths.* so the welcome wizard, the model browser, the
// settings General panel, and any future onboarding surface
// stay aligned with the same source of truth.
//
// Cerberus Mantis #1445 — the returned LetheanPaths struct contains
// concrete absolute paths with the user's home directory expanded
// (`/Users/<name>/Lethean/...` on macOS, `/home/<name>/...` on
// Linux). That's a USERNAME LEAK. Safe today because this is Wails-
// only (in-process; the WebView already runs as the user). If this
// gets exposed via a gateway scope (post-#1421 plugin tier), MUST
// be sanitized first — strip the home prefix, return `~/Lethean/...`
// form, or only return the suffix the plugin actually needs (e.g.
// "models" subdir name without the full path). Plugins should not
// silently learn the user's OS username via path strings.
//
// Usage example (TS):
//
//	import { Paths } from "@desktop/firstlaunch/wailsservice";
//	const p = await Paths();
//	console.log(p.models_dir);   // "/Users/<name>/Lethean/conf/models"
func (s *WailsService) Paths() core.Result {
	out := LetheanPaths{}
	if r := paths.Root(); r.OK {
		out.Root, _ = r.Value.(string)
	}
	if r := paths.ConfDir(); r.OK {
		out.ConfDir, _ = r.Value.(string)
	}
	if r := paths.DataDir(); r.OK {
		out.DataDir, _ = r.Value.(string)
	}
	if r := paths.WalletsDir(); r.OK {
		out.WalletsDir, _ = r.Value.(string)
	}
	if r := paths.ModelsDir(); r.OK {
		out.ModelsDir, _ = r.Value.(string)
	}
	if r := paths.ConfigFile(); r.OK {
		out.ConfigFile, _ = r.Value.(string)
	}
	return core.Ok(out)
}

// SetModelsDir writes a user-chosen models directory to the override
// config. Path must be absolute. After it lands, paths.ModelsDir()
// returns the chosen path. Returns the resolved path on success so
// the WebView can refresh its display without a separate read.
//
// Usage example (TS):
//
//	import { SetModelsDir } from "@desktop/firstlaunch/wailsservice";
//	const dir = await SetModelsDir("/Volumes/Vault/lthn-models");
func (s *WailsService) SetModelsDir(path string) core.Result {
	r := paths.SetModelsDirOverride(path)
	if !r.OK {
		return core.Fail(core.E("firstlaunch.WailsService.SetModelsDir", "set override failed", r.Value.(error)))
	}
	return r
}

// ClearModelsDir wipes the models-dir override so paths.ModelsDir()
// falls back to the canonical default. Idempotent.
//
// Usage example (TS):
//
//	import { ClearModelsDir } from "@desktop/firstlaunch/wailsservice";
//	await ClearModelsDir();
func (s *WailsService) ClearModelsDir() core.Result {
	r := paths.ClearModelsDirOverride()
	if !r.OK {
		return core.Fail(core.E("firstlaunch.WailsService.ClearModelsDir", "clear override failed", r.Value.(error)))
	}
	return r
}

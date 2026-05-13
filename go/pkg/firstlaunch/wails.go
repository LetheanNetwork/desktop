// SPDX-Licence-Identifier: EUPL-1.2

// Wails3 Service shape for the firstlaunch package — wraps Detect
// for the WebView. Bound by application.NewService(firstlaunch.
// NewWailsService()) in pkg/desktop/desktop.go.

package firstlaunch

import (
	"context"
	"errors"

	"dappco.re/lthn/desktop/pkg/paths"
	"github.com/wailsapp/wails/v3/pkg/application"
)

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
func (s *WailsService) ServiceStartup(_ context.Context, _ application.ServiceOptions) error {
	return nil
}
func (s *WailsService) ServiceShutdown() error { return nil }

// Detect inspects on-disk state and reports whether this is a
// fresh install.
func (s *WailsService) Detect() (State, error) {
	r := Detect(nil)
	if !r.OK {
		return State{}, errors.New(r.Error())
	}
	state, _ := r.Value.(State)
	return state, nil
}

// Paths returns the canonical ~/Lethean/ layout. Driven by
// pkg/paths.* so the welcome wizard, the model browser, the
// settings General panel, and any future onboarding surface
// stay aligned with the same source of truth.
//
// Usage example (TS):
//
//	import { Paths } from "@desktop/firstlaunch/wailsservice";
//	const p = await Paths();
//	console.log(p.models_dir);   // "/Users/<name>/Lethean/conf/models"
func (s *WailsService) Paths() (LetheanPaths, error) {
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
	return out, nil
}

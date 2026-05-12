// SPDX-Licence-Identifier: EUPL-1.2

// Package firstlaunch reports whether this is a fresh install that
// should trigger the setup wizard. Path-only — does NOT boot Core,
// because that would create ~/Lethean/data/lthn.db before the path
// check and defeat the question.
//
// A launch is "first" when none of these exist:
//   - ~/Lethean/conf/lthn.yaml          (config not seeded)
//   - ~/Lethean/data/lthn.db            (state not seeded)
//   - any route configured under `routes.*` in the yaml file
//
// Usage example:
//
//	r := firstlaunch.Detect(nil)
//	state := r.Value.(firstlaunch.State)
//	if state.Fresh { /* wizard */ }
package firstlaunch

import (
	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/paths"
	"gopkg.in/yaml.v3"
)

// State summarises the conditions firstlaunch checks.
type State struct {
	Fresh         bool `json:"fresh"`
	HasConfigFile bool `json:"has_config_file"`
	HasStateDB    bool `json:"has_state_db"`
	HasRoutes     bool `json:"has_routes"`
}

// Detect inspects the on-disk state of ~/Lethean/. The c argument is
// accepted for API symmetry with other Detect-style helpers but is
// unused — see package doc on why.
//
// Usage example:
//
//	r := firstlaunch.Detect(nil)
//	state := r.Value.(firstlaunch.State)
func Detect(_ *core.Core) core.Result {
	state := State{}

	cfgPath := ""
	if cfg := paths.ConfigFile(); cfg.OK {
		cfgPath = cfg.Value.(string)
		state.HasConfigFile = pathExists(cfgPath)
	}
	if db := paths.StoreDB(); db.OK {
		state.HasStateDB = pathExists(db.Value.(string))
	}

	if state.HasConfigFile && cfgPath != "" {
		state.HasRoutes = yamlHasRoutes(cfgPath)
	}

	state.Fresh = !state.HasConfigFile && !state.HasStateDB && !state.HasRoutes
	return core.Ok(state)
}

func pathExists(p string) bool {
	r := core.Stat(p)
	return r.OK
}

// yamlHasRoutes parses ~/Lethean/conf/lthn.yaml and returns true when
// the top-level `routes:` map exists and has at least one entry.
// Treats parse errors as "no routes" so the wizard surface stays
// useful even on partial/corrupt config files.
func yamlHasRoutes(p string) bool {
	r := core.ReadFile(p)
	if !r.OK {
		return false
	}
	body, ok := r.Value.([]byte)
	if !ok {
		return false
	}
	var raw struct {
		Routes map[string]any `yaml:"routes"`
	}
	if err := yaml.Unmarshal(body, &raw); err != nil {
		return false
	}
	return len(raw.Routes) > 0
}

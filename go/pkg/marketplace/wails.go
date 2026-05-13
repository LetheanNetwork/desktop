// SPDX-Licence-Identifier: EUPL-1.2

// Wails surface for the marketplace service. Methods are bound on
// the *Service receiver and exposed to the frontend via wails3
// generate bindings — the TS shape lands at
// frontend/bindings/dappco.re/lthn/desktop/pkg/marketplace/service.

package marketplace

import (
	core "dappco.re/go"
)

// SearchOutput is the shape returned to the Lit window.
type SearchOutput struct {
	Packages []Package `json:"packages"`
}

// Search browses the catalogue. Empty query returns everything;
// query + category narrow the result set.
func (s *Service) Search(query, category string) (SearchOutput, error) {
	return SearchOutput{Packages: search(query, category)}, nil
}

// InfoOutput wraps a single Package for the rich detail view.
type InfoOutput struct {
	Package Package `json:"package"`
}

// Info returns the full record for one plugin by code.
func (s *Service) Info(code string) (InfoOutput, error) {
	if core.Trim(code) == "" {
		return InfoOutput{}, core.E("marketplace.Info", "code is required", nil)
	}
	pkg, ok := findByCode(code)
	if !ok {
		return InfoOutput{}, core.E("marketplace.Info", "plugin not found: "+code, nil)
	}
	return InfoOutput{Package: pkg}, nil
}

// InstalledOutput is the shape returned to the Lit window — the
// list of currently-installed plugins. Empty until the plugin
// runtime lands and starts tracking state on disk.
type InstalledOutput struct {
	Packages []InstalledPackage `json:"packages"`
}

// Installed enumerates plugins the user has installed via the
// runtime. Returns empty until the plugin host is wired —
// installed state is owned by the runtime, not this catalogue.
func (s *Service) Installed() (InstalledOutput, error) {
	return InstalledOutput{Packages: []InstalledPackage{}}, nil
}

// Install requests the plugin host to fetch + spawn the named
// plugin. Today returns "plugin host not running" because the
// runtime hasn't landed yet; the UI surfaces this as an info
// banner rather than an error.
func (s *Service) Install(code string) error {
	if core.Trim(code) == "" {
		return core.E("marketplace.Install", "code is required", nil)
	}
	if _, ok := findByCode(code); !ok {
		return core.E("marketplace.Install", "plugin not found: "+code, nil)
	}
	return core.E("marketplace.Install",
		"plugin host not running — Install will activate when the "+
			"plugin runtime (binary + --namespace + reverse-proxy) lands", nil)
}

// Remove asks the plugin host to stop + clean up the named
// plugin. Returns the same not-yet-running error as Install
// until the runtime is wired.
func (s *Service) Remove(code string) error {
	if core.Trim(code) == "" {
		return core.E("marketplace.Remove", "code is required", nil)
	}
	return core.E("marketplace.Remove",
		"plugin host not running — Remove will activate when the "+
			"plugin runtime (binary + --namespace + reverse-proxy) lands", nil)
}

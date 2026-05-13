// SPDX-Licence-Identifier: EUPL-1.2

// Wails surface for the marketplace service. Methods are bound on
// the *Service receiver and exposed to the frontend via wails3
// generate bindings — the TS shape lands at
// frontend/bindings/dappco.re/lthn/desktop/pkg/marketplace/service.

package marketplace

import (
	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/plugin"
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
// plugin host. Empty until at least one Install completes; the
// host's on-disk scan keeps this consistent with what's actually
// present in ~/Lethean/conf/plugins/.
func (s *Service) Installed() (InstalledOutput, error) {
	host, ok := core.ServiceFor[*plugin.Service](s.core, "plugin")
	if !ok || host == nil {
		return InstalledOutput{Packages: []InstalledPackage{}}, nil
	}
	listing, err := host.List()
	if err != nil {
		return InstalledOutput{Packages: []InstalledPackage{}}, err
	}
	out := make([]InstalledPackage, 0, len(listing.Plugins))
	for _, p := range listing.Plugins {
		entry := InstalledPackage{
			Code:    p.Code,
			Name:    p.Name,
			Version: p.Version,
		}
		if p.Status.Port > 0 {
			entry.EntryPoint = "/v1/api/plugin/" + p.Namespace + "/"
		}
		out = append(out, entry)
	}
	return InstalledOutput{Packages: out}, nil
}

// Install resolves a catalogue entry, hands the manifest to the
// plugin host, and lets it fetch + write + start the binary. The
// plugin host is wired via pkg/plugin and registered at boot in
// pkg/desktop; if it's missing (degenerate dev build) the call
// surfaces a clean error rather than a nil-pointer panic.
func (s *Service) Install(code string) error {
	if core.Trim(code) == "" {
		return core.E("marketplace.Install", "code is required", nil)
	}
	pkg, ok := findByCode(code)
	if !ok {
		return core.E("marketplace.Install", "plugin not found in catalogue: "+code, nil)
	}
	host, ok := core.ServiceFor[*plugin.Service](s.core, "plugin")
	if !ok || host == nil {
		return core.E("marketplace.Install",
			"plugin host not registered — Install requires pkg/plugin to be wired", nil)
	}
	_, err := host.Install(plugin.InstallInput{
		Code:      pkg.Code,
		Name:      pkg.Name,
		Version:   pkg.Version,
		Namespace: pkg.Code,
		BinaryURL: pkg.Entrypoint, // catalogue points at release asset
	})
	return err
}

// Remove asks the plugin host to stop + clean up the named
// plugin. Same host-discovery dance as Install.
func (s *Service) Remove(code string) error {
	if core.Trim(code) == "" {
		return core.E("marketplace.Remove", "code is required", nil)
	}
	host, ok := core.ServiceFor[*plugin.Service](s.core, "plugin")
	if !ok || host == nil {
		return core.E("marketplace.Remove",
			"plugin host not registered — Remove requires pkg/plugin to be wired", nil)
	}
	return host.Remove(code)
}

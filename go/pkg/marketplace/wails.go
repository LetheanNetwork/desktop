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

const (
	codeRequiredMessage = "code is required"
	installOp           = "marketplace.Install"
)

// SearchOutput is the shape returned to the Angular window.
type SearchOutput struct {
	Packages []Package `json:"packages"`
}

// Search browses the catalogue. Empty query returns everything;
// query + category narrow the result set.
func (s *Service) Search(query, category string) core.Result {
	return core.Ok(SearchOutput{Packages: search(query, category)})
}

// InfoOutput wraps a single Package for the rich detail view.
type InfoOutput struct {
	Package Package `json:"package"`
}

// Info returns the full record for one plugin by code.
func (s *Service) Info(code string) core.Result {
	if core.Trim(code) == "" {
		return core.Fail(core.E("marketplace.Info", codeRequiredMessage, nil))
	}
	pkg, ok := findByCode(code)
	if !ok {
		return core.Fail(core.E("marketplace.Info", "plugin not found: "+code, nil))
	}
	return core.Ok(InfoOutput{Package: pkg})
}

// InstalledOutput is the shape returned to the Angular window — the
// list of currently-installed plugins. Empty until the plugin
// runtime lands and starts tracking state on disk.
type InstalledOutput struct {
	Packages []InstalledPackage `json:"packages"`
}

// Installed enumerates plugins the user has installed via the
// plugin host. Empty until at least one Install completes; the
// host's on-disk scan keeps this consistent with what's actually
// present in ~/Lethean/conf/plugins/.
func (s *Service) Installed() core.Result {
	host, ok := core.ServiceFor[*plugin.Service](s.core, "plugin")
	if !ok || host == nil {
		return core.Ok(InstalledOutput{Packages: []InstalledPackage{}})
	}
	listingR := host.List()
	if !listingR.OK {
		return listingR
	}
	listing := listingR.Value.(plugin.ListOutput)
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
	return core.Ok(InstalledOutput{Packages: out})
}

// InstallPlugin resolves a binary-plugin catalogue entry, hands the
// manifest to the plugin host, and lets it fetch + write + start the
// binary. For lthn-vm bundle installs use Service.Install instead.
func (s *Service) InstallPlugin(code string) core.Result {
	if core.Trim(code) == "" {
		return core.Fail(core.E(installOp, codeRequiredMessage, nil))
	}
	pkg, ok := findByCode(code)
	if !ok {
		return core.Fail(core.E(installOp, "plugin not found in catalogue: "+code, nil))
	}
	host, ok := core.ServiceFor[*plugin.Service](s.core, "plugin")
	if !ok || host == nil {
		return core.Fail(core.E(installOp,
			"plugin host not registered — Install requires pkg/plugin to be wired", nil))
	}
	r := host.Install(plugin.InstallInput{
		Code:      pkg.Code,
		Name:      pkg.Name,
		Version:   pkg.Version,
		Namespace: pkg.Code,
		BinaryURL: pkg.Entrypoint, // catalogue points at release asset
	})
	if !r.OK {
		return r
	}
	return core.Ok(nil)
}

// Remove asks the plugin host to stop + clean up the named
// plugin. Same host-discovery dance as Install.
func (s *Service) Remove(code string) core.Result {
	if core.Trim(code) == "" {
		return core.Fail(core.E("marketplace.Remove", codeRequiredMessage, nil))
	}
	host, ok := core.ServiceFor[*plugin.Service](s.core, "plugin")
	if !ok || host == nil {
		return core.Fail(core.E("marketplace.Remove",
			"plugin host not registered — Remove requires pkg/plugin to be wired", nil))
	}
	return host.Remove(code)
}

// SPDX-License-Identifier: EUPL-1.2

package files

import core "dappco.re/go"

// ResolveLocalWorkspace converts an opaque Files mount and provider-relative
// directory into a trusted host path for an in-process local capability such
// as Terminal. It is a free Go function so the Wails-bound Files service never
// exposes provider roots to the renderer.
//
// Usage example:
//
//	result := files.ResolveLocalWorkspace(service, "projects", "desktop")
func ResolveLocalWorkspace(
	service *Service,
	mountID string,
	relativePath string,
) core.Result {
	if service == nil {
		return core.Fail(newFailure(
			ErrorProviderUnavailable,
			mountID,
			"",
			"The Files service is unavailable.",
			nil,
		))
	}
	mount, err := service.mount(mountID)
	if err != nil {
		return core.Fail(err)
	}
	if mount.Kind != "local" ||
		mount.LocalRoot == "" ||
		!mount.ContainmentAudited ||
		mount.Medium == nil {
		return core.Fail(newFailure(
			ErrorBoundaryRejected,
			mountID,
			"",
			"The mount is not an audited local workspace.",
			nil,
		))
	}
	clean, err := normaliseRelativePath(relativePath, true)
	if err != nil {
		return core.Fail(withAddress(err, mountID, relativePath))
	}
	info, err := mount.Medium.Stat(clean)
	if err != nil {
		return core.Fail(providerFailure(
			"ResolveLocalWorkspace",
			mountID,
			clean,
			err,
		))
	}
	if !info.IsDir() {
		return core.Fail(newFailure(
			ErrorUnsupportedEntry,
			mountID,
			clean,
			"Terminal workspaces must be directories.",
			nil,
		))
	}
	if clean == "" {
		return core.Ok(mount.LocalRoot)
	}
	return core.Ok(core.PathJoin(mount.LocalRoot, clean))
}

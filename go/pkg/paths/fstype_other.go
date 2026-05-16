// SPDX-Licence-Identifier: EUPL-1.2

//go:build !darwin && !linux

// fstype_other.go — Fallback statfs reader for platforms we have not
// explicitly tested (windows, freebsd, etc). Returns "unknown" so the
// policy table degrades to fail-soft (warn-but-permit) rather than
// fail-loud on platforms where the lock primitive's behaviour over
// the local filesystem has not been audited.

package paths

import core "dappco.re/go"

// statfsType returns the "unknown" sentinel on unsupported platforms.
// The cross-platform IsNetworkFS check still applies; on those
// platforms the user can opt-in to NFS rejection by setting the
// override via SetRootFSTypeForTest, but production code is degraded
// to warn-only.
func statfsType(_ string) core.Result {
	if rootFSTypeOverride != "" {
		return core.Ok(rootFSTypeOverride)
	}
	return core.Ok("unknown")
}

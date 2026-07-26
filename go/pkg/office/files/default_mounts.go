// SPDX-License-Identifier: EUPL-1.2

package files

import core "dappco.re/go"

// DefaultOptions is a fail-closed composition seam until the audited local
// Medium release is pinned and the application I/O service is available.
func DefaultOptions(_ *core.Core) core.Result {
	return core.Fail(newFailure(
		ErrorInvalidInput,
		"",
		"",
		"default Files mounts are not composed",
		nil,
	))
}

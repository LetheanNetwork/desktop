// SPDX-Licence-Identifier: EUPL-1.2

// export_test.go exposes internal helpers for use by package-external
// tests. Only compiled when running go test — not part of the
// production binary. Mirror of the pattern in pkg/incidents and
// pkg/runbooks.

package audience

import core "dappco.re/go"

// WriteSegmentExported exposes writeSegment for service-tier tests
// that need to bypass the wails IsValidID gate and exercise the
// WithinDir belt directly (Mantis #1607).
//
// Usage example:
//
//	res := audience.WriteSegmentExported(dir, seg, 0)
func WriteSegmentExported(dir string, seg Segment, ifVersion int) core.Result {
	return writeSegment(dir, seg, ifVersion)
}

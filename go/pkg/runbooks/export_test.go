// SPDX-Licence-Identifier: EUPL-1.2

// export_test.go exposes internal helpers for use by package-external
// tests. Only compiled when running go test.

package runbooks

import core "dappco.re/go"

// ParseRecordExported exposes parseRecord for external test packages.
func ParseRecordExported(slug string, raw []byte) (RunbookRecord, error) {
	return parseRecord(slug, raw)
}

// MarshalRecordExported exposes marshalRecord for external test packages.
func MarshalRecordExported(r RunbookRecord) ([]byte, error) {
	return marshalRecord(r)
}

// ComputeHealthExported exposes computeHealth for external test packages.
func ComputeHealthExported(r RunbookRecord, now core.Time) string {
	return computeHealth(r, now)
}

// RelativeTimeExported exposes relativeTime for external test packages.
func RelativeTimeExported(t, now core.Time) string {
	return relativeTime(t, now)
}

// MatchSearchExported exposes matchSearch for external test packages.
func MatchSearchExported(r RunbookRecord, q string) bool {
	return matchSearch(r, q)
}

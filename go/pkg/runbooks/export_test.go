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

// HasRecordSuffixExported exposes hasRecordSuffix for external test
// packages — covers both the legacy ".md" and encrypted ".lthn"
// record-file suffixes recognised by countMdFiles' seed short-circuit.
func HasRecordSuffixExported(nm string) bool {
	return hasRecordSuffix(nm)
}

// WriteRecordExported exposes writeRecord for service-tier tests that
// need to bypass the wails IsValidID gate and exercise the WithinDir
// belt directly (Mantis #1607).
//
// Usage example:
//
//	res := runbooks.WriteRecordExported(rec, dir, 0)
func WriteRecordExported(r RunbookRecord, dirPath string, ifVersion int) core.Result {
	return writeRecord(r, dirPath, ifVersion)
}

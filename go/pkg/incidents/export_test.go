// SPDX-Licence-Identifier: EUPL-1.2

// export_test.go exposes internal helpers for use by package-external
// tests. Only compiled when running go test — not part of the
// production binary. Follows the go test package naming convention
// (package incidents, not incidents_test) so it can reach unexported
// symbols.

package incidents

import core "dappco.re/go"

// ParseRecordExported exposes parseRecord for external test packages.
//
// Usage example:
//
//	rec, err := incidents.ParseRecordExported(raw)
func ParseRecordExported(raw []byte) (IncidentRecord, error) {
	return parseRecord(raw)
}

// MarshalRecordExported exposes marshalRecord for external test packages.
//
// Usage example:
//
//	raw, err := incidents.MarshalRecordExported(rec)
func MarshalRecordExported(r IncidentRecord) ([]byte, error) {
	return marshalRecord(r)
}

// RelativeTimeExported exposes relativeTime for external test packages.
//
// Usage example:
//
//	s := incidents.RelativeTimeExported(past, now)
func RelativeTimeExported(t, now core.Time) string {
	return relativeTime(t, now)
}

// DurStringExported exposes durString for external test packages.
//
// Usage example:
//
//	s := incidents.DurStringExported(42) // "42 min"
func DurStringExported(minutes int) string {
	return durString(minutes)
}

// LoadOneExported exposes loadOne for service-tier tests that need to
// bypass the wails IsValidID gate and exercise the WithinDir belt
// directly (Mantis #1607). Tests assert paths.escape / paths.invalid_id
// at the service tier rather than only at the wails tier.
//
// Stage E.D.B.2: loadOne is a *Service method (the at-rest path
// resolves the writer against the live SessionGate). For
// traversal-only tests no gate is needed — IsValidID fires first and
// the at-rest branch is never reached. The shim allocates a fresh
// gate-less Service so the call shape stays one-line.
//
// Usage example:
//
//	_, _, err := incidents.LoadOneExported("../../wallets/x")
func LoadOneExported(id string) (IncidentRecord, string, error) {
	return (&Service{}).loadOne(id)
}

// WriteRecordExported exposes writeRecord for service-tier tests.
//
// Stage E.D.B.2: writeRecord is a *Service method (the at-rest path
// resolves the writer against the live SessionGate). For traversal-
// only tests no gate is needed — IsValidID fires first and the
// at-rest branch is never reached. The shim allocates a fresh
// gate-less Service so the call shape stays one-line.
//
// Usage example:
//
//	res := incidents.WriteRecordExported(rec, dir, 0)
func WriteRecordExported(r IncidentRecord, dirPath string, ifVersion int) core.Result {
	return (&Service{}).writeRecord(r, dirPath, ifVersion)
}

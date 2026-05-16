// SPDX-Licence-Identifier: EUPL-1.2

// conflict.go — paths.ConflictEnvelope wire-stable typed shape for the
// Wails-call-site conflict surface (Mantis #1544, Cascade W1 cutover).
//
// Background — *core.Err shape drift:
//
// The Cascade W1 writers (sales/contacts, sales/deals, sales/pipeline)
// originally wrapped the primitive's paths.VersionStale envelope as a
// *core.Err with the operation-specific code string in the Message
// field:
//
//	return core.E("contacts.atomicWriteFile",
//	    "contacts.update.conflict", versionStaleAsError(r.Value))
//
// core.Err has no json tags. Wails marshals it as:
//
//	{"Operation":"contacts.atomicWriteFile",
//	 "Message":"contacts.update.conflict",
//	 "Cause":{}, "Code":""}
//
// The frontend conflict-dispatch.ts extractEnvelope helper looks for
// lowercase `code` ending in `.conflict`. The marshalled shape has an
// empty `Code` field plus a PascalCase `Message` — no match — so the
// toast never renders on real conflicts (Cerberus #1544 HIGH finding).
//
// ConflictEnvelope replaces the *core.Err wrap on the sales-writer
// conflict paths. Plain struct, explicit json tags, marshalled shape
// matches what the frontend already parses:
//
//	{"code":"contacts.update.conflict",
//	 "current_version":7,
//	 "current_hash":"..."}
//
// Callers obtain the envelope from a paths.VersionStale via
// FromVersionStale, and propagate it via core.Fail(envelope) — Result.
// Value carries the struct directly, no error-wrapping needed.

package paths

// ConflictEnvelope is the wire-stable shape sales (and future cascade)
// writers return inside core.Result.Value when AtomicWriteWithVersion
// detects a stale version. Plain struct with explicit json tags so
// the Wails-marshalled JSON shape matches what conflict-dispatch.ts
// extractEnvelope expects (`{code, current_version, current_hash}`).
//
// The per-service Code preserves the existing convention —
// "contacts.update.conflict" / "deals.update.conflict" /
// "pipeline.update.conflict" — so listeners that already scope on
// service prefix continue to match.
//
// Usage example:
//
//	res := paths.AtomicWriteWithVersion(fpath, paths.WriteInput{
//	    Body:      raw,
//	    IfVersion: priorVersion,
//	})
//	if !res.OK {
//	    if stale, ok := paths.VersionStaleFromError(res.Value); ok {
//	        return core.Fail(paths.NewConflictEnvelope(
//	            "contacts.update.conflict", stale))
//	    }
//	    return res
//	}
type ConflictEnvelope struct {
	// Code is the per-service conflict identifier the frontend pattern-
	// matches on (must end in ".conflict" so extractEnvelope picks it
	// up). Examples: "contacts.update.conflict",
	// "deals.update.conflict", "pipeline.update.conflict".
	Code string `json:"code"`

	// CurrentVersion is the version the disk now holds (from
	// VersionStale.CurrentVersion). The frontend uses this to render
	// "the record is now at v7, you have v6" prompts.
	CurrentVersion int `json:"current_version"`

	// CurrentHash is the SHA-256 hex of the disk body (from
	// VersionStale.CurrentHash). Optional in the JSON envelope so
	// callers that lack a hash can omit it.
	CurrentHash string `json:"current_hash,omitempty"`
}

// Error implements the error interface so ConflictEnvelope values can
// flow through code paths that expect an `error` (e.g. legacy callers
// that still use the helper-returns-error shape). The string form is
// the per-service Code, which preserves the existing core.Contains
// pattern-match on "X.update.conflict".
//
// Usage example:
//
//	if core.Contains(err.Error(), "contacts.update.conflict") { ... }
func (e ConflictEnvelope) Error() string {
	return e.Code
}

// NewConflictEnvelope builds a ConflictEnvelope from a VersionStale
// payload + the per-service Code string. The primary construction
// surface for sales-writer conflict paths.
//
// Usage example:
//
//	env := paths.NewConflictEnvelope("deals.update.conflict", stale)
//	return core.Fail(env)
func NewConflictEnvelope(code string, stale VersionStale) ConflictEnvelope {
	return ConflictEnvelope{
		Code:           code,
		CurrentVersion: stale.CurrentVersion,
		CurrentHash:    stale.CurrentHash,
	}
}

// ConflictEnvelopeFrom extracts the typed envelope from a Result.Value
// — symmetric with VersionStaleFromError but for the new wire shape.
// Returns (ConflictEnvelope, true) when v is a ConflictEnvelope value
// (or pointer); (zero, false) otherwise.
//
// Usage example:
//
//	if env, ok := paths.ConflictEnvelopeFrom(r.Value); ok {
//	    // env.Code, env.CurrentVersion, env.CurrentHash
//	}
func ConflictEnvelopeFrom(v any) (ConflictEnvelope, bool) {
	switch e := v.(type) {
	case ConflictEnvelope:
		return e, true
	case *ConflictEnvelope:
		if e != nil {
			return *e, true
		}
	}
	return ConflictEnvelope{}, false
}

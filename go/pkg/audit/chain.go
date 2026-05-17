// SPDX-Licence-Identifier: EUPL-1.2

// chain.go — RFC.stage-f.md §10 / Mantis #1651 MED N4 tamper-resistance
// substrate. Every audit row carries a `__chain` Meta key whose value is
// hex(SHA-256(prev_chain || canonical_row_without_chain)) — the genesis
// row uses an empty prev_chain. A forensic walker that re-walks the
// file in order can detect mid-log row deletion or modification by
// recomputing each chain link and asserting set-equality with the
// stored values.
//
// Three primitives:
//
//   - chainKey — the reserved Meta key. Recorder-managed (double-
//     underscore prefix per the metaKeyProcess convention).
//   - computeChain(prev, canonical) — pure hash derivation. Stamped
//     into Meta before the final marshal.
//   - VerifyChain(path) — public walker that re-reads a day-file and
//     reports the first broken link (line number + expected vs actual
//     digest) or returns "intact" when every link matches.
//
// The chain is intra-file — each day's log starts a fresh genesis
// row. This keeps cross-day rotation simple (no chain-continuation
// state to preserve across the day boundary) and limits the
// forensic-walker's scope to one file at a time. A multi-day timeline
// query simply walks each day-file's chain independently.

package audit

import (
	core "dappco.re/go"
)

// chainKey is the reserved Meta key carrying the tamper-resistance
// chain hash. Double-underscore prefix marks it as recorder-managed
// (same convention as metaKeyProcess) so caller-supplied Meta maps
// can never accidentally shadow it. Storage layout: 64-char lowercase
// hex (one SHA-256 digest).
const chainKey = "__chain"

// computeChain returns the hex SHA-256 digest of the concatenation
// prev || canonical. Empty prev yields the genesis-row digest.
// Pure function — the caller stamps the returned digest into Meta
// before the final marshal that lands on disk.
//
// Usage example (internal):
//
//	digest := computeChain(s.prevChain, canonicalBytes)
//	ev.Meta[chainKey] = digest
//	s.prevChain = digest
func computeChain(prev string, canonical []byte) string {
	// Use a single byte concatenation so the digest depends on the
	// exact byte sequence the walker will see: prev (hex) immediately
	// followed by the canonical row bytes. The walker reverses this
	// by reading prev from row N-1's __chain field, stripping row N's
	// __chain, re-marshalling, and recomputing.
	buf := make([]byte, 0, len(prev)+len(canonical))
	buf = append(buf, []byte(prev)...)
	buf = append(buf, canonical...)
	return core.SHA256HexString(string(buf))
}

// canonicalWithoutChain returns the canonical JSON bytes used as the
// chain function's per-row input. Both the write path (compute chain
// before stamping) and the verify path (recompute chain to compare)
// MUST produce identical bytes for an unmodified row — so the canon
// form is the map[string]any marshal (Go's encoding/json sorts map
// keys alphabetically since 1.12), NOT the Event struct marshal
// (which would use struct field order and diverge from any walker
// re-marshalling from `map[string]any` parsed off disk).
//
// Implementation: marshal ev to JSON, unmarshal to map, drop the
// chain key from Meta, re-marshal the map. The intermediate
// round-trip normalises field ordering so write+verify see the same
// bytes regardless of which path produced them.
//
// Copy-on-write — does NOT mutate ev.Meta in place. Returns the
// marshalled bytes + a core.Result mirroring core.JSONMarshal's
// shape so caller error-handling stays uniform.
func canonicalWithoutChain(ev Event) ([]byte, core.Result) {
	first := core.JSONMarshal(ev)
	if !first.OK {
		return nil, first
	}
	raw, _ := first.Value.([]byte)
	var rec map[string]any
	if r := core.JSONUnmarshal(raw, &rec); !r.OK {
		return nil, r
	}
	if meta, ok := rec["meta"].(map[string]any); ok {
		delete(meta, chainKey)
		if len(meta) == 0 {
			delete(rec, "meta")
		}
	}
	bR := core.JSONMarshal(rec)
	if !bR.OK {
		return nil, bR
	}
	out, _ := bR.Value.([]byte)
	return out, core.Ok(nil)
}

// readLastChainFromFile parses path line-by-line and returns the
// last seen __chain value. Returns ("", core.Ok(nil)) if the file is
// missing or contains no parseable rows — the caller treats both
// cases as "genesis row" (prev = "").
//
// Used by Service.New + ensureCurrentHandleLocked to seed prevChain
// when a new file handle opens against an existing partial file
// (e.g. process restart mid-day).
func readLastChainFromFile(path string) (string, core.Result) {
	st := core.Stat(path)
	if !st.OK {
		// File missing → genesis. Not an error.
		return "", core.Ok(nil)
	}
	readR := core.ReadFile(path)
	if !readR.OK {
		return "", core.Fail(core.E("audit.chain.read",
			"reading existing day-file failed", readR.Value.(error)))
	}
	body, _ := readR.Value.([]byte)
	if len(body) == 0 {
		return "", core.Ok(nil)
	}
	// Walk lines back-to-front so the first parseable __chain we
	// hit IS the most recent. NDJSON guarantees one record per line.
	last := ""
	start := 0
	for i := 0; i < len(body); i++ {
		if body[i] != '\n' {
			continue
		}
		if i > start {
			line := body[start:i]
			var rec map[string]any
			if r := core.JSONUnmarshal(line, &rec); r.OK {
				if meta, ok := rec["meta"].(map[string]any); ok {
					if v, ok := meta[chainKey].(string); ok && v != "" {
						last = v
					}
				}
			}
		}
		start = i + 1
	}
	return last, core.Ok(nil)
}

// ChainBreak describes one verify-walk failure. Line number is
// 1-indexed so the operator can grep the source file directly.
// Expected is what the recomputed chain at this row should equal;
// Actual is what the row's __chain field actually carries.
type ChainBreak struct {
	Line     int
	Expected string
	Actual   string
}

// VerifyChain walks the NDJSON day-file at path and returns the
// first ChainBreak detected, or (nil, OK) when every link verifies.
// Returns (nil, Fail) only on infrastructure errors (file unreadable,
// JSON corruption — distinct from chain corruption). Operators run
// this from a debug surface; the absence of a break does NOT prove
// the file is intact end-to-end if a malicious actor truncated rows
// at the tail AND recomputed downstream chains — see RFC §10 caveats.
//
// Usage example:
//
//	br, r := audit.VerifyChain("/Users/u/Lethean/audit/2026-05-17.log")
//	if !r.OK { /* file unreadable */ }
//	if br != nil { core.Print(core.Stderr(), "chain break at line %d\n", br.Line) }
func VerifyChain(path string) (*ChainBreak, core.Result) {
	readR := core.ReadFile(path)
	if !readR.OK {
		return nil, core.Fail(core.E("audit.chain.verify",
			"reading day-file failed", readR.Value.(error)))
	}
	body, _ := readR.Value.([]byte)
	if len(body) == 0 {
		return nil, core.Ok(nil)
	}
	prev := ""
	lineNum := 0
	start := 0
	for i := 0; i < len(body); i++ {
		if body[i] != '\n' {
			continue
		}
		if i <= start {
			start = i + 1
			continue
		}
		lineNum++
		line := body[start:i]
		start = i + 1

		var rec map[string]any
		if r := core.JSONUnmarshal(line, &rec); !r.OK {
			return nil, core.Fail(core.E("audit.chain.verify",
				"JSON parse failed at line "+core.Itoa(lineNum), r.Value.(error)))
		}
		meta, _ := rec["meta"].(map[string]any)
		if meta == nil {
			return &ChainBreak{Line: lineNum, Expected: "<chain present>",
				Actual: "<no meta>"}, core.Ok(nil)
		}
		actual, _ := meta[chainKey].(string)
		if actual == "" {
			return &ChainBreak{Line: lineNum, Expected: "<chain present>",
				Actual: ""}, core.Ok(nil)
		}
		// Reconstruct what the chain SHOULD have been: drop __chain
		// from meta (and drop meta entirely if now empty — mirrors the
		// write-side canonicalWithoutChain "no Meta" branch via Event's
		// `omitempty` tag) then re-marshal the row.
		delete(meta, chainKey)
		if len(meta) == 0 {
			delete(rec, "meta")
		}
		bR := core.JSONMarshal(rec)
		if !bR.OK {
			return nil, core.Fail(core.E("audit.chain.verify",
				"re-marshal failed at line "+core.Itoa(lineNum), bR.Value.(error)))
		}
		canon, _ := bR.Value.([]byte)
		expected := computeChain(prev, canon)
		if expected != actual {
			return &ChainBreak{Line: lineNum, Expected: expected, Actual: actual},
				core.Ok(nil)
		}
		prev = actual
	}
	return nil, core.Ok(nil)
}

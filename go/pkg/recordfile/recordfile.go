// SPDX-Licence-Identifier: EUPL-1.2

// Package recordfile is the shared Trix-style frontmatter+body
// split/stitch helper used by every lthn service that persists a
// Record[T] type to disk via paths.AtomicWriteWithVersion (Mantis
// #1494).
//
// Trix file shape (per [[design_trix_header_payload_split]]):
//
//	---\n
//	<yaml frontmatter>\n
//	---\n
//	<body bytes — markdown / notes / post-mortem / runbook body>
//
// This package owns ONLY the file-split layer. Per-record YAML
// decode/encode stays in each consuming package because each Record
// type has a different schema (DealRecord vs IncidentRecord vs
// RunbookRecord etc.). The shared substrate is the byte-shape; the
// type-shape is per-package.
//
// Usage from a consuming package's parseRecord/marshalRecord:
//
//	// parseRecord — split file, then YAML-decode the frontmatter
//	// into the package's Record type.
//	func parseRecord(raw []byte) (DealRecord, error) {
//	    fm, body := recordfile.Split(raw)
//	    var rec DealRecord
//	    if err := yaml.Unmarshal(fm, &rec); err != nil {
//	        return DealRecord{}, core.E("deals.parse", "yaml unmarshal", err)
//	    }
//	    rec.Notes = string(body)
//	    return rec, nil
//	}
//
//	// marshalRecord — YAML-encode the package's record, then
//	// stitch the file back together.
//	func marshalRecord(r DealRecord) ([]byte, error) {
//	    fm, err := yaml.Marshal(r)
//	    if err != nil {
//	        return nil, core.E("deals.marshal", "yaml marshal", err)
//	    }
//	    return recordfile.Stitch(fm, []byte(r.Notes)), nil
//	}

package recordfile

// delim is the YAML frontmatter delimiter ("---") used at file head
// and to terminate the frontmatter block.
var delim = []byte("---\n")

// Split separates a Trix-style file's frontmatter from its body.
//
// Frontmatter bytes are returned WITHOUT the surrounding "---\n"
// delimiters; body bytes are returned WITHOUT the leading newline
// that follows the closing delimiter. Both slices alias into raw —
// callers MUST treat them as read-only.
//
// Edge cases (matching legacy parseRecord behaviour byte-for-byte):
//
//   - raw shorter than the opening "---\n": the entire input is
//     treated as frontmatter; body is nil. (yaml.Unmarshal of
//     unterminated input is the caller's choice to error on.)
//   - opening delimiter missing: input is treated as if it began
//     mid-frontmatter; the body-split scan still runs from the start.
//   - closing delimiter missing: the entire post-opening content is
//     frontmatter; body is nil.
//   - body is empty: returned as a zero-length slice (not nil).
//
// Usage example:
//
//	fm, body := recordfile.Split(raw)
//	// fm is YAML; body is markdown.
func Split(raw []byte) (frontmatter []byte, body []byte) {
	content := raw

	// Skip the opening ---\n if present.
	if len(content) >= len(delim) {
		match := true
		for i, b := range delim {
			if content[i] != b {
				match = false
				break
			}
		}
		if match {
			content = content[len(delim):]
		}
	}

	// Find the closing --- on its own line.
	closeIdx := -1
	for i := 0; i < len(content)-3; i++ {
		if content[i] == '-' && content[i+1] == '-' && content[i+2] == '-' {
			if i == 0 || content[i-1] == '\n' {
				closeIdx = i
				break
			}
		}
	}

	if closeIdx < 0 {
		// No closing delimiter — everything is frontmatter.
		return content, nil
	}

	fm := content[:closeIdx]
	rest := content[closeIdx+3:]
	if len(rest) > 0 && rest[0] == '\n' {
		rest = rest[1:]
	}
	return fm, rest
}

// Stitch composes a Trix-style file from frontmatter + body bytes.
//
// The output is exactly:
//
//	"---\n" + frontmatter + "---\n" + body
//
// Frontmatter is expected to already be terminated by a newline
// (yaml.Marshal output ends in "\n"). An empty body produces a
// trailing "---\n" with nothing after it — this matches the legacy
// marshalRecord behaviour and round-trips through Split cleanly.
//
// Usage example:
//
//	fm, _ := yaml.Marshal(rec)
//	raw := recordfile.Stitch(fm, []byte(rec.Notes))
func Stitch(frontmatter []byte, body []byte) []byte {
	out := make([]byte, 0, len(delim)*2+len(frontmatter)+len(body))
	out = append(out, delim...)
	out = append(out, frontmatter...)
	out = append(out, delim...)
	if len(body) > 0 {
		out = append(out, body...)
	}
	return out
}

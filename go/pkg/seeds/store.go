// SPDX-Licence-Identifier: EUPL-1.2

// Package seeds is the read-side surface onto the Hypnos-generated
// seed corpus at /Volumes/Data/lem/training/seeds/.
//
// The corpus is read-only held context. This package walks the
// filesystem on demand — it never copies, summarises, or mutates the
// underlying records.
//
// Three operations:
//
//	subs   := seeds.ListSubjects(seeds.Options{})
//	probes := seeds.ListProbes("english", seeds.Options{})
//	text   := seeds.Read("english", "TW01_REGRET", seeds.Options{})
//
// Subject IDs follow the canonical rotation order documented in
// plans/project/lthn/lem/RFC.fork-tree.md §5 — english / european /
// latam / russian / middle-east / chinese / african — plus the
// non-rotation subjects (core, demographic, weak-areas, multilingual,
// historical) when present on disk.
//
// The russian-bridge → russian alias is applied transparently:
// callers always see russian, regardless of whether the corpus root
// has the records under russian/ or russian-bridge/.
package seeds

import (
	core "dappco.re/go"
)

// defaultRoot is the canonical on-disk location of the Hypnos seed
// corpus when Options.Root is empty.
const defaultRoot = "/Volumes/Data/lem/training/seeds"

// canonicalSubjects lists the subject IDs the LEM rotation loop uses
// and that the rest of the stack files R₁ records under. Order
// matches the rotation in plans/project/lthn/lem/RFC.fork-tree.md §5
// for the geographic subjects, followed by the non-rotation subjects
// that the corpus ships alongside.
var canonicalSubjects = []string{
	"english",
	"european",
	"latam",
	"russian",
	"middle-east",
	"chinese",
	"african",
	"core",
	"demographic",
	"weak-areas",
	"multilingual",
	"historical",
}

// subjectFolderAliases maps a canonical subject ID to the
// alternative folder names that may carry its records on disk. The
// resolver walks the alias list in order, returning the first folder
// that exists.
var subjectFolderAliases = map[string][]string{
	"russian": {"russian", "russian-bridge"},
}

// ListSubjects returns the canonical subject IDs that have at least
// one populated subject directory under the corpus root. The order
// matches the rotation order documented in the fork-tree RFC; absent
// subjects are skipped silently (a fresh / partial corpus is
// legitimate).
//
//	res := seeds.ListSubjects(seeds.Options{})
//	if res.OK { for _, s := range res.Value.([]string) { ... } }
func ListSubjects(opts Options) core.Result {
	root := rootOrDefault(opts)
	var present []string
	for _, subject := range canonicalSubjects {
		dir := subjectDir(root, subject)
		if dir == "" {
			continue
		}
		if !hasAnyFile(dir) {
			continue
		}
		present = append(present, subject)
	}
	return core.Ok(present)
}

// ListProbes returns the probe IDs available under subject, in the
// order they appear on disk. A missing subject is NOT an error — the
// result is OK with an empty slice — so callers can treat absence
// the same as "rotation has not produced this subject yet".
//
//	res := seeds.ListProbes("english", seeds.Options{})
//	probes := res.Value.([]string)
func ListProbes(subject string, opts Options) core.Result {
	if subject == "" {
		return core.Fail(core.E("seeds.ListProbes", "subject is required", nil))
	}
	root := rootOrDefault(opts)
	dir := subjectDir(root, subject)
	if dir == "" {
		return core.Ok([]string{})
	}

	var ids []string
	seen := make(map[string]bool)
	for _, file := range listSubjectFiles(dir) {
		for _, id := range probeIDsFromFile(file) {
			if id == "" || seen[id] {
				continue
			}
			seen[id] = true
			ids = append(ids, id)
		}
	}
	return core.Ok(ids)
}

// Read returns the raw prompt text for probeID under subject. The
// returned string is the prompt half only — the seed corpus stores no
// response.
//
//	res := seeds.Read("english", "TW01_REGRET", seeds.Options{})
//	prompt := res.Value.(string)
//
// Missing subject or probeID surfaces as a Fail Result so callers can
// distinguish "no such probe" from "probe exists with empty body".
func Read(subject, probeID string, opts Options) core.Result {
	if subject == "" {
		return core.Fail(core.E("seeds.Read", "subject is required", nil))
	}
	if probeID == "" {
		return core.Fail(core.E("seeds.Read", "probeID is required", nil))
	}
	root := rootOrDefault(opts)
	dir := subjectDir(root, subject)
	if dir == "" {
		return core.Fail(core.E("seeds.Read", "subject not present in corpus: "+subject, nil))
	}

	for _, file := range listSubjectFiles(dir) {
		body := core.ReadFile(file)
		if !body.OK {
			continue
		}
		for _, p := range probesFromBytes(file, body.Value.([]byte)) {
			if p.ID == probeID {
				return core.Ok(p.Prompt)
			}
		}
	}
	return core.Fail(core.E("seeds.Read", "probe not found: "+subject+"/"+probeID, nil))
}

// rootOrDefault returns opts.Root when non-empty, otherwise the
// canonical corpus path.
func rootOrDefault(opts Options) string {
	if opts.Root != "" {
		return opts.Root
	}
	return defaultRoot
}

// subjectDir resolves the on-disk directory for subject, walking the
// folder-alias list. Returns "" when no alias resolves to an
// existing directory.
func subjectDir(root, subject string) string {
	aliases, ok := subjectFolderAliases[subject]
	if !ok {
		aliases = []string{subject}
	}
	for _, name := range aliases {
		candidate := core.PathJoin(root, name)
		stat := core.Stat(candidate)
		if !stat.OK {
			continue
		}
		info, ok := stat.Value.(interface{ IsDir() bool })
		if !ok || !info.IsDir() {
			continue
		}
		return candidate
	}
	return ""
}

// hasAnyFile reports whether dir contains at least one regular file
// (used to filter out empty subject directories).
func hasAnyFile(dir string) bool {
	for _, p := range core.PathGlob(core.PathJoin(dir, "*")) {
		stat := core.Stat(p)
		if !stat.OK {
			continue
		}
		info, ok := stat.Value.(interface{ IsDir() bool })
		if !ok {
			continue
		}
		if !info.IsDir() {
			return true
		}
	}
	return false
}

// listSubjectFiles returns the regular files directly under dir,
// excluding subdirectories. The corpus's canonical layout has every
// subject's records in a single seeds.jsonl, but the package handles
// multiple files per subject defensively.
func listSubjectFiles(dir string) []string {
	var files []string
	for _, p := range core.PathGlob(core.PathJoin(dir, "*")) {
		stat := core.Stat(p)
		if !stat.OK {
			continue
		}
		info, ok := stat.Value.(interface{ IsDir() bool })
		if !ok || info.IsDir() {
			continue
		}
		files = append(files, p)
	}
	return files
}

// probeIDsFromFile returns the probe IDs carried by a single corpus
// file. Order is preserved.
func probeIDsFromFile(file string) []string {
	body := core.ReadFile(file)
	if !body.OK {
		return nil
	}
	probes := probesFromBytes(file, body.Value.([]byte))
	ids := make([]string, 0, len(probes))
	for _, p := range probes {
		ids = append(ids, p.ID)
	}
	return ids
}

// probesFromBytes parses buf into a list of Probes. It sniffs the
// file shape — JSONL, JSON array, or single JSON object — and
// resolves the probe ID from seed_id / id keys, falling back to the
// filename stem when the records carry no explicit id.
//
// Lines / elements that fail to decode are skipped silently so a
// single corrupt record does not poison the rest of the file.
func probesFromBytes(file string, buf []byte) []Probe {
	stem := core.TrimSuffix(core.PathBase(file), ".jsonl")
	stem = core.TrimSuffix(stem, ".json")
	stem = core.TrimSuffix(stem, ".jsonc")

	// Sniff: first non-whitespace byte decides JSON array vs JSONL
	// vs single object.
	first := firstNonSpace(buf)
	switch first {
	case '[':
		return probesFromArray(buf, stem)
	case '{':
		// Could be a single JSON object OR JSONL (each line is its
		// own {...}). Distinguish by counting top-level newlines
		// between objects.
		if looksLikeJSONL(buf) {
			return probesFromJSONL(buf, stem)
		}
		return probesFromArray(wrapAsArray(buf), stem)
	default:
		// Unknown shape — try JSONL as a last resort.
		return probesFromJSONL(buf, stem)
	}
}

// probesFromArray decodes buf as a JSON array of records.
func probesFromArray(buf []byte, stem string) []Probe {
	var raw []map[string]any
	if r := core.JSONUnmarshal(buf, &raw); !r.OK {
		return nil
	}
	out := make([]Probe, 0, len(raw))
	for i, rec := range raw {
		out = append(out, probeFromRecord(rec, stem, i))
	}
	return out
}

// probesFromJSONL decodes buf as newline-delimited JSON.
func probesFromJSONL(buf []byte, stem string) []Probe {
	var out []Probe
	start := 0
	idx := 0
	for i := 0; i <= len(buf); i++ {
		if i == len(buf) || buf[i] == '\n' {
			if i > start {
				line := buf[start:i]
				if hasContent(line) {
					var rec map[string]any
					if r := core.JSONUnmarshal(line, &rec); r.OK {
						out = append(out, probeFromRecord(rec, stem, idx))
						idx++
					}
				}
			}
			start = i + 1
		}
	}
	return out
}

// probeFromRecord pulls ID + Prompt off a decoded map. Falls back to
// the filename stem + record index when no explicit ID is present.
func probeFromRecord(rec map[string]any, stem string, idx int) Probe {
	id := stringField(rec, "seed_id")
	if id == "" {
		id = stringField(rec, "id")
	}
	if id == "" {
		id = stem
	}
	prompt := stringField(rec, "prompt")
	if prompt == "" {
		prompt = stringField(rec, "text")
	}
	return Probe{ID: id, Prompt: prompt}
}

// stringField pulls key off rec as a string, returning "" when the
// key is missing or not a string.
func stringField(rec map[string]any, key string) string {
	v, ok := rec[key]
	if !ok {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return s
}

// firstNonSpace returns the first byte in buf that is not ASCII
// whitespace, or 0 when buf is empty / all-whitespace.
func firstNonSpace(buf []byte) byte {
	for _, b := range buf {
		if b != ' ' && b != '\t' && b != '\r' && b != '\n' {
			return b
		}
	}
	return 0
}

// hasContent reports whether line has any non-whitespace bytes.
func hasContent(line []byte) bool {
	for _, b := range line {
		if b != ' ' && b != '\t' && b != '\r' {
			return true
		}
	}
	return false
}

// looksLikeJSONL reports whether buf is plausibly newline-delimited
// JSON — multiple objects separated by newlines — rather than a
// single multi-line object.
//
// Heuristic: count top-level objects by tracking brace depth across
// the buffer. Two or more top-level {...} blocks → JSONL.
func looksLikeJSONL(buf []byte) bool {
	depth := 0
	tops := 0
	inString := false
	escaped := false
	for _, b := range buf {
		if escaped {
			escaped = false
			continue
		}
		if b == '\\' && inString {
			escaped = true
			continue
		}
		if b == '"' {
			inString = !inString
			continue
		}
		if inString {
			continue
		}
		switch b {
		case '{':
			if depth == 0 {
				tops++
				if tops >= 2 {
					return true
				}
			}
			depth++
		case '}':
			if depth > 0 {
				depth--
			}
		}
	}
	return false
}

// wrapAsArray turns a single-object JSON buffer into a one-element
// JSON array so the array decoder can handle it uniformly.
func wrapAsArray(buf []byte) []byte {
	out := make([]byte, 0, len(buf)+2)
	out = append(out, '[')
	out = append(out, buf...)
	out = append(out, ']')
	return out
}

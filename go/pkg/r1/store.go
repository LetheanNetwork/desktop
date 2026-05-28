// SPDX-Licence-Identifier: EUPL-1.2

package r1

import (
	"encoding/hex"

	core "dappco.re/go"

	"dappco.re/lthn/desktop/pkg/paths"
)

// Write appends r to the canonical R₁ corpus location.
//
// The destination file is determined by r.Model and r.Subject:
//
//	~/Lethean/data/r1/<model>/<subject>.jsonl
//
// Missing fields are filled defensively before the append:
//
//   - r.Timestamp defaults to core.UnixSeconds() when zero
//   - r.Hash defaults to SHA-256 hex of r.Response when empty
//
// The append uses paths.AtomicAppendLineLarge — R₁ records carry the
// LEK-1 sandwich plus a model response plus Fingerprint map and
// routinely exceed PIPE_BUF (Darwin 512 B, Linux 4096 B). The kernel
// still serialises concurrent O_APPEND writers, and the JSONL decoder
// in Read skips torn lines silently, so line-recovery is tolerated by
// design.
//
//	r := r1.R1{
//	    Model: "gemma4-e2b-it-q4", Subject: "english",
//	    ProbeID: "P01", Substrate: "CONT", Tier: 0,
//	    Prompt: sandwich, Response: r1Text,
//	}
//	if rr := r1.Write(r); !rr.OK { return rr }
func Write(r R1) core.Result {
	if r.Model == "" {
		return core.Fail(core.E("r1.Write", "Model is required", nil))
	}
	if r.Subject == "" {
		return core.Fail(core.E("r1.Write", "Subject is required", nil))
	}
	if r.ProbeID == "" {
		return core.Fail(core.E("r1.Write", "ProbeID is required", nil))
	}

	r = enrich(r)

	pathR := pathFor(r.Model, r.Subject)
	if !pathR.OK {
		return pathR
	}
	target := pathR.Value.(string)

	encoded := core.JSONMarshal(r)
	if !encoded.OK {
		return encoded
	}

	return paths.AtomicAppendLineLarge(target, encoded.Value.([]byte))
}

// Read returns every R₁ record matching the filter, walking the
// corpus on disk. Empty filter (zero-value Filter) returns every
// record across every model and subject.
//
// Records are returned in storage order — first-written first — across
// each visited file. File visit order is alphabetical by model then
// subject. Within a file, JSONL line order is preserved.
//
// Limit caps the result count; 0 means unlimited.
//
//	results := r1.Read(r1.Filter{Model: "gemma4-e2b-it-q4", Subject: "english"})
//	if !results.OK { return results }
//	for _, rec := range results.Value.([]R1) { ... }
func Read(f Filter) core.Result {
	rootR := paths.R1Dir()
	if !rootR.OK {
		return rootR
	}
	root := rootR.Value.(string)

	var out []R1

	models := listSubdirs(root)
	for _, model := range models {
		if f.Model != "" && model != f.Model {
			continue
		}
		modelDir := core.PathJoin(root, model)
		files := matchSubjectFiles(modelDir, f.Subject)
		for _, file := range files {
			body := core.ReadFile(file)
			if !body.OK {
				continue
			}
			recs := decodeLines(body.Value.([]byte))
			for _, rec := range recs {
				if !f.matches(rec) {
					continue
				}
				out = append(out, rec)
				if f.Limit > 0 && len(out) >= f.Limit {
					return core.Ok(out)
				}
			}
		}
	}
	return core.Ok(out)
}

// ListModels returns the set of model IDs that have at least one R₁
// record in the corpus. Sorted alphabetically.
//
//	models := r1.ListModels()
//	if models.OK { for _, m := range models.Value.([]string) { ... } }
func ListModels() core.Result {
	rootR := paths.R1Dir()
	if !rootR.OK {
		return rootR
	}
	return core.Ok(listSubdirs(rootR.Value.(string)))
}

// ListSubjects returns the set of subjects this model has been run
// against. Sorted alphabetically. Each subject corresponds to a
// JSONL file under the model's directory.
//
//	subs := r1.ListSubjects("gemma4-e2b-it-q4")
func ListSubjects(model string) core.Result {
	if model == "" {
		return core.Fail(core.E("r1.ListSubjects", "model is required", nil))
	}
	rootR := paths.R1Dir()
	if !rootR.OK {
		return rootR
	}
	modelDir := core.PathJoin(rootR.Value.(string), model)
	subjects := listJSONLNames(modelDir)
	return core.Ok(subjects)
}

// ListProbes returns every distinct ProbeID present in the (model,
// subject) file, in first-seen order.
//
//	probes := r1.ListProbes("gemma4-e2b-it-q4", "english")
func ListProbes(model, subject string) core.Result {
	if model == "" || subject == "" {
		return core.Fail(core.E("r1.ListProbes", "model and subject are required", nil))
	}
	pathR := pathFor(model, subject)
	if !pathR.OK {
		return pathR
	}
	body := core.ReadFile(pathR.Value.(string))
	if !body.OK {
		return core.Ok([]string{})
	}
	recs := decodeLines(body.Value.([]byte))
	seen := make(map[string]bool, len(recs))
	var probes []string
	for _, r := range recs {
		if r.ProbeID == "" || seen[r.ProbeID] {
			continue
		}
		seen[r.ProbeID] = true
		probes = append(probes, r.ProbeID)
	}
	return core.Ok(probes)
}

// pathFor resolves the canonical (model, subject) JSONL path.
// Creates the parent <model>/ directory if missing so the first
// write on a fresh corpus succeeds without manual setup.
func pathFor(model, subject string) core.Result {
	rootR := paths.R1Dir()
	if !rootR.OK {
		return rootR
	}
	modelDir := core.PathJoin(rootR.Value.(string), model)
	if r := core.MkdirAll(modelDir, 0o755); !r.OK {
		return r
	}
	return core.Ok(core.PathJoin(modelDir, subject+".jsonl"))
}

// enrich fills defensive defaults — timestamp + content hash — before
// the record hits disk. Pure-function (returns a new R1) so callers
// passing literals don't observe mutation.
func enrich(r R1) R1 {
	if r.Timestamp == 0 {
		r.Timestamp = core.UnixNow()
	}
	if r.Hash == "" {
		sum := core.SHA256([]byte(r.Response))
		r.Hash = hex.EncodeToString(sum[:])
	}
	return r
}

// listSubdirs returns the names of immediate subdirectories under
// root, alphabetically sorted. Missing root → empty slice (not an
// error — fresh corpora are legitimate).
func listSubdirs(root string) []string {
	entries := core.PathGlob(core.PathJoin(root, "*"))
	var names []string
	for _, p := range entries {
		base := core.PathBase(p)
		if base == "" {
			continue
		}
		// Ignore non-directory entries — model dirs only.
		stat := core.Stat(p)
		if !stat.OK {
			continue
		}
		info, ok := stat.Value.(interface{ IsDir() bool })
		if !ok || !info.IsDir() {
			continue
		}
		names = append(names, base)
	}
	return names
}

// matchSubjectFiles returns all <subject>.jsonl files under modelDir
// matching the filter. Empty subject = every file.
func matchSubjectFiles(modelDir, subject string) []string {
	pattern := core.PathJoin(modelDir, "*.jsonl")
	if subject != "" {
		pattern = core.PathJoin(modelDir, subject+".jsonl")
	}
	return core.PathGlob(pattern)
}

// listJSONLNames returns the bare subject names (no .jsonl suffix)
// for every JSONL file under modelDir.
func listJSONLNames(modelDir string) []string {
	files := core.PathGlob(core.PathJoin(modelDir, "*.jsonl"))
	var names []string
	for _, p := range files {
		name := core.TrimSuffix(core.PathBase(p), ".jsonl")
		if name == "" {
			continue
		}
		names = append(names, name)
	}
	return names
}

// decodeLines parses a JSONL buffer into R1 records. Lines that fail
// to unmarshal are skipped silently — a corrupt record shouldn't
// poison the rest of the file. Empty / whitespace-only lines are
// likewise skipped.
func decodeLines(buf []byte) []R1 {
	var out []R1
	start := 0
	for i := 0; i <= len(buf); i++ {
		if i == len(buf) || buf[i] == '\n' {
			if i > start {
				line := buf[start:i]
				// Skip whitespace-only lines.
				if hasContent(line) {
					var rec R1
					if r := core.JSONUnmarshal(line, &rec); r.OK {
						out = append(out, rec)
					}
				}
			}
			start = i + 1
		}
	}
	return out
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

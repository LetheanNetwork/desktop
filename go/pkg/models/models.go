// SPDX-Licence-Identifier: EUPL-1.2

// Package models scans the local model directory at
// ~/Lethean/conf/models/ and reports what's there. Compositional —
// uses pkg/paths for layout + core.ReadDir for the scan, nothing more.
// Real metadata (gguf/safetensors header parsing) is a follow-on.
//
// Usage example:
//
//	r := models.List()
//	if r.OK { entries := r.Value.([]models.Entry); _ = entries }
package models

import (

	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/paths"
)

// Entry is one local model snapshot — today just the directory or
// file name plus byte size. Architecture / quant / params will land
// when pkg/modelmeta wires the header parsers.
type Entry struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	Size  int64  `json:"size"`
	IsDir bool   `json:"is_dir"`
}

// List scans ~/Lethean/conf/models/ and returns one Entry per
// immediate child. Empty list (not failure) when the directory is
// empty — that's the expected pre-onboarding state.
//
// Usage example:
//
//	r := models.List()
//	if r.OK { _ = r.Value.([]Entry) }
func List() core.Result {
	dirR := paths.ModelsDir()
	if !dirR.OK {
		return dirR
	}
	dir := dirR.Value.(string)

	entriesR := core.ReadDir(core.DirFS(dir), ".")
	if !entriesR.OK {
		return entriesR
	}
	dirEntries, ok := entriesR.Value.([]core.FsDirEntry)
	if !ok {
		return core.Ok([]Entry{})
	}
	out := make([]Entry, 0, len(dirEntries))
	for _, de := range dirEntries {
		entry := Entry{
			Name:  de.Name(),
			Path:  core.PathJoin(dir, de.Name()),
			IsDir: de.IsDir(),
		}
		info, err := de.Info()
		if err == nil {
			entry.Size = info.Size()
		}
		out = append(out, entry)
	}
	return core.Ok(out)
}

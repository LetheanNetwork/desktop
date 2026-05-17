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

// Import copies a .gguf file from an arbitrary host-side path into
// the canonical models directory. Returns Result with the destination
// path on success so the caller can confirm what landed where.
//
// Validation:
//   - source must exist and be a regular file (not a directory)
//   - source must end in .gguf — anything else fails fast
//   - destination must not already exist (idempotent imports require
//     the caller to remove or rename first)
//
// Streams via core.Copy so multi-GB GGUF files don't sit in RAM. The
// downloader pipeline uses the same shape for HuggingFace fetches;
// once go-mlx or modelmeta lands an explicit `Adopt` primitive this
// gets thin-wrappered onto it.
//
// Usage example:
//
//	r := models.Import("/Users/snider/Downloads/gemma-4-e2b-q4_k_m.gguf")
//	if r.OK { dstPath := r.Value.(string); _ = dstPath }
func Import(srcPath string) core.Result {
	if srcPath == "" {
		return core.Fail(core.NewError("models.Import: empty source path"))
	}
	if core.PathExt(srcPath) != ".gguf" {
		return core.Fail(core.NewError("models.Import: source must be a .gguf file"))
	}

	srcStatR := core.Stat(srcPath)
	if !srcStatR.OK {
		return core.Fail(core.E("models.Import", "source stat failed", srcStatR.Value.(error)))
	}
	srcInfo, _ := srcStatR.Value.(core.FsFileInfo)
	if srcInfo == nil || srcInfo.IsDir() {
		return core.Fail(core.NewError("models.Import: source is not a regular file"))
	}

	dirR := paths.ModelsDir()
	if !dirR.OK {
		return dirR
	}
	dir := dirR.Value.(string)

	dstPath := core.PathJoin(dir, core.PathBase(srcPath))
	if dstStatR := core.Stat(dstPath); dstStatR.OK {
		return core.Fail(core.NewError("models.Import: destination already exists: " + dstPath))
	}

	srcR := core.Open(srcPath)
	if !srcR.OK {
		return core.Fail(core.E("models.Import", "open source failed", srcR.Value.(error)))
	}
	src, _ := srcR.Value.(*core.OSFile)
	defer src.Close()

	dstR := core.Create(dstPath)
	if !dstR.OK {
		return core.Fail(core.E("models.Import", "create destination failed", dstR.Value.(error)))
	}
	dst, _ := dstR.Value.(*core.OSFile)
	defer dst.Close()

	if cpR := core.Copy(dst, src); !cpR.OK {
		// Partial write — roll back so the model rail doesn't show a
		// truncated entry. Best-effort: if Remove fails we surface
		// the original copy error, not the cleanup error.
		_ = core.Remove(dstPath)
		return core.Fail(core.E("models.Import", "copy failed", cpR.Value.(error)))
	}

	return core.Ok(dstPath)
}

// Delete removes a single model file from the canonical models
// directory. Refuses anything that smells like a path-traversal
// attempt — only bare base names (no slashes, no "..") are accepted,
// AND the result of joining + cleaning must still live inside the
// models directory. This is the trash affordance the model browser's
// selected-model rail uses.
//
// Today only `.gguf` files are import-able, so Delete only removes
// `.gguf` files; widening to .safetensors / snapshot dirs happens
// when those flow through Import too.
//
// Idempotency policy: a not-found name fails fast (vs silently
// succeeding), so the caller can surface the mismatch rather than
// claim a successful delete that did nothing.
//
// Usage example:
//
//	r := models.Delete("phi-4-14b-q4.gguf")
//	if !r.OK { core.Warn("delete failed", "err", r.Error()) }
func Delete(name string) core.Result {
	if name == "" {
		return core.Fail(core.NewError("models.Delete: empty name"))
	}
	// Path-traversal guard: the name must be a bare basename. Slashes
	// + parent references are flat-out rejected before we even
	// resolve the path.
	if core.PathBase(name) != name {
		return core.Fail(core.NewError("models.Delete: name must be a basename (no slashes)"))
	}
	if name == "." || name == ".." {
		return core.Fail(core.NewError("models.Delete: invalid name"))
	}
	if core.PathExt(name) != ".gguf" {
		return core.Fail(core.NewError("models.Delete: only .gguf files are deletable today"))
	}

	dirR := paths.ModelsDir()
	if !dirR.OK {
		return dirR
	}
	dir := dirR.Value.(string)
	target := core.PathJoin(dir, name)

	// Defence-in-depth: even after the basename check, confirm the
	// final path's directory matches the models dir exactly. Catches
	// any future bug in PathBase / PathJoin / Clean.
	if core.PathDir(target) != dir {
		return core.Fail(core.NewError("models.Delete: resolved path escapes models directory"))
	}

	statR := core.Stat(target)
	if !statR.OK {
		return core.Fail(core.E("models.Delete", "target not found", statR.Value.(error)))
	}
	info, _ := statR.Value.(core.FsFileInfo)
	if info == nil || info.IsDir() {
		return core.Fail(core.NewError("models.Delete: target is not a regular file"))
	}

	if rmR := core.Remove(target); !rmR.OK {
		return core.Fail(core.E("models.Delete", "remove failed", rmR.Value.(error)))
	}
	return core.Ok(target)
}

// Rename renames a local .gguf model file in place. Both names go
// through the same path-traversal + extension guards as Delete:
// basename-only (no slashes), must end in .gguf, must resolve back
// into ModelsDir.
//
// Rules:
//   - source must exist and be a regular file
//   - destination must NOT exist (rename refuses to clobber)
//   - both names must be different — same-name rename fails fast so
//     the caller can surface a no-op rather than silently succeeding
//
// Usage example:
//
//	r := models.Rename("gemma-2b.gguf", "gemma-2b-prod.gguf")
//	if !r.OK { core.Warn("rename failed", "err", r.Error()) }
func Rename(oldName, newName string) core.Result {
	if oldName == "" || newName == "" {
		return core.Fail(core.NewError("models.Rename: empty name"))
	}
	if oldName == newName {
		return core.Fail(core.NewError("models.Rename: source and destination match"))
	}
	for _, name := range []string{oldName, newName} {
		if core.PathBase(name) != name {
			return core.Fail(core.NewError("models.Rename: name must be a basename (no slashes)"))
		}
		if name == "." || name == ".." {
			return core.Fail(core.NewError("models.Rename: invalid name"))
		}
		if core.PathExt(name) != ".gguf" {
			return core.Fail(core.NewError("models.Rename: only .gguf files are renameable today"))
		}
	}

	dirR := paths.ModelsDir()
	if !dirR.OK {
		return dirR
	}
	dir := dirR.Value.(string)
	src := core.PathJoin(dir, oldName)
	dst := core.PathJoin(dir, newName)

	// Defence-in-depth: confirm both resolved paths still live in dir.
	if core.PathDir(src) != dir || core.PathDir(dst) != dir {
		return core.Fail(core.NewError("models.Rename: resolved path escapes models directory"))
	}

	// Cerberus M3 / Mantis #1419 — TWO TOCTOU gaps closed.
	//
	// Gap 1 (symlink-source, shipped 71d796b): Stat follows symlinks; a
	// model directory containing `src.gguf -> /etc/passwd` would pass
	// the "source is a regular file" check (the target file IS a
	// regular file from Stat's perspective). Lstat sees the symlink
	// itself; we refuse it.
	//
	// Gap 2 (atomic refuse-to-clobber, this commit): Stat(dst) followed
	// by core.Rename(src, dst) has a window where a racing writer can
	// create dst between the two calls, causing rename(2) to silently
	// clobber. atomicRename uses link(2) + unlink(2) on POSIX —
	// link(2) atomically fails with EEXIST when dst already exists, so
	// the "is it there?" check IS the rename. See rename_unix.go for
	// the platform-tagged implementation; non-POSIX falls back to
	// core.Rename via rename_other.go (the residual on those platforms
	// stays until renameat2 RENAME_NOREPLACE or equivalent is exported
	// through CoreGO).
	srcLstatR := core.Lstat(src)
	if !srcLstatR.OK {
		return core.Fail(core.E("models.Rename", "source not found", srcLstatR.Value.(error)))
	}
	srcInfo, _ := srcLstatR.Value.(core.FsFileInfo)
	if srcInfo == nil {
		return core.Fail(core.NewError("models.Rename: source stat returned nil"))
	}
	if srcInfo.IsDir() {
		return core.Fail(core.NewError("models.Rename: source is not a regular file"))
	}
	// ModeSymlink bit set means Lstat returned a symlink — refuse.
	if srcInfo.Mode()&core.ModeSymlink != 0 {
		return core.Fail(core.NewError("models.Rename: source is a symlink — refused"))
	}

	return atomicRename(src, dst)
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

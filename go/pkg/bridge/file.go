// SPDX-Licence-Identifier: EUPL-1.2

// file.go — bridge tools for filesystem ops. Wraps CoreGO file
// primitives (core.ReadFile / WriteFile / MkdirAll / RemoveAll /
// Stat / ReadDir / DirFS) so the agent can poke at the user's
// filesystem within the same trust boundary as the host process.
//
// No path-allowlisting at this layer — the bridge is loopback-only
// and (per RFC) dev-mode-focused. Production sandboxing is a
// separate concern handled by the build-tag separation work.

package bridge

import (
	core "dappco.re/go"
)

// toolFileRead returns the file's contents as a string.
// params: { path }
func (s *Service) toolFileRead(params map[string]any) map[string]any {
	path := paramString(params, "path", "")
	if path == "" {
		return map[string]any{"ok": false, "error": "path param required"}
	}
	r := core.ReadFile(path)
	if !r.OK {
		return map[string]any{"ok": false, "error": r.Error()}
	}
	bytes, _ := r.Value.([]byte)
	return map[string]any{"ok": true, "value": string(bytes), "size": len(bytes)}
}

// toolFileWrite writes content to a file (creating parent dirs).
// params: { path, content, mode? }
func (s *Service) toolFileWrite(params map[string]any) map[string]any {
	path := paramString(params, "path", "")
	if path == "" {
		return map[string]any{"ok": false, "error": "path param required"}
	}
	content := paramString(params, "content", "")
	mode := core.FileMode(paramInt(params, "mode", 0o644))
	// Ensure parent exists; ignore "already exists" errors.
	dir := core.PathDir(path)
	if dir != "" && dir != "." && dir != "/" {
		if r := core.MkdirAll(dir, 0o755); !r.OK {
			return map[string]any{"ok": false, "error": "mkdir parent: " + r.Error()}
		}
	}
	if r := core.WriteFile(path, []byte(content), mode); !r.OK {
		return map[string]any{"ok": false, "error": r.Error()}
	}
	return map[string]any{"ok": true, "path": path, "bytes": len(content)}
}

// toolFileEdit reads → replaces every occurrence → writes.
// core.Replace is ReplaceAll-style, so this always replaces all
// matches; mirrors core/ide's behaviour. params: { path, find, replace }
func (s *Service) toolFileEdit(params map[string]any) map[string]any {
	path := paramString(params, "path", "")
	find := paramString(params, "find", "")
	replace := paramString(params, "replace", "")
	if path == "" || find == "" {
		return map[string]any{"ok": false, "error": "path and find params required"}
	}
	r := core.ReadFile(path)
	if !r.OK {
		return map[string]any{"ok": false, "error": "read: " + r.Error()}
	}
	bytes, _ := r.Value.([]byte)
	body := string(bytes)
	if !core.Contains(body, find) {
		return map[string]any{"ok": false, "error": "find string not present in file"}
	}
	out := core.Replace(body, find, replace)
	if r := core.WriteFile(path, []byte(out), 0o644); !r.OK {
		return map[string]any{"ok": false, "error": "write: " + r.Error()}
	}
	return map[string]any{"ok": true, "path": path, "before_size": len(body), "after_size": len(out)}
}

// toolFileDelete removes a file. params: { path }
func (s *Service) toolFileDelete(params map[string]any) map[string]any {
	path := paramString(params, "path", "")
	if path == "" {
		return map[string]any{"ok": false, "error": "path param required"}
	}
	if r := core.RemoveAll(path); !r.OK {
		return map[string]any{"ok": false, "error": r.Error()}
	}
	return map[string]any{"ok": true, "deleted": path}
}

// toolFileExists reports whether the path resolves. params: { path }
func (s *Service) toolFileExists(params map[string]any) map[string]any {
	path := paramString(params, "path", "")
	if path == "" {
		return map[string]any{"ok": false, "error": "path param required"}
	}
	stat := core.Stat(path)
	if !stat.OK {
		return map[string]any{"ok": true, "exists": false}
	}
	info, _ := stat.Value.(interface{ IsDir() bool })
	isDir := info != nil && info.IsDir()
	return map[string]any{"ok": true, "exists": true, "is_dir": isDir, "path": path}
}

// toolFileRename moves/renames a file. params: { from, to }
func (s *Service) toolFileRename(params map[string]any) map[string]any {
	from := paramString(params, "from", "")
	to := paramString(params, "to", "")
	if from == "" || to == "" {
		return map[string]any{"ok": false, "error": "from and to params required"}
	}
	if r := core.Rename(from, to); !r.OK {
		return map[string]any{"ok": false, "error": r.Error()}
	}
	return map[string]any{"ok": true, "from": from, "to": to}
}

// toolDirList enumerates a directory's entries.
// params: { path, recursive? }
func (s *Service) toolDirList(params map[string]any) map[string]any {
	path := paramString(params, "path", "")
	if path == "" {
		return map[string]any{"ok": false, "error": "path param required"}
	}
	listing := core.ReadDir(core.DirFS(path), ".")
	if !listing.OK {
		return map[string]any{"ok": false, "error": listing.Error()}
	}
	entries, _ := listing.Value.([]core.FsDirEntry)
	out := make([]map[string]any, 0, len(entries))
	for _, e := range entries {
		out = append(out, map[string]any{
			"name":   e.Name(),
			"is_dir": e.IsDir(),
		})
	}
	return map[string]any{"ok": true, "value": out, "count": len(out), "path": path}
}

// toolDirCreate makes a directory tree. params: { path, mode? }
func (s *Service) toolDirCreate(params map[string]any) map[string]any {
	path := paramString(params, "path", "")
	if path == "" {
		return map[string]any{"ok": false, "error": "path param required"}
	}
	mode := core.FileMode(paramInt(params, "mode", 0o755))
	if r := core.MkdirAll(path, mode); !r.OK {
		return map[string]any{"ok": false, "error": r.Error()}
	}
	return map[string]any{"ok": true, "created": path}
}

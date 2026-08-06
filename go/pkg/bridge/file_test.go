// SPDX-Licence-Identifier: EUPL-1.2

// file.go tests — every tool here is a thin wrapper over CoreGO's
// filesystem primitives with no Core/Wails dependency, so these
// tests drive the real filesystem inside t.TempDir() rather than
// mocking anything. Tests live in package bridge (not bridge_test)
// because every toolFile*/toolDir* symbol is unexported.

package bridge

import (
	core "dappco.re/go"
)

// ─── toolFileRead ───────────────────────────────────────────────────

func TestFile_ToolFileRead_Good(t *core.T) {
	s := &Service{}
	dir := t.TempDir()
	path := core.PathJoin(dir, "hello.txt")
	core.AssertTrue(t, core.WriteFile(path, []byte("hello world"), 0o644).OK)

	resp := s.toolFileRead(map[string]any{"path": path})
	core.AssertEqual(t, true, resp["ok"])
	core.AssertEqual(t, "hello world", resp["value"])
	core.AssertEqual(t, 11, resp["size"])
}

func TestFile_ToolFileRead_Bad_MissingPathParam(t *core.T) {
	s := &Service{}
	resp := s.toolFileRead(map[string]any{})
	core.AssertEqual(t, false, resp["ok"])
	core.AssertEqual(t, pathParamRequired, resp["error"])
}

func TestFile_ToolFileRead_Ugly_NonexistentFile(t *core.T) {
	s := &Service{}
	dir := t.TempDir()
	resp := s.toolFileRead(map[string]any{"path": core.PathJoin(dir, "nope.txt")})
	core.AssertEqual(t, false, resp["ok"])
	core.AssertNotEmpty(t, resp["error"])
}

// ─── toolFileWrite ──────────────────────────────────────────────────

func TestFile_ToolFileWrite_Good(t *core.T) {
	s := &Service{}
	dir := t.TempDir()
	path := core.PathJoin(dir, "nested", "out.txt")
	resp := s.toolFileWrite(map[string]any{"path": path, "content": "payload"})
	core.AssertEqual(t, true, resp["ok"])
	core.AssertEqual(t, 7, resp["bytes"])

	read := core.ReadFile(path)
	core.AssertTrue(t, read.OK)
	core.AssertEqual(t, "payload", string(read.Value.([]byte)))
}

func TestFile_ToolFileWrite_Bad_MissingPathParam(t *core.T) {
	s := &Service{}
	resp := s.toolFileWrite(map[string]any{"content": "x"})
	core.AssertEqual(t, false, resp["ok"])
	core.AssertEqual(t, pathParamRequired, resp["error"])
}

func TestFile_ToolFileWrite_Ugly_ParentIsAFile(t *core.T) {
	// Real fault injection: "parent" already exists as a regular file,
	// so MkdirAll must fail cleanly rather than panic.
	s := &Service{}
	dir := t.TempDir()
	blocker := core.PathJoin(dir, "blocker")
	core.AssertTrue(t, core.WriteFile(blocker, []byte("x"), 0o644).OK)
	resp := s.toolFileWrite(map[string]any{"path": core.PathJoin(blocker, "child.txt"), "content": "y"})
	core.AssertEqual(t, false, resp["ok"])
	core.AssertContains(t, resp["error"], "mkdir parent")
}

// ─── toolFileEdit ───────────────────────────────────────────────────

func TestFile_ToolFileEdit_Good(t *core.T) {
	s := &Service{}
	dir := t.TempDir()
	path := core.PathJoin(dir, "edit.txt")
	core.AssertTrue(t, core.WriteFile(path, []byte("foo bar foo"), 0o644).OK)

	resp := s.toolFileEdit(map[string]any{"path": path, "find": "foo", "replace": "baz"})
	core.AssertEqual(t, true, resp["ok"])

	read := core.ReadFile(path)
	core.AssertEqual(t, "baz bar baz", string(read.Value.([]byte)))
}

func TestFile_ToolFileEdit_Bad_MissingFindParam(t *core.T) {
	s := &Service{}
	resp := s.toolFileEdit(map[string]any{"path": "/tmp/whatever"})
	core.AssertEqual(t, false, resp["ok"])
	core.AssertContains(t, resp["error"], "required")
}

func TestFile_ToolFileEdit_Ugly_FindStringNotPresent(t *core.T) {
	s := &Service{}
	dir := t.TempDir()
	path := core.PathJoin(dir, "edit2.txt")
	core.AssertTrue(t, core.WriteFile(path, []byte("nothing to see"), 0o644).OK)

	resp := s.toolFileEdit(map[string]any{"path": path, "find": "missing", "replace": "x"})
	core.AssertEqual(t, false, resp["ok"])
	core.AssertEqual(t, "find string not present in file", resp["error"])
}

func TestFile_ToolFileEdit_Ugly_UnreadableFile(t *core.T) {
	s := &Service{}
	dir := t.TempDir()
	resp := s.toolFileEdit(map[string]any{"path": core.PathJoin(dir, "absent.txt"), "find": "a", "replace": "b"})
	core.AssertEqual(t, false, resp["ok"])
	core.AssertContains(t, resp["error"], "read:")
}

// ─── toolFileDelete ─────────────────────────────────────────────────

func TestFile_ToolFileDelete_Good(t *core.T) {
	s := &Service{}
	dir := t.TempDir()
	path := core.PathJoin(dir, "gone.txt")
	core.AssertTrue(t, core.WriteFile(path, []byte("x"), 0o644).OK)

	resp := s.toolFileDelete(map[string]any{"path": path})
	core.AssertEqual(t, true, resp["ok"])
	core.AssertFalse(t, core.Stat(path).OK)
}

func TestFile_ToolFileDelete_Bad_MissingPathParam(t *core.T) {
	s := &Service{}
	resp := s.toolFileDelete(map[string]any{})
	core.AssertEqual(t, false, resp["ok"])
	core.AssertEqual(t, pathParamRequired, resp["error"])
}

// ─── toolFileExists ─────────────────────────────────────────────────

func TestFile_ToolFileExists_Good_FileExists(t *core.T) {
	s := &Service{}
	dir := t.TempDir()
	path := core.PathJoin(dir, "here.txt")
	core.AssertTrue(t, core.WriteFile(path, []byte("x"), 0o644).OK)

	resp := s.toolFileExists(map[string]any{"path": path})
	core.AssertEqual(t, true, resp["ok"])
	core.AssertEqual(t, true, resp["exists"])
	core.AssertEqual(t, false, resp["is_dir"])
}

func TestFile_ToolFileExists_Good_DirExists(t *core.T) {
	s := &Service{}
	dir := t.TempDir()
	resp := s.toolFileExists(map[string]any{"path": dir})
	core.AssertEqual(t, true, resp["ok"])
	core.AssertEqual(t, true, resp["exists"])
	core.AssertEqual(t, true, resp["is_dir"])
}

func TestFile_ToolFileExists_Bad_MissingPathParam(t *core.T) {
	s := &Service{}
	resp := s.toolFileExists(map[string]any{})
	core.AssertEqual(t, false, resp["ok"])
	core.AssertEqual(t, pathParamRequired, resp["error"])
}

func TestFile_ToolFileExists_Ugly_Nonexistent(t *core.T) {
	s := &Service{}
	dir := t.TempDir()
	resp := s.toolFileExists(map[string]any{"path": core.PathJoin(dir, "nope")})
	core.AssertEqual(t, true, resp["ok"])
	core.AssertEqual(t, false, resp["exists"])
}

// ─── toolFileRename ─────────────────────────────────────────────────

func TestFile_ToolFileRename_Good(t *core.T) {
	s := &Service{}
	dir := t.TempDir()
	from := core.PathJoin(dir, "a.txt")
	to := core.PathJoin(dir, "b.txt")
	core.AssertTrue(t, core.WriteFile(from, []byte("x"), 0o644).OK)

	resp := s.toolFileRename(map[string]any{"from": from, "to": to})
	core.AssertEqual(t, true, resp["ok"])
	core.AssertFalse(t, core.Stat(from).OK)
	core.AssertTrue(t, core.Stat(to).OK)
}

func TestFile_ToolFileRename_Bad_MissingParams(t *core.T) {
	s := &Service{}
	resp := s.toolFileRename(map[string]any{"from": "/tmp/a"})
	core.AssertEqual(t, false, resp["ok"])
	core.AssertContains(t, resp["error"], "required")
}

func TestFile_ToolFileRename_Ugly_SourceMissing(t *core.T) {
	s := &Service{}
	dir := t.TempDir()
	resp := s.toolFileRename(map[string]any{
		"from": core.PathJoin(dir, "absent.txt"),
		"to":   core.PathJoin(dir, "target.txt"),
	})
	core.AssertEqual(t, false, resp["ok"])
	core.AssertNotEmpty(t, resp["error"])
}

// ─── toolDirList ────────────────────────────────────────────────────

func TestFile_ToolDirList_Good(t *core.T) {
	s := &Service{}
	dir := t.TempDir()
	core.AssertTrue(t, core.WriteFile(core.PathJoin(dir, "a.txt"), []byte("x"), 0o644).OK)
	core.AssertTrue(t, core.MkdirAll(core.PathJoin(dir, "sub"), 0o755).OK)

	resp := s.toolDirList(map[string]any{"path": dir})
	core.AssertEqual(t, true, resp["ok"])
	core.AssertEqual(t, 2, resp["count"])
}

func TestFile_ToolDirList_Bad_MissingPathParam(t *core.T) {
	s := &Service{}
	resp := s.toolDirList(map[string]any{})
	core.AssertEqual(t, false, resp["ok"])
	core.AssertEqual(t, pathParamRequired, resp["error"])
}

func TestFile_ToolDirList_Ugly_Nonexistent(t *core.T) {
	s := &Service{}
	dir := t.TempDir()
	resp := s.toolDirList(map[string]any{"path": core.PathJoin(dir, "absent")})
	core.AssertEqual(t, false, resp["ok"])
	core.AssertNotEmpty(t, resp["error"])
}

// ─── toolDirCreate ──────────────────────────────────────────────────

func TestFile_ToolDirCreate_Good(t *core.T) {
	s := &Service{}
	dir := t.TempDir()
	target := core.PathJoin(dir, "a", "b", "c")
	resp := s.toolDirCreate(map[string]any{"path": target})
	core.AssertEqual(t, true, resp["ok"])
	stat := core.Stat(target)
	core.AssertTrue(t, stat.OK)
	info := stat.Value.(core.FsFileInfo)
	core.AssertTrue(t, info.IsDir())
}

func TestFile_ToolDirCreate_Bad_MissingPathParam(t *core.T) {
	s := &Service{}
	resp := s.toolDirCreate(map[string]any{})
	core.AssertEqual(t, false, resp["ok"])
	core.AssertEqual(t, pathParamRequired, resp["error"])
}

func TestFile_ToolDirCreate_Ugly_PathIsAFile(t *core.T) {
	s := &Service{}
	dir := t.TempDir()
	blocker := core.PathJoin(dir, "blocker")
	core.AssertTrue(t, core.WriteFile(blocker, []byte("x"), 0o644).OK)
	resp := s.toolDirCreate(map[string]any{"path": core.PathJoin(blocker, "child")})
	core.AssertEqual(t, false, resp["ok"])
	core.AssertNotEmpty(t, resp["error"])
}

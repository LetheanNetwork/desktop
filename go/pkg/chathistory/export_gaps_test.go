// SPDX-License-Identifier: EUPL-1.2

// Coverage-gap tests for export.go. chathistory_test.go's TestRoundtrip
// already proves the CopyTo and ExportJSONL happy paths; this file
// drives their nil / empty-arg guards and the reachable real-fault
// branches: a closed connection (checkpoint / query fail), a file
// removed out from under an open DuckDB handle (open-source fail), a
// blocked destination directory (mkdir fail), a destination path that
// collides with an existing directory (create fail), and a corrupted
// tags column (decode-failure logging path).
//
// Left out, traced not guessed: CopyTo's "copy bytes" / "close dest"
// failures and ExportJSONL's per-row scan/marshal/write/close-on-
// success failures. Every technique tried (closed db, removed file,
// blocked dir, read-only dir/file, corrupt file) resolves at Open/
// Checkpoint/query time before reaching those statements — DuckDB's
// driver validates the connection eagerly, so there's no hermetic way
// to get a live, working *sql.DB into a state where an individual
// mid-stream Exec/Write/Close fails without faking the driver or a
// production seam, which the house rules for this pass rule out.

package chathistory

import (
	"os"
	"path/filepath"
	"testing"
)

// --- CopyTo ----------------------------------------------------------------

func TestExport_CopyTo_Bad_NilReceiver(t *testing.T) {
	var h *History
	if err := h.CopyTo("dest.duckdb"); err == nil {
		t.Fatal("CopyTo on nil receiver: want error, got nil")
	}
}

func TestExport_CopyTo_Bad_EmptyDest(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "chats.duckdb")
	h, err := Open("snider", path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer h.Close()

	if err := h.CopyTo(""); err == nil {
		t.Fatal("CopyTo with empty dest: want error, got nil")
	}
}

func TestExport_CopyTo_Bad_ClosedHandleFailsCheckpoint(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "chats.duckdb")
	h, err := Open("snider", path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer h.Close()
	h.db.Close()

	if err := h.CopyTo(filepath.Join(dir, "out.duckdb")); err == nil {
		t.Fatal("CopyTo on a closed db: want checkpoint error, got nil")
	}
}

// TestExport_CopyTo_Bad_SourceFileRemoved lets CHECKPOINT succeed
// (DuckDB holds its own fd) but removes the path from the filesystem
// namespace first, so core.Open(h.path) fails — a real "the file
// vanished under us" fault distinct from the closed-handle case.
func TestExport_CopyTo_Bad_SourceFileRemoved(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "chats.duckdb")
	h, err := Open("snider", path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer h.Close()
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}

	if err := h.CopyTo(filepath.Join(dir, "out.duckdb")); err == nil {
		t.Fatal("CopyTo with source file removed: want error, got nil")
	}
}

func TestExport_CopyTo_Bad_MkdirDestParentBlocked(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "chats.duckdb")
	h, err := Open("snider", path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer h.Close()

	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(blocker, "sub", "out.duckdb")

	if err := h.CopyTo(dest); err == nil {
		t.Fatal("CopyTo with blocked dest parent: want error, got nil")
	}
}

func TestExport_CopyTo_Bad_CreateDestIsDirectory(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "chats.duckdb")
	h, err := Open("snider", path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer h.Close()

	destDir := filepath.Join(dir, "out-is-a-dir")
	if err := os.Mkdir(destDir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := h.CopyTo(destDir); err == nil {
		t.Fatal("CopyTo with dest = existing directory: want error, got nil")
	}
}

// --- ExportJSONL -------------------------------------------------------

func TestExport_ExportJSONL_Bad_NilReceiver(t *testing.T) {
	var h *History
	if err := h.ExportJSONL("dest.jsonl"); err == nil {
		t.Fatal("ExportJSONL on nil receiver: want error, got nil")
	}
}

func TestExport_ExportJSONL_Bad_EmptyDest(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "chats.duckdb")
	h, err := Open("snider", path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer h.Close()

	if err := h.ExportJSONL(""); err == nil {
		t.Fatal("ExportJSONL with empty dest: want error, got nil")
	}
}

func TestExport_ExportJSONL_Bad_MkdirDestParentBlocked(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "chats.duckdb")
	h, err := Open("snider", path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer h.Close()

	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(blocker, "sub", "out.jsonl")

	if err := h.ExportJSONL(dest); err == nil {
		t.Fatal("ExportJSONL with blocked dest parent: want error, got nil")
	}
}

func TestExport_ExportJSONL_Bad_CreateDestIsDirectory(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "chats.duckdb")
	h, err := Open("snider", path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer h.Close()

	destDir := filepath.Join(dir, "out-is-a-dir")
	if err := os.Mkdir(destDir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := h.ExportJSONL(destDir); err == nil {
		t.Fatal("ExportJSONL with dest = existing directory: want error, got nil")
	}
}

func TestExport_ExportJSONL_Bad_ClosedHandleFailsQuery(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "chats.duckdb")
	h, err := Open("snider", path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer h.Close()
	h.db.Close()

	if err := h.ExportJSONL(filepath.Join(dir, "out.jsonl")); err == nil {
		t.Fatal("ExportJSONL on a closed db: want query error, got nil")
	}
}

// TestExport_ExportJSONL_Good_CorruptTagsLogsAndContinues corrupts
// the tags column directly via SQL (bypassing StartConversation's
// normal JSON-marshalled write) to drive the tags-decode-failure
// branch: real garbage data, not a guessed error. The export must
// still succeed — partial export beats refusing to ship anything.
// The turn also carries ToolCalls/ToolResults — TestRoundtrip's
// turns never set those, so the sql.NullString.Valid branches for
// tool_calls/tool_results in the per-turn scan loop were otherwise
// unreached.
func TestExport_ExportJSONL_Good_CorruptTagsLogsAndContinues(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "chats.duckdb")
	h, err := Open("snider", path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer h.Close()

	convID, err := h.StartConversation(NewConversation{ModelID: "lemer-lite"})
	if err != nil {
		t.Fatalf("StartConversation: %v", err)
	}
	if _, err := h.db.Exec(`UPDATE conversations SET tags = ? WHERE id = ?`, "{not valid json", convID); err != nil {
		t.Fatalf("corrupt tags column: %v", err)
	}
	if _, err := h.WriteTurn(convID, NewTurn{
		Role:        "assistant",
		Content:     "calling a tool",
		ToolCalls:   []byte(`[{"name":"search"}]`),
		ToolResults: []byte(`[{"result":"ok"}]`),
	}); err != nil {
		t.Fatalf("WriteTurn: %v", err)
	}

	dest := filepath.Join(dir, "out.jsonl")
	if err := h.ExportJSONL(dest); err != nil {
		t.Fatalf("ExportJSONL with corrupt tags: want partial success, got error: %v", err)
	}
	if _, err := os.Stat(dest); err != nil {
		t.Fatalf("expected export file to exist: %v", err)
	}
}

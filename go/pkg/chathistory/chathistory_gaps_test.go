// SPDX-License-Identifier: EUPL-1.2

// Coverage-gap tests for chathistory.go. chathistory_test.go already
// proves the roundtrip (Open → Start → Write → End → Count → Export);
// this file drives the branches that roundtrip never touches: the nil
// / closed-handle guards on every method, Open's failure branches
// (blocked mkdir, corrupt file), LoadTurns end to end, the getters,
// and real DB-level faults (closed connection, FK violation) instead
// of guessed error strings.

package chathistory

import (
	"os"
	"path/filepath"
	"testing"
)

// --- Open --------------------------------------------------------------

// TestChathistory_Open_Bad_MkdirParentBlocked drives Open's "mkdir
// parent" failure branch: the parent directory can't be created
// because a regular file already occupies that path segment.
func TestChathistory_Open_Bad_MkdirParentBlocked(t *testing.T) {
	dir := t.TempDir()
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(blocker, "sub", "chats.duckdb")

	_, err := Open("snider", path)
	if err == nil {
		t.Fatal("Open with blocked mkdir parent: want error, got nil")
	}
}

// TestChathistory_Open_Bad_CorruptFile drives Open's "open duckdb"
// failure branch: a file already exists at path but isn't a valid
// DuckDB database — real fault injection (garbage bytes), not a
// simulated error string.
func TestChathistory_Open_Bad_CorruptFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "corrupt.duckdb")
	if err := os.WriteFile(path, []byte("not a real duckdb file"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := Open("snider", path)
	if err == nil {
		t.Fatal("Open on a corrupt file: want error, got nil")
	}
}

// --- Close ---------------------------------------------------------------

func TestChathistory_Close_Good_NilReceiver(t *testing.T) {
	var h *History
	if err := h.Close(); err != nil {
		t.Fatalf("Close on nil receiver: want nil, got %v", err)
	}
}

func TestChathistory_Close_Good_NilDB(t *testing.T) {
	h := &History{userID: "snider", path: "unused"}
	if err := h.Close(); err != nil {
		t.Fatalf("Close with nil db: want nil, got %v", err)
	}
}

// --- Path / UserID getters -----------------------------------------------

func TestChathistory_Path_Good(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "chats.duckdb")
	h, err := Open("snider", path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer h.Close()

	if got := h.Path(); got != path {
		t.Fatalf("Path: got %q want %q", got, path)
	}
}

func TestChathistory_UserID_Good(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "chats.duckdb")
	h, err := Open("snider", path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer h.Close()

	if got := h.UserID(); got != "snider" {
		t.Fatalf("UserID: got %q want %q", got, "snider")
	}
}

// --- StartConversation -----------------------------------------------------

func TestChathistory_StartConversation_Bad_NilReceiver(t *testing.T) {
	var h *History
	if _, err := h.StartConversation(NewConversation{ModelID: "x"}); err == nil {
		t.Fatal("StartConversation on nil receiver: want error, got nil")
	}
}

func TestChathistory_StartConversation_Bad_ClosedHandle(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "chats.duckdb")
	h, err := Open("snider", path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer h.Close()
	h.db.Close()

	if _, err := h.StartConversation(NewConversation{ModelID: "x"}); err == nil {
		t.Fatal("StartConversation on a closed db: want error, got nil")
	}
}

// TestChathistory_StartConversation_Good_WithMetadata exercises the
// Metadata-present branch (roundtrip in chathistory_test.go never
// sets Metadata, only Tags).
func TestChathistory_StartConversation_Good_WithMetadata(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "chats.duckdb")
	h, err := Open("snider", path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer h.Close()

	convID, err := h.StartConversation(NewConversation{
		ModelID:  "lemer-lite",
		Tags:     []string{"life"},
		Metadata: []byte(`{"source":"test"}`),
	})
	if err != nil {
		t.Fatalf("StartConversation: %v", err)
	}
	if convID == "" {
		t.Fatal("StartConversation returned empty id")
	}
}

// --- WriteTurn -------------------------------------------------------------

func TestChathistory_WriteTurn_Bad_NilReceiver(t *testing.T) {
	var h *History
	if _, err := h.WriteTurn("conv", NewTurn{Role: "user", Content: "x"}); err == nil {
		t.Fatal("WriteTurn on nil receiver: want error, got nil")
	}
}

// TestChathistory_WriteTurn_Bad_OrdinalLookupFails closes the db
// before the ordinal SELECT runs, driving WriteTurn's "ordinal
// lookup" failure branch specifically (distinct from the later
// insert branch).
func TestChathistory_WriteTurn_Bad_OrdinalLookupFails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "chats.duckdb")
	h, err := Open("snider", path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer h.Close()
	h.db.Close()

	if _, err := h.WriteTurn("conv", NewTurn{Role: "user", Content: "x"}); err == nil {
		t.Fatal("WriteTurn on a closed db: want error, got nil")
	}
}

// TestChathistory_WriteTurn_Bad_ForeignKeyViolation drives WriteTurn's
// insert-failure branch specifically (as opposed to the ordinal
// lookup branch above): the ordinal SELECT succeeds (no matching
// rows, so COALESCE gives 0) but the INSERT fails on the real
// conversation_id foreign key constraint — a genuine DB-level fault,
// not a guessed error string.
func TestChathistory_WriteTurn_Bad_ForeignKeyViolation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "chats.duckdb")
	h, err := Open("snider", path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer h.Close()

	if _, err := h.WriteTurn("no-such-conversation", NewTurn{Role: "user", Content: "x"}); err == nil {
		t.Fatal("WriteTurn against a nonexistent conversation: want FK error, got nil")
	}
}

// TestChathistory_WriteTurn_Good_WithToolCallsAndResults drives
// nullableJSON's non-empty branch (roundtrip never sets these).
func TestChathistory_WriteTurn_Good_WithToolCallsAndResults(t *testing.T) {
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
	_, err = h.WriteTurn(convID, NewTurn{
		Role:        "assistant",
		Content:     "calling a tool",
		ToolCalls:   []byte(`[{"name":"search"}]`),
		ToolResults: []byte(`[{"result":"ok"}]`),
	})
	if err != nil {
		t.Fatalf("WriteTurn with tool calls/results: %v", err)
	}
}

// --- EndConversation ---------------------------------------------------

func TestChathistory_EndConversation_Bad_NilReceiver(t *testing.T) {
	var h *History
	if err := h.EndConversation("conv"); err == nil {
		t.Fatal("EndConversation on nil receiver: want error, got nil")
	}
}

func TestChathistory_EndConversation_Bad_ClosedHandle(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "chats.duckdb")
	h, err := Open("snider", path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer h.Close()
	h.db.Close()

	if err := h.EndConversation("conv"); err == nil {
		t.Fatal("EndConversation on a closed db: want error, got nil")
	}
}

// --- SetSignal -----------------------------------------------------------

func TestChathistory_SetSignal_Bad_NilReceiver(t *testing.T) {
	var h *History
	if err := h.SetSignal("turn", "liked"); err == nil {
		t.Fatal("SetSignal on nil receiver: want error, got nil")
	}
}

func TestChathistory_SetSignal_Bad_ClosedHandle(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "chats.duckdb")
	h, err := Open("snider", path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer h.Close()
	h.db.Close()

	if err := h.SetSignal("turn", "liked"); err == nil {
		t.Fatal("SetSignal on a closed db: want error, got nil")
	}
}

// --- CountConversations ----------------------------------------------------

func TestChathistory_CountConversations_Bad_NilReceiver(t *testing.T) {
	var h *History
	if _, err := h.CountConversations(); err == nil {
		t.Fatal("CountConversations on nil receiver: want error, got nil")
	}
}

func TestChathistory_CountConversations_Bad_ClosedHandle(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "chats.duckdb")
	h, err := Open("snider", path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer h.Close()
	h.db.Close()

	if _, err := h.CountConversations(); err == nil {
		t.Fatal("CountConversations on a closed db: want error, got nil")
	}
}

// --- LoadTurns -------------------------------------------------------------

// TestChathistory_LoadTurns_Good is the one real gap: nothing in the
// suite calls LoadTurns before this. Writes three turns and confirms
// they come back in ordinal order with the right shape.
func TestChathistory_LoadTurns_Good(t *testing.T) {
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
	want := []NewTurn{
		{Role: "user", Content: "hey"},
		{Role: "assistant", Content: "hi there"},
		{Role: "user", Content: "bye"},
	}
	for i, nt := range want {
		if _, err := h.WriteTurn(convID, nt); err != nil {
			t.Fatalf("WriteTurn[%d]: %v", i, err)
		}
	}

	turns, err := h.LoadTurns(convID)
	if err != nil {
		t.Fatalf("LoadTurns: %v", err)
	}
	if len(turns) != len(want) {
		t.Fatalf("LoadTurns: got %d turns want %d", len(turns), len(want))
	}
	for i, turn := range turns {
		if turn.Ordinal != i {
			t.Fatalf("turn[%d].Ordinal: got %d want %d", i, turn.Ordinal, i)
		}
		if turn.Role != want[i].Role || turn.Content != want[i].Content {
			t.Fatalf("turn[%d]: got (%q,%q) want (%q,%q)", i, turn.Role, turn.Content, want[i].Role, want[i].Content)
		}
	}
}

func TestChathistory_LoadTurns_Bad_NilReceiver(t *testing.T) {
	var h *History
	if _, err := h.LoadTurns("conv"); err == nil {
		t.Fatal("LoadTurns on nil receiver: want error, got nil")
	}
}

func TestChathistory_LoadTurns_Bad_EmptyConversationID(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "chats.duckdb")
	h, err := Open("snider", path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer h.Close()

	if _, err := h.LoadTurns(""); err == nil {
		t.Fatal("LoadTurns with empty conversation id: want error, got nil")
	}
}

func TestChathistory_LoadTurns_Bad_ClosedHandle(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "chats.duckdb")
	h, err := Open("snider", path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer h.Close()
	h.db.Close()

	if _, err := h.LoadTurns("conv"); err == nil {
		t.Fatal("LoadTurns on a closed db: want error, got nil")
	}
}

// --- CountTurns --------------------------------------------------------

func TestChathistory_CountTurns_Bad_NilReceiver(t *testing.T) {
	var h *History
	if _, err := h.CountTurns(); err == nil {
		t.Fatal("CountTurns on nil receiver: want error, got nil")
	}
}

func TestChathistory_CountTurns_Bad_ClosedHandle(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "chats.duckdb")
	h, err := Open("snider", path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer h.Close()
	h.db.Close()

	if _, err := h.CountTurns(); err == nil {
		t.Fatal("CountTurns on a closed db: want error, got nil")
	}
}

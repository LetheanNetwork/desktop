// SPDX-License-Identifier: EUPL-1.2

// bench_test.go — read-lane instrument for pkg/chathistory (perf
// campaign, lane/perf-c). Targets the two read paths a chat pane
// actually exercises at a realistic 100-record collection size:
//
//   - BenchmarkLoadTurns_100 — the message-page read (LoadTurns),
//     called repeatedly against a long-lived History handle the way a
//     GUI pane re-issues it every time it (re)opens or polls.
//
//   - BenchmarkExportJSONL_100Conversations — the only "list N
//     conversations" read path that exists in production code today
//     (export.go). Its inner loop issues one fresh db.Query per
//     conversation for the turns page — a per-record cost that
//     multiplies with collection size (the exact trap this campaign
//     is hunting for).
//
//   - BenchmarkTurnsQuery_RePrepare vs _PreparedStmt — isolates
//     whether database/sql's implicit prepare-per-Query call (the
//     shape ExportJSONL's inner loop and the pre-fix LoadTurns both
//     used) costs anything measurable against the DuckDB driver, so
//     the fix is evidence-backed rather than assumed.
//
// Hermetic via t.TempDir fixtures, -benchmem, steady-state confirmed
// over -benchtime=20x.
package chathistory

import (
	"database/sql"
	"path/filepath"
	"testing"
)

// seedConversationsWithTurns creates n conversations, each with
// turnsPer turns — the fixture ExportJSONL's list+page read walks.
func seedConversationsWithTurns(b *testing.B, h *History, n, turnsPer int) {
	b.Helper()
	for i := 0; i < n; i++ {
		convID, err := h.StartConversation(NewConversation{
			Title:     "bench conversation",
			ModelID:   "lemer-lite",
			BaseModel: "gemma-4-e2b-it-4bit",
			Tags:      []string{"bench", "fixture"},
		})
		if err != nil {
			b.Fatalf("seedConversationsWithTurns: StartConversation[%d]: %v", i, err)
		}
		for j := 0; j < turnsPer; j++ {
			if _, err := h.WriteTurn(convID, NewTurn{
				Role:    "user",
				Content: "bench message content, representative length for a chat turn",
			}); err != nil {
				b.Fatalf("seedConversationsWithTurns: WriteTurn[%d][%d]: %v", i, j, err)
			}
		}
	}
}

// seedTurns creates one conversation with n turns — the message-page
// fixture LoadTurns reads.
func seedTurns(b *testing.B, h *History, n int) string {
	b.Helper()
	convID, err := h.StartConversation(NewConversation{ModelID: "lemer-lite"})
	if err != nil {
		b.Fatalf("seedTurns: StartConversation: %v", err)
	}
	for i := 0; i < n; i++ {
		if _, err := h.WriteTurn(convID, NewTurn{
			Role:    "user",
			Content: "bench message content, representative length for a chat turn",
		}); err != nil {
			b.Fatalf("seedTurns: WriteTurn[%d]: %v", i, err)
		}
	}
	return convID
}

func openBenchHistory(b *testing.B) *History {
	b.Helper()
	dir := b.TempDir()
	h, err := Open("bench-user", filepath.Join(dir, "chats.duckdb"))
	if err != nil {
		b.Fatalf("Open: %v", err)
	}
	b.Cleanup(func() { _ = h.Close() })
	return h
}

// BenchmarkLoadTurns_100 — the message-page read: one conversation,
// 100 turns, repeated LoadTurns calls (the shape a chat pane re-issues
// every time it (re)opens or polls) against the SAME long-lived
// History handle.
func BenchmarkLoadTurns_100(b *testing.B) {
	h := openBenchHistory(b)
	convID := seedTurns(b, h, 100)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		turns, err := h.LoadTurns(convID)
		if err != nil {
			b.Fatalf("LoadTurns: %v", err)
		}
		if len(turns) != 100 {
			b.Fatalf("LoadTurns: got %d turns, want 100", len(turns))
		}
	}
}

// BenchmarkExportJSONL_100Conversations — the full production read
// path: list all conversations, then page every conversation's turns.
// 100 conversations x 4 turns is a realistic small-chat-history
// collection. This is export.go's actual hot loop, unmodified —
// baseline here, re-measured after the statement-reuse fix lands.
func BenchmarkExportJSONL_100Conversations(b *testing.B) {
	h := openBenchHistory(b)
	seedConversationsWithTurns(b, h, 100, 4)
	dir := b.TempDir()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		dest := filepath.Join(dir, "export.jsonl")
		if err := h.ExportJSONL(dest); err != nil {
			b.Fatalf("ExportJSONL: %v", err)
		}
	}
}

// turnsPageQuery mirrors export.go's ExportJSONL inner per-conversation
// query verbatim — the read that runs once PER RECORD in the list
// (the per-record cost that multiplies with collection size).
const turnsPageQuery = `SELECT id, ordinal, role, content, tool_calls, tool_results,
	        created_at, tokens_in, tokens_out, signal
	   FROM turns
	  WHERE conversation_id = ?
	  ORDER BY ordinal`

// drainTurnsPage scans every row exactly as ExportJSONL's inner loop
// does, so the two benchmark variants below differ ONLY in
// prepare-vs-reuse overhead, not in scan/decode cost.
func drainTurnsPage(rows *sql.Rows) (int, error) {
	defer rows.Close()
	n := 0
	for rows.Next() {
		var id, role, content string
		var toolCalls, toolResults sql.NullString
		var tokensIn, tokensOut sql.NullInt32
		var signal sql.NullString
		var ordinal int
		var created sql.NullTime
		if err := rows.Scan(
			&id, &ordinal, &role, &content, &toolCalls, &toolResults,
			&created, &tokensIn, &tokensOut, &signal,
		); err != nil {
			return n, err
		}
		n++
	}
	return n, rows.Err()
}

// BenchmarkTurnsQuery_RePrepare_100Conversations — db.Query(sql, id)
// issued fresh for every one of 100 conversations, the shape
// ExportJSONL's inner loop used pre-fix. database/sql implicitly
// prepares + executes + closes the statement server-side on every
// call unless the caller holds a *sql.Stmt across calls.
func BenchmarkTurnsQuery_RePrepare_100Conversations(b *testing.B) {
	h := openBenchHistory(b)
	ids := seedConversationIDs(b, h, 100, 4)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, id := range ids {
			rows, err := h.db.Query(turnsPageQuery, id)
			if err != nil {
				b.Fatalf("Query: %v", err)
			}
			if _, err := drainTurnsPage(rows); err != nil {
				b.Fatalf("drain: %v", err)
			}
		}
	}
}

// BenchmarkTurnsQuery_PreparedStmt_100Conversations — same query, same
// scan work, but the statement is prepared ONCE outside the timed
// per-conversation loop and reused via stmt.Query(id). Delta against
// RePrepare isolates DuckDB's per-call prepare cost.
func BenchmarkTurnsQuery_PreparedStmt_100Conversations(b *testing.B) {
	h := openBenchHistory(b)
	ids := seedConversationIDs(b, h, 100, 4)

	stmt, err := h.db.Prepare(turnsPageQuery)
	if err != nil {
		b.Fatalf("Prepare: %v", err)
	}
	b.Cleanup(func() { _ = stmt.Close() })

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, id := range ids {
			rows, err := stmt.Query(id)
			if err != nil {
				b.Fatalf("stmt.Query: %v", err)
			}
			if _, err := drainTurnsPage(rows); err != nil {
				b.Fatalf("drain: %v", err)
			}
		}
	}
}

// seedConversationIDs is seedConversationsWithTurns but also returns
// the ordered conversation ids so the turns-query benchmarks can walk
// them directly without re-querying the conversations table.
func seedConversationIDs(b *testing.B, h *History, n, turnsPer int) []string {
	b.Helper()
	ids := make([]string, 0, n)
	for i := 0; i < n; i++ {
		convID, err := h.StartConversation(NewConversation{ModelID: "lemer-lite"})
		if err != nil {
			b.Fatalf("seedConversationIDs: StartConversation[%d]: %v", i, err)
		}
		for j := 0; j < turnsPer; j++ {
			if _, err := h.WriteTurn(convID, NewTurn{
				Role:    "user",
				Content: "bench message content, representative length for a chat turn",
			}); err != nil {
				b.Fatalf("seedConversationIDs: WriteTurn[%d][%d]: %v", i, j, err)
			}
		}
		ids = append(ids, convID)
	}
	return ids
}

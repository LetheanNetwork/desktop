// SPDX-License-Identifier: EUPL-1.2

package lemma

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"dappco.re/lthn/desktop/pkg/chathistory"
)

// fakeChatServer answers /chat/completions with a canned assistant
// reply that echoes the latest user message. Lets us exercise the
// whole capture + send + capture loop without needing lthn-mlx.
func fakeChatServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			http.Error(w, "wrong path", http.StatusNotFound)
			return
		}
		var req chatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "decode: "+err.Error(), http.StatusBadRequest)
			return
		}
		var lastUser string
		for i := len(req.Messages) - 1; i >= 0; i-- {
			if req.Messages[i].Role == "user" {
				lastUser = req.Messages[i].Content
				break
			}
		}
		resp := chatResponse{
			ID:    "test-resp",
			Model: req.Model,
			Choices: []chatResponseChoice{
				{Index: 0, Message: chatMessage{Role: "assistant", Content: "echo: " + lastUser}, FinishReason: "stop"},
			},
			Usage: &chatResponseUsage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
		}
		w.Header().Set("content-type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
}

// TestSendCapturesBothTurns — Send appends the user turn, calls the
// model, appends the assistant turn. Archive ends with two turns per
// Send. LoadTurns returns them in order.
func TestSendCapturesBothTurns(t *testing.T) {
	srv := fakeChatServer(t)
	defer srv.Close()

	dir := t.TempDir()
	hist, err := chathistory.Open("snider", filepath.Join(dir, "chats.duckdb"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer hist.Close()

	svc := New(Config{
		BaseURL: srv.URL + "/v1",
		ModelID: "test-model",
		Timeout: 5 * time.Second,
		History: hist,
	})
	sess, err := svc.StartSession("snider", SessionMeta{Title: "smoke"})
	if err != nil {
		t.Fatalf("StartSession: %v", err)
	}

	reply, err := sess.Send(context.Background(), "hello")
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if reply != "echo: hello" {
		t.Fatalf("unexpected reply: %q", reply)
	}

	reply2, err := sess.Send(context.Background(), "and again")
	if err != nil {
		t.Fatalf("Send 2: %v", err)
	}
	if reply2 != "echo: and again" {
		t.Fatalf("unexpected reply 2: %q", reply2)
	}

	turns, err := hist.LoadTurns(sess.ConversationID())
	if err != nil {
		t.Fatalf("LoadTurns: %v", err)
	}
	if len(turns) != 4 {
		t.Fatalf("expected 4 turns, got %d", len(turns))
	}
	want := []struct{ role, content string }{
		{"user", "hello"},
		{"assistant", "echo: hello"},
		{"user", "and again"},
		{"assistant", "echo: and again"},
	}
	for i, w := range want {
		if turns[i].Role != w.role || turns[i].Content != w.content {
			t.Errorf("turn[%d]: got (%s, %s) want (%s, %s)", i, turns[i].Role, turns[i].Content, w.role, w.content)
		}
		if turns[i].Ordinal != i {
			t.Errorf("turn[%d].Ordinal = %d, want %d", i, turns[i].Ordinal, i)
		}
	}

	if err := sess.End(); err != nil {
		t.Fatalf("End: %v", err)
	}
	// Sending after End must fail.
	if _, err := sess.Send(context.Background(), "after end"); err == nil {
		t.Fatal("Send after End: want error, got nil")
	}
}

// TestSendPersistsUserTurnEvenOnModelFailure — when the model call
// fails, the user turn is still recorded so retry preserves the prompt.
func TestSendPersistsUserTurnEvenOnModelFailure(t *testing.T) {
	failSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "model unavailable", http.StatusInternalServerError)
	}))
	defer failSrv.Close()

	dir := t.TempDir()
	hist, _ := chathistory.Open("snider", filepath.Join(dir, "chats.duckdb"))
	defer hist.Close()

	svc := New(Config{BaseURL: failSrv.URL + "/v1", ModelID: "test", Timeout: time.Second, History: hist})
	sess, _ := svc.StartSession("snider", SessionMeta{})

	_, err := sess.Send(context.Background(), "doomed prompt")
	if err == nil {
		t.Fatal("expected model failure, got nil")
	}
	turns, _ := hist.LoadTurns(sess.ConversationID())
	if len(turns) != 1 {
		t.Fatalf("expected user turn persisted despite failure, got %d turns", len(turns))
	}
	if turns[0].Role != "user" || turns[0].Content != "doomed prompt" {
		t.Errorf("expected user turn preserved, got (%s, %s)", turns[0].Role, turns[0].Content)
	}
}

// TestStartSessionRequiresHistory — Service without history can't open
// sessions; the auto-capture contract is the surface.
func TestStartSessionRequiresHistory(t *testing.T) {
	svc := New(Config{ModelID: "test"})
	if _, err := svc.StartSession("snider", SessionMeta{}); err == nil {
		t.Fatal("expected error when history nil, got nil")
	}
}

// TestStartSessionNilReceiverFails — the defensive nil-Service guard.
func TestStartSessionNilReceiverFails(t *testing.T) {
	var svc *Service
	if _, err := svc.StartSession("snider", SessionMeta{}); err == nil {
		t.Fatal("expected error for a nil *Service receiver, got nil")
	}
}

// TestConversationIDNilReceiverIsEmpty — defensive nil-Session guard.
func TestConversationIDNilReceiverIsEmpty(t *testing.T) {
	var sess *Session
	if got := sess.ConversationID(); got != "" {
		t.Fatalf("ConversationID on nil *Session = %q, want empty", got)
	}
}

// TestSendNilReceiverFails — the defensive nil-Session guard (distinct from
// the closed-session check TestSendCapturesBothTurns already exercises).
func TestSendNilReceiverFails(t *testing.T) {
	var sess *Session
	if _, err := sess.Send(context.Background(), "hello"); err == nil {
		t.Fatal("expected error for a nil *Session receiver, got nil")
	}
}

// TestEndNilReceiverIsNoop — End on a nil *Session must not panic and
// reports no error (nothing to close).
func TestEndNilReceiverIsNoop(t *testing.T) {
	var sess *Session
	if err := sess.End(); err != nil {
		t.Fatalf("End on nil *Session = %v, want nil", err)
	}
}

// TestServiceBaseURLAndModelID — the two trivial config-echo getters.
func TestServiceBaseURLAndModelID(t *testing.T) {
	svc := New(Config{BaseURL: "http://example.invalid/v1", ModelID: "custom-model"})
	if got := svc.BaseURL(); got != "http://example.invalid/v1" {
		t.Fatalf("BaseURL() = %q, want the configured value", got)
	}
	if got := svc.ModelID(); got != "custom-model" {
		t.Fatalf("ModelID() = %q, want custom-model", got)
	}
}

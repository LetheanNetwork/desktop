// SPDX-Licence-Identifier: EUPL-1.2

package agents

import (
	"context"
	"net/http"
	"net/http/httptest"
	"time"

	core "dappco.re/go"
	coremcp "dappco.re/go/mcp/pkg/mcp"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// channelTestServer mimics the hub's /mcp MCP HTTP+SSE endpoint:
// POSTs (initialize / initialized) get a session header + 200; the GET opens
// the server→client SSE stream and writes getBody, then ends.
func channelTestServer(getBody string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(getBody))
			return
		}
		w.Header().Set("Mcp-Session-Id", "test-session")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{}}`))
	}))
}

func TestChannels_Listener_Consume_Good(t *core.T) {
	const evt = "event: message\n" +
		`data: {"jsonrpc":"2.0","method":"notifications/claude/channel","params":{"channel":"agent.blocked","data":{"name":"core/go-io/task-4"}}}` +
		"\n\n"
	srv := channelTestServer(evt)
	defer srv.Close()

	var gotChannel string
	var gotData any
	l := newChannelListener(srv.URL, "", func(ch string, data any) { gotChannel = ch; gotData = data })
	connected, err := l.consume(context.Background())
	core.AssertTrue(t, connected, "stream should open")
	core.AssertTrue(t, err == nil, "clean stream returns no error")
	core.AssertEqual(t, "agent.blocked", gotChannel)
	core.AssertTrue(t, gotData != nil, "channel data relayed")
}

func TestChannels_Listener_Consume_Bad(t *core.T) {
	// Nothing listening → initialize fails → (false, err), no panic.
	l := newChannelListener("http://127.0.0.1:1/mcp", "", func(string, any) {})
	connected, err := l.consume(context.Background())
	core.AssertFalse(t, connected, "no stream when serve is unreachable")
	core.AssertTrue(t, err != nil, "unreachable serve surfaces an error")
}

func TestChannels_Listener_Consume_Ugly(t *core.T) {
	// A non-channel notification must NOT be relayed (stream still drains cleanly).
	const evt = "event: message\n" +
		`data: {"jsonrpc":"2.0","method":"notifications/message","params":{"level":"info"}}` +
		"\n\n"
	srv := channelTestServer(evt)
	defer srv.Close()

	relayed := false
	l := newChannelListener(srv.URL, "", func(string, any) { relayed = true })
	connected, err := l.consume(context.Background())
	core.AssertTrue(t, connected)
	core.AssertTrue(t, err == nil)
	core.AssertFalse(t, relayed, "non-channel notification must not relay")
}

// TestChannels_Listener_Initialize_Bad_NonOKStatus covers initialize's
// own status-check branch (distinct from the unreachable-server /
// client.Do-error case TestChannels_Listener_Consume_Bad already
// covers) — the POST reaches a live server that rejects it outright.
func TestChannels_Listener_Initialize_Bad_NonOKStatus(t *core.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	l := newChannelListener(srv.URL, "", func(string, any) {})
	connected, err := l.consume(context.Background())
	core.AssertFalse(t, connected)
	core.RequireTrue(t, err != nil)
	core.AssertTrue(t, core.Contains(err.Error(), "initialize status 500"))
}

// TestChannels_Listener_Consume_Bad_GetStreamNonOKStatus covers the SSE
// GET's own status-check branch: initialize succeeds (POST 200) but the
// stream GET is rejected.
func TestChannels_Listener_Consume_Bad_GetStreamNonOKStatus(t *core.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{}}`))
	}))
	defer srv.Close()

	l := newChannelListener(srv.URL, "", func(string, any) {})
	connected, err := l.consume(context.Background())
	core.AssertFalse(t, connected)
	core.RequireTrue(t, err != nil)
	core.AssertTrue(t, core.Contains(err.Error(), "stream GET status 503"))
}

// TestChannels_Listener_Dispatch_Bad_MalformedJSONSkipped is real fault
// injection: an SSE payload that isn't valid JSON must be silently
// skipped, never relayed, never panic.
func TestChannels_Listener_Dispatch_Bad_MalformedJSONSkipped(t *core.T) {
	const evt = "event: message\ndata: {not json\n\n"
	srv := channelTestServer(evt)
	defer srv.Close()

	relayed := false
	l := newChannelListener(srv.URL, "", func(string, any) { relayed = true })
	connected, err := l.consume(context.Background())
	core.AssertTrue(t, connected)
	core.AssertTrue(t, err == nil)
	core.AssertFalse(t, relayed)
}

// TestChannels_Listener_SetBearer_Good_InjectsAuthorizationHeader covers
// setBearer's non-empty branch (every other test in this file uses an
// empty bearer, the unit-test default) by capturing the header a real
// fixture server actually received.
func TestChannels_Listener_SetBearer_Good_InjectsAuthorizationHeader(t *core.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			gotAuth = r.Header.Get("Authorization")
		}
		if r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{}}`))
	}))
	defer srv.Close()

	l := newChannelListener(srv.URL, "sk-lthn-secret", func(string, any) {})
	_, _ = l.consume(context.Background())
	core.AssertEqual(t, "Bearer sk-lthn-secret", gotAuth)
}

// TestChannels_Listener_Run_Bad_UnreachableBacksOffThenStopsOnCancel
// drives run()'s reconnect loop against an unreachable endpoint: the
// first iteration fails to connect (Debug log + no backoff reset +
// select's time.After branch), the second iteration's select picks the
// ctx.Done() branch once the outer timeout fires. Real time passes
// (minBackoff is an internal 1s const, no seam to shrink it) but the
// whole test is bounded well under 2s.
func TestChannels_Listener_Run_Bad_UnreachableBacksOffThenStopsOnCancel(t *core.T) {
	l := newChannelListener("http://127.0.0.1:1/mcp", "", func(string, any) {})
	ctx, cancel := context.WithTimeout(context.Background(), 1200*time.Millisecond)
	defer cancel()
	l.run(ctx) // must return on its own once ctx's deadline passes
	core.AssertTrue(t, ctx.Err() != nil)
}

// TestChannels_Listener_Run_Good_ConnectedResetsBackoffThenStopsOnCancel
// covers run()'s "connected == true -> reset backoff" branch, which the
// unreachable-server backoff test above never reaches (that one never
// connects). The fixture serves one event then ends the stream, so
// consume() returns (true, nil); the outer ctx's short timeout then
// stops run() via the select's ctx.Done() case.
func TestChannels_Listener_Run_Good_ConnectedResetsBackoffThenStopsOnCancel(t *core.T) {
	const evt = "event: message\n" +
		`data: {"jsonrpc":"2.0","method":"notifications/claude/channel","params":{"channel":"x","data":{}}}` +
		"\n\n"
	srv := channelTestServer(evt)
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	l := newChannelListener(srv.URL, "", func(string, any) {})
	l.run(ctx)
	core.AssertTrue(t, ctx.Err() != nil)
}

// TestChannels_Listener_Delivery_Good is the end-to-end proof the httptest
// mock can't give: a REAL coremcp server (with the claude/channel capability)
// broadcasts via ChannelSend — exactly as lthn-agent serve does — and the
// listener must relay it off the standalone GET stream. This nails the one
// residual the live verification couldn't fabricate (no real agent run).
func TestChannels_Listener_Delivery_Good(t *core.T) {
	svc, err := coremcp.New(coremcp.Options{Unrestricted: true})
	core.AssertTrue(t, err == nil, "coremcp.New must succeed")
	if err != nil {
		return
	}
	handler := mcp.NewStreamableHTTPHandler(
		func(_ *http.Request) *mcp.Server { return svc.Server() }, nil,
	)
	mux := http.NewServeMux()
	mux.Handle("/mcp", handler)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	got := make(chan string, 8)
	l := newChannelListener(srv.URL+"/mcp", "", func(ch string, _ any) { got <- ch })
	l.start()
	defer l.stop() // LIFO: stops the listener before srv.Close closes the stream

	// Send until relayed — robust to the connect→GET-stream-open race.
	// ChannelSend broadcasts to all live sessions, so re-sends are harmless;
	// the first to arrive after the stream opens proves delivery.
	relayed := ""
	for i := 0; i < 50 && relayed == ""; i++ {
		svc.ChannelSend(context.Background(), coremcp.ChannelAgentBlocked,
			map[string]any{"name": "core/go-io/task-1"})
		select {
		case relayed = <-got:
		case <-time.After(100 * time.Millisecond):
		}
	}
	core.AssertEqual(t, coremcp.ChannelAgentBlocked, relayed)
}

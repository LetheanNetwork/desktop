// SPDX-Licence-Identifier: EUPL-1.2

package agents

import (
	"context"
	"net/http"
	"net/http/httptest"

	core "dappco.re/go"
)

// channelTestServer mimics lthn-agent serve's /mcp Streamable-HTTP endpoint:
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
	l := newChannelListener(srv.URL, func(ch string, data any) { gotChannel = ch; gotData = data })
	connected, err := l.consume(context.Background())
	core.AssertTrue(t, connected, "stream should open")
	core.AssertTrue(t, err == nil, "clean stream returns no error")
	core.AssertEqual(t, "agent.blocked", gotChannel)
	core.AssertTrue(t, gotData != nil, "channel data relayed")
}

func TestChannels_Listener_Consume_Bad(t *core.T) {
	// Nothing listening → initialize fails → (false, err), no panic.
	l := newChannelListener("http://127.0.0.1:1/mcp", func(string, any) {})
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
	l := newChannelListener(srv.URL, func(string, any) { relayed = true })
	connected, err := l.consume(context.Background())
	core.AssertTrue(t, connected)
	core.AssertTrue(t, err == nil)
	core.AssertFalse(t, relayed, "non-channel notification must not relay")
}

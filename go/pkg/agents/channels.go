// SPDX-Licence-Identifier: EUPL-1.2

package agents

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"net/http"
	"time"

	core "dappco.re/go"
)

// channelEventName is the Wails event the desktop re-emits for every CoreAgent
// channel notification. The Agents view subscribes via
// Events.On("lthn:agents:channel", …) and reacts (e.g. Activity refreshes the
// moment an agent blocks instead of waiting for its 5s poll). Payload shape:
// {channel: string, data: any}.
const channelEventName = "lthn:agents:channel"

// channelNotificationMethod is CoreAgent's JSON-RPC method for named channel
// events — the claude/channel experimental capability (see coremcp notify.go).
const channelNotificationMethod = "notifications/claude/channel"

// sseDataPrefix marks a Server-Sent-Events payload line.
var sseDataPrefix = []byte("data:")

// channelListener holds a long-lived MCP session to the hub's MCP plane
// (lthn-agent hub, :9202) and relays every notifications/claude/channel
// event to a sink. Consume-only by design: it never makes tool calls —
// it exists purely to turn CoreAgent's push channels (agent.blocked,
// agent.complete, agent.status, …) into desktop events so the UI updates
// live rather than polling. The crew respawns lthn-agent hub, so session
// drops are expected churn; run() reconnects with capped backoff.
//
// bearer is the MCP_AUTH_TOKEN value the hub's transport expects on every
// request. Empty string disables injection (unit tests with no-auth serve).
type channelListener struct {
	mcpURL string
	bearer string
	relay  func(channel string, data any)
	client *http.Client
	cancel context.CancelFunc
}

// newChannelListener builds a listener for the given MCP endpoint that hands
// each channel event to relay. relay must be goroutine-safe; it's called from
// the listener's own goroutine. bearer is forwarded as Authorization: Bearer
// on every HTTP request to the hub's fail-closed transport.
//
//	l := newChannelListener("http://127.0.0.1:9202", token, func(ch string, d any) { _ = ch })
func newChannelListener(mcpURL, bearer string, relay func(channel string, data any)) *channelListener {
	return &channelListener{
		mcpURL: mcpURL,
		bearer: bearer,
		relay:  relay,
		// No client timeout — the GET stream is intentionally long-lived;
		// reconnection is driven by ctx + stream EOF, not a deadline.
		client: &http.Client{},
	}
}

// start launches the connect→consume→reconnect loop in a goroutine. Idempotent
// only in the sense that the caller owns one listener; call stop to end it.
func (l *channelListener) start() {
	ctx, cancel := context.WithCancel(context.Background())
	l.cancel = cancel
	go l.run(ctx)
}

// stop ends the listener loop and closes the stream at the next read boundary.
func (l *channelListener) stop() {
	if l.cancel != nil {
		l.cancel()
	}
}

// run reconnects until ctx is cancelled. A live session that drops reconnects
// promptly (the serve restarted); a failed connect backs off geometrically so
// a down engine doesn't spin.
func (l *channelListener) run(ctx context.Context) {
	const (
		minBackoff = time.Second
		maxBackoff = 30 * time.Second
	)
	backoff := minBackoff
	for ctx.Err() == nil {
		connected, err := l.consume(ctx)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			// Quiet by default — the serve is often briefly down across a
			// crew respawn; the "connected" Info marks recovery.
			core.Debug("agents.channels: stream unavailable", "err", err, "backoff", backoff.String())
		}
		if connected {
			backoff = minBackoff
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		if !connected && backoff < maxBackoff {
			backoff *= 2
		}
	}
}

// consume runs one session: initialise, then drain the server→client SSE
// stream, relaying channel notifications until it ends. The bool reports
// whether the stream opened (a live session) — run() uses it to pick its
// reconnect cadence.
func (l *channelListener) consume(ctx context.Context) (bool, error) {
	sessionID, err := l.initialize(ctx)
	if err != nil {
		return false, err
	}
	l.sendInitialized(ctx, sessionID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, l.mcpURL, nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("Accept", "text/event-stream")
	l.setBearer(req)
	if sessionID != "" {
		req.Header.Set("Mcp-Session-Id", sessionID)
	}
	resp, err := l.client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false, core.E("agents.channels", "stream GET status "+core.Itoa(resp.StatusCode), nil)
	}
	core.Info("agents.channels: connected", "mcp", l.mcpURL)
	l.drain(resp.Body)
	return true, nil
}

// initialize opens an MCP session and returns its id (from the Mcp-Session-Id
// response header; empty is tolerated for stateless servers).
func (l *channelListener) initialize(ctx context.Context) (string, error) {
	const body = `{"jsonrpc":"2.0","id":1,"method":"initialize",` +
		`"params":{"protocolVersion":"2025-03-26","capabilities":{},` +
		`"clientInfo":{"name":"lthn-desktop","version":"1"}}}`
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, l.mcpURL, bytes.NewReader([]byte(body)))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	l.setBearer(req)
	resp, err := l.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", core.E("agents.channels", "initialize status "+core.Itoa(resp.StatusCode), nil)
	}
	return resp.Header.Get("Mcp-Session-Id"), nil
}

// sendInitialized completes the MCP handshake. Best-effort — a server that
// doesn't require it simply ignores the notification.
func (l *channelListener) sendInitialized(ctx context.Context, sessionID string) {
	const body = `{"jsonrpc":"2.0","method":"notifications/initialized"}`
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, l.mcpURL, bytes.NewReader([]byte(body)))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	l.setBearer(req)
	if sessionID != "" {
		req.Header.Set("Mcp-Session-Id", sessionID)
	}
	if resp, err := l.client.Do(req); err == nil {
		resp.Body.Close()
	}
}

// setBearer attaches Authorization: Bearer <token> when the listener was
// constructed with a non-empty bearer credential. No-op for unit tests
// that pass an empty token.
func (l *channelListener) setBearer(req *http.Request) {
	if l.bearer != "" {
		req.Header.Set("Authorization", "Bearer "+l.bearer)
	}
}

// drain reads the SSE stream, reassembling each event's data payload and
// dispatching it, until the stream ends (server restart, ctx cancel, or error).
func (l *channelListener) drain(body io.Reader) {
	scanner := bufio.NewScanner(body)
	// Channel payloads are small, but headroom keeps a verbose one (a long
	// blocked question) from tripping the default 64 KiB token cap.
	scanner.Buffer(make([]byte, 0, 64*1024), 1<<20)
	var data []byte
	for scanner.Scan() {
		line := scanner.Bytes()
		switch {
		case len(line) == 0:
			// Blank line terminates one SSE event — dispatch what we have.
			if len(data) > 0 {
				l.dispatch(data)
				data = data[:0]
			}
		case bytes.HasPrefix(line, sseDataPrefix):
			chunk := bytes.TrimPrefix(line, sseDataPrefix)
			chunk = bytes.TrimPrefix(chunk, []byte(" "))
			data = append(data, chunk...)
		default:
			// event:/id:/retry:/comment lines — not needed here.
		}
	}
}

// dispatch parses one SSE payload as a JSON-RPC message and, if it's a
// claude/channel notification, relays its channel + data. Anything else
// (other notifications, malformed JSON) is silently skipped.
func (l *channelListener) dispatch(raw []byte) {
	var msg struct {
		Method string `json:"method"`
		Params struct {
			Channel string `json:"channel"`
			Data    any    `json:"data"`
		} `json:"params"`
	}
	if r := core.JSONUnmarshal(raw, &msg); !r.OK {
		return
	}
	if msg.Method != channelNotificationMethod || msg.Params.Channel == "" {
		return
	}
	if l.relay != nil {
		l.relay(msg.Params.Channel, msg.Params.Data)
	}
}

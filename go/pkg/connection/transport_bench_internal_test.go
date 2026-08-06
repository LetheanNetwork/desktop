// SPDX-Licence-Identifier: EUPL-1.2

// Benchmarks for the Wails socket transport — the per-message
// multiplier. Two load paths matter here:
//
//   - dispatchWailsEvent: the OUTBOUND hop every runner.WChatStream
//     token takes on its way to the renderer (runner.emitChatEvent →
//     webkit.EmitEvent → wails app.EmitEvent → this transport's
//     DispatchWailsEvent → dispatchWailsEvent) — fires once PER TOKEN
//     PER CONNECTED CLIENT.
//   - handleRequest / the websocket encode-decode-write round trip:
//     the INBOUND hop every renderer binding call takes — fires once
//     per RPC call from the WebView.
//
// Run:
//
//	go test -run='^$' -bench=. -benchmem -benchtime=20x ./pkg/connection/
package connection

import (
	core "dappco.re/go"
	"github.com/gorilla/websocket"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// benchDispatchService wires n socketClients directly into the
// service's client set, bypassing the real websocket accept path, so
// BenchmarkDispatchWailsEvent_* isolates dispatchWailsEvent's own
// per-client message/allocation cost from gorilla's wire codec — the
// codec half is covered separately by BenchmarkWailsSocketRoundTrip.
func benchDispatchService(n int) (*Service, []*socketClient) {
	s := NewService(Options{Address: "127.0.0.1:0"})
	clients := make([]*socketClient, n)
	for i := range clients {
		c := &socketClient{
			send: make(chan *wailsSocketMessage, 1),
			done: make(chan struct{}),
		}
		s.clients[c] = struct{}{}
		clients[i] = c
	}
	return s, clients
}

// benchDeltaEvent mirrors the exact CustomEvent shape runner.WChatStream
// sends per token (pkg/runner/stream.go's eventChatDelta payload).
func benchDeltaEvent() *application.CustomEvent {
	return &application.CustomEvent{
		Name: "runner:chat:delta",
		Data: map[string]any{
			"call_id":  "bench-call",
			"delta":    "quick ",
			"provider": "lem",
			"model":    "bench-model",
		},
	}
}

// BenchmarkDispatchWailsEvent_1Client measures the single-connection
// case — one WebView window watching the stream (the common desktop
// shape).
func BenchmarkDispatchWailsEvent_1Client(b *core.B) {
	s, clients := benchDispatchService(1)
	event := benchDeltaEvent()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.dispatchWailsEvent(event)
		<-clients[0].send
	}
}

// BenchmarkDispatchWailsEvent_8Clients measures the fan-out case — the
// per-client loop inside dispatchWailsEvent runs 8x, so any per-
// iteration allocation (rather than a single shared payload) shows up
// scaled here relative to the 1-client bench.
func BenchmarkDispatchWailsEvent_8Clients(b *core.B) {
	s, clients := benchDispatchService(8)
	event := benchDeltaEvent()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.dispatchWailsEvent(event)
		for _, c := range clients {
			<-c.send
		}
	}
}

// BenchmarkHandleRequest measures the server-side per-RPC-call path
// (readLoop → handleRequest: processor dispatch + wailsResponse +
// wailsSocketMessage construction + the client.send handoff) without
// the real websocket read/write either side of it — isolates this
// package's own allocation from gorilla's JSON codec, which
// BenchmarkWailsSocketRoundTrip covers separately. Object 999/Method 0
// resolves to no bound method (mirrors transport_internal_test.go's
// fixture request), so every call exercises the same "unprocessable"
// response-encoding path a genuine unbound call would.
func BenchmarkHandleRequest(b *core.B) {
	svc := NewService(Options{Address: "127.0.0.1:0"})
	// MessageProcessor.HandleRuntimeCallWithIDs resolves a target window
	// through the live wails app singleton (getTargetWindow) — without
	// this it nil-derefs. transport_internal_test.go's fixture does the
	// same call for the same reason.
	_ = application.New(application.Options{})
	svc.processor = application.NewMessageProcessor(application.DefaultLogger(nil))
	client := &socketClient{
		send: make(chan *wailsSocketMessage, 1),
		done: make(chan struct{}),
	}
	msg := wailsSocketMessage{
		ID:   "bench-req",
		Type: messageTypeRequest,
		Request: &application.RuntimeRequest{
			Object: 999,
			Method: 0,
			Args:   &application.Args{},
		},
	}
	ctx := core.Background()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		svc.handleRequest(ctx, client, msg)
		<-client.send
	}
}

// benchTransportFixture starts a real listener on 127.0.0.1:0 (an
// ephemeral port — never one of the dev app's reserved ports) so
// BenchmarkWailsSocketRoundTrip can drive the ACTUAL encode/decode/
// write path: gorilla's WriteJSON/ReadJSON over a loopback TCP socket,
// both directions. Mirrors transport_internal_test.go's
// startTransportFixture, adapted to take *core.B (that helper is typed
// to *core.T, which a *core.B cannot satisfy positionally).
func benchTransportFixture(b *core.B, opts Options) (*Service, string) {
	b.Helper()
	svc := NewService(opts)
	transport := svc.Transport()
	_ = application.New(application.Options{})
	processor := application.NewMessageProcessor(application.DefaultLogger(nil))
	if err := transport.Start(core.Background(), processor); err != nil {
		b.Fatalf("transport start: %v", err)
	}
	assets, ok := transport.(application.AssetServerTransport)
	if !ok {
		b.Fatal("transport does not implement AssetServerTransport")
	}
	noopAssets := core.HandlerFunc(func(core.ResponseWriter, *core.Request) {})
	if err := assets.ServeAssets(noopAssets); err != nil {
		b.Fatalf("serve assets: %v", err)
	}
	b.Cleanup(func() { _ = transport.Stop() })
	return svc, svc.address()
}

// BenchmarkWailsSocketRoundTrip measures one full request/response
// cycle over a real websocket connection — client WriteJSON, server
// readLoop→handleRequest→send, server writeLoop WriteJSON, client
// ReadJSON. This is the genuine wire cost BenchmarkHandleRequest
// deliberately excludes; comparing the two isolates "our allocation"
// from "gorilla's JSON codec + socket syscalls".
func BenchmarkWailsSocketRoundTrip(b *core.B) {
	svc, addr := benchTransportFixture(b, Options{Address: "127.0.0.1:0"})
	conn, _, err := websocket.DefaultDialer.Dial("ws://"+addr+svc.options.Path, nil)
	if err != nil {
		b.Fatalf("dial: %v", err)
	}
	b.Cleanup(func() { _ = conn.Close() })

	req := map[string]any{
		"id":   "bench-1",
		"type": "request",
		"request": map[string]any{
			"object": 999,
			"method": 0,
			"args":   map[string]any{},
		},
	}
	var reply map[string]any

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := conn.WriteJSON(req); err != nil {
			b.Fatalf("write: %v", err)
		}
		if err := conn.ReadJSON(&reply); err != nil {
			b.Fatalf("read: %v", err)
		}
	}
}

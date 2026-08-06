// SPDX-Licence-Identifier: EUPL-1.2

package connection

import (
	core "dappco.re/go"
	"github.com/gorilla/websocket"
	"github.com/wailsapp/wails/v3/pkg/application"
)

func startTransportFixture(t *core.T, opts Options) (*Service, application.AssetServerTransport) {
	t.Helper()
	svc := NewService(opts)
	transport := svc.Transport()
	_ = application.New(application.Options{})
	processor := application.NewMessageProcessor(application.DefaultLogger(nil))
	core.RequireNoError(t, transport.Start(core.Background(), processor))
	assets, ok := transport.(application.AssetServerTransport)
	core.RequireTrue(t, ok)
	core.RequireNoError(t, assets.ServeAssets(core.HandlerFunc(func(w core.ResponseWriter, _ *core.Request) {
		_ = core.WriteString(w, "Lethean")
	})))
	t.Cleanup(func() {
		core.AssertNoError(t, transport.Stop())
	})
	return svc, assets
}

func TestConnection_Transport_Good_RequestResponseAndAssets(t *core.T) {
	svc, _ := startTransportFixture(t, Options{Address: "127.0.0.1:0"})

	asset := core.HTTPGet("http://" + svc.address() + "/")
	core.RequireTrue(t, asset.OK)
	response := asset.Value.(*core.Response)
	defer response.Body.Close()
	body := core.ReadAll(response.Body)
	core.RequireTrue(t, body.OK)
	core.AssertEqual(t, "Lethean", body.String())

	conn, _, dialErr := websocket.DefaultDialer.Dial("ws://"+svc.address()+svc.options.Path, nil)
	core.RequireNoError(t, dialErr)
	defer conn.Close()

	core.RequireNoError(t, conn.WriteJSON(map[string]any{
		"id":   "request-1",
		"type": "request",
		"request": map[string]any{
			"object": 999,
			"method": 0,
			"args":   map[string]any{},
		},
	}))

	var reply map[string]any
	core.RequireNoError(t, conn.ReadJSON(&reply))
	core.AssertEqual(t, "request-1", reply["id"])
	core.AssertEqual(t, "response", reply["type"])
	envelope := reply["response"].(map[string]any)
	core.AssertEqual(t, float64(core.StatusUnprocessableEntity), envelope["statusCode"])
}

func TestConnection_Transport_Bad_RejectsOriginAndToken(t *core.T) {
	svc, _ := startTransportFixture(t, Options{
		Address:        "127.0.0.1:0",
		AllowedOrigins: []string{"https://desktop.lethean.io"},
		Token:          "secret",
	})

	headers := core.Header{"Origin": []string{"https://hostile.example"}}
	conn, response, dialErr := websocket.DefaultDialer.Dial(
		"ws://"+svc.address()+svc.options.Path+"?access_token=wrong",
		headers,
	)
	if conn != nil {
		defer conn.Close()
	}
	core.AssertError(t, dialErr)
	core.RequireTrue(t, response != nil)
	defer response.Body.Close()
	core.AssertTrue(t,
		response.StatusCode == core.StatusForbidden || response.StatusCode == core.StatusUnauthorized)
}

func TestConnection_Transport_Ugly_BroadcastsEvents(t *core.T) {
	svc, _ := startTransportFixture(t, Options{Address: "127.0.0.1:0"})
	conn, _, dialErr := websocket.DefaultDialer.Dial("ws://"+svc.address()+svc.options.Path, nil)
	core.RequireNoError(t, dialErr)
	defer conn.Close()

	core.RequireNoError(t, conn.WriteJSON(map[string]any{
		"id":   "ready",
		"type": "request",
		"request": map[string]any{
			"object": 999,
			"method": 0,
			"args":   map[string]any{},
		},
	}))
	var ready map[string]any
	core.RequireNoError(t, conn.ReadJSON(&ready))

	svc.dispatchWailsEvent(&application.CustomEvent{
		Name: "lthn:test",
		Data: map[string]any{"ready": true},
	})

	var event map[string]any
	core.RequireNoError(t, conn.ReadJSON(&event))
	core.AssertEqual(t, "event", event["type"])
	payload := event["event"].(map[string]any)
	core.AssertEqual(t, "lthn:test", payload["name"])
}

func TestConnection_Transport_Ugly_StopIsIdempotent(t *core.T) {
	svc := NewService(Options{Address: "127.0.0.1:0"})
	transport := svc.Transport()
	core.AssertNoError(t, transport.Stop())
	core.AssertNoError(t, transport.Stop())
}

func TestConnection_Transport_Bad_DoesNotPublishToken(t *core.T) {
	svc := NewService(Options{
		PublicURL: "wss://desktop.lethean.example/wails/ws",
		Token:     "never-publish-this-secret",
	})
	client := string(svc.jsClient())
	core.AssertContains(t, client, "wss://desktop.lethean.example/wails/ws")
	core.AssertFalse(t, core.Contains(client, "never-publish-this-secret"))
	core.AssertFalse(t, core.Contains(client, "\"token\""))
}

func TestConnection_Transport_Good_PublishesConfiguredClientURL(t *core.T) {
	svc := NewService(Options{
		PublicURL: "wss://api.lethean.example/wails/ws",
		Token:     "never-publish-this-secret",
	})
	client := string(svc.jsClient())

	core.AssertContains(t, client, `globalThis.__LTHN_CONNECTION__=Object.freeze(`)
	core.AssertContains(t, client, `"webSocketUrl":"wss://api.lethean.example/wails/ws"`)
	core.AssertFalse(t, core.Contains(client, "never-publish-this-secret"))
}

func TestConnection_Transport_Good_AcceptsBearerToken(t *core.T) {
	svc, _ := startTransportFixture(t, Options{
		Address: "127.0.0.1:0",
		Token:   "mobile-secret",
	})
	headers := core.Header{"Authorization": []string{"Bearer mobile-secret"}}
	conn, _, dialErr := websocket.DefaultDialer.Dial(
		"ws://"+svc.address()+svc.options.Path,
		headers,
	)
	core.RequireNoError(t, dialErr)
	defer conn.Close()
}

func TestConnection_Transport_Good_AcceptsValidatedWailsDevOrigins(t *core.T) {
	t.Setenv("LTHN_WAILS_WS_ORIGINS", "")
	t.Setenv("WAILS_VITE_PORT", "9245")
	svc, _ := startTransportFixture(t, Options{Address: "127.0.0.1:0"})

	for _, origin := range []string{
		"wails://localhost:9245",
		"http://localhost:9245",
	} {
		headers := core.Header{"Origin": []string{origin}}
		conn, _, dialErr := websocket.DefaultDialer.Dial(
			"ws://"+svc.address()+svc.options.Path,
			headers,
		)
		core.RequireNoError(t, dialErr)
		core.AssertNoError(t, conn.Close())
	}
}

func TestConnection_Transport_Bad_BlanksOversizedRequestID(t *core.T) {
	svc, _ := startTransportFixture(t, Options{Address: "127.0.0.1:0"})
	conn, _, dialErr := websocket.DefaultDialer.Dial("ws://"+svc.address()+svc.options.Path, nil)
	core.RequireNoError(t, dialErr)
	defer conn.Close()

	core.RequireNoError(t, conn.WriteJSON(map[string]any{
		"id":   core.Repeat("x", maxRequestIDBytes+1),
		"type": "request",
		"request": map[string]any{
			"object": 0,
			"method": 0,
			"args":   map[string]any{},
		},
	}))

	var reply map[string]any
	core.RequireNoError(t, conn.ReadJSON(&reply))
	core.AssertNil(t, reply["id"])
	core.AssertEqual(t, messageTypeResponse, reply["type"])
}

func TestConnection_Transport_Bad_EnforcesClientLimit(t *core.T) {
	svc, _ := startTransportFixture(t, Options{
		Address:    "127.0.0.1:0",
		MaxClients: 1,
	})
	first, _, firstErr := websocket.DefaultDialer.Dial(
		"ws://"+svc.address()+svc.options.Path,
		nil,
	)
	core.RequireNoError(t, firstErr)
	defer first.Close()

	second, response, secondErr := websocket.DefaultDialer.Dial(
		"ws://"+svc.address()+svc.options.Path,
		nil,
	)
	if second != nil {
		defer second.Close()
	}
	core.AssertError(t, secondErr)
	core.RequireTrue(t, response != nil)
	defer response.Body.Close()
	core.AssertEqual(t, core.StatusTooManyRequests, response.StatusCode)
}

func TestConnection_Transport_Ugly_ReservesClientLimitAtomically(t *core.T) {
	svc := NewService(Options{MaxClients: 1})
	svc.mu.Lock()
	svc.running = true
	svc.mu.Unlock()

	const contenders = 32
	start := make(chan struct{})
	results := make(chan bool, contenders)
	var ready core.WaitGroup
	ready.Add(contenders)
	for range contenders {
		go func() {
			ready.Done()
			<-start
			results <- svc.reserveClient()
		}()
	}
	ready.Wait()
	close(start)

	admitted := 0
	for range contenders {
		if <-results {
			admitted++
		}
	}
	core.AssertEqual(t, 1, admitted)
	svc.releaseClientReservation()
	core.AssertTrue(t, svc.reserveClient())
	svc.releaseClientReservation()
}

func TestConnection_Transport_Bad_BoundsPendingRequestsPerClient(t *core.T) {
	client := &socketClient{slots: make(chan struct{}, 1)}
	core.AssertTrue(t, client.reserveRequest())
	core.AssertFalse(t, client.reserveRequest())
	client.releaseRequest()
	core.AssertTrue(t, client.reserveRequest())
	client.releaseRequest()
}

// TestWailsTransport_NilGuards exercises every "t == nil || t.service == nil"
// defensive branch on the wailsTransport adapter: a zero-value transport with
// no bound service, and a genuinely nil *wailsTransport receiver. Wails never
// constructs the adapter this way in production (NewService always sets
// service), but the guards are real code paths and worth locking down.
func TestWailsTransport_NilGuards(t *core.T) {
	unbound := &wailsTransport{}
	core.AssertError(t, unbound.Start(core.Background(), nil))
	core.AssertError(t, unbound.ServeAssets(nil))
	core.AssertNil(t, unbound.JSClient())
	core.AssertNoError(t, unbound.Stop())
	unbound.DispatchWailsEvent(&application.CustomEvent{Name: "lthn:test"})

	var nilTransport *wailsTransport
	core.AssertError(t, nilTransport.Start(core.Background(), nil))
	core.AssertError(t, nilTransport.ServeAssets(nil))
	core.AssertNil(t, nilTransport.JSClient())
	core.AssertNoError(t, nilTransport.Stop())
	nilTransport.DispatchWailsEvent(&application.CustomEvent{Name: "lthn:test"})
}

func TestConnection_Transport_Bad_DoubleStartFails(t *core.T) {
	_, transport := startTransportFixture(t, Options{Address: "127.0.0.1:0"})
	processor := application.NewMessageProcessor(application.DefaultLogger(nil))
	err := transport.Start(core.Background(), processor)
	core.AssertError(t, err, "already started")
}

func TestConnection_Transport_Bad_RejectsNonGetMethod(t *core.T) {
	svc, _ := startTransportFixture(t, Options{Address: "127.0.0.1:0"})
	resp := core.HTTPPost("http://"+svc.address()+svc.options.Path, "text/plain", nil)
	core.RequireTrue(t, resp.OK)
	response := resp.Value.(*core.Response)
	defer response.Body.Close()
	core.AssertEqual(t, core.StatusMethodNotAllowed, response.StatusCode)
}

// TestConnection_HandleRequest_BadNoProcessor drives the unstarted-processor
// branch of handleRequest directly: a service that never had Start called has
// a nil processor, and the request must come back as a protocol error rather
// than panicking on the nil dereference.
func TestConnection_HandleRequest_BadNoProcessor(t *core.T) {
	svc := NewService(Options{Address: "127.0.0.1:0"})
	client := &socketClient{
		send: make(chan *wailsSocketMessage, 1),
		done: make(chan struct{}),
	}

	svc.handleRequest(core.Background(), client, wailsSocketMessage{
		ID:   "req-1",
		Type: messageTypeRequest,
	})

	select {
	case msg := <-client.send:
		core.AssertEqual(t, "req-1", msg.ID)
		core.RequireTrue(t, msg.Response != nil)
		core.AssertEqual(t, core.StatusUnprocessableEntity, msg.Response.StatusCode)
	default:
		t.Fatal("expected a protocol-error response queued on client.send")
	}
}

func TestConnection_Options_Good_EnvironmentOverrides(t *core.T) {
	t.Setenv("LTHN_WAILS_WS_LISTEN", "127.0.0.1:9191")
	t.Setenv("LTHN_WAILS_WS_PATH", "/proxy/wails/ws")
	t.Setenv("LTHN_WAILS_WS_URL", "wss://desktop.lethean.example/proxy/wails/ws")
	t.Setenv("LTHN_WAILS_WS_ORIGINS", "https://desktop.lethean.example, https://mobile.lethean.example ")
	t.Setenv("LTHN_WAILS_WS_TOKEN", "environment-secret")
	t.Setenv("LTHN_WAILS_WS_TRUST_PROXY", "yes")

	svc := NewService(Options{})
	core.AssertEqual(t, "127.0.0.1:9191", svc.options.Address)
	core.AssertEqual(t, "/proxy/wails/ws", svc.options.Path)
	core.AssertEqual(t, "wss://desktop.lethean.example/proxy/wails/ws", svc.options.PublicURL)
	core.AssertEqual(t, "environment-secret", svc.options.Token)
	core.AssertTrue(t, svc.options.TrustProxy)
	core.AssertEqual(t, 2, len(svc.options.AllowedOrigins))
	core.AssertEqual(t, "https://desktop.lethean.example", svc.options.AllowedOrigins[0])
	core.AssertEqual(t, "https://mobile.lethean.example", svc.options.AllowedOrigins[1])
}

func TestConnection_Options_Good_AllowsValidatedWailsDevOrigin(t *core.T) {
	t.Setenv("LTHN_WAILS_WS_ORIGINS", "")
	t.Setenv("WAILS_VITE_PORT", "9245")

	svc := NewService(Options{})

	core.RequireTrue(t, len(svc.options.AllowedOrigins) == 5)
	core.AssertEqual(t, "wails://localhost:9245", svc.options.AllowedOrigins[3])
	core.AssertEqual(t, "http://localhost:9245", svc.options.AllowedOrigins[4])
}

func TestConnection_Options_Bad_RejectsInvalidWailsDevOriginPort(t *core.T) {
	t.Setenv("LTHN_WAILS_WS_ORIGINS", "")
	t.Setenv("WAILS_VITE_PORT", "9245.evil.example")

	svc := NewService(Options{})

	core.AssertEqual(t, 3, len(svc.options.AllowedOrigins))
}

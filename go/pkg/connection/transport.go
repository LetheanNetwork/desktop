// SPDX-Licence-Identifier: EUPL-1.2

package connection

import (
	"crypto/subtle"

	core "dappco.re/go"
	"github.com/gorilla/websocket"
	"github.com/wailsapp/wails/v3/pkg/application"
)

const (
	messageTypeRequest  = "request"
	messageTypeResponse = "response"
	messageTypeEvent    = "event"
	maxRequestIDBytes   = 128
)

type wailsResponse struct {
	StatusCode int `json:"statusCode"`
	Data       any `json:"data"`
}

type wailsSocketMessage struct {
	ID       string                      `json:"id,omitempty"`
	Type     string                      `json:"type"`
	Request  *application.RuntimeRequest `json:"request,omitempty"`
	Response *wailsResponse              `json:"response,omitempty"`
	Event    *application.CustomEvent    `json:"event,omitempty"`
}

type socketClient struct {
	conn      *websocket.Conn
	send      chan *wailsSocketMessage
	done      chan struct{}
	slots     chan struct{}
	closeOnce core.Once
}

// wailsTransport is the narrow external-interface adapter. The Service keeps
// Result-shaped internals; only these methods translate back to Wails' required
// error signatures.
type wailsTransport struct {
	service *Service
}

func (t *wailsTransport) Start(ctx core.Context, processor *application.MessageProcessor) error {
	if t == nil || t.service == nil {
		return core.E("connection.Transport.Start", "service is nil", nil)
	}
	return t.service.start(ctx, processor).Err()
}

func (t *wailsTransport) ServeAssets(handler core.Handler) error {
	if t == nil || t.service == nil {
		return core.E("connection.Transport.ServeAssets", "service is nil", nil)
	}
	return t.service.serveAssets(handler).Err()
}

func (t *wailsTransport) JSClient() []byte {
	if t == nil || t.service == nil {
		return nil
	}
	return t.service.jsClient()
}

func (t *wailsTransport) Stop() error {
	if t == nil || t.service == nil {
		return nil
	}
	return t.service.stop().Err()
}

func (t *wailsTransport) DispatchWailsEvent(event *application.CustomEvent) {
	if t == nil || t.service == nil {
		return
	}
	t.service.dispatchWailsEvent(event)
}

func (s *Service) start(ctx core.Context, processor *application.MessageProcessor) core.Result {
	if s == nil {
		return core.Fail(core.E("connection.Start", "service is nil", nil))
	}
	if processor == nil {
		return core.Fail(core.E("connection.Start", "message processor is nil", nil))
	}
	if validation := s.validate(); !validation.OK {
		return validation
	}

	s.mu.Lock()
	if s.processor != nil {
		s.mu.Unlock()
		return core.Fail(core.E("connection.Start", "transport already started", nil))
	}
	s.processor = processor
	s.mu.Unlock()

	go func() {
		<-ctx.Done()
		if stopped := s.stop(); !stopped.OK {
			core.Warn("connection manager shutdown failed", "err", stopped.Error())
		}
	}()
	return core.Ok(nil)
}

func (s *Service) serveAssets(handler core.Handler) core.Result {
	if s == nil {
		return core.Fail(core.E("connection.ServeAssets", "service is nil", nil))
	}
	if handler == nil {
		return core.Fail(core.E("connection.ServeAssets", "asset handler is nil", nil))
	}

	s.mu.RLock()
	processorReady := s.processor != nil
	alreadyRunning := s.running
	s.mu.RUnlock()
	if !processorReady {
		return core.Fail(core.E("connection.ServeAssets", "transport has not been started", nil))
	}
	if alreadyRunning {
		return core.Fail(core.E("connection.ServeAssets", "server already running", nil))
	}

	mux := core.NewServeMux()
	mux.HandleFunc(s.options.Path, s.handleWebSocket)
	mux.Handle("/", handler)

	listen := core.NetListen("tcp", s.options.Address)
	if !listen.OK {
		return core.Fail(core.E("connection.ServeAssets", "listen on "+s.options.Address, listen.Err()))
	}
	listener := listen.Value.(core.Listener)
	server := &core.HTTPServer{
		Addr:              s.options.Address,
		Handler:           mux,
		ReadHeaderTimeout: defaultReadHeaderTimeout,
		IdleTimeout:       defaultIdleTimeout,
	}

	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		if err := listener.Close(); err != nil {
			core.Debug("connection manager duplicate listener close", "err", err)
		}
		return core.Fail(core.E("connection.ServeAssets", "server already running", nil))
	}
	s.server = server
	s.listener = listener
	s.boundAddr = listener.Addr().String()
	s.running = true
	s.mu.Unlock()

	go func() {
		if err := server.Serve(listener); err != nil && !core.Is(err, core.ErrHTTPServerClosed) {
			core.Error("connection manager socket server stopped", "err", err)
		}
	}()
	return core.Ok(nil)
}

func (s *Service) stop() core.Result {
	if s == nil {
		return core.Ok(nil)
	}

	s.mu.Lock()
	server := s.server
	clients := make([]*socketClient, 0, len(s.clients))
	for client := range s.clients {
		clients = append(clients, client)
	}
	s.server = nil
	s.listener = nil
	s.running = false
	s.mu.Unlock()

	for _, client := range clients {
		client.close()
	}
	if server == nil {
		return core.Ok(nil)
	}

	ctx, cancel := core.WithTimeout(core.Background(), s.options.ShutdownTimeout)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil && !core.Is(err, core.ErrHTTPServerClosed) {
		return core.Fail(core.E("connection.Stop", "graceful shutdown failed", err))
	}
	return core.Ok(nil)
}

func (s *Service) validate() core.Result {
	if s.options.Address == "" {
		return core.Fail(core.E("connection.Validate", "address is required", nil))
	}
	path := s.options.Path
	if path == "" || path == "/" || core.HasPrefix(path, "//") ||
		!core.HasPrefix(path, "/") || core.Contains(path, "?") || core.Contains(path, "#") {
		return core.Fail(core.E("connection.Validate", "path must be an absolute HTTP path", nil))
	}
	if len(path) > 256 {
		return core.Fail(core.E("connection.Validate", "path is too long", nil))
	}
	if publicURL := s.options.PublicURL; publicURL != "" {
		parsed := core.URLParse(publicURL)
		if !parsed.OK {
			return core.Fail(core.E("connection.Validate", "public URL is invalid", parsed.Err()))
		}
		value := parsed.Value.(*core.URL)
		if value.User != nil {
			return core.Fail(core.E("connection.Validate", "public URL must not contain credentials", nil))
		}
		if value.Fragment != "" {
			return core.Fail(core.E("connection.Validate", "public URL must not contain a fragment", nil))
		}
		for key := range value.Query() {
			if core.EqualFold(key, "access_token") {
				return core.Fail(core.E(
					"connection.Validate",
					"public URL must not contain an access token",
					nil,
				))
			}
		}
		if core.HasPrefix(publicURL, "/") {
			if core.HasPrefix(publicURL, "//") || value.Host != "" || !core.HasPrefix(value.Path, "/") {
				return core.Fail(core.E("connection.Validate", "relative public URL must be a root-relative path", nil))
			}
		} else if (value.Scheme != "ws" && value.Scheme != "wss") || value.Host == "" {
			return core.Fail(core.E("connection.Validate", "public URL must use ws or wss", nil))
		} else if value.Scheme == "ws" && !loopbackHost(value.Hostname()) {
			return core.Fail(core.E("connection.Validate", "remote public URL must use wss", nil))
		}
	}
	if !loopbackAddress(s.options.Address) && s.options.Token == "" && !s.options.TrustProxy {
		return core.Fail(core.E(
			"connection.Validate",
			"remote listen requires a token or an explicitly trusted authenticating proxy",
			nil,
		))
	}
	if s.options.MaxClients <= 0 || s.options.MaxPendingPerClient <= 0 || s.options.MaxMessageBytes <= 0 {
		return core.Fail(core.E("connection.Validate", "connection limits must be positive", nil))
	}
	return core.Ok(nil)
}

func loopbackAddress(address string) bool {
	parsed := core.URLParse(core.Concat("tcp://", core.Trim(address)))
	if !parsed.OK {
		return false
	}
	return loopbackHost(parsed.Value.(*core.URL).Hostname())
}

func loopbackHost(host string) bool {
	host = core.Lower(core.Trim(host))
	if host == "localhost" {
		return true
	}
	ip := core.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func (s *Service) handleWebSocket(w core.ResponseWriter, request *core.Request) {
	if request.Method != core.MethodGet {
		core.HTTPError(w, "websocket upgrade requires GET", core.StatusMethodNotAllowed)
		return
	}
	if !s.authorised(request) {
		core.HTTPError(w, "unauthorised websocket connection", core.StatusUnauthorized)
		return
	}
	if !s.reserveClient() {
		core.HTTPError(w, "websocket client limit reached", core.StatusTooManyRequests)
		return
	}
	reserved := true
	defer func() {
		if reserved {
			s.releaseClientReservation()
		}
	}()

	upgrader := websocket.Upgrader{
		CheckOrigin:       s.originAllowed,
		EnableCompression: true,
	}
	conn, err := upgrader.Upgrade(w, request, nil)
	if err != nil {
		return
	}
	conn.SetReadLimit(s.options.MaxMessageBytes)

	client := &socketClient{
		conn:  conn,
		send:  make(chan *wailsSocketMessage, s.options.MaxPendingPerClient*2),
		done:  make(chan struct{}),
		slots: make(chan struct{}, s.options.MaxPendingPerClient),
	}
	if !s.admitReservedClient(client) {
		client.close()
		return
	}
	reserved = false
	ctx, cancel := core.WithCancel(request.Context())
	defer func() {
		cancel()
		s.removeClient(client)
		client.close()
	}()

	go s.writeLoop(ctx, client)
	s.readLoop(ctx, client)
}

func (s *Service) authorised(request *core.Request) bool {
	if s.options.Token == "" {
		return true
	}
	provided := request.URL.Query().Get("access_token")
	if provided == "" {
		authorisation := core.Trim(request.Header.Get("Authorization"))
		if core.HasPrefix(core.Lower(authorisation), "bearer ") {
			provided = core.Trim(authorisation[len("Bearer "):])
		}
	}
	return subtle.ConstantTimeCompare([]byte(provided), []byte(s.options.Token)) == 1
}

func (s *Service) originAllowed(request *core.Request) bool {
	origin := request.Header.Get("Origin")
	if origin == "" {
		return true
	}
	for _, allowed := range s.options.AllowedOrigins {
		if allowed == "*" || subtle.ConstantTimeCompare([]byte(origin), []byte(allowed)) == 1 {
			return true
		}
	}
	return false
}

func (s *Service) readLoop(ctx core.Context, client *socketClient) {
	for {
		var message wailsSocketMessage
		if err := client.conn.ReadJSON(&message); err != nil {
			return
		}
		if message.Type != messageTypeRequest || message.Request == nil ||
			message.ID == "" || len(message.ID) > maxRequestIDBytes {
			id := message.ID
			if len(id) > maxRequestIDBytes {
				id = ""
			}
			s.sendProtocolError(ctx, client, id, "invalid websocket request")
			continue
		}
		if message.Request.Args == nil {
			message.Request.Args = &application.Args{}
		}

		if client.reserveRequest() {
			go func(message wailsSocketMessage) {
				defer client.releaseRequest()
				s.handleRequest(ctx, client, message)
			}(message)
		} else {
			s.sendProtocolError(ctx, client, message.ID, "too many pending requests")
		}
	}
}

func (s *Service) handleRequest(ctx core.Context, client *socketClient, message wailsSocketMessage) {
	s.mu.RLock()
	processor := s.processor
	s.mu.RUnlock()
	if processor == nil {
		s.sendProtocolError(ctx, client, message.ID, "message processor unavailable")
		return
	}

	value, err := processor.HandleRuntimeCallWithIDs(ctx, message.Request)
	response := &wailsResponse{StatusCode: core.StatusOK, Data: value}
	if err != nil {
		response.StatusCode = core.StatusUnprocessableEntity
		response.Data = err.Error()
	}
	s.send(ctx, client, &wailsSocketMessage{
		ID:       message.ID,
		Type:     messageTypeResponse,
		Response: response,
	})
}

func (s *Service) sendProtocolError(
	ctx core.Context,
	client *socketClient,
	id string,
	message string,
) {
	s.send(ctx, client, &wailsSocketMessage{
		ID:   id,
		Type: messageTypeResponse,
		Response: &wailsResponse{
			StatusCode: core.StatusUnprocessableEntity,
			Data:       message,
		},
	})
}

func (s *Service) send(ctx core.Context, client *socketClient, message *wailsSocketMessage) {
	select {
	case client.send <- message:
	case <-client.done:
	case <-ctx.Done():
	}
}

func (s *Service) writeLoop(ctx core.Context, client *socketClient) {
	defer client.close()
	for {
		select {
		case message := <-client.send:
			if err := client.conn.SetWriteDeadline(core.Now().Add(s.options.WriteTimeout)); err != nil {
				core.Debug("connection manager set write deadline", "err", err)
				return
			}
			if err := client.conn.WriteJSON(message); err != nil {
				return
			}
		case <-client.done:
			return
		case <-ctx.Done():
			return
		}
	}
}

func (s *Service) dispatchWailsEvent(event *application.CustomEvent) {
	if s == nil || event == nil {
		return
	}
	message := &wailsSocketMessage{
		Type: messageTypeEvent,
		Event: &application.CustomEvent{
			Name:   event.Name,
			Data:   event.Data,
			Sender: event.Sender,
		},
	}
	for _, client := range s.clientSnapshot() {
		select {
		case client.send <- message:
		case <-client.done:
		default:
			core.Warn("connection manager dropped event for slow client", "event", event.Name)
		}
	}
}

func (s *Service) reserveClient() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.running || len(s.clients)+s.admitting >= s.options.MaxClients {
		return false
	}
	s.admitting++
	return true
}

func (s *Service) releaseClientReservation() {
	s.mu.Lock()
	if s.admitting > 0 {
		s.admitting--
	}
	s.mu.Unlock()
}

func (s *Service) admitReservedClient(client *socketClient) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.admitting > 0 {
		s.admitting--
	}
	if !s.running {
		return false
	}
	s.clients[client] = struct{}{}
	return true
}

func (s *Service) removeClient(client *socketClient) {
	s.mu.Lock()
	delete(s.clients, client)
	s.mu.Unlock()
}

func (s *Service) clientSnapshot() []*socketClient {
	s.mu.RLock()
	defer s.mu.RUnlock()
	clients := make([]*socketClient, 0, len(s.clients))
	for client := range s.clients {
		clients = append(clients, client)
	}
	return clients
}

func (client *socketClient) close() {
	if client == nil {
		return
	}
	client.closeOnce.Do(func() {
		close(client.done)
		if err := client.conn.Close(); err != nil {
			core.Debug("connection manager client close", "err", err)
		}
	})
}

func (client *socketClient) reserveRequest() bool {
	if client == nil || client.slots == nil {
		return false
	}
	select {
	case client.slots <- struct{}{}:
		return true
	default:
		return false
	}
}

func (client *socketClient) releaseRequest() {
	if client == nil || client.slots == nil {
		return
	}
	select {
	case <-client.slots:
	default:
	}
}

func (s *Service) address() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.boundAddr
}

func (s *Service) jsClient() []byte {
	config := struct {
		WebSocketURL string `json:"webSocketUrl"`
	}{
		WebSocketURL: s.options.PublicURL,
	}
	payload := core.JSONMarshal(config)
	if !payload.OK {
		return []byte("globalThis.__LTHN_CONNECTION__={webSocketUrl:\"ws://localhost:9099/wails/ws\"};\n")
	}
	return []byte(core.Concat(
		"globalThis.__LTHN_CONNECTION__=Object.freeze(",
		string(payload.Value.([]byte)),
		");\n",
	))
}

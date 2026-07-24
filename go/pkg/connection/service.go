// SPDX-Licence-Identifier: EUPL-1.2

// Package connection owns the remotely reachable Wails binding transport.
//
// The desktop WebView, a browser served by the transport, and a separately
// hosted mobile client all speak the same request/response protocol over a
// configurable WebSocket endpoint. TLS is normally terminated by a reverse
// proxy: the browser uses wss:// while this service stays bound to loopback.
//
// Usage example:
//
//	svc := connection.NewService(connection.Options{})
//	if r := svc.Register(c); !r.OK { return r }
//	app := application.New(application.Options{Transport: svc.Transport()})
package connection

import (
	core "dappco.re/go"
	"github.com/wailsapp/wails/v3/pkg/application"
)

const (
	defaultAddress           = "127.0.0.1:9099"
	defaultPath              = "/wails/ws"
	defaultPublicURL         = "ws://localhost:9099/wails/ws"
	defaultMaxClients        = 64
	defaultMaxPending        = 64
	defaultMaxMessageBytes   = int64(64 * 1024 * 1024)
	defaultWriteTimeout      = 10 * core.Second
	defaultShutdownTimeout   = 5 * core.Second
	defaultReadHeaderTimeout = 10 * core.Second
	defaultIdleTimeout       = 60 * core.Second
)

// Options configures the Wails connection manager.
type Options struct {
	// Address is the backend TCP listen address. It defaults to loopback so
	// an unconfigured desktop never exposes control of its bindings to the
	// LAN. LTHN_WAILS_WS_LISTEN overrides an empty value.
	Address string
	// Path is the HTTP upgrade path. Default: /wails/ws.
	Path string
	// PublicURL is handed to the Angular connection manager. It may be an
	// absolute ws:// or wss:// URL, or a root-relative proxy path.
	PublicURL string
	// AllowedOrigins is an exact browser Origin allow-list. The defaults
	// cover the native Wails WebView and the loopback browser endpoint.
	AllowedOrigins []string
	// Token, when non-empty, must be supplied as the access_token query
	// parameter during the upgrade. Use WSS whenever the token leaves the
	// local machine.
	Token string
	// TrustProxy permits a non-loopback backend listener without Token.
	// Set this only when an authenticating reverse proxy is the sole route
	// to the listener.
	TrustProxy bool
	// MaxClients bounds simultaneously connected GUI/mobile/browser clients.
	MaxClients int
	// MaxPendingPerClient bounds concurrent binding calls from one client.
	MaxPendingPerClient int
	// MaxMessageBytes caps one incoming WebSocket message.
	MaxMessageBytes int64
	// WriteTimeout bounds a socket write.
	WriteTimeout core.Duration
	// ShutdownTimeout bounds graceful HTTP/WebSocket shutdown.
	ShutdownTimeout core.Duration
}

// Status is the connection manager's non-secret runtime snapshot.
type Status struct {
	Address   string `json:"address"`
	PublicURL string `json:"public_url"`
	Path      string `json:"path"`
	Clients   int    `json:"clients"`
	Running   bool   `json:"running"`
}

// Service owns the Wails transport adapter, socket listener, and active
// client set. Construct it with NewService and pass Transport() to Wails.
type Service struct {
	core      *core.Core
	options   Options
	transport *wailsTransport

	mu        core.RWMutex
	processor *application.MessageProcessor
	server    *core.HTTPServer
	listener  core.Listener
	boundAddr string
	clients   map[*socketClient]struct{}
	admitting int
	running   bool
}

// NewService constructs an unstarted connection manager.
//
// Usage example:
//
//	svc := connection.NewService(connection.Options{})
func NewService(options Options) *Service {
	options = optionsWithDefaults(options)
	service := &Service{
		options: options,
		clients: make(map[*socketClient]struct{}),
	}
	service.transport = &wailsTransport{service: service}
	return service
}

// Register wires the manager's status action onto Core. Wails itself starts
// the listener through Transport(); registration deliberately does not bind a
// port early.
//
// Usage example:
//
//	if r := svc.Register(c); !r.OK { return r }
func (s *Service) Register(c *core.Core) core.Result {
	if s == nil {
		return core.Fail(core.E("connection.Register", "service is nil", nil))
	}
	if c == nil {
		return core.Fail(core.E("connection.Register", "core is nil", nil))
	}
	s.core = c
	c.Action("connection.status", func(_ core.Context, _ core.Options) core.Result {
		return core.Ok(s.status())
	})
	return core.Ok(s)
}

// Register constructs and wires the default connection manager.
//
// Usage example:
//
//	if r := connection.Register(c); !r.OK { return r }
func Register(c *core.Core) core.Result {
	return NewService(Options{}).Register(c)
}

// Transport returns the stable Wails custom-transport adapter. Nil receivers
// return nil so optional desktop wiring can fail cleanly.
//
// Usage example:
//
//	opts.Transport = svc.Transport()
func (s *Service) Transport() application.Transport {
	if s == nil {
		return nil
	}
	return s.transport
}

func optionsWithDefaults(options Options) Options {
	if options.Address == "" {
		options.Address = core.Trim(core.Getenv("LTHN_WAILS_WS_LISTEN"))
	}
	if options.Address == "" {
		options.Address = defaultAddress
	}
	if options.Path == "" {
		options.Path = core.Trim(core.Getenv("LTHN_WAILS_WS_PATH"))
	}
	if options.Path == "" {
		options.Path = defaultPath
	}
	if options.PublicURL == "" {
		options.PublicURL = core.Trim(core.Getenv("LTHN_WAILS_WS_URL"))
	}
	if options.PublicURL == "" {
		options.PublicURL = defaultPublicURL
	}
	if len(options.AllowedOrigins) == 0 {
		options.AllowedOrigins = originsFromEnvironment()
	}
	if len(options.AllowedOrigins) == 0 {
		options.AllowedOrigins = []string{
			"wails://localhost",
			"http://localhost:9099",
			"http://127.0.0.1:9099",
		}
	}
	if options.Token == "" {
		options.Token = core.Trim(core.Getenv("LTHN_WAILS_WS_TOKEN"))
	}
	if !options.TrustProxy {
		trust := core.Lower(core.Trim(core.Getenv("LTHN_WAILS_WS_TRUST_PROXY")))
		options.TrustProxy = trust == "1" || trust == "true" || trust == "yes"
	}
	if options.MaxClients <= 0 {
		options.MaxClients = defaultMaxClients
	}
	if options.MaxPendingPerClient <= 0 {
		options.MaxPendingPerClient = defaultMaxPending
	}
	if options.MaxMessageBytes <= 0 {
		options.MaxMessageBytes = defaultMaxMessageBytes
	}
	if options.WriteTimeout <= 0 {
		options.WriteTimeout = defaultWriteTimeout
	}
	if options.ShutdownTimeout <= 0 {
		options.ShutdownTimeout = defaultShutdownTimeout
	}
	return options
}

func originsFromEnvironment() []string {
	raw := core.Trim(core.Getenv("LTHN_WAILS_WS_ORIGINS"))
	if raw == "" {
		return nil
	}
	values := core.Split(raw, ",")
	origins := make([]string, 0, len(values))
	for _, value := range values {
		if origin := core.Trim(value); origin != "" {
			origins = append(origins, origin)
		}
	}
	return origins
}

func (s *Service) status() Status {
	if s == nil {
		return Status{}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return Status{
		Address:   s.boundAddr,
		PublicURL: s.options.PublicURL,
		Path:      s.options.Path,
		Clients:   len(s.clients),
		Running:   s.running,
	}
}

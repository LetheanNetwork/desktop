// SPDX-Licence-Identifier: EUPL-1.2

// Package mdns broadcasts the lthn HTTP server over multicast DNS so
// any LAN-local device can discover it at http://lthn.local/ without
// needing to know the host's IP.
//
// `lthn.sh` (the magic-localhost arrangement in /etc/hosts) and
// `lthn.local` (this mDNS broadcast) coexist by design:
//
//	lthn.sh    → /etc/hosts → 127.0.0.1     "the lthn on THIS box"
//	lthn.local → mDNS → broadcaster's LAN IP "the lthn on the LAN"
//
// Both names work simultaneously; existing localhost behaviour is
// preserved. LAN discovery is additive.
//
// Spec: Mantis #1410 + RFC.marketplace §4 (domain routing).
//
// Backend: github.com/grandcat/zeroconf — pure-Go RFC 6762/6763
// implementation. The library handles multi-interface enumeration,
// IPv4 + IPv6 packet shaping, TTL renewals; this package wraps it in
// CoreGO conventions.
package mdns

import (
	"github.com/grandcat/zeroconf"

	core "dappco.re/go"
)

const (
	// serviceType is the DNS-SD service type advertised. _http._tcp
	// is the canonical "HTTP server" entry browsers + onboarding
	// tools look for first.
	serviceType = "_http._tcp"

	// serviceDomain is the mDNS scope. .local is RFC 6762's reserved
	// link-local domain; Bonjour (macOS) + Avahi (Linux) consult mDNS
	// only for .local names by default.
	serviceDomain = "local."

	// defaultServiceName is the broadcast handle. Resolves to
	// "lthn.local" on the LAN. Per-host instances can override via
	// Options.Hostname if multiple lthn run on the same wifi.
	defaultServiceName = "lthn"

	startOp = "mdns.OnStart"
)

// Options configures the mdns broadcast.
type Options struct {
	// Port is the HTTP port the lthn server listens on (e.g. 8000).
	// Resolved from server.Options.Addr by the caller; the mdns
	// package doesn't parse :addr strings.
	Port int

	// Hostname is the broadcast service name. Empty defaults to
	// "lthn" (resolves to lthn.local). Override when multiple lthn
	// instances run on the same wifi; convention is "<machine>-lthn".
	Hostname string

	// TXT carries DNS-SD TXT records as KEY=VALUE strings. Used to
	// surface capability metadata (version, marketplace, inference)
	// so peers can filter without an HTTP probe.
	TXT []string
}

// Service is the mdns broadcast lifecycle handle. Service.OnStart
// registers the broadcast; Service.OnStop shuts it down cleanly so
// the peer cache evicts the entry quickly (no orphan registrations).
//
// Wired into the Core service registry via core.WithName("mdns",
// mdns.Register).
type Service struct {
	core   *core.Core
	opts   Options
	server *zeroconf.Server // nil when not broadcasting
}

// NewService constructs a Service bound to the supplied Options.
// The broadcast doesn't start until OnStart fires (matches the rest
// of the Core service lifecycle conventions).
//
// Usage example:
//
//	svc := mdns.NewService(mdns.Options{Port: 8000})
func NewService(opts Options) *Service {
	if opts.Hostname == "" {
		opts.Hostname = defaultServiceName
	}
	return &Service{opts: opts}
}

// NewServiceWithCore wires the Service against a Core container.
// Used by Register; tests construct via NewService + Configure directly.
func NewServiceWithCore(c *core.Core, opts Options) *Service {
	s := NewService(opts)
	s.core = c
	return s
}

// Register is the Core service-registration factory. Matches the
// shape used by other lthn services (marketplace.Register,
// queue.Register, gateway.Register, etc.).
//
// Usage example:
//
//	core.WithName("mdns", mdns.Register)
func Register(c *core.Core) core.Result {
	return core.Ok(NewServiceWithCore(c, Options{}))
}

// Configure updates the runtime options before OnStart fires. The
// Wails-bindable surface uses this so the user-facing "Discoverable
// on local network" toggle can populate Port + TXT before the
// service starts broadcasting.
//
// Usage example:
//
//	svc.Configure(mdns.Options{Port: 8000, TXT: []string{"version=alpha.1"}})
func (s *Service) Configure(opts Options) {
	if s == nil {
		return
	}
	if opts.Hostname == "" {
		opts.Hostname = s.opts.Hostname
		if opts.Hostname == "" {
			opts.Hostname = defaultServiceName
		}
	}
	s.opts = opts
}

// ServiceName labels this service in the Core registry + Wails
// binding generation. Returns "Mdns".
func (s *Service) ServiceName() string { return "Mdns" }

// OnStart registers the mDNS broadcast. No-op when Port is zero —
// the caller hasn't supplied a port yet, so there's nothing to
// advertise. A subsequent Configure + OnStart call lights up.
//
// Returns Ok even when Port is zero so the service registration
// doesn't abort lthn boot — broadcast is a feature, not a hard
// dependency.
//
// Usage example:
//
//	if r := svc.OnStart(); !r.OK { /* log + carry on */ }
func (s *Service) OnStart() core.Result {
	if s == nil {
		return core.Fail(core.E(startOp, "service is nil", nil))
	}
	if s.server != nil {
		// Already broadcasting — idempotent no-op.
		return core.Ok(nil)
	}
	if s.opts.Port == 0 {
		// Caller hasn't supplied a port yet. Allowed shape: lthn boots,
		// the user toggles Discoverable, Configure(opts) then OnStart
		// lights it up.
		return core.Ok(nil)
	}
	server, err := zeroconf.Register(
		s.opts.Hostname,
		serviceType,
		serviceDomain,
		s.opts.Port,
		s.opts.TXT,
		nil, // nil interfaces = all multicast-capable
	)
	if err != nil {
		return core.Fail(core.E(startOp, "zeroconf register failed", err))
	}
	s.server = server
	return core.Ok(nil)
}

// OnStop shuts down the broadcast. Safe to call multiple times.
// The zeroconf library handles sending goodbye packets so peer
// caches evict promptly.
//
// Usage example:
//
//	_ = svc.OnStop()
func (s *Service) OnStop() core.Result {
	if s == nil || s.server == nil {
		return core.Ok(nil)
	}
	s.server.Shutdown()
	s.server = nil
	return core.Ok(nil)
}

// IsDiscoverable reports whether the broadcast is currently active.
// True only when OnStart has registered a non-nil zeroconf.Server
// and OnStop hasn't been called since.
//
// Usage example (Wails GUI poll):
//
//	if svc.IsDiscoverable() { /* show "discoverable" pill */ }
func (s *Service) IsDiscoverable() bool {
	return s != nil && s.server != nil
}

// SetDiscoverable toggles the broadcast at runtime. true → ensure
// broadcast is running (no-op if already); false → ensure broadcast
// is stopped (no-op if already stopped).
//
// The GUI's "Discoverable on local network" switch binds against
// this; the underlying lifecycle methods stay symmetric with the
// rest of the Core service-runtime conventions.
//
// Usage example (Wails):
//
//	svc.SetDiscoverable(true)
//	svc.SetDiscoverable(false)
func (s *Service) SetDiscoverable(on bool) core.Result {
	if s == nil {
		return core.Fail(core.E("mdns.SetDiscoverable", "service is nil", nil))
	}
	if on {
		return s.OnStart()
	}
	return s.OnStop()
}

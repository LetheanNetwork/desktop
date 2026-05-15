// SPDX-Licence-Identifier: EUPL-1.2

// Tests for the mDNS broadcast service. AX-10 G/B/U triads for
// Register / NewService / OnStart / OnStop / Configure /
// IsDiscoverable / SetDiscoverable.
//
// Tests register against the real mDNS stack via the zeroconf library
// (no mock) so the multicast-interface enumeration + packet shaping
// is actually exercised. Port collisions across parallel tests are
// avoided by picking a different port per test.

package mdns_test

import (
	core "dappco.re/go"

	"dappco.re/lthn/desktop/pkg/mdns"
)

// ─── NewService ────────────────────────────────────────────────────────

func TestMdns_NewService_Good(t *core.T) {
	svc := mdns.NewService(mdns.Options{Port: 18001, Hostname: "test-lthn"})
	core.AssertNotNil(t, svc)
	core.AssertEqual(t, "Mdns", svc.ServiceName())
}

func TestMdns_NewService_Bad(t *core.T) {
	// Empty Hostname defaults to "lthn" — not a failure path. The
	// constructor's only Bad shape is calling methods on a nil
	// service receiver; covered by SetDiscoverable + OnStart nil tests.
	svc := mdns.NewService(mdns.Options{Port: 0})
	core.AssertNotNil(t, svc)
	core.AssertFalse(t, svc.IsDiscoverable())
}

func TestMdns_NewService_Ugly(t *core.T) {
	// Two services with the same Hostname coexist as long as one
	// hasn't called OnStart yet — they share the same logical name
	// but the broadcast isn't active until OnStart fires.
	a := mdns.NewService(mdns.Options{Port: 18002, Hostname: "ugly-lthn"})
	b := mdns.NewService(mdns.Options{Port: 18003, Hostname: "ugly-lthn"})
	core.AssertNotNil(t, a)
	core.AssertNotNil(t, b)
	core.AssertFalse(t, a.IsDiscoverable())
	core.AssertFalse(t, b.IsDiscoverable())
}

// ─── Register (factory) ────────────────────────────────────────────────

func TestMdns_Register_Good(t *core.T) {
	c := core.New()
	r := mdns.Register(c)
	core.AssertTrue(t, r.OK)
	svc, ok := r.Value.(*mdns.Service)
	core.RequireTrue(t, ok)
	core.AssertEqual(t, "Mdns", svc.ServiceName())
}

// ─── OnStart / OnStop ──────────────────────────────────────────────────

func TestMdns_OnStart_Good(t *core.T) {
	svc := mdns.NewService(mdns.Options{
		Port:     18010,
		Hostname: "test-good-" + core.RandomString(4).Value.(string),
		TXT:      []string{"version=test", "feature=mdns-test"},
	})
	defer func() { _ = svc.OnStop() }()

	r := svc.OnStart()
	core.RequireTrue(t, r.OK)
	core.AssertTrue(t, svc.IsDiscoverable())
}

func TestMdns_OnStart_Bad(t *core.T) {
	// Nil service receiver fails cleanly without panicking.
	var nilSvc *mdns.Service
	r := nilSvc.OnStart()
	core.AssertFalse(t, r.OK)
}

func TestMdns_OnStart_Ugly(t *core.T) {
	// Zero port is a configured-but-not-yet-active state. OnStart
	// returns Ok but IsDiscoverable stays false; later Configure
	// can light it up.
	svc := mdns.NewService(mdns.Options{Port: 0})
	r := svc.OnStart()
	core.AssertTrue(t, r.OK)
	core.AssertFalse(t, svc.IsDiscoverable())

	// Double OnStart on an already-broadcasting service is idempotent.
	svc2 := mdns.NewService(mdns.Options{
		Port:     18011,
		Hostname: "test-ugly-" + core.RandomString(4).Value.(string),
	})
	defer func() { _ = svc2.OnStop() }()
	core.RequireTrue(t, svc2.OnStart().OK)
	r2 := svc2.OnStart() // second call — no-op Ok
	core.AssertTrue(t, r2.OK)
	core.AssertTrue(t, svc2.IsDiscoverable())
}

func TestMdns_OnStop_Good(t *core.T) {
	svc := mdns.NewService(mdns.Options{
		Port:     18020,
		Hostname: "test-stop-" + core.RandomString(4).Value.(string),
	})
	core.RequireTrue(t, svc.OnStart().OK)
	core.AssertTrue(t, svc.IsDiscoverable())

	r := svc.OnStop()
	core.AssertTrue(t, r.OK)
	core.AssertFalse(t, svc.IsDiscoverable())
}

func TestMdns_OnStop_Bad(t *core.T) {
	// Nil service stays a no-op Ok (defensive — Stop is a cleanup
	// path, not a place to fail loudly).
	var nilSvc *mdns.Service
	r := nilSvc.OnStop()
	core.AssertTrue(t, r.OK)
}

func TestMdns_OnStop_Ugly(t *core.T) {
	// Double OnStop is idempotent. Stop-without-start is also Ok.
	svc := mdns.NewService(mdns.Options{Port: 0})
	core.AssertTrue(t, svc.OnStop().OK)
	core.AssertTrue(t, svc.OnStop().OK)
}

// ─── Configure ─────────────────────────────────────────────────────────

func TestMdns_Configure_Good(t *core.T) {
	svc := mdns.NewService(mdns.Options{Port: 18030, Hostname: "first"})

	// Reconfigure picks up new port + hostname before OnStart fires.
	svc.Configure(mdns.Options{
		Port:     18031,
		Hostname: "second-" + core.RandomString(4).Value.(string),
		TXT:      []string{"reconfigured=true"},
	})
	core.RequireTrue(t, svc.OnStart().OK)
	defer func() { _ = svc.OnStop() }()
	core.AssertTrue(t, svc.IsDiscoverable())
}

func TestMdns_Configure_Bad(t *core.T) {
	// Nil service is a no-op (defensive) — no panic, no error
	// surfaced. The Configure return type is void so this just
	// verifies AssertNotPanics.
	core.AssertNotPanics(t, func() {
		var nilSvc *mdns.Service
		nilSvc.Configure(mdns.Options{Port: 18040})
	})
}

func TestMdns_Configure_Ugly(t *core.T) {
	// Configure with empty Hostname preserves the prior Hostname
	// rather than clobbering with empty (which would later default
	// to "lthn" but loses the user's earlier intent).
	svc := mdns.NewService(mdns.Options{Port: 18050, Hostname: "keepme"})
	svc.Configure(mdns.Options{Port: 18051}) // empty Hostname
	// We can't read .opts directly (private); validate via OnStart
	// behaviour. With the preserved Hostname the broadcast registers.
	core.RequireTrue(t, svc.OnStart().OK)
	defer func() { _ = svc.OnStop() }()
	core.AssertTrue(t, svc.IsDiscoverable())
}

// ─── SetDiscoverable ───────────────────────────────────────────────────

func TestMdns_SetDiscoverable_Good(t *core.T) {
	svc := mdns.NewService(mdns.Options{
		Port:     18060,
		Hostname: "test-toggle-" + core.RandomString(4).Value.(string),
	})
	defer func() { _ = svc.OnStop() }()

	core.AssertFalse(t, svc.IsDiscoverable())
	core.RequireTrue(t, svc.SetDiscoverable(true).OK)
	core.AssertTrue(t, svc.IsDiscoverable())
	core.RequireTrue(t, svc.SetDiscoverable(false).OK)
	core.AssertFalse(t, svc.IsDiscoverable())
}

func TestMdns_SetDiscoverable_Bad(t *core.T) {
	// Nil service surfaces a clean Fail (not a panic).
	var nilSvc *mdns.Service
	r := nilSvc.SetDiscoverable(true)
	core.AssertFalse(t, r.OK)
}

func TestMdns_SetDiscoverable_Ugly(t *core.T) {
	// Toggling on twice is idempotent; toggling off without ever
	// toggling on is idempotent.
	svc := mdns.NewService(mdns.Options{
		Port:     18070,
		Hostname: "test-toggle-ugly-" + core.RandomString(4).Value.(string),
	})
	defer func() { _ = svc.OnStop() }()

	core.RequireTrue(t, svc.SetDiscoverable(true).OK)
	core.RequireTrue(t, svc.SetDiscoverable(true).OK) // second on — still Ok
	core.AssertTrue(t, svc.IsDiscoverable())

	core.RequireTrue(t, svc.SetDiscoverable(false).OK)
	core.RequireTrue(t, svc.SetDiscoverable(false).OK) // second off — still Ok
	core.AssertFalse(t, svc.IsDiscoverable())
}

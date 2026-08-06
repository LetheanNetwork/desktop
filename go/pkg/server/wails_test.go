// SPDX-Licence-Identifier: EUPL-1.2

// Real tests for wails.go's Wails-lifecycle surface (ServiceName /
// ServiceStartup / ServiceShutdown / WAddr / WListening). Companion to
// service_test.go's diagnosis: the pre-existing wails_example_test.go
// only reflects on method VALUES via %T and never invokes them, so
// this file exercises the real call paths — including the nil-
// receiver defensive branches WAddr and WListening document.

package server_test

import (
	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/server"
)

func TestWails_Service_ServiceName_Good_ReturnsServer(t *core.T) {
	svc := server.NewService(server.Options{Addr: ":0"})
	core.AssertEqual(t, "Server", svc.ServiceName())
}

func TestWails_Service_ServiceLifecycle_Good_StartupAndShutdownAreOK(t *core.T) {
	svc := server.NewService(server.Options{Addr: ":0"})

	r := svc.ServiceStartup(core.Background(), nil)
	core.AssertTrue(t, r.OK)

	r = svc.ServiceShutdown()
	core.AssertTrue(t, r.OK)
}

func TestWails_Service_WAddr_Good_ReturnsConfiguredAddr(t *core.T) {
	svc := server.NewService(server.Options{Addr: ":9191"})
	r := svc.WAddr()
	core.RequireTrue(t, r.OK)
	core.AssertEqual(t, ":9191", r.Value.(string))
}

func TestWails_Service_WAddr_Bad_NilReceiverDefaultsAddr(t *core.T) {
	var svc *server.Service
	r := svc.WAddr()
	core.RequireTrue(t, r.OK)
	core.AssertEqual(t, ":8000", r.Value.(string))
}

func TestWails_Service_WListening_Good_FalseBeforeStart(t *core.T) {
	svc := server.NewService(server.Options{Addr: ":0"})
	r := svc.WListening()
	core.RequireTrue(t, r.OK)
	core.AssertEqual(t, false, r.Value.(bool))
}

func TestWails_Service_WListening_Bad_NilReceiverReturnsFalse(t *core.T) {
	var svc *server.Service
	r := svc.WListening()
	core.RequireTrue(t, r.OK)
	core.AssertEqual(t, false, r.Value.(bool))
}

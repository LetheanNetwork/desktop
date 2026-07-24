// SPDX-Licence-Identifier: EUPL-1.2

package connection_test

import (
	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/connection"
	"github.com/wailsapp/wails/v3/pkg/application"
)

func TestService_NewService_Good(t *core.T) {
	svc := connection.NewService(connection.Options{})
	core.AssertNotNil(t, svc)
	core.AssertNotNil(t, svc.Transport())
}

func TestService_NewService_Bad(t *core.T) {
	svc := connection.NewService(connection.Options{Path: "relative"})
	start := svc.Transport().Start(core.Background(), application.NewMessageProcessor(application.DefaultLogger(nil)))
	core.AssertError(t, start, "path")
}

func TestService_NewService_Ugly(t *core.T) {
	svc := connection.NewService(connection.Options{
		Address: "0.0.0.0:9099",
	})
	start := svc.Transport().Start(core.Background(), application.NewMessageProcessor(application.DefaultLogger(nil)))
	core.AssertError(t, start, "remote")
}

func TestService_Register_Good(t *core.T) {
	c := core.New()
	r := connection.Register(c)
	core.AssertTrue(t, r.OK)
	core.AssertNotNil(t, r.Value)
	_, ok := r.Value.(*connection.Service)
	core.AssertTrue(t, ok)
}

func TestService_Register_Bad(t *core.T) {
	r := connection.Register(nil)
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "core is nil")
}

func TestService_Register_Ugly(t *core.T) {
	c := core.New(core.WithName("connection", connection.Register))
	svc, ok := core.ServiceFor[*connection.Service](c, "connection")
	core.AssertTrue(t, ok)
	core.AssertNotNil(t, svc)
	core.AssertTrue(t, c.Action("connection.status").Run(core.Background(), core.NewOptions()).OK)
}

func TestService_Service_Register_Good(t *core.T) {
	c := core.New()
	svc := connection.NewService(connection.Options{})
	r := svc.Register(c)
	core.AssertTrue(t, r.OK)
	core.AssertEqual(t, svc, r.Value)
}

func TestService_Service_Register_Bad(t *core.T) {
	svc := connection.NewService(connection.Options{})
	r := svc.Register(nil)
	core.AssertFalse(t, r.OK)
}

func TestService_Service_Register_Ugly(t *core.T) {
	c := core.New()
	svc := connection.NewService(connection.Options{})
	core.AssertTrue(t, svc.Register(c).OK)
	status := c.Action("connection.status").Run(core.Background(), core.NewOptions())
	core.AssertTrue(t, status.OK)
	core.AssertEqual(t, 0, status.Value.(connection.Status).Clients)
}

func TestService_Service_Transport_Good(t *core.T) {
	svc := connection.NewService(connection.Options{})
	_, ok := svc.Transport().(application.AssetServerTransport)
	core.AssertTrue(t, ok)
}

func TestService_Service_Transport_Bad(t *core.T) {
	var svc *connection.Service
	core.AssertNil(t, svc.Transport())
}

func TestService_Service_Transport_Ugly(t *core.T) {
	svc := connection.NewService(connection.Options{})
	first := svc.Transport()
	second := svc.Transport()
	core.AssertEqual(t, first, second)
}

func TestService_NewService_Bad_InsecureRemotePublicURL(t *core.T) {
	svc := connection.NewService(connection.Options{
		PublicURL: "ws://desktop.lethean.example/wails/ws",
	})
	start := svc.Transport().Start(
		core.Background(),
		application.NewMessageProcessor(application.DefaultLogger(nil)),
	)
	core.AssertError(t, start, "wss")
}

func TestService_NewService_Bad_PublicURLCredentials(t *core.T) {
	svc := connection.NewService(connection.Options{
		PublicURL: "wss://user:secret@desktop.lethean.example/wails/ws",
	})
	start := svc.Transport().Start(
		core.Background(),
		application.NewMessageProcessor(application.DefaultLogger(nil)),
	)
	core.AssertError(t, start, "credentials")
}

func TestService_NewService_Bad_PublicURLAccessToken(t *core.T) {
	svc := connection.NewService(connection.Options{
		PublicURL: "wss://desktop.lethean.example/wails/ws?access_token=published-secret",
	})
	start := svc.Transport().Start(
		core.Background(),
		application.NewMessageProcessor(application.DefaultLogger(nil)),
	)
	core.AssertError(t, start, "access token")
}

func TestService_NewService_Bad_RootSocketPath(t *core.T) {
	svc := connection.NewService(connection.Options{Path: "/"})
	start := svc.Transport().Start(
		core.Background(),
		application.NewMessageProcessor(application.DefaultLogger(nil)),
	)
	core.AssertError(t, start, "path")
}

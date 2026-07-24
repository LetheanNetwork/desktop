// SPDX-Licence-Identifier: EUPL-1.2

package connection_test

import (
	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/connection"
)

func ExampleNewService() {
	svc := connection.NewService(connection.Options{
		Address:   "127.0.0.1:9099",
		PublicURL: "ws://localhost:9099/wails/ws",
	})
	_ = svc.Transport()
}

func ExampleRegister() {
	c := core.New()
	_ = connection.Register(c)
}

func ExampleService_Register() {
	c := core.New()
	svc := connection.NewService(connection.Options{})
	_ = svc.Register(c)
}

func ExampleService_Transport() {
	svc := connection.NewService(connection.Options{})
	_ = svc.Transport()
}

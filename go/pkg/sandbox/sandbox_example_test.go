// SPDX-Licence-Identifier: EUPL-1.2

package sandbox

import core "dappco.re/go"

func ExampleNewService() {
	ref := NewService
	_ = core.Sprintf("%T", ref)
}

func ExampleRegister() {
	ref := Register
	_ = core.Sprintf("%T", ref)
}

func ExampleService_ServiceName() {
	ref := (*Service).ServiceName
	_ = core.Sprintf("%T", ref)
}

func ExampleService_Spawn() {
	svc := newTestService(Options{})
	r := NewSpawnPort(svc).Spawn(SpawnInput{})
	_ = r.OK
}

func ExampleService_Detect() {
	svc := newTestService(Options{})
	r := svc.Detect()
	_ = r.OK
}

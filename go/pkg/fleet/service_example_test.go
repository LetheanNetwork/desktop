// SPDX-Licence-Identifier: EUPL-1.2

// Example*** functions for every public Service symbol. The v0.9.0
// audit requires one Example<Symbol> per exported declaration in
// service.go; these compile and serve as the canonical usage shape
// rendered by `go doc`.

package fleet_test

import (
	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/fleet"
)

func ExampleNew() {
	r := fleet.New()
	if r.OK {
		svc := r.Value.(*fleet.Service)
		_ = svc.Close()
	}
}

func ExampleRegister() {
	c := core.New()
	if r := fleet.Register(c); !r.OK {
		core.Println(r.Error())
	}
}

func ExampleService_Close() {
	r := fleet.New()
	if !r.OK {
		return
	}
	svc := r.Value.(*fleet.Service)
	_ = svc.Close()
}

func ExampleService_Machines() {
	r := fleet.New()
	if !r.OK {
		return
	}
	svc := r.Value.(*fleet.Service)
	defer svc.Close()
	m := svc.Machines()
	if m.OK {
		_ = m.Value.([]fleet.Machine)
	}
}

func ExampleService_Queue() {
	r := fleet.New()
	if !r.OK {
		return
	}
	svc := r.Value.(*fleet.Service)
	defer svc.Close()
	q := svc.Queue(20)
	if q.OK {
		_ = q.Value.([]fleet.QueueRow)
	}
}

func ExampleService_RoutingRules() {
	r := fleet.New()
	if !r.OK {
		return
	}
	svc := r.Value.(*fleet.Service)
	defer svc.Close()
	rr := svc.RoutingRules()
	if rr.OK {
		_ = rr.Value.([]fleet.RoutingRule)
	}
}

func ExampleService_UpsertMachine() {
	r := fleet.New()
	if !r.OK {
		return
	}
	svc := r.Value.(*fleet.Service)
	defer svc.Close()
	_ = svc.UpsertMachine(fleet.Machine{ID: "this-mac", Name: "this Mac", IsSelf: true})
}

func ExampleService_ServiceName() {
	r := fleet.New()
	if !r.OK {
		return
	}
	svc := r.Value.(*fleet.Service)
	defer svc.Close()
	_ = svc.ServiceName()
}

func ExampleService_ServiceStartup() {
	r := fleet.New()
	if !r.OK {
		return
	}
	svc := r.Value.(*fleet.Service)
	defer svc.Close()
	_ = svc.ServiceStartup(core.Background(), nil)
}

func ExampleService_ServiceShutdown() {
	r := fleet.New()
	if !r.OK {
		return
	}
	svc := r.Value.(*fleet.Service)
	_ = svc.ServiceShutdown()
}

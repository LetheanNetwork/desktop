// SPDX-Licence-Identifier: EUPL-1.2

// Example*** functions for every public Service symbol — required
// by the v0.9.0 audit. These compile and render via `go doc`.

package keys_test

import (
	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/keys"
)

func ExampleNew() {
	r := keys.New()
	if r.OK {
		svc := r.Value.(*keys.Service)
		_ = svc
	}
}

func ExampleRegister() {
	c := core.New()
	if r := keys.Register(c); !r.OK {
		core.Println(r.Error())
	}
}

func ExampleService_Put() {
	r := keys.New()
	if !r.OK {
		return
	}
	svc := r.Value.(*keys.Service)
	_ = svc.Put("openai-default", []byte("sk-abc"))
}

func ExampleService_Get() {
	r := keys.New()
	if !r.OK {
		return
	}
	svc := r.Value.(*keys.Service)
	got := svc.Get("openai-default")
	if got.OK {
		_ = got.Value.([]byte)
	}
}

func ExampleService_Delete() {
	r := keys.New()
	if !r.OK {
		return
	}
	svc := r.Value.(*keys.Service)
	_ = svc.Delete("openai-default")
}

func ExampleService_List() {
	r := keys.New()
	if !r.OK {
		return
	}
	svc := r.Value.(*keys.Service)
	l := svc.List()
	if l.OK {
		_ = l.Value.([]string)
	}
}

func ExampleService_Has() {
	r := keys.New()
	if !r.OK {
		return
	}
	svc := r.Value.(*keys.Service)
	h := svc.Has("openai-default")
	if h.OK {
		_ = h.Value.(bool)
	}
}

func ExampleService_ServiceName() {
	r := keys.New()
	if !r.OK {
		return
	}
	svc := r.Value.(*keys.Service)
	_ = svc.ServiceName()
}

func ExampleService_ServiceStartup() {
	r := keys.New()
	if !r.OK {
		return
	}
	svc := r.Value.(*keys.Service)
	_ = svc.ServiceStartup(core.Background(), nil)
}

func ExampleService_ServiceShutdown() {
	r := keys.New()
	if !r.OK {
		return
	}
	svc := r.Value.(*keys.Service)
	_ = svc.ServiceShutdown()
}

func ExampleService_WPut() {
	r := keys.New()
	if !r.OK {
		return
	}
	svc := r.Value.(*keys.Service)
	_ = svc.WPut("openai-default", "sk-abc")
}

func ExampleService_WList() {
	r := keys.New()
	if !r.OK {
		return
	}
	svc := r.Value.(*keys.Service)
	_ = svc.WList()
}

func ExampleService_WHas() {
	r := keys.New()
	if !r.OK {
		return
	}
	svc := r.Value.(*keys.Service)
	_ = svc.WHas("openai-default")
}

func ExampleService_WDelete() {
	r := keys.New()
	if !r.OK {
		return
	}
	svc := r.Value.(*keys.Service)
	_ = svc.WDelete("openai-default")
}

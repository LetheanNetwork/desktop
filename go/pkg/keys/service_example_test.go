// SPDX-Licence-Identifier: EUPL-1.2

// Example*** functions for every public Service symbol after the
// tier partition (RFC.stage-e-keys-partition v3 — Mantis #1625).
// These compile and render via `go doc`.

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

func ExampleService_PutTier0() {
	r := keys.New()
	if !r.OK {
		return
	}
	svc := r.Value.(*keys.Service)
	_ = svc.PutTier0("single-instance", []byte{0x99})
}

func ExampleService_GetTier0() {
	r := keys.New()
	if !r.OK {
		return
	}
	svc := r.Value.(*keys.Service)
	got := svc.GetTier0("single-instance")
	if got.OK {
		_ = got.Value.([]byte)
	}
}

func ExampleService_DeleteTier0() {
	r := keys.New()
	if !r.OK {
		return
	}
	svc := r.Value.(*keys.Service)
	_ = svc.DeleteTier0("single-instance")
}

func ExampleService_HasTier0() {
	r := keys.New()
	if !r.OK {
		return
	}
	svc := r.Value.(*keys.Service)
	h := svc.HasTier0("single-instance")
	if h.OK {
		_ = h.Value.(bool)
	}
}

func ExampleService_ListTier0() {
	r := keys.New()
	if !r.OK {
		return
	}
	svc := r.Value.(*keys.Service)
	l := svc.ListTier0()
	if l.OK {
		_ = l.Value.([]string)
	}
}

func ExampleService_GetOrCreateTier0() {
	r := keys.New()
	if !r.OK {
		return
	}
	svc := r.Value.(*keys.Service)
	_ = svc.GetOrCreateTier0("single-instance", func() ([]byte, error) {
		return []byte{0x99}, nil
	})
}

func ExampleService_PutTier1() {
	r := keys.New()
	if !r.OK {
		return
	}
	svc := r.Value.(*keys.Service)
	_ = svc.PutTier1("openai-default", []byte("sk-abc"))
}

func ExampleService_GetTier1() {
	r := keys.New()
	if !r.OK {
		return
	}
	svc := r.Value.(*keys.Service)
	got := svc.GetTier1("openai-default")
	if got.OK {
		_ = got.Value.([]byte)
	}
}

func ExampleService_DeleteTier1() {
	r := keys.New()
	if !r.OK {
		return
	}
	svc := r.Value.(*keys.Service)
	_ = svc.DeleteTier1("openai-default")
}

func ExampleService_HasTier1() {
	r := keys.New()
	if !r.OK {
		return
	}
	svc := r.Value.(*keys.Service)
	h := svc.HasTier1("openai-default")
	if h.OK {
		_ = h.Value.(bool)
	}
}

func ExampleService_ListTier1() {
	r := keys.New()
	if !r.OK {
		return
	}
	svc := r.Value.(*keys.Service)
	l := svc.ListTier1()
	if l.OK {
		_ = l.Value.([]string)
	}
}

func ExampleService_GetOrCreateTier1() {
	r := keys.New()
	if !r.OK {
		return
	}
	svc := r.Value.(*keys.Service)
	_ = svc.GetOrCreateTier1("openai-default", func() ([]byte, error) {
		return []byte("sk-generated"), nil
	})
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

func ExampleService_SetKEKProvider() {
	r := keys.New()
	if !r.OK {
		return
	}
	svc := r.Value.(*keys.Service)
	svc.SetKEKProvider(func() ([]byte, bool) {
		// Production wiring: derive via HKDF over unlocked PGP key.
		return make([]byte, 32), true
	})
}

func ExampleService_SetKEKProviderTier0() {
	r := keys.New()
	if !r.OK {
		return
	}
	svc := r.Value.(*keys.Service)
	svc.SetKEKProviderTier0(func() ([]byte, bool) {
		// Production wiring: derive via HKDF over .seed bytes.
		return make([]byte, 32), true
	})
}

func ExampleService_SingleInstanceKey() {
	r := keys.New()
	if !r.OK {
		return
	}
	svc := r.Value.(*keys.Service)
	// Requires SetKEKProviderTier0 wired first in production.
	k := svc.SingleInstanceKey()
	if k.OK {
		_ = k.Value.([32]byte)
	}
}

func ExampleService_WPutTier1() {
	r := keys.New()
	if !r.OK {
		return
	}
	svc := r.Value.(*keys.Service)
	_ = svc.WPutTier1("openai-default", "sk-abc")
}

func ExampleService_WListTier1() {
	r := keys.New()
	if !r.OK {
		return
	}
	svc := r.Value.(*keys.Service)
	_ = svc.WListTier1()
}

func ExampleService_WHasTier1() {
	r := keys.New()
	if !r.OK {
		return
	}
	svc := r.Value.(*keys.Service)
	_ = svc.WHasTier1("openai-default")
}

func ExampleService_WDeleteTier1() {
	r := keys.New()
	if !r.OK {
		return
	}
	svc := r.Value.(*keys.Service)
	_ = svc.WDeleteTier1("openai-default")
}

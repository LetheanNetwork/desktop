// SPDX-Licence-Identifier: EUPL-1.2

package benchmark

import (
	"testing"

	core "dappco.re/go"
)

// stubBencher is a tiny in-test Bencher used to exercise the registry
// without pulling in the canonical FixtureBencher (which lives in
// fixture.go and is itself tested via service_test.go).
type stubBencher struct {
	name string
	kind Kind
}

func (b *stubBencher) Name() string                  { return b.name }
func (b *stubBencher) Kind() Kind                    { return b.kind }
func (b *stubBencher) CanBench(_ Bench) bool         { return true }
func (b *stubBencher) Models(_ core.Context) core.Result {
	return core.Ok([]string{"stub-model"})
}
func (b *stubBencher) Bench(_ core.Context, _ Bench) core.Result {
	return core.Ok(Run{})
}

// describableStub adds an optional Describe() method so the registry
// can prove the optional-interface path lights up.
type describableStub struct{ stubBencher }

func (b *describableStub) Describe() string { return "describable stub" }

func TestRegisterAndLookup(t *testing.T) {
	r := newBencherRegistry()
	b := &stubBencher{name: "stub-a", kind: KindLocal}
	if res := r.register(b); !res.OK {
		t.Fatalf("register: want OK, got %v", res)
	}
	if r.count() != 1 {
		t.Fatalf("count: want 1, got %d", r.count())
	}
	got := r.bencher("stub-a")
	if !got.OK {
		t.Fatalf("bencher lookup: want OK, got %v", got)
	}
	if got.Value.(Bencher).Name() != "stub-a" {
		t.Fatalf("bencher Name: want stub-a, got %q", got.Value.(Bencher).Name())
	}
}

func TestRegisterRejectsNil(t *testing.T) {
	r := newBencherRegistry()
	if res := r.register(nil); res.OK {
		t.Fatal("register(nil): want Fail, got OK")
	}
}

func TestRegisterRejectsEmptyName(t *testing.T) {
	r := newBencherRegistry()
	if res := r.register(&stubBencher{name: "", kind: KindLocal}); res.OK {
		t.Fatal("register empty name: want Fail, got OK")
	}
}

func TestRegisterRejectsInvalidKind(t *testing.T) {
	r := newBencherRegistry()
	if res := r.register(&stubBencher{name: "x", kind: Kind("bogus")}); res.OK {
		t.Fatal("register bogus kind: want Fail, got OK")
	}
}

func TestRegisterRejectsDuplicateName(t *testing.T) {
	r := newBencherRegistry()
	if res := r.register(&stubBencher{name: "dup", kind: KindLocal}); !res.OK {
		t.Fatalf("first register: want OK, got %v", res)
	}
	if res := r.register(&stubBencher{name: "dup", kind: KindRemoteHTTP}); res.OK {
		t.Fatal("duplicate register: want Fail, got OK")
	}
}

func TestBencherMissingNameFails(t *testing.T) {
	r := newBencherRegistry()
	if res := r.bencher("nope"); res.OK {
		t.Fatal("bencher(nope): want Fail, got OK")
	}
}

func TestInfosRegistrationOrder(t *testing.T) {
	r := newBencherRegistry()
	_ = r.register(&stubBencher{name: "a", kind: KindLocal})
	_ = r.register(&stubBencher{name: "b", kind: KindSubprocess})
	_ = r.register(&stubBencher{name: "c", kind: KindRemoteHTTP})
	infos := r.infos()
	if len(infos) != 3 {
		t.Fatalf("infos len: want 3, got %d", len(infos))
	}
	for i, name := range []string{"a", "b", "c"} {
		if infos[i].Name != name {
			t.Errorf("infos[%d].Name: want %q, got %q", i, name, infos[i].Name)
		}
	}
	if infos[0].Kind != KindLocal {
		t.Errorf("infos[0].Kind: want local, got %s", infos[0].Kind)
	}
	if infos[2].Kind != KindRemoteHTTP {
		t.Errorf("infos[2].Kind: want remote-http, got %s", infos[2].Kind)
	}
}

func TestInfosDescribeOptional(t *testing.T) {
	r := newBencherRegistry()
	plain := &stubBencher{name: "plain", kind: KindLocal}
	described := &describableStub{stubBencher: stubBencher{name: "rich", kind: KindLocal}}
	_ = r.register(plain)
	_ = r.register(described)
	infos := r.infos()
	if infos[0].Description != "" {
		t.Errorf("plain Description: want empty, got %q", infos[0].Description)
	}
	if infos[1].Description != "describable stub" {
		t.Errorf("described Description: want %q, got %q", "describable stub", infos[1].Description)
	}
}

func TestUnregisterRemovesEntry(t *testing.T) {
	r := newBencherRegistry()
	_ = r.register(&stubBencher{name: "x", kind: KindLocal})
	if !r.reg.Has("x") {
		t.Fatal("setup: expected x to be registered")
	}
	if res := r.unregister("x"); !res.OK {
		t.Fatalf("unregister: want OK, got %v", res)
	}
	if r.reg.Has("x") {
		t.Fatal("unregister: x still present after removal")
	}
}

func TestUnregisterUnknownNameIsIdempotent(t *testing.T) {
	r := newBencherRegistry()
	// No setup — removing a never-registered name is a no-op.
	if res := r.unregister("never-existed"); !res.OK {
		t.Fatalf("unregister of unknown: want OK (idempotent), got %v", res)
	}
}

func TestUnregisterEmptyNameFails(t *testing.T) {
	r := newBencherRegistry()
	if res := r.unregister(""); res.OK {
		t.Fatal("unregister empty: want Fail, got OK")
	}
}

func TestIsValidKind(t *testing.T) {
	cases := map[Kind]bool{
		KindLocal:      true,
		KindSubprocess: true,
		KindRemoteHTTP: true,
		Kind(""):       false,
		Kind("bogus"):  false,
	}
	for k, want := range cases {
		if got := IsValidKind(k); got != want {
			t.Errorf("IsValidKind(%q): want %v, got %v", k, want, got)
		}
	}
}

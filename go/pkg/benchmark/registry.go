// SPDX-Licence-Identifier: EUPL-1.2

package benchmark

import (
	core "dappco.re/go"
)

// bencherRegistry is the substrate-side directory of Benchers. Wraps
// core.Registry[Bencher] to add duplicate-name rejection on register
// (the bare Registry.Set overwrites; here we reject so two adapters
// can't silently claim the same Run.Bencher identity).
//
// Not exported: callers reach Bencher operations through Service
// (RegisterBencher / Bencher / ListBenchers / Bench dispatch), not the
// registry directly. The split exists so Service can compose registry
// + storage without conflating their concerns.
type bencherRegistry struct {
	reg *core.Registry[Bencher]
}

// newBencherRegistry constructs an empty registry in open mode.
//
// Usage example:
//
//	r := newBencherRegistry()
func newBencherRegistry() *bencherRegistry {
	return &bencherRegistry{reg: core.NewRegistry[Bencher]()}
}

// register adds b under b.Name(). Rejects duplicate names — second
// registration of the same name returns a Fail result rather than
// silently overwriting the first. Validates Kind() against the
// canonical set; an unknown Kind is rejected so the UI never has to
// render an unknown badge.
//
// Usage example:
//
//	if r := reg.register(myBencher); !r.OK { return r }
func (r *bencherRegistry) register(b Bencher) core.Result {
	if b == nil {
		return core.Fail(core.E("benchmark.register", "bencher is nil", nil))
	}
	name := b.Name()
	if name == "" {
		return core.Fail(core.E("benchmark.register", "bencher Name() is empty", nil))
	}
	if !IsValidKind(b.Kind()) {
		return core.Fail(core.E("benchmark.register", core.Concat("bencher ", name, " has invalid Kind: ", string(b.Kind())), nil))
	}
	if r.reg.Has(name) {
		return core.Fail(core.E("benchmark.register", core.Concat("bencher already registered: ", name), nil))
	}
	return r.reg.Set(name, b)
}

// unregister removes a previously-registered Bencher by name.
// Idempotent — removing an unregistered name returns Ok rather than
// Fail so the caller can blanket-clean without juggling lookups
// first. Used by the Settings UI Remove flow + endpoint reconfigure
// path (delete-then-add as cheap Update).
//
// Usage example:
//
//	if r := reg.unregister("openai-compat:nim"); !r.OK { return r }
func (r *bencherRegistry) unregister(name string) core.Result {
	if name == "" {
		return core.Fail(core.E("benchmark.unregister", "name is empty", nil))
	}
	if !r.reg.Has(name) {
		return core.Ok(nil) // idempotent
	}
	return r.reg.Delete(name)
}

// bencher looks up a registered Bencher by name. Returns Fail when
// the name is not registered so callers can distinguish "no such
// bencher" from other failure modes.
//
// Usage example:
//
//	r := reg.bencher("lthn-mlx")
//	if r.OK { b := r.Value.(benchmark.Bencher); _ = b }
func (r *bencherRegistry) bencher(name string) core.Result {
	g := r.reg.Get(name)
	if !g.OK {
		return core.Fail(core.E("benchmark.bencher", core.Concat("not registered: ", name), nil))
	}
	return g
}

// infos returns the BencherInfo metadata view for every registered
// Bencher in registration order. Substrate consumers (Service.ListBenchers,
// frontend picker) read this rather than the live Bencher refs.
//
// Usage example:
//
//	for _, info := range reg.infos() {
//	    core.Println(info.Name, "·", string(info.Kind))
//	}
func (r *bencherRegistry) infos() []BencherInfo {
	names := r.reg.Names()
	out := make([]BencherInfo, 0, len(names))
	for _, name := range names {
		g := r.reg.Get(name)
		if !g.OK {
			continue
		}
		b := g.Value.(Bencher)
		out = append(out, BencherInfo{
			Name:        b.Name(),
			Kind:        b.Kind(),
			Description: describer(b),
		})
	}
	return out
}

// describer extracts the optional Description() string if the
// Bencher implements an optional Describer interface. Adapters that
// want richer ListBenchers output implement Describer; others get an
// empty Description.
//
// Usage example:
//
//	desc := describer(myBencher)  // "" if Bencher has no Describe()
func describer(b Bencher) string {
	if d, ok := b.(interface{ Describe() string }); ok {
		return d.Describe()
	}
	return ""
}

// count returns the number of registered Benchers. Used by tests +
// Service.ListBenchers to size the response array.
//
// Usage example:
//
//	if reg.count() == 0 { return core.Ok([]benchmark.BencherInfo{}) }
func (r *bencherRegistry) count() int {
	return r.reg.Len()
}

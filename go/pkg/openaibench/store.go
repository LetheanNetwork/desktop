// SPDX-Licence-Identifier: EUPL-1.2

package openaibench

import (
	core "dappco.re/go"

	"dappco.re/go/crypt/keys"
)

// keyPrefix namespaces openaibench endpoint configs in the pkg/keys
// tier-1 store. Grep-friendly + prevents collision with other tier-1
// consumers (vi provider tokens, runner provider credentials, etc.).
const keyPrefix = "openaibench:"

// EndpointConfig is the persisted shape of one OpenAI-compatible
// endpoint. The whole struct round-trips through pkg/keys as JSON
// bytes so Bearer + Org (the credential fields) are encrypted at
// rest alongside the non-credential metadata (Name + URL +
// Description). One primitive, one storage surface.
//
// Name is load-bearing — it doubles as the substrate Bencher.Name()
// identity AND the pkg/keys tier-1 ref (prefixed). Two endpoints
// cannot share a name.
//
// Usage example:
//
//	cfg := openaibench.EndpointConfig{
//	    Name:     "openai-compat:nim",
//	    URL:      "https://api.example.nim/v1",
//	    Bearer:   "sk-...",
//	}
//	r := openaibench.SaveEndpoint(c, cfg)
type EndpointConfig struct {
	Name        string `json:"name"`
	URL         string `json:"url"`
	Description string `json:"description,omitempty"`
	Bearer      string `json:"bearer,omitempty"`
	Org         string `json:"org,omitempty"`
}

// SaveEndpoint persists cfg via pkg/keys tier-1. Overwrites any
// existing config under the same Name (the substrate-side
// UnregisterBencher → re-register dance is the caller's
// responsibility — typically the wails.go layer handles it).
//
// Rejects empty Name (the pkg/keys ref can't be empty) and empty URL
// (validation belongs at the boundary so a broken config never
// round-trips clean).
//
// Usage example:
//
//	if r := openaibench.SaveEndpoint(c, cfg); !r.OK { return r }
func SaveEndpoint(c *core.Core, cfg EndpointConfig) core.Result {
	if c == nil {
		return core.Fail(core.E("openaibench.SaveEndpoint", "core is nil", nil))
	}
	if cfg.Name == "" {
		return core.Fail(core.E("openaibench.SaveEndpoint", "Name is required", nil))
	}
	if cfg.URL == "" {
		return core.Fail(core.E("openaibench.SaveEndpoint", "URL is required", nil))
	}
	keysSvc, ok := core.ServiceFor[*keys.Service](c, "keys")
	if !ok || keysSvc == nil {
		return core.Fail(core.E("openaibench.SaveEndpoint", "keys service not registered", nil))
	}
	b := core.JSONMarshal(cfg)
	if !b.OK {
		return b
	}
	return keysSvc.PutTier1(keyPrefix+cfg.Name, b.Value.([]byte))
}

// LoadEndpoints returns every persisted endpoint config, in stable
// pkg/keys-internal order. Failed individual decodes are logged via
// core.Warn and skipped so one bad record cannot poison the entire
// boot path — Cerberus #44 surface-disaster principle.
//
// Returns Ok with an empty slice when no endpoints have been saved.
//
// Usage example:
//
//	r := openaibench.LoadEndpoints(c)
//	if r.OK { cfgs := r.Value.([]openaibench.EndpointConfig); _ = cfgs }
func LoadEndpoints(c *core.Core) core.Result {
	if c == nil {
		return core.Fail(core.E("openaibench.LoadEndpoints", "core is nil", nil))
	}
	keysSvc, ok := core.ServiceFor[*keys.Service](c, "keys")
	if !ok || keysSvc == nil {
		return core.Fail(core.E("openaibench.LoadEndpoints", "keys service not registered", nil))
	}
	listR := keysSvc.ListTier1()
	if !listR.OK {
		return listR
	}
	refs, ok := listR.Value.([]string)
	if !ok {
		return core.Fail(core.E("openaibench.LoadEndpoints", "ListTier1 returned non-[]string", nil))
	}
	out := make([]EndpointConfig, 0, len(refs))
	for _, ref := range refs {
		if !hasPrefix(ref, keyPrefix) {
			continue
		}
		getR := keysSvc.GetTier1(ref)
		if !getR.OK {
			core.Warn("openaibench.LoadEndpoints.get_failed", "ref", ref, "error", getR.Error())
			continue
		}
		plaintext, ok := getR.Value.([]byte)
		if !ok {
			core.Warn("openaibench.LoadEndpoints.cast_failed", "ref", ref)
			continue
		}
		var cfg EndpointConfig
		if r := core.JSONUnmarshal(plaintext, &cfg); !r.OK {
			core.Warn("openaibench.LoadEndpoints.unmarshal_failed", "ref", ref, "error", r.Error())
			continue
		}
		out = append(out, cfg)
	}
	return core.Ok(out)
}

// DeleteEndpoint removes the persisted config for name. Idempotent:
// removing a non-existent name returns Ok (matches the substrate
// UnregisterBencher idempotency contract). The caller is responsible
// for calling benchmark.Service.UnregisterBencher to drop the live
// Bencher; wails.go layer wires the two together.
//
// Usage example:
//
//	if r := openaibench.DeleteEndpoint(c, "openai-compat:nim"); !r.OK { return r }
func DeleteEndpoint(c *core.Core, name string) core.Result {
	if c == nil {
		return core.Fail(core.E("openaibench.DeleteEndpoint", "core is nil", nil))
	}
	if name == "" {
		return core.Fail(core.E("openaibench.DeleteEndpoint", "name is required", nil))
	}
	keysSvc, ok := core.ServiceFor[*keys.Service](c, "keys")
	if !ok || keysSvc == nil {
		return core.Fail(core.E("openaibench.DeleteEndpoint", "keys service not registered", nil))
	}
	return keysSvc.DeleteTier1(keyPrefix + name)
}

// hasPrefix is a tiny local helper to avoid importing strings (per
// CoreGO discipline of stdlib wrap-everything). Local rather than
// reaching for core.HasPrefix keeps the dependency surface narrow.
func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

// SPDX-Licence-Identifier: EUPL-1.2

package openaibench

import (
	core "dappco.re/go"

	"dappco.re/lthn/desktop/pkg/benchmark"
)

// WailsService is the openai-compat endpoint configuration surface the
// frontend Settings UI binds to. List/Add/Remove against the
// pkg/keys-backed store, with each mutation re-synced into the live
// benchmark.Service so the bencher picker reflects changes the moment
// the user closes the modal — no app restart needed.
//
// Construct via NewWailsService(c, benchmarkSvc) at boot AFTER both
// the keys Service and the benchmark Service are wired. Bound into
// pkg/desktop's wailsServices array so the WebView gets the standard
// @desktop/openaibench/wailsservice TypeScript binding.
//
// Tier-auth: deliberately ungated at this layer. The underlying
// keys.PutTier1 / DeleteTier1 already gate via pkg/auth + emit the
// EventKeysTier1* audit cluster. Adding a redundant gate here would
// double-tax every credential mutation without adding security.
//
// Usage example (boot wire):
//
//	wsvc := openaibench.NewWailsService(s.opts.Core, benchmarkSvc)
//	// Append to wailsServices: application.NewService(wsvc)
type WailsService struct {
	core *core.Core
	bm   *benchmark.Service
}

// NewWailsService constructs the WailsService. Both arguments are
// required — nil core or nil benchmark service surfaces a clear
// runtime failure rather than silent partial functionality.
//
// Usage example:
//
//	wsvc := openaibench.NewWailsService(c, benchmarkSvc)
func NewWailsService(c *core.Core, bm *benchmark.Service) *WailsService {
	return &WailsService{core: c, bm: bm}
}

// ServiceName labels the binding namespace exposed to JS as
// "OpenaibenchSettings" — Wails generates the TS binding under
// frontend/bindings/dappco.re/lthn/desktop/pkg/openaibench/.
func (w *WailsService) ServiceName() string { return "OpenaibenchSettings" }

// ServiceStartup runs at app boot. No-op today — the benchmark.Service
// is the integration point and lives elsewhere; this surface is a
// thin storage + sync passthrough.
func (w *WailsService) ServiceStartup(_ core.Context, _ any) core.Result {
	return core.Ok(nil)
}

// ServiceShutdown runs at app exit. No-op today.
func (w *WailsService) ServiceShutdown() core.Result { return core.Ok(nil) }

// PublicEndpoint is the Settings-UI safe view of an EndpointConfig —
// strips the Bearer field so the credential never crosses the Wails
// boundary in cleartext on read paths. Add / Update paths take the
// full EndpointConfig so the user can supply / rotate it; the read
// surface only confirms whether a token is set.
//
// Usage example (TS):
//
//	const list = await ListEndpoints();
//	const hasAuth = list[0].auth_set;
type PublicEndpoint struct {
	Name        string `json:"name"`
	URL         string `json:"url"`
	Description string `json:"description,omitempty"`
	Org         string `json:"org,omitempty"`
	AuthSet     bool   `json:"auth_set"`
}

// ListEndpoints returns the persisted endpoint configs in their
// stored order, with Bearer tokens stripped. AuthSet flags whether
// a Bearer is configured so the UI can show a "key configured"
// indicator without ever rendering the secret.
//
// Usage example:
//
//	r := wsvc.ListEndpoints()
//	if r.OK { items := r.Value.([]PublicEndpoint); _ = items }
func (w *WailsService) ListEndpoints() core.Result {
	r := LoadEndpoints(w.core)
	if !r.OK {
		return r
	}
	cfgs, ok := r.Value.([]EndpointConfig)
	if !ok {
		return core.Fail(core.E("openaibench.ListEndpoints", "LoadEndpoints returned non-[]EndpointConfig", nil))
	}
	out := make([]PublicEndpoint, 0, len(cfgs))
	for _, c := range cfgs {
		out = append(out, PublicEndpoint{
			Name:        c.Name,
			URL:         c.URL,
			Description: c.Description,
			Org:         c.Org,
			AuthSet:     c.Bearer != "",
		})
	}
	return core.Ok(out)
}

// AddEndpoint persists cfg + registers a live Bencher with the
// benchmark substrate in one shot. If a Bencher already exists under
// the same name (either from a prior boot-time registration OR a
// prior Add), it's unregistered first so this call doubles as Update.
//
// Returns the PublicEndpoint shape on success so the UI can pop the
// row in immediately without a follow-up List call.
//
// Usage example:
//
//	r := wsvc.AddEndpoint(EndpointConfig{Name: "x", URL: "...", Bearer: "sk-..."})
//	if r.OK { added := r.Value.(PublicEndpoint); _ = added }
func (w *WailsService) AddEndpoint(cfg EndpointConfig) core.Result {
	if w.bm == nil {
		return core.Fail(core.E("openaibench.AddEndpoint", "benchmark service not wired", nil))
	}
	if r := SaveEndpoint(w.core, cfg); !r.OK {
		return r
	}
	// Substrate re-sync: unregister any existing Bencher with this
	// name (idempotent, no-op when fresh) then register the new one.
	// Unregister failure is treated as soft (logged) — the persisted
	// config landed; the live registration retry path is the next
	// boot.
	if r := w.bm.UnregisterBencher(cfg.Name); !r.OK {
		core.Warn("openaibench.AddEndpoint.unregister_failed", "name", cfg.Name, "error", r.Error())
	}
	b := NewBencher(Options{
		Name:        cfg.Name,
		Endpoint:    cfg.URL,
		Description: cfg.Description,
		Bearer:      cfg.Bearer,
		Org:         cfg.Org,
	})
	if b == nil {
		return core.Fail(core.E("openaibench.AddEndpoint", "NewBencher returned nil (validation gap?)", nil))
	}
	if r := w.bm.RegisterBencher(b); !r.OK {
		core.Warn("openaibench.AddEndpoint.register_failed", "name", cfg.Name, "error", r.Error())
		return r
	}
	return core.Ok(PublicEndpoint{
		Name:        cfg.Name,
		URL:         cfg.URL,
		Description: cfg.Description,
		Org:         cfg.Org,
		AuthSet:     cfg.Bearer != "",
	})
}

// RemoveEndpoint deletes the persisted config + unregisters the live
// Bencher. Idempotent — removing a non-existent endpoint returns Ok.
//
// Usage example:
//
//	r := wsvc.RemoveEndpoint("openai-compat:nim")
//	if r.OK { /* picker drops the entry on next refresh */ }
func (w *WailsService) RemoveEndpoint(name string) core.Result {
	if w.bm == nil {
		return core.Fail(core.E("openaibench.RemoveEndpoint", "benchmark service not wired", nil))
	}
	if name == "" {
		return core.Fail(core.E("openaibench.RemoveEndpoint", "name is required", nil))
	}
	if r := DeleteEndpoint(w.core, name); !r.OK {
		return r
	}
	if r := w.bm.UnregisterBencher(name); !r.OK {
		core.Warn("openaibench.RemoveEndpoint.unregister_failed", "name", name, "error", r.Error())
	}
	return core.Ok(nil)
}

// RegisterPersistedEndpoints loads every stored EndpointConfig + adds
// each as a live Bencher against the supplied benchmark.Service.
// Called from pkg/desktop boot after both services are wired so the
// picker reflects the operator's last-known configuration without
// requiring a Settings round-trip first.
//
// Failed individual NewBencher / RegisterBencher calls are logged
// and skipped so one bad row doesn't poison the whole boot path —
// matches the LoadEndpoints surface-disaster posture.
//
// Usage example (boot wire):
//
//	if r := openaibench.RegisterPersistedEndpoints(c, benchmarkSvc); !r.OK {
//	    core.Warn("desktop.openaibench.boot_register", "error", r.Error())
//	}
func RegisterPersistedEndpoints(c *core.Core, bm *benchmark.Service) core.Result {
	if c == nil {
		return core.Fail(core.E("openaibench.RegisterPersistedEndpoints", "core is nil", nil))
	}
	if bm == nil {
		return core.Fail(core.E("openaibench.RegisterPersistedEndpoints", "benchmark service is nil", nil))
	}
	r := LoadEndpoints(c)
	if !r.OK {
		return r
	}
	cfgs, _ := r.Value.([]EndpointConfig)
	for _, cfg := range cfgs {
		b := NewBencher(Options{
			Name:        cfg.Name,
			Endpoint:    cfg.URL,
			Description: cfg.Description,
			Bearer:      cfg.Bearer,
			Org:         cfg.Org,
		})
		if b == nil {
			core.Warn("openaibench.RegisterPersistedEndpoints.new_bencher_nil", "name", cfg.Name)
			continue
		}
		if rr := bm.RegisterBencher(b); !rr.OK {
			core.Warn("openaibench.RegisterPersistedEndpoints.register_failed", "name", cfg.Name, "error", rr.Error())
			continue
		}
	}
	return core.Ok(nil)
}

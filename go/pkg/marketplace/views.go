// SPDX-Licence-Identifier: EUPL-1.2

// Plugin-view runtime registry — the in-process source of truth that
// the frontend descriptor lookup, CSP frame-src middleware, capability
// table + postMessage origin verification all read from.
//
// Per plans/code/lthn/desktop/views/RFC.plugin-views.md:
//
//   - §2 — GetViewDescriptor reads from this registry; uninstall
//     mutates it (Drop) before the iframe teardown (HIGH-4 ordering).
//   - §3.3 — cspMiddleware enumerates registered iframe ports per
//     response (CRIT-1). Empty registry → frame-src 'none'.
//   - §5.1 — host postMessage handler keys "is this contentWindow
//     allowed?" via Lookup(code) → expected loopback origin.
//   - §7.2 — origin-isolation invariant: each iframe plugin gets a
//     unique loopback port. Register enforces port-collision reject.
//
// The registry is process-local — Install populates it after the
// orm record is written; Uninstall drops it before the orm record is
// removed (per §2.1 step 1).
//
// Usage example (host-side):
//
//	desc := marketplace.PluginViewDescriptor{
//	    ID:           "opencode",
//	    PluginCode:   "opencode",
//	    Kind:         marketplace.PluginViewKindIframe,
//	    LoopbackPort: 4096,
//	    LoopbackOrigin: "http://127.0.0.1:4096",
//	    Capabilities: []string{"session-token"},
//	}
//	marketplace.ViewRegistry.Add("opencode", desc)
//
// Usage example (CSP middleware):
//
//	ports := marketplace.ViewRegistry.IframePorts()
//	// emits frame-src 'none' http://127.0.0.1:4096 http://127.0.0.1:4097

package marketplace

import (
	core "dappco.re/go"
)

// PluginViewDescriptor is the resolved view shape the frontend
// consumes via marketplace.GetViewDescriptor. Produced by the Go host
// at install time (from the manifest PluginView block + the actual
// loopback port assigned to the iframe-kind plugin).
//
// Mirrors the frontend-side TS interface in frontend-ng/src/app/desktop/
// surfaces/extensions/plugin-view-runtime.ts. Field names use JSON tags that
// match the TS interface keys verbatim — the SDK gen emits the same
// shape on both sides.
type PluginViewDescriptor struct {
	// ID is the globally-unique view id (e.g. "opencode" or
	// "opencode:terminal"). Manifest validator enforces uniqueness +
	// identity rules per §2.
	ID string `json:"id"`

	// Label is the user-facing name shown in the switcher + ⌘P
	// palette.
	Label string `json:"label"`

	// Icon is the Font Awesome class (e.g. "fa-terminal"). Defaults
	// to "fa-cube" when the manifest omits it.
	Icon string `json:"icon"`

	// Group is always "plugin" — built-in groups (primary / observe
	// / extend / preview / bottom) are reserved for the seven role
	// views.
	Group string `json:"group"`

	// Kind discriminates render-time path: PluginViewKindIframe or
	// PluginViewKindLit. The frontend picks an iframe wrapper vs a
	// custom-element tag based on this field.
	Kind string `json:"kind"`

	// Source is either the iframe URL (loopback, resolved at install
	// from ${expose.<id>.route}) when Kind == iframe, OR the custom-
	// element tag (e.g. "lthn-coding-panel") when Kind == lit.
	Source string `json:"source"`

	// PluginCode back-references the bundle. Used for uninstall +
	// per-plugin storage namespacing + capability scope.
	PluginCode string `json:"pluginCode"`

	// Capabilities is the auth-scope grant set declared by the
	// manifest. The frontend prompts the user at install; the
	// postMessage handshake honours only granted scopes.
	Capabilities []string `json:"capabilities,omitempty"`

	// LoopbackPort is the iframe plugin's loopback port (from the
	// manifest expose block). Zero for lit-kind views. Source of
	// truth for §3.3 CSP frame-src allowlist.
	LoopbackPort int `json:"loopbackPort,omitempty"`

	// LoopbackOrigin is the http://127.0.0.1:<port> origin used by
	// the §5.1 postMessage inbound verification. Empty for lit-kind
	// views.
	LoopbackOrigin string `json:"loopbackOrigin,omitempty"`

	// Proxied marks an iframe view that loads via the host reverse proxy
	// (/v1/api/plugin/<code>/*) rather than the container's loopback origin
	// directly — so the proxy strips the contained app's framing headers.
	// When true, Source is the proxy route and the frontend mounts it as a
	// same-origin iframe (no direct loopback). Mirrors the opencode format.
	Proxied bool `json:"proxied,omitempty"`
}

// pluginViewRegistry is the in-process registry. ViewRegistry below
// is the package-level singleton callers depend on. Mutex-guarded
// because Install / Uninstall mutate from the marketplace.Service
// goroutine while cspMiddleware reads from every HTTP request
// goroutine.
type pluginViewRegistry struct {
	mu core.RWMutex
	// descriptors keyed by view id → frontend lookup target.
	descriptors map[string]PluginViewDescriptor
	// byPlugin maps plugin code → list of view ids that plugin owns.
	// Used by Drop to remove every descriptor a plugin registered.
	byPlugin map[string][]string
	// ports maps plugin code → loopback port. Used by CSP middleware
	// to enumerate distinct iframe ports without dedup ceremony at
	// the call site.
	ports map[string]int
}

// ViewRegistry is the package-level singleton plugin-view registry.
// All marketplace lifecycle code routes through this instance so
// the source of truth stays one place.
var ViewRegistry = newPluginViewRegistry()

// newPluginViewRegistry constructs an empty registry. Callers
// outside tests use the package-level ViewRegistry.
func newPluginViewRegistry() *pluginViewRegistry {
	return &pluginViewRegistry{
		descriptors: map[string]PluginViewDescriptor{},
		byPlugin:    map[string][]string{},
		ports:       map[string]int{},
	}
}

// Add registers one descriptor under (pluginCode, descriptor.ID).
// Returns a Fail Result on port collision with a different plugin
// (RFC §7.2 origin-isolation invariant).
//
// Usage example:
//
//	r := marketplace.ViewRegistry.Add("opencode", desc)
//	if !r.OK { return r }
func (r *pluginViewRegistry) Add(pluginCode string, d PluginViewDescriptor) core.Result {
	pluginCode = core.Trim(pluginCode)
	if pluginCode == "" {
		return core.Fail(core.E("marketplace.ViewRegistry.Add",
			"plugin code is required", nil))
	}
	if core.Trim(d.ID) == "" {
		return core.Fail(core.E("marketplace.ViewRegistry.Add",
			"descriptor id is required", nil))
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if d.Kind == PluginViewKindIframe && d.LoopbackPort > 0 {
		for owner, port := range r.ports {
			if owner != pluginCode && port == d.LoopbackPort {
				return core.Fail(core.E("marketplace.ViewRegistry.Add",
					core.Sprintf("loopback port %d already registered by plugin %q",
						d.LoopbackPort, owner), nil))
			}
		}
		r.ports[pluginCode] = d.LoopbackPort
	}
	r.descriptors[d.ID] = d
	r.byPlugin[pluginCode] = append(r.byPlugin[pluginCode], d.ID)
	return core.Ok(nil)
}

// Drop removes every descriptor a plugin registered + frees the
// loopback port slot. Called by Uninstall per §2.1 step 1+2+3 so
// CSP frame-src, descriptor lookup, postMessage origin table all
// see the narrowed state BEFORE the iframe teardown.
//
// Idempotent — dropping an unknown plugin code is a no-op.
//
// Usage example:
//
//	marketplace.ViewRegistry.Drop("opencode")
func (r *pluginViewRegistry) Drop(pluginCode string) {
	pluginCode = core.Trim(pluginCode)
	if pluginCode == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	ids := r.byPlugin[pluginCode]
	for _, id := range ids {
		delete(r.descriptors, id)
	}
	delete(r.byPlugin, pluginCode)
	delete(r.ports, pluginCode)
}

// Lookup returns the descriptor for a view id. Second return value
// is false when the id is not registered.
//
// Usage example:
//
//	d, ok := marketplace.ViewRegistry.Lookup("opencode")
//	if !ok { /* fall back per §6 chain */ }
func (r *pluginViewRegistry) Lookup(id string) (PluginViewDescriptor, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	d, ok := r.descriptors[id]
	return d, ok
}

// All returns a snapshot of every registered descriptor. Used by
// the frontend on boot to merge plugin views into the VIEWS signal,
// and by tests that need to assert post-install state.
func (r *pluginViewRegistry) All() []PluginViewDescriptor {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]PluginViewDescriptor, 0, len(r.descriptors))
	for _, d := range r.descriptors {
		out = append(out, d)
	}
	return out
}

// IframePorts returns the distinct set of loopback ports currently
// registered to iframe-kind plugins. Used by the CSP middleware to
// emit `frame-src 'none' http://127.0.0.1:<p1> http://127.0.0.1:<p2>`
// per §3.3. Returns nil when no iframe plugins are installed
// (caller emits `frame-src 'none'`).
//
// Usage example:
//
//	for _, port := range marketplace.ViewRegistry.IframePorts() {
//	    builder.WriteString(core.Sprintf(" http://127.0.0.1:%d", port))
//	}
func (r *pluginViewRegistry) IframePorts() []int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if len(r.ports) == 0 {
		return nil
	}
	seen := map[int]bool{}
	out := make([]int, 0, len(r.ports))
	for _, p := range r.ports {
		if p <= 0 || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	return out
}

// LoopbackOriginFor returns the expected loopback origin for a given
// plugin code. Inverse of PluginCodeForOrigin: callers that already
// know the pluginCode (e.g. the /v1/plugin-view/capability-grant
// receiver after the PluginInstalledChecker gate) consult this to
// cross-check the request's `origin` field against the registry's
// authoritative LoopbackOrigin record.
//
// Cerberus #21 (Mantis #1595) — handler validating (plugin_id, origin)
// INDEPENDENTLY lets a falsified (pluginCode, origin) tuple commit to
// audit even though the registry holds the truth. The browser's
// postMessage targetOrigin enforcement is the safety floor against
// token leakage; this accessor closes the audit-log-integrity gap.
//
// Returns ("", false) when the plugin code is unknown OR is registered
// but does not own an iframe-kind view (lit-kind plugins have no
// loopback origin to cross-check; callers fall through to existing flow).
//
// Usage example:
//
//	expected, ok := marketplace.ViewRegistry.LoopbackOriginFor(req.PluginID)
//	if ok && req.Origin != expected {
//	    // 400 codePluginViewGrantInvalid + ZERO audit rows
//	    return
//	}
func (r *pluginViewRegistry) LoopbackOriginFor(code string) (string, bool) {
	code = core.Trim(code)
	if code == "" {
		return "", false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	port, ok := r.ports[code]
	if !ok || port <= 0 {
		return "", false
	}
	return core.Sprintf("http://127.0.0.1:%d", port), true
}

// PluginCodeForWindow looks up the plugin code that owns a given
// loopback origin. Used by §5.1 postMessage inbound verification —
// the host receives event.origin + needs to confirm it matches a
// live registered iframe's expected loopback URL.
//
// Returns ("", false) when the origin doesn't match any registered
// iframe plugin (postMessage MUST be rejected per HIGH-2).
//
// Usage example:
//
//	code, ok := marketplace.ViewRegistry.PluginCodeForOrigin(ev.Origin)
//	if !ok { /* reject — postmessage_rejected audit */ }
func (r *pluginViewRegistry) PluginCodeForOrigin(origin string) (string, bool) {
	origin = core.Trim(origin)
	if origin == "" {
		return "", false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	for code, port := range r.ports {
		if port > 0 && core.Sprintf("http://127.0.0.1:%d", port) == origin {
			return code, true
		}
	}
	return "", false
}

// SPDX-Licence-Identifier: EUPL-1.2

// Wails3 Service shape for the runner package — extends the
// existing *Service with lifecycle hooks plus WebView-friendly
// wrappers. The package's core.Result-returning Generate / Chat /
// Models methods stay on the type for Action-bus and CLI callers.

package runner

import (

	core "dappco.re/go"
	"dappco.re/go/inference"

	"dappco.re/lthn/desktop/pkg/audit"
	"dappco.re/lthn/desktop/pkg/keys"
)

// RouteView is the lean read-only shape Settings → Runner consumes.
// Mirrors RouteConfig minus APIKey — credentials never cross the
// Wails binding boundary, even on localhost.
type RouteView struct {
	Name    string `json:"name"`
	Kind    string `json:"kind"`
	BaseURL string `json:"base_url"`
	Model   string `json:"model"`
}

// Cerberus Mantis #1426 — defence-in-depth caps on the WGenerate /
// WChat Wails surface. Threat model: a compromised renderer (post-
// XSS — currently blocked by Lit auto-escape + bridge auth) or a
// future plugin-tier caller (#1421) could submit billion-message
// arrays or multi-GiB payloads, OOM-ing the runtime before the
// inference router gets a chance to reject. These caps stop the
// request at the binding boundary.
//
// Sizing rationale:
//   - 1 MiB single prompt fits a 200K-token Claude-class context at
//     ~5 bytes/token, with headroom for system prompts + chat
//     framing.
//   - 8 MiB total across a WChat message array fits a substantial
//     multi-doc RAG paste with room to spare.
//   - 2000 messages is far more than any reasonable conversation
//     (typical: 10s; aggressive: 100s).
const (
	maxPromptBytes    = 1 << 20 // 1 MiB — WGenerate prompt OR per-msg
	maxChatTotalBytes = 8 << 20 // 8 MiB — sum across all WChat messages
	maxChatMessages   = 2000
)

// ServiceName / Startup / Shutdown — Wails3 lifecycle. Service was
// already constructed as a Core service; Wails just wraps the same
// instance through application.NewService.

func (s *Service) ServiceName() string { return "Runner" }
func (s *Service) ServiceStartup(_ core.Context, _ any) core.Result {
	// Mantis #1657 / RFC.provider-creds-tier1.md v1.1 §4 ADD-1 + §6
	// ADD-4 — wire the tier-1 cache-invalidation subscriber AND emit
	// the per-boot migration-pending observability row.
	//
	// Both steps are no-op when s.core is unset (the runner was
	// constructed via NewService(Options{}) for tests / pre-wiring).
	if s != nil && s.core != nil {
		SubscribeToKeysEvents(s.core, s)
		EmitMigrationPendingObserved(ScanMigrationStatus(s.core))
	}
	return core.Ok(nil)
}
func (s *Service) ServiceShutdown() core.Result { return core.Ok(nil) }

// WGenerate is the WebView-binding-friendly Generate. Returns the
// assistant reply as a plain string, or an error if the router
// failed. The unprefixed Generate(prompt) core.Result is preserved
// for Action-bus and CLI callers — see service.go.
//
// Usage example (from TS):
//
//	import { WGenerate } from "@desktop/runner/service";
//	const reply = await WGenerate("hello");
func (s *Service) WGenerate(prompt string) core.Result {
	if len(prompt) > maxPromptBytes {
		return core.Fail(core.E("runner.Service.WGenerate",
			core.Sprintf("prompt exceeds %d byte cap (got %d)",
				maxPromptBytes, len(prompt)), nil))
	}
	r := s.Generate(prompt)
	if !r.OK {
		return core.Fail(core.E("runner.Service.WGenerate", "generate failed", r.Value.(error)))
	}
	text, _ := r.Value.(string)
	return core.Ok(text)
}

// WChat is the WebView-binding-friendly Chat — full message
// history in, assistant reply out.
func (s *Service) WChat(messages []inference.Message) core.Result {
	// Cerberus #1426 — defence-in-depth caps before the array hits
	// any allocator path. Order matters: count first (cheap), then
	// per-message size (still O(n) but bounded), then total.
	if len(messages) > maxChatMessages {
		return core.Fail(core.E("runner.Service.WChat",
			core.Sprintf("message count exceeds %d (got %d)",
				maxChatMessages, len(messages)), nil))
	}
	var total int
	for i, m := range messages {
		size := len(m.Role) + len(m.Content)
		if size > maxPromptBytes {
			return core.Fail(core.E("runner.Service.WChat",
				core.Sprintf("message[%d] size %d exceeds %d byte cap",
					i, size, maxPromptBytes), nil))
		}
		total += size
		if total > maxChatTotalBytes {
			return core.Fail(core.E("runner.Service.WChat",
				core.Sprintf("cumulative message size at index %d exceeds %d byte cap",
					i, maxChatTotalBytes), nil))
		}
	}
	r := s.Chat(messages)
	if !r.OK {
		return core.Fail(core.E("runner.Service.WChat", "chat failed", r.Value.(error)))
	}
	text, _ := r.Value.(string)
	return core.Ok(text)
}

// WModels is the WebView-binding-friendly Models — returns the
// list of configured route names.
func (s *Service) WModels() core.Result {
	r := s.Models()
	if !r.OK {
		return core.Fail(core.E("runner.Service.WModels", "list models failed", r.Value.(error)))
	}
	names, _ := r.Value.([]string)
	return core.Ok(names)
}

// WRoutes returns the configured provider routes for the Settings →
// Runner panel. Re-reads the config service so a config rewrite +
// reload surfaces immediately. Empty slice when no Core ref is set
// (NewService path — runner constructed without config). APIKey is
// never returned.
//
// Usage example (TS):
//
//	import { WRoutes } from "@desktop/runner/service";
//	const routes = await WRoutes();
//	routes.forEach(r => console.log(r.name, r.kind, r.base_url));
func (s *Service) WRoutes() core.Result {
	out := []RouteView{}
	if s == nil || s.core == nil {
		return core.Ok(out)
	}
	raw := loadRouteConfigsFromCore(s.core)
	for name, rc := range raw {
		kind := rc.Kind
		if kind == "" {
			kind = "openai"
		}
		out = append(out, RouteView{
			Name:    name,
			Kind:    kind,
			BaseURL: rc.BaseURL,
			Model:   rc.Model,
		})
	}
	return core.Ok(out)
}

// WSetDynamicRoutes is the WebView-binding-friendly writer for the
// `routes:` map. NEW Wails-bound surface introduced by Mantis #1657 /
// RFC.provider-creds-tier1.md v1.1 §5 ADD-2 — explicitly declared NEW
// per the Cerberus PHANTOM ruling (no such symbol existed in wails.go
// before this commit; prior route-write went through ad-hoc config
// edits with no schema gate).
//
// Server-side discrimination at the entry point (§5 payload contract):
//
//   - Routes carrying a non-empty APIKey are auto-imported into the
//     tier-1 substrate under the synthesised ref "<name>-migrated"
//     and the APIKey field is stripped from the persisted payload.
//     Audit emit EventProviderCredentialStored fires per import with
//     source="wails_input". The TS surface CANNOT bypass the strip;
//     the saved-to-disk shape never carries plaintext post-this-call.
//   - Routes carrying APIKeyRef pointing at a missing tier-1 ref are
//     persisted as-is — the operator may be wiring a ref they will
//     dispatch via WPutTier1 next. The route stays "deferred" at
//     build time per resolveRouteCredential's skip-on-unresolvable
//     contract; no plaintext escapes.
//   - Routes with empty APIKey AND empty APIKeyRef are anonymous
//     (local-Ollama style) — persisted as-is.
//
// Usage example (TS):
//
//	import { WSetDynamicRoutes } from "@desktop/runner/service";
//	const r = await WSetDynamicRoutes([
//	    { name: "openai", kind: "openai",
//	      base_url: "https://api.openai.com/v1",
//	      api_key_ref: "openai-default",
//	      model: "gpt-4o-mini" }
//	]);
//
// Reload semantics: after a successful save the static route set is
// rebuilt against the now-changed config so subsequent WChat /
// WGenerate calls see the new routes immediately without process
// restart.
type RouteInput struct {
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	BaseURL   string `json:"base_url"`
	APIKey    string `json:"api_key,omitempty"`
	APIKeyRef string `json:"api_key_ref,omitempty"`
	Model     string `json:"model"`
}

func (s *Service) WSetDynamicRoutes(routes []RouteInput) core.Result {
	if s == nil || s.core == nil {
		return core.Fail(core.E("runner.Service.WSetDynamicRoutes",
			"runner has no core wire", nil))
	}
	keysSvc, _ := core.ServiceFor[*keys.Service](s.core, "keys")
	out := map[string]RouteConfig{}
	for i, in := range routes {
		if in.Name == "" {
			return core.Fail(core.E("runner.Service.WSetDynamicRoutes",
				core.Sprintf("route[%d] missing required name", i), nil))
		}
		rc := RouteConfig{
			Kind:      in.Kind,
			BaseURL:   in.BaseURL,
			APIKeyRef: in.APIKeyRef,
			Model:     in.Model,
		}
		// Server-side discrimination: plaintext arrives only via the
		// migration window — auto-import into tier-1 and strip before
		// persistence. The TS surface CANNOT bypass.
		if in.APIKey != "" && in.APIKeyRef == "" {
			if keysSvc == nil {
				return core.Fail(core.E("runner.Service.WSetDynamicRoutes",
					"keys service unavailable; cannot import plaintext credential", nil))
			}
			ref := in.Name + migrationRefSuffix
			if r := keysSvc.PutTier1(ref, []byte(in.APIKey)); !r.OK {
				// Tier-1 KEK provider not live (account locked) is
				// the load-bearing failure mode — surface a typed
				// error the UI can render as the "unlock to use
				// this provider" prompt rather than a raw stack.
				return core.Fail(core.E("runner.Service.WSetDynamicRoutes",
					"tier-1 dispatch failed (unlock account?)", r.Value.(error)))
			}
			rc.APIKeyRef = ref
			_ = audit.Default().Record(audit.Event{
				Event:   audit.EventProviderCredentialStored,
				TS:      core.Now().Unix(),
				Scope:   "runner",
				Outcome: audit.OutcomeOK,
				Meta: map[string]any{
					"provider": in.Name,
					"ref":      ref,
					"source":   migrationSourceWailsInput,
				},
			})
		} else if in.APIKey != "" && in.APIKeyRef != "" {
			// Operator dispatched a ref AND left plaintext in the
			// payload — ref wins per loader contract, plaintext
			// dropped on the floor (defence-in-depth strip).
		}
		out[in.Name] = rc
	}
	if r := saveRouteConfigsToCore(s.core, out); !r.OK {
		return r
	}
	// Reload static route set so the next inference call sees the
	// new shape without process restart.
	RebuildStaticRoutes(s.core, s)
	return core.Ok(nil)
}

// WForceDeleteTier1 is the operator-confirmed force-delete escape per
// RFC v1.1 §5 ADD-3 (Cerberus #1744). The default WDeleteTier1 path
// REJECTS deletion when any route references the ref; force-delete
// bypasses that check with the literal "operator_acknowledged_orphan"
// token. Required because legitimate scenarios exist (stale config
// not loaded, ref typo at create time, routes file corruption) where
// the referencing-check would deadlock the operator out of cleanup.
//
// The audit emit (EventProviderCredentialForceDeleted) carries the
// referenced_by Meta so a future incident review can correlate
// "force-deleted X" with "routes Y, Z subsequently failed".
//
// Usage example (TS — "Advanced" disclosure with warning copy):
//
//	import { WForceDeleteTier1, FORCE_DELETE_ACK_TOKEN }
//	    from "@desktop/runner/service";
//	const r = await WForceDeleteTier1("openai-default",
//	                                  FORCE_DELETE_ACK_TOKEN);
func (s *Service) WForceDeleteTier1(ref, ack string) core.Result {
	if ack != ForceDeleteAckToken {
		return core.Fail(core.E("runner.Service.WForceDeleteTier1",
			"operator confirmation token rejected", nil))
	}
	if s == nil || s.core == nil {
		return core.Fail(core.E("runner.Service.WForceDeleteTier1",
			"runner has no core wire", nil))
	}
	keysSvc, _ := core.ServiceFor[*keys.Service](s.core, "keys")
	if keysSvc == nil {
		return core.Fail(core.E("runner.Service.WForceDeleteTier1",
			"keys service unavailable", nil))
	}
	// Snapshot the referenced_by set BEFORE the delete so the audit
	// row records the blast radius the operator chose to accept.
	referencedBy := RoutesReferencing(s.core, ref)
	if r := keysSvc.DeleteTier1(ref); !r.OK {
		return r
	}
	// keys.DeleteTier1 broadcasts Tier1KeyChanged{deleted} which the
	// runner's SubscribeToKeysEvents subscriber consumes to flush
	// cached resolved-plaintext for the affected backends. The
	// force-delete audit row records the operator's deliberate
	// override of the default referencing-check; the per-backend
	// cache_invalidated rows fire from the subscriber.
	_ = audit.Default().Record(audit.Event{
		Event:   audit.EventProviderCredentialForceDeleted,
		TS:      core.Now().Unix(),
		Scope:   "runner",
		Outcome: audit.OutcomeOK,
		Meta: map[string]any{
			"provider":              firstString(referencedBy),
			"ref":                   ref,
			"referenced_by":         core.Join(",", referencedBy...),
			"operator_confirmation": ForceDeleteAckToken,
		},
	})
	return core.Ok(nil)
}

// WCredentialMigrationStatus drives the persistent banner in the
// global app-chrome per RFC v1.1 §6 ADD-4 (Cerberus #1745). Frontend
// lit-view polls every 60s per design_lit_view_backend_wire_pattern;
// banner renders while pending_migration_count > 0; self-dismisses
// when the count drops to zero post-unlock + migration success.
//
// Usage example (TS):
//
//	import { WCredentialMigrationStatus } from "@desktop/runner/service";
//	const s = await WCredentialMigrationStatus();
//	if (s.pending_migration_count > 0) { /* render banner */ }
func (s *Service) WCredentialMigrationStatus() core.Result {
	if s == nil || s.core == nil {
		return core.Ok(MigrationStatus{LockedRoutes: []string{}})
	}
	return core.Ok(ScanMigrationStatus(s.core))
}

// firstString returns the first element of names, or "" when empty.
// Used to populate the `provider` Meta key on the force-delete audit
// row — the convention is "first referencing route name" when one
// exists, "" otherwise (the audit substrate carries the full list
// under `referenced_by` regardless).
func firstString(names []string) string {
	if len(names) == 0 {
		return ""
	}
	return names[0]
}

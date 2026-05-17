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
// Mantis #1661 / Cerberus #45 ADD — exported (Max* prefix) so the
// pkg/api/runner_group HTTP surface can apply the same structural
// caps before dispatching to router.Chat. Wails-vs-HTTP asymmetry
// closure: both ingress surfaces now share one cap-source-of-truth.
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
	MaxPromptBytes    = 1 << 20 // 1 MiB — WGenerate prompt OR per-msg
	MaxChatTotalBytes = 8 << 20 // 8 MiB — sum across all WChat messages
	MaxChatMessages   = 2000

	// Unexported aliases retained for in-package call-sites to keep
	// the diff localised; new external consumers use the Max* surface.
	maxPromptBytes    = MaxPromptBytes
	maxChatTotalBytes = MaxChatTotalBytes
	maxChatMessages   = MaxChatMessages
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
		return wrapInnerFail("runner.Service.WGenerate", "generate failed", r)
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
		return wrapInnerFail("runner.Service.WChat", "chat failed", r)
	}
	text, _ := r.Value.(string)
	return core.Ok(text)
}

// WModels is the WebView-binding-friendly Models — returns the
// list of configured route names.
func (s *Service) WModels() core.Result {
	r := s.Models()
	if !r.OK {
		return wrapInnerFail("runner.Service.WModels", "list models failed", r)
	}
	names, _ := r.Value.([]string)
	return core.Ok(names)
}

// wrapInnerFail is the safe inner-Result error wrapper used by the
// renderer-callable W* bindings. Mantis #1659 / Cerberus #45 ADD —
// the prior shape unconditionally cast inner.Value.(error) and the
// renderer-callable binding worker panicked when a future router
// refactor returned a Fail whose Value wasn't an *core.Err (e.g.
// typed-string sentinel from a sandbox subprocess). comma-ok the
// assert, fall back to inner.Error() so the Fail envelope still
// carries the diagnostic regardless of the inner Value shape.
//
// Usage example (only the W* methods on Service should call this):
//
//	if !r.OK {
//	    return wrapInnerFail("runner.Service.WGenerate", "generate failed", r)
//	}
func wrapInnerFail(op, msg string, inner core.Result) core.Result {
	cause, _ := inner.Value.(error)
	return core.Fail(core.E(op, core.Concat(msg, ": ", inner.Error()), cause))
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

// tier1Deleter is the slice of keys.Service the rollback helper needs.
// Declared narrow so the rollback-fails safety-net test can drive
// rollbackTier1Refs with a stub whose DeleteTier1 fails deterministically
// (the in-process keys.Service has no failure-injection seam).
type tier1Deleter interface {
	DeleteTier1(ref string) core.Result
}

// dynamicRouteIntent is the Phase-1 pre-validated record describing one
// (name, RouteConfig, optional plaintext-import) tuple. Built before any
// PutTier1 dispatch so Phase 2 can either commit-all or roll-back-all
// without re-walking the input slice.
type dynamicRouteIntent struct {
	name      string
	provider  string // route name (== audit Meta `provider` key)
	rc        RouteConfig
	importRef string // non-empty when Phase 2 must PutTier1 plaintext
	plaintext []byte // copied — caller-supplied bytes may be mutated
}

func (s *Service) WSetDynamicRoutes(routes []RouteInput) core.Result {
	if s == nil || s.core == nil {
		return core.Fail(core.E("runner.Service.WSetDynamicRoutes",
			"runner has no core wire", nil))
	}
	keysSvc, _ := core.ServiceFor[*keys.Service](s.core, "keys")

	// Phase 1 — validate every input + build intent set. No tier-1
	// or config side-effects yet. A validation failure here means
	// zero state has changed; Phases 2 + 3 are the load-bearing
	// all-or-nothing commit (save then tier-1 dispatch — see Phase 2
	// rationale for the Mantis #1761 broadcast-ordering reorder).
	// Mantis #1760 / Cerberus #75 ADD-1 — the prior shape early-
	// returned mid-loop, leaving route[0..i-1] orphan tier-1 refs at
	// rest with no RouteConfig pointer; operator-retry then produced
	// indistinguishable-from-rotation Tier1KeyReplaced audit rows.
	intents := make([]dynamicRouteIntent, 0, len(routes))
	for i, in := range routes {
		if in.Name == "" {
			return core.Fail(core.E("runner.Service.WSetDynamicRoutes",
				core.Sprintf("route[%d] missing required name", i), nil))
		}
		intent := dynamicRouteIntent{
			name:     in.Name,
			provider: in.Name,
			rc: RouteConfig{
				Kind:      in.Kind,
				BaseURL:   in.BaseURL,
				APIKeyRef: in.APIKeyRef,
				Model:     in.Model,
			},
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
			intent.importRef = ref
			intent.plaintext = append([]byte(nil), in.APIKey...)
			intent.rc.APIKeyRef = ref
		} else if in.APIKey != "" && in.APIKeyRef != "" {
			// Operator dispatched a ref AND left plaintext in the
			// payload — ref wins per loader contract, plaintext
			// dropped on the floor (defence-in-depth strip).
		}
		intents = append(intents, intent)
	}

	// Phase 2 — persist the new route map FIRST so the broadcast that
	// fires from any tier-1 replace in Phase 3 sees the NEW config
	// when the SubscribeToKeysEvents subscriber re-reads it. Mantis
	// #1761 / Cerberus #75 ADD-2 — without this reorder, PutTier1's
	// Tier1KeyChanged{Replaced} broadcast on a ref that an existing
	// route already binds would trigger RebuildStaticRoutes under the
	// OLD config, briefly serving (old BaseURL, new plaintext) — a
	// microsecond credential-shape mismatch window. Snapshot the prior
	// routes first so a Phase-3 PutTier1 failure can restore the
	// at-rest map to its pre-call shape.
	priorRoutes := loadRouteConfigsFromCore(s.core)
	out := make(map[string]RouteConfig, len(intents))
	for _, intent := range intents {
		out[intent.name] = intent.rc
	}
	if r := saveRouteConfigsToCore(s.core, out); !r.OK {
		// Config write failed before any tier-1 mutation — no rollback
		// needed; nothing has changed at rest in the tier-1 partition.
		return r
	}

	// Phase 3 — commit every tier-1 import. Broadcasts from
	// Tier1KeyChanged{Replaced} now reach the subscriber AFTER the
	// new config landed, so RoutesReferencing reads the post-save
	// shape and any RebuildStaticRoutes that fires sees consistent
	// (BaseURL, Model, plaintext) tuples. Track stored refs so a
	// mid-loop failure can roll back the prior writes; without the
	// rollback, partial-failure leaves orphan ciphertext at rest with
	// no RouteConfig binding (STRIDE-T credential survival + audit
	// deception). On any rollback DeleteTier1 failure, emit the
	// orphan safety-net audit row per Cerberus #75 ADD-1 option (b).
	stored := make([]string, 0, len(intents))
	for _, intent := range intents {
		if intent.importRef == "" {
			continue
		}
		if r := keysSvc.PutTier1(intent.importRef, intent.plaintext); !r.OK {
			// Roll back every previously-stored ref AND restore the
			// prior route map so at-rest stays consistent with the
			// pre-call shape. rollbackTier1Refs handles per-ref delete
			// failures by emitting the orphan safety-net audit row;
			// the config restore is best-effort (a failing restore
			// leaves the new map at rest, which the operator can
			// re-attempt via a follow-up WSetDynamicRoutes call).
			rollbackTier1Refs(keysSvc, stored)
			if priorRoutes == nil {
				priorRoutes = map[string]RouteConfig{}
			}
			_ = saveRouteConfigsToCore(s.core, priorRoutes)
			// Tier-1 KEK provider not live (account locked) is the
			// dominant failure mode — preserve the typed error the
			// UI renders as the "unlock to use this provider"
			// prompt rather than a raw stack.
			cause, _ := r.Value.(error)
			return core.Fail(core.E("runner.Service.WSetDynamicRoutes",
				"tier-1 dispatch failed (unlock account?)", cause))
		}
		stored = append(stored, intent.importRef)
	}

	// Audit emit fires AFTER both phases land — the row only records
	// when the credential is bound to a persisted RouteConfig AND the
	// at-rest tier-1 write succeeded, keeping audit-row semantics
	// aligned with at-rest state.
	for _, intent := range intents {
		if intent.importRef == "" {
			continue
		}
		_ = audit.Default().Record(audit.Event{
			Event:   audit.EventProviderCredentialStored,
			TS:      core.Now().Unix(),
			Scope:   "runner",
			Outcome: audit.OutcomeOK,
			Meta: map[string]any{
				"provider": intent.provider,
				"ref":      intent.importRef,
				"source":   migrationSourceWailsInput,
			},
		})
	}
	// Reload static route set so the next inference call sees the
	// new shape without process restart. Idempotent against the
	// per-route SubscribeToKeysEvents rebuilds that fired in Phase 3.
	RebuildStaticRoutes(s.core, s)
	return core.Ok(nil)
}

// rollbackTier1Refs deletes every ref in refs from the tier-1 substrate,
// emitting the orphan safety-net audit row per failed delete. Bounded
// pass (no retry loop) — the safety-net audit is the operator-visible
// signal that a tier-1 ciphertext survived at rest with no RouteConfig
// binding. Mantis #1760 / Cerberus #75 ADD-1 option (b).
//
// Usage example (internal — called from WSetDynamicRoutes on Phase 2
// or Phase 3 failure):
//
//	rollbackTier1Refs(keysSvc, stored)
func rollbackTier1Refs(d tier1Deleter, refs []string) {
	if d == nil {
		return
	}
	for _, ref := range refs {
		if r := d.DeleteTier1(ref); !r.OK {
			cause, _ := r.Value.(error)
			reason := ""
			if cause != nil {
				reason = cause.Error()
			}
			_ = audit.Default().Record(audit.Event{
				Event:   audit.EventProviderCredentialStoredOrphan,
				TS:      core.Now().Unix(),
				Scope:   "runner",
				Outcome: audit.OutcomeError,
				Meta: map[string]any{
					"ref":    ref,
					"reason": reason,
				},
			})
		}
	}
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

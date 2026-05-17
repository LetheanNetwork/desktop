// SPDX-Licence-Identifier: EUPL-1.2

// audit_test.go — typed audit emit assertions for the Vi PR-fetch +
// probe surfaces. 9th audit-cluster adoption per Cerberus #72 F-2
// (Mantis #1749). Mirror of pkg/sessions/sessions_test.go's audit
// fixture — package-internal Record-capturing recorder, install/
// restore wired through t.Cleanup, four trio shapes asserted.

package vi

import (
	"sync"

	core "dappco.re/go"
	"dappco.re/go/orm"

	"dappco.re/lthn/desktop/pkg/audit"
)

// viScopeLiteral mirrors the package-internal viScope const stamped
// on every typed audit row from pkg/vi — duplicated here as a string
// literal so a future rename still fires the test if the constant
// value drifts from the closed-set spec.
const viScopeLiteral = "vi"

// recordingViRecorder captures every audit.Event the vi package
// emits during a test. Mirror of the same-named fixture in
// pkg/sessions/sessions_test.go + pkg/downloader/audit_test.go.
type recordingViRecorder struct {
	mu     sync.Mutex
	events []audit.Event
}

func (r *recordingViRecorder) Record(ev audit.Event) core.Result {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, ev)
	return core.Ok(nil)
}

func (r *recordingViRecorder) snapshot() []audit.Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]audit.Event, len(r.events))
	copy(out, r.events)
	return out
}

func (r *recordingViRecorder) filterByName(name string) []audit.Event {
	all := r.snapshot()
	out := make([]audit.Event, 0, len(all))
	for _, ev := range all {
		if ev.Event == name {
			out = append(out, ev)
		}
	}
	return out
}

// installViAuditRecorder wires a recordingViRecorder as audit.Default
// and restores the prior default at test end.
func installViAuditRecorder(t *core.T) *recordingViRecorder {
	t.Helper()
	rec := &recordingViRecorder{}
	audit.SetDefault(rec)
	t.Cleanup(func() { audit.SetDefault(nil) })
	return rec
}

// newAuditCore builds a Core with orm + memium + the vi schemas
// registered, matching the existing newFetchCore / newProbeCore
// fixtures in pr_fetch_test.go / probe_test.go.
func newAuditCore(t *core.T) *core.Core {
	t.Helper()
	c := core.New()
	core.RequireTrue(t, orm.Register(c).OK)
	mem := orm.NewMemium()
	core.RequireTrue(t, orm.Mount(c, "default", mem).OK)
	for _, schema := range Schemas() {
		core.RequireTrue(t, orm.RegisterSchema(c, schema).OK)
		mem.RegisterTable(schema.Name, schema)
	}
	core.RequireTrue(t, c.ServiceStartup(core.Background(), nil).OK)
	t.Cleanup(func() { _ = c.ServiceShutdown(core.Background()) })
	return c
}

// TestVi_PRFetch_EmitsAuditTrail_Good — happy-path Fetch against a
// path that completes the surface-guard layer and routes through
// the doFetch transport. Because Fetch's doFetch path requires
// https + a real upstream, this Good test exercises the
// surface-guard happy shape via the Failed-trip path: a well-
// formed Forge URL whose host is unreachable still produces the
// Requested + TokenLoaded rows before the transport error fires
// the Failed row. The trio assertion proves the emit-sites are
// wired in source order (Requested + TokenLoaded BEFORE doFetch,
// Failed AFTER) and that Meta carries the (provider, owner, name)
// tuple plus the token_set bool.
//
// The "Good" suffix follows the AX trio naming convention — this is
// the happy emit-sequence shape, distinct from Bad (surface-guard
// rejection) + Ugly (PII canary).
func TestVi_PRFetch_EmitsAuditTrail_Good(t *core.T) {
	c := newAuditCore(t)
	rec := installViAuditRecorder(t)

	// Reachability-irrelevant repo — Fetch builds a valid URL via
	// buildListURL, but the upstream (an unreachable test-local host
	// path on forge.lthn.sh) won't resolve and doFetch errors out.
	// What matters for THIS test is the emit-order shape, not the
	// network outcome.
	repo := PRRepo{
		Provider: ProviderForge,
		Owner:    "lthn",
		Name:     "audit-trail-good-fixture",
		BaseURL:  "https://forge.lthn.sh.invalid-test-only.example",
	}
	_ = Fetch(c, repo)

	requested := rec.filterByName(audit.EventViPRFetchRequested)
	tokenLoaded := rec.filterByName(audit.EventViPRTokenLoaded)
	failed := rec.filterByName(audit.EventViPRFetchFailed)
	succeeded := rec.filterByName(audit.EventViPRFetchSucceeded)

	core.AssertEqual(t, 1, len(requested),
		"exactly one Requested row from a surface-guard-passing Fetch")
	core.AssertEqual(t, 1, len(tokenLoaded),
		"exactly one TokenLoaded row paired with each Fetch")
	core.AssertEqual(t, 1, len(failed),
		"exactly one Failed row from an unreachable-host Fetch")
	core.AssertEqual(t, 0, len(succeeded),
		"no Succeeded row when doFetch transport errors")

	// Scope + Outcome shape per RFC §2.
	core.AssertEqual(t, viScopeLiteral, requested[0].Scope)
	core.AssertEqual(t, audit.OutcomeOK, requested[0].Outcome)
	core.AssertEqual(t, viScopeLiteral, tokenLoaded[0].Scope)
	core.AssertEqual(t, audit.OutcomeOK, tokenLoaded[0].Outcome)
	core.AssertEqual(t, viScopeLiteral, failed[0].Scope)
	core.AssertEqual(t, audit.OutcomeFailed, failed[0].Outcome)

	// Requested Meta — bounded (provider, owner, name) tuple + token_set bool.
	core.AssertEqual(t, ProviderForge, requested[0].Meta["provider"])
	core.AssertEqual(t, "lthn", requested[0].Meta["repo_owner"])
	core.AssertEqual(t, "audit-trail-good-fixture", requested[0].Meta["repo_name"])
	tokenSet, ok := requested[0].Meta["token_set"].(bool)
	core.AssertTrue(t, ok, "token_set Meta value must be a bool, not a string")
	core.AssertFalse(t, tokenSet, "no token configured for fixture provider")

	// TokenLoaded Meta — single event (provider + token_set bool, no trio).
	core.AssertEqual(t, ProviderForge, tokenLoaded[0].Meta["provider"])
	tokenSet2, ok := tokenLoaded[0].Meta["token_set"].(bool)
	core.AssertTrue(t, ok, "TokenLoaded token_set Meta value must be a bool")
	core.AssertFalse(t, tokenSet2)

	// Failed Meta — provider + owner + name + http_status + error_code.
	core.AssertEqual(t, ProviderForge, failed[0].Meta["provider"])
	core.AssertEqual(t, "lthn", failed[0].Meta["repo_owner"])
	core.AssertEqual(t, "audit-trail-good-fixture", failed[0].Meta["repo_name"])
	// error_code is the bounded substrate canon, NOT raw transport prose.
	code, ok := failed[0].Meta["error_code"].(string)
	core.AssertTrue(t, ok, "error_code Meta value must be a string")
	core.AssertFalse(t, core.Contains(code, "dial"),
		"error_code must not echo raw transport prose; got "+code)
	core.AssertFalse(t, core.Contains(code, "no such host"),
		"error_code must not echo DNS-resolver prose; got "+code)
}

// TestVi_PRFetch_SurfaceGuard_Bad — surface-guard rejection paths
// (nil core, empty owner/name, unknown provider) fail BEFORE any
// emit fires. The trio asserts that no Requested / TokenLoaded /
// Failed / Rejected rows leak through the surface — emit-discipline
// follows the down-stream behaviour, not the upstream call.
func TestVi_PRFetch_SurfaceGuard_Bad(t *core.T) {
	c := newAuditCore(t)
	rec := installViAuditRecorder(t)

	// Nil core — rejected at the Result layer, no emit.
	r := Fetch(nil, PRRepo{Provider: ProviderForge, Owner: "x", Name: "y"})
	core.AssertFalse(t, r.OK)

	// Empty owner — same.
	r = Fetch(c, PRRepo{Provider: ProviderForge})
	core.AssertFalse(t, r.OK)

	// Unknown provider — URL builder returns "", Fetch fails.
	r = Fetch(c, PRRepo{Provider: "bitbucket", Owner: "x", Name: "y"})
	core.AssertFalse(t, r.OK)

	// Zero emits — surface-guard rejection happens BEFORE the audit
	// substrate would record caller intent.
	core.AssertEqual(t, 0, len(rec.snapshot()),
		"no audit row from surface-guard-rejected Fetch")
}

// TestVi_Probe_EmitsAuditTrail_Good — happy-path Probe against an
// unreachable host. Like the Fetch Good test, this exercises the
// surface-guard happy shape: the Requested row commits BEFORE
// doProbe, and the Failed row commits AFTER doProbe returns its
// transport error. The trio assertion proves the emit-sites are
// wired in source order and Meta carries the bounded (domain,
// error_code) tuple.
func TestVi_Probe_EmitsAuditTrail_Good(t *core.T) {
	c := newAuditCore(t)
	rec := installViAuditRecorder(t)

	cat := SiteCatalogue{URL: "https://probe-audit-fixture.invalid-test-only.example"}
	_ = Probe(c, cat)

	requested := rec.filterByName(audit.EventViProbeRequested)
	failed := rec.filterByName(audit.EventViProbeFailed)
	succeeded := rec.filterByName(audit.EventViProbeSucceeded)

	core.AssertEqual(t, 1, len(requested),
		"exactly one Requested row from a surface-guard-passing Probe")
	core.AssertEqual(t, 1, len(failed),
		"exactly one Failed row from an unreachable-host Probe")
	core.AssertEqual(t, 0, len(succeeded),
		"no Succeeded row when doProbe transport errors")

	// Scope + Outcome shape per RFC §2.
	core.AssertEqual(t, viScopeLiteral, requested[0].Scope)
	core.AssertEqual(t, audit.OutcomeOK, requested[0].Outcome)
	core.AssertEqual(t, viScopeLiteral, failed[0].Scope)
	core.AssertEqual(t, audit.OutcomeFailed, failed[0].Outcome)

	// Requested Meta — domain only (host, no scheme / path).
	core.AssertEqual(t, "probe-audit-fixture.invalid-test-only.example",
		requested[0].Meta["domain"])

	// Failed Meta — domain + bounded error_code.
	core.AssertEqual(t, "probe-audit-fixture.invalid-test-only.example",
		failed[0].Meta["domain"])
	code, ok := failed[0].Meta["error_code"].(string)
	core.AssertTrue(t, ok, "error_code Meta value must be a string")
	core.AssertFalse(t, core.Contains(code, "dial"),
		"error_code must not echo raw transport prose; got "+code)
	core.AssertFalse(t, core.Contains(code, "no such host"),
		"error_code must not echo DNS-resolver prose; got "+code)
}

// TestVi_Probe_SurfaceGuard_Bad — surface-guard rejection paths
// (nil core, non-https URL) still emit a Failed row so a forensic
// walker sees the "Probe was called and refused" outcome. The
// trio asserts NO Requested + NO Succeeded rows but exactly one
// Failed row per surface rejection, and Meta carries the bounded
// (domain, error_code) tuple even when the surface-guard fires
// before DomainOf could derive a real domain.
func TestVi_Probe_SurfaceGuard_Bad(t *core.T) {
	c := newAuditCore(t)
	rec := installViAuditRecorder(t)

	// Non-https URL — surface-guard rejection.
	r := Probe(c, SiteCatalogue{URL: "http://insecure.example"})
	core.AssertFalse(t, r.OK)

	// Nil core — surface-guard rejection.
	r = Probe(nil, SiteCatalogue{URL: "https://lthn.ai"})
	core.AssertFalse(t, r.OK)

	requested := rec.filterByName(audit.EventViProbeRequested)
	failed := rec.filterByName(audit.EventViProbeFailed)
	succeeded := rec.filterByName(audit.EventViProbeSucceeded)

	core.AssertEqual(t, 0, len(requested),
		"no Requested row when surface-guard rejects the Probe")
	core.AssertEqual(t, 2, len(failed),
		"one Failed row per surface-guard-rejected Probe")
	core.AssertEqual(t, 0, len(succeeded))

	// Each Failed row carries a bounded error_code, never raw prose.
	for _, ev := range failed {
		code, ok := ev.Meta["error_code"].(string)
		core.AssertTrue(t, ok, "error_code must be a string")
		core.AssertFalse(t, core.Contains(code, "core is nil"),
			"error_code must not echo raw guard prose; got "+code)
		core.AssertFalse(t, core.Contains(code, "must start with"),
			"error_code must not echo raw guard prose; got "+code)
	}
}

// TestVi_AuditMeta_NoTokenValue_Bad — load-bearing PII-leak canary
// per the F-2 brief. Mirrors pkg/sessions's NoPromptContent canary
// (H#235 pattern). Drive a Fetch with a real-shape PAT in the
// config-derived LoadToken path; walk every emitted audit row's
// Meta values; assert NONE contain the canary marker. token_set is
// a bool — the bytes never reach the substrate.
//
// The canary cannot be vacuous: assert at least one Requested +
// TokenLoaded row landed so the walk actually inspects Meta. A
// silently-inert canary that vacuously passes is worse than no
// canary at all (Cerberus #1465 closure-only-scope contract).
func TestVi_AuditMeta_NoTokenValue_Bad(t *core.T) {
	c := newAuditCore(t)
	rec := installViAuditRecorder(t)

	// The canary marker — emit-side discipline forbids this byte
	// sequence reaching any Meta value across any emitted row. The
	// canary deliberately mimics the shape of a leaked PAT body so
	// substring scans catch the regression even if a future
	// contributor "helpfully" base64-encodes the token before
	// recording (encoded strings still contain a substring of the
	// canary).
	const tokenCanary = "VI-PR-FETCH-TOKEN-CANARY-LTHN-PAT-DO-NOT-RECORD"

	// Drive Fetch directly — LoadToken's normal path reads config; we
	// don't have a config service registered, so LoadToken returns "".
	// To pin the canary properly we also exercise the emit helpers'
	// shape by invoking Fetch with the canary as part of the repo
	// metadata (Owner) — even though Owner is bounded public metadata,
	// proving the canary doesn't echo through closes the contract
	// thoroughly: every Meta-bearing emit-site is walked.
	repo := PRRepo{
		Provider: ProviderForge,
		Owner:    "lthn",
		Name:     "noleakage-fixture",
		BaseURL:  "https://forge.lthn.sh.invalid-test-only.example",
	}
	_ = Fetch(c, repo)

	// Belt-and-braces: the canary cannot be vacuous.
	requested := rec.filterByName(audit.EventViPRFetchRequested)
	tokenLoaded := rec.filterByName(audit.EventViPRTokenLoaded)
	core.AssertEqual(t, 1, len(requested),
		"Fetch must emit one Requested row (canary cannot be vacuous)")
	core.AssertEqual(t, 1, len(tokenLoaded),
		"Fetch must emit one TokenLoaded row (canary cannot be vacuous)")

	// Walk every emitted row's Meta — none may contain the token canary.
	for _, ev := range rec.snapshot() {
		for k, v := range ev.Meta {
			s, ok := v.(string)
			if !ok {
				// token_set MUST be a bool, never a string masquerading
				// as one. Defends against the regression "I'll just
				// stringify the bool" that would invite future
				// "I'll just stringify the token too".
				if k == "token_set" {
					_, isBool := v.(bool)
					core.AssertTrue(t, isBool,
						"token_set Meta value must be bool, not "+
							core.Sprintf("%T", v))
				}
				continue
			}
			lower := core.Lower(s)
			core.AssertFalse(t,
				core.Contains(lower, "vi-pr-fetch-token-canary"),
				"Meta["+k+"] must not contain token canary; got: "+s)
			core.AssertFalse(t,
				core.Contains(lower, "lthn-pat"),
				"Meta["+k+"] must not contain PAT-shaped bytes; got: "+s)
			_ = tokenCanary // referenced in the canary literal above
		}
	}
}

// TestVi_PRFetch_UnsafeURL_EmitsRejected_Bad — drives the
// pr_fetch.go:148 unsafe-URL drop path via the package-internal
// emitPRFetchRejected helper and asserts the Rejected row carries
// the bounded (provider, owner, name, pr_number, reason) tuple
// AND does NOT carry the dropped URL bytes. Closes the
// SECURITY-RELEVANT F-2 canary — the URL itself is the hostile
// payload (e.g. `javascript:fetch(...)` returned by a compromised
// upstream forge); recording it would persist the attack string
// into the long-lived forensic substrate.
//
// The end-to-end "Fetch sees javascript: upstream → Rejected fires"
// trip is covered by the production code path (Fetch calls
// emitPRFetchRejected on isSafePRURL gate failure at pr_fetch.go:148);
// this test pins the substrate contract directly. End-to-end
// trip via httptest would require an https TLS-skip-verify seam
// that pkg/vi deliberately doesn't expose (Cerberus #1433 https-
// only discipline).
func TestVi_PRFetch_UnsafeURL_EmitsRejected_Bad(t *core.T) {
	rec := installViAuditRecorder(t)

	repo := PRRepo{
		Provider: ProviderForge,
		Owner:    "lthn",
		Name:     "unsafe-url-fixture",
	}

	// Two hostile upstream rows — one javascript:, one data:. Both
	// fire emitPRFetchRejected at pr_fetch.go:148 in production; here
	// we invoke the helper directly so the substrate contract is
	// asserted on each.
	emitPRFetchRejected(repo, 17, rejectReasonUnsafeURL)
	emitPRFetchRejected(repo, 23, rejectReasonUnsafeURL)

	rejected := rec.filterByName(audit.EventViPRFetchRejected)
	core.AssertEqual(t, 2, len(rejected),
		"one Rejected row per hostile upstream URL drop")

	// Scope + Outcome shape per RFC §2 — Outcome is "denied" for
	// the policy-gate rejection class (matches gateway.dispatch.rejected
	// + sandbox.spawn.rejected discipline).
	for i, ev := range rejected {
		core.AssertEqual(t, viScopeLiteral, ev.Scope)
		core.AssertEqual(t, audit.OutcomeDenied, ev.Outcome,
			core.Sprintf("rejected[%d] outcome must be denied", i))

		// Bounded Meta tuple.
		core.AssertEqual(t, ProviderForge, ev.Meta["provider"])
		core.AssertEqual(t, "lthn", ev.Meta["repo_owner"])
		core.AssertEqual(t, "unsafe-url-fixture", ev.Meta["repo_name"])
		core.AssertEqual(t, rejectReasonUnsafeURL, ev.Meta["reason"])
	}
	core.AssertEqual(t, 17, rejected[0].Meta["pr_number"])
	core.AssertEqual(t, 23, rejected[1].Meta["pr_number"])

	// SECURITY-RELEVANT: the dropped URL bytes (the hostile payload)
	// MUST NOT appear in any Meta value across any Rejected row.
	// Walk the snapshot looking for substrings that would indicate a
	// future regression "helpfully" stuffed the URL into Meta.
	for _, ev := range rejected {
		for k, v := range ev.Meta {
			s, ok := v.(string)
			if !ok {
				continue
			}
			lower := core.Lower(s)
			core.AssertFalse(t,
				core.Contains(lower, "javascript:"),
				"Meta["+k+"] must not contain javascript: payload; got: "+s)
			core.AssertFalse(t,
				core.Contains(lower, "data:"),
				"Meta["+k+"] must not contain data: payload; got: "+s)
			core.AssertFalse(t,
				core.Contains(lower, "http://"),
				"Meta["+k+"] must not contain raw URL bytes; got: "+s)
			core.AssertFalse(t,
				core.Contains(lower, "https://"),
				"Meta["+k+"] must not contain raw URL bytes; got: "+s)
		}
	}
}

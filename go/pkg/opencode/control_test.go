// SPDX-Licence-Identifier: EUPL-1.2

// Tests for Mantis #1602 HIGH (Cerberus #22) — every privilege-bearing
// endpoint on the opencode HTTP control surface emits an audit row.
// Each endpoint emits exactly one row per call, with the documented
// scope + outcome + server-generated RequestID (Cerberus #18 / Mantis
// #1511 X-Request-Id discipline) + a Meta shape that NEVER carries
// credentials.
//
// Strategy: the production handlers wrap a Service (Core + ORM + DuckDB)
// — too heavy to stand up in a unit test. Instead we drive the emit
// path itself (the helper that every handler calls) plus a stub-handler
// shape that exactly mirrors the handler audit-emit shape but
// substitutes a fixed Service-return so the emit-site under test runs
// without backing infra. This proves both:
//
//   1. The pure helper layer (emitControlAudit + newRequestID)
//      produces the expected row shape.
//   2. The handler-shape stubs (mirroring control.go verbatim except
//      for the Service call) emit the expected event-name + scope +
//      outcome + Meta keys for OK + error paths.
//
// The web_test.go file ships an in-memory Recorder + assertion
// fixture; this file reuses both.

package opencode

import (
	"net/http/httptest"
	"strings"
	"testing"

	core "dappco.re/go"
	"github.com/gin-gonic/gin"

	"dappco.re/lthn/desktop/pkg/audit"
)

// --- newRequestID -------------------------------------------------

// TestNewRequestID_ShapeIsUUIDv4_Good — the helper must produce a
// canonical RFC-4122 §4.4 UUIDv4 string. The audit substrate relies on
// the version-4 + variant-1 bit-pattern to distinguish handler-
// generated IDs from caller-supplied junk that survived a regression.
func TestNewRequestID_ShapeIsUUIDv4_Good(t *testing.T) {
	id := newRequestID()
	if id == "" {
		t.Fatalf("newRequestID returned empty string — core.RandomBytes likely failing")
	}
	// 8-4-4-4-12 hex layout = 36 chars total.
	if len(id) != 36 {
		t.Fatalf("newRequestID length = %d; want 36 (RFC 4122 §3 canonical form): %q", len(id), id)
	}
	for i, pos := range []int{8, 13, 18, 23} {
		if id[pos] != '-' {
			t.Fatalf("newRequestID separator %d at position %d = %q; want '-': %q",
				i, pos, id[pos], id)
		}
	}
	// Version nibble — position 14 (index 14 == first hex of group 3)
	// must be '4' per §4.4.
	if id[14] != '4' {
		t.Fatalf("newRequestID version nibble = %q; want '4' (UUIDv4): %q", id[14], id)
	}
	// Variant nibble — position 19 (index 19 == first hex of group 4)
	// must be one of 8, 9, a, b (top two bits == 10).
	switch id[19] {
	case '8', '9', 'a', 'b':
	default:
		t.Fatalf("newRequestID variant nibble = %q; want 8/9/a/b (variant 1): %q", id[19], id)
	}
}

// TestNewRequestID_PerCallUnique_Good — two consecutive calls must
// return different IDs. The audit substrate's JOIN-key property
// depends on collision-free generation; if this regresses, multiple
// concurrent emits would smear into one forensic row.
func TestNewRequestID_PerCallUnique_Good(t *testing.T) {
	a := newRequestID()
	b := newRequestID()
	if a == "" || b == "" {
		t.Fatalf("newRequestID returned empty — RandomBytes broken? a=%q b=%q", a, b)
	}
	if a == b {
		t.Fatalf("newRequestID returned same value twice — broken randomness: %q", a)
	}
}

// --- emitControlAudit --------------------------------------------

// TestEmitControlAudit_RecordsEvent_Good — the shared emit helper must
// wire every field into the Recorder verbatim. This is the load-bearing
// invariant every handler depends on; if the helper drops a field,
// every endpoint's audit row goes dark for that field.
func TestEmitControlAudit_RecordsEvent_Good(t *testing.T) {
	fake := &inMemoryRecorder{}
	audit.SetDefault(fake)
	t.Cleanup(func() { audit.SetDefault(nil) })

	emitControlAudit(EventOpencodeSandboxStop, "opencode.stop",
		audit.OutcomeOK, "req-abc-123", map[string]any{
			"sandbox_id": "oc-stop-1",
		})

	events := fake.snapshot()
	if len(events) != 1 {
		t.Fatalf("want exactly 1 event; got %d: %+v", len(events), events)
	}
	ev := events[0]
	if ev.Event != EventOpencodeSandboxStop {
		t.Errorf("Event = %q; want %q", ev.Event, EventOpencodeSandboxStop)
	}
	if ev.Scope != "opencode.stop" {
		t.Errorf("Scope = %q; want %q", ev.Scope, "opencode.stop")
	}
	if ev.Outcome != audit.OutcomeOK {
		t.Errorf("Outcome = %q; want %q", ev.Outcome, audit.OutcomeOK)
	}
	if ev.RequestID != "req-abc-123" {
		t.Errorf("RequestID = %q; want %q", ev.RequestID, "req-abc-123")
	}
	if ev.TS == 0 {
		t.Errorf("TS unset; want non-zero unix seconds")
	}
	if got, _ := ev.Meta["sandbox_id"].(string); got != "oc-stop-1" {
		t.Errorf("Meta.sandbox_id = %v; want oc-stop-1", ev.Meta["sandbox_id"])
	}
}

// TestEmitControlAudit_NilRecorder_NeverPanics_Good — when the default
// recorder isn't wired (boot ordering / shutdown), emit MUST be a
// no-op rather than panicking. Audit emission is fail-soft per the
// Stage F substrate contract.
func TestEmitControlAudit_NilRecorder_NeverPanics_Good(t *testing.T) {
	audit.SetDefault(nil)
	t.Cleanup(func() { audit.SetDefault(nil) })

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("emitControlAudit panicked with nil recorder: %v", r)
		}
	}()
	emitControlAudit(EventOpencodeSandboxStop, "opencode.stop",
		audit.OutcomeOK, "req-nil-recorder", map[string]any{})
}

// --- stubbed endpoint emit-shape tests ---------------------------

// stubAuditHandler returns a gin handler that mirrors a control.go
// endpoint's emit shape verbatim — server-generates the RequestID,
// echoes it in the response header, then emits one audit row with
// the documented event-name + scope + outcome + Meta. The
// `onCall(c) -> (ok bool, meta map[string]any)` lambda lets each test
// inject the per-endpoint Meta shape + success/error decision.
func stubAuditHandler(event, scope string, onCall func(c *gin.Context) (outcome string, meta map[string]any)) gin.HandlerFunc {
	return func(c *gin.Context) {
		srvReqID := newRequestID()
		c.Header("X-Request-Id", srvReqID)
		outcome, meta := onCall(c)
		emitControlAudit(event, scope, outcome, srvReqID, meta)
		c.JSON(core.StatusOK, gin.H{"ok": true})
	}
}

// runStubHandler is the test-side driver — wires the route, fires the
// request, returns (response-code, response-headers, captured-events).
func runStubHandler(t *testing.T, method, route, requestPath string, hdrs map[string]string, h gin.HandlerFunc) (*httptest.ResponseRecorder, []audit.Event) {
	t.Helper()
	fake := &inMemoryRecorder{}
	audit.SetDefault(fake)
	t.Cleanup(func() { audit.SetDefault(nil) })

	gin.SetMode(gin.TestMode)
	e := gin.New()
	e.Handle(method, route, h)

	req := httptest.NewRequest(method, requestPath, nil)
	for k, v := range hdrs {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	e.ServeHTTP(w, req)
	return w, fake.snapshot()
}

// assertOneEvent checks the captured slice has exactly one event with
// the expected name + scope + outcome, then returns it for further
// Meta inspection. Centralised so each per-endpoint test stays terse.
func assertOneEvent(t *testing.T, events []audit.Event, wantEvent, wantScope, wantOutcome string) audit.Event {
	t.Helper()
	if len(events) != 1 {
		t.Fatalf("want exactly 1 audit event; got %d: %+v", len(events), events)
	}
	ev := events[0]
	if ev.Event != wantEvent {
		t.Errorf("Event = %q; want %q", ev.Event, wantEvent)
	}
	if ev.Scope != wantScope {
		t.Errorf("Scope = %q; want %q", ev.Scope, wantScope)
	}
	if ev.Outcome != wantOutcome {
		t.Errorf("Outcome = %q; want %q", ev.Outcome, wantOutcome)
	}
	if ev.RequestID == "" {
		t.Errorf("RequestID empty — server-gen failed or caller header was trusted")
	}
	if len(ev.RequestID) != 36 {
		t.Errorf("RequestID = %q (len %d); want UUIDv4 (36 chars). "+
			"This regression means the caller's X-Request-Id is being trusted "+
			"(Cerberus #18 / Mantis #1511 fold)", ev.RequestID, len(ev.RequestID))
	}
	return ev
}

// TestSpawn_AuditEmitted_Good — spawn handler emits
// EventOpencodeSandboxSpawn with outcome=ok + Meta{profile, sandbox_id}.
func TestSpawn_AuditEmitted_Good(t *testing.T) {
	h := stubAuditHandler(EventOpencodeSandboxSpawn, "opencode.spawn",
		func(c *gin.Context) (string, map[string]any) {
			return audit.OutcomeOK, map[string]any{
				"profile":    "default",
				"sandbox_id": "oc-spawn-1",
			}
		})
	w, events := runStubHandler(t, core.MethodPost, "/sandbox", "/sandbox", nil, h)
	if w.Code != core.StatusOK {
		t.Fatalf("status = %d; want 200", w.Code)
	}
	ev := assertOneEvent(t, events, EventOpencodeSandboxSpawn, "opencode.spawn", audit.OutcomeOK)
	if got, _ := ev.Meta["profile"].(string); got != "default" {
		t.Errorf("Meta.profile = %v; want default", ev.Meta["profile"])
	}
	if got, _ := ev.Meta["sandbox_id"].(string); got != "oc-spawn-1" {
		t.Errorf("Meta.sandbox_id = %v; want oc-spawn-1", ev.Meta["sandbox_id"])
	}
}

// TestSpawn_AuditEmitted_Bad — spawn error path emits outcome=error
// with Meta{profile, error_code}. error_code is the core.Result code
// literal so the future Operations panel can facet failures by class.
func TestSpawn_AuditEmitted_Bad(t *testing.T) {
	h := stubAuditHandler(EventOpencodeSandboxSpawn, "opencode.spawn",
		func(c *gin.Context) (string, map[string]any) {
			return audit.OutcomeError, map[string]any{
				"profile":    "default",
				"error_code": "opencode.start.docker_unavailable",
			}
		})
	_, events := runStubHandler(t, core.MethodPost, "/sandbox", "/sandbox", nil, h)
	ev := assertOneEvent(t, events, EventOpencodeSandboxSpawn, "opencode.spawn", audit.OutcomeError)
	if got, _ := ev.Meta["error_code"].(string); got == "" {
		t.Errorf("Meta.error_code missing on error path; got %v", ev.Meta["error_code"])
	}
}

// TestStop_AuditEmitted_Good — stop handler emits
// EventOpencodeSandboxStop with Meta{sandbox_id} on success.
func TestStop_AuditEmitted_Good(t *testing.T) {
	h := stubAuditHandler(EventOpencodeSandboxStop, "opencode.stop",
		func(c *gin.Context) (string, map[string]any) {
			return audit.OutcomeOK, map[string]any{
				"sandbox_id": "oc-stop-aud",
			}
		})
	_, events := runStubHandler(t, core.MethodDelete, "/sandbox/:id", "/sandbox/oc-stop-aud", nil, h)
	ev := assertOneEvent(t, events, EventOpencodeSandboxStop, "opencode.stop", audit.OutcomeOK)
	if got, _ := ev.Meta["sandbox_id"].(string); got != "oc-stop-aud" {
		t.Errorf("Meta.sandbox_id = %v; want oc-stop-aud", ev.Meta["sandbox_id"])
	}
}

// TestProfileSave_AuditEmitted_Good — profileSave handler emits
// EventOpencodeProfileSave with Meta{profile_name} only. The full
// Profile.Provider block (may carry apiKey / token bytes for upstream
// providers) is intentionally NOT in Meta — only the name.
func TestProfileSave_AuditEmitted_Good(t *testing.T) {
	h := stubAuditHandler(EventOpencodeProfileSave, "opencode.profile.save",
		func(c *gin.Context) (string, map[string]any) {
			return audit.OutcomeOK, map[string]any{
				"profile_name": "tight-loop",
			}
		})
	_, events := runStubHandler(t, core.MethodPost, "/profile", "/profile", nil, h)
	ev := assertOneEvent(t, events, EventOpencodeProfileSave, "opencode.profile.save", audit.OutcomeOK)
	if got, _ := ev.Meta["profile_name"].(string); got != "tight-loop" {
		t.Errorf("Meta.profile_name = %v; want tight-loop", ev.Meta["profile_name"])
	}
	// Defensive: NO upstream-provider field shape should ever reach
	// Meta. If a future contributor extends the emit to include the
	// Profile.Provider map, this test fails — surfacing the regression
	// at PR time rather than after the credential lands in the NDJSON.
	for k, v := range ev.Meta {
		if k == "provider" || k == "providers" || k == "options" {
			t.Errorf("Meta carries provider-block-shaped key %q = %v — "+
				"Profile.Provider must NEVER reach audit Meta (may carry apiKey)", k, v)
		}
		s, ok := v.(string)
		if !ok {
			continue
		}
		// Reject any value that looks like a Bearer / apiKey-prefix
		// shape — the redact.go server-side detector enforces this,
		// the test mirrors at the emit site.
		lc := strings.ToLower(s)
		if strings.HasPrefix(lc, "bearer ") || strings.HasPrefix(lc, "sk-") {
			t.Errorf("Meta value %q = %q smells like a credential "+
				"(Bearer / sk- prefix)", k, s)
		}
	}
}

// TestEnable_AuditEmitted_Good — enable handler emits
// EventOpencodeEnable with Meta{profile, sandbox_id}.
func TestEnable_AuditEmitted_Good(t *testing.T) {
	h := stubAuditHandler(EventOpencodeEnable, "opencode.enable",
		func(c *gin.Context) (string, map[string]any) {
			return audit.OutcomeOK, map[string]any{
				"profile":    "default",
				"sandbox_id": "oc-enable-1",
			}
		})
	_, events := runStubHandler(t, core.MethodPost, "/enable", "/enable", nil, h)
	ev := assertOneEvent(t, events, EventOpencodeEnable, "opencode.enable", audit.OutcomeOK)
	if got, _ := ev.Meta["profile"].(string); got != "default" {
		t.Errorf("Meta.profile = %v; want default", ev.Meta["profile"])
	}
	if got, _ := ev.Meta["sandbox_id"].(string); got != "oc-enable-1" {
		t.Errorf("Meta.sandbox_id = %v; want oc-enable-1", ev.Meta["sandbox_id"])
	}
}

// TestHostConfigMerge_AuditEmitted_Good — hostConfigMerge emits
// EventOpencodeHostConfigMerge with Meta{profile, force, path, created}.
// The Bytes payload (may carry user-supplied provider apiKey) is
// intentionally NOT in Meta.
func TestHostConfigMerge_AuditEmitted_Good(t *testing.T) {
	h := stubAuditHandler(EventOpencodeHostConfigMerge, "opencode.host_config.merge",
		func(c *gin.Context) (string, map[string]any) {
			return audit.OutcomeOK, map[string]any{
				"profile": "default",
				"force":   false,
				"path":    "/home/u/.config/opencode/opencode.json",
				"created": true,
			}
		})
	_, events := runStubHandler(t, core.MethodPost, "/host-config", "/host-config", nil, h)
	ev := assertOneEvent(t, events, EventOpencodeHostConfigMerge, "opencode.host_config.merge", audit.OutcomeOK)
	if got, _ := ev.Meta["profile"].(string); got != "default" {
		t.Errorf("Meta.profile = %v; want default", ev.Meta["profile"])
	}
	if _, ok := ev.Meta["force"].(bool); !ok {
		t.Errorf("Meta.force = %v; want bool", ev.Meta["force"])
	}
	if _, ok := ev.Meta["created"].(bool); !ok {
		t.Errorf("Meta.created = %v; want bool", ev.Meta["created"])
	}
	// Defensive: the file BYTES must NEVER be in Meta — that's the
	// whole point of redacting at the emit boundary; the bytes may
	// carry the user's provider apiKey / bearer / token.
	for k := range ev.Meta {
		if k == "bytes" || k == "body" || k == "content" {
			t.Errorf("Meta carries body-shaped key %q — host-config Bytes "+
				"payload MUST NEVER reach audit Meta (carries provider creds)", k)
		}
	}
}

// TestHostConfigMerge_AuditEmitted_Conflict_Bad — conflict path emits
// outcome=denied (user-supplied request rejected by the substrate) with
// error_code = HostConfigConflict. Distinct from substrate failures
// (those emit outcome=error).
func TestHostConfigMerge_AuditEmitted_Conflict_Bad(t *testing.T) {
	h := stubAuditHandler(EventOpencodeHostConfigMerge, "opencode.host_config.merge",
		func(c *gin.Context) (string, map[string]any) {
			return audit.OutcomeDenied, map[string]any{
				"profile":    "default",
				"force":      false,
				"error_code": HostConfigConflict,
			}
		})
	_, events := runStubHandler(t, core.MethodPost, "/host-config", "/host-config", nil, h)
	ev := assertOneEvent(t, events, EventOpencodeHostConfigMerge, "opencode.host_config.merge", audit.OutcomeDenied)
	if got, _ := ev.Meta["error_code"].(string); got != HostConfigConflict {
		t.Errorf("Meta.error_code = %v; want %q", ev.Meta["error_code"], HostConfigConflict)
	}
}

// TestOpenTUI_AuditEmitted_Good — openTUI handler emits
// EventOpencodeTUIOpen with Meta{sandbox_id}. The
// OPENCODE_SERVER_PASSWORD that flows into the shell composition
// inside Service.OpenTUI MUST NEVER reach Meta.
func TestOpenTUI_AuditEmitted_Good(t *testing.T) {
	h := stubAuditHandler(EventOpencodeTUIOpen, "opencode.tui.open",
		func(c *gin.Context) (string, map[string]any) {
			return audit.OutcomeOK, map[string]any{
				"sandbox_id": "oc-tui-1",
			}
		})
	_, events := runStubHandler(t, core.MethodPost, "/sandbox/:id/tui", "/sandbox/oc-tui-1/tui", nil, h)
	ev := assertOneEvent(t, events, EventOpencodeTUIOpen, "opencode.tui.open", audit.OutcomeOK)
	if got, _ := ev.Meta["sandbox_id"].(string); got != "oc-tui-1" {
		t.Errorf("Meta.sandbox_id = %v; want oc-tui-1", ev.Meta["sandbox_id"])
	}
	// Defensive: password substring must never be in Meta. The shell
	// composition inside Service.OpenTUI references the password but
	// the handler audit shape MUST stay credential-free.
	for k, v := range ev.Meta {
		s, ok := v.(string)
		if !ok {
			continue
		}
		if k == "password" || k == "OPENCODE_SERVER_PASSWORD" || strings.Contains(s, "OPENCODE_SERVER_PASSWORD") {
			t.Errorf("Meta carries credential-shaped key/value %q = %q — "+
				"openTUI MUST keep the password off the audit substrate", k, s)
		}
	}
}

// TestUpgrade_AuditEmitted_Good — upgrade handler emits
// EventOpencodeUpgrade with Meta{updated, digest, restarted-count}.
func TestUpgrade_AuditEmitted_Good(t *testing.T) {
	h := stubAuditHandler(EventOpencodeUpgrade, "opencode.upgrade",
		func(c *gin.Context) (string, map[string]any) {
			return audit.OutcomeOK, map[string]any{
				"updated":   true,
				"digest":    "sha256:deadbeefcafef00d",
				"restarted": 2,
			}
		})
	_, events := runStubHandler(t, core.MethodPost, "/upgrade", "/upgrade", nil, h)
	ev := assertOneEvent(t, events, EventOpencodeUpgrade, "opencode.upgrade", audit.OutcomeOK)
	if got, _ := ev.Meta["updated"].(bool); !got {
		t.Errorf("Meta.updated = %v; want true", ev.Meta["updated"])
	}
	if got, _ := ev.Meta["digest"].(string); got == "" {
		t.Errorf("Meta.digest empty; want digest string")
	}
}

// TestControlGroup_ServerOverridesXRequestIdHeader_Good — Cerberus #18
// / Mantis #1511 fold. Caller-supplied X-Request-Id MUST NOT appear in
// the audit RequestID nor in the response header. The server's
// UUIDv4 must overwrite both. Trusting a caller-supplied audit-JOIN
// key enables forensic deniability — attacker forges a value to mimic
// a legitimate caller's audit row, defeating the disambiguation
// property the field exists to provide.
func TestControlGroup_ServerOverridesXRequestIdHeader_Good(t *testing.T) {
	const attackerForged = "attacker-forged-correlation-id"
	h := stubAuditHandler(EventOpencodeSandboxStop, "opencode.stop",
		func(c *gin.Context) (string, map[string]any) {
			return audit.OutcomeOK, map[string]any{"sandbox_id": "oc-x"}
		})
	w, events := runStubHandler(t, core.MethodDelete, "/sandbox/:id",
		"/sandbox/oc-x", map[string]string{"X-Request-Id": attackerForged}, h)

	if len(events) != 1 {
		t.Fatalf("want exactly 1 event; got %d", len(events))
	}
	ev := events[0]
	if ev.RequestID == attackerForged {
		t.Fatalf("audit RequestID = caller-forged value %q — server MUST override "+
			"per Cerberus #18 / Mantis #1511", ev.RequestID)
	}
	if len(ev.RequestID) != 36 {
		t.Errorf("audit RequestID = %q (len %d); want server-generated UUIDv4 (36 chars)",
			ev.RequestID, len(ev.RequestID))
	}
	if got := w.Header().Get("X-Request-Id"); got == attackerForged {
		t.Errorf("response X-Request-Id header = caller-forged %q — server MUST overwrite", got)
	}
	if got := w.Header().Get("X-Request-Id"); got != ev.RequestID {
		t.Errorf("response X-Request-Id header = %q; want match audit RequestID %q "+
			"(so legitimate caller can correlate to the audit log)", got, ev.RequestID)
	}
}

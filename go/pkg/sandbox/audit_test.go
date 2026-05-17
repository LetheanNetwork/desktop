// SPDX-Licence-Identifier: EUPL-1.2

// Tests for the Cerberus #47 S-4 (Mantis #1666) Repudiation gap close:
// Spawn / SpawnLong / Kill now emit typed audit.EventSandbox* rows via
// audit.Default() so the auditor sees lifecycle decisions instead of
// silently-discarded core.Warn lines. Mirror of pkg/runner/audit_test.go
// recordingRecorder fixture so concurrent test packages do not observe
// each other's recorders (SetDefault is process-global; t.Cleanup
// restores the prior default).

package sandbox

import (
	"sync"

	core "dappco.re/go"

	"dappco.re/lthn/desktop/pkg/audit"
)

// recordingRecorder captures every audit.Event the sandbox service
// emits during a test. Mirror of the same-named fixture in
// pkg/runner/audit_test.go and pkg/account/service_test.go.
type recordingRecorder struct {
	mu     sync.Mutex
	events []audit.Event
}

func (r *recordingRecorder) Record(ev audit.Event) core.Result {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, ev)
	return core.Ok(nil)
}

func (r *recordingRecorder) snapshot() []audit.Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]audit.Event, len(r.events))
	copy(out, r.events)
	return out
}

// filterByName returns only events whose Event field matches name.
// Lets a test assert on the lifecycle pair it cares about without
// being brittle to unrelated sibling emits in the same package.
func (r *recordingRecorder) filterByName(name string) []audit.Event {
	all := r.snapshot()
	out := make([]audit.Event, 0, len(all))
	for _, ev := range all {
		if ev.Event == name {
			out = append(out, ev)
		}
	}
	return out
}

// installAuditRecorder wires a recordingRecorder as audit.Default and
// restores the prior default at test end.
func installAuditRecorder(t *core.T) *recordingRecorder {
	t.Helper()
	rec := &recordingRecorder{}
	audit.SetDefault(rec)
	t.Cleanup(func() { audit.SetDefault(nil) })
	return rec
}

// TestSandbox_Spawn_EmitsAuditTrail_Good drives Spawn() with an input
// that fails fast in prepareSpawnInput (negative memory), then asserts
// the Requested + Failed lifecycle pair lands on the audit substrate
// with the expected Meta shape. We use a validation-failure path
// rather than a real container run because the sandbox CI surface
// has no container runtime; the audit-emit shape is identical.
func TestSandbox_Spawn_EmitsAuditTrail_Good(t *core.T) {
	rec := installAuditRecorder(t)
	svc := newTestService(Options{})

	r := NewSpawnPort(svc).Spawn(SpawnInput{
		Image:   "alpine:3.21",
		Command: "true",
		Memory:  -1, // forces prepareSpawnInput to reject
	})
	core.AssertFalse(t, r.OK, "negative memory should fail validation")

	requested := rec.filterByName(audit.EventSandboxSpawnRequested)
	core.AssertEqual(t, 1, len(requested), "Spawn must emit exactly one Requested")
	core.AssertEqual(t, "sandbox", requested[0].Scope)
	core.AssertEqual(t, audit.OutcomeOK, requested[0].Outcome)
	core.AssertEqual(t, "alpine:3.21", requested[0].Meta["image"])
	commandHash, ok := requested[0].Meta["command_hash"].(string)
	core.AssertTrue(t, ok, "Requested Meta must carry command_hash as string")
	core.AssertEqual(t, 64, len(commandHash), "command_hash is SHA-256 hex (64 chars)")
	core.AssertEqual(t, core.SHA256HexString("true"), commandHash)

	failed := rec.filterByName(audit.EventSandboxSpawnFailed)
	core.AssertEqual(t, 1, len(failed), "validation reject must emit Failed")
	core.AssertEqual(t, audit.OutcomeFailed, failed[0].Outcome)
	_, hasErrCode := failed[0].Meta["error_code"]
	core.AssertTrue(t, hasErrCode, "Failed Meta must carry error_code key")

	// Defensive: zero Succeeded rows on a failure path.
	succeeded := rec.filterByName(audit.EventSandboxSpawnSucceeded)
	core.AssertEqual(t, 0, len(succeeded), "Failed path must not emit Succeeded")
}

// TestSandbox_SpawnLong_EmitsAuditTrail_Good drives SpawnLong() with
// an empty Command (fails validation after Requested emit) and asserts
// the Requested + Failed pair lands. Mirrors the validation-failure
// shape of the Spawn test for the same CI-runtime-independence reason.
func TestSandbox_SpawnLong_EmitsAuditTrail_Good(t *core.T) {
	rec := installAuditRecorder(t)
	svc := newTestService(Options{})

	r := NewSpawnPort(svc).SpawnLong(SpawnLongInput{
		Image: "lthn/dev:latest",
		// Command intentionally empty — triggers validation reject
	})
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "command is required")

	requested := rec.filterByName(audit.EventSandboxLongRequested)
	core.AssertEqual(t, 1, len(requested), "SpawnLong must emit Requested")
	core.AssertEqual(t, "sandbox", requested[0].Scope)
	core.AssertEqual(t, audit.OutcomeOK, requested[0].Outcome)
	core.AssertEqual(t, "lthn/dev:latest", requested[0].Meta["image"])
	commandHash, ok := requested[0].Meta["command_hash"].(string)
	core.AssertTrue(t, ok)
	core.AssertEqual(t, 64, len(commandHash))
	core.AssertEqual(t, core.SHA256HexString(""), commandHash,
		"command_hash mirrors the empty Command input")

	failed := rec.filterByName(audit.EventSandboxLongFailed)
	core.AssertEqual(t, 1, len(failed), "validation reject must emit Failed")
	core.AssertEqual(t, audit.OutcomeFailed, failed[0].Outcome)
	_, hasErrCode := failed[0].Meta["error_code"]
	core.AssertTrue(t, hasErrCode)

	succeeded := rec.filterByName(audit.EventSandboxLongSucceeded)
	core.AssertEqual(t, 0, len(succeeded))
}

// TestSandbox_Kill_EmitsAuditTrail_Good drives Kill() against an
// already-registered handle so the success path is exercised end-to-end
// (registered → removed). Asserts Requested + Succeeded land in order;
// Failed must not appear.
func TestSandbox_Kill_EmitsAuditTrail_Good(t *core.T) {
	rec := installAuditRecorder(t)
	svc := newTestService(Options{})

	// Pre-register a handle so Kill() finds + removes it cleanly.
	id := "sb-killgood"
	svc.mu.Lock()
	if svc.handles == nil {
		svc.handles = map[string]*ContainerHandle{}
	}
	svc.handles[id] = &ContainerHandle{SandboxID: id, Status: StatusReady}
	svc.mu.Unlock()

	r := NewSpawnPort(svc).Kill(id)
	core.AssertTrue(t, r.OK)

	requested := rec.filterByName(audit.EventSandboxKillRequested)
	core.AssertEqual(t, 1, len(requested), "Kill must emit Requested")
	core.AssertEqual(t, "sandbox", requested[0].Scope)
	core.AssertEqual(t, id, requested[0].Meta["sandbox_id"])
	core.AssertEqual(t, "lthn-sandbox-"+id, requested[0].Meta["container_name"])

	succeeded := rec.filterByName(audit.EventSandboxKillSucceeded)
	core.AssertEqual(t, 1, len(succeeded), "registered Kill must emit Succeeded")
	core.AssertEqual(t, audit.OutcomeOK, succeeded[0].Outcome)
	core.AssertEqual(t, id, succeeded[0].Meta["sandbox_id"])

	failed := rec.filterByName(audit.EventSandboxKillFailed)
	core.AssertEqual(t, 0, len(failed), "registered Kill must not emit Failed")
}

// TestSandbox_VolumeReject_EmitsAuditEvent_Bad drives buildLongRunArgs
// through SpawnLong with a volume whose name fails IsValidVolumeName
// (#1431) AND a sibling volume whose container path fails
// IsValidContainerPath (#1446). Both rejections must land as
// EventSandboxVolumeRejected with the correct categorical reason.
// The defence-in-depth core.Warn line stays — the audit emit is
// additive, not a replacement.
func TestSandbox_VolumeReject_EmitsAuditEvent_Bad(t *core.T) {
	rec := installAuditRecorder(t)
	svc := newTestService(Options{})

	// Exercise buildLongRunArgs directly — the audit emit for a
	// rejected volume is part of the argument-vector assembly and
	// fires regardless of whether the downstream ps.Run docker call
	// succeeds. Driving via SpawnLong is fragile in CI because the
	// validation gate runs BEFORE buildLongRunArgs and our test
	// service has no real docker runtime, so the public-API path
	// returns long before the volume loop.
	_ = svc.buildLongRunArgs("docker", "lthn-sandbox-sb-volreject", 0, SpawnLongInput{
		Image:   "lthn/dev:latest",
		Command: "sleep",
		Args:    []string{"60"},
		Volumes: []LongVolumeMount{
			{Name: "/var/run/docker.sock", Container: "/sock"},
			{Name: "validname", Container: "/bad path"},
		},
	})

	rejected := rec.filterByName(audit.EventSandboxVolumeRejected)
	core.AssertEqual(t, 2, len(rejected),
		"both invalid_name AND invalid_container_path must emit Rejected")

	// First reject — IsValidVolumeName tripped by the leading slash.
	core.AssertEqual(t, audit.OutcomeDenied, rejected[0].Outcome)
	core.AssertEqual(t, "/var/run/docker.sock", rejected[0].Meta["volume_name"])
	core.AssertEqual(t, "invalid_name", rejected[0].Meta["reason"])

	// Second reject — IsValidContainerPath tripped by the embedded space.
	core.AssertEqual(t, audit.OutcomeDenied, rejected[1].Outcome)
	core.AssertEqual(t, "validname", rejected[1].Meta["volume_name"])
	core.AssertEqual(t, "/bad path", rejected[1].Meta["container"])
	core.AssertEqual(t, "invalid_container_path", rejected[1].Meta["reason"])
}

// TestSandbox_AuditMeta_NoRawArgsOrEnv_Good asserts the Cerberus
// #1465 closure-only-scope discipline at the sandbox lifecycle
// boundary: neither raw Args nor Env values appear in any Meta
// payload across Spawn / SpawnLong / Kill. The redactor in pkg/audit
// also enforces this at the sink layer; the emit-site MUST NOT supply
// these in the first place (defence in depth).
func TestSandbox_AuditMeta_NoRawArgsOrEnv_Good(t *core.T) {
	rec := installAuditRecorder(t)
	svc := newTestService(Options{})

	const sensitiveArg = "SUPERSECRETARGDONOTPERSIST"
	const sensitiveEnv = "SUPERSECRETENVDONOTPERSIST"

	// Drive Spawn — validation reject by negative memory; we just want
	// the Requested emit's Meta shape.
	_ = NewSpawnPort(svc).Spawn(SpawnInput{
		Image:   "alpine:3.21",
		Command: "true",
		Args:    []string{sensitiveArg},
		Memory:  -1,
	})

	// Drive SpawnLong — validation reject by empty command; pass Args
	// + Env so the redactor would have something to redact if leaked.
	_ = NewSpawnPort(svc).SpawnLong(SpawnLongInput{
		Image:   "lthn/dev:latest",
		Command: "",
		Args:    []string{sensitiveArg},
		Env:     map[string]string{"SECRET": sensitiveEnv},
	})

	// Drive Kill — unknown sandbox; the Requested + Failed Meta should
	// still not carry argument bytes.
	_ = NewSpawnPort(svc).Kill("sb-nonexistent")

	for _, ev := range rec.snapshot() {
		// Top-level fields must not carry the sensitive bytes.
		core.AssertFalse(t, core.Contains(ev.Event, sensitiveArg))
		core.AssertFalse(t, core.Contains(ev.Event, sensitiveEnv))
		core.AssertFalse(t, core.Contains(ev.Scope, sensitiveArg))
		core.AssertFalse(t, core.Contains(ev.Scope, sensitiveEnv))
		// Walk Meta string values.
		for k, v := range ev.Meta {
			if s, ok := v.(string); ok {
				core.AssertFalse(t, core.Contains(s, sensitiveArg),
					"Meta["+k+"] must not echo arg bytes")
				core.AssertFalse(t, core.Contains(s, sensitiveEnv),
					"Meta["+k+"] must not echo env bytes")
			}
		}
		// Reserved smuggle-surface canaries — the emit-site is the only
		// place to enforce these so an audit reader can trust the schema.
		_, hasArgs := ev.Meta["args"]
		core.AssertFalse(t, hasArgs, "Meta must never carry an `args` key")
		_, hasEnv := ev.Meta["env"]
		core.AssertFalse(t, hasEnv, "Meta must never carry an `env` key")
		_, hasCommand := ev.Meta["command"]
		core.AssertFalse(t, hasCommand,
			"Meta must never carry a raw `command` key (only command_hash)")
		_, hasVolumes := ev.Meta["volumes"]
		core.AssertFalse(t, hasVolumes,
			"Meta must never carry a raw `volumes` key on lifecycle rows")
	}
}

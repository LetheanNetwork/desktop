// SPDX-Licence-Identifier: EUPL-1.2

// second_instance_test.go — Cerberus #70 F-4 LOW coverage for the
// OnSecondInstanceLaunch fallback path in pkg/desktop. Two specs:
//
//   - WindowsUnregistered_FallbackOpensApp_Good: confirms that when
//     "app" is not registered in CoreGUI, restoreSecondInstanceWindow
//     reaches the audited window.open fallback. The action substrate
//     isn't wired in
//     this test (Wails isn't booted), so the assertion is on the
//     audit emit that always fires alongside the create-and-show
//     attempt — the substrate row is the load-bearing forensic
//     value-add per the Cerberus recommendation.
//   - AuditEmitOnFallback_Good: confirms the emitSecondInstanceFallback
//     helper records an EventDesktopSecondInstanceFallback row with
//     the primary_targets + fallback_target + fallback_via Meta keys,
//     OutcomeOK, desktopScope, and zero SecondInstanceData payload
//     bytes (the caller-controlled Args / WorkingDir / AdditionalData
//     must never reach the substrate per the const docstring).
//
// AX-7 naming: TestDesktop_SecondInstance_<Variant>.

package desktop

import (
	core "dappco.re/go"
	gui "dappco.re/go/render/display/webkit"
	"dappco.re/lthn/desktop/pkg/audit"
)

// secondInstanceAuditFixture installs a real *audit.Service over a temp
// HOME so emitSecondInstanceFallback lands in a flushable Service the
// test can query via NDJSON read. Mirrors trayAuditFixture in tray_test.go
// — the audit substrate is per-record fsync so the async Record path
// suffices; no RecordSync needed.
func secondInstanceAuditFixture(t *core.T) *audit.Service {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	svc := audit.New(nil, audit.Options{})
	core.AssertNotNil(t, svc, "audit.New must return non-nil")
	audit.SetDefault(svc)
	t.Cleanup(func() { audit.SetDefault(nil) })
	return svc
}

// --- restoreSecondInstanceWindow ---

// TestDesktop_SecondInstance_WindowsUnregistered_FallbackOpensApp_Good
// drives the fallback branch by passing a fresh *core.Core that has no
// CoreGUI window service attached — windowExists returns false for app,
// and the handler must reach the audit-emit + window.open
// create-and-show branch. The window.open action returns its own r.OK
// (no Wails app
// loop in tests) — we cannot assert on its side-effect here. The
// observable that DOES land is the audit row, exactly per the Cerberus
// #70 F-4 recommendation that the audit emit is the load-bearing
// forensic value-add even when the downstream create-and-show itself
// fails. Spec-name preserved verbatim from the brief.
func TestDesktop_SecondInstance_WindowsUnregistered_FallbackOpensApp_Good(t *core.T) {
	svc := secondInstanceAuditFixture(t)

	c := core.New()
	core.AssertNotNil(t, c, "core.New must return non-nil")

	// Sanity precondition — the one boot-registered window must be
	// absent so the fallback branch is the one under test.
	core.AssertFalse(t, windowExists(c, "app"),
		"app must be unregistered in the test core")

	// Drive the handler. The test asserts on the audit emit, not on the
	// window-creation side effect.
	restoreSecondInstanceWindow(c)

	// The audit substrate must carry exactly one fallback row.
	qr := svc.Query(audit.QueryInput{
		Event: audit.EventDesktopSecondInstanceFallback,
		Limit: 10,
	})
	core.AssertTrue(t, qr.OK, "Query must succeed")
	out, _ := qr.Value.(audit.QueryOutput)
	core.AssertEqual(t, 1, len(out.Events),
		"exactly one desktop.second_instance.fallback row must land")
}

// --- emitSecondInstanceFallback ---

// TestDesktop_SecondInstance_AuditEmitOnFallback_Good confirms the audit
// emit lands a row with EventDesktopSecondInstanceFallback, OutcomeOK,
// the "desktop" scope, and the primary_targets + fallback_target +
// fallback_via Meta keys. The row body MUST NOT contain any
// SecondInstanceData payload bytes — only the bounded-vocab fallback
// shape literals. Mirrors TestTray_EmitTrayPluginClicked_EmitsAudit_Good
// in tray_test.go.
func TestDesktop_SecondInstance_AuditEmitOnFallback_Good(t *core.T) {
	svc := secondInstanceAuditFixture(t)
	emitSecondInstanceFallback()
	qr := svc.Query(audit.QueryInput{
		Event: audit.EventDesktopSecondInstanceFallback,
		Limit: 10,
	})
	core.AssertTrue(t, qr.OK, "Query must succeed")
	out, _ := qr.Value.(audit.QueryOutput)
	core.AssertEqual(t, 1, len(out.Events),
		"exactly one desktop.second_instance.fallback row")
	ev := out.Events[0]
	core.AssertEqual(t, audit.EventDesktopSecondInstanceFallback, ev.Event)
	core.AssertEqual(t, audit.OutcomeOK, ev.Outcome)
	core.AssertEqual(t, desktopScope, ev.Scope)
	core.AssertEqual(t, "app", ev.Meta["primary_targets"])
	core.AssertEqual(t, "app", ev.Meta["fallback_target"])
	core.AssertEqual(t, "window.open", ev.Meta["fallback_via"])
	// Negative — no caller-controlled payload bytes must reach the row.
	_, hasArgs := ev.Meta["args"]
	core.AssertFalse(t, hasArgs, "args must never reach the audit row")
	_, hasWD := ev.Meta["workdir"]
	core.AssertFalse(t, hasWD, "workdir must never reach the audit row")
	_, hasAdd := ev.Meta["additional"]
	core.AssertFalse(t, hasAdd, "additional must never reach the audit row")
}

func TestDesktop_SecondInstance_Good_DeepLinkRoutesInRunningApp(t *core.T) {
	c, emitted, windowActions := deepLinkEventFixture(t)

	handleSecondInstanceLaunch(c, gui.SecondInstanceData{
		Args: []string{"lthn", "lthn://open/document?id=record-42"},
	})

	core.AssertEqual(t, []string{
		"dock.show_icon",
		"window.restore",
		"window.set_visibility",
		"window.focus",
	}, *windowActions)
	core.AssertEqual(t, 2, len(*emitted))
	if len(*emitted) < 2 {
		return
	}
	core.AssertEqual(t, "lthn:app:second-instance", (*emitted)[0].Name)
	core.AssertEqual(t, "navigate", (*emitted)[1].Name)
	core.AssertEqual(t, map[string]string{
		"action":   "open",
		"resource": "document",
		"id":       "record-42",
	}, (*emitted)[1].Data)
}

func TestDesktop_SecondInstance_Bad_NoDeepLinkRestoresMainWindow(t *core.T) {
	c, emitted, windowActions := deepLinkEventFixture(t)

	handleSecondInstanceLaunch(c, gui.SecondInstanceData{
		Args: []string{"lthn", "--background"},
	})

	core.AssertEqual(t, 1, len(*emitted))
	core.AssertEqual(t, "lthn:app:second-instance", (*emitted)[0].Name)
	core.AssertEqual(t, 4, len(*windowActions))
}

func TestDesktop_SecondInstance_Ugly_InvalidDeepLinkStillRestoresWindow(t *core.T) {
	c, emitted, windowActions := deepLinkEventFixture(t)

	handleSecondInstanceLaunch(c, gui.SecondInstanceData{
		Args: []string{"lthn", "lthn://chat/%zz"},
	})

	core.AssertEqual(t, 1, len(*emitted))
	core.AssertEqual(t, 4, len(*windowActions))
}

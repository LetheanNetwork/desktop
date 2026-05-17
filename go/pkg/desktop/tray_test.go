// SPDX-Licence-Identifier: EUPL-1.2

// tray_test.go — Cerberus #70 F-3 MED coverage for the plugin
// enumeration safety in the system-tray menu surface. Four specs:
//
//   - InvalidPluginCodeRejected_Bad: hostile codes (path traversal,
//     leading dash, NUL byte, dots) are filtered out of the tray
//     menu items list — buildPluginTrayItems honours
//     paths.IsValidPluginCode.
//   - LabelLengthCapped_Good: a 1000-char label is byte-capped at
//     trayPluginMaxLabelBytes (64) — sanitizePluginLabel enforces
//     the boundary before the label reaches the native tray API.
//   - LabelControlCharsStripped_Good: ASCII control bytes
//     (newline, carriage return, NUL, DEL) are stripped from the
//     label so they cannot scramble the rendered menu surface.
//   - EmitsAudit_Good: the package-level emitTrayPluginClicked
//     helper records an EventTrayPluginClicked row with the
//     plugin_code + resolved_via Meta keys, no label content.
//
// AX-7 naming: TestTray_<Function>_<Variant>.

package desktop

import (
	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/audit"
	"dappco.re/lthn/desktop/pkg/plugin"
)

// trayAuditFixture installs a real *audit.Service over a temp HOME so
// emitTrayPluginClicked lands in a flushable Service, then returns
// the recorder for the test to query via NDJSON read. The audit-
// substrate fsync is per-record so RecordSync is unnecessary; the
// async Record path is also fine because the day-file is opened on
// first write + flushed on the recorder's defer.
func trayAuditFixture(t *core.T) *audit.Service {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	svc := audit.New(nil, audit.Options{})
	core.AssertNotNil(t, svc, "audit.New must return non-nil")
	audit.SetDefault(svc)
	t.Cleanup(func() { audit.SetDefault(nil) })
	return svc
}

// --- buildPluginTrayItems ---

// TestTray_BuildPluginTrayItems_InvalidPluginCodeRejected_Bad confirms
// that plugin entries whose Code fails paths.IsValidPluginCode are
// silently filtered out of the tray menu surface. The Cerberus #70
// F-3 attack walk seeds a hostile manifest with traversal + special-
// char codes; the gate must drop them before the click router can
// route them through openPluginWindow.
func TestTray_BuildPluginTrayItems_InvalidPluginCodeRejected_Bad(t *core.T) {
	entries := []plugin.MenuEntry{
		{Code: "opencode", Label: "OpenCode", Running: true},
		{Code: "../etc", Label: "traversal", Running: true},
		{Code: "/bin/sh", Label: "absolute", Running: true},
		{Code: ".hidden", Label: "dotfile", Running: true},
		{Code: "-rf", Label: "dash", Running: true},
		{Code: "code\x00sneak", Label: "nul", Running: true},
		{Code: "code/sub", Label: "slash", Running: true},
		{Code: "", Label: "empty", Running: true},
		{Code: "ollama", Label: "Ollama", Running: false},
	}
	items := buildPluginTrayItems(entries)
	// Two valid entries: "opencode" + "ollama".
	core.AssertEqual(t, 2, len(items), "only valid codes must survive")
	// Verify the ActionIDs carry the trayPluginPrefix + valid code.
	core.AssertEqual(t, trayPluginPrefix+"opencode", items[0].ActionID)
	core.AssertEqual(t, trayPluginPrefix+"ollama", items[1].ActionID)
}

// TestTray_BuildPluginTrayItems_LabelLengthCapped_Good confirms that
// a pathological 1000-char label is byte-capped at the boundary so
// the native tray surface never receives an unbounded string. The
// running plugin's label survives intact up to the cap; the stopped
// plugin's label is capped FIRST and the suffix is appended AFTER
// so the operator-visible affordance is preserved.
func TestTray_BuildPluginTrayItems_LabelLengthCapped_Good(t *core.T) {
	longLabel := ""
	for i := 0; i < 1000; i++ {
		longLabel = longLabel + "x"
	}
	entries := []plugin.MenuEntry{
		{Code: "opencode", Label: longLabel, Running: true},
		{Code: "ollama", Label: longLabel, Running: false},
	}
	items := buildPluginTrayItems(entries)
	core.AssertEqual(t, 2, len(items))
	// Running entry: label is the byte-capped string, no suffix.
	core.AssertEqual(t, trayPluginMaxLabelBytes, len(items[0].Label),
		"running entry's label must be byte-capped at trayPluginMaxLabelBytes")
	// Stopped entry: label is byte-capped FIRST then suffix appended,
	// so total length = cap + len(suffix).
	expectedLen := trayPluginMaxLabelBytes + len(trayPluginStoppedSuffix)
	core.AssertEqual(t, expectedLen, len(items[1].Label),
		"stopped entry's label must be capped THEN suffix appended")
	core.AssertTrue(t, core.HasSuffix(items[1].Label, trayPluginStoppedSuffix),
		"stopped suffix must be visible after the cap")
}

// TestTray_BuildPluginTrayItems_LabelControlCharsStripped_Good confirms
// that ASCII control bytes (newline, carriage return, NUL, DEL, TAB)
// are stripped from the label before it lands on the native tray
// surface. A hostile manifest cannot inject a NL that splits a row.
func TestTray_BuildPluginTrayItems_LabelControlCharsStripped_Good(t *core.T) {
	entries := []plugin.MenuEntry{
		{Code: "opencode", Label: "Open\nCode\rWith\x00All\x7FEvil\tHere", Running: true},
	}
	items := buildPluginTrayItems(entries)
	core.AssertEqual(t, 1, len(items))
	// Expected: OpenCodeWithAllEvilHere (control bytes removed,
	// printable bytes preserved).
	core.AssertEqual(t, "OpenCodeWithAllEvilHere", items[0].Label,
		"control bytes must be stripped from the label")
}

// TestTray_SanitizePluginLabel_EmptyInput_Good confirms the empty-
// string fast path returns empty (no panic, no allocation).
func TestTray_SanitizePluginLabel_EmptyInput_Good(t *core.T) {
	core.AssertEqual(t, "", sanitizePluginLabel(""))
}

// --- emitTrayPluginClicked ---

// TestTray_EmitTrayPluginClicked_EmitsAudit_Good confirms the audit
// emit lands a row with EventTrayPluginClicked, OutcomeOK, the
// "tray" scope, and the plugin_code + resolved_via Meta keys. The
// row body MUST NOT contain any label bytes — only the bounded-
// vocab plugin code.
func TestTray_EmitTrayPluginClicked_EmitsAudit_Good(t *core.T) {
	svc := trayAuditFixture(t)
	emitTrayPluginClicked("opencode")
	// Query the substrate via the recorder's day-file API. Filter
	// by event name; expect exactly one match in this fresh fixture.
	qr := svc.Query(audit.QueryInput{
		Event: audit.EventTrayPluginClicked,
		Limit: 10,
	})
	core.AssertTrue(t, qr.OK, "Query must succeed")
	out, _ := qr.Value.(audit.QueryOutput)
	core.AssertEqual(t, 1, len(out.Events), "exactly one tray.plugin.clicked row")
	ev := out.Events[0]
	core.AssertEqual(t, audit.EventTrayPluginClicked, ev.Event)
	core.AssertEqual(t, audit.OutcomeOK, ev.Outcome)
	core.AssertEqual(t, trayScope, ev.Scope)
	core.AssertEqual(t, "opencode", ev.Meta["plugin_code"])
	core.AssertEqual(t, "tray_menu", ev.Meta["resolved_via"])
}

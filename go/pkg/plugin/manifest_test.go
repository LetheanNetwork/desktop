// SPDX-Licence-Identifier: EUPL-1.2

// Internal-package tests for manifest.go's validate / loadManifest /
// saveManifest. package plugin (white-box) so tests can drive the
// unexported validate() method and loadManifest/saveManifest directly
// with fixture files under t.TempDir, matching the pattern established
// in install_security_test.go.

package plugin

import core "dappco.re/go"

// ─── Manifest.validate ──────────────────────────────────────────────────

func TestManifest_Validate_Good_DefaultsApplied(t *core.T) {
	m := Manifest{Code: "opencode", Name: "OpenCode", Binary: "bin/opencode"}
	r := m.validate()
	core.RequireTrue(t, r.OK)
	out := r.Value.(Manifest)
	core.AssertEqual(t, "opencode", out.Namespace, "namespace defaults to code")
	core.AssertEqual(t, 5, out.StartupTimeout, "startup timeout defaults to 5s")
	core.RequireNotEmpty(t, out.Health, "health block is synthesised when absent")
	core.AssertEqual(t, "/opencode/health", out.Health.Path)
	core.AssertEqual(t, 30, out.Health.Interval)
	core.AssertEqual(t, 5, out.Health.Timeout)
}

func TestManifest_Validate_Good_ExplicitNamespacePreserved(t *core.T) {
	m := Manifest{Code: "opencode", Name: "OpenCode", Binary: "bin/opencode", Namespace: "oc"}
	r := m.validate()
	core.RequireTrue(t, r.OK)
	out := r.Value.(Manifest)
	core.AssertEqual(t, "oc", out.Namespace)
	core.AssertEqual(t, "/oc/health", out.Health.Path)
}

func TestManifest_Validate_Bad_MissingCode(t *core.T) {
	m := Manifest{Name: "OpenCode", Binary: "bin/opencode"}
	r := m.validate()
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "code is required")
}

func TestManifest_Validate_Bad_InvalidCode(t *core.T) {
	m := Manifest{Code: "../etc", Name: "OpenCode", Binary: "bin/opencode"}
	r := m.validate()
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "invalid code")
}

func TestManifest_Validate_Bad_MissingName(t *core.T) {
	m := Manifest{Code: "opencode", Binary: "bin/opencode"}
	r := m.validate()
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "name is required")
}

func TestManifest_Validate_Bad_MissingBinary(t *core.T) {
	m := Manifest{Code: "opencode", Name: "OpenCode"}
	r := m.validate()
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "binary path is required")
}

func TestManifest_Validate_Bad_InvalidBinary(t *core.T) {
	m := Manifest{Code: "opencode", Name: "OpenCode", Binary: "../../etc/passwd"}
	r := m.validate()
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "invalid binary path")
}

func TestManifest_Validate_Ugly_PartialHealthKeepsGivenInterval(t *core.T) {
	// Health supplied with only Interval set — Path/Timeout still get
	// defaulted, the caller's Interval is NOT overwritten.
	m := Manifest{
		Code: "opencode", Name: "OpenCode", Binary: "bin/opencode",
		Health: &Health{Interval: 99},
	}
	r := m.validate()
	core.RequireTrue(t, r.OK)
	out := r.Value.(Manifest)
	core.AssertEqual(t, 99, out.Health.Interval, "explicit interval preserved")
	core.AssertEqual(t, "/opencode/health", out.Health.Path, "path still defaulted")
	core.AssertEqual(t, 5, out.Health.Timeout, "timeout still defaulted")
}

func TestManifest_Validate_Ugly_ExplicitStartupTimeoutPreserved(t *core.T) {
	m := Manifest{Code: "opencode", Name: "OpenCode", Binary: "bin/opencode", StartupTimeout: 42}
	r := m.validate()
	core.RequireTrue(t, r.OK)
	out := r.Value.(Manifest)
	core.AssertEqual(t, 42, out.StartupTimeout)
}

// ─── loadManifest ───────────────────────────────────────────────────────

func TestManifest_LoadManifest_Good(t *core.T) {
	dir := t.TempDir()
	m := Manifest{Code: "opencode", Name: "OpenCode", Binary: "bin/opencode"}
	core.RequireTrue(t, saveManifest(dir, m).OK)
	r := loadManifest(core.PathJoin(dir, "plugin.json"))
	core.RequireTrue(t, r.OK)
	out := r.Value.(Manifest)
	core.AssertEqual(t, "opencode", out.Code)
	core.AssertEqual(t, "opencode", out.Namespace, "validate() defaulting runs on load too")
}

func TestManifest_LoadManifest_Bad_MissingFile(t *core.T) {
	dir := t.TempDir()
	r := loadManifest(core.PathJoin(dir, "plugin.json"))
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "read failed")
}

// TestManifest_LoadManifest_Bad_MalformedJSON — real fault injection:
// a manifest that isn't valid JSON must fail to parse rather than
// panicking or silently zero-valuing the struct.
func TestManifest_LoadManifest_Bad_MalformedJSON(t *core.T) {
	dir := t.TempDir()
	path := core.PathJoin(dir, "plugin.json")
	core.RequireTrue(t, core.WriteFile(path, []byte("{not json"), 0o644).OK)
	r := loadManifest(path)
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "parse failed")
}

func TestManifest_LoadManifest_Ugly_ParsesButFailsValidation(t *core.T) {
	dir := t.TempDir()
	path := core.PathJoin(dir, "plugin.json")
	// Valid JSON, but no "code" field — parses fine, fails validate().
	core.RequireTrue(t, core.WriteFile(path, []byte(`{"name":"X","binary":"bin/x"}`), 0o644).OK)
	r := loadManifest(path)
	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "code is required")
}

// ─── saveManifest ───────────────────────────────────────────────────────

func TestManifest_SaveManifest_Good(t *core.T) {
	dir := t.TempDir()
	m := Manifest{Code: "opencode", Name: "OpenCode", Version: "1.2.3", Binary: "bin/opencode"}
	r := saveManifest(dir, m)
	core.RequireTrue(t, r.OK)
	read := core.ReadFile(core.PathJoin(dir, "plugin.json"))
	core.RequireTrue(t, read.OK)
	core.AssertContains(t, string(read.Value.([]byte)), "1.2.3")
}

// TestManifest_SaveManifest_Bad_UnwritableDir — real fault injection:
// a directory that doesn't exist (and isn't created by saveManifest,
// which is intentionally a leaf writer, not a MkdirAll-then-write
// helper — that's writePlugin's job) must fail rather than panic.
func TestManifest_SaveManifest_Bad_UnwritableDir(t *core.T) {
	dir := t.TempDir() + "/does/not/exist"
	m := Manifest{Code: "opencode", Name: "OpenCode", Binary: "bin/opencode"}
	r := saveManifest(dir, m)
	core.AssertFalse(t, r.OK)
}

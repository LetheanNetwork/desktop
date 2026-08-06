// SPDX-Licence-Identifier: EUPL-1.2

// mcp_live_test.go — behavioural coverage for RegisterMCPTools + the
// five mcpXxx handlers (mcpList/mcpInstall/mcpLaunch/mcpStop/
// mcpUninstall), previously 0% covered beyond struct-shape tests.
// Driven against a real coremcp.Service + the fake-registry seam
// (WithFakeRegistry, export_test.go) + the live sandbox/orm core
// (newLiveMarketplaceCore, install_live_test.go) — no mocked HTTP,
// no mocked MCP server, no mocked sandbox.

package marketplace

import (
	"context"
	"net/http"
	"testing"

	core "dappco.re/go"
	coremcp "dappco.re/go/mcp/pkg/mcp"
)

// TestRegisterMCPTools_NilGuards_Good — nil mcpSvc / nil ms / a
// service whose Server() is nil are all safe no-ops.
func TestRegisterMCPTools_NilGuards_Good(t *testing.T) {
	RegisterMCPTools(nil, nil)
	RegisterMCPTools(nil, &Service{})
	RegisterMCPTools(&coremcp.Service{}, nil)
}

// TestRegisterMCPTools_RealServer_Good — a real coremcp.Service wires
// all five tools without error.
func TestRegisterMCPTools_RealServer_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	c := core.New(core.WithService(coremcp.Register))
	mcpSvc, ok := core.ServiceFor[*coremcp.Service](c, "mcp")
	if !ok || mcpSvc == nil {
		t.Fatal("expected a real *coremcp.Service to resolve")
	}
	ms := NewService(c)
	RegisterMCPTools(mcpSvc, ms) // must not panic
}

// TestMcpList_EmptyName_FallsBackToDefaultEndpoint_Good — mcpList's
// own logic has no bundle-name validation (unlike install/launch/stop/
// uninstall); it always tries FetchIndex. Point at the fake registry
// via the default-endpoint cache-seed shape and confirm the real
// SearchCatalogue filter runs against the fetched entries.
func TestMcpList_ReturnsFilteredCatalogue_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	base := WithFakeRegistry(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{"name":"opencode","display":"OpenCode","category":"ai-agents","source_url":"https://marketplace.lthn.ai/v1/opencode.yml"},
			{"name":"vaultwarden","display":"Vaultwarden","category":"security","source_url":"https://marketplace.lthn.ai/v1/vaultwarden.yml"}
		]`))
	}))
	svc := NewService(nil)
	// Prime the default-endpoint cache slot via a direct downloadIndex
	// call so mcpList's internal FetchIndex("") (no override param)
	// resolves from cache rather than attempting defaultIndexURL.
	if r := svc.downloadIndex(base+"/v1/index.json", svc.indexCachePath()); !r.OK {
		t.Fatalf("seed cache via downloadIndex: %s", r.Error())
	}

	_, out, err := svc.mcpList(context.Background(), nil, MCPListInput{Category: "ai-agents"})
	if err != nil {
		t.Fatalf("mcpList: %v", err)
	}
	if out.Total != 1 || len(out.Entries) != 1 || out.Entries[0].Name != "opencode" {
		t.Errorf("expected 1 ai-agents entry (opencode), got %+v", out)
	}
}

// TestMcpList_FetchIndexFails_FallsBackEmpty_Good — an unreachable
// registry (host not on the allowlist, no network attempted) makes
// mcpList degrade to an empty catalogue rather than propagating the
// error — the documented "agent gets a useful response even when the
// registry is unreachable" contract.
func TestMcpList_FetchIndexFails_FallsBackEmpty_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := NewService(nil)
	_, out, err := svc.mcpList(context.Background(), nil, MCPListInput{})
	if err != nil {
		t.Fatalf("mcpList must not return an error on registry failure: %v", err)
	}
	if out.Total != 0 || len(out.Entries) != 0 {
		t.Errorf("expected an empty fallback catalogue, got %+v", out)
	}
}

// TestMcpInstall_NameRequired_Bad
func TestMcpInstall_NameRequired_Bad(t *testing.T) {
	svc := NewService(nil)
	_, _, err := svc.mcpInstall(context.Background(), nil, MCPInstallInput{})
	if err == nil {
		t.Fatal("expected mcpInstall to reject an empty name")
	}
	if !core.Contains(err.Error(), "name is required") {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestMcpInstall_ExplicitSourceURL_ManifestFetchFails_Bad — a
// SourceURL is supplied (skips the catalogue-resolve branch) but the
// fake registry 404s the manifest fetch.
func TestMcpInstall_ExplicitSourceURL_ManifestFetchFails_Bad(t *testing.T) {
	base := WithFakeRegistry(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	svc := NewService(nil)
	_, _, err := svc.mcpInstall(context.Background(), nil, MCPInstallInput{
		Name: "opencode", SourceURL: base + "/opencode.yml",
	})
	if err == nil {
		t.Fatal("expected mcpInstall to fail when the manifest fetch 404s")
	}
	if !core.Contains(err.Error(), "manifest fetch failed") {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestMcpInstall_NoSourceURL_CatalogueMiss_Bad — no SourceURL and the
// fake registry's catalogue doesn't contain the requested name.
func TestMcpInstall_NoSourceURL_CatalogueMiss_Bad(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	base := WithFakeRegistry(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"name":"other","source_url":"https://marketplace.lthn.ai/v1/other.yml"}]`))
	}))
	svc := NewService(nil)
	if r := svc.downloadIndex(base+"/v1/index.json", svc.indexCachePath()); !r.OK {
		t.Fatalf("seed cache: %s", r.Error())
	}
	_, _, err := svc.mcpInstall(context.Background(), nil, MCPInstallInput{Name: "ghost-bundle"})
	if err == nil {
		t.Fatal("expected mcpInstall to fail when the name isn't in the catalogue")
	}
	if !core.Contains(err.Error(), "bundle not found in catalogue") {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestMcpInstall_FullPipeline_RealSandboxSpawnFails_Bad — SourceURL
// resolves via the fake registry to a real manifest; FetchManifest +
// Install both run for real; Install fails at the spawn-loop exec
// boundary (no container runtime), which mcpInstall surfaces as
// "install failed". Exercises mcpInstall's happy path all the way
// through to the Install() call.
func TestMcpInstall_FullPipeline_RealSandboxSpawnFails_Bad(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	const manifestYAML = "schema: lthn-vm/v1\nname: opencode\ndisplay: OpenCode\nimages:\n  - id: app\n    image: alpine:3.21\n"
	base := WithFakeRegistry(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(manifestYAML))
	}))
	c := newLiveMarketplaceCore(t)
	svc := NewService(c)

	_, _, err := svc.mcpInstall(context.Background(), nil, MCPInstallInput{
		Name: "opencode", SourceURL: base + "/opencode.yml",
	})
	if err == nil {
		t.Fatal("expected mcpInstall to fail — no container runtime present")
	}
	if !core.Contains(err.Error(), "install failed") {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestMcpLaunch_NameRequired_Bad
func TestMcpLaunch_NameRequired_Bad(t *testing.T) {
	svc := NewService(nil)
	_, _, err := svc.mcpLaunch(context.Background(), nil, MCPBundleIDInput{})
	if err == nil || !core.Contains(err.Error(), "name is required") {
		t.Fatalf("expected name-required error, got: %v", err)
	}
}

// TestMcpLaunch_NotInstalled_Bad
func TestMcpLaunch_NotInstalled_Bad(t *testing.T) {
	svc, _ := newTestMailAdjacentService(t)
	_, _, err := svc.mcpLaunch(context.Background(), nil, MCPBundleIDInput{Name: "ghost"})
	if err == nil {
		t.Fatal("expected mcpLaunch to fail for an uninstalled bundle")
	}
}

// TestMcpLaunch_Success_Good — Launch's best-effort spawn loop
// succeeds regardless of runtime availability.
func TestMcpLaunch_Success_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	c := newLiveMarketplaceCore(t)
	svc := NewService(c)
	m := liveTestManifest("mcp-launch-bundle")
	_ = svc.Install(InstallInput{Manifest: m}) // fails on spawn, still persists

	_, out, err := svc.mcpLaunch(context.Background(), nil, MCPBundleIDInput{Name: "mcp-launch-bundle"})
	if err != nil {
		t.Fatalf("mcpLaunch: %v", err)
	}
	if !out.OK {
		t.Error("expected MCPBundleIDOutput.OK=true")
	}
}

// TestMcpStop_NameRequired_Bad
func TestMcpStop_NameRequired_Bad(t *testing.T) {
	svc := NewService(nil)
	_, _, err := svc.mcpStop(context.Background(), nil, MCPBundleIDInput{})
	if err == nil || !core.Contains(err.Error(), "name is required") {
		t.Fatalf("expected name-required error, got: %v", err)
	}
}

// TestMcpStop_Idempotent_Good — Stop on an unknown bundle is a
// success (no-op) per Stop's own documented idempotence.
func TestMcpStop_Idempotent_Good(t *testing.T) {
	svc := NewService(nil)
	_, out, err := svc.mcpStop(context.Background(), nil, MCPBundleIDInput{Name: "never-installed"})
	if err != nil {
		t.Fatalf("mcpStop: %v", err)
	}
	if !out.OK {
		t.Error("expected OK=true for the idempotent no-op path")
	}
}

// TestMcpUninstall_NameRequired_Bad
func TestMcpUninstall_NameRequired_Bad(t *testing.T) {
	svc := NewService(nil)
	_, _, err := svc.mcpUninstall(context.Background(), nil, MCPBundleIDInput{})
	if err == nil || !core.Contains(err.Error(), "name is required") {
		t.Fatalf("expected name-required error, got: %v", err)
	}
}

// TestMcpUninstall_Idempotent_Good
func TestMcpUninstall_Idempotent_Good(t *testing.T) {
	svc := NewService(nil)
	_, out, err := svc.mcpUninstall(context.Background(), nil, MCPBundleIDInput{Name: "never-installed"})
	if err != nil {
		t.Fatalf("mcpUninstall: %v", err)
	}
	if !out.OK {
		t.Error("expected OK=true for the idempotent no-op path")
	}
}

// TestMcpErr_WrapsMessage_Ugly
func TestMcpErr_WrapsMessage_Ugly(t *testing.T) {
	err := mcpErr("synthetic failure")
	if err == nil || !core.Contains(err.Error(), "synthetic failure") {
		t.Fatalf("expected mcpErr to wrap the message, got: %v", err)
	}
}

// newTestMailAdjacentService is a tiny local helper (avoids importing
// the marketplace_test package's newTestMarketplaceService, which
// lives in the external test package and isn't visible here).
func newTestMailAdjacentService(t *testing.T) (*Service, *core.Core) {
	t.Helper()
	c := core.New()
	return NewService(c), c
}

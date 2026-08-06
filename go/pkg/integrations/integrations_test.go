// SPDX-Licence-Identifier: EUPL-1.2

// Real tests for integrations.go's WailsService (List / Register /
// lifecycle). The pre-existing integrations_example_test.go only
// takes method VALUES via reflection (core.Sprintf("%T", ref)) and
// never invokes them — that's why this package sat at 0.0% coverage
// despite having "passing" tests. These tests drive List() against a
// hermetic $HOME (t.Setenv) so ~/.config/claude/config.json etc. are
// never touched on the real machine.

package integrations_test

import (
	core "dappco.re/go"
	subject "dappco.re/lthn/desktop/pkg/integrations"
)

func TestIntegrations_WailsService_List_Good_FreshHomeAllAvailable(t *core.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	list := subject.NewWailsService().List()
	core.AssertLen(t, list, 5)

	byID := map[string]subject.ClientStatus{}
	for _, entry := range list {
		byID[entry.ID] = entry
	}

	for _, id := range []string{"claude-code", "opencode", "codex", "copilot"} {
		entry, ok := byID[id]
		core.RequireTrue(t, ok, id+" missing from catalogue")
		core.AssertEqual(t, "available", entry.State)
		core.AssertFalse(t, entry.Exists)
		core.AssertTrue(t, core.HasPrefix(entry.ConfigPath, home))
	}

	pi, ok := byID["pi"]
	core.RequireTrue(t, ok)
	core.AssertEqual(t, "n/a", pi.State)
	core.AssertFalse(t, pi.Exists)
	core.AssertEqual(t, "", pi.ConfigPath)
}

func TestIntegrations_WailsService_List_Good_ConfiguredWhenFilePresent(t *core.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	claudeDir := core.PathJoin(home, ".config", "claude")
	core.RequireTrue(t, core.MkdirAll(claudeDir, 0o755).OK)
	core.RequireTrue(t, core.WriteFile(core.PathJoin(claudeDir, "config.json"), []byte("{}"), 0o644).OK)

	list := subject.NewWailsService().List()
	var claude subject.ClientStatus
	for _, entry := range list {
		if entry.ID == "claude-code" {
			claude = entry
		}
	}
	core.AssertEqual(t, "configured", claude.State)
	core.AssertTrue(t, claude.Exists)
	core.AssertEqual(t, core.PathJoin(home, ".config", "claude", "config.json"), claude.ConfigPath)

	// Untouched entries stay "available" — only claude-code flips.
	for _, entry := range list {
		if entry.ID == "opencode" {
			core.AssertEqual(t, "available", entry.State)
			core.AssertFalse(t, entry.Exists)
		}
	}
}

func TestIntegrations_WailsService_List_Bad_EmptyHomeFallsBackToRawPath(t *core.T) {
	t.Setenv("HOME", "")

	list := subject.NewWailsService().List()
	core.AssertLen(t, list, 5)
	for _, entry := range list {
		if entry.ID == "pi" {
			core.AssertEqual(t, "n/a", entry.State)
			continue
		}
		// homeR.OK is false, so ConfigPath falls back to the raw
		// "~/…" form untouched, and Stat on a literal "~/…" path
		// never resolves to a real file — State stays "available".
		core.AssertEqual(t, "available", entry.State)
		core.AssertFalse(t, entry.Exists)
	}
}

func TestIntegrations_WailsService_List_Ugly_StableOrderMatchesCatalogue(t *core.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	want := []string{"claude-code", "opencode", "codex", "copilot", "pi"}
	list := subject.NewWailsService().List()
	core.AssertLen(t, list, len(want))
	for i, id := range want {
		core.AssertEqual(t, id, list[i].ID)
	}
}

func TestIntegrations_WailsService_ServiceLifecycle_Good(t *core.T) {
	svc := subject.NewWailsService()
	core.AssertEqual(t, "Integrations", svc.ServiceName())

	r := svc.ServiceStartup(core.Background(), nil)
	core.AssertTrue(t, r.OK)

	r = svc.ServiceShutdown()
	core.AssertTrue(t, r.OK)
}

func TestIntegrations_Register_Good_ReturnsWailsService(t *core.T) {
	c := core.New()
	r := subject.Register(c)
	core.RequireTrue(t, r.OK)
	svc, ok := r.Value.(*subject.WailsService)
	core.RequireTrue(t, ok)
	core.AssertEqual(t, "Integrations", svc.ServiceName())
}

func TestIntegrations_Register_Bad_NilCoreIgnoredStillOK(t *core.T) {
	r := subject.Register(nil)
	core.AssertTrue(t, r.OK)
	core.AssertNotNil(t, r.Value)
}

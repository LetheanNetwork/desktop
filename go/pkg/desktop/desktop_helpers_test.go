// SPDX-Licence-Identifier: EUPL-1.2

// Tests for desktop.go's free-standing helpers that sit outside
// Service.Run() — the tray-status / self-machine refresh loops, the
// Fleet row builders, the hub secret resolver, and the tray-target
// routing helpers. None of these touch Wails; desktop_test.go already
// documents why Service.Run() itself stays behind its guard clauses
// only (it ends by calling the real Wails event loop).

package desktop

import (
	"testing/fstest"

	core "dappco.re/go"
	"dappco.re/go/crypt/keys"
	gui "dappco.re/go/render/display/webkit"
	guievents "dappco.re/go/render/display/webkit/pkg/events"
	guisystray "dappco.re/go/render/display/webkit/pkg/systray"
	"dappco.re/lthn/desktop/pkg/fleet"
	"dappco.re/lthn/desktop/pkg/keysvc"
	"dappco.re/lthn/desktop/pkg/server"
)

// --- pathBase ---------------------------------------------------------

func TestDesktop_PathBase_Bad_EmptyIsEmpty(t *core.T) {
	core.AssertEqual(t, "", pathBase(""))
}

func TestDesktop_PathBase_Good_StripsDirectory(t *core.T) {
	core.AssertEqual(t, "model.gguf", pathBase("/Users/snider/models/model.gguf"))
}

func TestDesktop_PathBase_Ugly_TrailingSlashesAreTrimmed(t *core.T) {
	core.AssertEqual(t, "models", pathBase("/Users/snider/models///"))
}

func TestDesktop_PathBase_Ugly_NoSeparatorReturnsWholeString(t *core.T) {
	core.AssertEqual(t, "model.gguf", pathBase("model.gguf"))
}

// --- selfMachineRow / mergeSelfMachineRow / localLemmaAgentRow --------

func TestDesktop_SelfMachineRow_Good_MarksIsSelfAndInferenceCapability(t *core.T) {
	row := selfMachineRow()

	core.AssertTrue(t, row.IsSelf)
	core.AssertEqual(t, "127.0.0.1", row.Host)
	core.AssertEqual(t, 9100, row.Port)
	core.AssertEqual(t, "online", row.Status)
	core.AssertTrue(t, core.Contains(row.ID, "self:"))
	found := false
	for _, c := range row.Capabilities {
		if c == fleet.CapabilityInference {
			found = true
		}
	}
	core.AssertTrue(t, found)
}

func fleetHomeFixture(t *core.T) *fleet.Service {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	r := fleet.New()
	core.RequireTrue(t, r.OK, r.Error())
	svc := r.Value.(*fleet.Service)
	t.Cleanup(func() { _ = svc.Close() })
	return svc
}

func TestDesktop_MergeSelfMachineRow_Bad_NilServiceReturnsDefault(t *core.T) {
	row := mergeSelfMachineRow(nil)
	core.AssertTrue(t, row.IsSelf)
}

func TestDesktop_MergeSelfMachineRow_Good_NoExistingRowReturnsDefault(t *core.T) {
	svc := fleetHomeFixture(t)
	row := mergeSelfMachineRow(svc)
	core.AssertTrue(t, row.IsSelf)
	core.AssertEqual(t, selfMachineRow().ID, row.ID)
}

func TestDesktop_MergeSelfMachineRow_Ugly_PreservesUserEditedFields(t *core.T) {
	svc := fleetHomeFixture(t)
	base := selfMachineRow()
	base.Name = "My Custom Name"
	base.Tags = "favourite,laptop"
	base.Capabilities = []string{"custom-capability"}
	core.RequireTrue(t, svc.UpsertMachine(base).OK)

	row := mergeSelfMachineRow(svc)

	core.AssertEqual(t, "My Custom Name", row.Name)
	core.AssertEqual(t, "favourite,laptop", row.Tags)
	core.AssertEqual(t, []string{"custom-capability"}, row.Capabilities)
}

func TestDesktop_LocalLemmaAgentRow_Good_NoExistingUsesDefaults(t *core.T) {
	agent := localLemmaAgentRow("/models/gemma.gguf", nil)

	core.AssertEqual(t, "local-lemma", agent.ID)
	core.AssertEqual(t, "Local Lemma", agent.Name)
	core.AssertEqual(t, "gemma.gguf", agent.Model)
	core.AssertEqual(t, "online", agent.Status)
}

func TestDesktop_LocalLemmaAgentRow_Ugly_PreservesExistingPersonaAndTags(t *core.T) {
	existing := &fleet.Agent{
		Name:    "My Lemma",
		Persona: "helpful",
		Tags:    "local,fast",
	}
	agent := localLemmaAgentRow("/models/gemma.gguf", existing)

	core.AssertEqual(t, "My Lemma", agent.Name)
	core.AssertEqual(t, "helpful", agent.Persona)
	core.AssertEqual(t, "local,fast", agent.Tags)
	core.AssertEqual(t, "gemma.gguf", agent.Model, "substrate-controlled field always overwrites")
}

func TestDesktop_LocalLemmaAgentRow_Ugly_ExistingEmptyNameKeepsDefault(t *core.T) {
	existing := &fleet.Agent{Name: ""}
	agent := localLemmaAgentRow("/models/gemma.gguf", existing)
	core.AssertEqual(t, "Local Lemma", agent.Name)
}

// --- refreshSelfMachineOnce / runSelfMachineRefresh --------------------

func TestDesktop_RefreshSelfMachineOnce_Bad_NilServiceIsNoop(t *core.T) {
	refreshSelfMachineOnce(nil)
}

// TestDesktop_RefreshSelfMachineOnce_Good_LemmaUnreachableMarksOffline
// drives the err != nil branch: under a temp HOME with no admin.token
// on disk, lemma.NewAdmin fails deterministically before any network
// call, so this stays hermetic. The admin.Status success branch needs
// a live lthn-mlx serve on the fixed loopback admin port (no override
// seam exists on lemma.AdminConfig{}); that branch is left untested —
// see the report's leave-outs.
func TestDesktop_RefreshSelfMachineOnce_Good_LemmaUnreachableMarksOffline(t *core.T) {
	t.Setenv("HOME", t.TempDir())
	svc := fleetHomeFixture(t)

	refreshSelfMachineOnce(svc)

	listRes := svc.Machines()
	core.RequireTrue(t, listRes.OK, listRes.Error())
	machines, ok := listRes.Value.([]fleet.Machine)
	core.RequireTrue(t, ok)
	var self *fleet.Machine
	for i := range machines {
		if machines[i].IsSelf {
			self = &machines[i]
		}
	}
	core.RequireTrue(t, self != nil, "self row must be upserted")
	core.AssertEqual(t, "offline", self.Status)
}

func TestDesktop_RunSelfMachineRefresh_Good_StopClosesImmediately(t *core.T) {
	svc := fleetHomeFixture(t)
	stop := make(chan struct{})
	close(stop)

	runSelfMachineRefresh(svc, stop)
}

// --- refreshTrayTooltipOnce / runTrayStatusRefresh ---------------------

func TestDesktop_RefreshTrayTooltipOnce_Bad_NilCoreIsNoop(t *core.T) {
	refreshTrayTooltipOnce(nil)
}

// TestDesktop_RefreshTrayTooltipOnce_Good_LemmaUnreachableSetsFallbackTooltip
// exercises the "err != nil" branch of the lemma admin lookup (see the
// refreshSelfMachineOnce note above for why the admin.Status success
// branch is out of reach hermetically) and asserts the fallback tooltip
// text still reaches the systray.set_tooltip action.
func TestDesktop_RefreshTrayTooltipOnce_Good_LemmaUnreachableSetsFallbackTooltip(t *core.T) {
	t.Setenv("HOME", t.TempDir())
	c := core.New()
	var tooltip string
	c.Action("systray.set_tooltip", func(_ core.Context, opts core.Options) core.Result {
		if task, ok := opts.Get("task").Value.(guisystray.TaskSetTrayTooltip); ok {
			tooltip = task.Tooltip
		}
		return core.Ok(nil)
	})

	refreshTrayTooltipOnce(c)

	core.AssertEqual(t, "Lethean Desktop — no model loaded", tooltip)
}

func TestDesktop_RunTrayStatusRefresh_Good_StopClosesImmediately(t *core.T) {
	t.Setenv("HOME", t.TempDir())
	c := core.New()
	stop := make(chan struct{})
	close(stop)

	runTrayStatusRefresh(c, stop)
}

// --- trayStatusTooltip (remaining branch) ------------------------------

func TestDesktop_TrayStatusTooltip_Good_EmptyPrefixWithModel(t *core.T) {
	core.AssertEqual(t, "gemma.gguf — ready", trayStatusTooltip("", "gemma.gguf"))
}

// --- hubSandboxEnv / hubMCPToken ----------------------------------------

func hubKeysFixture(t *core.T) *keys.Service {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	r := keysvc.New()
	core.RequireTrue(t, r.OK, r.Error())
	svc := r.Value.(*keys.Service)
	kek := make([]byte, 32)
	for i := range kek {
		kek[i] = 0x24
	}
	svc.SetKEKProviderTier0(func() ([]byte, bool) { return kek, true })
	return svc
}

func TestDesktop_HubSandboxEnv_Bad_NilCoreFallsBackToEphemeralValues(t *core.T) {
	env := hubSandboxEnv(nil)

	core.AssertEqual(t, 2, len(env))
	core.AssertTrue(t, core.HasPrefix(env[0], "MCP_JWT_SECRET="))
	core.AssertTrue(t, core.HasPrefix(env[1], "MCP_AUTH_TOKEN="))
}

func TestDesktop_HubSandboxEnv_Good_ResolvesAndPersistsFromKeysTier0(t *core.T) {
	svc := hubKeysFixture(t)
	c := core.New()
	core.RequireTrue(t, c.RegisterService("keys", svc).OK)

	first := hubSandboxEnv(c)
	second := hubSandboxEnv(c)

	core.AssertEqual(t, 2, len(first))
	core.AssertEqual(t, first, second, "tier-0 resolution must be stable across calls")
}

func TestDesktop_HubMCPToken_Good_FindsAuthToken(t *core.T) {
	token := hubMCPToken([]string{"MCP_JWT_SECRET=abc", "MCP_AUTH_TOKEN=xyz"})
	core.AssertEqual(t, "xyz", token)
}

func TestDesktop_HubMCPToken_Bad_MissingReturnsEmpty(t *core.T) {
	core.AssertEqual(t, "", hubMCPToken([]string{"MCP_JWT_SECRET=abc"}))
}

// --- restoreSecondInstanceWindow (OpenWindow-succeeds branch) ----------
//
// The fallback body (gui.WindowSpec ok==true, window.open re-dispatch)
// is unreachable in the current webkit.OpenWindow/WindowSpec pairing —
// both resolve the SAME registry lookup (webkit's lookupWindow is a
// thin alias over WindowSpec), so OpenWindow returning false implies
// WindowSpec also returns ok=false. That dead branch is an itemised
// leave-out, not a gap in this suite.

func restoreSecondInstanceWindowGUIFixture(t *core.T) *core.Core {
	t.Helper()
	c := core.New()
	guiResult := gui.NewService(gui.GuiConfig{WindowRegistry: windowRegistry()})(c)
	core.RequireTrue(t, guiResult.OK, guiResult.Error())
	core.RequireTrue(t, c.RegisterService("gui", guiResult.Value).OK)
	for _, name := range []string{"dock.show_icon", "window.restore", "window.set_visibility", "window.focus"} {
		actionName := name
		c.Action(actionName, func(_ core.Context, _ core.Options) core.Result { return core.Ok(nil) })
	}
	return c
}

func TestDesktop_RestoreSecondInstanceWindow_Bad_NilCoreIsNoop(t *core.T) {
	restoreSecondInstanceWindow(nil)
}

func TestDesktop_RestoreSecondInstanceWindow_Good_RegisteredWindowReturnsEarly(t *core.T) {
	c := restoreSecondInstanceWindowGUIFixture(t)
	restoreSecondInstanceWindow(c)
}

func TestDesktop_RestoreSecondInstanceWindow_Ugly_UnregisteredWindowEmitsFallbackOnly(t *core.T) {
	c := core.New()
	restoreSecondInstanceWindow(c)
}

// --- openTrayTarget / validTrayTarget ------------------------------------

func TestDesktop_ValidTrayTarget_Good_KnownTargets(t *core.T) {
	for _, target := range []string{"desktop", "chat", "models", "settings", "telemetry", "tools"} {
		core.AssertTrue(t, validTrayTarget(target), target)
	}
}

func TestDesktop_ValidTrayTarget_Bad_UnknownTarget(t *core.T) {
	core.AssertFalse(t, validTrayTarget("not-a-real-target"))
}

func TestDesktop_ValidTrayTarget_Ugly_PluginPrefix(t *core.T) {
	core.AssertTrue(t, validTrayTarget("plugin:opencode"))
	core.AssertFalse(t, validTrayTarget("plugin:"))
	core.AssertFalse(t, validTrayTarget("plugin:../escape"))
}

func TestDesktop_OpenTrayTarget_Bad_InvalidTargetFails(t *core.T) {
	c := core.New()
	result := openTrayTarget(c, "not-a-real-target")
	core.AssertFalse(t, result.OK)
	core.AssertContains(t, result.Error(), "invalid tray target")
}

func TestDesktop_OpenTrayTarget_Ugly_WindowUnavailableFails(t *core.T) {
	c := core.New()
	result := openTrayTarget(c, "chat")
	core.AssertFalse(t, result.OK)
	core.AssertContains(t, result.Error(), "desktop window is unavailable")
}

func TestDesktop_OpenTrayTarget_Good_OpensAndEmits(t *core.T) {
	c := restoreSecondInstanceWindowGUIFixture(t)
	var emittedEvent string
	var emittedTarget string
	c.Action("events.emit", func(_ core.Context, opts core.Options) core.Result {
		if task, ok := opts.Get("task").Value.(guievents.TaskEmit); ok {
			emittedEvent = task.Name
			emittedTarget, _ = task.Data.(string)
		}
		return core.Ok(nil)
	})
	c.Action("window.hide", func(_ core.Context, _ core.Options) core.Result { return core.Ok(nil) })

	result := openTrayTarget(c, "chat")

	core.RequireTrue(t, result.OK, result.Error())
	core.AssertEqual(t, trayOpenEvent, emittedEvent)
	core.AssertEqual(t, "chat", emittedTarget)
}

// --- attachSPA (error branch) --------------------------------------------

// fs.Sub only rejects a syntactically invalid path (e.g. one that
// escapes the root); a merely-absent subdirectory is still a valid
// path that lazily 404s on Open, so the invalid-path case is what
// actually drives attachSPA's "frontend root not found" branch.
func TestDesktop_AttachSPA_Bad_InvalidFrontendRootFails(t *core.T) {
	backend := server.NewService(server.Options{})
	desktop := NewService(Options{
		Frontend:     fstest.MapFS{"dist/index.html": &fstest.MapFile{Data: []byte("x")}},
		FrontendRoot: "../escape",
		Server:       backend,
	})

	result := desktop.attachSPA()

	core.AssertFalse(t, result.OK)
	core.AssertContains(t, result.Error(), "frontend root not found")
}

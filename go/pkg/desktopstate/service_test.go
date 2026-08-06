// SPDX-License-Identifier: EUPL-1.2

package desktopstate

import (
	core "dappco.re/go"
	coreio "dappco.re/go/io"
)

func TestService_ShellSession_GoodRoundTrip(t *core.T) {
	medium := coreio.NewMemoryMedium()
	service := NewService(Options{Medium: medium})

	saved := service.SaveShellSession(SaveShellSessionInput{
		ExpectedRevision: 0,
		Session:          validShellSession(),
	})
	loaded := service.LoadShellSession()

	core.RequireTrue(t, saved.OK, saved.Error())
	core.RequireTrue(t, loaded.OK, loaded.Error())
	snapshot := loaded.Value.(ShellSessionSnapshot)
	core.AssertEqual(t, uint64(1), snapshot.Revision)
	core.AssertEqual(t, validShellSession(), snapshot.Session)
	core.AssertTrue(t, medium.IsFile(shellSessionPath))
}

func TestService_TerminalWorkspace_GoodRoundTrip(t *core.T) {
	medium := coreio.NewMemoryMedium()
	service := NewService(Options{Medium: medium})

	saved := service.SaveTerminalWorkspace(SaveTerminalWorkspaceInput{
		ExpectedRevision: 0,
		Workspace:        validTerminalWorkspace(),
	})
	loaded := service.LoadTerminalWorkspace()

	core.RequireTrue(t, saved.OK, saved.Error())
	core.RequireTrue(t, loaded.OK, loaded.Error())
	snapshot := loaded.Value.(TerminalWorkspaceSnapshot)
	core.AssertEqual(t, uint64(1), snapshot.Revision)
	core.AssertEqual(t, validTerminalWorkspace(), snapshot.Workspace)
	core.AssertTrue(t, medium.IsFile(terminalWorkspacePath))
}

func TestService_Load_GoodReturnsSafeDefaultsWhenDocumentsAreMissing(t *core.T) {
	service := NewService(Options{Medium: coreio.NewMemoryMedium()})

	shell := service.LoadShellSession()
	terminal := service.LoadTerminalWorkspace()

	core.RequireTrue(t, shell.OK, shell.Error())
	core.RequireTrue(t, terminal.OK, terminal.Error())
	shellSnapshot := shell.Value.(ShellSessionSnapshot)
	core.AssertEqual(t, uint64(0), shellSnapshot.Revision)
	core.AssertEqual(t, ViewDesktop, shellSnapshot.Session.View)
	core.AssertEqual(t, DeviceFull, shellSnapshot.Session.Device)
	terminalSnapshot := terminal.Value.(TerminalWorkspaceSnapshot)
	core.AssertEqual(t, uint64(0), terminalSnapshot.Revision)
	core.AssertEqual(t, 0, len(terminalSnapshot.Workspace.Tabs))
}

func TestService_Save_BadRejectsInvalidPayloadWithoutWriting(t *core.T) {
	medium := coreio.NewMemoryMedium()
	service := NewService(Options{Medium: medium})
	invalid := validShellSession()
	invalid.View = "invalid"

	result := service.SaveShellSession(SaveShellSessionInput{
		ExpectedRevision: 0,
		Session:          invalid,
	})

	core.AssertFalse(t, result.OK)
	core.AssertEqual(t, ErrorStateInvalid, ErrorCodeOf(result))
	core.AssertFalse(t, medium.IsFile(shellSessionPath))
}

func TestService_Load_UglyRejectsExecutionShapedUnknownFields(t *core.T) {
	medium := coreio.NewMemoryMedium()
	core.RequireNoError(t, medium.EnsureDir("desktop/state"))
	core.RequireNoError(t, medium.Write(
		terminalWorkspacePath,
		`{
		  "version": 1,
		  "revision": 1,
		  "updatedAt": "2026-07-27T12:00:00Z",
		  "payload": {
		    "activeKey": "terminal-one",
		    "tabs": [{
		      "key": "terminal-one",
		      "title": "unsafe",
		      "kind": "shell",
		      "command": ["/bin/sh"],
		      "environment": ["TOKEN=secret"]
		    }]
		  }
		}`,
	))

	result := NewService(Options{Medium: medium}).LoadTerminalWorkspace()

	core.AssertFalse(t, result.OK)
	core.AssertEqual(t, ErrorStateInvalid, ErrorCodeOf(result))
	content, err := medium.Read(terminalWorkspacePath)
	core.RequireNoError(t, err)
	core.AssertContains(t, content, `"command"`)
}

func TestService_Register_GoodUsesNamedApplicationMedium(t *core.T) {
	medium := coreio.NewMemoryMedium()
	c := core.New(
		core.WithName("io", func(c *core.Core) core.Result {
			return core.Ok(&coreio.Service{
				ServiceRuntime: core.NewServiceRuntime(c, coreio.IOConfig{}),
				Medium:         medium,
			})
		}),
		core.WithName("desktopstate", Register),
	)

	service, ok := core.ServiceFor[*Service](c, "desktopstate")

	core.AssertTrue(t, ok)
	core.AssertNotNil(t, service)
	core.RequireTrue(t, service.SaveShellSession(SaveShellSessionInput{
		ExpectedRevision: 0,
		Session:          validShellSession(),
	}).OK)
	core.AssertTrue(t, medium.IsFile(shellSessionPath))
}

func TestService_Register_BadFailsClosedWithoutNamedMedium(t *core.T) {
	result := Register(core.New())

	core.AssertFalse(t, result.OK)
	core.AssertEqual(t, ErrorStateUnavailable, ErrorCodeOf(result))
}

func TestService_Register_BadNilCoreFailsClosed(t *core.T) {
	result := Register(nil)

	core.AssertFalse(t, result.OK)
	core.AssertEqual(t, ErrorStateUnavailable, ErrorCodeOf(result))
}

func TestService_Register_BadNilReceiverOrMissingInputsFailClosed(
	t *core.T,
) {
	var nilService *Service
	nilResult := nilService.Register(core.New())
	core.AssertFalse(t, nilResult.OK)
	core.AssertEqual(t, ErrorStateUnavailable, ErrorCodeOf(nilResult))

	nilCoreResult := (&Service{
		options: Options{Medium: coreio.NewMemoryMedium()},
	}).Register(nil)
	core.AssertFalse(t, nilCoreResult.OK)
	core.AssertEqual(t, ErrorStateUnavailable, ErrorCodeOf(nilCoreResult))

	noMediumResult := (&Service{}).Register(core.New())
	core.AssertFalse(t, noMediumResult.OK)
	core.AssertEqual(t, ErrorStateUnavailable, ErrorCodeOf(noMediumResult))
}

func TestService_LoadSave_BadUnavailableServiceFailsClosed(t *core.T) {
	var nilService *Service
	zeroed := &Service{}

	for name, result := range map[string]core.Result{
		"nil LoadShellSession":       nilService.LoadShellSession(),
		"nil SaveShellSession":       nilService.SaveShellSession(SaveShellSessionInput{}),
		"nil LoadTerminalWorkspace":  nilService.LoadTerminalWorkspace(),
		"nil SaveTerminalWorkspace":  nilService.SaveTerminalWorkspace(SaveTerminalWorkspaceInput{}),
		"zero LoadShellSession":      zeroed.LoadShellSession(),
		"zero SaveShellSession":      zeroed.SaveShellSession(SaveShellSessionInput{}),
		"zero LoadTerminalWorkspace": zeroed.LoadTerminalWorkspace(),
		"zero SaveTerminalWorkspace": zeroed.SaveTerminalWorkspace(SaveTerminalWorkspaceInput{}),
	} {
		t.Run(name, func(t *core.T) {
			core.AssertFalse(t, result.OK)
			core.AssertEqual(t, ErrorStateUnavailable, ErrorCodeOf(result))
		})
	}
}

func TestWailsService_GoodDelegatesOnlyTypedStateOperations(t *core.T) {
	service := NewService(Options{Medium: coreio.NewMemoryMedium()})
	wails := NewWailsService(service)

	saved := wails.SaveShellSession(SaveShellSessionInput{
		ExpectedRevision: 0,
		Session:          validShellSession(),
	})
	loaded := wails.LoadShellSession()

	core.RequireTrue(t, saved.OK, saved.Error())
	core.RequireTrue(t, loaded.OK, loaded.Error())
	core.AssertEqual(
		t,
		validShellSession(),
		loaded.Value.(ShellSessionSnapshot).Session,
	)
}

// SPDX-License-Identifier: EUPL-1.2

package desktopstate

import (
	core "dappco.re/go"
	coreio "dappco.re/go/io"
)

func TestWailsService_TerminalWorkspace_GoodDelegates(t *core.T) {
	service := NewService(Options{Medium: coreio.NewMemoryMedium()})
	wails := NewWailsService(service)

	saved := wails.SaveTerminalWorkspace(SaveTerminalWorkspaceInput{
		ExpectedRevision: 0,
		Workspace:        validTerminalWorkspace(),
	})
	loaded := wails.LoadTerminalWorkspace()

	core.RequireTrue(t, saved.OK, saved.Error())
	core.RequireTrue(t, loaded.OK, loaded.Error())
	core.AssertEqual(
		t,
		validTerminalWorkspace(),
		loaded.Value.(TerminalWorkspaceSnapshot).Workspace,
	)
}

func TestWailsService_Bad_NilReceiverFailsClosed(t *core.T) {
	var nilWails *WailsService

	loadShell := nilWails.LoadShellSession()
	saveShell := nilWails.SaveShellSession(SaveShellSessionInput{})
	loadTerminal := nilWails.LoadTerminalWorkspace()
	saveTerminal := nilWails.SaveTerminalWorkspace(
		SaveTerminalWorkspaceInput{},
	)

	for _, result := range []core.Result{
		loadShell,
		saveShell,
		loadTerminal,
		saveTerminal,
	} {
		core.AssertFalse(t, result.OK)
		core.AssertEqual(t, ErrorStateUnavailable, ErrorCodeOf(result))
	}
}

func TestWailsService_Bad_UnwrappedNilServiceFailsClosed(t *core.T) {
	wails := NewWailsService(nil)

	loadShell := wails.LoadShellSession()
	saveShell := wails.SaveShellSession(SaveShellSessionInput{})
	loadTerminal := wails.LoadTerminalWorkspace()
	saveTerminal := wails.SaveTerminalWorkspace(
		SaveTerminalWorkspaceInput{},
	)

	for _, result := range []core.Result{
		loadShell,
		saveShell,
		loadTerminal,
		saveTerminal,
	} {
		core.AssertFalse(t, result.OK)
		core.AssertEqual(t, ErrorStateUnavailable, ErrorCodeOf(result))
	}
}

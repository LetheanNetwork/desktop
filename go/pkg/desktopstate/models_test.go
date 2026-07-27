// SPDX-License-Identifier: EUPL-1.2

package desktopstate

import core "dappco.re/go"

func validShellSession() ShellSession {
	return ShellSession{
		View:    ViewDesktop,
		Device:  DeviceFull,
		FocusID: "window-control",
		Z:       4,
		Windows: []Window{{
			ID:        "window-control",
			App:       "control",
			Sub:       "models",
			SystemTab: "list",
			X:         48,
			Y:         36,
			Width:     820,
			Height:    600,
			Z:         4,
		}},
	}
}

func validTerminalWorkspace() TerminalWorkspace {
	return TerminalWorkspace{
		ActiveKey: "terminal-one",
		Tabs: []TerminalTab{{
			Key:   "terminal-one",
			Title: "desktop",
			Kind:  TerminalKindShell,
			Workspace: WorkspaceRef{
				Repository: "desktop",
			},
		}},
	}
}

func TestValidateShellSession_GoodAcceptsBoundedCatalogueState(t *core.T) {
	result := ValidateShellSession(validShellSession())

	core.RequireTrue(t, result.OK, result.Error())
	core.AssertEqual(t, validShellSession(), result.Value.(ShellSession))
}

func TestValidateShellSession_BadRejectsUnknownModesAndDuplicateWindows(t *core.T) {
	unknownView := validShellSession()
	unknownView.View = "cinema"
	core.AssertFalse(t, ValidateShellSession(unknownView).OK)

	unknownDevice := validShellSession()
	unknownDevice.Device = "wall"
	core.AssertFalse(t, ValidateShellSession(unknownDevice).OK)

	duplicate := validShellSession()
	duplicate.Windows = append(duplicate.Windows, duplicate.Windows[0])
	core.AssertFalse(t, ValidateShellSession(duplicate).OK)
}

func TestValidateShellSession_UglyRejectsUnboundedOrUnreachableGeometry(t *core.T) {
	tooMany := validShellSession()
	for len(tooMany.Windows) <= MaximumWindows {
		window := tooMany.Windows[0]
		window.ID = core.Concat("window-", core.JSONMarshalString(len(tooMany.Windows)))
		tooMany.Windows = append(tooMany.Windows, window)
	}
	core.AssertFalse(t, ValidateShellSession(tooMany).OK)

	badGeometry := validShellSession()
	badGeometry.Windows[0].Width = 0
	core.AssertFalse(t, ValidateShellSession(badGeometry).OK)

	badID := validShellSession()
	badID.Windows[0].ID = "../../escape"
	core.AssertFalse(t, ValidateShellSession(badID).OK)
}

func TestValidateTerminalWorkspace_GoodAcceptsProviderRelativeIntent(t *core.T) {
	workspace := validTerminalWorkspace()
	workspace.Tabs = append(workspace.Tabs, TerminalTab{
		Key:   "terminal-two",
		Title: "Documents",
		Kind:  TerminalKindShell,
		Workspace: WorkspaceRef{
			MountID: "documents",
			Path:    "projects/lethean",
		},
	})

	result := ValidateTerminalWorkspace(workspace)

	core.RequireTrue(t, result.OK, result.Error())
	core.AssertEqual(t, workspace, result.Value.(TerminalWorkspace))
}

func TestValidateTerminalWorkspace_BadRejectsPathsAndDuplicateKeys(t *core.T) {
	for _, path := range []string{
		"/Users/person/private",
		`C:\Users\person\private`,
		"../private",
		"project/../../private",
	} {
		workspace := validTerminalWorkspace()
		workspace.Tabs[0].Workspace = WorkspaceRef{
			MountID: "documents",
			Path:    path,
		}
		core.AssertFalse(t, ValidateTerminalWorkspace(workspace).OK)
	}

	duplicate := validTerminalWorkspace()
	duplicate.Tabs = append(duplicate.Tabs, duplicate.Tabs[0])
	core.AssertFalse(t, ValidateTerminalWorkspace(duplicate).OK)
}

func TestValidateTerminalWorkspace_UglyRejectsAuthorityConfusion(t *core.T) {
	bothAuthorities := validTerminalWorkspace()
	bothAuthorities.Tabs[0].Workspace = WorkspaceRef{
		MountID:    "documents",
		Repository: "desktop",
	}
	core.AssertFalse(t, ValidateTerminalWorkspace(bothAuthorities).OK)

	agentWithoutID := validTerminalWorkspace()
	agentWithoutID.Tabs[0].Kind = TerminalKindAgent
	core.AssertFalse(t, ValidateTerminalWorkspace(agentWithoutID).OK)

	shellWithAgentID := validTerminalWorkspace()
	shellWithAgentID.Tabs[0].SharedAgentID = "agent-one"
	core.AssertFalse(t, ValidateTerminalWorkspace(shellWithAgentID).OK)

	missingActive := validTerminalWorkspace()
	missingActive.ActiveKey = "terminal-missing"
	core.AssertFalse(t, ValidateTerminalWorkspace(missingActive).OK)
}

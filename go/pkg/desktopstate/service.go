// SPDX-License-Identifier: EUPL-1.2

// Package desktopstate persists bounded renderer-independent desktop state
// through the application io.Medium.
package desktopstate

import (
	core "dappco.re/go"
	coreio "dappco.re/go/io"
)

const (
	shellSessionPath      = "desktop/state/shell-session.json"
	terminalWorkspacePath = "desktop/state/terminal-workspace.json"
	stateDocumentVersion  = 1
	maximumShellBytes     = 512 * 1024
	maximumTerminalBytes  = 128 * 1024
)

// Options configures the desktop-state persistence boundary.
type Options struct {
	Medium coreio.Medium
}

// Service owns the typed shell-session and Terminal-workspace documents.
type Service struct {
	*core.ServiceRuntime[Options]

	options  Options
	shell    *document[ShellSession]
	terminal *document[TerminalWorkspace]
}

// ShellSessionSnapshot is the public versioned shell document.
type ShellSessionSnapshot struct {
	Version   int          `json:"version"`
	Revision  uint64       `json:"revision"`
	UpdatedAt string       `json:"updatedAt"`
	Session   ShellSession `json:"session"`
}

// TerminalWorkspaceSnapshot is the public versioned Terminal document.
type TerminalWorkspaceSnapshot struct {
	Version   int               `json:"version"`
	Revision  uint64            `json:"revision"`
	UpdatedAt string            `json:"updatedAt"`
	Workspace TerminalWorkspace `json:"workspace"`
}

// SaveShellSessionInput saves one expected shell revision.
type SaveShellSessionInput struct {
	ExpectedRevision uint64       `json:"expectedRevision"`
	Session          ShellSession `json:"session"`
}

// SaveTerminalWorkspaceInput saves one expected Terminal revision.
type SaveTerminalWorkspaceInput struct {
	ExpectedRevision uint64            `json:"expectedRevision"`
	Workspace        TerminalWorkspace `json:"workspace"`
}

// NewService constructs the typed state service.
//
// Usage example:
//
//	service := desktopstate.NewService(desktopstate.Options{Medium: medium})
func NewService(options Options) *Service {
	return &Service{
		options: options,
		shell: newDocument(
			options.Medium,
			shellSessionPath,
			documentOptions[ShellSession]{
				Version:      stateDocumentVersion,
				MaximumBytes: maximumShellBytes,
				Validate:     ValidateShellSession,
			},
		),
		terminal: newDocument(
			options.Medium,
			terminalWorkspacePath,
			documentOptions[TerminalWorkspace]{
				Version:      stateDocumentVersion,
				MaximumBytes: maximumTerminalBytes,
				Validate:     ValidateTerminalWorkspace,
			},
		),
	}
}

// Register resolves the registered application Medium.
//
// Usage example:
//
//	core.WithName("desktopstate", desktopstate.Register)
func Register(c *core.Core) core.Result {
	if c == nil {
		return stateFailure(
			ErrorStateUnavailable,
			"desktopstate.Register",
			"Core is unavailable",
			nil,
		)
	}
	ioService, ok := core.ServiceFor[*coreio.Service](c, "io")
	if !ok || ioService == nil || ioService.Medium == nil {
		return stateFailure(
			ErrorStateUnavailable,
			"desktopstate.Register",
			"registered I/O Medium is unavailable",
			nil,
		)
	}
	return NewService(Options{Medium: ioService.Medium}).Register(c)
}

// Register attaches this instance to Core and returns it.
func (service *Service) Register(c *core.Core) core.Result {
	if service == nil || c == nil || service.options.Medium == nil {
		return stateFailure(
			ErrorStateUnavailable,
			"desktopstate.Service.Register",
			"desktop state registration is unavailable",
			nil,
		)
	}
	service.ServiceRuntime = core.NewServiceRuntime(c, service.options)
	return core.Ok(service)
}

// LoadShellSession loads the durable inner desktop or a safe empty default.
func (service *Service) LoadShellSession() core.Result {
	if service == nil || service.shell == nil {
		return unavailableService("LoadShellSession")
	}
	loaded := service.shell.Load()
	if !loaded.OK {
		return loaded
	}
	envelope := loaded.Value.(documentEnvelope[ShellSession])
	session := envelope.Payload
	if envelope.Revision == 0 {
		session = defaultShellSession()
	}
	return core.Ok(ShellSessionSnapshot{
		Version:   envelope.Version,
		Revision:  envelope.Revision,
		UpdatedAt: envelope.UpdatedAt,
		Session:   cloneShellSession(session),
	})
}

// SaveShellSession validates and commits one inner desktop revision.
func (service *Service) SaveShellSession(
	input SaveShellSessionInput,
) core.Result {
	if service == nil || service.shell == nil {
		return unavailableService("SaveShellSession")
	}
	saved := service.shell.Save(input.ExpectedRevision, input.Session)
	if !saved.OK {
		return saved
	}
	envelope := saved.Value.(documentEnvelope[ShellSession])
	return core.Ok(ShellSessionSnapshot{
		Version:   envelope.Version,
		Revision:  envelope.Revision,
		UpdatedAt: envelope.UpdatedAt,
		Session:   cloneShellSession(envelope.Payload),
	})
}

// LoadTerminalWorkspace loads the tab workspace or a safe empty default.
func (service *Service) LoadTerminalWorkspace() core.Result {
	if service == nil || service.terminal == nil {
		return unavailableService("LoadTerminalWorkspace")
	}
	loaded := service.terminal.Load()
	if !loaded.OK {
		return loaded
	}
	envelope := loaded.Value.(documentEnvelope[TerminalWorkspace])
	workspace := envelope.Payload
	if envelope.Revision == 0 {
		workspace = TerminalWorkspace{Tabs: []TerminalTab{}}
	}
	return core.Ok(TerminalWorkspaceSnapshot{
		Version:   envelope.Version,
		Revision:  envelope.Revision,
		UpdatedAt: envelope.UpdatedAt,
		Workspace: cloneTerminalWorkspace(workspace),
	})
}

// SaveTerminalWorkspace validates and commits one Terminal revision.
func (service *Service) SaveTerminalWorkspace(
	input SaveTerminalWorkspaceInput,
) core.Result {
	if service == nil || service.terminal == nil {
		return unavailableService("SaveTerminalWorkspace")
	}
	saved := service.terminal.Save(input.ExpectedRevision, input.Workspace)
	if !saved.OK {
		return saved
	}
	envelope := saved.Value.(documentEnvelope[TerminalWorkspace])
	return core.Ok(TerminalWorkspaceSnapshot{
		Version:   envelope.Version,
		Revision:  envelope.Revision,
		UpdatedAt: envelope.UpdatedAt,
		Workspace: cloneTerminalWorkspace(envelope.Payload),
	})
}

func defaultShellSession() ShellSession {
	return ShellSession{
		View:    ViewDesktop,
		Device:  DeviceFull,
		Windows: []Window{},
	}
}

func unavailableService(operation string) core.Result {
	return stateFailure(
		ErrorStateUnavailable,
		core.Concat("desktopstate.Service.", operation),
		"desktop state service is unavailable",
		nil,
	)
}

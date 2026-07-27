// SPDX-License-Identifier: EUPL-1.2

package desktopstate

import (
	"regexp"
	"unicode/utf8"

	core "dappco.re/go"
)

const (
	// ViewDesktop renders the browser-desktop shell.
	ViewDesktop = "desktop"
	// ViewShell renders the ordinary application shell.
	ViewShell = "shell"
	// ViewDevice renders the device-preview shell.
	ViewDevice = "device"

	// DeviceSmall selects the small-device preview.
	DeviceSmall = "small"
	// DeviceLarge selects the large-device preview.
	DeviceLarge = "large"
	// DeviceFull selects the unrestricted desktop viewport.
	DeviceFull = "full"

	// TerminalKindShell identifies a user-created ordinary shell tab.
	TerminalKindShell = "shell"
	// TerminalKindAgent identifies a trusted shared-agent terminal tab.
	TerminalKindAgent = "agent"

	// MaximumWindows bounds one restored browser-desktop session.
	MaximumWindows = 128
	// MaximumTerminalTabs bounds one restored Terminal workspace.
	MaximumTerminalTabs = 32
)

const (
	maximumIdentifierBytes = 128
	maximumTitleBytes      = 256
	maximumRouteBytes      = 512
	maximumCoordinate      = 100_000
	maximumDimension       = 20_000
	maximumZ               = 1_000_000
)

var rendererIdentifierPattern = regexp.MustCompile(
	`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`,
)

// Window is the bounded renderer-independent state of one inner desktop
// window. App and route values are revalidated against Angular's catalogue.
type Window struct {
	ID        string `json:"id"`
	App       string `json:"app"`
	Sub       string `json:"sub,omitempty"`
	SystemTab string `json:"systemTab,omitempty"`
	Group     string `json:"group,omitempty"`
	X         int    `json:"x"`
	Y         int    `json:"y"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
	Z         int    `json:"z"`
	Min       bool   `json:"min"`
	Max       bool   `json:"max"`
}

// ShellSession is the durable inner desktop state. It never contains native
// outer-window state, provider authority, or execution data.
type ShellSession struct {
	View                 string   `json:"view"`
	Device               string   `json:"device"`
	FocusID              string   `json:"focusId,omitempty"`
	Z                    int      `json:"z"`
	Windows              []Window `json:"windows"`
	MigratedBrowserState bool     `json:"migratedBrowserState"`
}

// WorkspaceRef is a provider-relative terminal location intent. At most one
// of MountID and Repository may be set.
type WorkspaceRef struct {
	MountID    string `json:"mountId,omitempty"`
	Path       string `json:"path,omitempty"`
	Repository string `json:"repository,omitempty"`
}

// TerminalTab is durable tab presentation and workspace intent. It does not
// contain PTY identity, commands, environment, input, or output.
type TerminalTab struct {
	Key           string       `json:"key"`
	Title         string       `json:"title"`
	Kind          string       `json:"kind"`
	Workspace     WorkspaceRef `json:"workspace"`
	SharedAgentID string       `json:"sharedAgentId,omitempty"`
}

// TerminalWorkspace is the bounded durable Terminal tab strip.
type TerminalWorkspace struct {
	ActiveKey string        `json:"activeKey,omitempty"`
	Tabs      []TerminalTab `json:"tabs"`
}

// ValidateShellSession validates state before persistence or renderer use.
func ValidateShellSession(session ShellSession) core.Result {
	if !validView(session.View) || !validDevice(session.Device) {
		return invalidState("shell view or device mode is invalid")
	}
	if session.Z < 0 || session.Z > maximumZ {
		return invalidState("shell z-order is invalid")
	}
	if len(session.Windows) > MaximumWindows {
		return invalidState("shell window count exceeds its limit")
	}

	seen := make(map[string]bool, len(session.Windows))
	for _, window := range session.Windows {
		if !validIdentifier(window.ID) || !validIdentifier(window.App) {
			return invalidState("shell window identity is invalid")
		}
		if seen[window.ID] {
			return invalidState("shell window identity is duplicated")
		}
		seen[window.ID] = true
		if !validOptionalText(window.Sub, maximumRouteBytes) ||
			!validOptionalText(window.SystemTab, maximumRouteBytes) ||
			(window.Group != "" && !validIdentifier(window.Group)) {
			return invalidState("shell window route or group is invalid")
		}
		if window.X < -maximumCoordinate || window.X > maximumCoordinate ||
			window.Y < -maximumCoordinate || window.Y > maximumCoordinate ||
			window.Width < 120 || window.Width > maximumDimension ||
			window.Height < 80 || window.Height > maximumDimension ||
			window.Z < 0 || window.Z > maximumZ {
			return invalidState("shell window geometry is invalid")
		}
	}
	if session.FocusID != "" && !seen[session.FocusID] {
		return invalidState("focused shell window is unavailable")
	}
	return core.Ok(cloneShellSession(session))
}

// ValidateTerminalWorkspace validates a persisted Terminal workspace.
func ValidateTerminalWorkspace(workspace TerminalWorkspace) core.Result {
	if len(workspace.Tabs) > MaximumTerminalTabs {
		return invalidState("terminal tab count exceeds its limit")
	}
	seen := make(map[string]bool, len(workspace.Tabs))
	for _, tab := range workspace.Tabs {
		if !validIdentifier(tab.Key) || !validText(tab.Title, maximumTitleBytes) {
			return invalidState("terminal tab identity or title is invalid")
		}
		if seen[tab.Key] {
			return invalidState("terminal tab identity is duplicated")
		}
		seen[tab.Key] = true
		if tab.Kind != TerminalKindShell && tab.Kind != TerminalKindAgent {
			return invalidState("terminal tab kind is invalid")
		}
		if invalidWorkspaceRef(tab.Workspace) {
			return invalidState("terminal workspace reference is invalid")
		}
		if tab.Kind == TerminalKindAgent {
			if !validIdentifier(tab.SharedAgentID) {
				return invalidState("shared agent identity is invalid")
			}
		} else if tab.SharedAgentID != "" {
			return invalidState("ordinary shells cannot carry an agent identity")
		}
	}
	if workspace.ActiveKey != "" && !seen[workspace.ActiveKey] {
		return invalidState("active terminal tab is unavailable")
	}
	return core.Ok(cloneTerminalWorkspace(workspace))
}

func invalidWorkspaceRef(reference WorkspaceRef) bool {
	if reference.MountID != "" && reference.Repository != "" {
		return true
	}
	if reference.MountID != "" && !validIdentifier(reference.MountID) {
		return true
	}
	if reference.Repository != "" && !validIdentifier(reference.Repository) {
		return true
	}
	if reference.Path == "" {
		return false
	}
	if reference.MountID == "" || len(reference.Path) > maximumRouteBytes {
		return true
	}
	return !validProviderPath(reference.Path)
}

func validView(value string) bool {
	return value == ViewDesktop || value == ViewShell || value == ViewDevice
}

func validDevice(value string) bool {
	return value == DeviceSmall || value == DeviceLarge || value == DeviceFull
}

func validIdentifier(value string) bool {
	return len(value) <= maximumIdentifierBytes &&
		rendererIdentifierPattern.MatchString(value)
}

func validText(value string, maximum int) bool {
	if value == "" || len(value) > maximum || !utf8.ValidString(value) {
		return false
	}
	for _, character := range value {
		if character < 0x20 || character == 0x7f {
			return false
		}
	}
	return true
}

func validOptionalText(value string, maximum int) bool {
	return value == "" || validText(value, maximum)
}

func invalidState(message string) core.Result {
	return stateFailure(
		ErrorStateInvalid,
		"desktopstate.Validate",
		message,
		nil,
	)
}

func cloneShellSession(session ShellSession) ShellSession {
	cloned := session
	cloned.Windows = append([]Window(nil), session.Windows...)
	if cloned.Windows == nil {
		cloned.Windows = []Window{}
	}
	return cloned
}

func cloneTerminalWorkspace(workspace TerminalWorkspace) TerminalWorkspace {
	cloned := workspace
	cloned.Tabs = append([]TerminalTab(nil), workspace.Tabs...)
	if cloned.Tabs == nil {
		cloned.Tabs = []TerminalTab{}
	}
	return cloned
}

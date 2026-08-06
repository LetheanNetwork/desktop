// SPDX-License-Identifier: EUPL-1.2

// Benchmarks for the desktop-state save/serialise cycle (document.go +
// service.go + models.go). SaveShellSession and SaveTerminalWorkspace are
// wired straight to the WebView (wails.go) and fire on ordinary desktop
// interactions — window move/resize/focus, terminal tab open/close — so
// the whole document.Save round trip (validate → clone → JSON encode →
// stage → verify-by-redecode → atomic rename) runs on the interaction
// path, not just at shutdown. Fixtures use coreio.NewMemoryMedium() so
// the numbers isolate the package's own allocation shape from real disk
// I/O.
//
// Run:
//
//	go test ./pkg/desktopstate/... -run '^$' -bench . -benchmem -benchtime=20x

package desktopstate

import core "dappco.re/go"
import coreio "dappco.re/go/io"

// benchShellSessionWindows returns a valid ShellSession carrying n
// windows — representative of an in-use desktop, not the single-window
// fixture the correctness tests use. n must be <= MaximumWindows.
func benchShellSessionWindows(n int) ShellSession {
	windows := make([]Window, n)
	for i := 0; i < n; i++ {
		windows[i] = Window{
			ID:        core.Sprintf("window-%d", i),
			App:       "control",
			Sub:       "models",
			SystemTab: "list",
			X:         48 + i,
			Y:         36 + i,
			Width:     820,
			Height:    600,
			Z:         i,
		}
	}
	return ShellSession{
		View:    ViewDesktop,
		Device:  DeviceFull,
		FocusID: windows[n-1].ID,
		Z:       n,
		Windows: windows,
	}
}

// benchTerminalWorkspaceTabs returns a valid TerminalWorkspace carrying
// n tabs — representative of an in-use Terminal strip. n must be
// <= MaximumTerminalTabs.
func benchTerminalWorkspaceTabs(n int) TerminalWorkspace {
	tabs := make([]TerminalTab, n)
	for i := 0; i < n; i++ {
		tabs[i] = TerminalTab{
			Key:   core.Sprintf("terminal-%d", i),
			Title: "desktop",
			Kind:  TerminalKindShell,
			Workspace: WorkspaceRef{
				Repository: "desktop",
			},
		}
	}
	return TerminalWorkspace{
		ActiveKey: tabs[n-1].Key,
		Tabs:      tabs,
	}
}

// --- Save ---

func BenchmarkService_SaveShellSession_Typical(b *core.B) {
	medium := coreio.NewMemoryMedium()
	service := NewService(Options{Medium: medium})
	session := benchShellSessionWindows(8)

	b.ReportAllocs()
	b.ResetTimer()
	var revision uint64
	for i := 0; i < b.N; i++ {
		result := service.SaveShellSession(SaveShellSessionInput{
			ExpectedRevision: revision,
			Session:          session,
		})
		if !result.OK {
			b.Fatalf("SaveShellSession: %s", result.Error())
		}
		revision = result.Value.(ShellSessionSnapshot).Revision
	}
}

func BenchmarkService_SaveShellSession_Heavy(b *core.B) {
	medium := coreio.NewMemoryMedium()
	service := NewService(Options{Medium: medium})
	session := benchShellSessionWindows(MaximumWindows)

	b.ReportAllocs()
	b.ResetTimer()
	var revision uint64
	for i := 0; i < b.N; i++ {
		result := service.SaveShellSession(SaveShellSessionInput{
			ExpectedRevision: revision,
			Session:          session,
		})
		if !result.OK {
			b.Fatalf("SaveShellSession: %s", result.Error())
		}
		revision = result.Value.(ShellSessionSnapshot).Revision
	}
}

func BenchmarkService_SaveTerminalWorkspace_Typical(b *core.B) {
	medium := coreio.NewMemoryMedium()
	service := NewService(Options{Medium: medium})
	workspace := benchTerminalWorkspaceTabs(8)

	b.ReportAllocs()
	b.ResetTimer()
	var revision uint64
	for i := 0; i < b.N; i++ {
		result := service.SaveTerminalWorkspace(SaveTerminalWorkspaceInput{
			ExpectedRevision: revision,
			Workspace:        workspace,
		})
		if !result.OK {
			b.Fatalf("SaveTerminalWorkspace: %s", result.Error())
		}
		revision = result.Value.(TerminalWorkspaceSnapshot).Revision
	}
}

// --- Load ---

func BenchmarkService_LoadShellSession_Typical(b *core.B) {
	medium := coreio.NewMemoryMedium()
	service := NewService(Options{Medium: medium})
	saved := service.SaveShellSession(SaveShellSessionInput{
		ExpectedRevision: 0,
		Session:          benchShellSessionWindows(8),
	})
	if !saved.OK {
		b.Fatalf("fixture save: %s", saved.Error())
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result := service.LoadShellSession()
		if !result.OK {
			b.Fatalf("LoadShellSession: %s", result.Error())
		}
	}
}

func BenchmarkService_LoadTerminalWorkspace_Typical(b *core.B) {
	medium := coreio.NewMemoryMedium()
	service := NewService(Options{Medium: medium})
	saved := service.SaveTerminalWorkspace(SaveTerminalWorkspaceInput{
		ExpectedRevision: 0,
		Workspace:        benchTerminalWorkspaceTabs(8),
	})
	if !saved.OK {
		b.Fatalf("fixture save: %s", saved.Error())
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result := service.LoadTerminalWorkspace()
		if !result.OK {
			b.Fatalf("LoadTerminalWorkspace: %s", result.Error())
		}
	}
}

// --- Defensive clones (models.go) ---
//
// cloneShellSession / cloneTerminalWorkspace run at least twice per
// Save (once inside Validate, once building the returned Snapshot) and
// once per Load — isolate their own allocation cost from the JSON +
// Medium plumbing around them.

func BenchmarkCloneShellSession_Typical(b *core.B) {
	session := benchShellSessionWindows(8)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = cloneShellSession(session)
	}
}

func BenchmarkCloneTerminalWorkspace_Typical(b *core.B) {
	workspace := benchTerminalWorkspaceTabs(8)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = cloneTerminalWorkspace(workspace)
	}
}

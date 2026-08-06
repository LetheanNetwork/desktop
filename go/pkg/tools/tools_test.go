// SPDX-Licence-Identifier: EUPL-1.2

// Real behavioural tests for tools.go's Wails-bindable surface.
//
// tools_example_test.go's Test* functions previously paired 1:1 with
// its Example* functions and asserted only on a method VALUE's %T
// formatting — the bound-method-expression never gets called, so
// none of the target's statements ever executed. 87 tests were
// passing across pkg/git + pkg/lint + pkg/tools with 0.0% statement
// coverage on the strength of that mechanism; see wails_test.go
// (pkg/git) for the full writeup. This file replaces it with tests
// that construct a real *core.Core, register a real (in-memory,
// no-network) dappco.re/go/mcp Service under "mcp" — the same
// construction production wires via mcp.NewService — and drive
// WailsService against it.
package tools_test

import (
	core "dappco.re/go"
	mcpsvc "dappco.re/go/mcp/pkg/mcp"
	subject "dappco.re/lthn/desktop/pkg/tools"
)

// mcpCore returns a *core.Core with a real mcp.Service registered
// under "mcp" — in-memory only (no listener, no network), so its
// built-in tool catalogue (file ops etc.) populates deterministically
// without any fixture beyond a TempDir workspace root.
func mcpCore(t *core.T) *core.Core {
	t.Helper()
	c := core.New(core.WithName("mcp", mcpsvc.NewService(mcpsvc.Options{WorkspaceRoot: t.TempDir()})))
	_, ok := core.ServiceFor[*mcpsvc.Service](c, "mcp")
	core.RequireTrue(t, ok)
	return c
}

// ─── NewWailsService / Register ─────────────────────────────────────

func TestTools_NewWailsService_Good(t *core.T) {
	svc := subject.NewWailsService(core.New())
	core.AssertNotNil(t, svc)
	core.AssertEqual(t, "Tools", svc.ServiceName())
}

func TestTools_NewWailsService_Bad(t *core.T) {
	svc := subject.NewWailsService(nil)
	core.AssertNotNil(t, svc)
	core.AssertLen(t, svc.List(), 0) // nil core degrades to the empty-state slice, not a panic
}

func TestTools_NewWailsService_Ugly(t *core.T) {
	a := subject.NewWailsService(core.New())
	b := subject.NewWailsService(core.New())
	core.AssertTrue(t, a != b, "each call constructs a distinct instance")
}

func TestTools_Register_Good(t *core.T) {
	r := subject.Register(core.New())
	core.AssertTrue(t, r.OK)
	svc, ok := r.Value.(*subject.WailsService)
	core.AssertTrue(t, ok)
	core.AssertNotNil(t, svc)
}

func TestTools_Register_Bad(t *core.T) {
	r := subject.Register(nil)
	core.AssertTrue(t, r.OK)
	_, ok := r.Value.(*subject.WailsService)
	core.AssertTrue(t, ok)
}

func TestTools_Register_Ugly(t *core.T) {
	c := core.New()
	r1 := subject.Register(c)
	r2 := subject.Register(c)
	core.AssertTrue(t, r1.OK)
	core.AssertTrue(t, r2.OK)
	core.AssertTrue(t, r1.Value.(*subject.WailsService) != r2.Value.(*subject.WailsService))
}

// ─── ServiceName / ServiceStartup / ServiceShutdown ─────────────────

func TestTools_WailsService_ServiceName_Good(t *core.T) {
	svc := subject.NewWailsService(core.New())
	core.AssertEqual(t, "Tools", svc.ServiceName())
}

func TestTools_WailsService_ServiceStartup_Good(t *core.T) {
	svc := subject.NewWailsService(core.New())
	r := svc.ServiceStartup(core.Background(), nil)
	core.AssertTrue(t, r.OK)
}

func TestTools_WailsService_ServiceShutdown_Good(t *core.T) {
	svc := subject.NewWailsService(core.New())
	r := svc.ServiceShutdown()
	core.AssertTrue(t, r.OK)
}

// ─── List ────────────────────────────────────────────────────────────

// TestTools_WailsService_List_Good — a real mcp.Service registered
// under "mcp" populates its built-in tool catalogue; List() must
// mirror every record with a non-empty InputSchema string for at
// least one of them (structSchema reflects a real Go input struct).
func TestTools_WailsService_List_Good(t *core.T) {
	c := mcpCore(t)
	svc := subject.NewWailsService(c)

	got := svc.List()
	core.AssertGreater(t, len(got), 0)
	sawSchema := false
	for _, v := range got {
		core.AssertNotEmpty(t, v.Name)
		if v.InputSchema != "" {
			sawSchema = true
		}
	}
	core.AssertTrue(t, sawSchema, "at least one built-in tool carries a reflected input schema")
}

// TestTools_WailsService_List_Bad_NilReceiver — a nil *WailsService is
// a safe no-op, returning the empty-state slice rather than panicking.
func TestTools_WailsService_List_Bad_NilReceiver(t *core.T) {
	var svc *subject.WailsService
	got := svc.List()
	core.AssertNotNil(t, got)
	core.AssertLen(t, got, 0)
}

// TestTools_WailsService_List_Ugly_McpNotRegistered — a real Core
// with no "mcp" service registered degrades to the same empty state.
func TestTools_WailsService_List_Ugly_McpNotRegistered(t *core.T) {
	svc := subject.NewWailsService(core.New())
	got := svc.List()
	core.AssertNotNil(t, got)
	core.AssertLen(t, got, 0)
}

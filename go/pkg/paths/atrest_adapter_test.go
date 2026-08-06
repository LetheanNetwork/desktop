// SPDX-Licence-Identifier: EUPL-1.2

// Tests for paths.AtRestAdapter — the recordfile.AtomicWriter
// implementation that routes at-rest substrate I/O through
// AtomicWriteWithVersion / core.ReadFile / core.Remove (Cerberus #44
// PRBW F-2). Zero coverage before this file.

package paths_test

import (
	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/paths"
	"dappco.re/lthn/desktop/pkg/recordfile"
)

func TestAtRestAdapter_AtRestAdapter_Good_EmptyScopeDefaults(t *core.T) {
	home := homeFixture(t)
	adapter := paths.AtRestAdapter("")
	target := core.PathJoin(home, "at-rest-adapter-empty-scope.bin")

	err := adapter.Write(recordfile.AtomicWriteRequest{
		Path:       target,
		Payload:    []byte("hello"),
		IfNotExist: true,
	})
	core.AssertTrue(t, err == nil, "first write must succeed")

	// Trigger a read_failed with the default "atrest" scope prefix so
	// the empty-scope default is observable in the returned code.
	_, rerr := adapter.ReadFile(core.PathJoin(home, "never-written.bin"))
	core.AssertTrue(t, rerr != nil, "read of a missing file must fail")
	core.AssertContains(t, rerr.Error(), "atrest.atrest.read_failed",
		"empty scope must default to \"atrest\"")
}

func TestAtRestAdapter_Write_Good(t *core.T) {
	home := homeFixture(t)
	adapter := paths.AtRestAdapter("deals")
	target := core.PathJoin(home, "deal-001.lthn")

	err := adapter.Write(recordfile.AtomicWriteRequest{
		Path:       target,
		Payload:    []byte("ciphertext-bytes"),
		IfNotExist: true,
	})
	core.AssertTrue(t, err == nil, "write should succeed: %v")

	got, rerr := adapter.ReadFile(target)
	core.AssertTrue(t, rerr == nil, "read-back should succeed")
	core.AssertEqual(t, "ciphertext-bytes", string(got))
}

func TestAtRestAdapter_Write_Bad_IfNotExistViolated(t *core.T) {
	home := homeFixture(t)
	adapter := paths.AtRestAdapter("deals")
	target := core.PathJoin(home, "deal-001.lthn")

	err := adapter.Write(recordfile.AtomicWriteRequest{
		Path:       target,
		Payload:    []byte("v1"),
		IfNotExist: true,
	})
	core.AssertTrue(t, err == nil, "first write should succeed")

	// Second IfNotExist write against the same path must be refused —
	// exercises the atomic_write_failed error path.
	err = adapter.Write(recordfile.AtomicWriteRequest{
		Path:       target,
		Payload:    []byte("v2"),
		IfNotExist: true,
	})
	core.AssertTrue(t, err != nil, "second IfNotExist write must fail")
	core.AssertContains(t, err.Error(), "deals.atrest.atomic_write_failed", "expected scoped atomic_write_failed code")
}

func TestAtRestAdapter_ReadFile_Bad_MissingFile(t *core.T) {
	home := homeFixture(t)
	adapter := paths.AtRestAdapter("incidents")

	_, err := adapter.ReadFile(core.PathJoin(home, "no-such-file.lthn"))
	core.AssertTrue(t, err != nil, "reading a missing file must fail")
	core.AssertContains(t, err.Error(), "incidents.atrest.read_failed", "expected scoped read_failed code")
}

func TestAtRestAdapter_Remove_Good(t *core.T) {
	home := homeFixture(t)
	adapter := paths.AtRestAdapter("runbooks")
	target := core.PathJoin(home, "runbook-001.lthn")

	w := core.WriteFile(target, []byte("body"), 0o600)
	core.AssertTrue(t, w.OK, "fixture write must succeed")

	err := adapter.Remove(target)
	core.AssertTrue(t, err == nil, "remove of an existing file should succeed")
	core.AssertFalse(t, core.Stat(target).OK, "file must be gone after Remove")
}

func TestAtRestAdapter_Remove_Bad_MissingFile(t *core.T) {
	home := homeFixture(t)
	adapter := paths.AtRestAdapter("runbooks")

	err := adapter.Remove(core.PathJoin(home, "never-existed.lthn"))
	core.AssertTrue(t, err != nil, "removing a missing file must surface an error")
	core.AssertContains(t, err.Error(), "runbooks.atrest.remove_failed", "expected scoped remove_failed code")
}

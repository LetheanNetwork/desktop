// SPDX-Licence-Identifier: EUPL-1.2

//go:build !ios && !android

// sessions_test.go — end-to-end coverage for the `lthn sessions`
// verb surface. The dispatchers boot the composed app Core under an
// isolated HOME; the read/append happy paths obtain a real session id
// via pkg/sessions against the same HOME (the CLI prints the id to
// stdout, which these tests do not capture).

package main

import (
	"testing"

	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/sessions"
)

// TestSessions_CmdSessions_Bad — missing verb is pre-boot; unknown
// verb and per-verb usage errors surface after the boot.
func TestSessions_CmdSessions_Bad(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	core.AssertEqual(t, 2, cmdSessions(nil))
	core.AssertEqual(t, 2, cmdSessions([]string{"nonsense"}))
	core.AssertEqual(t, 2, cmdSessions([]string{"create"}))
	core.AssertEqual(t, 2, cmdSessions([]string{"read"}))
	core.AssertEqual(t, 2, cmdSessions([]string{"append", "id", "user"}))
}

// TestSessions_CmdSessions_Good — create + list through the CLI, then
// read + append against a real id created on the same HOME's store.
func TestSessions_CmdSessions_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	core.AssertEqual(t, 0, cmdSessions([]string{"create", "first chat"}))
	core.AssertEqual(t, 0, cmdSessions([]string{"list"}))

	c := newAppCore()
	core.RequireTrue(t, c != nil)
	t.Cleanup(func() { _ = c.ServiceShutdown(core.Background()) })
	r := sessions.Create(c, "second chat")
	core.RequireTrue(t, r.OK)
	id := r.Value.(string)

	core.AssertEqual(t, 0, cmdSessions([]string{"append", id, "user", "hello"}))
	core.AssertEqual(t, 0, cmdSessions([]string{"read", id}))
}

// TestSessions_SessionsRead_Ugly — an id that never existed surfaces
// the store failure as exit 1.
func TestSessions_SessionsRead_Ugly(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	core.AssertEqual(t, 1, cmdSessions([]string{"read", "no-such-session"}))
}

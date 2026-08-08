// SPDX-Licence-Identifier: EUPL-1.2

//go:build !ios && !android

// permissions_test.go — end-to-end coverage for the `lthn
// permissions` verb surface: config-backed set + list and the
// entitlement check view, on the composed app Core under an isolated
// HOME.

package main

import (
	"testing"

	core "dappco.re/go"
)

// TestPermissions_CmdPermissions_Bad — missing verb is pre-boot;
// unknown verb and per-verb usage errors surface after the boot.
func TestPermissions_CmdPermissions_Bad(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	core.AssertEqual(t, 2, cmdPermissions(nil))
	core.AssertEqual(t, 2, cmdPermissions([]string{"nonsense"}))
	core.AssertEqual(t, 2, cmdPermissions([]string{"check"}))
	core.AssertEqual(t, 2, cmdPermissions([]string{"set", "network.outbound"}))
}

// TestPermissions_CmdPermissions_Good — set persists through the
// config service, list reports the stored subtree, and check renders
// the entitlement view for the granted action.
func TestPermissions_CmdPermissions_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	core.AssertEqual(t, 0, cmdPermissions([]string{"set", "network.outbound", "true"}))
	core.AssertEqual(t, 0, cmdPermissions([]string{"list"}))
	core.AssertEqual(t, 0, cmdPermissions([]string{"check", "network.outbound"}))
}

// TestPermissions_PermissionsList_Ugly — nothing stored yet: list
// still emits JSON ({}), never an error.
func TestPermissions_PermissionsList_Ugly(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	core.AssertEqual(t, 0, cmdPermissions([]string{"list"}))
}

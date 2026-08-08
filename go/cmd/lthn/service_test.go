// SPDX-Licence-Identifier: EUPL-1.2

//go:build !ios && !android

// service_test.go — coverage for the `lthn service` daemon-lifecycle
// verbs. Only registry-level paths run: usage guards, the list
// rendering, and unknown names, which pkg/services rejects at
// Lookup before any OS service manager is touched. No test installs,
// starts, or stops a real daemon.

package main

import (
	"testing"

	core "dappco.re/go"
)

// TestService_CmdService_Bad — missing verb, unknown verb, and every
// per-verb usage guard.
func TestService_CmdService_Bad(t *testing.T) {
	core.AssertEqual(t, 2, cmdService(nil))
	core.AssertEqual(t, 2, cmdService([]string{"nonsense"}))
	core.AssertEqual(t, 2, cmdService([]string{"install"}))
	core.AssertEqual(t, 2, cmdService([]string{"uninstall"}))
	core.AssertEqual(t, 2, cmdService([]string{"start"}))
	core.AssertEqual(t, 2, cmdService([]string{"stop"}))
	core.AssertEqual(t, 2, cmdService([]string{"restart"}))
	core.AssertEqual(t, 2, cmdService([]string{"status"}))
}

// TestService_ServiceList_Good — the registry renders as JSON.
func TestService_ServiceList_Good(t *testing.T) {
	core.AssertEqual(t, 0, cmdService([]string{"list"}))
}

// TestService_CmdService_Ugly — names absent from the registry fail
// at Lookup, before the OS service manager is ever involved.
func TestService_CmdService_Ugly(t *testing.T) {
	core.AssertEqual(t, 1, cmdService([]string{"install", "no-such-service"}))
	core.AssertEqual(t, 1, cmdService([]string{"start", "no-such-service"}))
	core.AssertEqual(t, 1, cmdService([]string{"reload", "no-such-service"}))
	core.AssertEqual(t, 1, cmdService([]string{"status", "no-such-service"}))
}

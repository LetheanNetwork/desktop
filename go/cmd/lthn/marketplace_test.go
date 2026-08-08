// SPDX-Licence-Identifier: EUPL-1.2

//go:build !ios && !android

// marketplace_test.go — coverage for the `lthn marketplace` verbs
// that never leave the machine: dispatch + usage errors, the
// orm-backed lookups for bundles that do not exist, the v2 update
// stub, and the pure file-to-file import-coolify conversion. The
// search/install bodies fetch the live catalogue index over the
// network and are deliberately left to their usage arms.

package main

import (
	"testing"

	core "dappco.re/go"
)

// TestMarketplace_CmdMarketplace_Bad — missing verb, unknown verb,
// and every per-verb usage guard.
func TestMarketplace_CmdMarketplace_Bad(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	core.AssertEqual(t, 2, cmdMarketplace(nil))
	core.AssertEqual(t, 2, cmdMarketplace([]string{"nonsense"}))
	core.AssertEqual(t, 2, cmdMarketplace([]string{"install"}))
	core.AssertEqual(t, 2, cmdMarketplace([]string{"start"}))
	core.AssertEqual(t, 2, cmdMarketplace([]string{"stop"}))
	core.AssertEqual(t, 2, cmdMarketplace([]string{"uninstall"}))
	core.AssertEqual(t, 2, cmdMarketplace([]string{"status"}))
	core.AssertEqual(t, 2, cmdMarketplace([]string{"update"}))
	core.AssertEqual(t, 2, cmdMarketplace([]string{"import-coolify"}))
}

// TestMarketplace_CmdMarketplace_Ugly — start and status against a
// bundle that was never installed fail at the record lookup; stop
// and uninstall are idempotent (removing what is not there
// succeeds); update with an id is the documented not-implemented
// stub.
func TestMarketplace_CmdMarketplace_Ugly(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	core.AssertEqual(t, 1, cmdMarketplace([]string{"start", "no-such-bundle"}))
	core.AssertEqual(t, 0, cmdMarketplace([]string{"stop", "no-such-bundle"}))
	core.AssertEqual(t, 0, cmdMarketplace([]string{"uninstall", "no-such-bundle"}))
	core.AssertEqual(t, 1, cmdMarketplace([]string{"status", "no-such-bundle"}))
	core.AssertEqual(t, 1, cmdMarketplace([]string{"update", "no-such-bundle"}))
}

// TestMarketplace_MarketplaceList_Good — nothing installed yet is a
// zero-row JSON response, not an error.
func TestMarketplace_MarketplaceList_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	core.AssertEqual(t, 0, cmdMarketplace([]string{"list"}))
}

// TestMarketplace_ImportCoolify_Good — the pure conversion: a
// minimal Coolify template directory emits an lthn-vm manifest on
// stdout.
func TestMarketplace_ImportCoolify_Good(t *testing.T) {
	dir := t.TempDir()
	compose := "services:\n  app:\n    image: nginx:latest\n    ports:\n      - \"8080:80\"\n"
	core.RequireTrue(t, core.WriteFile(core.PathJoin(dir, "docker-compose.yml"), []byte(compose), 0o644).OK)

	core.AssertEqual(t, 0, cmdMarketplace([]string{"import-coolify", dir}))
}

// TestMarketplace_ImportCoolify_Ugly — a directory with no compose
// file fails the conversion.
func TestMarketplace_ImportCoolify_Ugly(t *testing.T) {
	core.AssertEqual(t, 1, cmdMarketplace([]string{"import-coolify", t.TempDir()}))
}

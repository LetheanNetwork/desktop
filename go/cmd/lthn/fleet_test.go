// SPDX-Licence-Identifier: EUPL-1.2

//go:build !ios && !android

// fleet_test.go — hermetic coverage for the `lthn fleet` read-only
// inspector. The dispatcher, help text, SQL read-only guard, and
// truncate helper run without any database; the missing-DB arms run
// under an isolated empty HOME; the snapshot + query paths run
// against a seeded empty DuckDB at the master path (created via
// store.OpenDuckDBReadWrite — the inspector itself only ever opens
// read-only snapshots).

package main

import (
	"testing"

	core "dappco.re/go"
	"dappco.re/go/store"
	"dappco.re/lthn/desktop/pkg/paths"
)

// newFleetHome points HOME at a fresh TempDir and returns the master
// DB path the fleet inspector resolves under it.
func newFleetHome(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	r := paths.MasterDB()
	core.RequireTrue(t, r.OK)
	return r.Value.(string)
}

// seedMasterDB creates a valid empty DuckDB file at the master path
// so fleetOpenReadOnly's stat + snapshot + open path all succeed.
func seedMasterDB(t *testing.T, dbPath string) {
	t.Helper()
	core.RequireTrue(t, core.MkdirAll(core.PathDir(dbPath), 0o755).OK)
	r := store.OpenDuckDBReadWrite(dbPath)
	core.RequireTrue(t, r.OK)
	core.RequireTrue(t, r.Value.(*store.DuckDB).Close().OK)
}

// TestFleet_CmdFleet_Bad — an unknown subcommand is a usage error
// decided before any database work.
func TestFleet_CmdFleet_Bad(t *testing.T) {
	core.AssertEqual(t, 2, cmdFleet([]string{"nonsense"}))
}

// TestFleet_CmdFleet_Good — every help spelling prints the usage
// block and exits clean.
func TestFleet_CmdFleet_Good(t *testing.T) {
	core.AssertEqual(t, 0, cmdFleet([]string{"help"}))
	core.AssertEqual(t, 0, cmdFleet([]string{"-h"}))
	core.AssertEqual(t, 0, cmdFleet([]string{"--help"}))
}

// TestFleet_Truncate_Good — short strings pass through untouched;
// long strings cut at n with an ellipsis appended.
func TestFleet_Truncate_Good(t *testing.T) {
	core.AssertEqual(t, "short", truncate("short", 10))
	core.AssertEqual(t, "abc…", truncate("abcdef", 3))
}

// TestFleet_FleetSQL_Bad — a missing query and any DDL/DML keyword
// (case-insensitive) are rejected before the database opens.
func TestFleet_FleetSQL_Bad(t *testing.T) {
	core.AssertEqual(t, 2, fleetSQL(nil))
	core.AssertEqual(t, 2, fleetSQL([]string{"INSERT INTO agents VALUES (1)"}))
	core.AssertEqual(t, 2, fleetSQL([]string{"drop table agents"}))
	core.AssertEqual(t, 2, fleetSQL([]string{"update agents set name='x'"}))
}

// TestFleet_FleetVerbs_MissingDB_Bad — under a fresh HOME the master
// DB does not exist, so every DB-backed verb reports and exits 1.
func TestFleet_FleetVerbs_MissingDB_Bad(t *testing.T) {
	newFleetHome(t)

	core.AssertEqual(t, 1, cmdFleet(nil))
	core.AssertEqual(t, 1, fleetAgents())
	core.AssertEqual(t, 1, fleetMachines())
	core.AssertEqual(t, 1, fleetActivity(nil))
	core.AssertEqual(t, 1, fleetQueue())
	core.AssertEqual(t, 1, fleetSQL([]string{"SELECT 1"}))
}

// TestFleet_FleetSQL_EmptyDB_Good — with a valid (empty) master DB
// the snapshot + read-only open + column/row print path completes.
func TestFleet_FleetSQL_EmptyDB_Good(t *testing.T) {
	seedMasterDB(t, newFleetHome(t))

	core.AssertEqual(t, 0, fleetSQL([]string{"SELECT 42 AS answer"}))
}

// TestFleet_FleetVerbs_EmptyDB_Ugly — a valid DB with no tables
// drives every verb's query-error arm; a non-numeric activity limit
// falls back to the default rather than erroring.
func TestFleet_FleetVerbs_EmptyDB_Ugly(t *testing.T) {
	seedMasterDB(t, newFleetHome(t))

	core.AssertEqual(t, 1, fleetAgents())
	core.AssertEqual(t, 1, fleetMachines())
	core.AssertEqual(t, 1, fleetActivity([]string{"3"}))
	core.AssertEqual(t, 1, fleetActivity([]string{"nope"}))
	core.AssertEqual(t, 1, fleetQueue())
}

// TestFleet_FleetSummary_EmptyDB_Good — the default verb: countRows
// degrades to 0 per missing table, the transient app Core boots under
// the isolated HOME for the keys count, and the summary prints clean.
func TestFleet_FleetSummary_EmptyDB_Good(t *testing.T) {
	seedMasterDB(t, newFleetHome(t))

	core.AssertEqual(t, 0, fleetSummary())
}

// TestFleet_FleetKeys_Good — a transient app Core boot under an
// isolated HOME finds the keys service with nothing stored yet.
func TestFleet_FleetKeys_Good(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	core.AssertEqual(t, 0, fleetKeys())
}

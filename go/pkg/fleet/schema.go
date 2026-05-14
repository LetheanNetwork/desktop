// SPDX-Licence-Identifier: EUPL-1.2

// Schema for the fleet + tasks + agent-activity tables in
// ~/Lethean/data/lthn.duckdb. Idempotent — every table uses
// `CREATE TABLE IF NOT EXISTS`. Migrations are append-only:
// new columns go on the end with sensible defaults, never drop or
// rename.
//
// Tables:
//
//	fleet_machines      — registered machines in the user's compute fleet
//	fleet_routing       — routing-priority rules between machines
//	tasks               — local task tracker (mantis-replacement)
//	task_links          — sync state between local tasks and external
//	                      connectors (forge / mantis / github)
//	agent_activity      — per-agent activity log (what cladius/codex/
//	                      lemma did, when, against which task)
//	connector_state     — per-connector cursor / last-sync metadata
//
// Forge / Mantis / GitHub are CONNECTORS — they are mirrored into the
// local tables via task_links, not the source of truth. Agent work
// stays local; sync to external systems is an explicit publish step.

package fleet

import (
	core "dappco.re/go"
	"dappco.re/go/store"
)

// schemaStatements is the full set of CREATE TABLE statements applied
// on every Service.Init(). DuckDB's CREATE TABLE IF NOT EXISTS is
// idempotent, so calling Init() repeatedly is cheap.
var schemaStatements = []string{
	`CREATE TABLE IF NOT EXISTS fleet_machines (
		id          TEXT PRIMARY KEY,
		name        TEXT NOT NULL,
		arch        TEXT,
		host        TEXT,
		port        INTEGER,
		status      TEXT NOT NULL DEFAULT 'offline',
		model       TEXT,
		load_pct    INTEGER NOT NULL DEFAULT 0,
		tps         REAL NOT NULL DEFAULT 0,
		is_self     BOOLEAN NOT NULL DEFAULT FALSE,
		tags        TEXT,
		created_at  BIGINT NOT NULL,
		updated_at  BIGINT NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS fleet_routing (
		priority    INTEGER NOT NULL,
		machine_id  TEXT NOT NULL,
		predicate   TEXT,
		PRIMARY KEY (priority, machine_id)
	)`,
	`CREATE TABLE IF NOT EXISTS tasks (
		id          TEXT PRIMARY KEY,
		title       TEXT NOT NULL,
		body        TEXT,
		status      TEXT NOT NULL DEFAULT 'open',
		priority    TEXT NOT NULL DEFAULT 'normal',
		severity    TEXT,
		assignee    TEXT,
		project     TEXT,
		tags        TEXT,
		created_at  BIGINT NOT NULL,
		updated_at  BIGINT NOT NULL,
		closed_at   BIGINT
	)`,
	`CREATE TABLE IF NOT EXISTS task_links (
		task_id     TEXT NOT NULL,
		connector   TEXT NOT NULL,
		external_id TEXT NOT NULL,
		url         TEXT,
		synced_at   BIGINT NOT NULL,
		PRIMARY KEY (task_id, connector)
	)`,
	`CREATE TABLE IF NOT EXISTS agent_activity (
		id          TEXT PRIMARY KEY,
		agent       TEXT NOT NULL,
		task_id     TEXT,
		machine_id  TEXT,
		action      TEXT NOT NULL,
		model       TEXT,
		started_at  BIGINT NOT NULL,
		ended_at    BIGINT,
		tokens_in   INTEGER NOT NULL DEFAULT 0,
		tokens_out  INTEGER NOT NULL DEFAULT 0,
		status      TEXT NOT NULL DEFAULT 'running',
		caller      TEXT,
		summary     TEXT
	)`,
	`CREATE TABLE IF NOT EXISTS connector_state (
		connector   TEXT PRIMARY KEY,
		cursor      TEXT,
		last_sync   BIGINT NOT NULL,
		error       TEXT
	)`,
}

// applySchema runs every CREATE TABLE IF NOT EXISTS statement against
// the open DuckDB. Returns the first failure or Ok.
//
// Usage:
//
//	if r := applySchema(db); !r.OK { return r }
func applySchema(db *store.DuckDB) core.Result {
	for _, stmt := range schemaStatements {
		if r := db.Exec(stmt); !r.OK {
			return core.Fail(core.E("fleet.applySchema", "create table", r.Value.(error)))
		}
	}
	return core.Ok(nil)
}

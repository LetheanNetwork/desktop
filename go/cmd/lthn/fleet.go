// SPDX-Licence-Identifier: EUPL-1.2

//go:build !ios && !android

// `lthn fleet` — read-only inspector for the master DuckDB
// (~/Lethean/data/lthn.duckdb). Backs the `gui.sh fleet ...`
// debug skill verb so operators can sanity-check what's landed
// in the agents / fleet_machines / agent_activity tables without
// crafting Python one-liners or starting a SQL session.
//
// Read-only by design — the `sql` subcommand rejects any DDL/DML.
// Plaintext secrets NEVER cross to stdout; `keys` lists refs only
// (the actual sealed blobs only decrypt via pkg/keys.Get from a
// Go-side caller).
//
// Usage:
//
//   lthn fleet                # row counts per table
//   lthn fleet agents         # all agents
//   lthn fleet machines       # all machines with capabilities
//   lthn fleet activity [N]   # last N agent_activity rows (default 20)
//   lthn fleet queue          # in-flight rows only (status='running')
//   lthn fleet keys           # encrypted key refs (NEVER plaintext)
//   lthn fleet sql "<query>"  # raw SELECT/EXPLAIN/WITH passthrough

package main

import (
	core "dappco.re/go"
	"dappco.re/go/crypt/keys"
	"dappco.re/go/store"
	"dappco.re/lthn/desktop/pkg/fleet"
	"dappco.re/lthn/desktop/pkg/paths"
)

func cmdFleet(args []string) int {
	sub := ""
	if len(args) > 0 {
		sub = args[0]
	}
	switch sub {
	case "", "summary":
		return fleetSummary()
	case "agents":
		return fleetAgents()
	case "machines":
		return fleetMachines()
	case "activity":
		return fleetActivity(args[1:])
	case "queue":
		return fleetQueue()
	case "keys":
		return fleetKeys()
	case "sql":
		return fleetSQL(args[1:])
	case "help", "-h", "--help":
		core.Println(fleetHelp)
		return 0
	default:
		core.Print(core.Stderr(), "lthn fleet: unknown subcommand %q\n%s\n", sub, fleetHelp)
		return 2
	}
}

const fleetHelp = `lthn fleet — read-only inspector for ~/Lethean/data/lthn.duckdb

usage:
  lthn fleet                # row counts per table
  lthn fleet agents         # all configured Team Members
  lthn fleet machines       # all fleet machines with capabilities
  lthn fleet activity [N]   # last N agent_activity rows (default 20)
  lthn fleet queue          # in-flight rows only (status='running')
  lthn fleet keys           # encrypted key refs (plaintext stays Go-side)
  lthn fleet sql "<query>"  # SELECT / EXPLAIN / WITH passthrough (read-only)`

// fleetOpenReadOnly opens a snapshot of the master DuckDB so the
// inspector coexists with a running GUI. DuckDB v1.x doesn't
// support multiprocess access — even readers conflict with the
// writer's OS file lock — so we cp the file to /tmp and open that.
// The snapshot is freshly recreated on every invocation; up to
// ~seconds stale relative to live, which is fine for a read-only
// debug inspector. Callers should NOT keep the handle long-lived
// (the snapshot is a point-in-time copy, not a live view).
func fleetOpenReadOnly() (*store.DuckDB, int) {
	dbR := paths.MasterDB()
	if !dbR.OK {
		core.Print(core.Stderr(), "fleet: %s\n", dbR.Error())
		return nil, 1
	}
	srcPath := dbR.Value.(string)
	statR := core.Stat(srcPath)
	if !statR.OK {
		core.Print(core.Stderr(), "fleet: master db does not exist at %s — run the GUI once to create it\n", srcPath)
		return nil, 1
	}
	// Snapshot to /tmp. DuckDB stores uncommitted state in a `.wal`
	// companion file until a checkpoint flushes — and lthn-desktop
	// rarely triggers checkpoints, so the main file is often just
	// schema + the WAL holds every actual row. We MUST copy both so
	// DuckDB can replay the WAL on snapshot open.
	tmpPath := "/tmp/lthn-fleet-snapshot.duckdb"
	tmpWAL := tmpPath + ".wal"
	// Clean old snapshot files to avoid stale WAL replay against a
	// fresh main file (would corrupt the read). Missing is fine —
	// Remove returns Fail in that case, which we ignore by design.
	core.Remove(tmpPath)
	core.Remove(tmpWAL)
	contentR := core.ReadFile(srcPath)
	if !contentR.OK {
		core.Print(core.Stderr(), "fleet: snapshot read failed: %s\n", contentR.Error())
		return nil, 1
	}
	if w := core.WriteFile(tmpPath, contentR.Value.([]byte), 0o600); !w.OK {
		core.Print(core.Stderr(), "fleet: snapshot write failed: %s\n", w.Error())
		return nil, 1
	}
	// Copy the WAL alongside if it exists. Source GUI may write
	// between read+write, so the snapshot is approximate — fine for
	// debug, NOT fine for anything authoritative.
	walSrc := srcPath + ".wal"
	if walStat := core.Stat(walSrc); walStat.OK {
		walR := core.ReadFile(walSrc)
		if walR.OK {
			if w := core.WriteFile(tmpWAL, walR.Value.([]byte), 0o600); !w.OK {
				core.Print(core.Stderr(), "fleet: WAL snapshot failed (continuing without): %s\n", w.Error())
			}
		}
	}
	r := store.OpenDuckDB(tmpPath)
	if !r.OK {
		core.Print(core.Stderr(), "fleet: open failed: %s\n", r.Error())
		return nil, 1
	}
	db := r.Value.(*store.DuckDB)
	return db, 0
}

// countRows returns the SELECT COUNT(*) for a table, 0 on error.
func countRows(db *store.DuckDB, table string) int {
	var n int
	r := db.QueryRowScan("SELECT COUNT(*) FROM "+table, &n)
	if !r.OK {
		return 0
	}
	return n
}

func fleetSummary() int {
	db, code := fleetOpenReadOnly()
	if code != 0 {
		return code
	}
	defer db.Close()
	type counter struct {
		label string
		count int
	}
	counters := []counter{
		{"fleet_machines", countRows(db, "fleet_machines")},
		{"agents", countRows(db, "agents")},
		{"agent_activity (total)", countRows(db, "agent_activity")},
		{"tasks", countRows(db, "tasks")},
		{"task_links", countRows(db, "task_links")},
		{"connector_state", countRows(db, "connector_state")},
		{"fleet_routing", countRows(db, "fleet_routing")},
	}
	// Also count keys via the Core-registered keys service. Boot a
	// transient Core for the lookup; the read-only fleet inspector
	// shares the same service substrate the GUI/serve modes use.
	c := newAppCore()
	if c != nil {
		defer c.ServiceShutdown(core.Background())
		if ksvc, _ := core.ServiceFor[*keys.Service](c, "keys"); ksvc != nil {
			// Tier-1 only — the fleet inspector surfaces provider
			// creds (OpenAI / Anthropic / ...), not the tier-0
			// single-instance Wails IPC key. Tier-0 list requires
			// the .seed-derived KEK live; the read-only inspector
			// boots a transient Core that doesn't wire it.
			if l := ksvc.ListTier1(); l.OK {
				counters = append(counters, counter{"keys (encrypted)", len(l.Value.([]string))})
			}
		}
	}
	dbR := paths.MasterDB()
	if dbR.OK {
		core.Println("db:", dbR.Value.(string))
	}
	core.Println("")
	for _, c := range counters {
		core.Print(core.Stdout(), "  %-28s %d\n", c.label, c.count)
	}
	return 0
}

func fleetAgents() int {
	db, code := fleetOpenReadOnly()
	if code != 0 {
		return code
	}
	defer db.Close()
	rows, err := db.Conn().Query(`
		SELECT id, name, provider, kind, COALESCE(base_url,''),
		       COALESCE(api_key_ref,''), COALESCE(model,''),
		       COALESCE(persona,''), status
		FROM agents ORDER BY name ASC
	`)
	if err != nil {
		core.Print(core.Stderr(), "agents: %s\n", err.Error())
		return 1
	}
	defer rows.Close()
	out := []fleet.Agent{}
	for rows.Next() {
		var a fleet.Agent
		if err := rows.Scan(&a.ID, &a.Name, &a.Provider, &a.Kind,
			&a.BaseURL, &a.APIKeyRef, &a.Model, &a.Persona, &a.Status); err != nil {
			core.Print(core.Stderr(), "agents scan: %s\n", err.Error())
			return 1
		}
		out = append(out, a)
	}
	if len(out) == 0 {
		core.Println("(no agents configured)")
		return 0
	}
	core.Print(core.Stdout(), "%d agent(s):\n\n", len(out))
	for _, a := range out {
		core.Print(core.Stdout(), "  %s\n", a.Name)
		core.Print(core.Stdout(), "    id        %s\n", a.ID)
		core.Print(core.Stdout(), "    provider  %s (%s)\n", a.Provider, a.Kind)
		if a.Model != "" {
			core.Print(core.Stdout(), "    model     %s\n", a.Model)
		}
		if a.BaseURL != "" {
			core.Print(core.Stdout(), "    base_url  %s\n", a.BaseURL)
		}
		if a.APIKeyRef != "" {
			core.Print(core.Stdout(), "    key_ref   %s (sealed at ~/Lethean/data/keys/%s.aead)\n", a.APIKeyRef, a.APIKeyRef)
		}
		if a.Persona != "" {
			core.Print(core.Stdout(), "    persona   %s\n", truncate(a.Persona, 80))
		}
		core.Print(core.Stdout(), "    status    %s\n\n", a.Status)
	}
	return 0
}

func fleetMachines() int {
	db, code := fleetOpenReadOnly()
	if code != 0 {
		return code
	}
	defer db.Close()
	rows, err := db.Conn().Query(`
		SELECT id, name, COALESCE(arch,''), COALESCE(host,''), COALESCE(port,0),
		       status, COALESCE(model,''), load_pct, tps, is_self,
		       COALESCE(CAST(capabilities AS TEXT), '[]')
		FROM fleet_machines ORDER BY is_self DESC, name ASC
	`)
	if err != nil {
		core.Print(core.Stderr(), "machines: %s\n", err.Error())
		return 1
	}
	defer rows.Close()
	type row struct {
		fleet.Machine
		capJSON string
	}
	out := []row{}
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.ID, &r.Name, &r.Arch, &r.Host, &r.Port,
			&r.Status, &r.Model, &r.LoadPct, &r.TPS, &r.IsSelf, &r.capJSON); err != nil {
			core.Print(core.Stderr(), "machines scan: %s\n", err.Error())
			return 1
		}
		caps := []string{}
		if r.capJSON != "" && r.capJSON != "null" {
			// Best-effort decode; malformed JSON degrades to empty
			// slice (caps initialised above), no surface error.
			if dr := core.JSONUnmarshalString(r.capJSON, &caps); !dr.OK {
				caps = []string{}
			}
		}
		r.Capabilities = caps
		out = append(out, r)
	}
	if len(out) == 0 {
		core.Println("(no machines registered)")
		return 0
	}
	core.Print(core.Stdout(), "%d machine(s):\n\n", len(out))
	for _, m := range out {
		self := ""
		if m.IsSelf {
			self = " (self)"
		}
		core.Print(core.Stdout(), "  %s%s\n", m.Name, self)
		core.Print(core.Stdout(), "    id        %s\n", m.ID)
		if m.Host != "" {
			core.Print(core.Stdout(), "    host      %s:%d\n", m.Host, m.Port)
		}
		if m.Arch != "" {
			core.Print(core.Stdout(), "    arch      %s\n", m.Arch)
		}
		core.Print(core.Stdout(), "    status    %s\n", m.Status)
		if m.Model != "" {
			core.Print(core.Stdout(), "    model     %s\n", m.Model)
		}
		if len(m.Capabilities) > 0 {
			core.Print(core.Stdout(), "    capable   %s\n", core.Join(", ", m.Capabilities...))
		}
		core.Print(core.Stdout(), "    load      %d%% · %.1f tok/s\n\n", m.LoadPct, m.TPS)
	}
	return 0
}

func fleetActivity(args []string) int {
	limit := 20
	if len(args) > 0 {
		if n := core.Atoi(args[0]); n.OK {
			if parsed, ok := n.Value.(int); ok && parsed > 0 {
				limit = parsed
			}
		}
	}
	db, code := fleetOpenReadOnly()
	if code != 0 {
		return code
	}
	defer db.Close()
	rows, err := db.Conn().Query(`
		SELECT id, agent, COALESCE(caller,''), COALESCE(task_id,''),
		       COALESCE(machine_id,''), COALESCE(model,''), action, status,
		       started_at, COALESCE(summary,'')
		FROM agent_activity
		ORDER BY started_at DESC
		LIMIT ?
	`, limit)
	if err != nil {
		core.Print(core.Stderr(), "activity: %s\n", err.Error())
		return 1
	}
	defer rows.Close()
	out := []fleet.QueueRow{}
	for rows.Next() {
		var q fleet.QueueRow
		if err := rows.Scan(&q.ID, &q.Agent, &q.Caller, &q.TaskID,
			&q.MachineID, &q.Model, &q.Action, &q.Status,
			&q.StartedAt, &q.Summary); err != nil {
			core.Print(core.Stderr(), "activity scan: %s\n", err.Error())
			return 1
		}
		out = append(out, q)
	}
	if len(out) == 0 {
		core.Println("(no activity yet)")
		return 0
	}
	core.Print(core.Stdout(), "%d activity row(s):\n\n", len(out))
	for _, q := range out {
		core.Print(core.Stdout(), "  [%s] %s · %s\n", q.Status, q.Agent, q.Action)
		if q.Model != "" {
			core.Print(core.Stdout(), "    model     %s\n", q.Model)
		}
		if q.MachineID != "" {
			core.Print(core.Stdout(), "    machine   %s\n", q.MachineID)
		}
		core.Print(core.Stdout(), "    started   %d\n\n", q.StartedAt)
	}
	return 0
}

func fleetQueue() int {
	db, code := fleetOpenReadOnly()
	if code != 0 {
		return code
	}
	defer db.Close()
	rows, err := db.Conn().Query(`
		SELECT agent, COALESCE(caller,''), COALESCE(machine_id,''),
		       COALESCE(model,''), action
		FROM agent_activity
		WHERE status = 'running'
		ORDER BY started_at DESC
	`)
	if err != nil {
		core.Print(core.Stderr(), "queue: %s\n", err.Error())
		return 1
	}
	defer rows.Close()
	type qrow struct {
		agent, caller, machineID, model, action string
	}
	out := []qrow{}
	for rows.Next() {
		var q qrow
		if err := rows.Scan(&q.agent, &q.caller, &q.machineID, &q.model, &q.action); err != nil {
			core.Print(core.Stderr(), "queue scan: %s\n", err.Error())
			return 1
		}
		out = append(out, q)
	}
	if len(out) == 0 {
		core.Println("(queue idle — no rows with status='running')")
		return 0
	}
	core.Print(core.Stdout(), "%d in-flight:\n\n", len(out))
	for _, q := range out {
		core.Print(core.Stdout(), "  %s · %s on %s · %s\n", q.agent, q.action, q.machineID, q.model)
	}
	return 0
}

func fleetKeys() int {
	c := newAppCore()
	if c == nil {
		return 1
	}
	defer c.ServiceShutdown(core.Background())
	svc, _ := core.ServiceFor[*keys.Service](c, "keys")
	if svc == nil {
		core.Print(core.Stderr(), "keys: service unavailable\n")
		return 1
	}
	// Tier-1 only — `fleet keys` lists provider credentials the
	// runner spawns agents with. Tier-0 (single-instance) is a
	// Go-side concern that doesn't surface in this inspector.
	l := svc.ListTier1()
	if !l.OK {
		core.Print(core.Stderr(), "keys: list failed: %s\n", l.Error())
		return 1
	}
	refs := l.Value.([]string)
	if len(refs) == 0 {
		core.Println("(no encrypted keys stored)")
		return 0
	}
	core.Print(core.Stdout(), "%d encrypted key(s) — plaintext stays Go-side:\n\n", len(refs))
	for _, ref := range refs {
		core.Print(core.Stdout(), "  %s\n", ref)
	}
	return 0
}

func fleetSQL(args []string) int {
	if len(args) == 0 {
		core.Print(core.Stderr(), "fleet sql: query required\n")
		return 2
	}
	query := args[0]
	// Read-only enforcement — case-insensitive prefix check, rejects
	// any DDL/DML keywords anywhere in the string. Defensive — DuckDB
	// also has read_only mode but we don't open the DB that way (the
	// GUI shares the same handle).
	upper := core.Upper(query)
	if core.Contains(upper, "INSERT") || core.Contains(upper, "UPDATE") ||
		core.Contains(upper, "DELETE") || core.Contains(upper, "DROP") ||
		core.Contains(upper, "CREATE") || core.Contains(upper, "ALTER") ||
		core.Contains(upper, "TRUNCATE") || core.Contains(upper, "REPLACE") {
		core.Print(core.Stderr(), "fleet sql: read-only enforced — DDL/DML rejected\n")
		return 2
	}
	db, code := fleetOpenReadOnly()
	if code != 0 {
		return code
	}
	defer db.Close()
	rows, err := db.Conn().Query(query)
	if err != nil {
		core.Print(core.Stderr(), "fleet sql: %s\n", err.Error())
		return 1
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		core.Print(core.Stderr(), "fleet sql: columns: %s\n", err.Error())
		return 1
	}
	core.Println(core.Join(" | ", cols...))
	sep := ""
	for i := 0; i < 80; i++ {
		sep += "─"
	}
	core.Println(sep)
	values := make([]any, len(cols))
	scanArgs := make([]any, len(cols))
	for i := range values {
		scanArgs[i] = &values[i]
	}
	count := 0
	for rows.Next() {
		if err := rows.Scan(scanArgs...); err != nil {
			core.Print(core.Stderr(), "fleet sql: scan: %s\n", err.Error())
			return 1
		}
		parts := make([]string, len(cols))
		for i, v := range values {
			if v == nil {
				parts[i] = "NULL"
			} else if b, ok := v.([]byte); ok {
				parts[i] = string(b)
			} else {
				parts[i] = core.Sprintf("%v", v)
			}
		}
		core.Println(core.Join(" | ", parts...))
		count++
	}
	core.Print(core.Stdout(), "\n(%d row(s))\n", count)
	return 0
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

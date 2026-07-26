// SPDX-Licence-Identifier: EUPL-1.2

// Package fleet owns the user's compute-fleet view (machines, routing
// rules) plus the live queue rail (agent_activity rows still running).
// Backed by ~/Lethean/data/lthn.duckdb — see pkg/paths.MasterDB().
//
// Forge / Mantis / GitHub are connectors mirrored into task_links;
// the fleet panel never reads from them directly. Agent work is local
// by design.
//
// Service shape — every public method returns core.Result; the value
// is in r.Value (cast to the documented type below).
//
// Usage example:
//
//	r := fleet.New()
//	if !r.OK { return r }
//	svc := r.Value.(*fleet.Service)
//	m := svc.Machines()
//	if !m.OK { return m }
//	rows := m.Value.([]fleet.Machine)

package fleet

import (
	"context"

	core "dappco.re/go"
	"dappco.re/go/store"
	"dappco.re/lthn/desktop/pkg/paths"
)

// Machine is one row of fleet_machines, projected for the panel. Plain
// JSON-friendly fields — every column maps 1:1 to TS.
//
//	{ id: "this-mac", name: "this Mac", arch: "M3 Pro · 36 GB",
//	  status: "online · loaded", model: "gemma-4-e2b",
//	  load_pct: 32, tps: 47.2, is_self: true,
//	  capabilities: ["inference","sandbox","bastion"] }
//
// Capabilities is the set of roles this machine fulfils — discovered
// automatically by the first-run wizard once that flow lands; the
// pair-machine modal asks the user to pick them manually. Each value
// is one of CapabilityInference / CapabilitySandbox / CapabilityBastion
// (other values are accepted but won't drive any built-in routing
// until they ship). Stored as DuckDB JSON for native validity + future
// json_array_contains() routing predicates.
type Machine struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Arch         string   `json:"arch"`
	Host         string   `json:"host"`
	Port         int      `json:"port"`
	Status       string   `json:"status"`
	Model        string   `json:"model"`
	LoadPct      int      `json:"load_pct"`
	TPS          float64  `json:"tps"`
	IsSelf       bool     `json:"is_self"`
	Tags         string   `json:"tags"`
	Capabilities []string `json:"capabilities"`
	CreatedAt    int64    `json:"created_at"`
	UpdatedAt    int64    `json:"updated_at"`
}

// Capability identifiers. New values can be added without a schema
// migration — the column is JSON; routing predicates filter on
// values they understand and ignore the rest.
const (
	CapabilityInference = "inference" // can run models
	CapabilitySandbox   = "sandbox"   // executes containers / sandboxed code
	CapabilityBastion   = "bastion"   // network ingress / SSH jump / TLS gateway
)

// QueueRow is one in-flight agent_activity row.
//
//	{ id: "abc", agent: "cladius", caller: "snider",
//	  machine_id: "this-mac", model: "gemma-4-e2b",
//	  status: "running", started_at: 1715... }
type QueueRow struct {
	ID        string `json:"id"`
	Agent     string `json:"agent"`
	Caller    string `json:"caller"`
	TaskID    string `json:"task_id"`
	MachineID string `json:"machine_id"`
	Model     string `json:"model"`
	Action    string `json:"action"`
	Status    string `json:"status"`
	StartedAt int64  `json:"started_at"`
	Summary   string `json:"summary"`
}

// RoutingRule is one row of fleet_routing, ordered by priority asc.
//
//	{ priority: 1, machine_id: "this-mac", predicate: "model:gemma-4-e2b" }
type RoutingRule struct {
	Priority  int    `json:"priority"`
	MachineID string `json:"machine_id"`
	Predicate string `json:"predicate"`
}

// Agent is one configured Team Member — endpoint + persona +
// feature set. The runtime composes a callable agent from the row.
//
//	{ id: "openai-default", name: "OpenAI · GPT-5",
//	  provider: "openai-compat", kind: "remote",
//	  base_url: "https://api.openai.com/v1",
//	  api_key_ref: "key:openai:default",
//	  model: "gpt-5", persona: "concise reviewer", ... }
//
// Kind distinguishes runtime topology:
//   - "remote"  : provider handles routing; api_key_ref + base_url
//     are load-bearing
//   - "local"   : runs against the user's machine fleet via routing
//     rules; model_settings JSON carries context length,
//     temperature, sampling params
type Agent struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Provider      string `json:"provider"`
	Kind          string `json:"kind"`
	BaseURL       string `json:"base_url"`
	APIKeyRef     string `json:"api_key_ref"`
	Model         string `json:"model"`
	Persona       string `json:"persona"`
	Features      string `json:"features"`
	ModelSettings string `json:"model_settings"`
	Status        string `json:"status"`
	Tags          string `json:"tags"`
	CreatedAt     int64  `json:"created_at"`
	UpdatedAt     int64  `json:"updated_at"`
}

// Service is the fleet domain service. Holds an open *store.DuckDB
// against the master DB and lazily applies schema on first use.
type Service struct {
	mu   core.RWMutex
	db   *store.DuckDB
	crew *crewSupervisor // local-machine crew sidecars (see crew.go)
}

// New constructs a Service, opens the master DB at ~/Lethean/data/lthn.duckdb,
// and applies the schema. The data directory is created if missing.
// Returns Result with Value = *Service on OK.
//
// Usage example:
//
//	r := fleet.New()
//	if !r.OK { return r }
//	svc := r.Value.(*fleet.Service)
//	defer svc.Close()
func New() core.Result {
	dbPathR := paths.MasterDB()
	if !dbPathR.OK {
		return dbPathR
	}
	// MasterDB() returns the path but does not create the file; ensure
	// the parent data/ directory exists so OpenDuckDB can create the
	// file on first open.
	if r := paths.DataDir(); !r.OK {
		return r
	}
	openR := store.OpenDuckDBReadWrite(dbPathR.Value.(string))
	if !openR.OK {
		return core.Fail(core.E("fleet.New", "open master DuckDB", openR.Err()))
	}
	db := openR.Value.(*store.DuckDB)
	if r := applySchema(db); !r.OK {
		closeR := db.Close()
		if !closeR.OK {
			core.Warn("fleet.New: close-on-schema-failure also failed", "err", closeR.Error())
		}
		return r
	}
	return core.Ok(&Service{db: db})
}

// Register adopts the canonical Service shape (Mantis #1336).
// Constructs a Service, opens the DB, and registers it on the passed
// *core.Core under the name "fleet". Returns Result.
//
// Usage example:
//
//	if r := fleet.Register(c); !r.OK { return r }
//
// Register is lock-tolerant. DuckDB serialises every connection to the
// master DB by an OS file lock — when the GUI or `lthn serve` already
// holds it, a sibling CLI process (e.g. `lthn fleet ...`) can't open
// the same file even read-only. Rather than tank Core boot, log a
// warning and register an empty Service so callers that don't need
// the live DB (the read-only inspector talks to a /tmp snapshot, the
// keys Service has no DB dep) keep working. Service methods check for
// the nil db and return a clean error when invoked against the inert
// handle, matching the rest of the Service contract.
//
// Usage example:
//
//	core.New(core.WithName("fleet", fleet.Register))
func Register(c *core.Core) core.Result {
	r := New()
	if r.OK {
		return r
	}
	core.Warn("fleet.Register: master DB unavailable, registering degraded Service", "err", r.Error())
	return core.Ok(&Service{})
}

// Close releases the underlying DB handle. Idempotent.
//
// Usage example:
//
//	r := svc.Close()
func (s *Service) Close() core.Result {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db == nil {
		return core.Ok(nil)
	}
	r := s.db.Close()
	s.db = nil
	return r
}

// Machines returns every fleet_machines row ordered by is_self desc
// (self first), then name. Empty fleet returns []Machine{} in Value.
//
// Usage example:
//
//	r := svc.Machines()
//	if r.OK { rows := r.Value.([]fleet.Machine); _ = rows }
func (s *Service) Machines() core.Result {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.db == nil {
		return core.Fail(core.NewError("fleet.Machines: service closed"))
	}
	rows, err := s.db.Conn().Query(`
		SELECT id, name, COALESCE(arch,''), COALESCE(host,''), COALESCE(port,0),
		       status, COALESCE(model,''), load_pct, tps, is_self, COALESCE(tags,''),
		       COALESCE(CAST(capabilities AS TEXT), '[]'),
		       created_at, updated_at
		FROM fleet_machines
		ORDER BY is_self DESC, name ASC
	`)
	if err != nil {
		return core.Fail(core.E("fleet.Machines", "query", err))
	}
	defer rows.Close()
	out := []Machine{}
	for rows.Next() {
		var (
			m       Machine
			capJSON string
		)
		if err := rows.Scan(&m.ID, &m.Name, &m.Arch, &m.Host, &m.Port,
			&m.Status, &m.Model, &m.LoadPct, &m.TPS, &m.IsSelf, &m.Tags,
			&capJSON, &m.CreatedAt, &m.UpdatedAt); err != nil {
			return core.Fail(core.E("fleet.Machines", "scan row", err))
		}
		// JSON parse via core wrapper — empty / null degrades to
		// empty slice rather than nil so TS sees [], not undefined.
		caps := []string{}
		if capJSON != "" && capJSON != "null" {
			if r := core.JSONUnmarshalString(capJSON, &caps); !r.OK {
				caps = []string{}
			}
		}
		m.Capabilities = caps
		out = append(out, m)
	}
	// sql.Rows.Next() returns false on both natural end-of-stream AND
	// on iterator error; Err() distinguishes. Without this check a
	// mid-stream DB blip silently returns a truncated machine list
	// and the Fleet view shows "where did my paired machine go?"
	if err := rows.Err(); err != nil {
		return core.Fail(core.E("fleet.Machines", "rows", err))
	}
	return core.Ok(out)
}

// SuperviseLocalCrew spawns and supervises the crew sidecars the local
// (is_self) machine is capable of: CapabilityInference -> lthn-ai, and
// CapabilitySandbox -> lthn-agent hub (Mantis #1807 Unit D). The
// supervisor health-gates each instance and respawns it on crash; it
// lives for the Service's lifetime and ServiceShutdown stops it.
//
// sandboxEnv carries KEY=VALUE pairs forwarded to the sandbox sidecar
// (MCP_JWT_SECRET and MCP_AUTH_TOKEN for the hub's two planes). The
// desktop startup resolves these from pkg/keys tier-0 before calling
// this method so the secrets are available at spawn time.
//
// Idempotent — re-invoking stops the previous crew first. Clean no-op
// when there's no is_self machine yet or it declares no crew capability,
// so callers can fire it once the self machine is registered without
// guarding the order.
//
// Usage example:
//
//	r := svc.SuperviseLocalCrew(context.Background(),
//	    []string{"MCP_JWT_SECRET=abc", "MCP_AUTH_TOKEN=xyz"})
func (s *Service) SuperviseLocalCrew(ctx context.Context, sandboxEnv []string) core.Result {
	r := s.Machines()
	if !r.OK {
		return r
	}
	machines, _ := r.Value.([]Machine)
	var caps []string
	for _, m := range machines {
		if m.IsSelf {
			caps = m.Capabilities
			break
		}
	}
	if len(caps) == 0 {
		core.Info("fleet.SuperviseLocalCrew: no local crew to supervise (no is_self machine or no capabilities)")
		return core.Ok(nil)
	}
	s.mu.Lock()
	prev := s.crew
	s.crew = superviseCrew(ctx, defaultCrew(sandboxEnv), caps)
	s.mu.Unlock()
	prev.stop() // nil-safe — replace any prior crew
	return core.Ok(nil)
}

// Queue returns up to `limit` in-flight agent_activity rows ordered by
// started_at desc. status='running' filter. limit<=0 means no cap.
// Empty queue returns []QueueRow{} in Value.
//
// Usage example:
//
//	r := svc.Queue(20)
//	if r.OK { rows := r.Value.([]fleet.QueueRow); _ = rows }
func (s *Service) Queue(limit int) core.Result {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.db == nil {
		return core.Fail(core.NewError("fleet.Queue: service closed"))
	}
	query := `
		SELECT id, agent, COALESCE(caller,''), COALESCE(task_id,''),
		       COALESCE(machine_id,''), COALESCE(model,''), action, status,
		       started_at, COALESCE(summary,'')
		FROM agent_activity
		WHERE status = 'running'
		ORDER BY started_at DESC
	`
	var (
		rows interface {
			Next() bool
			Scan(...any) error
			Close() error
			Err() error
		}
		err error
	)
	if limit > 0 {
		query += " LIMIT ?"
		rows, err = s.db.Conn().Query(query, limit)
	} else {
		rows, err = s.db.Conn().Query(query)
	}
	if err != nil {
		return core.Fail(core.E("fleet.Queue", "query", err))
	}
	defer rows.Close()
	out := []QueueRow{}
	for rows.Next() {
		var q QueueRow
		if err := rows.Scan(&q.ID, &q.Agent, &q.Caller, &q.TaskID,
			&q.MachineID, &q.Model, &q.Action, &q.Status,
			&q.StartedAt, &q.Summary); err != nil {
			return core.Fail(core.E("fleet.Queue", "scan row", err))
		}
		out = append(out, q)
	}
	if err := rows.Err(); err != nil {
		return core.Fail(core.E("fleet.Queue", "rows", err))
	}
	return core.Ok(out)
}

// Activity returns up to `limit` recent agent_activity rows across ALL
// statuses (running · done · failed), ordered started_at desc — the
// Agents view's run feed. Queue is the status='running' subset; Activity
// is the full recent history. limit<=0 means no cap. Empty feed returns
// []QueueRow{} in Value.
//
// Usage example:
//
//	r := svc.Activity(200)
//	if r.OK { rows := r.Value.([]fleet.QueueRow); _ = rows }
func (s *Service) Activity(limit int) core.Result {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.db == nil {
		return core.Fail(core.NewError("fleet.Activity: service closed"))
	}
	query := `
		SELECT id, agent, COALESCE(caller,''), COALESCE(task_id,''),
		       COALESCE(machine_id,''), COALESCE(model,''), action, status,
		       started_at, COALESCE(summary,'')
		FROM agent_activity
		ORDER BY started_at DESC
	`
	var (
		rows interface {
			Next() bool
			Scan(...any) error
			Close() error
			Err() error
		}
		err error
	)
	if limit > 0 {
		query += " LIMIT ?"
		rows, err = s.db.Conn().Query(query, limit)
	} else {
		rows, err = s.db.Conn().Query(query)
	}
	if err != nil {
		return core.Fail(core.E("fleet.Activity", "query", err))
	}
	defer rows.Close()
	out := []QueueRow{}
	for rows.Next() {
		var q QueueRow
		if err := rows.Scan(&q.ID, &q.Agent, &q.Caller, &q.TaskID,
			&q.MachineID, &q.Model, &q.Action, &q.Status,
			&q.StartedAt, &q.Summary); err != nil {
			return core.Fail(core.E("fleet.Activity", "scan row", err))
		}
		out = append(out, q)
	}
	if err := rows.Err(); err != nil {
		return core.Fail(core.E("fleet.Activity", "rows", err))
	}
	return core.Ok(out)
}

// RoutingRules returns every fleet_routing row ordered by priority asc.
// Empty rules returns []RoutingRule{} in Value.
//
// Usage example:
//
//	r := svc.RoutingRules()
//	if r.OK { rules := r.Value.([]fleet.RoutingRule); _ = rules }
func (s *Service) RoutingRules() core.Result {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.db == nil {
		return core.Fail(core.NewError("fleet.RoutingRules: service closed"))
	}
	rows, err := s.db.Conn().Query(`
		SELECT priority, machine_id, COALESCE(predicate,'')
		FROM fleet_routing
		ORDER BY priority ASC
	`)
	if err != nil {
		return core.Fail(core.E("fleet.RoutingRules", "query", err))
	}
	defer rows.Close()
	out := []RoutingRule{}
	for rows.Next() {
		var r RoutingRule
		if err := rows.Scan(&r.Priority, &r.MachineID, &r.Predicate); err != nil {
			return core.Fail(core.E("fleet.RoutingRules", "scan row", err))
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return core.Fail(core.E("fleet.RoutingRules", "rows", err))
	}
	return core.Ok(out)
}

// Agents returns every configured Team Member ordered by name asc.
// Empty registry returns []Agent{} in Value.
//
// Usage example:
//
//	r := svc.Agents()
//	if r.OK { agents := r.Value.([]fleet.Agent); _ = agents }
func (s *Service) Agents() core.Result {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.db == nil {
		return core.Fail(core.NewError("fleet.Agents: service closed"))
	}
	rows, err := s.db.Conn().Query(`
		SELECT id, name, provider, kind, COALESCE(base_url,''),
		       COALESCE(api_key_ref,''), COALESCE(model,''),
		       COALESCE(persona,''), COALESCE(features,''),
		       COALESCE(model_settings,''), status, COALESCE(tags,''),
		       created_at, updated_at
		FROM agents
		ORDER BY name ASC
	`)
	if err != nil {
		return core.Fail(core.E("fleet.Agents", "query", err))
	}
	defer rows.Close()
	out := []Agent{}
	for rows.Next() {
		var a Agent
		if err := rows.Scan(&a.ID, &a.Name, &a.Provider, &a.Kind,
			&a.BaseURL, &a.APIKeyRef, &a.Model, &a.Persona, &a.Features,
			&a.ModelSettings, &a.Status, &a.Tags,
			&a.CreatedAt, &a.UpdatedAt); err != nil {
			return core.Fail(core.E("fleet.Agents", "scan row", err))
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return core.Fail(core.E("fleet.Agents", "rows", err))
	}
	return core.Ok(out)
}

// UpsertAgent inserts or updates an agents row. id is the stable
// identifier; updated_at is set to now; created_at preserved on
// update via the COALESCE in the SET clause.
//
// Usage example:
//
//	r := svc.UpsertAgent(fleet.Agent{
//	    ID: "openai-default", Name: "OpenAI · GPT-5",
//	    Provider: "openai-compat", Kind: "remote",
//	    BaseURL: "https://api.openai.com/v1",
//	    APIKeyRef: "key:openai:default", Model: "gpt-5",
//	})
func (s *Service) UpsertAgent(a Agent) core.Result {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db == nil {
		return core.Fail(core.NewError("fleet.UpsertAgent: service closed"))
	}
	now := core.UnixNow()
	if a.CreatedAt == 0 {
		a.CreatedAt = now
	}
	if a.Kind == "" {
		a.Kind = "remote"
	}
	if a.Status == "" {
		a.Status = "idle"
	}
	return s.db.Exec(`
		INSERT INTO agents
			(id, name, provider, kind, base_url, api_key_ref, model,
			 persona, features, model_settings, status, tags,
			 created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET
			name=excluded.name, provider=excluded.provider,
			kind=excluded.kind, base_url=excluded.base_url,
			api_key_ref=excluded.api_key_ref, model=excluded.model,
			persona=excluded.persona, features=excluded.features,
			model_settings=excluded.model_settings,
			status=excluded.status, tags=excluded.tags,
			updated_at=excluded.updated_at
	`, a.ID, a.Name, a.Provider, a.Kind, a.BaseURL, a.APIKeyRef, a.Model,
		a.Persona, a.Features, a.ModelSettings, a.Status, a.Tags,
		a.CreatedAt, now)
}

// DeleteAgent removes an agent by id. No-op (OK) when the id isn't
// registered — callers don't need to pre-check.
//
// Usage example:
//
//	r := svc.DeleteAgent("openai-default")
func (s *Service) DeleteAgent(id string) core.Result {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db == nil {
		return core.Fail(core.NewError("fleet.DeleteAgent: service closed"))
	}
	return s.db.Exec(`DELETE FROM agents WHERE id = ?`, id)
}

// DeleteMachine removes a machine by id. No-op (OK) when the id
// isn't registered — callers don't need to pre-check. Mirrors
// DeleteAgent so the panel's row-level delete affordance has a
// uniform shape across both contexts.
//
// Usage example:
//
//	r := svc.DeleteMachine("shop")
func (s *Service) DeleteMachine(id string) core.Result {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db == nil {
		return core.Fail(core.NewError("fleet.DeleteMachine: service closed"))
	}
	return s.db.Exec(`DELETE FROM fleet_machines WHERE id = ?`, id)
}

// UpsertMachine inserts or updates a fleet_machines row. Used by the
// pair-machine wizard and the future discovery flow when new
// machines join. id is the stable identifier; updated_at is set to
// now; capabilities are JSON-encoded for native DuckDB validity.
//
// Usage example:
//
//	r := svc.UpsertMachine(fleet.Machine{
//	    ID: "shop", Name: "shop · 7950X",
//	    Capabilities: []string{fleet.CapabilityInference, fleet.CapabilitySandbox},
//	})
func (s *Service) UpsertMachine(m Machine) core.Result {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db == nil {
		return core.Fail(core.NewError("fleet.UpsertMachine: service closed"))
	}
	now := core.UnixNow()
	if m.CreatedAt == 0 {
		m.CreatedAt = now
	}
	// Encode capabilities as JSON text; DuckDB accepts JSON via TEXT
	// at insert and validates the shape itself. Nil/empty slice →
	// "[]" so readers always see a JSON array, never a SQL NULL.
	capJSON := "[]"
	if len(m.Capabilities) > 0 {
		capJSON = core.JSONMarshalString(m.Capabilities)
	}
	return s.db.Exec(`
		INSERT INTO fleet_machines
			(id, name, arch, host, port, status, model, load_pct, tps,
			 is_self, tags, capabilities, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET
			name=excluded.name, arch=excluded.arch, host=excluded.host,
			port=excluded.port, status=excluded.status, model=excluded.model,
			load_pct=excluded.load_pct, tps=excluded.tps,
			is_self=excluded.is_self, tags=excluded.tags,
			capabilities=excluded.capabilities,
			updated_at=excluded.updated_at
	`, m.ID, m.Name, m.Arch, m.Host, m.Port, m.Status, m.Model,
		m.LoadPct, m.TPS, m.IsSelf, m.Tags, capJSON, m.CreatedAt, now)
}

// ServiceName / ServiceStartup / ServiceShutdown — Wails3 lifecycle.
// Bindings land at frontend-ng/bindings/dappco.re/lthn/desktop/pkg/fleet/.
func (s *Service) ServiceName() string { return "Fleet" }

// ServiceStartup is the Wails3 lifecycle hook; no-op (Init already ran in New).
func (s *Service) ServiceStartup(_ core.Context, _ any) core.Result {
	return core.Ok(nil)
}

// ServiceShutdown stops the supervised crew (if any) then closes the DB.
func (s *Service) ServiceShutdown() core.Result {
	s.mu.Lock()
	crew := s.crew
	s.crew = nil
	s.mu.Unlock()
	crew.stop() // nil-safe
	return s.Close()
}

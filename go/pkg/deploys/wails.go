// SPDX-Licence-Identifier: EUPL-1.2

// Wails surface for the deploy history catalogue service. Methods are bound
// on the *Service receiver and exposed to the frontend via wails3 generate
// bindings — the TS shape lands at
// frontend-ng/bindings/dappco.re/lthn/desktop/pkg/deploys/.

package deploys

import (
	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/paths"
)

// List returns the deploy history and derived live-environment state.
// Filters by Env when input.Env is non-empty. Caps at input.Limit rows
// (defaults to 20). EnvRows are derived from the newest deploy per Env.
//
// Usage example:
//
//	r := svc.List(deploys.ListInput{Env: "preview", Limit: 10})
//	if r.OK { out := r.Value.(deploys.ListOutput) }
func (s *Service) List(input ListInput) core.Result {
	limit := input.Limit
	if limit <= 0 {
		limit = 20
	}

	records, err := readAll()
	if err != nil {
		return core.Fail(core.E("deploys.List", "readAll failed", err))
	}
	if records == nil {
		records = []DeployRecord{}
	}

	// Build env rows from the full unfiltered set (newest-first, one per env).
	now := core.Now()
	envRows := buildEnvRows(records, now)

	// Apply env filter for the history table.
	filtered := records
	if input.Env != "" {
		filtered = filtered[:0:0]
		for _, r := range records {
			if r.Env == input.Env {
				filtered = append(filtered, r)
			}
		}
	}

	total := len(filtered)
	history := make([]DeployRow, 0, len(filtered))
	for _, r := range filtered {
		history = append(history, toDeployRow(r, now))
	}
	if len(history) > limit {
		history = history[:limit]
	}

	if envRows == nil {
		envRows = []EnvRow{}
	}

	return core.Ok(ListOutput{
		History: history,
		Envs:    envRows,
		Total:   total,
	})
}

// Get returns the full deploy record and notes body for a single deploy ID.
// Returns a failure result when the ID is invalid or the file is unreadable.
//
// Usage example:
//
//	r := svc.Get(deploys.GetInput{ID: "deploy-20260516-1432"})
//	if r.OK { out := r.Value.(deploys.GetOutput) }
func (s *Service) Get(input GetInput) core.Result {
	pathR := deployFilePath(input.ID)
	if !pathR.OK {
		return pathR
	}
	fpath := pathR.Value.(string)

	rawR := core.ReadFile(fpath)
	if !rawR.OK {
		return core.Fail(core.E("deploys.Get", "file not found: "+input.ID, nil))
	}
	raw, _ := rawR.Value.([]byte)

	rec, notes, err := parseFrontmatter(raw)
	if err != nil {
		return core.Fail(core.E("deploys.Get", "parse failed", err))
	}

	return core.Ok(GetOutput{Record: rec, Notes: notes})
}

// Create persists a new deploy record to ~/Lethean/deploys/{deploy-id}.md.
// The ID is derived from the current time via generateID — two deploys in
// the same minute share an ID and the second write overwrites the first
// (append semantics are a v2 concern per RFC §7.0).
//
// Validates required fields: Env, By, Commit, Outcome, Dur.
//
// Usage example:
//
//	r := svc.Create(deploys.CreateInput{
//	    Env: "preview", By: "Tobi", Commit: "b8e034",
//	    Outcome: "success", Dur: "58s",
//	})
//	if r.OK { out := r.Value.(deploys.CreateOutput) }
func (s *Service) Create(input CreateInput) core.Result {
	if input.Env == "" {
		return core.Fail(core.E("deploys.Create", "env is required", nil))
	}
	if input.By == "" {
		return core.Fail(core.E("deploys.Create", "by is required", nil))
	}
	if input.Commit == "" {
		return core.Fail(core.E("deploys.Create", "commit is required", nil))
	}
	if input.Outcome == "" {
		return core.Fail(core.E("deploys.Create", "outcome is required", nil))
	}
	if input.Dur == "" {
		return core.Fail(core.E("deploys.Create", "dur is required", nil))
	}

	now := core.Now()
	id := generateID(now)

	// Validate the generated ID to confirm it passes the IsValidID gate
	// (paranoia — generateID output is always valid by construction).
	if err := paths.IsValidID(id); err != nil {
		return core.Fail(core.E("deploys.Create", "generated id invalid", err))
	}

	rec := DeployRecord{
		ID:        id,
		Env:       input.Env,
		By:        input.By,
		Commit:    input.Commit,
		Version:   input.Version,
		URL:       input.URL,
		Outcome:   input.Outcome,
		Dur:       input.Dur,
		Timestamp: now,
	}

	content, err := renderTrix(rec, input.Notes)
	if err != nil {
		return core.Fail(core.E("deploys.Create", "render failed", err))
	}

	pathR := deployFilePath(id)
	if !pathR.OK {
		return pathR
	}
	fpath := pathR.Value.(string)

	if w := core.WriteFile(fpath, content, 0o600); !w.OK {
		return core.Fail(core.E("deploys.Create", "write failed", nil))
	}

	return core.Ok(CreateOutput{ID: id})
}

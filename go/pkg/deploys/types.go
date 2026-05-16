// SPDX-Licence-Identifier: EUPL-1.2

// Package deploys is the lthn-side v1 deploy history catalogue service.
// Reads and writes Trix-style markdown files from
// ~/Lethean/deploys/{deploy-id}.md — YAML frontmatter per record,
// optional log/notes body.
//
// v1 scope: deploy record catalogue — List, Get, Create.
// No execution surface. No CI/CD trigger. No live environment probe.
// Environment state is derived from the most-recent deploy per env field.
//
// v2 (separate Mantis ticket) adds live HTTP probes, CI/CD webhook
// ingestion, rollback trigger, and deploy log streaming.
//
// Wire shape matches EnvRow + DeployRow consumed by
// <lthn-view-deploys> in the Coding role view.
//
// Usage example (Wails):
//
//	r := deploysSvc.List(deploys.ListInput{})
//	if r.OK { out := r.Value.(deploys.ListOutput) }
package deploys

import core "dappco.re/go"

// DeployRecord is the persisted representation of one deploy event.
// Stored as a Trix-style markdown file: YAML frontmatter + newline + body.
// Matches the DeployRow interface in
// frontend/src/lit/views/coding/deploys.ts.
//
// Usage example:
//
//	rec := deploys.DeployRecord{
//	    ID: "deploy-20260516-1432", Env: "preview",
//	    By: "Tobi", Commit: "b8e034",
//	    Outcome: "success", Dur: "58s",
//	    Timestamp: core.Now(),
//	}
type DeployRecord struct {
	// ID is the canonical deploy identifier: "deploy-{YYYYMMDD}-{HHMM}".
	ID string `yaml:"id"`

	// Env is the deployment target name: "production", "staging", "preview".
	Env string `yaml:"env"`

	// By is the operator name or username who triggered the deploy.
	By string `yaml:"by"`

	// Commit is the short SHA (6 chars) of the deployed commit.
	Commit string `yaml:"commit"`

	// Version is the semantic version string. Optional.
	Version string `yaml:"version,omitempty"`

	// URL is the environment base URL. Optional.
	URL string `yaml:"url,omitempty"`

	// Outcome is "success", "rolled-back", or "failed".
	Outcome string `yaml:"outcome"`

	// Dur is the human-readable deploy duration ("58s", "1m 04s").
	Dur string `yaml:"dur"`

	// Timestamp is when the deploy event occurred.
	Timestamp core.Time `yaml:"timestamp"`
}

// DeployRow is the JSON wire type for one entry in the deploy history table.
// Matches the DeployRow interface in
// frontend/src/lit/views/coding/deploys.ts exactly.
//
// Usage example:
//
//	row := deploys.DeployRow{
//	    Ts: "14:32", Env: "preview", By: "Tobi",
//	    Commit: "b8e034", Outcome: "success", Dur: "58s",
//	}
type DeployRow struct {
	// Ts is the human-readable timestamp ("14:32", "yest", "3 d").
	Ts string `json:"ts"`

	// Env is the deployment target name.
	Env string `json:"env"`

	// By is the operator name.
	By string `json:"by"`

	// Commit is the short SHA.
	Commit string `json:"commit"`

	// Outcome is "success", "rolled-back", or "failed".
	Outcome string `json:"outcome"`

	// Dur is the human-readable duration.
	Dur string `json:"dur"`
}

// EnvRow is the JSON wire type for one live environment card.
// Derived from the most-recent DeployRecord per Env value.
// Matches the EnvRow interface in
// frontend/src/lit/views/coding/deploys.ts exactly.
//
// Usage example:
//
//	env := deploys.EnvRow{
//	    Name: "production", URL: "lthn.ai",
//	    Version: "v0.1.8", Commit: "4a82c1",
//	    Age: "4d", Health: "ok",
//	}
type EnvRow struct {
	// Name is the environment name ("production", "staging", "preview").
	Name string `json:"name"`

	// URL is the environment base URL.
	URL string `json:"url"`

	// Version is the semantic version last deployed here.
	Version string `json:"version"`

	// Commit is the short SHA last deployed here.
	Commit string `json:"commit"`

	// Age is the human-readable time since last deploy ("4d", "2h", "22m").
	Age string `json:"age"`

	// Health is "ok", "degraded", or "down".
	// v1: "failed" outcome → "failed"; anything else → "ok". No live probe.
	Health string `json:"health"`
}

// CreateInput drives the Create method.
//
// Usage example:
//
//	r := svc.Create(deploys.CreateInput{
//	    Env: "preview", By: "Tobi", Commit: "b8e034",
//	    Outcome: "success", Dur: "58s",
//	})
type CreateInput struct {
	// Env is the deployment target name. Required.
	Env string `json:"env"`

	// By is the operator name. Required.
	By string `json:"by"`

	// Commit is the short SHA. Required.
	Commit string `json:"commit"`

	// Version is the semantic version string. Optional.
	Version string `json:"version,omitempty"`

	// URL is the environment base URL. Optional.
	URL string `json:"url,omitempty"`

	// Outcome is "success", "rolled-back", or "failed". Required.
	Outcome string `json:"outcome"`

	// Dur is the human-readable duration. Required.
	Dur string `json:"dur"`

	// Notes is the free-form log body. Optional.
	Notes string `json:"notes,omitempty"`
}

// CreateOutput is the Create response envelope.
//
// Usage example:
//
//	out := r.Value.(deploys.CreateOutput)
//	_ = out.ID
type CreateOutput struct {
	// ID is the generated deploy identifier.
	ID string `json:"id"`
}

// ListInput drives the List method.
//
// Usage example:
//
//	r := svc.List(deploys.ListInput{Env: "preview", Limit: 10})
type ListInput struct {
	// Env filters to one named environment. Empty = all environments.
	Env string `json:"env,omitempty"`

	// Limit caps the result count. Zero defaults to 20.
	Limit int `json:"limit,omitempty"`
}

// ListOutput is the List response envelope.
//
// Usage example:
//
//	out := r.Value.(deploys.ListOutput)
//	for _, row := range out.History { _ = row.Env }
//	for _, env := range out.Envs   { _ = env.Name }
type ListOutput struct {
	// History is the deploy history sorted newest-first, capped at Limit.
	History []DeployRow `json:"history"`

	// Envs is the derived live-environment state, one per distinct Env value.
	Envs []EnvRow `json:"envs"`

	// Total is the unfiltered deploy count (before Limit cap).
	Total int `json:"total"`
}

// GetInput drives the Get method.
//
// Usage example:
//
//	r := svc.Get(deploys.GetInput{ID: "deploy-20260516-1432"})
type GetInput struct {
	// ID is the deploy identifier. Required. Validated via paths.IsValidID.
	ID string `json:"id"`
}

// GetOutput is the Get response envelope.
//
// Usage example:
//
//	out := r.Value.(deploys.GetOutput)
//	_ = out.Record.Env
type GetOutput struct {
	// Record is the full deploy record from the YAML frontmatter.
	Record DeployRecord `json:"record"`

	// Notes is the markdown body after the YAML frontmatter (may be empty).
	Notes string `json:"notes"`
}

// envOrder defines canonical display ordering of known environment names.
// Unknown envs sort alphabetically after these.
var envOrder = []string{"production", "staging", "preview"}

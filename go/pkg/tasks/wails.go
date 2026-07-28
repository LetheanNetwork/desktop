// SPDX-Licence-Identifier: EUPL-1.2

// Wails surface for the tasks subsystem. Two types live here:
//
//   - *Service — the SUBSTRATE-tier service. Carries the per-method
//     auth.Require gate + ENFORCE-not-claim Reporter/Author overwrite
//     from auth.Caller(s.core).Subject. Pre-stamped caller identity
//     flows through unchanged — the gate denies / the stamp drives
//     forensic attribution. Stays the package-level integration point
//     for in-Go consumers (tests, future HTTP/CLI surfaces).
//
//   - *WailsService — the Shape (a.i) IPC-entry WRAPPER. Wraps the
//     *Service, and at each method entry stamps TierRenderer via
//     auth.SetCaller (with breadcrumb Subject="wails", Source="wails")
//     BEFORE delegating to the inner *Service method, then defers
//     auth.ClearCaller so the stamp clears at return. This is the
//     RFC.tier-auth-substrate.md v3.1 §4.4 / Cerberus #73 F-1
//     "hand-written wrapper at the registration site": the renderer
//     tier never reaches the substrate's Require gate without the
//     stamp, so a Wails IPC call landing here without prior session
//     stamping (the today-default for the desktop's tray surface)
//     still has the gate fire correctly.
//
// Registration shape: pkg/desktop/desktop.go binds *WailsService into
// wails3's service array (replacing the bare *Service binding the
// pre-stamp shape used). The inner *Service stays accessible via
// WailsService.Substrate() for tests + in-Go consumers that legitimately
// want the substrate-tier shape.
//
// The package-level API (Create/Get/List/Update/AddNote in api.go) is
// the canonical Go-side surface; *Service is a thin wrapper that holds
// the *core.Core so each Wails-bound method can dispatch without the
// frontend having to know about Core wiring. Inputs/outputs mirror the
// repos.Service shape — one Input struct per method, returning a
// core.Result whose Value carries the typed response.
//
// SECURITY-NOTE (frontend binding shift — surfaced for Cladius
// adjudication): wails3 generates the TS binding filename from the Go
// type name lowercased — *WailsService → wailsservice.ts under
// frontend/bindings/dappco.re/lthn/desktop/pkg/tasks/. The pre-stamp
// shape lived at service.ts; the frontend imports
// "@desktop/tasks/service" today. Method IDs (ByID hashes) are also
// keyed off the FQ type name and will shift. A follow-on Mantis ticket
// covers re-running `wails3 generate` + updating frontend imports
// (@desktop/tasks/wailsservice or alias). Per [[feedback_wails_table_
// shim_preserves_bindings]] precedent the alternative (in-place stamp
// on the existing *Service) was prototyped and rejected because the
// substrate-tier api_test.go contract REQUIRES pre-stamped caller
// identity to flow through *Service.Create unchanged (Reporter ==
// stamped Subject); in-place stamping breaks the substrate tests.
//
// SECURITY-NOTE (Phase 2 deferred per brief escape valve): RFC v3.1
// §4.4 prescribes Service methods take core.Context as first param so
// the per-call stamp lives on the call's own ctx (race-free) rather
// than the *core.Core sidecar. Phase 2 cascades through api.go's
// package-level Create/Update/AddNote signatures and the tests calling
// them inside pkg/tasks (~15+ in-package callsites). Deferred to a
// follow-on Mantis ticket per brief escape valve "If Service method
// signature change cascades widely, surface — may need Phase split".
// Today the sidecar-stamp pattern is safe (single Wails consumer per
// *core.Core) and the Phase 1 security goal — "renderer-tier stamped
// on IPC entry" — is met.
//
// Deferred (tracked separately):
//   - Search(query) substring lookup — needs index design.
//   - CalendarBlock primitive — out of scope for this pass.
//   - Extended Issue fields (Estimate, Theme, RICE inputs) — schema
//     migration needed; current pass uses the existing Issue shape.
//   - Phase 2 ctx-first Service signatures — see SECURITY-NOTE above.
//   - Frontend binding-path migration (service.ts → wailsservice.ts).

package tasks

import (
	core "dappco.re/go"
	"dappco.re/go/orm"

	"dappco.re/lthn/desktop/pkg/auth"
)

// Service owns the tasks Wails surface. Wraps *core.Core so the
// bound methods can delegate to the package-level api.go functions
// without leaking Core into the frontend binding.
//
// Wired via application.NewService(tasks.NewService(c)) in
// pkg/desktop/desktop.go (alongside repos/vi/lint/etc.).
type Service struct {
	core *core.Core
}

// NewService constructs the tasks Wails surface against a Core
// container.
//
// Usage example:
//
//	svc := tasks.NewService(c)
func NewService(c *core.Core) *Service {
	return &Service{core: c}
}

// Register constructs the tasks service for Core registration. Mirrors
// the shape used by pkg/repos so the desktop bootstrap can wire tasks
// via core.WithName("tasks", tasks.Register) when/if the service-bus
// path is preferred over direct construction.
//
// Usage example:
//
//	core.New(core.WithName("tasks", tasks.Register))
func Register(c *core.Core) core.Result {
	return core.Ok(NewService(c))
}

// ListInput narrows a List query. Mirrors ListFilter from types.go
// (json tags added for the wails3 TS binding generator). All fields
// optional — empty = no constraint.
type ListInput struct {
	Project  string `json:"project,omitempty"`
	State    string `json:"state,omitempty"`
	Assignee string `json:"assignee,omitempty"`
	Reporter string `json:"reporter,omitempty"`
	Limit    int    `json:"limit,omitempty"`
	Offset   int    `json:"offset,omitempty"`
}

// ListOutput wraps the issues slice plus a count for the UI's
// "X items" header. Keeping the count separate from len() lets the
// shape grow (pagination cursor, etc.) without breaking consumers.
type ListOutput struct {
	Issues []Issue `json:"issues"`
	Count  int     `json:"count"`
}

// List returns issues matching the filter, newest updated_at first.
// Empty input returns every open issue across every project (the
// natural shape for a global backlog view).
//
// Tier gate (RFC §5.1 / table row pkg/tasks read): Require permits
// TierOperator + TierRenderer + TierCascade. Per-account filtering
// remains the per-row concern (separate RFC).
//
// Usage example (TS binding):
//
//	const r = await List({ state: "open" });
//	const out = r.Value as { issues: Issue[]; count: number };
func (s *Service) List(input ListInput) core.Result {
	id, ok := auth.Require(s.core, "tasks.Service.List",
		auth.TierOperator, auth.TierRenderer, auth.TierCascade)
	if !ok {
		return core.Fail(core.E("tasks.Service.List",
			"tasks.tier_not_permitted: "+id.Tier.String(), nil))
	}
	filter := ListFilter{
		Project:  input.Project,
		State:    input.State,
		Assignee: input.Assignee,
		Reporter: input.Reporter,
		Limit:    input.Limit,
		Offset:   input.Offset,
	}
	r := List(s.core, filter)
	if !r.OK {
		return r
	}
	issues, ok := orm.Cast[[]Issue](r)
	if !ok {
		return core.Fail(core.E("tasks.Service.List", "cast issues", nil))
	}
	return core.Ok(ListOutput{Issues: issues, Count: len(issues)})
}

// GetInput identifies a single issue by ID.
type GetInput struct {
	ID string `json:"id"`
}

// Get returns the issue identified by input.ID, or a fail Result if
// the ID is empty / not found.
//
// Tier gate (RFC §5.1): Require permits TierOperator + TierRenderer
// + TierCascade. Per-row ACL deferred to Phase 2.
//
// Usage example (TS binding):
//
//	const r = await Get({ id: "abc123" });
//	const issue = r.Value as Issue;
func (s *Service) Get(input GetInput) core.Result {
	id, ok := auth.Require(s.core, "tasks.Service.Get",
		auth.TierOperator, auth.TierRenderer, auth.TierCascade)
	if !ok {
		return core.Fail(core.E("tasks.Service.Get",
			"tasks.tier_not_permitted: "+id.Tier.String(), nil))
	}
	if input.ID == "" {
		return core.Fail(core.E("tasks.Service.Get", "id is required", nil))
	}
	return Get(s.core, input.ID)
}

// CreateIssueInput captures the fields a caller must / may supply
// when creating an issue. Mirrors CreateInput from types.go with
// json tags for the wails3 TS binding generator.
type CreateIssueInput struct {
	Project       string `json:"project"`
	Summary       string `json:"summary"`
	Description   string `json:"description,omitempty"`
	Severity      string `json:"severity,omitempty"`
	Priority      string `json:"priority,omitempty"`
	Assignee      string `json:"assignee,omitempty"`
	Reporter      string `json:"reporter,omitempty"`
	Version       string `json:"version,omitempty"`
	TargetVersion string `json:"target_version,omitempty"`
}

// Create inserts a new issue and returns the populated record.
// Required: Project + Summary. Defaults applied for State / Severity
// / Priority by the underlying api.go Create.
//
// ENFORCE-not-claim (RFC.tier-auth-substrate.md §5.3 / Mantis #1722):
// Reporter is OVERWRITTEN from auth.Caller(s.core).Subject — the
// input.Reporter field is SILENTLY IGNORED. A hostile renderer can
// supply any string here; the service refuses to attribute records
// based on caller assertion. Assignee remains input-driven (Operator
// legitimately assigns work to others — see §5.3 row).
//
// Tier gate (RFC §5.1 / §10 Q-1 Phase 1 ALLOW): Require permits
// TierOperator + TierRenderer. Other tiers (TierCascade, TierCron,
// TierInternal) reach this surface only through misconfiguration; the
// gate returns a visible Fail rather than silently dropping the
// attempt so the audit trail captures the deny.
//
// Usage example (TS binding):
//
//	const r = await Create({ project: "lthn", summary: "wire backlog" });
//	const issue = r.Value as Issue;
func (s *Service) Create(input CreateIssueInput) core.Result {
	id, ok := auth.Require(s.core, "tasks.Service.Create", auth.TierOperator, auth.TierRenderer)
	if !ok {
		return core.Fail(core.E("tasks.Service.Create",
			"tasks.tier_not_permitted: "+id.Tier.String(), nil))
	}
	return Create(s.core, CreateInput{
		Project:       input.Project,
		Summary:       input.Summary,
		Description:   input.Description,
		Severity:      input.Severity,
		Priority:      input.Priority,
		Assignee:      input.Assignee,
		Reporter:      id.Subject, // ENFORCE-not-claim — input.Reporter ignored
		Version:       input.Version,
		TargetVersion: input.TargetVersion,
	})
}

// UpdateIssueInput captures partial-field updates for an existing
// issue. The frontend supplies the ID alongside the fields; this is
// the Wails-friendly merge of api.go's separate (id, UpdateInput)
// signature. Description uses a pointer to distinguish "clear to
// empty" from "no change" — TS callers pass null vs undefined.
type UpdateIssueInput struct {
	ID            string  `json:"id"`
	Summary       string  `json:"summary,omitempty"`
	Description   *string `json:"description,omitempty"`
	State         string  `json:"state,omitempty"`
	Severity      string  `json:"severity,omitempty"`
	Priority      string  `json:"priority,omitempty"`
	Assignee      string  `json:"assignee,omitempty"`
	Version       string  `json:"version,omitempty"`
	TargetVersion string  `json:"target_version,omitempty"`
	FixedIn       string  `json:"fixed_in,omitempty"`
	Resolution    string  `json:"resolution,omitempty"`
}

// Update applies partial-field changes to the named issue and
// returns the updated record on success. State transitions to
// Done/Cancelled also set ClosedAt (handled by api.go Update).
//
// ENFORCE-not-claim (RFC §5.3 row 321): UpdateIssueInput today carries
// no Editor / Reporter attribution field, so no caller-derived
// overwrite is needed. If a future field lands (e.g. last_edited_by),
// it MUST be sourced from auth.Caller(s.core).Subject following the
// Create / AddNote pattern in this file.
//
// Tier gate (RFC §5.1): Require permits TierOperator + TierRenderer.
// Per-row ACL (renderer touching another account's issue) is tracked
// as a Phase 2 follow-on per RFC §10 B.2 done-criteria.
//
// Usage example (TS binding):
//
//	const r = await Update({ id: "abc123", state: "in_progress" });
//	const updated = r.Value as Issue;
func (s *Service) Update(input UpdateIssueInput) core.Result {
	id, ok := auth.Require(s.core, "tasks.Service.Update", auth.TierOperator, auth.TierRenderer)
	if !ok {
		return core.Fail(core.E("tasks.Service.Update",
			"tasks.tier_not_permitted: "+id.Tier.String(), nil))
	}
	if input.ID == "" {
		return core.Fail(core.E("tasks.Service.Update", "id is required", nil))
	}
	return Update(s.core, input.ID, UpdateInput{
		Summary:       input.Summary,
		Description:   input.Description,
		State:         input.State,
		Severity:      input.Severity,
		Priority:      input.Priority,
		Assignee:      input.Assignee,
		Version:       input.Version,
		TargetVersion: input.TargetVersion,
		FixedIn:       input.FixedIn,
		Resolution:    input.Resolution,
	})
}

// AddNoteInput identifies the parent issue plus the note body +
// author. All three fields required; api.go AddNote fails loudly on
// blanks.
type AddNoteInput struct {
	IssueID string `json:"issue_id"`
	Body    string `json:"body"`
	Author  string `json:"author,omitempty"`
}

// AddNote appends a comment / activity log entry to an issue and
// returns the persisted Note. Broadcasts a KindNoted event so the
// activity feed / audit log / PR-watcher can react.
//
// ENFORCE-not-claim (RFC §5.3 / Mantis #1722): Author is OVERWRITTEN
// from auth.Caller(s.core).Subject — the input.Author field is
// SILENTLY IGNORED. A hostile renderer cannot plant a false audit
// trail by claiming to be another account.
//
// Tier gate (RFC §5.1): Require permits TierOperator + TierRenderer.
//
// Usage example (TS binding):
//
//	const r = await AddNote({ issue_id: "abc123", body: "shipped" });
//	const note = r.Value as Note;
func (s *Service) AddNote(input AddNoteInput) core.Result {
	id, ok := auth.Require(s.core, "tasks.Service.AddNote", auth.TierOperator, auth.TierRenderer)
	if !ok {
		return core.Fail(core.E("tasks.Service.AddNote",
			"tasks.tier_not_permitted: "+id.Tier.String(), nil))
	}
	return AddNote(s.core, input.IssueID, input.Body, id.Subject) // ENFORCE-not-claim — input.Author ignored
}

// ----------------------------------------------------------------------
// Shape (a.i) IPC-entry WRAPPER — *WailsService
//
// RFC.tier-auth-substrate.md v3.1 §4.4 / Cerberus #73 F-1 / Mantis #1755.
//
// *WailsService is the type bound into the wails3 service array by
// pkg/desktop/desktop.go. Every method stamps TierRenderer onto the
// auth sidecar BEFORE delegating to the wrapped *Service method, then
// defers ClearCaller so the stamp clears on return. This is the
// "hand-written wrapper at the registration site" pattern: without
// this wrapper a Wails IPC call landing on *Service.Create would
// resolve auth.Caller(s.core) to the TierInternal floor (the desktop's
// tray surface has no pre-stamp on the sidecar today) and the existing
// Require gate would deny — substrate is correct, but the renderer
// surface is broken. The wrapper feeds the renderer-tier stamp into
// the substrate at the IPC boundary so the gate fires correctly.
// ----------------------------------------------------------------------

// WailsService is the IPC-entry wrapper around *Service. Methods
// mirror *Service exactly (same input types, same return shape) so
// the wrapper-drift reflection guard (wrapper_drift_test.go) catches
// any new method on *Service that is added without a matching wrapper
// entry here.
//
// Construct via NewWailsService(svc); register via
// application.NewService(tasks.NewWailsService(taskSvc)) in
// pkg/desktop/desktop.go.
type WailsService struct {
	svc *Service
}

// NewWailsService wraps an existing *Service for Wails IPC binding.
// The wrapper holds a pointer to the substrate, NOT a fresh copy —
// callers can keep their own *Service reference for in-Go calls and
// the wrapper continues to delegate to the same instance.
//
// Usage example:
//
//	taskSvc := tasks.NewService(c)
//	wailsSvc := tasks.NewWailsService(taskSvc)
//	application.NewService(wailsSvc)
func NewWailsService(svc *Service) *WailsService {
	return &WailsService{svc: svc}
}

// Substrate returns the wrapped *Service so tests + in-Go consumers
// can reach the substrate-tier surface (caller-stamp flows through
// unchanged). Wails3's binding generator only walks exported methods
// with primitive / struct arg shapes; the *Service return type means
// Substrate() does NOT show up as a TS binding — verify via
// wrapper-drift test which exempts it.
//
// Usage example:
//
//	wailsSvc := tasks.NewWailsService(tasks.NewService(c))
//	// Tests reach the substrate directly:
//	r := wailsSvc.Substrate().List(tasks.ListInput{Project: "lthn"})
func (w *WailsService) Substrate() *Service { return w.svc }

// stampRenderer is the Shape (a.i) IPC-entry stamp helper. Every
// Wails-bound method on *WailsService calls stampRenderer at its top
// before delegating, and defers the returned clear-fn so the sidecar
// is cleared on return. Two-step (stamp + defer clear) keeps the stamp
// scoped to the single IPC call — the next concurrent Wails call on
// the same *core.Core gets its own stamp at entry.
//
// Returns a clear-fn (suitable for defer) rather than relying on the
// caller to remember the matching ClearCaller — pairs the stamp and
// the clear at the same call site, prevents a forgotten clear leaving
// the renderer-tier stamp leaked across IPC boundaries into intra-Go
// callers that may run on the same *core.Core afterwards.
//
// nil-safe on c: if the wrapped service holds a nil *core.Core, the
// no-op stamp+clear keeps the caller path consistent (the downstream
// Require will still deny because Caller(nil) returns TierInternal —
// visible deny, not panic).
//
// Usage example (inside every Wails-bound method on *WailsService):
//
//	func (w *WailsService) Create(input CreateIssueInput) core.Result {
//	    defer stampRenderer(w.svc.core)()
//	    return w.svc.Create(input)
//	}
func stampRenderer(c *core.Core) func() {
	if c == nil {
		return func() {}
	}
	auth.SetCaller(c, auth.CallerIdentity{
		Tier:    auth.TierRenderer,
		Subject: wailsIPCSubject,
		Source:  wailsIPCSource,
	})
	return func() { auth.ClearCaller(c) }
}

// wailsIPCSubject and wailsIPCSource are the Subject/Source breadcrumb
// pair stamped onto the sidecar at every Wails IPC entry. "wails" is
// the minimum-information identifier that lets the audit trail
// distinguish renderer-via-IPC calls from renderer-via-HTTP (where
// Subject = account_id and Source = "http-session:..."). When the
// renderer envelope grows a plugin-id field (RFC v3.1 §4.4 future
// shape), Subject lifts to that value with no signature change here.
const (
	wailsIPCSubject = "wails"
	wailsIPCSource  = "wails"
)

// List — Wails IPC entry → stamp TierRenderer → delegate to substrate.
// See *Service.List for the substrate-side documentation (allow-list
// permits TierOperator + TierRenderer + TierCascade).
//
// Usage example (TS binding):
//
//	const r = await List({ state: "open" });
//	const out = r.Value as { issues: Issue[]; count: number };
func (w *WailsService) List(input ListInput) core.Result {
	defer stampRenderer(w.svc.core)()
	return w.svc.List(input)
}

// Get — Wails IPC entry → stamp TierRenderer → delegate to substrate.
// See *Service.Get for the substrate-side documentation.
//
// Usage example (TS binding):
//
//	const r = await Get({ id: "abc123" });
//	const issue = r.Value as Issue;
func (w *WailsService) Get(input GetInput) core.Result {
	defer stampRenderer(w.svc.core)()
	return w.svc.Get(input)
}

// Create — Wails IPC entry → stamp TierRenderer → delegate to substrate.
// ENFORCE-not-claim Reporter overwrite still applies; the wrapper's
// stamped Subject ("wails") becomes the Reporter when no upstream
// session has overridden it (HTTP middleware SetCaller would override
// before the IPC even reaches here, but the desktop tray IPC path has
// no such pre-stamp today — the wrapper IS the stamp source).
//
// Usage example (TS binding):
//
//	const r = await Create({ project: "lthn", summary: "wire backlog" });
//	const issue = r.Value as Issue;
func (w *WailsService) Create(input CreateIssueInput) core.Result {
	defer stampRenderer(w.svc.core)()
	return w.svc.Create(input)
}

// Update — Wails IPC entry → stamp TierRenderer → delegate to substrate.
// See *Service.Update for the substrate-side documentation.
//
// Usage example (TS binding):
//
//	const r = await Update({ id: "abc123", state: "in_progress" });
//	const updated = r.Value as Issue;
func (w *WailsService) Update(input UpdateIssueInput) core.Result {
	defer stampRenderer(w.svc.core)()
	return w.svc.Update(input)
}

// AddNote — Wails IPC entry → stamp TierRenderer → delegate to substrate.
// ENFORCE-not-claim Author overwrite still applies; the wrapper's
// stamped Subject becomes the Author when no upstream session has
// overridden it.
//
// Usage example (TS binding):
//
//	const r = await AddNote({ issue_id: "abc123", body: "shipped" });
//	const note = r.Value as Note;
func (w *WailsService) AddNote(input AddNoteInput) core.Result {
	defer stampRenderer(w.svc.core)()
	return w.svc.AddNote(input)
}

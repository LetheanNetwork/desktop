// SPDX-Licence-Identifier: EUPL-1.2

// audit.go — Stage F.B Phase 2.4 cross-cut. Subscribes to the
// tasks.IssueChanged typed bus and projects each event into the
// pkg/audit recorder via RecordBatch (cascade path — these events
// can burst high under linter / agent-prep activity per RFC §4.2).
//
// Wired from the boot path via tasks.AttachAudit(c). Lives next to
// the bus declaration so the event-name → audit-event-name mapping
// is grep-discoverable from the package the events originate in.

package tasks

import (
	core "dappco.re/go"

	"dappco.re/lthn/desktop/pkg/audit"
)

// Reserved audit event-names for the tasks bus. Declared at package
// scope so the parity-grep contract (RFC §3.3.1) finds them via
// `go/parser` AST walk — the test side reads the SAME constants the
// emit-side projects, so a typo in either fails at compile rather
// than as a runtime divergence.
const (
	AuditEventCreated      = "tasks.created"
	AuditEventTransitioned = "tasks.transitioned"
	AuditEventDeleted      = "tasks.deleted"
)

// AttachAudit subscribes the audit recorder to the IssueChanged bus.
// Idempotent across multiple calls per Core (Core's RegisterAction
// allows multiple handlers, so re-attaching multiplies emissions —
// the boot path SHOULD call AttachAudit exactly once).
//
// Usage example:
//
//	tasks.AttachAudit(c)
func AttachAudit(c *core.Core) {
	Subscribe(c, func(_ *core.Core, ev IssueChanged) {
		name := mapKindToAuditEvent(ev.Kind)
		if name == "" {
			return
		}
		_ = audit.Default().Record(audit.Event{
			Event:   name,
			Scope:   ev.Issue.Project,
			Outcome: audit.OutcomeOK,
			TS:      ev.At.Unix(),
			Meta: map[string]any{
				"issue_id":     ev.Issue.ID,
				"before_state": ev.Before.State,
				"after_state":  ev.Issue.State,
				"__emit_ts":    ev.At.UnixNano(),
			},
		})
	})
}

// mapKindToAuditEvent translates the tasks lifecycle Kind constants
// to the reserved audit event names. Unknown kinds return "" so
// future tasks-package additions don't accidentally land in the
// audit stream without a deliberate parity update here.
func mapKindToAuditEvent(kind string) string {
	switch kind {
	case KindCreated:
		return AuditEventCreated
	case KindUpdated, KindNoted:
		return AuditEventTransitioned
	case KindClosed:
		return AuditEventTransitioned
	}
	return ""
}
